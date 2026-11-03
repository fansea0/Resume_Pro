package interview

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// graphTestConfig 是图测试用的小配置：2 道主问题、追问预算 1、单问上限 1。
// 实际主问题数以落库行为准（见接口契约 §6），这里保持与传入题数一致。
func graphTestConfig() InterviewConfig {
	return InterviewConfig{
		QuestionCount:            2,
		LabelCounts:              map[QuestionLabel]int{LabelOctopus: 1, LabelProject: 1},
		FollowUpBudget:           1,
		PerQuestionFollowUpLimit: 1,
	}
}

// graphQuestions 造 2 道带真实题面的主问题（接口契约 §2：图测试必须用
// CreateInterviewWithQuestions 落真实题面，否则 ask_question 读到空题面）。
func graphQuestions() []MainQuestionInput {
	return []MainQuestionInput{
		{
			Label:          LabelOctopus,
			Question:       "Go 的 channel 底层结构？",
			StandardAnswer: "hchan 结构",
			MustPoints:     []string{"hchan"},
			BonusPoints:    []string{"sendq/recvq"},
		},
		{
			Label:       LabelProject,
			Question:    "项目里的缓存怎么做的？",
			MustPoints:  []string{"场景"},
			BonusPoints: []string{"布隆过滤器"},
		},
	}
}

// gradeJSON 构造评分节点约定的 LLM 输出 {"grade":"...","followUp":"..."}。
func gradeJSON(grade, followUp string) string {
	b, _ := json.Marshal(map[string]string{"grade": grade, "followUp": followUp})
	return string(b)
}

// graphTestStore 是图测试用的内存 checkpoint store：零网络依赖（不用 Redis），
// 实现 compose.CheckPointStore 三方法 + CheckPointDeleter.Delete。
type graphTestStore struct {
	mu sync.Mutex
	m  map[string][]byte
}

func newGraphTestStore() *graphTestStore {
	return &graphTestStore{m: map[string][]byte{}}
}

func (s *graphTestStore) Get(_ context.Context, checkPointID string) ([]byte, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[checkPointID]
	return v, ok, nil
}

func (s *graphTestStore) Set(_ context.Context, checkPointID string, cp []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[checkPointID] = cp
	return nil
}

func (s *graphTestStore) Delete(_ context.Context, checkPointID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, checkPointID)
	return nil
}

// startTwoQuestionInterview 造数并启动面试图，返回第一题。
func startTwoQuestionInterview(t *testing.T, cfg InterviewConfig, fake *fakeResponses) (*Repository, *Interview, *Graph, *graphTestStore) {
	t.Helper()
	repo, db := newTestRepo(t)
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	iv, err := repo.CreateInterviewWithQuestions(ctx, 1, cfg, 1, 1, graphQuestions())
	if err != nil {
		t.Fatalf("create interview: %v", err)
	}
	store := newGraphTestStore()
	graph, err := BuildGraph(repo, fake, store)
	if err != nil {
		t.Fatalf("build graph: %v", err)
	}
	first, err := graph.StartInterview(ctx, iv.ID)
	if err != nil {
		t.Fatalf("start interview: %v", err)
	}
	if text := agenticMessageText(first); !strings.Contains(text, "channel 底层结构") {
		t.Fatalf("first question = %q, want channel question", text)
	}
	return repo, iv, graph, store
}

// mainQuestion 读回第 i 道主问题行。
func mainQuestion(t *testing.T, repo *Repository, interviewID int64, i int) Question {
	t.Helper()
	qs, err := repo.MainQuestions(context.Background(), interviewID)
	if err != nil {
		t.Fatalf("main questions: %v", err)
	}
	if i >= len(qs) {
		t.Fatalf("main question %d not found, only %d", i, len(qs))
	}
	return qs[i]
}

// TestGraphSingleTurn 单轮：B+ 无追问 → 推进到第二题、未结束。
func TestGraphSingleTurn(t *testing.T) {
	fake := &fakeResponses{payloads: []string{gradeJSON("B+", "")}}
	repo, iv, graph, _ := startTwoQuestionInterview(t, graphTestConfig(), fake)

	res, err := graph.Answer(context.Background(), iv.ID, answerPayload{Answer: "channel 是 hchan"})
	if err != nil {
		t.Fatalf("answer: %v", err)
	}
	if res.Finished || res.NextQuestion == nil {
		t.Fatalf("expected next question, got %+v", res)
	}
	if res.Grade != GradeBPlus {
		t.Errorf("grade = %s, want B+", res.Grade)
	}
	if want := graphQuestions()[1].Question; res.NextQuestion.Question != want {
		t.Errorf("next question = %q, want %q", res.NextQuestion.Question, want)
	}
	if res.NextQuestion.ParentID != nil {
		t.Errorf("next question should be a main question, got follow-up %+v", res.NextQuestion)
	}
	// 首答的答案与评分已落主问题行
	q := mainQuestion(t, repo, iv.ID, 0)
	if q.UserAnswer != "channel 是 hchan" || q.Grade != "B+" {
		t.Errorf("main[0] = answer %q grade %q, want written", q.UserAnswer, q.Grade)
	}
}

// TestGraphFollowUpChain 追问链：B+ 带追问 → 返回追问 → 再答 → 推进到第二题。
func TestGraphFollowUpChain(t *testing.T) {
	fake := &fakeResponses{payloads: []string{
		gradeJSON("B", "为什么 hchan 需要 ring buffer？"),
		gradeJSON("B+", ""),
	}}
	repo, iv, graph, _ := startTwoQuestionInterview(t, graphTestConfig(), fake)
	ctx := context.Background()

	res, err := graph.Answer(ctx, iv.ID, answerPayload{Answer: "channel 是 hchan"})
	if err != nil {
		t.Fatalf("answer 1: %v", err)
	}
	if res.Finished || res.NextQuestion == nil {
		t.Fatalf("expected follow-up question, got %+v", res)
	}
	if res.Grade != GradeB {
		t.Errorf("grade = %s, want B", res.Grade)
	}
	if res.FollowUp != "为什么 hchan 需要 ring buffer？" {
		t.Errorf("followUp = %q", res.FollowUp)
	}
	if res.NextQuestion.ParentID == nil || res.NextQuestion.FollowUpRound != 1 {
		t.Fatalf("next question should be follow-up round 1, got %+v", res.NextQuestion)
	}
	if res.NextQuestion.Question != "为什么 hchan 需要 ring buffer？" {
		t.Errorf("follow-up text = %q", res.NextQuestion.Question)
	}
	// 追问行已挂在主问题下（契约 §7：parent_id = 主问题 id）
	chain, err := repo.QuestionChain(ctx, iv.ID, mainQuestion(t, repo, iv.ID, 0).ID)
	if err != nil {
		t.Fatalf("chain: %v", err)
	}
	if len(chain) != 2 {
		t.Fatalf("chain length = %d, want 2", len(chain))
	}

	res2, err := graph.Answer(ctx, iv.ID, answerPayload{Answer: "ring buffer 用于缓存发送与接收"})
	if err != nil {
		t.Fatalf("answer 2: %v", err)
	}
	if res2.Finished || res2.NextQuestion == nil {
		t.Fatalf("expected next main question, got %+v", res2)
	}
	if res2.Grade != GradeBPlus {
		t.Errorf("grade = %s, want B+", res2.Grade)
	}
	if res2.NextQuestion.ParentID != nil {
		t.Errorf("next question should be main question 2, got %+v", res2.NextQuestion)
	}
	// 追问轮只传新回答（1 条 user 消息），不重拼题目上下文
	if len(fake.inputs[1]) != 1 {
		t.Errorf("follow-up grade input messages = %d, want 1", len(fake.inputs[1]))
	}
}

// TestGraphSkipFirstAnswerNoLLMCall 首答 skip：结算 C、不调 LLM、推进到下一题。
func TestGraphSkipFirstAnswerNoLLMCall(t *testing.T) {
	fake := &fakeResponses{payloads: []string{""}}
	repo, iv, graph, _ := startTwoQuestionInterview(t, graphTestConfig(), fake)

	res, err := graph.Answer(context.Background(), iv.ID, answerPayload{Skip: true})
	if err != nil {
		t.Fatalf("skip answer: %v", err)
	}
	if res.Grade != GradeC {
		t.Errorf("grade = %s, want C", res.Grade)
	}
	if res.FollowUp != "" {
		t.Errorf("followUp = %q, want empty", res.FollowUp)
	}
	if res.Finished || res.NextQuestion == nil || res.NextQuestion.ParentID != nil {
		t.Fatalf("expected next main question, got %+v", res)
	}
	if fake.calls != 0 {
		t.Errorf("llm calls = %d, want 0 (skip 不调 LLM)", fake.calls)
	}
	q := mainQuestion(t, repo, iv.ID, 0)
	if q.Grade != "C" {
		t.Errorf("main[0].grade = %q, want C", q.Grade)
	}
	if q.UserAnswer != "" {
		t.Errorf("main[0].user_answer = %q, want empty（skip 不写答案）", q.UserAnswer)
	}
}

// TestGraphSkipDuringFollowUpKeepsParentGrade 追问中 skip：保持父等级不降。
func TestGraphSkipDuringFollowUpKeepsParentGrade(t *testing.T) {
	fake := &fakeResponses{payloads: []string{gradeJSON("B+", "追问一下？")}}
	repo, iv, graph, _ := startTwoQuestionInterview(t, graphTestConfig(), fake)
	ctx := context.Background()

	if _, err := graph.Answer(ctx, iv.ID, answerPayload{Answer: "很好的回答"}); err != nil {
		t.Fatalf("answer 1: %v", err)
	}
	res, err := graph.Answer(ctx, iv.ID, answerPayload{Skip: true})
	if err != nil {
		t.Fatalf("skip answer: %v", err)
	}
	if res.Grade != GradeBPlus {
		t.Errorf("grade = %s, want B+（保持父等级）", res.Grade)
	}
	if res.Finished || res.NextQuestion == nil || res.NextQuestion.ParentID != nil {
		t.Fatalf("expected next main question, got %+v", res)
	}
	if fake.calls != 1 {
		t.Errorf("llm calls = %d, want 1", fake.calls)
	}
	if q := mainQuestion(t, repo, iv.ID, 0); q.Grade != "B+" {
		t.Errorf("main[0].grade = %q, want B+", q.Grade)
	}
}

// TestGraphFollowUpBudgetExhausted 预算用尽：即使模型想追问也不再追问。
func TestGraphFollowUpBudgetExhausted(t *testing.T) {
	fake := &fakeResponses{payloads: []string{
		gradeJSON("B", "追问 1？"),
		gradeJSON("B+", "还想再追问？"),
	}}
	_, iv, graph, _ := startTwoQuestionInterview(t, graphTestConfig(), fake)
	ctx := context.Background()

	if _, err := graph.Answer(ctx, iv.ID, answerPayload{Answer: "回答 1"}); err != nil {
		t.Fatalf("answer 1: %v", err)
	}
	res, err := graph.Answer(ctx, iv.ID, answerPayload{Answer: "回答 2"})
	if err != nil {
		t.Fatalf("answer 2: %v", err)
	}
	if res.Finished || res.NextQuestion == nil {
		t.Fatalf("expected next question, got %+v", res)
	}
	if res.NextQuestion.ParentID != nil {
		t.Errorf("budget exhausted: next should be main question 2, got follow-up %+v", res.NextQuestion)
	}
	if want := graphQuestions()[1].Question; res.NextQuestion.Question != want {
		t.Errorf("next question = %q, want %q", res.NextQuestion.Question, want)
	}
}

// TestGraphClampOnlyRaises 追问轮返回低等级：主问题行等级只升不降。
func TestGraphClampOnlyRaises(t *testing.T) {
	fake := &fakeResponses{payloads: []string{
		gradeJSON("B+", "追问一下？"),
		gradeJSON("B", ""),
	}}
	repo, iv, graph, _ := startTwoQuestionInterview(t, graphTestConfig(), fake)
	ctx := context.Background()

	if _, err := graph.Answer(ctx, iv.ID, answerPayload{Answer: "回答 1"}); err != nil {
		t.Fatalf("answer 1: %v", err)
	}
	if _, err := graph.Answer(ctx, iv.ID, answerPayload{Answer: "回答 2"}); err != nil {
		t.Fatalf("answer 2: %v", err)
	}
	q := mainQuestion(t, repo, iv.ID, 0)
	if q.Grade != "B+" {
		t.Errorf("main[0].grade = %q, want B+（clamp 只升不降）", q.Grade)
	}
	chain, err := repo.QuestionChain(ctx, iv.ID, q.ID)
	if err != nil {
		t.Fatalf("chain: %v", err)
	}
	if chain[len(chain)-1].Grade != "B+" {
		t.Errorf("follow-up row grade = %q, want B+（回写链最终等级）", chain[len(chain)-1].Grade)
	}
}

// TestGraphFinishLastQuestion 最后一题答完：Finished + 总评落库 + checkpoint 删除。
func TestGraphFinishLastQuestion(t *testing.T) {
	reviewText := "整体评价：候选人基础扎实，项目 depth 一般。"
	fake := &fakeResponses{payloads: []string{
		gradeJSON("B+", ""),
		gradeJSON("B+", ""),
		reviewText,
	}}
	repo, iv, graph, store := startTwoQuestionInterview(t, graphTestConfig(), fake)
	ctx := context.Background()

	res1, err := graph.Answer(ctx, iv.ID, answerPayload{Answer: "回答 1"})
	if err != nil {
		t.Fatalf("answer 1: %v", err)
	}
	if res1.Finished {
		t.Fatal("interview should not finish after question 1")
	}
	res2, err := graph.Answer(ctx, iv.ID, answerPayload{Answer: "回答 2"})
	if err != nil {
		t.Fatalf("answer 2: %v", err)
	}
	if !res2.Finished {
		t.Fatalf("expected finished, got %+v", res2)
	}
	if res2.NextQuestion != nil {
		t.Errorf("next question = %+v, want nil", res2.NextQuestion)
	}
	if res2.Grade != GradeBPlus {
		t.Errorf("last grade = %s, want B+", res2.Grade)
	}
	if res2.OverallGrade != GradeBPlus {
		t.Errorf("overall grade = %s, want B+（服务端 ComputeOverallGrade）", res2.OverallGrade)
	}
	if res2.OverallReview != reviewText {
		t.Errorf("overall review = %q, want %q", res2.OverallReview, reviewText)
	}
	// 落库断言
	got, err := repo.GetInterview(ctx, 1, iv.ID)
	if err != nil {
		t.Fatalf("get interview: %v", err)
	}
	if got.Status != "completed" {
		t.Errorf("status = %s, want completed", got.Status)
	}
	if got.OverallGrade == nil || *got.OverallGrade != string(GradeBPlus) {
		t.Errorf("overall_grade = %v, want B+", got.OverallGrade)
	}
	if got.OverallReview == nil || *got.OverallReview != reviewText {
		t.Errorf("overall_review = %v, want %q", got.OverallReview, reviewText)
	}
	if got.CompletedAt == nil {
		t.Error("completed_at = nil, want set")
	}
	// checkpoint 已删除（内存 store Get 返回 miss）
	if _, ok, err := store.Get(ctx, strconv.FormatInt(iv.ID, 10)); err != nil || ok {
		t.Fatalf("checkpoint should be deleted, ok = %v err = %v", ok, err)
	}
}

// TestGraphGradeRepairsOnce 评分输出非法 JSON：按契约修复模式重试一次。
func TestGraphGradeRepairsOnce(t *testing.T) {
	fake := &fakeResponses{payloads: []string{
		"这不是 JSON",
		gradeJSON("B", ""),
	}}
	_, iv, graph, _ := startTwoQuestionInterview(t, graphTestConfig(), fake)

	res, err := graph.Answer(context.Background(), iv.ID, answerPayload{Answer: "回答"})
	if err != nil {
		t.Fatalf("answer: %v", err)
	}
	if res.Grade != GradeB {
		t.Errorf("grade = %s, want B（修复重试后）", res.Grade)
	}
	if fake.calls != 2 {
		t.Errorf("llm calls = %d, want 2", fake.calls)
	}
}

// TestGraphAnswerWithoutSession 未启动面试就 Answer：返回明确错误。
func TestGraphAnswerWithoutSession(t *testing.T) {
	repo, db := newTestRepo(t)
	defer db.Close()
	ctx := context.Background()
	iv, err := repo.CreateInterviewWithQuestions(ctx, 1, graphTestConfig(), 1, 1, graphQuestions())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	graph, err := BuildGraph(repo, &fakeResponses{}, newGraphTestStore())
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if _, err := graph.Answer(ctx, iv.ID, answerPayload{Answer: "hi"}); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("err = %v, want ErrSessionNotFound", err)
	}
}

// TestGraphStartInterviewRejectsEndedInterview 契约 §10 守卫：对已完成/已终止的面试
// StartInterview 必须报错，而不是整场从 START 重启图（否则随后的 Answer 会从第 1 题
// 覆写已答数据）。
func TestGraphStartInterviewRejectsEndedInterview(t *testing.T) {
	repo, db := newTestRepo(t)
	defer db.Close()
	ctx := context.Background()
	iv, err := repo.CreateInterviewWithQuestions(ctx, 1, graphTestConfig(), 1, 1, graphQuestions())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := repo.UpdateInterviewStatus(ctx, iv.ID, "completed", nil, nil); err != nil {
		t.Fatalf("mark completed: %v", err)
	}
	graph, err := BuildGraph(repo, &fakeResponses{payloads: []string{gradeJSON("A", "")}}, newGraphTestStore())
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if _, err := graph.StartInterview(ctx, iv.ID); err == nil {
		t.Fatal("StartInterview on a completed interview should fail")
	}
}

// TestGraphAnswerAfterGraphRebuild 进程重启（Graph 实例重建、interruptID 丢失）后
// Answer：先重放让 ask_question 重新中断拿到新 interruptID，再定向恢复评分推进。
func TestGraphAnswerAfterGraphRebuild(t *testing.T) {
	fake := &fakeResponses{payloads: []string{gradeJSON("B+", "")}}
	repo, db := newTestRepo(t)
	defer db.Close()
	ctx := context.Background()
	iv, err := repo.CreateInterviewWithQuestions(ctx, 1, graphTestConfig(), 1, 1, graphQuestions())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	store := newGraphTestStore()
	first, err := BuildGraph(repo, &fakeResponses{}, store)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if _, err := first.StartInterview(ctx, iv.ID); err != nil {
		t.Fatalf("start: %v", err)
	}

	// 模拟重启：新建 Graph（共享 repo 与 store），内存中的 interruptID 为空
	rebuilt, err := BuildGraph(repo, fake, store)
	if err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	res, err := rebuilt.Answer(ctx, iv.ID, answerPayload{Answer: "channel 是 hchan"})
	if err != nil {
		t.Fatalf("answer after rebuild: %v", err)
	}
	if res.Finished || res.NextQuestion == nil || res.Grade != GradeBPlus {
		t.Fatalf("res = %+v, want next question with grade B+", res)
	}
	if fake.calls != 1 {
		t.Errorf("llm calls = %d, want 1", fake.calls)
	}
}
