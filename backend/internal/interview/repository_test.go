package interview

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"resumepro/backend/internal/store"
)

func newTestRepo(t *testing.T) (*Repository, *sqlx.DB) {
	t.Helper()
	db, err := store.OpenSQLite(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	// :memory: 库按连接隔离，限制为单连接，避免迁移与后续查询落到不同的库上
	//（与 internal/store/migrate_test.go 的既有做法一致）。
	db.SetMaxOpenConns(1)
	if err := store.Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewRepository(db), db
}

func TestCreateInterviewAndQuestions(t *testing.T) {
	repo, db := newTestRepo(t)
	defer db.Close()
	cfg := DefaultInterviewConfig.Resolved()
	iv, err := repo.CreateInterview(context.Background(), 1, cfg, 1, 1)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if iv.Status != "active" {
		t.Errorf("status = %s, want active", iv.Status)
	}
	qs, err := repo.MainQuestions(context.Background(), iv.ID)
	if err != nil {
		t.Fatalf("main questions: %v", err)
	}
	if len(qs) != cfg.QuestionCount {
		t.Errorf("questions = %d, want %d", len(qs), cfg.QuestionCount)
	}
	for i, q := range qs {
		if q.ParentID != nil {
			t.Errorf("q[%d] parent not nil", i)
		}
	}
}

func TestCreateInterviewRejectsSecondActive(t *testing.T) {
	repo, db := newTestRepo(t)
	defer db.Close()
	cfg := DefaultInterviewConfig.Resolved()
	if _, err := repo.CreateInterview(context.Background(), 1, cfg, 1, 1); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if _, err := repo.CreateInterview(context.Background(), 1, cfg, 1, 1); !errors.Is(err, ErrActiveInterviewExists) {
		t.Errorf("second create err = %v, want ErrActiveInterviewExists", err)
	}
}

func TestQuestionChainAndFollowUp(t *testing.T) {
	repo, db := newTestRepo(t)
	defer db.Close()
	cfg := DefaultInterviewConfig.Resolved()
	iv, _ := repo.CreateInterview(context.Background(), 1, cfg, 1, 1)
	qs, _ := repo.MainQuestions(context.Background(), iv.ID)
	parentID := qs[0].ID
	fuID, err := repo.InsertFollowUp(context.Background(), iv.ID, parentID, 1, "追问问题", "", `{"must_points":[],"bonus_points":[]}`)
	if err != nil {
		t.Fatalf("insert follow-up: %v", err)
	}
	chain, err := repo.QuestionChain(context.Background(), iv.ID, parentID)
	if err != nil {
		t.Fatalf("chain: %v", err)
	}
	if len(chain) != 2 || chain[1].ID != fuID || chain[1].ParentID == nil || *chain[1].ParentID != parentID {
		t.Errorf("chain = %+v, want [main, followup]", chain)
	}
	if err := repo.UpdateAnswer(context.Background(), parentID, "我的回答", "B+"); err != nil {
		t.Fatalf("update answer: %v", err)
	}
	q, _ := repo.QuestionChain(context.Background(), iv.ID, parentID)
	if q[0].UserAnswer != "我的回答" || q[0].Grade != "B+" {
		t.Errorf("updated row = %+v", q[0])
	}
}

// --- 以下为任务 4 追加的契约测试 ---

// TestCreateInterviewRejectedLeavesNoDirtyData 验证：第二次创建命中 active 冲突后，
// 事务整体回滚，不留下多余的面试行或问题行。
func TestCreateInterviewRejectedLeavesNoDirtyData(t *testing.T) {
	repo, db := newTestRepo(t)
	defer db.Close()
	ctx := context.Background()
	cfg := DefaultInterviewConfig.Resolved()
	if _, err := repo.CreateInterview(ctx, 1, cfg, 1, 1); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if _, err := repo.CreateInterview(ctx, 1, cfg, 1, 1); !errors.Is(err, ErrActiveInterviewExists) {
		t.Fatalf("second create err = %v, want ErrActiveInterviewExists", err)
	}
	var interviews, questions int
	if err := db.GetContext(ctx, &interviews, `SELECT COUNT(*) FROM interviews`); err != nil {
		t.Fatalf("count interviews: %v", err)
	}
	if err := db.GetContext(ctx, &questions, `SELECT COUNT(*) FROM interview_questions`); err != nil {
		t.Fatalf("count questions: %v", err)
	}
	if interviews != 1 {
		t.Errorf("interviews = %d, want 1", interviews)
	}
	if questions != cfg.QuestionCount {
		t.Errorf("questions = %d, want %d", questions, cfg.QuestionCount)
	}
}

// TestCreateInterviewConcurrentSameTenant 验证并发创建同租户时只有一个成功。
func TestCreateInterviewConcurrentSameTenant(t *testing.T) {
	repo, db := newTestRepo(t)
	defer db.Close()
	ctx := context.Background()
	cfg := DefaultInterviewConfig.Resolved()

	var wg sync.WaitGroup
	results := make([]error, 2)
	for i := range results {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, results[idx] = repo.CreateInterview(ctx, 1, cfg, 1, 1)
		}(i)
	}
	wg.Wait()

	created, rejected := 0, 0
	for _, err := range results {
		switch {
		case err == nil:
			created++
		case errors.Is(err, ErrActiveInterviewExists):
			rejected++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if created != 1 || rejected != 1 {
		t.Errorf("created = %d, rejected = %d, want 1/1", created, rejected)
	}
}

// TestCreateInterviewConcurrentSameTenantFileDB 用文件库 + 多连接验证并发创建的串行化：
// 事务内先取 SQLite 写锁再检查 active，否则并发事务会各自读到「没有 active 面试」。
func TestCreateInterviewConcurrentSameTenantFileDB(t *testing.T) {
	db, err := store.OpenSQLite(filepath.Join(t.TempDir(), "concurrent.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	db.SetMaxOpenConns(4)
	defer db.Close()
	ctx := context.Background()
	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := NewRepository(db)
	cfg := DefaultInterviewConfig.Resolved()

	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range errs {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = repo.CreateInterview(ctx, 1, cfg, 1, 1)
		}(i)
	}
	wg.Wait()

	created, rejected := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			created++
		case errors.Is(err, ErrActiveInterviewExists):
			rejected++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if created != 1 || rejected != 3 {
		t.Errorf("created = %d, rejected = %d, want 1/3", created, rejected)
	}
	var interviews, questions int
	if err := db.GetContext(ctx, &interviews, `SELECT COUNT(*) FROM interviews`); err != nil {
		t.Fatalf("count interviews: %v", err)
	}
	if err := db.GetContext(ctx, &questions, `SELECT COUNT(*) FROM interview_questions`); err != nil {
		t.Fatalf("count questions: %v", err)
	}
	if interviews != 1 || questions != cfg.QuestionCount {
		t.Errorf("interviews = %d, questions = %d, want 1/%d", interviews, questions, cfg.QuestionCount)
	}
}

// TestCreateInterviewPersistsLabelDistribution 钉住 label 列的落库契约（0/1/2 枚举）。
func TestCreateInterviewPersistsLabelDistribution(t *testing.T) {
	repo, db := newTestRepo(t)
	defer db.Close()
	ctx := context.Background()
	cfg := DefaultInterviewConfig.Resolved()
	iv, err := repo.CreateInterview(ctx, 1, cfg, 1, 1)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	qs, err := repo.MainQuestions(ctx, iv.ID)
	if err != nil {
		t.Fatalf("main questions: %v", err)
	}
	counts := map[QuestionLabel]int{}
	for _, q := range qs {
		counts[q.Label]++
	}
	for label, want := range cfg.LabelCounts {
		if counts[label] != want {
			t.Errorf("label %d count = %d, want %d", label, counts[label], want)
		}
	}
}

// TestNullColumnsReadBackAsEmpty 验证可空列（standard_answer/user_answer/extra/grade）为 NULL 时
// 读回空串且不报错；interviews 的可空列读回 nil。
func TestNullColumnsReadBackAsEmpty(t *testing.T) {
	repo, db := newTestRepo(t)
	defer db.Close()
	ctx := context.Background()
	cfg := DefaultInterviewConfig.Resolved()
	iv, err := repo.CreateInterview(ctx, 1, cfg, 1, 1)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	qs, err := repo.MainQuestions(ctx, iv.ID)
	if err != nil {
		t.Fatalf("main questions: %v", err)
	}
	for i, q := range qs {
		if q.StandardAnswer != "" || q.UserAnswer != "" || q.Extra != "" || q.Grade != "" {
			t.Errorf("q[%d] = %+v, want empty nullable fields", i, q)
		}
	}
	if iv.OverallGrade != nil || iv.OverallReview != nil || iv.CompletedAt != nil {
		t.Errorf("interview = %+v, want nil nullable fields", iv)
	}
	if iv.CreatedAt == nil || iv.UpdatedAt == nil {
		t.Errorf("interview timestamps = %+v, want non-nil", iv)
	}
}

// TestCreateInterviewWithQuestionsPersistsContent 验证生题结果能一次落库（题面/标准答案/必答加分点）。
func TestCreateInterviewWithQuestionsPersistsContent(t *testing.T) {
	repo, db := newTestRepo(t)
	defer db.Close()
	ctx := context.Background()
	cfg := DefaultInterviewConfig.Resolved()
	inputs := []MainQuestionInput{
		{Label: LabelOctopus, Question: "channel 底层结构？", StandardAnswer: "hchan", MustPoints: []string{"hchan"}, BonusPoints: []string{"sendq"}},
		{Label: LabelProject, Question: "项目里的缓存怎么做？"},
	}
	iv, err := repo.CreateInterviewWithQuestions(ctx, 1, cfg, 1, 1, inputs)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	qs, err := repo.MainQuestions(ctx, iv.ID)
	if err != nil {
		t.Fatalf("main questions: %v", err)
	}
	if len(qs) != len(inputs) {
		t.Fatalf("questions = %d, want %d", len(qs), len(inputs))
	}
	if qs[0].Question != inputs[0].Question || qs[0].StandardAnswer != inputs[0].StandardAnswer {
		t.Errorf("q[0] = %+v, want %+v", qs[0], inputs[0])
	}
	if qs[0].Label != LabelOctopus || qs[1].Label != LabelProject {
		t.Errorf("labels = %d/%d, want %d/%d", qs[0].Label, qs[1].Label, LabelOctopus, LabelProject)
	}
	must, bonus := repo.ExtraFields(qs[0].Extra)
	if len(must) != 1 || must[0] != "hchan" || len(bonus) != 1 || bonus[0] != "sendq" {
		t.Errorf("extra = %q -> must %v bonus %v", qs[0].Extra, must, bonus)
	}
	if qs[1].Extra != "" {
		t.Errorf("q[1].Extra = %q, want empty", qs[1].Extra)
	}
}

// TestSetMainQuestionsFillsPlaceholders 验证占位主问题可被生题结果回填。
func TestSetMainQuestionsFillsPlaceholders(t *testing.T) {
	repo, db := newTestRepo(t)
	defer db.Close()
	ctx := context.Background()
	cfg := DefaultInterviewConfig.Resolved()
	iv, err := repo.CreateInterview(ctx, 1, cfg, 1, 1)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	inputs := []MainQuestionInput{
		{Label: LabelOctopus, Question: "Q1", StandardAnswer: "A1"},
		{Label: LabelAlgorithm, Question: "Q2"},
	}
	if err := repo.SetMainQuestions(ctx, iv.ID, inputs); err != nil {
		t.Fatalf("set main questions: %v", err)
	}
	qs, err := repo.MainQuestions(ctx, iv.ID)
	if err != nil {
		t.Fatalf("main questions: %v", err)
	}
	if qs[0].Question != "Q1" || qs[0].StandardAnswer != "A1" || qs[0].Label != LabelOctopus {
		t.Errorf("q[0] = %+v", qs[0])
	}
	if qs[1].Question != "Q2" || qs[1].Label != LabelAlgorithm {
		t.Errorf("q[1] = %+v", qs[1])
	}
	if len(qs) != cfg.QuestionCount {
		t.Errorf("questions = %d, want %d", len(qs), cfg.QuestionCount)
	}
}

// TestConfigSnapshotRoundTrip 验证 config_snapshot 存的是 InterviewConfig 的 JSON 且能原样读回。
func TestConfigSnapshotRoundTrip(t *testing.T) {
	repo, db := newTestRepo(t)
	defer db.Close()
	ctx := context.Background()
	cfg := DefaultInterviewConfig.Resolved()
	iv, err := repo.CreateInterview(ctx, 1, cfg, 1, 1)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	var decoded InterviewConfig
	if err := json.Unmarshal([]byte(iv.ConfigSnapshot), &decoded); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	if decoded.QuestionCount != cfg.QuestionCount {
		t.Errorf("question count = %d, want %d", decoded.QuestionCount, cfg.QuestionCount)
	}
	if decoded.FollowUpBudget != cfg.FollowUpBudget {
		t.Errorf("follow up budget = %d, want %d", decoded.FollowUpBudget, cfg.FollowUpBudget)
	}
	if decoded.PerQuestionFollowUpLimit != cfg.PerQuestionFollowUpLimit {
		t.Errorf("per question limit = %d, want %d", decoded.PerQuestionFollowUpLimit, cfg.PerQuestionFollowUpLimit)
	}
	for label, want := range cfg.LabelCounts {
		if decoded.LabelCounts[label] != want {
			t.Errorf("label %d count = %d, want %d", label, decoded.LabelCounts[label], want)
		}
	}
}

func TestUpdateInterviewStatus(t *testing.T) {
	repo, db := newTestRepo(t)
	defer db.Close()
	ctx := context.Background()
	cfg := DefaultInterviewConfig.Resolved()
	iv, err := repo.CreateInterview(ctx, 1, cfg, 1, 1)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	grade, review := "A", "总体不错"
	if err := repo.UpdateInterviewStatus(ctx, iv.ID, "completed", &grade, &review); err != nil {
		t.Fatalf("update status: %v", err)
	}
	got, err := repo.GetInterview(ctx, 1, iv.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != "completed" {
		t.Errorf("status = %s, want completed", got.Status)
	}
	if got.OverallGrade == nil || *got.OverallGrade != grade {
		t.Errorf("overall grade = %v, want %s", got.OverallGrade, grade)
	}
	if got.OverallReview == nil || *got.OverallReview != review {
		t.Errorf("overall review = %v, want %s", got.OverallReview, review)
	}
	if got.CompletedAt == nil {
		t.Error("completed_at = nil, want set")
	}
}

func TestUpdateInterviewStatusUnknownID(t *testing.T) {
	repo, db := newTestRepo(t)
	defer db.Close()
	if err := repo.UpdateInterviewStatus(context.Background(), 999, "completed", nil, nil); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestUpdateAnswerUnknownID(t *testing.T) {
	repo, db := newTestRepo(t)
	defer db.Close()
	if err := repo.UpdateAnswer(context.Background(), 999, "答案", "B"); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestUpdateGradeOnlyTouchesGrade(t *testing.T) {
	repo, db := newTestRepo(t)
	defer db.Close()
	ctx := context.Background()
	cfg := DefaultInterviewConfig.Resolved()
	iv, err := repo.CreateInterview(ctx, 1, cfg, 1, 1)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	qs, err := repo.MainQuestions(ctx, iv.ID)
	if err != nil {
		t.Fatalf("main questions: %v", err)
	}
	if err := repo.UpdateAnswer(ctx, qs[0].ID, "我的回答", "C"); err != nil {
		t.Fatalf("update answer: %v", err)
	}
	if err := repo.UpdateGrade(ctx, qs[0].ID, "A"); err != nil {
		t.Fatalf("update grade: %v", err)
	}
	chain, err := repo.QuestionChain(ctx, iv.ID, qs[0].ID)
	if err != nil {
		t.Fatalf("chain: %v", err)
	}
	if chain[0].Grade != "A" {
		t.Errorf("grade = %s, want A", chain[0].Grade)
	}
	if chain[0].UserAnswer != "我的回答" {
		t.Errorf("user answer = %q, want 我的回答", chain[0].UserAnswer)
	}
}

func TestGetInterviewNotFound(t *testing.T) {
	repo, db := newTestRepo(t)
	defer db.Close()
	ctx := context.Background()
	cfg := DefaultInterviewConfig.Resolved()
	iv, err := repo.CreateInterview(ctx, 1, cfg, 1, 1)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := repo.GetInterview(ctx, 1, iv.ID+999); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
	if _, err := repo.GetInterview(ctx, 2, iv.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("cross tenant err = %v, want ErrNotFound", err)
	}
}

func TestListInterviewsOrdersByUpdatedAtDesc(t *testing.T) {
	repo, db := newTestRepo(t)
	defer db.Close()
	ctx := context.Background()
	cfg := DefaultInterviewConfig.Resolved()
	first, err := repo.CreateInterview(ctx, 1, cfg, 1, 1)
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	if err := repo.UpdateInterviewStatus(ctx, first.ID, "completed", nil, nil); err != nil {
		t.Fatalf("complete first: %v", err)
	}
	second, err := repo.CreateInterview(ctx, 1, cfg, 1, 1)
	if err != nil {
		t.Fatalf("create second: %v", err)
	}
	if _, err := repo.CreateInterview(ctx, 2, cfg, 1, 1); err != nil {
		t.Fatalf("create other tenant: %v", err)
	}

	list, err := repo.ListInterviews(ctx, 1)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("list = %d, want 2", len(list))
	}
	if list[0].ID != second.ID || list[1].ID != first.ID {
		t.Errorf("order = %d/%d, want %d/%d", list[0].ID, list[1].ID, second.ID, first.ID)
	}
}

func TestExtraFields(t *testing.T) {
	repo, db := newTestRepo(t)
	defer db.Close()
	must, bonus := repo.ExtraFields(`{"must_points":["a","b"],"bonus_points":["c"]}`)
	if len(must) != 2 || must[0] != "a" || must[1] != "b" {
		t.Errorf("must = %v, want [a b]", must)
	}
	if len(bonus) != 1 || bonus[0] != "c" {
		t.Errorf("bonus = %v, want [c]", bonus)
	}
	must, bonus = repo.ExtraFields("not json")
	if must != nil || bonus != nil {
		t.Errorf("invalid json -> %v/%v, want nil/nil", must, bonus)
	}
	must, bonus = repo.ExtraFields("")
	if must != nil || bonus != nil {
		t.Errorf("empty -> %v/%v, want nil/nil", must, bonus)
	}
}

func TestInsertFollowUpUnknownParent(t *testing.T) {
	repo, db := newTestRepo(t)
	defer db.Close()
	ctx := context.Background()
	cfg := DefaultInterviewConfig.Resolved()
	iv, err := repo.CreateInterview(ctx, 1, cfg, 1, 1)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := repo.InsertFollowUp(ctx, iv.ID, 999, 1, "追问", "", ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

// --- 题目语音状态（任务 1 追加） ---

// newRepositoryFixture 建库、迁移并创建一场空的 active 面试，返回仓储、上下文与面试 ID。
//
// 面试不带占位主问题（显式传空题目列表），题目一律由 insertMainQuestion 插入，
// 避免占位行混进「哪些题目需要语音」这类按题目维度的查询。
func newRepositoryFixture(t *testing.T) (*Repository, context.Context, int64) {
	t.Helper()
	repo, db := newTestRepo(t)
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	cfg := DefaultInterviewConfig.Resolved()
	iv, err := repo.CreateInterviewWithQuestions(ctx, 1, cfg, 1, 1, []MainQuestionInput{})
	if err != nil {
		t.Fatalf("create interview: %v", err)
	}
	return repo, ctx, iv.ID
}

// insertMainQuestion 直接向 interview_questions 插入一道指定题面的主问题，返回题目 ID。
func insertMainQuestion(t *testing.T, repo *Repository, interviewID int64, text string) int64 {
	t.Helper()
	result, err := repo.db.ExecContext(context.Background(), `
		INSERT INTO interview_questions (interview_id, parent_id, follow_up_round, label, question, standard_answer, user_answer, extra, grade, created_at)
		VALUES (?, NULL, 0, ?, ?, NULL, NULL, NULL, NULL, ?)`,
		interviewID, int(LabelOctopus), text, time.Now().UTC())
	if err != nil {
		t.Fatalf("insert question: %v", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("last insert id: %v", err)
	}
	return id
}

func TestRepositoryUpdateAudioState(t *testing.T) {
	repo, ctx, interviewID := newRepositoryFixture(t)
	questionID := insertMainQuestion(t, repo, interviewID, "Go 的 channel 是什么？")

	if err := repo.UpdateAudioState(ctx, questionID, "pending"); err != nil {
		t.Fatalf("update audio state: %v", err)
	}
	row, err := repo.QuestionByID(ctx, questionID)
	if err != nil {
		t.Fatalf("question by id: %v", err)
	}
	if row.AudioState == nil || *row.AudioState != "pending" {
		t.Fatalf("audio state = %v, want pending", row.AudioState)
	}
}

func TestRepositoryActiveQuestionsNeedingAudio(t *testing.T) {
	repo, ctx, interviewID := newRepositoryFixture(t)
	ready := insertMainQuestion(t, repo, interviewID, "已就绪")
	missing := insertMainQuestion(t, repo, interviewID, "未生成")
	if err := repo.UpdateAudioState(ctx, ready, "ready"); err != nil {
		t.Fatalf("mark ready: %v", err)
	}

	items, err := repo.ActiveQuestionsNeedingAudio(ctx)
	if err != nil {
		t.Fatalf("active questions needing audio: %v", err)
	}
	if len(items) != 1 || items[0].QuestionID != missing {
		t.Fatalf("items = %+v, want only question %d", items, missing)
	}
	if items[0].Text != "未生成" || items[0].InterviewID != interviewID {
		t.Fatalf("item payload = %+v", items[0])
	}
}

// TestRepositoryActiveQuestionsNeedingAudioSkipsFinishedInterview 验证已结束的面试不再补语音。
func TestRepositoryActiveQuestionsNeedingAudioSkipsFinishedInterview(t *testing.T) {
	repo, ctx, interviewID := newRepositoryFixture(t)
	insertMainQuestion(t, repo, interviewID, "已结束面试的题")
	if err := repo.UpdateInterviewStatus(ctx, interviewID, "completed", nil, nil); err != nil {
		t.Fatalf("complete interview: %v", err)
	}

	items, err := repo.ActiveQuestionsNeedingAudio(ctx)
	if err != nil {
		t.Fatalf("active questions needing audio: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("items = %+v, want empty", items)
	}
}

// TestRepositoryClaimAudioStateIsExclusive 抢占是原子的：第一个调用者拿到 true，第二个拿到 false。
func TestRepositoryClaimAudioStateIsExclusive(t *testing.T) {
	repo, ctx, interviewID := newRepositoryFixture(t)
	questionID := insertMainQuestion(t, repo, interviewID, "题面")

	claimed, err := repo.ClaimAudioState(ctx, questionID)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if !claimed {
		t.Fatal("first claim should succeed")
	}
	claimed, err = repo.ClaimAudioState(ctx, questionID)
	if err != nil {
		t.Fatalf("second claim: %v", err)
	}
	if claimed {
		t.Fatal("second claim should fail")
	}

	// ready 之后不能再抢。
	if err := repo.UpdateAudioState(ctx, questionID, "ready"); err != nil {
		t.Fatalf("mark ready: %v", err)
	}
	claimed, err = repo.ClaimAudioState(ctx, questionID)
	if err != nil {
		t.Fatalf("claim after ready: %v", err)
	}
	if claimed {
		t.Fatal("ready question must not be claimable")
	}
}

// TestRepositoryClaimAudioStateCanRetryFailed 验证 failed 的题目可以重新抢到生成权。
func TestRepositoryClaimAudioStateCanRetryFailed(t *testing.T) {
	repo, ctx, interviewID := newRepositoryFixture(t)
	questionID := insertMainQuestion(t, repo, interviewID, "题面")
	if err := repo.UpdateAudioState(ctx, questionID, "failed"); err != nil {
		t.Fatalf("mark failed: %v", err)
	}

	claimed, err := repo.ClaimAudioState(ctx, questionID)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if !claimed {
		t.Fatal("failed question should be claimable again")
	}
}

// TestRepositoryReleaseAudioState 验证归还在生成权：pending 回到未生成，且不影响其他状态。
func TestRepositoryReleaseAudioState(t *testing.T) {
	repo, ctx, interviewID := newRepositoryFixture(t)
	questionID := insertMainQuestion(t, repo, interviewID, "题面")
	if _, err := repo.ClaimAudioState(ctx, questionID); err != nil {
		t.Fatalf("claim: %v", err)
	}

	if err := repo.ReleaseAudioState(ctx, questionID); err != nil {
		t.Fatalf("release: %v", err)
	}
	row, err := repo.QuestionByID(ctx, questionID)
	if err != nil {
		t.Fatalf("question by id: %v", err)
	}
	if row.AudioState != nil {
		t.Fatalf("audio state = %q, want NULL after release", *row.AudioState)
	}

	// 归还可再次抢占。
	claimed, err := repo.ClaimAudioState(ctx, questionID)
	if err != nil {
		t.Fatalf("claim after release: %v", err)
	}
	if !claimed {
		t.Fatal("released question should be claimable again")
	}
}

func TestRepositoryResetStalePendingAudioOnlyTouchesActiveInterviews(t *testing.T) {
	repo, ctx, activeID := newRepositoryFixture(t)
	stale := insertMainQuestion(t, repo, activeID, "崩溃残留")
	if _, err := repo.ClaimAudioState(ctx, stale); err != nil {
		t.Fatalf("claim: %v", err)
	}

	// 另一场已结束的面试：pending 不该被这次启动清理碰到。
	cfg := DefaultInterviewConfig.Resolved()
	finished, err := repo.CreateInterviewWithQuestions(ctx, 2, cfg, 1, 1, []MainQuestionInput{})
	if err != nil {
		t.Fatalf("create other interview: %v", err)
	}
	kept := insertMainQuestion(t, repo, finished.ID, "已结束面试的题")
	if _, err := repo.ClaimAudioState(ctx, kept); err != nil {
		t.Fatalf("claim other: %v", err)
	}
	if err := repo.UpdateInterviewStatus(ctx, finished.ID, "completed", nil, nil); err != nil {
		t.Fatalf("complete other: %v", err)
	}

	affected, err := repo.ResetStalePendingAudio(ctx)
	if err != nil {
		t.Fatalf("reset stale: %v", err)
	}
	if affected != 1 {
		t.Fatalf("affected = %d, want 1", affected)
	}
	row, err := repo.QuestionByID(ctx, stale)
	if err != nil {
		t.Fatalf("question by id: %v", err)
	}
	if row.AudioState != nil {
		t.Fatalf("audio state = %q, want NULL after reset", *row.AudioState)
	}
	row, err = repo.QuestionByID(ctx, kept)
	if err != nil {
		t.Fatalf("question by id: %v", err)
	}
	if row.AudioState == nil || *row.AudioState != "pending" {
		t.Fatalf("audio state = %v, want pending (finished interview untouched)", row.AudioState)
	}
}

func TestRepositoryResetAudioStateByInterview(t *testing.T) {
	repo, ctx, interviewID := newRepositoryFixture(t)
	first := insertMainQuestion(t, repo, interviewID, "第一题")
	second := insertMainQuestion(t, repo, interviewID, "第二题")
	for _, id := range []int64{first, second} {
		if _, err := repo.ClaimAudioState(ctx, id); err != nil {
			t.Fatalf("claim %d: %v", id, err)
		}
	}
	if err := repo.ResetAudioStateByInterview(ctx, interviewID); err != nil {
		t.Fatalf("reset by interview: %v", err)
	}
	for _, id := range []int64{first, second} {
		row, err := repo.QuestionByID(ctx, id)
		if err != nil {
			t.Fatalf("question by id: %v", err)
		}
		if row.AudioState != nil {
			t.Fatalf("question %d audio state = %q, want NULL", id, *row.AudioState)
		}
	}
}
