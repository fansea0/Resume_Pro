package interview

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

// ---- Service 测试造数 ----

// serviceTestConfig 与落库题数一致的两题配置（接口契约 §6：主问题数以传入题数为准，
// 这里保持 cfg 与生题 payload 的题数一致）。
func serviceTestConfig() InterviewConfig {
	return InterviewConfig{
		QuestionCount:            2,
		LabelCounts:              map[QuestionLabel]int{LabelOctopus: 1, LabelProject: 1},
		FollowUpBudget:           1,
		PerQuestionFollowUpLimit: 1,
	}
}

// defaultConfigQuestionsPayload 生成与 DefaultInterviewConfig 的 LabelCounts 对齐的
// 12 道题（5 八股 + 2 算法 + 5 项目），供 cfg=nil 的默认配置路径使用。
func defaultConfigQuestionsPayload(t *testing.T) string {
	t.Helper()
	qs := make([]map[string]any, 0, DefaultInterviewConfig.QuestionCount)
	for i := 0; i < DefaultInterviewConfig.LabelCounts[LabelOctopus]; i++ {
		qs = append(qs, octopusQuestion())
	}
	for i := 0; i < DefaultInterviewConfig.LabelCounts[LabelAlgorithm]; i++ {
		qs = append(qs, algorithmQuestion())
	}
	for i := 0; i < DefaultInterviewConfig.LabelCounts[LabelProject]; i++ {
		qs = append(qs, projectQuestion())
	}
	return questionsPayload(t, qs...)
}

// newTestService 组装真 repo + miniredis store + fake 模型的完整 Service：
// genPayloads 喂生题器（主问题 JSON），gradePayloads 喂面试图（评分 JSON）。
// 同一 *RedisStore 实例同时充当 graph 的 checkpoint store 与 Service 的并发锁
//（RedisStore 两边都满足），与生产 T9 的注入方式一致；miniredis 替身不依赖外部 Redis。
func newTestService(t *testing.T, genPayloads, gradePayloads []string) (*Repository, *Service, *RedisStore, *miniredis.Miniredis) {
	t.Helper()
	repo, db := newTestRepo(t)
	t.Cleanup(func() { _ = db.Close() })
	store, mr := redisStore(t)
	graph, err := BuildGraph(repo, &fakeResponses{payloads: gradePayloads}, store)
	if err != nil {
		t.Fatalf("build graph: %v", err)
	}
	gen := NewQuestionGenerator(&fakeResponses{payloads: genPayloads}, 1)
	return repo, NewService(repo, store, gen, graph), store, mr
}

func twoQuestionGenPayload(t *testing.T) []string {
	t.Helper()
	return []string{questionsPayload(t, octopusQuestion(), projectQuestion())}
}

// TestServiceCreateAndAnswer 创建 → 首题 → 回答评分。
// 简报 Step 1 语义保留；造数按控制器裁定 F3 修正为「合法题集 + 题数一致的 cfg」
//（简报草图的空 questions[] 与 `payload string` 字段均与实际 API 不符）。
func TestServiceCreateAndAnswer(t *testing.T) {
	repo, svc, _, _ := newTestService(t, twoQuestionGenPayload(t), []string{gradeJSON("A", "")})
	ctx := context.Background()
	cfg := serviceTestConfig()

	iv, err := svc.CreateInterview(ctx, 1, 3, 7, "简历文本", &cfg)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if iv.Status != "active" {
		t.Errorf("status = %s, want active", iv.Status)
	}
	if iv.TargetJobID != 3 || iv.ResumeID != 7 {
		t.Errorf("targetJobID = %d, resumeID = %d, want 3, 7", iv.TargetJobID, iv.ResumeID)
	}

	// 主问题按生题结果落真实题面（契约 §1：禁止题面为空的占位行），
	// must/bonus 点经 extra 列 JSON 往返（契约 §8）
	mains, err := repo.MainQuestions(ctx, iv.ID)
	if err != nil {
		t.Fatalf("main questions: %v", err)
	}
	if len(mains) != 2 {
		t.Fatalf("main questions = %d, want 2", len(mains))
	}
	if mains[0].Question != "Go 的 channel 底层结构？" || mains[0].StandardAnswer != "hchan 结构" {
		t.Errorf("main[0] = %q / %q, want generated content", mains[0].Question, mains[0].StandardAnswer)
	}
	must, bonus := repo.ExtraFields(mains[0].Extra)
	if len(must) != 1 || must[0] != "hchan" || len(bonus) != 1 || bonus[0] != "sendq/recvq" {
		t.Errorf("extra = %q, want must/bonus points round-trip", mains[0].Extra)
	}

	// 配置快照落 Service 解析后的配置
	var snap InterviewConfig
	if err := json.Unmarshal([]byte(iv.ConfigSnapshot), &snap); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	if snap.QuestionCount != 2 || snap.FollowUpBudget != 1 {
		t.Errorf("snapshot = %+v, want questionCount 2 / followUpBudget 1", snap)
	}

	// 首题：独立方法 FirstQuestion（保持契约 §1 的 (*Interview, error) 签名）
	first, err := svc.FirstQuestion(ctx, 1, iv.ID)
	if err != nil {
		t.Fatalf("first question: %v", err)
	}
	if first.Question != mains[0].Question || first.ParentID != nil {
		t.Errorf("first question = %+v, want main question 1", first)
	}

	// 回答 → LLM 评分 A → 无追问 → 下一题
	res, err := svc.Answer(ctx, 1, iv.ID, answerPayload{Answer: "答 A 水平"})
	if err != nil {
		t.Fatalf("answer: %v", err)
	}
	if res.Grade != GradeA {
		t.Errorf("grade = %s, want A", res.Grade)
	}
	if res.Finished || res.NextQuestion == nil || res.NextQuestion.ParentID != nil {
		t.Fatalf("res = %+v, want next main question", res)
	}
	// 答案与评分已落库
	if q := mainQuestion(t, repo, iv.ID, 0); q.UserAnswer != "答 A 水平" || q.Grade != "A" {
		t.Errorf("main[0] = answer %q grade %q, want written", q.UserAnswer, q.Grade)
	}
}

// TestServiceCreateUsesDefaultConfig cfg=nil：默认配置生效（12 题），
// FollowUpBudget=-1 按 Resolved() 解析为 N/2 并落快照。
func TestServiceCreateUsesDefaultConfig(t *testing.T) {
	_, svc, _, _ := newTestService(t, []string{defaultConfigQuestionsPayload(t)}, []string{gradeJSON("A", "")})
	ctx := context.Background()

	iv, err := svc.CreateInterview(ctx, 1, 1, 1, "简历文本", nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	mains, err := svc.Questions(ctx, 1, iv.ID)
	if err != nil {
		t.Fatalf("questions: %v", err)
	}
	if len(mains) != DefaultInterviewConfig.QuestionCount {
		t.Fatalf("questions = %d, want %d", len(mains), DefaultInterviewConfig.QuestionCount)
	}
	var snap InterviewConfig
	if err := json.Unmarshal([]byte(iv.ConfigSnapshot), &snap); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	if want := DefaultInterviewConfig.QuestionCount / 2; snap.FollowUpBudget != want {
		t.Errorf("snapshot followUpBudget = %d, want %d（-1 解析为 N/2）", snap.FollowUpBudget, want)
	}
}

// TestServiceCreateResolvesNegativeBudget 用户 cfg 的 FollowUpBudget<0 同样解析为 N/2。
func TestServiceCreateResolvesNegativeBudget(t *testing.T) {
	_, svc, _, _ := newTestService(t, twoQuestionGenPayload(t), []string{gradeJSON("A", "")})
	ctx := context.Background()

	cfg := serviceTestConfig()
	cfg.FollowUpBudget = -1
	iv, err := svc.CreateInterview(ctx, 1, 1, 1, "简历文本", &cfg)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	var snap InterviewConfig
	if err := json.Unmarshal([]byte(iv.ConfigSnapshot), &snap); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	if snap.FollowUpBudget != 1 {
		t.Errorf("snapshot followUpBudget = %d, want 1（2/2）", snap.FollowUpBudget)
	}
}

// TestServiceCreateRejectsSecondActive 同租户已有 active 面试时创建被拒；
// 其它租户不受影响。
func TestServiceCreateRejectsSecondActive(t *testing.T) {
	_, svc, _, _ := newTestService(t, twoQuestionGenPayload(t), []string{gradeJSON("A", "")})
	ctx := context.Background()
	cfg := serviceTestConfig()

	if _, err := svc.CreateInterview(ctx, 1, 1, 1, "简历文本", &cfg); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if _, err := svc.CreateInterview(ctx, 1, 1, 1, "简历文本", &cfg); !errors.Is(err, ErrActiveInterviewExists) {
		t.Fatalf("second create err = %v, want ErrActiveInterviewExists", err)
	}
	if _, err := svc.CreateInterview(ctx, 2, 1, 1, "简历文本", &cfg); err != nil {
		t.Fatalf("other tenant create: %v", err)
	}
}

// TestServiceAnswerRejectsEnded 创建 → abort → answer 返回 ErrAlreadyEnded；
// 已终止的面试再次 abort 同样拒绝。
func TestServiceAnswerRejectsEnded(t *testing.T) {
	_, svc, _, _ := newTestService(t, twoQuestionGenPayload(t), []string{gradeJSON("A", "")})
	ctx := context.Background()
	cfg := serviceTestConfig()

	iv, err := svc.CreateInterview(ctx, 1, 1, 1, "简历文本", &cfg)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.Abort(ctx, 1, iv.ID); err != nil {
		t.Fatalf("abort: %v", err)
	}
	got, err := svc.Get(ctx, 1, iv.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != "aborted" || got.CompletedAt == nil {
		t.Errorf("status = %s, completedAt = %v, want aborted + completed_at set", got.Status, got.CompletedAt)
	}
	if _, err := svc.Answer(ctx, 1, iv.ID, answerPayload{Answer: "迟到回答"}); !errors.Is(err, ErrAlreadyEnded) {
		t.Fatalf("answer err = %v, want ErrAlreadyEnded", err)
	}
	if err := svc.Abort(ctx, 1, iv.ID); !errors.Is(err, ErrAlreadyEnded) {
		t.Fatalf("re-abort err = %v, want ErrAlreadyEnded", err)
	}
}

// TestServiceAnswerRejectsConcurrent 锁被他人预占时 answer 返回 ErrConcurrentAnswer，
// 且 Service 不得释放不属于自己的锁。
func TestServiceAnswerRejectsConcurrent(t *testing.T) {
	_, svc, store, _ := newTestService(t, twoQuestionGenPayload(t), []string{gradeJSON("A", "")})
	ctx := context.Background()
	cfg := serviceTestConfig()

	iv, err := svc.CreateInterview(ctx, 1, 1, 1, "简历文本", &cfg)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// 另一个处理者预占同一把锁（key 与 Service 同构造：面试 id 十进制串，契约 §9）
	lockKey := strconv.FormatInt(iv.ID, 10)
	if locked, err := store.TryLock(ctx, lockKey, 60); err != nil || !locked {
		t.Fatalf("pre-lock: locked=%v err=%v", locked, err)
	}
	if _, err := svc.Answer(ctx, 1, iv.ID, answerPayload{Answer: "并发回答"}); !errors.Is(err, ErrConcurrentAnswer) {
		t.Fatalf("err = %v, want ErrConcurrentAnswer", err)
	}
	// 被拒的调用方没有 Unlock 权限：预占锁必须仍在
	if again, err := store.TryLock(ctx, lockKey, 60); err != nil || again {
		t.Fatalf("lock after rejected answer: locked=%v err=%v, want still held", again, err)
	}
}

// TestServiceAnswerSessionExpired checkpoint 过期（7 天 TTL）后 answer 返回
// ErrSessionExpired（410 语义），面试行不受影响、已答数据仍可回看。
func TestServiceAnswerSessionExpired(t *testing.T) {
	repo, svc, _, mr := newTestService(t, twoQuestionGenPayload(t), []string{gradeJSON("A", "")})
	ctx := context.Background()
	cfg := serviceTestConfig()

	iv, err := svc.CreateInterview(ctx, 1, 1, 1, "简历文本", &cfg)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// checkpoint TTL 7 天：miniredis 时钟快进越过 TTL，模拟会话过期
	mr.FastForward(8 * 24 * time.Hour)
	if _, err := svc.Answer(ctx, 1, iv.ID, answerPayload{Answer: "回答"}); !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("err = %v, want ErrSessionExpired", err)
	}
	got, err := svc.Get(ctx, 1, iv.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != "active" {
		t.Errorf("status = %s, want active（过期不影响面试行）", got.Status)
	}
	if _, err := repo.MainQuestions(ctx, iv.ID); err != nil {
		t.Errorf("questions still readable after expiry: %v", err)
	}
}

// TestServiceTenantIsolation 非本人租户的 answer/abort/get/questions/firstQuestion
// 一律 ErrNotFound（租户隔离在 Service 层校验，契约 §4）。
func TestServiceTenantIsolation(t *testing.T) {
	_, svc, _, _ := newTestService(t, twoQuestionGenPayload(t), []string{gradeJSON("A", "")})
	ctx := context.Background()
	cfg := serviceTestConfig()

	iv, err := svc.CreateInterview(ctx, 1, 1, 1, "简历文本", &cfg)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Get(ctx, 2, iv.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("get err = %v, want ErrNotFound", err)
	}
	if _, err := svc.Questions(ctx, 2, iv.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("questions err = %v, want ErrNotFound", err)
	}
	if _, err := svc.FirstQuestion(ctx, 2, iv.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("first question err = %v, want ErrNotFound", err)
	}
	if _, err := svc.Answer(ctx, 2, iv.ID, answerPayload{Answer: "x"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("answer err = %v, want ErrNotFound", err)
	}
	if err := svc.Abort(ctx, 2, iv.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("abort err = %v, want ErrNotFound", err)
	}
	if _, err := svc.List(ctx, 2); err != nil {
		t.Errorf("list other tenant: %v", err)
	}
	// 本租户不受影响：仍可正常回答
	if _, err := svc.Answer(ctx, 1, iv.ID, answerPayload{Answer: "回答"}); err != nil {
		t.Errorf("own tenant answer: %v", err)
	}
}

// TestServiceQuestionsFlattensFollowUps 回看：主问题 + 追问链平铺（主问题在前、
// 追问紧跟父题），List 按 tenant 隔离。
func TestServiceQuestionsFlattensFollowUps(t *testing.T) {
	_, svc, _, _ := newTestService(t, twoQuestionGenPayload(t), []string{
		gradeJSON("B", "为什么 hchan 需要 ring buffer？"),
		gradeJSON("B+", ""),
	})
	ctx := context.Background()
	cfg := serviceTestConfig()

	iv, err := svc.CreateInterview(ctx, 1, 1, 1, "简历文本", &cfg)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// 第一答 B + 追问 → 插入追问行
	if _, err := svc.Answer(ctx, 1, iv.ID, answerPayload{Answer: "回答 1"}); err != nil {
		t.Fatalf("answer 1: %v", err)
	}
	qs, err := svc.Questions(ctx, 1, iv.ID)
	if err != nil {
		t.Fatalf("questions: %v", err)
	}
	if len(qs) != 3 {
		t.Fatalf("questions = %d, want 3（主问题 1 + 追问 + 主问题 2）", len(qs))
	}
	if qs[0].ParentID != nil {
		t.Errorf("qs[0] should be main question, got %+v", qs[0])
	}
	if qs[1].ParentID == nil || *qs[1].ParentID != qs[0].ID || qs[1].FollowUpRound != 1 {
		t.Errorf("qs[1] should be follow-up of qs[0] round 1, got %+v", qs[1])
	}
	if qs[2].ParentID != nil || qs[2].Question != "项目里的缓存怎么做？" {
		t.Errorf("qs[2] should be main question 2, got %+v", qs[2])
	}
	// 第二答 → 推进到主问题 2，无新追问行
	if _, err := svc.Answer(ctx, 1, iv.ID, answerPayload{Answer: "回答 2"}); err != nil {
		t.Fatalf("answer 2: %v", err)
	}
	qs2, err := svc.Questions(ctx, 1, iv.ID)
	if err != nil {
		t.Fatalf("questions after answer 2: %v", err)
	}
	if len(qs2) != 3 {
		t.Errorf("questions = %d, want 3（追问不新增）", len(qs2))
	}
	// List：本租户 1 场，其它租户 0 场
	list, err := svc.List(ctx, 1)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].ID != iv.ID {
		t.Errorf("list = %+v, want [interview %d]", list, iv.ID)
	}
	empty, err := svc.List(ctx, 2)
	if err != nil || len(empty) != 0 {
		t.Errorf("list other tenant = %v err = %v, want empty", empty, err)
	}
}
