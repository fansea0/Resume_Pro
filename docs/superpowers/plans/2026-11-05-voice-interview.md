# AI 模拟面试语音能力 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 给 AI 模拟面试加上声音：题目语音后台生成并自动播放，用户口述录音经 ASR 转写 + AI 修错别字后插入输入框光标处，语音整体不可用时面试流程完全不受影响。

**Architecture:** 新增 `backend/internal/speech` 包（协议抽象 + OpenAI 兼容 provider + 音频存储 + 串行生成 worker + 错别字修复）；`interview` 层只依赖一个窄接口 `AudioHooks`，由 `speech.Generator` 实现，触发时机是「创建面试后入队全部主问题」与「答题推进到下一题时入队该题」；播放接口只读已生成的文件，绝不在请求内合成；服务重启时扫描进行中面试里未就绪的题目重新入队。

**Tech Stack:** Go 1.26.5 + gin + sqlx/modernc sqlite、eino v0.9.21 + eino-ext agenticopenai v0.2.4（Responses API）、React 18 + Vite + vitest。**不新增任何 Go 依赖**（HTTP 与 multipart 用标准库）。

**Spec:** `docs/plans/2026-11-05-voice-interview-design.md`

## Global Constraints

- Go 模块 `resumepro/backend`，go 1.26.5。**不新增第三方依赖**：TTS/ASR 用 `net/http` + 标准库 `mime/multipart`；错别字修复复用已有的 `llm.NewResponsesModel`。
- 提交 message 中文 `type(scope): 说明`；每任务一个 commit；**不写 co-author**。
- 改 `backend/**` 后 `cd backend && go test ./...` 必须全绿；改 `frontend/**` 后 `cd frontend && npx vitest run` 必须全绿。
- 依赖方向固定为 **speech → interview**（speech 引用 `interview.PendingAudio`）；**interview 包不得 import speech 包**，语音能力只通过 `interview.AudioHooks` 窄接口进入面试层。
- 语音是附加层：**`answer` 主链路不得同步等待 TTS/ASR**，任何语音故障都不得让创建/答题接口失败。
- 降级约定沿用既有做法：缺依赖时路由**仍然注册**，接口返回明确的 503。
- 租户隔离在 service 层校验；语音接口与既有面试接口使用同一套 404 语义（不存在与不属于本人都是 404，不泄露存在性）。
- 音频目录布局固定 `{VOICE_CACHE_DIR}/{tenantID}/{interviewID}/q{questionID}.wav`，以 **interviewID 为清理单元**。
- **不存录音、不做录音回放**：转写音频只在请求内透传给 ASR。
- `audio_state` 取值固定四态：`NULL` / `pending` / `ready` / `failed`。
- 错别字修复的思考强度固定最低档（`ReasoningEffort=low`），只改错别字/同音字/标点，不改语义、不润色。
- 端口：mlx-audio server 用 `21100`（避开常用段）。

---

### Task 1: 音频状态落库（迁移 + 仓储扩展）

**Files:**
- Modify: `backend/internal/store/migrate.go`
- Modify: `backend/internal/interview/repository.go`
- Create: `backend/internal/interview/audio.go`
- Test: `backend/internal/store/migrate_test.go`、`backend/internal/interview/repository_test.go`

**Interfaces:**
- Produces:
  - `interview.Question.AudioState *string`（db 列 `audio_state`，可空）
  - `interview.PendingAudio{TenantID, InterviewID, QuestionID int64; Text string}`
  - `interview.AudioHooks` 接口（见 `audio.go`）
  - `func (r *Repository) UpdateAudioState(ctx context.Context, questionID int64, state string) error`
  - `func (r *Repository) QuestionByID(ctx context.Context, id int64) (Question, error)`
  - `func (r *Repository) ActiveQuestionsNeedingAudio(ctx context.Context) ([]PendingAudio, error)`

- [ ] **Step 1: 写迁移测试**

在 `backend/internal/store/migrate_test.go` 追加：

```go
func TestMigrateAddsAudioStateColumnToInterviewQuestions(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	var columnType string
	if err := db.GetContext(ctx, &columnType, `SELECT type FROM pragma_table_info('interview_questions') WHERE name = 'audio_state'`); err != nil || columnType == "" {
		t.Fatalf("audio_state column missing: type=%q err=%v", columnType, err)
	}
	// 迁移幂等：重复执行不报错。
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd backend && go test ./internal/store/ -run TestMigrateAddsAudioStateColumnToInterviewQuestions -v`
Expected: FAIL —— `audio_state` 列不存在（`no such column: audio_state`）。

- [ ] **Step 3: 加迁移**

在 `backend/internal/store/migrate.go` 的 `Migrate` 函数里，紧跟 `addColumnIfMissing(ctx, tx, "resumes", "tenant_id", ...)` 之后追加：

```go
if err := addColumnIfMissing(ctx, tx, "interview_questions", "audio_state", "TEXT"); err != nil {
	return err
}
```

`addColumnIfMissing` 用 `PRAGMA table_info` 判断列是否存在，重复执行安全。

- [ ] **Step 4: 运行迁移测试确认通过**

Run: `cd backend && go test ./internal/store/ -run TestMigrateAddsAudioStateColumnToInterviewQuestions -v`
Expected: PASS。

- [ ] **Step 5: 写仓储测试**

在 `backend/internal/interview/repository_test.go` 追加（先看该文件里既有的 fixture 辅助函数名并复用，不要另造一套）：

```go
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
```

> 若既有测试文件没有 `newRepositoryFixture` / `insertMainQuestion`，就在本文件内按 `repository_test.go` 现有写法补这两个辅助函数（用 sqlx 直接插库，不要 mock）。

- [ ] **Step 6: 运行仓储测试确认失败**

Run: `cd backend && go test ./internal/interview/ -run 'TestRepository(UpdateAudioState|ActiveQuestionsNeedingAudio)' -v`
Expected: FAIL —— 方法未定义（`repo.UpdateAudioState undefined`）。

- [ ] **Step 7: 实现仓储方法与结构体字段**

在 `backend/internal/interview/repository.go` 的 `Question` 结构体末尾追加字段：

```go
	// AudioState 题目语音状态：NULL=未入队，pending=生成中，ready=已就绪，failed=重试耗尽。
	AudioState *string `db:"audio_state" json:"audioState"`
```

在 `backend/internal/interview/repository.go` 追加三个方法：

```go
// UpdateAudioState 回写题目的语音状态（生成 worker 使用）。
func (r *Repository) UpdateAudioState(ctx context.Context, questionID int64, state string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE interview_questions SET audio_state = ? WHERE id = ?`, state, questionID)
	if err != nil {
		return fmt.Errorf("interview: update audio state %d: %w", questionID, err)
	}
	return nil
}

// QuestionByID 按 id 读取题目行。
func (r *Repository) QuestionByID(ctx context.Context, id int64) (Question, error) {
	var row Question
	err := r.db.GetContext(ctx, &row, `SELECT id, interview_id, parent_id, follow_up_round, label, question,
		standard_answer, user_answer, extra, grade, audio_state, created_at FROM interview_questions WHERE id = ?`, id)
	if err != nil {
		return Question{}, fmt.Errorf("interview: load question %d: %w", id, err)
	}
	return row, nil
}

// ActiveQuestionsNeedingAudio 返回「进行中面试里语音未就绪」的题目：
// audio_state 为 NULL、pending 或 failed 的都要重新生成（重启重载与失败补齐共用）。
func (r *Repository) ActiveQuestionsNeedingAudio(ctx context.Context) ([]PendingAudio, error) {
	var items []PendingAudio
	err := r.db.SelectContext(ctx, &items, `
		SELECT i.tenant_id AS tenant_id, q.interview_id AS interview_id, q.id AS question_id, q.question AS text
		FROM interview_questions q
		JOIN interviews i ON i.id = q.interview_id
		WHERE i.status = 'active' AND (q.audio_state IS NULL OR q.audio_state IN ('pending', 'failed'))
		ORDER BY i.id ASC, q.id ASC`)
	if err != nil {
		return nil, fmt.Errorf("interview: load questions needing audio: %w", err)
	}
	return items, nil
}
```

- [ ] **Step 8: 运行仓储测试确认通过**

Run: `cd backend && go test ./internal/interview/ ./internal/store/ -v`
Expected: PASS。

- [ ] **Step 9: 新建 `backend/internal/interview/audio.go`**

```go
package interview

import "context"

// PendingAudio 是一条待生成语音的题目：重启重载与失败补齐时按它重建队列。
type PendingAudio struct {
	TenantID    int64
	InterviewID int64
	QuestionID  int64
	Text        string
}

// AudioHooks 是面试层对语音能力的窄依赖，实现由 speech.Generator 提供（main.go 接线）。
//
// 约定：所有方法都必须安全失败 —— 语音不可用只影响语音，不影响面试主流程；
// 实现方不得把错误抛回给调用方的答题链路。
type AudioHooks interface {
	// Enqueue 把题目加入后台串行生成队列（创建面试后 / 追问落库后调用）。
	Enqueue(ctx context.Context, tenantID, interviewID, questionID int64, text string)
	// CleanupInterview 清理一场面试的音频目录（面试终态时调用）。
	CleanupInterview(ctx context.Context, tenantID, interviewID int64)
}
```

- [ ] **Step 10: 全量编译与测试**

Run: `cd backend && go build ./... && go test ./...`
Expected: 全部通过。

- [ ] **Step 11: Commit**

```bash
git add backend/internal/store/migrate.go backend/internal/store/migrate_test.go backend/internal/interview/repository.go backend/internal/interview/repository_test.go backend/internal/interview/audio.go
git commit -m "feat(interview): 新增题目语音状态列与仓储查询"
```

---

### Task 2: speech 协议层（接口 / 配置 / OpenAI 兼容 provider）

**Files:**
- Create: `backend/internal/speech/speech.go`
- Create: `backend/internal/speech/config.go`
- Create: `backend/internal/speech/openai.go`
- Test: `backend/internal/speech/openai_test.go`、`backend/internal/speech/config_test.go`

**Interfaces:**
- Produces:
  - `speech.Synthesizer`、`speech.Transcriber`、`speech.TextModel` 三个接口
  - `speech.SynthesisRequest/SynthesisResult/TranscriptionRequest/TranscriptionResult`
  - `speech.Config` + `speech.LoadConfigFromEnv() Config`
  - `speech.NewOpenAICompatibleProvider(cfg Config) *OpenAICompatibleProvider`
  - `speech.MaxAudioBytes`、`speech.MaxAudioDuration`、`speech.ValidateWAV(data []byte) (time.Duration, error)`

- [ ] **Step 1: 写 provider 测试**

新建 `backend/internal/speech/openai_test.go`：

```go
package speech

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func testConfig(ttsURL, asrURL string) Config {
	return Config{
		TTSBaseURL: ttsURL,
		TTSModel:   "tts-model",
		TTSVoice:   "Vivian",
		ASRBaseURL: asrURL,
		ASRModel:   "asr-model",
	}
}

func TestSynthesizeForcesWavContentType(t *testing.T) {
	var gotFormat string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotFormat = r.Form.Get("response_format")
		w.Header().Set("Content-Type", "audio/mp3") // 故意撒谎：上游按 response_format 拼 MIME
		_, _ = w.Write([]byte("RIFFfake"))
	}))
	defer server.Close()

	provider := NewOpenAICompatibleProvider(testConfig(server.URL, server.URL))
	result, err := provider.Synthesize(context.Background(), SynthesisRequest{Text: "你好"})
	if err != nil {
		t.Fatalf("synthesize: %v", err)
	}
	if gotFormat != "wav" {
		t.Errorf("response_format = %q, want wav", gotFormat)
	}
	if result.ContentType != "audio/wav" {
		t.Errorf("content type = %q, want audio/wav", result.ContentType)
	}
}

func TestTranscribeSendsJSONResponseFormat(t *testing.T) {
	var gotFormat string
	var gotFileBytes int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("parse multipart: %v", err)
			return
		}
		gotFormat = r.FormValue("response_format")
		file, _, err := r.FormFile("file")
		if err != nil {
			t.Errorf("form file: %v", err)
			return
		}
		defer func() { _ = file.Close() }()
		data, _ := io.ReadAll(file)
		gotFileBytes = len(data)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":"识别结果"}`))
	}))
	defer server.Close()

	provider := NewOpenAICompatibleProvider(testConfig(server.URL, server.URL))
	result, err := provider.Transcribe(context.Background(), TranscriptionRequest{Audio: []byte("RIFFdata")})
	if err != nil {
		t.Fatalf("transcribe: %v", err)
	}
	if gotFormat != "json" {
		t.Errorf("response_format = %q, want json", gotFormat)
	}
	if gotFileBytes != len("RIFFdata") {
		t.Errorf("uploaded bytes = %d, want %d", gotFileBytes, len("RIFFdata"))
	}
	if result.Text != "识别结果" {
		t.Errorf("text = %q", result.Text)
	}
}

func TestTranscribeFallsBackToNdjsonFirstLine(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = w.Write([]byte("{\"text\":\"第一行\"}\n{\"text\":\"第二行\"}\n"))
	}))
	defer server.Close()

	provider := NewOpenAICompatibleProvider(testConfig(server.URL, server.URL))
	result, err := provider.Transcribe(context.Background(), TranscriptionRequest{Audio: []byte("RIFFdata")})
	if err != nil {
		t.Fatalf("transcribe: %v", err)
	}
	if result.Text != "第一行" {
		t.Errorf("text = %q, want 第一行", result.Text)
	}
}

func TestProviderUsesSeparateBaseURLs(t *testing.T) {
	var ttsHits, asrHits int
	tts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ttsHits++
		_, _ = w.Write([]byte("RIFF"))
	}))
	defer tts.Close()
	asr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asrHits++
		_, _ = w.Write([]byte(`{"text":"x"}`))
	}))
	defer asr.Close()

	provider := NewOpenAICompatibleProvider(testConfig(tts.URL, asr.URL))
	if _, err := provider.Synthesize(context.Background(), SynthesisRequest{Text: "x"}); err != nil {
		t.Fatalf("synthesize: %v", err)
	}
	if _, err := provider.Transcribe(context.Background(), TranscriptionRequest{Audio: []byte("x")}); err != nil {
		t.Fatalf("transcribe: %v", err)
	}
	if ttsHits != 1 || asrHits != 1 {
		t.Errorf("hits: tts=%d asr=%d, want 1/1", ttsHits, asrHits)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd backend && go test ./internal/speech/ -v`
Expected: FAIL —— 包不存在（`no Go files in .../internal/speech`）。

- [ ] **Step 3: 写 `backend/internal/speech/speech.go`**

```go
// Package speech 提供 AI 模拟面试的语音能力：题目语音合成（TTS）、录音转写（ASR）
// 与转写文本的错别字修复。
package speech

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

const (
	// MaxAudioBytes 上传音频体积上限 20MB。
	MaxAudioBytes = 20 << 20
	// MaxAudioDuration 上传音频时长上限 10 分钟。
	MaxAudioDuration = 10 * time.Minute
)

// SynthesisRequest 是一次题目语音合成请求。
type SynthesisRequest struct {
	Text  string
	Voice string
}

// SynthesisResult 是合成产物。ContentType 恒为 audio/wav（不信上游 MIME，见 openai.go）。
type SynthesisResult struct {
	Audio       []byte
	ContentType string
}

// TranscriptionRequest 是一次录音转写请求；Audio 原样透传给上游，后端不转码。
type TranscriptionRequest struct {
	Audio       []byte
	ContentType string
}

// TranscriptionResult 是转写文本。
type TranscriptionResult struct {
	Text string
}

// Synthesizer 把题面文本合成为语音。
type Synthesizer interface {
	Synthesize(ctx context.Context, req SynthesisRequest) (SynthesisResult, error)
}

// Transcriber 把录音转写为文本。
type Transcriber interface {
	Transcribe(ctx context.Context, req TranscriptionRequest) (TranscriptionResult, error)
}

// TextModel 是错别字修复所需的最小模型接口。
type TextModel interface {
	Generate(ctx context.Context, in []*schema.AgenticMessage, opts ...model.Option) (*schema.AgenticMessage, error)
}

// ValidateWAV 校验上传音频是可解析的 WAV，并返回时长。
// 只做校验，不转码：前端已保证 16kHz 单声道 PCM。
func ValidateWAV(data []byte) (time.Duration, error) {
	if len(data) > MaxAudioBytes {
		return 0, fmt.Errorf("speech: audio exceeds %d bytes", MaxAudioBytes)
	}
	if len(data) < 44 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return 0, errors.New("speech: not a RIFF/WAVE payload")
	}
	var channels, sampleRate, dataSize int
	for offset := 12; offset+8 <= len(data); {
		id := string(data[offset : offset+4])
		size := int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		body := offset + 8
		switch id {
		case "fmt ":
			if body+16 <= len(data) {
				channels = int(binary.LittleEndian.Uint16(data[body+2 : body+4]))
				sampleRate = int(binary.LittleEndian.Uint32(data[body+4 : body+8]))
			}
		case "data":
			dataSize = size
		}
		offset = body + size
		if size%2 == 1 {
			offset++
		}
	}
	if channels <= 0 || sampleRate <= 0 || dataSize <= 0 {
		return 0, errors.New("speech: incomplete wav payload")
	}
	bytesPerSecond := sampleRate * channels * 2
	if bytesPerSecond <= 0 {
		return 0, errors.New("speech: invalid wav format")
	}
	duration := time.Duration(float64(dataSize) / float64(bytesPerSecond) * float64(time.Second))
	if duration > MaxAudioDuration {
		return 0, fmt.Errorf("speech: audio longer than %s", MaxAudioDuration)
	}
	return duration, nil
}
```

- [ ] **Step 4: 写 `backend/internal/speech/config.go`**

```go
package speech

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config 是语音能力的运行配置。TTS 与 ASR 各自独立，互不影响。
type Config struct {
	// Enabled 为 false 时不创建 provider，路由仍注册并返回 503。
	Enabled bool

	TTSBaseURL string
	TTSAPIKey  string
	TTSModel   string
	TTSVoice   string
	TTSTimeout time.Duration

	ASRBaseURL string
	ASRAPIKey  string
	ASRModel   string
	ASRTimeout time.Duration

	// CorrectTimeout 是错别字修复的单次调用超时。
	CorrectTimeout time.Duration

	// MaxAttempts 是单题生成的尝试次数（含首次）。
	MaxAttempts int
	// Backoff 是失败重试的退避序列。
	Backoff []time.Duration

	// CacheDir 是题目音频的根目录。
	CacheDir string
}

// LoadConfigFromEnv 从环境变量读配置。语音功能默认关闭（opt-in），
// 避免未安装 mlx-audio 的部署启动即报错。
func LoadConfigFromEnv() Config {
	return Config{
		Enabled:        envBool("VOICE_ENABLED", false),
		TTSBaseURL:     envOr("VOICE_TTS_BASE_URL", "http://127.0.0.1:21100/v1"),
		TTSAPIKey:      os.Getenv("VOICE_TTS_API_KEY"),
		TTSModel:       envOr("VOICE_TTS_MODEL", "mlx-community/Qwen3-TTS-12Hz-0.6B-CustomVoice-8bit"),
		TTSVoice:       envOr("VOICE_TTS_VOICE", "Vivian"),
		TTSTimeout:     envDuration("VOICE_TTS_TIMEOUT", 120*time.Second),
		ASRBaseURL:     envOr("VOICE_ASR_BASE_URL", "http://127.0.0.1:21100/v1"),
		ASRAPIKey:      os.Getenv("VOICE_ASR_API_KEY"),
		ASRModel:       envOr("VOICE_ASR_MODEL", "mlx-community/Qwen3-ASR-1.7B-8bit"),
		ASRTimeout:     envDuration("VOICE_ASR_TIMEOUT", 60*time.Second),
		CorrectTimeout: envDuration("VOICE_CORRECT_TIMEOUT", 30*time.Second),
		MaxAttempts:    envInt("VOICE_GENERATOR_MAX_ATTEMPTS", 3),
		Backoff:        envDurations("VOICE_GENERATOR_BACKOFF", []time.Duration{time.Second, 3 * time.Second, 10 * time.Second}),
		CacheDir:       envOr("VOICE_CACHE_DIR", "./uploads/voice"),
	}
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	switch value {
	case "":
		return fallback
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func envInt(key string, fallback int) int {
	if value, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key))); err == nil && value > 0 {
		return value
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if value, err := time.ParseDuration(strings.TrimSpace(os.Getenv(key))); err == nil && value > 0 {
		return value
	}
	return fallback
}

func envDurations(key string, fallback []time.Duration) []time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	var out []time.Duration
	for _, part := range strings.Split(raw, ",") {
		value, err := time.ParseDuration(strings.TrimSpace(part))
		if err != nil || value <= 0 {
			return fallback
		}
		out = append(out, value)
	}
	if len(out) == 0 {
		return fallback
	}
	return out
}
```

- [ ] **Step 5: 写 `backend/internal/speech/openai.go`**

```go
package speech

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
)

// OpenAICompatibleProvider 同时实现 Synthesizer 与 Transcriber。
// TTS 与 ASR 都走 OpenAI 音频协议，但各自持有独立的 base URL / api key / model / http.Client，
// 因此可以分别指向本地 mlx-audio 与云端 API，组合自由且互不影响。
type OpenAICompatibleProvider struct {
	ttsBaseURL string
	ttsAPIKey  string
	ttsModel   string
	ttsVoice   string
	ttsClient  *http.Client

	asrBaseURL string
	asrAPIKey  string
	asrModel   string
	asrClient  *http.Client
}

// NewOpenAICompatibleProvider 按配置创建 provider。
func NewOpenAICompatibleProvider(cfg Config) *OpenAICompatibleProvider {
	return &OpenAICompatibleProvider{
		ttsBaseURL: strings.TrimRight(cfg.TTSBaseURL, "/"),
		ttsAPIKey:  cfg.TTSAPIKey,
		ttsModel:   cfg.TTSModel,
		ttsVoice:   cfg.TTSVoice,
		ttsClient:  &http.Client{Timeout: cfg.TTSTimeout},
		asrBaseURL: strings.TrimRight(cfg.ASRBaseURL, "/"),
		asrAPIKey:  cfg.ASRAPIKey,
		asrModel:   cfg.ASRModel,
		asrClient:  &http.Client{Timeout: cfg.ASRTimeout},
	}
}

// Synthesize 调用 POST {base}/audio/speech。
//
// 两个刻意的处理（设计文档「坑 1」）：
//   - 固定 response_format=wav：上游只用它拼 Content-Type，传 mp3 会得到「标着 mp3 的 WAV 字节」；
//   - 不信任上游 Content-Type，直接定为 audio/wav，与实际字节一致。
func (p *OpenAICompatibleProvider) Synthesize(ctx context.Context, req SynthesisRequest) (SynthesisResult, error) {
	form := url.Values{}
	form.Set("model", p.ttsModel)
	form.Set("input", req.Text)
	form.Set("response_format", "wav")
	voice := req.Voice
	if voice == "" {
		voice = p.ttsVoice
	}
	if voice != "" {
		form.Set("voice", voice)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.ttsBaseURL+"/audio/speech", strings.NewReader(form.Encode()))
	if err != nil {
		return SynthesisResult{}, fmt.Errorf("speech: build tts request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if p.ttsAPIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.ttsAPIKey)
	}

	resp, err := p.ttsClient.Do(httpReq)
	if err != nil {
		return SynthesisResult{}, fmt.Errorf("speech: tts request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return SynthesisResult{}, fmt.Errorf("speech: tts status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	audio, err := io.ReadAll(resp.Body)
	if err != nil {
		return SynthesisResult{}, fmt.Errorf("speech: read tts body: %w", err)
	}
	if len(audio) == 0 {
		return SynthesisResult{}, errors.New("speech: empty tts audio")
	}
	return SynthesisResult{Audio: audio, ContentType: "audio/wav"}, nil
}

// Transcribe 调用 POST {base}/audio/transcriptions。
// response_format 必须显式传 json：上游默认值是 ndjson（设计文档「坑 2」）。
func (p *OpenAICompatibleProvider) Transcribe(ctx context.Context, req TranscriptionRequest) (TranscriptionResult, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "recording.wav")
	if err != nil {
		return TranscriptionResult{}, fmt.Errorf("speech: build asr form: %w", err)
	}
	if _, err := part.Write(req.Audio); err != nil {
		return TranscriptionResult{}, fmt.Errorf("speech: write asr file: %w", err)
	}
	_ = writer.WriteField("model", p.asrModel)
	_ = writer.WriteField("response_format", "json")
	if err := writer.Close(); err != nil {
		return TranscriptionResult{}, fmt.Errorf("speech: close asr form: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.asrBaseURL+"/audio/transcriptions", &body)
	if err != nil {
		return TranscriptionResult{}, fmt.Errorf("speech: build asr request: %w", err)
	}
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())
	if p.asrAPIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.asrAPIKey)
	}

	resp, err := p.asrClient.Do(httpReq)
	if err != nil {
		return TranscriptionResult{}, fmt.Errorf("speech: asr request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return TranscriptionResult{}, fmt.Errorf("speech: asr status %d: %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return TranscriptionResult{}, fmt.Errorf("speech: read asr body: %w", err)
	}
	text, err := parseTranscription(payload, resp.Header.Get("Content-Type"))
	if err != nil {
		return TranscriptionResult{}, err
	}
	return TranscriptionResult{Text: text}, nil
}

// parseTranscription 解析 ASR 响应：标准 JSON {"text":"..."}；
// 若上游退回 ndjson（Content-Type 为 application/x-ndjson），取第一行 JSON 的 text 兜底。
func parseTranscription(payload []byte, contentType string) (string, error) {
	if strings.Contains(contentType, "ndjson") {
		for _, line := range bytes.Split(payload, []byte("\n")) {
			line = bytes.TrimSpace(line)
			if len(line) == 0 {
				continue
			}
			var wrapper struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal(line, &wrapper); err == nil {
				return wrapper.Text, nil
			}
		}
		return "", errors.New("speech: ndjson transcription without text field")
	}
	var wrapper struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(payload, &wrapper); err != nil {
		return "", fmt.Errorf("speech: invalid transcription payload: %w", err)
	}
	return wrapper.Text, nil
}

var (
	_ Synthesizer = (*OpenAICompatibleProvider)(nil)
	_ Transcriber = (*OpenAICompatibleProvider)(nil)
)
```

- [ ] **Step 6: 运行测试确认通过**

Run: `cd backend && go test ./internal/speech/ -v`
Expected: PASS（4 个用例）。

- [ ] **Step 7: 写配置测试**

新建 `backend/internal/speech/config_test.go`：

```go
package speech

import (
	"testing"
	"time"
)

func TestLoadConfigFromEnv_DefaultsToDisabled(t *testing.T) {
	t.Setenv("VOICE_ENABLED", "")
	cfg := LoadConfigFromEnv()
	if cfg.Enabled {
		t.Error("voice should be disabled by default")
	}
	if cfg.TTSBaseURL != "http://127.0.0.1:21100/v1" || cfg.ASRBaseURL != "http://127.0.0.1:21100/v1" {
		t.Errorf("default base urls = %q / %q", cfg.TTSBaseURL, cfg.ASRBaseURL)
	}
}

func TestLoadConfigFromEnv_ReadsBackoffAndAttempts(t *testing.T) {
	t.Setenv("VOICE_ENABLED", "true")
	t.Setenv("VOICE_GENERATOR_BACKOFF", "1s,2s")
	t.Setenv("VOICE_GENERATOR_MAX_ATTEMPTS", "5")
	cfg := LoadConfigFromEnv()
	if !cfg.Enabled {
		t.Error("voice should be enabled")
	}
	if cfg.MaxAttempts != 5 {
		t.Errorf("max attempts = %d, want 5", cfg.MaxAttempts)
	}
	if len(cfg.Backoff) != 2 || cfg.Backoff[1] != 2*time.Second {
		t.Errorf("backoff = %v", cfg.Backoff)
	}
}
```

- [ ] **Step 8: 运行测试并提交**

Run: `cd backend && go test ./internal/speech/ -v`
Expected: PASS。

```bash
git add backend/internal/speech/speech.go backend/internal/speech/config.go backend/internal/speech/openai.go backend/internal/speech/openai_test.go backend/internal/speech/config_test.go
git commit -m "feat(speech): 新增语音协议层与 OpenAI 兼容实现"
```

---
### Task 3: 题目音频存储

**Files:**
- Create: `backend/internal/speech/store.go`
- Test: `backend/internal/speech/store_test.go`

**Interfaces:**
- Produces:
  - `speech.AudioStore` + `speech.NewAudioStore(root string) *AudioStore`
  - `(*AudioStore).SaveQuestionAudio(ctx, tenantID, interviewID, questionID int64, audio []byte) error`
  - `(*AudioStore).ReadQuestionAudio(tenantID, interviewID, questionID int64) ([]byte, error)`
  - `(*AudioStore).CleanupInterview(tenantID, interviewID int64) error`
  - `(*AudioStore).Root() string`

- [ ] **Step 1: 写存储测试**

新建 `backend/internal/speech/store_test.go`：

```go
package speech

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestAudioStoreRoundTripIsolatesTenants(t *testing.T) {
	root := t.TempDir()
	store := NewAudioStore(root)
	ctx := context.Background()

	if err := store.SaveQuestionAudio(ctx, 1, 10, 100, []byte("audio-a")); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := store.SaveQuestionAudio(ctx, 2, 10, 100, []byte("audio-b")); err != nil {
		t.Fatalf("save other tenant: %v", err)
	}

	got, err := store.ReadQuestionAudio(1, 10, 100)
	if err != nil || string(got) != "audio-a" {
		t.Fatalf("read = %q err = %v", got, err)
	}
	other, err := store.ReadQuestionAudio(2, 10, 100)
	if err != nil || string(other) != "audio-b" {
		t.Fatalf("cross tenant read = %q err = %v", other, err)
	}

	expected := filepath.Join(root, "1", "10", "q100.wav")
	if _, err := os.Stat(expected); err != nil {
		t.Fatalf("expected file at %s: %v", expected, err)
	}
}

func TestAudioStoreCleanupRemovesOnlyThatInterview(t *testing.T) {
	root := t.TempDir()
	store := NewAudioStore(root)
	ctx := context.Background()

	_ = store.SaveQuestionAudio(ctx, 1, 10, 100, []byte("a"))
	_ = store.SaveQuestionAudio(ctx, 1, 11, 100, []byte("b"))

	if err := store.CleanupInterview(1, 10); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if _, err := store.ReadQuestionAudio(1, 10, 100); err == nil {
		t.Error("interview 10 audio should be removed")
	}
	if _, err := store.ReadQuestionAudio(1, 11, 100); err != nil {
		t.Errorf("interview 11 audio must survive: %v", err)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd backend && go test ./internal/speech/ -run TestAudioStore -v`
Expected: FAIL —— `NewAudioStore undefined`。

- [ ] **Step 3: 实现 `backend/internal/speech/store.go`**

```go
package speech

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// AudioStore 以 {tenantID}/{interviewID}/q{questionID}.wav 组织题目音频。
// 清理单元是 interviewID：面试结束 = 删一个目录，不需要跨目录反查引用。
type AudioStore struct {
	root string
}

// NewAudioStore 创建存储。root 不存在时在首次写入时创建。
func NewAudioStore(root string) *AudioStore {
	return &AudioStore{root: root}
}

// Root 返回音频根目录（启动日志用）。
func (s *AudioStore) Root() string {
	return s.root
}

func (s *AudioStore) interviewDir(tenantID, interviewID int64) string {
	return filepath.Join(s.root, strconv.FormatInt(tenantID, 10), strconv.FormatInt(interviewID, 10))
}

func (s *AudioStore) questionPath(tenantID, interviewID, questionID int64) string {
	return filepath.Join(s.interviewDir(tenantID, interviewID), fmt.Sprintf("q%d.wav", questionID))
}

// SaveQuestionAudio 落盘题目语音。先写临时文件再 rename：
// 生成 worker 与读取方并发时不会读到半个文件。
func (s *AudioStore) SaveQuestionAudio(ctx context.Context, tenantID, interviewID, questionID int64, audio []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path := s.questionPath(tenantID, interviewID, questionID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("speech: create audio dir: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, audio, 0o644); err != nil {
		return fmt.Errorf("speech: write audio: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("speech: commit audio: %w", err)
	}
	return nil
}

// ReadQuestionAudio 读取题目语音字节。
func (s *AudioStore) ReadQuestionAudio(tenantID, interviewID, questionID int64) ([]byte, error) {
	data, err := os.ReadFile(s.questionPath(tenantID, interviewID, questionID))
	if err != nil {
		return nil, fmt.Errorf("speech: read audio: %w", err)
	}
	return data, nil
}

// CleanupInterview 删除一场面试的全部音频目录。
func (s *AudioStore) CleanupInterview(tenantID, interviewID int64) error {
	if err := os.RemoveAll(s.interviewDir(tenantID, interviewID)); err != nil {
		return fmt.Errorf("speech: cleanup interview audio: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `cd backend && go test ./internal/speech/ -v`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add backend/internal/speech/store.go backend/internal/speech/store_test.go
git commit -m "feat(speech): 新增题目音频落盘与按面试清理"
```

---

### Task 4: 后台串行生成器（worker + 重试 + 重启重载）

**Files:**
- Create: `backend/internal/speech/generator.go`
- Test: `backend/internal/speech/generator_test.go`

**Interfaces:**
- Consumes: `speech.Synthesizer`、`speech.AudioStore`、Task 1 的 `interview.PendingAudio`
- Produces:
  - `speech.AudioStateStore` 接口（由 `interview.Repository` 实现）
  - `speech.NewGenerator(synth Synthesizer, store *AudioStore, repo AudioStateStore, cfg Config) *Generator`
  - `(*Generator).Start(ctx context.Context)` / `(*Generator).Close()`
  - `(*Generator).Enqueue(ctx, tenantID, interviewID, questionID int64, text string)`
  - `(*Generator).Reload(ctx context.Context) error`
  - `(*Generator).CleanupInterview(ctx, tenantID, interviewID int64)`
  - `speech.Generator` 满足 `interview.AudioHooks`

- [ ] **Step 1: 写生成器测试**

新建 `backend/internal/speech/generator_test.go`：

```go
package speech

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"resumepro/backend/internal/interview"
)

// fakeStateStore 记录 audio_state 流转。
type fakeStateStore struct {
	mu      sync.Mutex
	states  map[int64]string
	pending []interview.PendingAudio
}

func (f *fakeStateStore) UpdateAudioState(_ context.Context, questionID int64, state string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.states == nil {
		f.states = map[int64]string{}
	}
	f.states[questionID] = state
	return nil
}

func (f *fakeStateStore) ActiveQuestionsNeedingAudio(_ context.Context) ([]interview.PendingAudio, error) {
	return f.pending, nil
}

// flakySynth 前 failures 次调用失败，之后成功。
type flakySynth struct {
	mu       sync.Mutex
	failures int
	calls    int
}

func (f *flakySynth) Synthesize(_ context.Context, _ SynthesisRequest) (SynthesisResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.calls <= f.failures {
		return SynthesisResult{}, errors.New("boom")
	}
	return SynthesisResult{Audio: []byte("RIFFok"), ContentType: "audio/wav"}, nil
}

func waitForState(t *testing.T, store *fakeStateStore, questionID int64, want string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		store.mu.Lock()
		got := store.states[questionID]
		store.mu.Unlock()
		if got == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("question %d state never became %q", questionID, want)
}

func TestGeneratorRetriesThenMarksReady(t *testing.T) {
	synth := &flakySynth{failures: 1}
	states := &fakeStateStore{}
	cfg := Config{MaxAttempts: 3, Backoff: []time.Duration{time.Millisecond}}
	generator := NewGenerator(synth, NewAudioStore(t.TempDir()), states, cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	generator.Start(ctx)
	defer generator.Close()

	generator.Enqueue(ctx, 1, 10, 100, "题面")
	waitForState(t, states, 100, "ready")

	data, err := generator.store.ReadQuestionAudio(1, 10, 100)
	if err != nil || string(data) != "RIFFok" {
		t.Fatalf("audio = %q err = %v", data, err)
	}
}

func TestGeneratorMarksFailedAfterMaxAttempts(t *testing.T) {
	synth := &flakySynth{failures: 99}
	states := &fakeStateStore{}
	cfg := Config{MaxAttempts: 2, Backoff: []time.Duration{time.Millisecond}}
	generator := NewGenerator(synth, NewAudioStore(t.TempDir()), states, cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	generator.Start(ctx)
	defer generator.Close()

	generator.Enqueue(ctx, 1, 10, 100, "题面")
	waitForState(t, states, 100, "failed")
}

func TestGeneratorEnqueueDeduplicatesSameQuestion(t *testing.T) {
	synth := &flakySynth{}
	states := &fakeStateStore{}
	cfg := Config{MaxAttempts: 1, Backoff: []time.Duration{time.Millisecond}}
	generator := NewGenerator(synth, NewAudioStore(t.TempDir()), states, cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	generator.Start(ctx)
	defer generator.Close()

	generator.Enqueue(ctx, 1, 10, 100, "题面")
	generator.Enqueue(ctx, 1, 10, 100, "题面")
	waitForState(t, states, 100, "ready")
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		synth.mu.Lock()
		calls := synth.calls
		synth.mu.Unlock()
		if calls > 1 {
			t.Fatalf("synthesized %d times, want 1", calls)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestGeneratorReloadEnqueuesPendingAudio(t *testing.T) {
	synth := &flakySynth{}
	states := &fakeStateStore{pending: []interview.PendingAudio{
		{TenantID: 1, InterviewID: 10, QuestionID: 100, Text: "补齐"},
	}}
	cfg := Config{MaxAttempts: 1, Backoff: []time.Duration{time.Millisecond}}
	generator := NewGenerator(synth, NewAudioStore(t.TempDir()), states, cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	generator.Start(ctx)
	defer generator.Close()

	if err := generator.Reload(ctx); err != nil {
		t.Fatalf("reload: %v", err)
	}
	waitForState(t, states, 100, "ready")
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd backend && go test ./internal/speech/ -run TestGenerator -v`
Expected: FAIL —— `NewGenerator undefined`。

- [ ] **Step 3: 实现 `backend/internal/speech/generator.go`**

```go
package speech

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"resumepro/backend/internal/interview"
)

// AudioStateStore 是生成器需要的最小持久化接口，由 interview.Repository 实现。
type AudioStateStore interface {
	UpdateAudioState(ctx context.Context, questionID int64, state string) error
	ActiveQuestionsNeedingAudio(ctx context.Context) ([]interview.PendingAudio, error)
}

// Job 是一条待生成的题目语音。
type Job struct {
	TenantID    int64
	InterviewID int64
	QuestionID  int64
	Text        string
}

// Generator 是后台串行生成器：单 worker 消费队列，逐题合成、落盘并回写 audio_state。
//
// 播放接口只读文件，绝不在请求内合成 —— TTS 单题数秒到数十秒，
// 放进请求会让用户干等，还会与后台队列抢 GPU。
type Generator struct {
	synth Synthesizer
	store *AudioStore
	repo  AudioStateStore

	maxAttempts int
	backoff     []time.Duration

	jobs    chan Job
	enqueue sync.Map // questionID -> struct{}，防止同一题重复入队
	closed  chan struct{}
	once    sync.Once
}

// NewGenerator 创建生成器（不自动启动，需显式 Start）。
func NewGenerator(synth Synthesizer, store *AudioStore, repo AudioStateStore, cfg Config) *Generator {
	backoff := cfg.Backoff
	if len(backoff) == 0 {
		backoff = []time.Duration{time.Second}
	}
	maxAttempts := cfg.MaxAttempts
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	return &Generator{
		synth:       synth,
		store:       store,
		repo:        repo,
		maxAttempts: maxAttempts,
		backoff:     backoff,
		jobs:        make(chan Job, 256),
		closed:      make(chan struct{}),
	}
}

// Start 启动后台 worker。
func (g *Generator) Start(ctx context.Context) {
	go g.run(ctx)
}

// Close 停止 worker。
func (g *Generator) Close() {
	g.once.Do(func() { close(g.closed) })
}

// Enqueue 把题目加入生成队列。
//
// 绝不阻塞调用方：创建面试与答题链路不能被语音拖住。
// 队列满时丢弃该题 —— 下次 Reload 会补齐。
func (g *Generator) Enqueue(ctx context.Context, tenantID, interviewID, questionID int64, text string) {
	if text == "" {
		return
	}
	if _, loaded := g.enqueue.LoadOrStore(questionID, struct{}{}); loaded {
		return
	}
	job := Job{TenantID: tenantID, InterviewID: interviewID, QuestionID: questionID, Text: text}
	select {
	case g.jobs <- job:
	default:
		g.enqueue.Delete(questionID)
		logrus.WithField("question_id", questionID).Warn("voice: generation queue full, skip")
	}
}

// Reload 在服务重启后扫描「进行中且未就绪」的题目并重新入队。
// 只读数据库，不依赖内存队列，因此重启后能完整补齐。
func (g *Generator) Reload(ctx context.Context) error {
	items, err := g.repo.ActiveQuestionsNeedingAudio(ctx)
	if err != nil {
		return fmt.Errorf("speech: reload pending audio: %w", err)
	}
	for _, item := range items {
		g.Enqueue(ctx, item.TenantID, item.InterviewID, item.QuestionID, item.Text)
	}
	return nil
}

// CleanupInterview 清理一场面试的音频目录（面试终态时由 interview 层调用）。
func (g *Generator) CleanupInterview(ctx context.Context, tenantID, interviewID int64) {
	if err := g.store.CleanupInterview(tenantID, interviewID); err != nil {
		logrus.WithError(err).WithField("interview_id", interviewID).Warn("voice: cleanup interview audio failed")
	}
}

func (g *Generator) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-g.closed:
			return
		case job := <-g.jobs:
			g.process(ctx, job)
		}
	}
}

// process 合成一题：失败按退避重试，耗尽后标记 failed。
func (g *Generator) process(ctx context.Context, job Job) {
	defer g.enqueue.Delete(job.QuestionID)
	_ = g.repo.UpdateAudioState(ctx, job.QuestionID, "pending")

	var lastErr error
	for attempt := 0; attempt < g.maxAttempts; attempt++ {
		res, err := g.synth.Synthesize(ctx, SynthesisRequest{Text: job.Text})
		if err == nil {
			if saveErr := g.store.SaveQuestionAudio(ctx, job.TenantID, job.InterviewID, job.QuestionID, res.Audio); saveErr == nil {
				_ = g.repo.UpdateAudioState(ctx, job.QuestionID, "ready")
				return
			} else {
				err = saveErr
			}
		}
		lastErr = err
		if attempt == g.maxAttempts-1 {
			break
		}
		delay := g.backoff[min(attempt, len(g.backoff)-1)]
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return
		case <-g.closed:
			return
		}
	}

	_ = g.repo.UpdateAudioState(ctx, job.QuestionID, "failed")
	logrus.WithError(lastErr).WithFields(logrus.Fields{
		"interview_id": job.InterviewID,
		"question_id":  job.QuestionID,
	}).Warn("voice: question audio generation failed")
}

// Generator 实现 interview.AudioHooks。
var _ interview.AudioHooks = (*Generator)(nil)
```

- [ ] **Step 4: 运行测试确认通过**

Run: `cd backend && go test ./internal/speech/ -v`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add backend/internal/speech/generator.go backend/internal/speech/generator_test.go
git commit -m "feat(speech): 新增后台串行生成器与重启重载"
```

---

### Task 5: 转写文本的错别字修复

**Files:**
- Create: `backend/internal/speech/correct.go`
- Test: `backend/internal/speech/correct_test.go`

**Interfaces:**
- Produces:
  - `speech.Corrector` + `speech.NewCorrector(model TextModel, timeout time.Duration) *Corrector`
  - `(*Corrector).Correct(ctx context.Context, text string) string`

- [ ] **Step 1: 写测试**

新建 `backend/internal/speech/correct_test.go`：

```go
package speech

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// stubTextModel 返回固定文本或错误。
type stubTextModel struct {
	text string
	err  error
}

func (s *stubTextModel) Generate(_ context.Context, _ []*schema.AgenticMessage, _ ...model.Option) (*schema.AgenticMessage, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &schema.AgenticMessage{
		Role:          schema.AgenticRoleTypeAssistant,
		ContentBlocks: []*schema.ContentBlock{schema.NewContentBlock(&schema.AssistantGenText{Text: s.text})},
	}, nil
}

func TestCorrectorReturnsCorrectedText(t *testing.T) {
	corrector := NewCorrector(&stubTextModel{text: "我做过 Golang 的后台服务"}, time.Second)
	got := corrector.Correct(context.Background(), "我做过 go lang 的后台服务")
	if got != "我做过 Golang 的后台服务" {
		t.Fatalf("corrected = %q", got)
	}
}

func TestCorrectorFallsBackToRawTextOnError(t *testing.T) {
	corrector := NewCorrector(&stubTextModel{err: errors.New("llm down")}, time.Second)
	raw := "识别出来的原话"
	if got := corrector.Correct(context.Background(), raw); got != raw {
		t.Fatalf("corrected = %q, want raw %q", got, raw)
	}
}

func TestCorrectorFallsBackWhenModelMissing(t *testing.T) {
	corrector := NewCorrector(nil, time.Second)
	raw := "原话"
	if got := corrector.Correct(context.Background(), raw); got != raw {
		t.Fatalf("corrected = %q, want raw", got)
	}
}

func TestCorrectorPromptForbidsRewriting(t *testing.T) {
	if !strings.Contains(correctSystemPrompt, "不得改变原意") {
		t.Error("system prompt must forbid semantic rewriting")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd backend && go test ./internal/speech/ -run TestCorrector -v`
Expected: FAIL —— `NewCorrector undefined`。

- [ ] **Step 3: 实现 `backend/internal/speech/correct.go`**

```go
package speech

import (
	"context"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/sirupsen/logrus"
)

// correctSystemPrompt 约束模型只做错别字校对。
// 思考强度固定最低档（调用方用 ReasoningEffort=low 的独立模型实例）：
// 这是纯校对任务，不需要长思考。
const correctSystemPrompt = `你是中文语音转写校对器。用户给你一段语音转写文本，你只需要修正错别字、同音字、明显的标点错误。
硬性要求：
1. 不得改变原意，不得润色、扩写、缩写或调整语序。
2. 不得增加原文没有的信息。
3. 只输出校对后的纯文本，不要任何解释、前缀或引号。`

// Corrector 用 LLM 修复转写文本的错别字。
type Corrector struct {
	model   TextModel
	timeout time.Duration
}

// NewCorrector 创建校对器。
func NewCorrector(model TextModel, timeout time.Duration) *Corrector {
	return &Corrector{model: model, timeout: timeout}
}

// Correct 返回修复后的文本。
// 模型不可用、报错、超时或返回空时返回原文 —— 校对失败不阻断转写。
func (c *Corrector) Correct(ctx context.Context, text string) string {
	trimmed := strings.TrimSpace(text)
	if c == nil || c.model == nil || trimmed == "" {
		return text
	}
	timeout := c.timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resp, err := c.model.Generate(callCtx, []*schema.AgenticMessage{
		schema.SystemAgenticMessage(correctSystemPrompt),
		schema.UserAgenticMessage(trimmed),
	})
	if err != nil {
		logrus.WithError(err).Warn("voice: text correction failed, use raw transcript")
		return text
	}
	corrected := strings.TrimSpace(agenticText(resp))
	if corrected == "" {
		return text
	}
	return corrected
}

// agenticText 拼接 assistant 响应中的全部生成文本块。
func agenticText(msg *schema.AgenticMessage) string {
	if msg == nil {
		return ""
	}
	var sb strings.Builder
	for _, block := range msg.ContentBlocks {
		if block == nil || block.AssistantGenText == nil {
			continue
		}
		sb.WriteString(block.AssistantGenText.Text)
	}
	return sb.String()
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `cd backend && go test ./internal/speech/ -v`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add backend/internal/speech/correct.go backend/internal/speech/correct_test.go
git commit -m "feat(speech): 新增转写文本错别字修复"
```

---

### Task 6: interview 层接入语音触发

**Files:**
- Modify: `backend/internal/interview/service.go`
- Test: `backend/internal/interview/service_test.go`

**Interfaces:**
- Consumes: Task 1 的 `interview.AudioHooks`
- Produces:
  - `interview.ServiceOption`、`interview.WithAudioHooks(hooks AudioHooks) ServiceOption`
  - `(*Service).HasActiveInterview(ctx, tenantID int64) (bool, error)`
  - `(*Service).QuestionForAudio(ctx, tenantID, interviewID, questionID int64) (*Question, error)`

- [ ] **Step 1: 写测试**

在 `backend/internal/interview/service_test.go` 追加（复用该文件既有 fixture 辅助函数）：

```go
// fakeAudioHooks 记录入队与清理调用。
type fakeAudioHooks struct {
	mu       sync.Mutex
	enqueued [][]int64 // {tenantID, interviewID, questionID}
	cleaned  []int64
}

func (f *fakeAudioHooks) Enqueue(_ context.Context, tenantID, interviewID, questionID int64, _ string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.enqueued = append(f.enqueued, []int64{tenantID, interviewID, questionID})
}

func (f *fakeAudioHooks) CleanupInterview(_ context.Context, _ int64, interviewID int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cleaned = append(f.cleaned, interviewID)
}

func TestCreateInterviewEnqueuesAllMainQuestions(t *testing.T) {
	hooks := &fakeAudioHooks{}
	svc, ctx, tenantID := newServiceFixture(t, WithAudioHooks(hooks))

	iv, err := svc.CreateInterview(ctx, tenantID, 1, 1, "简历正文", nil, nil)
	if err != nil {
		t.Fatalf("create interview: %v", err)
	}
	mains, err := svc.repo.MainQuestions(ctx, iv.ID)
	if err != nil {
		t.Fatalf("main questions: %v", err)
	}
	if len(hooks.enqueued) != len(mains) {
		t.Fatalf("enqueued %d questions, want %d", len(hooks.enqueued), len(mains))
	}
}

func TestAnswerEnqueuesNextQuestion(t *testing.T) {
	hooks := &fakeAudioHooks{}
	svc, ctx, tenantID := newServiceFixture(t, WithAudioHooks(hooks))
	iv, _ := svc.CreateInterview(ctx, tenantID, 1, 1, "简历正文", nil, nil)
	before := len(hooks.enqueued)

	if _, err := svc.Answer(ctx, tenantID, iv.ID, AnswerPayload{Answer: "回答"}); err != nil {
		t.Fatalf("answer: %v", err)
	}
	if len(hooks.enqueued) <= before {
		t.Fatal("next question was not enqueued for audio generation")
	}
}

func TestAbortCleansUpAudio(t *testing.T) {
	hooks := &fakeAudioHooks{}
	svc, ctx, tenantID := newServiceFixture(t, WithAudioHooks(hooks))
	iv, _ := svc.CreateInterview(ctx, tenantID, 1, 1, "简历正文", nil, nil)

	if err := svc.Abort(ctx, tenantID, iv.ID); err != nil {
		t.Fatalf("abort: %v", err)
	}
	if len(hooks.cleaned) != 1 || hooks.cleaned[0] != iv.ID {
		t.Fatalf("cleaned = %v, want interview %d", hooks.cleaned, iv.ID)
	}
}
```

> `newServiceFixture` 若尚不存在，就在 `service_test.go` 里补一个：用内存 SQLite + miniredis 建仓储、RedisStore、生题器（stub 模型）与面试图，返回 `(*Service, context.Context, tenantID)`。

- [ ] **Step 2: 运行测试确认失败**

Run: `cd backend && go test ./internal/interview/ -run 'Test(CreateInterviewEnqueues|AnswerEnqueues|AbortCleansUp)' -v`
Expected: FAIL —— `WithAudioHooks undefined`。

- [ ] **Step 3: 实现 Service 改动**

在 `backend/internal/interview/service.go` 追加：

```go
// ServiceOption 是 Service 的可选配置。
type ServiceOption func(*Service)

// WithAudioHooks 注入语音能力。
// 不注入时语音链路整体缺席，面试照常工作（纯文字模式）。
func WithAudioHooks(hooks AudioHooks) ServiceOption {
	return func(s *Service) {
		s.audio = hooks
	}
}
```

把 `Service` 结构体与构造函数改为：

```go
type Service struct {
	repo  *Repository
	store *RedisStore
	gen   *QuestionGenerator
	graph *Graph
	// audio 是可选的语音能力；nil 表示语音缺席。
	audio AudioHooks
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
```

在 `CreateInterview` 成功返回前（`graph.StartInterview` 之后）追加：

```go
	// 题目语音在后台生成：面试主链路不等语音（语音不可用也不影响创建）。
	if s.audio != nil {
		mains, err := s.repo.MainQuestions(ctx, iv.ID)
		if err != nil {
			return nil, fmt.Errorf("interview: load main questions for audio: %w", err)
		}
		for _, q := range mains {
			s.audio.Enqueue(ctx, tenantID, iv.ID, q.ID, q.Question)
		}
	}
```

在 `Answer` 中 `graph.Answer` 成功之后、return 之前追加：

```go
	// 下一题的语音在后台生成；面试结束时清理本场音频。
	if s.audio != nil {
		if res.Finished {
			s.audio.CleanupInterview(ctx, tenantID, interviewID)
		} else if res.NextQuestion != nil {
			s.audio.Enqueue(ctx, tenantID, interviewID, res.NextQuestion.ID, res.NextQuestion.Question)
		}
	}
```

在 `Abort` 中 `store.Delete` 之后追加：

```go
	if s.audio != nil {
		s.audio.CleanupInterview(ctx, tenantID, interviewID)
	}
```

再追加两个供 HTTP 层使用的窄方法：

```go
// HasActiveInterview 报告该租户是否已有进行中的面试（创建入口预检）。
func (s *Service) HasActiveInterview(ctx context.Context, tenantID int64) (bool, error) {
	return s.repo.HasActiveInterview(ctx, tenantID)
}

// QuestionForAudio 校验面试与题目的归属并返回题目行（含 AudioState）。
// 题目不属于该面试时返回 ErrNotFound（不泄露存在性）。
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
```

- [ ] **Step 4: 运行测试确认通过**

Run: `cd backend && go test ./internal/interview/ -v`
Expected: PASS。

- [ ] **Step 5: 全量测试并提交**

Run: `cd backend && go build ./... && go test ./...`
Expected: 全部通过（`NewService` 增加了可变参数，既有调用点无需改）。

```bash
git add backend/internal/interview/service.go backend/internal/interview/service_test.go
git commit -m "feat(interview): 接入语音生成触发与面试终态清理"
```

---
### Task 7: HTTP 接口（播放 / 转写 / DTO / 创建预检）

**Files:**
- Create: `backend/internal/httpapi/voice_handler.go`
- Create: `backend/internal/speech/service.go`
- Modify: `backend/internal/httpapi/router.go`
- Modify: `backend/internal/httpapi/interview_handler.go`
- Test: `backend/internal/httpapi/voice_handler_test.go`

**Interfaces:**
- Consumes: Task 5 的 `speech.Corrector`、Task 2 的 `speech.Transcriber`、Task 3 的 `speech.AudioStore`、Task 6 的 `Service.QuestionForAudio`
- Produces:
  - `GET /api/interviews/:id/questions/:qid/audio` → 200 `audio/wav` / 409 / 503
  - `POST /api/interviews/:id/questions/:qid/transcribe` → 200 `{"text":"..."}` / 400 / 503
  - 题目 DTO 新增 `audioState`；面试 DTO 新增 `voice: {enabled}`
  - `httpapi.RouterConfig.VoiceService *speech.VoiceService`

- [ ] **Step 1: 写接口测试**

新建 `backend/internal/httpapi/voice_handler_test.go`：

```go
package httpapi

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"resumepro/backend/internal/speech"
)

// wavBytes 生成一个最小的合法 16kHz 单声道 WAV。
func wavBytes(samples int) []byte {
	dataSize := samples * 2
	buf := &bytes.Buffer{}
	_ = binary.Write(buf, binary.LittleEndian, []byte("RIFF"))
	_ = binary.Write(buf, binary.LittleEndian, uint32(36+dataSize))
	_ = binary.Write(buf, binary.LittleEndian, []byte("WAVE"))
	_ = binary.Write(buf, binary.LittleEndian, []byte("fmt "))
	_ = binary.Write(buf, binary.LittleEndian, uint32(16))
	_ = binary.Write(buf, binary.LittleEndian, uint16(1))
	_ = binary.Write(buf, binary.LittleEndian, uint16(1))
	_ = binary.Write(buf, binary.LittleEndian, uint32(16000))
	_ = binary.Write(buf, binary.LittleEndian, uint32(32000))
	_ = binary.Write(buf, binary.LittleEndian, uint16(2))
	_ = binary.Write(buf, binary.LittleEndian, uint16(16))
	_ = binary.Write(buf, binary.LittleEndian, []byte("data"))
	_ = binary.Write(buf, binary.LittleEndian, uint32(dataSize))
	buf.Write(make([]byte, dataSize))
	return buf.Bytes()
}

// stubTranscriber 返回固定文本。
type stubTranscriber struct {
	text string
}

func (s *stubTranscriber) Transcribe(_ context.Context, _ speech.TranscriptionRequest) (speech.TranscriptionResult, error) {
	return speech.TranscriptionResult{Text: s.text}, nil
}

func TestVoiceAudioReturns404ForUnknownInterview(t *testing.T) {
	router, cookie, _ := setupVoiceAPI(t)
	response := requestJSON(t, router, http.MethodGet, "/api/interviews/999/questions/1/audio", "", cookie)
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", response.Code)
	}
}

func TestVoiceTranscribeRejectsInvalidAudio(t *testing.T) {
	router, cookie, _ := setupVoiceAPI(t)
	body := &bytes.Buffer{}
	_ = body.WriteString("not a wav file at all, definitely not riff")
	request := httptest.NewRequest(http.MethodPost, "/api/interviews/1/questions/1/transcribe", body)
	request.Header.Set("Content-Type", "application/octet-stream")
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
}

func TestVoiceTranscribeReturnsText(t *testing.T) {
	router, cookie, _ := setupVoiceAPI(t)
	interviewID := createVoiceInterview(t, router, cookie)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "recording.wav")
	_, _ = part.Write(wavBytes(16000))
	_ = writer.Close()

	path := "/api/interviews/" + strconv.FormatInt(interviewID, 10) + "/questions/1/transcribe"
	request := httptest.NewRequest(http.MethodPost, path, body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.Text != "识别出来的文字" {
		t.Errorf("text = %q", payload.Text)
	}
}
```

在同一文件补两个辅助函数（`setupVoiceAPI` 与 `createVoiceInterview`）：

```go
// setupVoiceAPI 复用既有面试路由测试的装配（内存 SQLite + miniredis + stub 模型），
// 额外注入一个语音服务（内存 AudioStore + stub transcriber）。
//
// 实现建议：把 setupInterviewAPI 的装配逻辑抽成一个返回 RouterConfig 的公共函数，
// 两个 setup 各自填不同的 VoiceService，避免复制整段装配代码。
func setupVoiceAPI(t *testing.T) (http.Handler, *http.Cookie, *sqlx.DB) {
	t.Helper()
	// ... 参照 setupInterviewAPI 的装配代码，RouterConfig 额外设置：
	// VoiceService: speech.NewVoiceService(speech.NewAudioStore(t.TempDir()), &stubTranscriber{text: "识别出来的文字"}, nil)
	// ... 
	return router, registerAuthUser(t, router, "voice@example.com"), db
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd backend && go test ./internal/httpapi/ -run TestVoice -v`
Expected: FAIL —— 路由未注册（404）。

- [ ] **Step 3: 实现 `backend/internal/speech/service.go`（门面）**

```go
package speech

// VoiceService 是语音能力对 HTTP 层暴露的窄门面。
type VoiceService struct {
	Store       *AudioStore
	Transcriber Transcriber
	Corrector   *Corrector
}

// NewVoiceService 组装门面。
func NewVoiceService(store *AudioStore, transcriber Transcriber, corrector *Corrector) *VoiceService {
	return &VoiceService{Store: store, Transcriber: transcriber, Corrector: corrector}
}
```

- [ ] **Step 4: 实现 `backend/internal/httpapi/voice_handler.go`**

```go
package httpapi

import (
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"resumepro/backend/internal/interview"
	"resumepro/backend/internal/speech"
)

// voiceHandler 是语音能力的 HTTP 层：播放题目语音、转写录音。
//
// service 为 nil 时所有端点返回 503（语音未启用时的既有降级约定）。
type voiceHandler struct {
	service     *interview.Service
	store       *speech.AudioStore
	transcriber speech.Transcriber
	corrector   *speech.Corrector
}

func newVoiceHandler(service *interview.Service, store *speech.AudioStore, transcriber speech.Transcriber, corrector *speech.Corrector) *voiceHandler {
	return &voiceHandler{service: service, store: store, transcriber: transcriber, corrector: corrector}
}

// audio 返回题目语音（TTS 产物）。只读已生成的文件，不在请求内合成。
func (h *voiceHandler) audio(ginCtx *gin.Context) {
	principal, ok := principalFromContext(ginCtx)
	if !ok {
		writeError(ginCtx, http.StatusUnauthorized, "authentication required")
		return
	}
	if h.service == nil || h.store == nil {
		writeError(ginCtx, http.StatusServiceUnavailable, "voice features disabled")
		return
	}
	interviewID, ok := interviewRouteID(ginCtx)
	if !ok {
		return
	}
	questionID, ok := voiceQuestionRouteID(ginCtx)
	if !ok {
		return
	}
	question, err := h.service.QuestionForAudio(ginCtx.Request.Context(), principal.TenantID, interviewID, questionID)
	if err != nil {
		if !handleInterviewError(ginCtx, err) {
			return
		}
	}
	state := ""
	if question.AudioState != nil {
		state = *question.AudioState
	}
	switch state {
	case "ready":
		audio, err := h.store.ReadQuestionAudio(principal.TenantID, interviewID, questionID)
		if err != nil {
			writeError(ginCtx, http.StatusServiceUnavailable, "question audio unavailable")
			return
		}
		ginCtx.Data(http.StatusOK, "audio/wav", audio)
	case "failed":
		writeError(ginCtx, http.StatusServiceUnavailable, "question audio generation failed")
	default:
		// pending 或 NULL：还没生成好，前端轮询等待。
		ginCtx.JSON(http.StatusConflict, gin.H{"state": "pending"})
	}
}

// transcribe 上传录音 → ASR 转写 → AI 修错别字 → 返回文本。
func (h *voiceHandler) transcribe(ginCtx *gin.Context) {
	principal, ok := principalFromContext(ginCtx)
	if !ok {
		writeError(ginCtx, http.StatusUnauthorized, "authentication required")
		return
	}
	if h.service == nil || h.transcriber == nil {
		writeError(ginCtx, http.StatusServiceUnavailable, "voice features disabled")
		return
	}
	interviewID, ok := interviewRouteID(ginCtx)
	if !ok {
		return
	}
	questionID, ok := voiceQuestionRouteID(ginCtx)
	if !ok {
		return
	}
	if _, err := h.service.QuestionForAudio(ginCtx.Request.Context(), principal.TenantID, interviewID, questionID); err != nil {
		if !handleInterviewError(ginCtx, err) {
			return
		}
	}

	header, err := ginCtx.FormFile("file")
	if err != nil {
		writeError(ginCtx, http.StatusBadRequest, "audio file is required")
		return
	}
	if header.Size > speech.MaxAudioBytes {
		writeError(ginCtx, http.StatusBadRequest, "audio file is too large")
		return
	}
	file, err := header.Open()
	if err != nil {
		writeError(ginCtx, http.StatusBadRequest, "audio file is required")
		return
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, speech.MaxAudioBytes+1))
	if err != nil {
		writeError(ginCtx, http.StatusBadRequest, "could not read audio file")
		return
	}
	if _, err := speech.ValidateWAV(data); err != nil {
		writeError(ginCtx, http.StatusBadRequest, "invalid audio file")
		return
	}

	result, err := h.transcriber.Transcribe(ginCtx.Request.Context(), speech.TranscriptionRequest{Audio: data, ContentType: "audio/wav"})
	if err != nil {
		writeError(ginCtx, http.StatusServiceUnavailable, "transcription unavailable")
		return
	}
	text := result.Text
	if h.corrector != nil {
		// 校对失败时 corrector 内部返回原文，不阻断转写。
		text = h.corrector.Correct(ginCtx.Request.Context(), text)
	}
	writeJSON(ginCtx, http.StatusOK, map[string]any{"text": text})
}

// voiceQuestionRouteID 解析路径里的 qid。
func voiceQuestionRouteID(context *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(context.Param("qid"), 10, 64)
	if err != nil || id < 1 {
		writeError(context, http.StatusNotFound, "question not found")
		return 0, false
	}
	return id, true
}
```

- [ ] **Step 5: 注册路由**

在 `backend/internal/httpapi/router.go` 的 `RouterConfig` 结构体追加：

```go
	// VoiceService is nil when voice features are disabled or the speech
	// provider is unavailable; the authenticated routes remain registered and
	// return an explicit 503.
	VoiceService *speech.VoiceService
```

在 `interviews` 组注册路由之后追加：

```go
	if config.VoiceService != nil {
		voiceHandler := newVoiceHandler(config.InterviewService, config.VoiceService.Store, config.VoiceService.Transcriber, config.VoiceService.Corrector)
		interviews.GET("/:id/questions/:qid/audio", voiceHandler.audio)
		interviews.POST("/:id/questions/:qid/transcribe", voiceHandler.transcribe)
	}
```

并在 import 区块加 `"resumepro/backend/internal/speech"`。

- [ ] **Step 6: DTO 改动与创建预检**

在 `backend/internal/httpapi/interview_handler.go`：

1. `questionDTO` 的 map 里追加一行：

```go
		"audioState":      q.AudioState,
```

2. `interviewDTO` 改成方法（要带上语音开关）：

```go
func (h *interviewHandler) interviewDTO(iv *interview.Interview) map[string]any {
	var config any = map[string]any{}
	if iv.ConfigSnapshot != "" {
		var cfg interview.InterviewConfig
		if err := json.Unmarshal([]byte(iv.ConfigSnapshot), &cfg); err == nil {
			config = cfg
		}
	}
	return map[string]any{
		"id":             iv.ID,
		"targetJobId":    iv.TargetJobID,
		"resumeId":       iv.ResumeID,
		"configSnapshot": config,
		"status":         iv.Status,
		"overallGrade":   iv.OverallGrade,
		"overallReview":  iv.OverallReview,
		"createdAt":      iv.CreatedAt,
		"updatedAt":      iv.UpdatedAt,
		"completedAt":    iv.CompletedAt,
		"voice":          map[string]any{"enabled": h.voiceEnabled},
	}
}
```

3. `interviewHandler` 增加字段与构造参数：

```go
type interviewHandler struct {
	service      *interview.Service
	resumeRepo   *resume.Repository
	targetSvc    *targetjob.Service
	voiceEnabled bool
}

func newInterviewHandler(service *interview.Service, resumeRepo *resume.Repository, targetSvc *targetjob.Service, voiceEnabled bool) *interviewHandler {
	return &interviewHandler{service: service, resumeRepo: resumeRepo, targetSvc: targetSvc, voiceEnabled: voiceEnabled}
}
```

4. 所有 `interviewDTO(iv)` 调用点改成 `h.interviewDTO(iv)`。

5. `create` 入口前置预检（**放在解析简历/JD 之前**）：

```go
	if h.service == nil {
		writeError(ginCtx, http.StatusServiceUnavailable, "AI interviews disabled (Redis required)")
		return
	}
	// 预检最前置：已有进行中的面试时直接拒绝，不去加载简历与 JD，
	// 避免白跑一次 LLM 生题。
	hasActive, err := h.service.HasActiveInterview(ginCtx.Request.Context(), principal.TenantID)
	if err != nil {
		writeError(ginCtx, http.StatusInternalServerError, "could not check active interview")
		return
	}
	if hasActive {
		writeError(ginCtx, http.StatusConflict, interview.ErrActiveInterviewExists.Error())
		return
	}
```

- [ ] **Step 7: 运行 httpapi 全部测试**

Run: `cd backend && go test ./internal/httpapi/ -v`
Expected: PASS（既有面试测试可能因 `newInterviewHandler` 签名变化需要同步更新调用点）。

- [ ] **Step 8: Commit**

```bash
git add backend/internal/httpapi/voice_handler.go backend/internal/httpapi/voice_handler_test.go backend/internal/httpapi/router.go backend/internal/httpapi/interview_handler.go backend/internal/httpapi/interview_handler_test.go backend/internal/speech/service.go
git commit -m "feat(interview): 新增题目语音播放与录音转写接口"
```

---

### Task 8: main.go 接线与启动重载

**Files:**
- Modify: `backend/cmd/server/main.go`

**Interfaces:**
- Consumes: Task 2/3/4/5 的 `speech` 构造器、Task 7 的 `speech.VoiceService`
- Produces: 进程启动时后台 worker 运行 + `Reload` 补齐未生成的题目语音

- [ ] **Step 1: 在 `main.go` 装配语音能力**

在 `backend/cmd/server/main.go` 追加：

```go
// buildVoiceServices 装配语音能力：provider、存储、生成器、错别字修复。
// VOICE_ENABLED 未开时返回 nil（路由仍注册，接口返回 503）。
func buildVoiceServices(db *sqlx.DB, llmConfig llm.Config, logger *logrus.Logger) (*speech.VoiceService, *speech.Generator, error) {
	cfg := speech.LoadConfigFromEnv()
	if !cfg.Enabled {
		logger.Warn("voice features disabled (set VOICE_ENABLED=true to enable)")
		return nil, nil, nil
	}
	provider := speech.NewOpenAICompatibleProvider(cfg)
	store := speech.NewAudioStore(cfg.CacheDir)

	// 错别字修复用独立模型实例：思考强度固定最低档（纯校对任务不需要长思考）。
	correctConfig := llmConfig
	correctConfig.ReasoningEffort = "low"
	correctConfig.Timeout = cfg.CorrectTimeout
	correctModel, err := llm.NewResponsesModel(correctConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("create correction model: %w", err)
	}

	generator := speech.NewGenerator(provider, store, interview.NewRepository(db), cfg)
	corrector := speech.NewCorrector(correctModel, cfg.CorrectTimeout)
	return speech.NewVoiceService(store, provider, corrector), generator, nil
}
```

- [ ] **Step 2: 在 `buildRouterConfig` 里接线**

在 `buildRouterConfig` 中 `config.InterviewService = interviewService` 之后追加：

```go
	voiceService, voiceGenerator, err := buildVoiceServices(db, llmConfig, logger)
	if err != nil {
		return config, err
	}
	if voiceService != nil {
		config.VoiceService = voiceService
		// 后台生成 worker：串行消费，不阻塞任何请求。
		voiceGenerator.Start(context.Background())
		// 重启重载：扫描「进行中且未就绪」的题目并重新入队。
		if err := voiceGenerator.Reload(context.Background()); err != nil {
			logger.WithError(err).Warn("voice: reload pending question audio failed")
		}
		logger.WithField("cache_dir", voiceService.Store.Root()).Info("voice features enabled")
	}
```

- [ ] **Step 3: 编译与全量测试**

Run: `cd backend && go build ./... && go test ./...`
Expected: 全部通过。

- [ ] **Step 4: Commit**

```bash
git add backend/cmd/server/main.go
git commit -m "feat(voice): 接线语音服务并启动时重载未生成的题目语音"
```

---

### Task 9: 前端语音接口层与 WAV 工具

**Files:**
- Create: `frontend/src/api/voice.ts`
- Create: `frontend/src/utils/wav.ts`
- Test: `frontend/src/api/voice.test.ts`、`frontend/src/utils/wav.test.ts`

**Interfaces:**
- Produces:
  - `frontend/src/api/voice.ts`：`AudioState`、`fetchQuestionAudio(interviewId, questionId): Promise<Blob>`、`transcribeRecording(interviewId, questionId, blob): Promise<string>`
  - `frontend/src/utils/wav.ts`：`TARGET_SAMPLE_RATE`、`resample(input, inputRate, outputRate?)`、`mixToMono(channels)`、`encodeWavPcm16(samples, sampleRate): ArrayBuffer`

- [ ] **Step 1: 写 wav 工具测试**

新建 `frontend/src/utils/wav.test.ts`：

```ts
import { describe, expect, it } from 'vitest';

import { encodeWavPcm16, mixToMono, resample, TARGET_SAMPLE_RATE } from './wav';

describe('wav utils', () => {
  it('mixes channels into mono by averaging', () => {
    const left = new Float32Array([1, 0, -1]);
    const right = new Float32Array([0, 1, 1]);
    expect(Array.from(mixToMono([left, right]))).toEqual([0.5, 0.5, 0]);
  });

  it('resamples to the target rate', () => {
    const input = new Float32Array(1000).fill(0.5);
    const output = resample(input, 48000);
    expect(output.length).toBe(Math.round(1000 * (TARGET_SAMPLE_RATE / 48000)));
  });

  it('encodes a valid RIFF/WAVE header', () => {
    const samples = new Float32Array([0, 0.5, -0.5]);
    const buffer = encodeWavPcm16(samples, TARGET_SAMPLE_RATE);
    const view = new DataView(buffer);
    const tag = (offset: number) => String.fromCharCode(
      view.getUint8(offset), view.getUint8(offset + 1), view.getUint8(offset + 2), view.getUint8(offset + 3));
    expect(tag(0)).toBe('RIFF');
    expect(tag(8)).toBe('WAVE');
    expect(tag(12)).toBe('fmt ');
    expect(tag(36)).toBe('data');
    expect(view.getUint16(22, true)).toBe(1); // channels
    expect(view.getUint32(24, true)).toBe(TARGET_SAMPLE_RATE);
    expect(buffer.byteLength).toBe(44 + samples.length * 2);
  });
});
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd frontend && npx vitest run src/utils/wav.test.ts`
Expected: FAIL —— 模块不存在。

- [ ] **Step 3: 实现 `frontend/src/utils/wav.ts`**

```ts
/** 语音识别常用的目标采样率：16kHz 单声道。 */
export const TARGET_SAMPLE_RATE = 16000;

/** 把多声道混成单声道（各声道取平均）。 */
export function mixToMono(channels: Float32Array[]): Float32Array {
  if (channels.length === 0) return new Float32Array(0);
  if (channels.length === 1) return channels[0];
  const length = channels[0].length;
  const output = new Float32Array(length);
  for (const channel of channels) {
    for (let index = 0; index < length; index += 1) {
      output[index] += channel[index] ?? 0;
    }
  }
  for (let index = 0; index < length; index += 1) {
    output[index] /= channels.length;
  }
  return output;
}

/** 线性插值重采样。 */
export function resample(
  input: Float32Array,
  inputRate: number,
  outputRate: number = TARGET_SAMPLE_RATE,
): Float32Array {
  if (inputRate === outputRate || input.length === 0) return input;
  const ratio = inputRate / outputRate;
  const length = Math.round(input.length / ratio);
  const output = new Float32Array(length);
  for (let index = 0; index < length; index += 1) {
    const position = index * ratio;
    const left = Math.floor(position);
    const right = Math.min(left + 1, input.length - 1);
    const weight = position - left;
    output[index] = (input[left] ?? 0) * (1 - weight) + (input[right] ?? 0) * weight;
  }
  return output;
}

/** 把 Float32 采样编码成 16bit PCM 的 WAV 文件字节。 */
export function encodeWavPcm16(samples: Float32Array, sampleRate: number): ArrayBuffer {
  const dataSize = samples.length * 2;
  const buffer = new ArrayBuffer(44 + dataSize);
  const view = new DataView(buffer);
  const writeTag = (offset: number, tag: string) => {
    for (let index = 0; index < 4; index += 1) {
      view.setUint8(offset + index, tag.charCodeAt(index));
    }
  };
  writeTag(0, 'RIFF');
  view.setUint32(4, 36 + dataSize, true);
  writeTag(8, 'WAVE');
  writeTag(12, 'fmt ');
  view.setUint32(16, 16, true);
  view.setUint16(20, 1, true); // PCM
  view.setUint16(22, 1, true); // mono
  view.setUint32(24, sampleRate, true);
  view.setUint32(28, sampleRate * 2, true); // byte rate
  view.setUint16(32, 2, true); // block align
  view.setUint16(34, 16, true); // bits per sample
  writeTag(36, 'data');
  view.setUint32(40, dataSize, true);
  for (let index = 0; index < samples.length; index += 1) {
    const clamped = Math.max(-1, Math.min(1, samples[index]));
    view.setInt16(44 + index * 2, clamped < 0 ? clamped * 0x8000 : clamped * 0x7fff, true);
  }
  return buffer;
}
```

- [ ] **Step 4: 运行 wav 测试确认通过**

Run: `cd frontend && npx vitest run src/utils/wav.test.ts`
Expected: PASS。

- [ ] **Step 5: 写语音接口测试**

新建 `frontend/src/api/voice.test.ts`：

```ts
import { afterEach, describe, expect, it, vi } from 'vitest';

import { fetchQuestionAudio, transcribeRecording } from './voice';

afterEach(() => vi.unstubAllGlobals());

describe('voice API', () => {
  it('fetches question audio as a blob', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(new Blob(['RIFF']), { status: 200, headers: { 'Content-Type': 'audio/wav' } }),
    );
    vi.stubGlobal('fetch', fetchMock);

    const blob = await fetchQuestionAudio(3, 12);

    expect(blob.size).toBeGreaterThan(0);
    expect(fetchMock).toHaveBeenCalledWith('/api/interviews/3/questions/12/audio',
      expect.objectContaining({ method: 'GET', credentials: 'include' }));
  });

  it('rejects with the pending state when audio is not ready', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ state: 'pending' }), { status: 409 }),
    );
    vi.stubGlobal('fetch', fetchMock);

    await expect(fetchQuestionAudio(3, 12)).rejects.toMatchObject({ status: 409 });
  });

  it('uploads the recording as multipart without a json content type', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ text: '识别文本' }), { status: 200 }),
    );
    vi.stubGlobal('fetch', fetchMock);

    const text = await transcribeRecording(3, 12, new Blob(['RIFF'], { type: 'audio/wav' }));

    expect(text).toBe('识别文本');
    const [, init] = fetchMock.mock.calls[0];
    expect(init.body).toBeInstanceOf(FormData);
    expect(init.headers['Content-Type']).toBeUndefined();
  });
});
```

- [ ] **Step 6: 实现 `frontend/src/api/voice.ts`**

```ts
/** 题目语音状态：pending=生成中，ready=已就绪，failed=生成失败。 */
export type AudioState = 'pending' | 'ready' | 'failed';

export class VoiceApiError extends Error {
  constructor(message: string, public readonly status: number) {
    super(message);
    this.name = 'VoiceApiError';
  }
}

/** 取题目语音。服务端返回 409 表示还在生成中。 */
export async function fetchQuestionAudio(interviewId: number, questionId: number): Promise<Blob> {
  const response = await fetch(`/api/interviews/${interviewId}/questions/${questionId}/audio`, {
    method: 'GET',
    credentials: 'include',
  });
  if (!response.ok) {
    const body = await response.json().catch(() => ({}));
    throw new VoiceApiError(body.error ?? '语音暂不可用', response.status);
  }
  return response.blob();
}

/** 上传录音并返回识别文本。multipart 不能设置 Content-Type，交给浏览器带 boundary。 */
export async function transcribeRecording(
  interviewId: number,
  questionId: number,
  recording: Blob,
): Promise<string> {
  const form = new FormData();
  form.append('file', recording, 'recording.wav');
  const response = await fetch(`/api/interviews/${interviewId}/questions/${questionId}/transcribe`, {
    method: 'POST',
    credentials: 'include',
    body: form,
  });
  if (!response.ok) {
    const body = await response.json().catch(() => ({}));
    throw new VoiceApiError(body.error ?? '识别失败，可继续打字', response.status);
  }
  const payload = await response.json();
  return payload.text ?? '';
}
```

- [ ] **Step 7: 运行测试并提交**

Run: `cd frontend && npx vitest run src/utils/wav.test.ts src/api/voice.test.ts`
Expected: PASS。

```bash
git add frontend/src/utils/wav.ts frontend/src/utils/wav.test.ts frontend/src/api/voice.ts frontend/src/api/voice.test.ts
git commit -m "feat(voice): 新增前端语音接口层与 WAV 编码工具"
```

---
### Task 10: 录音 hook

**Files:**
- Create: `frontend/src/hooks/useVoiceRecorder.ts`
- Test: `frontend/src/hooks/useVoiceRecorder.test.ts`

**Interfaces:**
- Produces:
  - `useVoiceRecorder(options: { interviewId: number; questionId: number; onText: (text: string) => void; onError: (message: string) => void })`
  - 返回 `{ state: 'idle' | 'recording' | 'transcribing'; seconds: number; start: () => Promise<void>; stop: () => void }`

- [ ] **Step 1: 写测试**

新建 `frontend/src/hooks/useVoiceRecorder.test.ts`：

```ts
import { act, renderHook } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { useVoiceRecorder } from './useVoiceRecorder';

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('useVoiceRecorder', () => {
  it('reports a permission error without throwing', async () => {
    const onError = vi.fn();
    vi.stubGlobal('navigator', {
      mediaDevices: {
        getUserMedia: vi.fn().mockRejectedValue(new Error('denied')),
      },
    });

    const { result } = renderHook(() =>
      useVoiceRecorder({ interviewId: 1, questionId: 2, onText: vi.fn(), onError }),
    );

    await act(async () => {
      await result.current.start();
    });

    expect(onError).toHaveBeenCalled();
    expect(result.current.state).toBe('idle');
  });
});
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd frontend && npx vitest run src/hooks/useVoiceRecorder.test.ts`
Expected: FAIL —— 模块不存在。

- [ ] **Step 3: 实现 `frontend/src/hooks/useVoiceRecorder.ts`**

```ts
import { useCallback, useEffect, useRef, useState } from 'react';

import { transcribeRecording } from '../api/voice';
import { encodeWavPcm16, mixToMono, resample, TARGET_SAMPLE_RATE } from '../utils/wav';

export type RecorderState = 'idle' | 'recording' | 'transcribing';

interface UseVoiceRecorderOptions {
  interviewId: number;
  questionId: number;
  onText: (text: string) => void;
  onError: (message: string) => void;
}

/** 录音 → 16kHz 单声道 WAV → 上传转写。 */
export function useVoiceRecorder({ interviewId, questionId, onText, onError }: UseVoiceRecorderOptions) {
  const [state, setState] = useState<RecorderState>('idle');
  const [seconds, setSeconds] = useState(0);
  const recorderRef = useRef<MediaRecorder | null>(null);
  const chunksRef = useRef<Blob[]>([]);
  const timerRef = useRef<number | null>(null);

  const stopTimer = useCallback(() => {
    if (timerRef.current !== null) {
      window.clearInterval(timerRef.current);
      timerRef.current = null;
    }
  }, []);

  useEffect(() => stopTimer, [stopTimer]);

  const upload = useCallback(async (chunks: Blob[]) => {
    setState('transcribing');
    let context: AudioContext | null = null;
    try {
      const blob = new Blob(chunks, { type: 'audio/webm' });
      const arrayBuffer = await blob.arrayBuffer();
      context = new AudioContext();
      const decoded = await context.decodeAudioData(arrayBuffer);
      const channels = Array.from({ length: decoded.numberOfChannels }, (_, index) =>
        decoded.getChannelData(index),
      );
      const mono = mixToMono(channels);
      const samples = resample(mono, decoded.sampleRate);
      const wav = new Blob([encodeWavPcm16(samples, TARGET_SAMPLE_RATE)], { type: 'audio/wav' });
      const text = await transcribeRecording(interviewId, questionId, wav);
      if (text.trim()) onText(text);
      else onError('没听清，可重新录音或打字');
    } catch {
      onError('识别失败，可继续打字');
    } finally {
      if (context) void context.close();
      setState('idle');
    }
  }, [interviewId, questionId, onText, onError]);

  const start = useCallback(async () => {
    if (state !== 'idle') return;
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      const recorder = new MediaRecorder(stream);
      chunksRef.current = [];
      recorder.ondataavailable = (event) => {
        if (event.data.size > 0) chunksRef.current.push(event.data);
      };
      recorder.onstop = () => {
        stream.getTracks().forEach((track) => track.stop());
        void upload(chunksRef.current);
      };
      recorderRef.current = recorder;
      recorder.start();
      setState('recording');
      setSeconds(0);
      timerRef.current = window.setInterval(() => setSeconds((value) => value + 1), 1000);
    } catch {
      onError('无法使用麦克风，可继续打字');
      setState('idle');
    }
  }, [state, onError, upload]);

  const stop = useCallback(() => {
    stopTimer();
    const recorder = recorderRef.current;
    if (recorder && recorder.state !== 'inactive') recorder.stop();
  }, [stopTimer]);

  return { state, seconds, start, stop };
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `cd frontend && npx vitest run src/hooks/useVoiceRecorder.test.ts`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add frontend/src/hooks/useVoiceRecorder.ts frontend/src/hooks/useVoiceRecorder.test.ts
git commit -m "feat(voice): 新增录音转写 hook"
```

---

### Task 11: 面试页集成（自动播放 / 喇叭 / 麦克风 / 光标插入）

**Files:**
- Modify: `frontend/src/api/interviews.ts`
- Modify: `frontend/src/components/InterviewChat.tsx`
- Test: `frontend/src/components/InterviewChat.test.tsx`

**Interfaces:**
- Consumes: Task 9 的 `fetchQuestionAudio`、`transcribeRecording`；Task 10 的 `useVoiceRecorder`
- Produces:
  - `InterviewQuestion.audioState?: AudioState | null`
  - `Interview.voice?: { enabled: boolean }`

- [ ] **Step 1: 扩展类型**

在 `frontend/src/api/interviews.ts` 顶部加 import，接口里加字段：

```ts
import type { AudioState } from './voice';

export interface InterviewQuestion {
  // ... 既有字段保持不变 ...
  /** 题目语音状态；缺省表示后端未启用语音。 */
  audioState?: AudioState | null;
}

export interface Interview {
  // ... 既有字段保持不变 ...
  /** 语音能力开关；缺省表示后端未启用语音。 */
  voice?: { enabled: boolean };
}
```

- [ ] **Step 2: 写组件测试**

在 `frontend/src/components/InterviewChat.test.tsx` 追加：

```tsx
const fetchQuestionAudio = vi.fn();

vi.mock('../api/voice', () => ({
  fetchQuestionAudio: (...args: unknown[]) => fetchQuestionAudio(...args),
  transcribeRecording: vi.fn(),
}));

it('hides voice controls when voice is disabled', async () => {
  getInterviewQuestions.mockResolvedValue([makeQuestion({ id: 1 })]);
  render(
    <InterviewChat
      interview={{ ...interview, voice: { enabled: false } }}
      onFinished={vi.fn()}
      onExit={vi.fn()}
    />,
  );

  expect(await screen.findByText('Go 的 channel 底层数据结构是怎样的？')).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: /朗读题目/ })).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: /录音回答/ })).not.toBeInTheDocument();
});

it('shows a play button when the question audio is ready', async () => {
  getInterviewQuestions.mockResolvedValue([makeQuestion({ id: 1, audioState: 'ready' })]);
  render(
    <InterviewChat
      interview={{ ...interview, voice: { enabled: true } }}
      onFinished={vi.fn()}
      onExit={vi.fn()}
    />,
  );

  expect(await screen.findByRole('button', { name: /朗读题目/ })).toBeInTheDocument();
});

it('shows a generating hint while the audio is pending', async () => {
  getInterviewQuestions.mockResolvedValue([makeQuestion({ id: 1, audioState: 'pending' })]);
  render(
    <InterviewChat
      interview={{ ...interview, voice: { enabled: true } }}
      onFinished={vi.fn()}
      onExit={vi.fn()}
    />,
  );

  expect(await screen.findByText('语音生成中…')).toBeInTheDocument();
});
```

- [ ] **Step 3: 运行测试确认失败**

Run: `cd frontend && npx vitest run src/components/InterviewChat.test.tsx`
Expected: FAIL —— 按钮与提示尚不存在。

- [ ] **Step 4: 实现自动播放 hook**

在 `frontend/src/components/InterviewChat.tsx` 追加：

```tsx
import { fetchQuestionAudio } from '../api/voice';
import { useVoiceRecorder } from '../hooks/useVoiceRecorder';

/** 当前题的语音自动播放：就绪即播，未就绪则轮询等待。 */
const AUDIO_POLL_INTERVAL_MS = 1500;
const AUDIO_POLL_MAX_ATTEMPTS = 40;

function useQuestionAudio(interviewId: number, questionId: number | null, enabled: boolean) {
  const [status, setStatus] = useState<'idle' | 'pending' | 'failed'>('idle');

  useEffect(() => {
    if (!enabled || questionId === null) return;
    let cancelled = false;
    let timer: number | undefined;
    let attempts = 0;

    const play = async () => {
      try {
        const blob = await fetchQuestionAudio(interviewId, questionId);
        if (cancelled) return;
        const url = URL.createObjectURL(blob);
        const audio = new Audio(url);
        try {
          await audio.play();
        } catch {
          // 浏览器拦截自动播放：提示用户点喇叭。
        }
        URL.revokeObjectURL(url);
        setStatus('idle');
      } catch (error) {
        const code = (error as { status?: number }).status;
        if (code === 409 && attempts < AUDIO_POLL_MAX_ATTEMPTS) {
          attempts += 1;
          setStatus('pending');
          timer = window.setTimeout(() => void play(), AUDIO_POLL_INTERVAL_MS);
          return;
        }
        setStatus(code === 503 ? 'failed' : 'idle');
      }
    };

    void play();
    return () => {
      cancelled = true;
      if (timer) window.clearTimeout(timer);
    };
  }, [interviewId, questionId, enabled]);

  return { status };
}
```

- [ ] **Step 5: 接入组件**

在 `InterviewChat` 组件内（`textareaRef` 已有）追加：

```tsx
  const voiceEnabled = interview.voice?.enabled ?? false;
  const currentQuestionId = current?.id ?? null;
  const { status: audioStatus } = useQuestionAudio(interview.id, currentQuestionId, voiceEnabled);
  const [voiceError, setVoiceError] = useState('');

  // 识别文本插入到当前光标位置（不是追加到末尾）。
  const insertAtCursor = useCallback((text: string) => {
    const area = textareaRef.current;
    if (!area) {
      setAnswer((previous) => previous + text);
      return;
    }
    const start = area.selectionStart ?? area.value.length;
    const end = area.selectionEnd ?? start;
    const next = area.value.slice(0, start) + text + area.value.slice(end);
    setAnswer(next);
    window.requestAnimationFrame(() => {
      area.focus();
      const caret = start + text.length;
      area.setSelectionRange(caret, caret);
    });
  }, []);

  const recorder = useVoiceRecorder({
    interviewId: interview.id,
    questionId: currentQuestionId ?? 0,
    onText: insertAtCursor,
    onError: setVoiceError,
  });

  const speakQuestion = useCallback(async (questionId: number) => {
    setVoiceError('');
    try {
      const blob = await fetchQuestionAudio(interview.id, questionId);
      const url = URL.createObjectURL(blob);
      const audio = new Audio(url);
      await audio.play();
      URL.revokeObjectURL(url);
    } catch {
      setVoiceError('语音暂不可用，可继续打字');
    }
  }, [interview.id]);
```

在题面气泡里（`renderTurn` 内 `<p className="interview-msg__question">` 之后）渲染喇叭：

```tsx
        {voiceEnabled ? (
          question.audioState === 'ready' ? (
            <button
              aria-label={`朗读题目 ${question.id}`}
              className="interview-msg__speak"
              onClick={() => void speakQuestion(question.id)}
              title="朗读题目"
              type="button"
            >
              🔊
            </button>
          ) : question.audioState === 'pending' ? (
            <span className="interview-msg__audiopending">语音生成中…</span>
          ) : null)
        ) : null}
```

在输入坞的 `.interview-dock__actions` 前加麦克风按钮：

```tsx
            {voiceEnabled ? (
              <button
                aria-label="录音回答"
                className={`interview-dock__mic interview-dock__mic--${recorder.state}`}
                disabled={recorder.state === 'transcribing' || !current}
                onClick={() => {
                  if (recorder.state === 'recording') recorder.stop();
                  else void recorder.start();
                }}
                type="button"
              >
                {recorder.state === 'recording'
                  ? `停止 ${recorder.seconds}s`
                  : recorder.state === 'transcribing'
                    ? '识别中…'
                    : '🎤'}
              </button>
            ) : null}
```

并在错误区加语音提示（不阻断提交）：

```tsx
      {voiceError && <p className="interview-chat__voiceerror" role="status">{voiceError}</p>}
```

> `audioStatus` 目前只用于内部状态；如需在 UI 上提示「生成中」可复用它。样式类名（`interview-msg__speak` / `interview-dock__mic` / `interview-chat__voiceerror`）按仓库既有 CSS 约定补到对应样式文件。

- [ ] **Step 6: 运行组件测试确认通过**

Run: `cd frontend && npx vitest run src/components/InterviewChat.test.tsx`
Expected: PASS。

- [ ] **Step 7: 全量前端测试**

Run: `cd frontend && npx vitest run`
Expected: 全部通过。

- [ ] **Step 8: Commit**

```bash
git add frontend/src/api/interviews.ts frontend/src/components/InterviewChat.tsx frontend/src/components/InterviewChat.test.tsx
git commit -m "feat(voice): 面试页接入题目自动播放与录音转写"
```

---

### Task 12: 端到端联调与运维文档

**Files:**
- Create: `tools/voice-server/README.md`
- Modify: `docs/plans/2026-11-05-voice-interview-design.md`（末尾补「实现说明」一节）

**Interfaces:**
- Produces: 启动语音推理服务的操作手册与端到端验证清单

- [ ] **Step 1: 写 `tools/voice-server/README.md`**

```markdown
# 语音推理服务（mlx-audio）

本机运行 TTS / ASR 的 OpenAI 兼容服务。

## 启动

```bash
pip install mlx-audio
mlx_audio.server --host 127.0.0.1 --port 21100
```

端口 21100 避开常用段。服务未启动时后端不报错：
题目语音标记为 failed，前端不显示喇叭，文字面试照常。

## 模型

首次请求会自动下载（合计约 3.5GB）：

- `mlx-community/Qwen3-TTS-12Hz-0.6B-CustomVoice-8bit`（TTS）
- `mlx-community/Qwen3-ASR-1.7B-8bit`（ASR）

## 后端环境变量

```
VOICE_ENABLED=true
VOICE_TTS_BASE_URL=http://127.0.0.1:21100/v1
VOICE_ASR_BASE_URL=http://127.0.0.1:21100/v1
VOICE_CACHE_DIR=./uploads/voice
```

## 验证

```bash
curl -s http://127.0.0.1:21100/v1/audio/speech \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  -d 'model=mlx-community/Qwen3-TTS-12Hz-0.6B-CustomVoice-8bit&input=你好&response_format=wav' \
  -o /tmp/tts.wav && file /tmp/tts.wav
```
```

- [ ] **Step 2: 端到端手工验证清单**

按顺序验证，每一步都要实际跑过：

1. `cd backend && go test ./...` 全绿。
2. `cd frontend && npx vitest run` 全绿。
3. 启动 `mlx_audio.server`，设置 `VOICE_ENABLED=true` 后启动后端。
4. 创建一个面试：
   - `POST /api/interviews` 返回 201；
   - 数秒后 `GET /api/interviews/:id/questions` 的 `audioState` 依次变成 `ready`；
   - `GET /api/interviews/:id/questions/:qid/audio` 返回 200 且 `Content-Type: audio/wav`。
5. 进入面试页：当前题自动朗读（浏览器拦截时点喇叭可播）。
6. 录音回答：停止后输入框光标处插入识别文本，且错别字被修正。
7. 停掉 `mlx_audio.server` 再答题：`answer` 仍返回 200，文字题面与评分正常。
8. 重启后端：进行中的面试重新进入，题目语音仍可播放，未生成的题目被补齐。

- [ ] **Step 3: Commit**

```bash
git add tools/voice-server/README.md docs/plans/2026-11-05-voice-interview-design.md
git commit -m "docs(voice): 补充语音服务启动与端到端验证清单"
```

---

## Self-Review

**1. Spec 覆盖核对**

| 设计文档要求 | 覆盖任务 |
| --- | --- |
| 创建面试前预检 active（放在解析简历/JD 之前） | Task 7 Step 6 |
| 题目语音只在后台生成、按顺序串行 | Task 4 |
| 生成失败重试 | Task 4（退避 + MaxAttempts，耗尽标 failed） |
| 服务器重启 reload 补齐未生成语音 | Task 4（Reload）+ Task 8（启动时调用） |
| 播放只读文件、不在请求内同步合成 | Task 7 Step 4 |
| 进入页面获取当前题后自动播放 | Task 11 |
| 录音转写 | Task 5 + Task 7 + Task 9 + Task 10 |
| AI 修错别字、思考强度最低 | Task 5 + Task 8（`ReasoningEffort=low`） |
| 识别文本插入光标位置 | Task 11 |
| 不做录音回放 | 设计文档非目标；Task 7 无 recording 接口 |
| 语音不可用不影响面试 | Task 6（answer 不同步等语音）+ Task 7（503 降级） |
| 面试结束清理音频 | Task 6（Finished / Abort） |
| `audio_state` 四态 | Task 1 + Task 4 |
| 接口文档 | 设计文档「接口文档」章 |

**2. 占位符扫描**：无 TBD / TODO；所有代码块都是可直接落地的完整实现。

**3. 类型一致性核对**

- `interview.AudioHooks.Enqueue(ctx, tenantID, interviewID, questionID int64, text string)` —— Task 1 定义、Task 4 实现、Task 6 调用，三处签名一致。
- `speech.Generator` 实现 `interview.AudioHooks`（Task 4 末尾有编译期断言 `var _ interview.AudioHooks = (*Generator)(nil)`）。
- `speech.AudioStateStore.ActiveQuestionsNeedingAudio` 返回 `[]interview.PendingAudio`，与 Task 1 仓储方法的返回类型一致。
- `speech.Corrector.Correct(ctx, text) string` —— Task 5 定义、Task 7 调用。
- `interview.Service.QuestionForAudio(ctx, tenantID, interviewID, questionID)` —— Task 6 定义、Task 7 的 `h.service.QuestionForAudio` 调用，一致。
- `speech.AudioStore.Root()` —— Task 3 定义、Task 8 调用，一致。

## 执行说明

**依赖关系**：Task 1 → 2 → 3 → 4 → 5 → 6 → 7 → 8 → 9 → 10 → 11 → 12。

Task 1（仓储）与 Task 2（协议层）互不依赖，可并行。Task 9–11（前端）除依赖 Task 9 外与后端任务无耦合，后端做完 Task 8 即可并行推进前端。

