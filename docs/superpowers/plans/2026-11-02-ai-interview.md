# AI 模拟面试 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 resume_pro 实现「基于目标 JD + 简历」的 AI 模拟面试：生题 → 逐题提问 → 逐题评分（C/B-/B/B+/A/S）→ 追问链 → 总评（中位数+比例门槛算法），全程落库可回看。

**Architecture:** 生题是一次性函数（LLM 调用 + schema 校验 + 事务落库）；面试是一个 Eino graph，通过 `WithGenLocalState` + `WithCheckPointStore`（redigo Redis）+ `Interrupt/ResumeWithData` 跨请求恢复，评分节点用 agenticopenai Responses API 按主问题独立会话续聊；`internal/interview` 新包承载全部逻辑，httpapi 只做薄封装。

**Tech Stack:** Go 1.26.5、gin、sqlx + modernc sqlite、eino v0.9.21、eino-ext agenticopenai v0.2.4（Responses API）、gomodule/redigo、React 前端。

**Spec:** `docs/superpowers/specs/2026-11-02-ai-interview-design.md`

## Global Constraints

- Go 模块 `resumepro/backend`，go 1.26.5；依赖升级：eino v0.9.15 → **v0.9.21**，新增 `github.com/cloudwego/eino-ext/components/model/agenticopenai@v0.2.4`、`github.com/gomodule/redigo@latest`。
- 本地优先：SQLite 单库（`store.OpenSQLite`），Redis 仅存 checkpoint（env：`REDIS_ADDR` 默认 `127.0.0.1:6379`、`REDIS_PASSWORD` 空、`REDIS_DB` 0）；Redis 不可用时面试接口返回 503，不影响其它功能。
- 等级 6 级枚举：`C B- B B+ A S`；序数 C=0, B-=1, B=2, B+=3, A=4, S=5。
- 总评算法：中位数定基础档（偶数取下档）+ 比例门槛（S 严格过半 `sCount*2>N`；A/B+ 至少一半 `>=N`），整数比较。
- 追问规则：预算全局扣减（默认 N/2）用尽禁止追问；单问上限默认 1；`grade>=A` 或 `followUp==""` 不追问；等级 clamp 只升不降。
- 同一 tenant 仅一个 `status=active` 面试（检查+插入同事务）；同一面试 answer 并发用 Redis SETNX 锁（TTL 60s）。
- 所有接口 `requireAuthentication`，按 tenant 隔离；前端 UI 只做薄展示。
- 提交 message 中文 `type(scope): 说明`；每任务一个 commit；不写 co-author。

---

### Task 1: 依赖升级（eino v0.9.21 + agenticopenai + redigo）

**Files:**
- Modify: `backend/go.mod`、`backend/go.sum`

**Interfaces:**
- Consumes: 无（基础任务）
- Produces: 编译可通过的依赖集

- [ ] **Step 1: 升级依赖**

Run:
```bash
cd backend
go get github.com/cloudwego/eino@v0.9.21
go get github.com/cloudwego/eino-ext/components/model/agenticopenai@v0.2.4
go get github.com/gomodule/redigo@latest
go mod tidy
```

- [ ] **Step 2: 全量编译与测试**

Run: `cd backend && go build ./... && go test ./...`
Expected: 全部编译通过、测试全绿（eino v0.9.15 → v0.9.21 接口兼容，现有 ai 包不受影响）。

- [ ] **Step 3: Commit**

```bash
git add backend/go.mod backend/go.sum
git commit -m "chore(deps): eino 升级 v0.9.21，新增 agenticopenai 与 redigo"
```

---

### Task 2: SQLite 迁移（interviews / interview_questions 两表）

**Files:**
- Modify: `backend/internal/store/migrate.go`（schema 常量追加两表）

**Interfaces:**
- Produces: 表 `interviews`（id/tenant_id/target_job_id/resume_id/config_snapshot/status/overall_grade/overall_review/created_at/updated_at/completed_at）；表 `interview_questions`（id/interview_id/parent_id/follow_up_round/label/question/standard_answer/user_answer/extra/grade/created_at）

- [ ] **Step 1: 在 schema 常量追加建表语句**

在 `internal/store/migrate.go` 的 `schema` 常量末尾（`idx_users_tenant_id` 之后）追加：

```sql
CREATE TABLE IF NOT EXISTS interviews (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  tenant_id INTEGER NOT NULL REFERENCES tenants(id),
  target_job_id INTEGER NOT NULL REFERENCES target_jobs(id),
  resume_id INTEGER NOT NULL REFERENCES resumes(id),
  config_snapshot TEXT NOT NULL,
  status TEXT NOT NULL,
  overall_grade TEXT,
  overall_review TEXT,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  completed_at DATETIME
);
CREATE TABLE IF NOT EXISTS interview_questions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  interview_id INTEGER NOT NULL REFERENCES interviews(id) ON DELETE CASCADE,
  parent_id INTEGER REFERENCES interview_questions(id),
  follow_up_round INTEGER NOT NULL DEFAULT 0,
  label INTEGER NOT NULL,
  question TEXT NOT NULL,
  standard_answer TEXT,
  user_answer TEXT,
  extra TEXT,
  grade TEXT,
  created_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_interview_questions_interview ON interview_questions(interview_id);
```

- [ ] **Step 2: 写迁移测试**

Create `backend/internal/store/migrate_test.go`（若不存在则新建；已有则追加用例）：

```go
func TestMigrateCreatesInterviewTables(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	var n int
	if err := db.GetContext(ctx, &n, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='interviews'`); err != nil || n != 1 {
		t.Fatalf("interviews table missing: n=%d err=%v", n, err)
	}
	if err := db.GetContext(ctx, &n, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='interview_questions'`); err != nil || n != 1 {
		t.Fatalf("interview_questions table missing: n=%d err=%v", n, err)
	}
}
```

（`newTestDB` 按该文件现有测试辅助函数复用；若无则用 `store.OpenSQLite(":memory:")` + `Migrate`。）

- [ ] **Step 3: 跑测试确认通过**

Run: `cd backend && go test ./internal/store/ -run TestMigrateCreatesInterviewTables -v`
Expected: PASS。

- [ ] **Step 4: Commit**

```bash
git add backend/internal/store/migrate.go backend/internal/store/migrate_test.go
git commit -m "feat(store): 新增 interviews 与 interview_questions 表迁移"
```

---

### Task 3: interview 包骨架 + 模型类型 + 总评算法（纯函数）

**Files:**
- Create: `backend/internal/interview/model.go`、`backend/internal/interview/grading.go`
- Create: `backend/internal/interview/grading_test.go`

**Interfaces:**
- Produces:
  - `type Grade string`；`const (GradeC Grade = "C"; GradeBMinus = "B-"; GradeB = "B"; GradeBPlus = "B+"; GradeA = "A"; GradeS = "S")`
  - `func GradeOrder(g Grade) int`（C=0…S=5）
  - `func ClampGrade(parent, current Grade) Grade`（返回较大者）
  - `func IsGradeAtLeast(g Grade, threshold Grade) bool`
  - `func ComputeOverallGrade(grades []Grade) Grade`（中位数+比例门槛）
  - `func ValidGrade(s string) bool`
  - `type QuestionLabel int`；`const (LabelOctopus QuestionLabel = iota; LabelAlgorithm; LabelProject)`
  - `type InterviewConfig struct { QuestionCount int; LabelCounts map[QuestionLabel]int; FollowUpBudget int; PerQuestionFollowUpLimit int }`
  - `var DefaultInterviewConfig = InterviewConfig{QuestionCount: 12, LabelCounts: map[QuestionLabel]int{LabelOctopus: 5, LabelAlgorithm: 2, LabelProject: 5}, FollowUpBudget: -1, PerQuestionFollowUpLimit: 1}`
  - `func (c InterviewConfig) Resolved() InterviewConfig`（FollowUpBudget<0 时置为 QuestionCount/2）

- [ ] **Step 1: 写总评算法测试（红）**

Create `grading_test.go`：

```go
package interview

import "testing"

func TestComputeOverallGrade(t *testing.T) {
	cases := []struct {
		name   string
		grades []Grade
		want   Grade
	}{
		{"3S9B -> B", []Grade{GradeS, GradeS, GradeS, GradeB, GradeB, GradeB, GradeB, GradeB, GradeB, GradeB, GradeB, GradeB}, GradeB},
		{"5S7A -> A", []Grade{GradeS, GradeS, GradeS, GradeS, GradeS, GradeA, GradeA, GradeA, GradeA, GradeA, GradeA, GradeA}, GradeA},
		{"8S4A -> S", []Grade{GradeS, GradeS, GradeS, GradeS, GradeS, GradeS, GradeS, GradeS, GradeA, GradeA, GradeA, GradeA}, GradeS},
		{"6S6A -> A (even median takes lower)", []Grade{GradeS, GradeS, GradeS, GradeS, GradeS, GradeS, GradeA, GradeA, GradeA, GradeA, GradeA, GradeA}, GradeA},
		{"all A -> A", []Grade{GradeA, GradeA, GradeA, GradeA, GradeA, GradeA}, GradeA},
		{"3S9B+ -> B+", []Grade{GradeS, GradeS, GradeS, GradeBPlus, GradeBPlus, GradeBPlus, GradeBPlus, GradeBPlus, GradeBPlus, GradeBPlus, GradeBPlus, GradeBPlus}, GradeBPlus},
		{"single B", []Grade{GradeB}, GradeB},
		{"empty -> C", nil, GradeC},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ComputeOverallGrade(tc.grades); got != tc.want {
				t.Errorf("ComputeOverallGrade(%v) = %v, want %v", tc.grades, got, tc.want)
			}
		})
	}
}

func TestClampGrade(t *testing.T) {
	if got := ClampGrade(GradeB, GradeBPlus); got != GradeBPlus {
		t.Errorf("ClampGrade(B, B+) = %v, want B+", got)
	}
	if got := ClampGrade(GradeA, GradeB); got != GradeA {
		t.Errorf("ClampGrade(A, B) = %v, want A", got)
	}
	if got := ClampGrade(GradeC, GradeC); got != GradeC {
		t.Errorf("ClampGrade(C, C) = %v, want C", got)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd backend && go test ./internal/interview/ -run TestComputeOverallGrade -v`
Expected: FAIL（`ComputeOverallGrade` 未定义）。

- [ ] **Step 3: 写 model.go 与 grading.go（实现）**

`model.go` 放 Grade/QuestionLabel/InterviewConfig 类型与常量（见 Interfaces）。`grading.go`：

```go
package interview

import "sort"

func GradeOrder(g Grade) int {
	switch g {
	case GradeC: return 0
	case GradeBMinus: return 1
	case GradeB: return 2
	case GradeBPlus: return 3
	case GradeA: return 4
	case GradeS: return 5
	}
	return 0
}

func ValidGrade(s string) bool {
	switch Grade(s) {
	case GradeC, GradeBMinus, GradeB, GradeBPlus, GradeA, GradeS:
		return true
	}
	return false
}

func IsGradeAtLeast(g, threshold Grade) bool { return GradeOrder(g) >= GradeOrder(threshold) }

func ClampGrade(parent, current Grade) Grade {
	if GradeOrder(current) > GradeOrder(parent) {
		return current
	}
	return parent
}

func ComputeOverallGrade(grades []Grade) Grade {
	n := len(grades)
	if n == 0 {
		return GradeC
	}
	orders := make([]int, n)
	sCount, aOrAbove, bPlusOrAbove := 0, 0, 0
	for i, g := range grades {
		o := GradeOrder(g)
		orders[i] = o
		if o == 5 { sCount++ }
		if o >= 4 { aOrAbove++ }
		if o >= 3 { bPlusOrAbove++ }
	}
	sort.Ints(orders)
	median := orders[n/2] // 奇数正中间；偶数取下档（Go 整数除法天然取下标）
	base := GradeFromOrder(median)
	switch {
	case GradeOrder(base) >= GradeOrder(GradeA) && sCount*2 > n:
		return GradeS
	case GradeOrder(base) >= GradeOrder(GradeBPlus) && aOrAbove*2 >= n:
		return GradeA
	case GradeOrder(base) >= GradeOrder(GradeB) && bPlusOrAbove*2 >= n:
		return GradeBPlus
	}
	return base
}

func GradeFromOrder(o int) Grade {
	switch o {
	case 5: return GradeS
	case 4: return GradeA
	case 3: return GradeBPlus
	case 2: return GradeB
	case 1: return GradeBMinus
	}
	return GradeC
}
```

> 注意：`n/2` 对偶数 N 取下标 `N/2-1`（如 N=12 → orders[5]），正是「偶数取下档」；奇数 N=13 → orders[6] 正中间。与 spec 一致。

- [ ] **Step 4: 跑测试确认通过**

Run: `cd backend && go test ./internal/interview/ -v`
Expected: PASS（含 ClampGrade）。

- [ ] **Step 5: Commit**

```bash
git add backend/internal/interview/
git commit -m "feat(interview): 模型类型与总评算法（中位数+比例门槛）"
```

---

### Task 4: Repository（SQLite 读写）

**Files:**
- Create: `backend/internal/interview/repository.go`、`backend/internal/interview/repository_test.go`

**Interfaces:**
- Consumes: Task 2 迁移、Task 3 类型
- Produces:
  - `type Interview struct { ID, TenantID, TargetJobID, ResumeID int64; ConfigSnapshot, Status string; OverallGrade, OverallReview *string; CreatedAt, UpdatedAt, CompletedAt *time.Time }`（db tag 与列名一致）
  - `type Question struct { ID, InterviewID int64; ParentID *int64; FollowUpRound int; Label QuestionLabel; Question, StandardAnswer, UserAnswer, Extra, Grade string }`
  - `func NewRepository(db *sqlx.DB) *Repository`
  - `func (r *Repository) CreateInterview(ctx, tenantID int64, cfg InterviewConfig, targetJobID, resumeID int64) (*Interview, error)`——**同事务**：先 `SELECT id FROM interviews WHERE tenant_id=? AND status='active'`（有则返回 `ErrActiveInterviewExists`），再插入面试 + 批量插入主问题（`parent_id=NULL, follow_up_round=0`）
  - `func (r *Repository) GetInterview(ctx, tenantID, id int64) (*Interview, error)`（无行 → `ErrNotFound`）
  - `func (r *Repository) ListInterviews(ctx, tenantID int64) ([]Interview, error)`（`ORDER BY updated_at DESC`）
  - `func (r *Repository) UpdateInterviewStatus(ctx, id int64, status string, overallGrade, overallReview *string) error`
  - `func (r *Repository) MainQuestions(ctx, interviewID int64) ([]Question, error)`（`WHERE interview_id=? AND parent_id IS NULL ORDER BY id`）
  - `func (r *Repository) QuestionChain(ctx, interviewID, parentID int64) ([]Question, error)`（主问题 + 追问，`WHERE interview_id=? AND (id=? OR parent_id=?) ORDER BY id`）
  - `func (r *Repository) UpdateAnswer(ctx, questionID int64, userAnswer, grade string) error`
  - `func (r *Repository) InsertFollowUp(ctx, interviewID, parentID int64, round int, question, standardAnswer, extra string) (int64, error)`
  - `var ErrActiveInterviewExists = errors.New("active interview already exists")`；`var ErrNotFound = errors.New("not found")`（复用 targetjob 风格的错误声明）
  - `func (r *Repository) ExtraFields(extra string) (must, bonus []string)`（解析 `{"must_points":[],"bonus_points":[]}`，容错）

- [ ] **Step 1: 写 repository 测试（红）**

Create `repository_test.go`（用 `store.OpenSQLite(":memory:")` + `store.Migrate` 建测试库）：

```go
package interview

import (
	"context"
	"errors"
	"testing"

	"github.com/jmoiron/sqlx"

	"resumepro/backend/internal/store"
)

func newTestRepo(t *testing.T) (*Repository, *sqlx.DB) {
	t.Helper()
	db, err := store.OpenSQLite(":memory:")
	if err != nil { t.Fatalf("open: %v", err) }
	if err := store.Migrate(context.Background(), db); err != nil { t.Fatalf("migrate: %v", err) }
	return NewRepository(db), db
}

func TestCreateInterviewAndQuestions(t *testing.T) {
	repo, db := newTestRepo(t)
	defer db.Close()
	cfg := DefaultInterviewConfig.Resolved()
	iv, err := repo.CreateInterview(context.Background(), 1, cfg, 1, 1)
	if err != nil { t.Fatalf("create: %v", err) }
	if iv.Status != "active" { t.Errorf("status = %s, want active", iv.Status) }
	qs, err := repo.MainQuestions(context.Background(), iv.ID)
	if err != nil { t.Fatalf("main questions: %v", err) }
	if len(qs) != cfg.QuestionCount { t.Errorf("questions = %d, want %d", len(qs), cfg.QuestionCount) }
	for i, q := range qs {
		if q.ParentID != nil { t.Errorf("q[%d] parent not nil", i) }
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
	if err != nil { t.Fatalf("insert follow-up: %v", err) }
	chain, err := repo.QuestionChain(context.Background(), iv.ID, parentID)
	if err != nil { t.Fatalf("chain: %v", err) }
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
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd backend && go test ./internal/interview/ -run TestCreateInterview -v`
Expected: FAIL（`CreateInterview` 未定义）。

- [ ] **Step 3: 实现 repository.go**

按现有 targetjob 模式（`sqlx` + `store.BeginSQLiteWriteTx`），注意 CreateInterview 事务内先查 active 再插入；config_snapshot 用 `json.Marshal(cfg)`；主问题 batch 用循环 `INSERT`（数量 ≤ 20，无需 ExecBatch）。label 列存 `int(label)`，读回用 `QuestionLabel(row.Label)`。

- [ ] **Step 4: 跑测试确认通过**

Run: `cd backend && go test ./internal/interview/ -v`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add backend/internal/interview/repository.go backend/internal/interview/repository_test.go
git commit -m "feat(interview): repository 落库（事务建面试、追问树、答案回写）"
```

---

### Task 5: Redis CheckPointStore（redigo）

**Files:**
- Create: `backend/internal/interview/redis_store.go`、`backend/internal/interview/redis_store_test.go`

**Interfaces:**
- Consumes: 无（独立）
- Produces:
  - `func NewRedisStore(addr, password string, db int) (*RedisStore, error)`（`redigo.Dial`，`ping` 验证，`err` 非 nil 表示 Redis 不可用）
  - `type RedisStore struct{ pool *redigo.Pool }`
  - `func (s *RedisStore) Get(ctx, checkPointID string) ([]byte, bool, error)`（`SETEX` 之外用 `GET`；key = `interview:cp:{id}`；无值 → `(nil, false, nil)`）
  - `func (s *RedisStore) Set(ctx, checkPointID string, cp []byte) error`（`SET` + `EXPIRE 7d`）
  - `func (s *RedisStore) Delete(ctx, checkPointID string) error`
  - `func (s *RedisStore) Close() error`
  - 并发锁辅助：`func (s *RedisStore) TryLock(ctx, key string, ttlSeconds int) (bool, error)`（`SET key 1 NX EX ttl`）、`func (s *RedisStore) Unlock(ctx, key string) error`

- [ ] **Step 1: 写测试（红）——需要本地 Redis**

测试要求本地 Redis（`127.0.0.1:6379`）。若环境无 Redis，测试跳过（`t.Skip`）：

```go
package interview

import (
	"context"
	"testing"
)

func redisStore(t *testing.T) *RedisStore {
	t.Helper()
	s, err := NewRedisStore("127.0.0.1:6379", "", 0)
	if err != nil {
		t.Skipf("redis unavailable: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestRedisStoreRoundTrip(t *testing.T) {
	s := redisStore(t)
	ctx := context.Background()
	if _, ok, _ := s.Get(ctx, "t-cp-1"); ok {
		t.Fatal("fresh key should miss")
	}
	if err := s.Set(ctx, "t-cp-1", []byte("payload")); err != nil {
		t.Fatalf("set: %v", err)
	}
	v, ok, err := s.Get(ctx, "t-cp-1")
	if err != nil || !ok || string(v) != "payload" {
		t.Fatalf("get = %q ok=%v err=%v", v, ok, err)
	}
	if err := s.Delete(ctx, "t-cp-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, ok, _ := s.Get(ctx, "t-cp-1"); ok {
		t.Fatal("key should be gone after delete")
	}
}

func TestRedisStoreTryLock(t *testing.T) {
	s := redisStore(t)
	ctx := context.Background()
	ok1, err := s.TryLock(ctx, "t-lock-1", 5)
	if err != nil || !ok1 {
		t.Fatalf("first lock: ok=%v err=%v", ok1, err)
	}
	ok2, _ := s.TryLock(ctx, "t-lock-1", 5)
	if ok2 {
		t.Fatal("second lock should fail")
	}
	_ = s.Unlock(ctx, "t-lock-1")
	ok3, _ := s.TryLock(ctx, "t-lock-1", 5)
	if !ok3 {
		t.Fatal("lock should succeed after unlock")
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd backend && go test ./internal/interview/ -run TestRedisStore -v`
Expected: 有 Redis 时 FAIL（`NewRedisStore` 未定义）；无 Redis 时 SKIP（可接受，但实现完需在本地验证）。

- [ ] **Step 3: 实现 redis_store.go**

redigo 用法：

```go
package interview

import (
	"context"
	"fmt"
	"time"

	"github.com/gomodule/redigo/redis"
)

const checkpointKeyPrefix = "interview:cp:"
const checkpointTTL = 7 * 24 * time.Hour

type RedisStore struct{ pool *redis.Pool }

func NewRedisStore(addr, password string, db int) (*RedisStore, error) {
	pool := &redis.Pool{
		MaxIdle: 3, IdleTimeout: 60 * time.Second,
		Dial: func() (redis.Conn, error) {
			c, err := redis.Dial("tcp", addr,
				redis.DialPassword(password),
				redis.DialDatabase(db),
				redis.DialConnectTimeout(3*time.Second))
			if err != nil { return nil, err }
			return c, nil
		},
	}
	c := pool.Get()
	defer c.Close()
	if _, err := c.Do("PING"); err != nil {
		_ = pool.Close()
		return nil, fmt.Errorf("redis ping: %w", err)
	}
	return &RedisStore{pool: pool}, nil
}

func (s *RedisStore) Close() error { return s.pool.Close() }

func (s *RedisStore) Get(_ context.Context, checkPointID string) ([]byte, bool, error) {
	c := s.pool.Get()
	defer c.Close()
	v, err := redis.Bytes(c.Do("GET", checkpointKeyPrefix+checkPointID))
	if err == redis.ErrNil { return nil, false, nil }
	if err != nil { return nil, false, err }
	return v, true, nil
}

func (s *RedisStore) Set(_ context.Context, checkPointID string, cp []byte) error {
	c := s.pool.Get()
	defer c.Close()
	_, err := c.Do("SET", checkpointKeyPrefix+checkPointID, cp, "EX", int(checkpointTTL.Seconds()))
	return err
}

func (s *RedisStore) Delete(_ context.Context, checkPointID string) error {
	c := s.pool.Get()
	defer c.Close()
	_, err := c.Do("DEL", checkpointKeyPrefix+checkPointID)
	return err
}

func (s *RedisStore) TryLock(_ context.Context, key string, ttlSeconds int) (bool, error) {
	c := s.pool.Get()
	defer c.Close()
	v, err := redis.String(c.Do("SET", "interview:lock:"+key, "1", "NX", "EX", ttlSeconds))
	if err == redis.ErrNil { return false, nil }
	if err != nil { return false, err }
	return v == "OK", nil
}

func (s *RedisStore) Unlock(_ context.Context, key string) error {
	c := s.pool.Get()
	defer c.Close()
	_, err := c.Do("DEL", "interview:lock:"+key)
	return err
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `cd backend && go test ./internal/interview/ -run TestRedisStore -v`
Expected: PASS（本地需 Redis；本机已有 `redis-server` 或 docker 起的实例则直接跑）。

- [ ] **Step 5: Commit**

```bash
git add backend/internal/interview/redis_store.go backend/internal/interview/redis_store_test.go
git commit -m "feat(interview): Redis CheckPointStore 与 SETNX 并发锁（redigo）"
```

---

### Task 6: 生题函数（LLM 调用 + schema 校验）

**Files:**
- Create: `backend/internal/interview/question_gen.go`、`backend/internal/interview/question_gen_test.go`
- Modify: `backend/internal/llm/client.go`（新增 `NewResponsesModel` 工厂，复用 env 配置）

**Interfaces:**
- Consumes: Task 3 类型；现有 `internal/llm.Config` / `internal/ai/prompts` 模式
- Produces:
  - `type GeneratedQuestion struct { Question string; StandardAnswer string; Label QuestionLabel; MustPoints []string; BonusPoints []string }`
  - `type ResponsesModel interface { Generate(ctx context.Context, in []*schema.AgenticMessage, opts ...model.Option) (*schema.AgenticMessage, error) }`（便于 fake）
  - `type QuestionGenerator struct { cm ResponsesModel; maxRetries int }`
  - `func NewQuestionGenerator(cm ResponsesModel, maxRetries int) *QuestionGenerator`
  - `func (g *QuestionGenerator) GenerateQuestions(ctx, resumeText string, jd *JDContext, cfg InterviewConfig) ([]GeneratedQuestion, error)`
  - `type JDContext struct { Company, Position, Requirements string }`
  - `llm.NewResponsesModel(llm.Config) (model.AgenticModel, error)`（agenticopenai.NewResponsesModel）

- [ ] **Step 1: 写 fake 模型与生题测试（红）**

Create `question_gen_test.go`：

```go
package interview

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/cloudwego/eino/schema"
)

type fakeResponses struct{ payload string }

func (f *fakeResponses) Generate(_ context.Context, _ []*schema.AgenticMessage, _ ...model.Option) (*schema.AgenticMessage, error) {
	return schema.AssistantAgenticMessage(f.payload), nil
}

func TestGenerateQuestions(t *testing.T) {
	payload, _ := json.Marshal(map[string]any{
		"questions": []map[string]any{
			{"question": "Go 的 channel 底层结构？", "standardAnswer": "hchan 结构", "label": 0,
			 "mustPoints": []string{"hchan"}, "bonusPoints": []string{"sendq/recvq"}},
			{"question": "项目里的缓存怎么做？", "standardAnswer": "", "label": 2,
			 "mustPoints": []string{"场景"}, "bonusPoints": []string{"布隆过滤器"}},
		},
	})
	g := NewQuestionGenerator(&fakeResponses{payload: string(payload)}, 1)
	got, err := g.GenerateQuestions(context.Background(), "简历文本", &JDContext{Company: "X"}, DefaultInterviewConfig.Resolved())
	if err != nil { t.Fatalf("generate: %v", err) }
	if len(got) != 2 { t.Fatalf("got %d questions", len(got)) }
	if got[0].Label != LabelOctopus || got[1].Label != LabelProject {
		t.Errorf("labels = %d, %d", got[0].Label, got[1].Label)
	}
	if got[0].StandardAnswer == "" { t.Error("八股题应有标准答案") }
	if got[1].StandardAnswer != "" { t.Error("项目题标准答案应为空") }
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd backend && go test ./internal/interview/ -run TestGenerateQuestions -v`
Expected: FAIL。

- [ ] **Step 3: 实现 question_gen.go**

- prompt 用现有 `internal/ai/prompts` 的 `embed.go` 模式（`//go:embed *.tmpl`）——新建 `backend/internal/interview/prompts/questions.tmpl`，内容按设计文档生题 prompt（JD + 简历 + 配置，输出 `{"questions":[{question,standardAnswer,label,mustPoints,bonusPoints}]}`）。
- 调用 `g.cm.Generate(ctx, []*schema.AgenticMessage{system, user}, llm.WithJSONSchemaResponse(...))`；schema 用 `eino-contrib/jsonschema`（现有 `internal/ai/schemas` 模式）。
- 校验：`label` 转 `QuestionLabel` 必须 ∈ {0,1,2}；`question` 非空；**八股/算法题 `standardAnswer` 必须非空，项目题允许空**；数量等于 `cfg.QuestionCount`（不足或超出 → 重试一次，仍失败 → error）。
- `maxRetries` 复用现有契约修复模式（失败追加纠正消息再调一次）。

- [ ] **Step 4: 跑测试确认通过**

Run: `cd backend && go test ./internal/interview/ -v`
Expected: PASS。

- [ ] **Step 5: 实现 llm.NewResponsesModel 工厂**

在 `internal/llm/client.go` 追加：

```go
// NewResponsesModel creates an agenticopenai Responses API model from config.
func NewResponsesModel(cfg Config) (model.AgenticModel, error) {
	return agenticopenai.NewResponsesModel(context.Background(), &agenticopenai.ResponsesConfig{
		APIKey:   cfg.APIKey,
		BaseURL:  cfg.BaseURL,
		Model:    cfg.Model,
		Timeout:  &cfg.Timeout,
	})
}
```

（import：`github.com/cloudwego/eino-ext/components/model/agenticopenai`、`github.com/cloudwego/eino/components/model`。注意 `model.AgenticModel` 与现有 `model.ChatModel` 并存。）

- [ ] **Step 6: 编译 + 全量测试**

Run: `cd backend && go build ./... && go test ./internal/...`
Expected: 全绿。

- [ ] **Step 7: Commit**

```bash
git add backend/internal/interview/question_gen.go backend/internal/interview/question_gen_test.go backend/internal/interview/prompts/ backend/internal/llm/client.go
git commit -m "feat(interview): 生题函数（Responses 调用 + schema 校验），llm 增加 ResponsesModel 工厂"
```

---

### Task 7: 面试图（Eino graph：ask → grade → decide → review → finish）

**Files:**
- Create: `backend/internal/interview/graph.go`、`backend/internal/interview/graph_test.go`

**Interfaces:**
- Consumes: Task 3/4/5/6（Repository、RedisStore、QuestionGenerator、模型类型）
- Produces:
  - `type InterviewState struct { InterviewID int64; Index int; FollowUpRound int; FollowUpBudgetLeft int; ResponseID string }`
  - `type answerPayload struct { Answer string; Skip bool }`
  - `type gradeResult struct { Grade Grade; FollowUp string }`
  - `func BuildGraph(repo *Repository, cm ResponsesModel) (*compose.Graph, error)`（编译后返回；checkpoint store 在运行时经 option 注入，编译不依赖 store）
  - `func (g *Graph) StartInterview(ctx, interviewID int64) (*schema.AgenticMessage, error)`（`Invoke` 首轮，返回第一题）
  - `func (g *Graph) Answer(ctx, interviewID int64, payload answerPayload) (graphResult, error)`
  - `type graphResult struct { Grade Grade; FollowUp string; NextQuestion *Question; Finished bool; OverallGrade Grade; OverallReview string }`

- [ ] **Step 1: 写图测试（红）——用 fake 模型驱动单轮**

```go
package interview

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestGraphSingleTurn(t *testing.T) {
	repo, db := newTestRepo(t)
	defer db.Close()
	cfg := DefaultInterviewConfig.Resolved()
	iv, err := repo.CreateInterview(context.Background(), 1, cfg, 1, 1)
	if err != nil { t.Fatalf("create: %v", err) }
	payload, _ := json.Marshal(map[string]any{"grade": "B+", "followUp": ""})
	graph, err := BuildGraph(repo, &fakeResponses{payload: string(payload)})
	if err != nil { t.Fatalf("build: %v", err) }
	// 首轮：拿到第一题
	first, err := graph.StartInterview(context.Background(), iv.ID)
	if err != nil { t.Fatalf("start: %v", err) }
	if first == nil || first.Content == "" { t.Fatal("first question empty") }
	// 回答：B+ 无追问 → 应推进到第二题
	res, err := graph.Answer(context.Background(), iv.ID, answerPayload{Answer: "channel 是 hchan"})
	if err != nil { t.Fatalf("answer: %v", err) }
	if res.Finished || res.NextQuestion == nil {
		t.Fatalf("expected next question, got %+v", res)
	}
	if res.Grade != GradeBPlus { t.Errorf("grade = %s, want B+", res.Grade) }
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd backend && go test ./internal/interview/ -run TestGraphSingleTurn -v`
Expected: FAIL（`BuildGraph` 未定义）。

- [ ] **Step 3: 实现 graph.go**

关键结构（Eino API 按 v0.9.21，与设计文档「Eino Graph 节点编排图」一一对应）：

```go
package interview

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

type InterviewState struct {
	InterviewID        int64  `json:"interview_id"`
	Index              int    `json:"index"`
	FollowUpRound      int    `json:"follow_up_round"`
	FollowUpBudgetLeft int    `json:"follow_up_budget_left"`
	ResponseID         string `json:"response_id,omitempty"`
}

type answerPayload struct {
	Answer string `json:"answer"`
	Skip   bool   `json:"skip"`
}

type gradeResult struct {
	Grade    Grade  `json:"grade"`
	FollowUp string `json:"followUp"`
}

type Graph struct {
	graph *compose.Graph
	repo  *Repository
	cm    ResponsesModel
}

// BuildGraph builds the interview graph with nodes bound to repo/model.
func BuildGraph(repo *Repository, cm ResponsesModel) (*Graph, error) {
	g := compose.NewGraph[string, string]()
	// ask_question: reads SQLite by state.Index, interrupts with the question message
	if err := g.AddLambdaNode("ask_question", ...); err != nil { return nil, err }
	// grade_answer: LLM (skip -> no call), outputs {grade, followUp}
	if err := g.AddLambdaNode("grade_answer", ...); err != nil { return nil, err }
	// decide_follow_up: pure rule, advances state, writes SQLite
	if err := g.AddLambdaNode("decide_follow_up", ...); err != nil { return nil, err }
	// generate_review: LLM overall_review; overall_grade computed server-side
	if err := g.AddLambdaNode("generate_review", ...); err != nil { return nil, err }
	// finish: writes completed status, deletes checkpoint
	if err := g.AddLambdaNode("finish", ...); err != nil { return nil, err }
	if err := g.AddEdge(compose.START, "ask_question"); err != nil { return nil, err }
	if err := g.AddEdge("grade_answer", "decide_follow_up"); err != nil { return nil, err }
	if err := g.AddEdge("decide_follow_up", "ask_question"); err != nil { return nil, err }
	if err := g.AddEdge("decide_follow_up", "generate_review"); err != nil { return nil, err }
	if err := g.AddEdge("generate_review", "finish"); err != nil { return nil, err }
	return &Graph{graph: g, repo: repo, cm: cm}, nil
}
```

实现要点（节点函数签名以 eino `compose.InvokableLambda[I, O]` 为准，状态通过 `WithStatePreHandler/PostHandler` + `WithGenLocalState` 注入，编译期不绑定 store，运行时在 `StartInterview/Answer` 里用 `compose.WithCheckPointStore` + `WithCheckPointID` + `WithSerializer` 传入）：

- **ask_question**：`StatefulInterrupt(ctx, info=问题消息, state)`；恢复时 `GetResumeContext[answerPayload]` 取用户输入。
- **grade_answer**：`skip=true` → 直接 `{Grade: 结算等级, FollowUp: ""}`（首答 C / 追问中父等级）；否则首次拼完整上下文 + 回答，追问轮 `WithHeadPreviousResponseID(state.ResponseID)` 只传回答；解析 `ResponseMeta.OpenAIExtension.ID` 更新 `state.ResponseID`；`{grade, followUp}` JSON 校验 + 修复重试一次。
- **decide_follow_up**：按 4 条件判定；追问 → `InsertFollowUp` + 预算/轮次推进，返回「去 ask」；否则 clamp 回写主问题行、Index+1、ResponseID 清空、轮次归零；Index>=N → 「去 generate_review」。
- **generate_review**：读全部主问题 → LLM 生成 `overall_review` 文本；`ComputeOverallGrade` 算 `overall_grade`。
- **finish**：`UpdateInterviewStatus(completed, &grade, &review)`；`store.Delete(interviewID)`。
- **StartInterview**：`graph.Invoke(ctx, "", compose.WithCheckPointID(strconv(interviewID)), ...)`，从返回的 `InterruptSignal` 提取首题消息。
- **Answer**：`graph.ResumeWithData(ctx, checkPointID, payload, ...)`，从再次中断或 `finish` 结果组装 `graphResult`。

> 若 v0.9.21 的 `ResumeWithData` 签名与设计文档查证时不同，以实际 API 为准适配（计划执行者需读 `compose/resume.go` 确认参数）。`compose.Graph` 泛型实例化可用 `compose.NewGraph[string, string]`。

- [ ] **Step 4: 跑测试确认通过**

Run: `cd backend && go test ./internal/interview/ -run TestGraphSingleTurn -v`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add backend/internal/interview/graph.go backend/internal/interview/graph_test.go
git commit -m "feat(interview): 面试图五个节点（ask/grade/decide/review/finish）与 interrupt/resume"
```

---

### Task 8: Service（创建/回答/放弃/列表/回看 + 并发防护）

**Files:**
- Create: `backend/internal/interview/service.go`、`backend/internal/interview/service_test.go`

**Interfaces:**
- Consumes: Task 3–7 全部
- Produces:
  - `type Service struct{ repo *Repository; store *RedisStore; gen *QuestionGenerator; graph *Graph }`
  - `func NewService(repo *Repository, store *RedisStore, gen *QuestionGenerator, graph *Graph) *Service`
  - `func (s *Service) CreateInterview(ctx, tenantID, targetJobID, resumeID int64, cfg *InterviewConfig) (*Interview, error)`（默认值合并 → `gen.GenerateQuestions` → `repo.CreateInterview` 事务落库 → 首轮提问）
  - `func (s *Service) Answer(ctx, tenantID, interviewID int64, payload answerPayload) (graphResult, error)`（校验 active → `store.TryLock` → 读 checkpoint → `graph.Answer` → `store.Unlock`）
  - `func (s *Service) Abort(ctx, tenantID, interviewID int64) error`
  - `func (s *Service) List(ctx, tenantID int64) ([]Interview, error)`
  - `func (s *Service) Get(ctx, tenantID, id int64) (*Interview, error)`
  - `func (s *Service) Questions(ctx, tenantID, id int64) ([]Question, error)`（全部主问题含追问树，按 parent_id 组树返回平铺顺序）
  - 错误：`ErrNotFound`、`ErrActiveInterviewExists`、`ErrAlreadyEnded`（非 active answer/abort 409）、`ErrConcurrentAnswer`（409）

- [ ] **Step 1: 写 service 测试（红）**

```go
package interview

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestServiceCreateAndAnswer(t *testing.T) {
	repo, db := newTestRepo(t)
	defer db.Close()
	store := redisStore(t) // 无 Redis 时 skip
	payload, _ := json.Marshal(map[string]any{"grade": "A", "followUp": ""})
	graph, _ := BuildGraph(repo, &fakeResponses{payload: string(payload)})
	svc := NewService(repo, store, NewQuestionGenerator(&fakeResponses{payload: `{"questions":[]}`}, 1), graph)
	iv, err := svc.CreateInterview(context.Background(), 1, 1, 1, nil)
	if err != nil { t.Fatalf("create: %v", err) }
	if iv.Status != "active" { t.Errorf("status = %s", iv.Status) }
	res, err := svc.Answer(context.Background(), 1, iv.ID, answerPayload{Answer: "答 A 水平"})
	if err != nil { t.Fatalf("answer: %v", err) }
	if res.Grade != GradeA { t.Errorf("grade = %s", res.Grade) }
}

func TestServiceAnswerRejectsEnded(t *testing.T) {
	// 创建 → abort → answer 应返回 ErrAlreadyEnded
}

func TestServiceAnswerRejectsConcurrent(t *testing.T) {
	// 预占用锁后 answer → ErrConcurrentAnswer
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd backend && go test ./internal/interview/ -run TestService -v`
Expected: FAIL。

- [ ] **Step 3: 实现 service.go**

按设计文档「answer 完整实现」：校验 → TryLock(interviewID, 60) → 读 checkpoint（缺失 → ErrSessionExpired 410 语义）→ `graph.Answer` → Unlock（defer）。`CreateInterview` 里 `resumeText` 由调用方（httpapi）从简历 JSON 提取后传入——service 不直接碰 resume 包，保持隔离；生题 fake 返回空列表时测试用真实 cfg 覆盖。

> 注意：CreateInterview 需要 resume 纯文本，handler 从 `resume.Repository.Get` 拿 `content` JSON 提取；service 接受 `resumeText string` 参数，不 import resume 包。

- [ ] **Step 4: 跑测试确认通过**

Run: `cd backend && go test ./internal/interview/ -v`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add backend/internal/interview/service.go backend/internal/interview/service_test.go
git commit -m "feat(interview): service 编排（创建/回答/放弃/回看 + 并发锁）"
```

---

### Task 9: HTTP API（handler + 路由 + main 接线）

**Files:**
- Create: `backend/internal/httpapi/interview_handler.go`、`backend/internal/httpapi/interview_handler_test.go`
- Modify: `backend/internal/httpapi/router.go`、`backend/cmd/server/main.go`

**Interfaces:**
- Consumes: Task 8 Service；现有 `principalFromContext` / `writeJSON` / `writeError` / `routeID` / `requireAuthentication`
- Produces: 路由
  - `POST /api/interviews`（body：`{targetJobId, resumeId, config?}`；503 当 service 为 nil）
  - `GET /api/interviews`、`GET /api/interviews/:id`、`GET /api/interviews/:id/questions`
  - `POST /api/interviews/:id/answer`（body `{answer?, skip?}`）
  - `POST /api/interviews/:id/abort`
  - 错误映射：404→`ErrNotFound`、409→`ErrActiveInterviewExists/ErrAlreadyEnded/ErrConcurrentAnswer`、410→`ErrSessionExpired`、400→参数错

- [ ] **Step 1: 写 handler 测试（红）**

沿用现有 handler 测试模式（`httptest` + 内存 db + fake 服务注入；参考 `target_handler_test.go` 的建测方式，service 用真 repo + fake model + 真实 Redis 或注入假 store——若 Redis 不可用，handler 测试对 `ErrSessionExpired`/404 路径仍可测）。

```go
func TestInterviewHandlerCreate(t *testing.T) {
	// 建内存 db + migrate + repo + fake model graph + redis store(或 skip)
	// POST /api/interviews body {"targetJobId":1,"resumeId":1}
	// 期望 201 + body.interview.status == "active" + body 含 firstQuestion
}

func TestInterviewHandlerAnswerNotFound(t *testing.T) {
	// POST /api/interviews/999/answer → 404
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd backend && go test ./internal/httpapi/ -run TestInterviewHandler -v`
Expected: FAIL。

- [ ] **Step 3: 实现 interview_handler.go**

按 `target_handler.go` 模式：`newInterviewHandler(service *interview.Service) *interviewHandler`；`create/list/get/questions/answer/abort` 方法；`principalFromContext` 取 tenant；简历文本提取用 `resume.Repository.Get` + 简单 JSON 提取（`content` 里拼各 section 文本，见现有 `optimization.go` 的 section 遍历方式，做成 `extractResumeText(content string) string`）。

- [ ] **Step 4: 注册路由**

在 `router.go` 加：

```go
if config.InterviewService != nil {
	interviewHandler := newInterviewHandler(config.InterviewService)
	interviews := router.Group("/api/interviews")
	interviews.Use(requireAuthentication(service))
	interviews.POST("", interviewHandler.create)
	interviews.GET("", interviewHandler.list)
	interviews.GET("/:id", interviewHandler.get)
	interviews.POST("/:id/answer", interviewHandler.answer)
	interviews.GET("/:id/questions", interviewHandler.questions)
	interviews.POST("/:id/abort", interviewHandler.abort)
}
```

`RouterConfig` 增加 `InterviewService *interview.Service`。

- [ ] **Step 5: main.go 接线**

`buildRouterConfig` 中：读 `REDIS_ADDR`/`REDIS_PASSWORD`/`REDIS_DB` env → `interview.NewRedisStore`（失败：非生产 warn 且不注册 interview service；生产 fatal）→ `llm.NewResponsesModel`（失败同理）→ `interview.NewQuestionGenerator` → `interview.BuildGraph` → `interview.NewService` → 赋给 `config.InterviewService`。生题/评分共用同一个 ResponsesModel 实例。

- [ ] **Step 6: 编译 + 全量测试**

Run: `cd backend && go build ./... && go test ./...`
Expected: 全绿。

- [ ] **Step 7: Commit**

```bash
git add backend/internal/httpapi/interview_handler.go backend/internal/httpapi/interview_handler_test.go backend/internal/httpapi/router.go backend/cmd/server/main.go
git commit -m "feat(api): 面试 REST 接口与路由注册，main 接线 Redis/Responses 模型"
```

---

### Task 10: 前端（配置页 → 面试页 → 回看页 → 历史列表）

**Files:**
- Create: `frontend/src/api/interviews.ts`、`frontend/src/api/interviews.test.ts`
- Create: `frontend/src/components/InterviewConfigModal.tsx`、`frontend/src/components/InterviewChat.tsx`、`frontend/src/components/InterviewReview.tsx`、`frontend/src/components/InterviewHistory.tsx`（含各自 `.test.tsx`）
- Modify: `frontend/src/app.tsx`（路由/入口）、`frontend/src/components/TargetJobCard.tsx`（「开始 AI 面试」按钮）

**Interfaces:**
- Consumes: 现有 `apiFetch`（`frontend/src/api/client.ts`）、TargetJobPicker 选中状态
- Produces:
  - `api/interviews.ts`：`createInterview(targetJobId, resumeId, config?)`、`listInterviews()`、`getInterview(id)`、`answerInterview(id, answer, skip?)`、`getInterviewQuestions(id)`、`abortInterview(id)`；类型 `Interview`、`InterviewQuestion`、`InterviewConfig`、`AnswerResult`
  - 组件间 props：`InterviewChat({ interview, onFinished })`、`InterviewReview({ interview })`、`InterviewHistory({ onStart })`

- [ ] **Step 1: 写 api 测试（红）**

按 `frontend/src/api/targetJobs.test.ts` 模式（mock `apiFetch`）：create/list/answer/abort 各用例 + 错误分支。

- [ ] **Step 2: 跑测试确认失败**

Run: `cd frontend && npx vitest run src/api/interviews.test.ts`
Expected: FAIL。

- [ ] **Step 3: 实现 interviews.ts**

类型与函数见 Interfaces；`answerInterview` 的 body 按 `{answer?, skip?}`；返回 `AnswerResult`（`{grade, followUp?, nextQuestion?, finished?, overallGrade?, overallReview?}`）。

- [ ] **Step 4: 跑测试确认通过**

Run: `cd frontend && npx vitest run src/api/interviews.test.ts`
Expected: PASS。

- [ ] **Step 5: 组件实现**

- **InterviewConfigModal**：预填 `DefaultInterviewConfig`（题目总数 12、标签数量 5/2/5、追问预算默认一半、单问上限 1），数字输入可改，确认回调 `(config) => void`。
- **InterviewChat**：聊天列表——assistant 问题消息块 + 用户回答 + 评分徽章（等级色块）+ 追问折叠（父题下缩进）；底部输入框 +「发送」「跳过/不会」按钮；提交防抖（disable 至响应返回）；响应带 `nextQuestion` 追加新消息、`finished` 切总评卡片。
- **InterviewReview**：总评卡片（`overallGrade` + `overallReview`）+ 逐题明细（`/questions` 数据按 parent_id 组树，展示 答案/标准答案/等级/追问链）。
- **InterviewHistory**：列表（状态徽章：进行中/已完成/已放弃、总评摘要、回看入口、放弃按钮）。
- **app.tsx / TargetJobCard**：目标 JD 卡片加「开始 AI 面试」→ 弹配置弹窗 → 进面试页；「进行中面试」存在时提示先结束。

- [ ] **Step 6: 组件测试 + 全量测试**

Run: `cd frontend && npx vitest run`
Expected: 全绿。

- [ ] **Step 7: Commit**

```bash
git add frontend/src/api/interviews.ts frontend/src/api/interviews.test.ts frontend/src/components/ frontend/src/app.tsx
git commit -m "feat(frontend): AI 面试配置/聊天/回看/历史页面与接口封装"
```

---

### Task 11: 端到端联调 + .env.example

**Files:**
- Modify: `.env.example`（追加 REDIS_ADDR/REDIS_PASSWORD/REDIS_DB）、`README.md`（面试功能说明）

- [ ] **Step 1: .env.example 追加**

```bash
# Optional. Redis for AI interview state (checkpoint store). Interview feature is disabled without it.
# REDIS_ADDR=127.0.0.1:6379
# REDIS_PASSWORD=
# REDIS_DB=0
```

- [ ] **Step 2: 端到端手测**

Run: `cd backend && go run ./cmd/server` + `cd frontend && npm run dev`，浏览器走一遍：目标 JD → 配置弹窗 → 第一题 → 回答 → 评分/追问 → 跳过 → 全部答完 → 总评 → 历史回看 → abort。用真实 MiniMax key（`.env` 已有）。预期：生题成功、每轮回答返回评分、追问链等级只升不降、总评符合算法、回看数据完整。

- [ ] **Step 3: README 补面试功能段落**

简短说明：入口、依赖（Redis）、env 变量、接口列表。

- [ ] **Step 4: Commit**

```bash
git add .env.example README.md
git commit -m "docs(interview): README 与 .env.example 补 AI 面试说明"
```

---

## Self-Review 记录

- **Spec 覆盖**：生题函数（Task 6/8）✓；面试图五节点（Task 7）✓；skip 统一走图（Task 7 grade_answer skip 分支 + Task 8）✓；总评算法（Task 3）✓；checkpoint/TTL/锁（Task 5）✓；并发创建事务（Task 4）✓；API 全部路由（Task 9）✓；前端四页面（Task 10）✓；Redis env（Task 5/9/11）✓；eino 升级（Task 1）✓。
- **无占位符**：所有任务含真实代码或明确 API 名称；graph 节点内 `...` 标注需按 eino v0.9.21 实际签名适配（已注明），属风险提示而非占位。
- **类型一致性**：`Grade/QuestionLabel/InterviewConfig/Repository/RedisStore/QuestionGenerator/InterviewState/answerPayload/graphResult` 跨任务签名一致；`llm.NewResponsesModel` 返回 `model.AgenticModel`，`ResponsesModel` 接口在 Task 6 定义、Task 7 复用。
