package interview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"text/template"

	"github.com/cloudwego/eino-ext/components/model/agenticopenai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"

	"resumepro/backend/internal/interview/prompts"
)

// 面试图的五个节点（spec「Eino Graph 节点编排图」）：
//
//	START → ask_question ──(StatefulInterrupt 挂起，ResumeWithData 恢复)──→ grade_answer
//	      → decide_follow_up ──branch──→ ask_question（追问/下一题）
//	                                 └→ generate_review → finish → END
const (
	nodeAsk    = "ask_question"
	nodeGrade  = "grade_answer"
	nodeDecide = "decide_follow_up"
	nodeReview = "generate_review"
	nodeFinish = "finish"
)

// decide_follow_up 的两条路由（经 branch 选择）。
const (
	routeAsk    = "ask"
	routeReview = "review"
)

// ErrSessionNotFound 表示面试图会话不存在：面试尚未启动、已结束或 checkpoint 已过期。
var ErrSessionNotFound = errors.New("interview: graph session not found")

func init() {
	// InterviewState 以 any 形态存入 checkpoint（图本地状态与中断状态），
	// eino 的 InternalSerializer 要求自定义类型注册后才能序列化，否则中断报 unknown type。
	schema.RegisterName[*InterviewState]("resumepro_interview_interview_state")
}

// InterviewState 是面试图的本地状态，随 checkpoint 跨请求持久化（spec：state 是
// 进度与会话游标的唯一真源；面试整体状态由 interviews 表承载，不进 state）。
type InterviewState struct {
	// InterviewID 面试 id，首次进入 ask_question 时从图输入解析并初始化。
	InterviewID int64 `json:"interview_id"`
	// Index 当前主问题下标（指向 repo.MainQuestions 的位置，N 以实际行数为准）。
	Index int `json:"index"`
	// FollowUpRound 当前追问轮次；0 表示待答的是主问题首答，>0 时最新追问行是当前待答题。
	FollowUpRound int `json:"follow_up_round"`
	// FollowUpBudgetLeft 全场追问剩余预算（跨请求持久，创建时从 config_snapshot 初始化）。
	FollowUpBudgetLeft int `json:"follow_up_budget_left"`
	// ResponseID 当前主问题链的 Responses API 续聊游标；为空表示不传
	// WithHeadPreviousResponseID（fake 模型等无该字段的场景）。
	ResponseID string `json:"response_id,omitempty"`
}

// answerPayload 是一轮用户回答（ResumeWithData 的 data）。
type answerPayload struct {
	// Answer 用户回答原文；skip=true 时为空。
	Answer string `json:"answer"`
	// Skip 用户点了「不会 / 没补充」：不调 LLM，按结算等级处理（spec「用户跳过问题」）。
	Skip bool `json:"skip"`
}

// gradeResult 是评分节点的输出（LLM 约定 JSON 的 Go 视图）。
type gradeResult struct {
	Grade    Grade  `json:"grade"`
	FollowUp string `json:"followUp"`
}

// graphResult 是一轮回答对调用方的结果：追问/下一题（未结束）或总评（已结束）。
type graphResult struct {
	Grade         Grade     `json:"grade"`
	FollowUp      string    `json:"followUp"`
	NextQuestion  *Question `json:"nextQuestion,omitempty"`
	Finished      bool      `json:"finished"`
	OverallGrade  Grade     `json:"overallGrade,omitempty"`
	OverallReview string    `json:"overallReview,omitempty"`
}

// decisionOutput 是 decide_follow_up 节点的输出：branch 按路由选边，
// Grade/FollowUp 透传给后续节点（最终回传给调用方，让 Answer 能拼出本轮评分）。
type decisionOutput struct {
	Route    string `json:"route"`
	Grade    Grade  `json:"grade"`
	FollowUp string `json:"followUp"`
}

// reviewOutput 是 generate_review → finish 的负载，也是图的最终输出。
type reviewOutput struct {
	Grade         Grade  `json:"grade"`
	FollowUp      string `json:"followUp"`
	OverallGrade  Grade  `json:"overallGrade"`
	OverallReview string `json:"overallReview"`
}

// askQuestionInfo 是 ask_question 中断时经 InterruptCtx.Info 返回给调用方的信息
// （只在进程内传递，不进 checkpoint）。
type askQuestionInfo struct {
	// Message 问题原文（assistant 消息），前端按消息渲染。
	Message *schema.AgenticMessage
	// Question 问题行（主问题或追问行）。
	Question *Question
	// Grade 触发本轮提问的那次回答的评分（面试首启为空）。
	Grade Grade
	// FollowUp 触发本轮提问的那次回答的追问建议（面试首启为空）。
	FollowUp string
}

// Graph 是编译后的面试图：StartInterview / Answer 驱动跨请求的 interrupt/resume。
//
// Graph 可并发服务多场面试（每次 Invoke 有独立的 state 与 checkpoint）。
type Graph struct {
	runnable compose.Runnable[string, string]
	repo     *Repository
	cm       ResponsesModel
	store    compose.CheckPointStore

	mu            sync.Mutex
	lastInterrupt map[string]string // checkpointID -> 最近一次中断的 interruptID
}

// BuildGraph 构建并编译面试图。
//
// store 是编译选项（eino 裁定 F4）：生产用 *RedisStore（T9 注入），测试用内存实现。
func BuildGraph(repo *Repository, cm ResponsesModel, store compose.CheckPointStore) (*Graph, error) {
	if repo == nil {
		return nil, errors.New("interview: build graph requires a repository")
	}
	if cm == nil {
		return nil, errors.New("interview: build graph requires a responses model")
	}
	if store == nil {
		return nil, errors.New("interview: build graph requires a checkpoint store")
	}
	tmpl, err := template.ParseFS(prompts.FS, "grading.tmpl", "review.tmpl")
	if err != nil {
		return nil, fmt.Errorf("interview: parse graph prompts: %w", err)
	}

	g := compose.NewGraph[string, string](compose.WithGenLocalState(func(ctx context.Context) *InterviewState {
		return &InterviewState{}
	}))

	deps := graphDeps{repo: repo, cm: cm, store: store, tmpl: tmpl}
	if err := g.AddLambdaNode(nodeAsk, compose.InvokableLambda(deps.askQuestion)); err != nil {
		return nil, err
	}
	if err := g.AddLambdaNode(nodeGrade, compose.InvokableLambda(deps.gradeAnswer)); err != nil {
		return nil, err
	}
	if err := g.AddLambdaNode(nodeDecide, compose.InvokableLambda(deps.decideFollowUp)); err != nil {
		return nil, err
	}
	if err := g.AddLambdaNode(nodeReview, compose.InvokableLambda(deps.generateReview)); err != nil {
		return nil, err
	}
	if err := g.AddLambdaNode(nodeFinish, compose.InvokableLambda(deps.finishNode)); err != nil {
		return nil, err
	}

	if err := g.AddEdge(compose.START, nodeAsk); err != nil {
		return nil, err
	}
	if err := g.AddEdge(nodeAsk, nodeGrade); err != nil {
		return nil, err
	}
	if err := g.AddEdge(nodeGrade, nodeDecide); err != nil {
		return nil, err
	}
	// decide_follow_up 的两条出边用 branch（单节点多静态出边编译会报错）
	if err := g.AddBranch(nodeDecide, compose.NewGraphBranch[string](
		func(_ context.Context, in string) (string, error) {
			var decision decisionOutput
			if err := json.Unmarshal([]byte(in), &decision); err != nil {
				return "", fmt.Errorf("interview: route on invalid decision output: %w", err)
			}
			if decision.Route == routeReview {
				return nodeReview, nil
			}
			return nodeAsk, nil
		},
		map[string]bool{nodeAsk: true, nodeReview: true},
	)); err != nil {
		return nil, err
	}
	if err := g.AddEdge(nodeReview, nodeFinish); err != nil {
		return nil, err
	}
	if err := g.AddEdge(nodeFinish, compose.END); err != nil {
		return nil, err
	}

	runnable, err := g.Compile(context.Background(), compose.WithCheckPointStore(store))
	if err != nil {
		return nil, fmt.Errorf("interview: compile graph: %w", err)
	}
	return &Graph{
		runnable:      runnable,
		repo:          repo,
		cm:            cm,
		store:         store,
		lastInterrupt: map[string]string{},
	}, nil
}

// graphDeps 是节点方法的宿主：节点依赖的仓储/模型/存储与 prompt 模板打包在一起，
// 避免节点闭包过长。
type graphDeps struct {
	repo  *Repository
	cm    ResponsesModel
	store compose.CheckPointStore
	tmpl  *template.Template
}

// checkPointDeleter 与 eino internal/core.CheckPointDeleter 结构一致（compose 包
// 未再导出该可选接口），finish 节点用结构化断言删除 checkpoint。
type checkPointDeleter interface {
	Delete(ctx context.Context, checkPointID string) error
}

// StartInterview 启动面试图：首次 Invoke 让 ask_question 输出第一题并中断挂起，
// 返回的问题消息即前端要渲染的 assistant 消息。对已启动的面试重复调用会重新
// 中断在同一题上（读库幂等，无副作用）。
func (g *Graph) StartInterview(ctx context.Context, interviewID int64) (*schema.AgenticMessage, error) {
	if interviewID <= 0 {
		return nil, fmt.Errorf("interview: invalid interview id %d", interviewID)
	}
	checkPointID := strconv.FormatInt(interviewID, 10)

	// 首轮输入是 interview id，ask_question 用它初始化 state
	_, err := g.runnable.Invoke(ctx, checkPointID, compose.WithCheckPointID(checkPointID))
	if err != nil {
		info, existed := compose.ExtractInterruptInfo(err)
		if !existed || len(info.InterruptContexts) == 0 {
			return nil, err
		}
		ic := info.InterruptContexts[0]
		g.rememberInterrupt(checkPointID, ic.ID)
		aq, ok := ic.Info.(*askQuestionInfo)
		if !ok {
			return nil, fmt.Errorf("interview: unexpected interrupt info %T", ic.Info)
		}
		return aq.Message, nil
	}
	return nil, errors.New("interview: graph completed without asking a question")
}

// Answer 提交回答并驱动图继续：定向恢复 ask_question 的中断 → 评分 → 追问判定 →
// 下一轮提问（再次中断，返回下一题）或总评收尾（正常返回，Finished=true）。
func (g *Graph) Answer(ctx context.Context, interviewID int64, payload answerPayload) (graphResult, error) {
	if interviewID <= 0 {
		return graphResult{}, fmt.Errorf("interview: invalid interview id %d", interviewID)
	}
	checkPointID := strconv.FormatInt(interviewID, 10)

	// checkpoint 不存在：未启动、已完成或已过期，明确报错而不是静默重开一场
	if _, ok, err := g.store.Get(ctx, checkPointID); err != nil {
		return graphResult{}, err
	} else if !ok {
		return graphResult{}, ErrSessionNotFound
	}

	interruptID, ok := g.lastInterruptID(checkPointID)
	if !ok {
		// interruptID 只存在进程内存（见 rememberInterrupt 注释）：进程重启等场景
		// 丢失时，先重放一轮让 ask_question 重新中断（该节点只读库，幂等安全），
		// 拿到新的 interruptID 后再定向恢复。
		if _, err := g.runnable.Invoke(ctx, "", compose.WithCheckPointID(checkPointID)); err != nil {
			info, existed := compose.ExtractInterruptInfo(err)
			if !existed || len(info.InterruptContexts) == 0 {
				return graphResult{}, err
			}
			interruptID = info.InterruptContexts[0].ID
			g.rememberInterrupt(checkPointID, interruptID)
		} else {
			return graphResult{}, errors.New("interview: expected re-interrupt on replay, got normal completion")
		}
	}

	out, err := g.runnable.Invoke(
		compose.ResumeWithData(ctx, interruptID, payload),
		"", // 恢复轮的图输入被忽略（任务从 checkpoint 还原）
		compose.WithCheckPointID(checkPointID),
	)
	if err != nil {
		info, existed := compose.ExtractInterruptInfo(err)
		if !existed || len(info.InterruptContexts) == 0 {
			return graphResult{}, err
		}
		ic := info.InterruptContexts[0]
		g.rememberInterrupt(checkPointID, ic.ID)
		aq, ok := ic.Info.(*askQuestionInfo)
		if !ok {
			return graphResult{}, fmt.Errorf("interview: unexpected interrupt info %T", ic.Info)
		}
		return graphResult{Grade: aq.Grade, FollowUp: aq.FollowUp, NextQuestion: aq.Question}, nil
	}

	// 正常返回 = 总评收尾完成，图输出携带最后一题评分与总评
	var final reviewOutput
	if err := json.Unmarshal([]byte(out), &final); err != nil {
		return graphResult{}, fmt.Errorf("interview: invalid graph output: %w", err)
	}
	return graphResult{
		Grade:         final.Grade,
		FollowUp:      final.FollowUp,
		Finished:      true,
		OverallGrade:  final.OverallGrade,
		OverallReview: final.OverallReview,
	}, nil
}

// rememberInterrupt 记录某场面试最近一次中断的 interruptID，供下一次 Answer 定向恢复。
//
// eino 的 interruptID 是每次中断随机生成的 uuid（core.Interrupt → uuid.NewString()），
// 不是稳定的节点地址，因此必须在中断返回时捕获。它只存进程内存：重启后由 Answer 的
// 重放兜底路径重新获取。
func (g *Graph) rememberInterrupt(checkPointID, interruptID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.lastInterrupt[checkPointID] = interruptID
}

func (g *Graph) lastInterruptID(checkPointID string) (string, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	id, ok := g.lastInterrupt[checkPointID]
	return id, ok
}

// ---- ask_question ----

// askQuestion 节点：
//   - 恢复流（被 ResumeWithData 定向）：把用户回答序列化后交给 grade_answer；
//   - 全新进入（START 首启或 decide_follow_up 重入）：按 state 定位当前题，读库取题文
//     （幂等，不写库），以 StatefulInterrupt(问题消息, state) 挂起等用户回答。
func (dep graphDeps) askQuestion(ctx context.Context, in string) (string, error) {
	if wasInterrupted, _, _ := compose.GetInterruptState[*InterviewState](ctx); wasInterrupted {
		if isResume, hasData, data := compose.GetResumeContext[answerPayload](ctx); isResume && hasData {
			b, err := json.Marshal(data)
			if err != nil {
				return "", fmt.Errorf("interview: marshal answer payload: %w", err)
			}
			return string(b), nil
		}
		// 处于中断态但未被定向恢复：按 Strategy 2 必须再次中断以保持中断。
	}

	// 重入时上一个节点（decide_follow_up）的输出携带本轮评分结果，用于回传调用方；
	// 首启时图输入是 interview id 数字串，JSON 解析失败无碍（Grade/FollowUp 留空）。
	var decision decisionOutput
	_ = json.Unmarshal([]byte(in), &decision)

	var info *askQuestionInfo
	var snapshot *InterviewState
	err := compose.ProcessState[*InterviewState](ctx, func(ctx context.Context, state *InterviewState) error {
		if state.InterviewID == 0 {
			id, err := strconv.ParseInt(strings.TrimSpace(in), 10, 64)
			if err != nil || id <= 0 {
				return fmt.Errorf("interview: invalid graph input %q, want interview id", in)
			}
			iv, err := dep.repo.GetInterviewByID(ctx, id)
			if err != nil {
				return fmt.Errorf("interview: load interview %d: %w", id, err)
			}
			// 契约 §10 守卫：非 active 的面试不得（重）启图——checkpoint 已删时
			// 从 START 重跑会让随后的 Answer 从第 1 题覆写已答数据。Service 层
			// 已在入口校验 status，这里是图内的最后一道闸。
			if iv.Status != "active" {
				return fmt.Errorf("interview: interview %d is %s, not active", id, iv.Status)
			}
			var cfg InterviewConfig
			if err := json.Unmarshal([]byte(iv.ConfigSnapshot), &cfg); err != nil {
				return fmt.Errorf("interview: parse config snapshot: %w", err)
			}
			state.InterviewID = id
			state.FollowUpBudgetLeft = cfg.Resolved().FollowUpBudget
		}
		row, err := dep.currentQuestion(ctx, state)
		if err != nil {
			return err
		}
		copied := *state
		snapshot = &copied
		info = &askQuestionInfo{
			Message:  assistantAgenticMessage(row.Question),
			Question: &row,
			Grade:    decision.Grade,
			FollowUp: decision.FollowUp,
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return "", compose.StatefulInterrupt(ctx, info, snapshot)
}

// currentQuestion 定位当前待答题：FollowUpRound > 0 时是追问链的最后一行
// （decide_follow_up 刚插入的追问），否则是主问题 Index 指向的行。只读库，重入安全。
func (dep graphDeps) currentQuestion(ctx context.Context, state *InterviewState) (Question, error) {
	mains, err := dep.repo.MainQuestions(ctx, state.InterviewID)
	if err != nil {
		return Question{}, err
	}
	if state.Index >= len(mains) {
		return Question{}, fmt.Errorf("interview: question index %d out of range (%d main questions)", state.Index, len(mains))
	}
	main := mains[state.Index]
	if state.FollowUpRound <= 0 {
		return main, nil
	}
	chain, err := dep.repo.QuestionChain(ctx, state.InterviewID, main.ID)
	if err != nil {
		return Question{}, err
	}
	if len(chain) == 0 {
		return Question{}, errors.New("interview: empty question chain")
	}
	return chain[len(chain)-1], nil
}

// ---- grade_answer ----

// gradeAnswer 节点：skip=true 不调 LLM（首答结算 C / 追问中保持链最高等级）；
// 正常首答拼完整题目上下文，追问轮只传新回答 + WithHeadPreviousResponseID 续聊。
// 输出 {grade, followUp} JSON；解析失败按契约修复模式重试一次。
func (dep graphDeps) gradeAnswer(ctx context.Context, in string) (string, error) {
	var payload answerPayload
	if err := json.Unmarshal([]byte(in), &payload); err != nil {
		return "", fmt.Errorf("interview: invalid answer payload %q: %w", in, err)
	}

	var res gradeResult
	err := compose.ProcessState[*InterviewState](ctx, func(ctx context.Context, state *InterviewState) error {
		row, err := dep.currentQuestion(ctx, state)
		if err != nil {
			return err
		}
		if payload.Skip {
			res, err = dep.settleSkipGrade(ctx, state, row)
		} else {
			res, err = dep.gradeWithModel(ctx, state, row, payload)
		}
		return err
	})
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(res)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// settleSkipGrade 处理 skip：不调 LLM。主问题首答结算 C；追问中保持当前链最高等级
// （已答的不作废、父等级不降）。等级写回当前行，user_answer 保持为空
// （spec「用户跳过问题」：user_answer 记为空，不消耗追问预算）。
func (dep graphDeps) settleSkipGrade(ctx context.Context, state *InterviewState, row Question) (gradeResult, error) {
	settled := GradeC
	if state.FollowUpRound > 0 {
		if row.ParentID == nil {
			return gradeResult{}, errors.New("interview: follow-up round without parent question")
		}
		chain, err := dep.repo.QuestionChain(ctx, state.InterviewID, *row.ParentID)
		if err != nil {
			return gradeResult{}, err
		}
		for _, q := range chain {
			if ValidGrade(q.Grade) {
				settled = ClampGrade(settled, Grade(q.Grade))
			}
		}
	}
	if err := dep.repo.UpdateGrade(ctx, row.ID, string(settled)); err != nil {
		return gradeResult{}, err
	}
	return gradeResult{Grade: settled}, nil
}

// gradeWithModel 调 LLM 评分并回写当前行（user_answer + 原始评分；链级 clamp 由
// decide_follow_up 统一处理）。响应 id 从 ResponseMeta.OpenAIExtension.ID 取，
// fake 模型等没有该字段时容错为空（空则不传 WithHeadPreviousResponseID）。
func (dep graphDeps) gradeWithModel(ctx context.Context, state *InterviewState, row Question, payload answerPayload) (gradeResult, error) {
	var msgs []*schema.AgenticMessage
	if state.FollowUpRound <= 0 {
		system, err := renderPrompt(dep.tmpl, "grading.tmpl")
		if err != nil {
			return gradeResult{}, err
		}
		must, bonus := dep.repo.ExtraFields(row.Extra)
		questionCtx, err := json.Marshal(gradingContext{
			Question:       row.Question,
			Label:          labelNames[row.Label],
			StandardAnswer: row.StandardAnswer,
			MustPoints:     must,
			BonusPoints:    bonus,
		})
		if err != nil {
			return gradeResult{}, fmt.Errorf("interview: marshal grading context: %w", err)
		}
		msgs = []*schema.AgenticMessage{
			schema.SystemAgenticMessage(system),
			schema.UserAgenticMessage(string(questionCtx)),
			schema.UserAgenticMessage(payload.Answer),
		}
	} else {
		// 追问轮：题目上下文由 Responses 服务端经 previous_response_id 续上
		msgs = []*schema.AgenticMessage{schema.UserAgenticMessage(payload.Answer)}
	}

	var opts []model.Option
	if state.FollowUpRound > 0 && state.ResponseID != "" {
		opts = append(opts, agenticopenai.WithHeadPreviousResponseID(state.ResponseID))
	}

	resp, err := dep.cm.Generate(ctx, msgs, opts...)
	if err != nil {
		return gradeResult{}, fmt.Errorf("interview: grade llm call: %w", err)
	}
	content := agenticMessageText(resp)
	res, parseErr := parseGradeResult(content)
	if parseErr != nil {
		if ctx.Err() != nil {
			return gradeResult{}, ctx.Err()
		}
		retryResp, retryErr := dep.cm.Generate(ctx, repairGradeMessages(msgs, content, parseErr), opts...)
		if retryErr != nil {
			return gradeResult{}, fmt.Errorf("interview: grade llm call: %w", retryErr)
		}
		content = agenticMessageText(retryResp)
		if res, parseErr = parseGradeResult(content); parseErr != nil {
			return gradeResult{}, parseErr
		}
		resp = retryResp
	}
	if id := responseID(resp); id != "" {
		state.ResponseID = id
	}
	if err := dep.repo.UpdateAnswer(ctx, row.ID, payload.Answer, string(res.Grade)); err != nil {
		return gradeResult{}, err
	}
	return res, nil
}

// gradingContext 是主问题首答评分的用户消息负载。
type gradingContext struct {
	Question       string   `json:"question"`
	Label          string   `json:"label"`
	StandardAnswer string   `json:"standardAnswer,omitempty"`
	MustPoints     []string `json:"mustPoints,omitempty"`
	BonusPoints    []string `json:"bonusPoints,omitempty"`
}

// parseGradeResult 解析并校验评分输出：单个 JSON 对象；grade ∈ 六档；followUp 可为空串。
func parseGradeResult(content string) (gradeResult, error) {
	decoder := json.NewDecoder(strings.NewReader(content))
	var payload struct {
		Grade    string `json:"grade"`
		FollowUp string `json:"followUp"`
	}
	if err := decoder.Decode(&payload); err != nil {
		return gradeResult{}, fmt.Errorf("invalid grade JSON: %w", err)
	}
	if err := ensureJSONEnd(decoder); err != nil {
		return gradeResult{}, err
	}
	if !ValidGrade(payload.Grade) {
		return gradeResult{}, fmt.Errorf("invalid grade %q, want one of C/B-/B/B+/A/S", payload.Grade)
	}
	return gradeResult{Grade: Grade(payload.Grade), FollowUp: strings.TrimSpace(payload.FollowUp)}, nil
}

// repairGradeMessages 复用生题的契约修复模式：原消息 + 上一次无效输出 + 纠正指令。
func repairGradeMessages(messages []*schema.AgenticMessage, invalidContent string, validationErr error) []*schema.AgenticMessage {
	repaired := append([]*schema.AgenticMessage(nil), messages...)
	return append(repaired,
		assistantAgenticMessage(invalidContent),
		schema.UserAgenticMessage(fmt.Sprintf(
			"上一次输出不符合约定：%s。请只返回修正后的 JSON，不要解释或使用 markdown 包裹。",
			validationErr,
		)),
	)
}

// responseID 从 Responses 响应中取 response id；无该扩展（如 fake 模型）返回空串。
func responseID(msg *schema.AgenticMessage) string {
	if msg == nil || msg.ResponseMeta == nil || msg.ResponseMeta.OpenAIExtension == nil {
		return ""
	}
	return msg.ResponseMeta.OpenAIExtension.ID
}

// ---- decide_follow_up ----

// decideFollowUp 节点：纯规则（非 LLM）。四条件（followUp 为空 / grade≥A / 轮次到
// 单问上限 / 预算用尽）任一成立则不追问。该节点位于 ask_question 完成之后，恢复轮中
// 每次回答只执行一次，写库安全（见速查表「已知坑 3」）。
//
// 不追问分支：clamp 主问题链最终等级（只升不降）回写主问题行与最后一行，Index+1、
// 轮次归零、ResponseID 清空；Index≥N（N = 实际主问题行数）转总评，否则回提问节点。
// 追问分支：InsertFollowUp（parent_id = 主问题 id，契约 §7）、预算-1、轮次+1。
func (dep graphDeps) decideFollowUp(ctx context.Context, in string) (string, error) {
	var res gradeResult
	if err := json.Unmarshal([]byte(in), &res); err != nil {
		return "", fmt.Errorf("interview: invalid grade result %q: %w", in, err)
	}

	decision := decisionOutput{Route: routeAsk, Grade: res.Grade, FollowUp: res.FollowUp}
	err := compose.ProcessState[*InterviewState](ctx, func(ctx context.Context, state *InterviewState) error {
		iv, err := dep.repo.GetInterviewByID(ctx, state.InterviewID)
		if err != nil {
			return err
		}
		var cfg InterviewConfig
		if err := json.Unmarshal([]byte(iv.ConfigSnapshot), &cfg); err != nil {
			return fmt.Errorf("interview: parse config snapshot: %w", err)
		}
		cfg = cfg.Resolved()

		mains, err := dep.repo.MainQuestions(ctx, state.InterviewID)
		if err != nil {
			return err
		}
		if state.Index >= len(mains) {
			return fmt.Errorf("interview: question index %d out of range (%d main questions)", state.Index, len(mains))
		}
		main := mains[state.Index]

		if res.FollowUp != "" &&
			state.FollowUpRound < cfg.PerQuestionFollowUpLimit &&
			state.FollowUpBudgetLeft > 0 &&
			!IsGradeAtLeast(res.Grade, GradeA) {
			newRound := state.FollowUpRound + 1
			// 追问行的 extra 复用父题的 must/bonus（保留考察点信息；追问评分不依赖它）
			if _, err := dep.repo.InsertFollowUp(ctx, state.InterviewID, main.ID, newRound, res.FollowUp, "", main.Extra); err != nil {
				return err
			}
			state.FollowUpBudgetLeft--
			state.FollowUpRound = newRound
			return nil
		}

		// 不追问：clamp 链最终等级（只升不降），回写主问题行与最后一行
		chain, err := dep.repo.QuestionChain(ctx, state.InterviewID, main.ID)
		if err != nil {
			return err
		}
		final := GradeC
		for _, q := range chain {
			if ValidGrade(q.Grade) {
				final = ClampGrade(final, Grade(q.Grade))
			}
		}
		if !ValidGrade(string(final)) || final == GradeC && res.Grade != GradeC && ValidGrade(string(res.Grade)) {
			final = res.Grade // 兜底：链上没有任何有效等级时以本次评分为准
		}
		if err := dep.repo.UpdateGrade(ctx, main.ID, string(final)); err != nil {
			return err
		}
		if last := chain[len(chain)-1]; last.ID != main.ID {
			if err := dep.repo.UpdateGrade(ctx, last.ID, string(final)); err != nil {
				return err
			}
		}
		state.Index++
		state.FollowUpRound = 0
		state.ResponseID = ""
		if state.Index >= len(mains) {
			decision.Route = routeReview
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(decision)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// ---- generate_review ----

// reviewQuestion 是总评用户消息里的单题负载。
type reviewQuestion struct {
	Label      string `json:"label"`
	Question   string `json:"question"`
	Grade      string `json:"grade"`
	UserAnswer string `json:"userAnswer"`
}

// generateReview 节点：读全部主问题（含最终等级与答案）→ LLM 生成 overall_review 文本；
// overall_grade 由服务端 ComputeOverallGrade 计算（中位数定档 + 比例门槛），不让 LLM 定。
func (dep graphDeps) generateReview(ctx context.Context, in string) (string, error) {
	var decision decisionOutput
	if err := json.Unmarshal([]byte(in), &decision); err != nil {
		return "", fmt.Errorf("interview: invalid decision output %q: %w", in, err)
	}

	var out reviewOutput
	err := compose.ProcessState[*InterviewState](ctx, func(ctx context.Context, state *InterviewState) error {
		mains, err := dep.repo.MainQuestions(ctx, state.InterviewID)
		if err != nil {
			return err
		}
		if len(mains) == 0 {
			return errors.New("interview: no main questions to review")
		}
		grades := make([]Grade, 0, len(mains))
		rows := make([]reviewQuestion, 0, len(mains))
		for _, q := range mains {
			grades = append(grades, Grade(q.Grade))
			rows = append(rows, reviewQuestion{
				Label:      labelNames[q.Label],
				Question:   q.Question,
				Grade:      q.Grade,
				UserAnswer: q.UserAnswer,
			})
		}

		system, err := renderPrompt(dep.tmpl, "review.tmpl")
		if err != nil {
			return err
		}
		user, err := json.Marshal(rows)
		if err != nil {
			return fmt.Errorf("interview: marshal review questions: %w", err)
		}
		resp, err := dep.cm.Generate(ctx, []*schema.AgenticMessage{
			schema.SystemAgenticMessage(system),
			schema.UserAgenticMessage(string(user)),
		})
		if err != nil {
			return fmt.Errorf("interview: review llm call: %w", err)
		}
		review := strings.TrimSpace(agenticMessageText(resp))
		if review == "" {
			return errors.New("interview: empty overall review from model")
		}

		out = reviewOutput{
			Grade:         decision.Grade,
			FollowUp:      decision.FollowUp,
			OverallGrade:  ComputeOverallGrade(grades),
			OverallReview: review,
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// ---- finish ----

// finishNode 节点：status=completed 落库（总评等级 + 总评文本）→ 删除 checkpoint
// （store 实现 CheckPointDeleter 时）→ 输出透传总评结果作为图输出。
func (dep graphDeps) finishNode(ctx context.Context, in string) (string, error) {
	var out reviewOutput
	if err := json.Unmarshal([]byte(in), &out); err != nil {
		return "", fmt.Errorf("interview: invalid review output %q: %w", in, err)
	}
	var interviewID int64
	if err := compose.ProcessState[*InterviewState](ctx, func(_ context.Context, state *InterviewState) error {
		interviewID = state.InterviewID
		return nil
	}); err != nil {
		return "", err
	}

	grade, review := string(out.OverallGrade), out.OverallReview
	if err := dep.repo.UpdateInterviewStatus(ctx, interviewID, "completed", &grade, &review); err != nil {
		return "", fmt.Errorf("interview: mark interview %d completed: %w", interviewID, err)
	}
	if deleter, ok := dep.store.(checkPointDeleter); ok {
		if err := deleter.Delete(ctx, strconv.FormatInt(interviewID, 10)); err != nil {
			return "", fmt.Errorf("interview: delete checkpoint: %w", err)
		}
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// renderPrompt 渲染 prompts 目录下的模板（grading/review 模板无字段）。
func renderPrompt(tmpl *template.Template, name string) (string, error) {
	var sb strings.Builder
	if err := tmpl.ExecuteTemplate(&sb, name, nil); err != nil {
		return "", fmt.Errorf("interview: render prompt %s: %w", name, err)
	}
	return sb.String(), nil
}
