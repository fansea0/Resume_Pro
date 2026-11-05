package interview

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"resumepro/backend/internal/logging"
)

// Service 编排 AI 模拟面试的用例层：承上（repo 持久化 / graph 面试图 / gen 生题 /
// RedisStore 锁与 checkpoint），启下（T9 HTTP handler 只与本层对话）。
//
// 职责边界：
//   - 租户隔离在本层校验（契约 §4：repo 的写方法不带 tenant 参数，调用前先
//     GetInterview(tenantID, id) 确认归属）；
//   - resumeText 与 jd（目标岗位上下文）由调用方（T9 handler）分别从简历 JSON 与
//     target_jobs 行提取后传入，本层不 import resume / targetjob 包；
//   - 生题结果 → MainQuestionInput 的转换在本层完成（契约 §8），must/bonus 点由
//     repository 的 nullableExtra 序列化为 extra 列的 {"must_points":[],"bonus_points":[]}。
type Service struct {
	repo  *Repository
	store *RedisStore
	gen   *QuestionGenerator
	graph *Graph
	// audio 是可选的语音能力；nil 表示语音缺席（纯文字模式），面试照常工作。
	audio AudioHooks
}

// ServiceOption 是 Service 的可选配置。
type ServiceOption func(*Service)

// WithAudioHooks 注入语音能力。
// 不注入时语音链路整体缺席，面试照常工作（纯文字模式）。
// 传入 nil 是安全的：等价于不启用语音。
func WithAudioHooks(hooks AudioHooks) ServiceOption {
	return func(s *Service) {
		s.audio = hooks
	}
}

// NewService 创建 Service。
func NewService(repo *Repository, store *RedisStore, gen *QuestionGenerator, graph *Graph, options ...ServiceOption) *Service {
	service := &Service{repo: repo, store: store, gen: gen, graph: graph}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

var (
	// ErrAlreadyEnded 表示面试已结束（completed/aborted）：answer/abort 拒绝（409 语义）。
	ErrAlreadyEnded = errors.New("interview already ended")
	// ErrConcurrentAnswer 表示同一面试已有一次回答在处理中（409 语义）。
	ErrConcurrentAnswer = errors.New("interview is being processed, retry later")
	// ErrSessionExpired 表示图会话不存在或已过期：checkpoint 缺失（410 语义），
	// 已答数据仍在 SQLite 可回看。graph.ErrSessionNotFound 在本层映射为该错误。
	ErrSessionExpired = errors.New("interview session expired, start a new interview")
)

// answerLockTTLSeconds 是单次 answer 的并发锁 TTL（plan 定值 60s）。
//
// 单题最坏路径 = 评分调用（+契约修复重试一次）+ 可能的总评调用，约 3 次 LLM 往返；
// 常规 provider 单次调用秒级，60s 足够。若未来 provider 显著变慢导致锁过期，
// 退化行为是「过期后重复提交可能绕过防重」（spec 明文接受的取舍），且锁内的
// status=active 复检仍是一道闸，不会出现评分错乱级别的破坏。
const answerLockTTLSeconds = 60

// CreateInterview 创建一场面试：默认值合并 → LLM 生题（JD + 简历 + 配置）→ 事务落库
// → 启动面试图（首题挂起）。
//
// jd 是目标岗位上下文（JDContext），由调用方（T9 handler）从 target_jobs 行提取
// Company/Position/Requirements 构造；可为 nil（生题器按空 JD 兜底），但传入后生成
// 的题目才贴合目标岗位（spec「生题」：生题输入 = JD + 简历 + 配置）。
//
// cfg 为 nil 时用 DefaultInterviewConfig.Resolved()；非 nil 时用 cfg.Resolved()
// （FollowUpBudget<0 同样解析为 N/2）。同租户已有 active 面试返回 ErrActiveInterviewExists。
//
// 成功后图已挂起在第一题上；第一题通过 FirstQuestion 获取（保持契约 §1 的
// (*Interview, error) 签名，见 FirstQuestion 注释）。
func (s *Service) CreateInterview(ctx context.Context, tenantID, targetJobID, resumeID int64, resumeText string, jd *JDContext, cfg *InterviewConfig) (*Interview, error) {
	resolved := DefaultInterviewConfig.Resolved()
	if cfg != nil {
		resolved = cfg.Resolved()
	}

	// 生题前预检 active：事务里的检查要等 LLM 跑完才生效，注定失败的创建会白烧十几秒
	// 和一次 token 开销。落库事务内的检查保留，它负责拦下并发创建。
	hasActive, err := s.repo.HasActiveInterview(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("interview: check active interview: %w", err)
	}
	if hasActive {
		return nil, ErrActiveInterviewExists
	}

	generated, err := s.gen.GenerateQuestions(ctx, resumeText, jd, resolved)
	if err != nil {
		return nil, fmt.Errorf("interview: generate questions: %w", err)
	}

	inputs := make([]MainQuestionInput, 0, len(generated))
	for _, q := range generated {
		inputs = append(inputs, MainQuestionInput{
			Label:          q.Label,
			Question:       q.Question,
			StandardAnswer: q.StandardAnswer,
			MustPoints:     q.MustPoints,
			BonusPoints:    q.BonusPoints,
		})
	}

	// 契约 §1：必须走 CreateInterviewWithQuestions 落真实题面；
	// 禁止 repo.CreateInterview（题面为空的占位行会锁死该租户再次创建）。
	iv, err := s.repo.CreateInterviewWithQuestions(ctx, tenantID, resolved, targetJobID, resumeID, inputs)
	if err != nil {
		return nil, err
	}

	// 启动图：ask_question 读出第一题并中断挂起。新面试 status=active，天然满足
	// 契约 §10 的「只在刚创建的面试上调用 StartInterview」。
	if _, err := s.graph.StartInterview(ctx, iv.ID); err != nil {
		// 启动失败不留 active 僵尸面试（会锁死该租户再次创建）：尽力标记 aborted。
		if abortErr := s.repo.UpdateInterviewStatus(ctx, iv.ID, "aborted", nil, nil); abortErr != nil {
			return nil, fmt.Errorf("interview: start graph: %v (abort cleanup failed: %v)", err, abortErr)
		}
		return nil, fmt.Errorf("interview: start graph: %w", err)
	}

	// 题目语音在后台生成：面试主链路不等语音（语音不可用也不影响创建）。
	// 读取主问题失败只记日志：此时面试已创建成功，不能因为语音链路把创建结果变成失败
	// （设计文档「降级（关键）」：CreateInterview 仍返回 201）。
	if s.audio != nil {
		mains, err := s.repo.MainQuestions(ctx, iv.ID)
		if err != nil {
			logging.WithError(err).WithFields(logging.Fields{"interview_id": iv.ID}).Warn("interview: load main questions for audio")
		}
		for _, q := range mains {
			s.audio.Enqueue(ctx, tenantID, iv.ID, q.ID, q.Question)
		}
	}
	return iv, nil
}

// FirstQuestion 返回面试的第一道主问题（创建响应的「面试 + 第一题」里的第一题）。
//
// 设计裁定：不扩展 CreateInterview 的返回值（契约 §1 签名固定为 (*Interview, error)），
// 单独暴露本方法。新面试的图状态恒为 Index=0、FollowUpRound=0，ask_question 定位的
// 当前题就是 MainQuestions[0]，因此这里读库返回的首题与图挂起的题目一致。
func (s *Service) FirstQuestion(ctx context.Context, tenantID, interviewID int64) (*Question, error) {
	if _, err := s.repo.GetInterview(ctx, tenantID, interviewID); err != nil {
		return nil, err
	}
	mains, err := s.repo.MainQuestions(ctx, interviewID)
	if err != nil {
		return nil, err
	}
	if len(mains) == 0 {
		return nil, fmt.Errorf("interview: interview %d has no main questions", interviewID)
	}
	return &mains[0], nil
}

// Answer 提交一轮回答并驱动面试图：校验 → 并发锁 → 恢复图运行。
//
// 顺序（契约 §9/§10）：
//  1. GetInterview 校验存在 + 归属 + status=active（否则 ErrNotFound / ErrAlreadyEnded）；
//  2. TryLock(面试 id, 60s)，抢不到 → ErrConcurrentAnswer（只有抢到锁才 defer Unlock）；
//  3. 锁内复检 status=active：预检与拿锁之间面试可能已被 abort（abort 不抢答题锁），
//     这也是契约 §9 要求的「调 UpdateAnswer 前校验 active」第一道防线；
//  4. graph.Answer。checkpoint 缺失（未启动/已过期/被删）映射为 ErrSessionExpired。
func (s *Service) Answer(ctx context.Context, tenantID, interviewID int64, payload answerPayload) (graphResult, error) {
	if _, err := s.getActiveInterview(ctx, tenantID, interviewID); err != nil {
		return graphResult{}, err
	}

	lockKey := strconv.FormatInt(interviewID, 10)
	locked, err := s.store.TryLock(ctx, lockKey, answerLockTTLSeconds)
	if err != nil {
		return graphResult{}, fmt.Errorf("interview: acquire answer lock: %w", err)
	}
	if !locked {
		return graphResult{}, ErrConcurrentAnswer
	}
	defer func() { _ = s.store.Unlock(ctx, lockKey) }()

	if _, err := s.getActiveInterview(ctx, tenantID, interviewID); err != nil {
		return graphResult{}, err
	}

	res, err := s.graph.Answer(ctx, interviewID, payload)
	if err != nil {
		// graph.ErrSessionNotFound（导出哨兵，graph.go）：checkpoint 缺失 = 会话过期
		if errors.Is(err, ErrSessionNotFound) {
			return graphResult{}, ErrSessionExpired
		}
		return graphResult{}, err
	}

	// 下一题的语音在后台生成；面试结束时清理本场音频。
	// 语音链路的问题不影响答题结果（设计文档「清理失败不影响答题返回值」）。
	if s.audio != nil {
		if res.Finished {
			s.audio.CleanupInterview(ctx, tenantID, interviewID)
		} else if res.NextQuestion != nil {
			s.audio.Enqueue(ctx, tenantID, interviewID, res.NextQuestion.ID, res.NextQuestion.Question)
		}
	}
	return res, nil
}

// Abort 终止面试：校验存在 + 归属 + active → 置 aborted（completed_at 落库，保留
// 已答数据、不生成总评）→ 删除 Redis checkpoint（之后无法再 resume/answer）。
func (s *Service) Abort(ctx context.Context, tenantID, interviewID int64) error {
	if _, err := s.getActiveInterview(ctx, tenantID, interviewID); err != nil {
		return err
	}
	if err := s.repo.UpdateInterviewStatus(ctx, interviewID, "aborted", nil, nil); err != nil {
		return fmt.Errorf("interview: abort %d: %w", interviewID, err)
	}
	if err := s.store.Delete(ctx, strconv.FormatInt(interviewID, 10)); err != nil {
		return fmt.Errorf("interview: delete checkpoint %d: %w", interviewID, err)
	}
	// 面试终态清理本场音频：清理失败只影响语音，不影响面试终态。
	if s.audio != nil {
		s.audio.CleanupInterview(ctx, tenantID, interviewID)
	}
	return nil
}

// List 按更新时间倒序列出租户的面试。
func (s *Service) List(ctx context.Context, tenantID int64) ([]Interview, error) {
	return s.repo.ListInterviews(ctx, tenantID)
}

// Get 读取租户内一场面试的详情；不存在（或不属于该租户）返回 ErrNotFound。
func (s *Service) Get(ctx context.Context, tenantID, id int64) (*Interview, error) {
	return s.repo.GetInterview(ctx, tenantID, id)
}

// Questions 返回面试的全部题目（回看）：主问题 + 各自追问链平铺，
// 主问题在前、追问紧跟父题，整体按主问题 id 升序。
func (s *Service) Questions(ctx context.Context, tenantID, id int64) ([]Question, error) {
	if _, err := s.repo.GetInterview(ctx, tenantID, id); err != nil {
		return nil, err
	}
	mains, err := s.repo.MainQuestions(ctx, id)
	if err != nil {
		return nil, err
	}
	// QuestionChain 返回「主问题 + 追问（主问题在前）」，逐链拼接即得平铺顺序
	out := make([]Question, 0, len(mains))
	for _, main := range mains {
		chain, err := s.repo.QuestionChain(ctx, id, main.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, chain...)
	}
	return out, nil
}

// HasActiveInterview 报告该租户是否已有进行中的面试（创建入口预检）。
func (s *Service) HasActiveInterview(ctx context.Context, tenantID int64) (bool, error) {
	return s.repo.HasActiveInterview(ctx, tenantID)
}

// QuestionForAudio 校验面试与题目的归属并返回题目行（含 AudioState）。
//
// 题目不属于该面试、或面试不属于该租户时，一律返回 ErrNotFound：
// 不区分「不存在」与「不属于你」，避免泄露资源是否存在。
func (s *Service) QuestionForAudio(ctx context.Context, tenantID, interviewID, questionID int64) (*Question, error) {
	if _, err := s.repo.GetInterview(ctx, tenantID, interviewID); err != nil {
		return nil, err
	}
	row, err := s.repo.QuestionByID(ctx, questionID)
	if err != nil {
		return nil, ErrNotFound
	}
	if row.InterviewID != interviewID {
		return nil, ErrNotFound
	}
	return &row, nil
}

// getActiveInterview 校验面试存在、属于该租户且 status=active，三者缺一即拒绝：
// 不存在/非本人 → ErrNotFound（不泄露存在性），已结束 → ErrAlreadyEnded。
func (s *Service) getActiveInterview(ctx context.Context, tenantID, interviewID int64) (*Interview, error) {
	iv, err := s.repo.GetInterview(ctx, tenantID, interviewID)
	if err != nil {
		return nil, err
	}
	if iv.Status != "active" {
		return nil, ErrAlreadyEnded
	}
	return iv, nil
}
