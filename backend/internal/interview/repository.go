package interview

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"resumepro/backend/internal/store"
)

var (
	// ErrNotFound 表示面试或问题不存在（或不属于当前租户）。
	ErrNotFound = errors.New("not found")
	// ErrActiveInterviewExists 表示该租户已有进行中的面试，创建被拒绝。
	ErrActiveInterviewExists = errors.New("active interview already exists")
)

// interviewColumns 是 interviews 表的读取列。
//
// interviews.overall_grade / overall_review / completed_at 是可空列：
// 扫描到 *string / *time.Time 时 NULL 会落成 nil（不报错），语义与「尚未写入」一致。
const interviewColumns = `id, tenant_id, target_job_id, resume_id, config_snapshot, status, ` +
	`overall_grade, overall_review, created_at, updated_at, completed_at`

// questionColumns 是 interview_questions 表的读取列。
//
// standard_answer / user_answer / extra / grade 是可空列，而 Question 用非指针 string 承接，
// 直接扫描 NULL 会报错，因此统一用 COALESCE 归一成空串（NULL 与空串在读侧等价）。
const questionColumns = `id, interview_id, parent_id, follow_up_round, label, question, ` +
	`COALESCE(standard_answer, '') AS standard_answer, ` +
	`COALESCE(user_answer, '') AS user_answer, ` +
	`COALESCE(extra, '') AS extra, ` +
	`COALESCE(grade, '') AS grade, ` +
	`created_at`

// questionColumnsWithAudio 在 questionColumns 基础上追加 audio_state 列。
//
// audio_state 是可空列（未生成语音时为 NULL），Question 用 *string 承接，NULL 落成 nil。
const questionColumnsWithAudio = questionColumns + `, audio_state`

// Interview 是一次面试的落库视图。
//
// OverallGrade / OverallReview / CompletedAt 为 nil 表示该列尚未写入（数据库 NULL）。
type Interview struct {
	ID             int64      `db:"id" json:"id"`
	TenantID       int64      `db:"tenant_id" json:"-"`
	TargetJobID    int64      `db:"target_job_id" json:"targetJobId"`
	ResumeID       int64      `db:"resume_id" json:"resumeId"`
	ConfigSnapshot string     `db:"config_snapshot" json:"configSnapshot"`
	Status         string     `db:"status" json:"status"`
	OverallGrade   *string    `db:"overall_grade" json:"overallGrade"`
	OverallReview  *string    `db:"overall_review" json:"overallReview"`
	CreatedAt      *time.Time `db:"created_at" json:"createdAt"`
	UpdatedAt      *time.Time `db:"updated_at" json:"updatedAt"`
	CompletedAt    *time.Time `db:"completed_at" json:"completedAt"`
}

// Question 是面试中的一道题：主问题（ParentID 为 nil）或追问。
type Question struct {
	ID             int64         `db:"id" json:"id"`
	InterviewID    int64         `db:"interview_id" json:"interviewId"`
	ParentID       *int64        `db:"parent_id" json:"parentId"`
	FollowUpRound  int           `db:"follow_up_round" json:"followUpRound"`
	Label          QuestionLabel `db:"label" json:"label"`
	Question       string        `db:"question" json:"question"`
	StandardAnswer string        `db:"standard_answer" json:"standardAnswer"`
	UserAnswer     string        `db:"user_answer" json:"userAnswer"`
	Extra          string        `db:"extra" json:"extra"`
	Grade          string        `db:"grade" json:"grade"`
	CreatedAt      time.Time     `db:"created_at" json:"createdAt"`
	// AudioState 题目语音状态：NULL=未入队，pending=生成中，ready=已就绪，failed=重试耗尽。
	AudioState *string `db:"audio_state" json:"audioState"`
}

// MainQuestionInput 是一道主问题的落库内容（与生题函数的一条输出对应）。
type MainQuestionInput struct {
	// Label 题目标签，落库为 int(label)。
	Label QuestionLabel
	// Question 题面。
	Question string
	// StandardAnswer 标准答案（项目题可为空）。
	StandardAnswer string
	// MustPoints 必答点，落库到 extra 列的 must_points。
	MustPoints []string
	// BonusPoints 加分点，落库到 extra 列的 bonus_points。
	BonusPoints []string
}

// Repository 是 AI 模拟面试数据的持久化层。
type Repository struct {
	db *sqlx.DB
}

// NewRepository 创建 Repository。
func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{db: db}
}

// CreateInterview 创建一场面试，并落下 cfg.QuestionCount 道主问题占位行（题面留空，
// 待生题结果回填；标签按 cfg.LabelCounts 分布）。
//
// 事务语义：同一租户仅允许一个 status='active' 的面试，检查与插入在同一事务内完成，
// 命中已有 active 时返回 ErrActiveInterviewExists 且不写入任何行。
func (r *Repository) CreateInterview(ctx context.Context, tenantID int64, cfg InterviewConfig, targetJobID, resumeID int64) (*Interview, error) {
	return r.CreateInterviewWithQuestions(ctx, tenantID, cfg, targetJobID, resumeID, nil)
}

// CreateInterviewWithQuestions 在同一事务里完成「active 检查 + 面试行插入 + 主问题插入」。
//
// questions 为 nil 时按 cfg 生成占位主问题；questions 非 nil 时按传入内容落库
// （题面、标准答案、必答/加分点一起写入）。返回带自增 ID 的面试行。
func (r *Repository) CreateInterviewWithQuestions(ctx context.Context, tenantID int64, cfg InterviewConfig, targetJobID, resumeID int64, questions []MainQuestionInput) (*Interview, error) {
	snapshot, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}

	tx, err := store.BeginSQLiteWriteTx(ctx, r.db)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	// 先取 SQLite 写锁，让并发创建串行化（与 resume.lockTargetJobReference 同思路）：
	// 否则两个事务可能都在对方提交前读到「没有 active 面试」并各自插入。
	if _, err := tx.ExecContext(ctx, `UPDATE interviews SET updated_at = updated_at WHERE tenant_id = ?`, tenantID); err != nil {
		return nil, err
	}

	var activeID int64
	err = tx.GetContext(ctx, &activeID, `SELECT id FROM interviews WHERE tenant_id = ? AND status = 'active' LIMIT 1`, tenantID)
	switch {
	case err == nil:
		return nil, ErrActiveInterviewExists
	case !errors.Is(err, sql.ErrNoRows):
		return nil, err
	}

	now := time.Now().UTC()
	result, err := tx.ExecContext(ctx, `
		INSERT INTO interviews (tenant_id, target_job_id, resume_id, config_snapshot, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'active', ?, ?)`,
		tenantID, targetJobID, resumeID, string(snapshot), now, now)
	if err != nil {
		return nil, err
	}
	interviewID, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	rows := questions
	if rows == nil {
		rows = placeholderQuestions(cfg)
	}
	for _, row := range rows {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO interview_questions (interview_id, parent_id, follow_up_round, label, question, standard_answer, user_answer, extra, grade, created_at)
			VALUES (?, NULL, 0, ?, ?, ?, NULL, ?, NULL, ?)`,
			interviewID, int(row.Label), row.Question, nullableText(row.StandardAnswer), nullableExtra(row), now); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetInterview(ctx, tenantID, interviewID)
}

// GetInterview 按租户读取一场面试；不存在（或不属于该租户）返回 ErrNotFound。
func (r *Repository) GetInterview(ctx context.Context, tenantID, id int64) (*Interview, error) {
	var interview Interview
	err := r.db.GetContext(ctx, &interview, `
		SELECT `+interviewColumns+`
		FROM interviews WHERE id = ? AND tenant_id = ?`, id, tenantID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &interview, nil
}

// HasActiveInterview 报告租户下是否已有进行中的面试，供创建流程在生题前快速失败。
func (r *Repository) HasActiveInterview(ctx context.Context, tenantID int64) (bool, error) {
	var count int
	err := r.db.GetContext(ctx, &count, `
		SELECT COUNT(1) FROM interviews WHERE tenant_id = ? AND status = 'active'`, tenantID)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// ListInterviews 按 updated_at 倒序列出租户下的面试（updated_at 相同时按 id 倒序）。
func (r *Repository) ListInterviews(ctx context.Context, tenantID int64) ([]Interview, error) {
	list := []Interview{}
	err := r.db.SelectContext(ctx, &list, `
		SELECT `+interviewColumns+`
		FROM interviews WHERE tenant_id = ? ORDER BY updated_at DESC, id DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	return list, nil
}

// UpdateInterviewStatus 更新面试状态与总评。
//
// overallGrade / overallReview 为 nil 表示「不改动该列」；非 nil 则写入该值。
// status 为 completed / aborted 时补齐 completed_at（已有值不覆盖）。
// 目标行不存在返回 ErrNotFound。
func (r *Repository) UpdateInterviewStatus(ctx context.Context, id int64, status string, overallGrade, overallReview *string) error {
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, `
		UPDATE interviews
		SET status = ?,
		    overall_grade = COALESCE(?, overall_grade),
		    overall_review = COALESCE(?, overall_review),
		    updated_at = ?,
		    completed_at = CASE WHEN ? IN ('completed', 'aborted') THEN COALESCE(completed_at, ?) ELSE completed_at END
		WHERE id = ?`,
		status, nullableArg(overallGrade), nullableArg(overallReview), now, status, now, id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// GetInterviewByID 按 id 读取面试行，不带租户过滤。
//
// 仅供面试图节点等内部组件使用（Service 层在入口已完成租户归属校验）；
// 对外的租户隔离读取必须走 GetInterview。
func (r *Repository) GetInterviewByID(ctx context.Context, id int64) (*Interview, error) {
	var interview Interview
	err := r.db.GetContext(ctx, &interview, `
		SELECT `+interviewColumns+`
		FROM interviews WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &interview, nil
}

// MainQuestions 返回面试的主问题（parent_id IS NULL），按 id 升序。
func (r *Repository) MainQuestions(ctx context.Context, interviewID int64) ([]Question, error) {
	list := []Question{}
	err := r.db.SelectContext(ctx, &list, `
		SELECT `+questionColumns+`
		FROM interview_questions
		WHERE interview_id = ? AND parent_id IS NULL
		ORDER BY id`, interviewID)
	if err != nil {
		return nil, err
	}
	return list, nil
}

// QuestionChain 返回主问题及其追问（主问题在前），按 id 升序。
func (r *Repository) QuestionChain(ctx context.Context, interviewID, parentID int64) ([]Question, error) {
	list := []Question{}
	err := r.db.SelectContext(ctx, &list, `
		SELECT `+questionColumns+`
		FROM interview_questions
		WHERE interview_id = ? AND (id = ? OR parent_id = ?)
		ORDER BY id`, interviewID, parentID, parentID)
	if err != nil {
		return nil, err
	}
	return list, nil
}

// UpdateAnswer 回写一道题的用户回答与评分；目标行不存在返回 ErrNotFound。
func (r *Repository) UpdateAnswer(ctx context.Context, questionID int64, userAnswer, grade string) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE interview_questions
		SET user_answer = ?, grade = ?
		WHERE id = ?`, userAnswer, grade, questionID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateGrade 只更新一道题的评分（追问链 clamp 后回写父题等级用），不动用户回答。
func (r *Repository) UpdateGrade(ctx context.Context, questionID int64, grade string) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE interview_questions
		SET grade = ?
		WHERE id = ?`, grade, questionID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// SetMainQuestions 用生题结果回填面试的主问题（按 id 顺序与既有主问题一一对应）。
//
// 题目数超过既有主问题数返回错误；少于时只回填前若干条，其余保持原样。
func (r *Repository) SetMainQuestions(ctx context.Context, interviewID int64, questions []MainQuestionInput) error {
	tx, err := store.BeginSQLiteWriteTx(ctx, r.db)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var ids []int64
	if err := tx.SelectContext(ctx, &ids, `
		SELECT id FROM interview_questions
		WHERE interview_id = ? AND parent_id IS NULL
		ORDER BY id`, interviewID); err != nil {
		return err
	}
	if len(questions) > len(ids) {
		return errors.New("interview: more questions than existing main questions")
	}
	for i, row := range questions {
		if _, err := tx.ExecContext(ctx, `
			UPDATE interview_questions
			SET label = ?, question = ?, standard_answer = ?, extra = ?
			WHERE id = ? AND interview_id = ?`,
			int(row.Label), row.Question, nullableText(row.StandardAnswer), nullableExtra(row), ids[i], interviewID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// InsertFollowUp 插入一条追问：parent_id 指向主问题，label 继承父问题，轮次为 round。
// 父问题不存在（或不属于该面试）返回 ErrNotFound。
func (r *Repository) InsertFollowUp(ctx context.Context, interviewID, parentID int64, round int, question, standardAnswer, extra string) (int64, error) {
	tx, err := store.BeginSQLiteWriteTx(ctx, r.db)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	var label int
	err = tx.GetContext(ctx, &label, `
		SELECT label FROM interview_questions WHERE id = ? AND interview_id = ?`, parentID, interviewID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}

	now := time.Now().UTC()
	result, err := tx.ExecContext(ctx, `
		INSERT INTO interview_questions (interview_id, parent_id, follow_up_round, label, question, standard_answer, user_answer, extra, grade, created_at)
		VALUES (?, ?, ?, ?, ?, ?, NULL, ?, NULL, ?)`,
		interviewID, parentID, round, label, question, nullableText(standardAnswer), nullableText(extra), now)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

// ExtraFields 解析 extra 列的 {"must_points":[],"bonus_points":[]}。
// 空串或非法 JSON 时返回空结果，不报错。
func (r *Repository) ExtraFields(extra string) (must, bonus []string) {
	if extra == "" {
		return nil, nil
	}
	var payload struct {
		MustPoints  []string `json:"must_points"`
		BonusPoints []string `json:"bonus_points"`
	}
	if err := json.Unmarshal([]byte(extra), &payload); err != nil {
		return nil, nil
	}
	return payload.MustPoints, payload.BonusPoints
}

// UpdateAudioState 回写题目的语音状态（生成 worker 使用）。
func (r *Repository) UpdateAudioState(ctx context.Context, questionID int64, state string) error {
	if _, err := r.db.ExecContext(ctx, `UPDATE interview_questions SET audio_state = ? WHERE id = ?`, state, questionID); err != nil {
		return fmt.Errorf("interview: update audio state %d: %w", questionID, err)
	}
	return nil
}

// QuestionByID 按 id 读取题目行（含语音状态）；不存在返回 ErrNotFound。
func (r *Repository) QuestionByID(ctx context.Context, id int64) (Question, error) {
	var row Question
	err := r.db.GetContext(ctx, &row, `
		SELECT `+questionColumnsWithAudio+`
		FROM interview_questions WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Question{}, ErrNotFound
	}
	if err != nil {
		return Question{}, fmt.Errorf("interview: load question %d: %w", id, err)
	}
	return row, nil
}

// ActiveQuestionsNeedingAudio 返回「进行中面试里语音未就绪」的题目。
//
// pending 不在这里：它表示「已抢占生成权」，由数据库的抢占语义保证不会被重复领取；
// 崩溃残留的 pending 在启动时由 ResetStalePendingAudio 归零。
func (r *Repository) ActiveQuestionsNeedingAudio(ctx context.Context) ([]PendingAudio, error) {
	items := []PendingAudio{}
	err := r.db.SelectContext(ctx, &items, `
		SELECT i.tenant_id AS tenant_id, q.interview_id AS interview_id, q.id AS question_id, q.question AS text
		FROM interview_questions q
		JOIN interviews i ON i.id = q.interview_id
		WHERE i.status = 'active' AND (q.audio_state IS NULL OR q.audio_state = 'failed')
		ORDER BY i.id ASC, q.id ASC`)
	if err != nil {
		return nil, fmt.Errorf("interview: load questions needing audio: %w", err)
	}
	return items, nil
}

// ClaimAudioState 原子抢占题目的语音生成权。
//
// 只有「从未生成(NULL)」或「已失败(failed)」的题目能被抢到；抢到即置为 pending。
// 返回 true 表示本次调用获得了生成权。
//
// 数据库是唯一真源：重启重载与创建面试可能同时给同一题入队，
// 条件 UPDATE 保证只有一方拿到生成权，不会重复合成。
func (r *Repository) ClaimAudioState(ctx context.Context, questionID int64) (bool, error) {
	result, err := r.db.ExecContext(ctx,
		`UPDATE interview_questions SET audio_state = 'pending' WHERE id = ? AND (audio_state IS NULL OR audio_state = 'failed')`,
		questionID)
	if err != nil {
		return false, fmt.Errorf("interview: claim audio state %d: %w", questionID, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("interview: claim audio state %d rows: %w", questionID, err)
	}
	return affected == 1, nil
}

// ReleaseAudioState 归还生成权：把 pending 放回未生成(NULL)。
// 入队后没能真正处理（队列满、进程退出）时调用。
func (r *Repository) ReleaseAudioState(ctx context.Context, questionID int64) error {
	if _, err := r.db.ExecContext(ctx,
		`UPDATE interview_questions SET audio_state = NULL WHERE id = ? AND audio_state = 'pending'`,
		questionID); err != nil {
		return fmt.Errorf("interview: release audio state %d: %w", questionID, err)
	}
	return nil
}

// ResetStalePendingAudio 把「进行中面试」里卡在 pending 的题目重置为未生成(NULL)。
// 只在服务启动时调用：此时还没有 worker 在跑，pending 一定是上次崩溃留下的残留。
func (r *Repository) ResetStalePendingAudio(ctx context.Context) (int64, error) {
	result, err := r.db.ExecContext(ctx,
		`UPDATE interview_questions SET audio_state = NULL
		 WHERE audio_state = 'pending'
		   AND interview_id IN (SELECT id FROM interviews WHERE status = 'active')`)
	if err != nil {
		return 0, fmt.Errorf("interview: reset stale pending audio: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("interview: reset stale pending audio rows: %w", err)
	}
	return affected, nil
}

// ResetAudioStateByInterview 清空一场面试全部题目的语音状态（面试终态清理时调用）。
func (r *Repository) ResetAudioStateByInterview(ctx context.Context, interviewID int64) error {
	if _, err := r.db.ExecContext(ctx,
		`UPDATE interview_questions SET audio_state = NULL WHERE interview_id = ?`, interviewID); err != nil {
		return fmt.Errorf("interview: reset audio state of interview %d: %w", interviewID, err)
	}
	return nil
}

// placeholderQuestions 按 cfg.LabelCounts 生成占位主问题：标签顺序为八股→算法→项目，
// 数量不足 cfg.QuestionCount 时用八股题补齐。
func placeholderQuestions(cfg InterviewConfig) []MainQuestionInput {
	if cfg.QuestionCount <= 0 {
		return nil
	}
	labels := make([]QuestionLabel, 0, cfg.QuestionCount)
	for label := LabelOctopus; label <= LabelProject; label++ {
		for i := 0; i < cfg.LabelCounts[label]; i++ {
			labels = append(labels, label)
		}
	}
	for len(labels) < cfg.QuestionCount {
		labels = append(labels, LabelOctopus)
	}
	labels = labels[:cfg.QuestionCount]

	rows := make([]MainQuestionInput, 0, len(labels))
	for _, label := range labels {
		rows = append(rows, MainQuestionInput{Label: label})
	}
	return rows
}

// nullableText 把空串归一成 SQL NULL：尚未有值时列保持 NULL，读回仍是空串。
func nullableText(value string) any {
	if value == "" {
		return nil
	}
	return value
}

// nullableArg 把可选的 *string 变成可空参数（nil → SQL NULL）。
func nullableArg(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

// nullableExtra 把必答/加分点序列化成 extra 列的 JSON；两者都为空时列保持 NULL。
func nullableExtra(row MainQuestionInput) any {
	if len(row.MustPoints) == 0 && len(row.BonusPoints) == 0 {
		return nil
	}
	must := row.MustPoints
	if must == nil {
		must = []string{}
	}
	bonus := row.BonusPoints
	if bonus == nil {
		bonus = []string{}
	}
	payload, err := json.Marshal(struct {
		MustPoints  []string `json:"must_points"`
		BonusPoints []string `json:"bonus_points"`
	}{MustPoints: must, BonusPoints: bonus})
	if err != nil {
		return nil
	}
	return string(payload)
}
