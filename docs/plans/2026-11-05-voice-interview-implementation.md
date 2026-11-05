# AI 模拟面试语音能力 — 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 AI 模拟面试加上「题目朗读（TTS）」与「录音转写（ASR）」，Go 后端通过 OpenAI 兼容音频协议对接可独立配置的 TTS / ASR 服务，音频落盘并在面试结束时清理。

**Architecture:** 新增 `backend/internal/speech` 包，对外暴露 `Synthesizer` / `Transcriber` 两个接口，唯一实现 `OpenAICompatibleProvider` 吃 OpenAI 音频协议。TTS 与 ASR 各自独立配置 base URL / api key / model，可指向不同服务。音频按 `{tenantID}/{interviewID}/` 分目录落盘，存在性写入 `interview_questions.audio_state` 列（唯一 schema 改动），面试状态变终态时删目录。前端用 MediaRecorder + Web Audio API 转 16k 单声道 WAV，后端零转码。

**Tech Stack:** Go 1.26 + Gin + sqlx + logrus；React 18 + TypeScript + Vite + vitest/testing-library；`golang.org/x/sync/singleflight`（已在 go.sum，需提升为直接依赖）。

## Global Constraints

- 设计文档：`docs/plans/2026-11-05-voice-interview-design.md`（已提交 `96aae7f`）。本计划的实现必须与该文档一致；有冲突以设计文档为准。
- 后端提交门禁（来自 `AGSIN.md`）：改 `backend/**` 后提交前必须 `cd backend && go test ./...` 通过。新增/修改 API 必须补路由级黑盒测试（成功路径、未认证 401、跨租户 404、非法输入 400、依赖不可用 503）。
- 提交信息用中文 `type(scope): 说明`（如 `feat(speech): 新增 OpenAI 兼容语音 provider`）。
- 前端注释与叙述用中文；代码标识符、库名保持英文。
- **后端不做任何音频转码**，只校验与字节透传。转码在浏览器完成。
- **不把转写文本落库**。唯一的 schema 改动是 `interview_questions.audio_state` 一列（可空）。
- 环境变量名固定为：`VOICE_ENABLED`、`VOICE_TTS_BASE_URL`、`VOICE_TTS_API_KEY`、`VOICE_TTS_MODEL`、`VOICE_TTS_VOICE`、`VOICE_ASR_BASE_URL`、`VOICE_ASR_API_KEY`、`VOICE_ASR_MODEL`、`VOICE_TIMEOUT`、`VOICE_CACHE_DIR`。**不得引入 `VOICE_BASE_URL` 这类合并变量。**
- 端口：本地推理服务用 **21100**（避开常用段）。
- 模型 ID：ASR `mlx-community/Qwen3-ASR-1.7B-8bit`，TTS `mlx-community/Qwen3-TTS-12Hz-0.6B-CustomVoice-8bit`。
- 上传限制：录音 WAV 最大 20MB、最长 10 分钟。
- 音频文件目录：`{VOICE_CACHE_DIR}/{tenantID}/{interviewID}/q{questionID}.wav`（题目语音）与 `r{questionID}.wav`（用户录音）。
- 三个已核实的上游行为必须处理（详见设计文档「三个必须处理的实现坑」）：请求固定 `response_format=wav` 且不信任上游 MIME；转写必须显式传 `response_format=json` 并对 ndjson 兜底；预合成逐题串行提交，前台请求最多等一题。
- 不引入新的前端运行时依赖（录音用浏览器原生 `MediaRecorder` / `AudioContext`）。
- **执行本计划时，先读相关文件的现有内容再改**，不要凭本计划臆造辅助函数名或字段名。计划中标注「先读…确认」的位置必须先读。

---

### Task 1: speech 包的类型与接口 + 配置加载

**Files:**
- Create: `backend/internal/speech/speech.go`
- Create: `backend/internal/speech/config.go`
- Test: `backend/internal/speech/config_test.go`

**Interfaces:**
- Consumes: 无（首个任务）
- Produces:
  - `type SynthesisRequest struct { Text string; Voice string; Language string }`
  - `type SynthesisResult struct { Audio []byte; ContentType string }`
  - `type TranscriptionRequest struct { Audio []byte; ContentType string; Language string }`
  - `type TranscriptionResult struct { Text string }`
  - `type Synthesizer interface { Synthesize(ctx context.Context, req SynthesisRequest) (SynthesisResult, error) }`
  - `type Transcriber interface { Transcribe(ctx context.Context, req TranscriptionRequest) (TranscriptionResult, error) }`
  - `type TTSConfig struct { BaseURL string; APIKey string; Model string; Voice string; Timeout time.Duration }`
  - `type ASRConfig struct { BaseURL string; APIKey string; Model string; Timeout time.Duration }`
  - `type Config struct { Enabled bool; TTS TTSConfig; ASR ASRConfig; CacheDir string }`
  - `func LoadConfigFromEnv() Config`

- [ ] **Step 1: 写失败的配置测试**

创建 `backend/internal/speech/config_test.go`：

```go
package speech

import (
	"testing"
	"time"
)

func TestLoadConfigFromEnvReadsIndependentTTSAndASRSettings(t *testing.T) {
	t.Setenv("VOICE_ENABLED", "true")
	t.Setenv("VOICE_TTS_BASE_URL", "http://127.0.0.1:21100/v1")
	t.Setenv("VOICE_TTS_API_KEY", "tts-key")
	t.Setenv("VOICE_TTS_MODEL", "tts-model")
	t.Setenv("VOICE_TTS_VOICE", "Vivian")
	t.Setenv("VOICE_ASR_BASE_URL", "https://example.invalid/v1")
	t.Setenv("VOICE_ASR_API_KEY", "asr-key")
	t.Setenv("VOICE_ASR_MODEL", "asr-model")
	t.Setenv("VOICE_TIMEOUT", "90s")
	t.Setenv("VOICE_CACHE_DIR", "/tmp/voice-cache")

	cfg := LoadConfigFromEnv()

	if !cfg.Enabled {
		t.Fatal("Enabled = false, want true")
	}
	if cfg.TTS.BaseURL != "http://127.0.0.1:21100/v1" {
		t.Errorf("TTS.BaseURL = %q", cfg.TTS.BaseURL)
	}
	if cfg.TTS.APIKey != "tts-key" {
		t.Errorf("TTS.APIKey = %q", cfg.TTS.APIKey)
	}
	if cfg.TTS.Voice != "Vivian" {
		t.Errorf("TTS.Voice = %q", cfg.TTS.Voice)
	}
	if cfg.ASR.BaseURL != "https://example.invalid/v1" {
		t.Errorf("ASR.BaseURL = %q", cfg.ASR.BaseURL)
	}
	if cfg.ASR.APIKey != "asr-key" {
		t.Errorf("ASR.APIKey = %q", cfg.ASR.APIKey)
	}
	if cfg.TTS.Timeout != 90*time.Second {
		t.Errorf("TTS.Timeout = %v", cfg.TTS.Timeout)
	}
	if cfg.ASR.Timeout != 90*time.Second {
		t.Errorf("ASR.Timeout = %v", cfg.ASR.Timeout)
	}
	if cfg.CacheDir != "/tmp/voice-cache" {
		t.Errorf("CacheDir = %q", cfg.CacheDir)
	}
}

func TestLoadConfigFromEnvDefaultsAndDisabled(t *testing.T) {
	t.Setenv("VOICE_ENABLED", "false")
	t.Setenv("VOICE_TTS_BASE_URL", "")
	t.Setenv("VOICE_ASR_BASE_URL", "")
	t.Setenv("VOICE_TIMEOUT", "")
	t.Setenv("VOICE_CACHE_DIR", "")

	cfg := LoadConfigFromEnv()

	if cfg.Enabled {
		t.Fatal("Enabled = true, want false")
	}
	if cfg.TTS.BaseURL != "http://127.0.0.1:21100/v1" {
		t.Errorf("TTS.BaseURL default = %q", cfg.TTS.BaseURL)
	}
	if cfg.ASR.BaseURL != "http://127.0.0.1:21100/v1" {
		t.Errorf("ASR.BaseURL default = %q", cfg.ASR.BaseURL)
	}
	if cfg.TTS.Timeout != 120*time.Second {
		t.Errorf("TTS.Timeout default = %v", cfg.TTS.Timeout)
	}
	if cfg.CacheDir != "./uploads/voice" {
		t.Errorf("CacheDir default = %q", cfg.CacheDir)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd backend && go test ./internal/speech/... -run TestLoadConfigFromEnv -v`
Expected: FAIL —— `undefined: LoadConfigFromEnv`（包目录尚不存在则报 `no Go files`）

- [ ] **Step 3: 实现 speech.go**

创建 `backend/internal/speech/speech.go`：

```go
// Package speech 提供面试语音能力（题目朗读与录音转写）的抽象层。
//
// 对外只暴露 Synthesizer 与 Transcriber 两个接口；实现走 OpenAI 音频协议
// （/v1/audio/speech 与 /v1/audio/transcriptions），因此本地推理服务与云端 API
// 是同一份代码，切换只改环境变量。
//
// TTS 与 ASR 的配置完全独立（各自的 BaseURL / APIKey / Model），可分别指向
// 不同服务实例。
package speech

import (
	"context"
)

// SynthesisRequest 是一次文本转语音的入参。
type SynthesisRequest struct {
	// Text 待朗读的文本。
	Text string
	// Voice 预置音色名；为空时由 provider 使用其默认值。
	Voice string
	// Language 语言代码；为空时由 provider 使用其默认值。
	Language string
}

// SynthesisResult 是一次合成的产物。
type SynthesisResult struct {
	// Audio 音频字节。
	Audio []byte
	// ContentType 音频 MIME 类型。
	ContentType string
}

// TranscriptionRequest 是一次音频转写的入参。
type TranscriptionRequest struct {
	// Audio 音频字节。
	Audio []byte
	// ContentType 音频 MIME 类型。
	ContentType string
	// Language 语言提示；为空时交给模型自动识别。
	Language string
}

// TranscriptionResult 是一次转写的产物。
type TranscriptionResult struct {
	// Text 识别出的文本。
	Text string
}

// Synthesizer 把文本合成为语音。
type Synthesizer interface {
	Synthesize(ctx context.Context, req SynthesisRequest) (SynthesisResult, error)
}

// Transcriber 把语音转写为文本。
type Transcriber interface {
	Transcribe(ctx context.Context, req TranscriptionRequest) (TranscriptionResult, error)
}
```

- [ ] **Step 4: 实现 config.go**

创建 `backend/internal/speech/config.go`：

```go
package speech

import (
	"os"
	"strings"
	"time"
)

const (
	defaultBaseURL  = "http://127.0.0.1:21100/v1"
	defaultTimeout  = 120 * time.Second
	defaultCacheDir = "./uploads/voice"
	defaultTTSModel = "mlx-community/Qwen3-TTS-12Hz-0.6B-CustomVoice-8bit"
	defaultTTSVoice = "Vivian"
	defaultASRModel = "mlx-community/Qwen3-ASR-1.7B-8bit"
)

// TTSConfig 是题目朗读侧的配置。
type TTSConfig struct {
	// BaseURL 是 OpenAI 兼容服务的根地址（到 /v1 为止）。
	BaseURL string
	// APIKey 鉴权 key；本地服务留空。
	APIKey string
	// Model 模型标识。
	Model string
	// Voice 预置音色名。
	Voice string
	// Timeout 单次请求超时。
	Timeout time.Duration
}

// ASRConfig 是录音转写侧的配置。
type ASRConfig struct {
	// BaseURL 是 OpenAI 兼容服务的根地址（到 /v1 为止）。
	BaseURL string
	// APIKey 鉴权 key；本地服务留空。
	APIKey string
	// Model 模型标识。
	Model string
	// Timeout 单次请求超时。
	Timeout time.Duration
}

// Config 是语音能力的整体配置。
type Config struct {
	// Enabled 为 false 时不创建 provider，接口统一返回 503。
	Enabled bool
	// TTS 题目朗读配置。
	TTS TTSConfig
	// ASR 录音转写配置。
	ASR ASRConfig
	// CacheDir 是音频落盘根目录。
	CacheDir string
}

// LoadConfigFromEnv 从环境变量读取语音配置。
//
// TTS 与 ASR 各自独立读取 base URL / api key / model，不共用变量，
// 因此任一侧切换到云端不会牵动另一侧。
func LoadConfigFromEnv() Config {
	timeout := defaultTimeout
	if raw := strings.TrimSpace(os.Getenv("VOICE_TIMEOUT")); raw != "" {
		if parsed, err := time.ParseDuration(raw); err == nil && parsed > 0 {
			timeout = parsed
		}
	}
	cacheDir := strings.TrimSpace(os.Getenv("VOICE_CACHE_DIR"))
	if cacheDir == "" {
		cacheDir = defaultCacheDir
	}
	return Config{
		Enabled: strings.EqualFold(strings.TrimSpace(os.Getenv("VOICE_ENABLED")), "true"),
		TTS: TTSConfig{
			BaseURL: envOrDefault("VOICE_TTS_BASE_URL", defaultBaseURL),
			APIKey:  strings.TrimSpace(os.Getenv("VOICE_TTS_API_KEY")),
			Model:   envOrDefault("VOICE_TTS_MODEL", defaultTTSModel),
			Voice:   envOrDefault("VOICE_TTS_VOICE", defaultTTSVoice),
			Timeout: timeout,
		},
		ASR: ASRConfig{
			BaseURL: envOrDefault("VOICE_ASR_BASE_URL", defaultBaseURL),
			APIKey:  strings.TrimSpace(os.Getenv("VOICE_ASR_API_KEY")),
			Model:   envOrDefault("VOICE_ASR_MODEL", defaultASRModel),
			Timeout: timeout,
		},
		CacheDir: cacheDir,
	}
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
```

- [ ] **Step 5: 运行测试确认通过**

Run: `cd backend && go test ./internal/speech/... -v`
Expected: PASS —— 两个配置测试均通过

- [ ] **Step 6: 提交**

```bash
cd backend && gofmt -w internal/speech/ && go test ./internal/speech/... && cd ..
git add backend/internal/speech/
git commit -m "feat(speech): 定义语音抽象接口与 TTS/ASR 独立配置"
```

### Task 2: OpenAI 兼容 provider（TTS + ASR）

**Files:**
- Create: `backend/internal/speech/openai.go`
- Test: `backend/internal/speech/openai_test.go`

**Interfaces:**
- Consumes: Task 1 的 `SynthesisRequest` / `TranscriptionRequest` / `TTSConfig` / `ASRConfig`
- Produces:
  - `type OpenAICompatibleProvider struct { ... }`
  - `func NewOpenAICompatibleProvider(tts TTSConfig, asr ASRConfig) *OpenAICompatibleProvider`
  - `func (p *OpenAICompatibleProvider) Synthesize(ctx context.Context, req SynthesisRequest) (SynthesisResult, error)`
  - `func (p *OpenAICompatibleProvider) Transcribe(ctx context.Context, req TranscriptionRequest) (TranscriptionResult, error)`
  - 构造器内部自建 `*http.Client`（用 Config 里的 Timeout）；测试用 `httptest.NewServer` 覆盖 base URL 即可，无需注入 client。

- [ ] **Step 1: 写失败的 provider 测试**

创建 `backend/internal/speech/openai_test.go`：

```go
package speech

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSynthesizeForcesWavAndIgnoresUpstreamContentType(t *testing.T) {
	var received struct {
		model  string
		input  string
		voice  string
		format string
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audio/speech" {
			t.Errorf("path = %q", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &received)
		// 上游谎报 mp3，provider 必须不采信。
		w.Header().Set("Content-Type", "audio/mp3")
		_, _ = w.Write([]byte("RIFFfakewavbytes"))
	}))
	defer server.Close()

	provider := NewOpenAICompatibleProvider(
		TTSConfig{BaseURL: server.URL + "/v1", Model: "tts-model", Voice: "Vivian"},
		ASRConfig{BaseURL: server.URL + "/v1"},
	)

	result, err := provider.Synthesize(context.Background(), SynthesisRequest{Text: "你好", Voice: "Vivian"})
	if err != nil {
		t.Fatalf("Synthesize error = %v", err)
	}
	if result.ContentType != "audio/wav" {
		t.Errorf("ContentType = %q, want audio/wav", result.ContentType)
	}
	if string(result.Audio) != "RIFFfakewavbytes" {
		t.Errorf("Audio = %q", string(result.Audio))
	}
	if received.format != "wav" {
		t.Errorf("response_format = %q, want wav", received.format)
	}
	if received.model != "tts-model" {
		t.Errorf("model = %q", received.model)
	}
	if received.input != "你好" {
		t.Errorf("input = %q", received.input)
	}
	if received.voice != "Vivian" {
		t.Errorf("voice = %q", received.voice)
	}
}

func TestTranscribeSendsResponseFormatJSONAndParsesText(t *testing.T) {
	var gotFormat, gotModel string
	var gotFileBytes []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audio/transcriptions" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("ParseMultipartForm error = %v", err)
		}
		gotFormat = r.FormValue("response_format")
		gotModel = r.FormValue("model")
		file, _, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("FormFile error = %v", err)
		}
		defer file.Close()
		gotFileBytes, _ = io.ReadAll(file)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":"识别出的文本"}`))
	}))
	defer server.Close()

	provider := NewOpenAICompatibleProvider(
		TTSConfig{BaseURL: server.URL + "/v1"},
		ASRConfig{BaseURL: server.URL + "/v1", Model: "asr-model"},
	)

	result, err := provider.Transcribe(context.Background(), TranscriptionRequest{
		Audio:       []byte("RIFFfakewavbytes"),
		ContentType: "audio/wav",
	})
	if err != nil {
		t.Fatalf("Transcribe error = %v", err)
	}
	if result.Text != "识别出的文本" {
		t.Errorf("Text = %q", result.Text)
	}
	if gotFormat != "json" {
		t.Errorf("response_format = %q, want json", gotFormat)
	}
	if gotModel != "asr-model" {
		t.Errorf("model = %q", gotModel)
	}
	if string(gotFileBytes) != "RIFFfakewavbytes" {
		t.Errorf("uploaded bytes = %q", string(gotFileBytes))
	}
}

func TestTranscribeFallsBackToNDJSONFirstLine(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = w.Write([]byte("{\"text\":\"第一行\"}\n{\"text\":\"第二行\"}\n"))
	}))
	defer server.Close()

	provider := NewOpenAICompatibleProvider(
		TTSConfig{BaseURL: server.URL + "/v1"},
		ASRConfig{BaseURL: server.URL + "/v1"},
	)

	result, err := provider.Transcribe(context.Background(), TranscriptionRequest{Audio: []byte("x")})
	if err != nil {
		t.Fatalf("Transcribe error = %v", err)
	}
	if result.Text != "第一行" {
		t.Errorf("Text = %q, want 第一行", result.Text)
	}
}

func TestTTSAndASRUseIndependentBaseURLs(t *testing.T) {
	var ttsHit, asrHit bool
	tts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ttsHit = true
		_, _ = w.Write([]byte("wav"))
	}))
	defer tts.Close()
	asr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asrHit = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":"ok"}`))
	}))
	defer asr.Close()

	provider := NewOpenAICompatibleProvider(
		TTSConfig{BaseURL: tts.URL + "/v1"},
		ASRConfig{BaseURL: asr.URL + "/v1"},
	)

	if _, err := provider.Synthesize(context.Background(), SynthesisRequest{Text: "x"}); err != nil {
		t.Fatalf("Synthesize error = %v", err)
	}
	if _, err := provider.Transcribe(context.Background(), TranscriptionRequest{Audio: []byte("x")}); err != nil {
		t.Fatalf("Transcribe error = %v", err)
	}
	if !ttsHit || !asrHit {
		t.Errorf("ttsHit = %v, asrHit = %v", ttsHit, asrHit)
	}
}

func TestTranscribeRejectsEmptyAudio(t *testing.T) {
	provider := NewOpenAICompatibleProvider(
		TTSConfig{BaseURL: "http://127.0.0.1:1/v1"},
		ASRConfig{BaseURL: "http://127.0.0.1:1/v1"},
	)
	if _, err := provider.Transcribe(context.Background(), TranscriptionRequest{}); err == nil {
		t.Fatal("Transcribe error = nil, want error for empty audio")
	}
}

func TestSynthesizeRejectsEmptyText(t *testing.T) {
	provider := NewOpenAICompatibleProvider(
		TTSConfig{BaseURL: "http://127.0.0.1:1/v1"},
		ASRConfig{BaseURL: "http://127.0.0.1:1/v1"},
	)
	if _, err := provider.Synthesize(context.Background(), SynthesisRequest{}); err == nil {
		t.Fatal("Synthesize error = nil, want error for empty text")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd backend && go test ./internal/speech/... -run 'TestSynthesize|TestTranscribe|TestTTSAndASR' -v`
Expected: FAIL —— `undefined: NewOpenAICompatibleProvider`

- [ ] **Step 3: 实现 openai.go**

创建 `backend/internal/speech/openai.go`：

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
	"strings"
)

// wavContentType 是本应用唯一信任的音频 MIME。
//
// 上游 mlx-audio 的 /v1/audio/speech 用 media_type=f"audio/{response_format}"
// 拼 Content-Type，但内部固定产出 WAV 字节；传 mp3 会得到标着 audio/mp3 的
// WAV 文件。因此 provider 一律按 wav 声明，不采信上游返回的 Content-Type。
const wavContentType = "audio/wav"

// OpenAICompatibleProvider 通过 OpenAI 音频协议对接 TTS 与 ASR。
//
// TTS 与 ASR 各自持有独立的 base URL / api key / model / http.Client，
// 可分别指向不同的服务实例。
type OpenAICompatibleProvider struct {
	ttsConfig TTSConfig
	asrConfig ASRConfig
	ttsClient *http.Client
	asrClient *http.Client
}

// NewOpenAICompatibleProvider 创建 provider。
func NewOpenAICompatibleProvider(tts TTSConfig, asr ASRConfig) *OpenAICompatibleProvider {
	return &OpenAICompatibleProvider{
		ttsConfig: tts,
		asrConfig: asr,
		ttsClient: &http.Client{Timeout: tts.Timeout},
		asrClient: &http.Client{Timeout: asr.Timeout},
	}
}

// Synthesize 把文本合成为语音。
func (p *OpenAICompatibleProvider) Synthesize(ctx context.Context, req SynthesisRequest) (SynthesisResult, error) {
	if strings.TrimSpace(req.Text) == "" {
		return SynthesisResult{}, errors.New("speech: synthesize text is required")
	}
	body := map[string]any{
		"model":           p.ttsConfig.Model,
		"input":           req.Text,
		"response_format": "wav",
	}
	voice := req.Voice
	if voice == "" {
		voice = p.ttsConfig.Voice
	}
	if voice != "" {
		body["voice"] = voice
	}
	if req.Language != "" {
		body["language"] = req.Language
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return SynthesisResult{}, fmt.Errorf("speech: encode synthesize request: %w", err)
	}
	endpoint := strings.TrimRight(p.ttsConfig.BaseURL, "/") + "/audio/speech"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return SynthesisResult{}, fmt.Errorf("speech: build synthesize request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.ttsConfig.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.ttsConfig.APIKey)
	}
	resp, err := p.ttsClient.Do(httpReq)
	if err != nil {
		return SynthesisResult{}, fmt.Errorf("speech: synthesize request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return SynthesisResult{}, fmt.Errorf("speech: synthesize status %d: %s", resp.StatusCode, strings.TrimSpace(string(detail)))
	}
	audio, err := io.ReadAll(resp.Body)
	if err != nil {
		return SynthesisResult{}, fmt.Errorf("speech: read synthesize response: %w", err)
	}
	if len(audio) == 0 {
		return SynthesisResult{}, errors.New("speech: synthesize returned empty audio")
	}
	return SynthesisResult{Audio: audio, ContentType: wavContentType}, nil
}

// Transcribe 把音频转写为文本。
func (p *OpenAICompatibleProvider) Transcribe(ctx context.Context, req TranscriptionRequest) (TranscriptionResult, error) {
	if len(req.Audio) == 0 {
		return TranscriptionResult{}, errors.New("speech: transcribe audio is required")
	}
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	part, err := writer.CreateFormFile("file", "recording.wav")
	if err != nil {
		return TranscriptionResult{}, fmt.Errorf("speech: build transcribe form: %w", err)
	}
	if _, err := part.Write(req.Audio); err != nil {
		return TranscriptionResult{}, fmt.Errorf("speech: write transcribe form: %w", err)
	}
	fields := map[string]string{
		"model":           p.asrConfig.Model,
		"response_format": "json",
	}
	if req.Language != "" {
		fields["language"] = req.Language
	}
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			return TranscriptionResult{}, fmt.Errorf("speech: write transcribe field %s: %w", key, err)
		}
	}
	if err := writer.Close(); err != nil {
		return TranscriptionResult{}, fmt.Errorf("speech: close transcribe form: %w", err)
	}
	endpoint := strings.TrimRight(p.asrConfig.BaseURL, "/") + "/audio/transcriptions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &buffer)
	if err != nil {
		return TranscriptionResult{}, fmt.Errorf("speech: build transcribe request: %w", err)
	}
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())
	if p.asrConfig.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.asrConfig.APIKey)
	}
	resp, err := p.asrClient.Do(httpReq)
	if err != nil {
		return TranscriptionResult{}, fmt.Errorf("speech: transcribe request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return TranscriptionResult{}, fmt.Errorf("speech: transcribe status %d: %s", resp.StatusCode, strings.TrimSpace(string(detail)))
	}
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return TranscriptionResult{}, fmt.Errorf("speech: read transcribe response: %w", err)
	}
	return parseTranscriptionPayload(payload)
}

// parseTranscriptionPayload 解析转写响应。
//
// 上游 /v1/audio/transcriptions 的 response_format 默认是 ndjson；
// provider 已显式请求 json，但对 ndjson 做兜底：取第一行 JSON 的 text 字段。
func parseTranscriptionPayload(payload []byte) (TranscriptionResult, error) {
	trimmed := strings.TrimSpace(string(payload))
	if trimmed == "" {
		return TranscriptionResult{}, errors.New("speech: transcribe returned empty payload")
	}
	var envelope struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(trimmed), &envelope); err == nil {
		return TranscriptionResult{Text: envelope.Text}, nil
	}
	firstLine := trimmed
	if index := strings.IndexAny(trimmed, "\r\n"); index >= 0 {
		firstLine = strings.TrimSpace(trimmed[:index])
	}
	if err := json.Unmarshal([]byte(firstLine), &envelope); err != nil {
		return TranscriptionResult{}, fmt.Errorf("speech: parse transcribe payload: %w", err)
	}
	return TranscriptionResult{Text: envelope.Text}, nil
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `cd backend && go test ./internal/speech/... -v`
Expected: PASS —— 全部通过

- [ ] **Step 5: 提交**

```bash
cd backend && gofmt -w internal/speech/ && go test ./internal/speech/... && cd ..
git add backend/internal/speech/
git commit -m "feat(speech): 新增 OpenAI 兼容 TTS/ASR provider"
```

---

### Task 3: 音频落盘存储与目录清理

**Files:**
- Create: `backend/internal/speech/store.go`
- Test: `backend/internal/speech/store_test.go`

**Interfaces:**
- Consumes: 无
- Produces:
  - `type AudioStore struct { root string }`
  - `func NewAudioStore(root string) *AudioStore`
  - `func (s *AudioStore) QuestionAudioPath(tenantID, interviewID, questionID int64) string`
  - `func (s *AudioStore) RecordingPath(tenantID, interviewID, questionID int64) string`
  - `func (s *AudioStore) SaveQuestionAudio(tenantID, interviewID, questionID int64, audio []byte) error`
  - `func (s *AudioStore) SaveRecording(tenantID, interviewID, questionID int64, audio []byte) error`
  - `func (s *AudioStore) ReadQuestionAudio(tenantID, interviewID, questionID int64) ([]byte, error)`
  - `func (s *AudioStore) ReadRecording(tenantID, interviewID, questionID int64) ([]byte, error)`
  - `func (s *AudioStore) CleanupInterview(tenantID, interviewID int64) error`
  - `var ErrAudioNotFound = errors.New("audio not found")`

- [ ] **Step 1: 写失败的存储测试**

创建 `backend/internal/speech/store_test.go`：

```go
package speech

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAudioStorePathsAreScopedByTenantAndInterview(t *testing.T) {
	store := NewAudioStore(t.TempDir())

	question := store.QuestionAudioPath(9, 3, 12)
	recording := store.RecordingPath(9, 3, 12)

	if filepath.Base(question) != "q12.wav" {
		t.Errorf("question base = %q", filepath.Base(question))
	}
	if filepath.Base(recording) != "r12.wav" {
		t.Errorf("recording base = %q", filepath.Base(recording))
	}
	if filepath.Dir(question) != filepath.Dir(recording) {
		t.Errorf("question and recording dirs differ: %q vs %q", filepath.Dir(question), filepath.Dir(recording))
	}
	if filepath.Base(filepath.Dir(question)) != "3" {
		t.Errorf("interview dir = %q", filepath.Base(filepath.Dir(question)))
	}
	if filepath.Base(filepath.Dir(filepath.Dir(question))) != "9" {
		t.Errorf("tenant dir = %q", filepath.Base(filepath.Dir(filepath.Dir(question))))
	}
}

func TestAudioStoreSavesAndReadsBothKinds(t *testing.T) {
	store := NewAudioStore(t.TempDir())

	if err := store.SaveQuestionAudio(1, 2, 3, []byte("q-bytes")); err != nil {
		t.Fatalf("SaveQuestionAudio error = %v", err)
	}
	if err := store.SaveRecording(1, 2, 3, []byte("r-bytes")); err != nil {
		t.Fatalf("SaveRecording error = %v", err)
	}
	question, err := store.ReadQuestionAudio(1, 2, 3)
	if err != nil {
		t.Fatalf("ReadQuestionAudio error = %v", err)
	}
	if string(question) != "q-bytes" {
		t.Errorf("question = %q", string(question))
	}
	recording, err := store.ReadRecording(1, 2, 3)
	if err != nil {
		t.Fatalf("ReadRecording error = %v", err)
	}
	if string(recording) != "r-bytes" {
		t.Errorf("recording = %q", string(recording))
	}
}

func TestAudioStoreReadMissingReturnsErrAudioNotFound(t *testing.T) {
	store := NewAudioStore(t.TempDir())
	if _, err := store.ReadQuestionAudio(1, 2, 3); !errors.Is(err, ErrAudioNotFound) {
		t.Errorf("error = %v, want ErrAudioNotFound", err)
	}
}

func TestAudioStoreCleanupRemovesOnlyThatInterview(t *testing.T) {
	store := NewAudioStore(t.TempDir())
	if err := store.SaveQuestionAudio(1, 2, 3, []byte("a")); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRecording(1, 2, 3, []byte("b")); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveQuestionAudio(1, 99, 3, []byte("c")); err != nil {
		t.Fatal(err)
	}
	if err := store.CleanupInterview(1, 2); err != nil {
		t.Fatalf("CleanupInterview error = %v", err)
	}
	if _, err := store.ReadQuestionAudio(1, 2, 3); !errors.Is(err, ErrAudioNotFound) {
		t.Errorf("cleaned interview still readable: %v", err)
	}
	if _, err := store.ReadQuestionAudio(1, 99, 3); err != nil {
		t.Errorf("other interview removed: %v", err)
	}
}

func TestAudioStoreCleanupIsIdempotent(t *testing.T) {
	store := NewAudioStore(t.TempDir())
	if err := store.CleanupInterview(1, 2); err != nil {
		t.Errorf("first cleanup error = %v", err)
	}
	if err := store.CleanupInterview(1, 2); err != nil {
		t.Errorf("second cleanup error = %v", err)
	}
}

func TestAudioStoreSaveRejectsEmptyPayload(t *testing.T) {
	store := NewAudioStore(t.TempDir())
	if err := store.SaveQuestionAudio(1, 2, 3, nil); err == nil {
		t.Fatal("SaveQuestionAudio error = nil, want error for empty payload")
	}
}

func TestAudioStoreWritesOnlyExpectedFiles(t *testing.T) {
	store := NewAudioStore(t.TempDir())
	if err := store.SaveQuestionAudio(1, 2, 3, []byte("x")); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(store.InterviewDir(1, 2))
	if err != nil {
		t.Fatalf("ReadDir error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd backend && go test ./internal/speech/... -run 'TestAudioStore' -v`
Expected: FAIL —— `undefined: NewAudioStore`

- [ ] **Step 3: 实现 store.go**

创建 `backend/internal/speech/store.go`：

```go
package speech

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// ErrAudioNotFound 表示该题没有对应的音频文件。
var ErrAudioNotFound = errors.New("audio not found")

// AudioStore 负责面试音频的落盘与清理。
//
// 目录结构：{root}/{tenantID}/{interviewID}/q{questionID}.wav 与 r{questionID}.wav。
// 以 interviewID 为清理单元 —— 面试结束即删除整个目录。
type AudioStore struct {
	root string
}

// NewAudioStore 创建存储，root 为音频根目录。
func NewAudioStore(root string) *AudioStore {
	return &AudioStore{root: root}
}

// InterviewDir 返回某租户某场面试的音频目录。
func (s *AudioStore) InterviewDir(tenantID, interviewID int64) string {
	return filepath.Join(s.root, strconv.FormatInt(tenantID, 10), strconv.FormatInt(interviewID, 10))
}

// QuestionAudioPath 返回题目语音文件路径。
func (s *AudioStore) QuestionAudioPath(tenantID, interviewID, questionID int64) string {
	return filepath.Join(s.InterviewDir(tenantID, interviewID), "q"+strconv.FormatInt(questionID, 10)+".wav")
}

// RecordingPath 返回用户录音文件路径。
func (s *AudioStore) RecordingPath(tenantID, interviewID, questionID int64) string {
	return filepath.Join(s.InterviewDir(tenantID, interviewID), "r"+strconv.FormatInt(questionID, 10)+".wav")
}

// SaveQuestionAudio 写入题目语音。
func (s *AudioStore) SaveQuestionAudio(tenantID, interviewID, questionID int64, audio []byte) error {
	return s.writeFile(s.QuestionAudioPath(tenantID, interviewID, questionID), audio)
}

// SaveRecording 写入用户录音。
func (s *AudioStore) SaveRecording(tenantID, interviewID, questionID int64, audio []byte) error {
	return s.writeFile(s.RecordingPath(tenantID, interviewID, questionID), audio)
}

// ReadQuestionAudio 读取题目语音。
func (s *AudioStore) ReadQuestionAudio(tenantID, interviewID, questionID int64) ([]byte, error) {
	return s.readFile(s.QuestionAudioPath(tenantID, interviewID, questionID))
}

// ReadRecording 读取用户录音。
func (s *AudioStore) ReadRecording(tenantID, interviewID, questionID int64) ([]byte, error) {
	return s.readFile(s.RecordingPath(tenantID, interviewID, questionID))
}

// CleanupInterview 删除某场面试的全部音频。目录不存在时视为成功（幂等）。
func (s *AudioStore) CleanupInterview(tenantID, interviewID int64) error {
	if err := os.RemoveAll(s.InterviewDir(tenantID, interviewID)); err != nil {
		return fmt.Errorf("speech: cleanup interview %d audio: %w", interviewID, err)
	}
	return nil
}

// writeFile 原子写入音频文件（先写临时文件再改名，避免读到半截文件）。
func (s *AudioStore) writeFile(path string, audio []byte) error {
	if len(audio) == 0 {
		return errors.New("speech: audio payload is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("speech: create audio directory: %w", err)
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, audio, 0o640); err != nil {
		return fmt.Errorf("speech: write audio file: %w", err)
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("speech: replace audio file: %w", err)
	}
	return nil
}

// readFile 读取音频文件；不存在返回 ErrAudioNotFound。
func (s *AudioStore) readFile(path string) ([]byte, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrAudioNotFound
		}
		return nil, fmt.Errorf("speech: read audio file: %w", err)
	}
	return payload, nil
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `cd backend && go test ./internal/speech/... -v`
Expected: PASS —— 全部通过

- [ ] **Step 5: 提交**

```bash
cd backend && gofmt -w internal/speech/ && go test ./internal/speech/... && cd ..
git add backend/internal/speech/
git commit -m "feat(speech): 新增面试音频落盘存储与目录清理"
```

### Task 4: 数据库 audio_state 列与仓储读写

**Files:**
- Modify: `backend/internal/store/migrate.go`
- Modify: `backend/internal/interview/repository.go`
- Test: `backend/internal/interview/repository_test.go`（追加用例）
- Test: `backend/internal/store/migrate_test.go`（追加幂等用例）

**Interfaces:**
- Consumes: 无
- Produces:
  - `interview.Question.AudioState string`（db tag `audio_state`，可空列用 COALESCE 归一成空串）
  - `func (r *Repository) UpdateAudioState(ctx context.Context, questionID int64, state string) error`
  - `func (r *Repository) ClearAudioStates(ctx context.Context, interviewID int64) error`

- [ ] **Step 1: 先读现有测试辅助函数**

Read: `backend/internal/interview/repository_test.go`（顶部 60 行）与 `backend/internal/store/migrate_test.go`（全文）
记录下现有的建库 / 建面试 / 建题辅助函数真实名称。下一步的测试代码必须使用**这些真实名称**，不要臆造。

- [ ] **Step 2: 写失败的迁移幂等测试**

在 `backend/internal/store/migrate_test.go` 末尾追加（按该文件既有的建库方式调整）：

```go
func TestMigrateAddsAudioStateColumnIdempotently(t *testing.T) {
	db, err := sqlx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	for i := 0; i < 2; i++ {
		if err := Migrate(context.Background(), db); err != nil {
			t.Fatalf("migrate run %d: %v", i, err)
		}
	}

	rows, err := db.Query(`PRAGMA table_info(interview_questions)`)
	if err != nil {
		t.Fatalf("query table info: %v", err)
	}
	defer rows.Close()
	found := false
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatalf("scan table info: %v", err)
		}
		if name == "audio_state" {
			found = true
		}
	}
	if !found {
		t.Fatal("interview_questions.audio_state column missing")
	}
}
```

- [ ] **Step 3: 写失败的仓储测试**

在 `backend/internal/interview/repository_test.go` 末尾追加（使用 Step 1 记下的真实辅助函数名）：

```go
func TestUpdateAudioStateAndClear(t *testing.T) {
	// 用 Step 1 记下的真实辅助函数建面试与题目
	interviewID, questionID := /* 真实辅助函数调用 */, /* 真实辅助函数调用 */

	if err := repo.UpdateAudioState(ctx, questionID, "ready"); err != nil {
		t.Fatalf("UpdateAudioState error = %v", err)
	}
	questions, err := repo.Questions(ctx, tenantID, interviewID)
	if err != nil {
		t.Fatalf("questions error = %v", err)
	}
	if len(questions) == 0 || questions[0].AudioState != "ready" {
		t.Fatalf("audio state not persisted: %+v", questions)
	}
	if err := repo.ClearAudioStates(ctx, interviewID); err != nil {
		t.Fatalf("ClearAudioStates error = %v", err)
	}
	questions, err = repo.Questions(ctx, tenantID, interviewID)
	if err != nil {
		t.Fatalf("questions error = %v", err)
	}
	if questions[0].AudioState != "" {
		t.Errorf("audio state = %q, want empty", questions[0].AudioState)
	}
}
```

- [ ] **Step 4: 运行测试确认失败**

Run: `cd backend && go test ./internal/store/... ./internal/interview/... -run 'AudioState|MigrateAddsAudioState' -v`
Expected: FAIL —— 列不存在 / `repo.UpdateAudioState` undefined

- [ ] **Step 5: 加迁移**

在 `backend/internal/store/migrate.go` 的 `schema` 常量里，`interview_questions` 建表语句的 `grade TEXT,` 之后追加：

```sql
  audio_state TEXT,
```

在 `Migrate` 函数里，`addColumnIfMissing(ctx, tx, "resume_versions", ...)` 之后追加：

```go
	if err := addColumnIfMissing(ctx, tx, "interview_questions", "audio_state", "TEXT"); err != nil {
		return err
	}
```

（`addColumnIfMissing` 已存在，重复执行安全 —— 这正是幂等迁移要用的既有辅助函数。）

- [ ] **Step 6: 改仓储**

在 `backend/internal/interview/repository.go`：

6a. `questionColumns` 常量改为：

```go
const questionColumns = `id, interview_id, parent_id, follow_up_round, label, question, ` +
	`COALESCE(standard_answer, '') AS standard_answer, ` +
	`COALESCE(user_answer, '') AS user_answer, ` +
	`COALESCE(extra, '') AS extra, ` +
	`COALESCE(grade, '') AS grade, ` +
	`COALESCE(audio_state, '') AS audio_state, ` +
	`created_at`
```

6b. `Question` 结构体新增字段（放在 `Grade` 之后）：

```go
	// AudioState 表示该题是否有可播放音频："ready"（题目语音已就绪）、
	// "recorded"（用户录音已保存）；空串表示尚无音频或已随面试结束清理。
	AudioState string `db:"audio_state" json:"audioState"`
```

6c. 在 `UpdateGrade` 之后新增两个方法：

```go
// UpdateAudioState 更新一道题的音频状态。
//
// state 为 "ready" / "recorded"；传空串表示清空。目标行不存在返回 ErrNotFound。
func (r *Repository) UpdateAudioState(ctx context.Context, questionID int64, state string) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE interview_questions
		SET audio_state = ?
		WHERE id = ?`, nullableText(state), questionID)
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

// ClearAudioStates 清空一场面试下所有题目的音频状态。
//
// 面试结束并删除音频目录后调用，使数据库状态与磁盘文件保持一致。
func (r *Repository) ClearAudioStates(ctx context.Context, interviewID int64) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE interview_questions
		SET audio_state = NULL
		WHERE interview_id = ?`, interviewID)
	return err
}
```

- [ ] **Step 7: 运行测试确认通过**

Run: `cd backend && go test ./internal/store/... ./internal/interview/... -v`
Expected: PASS —— 全部通过

- [ ] **Step 8: 提交**

```bash
cd backend && gofmt -w internal/store/ internal/interview/ && go test ./... && cd ..
git add backend/internal/store/migrate.go backend/internal/interview/repository.go backend/internal/store/migrate_test.go backend/internal/interview/repository_test.go
git commit -m "feat(interview): 新增 audio_state 列与音频状态读写"
```

---

### Task 5: 合成去重（singleflight）与预合成

**Files:**
- Create: `backend/internal/speech/cache.go`
- Test: `backend/internal/speech/cache_test.go`

**Interfaces:**
- Consumes: Task 1 的 `Synthesizer` 接口、Task 3 的 `AudioStore`
- Produces:
  - `type PrewarmQuestion struct { ID int64; Text string }`
  - `type SynthesisCache struct { ... }`
  - `func NewSynthesisCache(synthesizer Synthesizer, store *AudioStore) *SynthesisCache`
  - `func (c *SynthesisCache) EnsureQuestionAudio(ctx context.Context, tenantID, interviewID, questionID int64, text string) ([]byte, error)`
  - `func (c *SynthesisCache) Prewarm(ctx context.Context, tenantID, interviewID int64, questions []PrewarmQuestion) error`

- [ ] **Step 1: 写失败的 cache 测试**

创建 `backend/internal/speech/cache_test.go`：

```go
package speech

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type countingSynthesizer struct {
	calls atomic.Int32
	delay time.Duration
}

func (s *countingSynthesizer) Synthesize(ctx context.Context, req SynthesisRequest) (SynthesisResult, error) {
	s.calls.Add(1)
	if s.delay > 0 {
		time.Sleep(s.delay)
	}
	return SynthesisResult{Audio: []byte("audio:" + req.Text), ContentType: "audio/wav"}, nil
}

func TestEnsureQuestionAudioSynthesizesOnceAndReuses(t *testing.T) {
	synth := &countingSynthesizer{}
	store := NewAudioStore(t.TempDir())
	cache := NewSynthesisCache(synth, store)

	first, err := cache.EnsureQuestionAudio(context.Background(), 1, 2, 3, "第一题")
	if err != nil {
		t.Fatalf("first error = %v", err)
	}
	second, err := cache.EnsureQuestionAudio(context.Background(), 1, 2, 3, "第一题")
	if err != nil {
		t.Fatalf("second error = %v", err)
	}
	if string(first) != string(second) {
		t.Errorf("bytes differ: %q vs %q", string(first), string(second))
	}
	if synth.calls.Load() != 1 {
		t.Errorf("synthesize calls = %d, want 1", synth.calls.Load())
	}
}

func TestEnsureQuestionAudioConcurrentCallersShareOneSynthesis(t *testing.T) {
	synth := &countingSynthesizer{delay: 40 * time.Millisecond}
	store := NewAudioStore(t.TempDir())
	cache := NewSynthesisCache(synth, store)

	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			_, errs[index] = cache.EnsureQuestionAudio(context.Background(), 1, 2, 3, "并发题")
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d error = %v", i, err)
		}
	}
	if synth.calls.Load() != 1 {
		t.Errorf("synthesize calls = %d, want 1", synth.calls.Load())
	}
}

func TestEnsureQuestionAudioReadsExistingFileWithoutSynthesizing(t *testing.T) {
	synth := &countingSynthesizer{}
	store := NewAudioStore(t.TempDir())
	if err := store.SaveQuestionAudio(1, 2, 3, []byte("existing")); err != nil {
		t.Fatal(err)
	}
	cache := NewSynthesisCache(synth, store)

	payload, err := cache.EnsureQuestionAudio(context.Background(), 1, 2, 3, "已有")
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if string(payload) != "existing" {
		t.Errorf("payload = %q", string(payload))
	}
	if synth.calls.Load() != 0 {
		t.Errorf("synthesize calls = %d, want 0", synth.calls.Load())
	}
}

func TestPrewarmSynthesizesEveryQuestion(t *testing.T) {
	synth := &countingSynthesizer{}
	store := NewAudioStore(t.TempDir())
	cache := NewSynthesisCache(synth, store)

	questions := []PrewarmQuestion{{ID: 1, Text: "一"}, {ID: 2, Text: "二"}, {ID: 3, Text: "三"}}
	if err := cache.Prewarm(context.Background(), 1, 2, questions); err != nil {
		t.Fatalf("Prewarm error = %v", err)
	}
	if synth.calls.Load() != 3 {
		t.Errorf("synthesize calls = %d, want 3", synth.calls.Load())
	}
	for _, question := range questions {
		if _, err := store.ReadQuestionAudio(1, 2, question.ID); err != nil {
			t.Errorf("question %d not stored: %v", question.ID, err)
		}
	}
}

func TestPrewarmStopsOnContextCancel(t *testing.T) {
	synth := &countingSynthesizer{}
	store := NewAudioStore(t.TempDir())
	cache := NewSynthesisCache(synth, store)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := cache.Prewarm(ctx, 1, 2, []PrewarmQuestion{{ID: 1, Text: "一"}, {ID: 2, Text: "二"}})
	if err == nil {
		t.Fatal("Prewarm error = nil, want context error")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd backend && go test ./internal/speech/... -run 'TestEnsureQuestionAudio|TestPrewarm' -v`
Expected: FAIL —— `undefined: NewSynthesisCache`

- [ ] **Step 3: 把 x/sync 提升为直接依赖**

Run: `cd backend && go get golang.org/x/sync@v0.22.0`
Expected: `backend/go.mod` 中 `golang.org/x/sync v0.22.0` 从 `// indirect` 移到直接依赖块

- [ ] **Step 4: 实现 cache.go**

创建 `backend/internal/speech/cache.go`：

```go
package speech

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/sync/singleflight"
)

// PrewarmQuestion 是预合成队列里的一道题。
type PrewarmQuestion struct {
	// ID 题目 id。
	ID int64
	// Text 题面文本。
	Text string
}

// SynthesisCache 负责题目语音的落盘复用与合成去重。
//
// 同一题的并发请求通过 singleflight 合并为一次合成；预合成逐题串行提交，
// 使用户点喇叭时最多等一题（上游推理服务是单 worker 串行的，见设计文档坑 3）。
type SynthesisCache struct {
	synthesizer Synthesizer
	store       *AudioStore
	group       singleflight.Group
}

// NewSynthesisCache 创建缓存。
func NewSynthesisCache(synthesizer Synthesizer, store *AudioStore) *SynthesisCache {
	return &SynthesisCache{synthesizer: synthesizer, store: store}
}

// EnsureQuestionAudio 返回某题的题目语音字节。
//
// 文件已存在则直接读盘；否则合成一次并落盘。并发调用共享同一次合成。
func (c *SynthesisCache) EnsureQuestionAudio(ctx context.Context, tenantID, interviewID, questionID int64, text string) ([]byte, error) {
	if payload, err := c.store.ReadQuestionAudio(tenantID, interviewID, questionID); err == nil {
		return payload, nil
	} else if !errors.Is(err, ErrAudioNotFound) {
		return nil, err
	}

	key := fmt.Sprintf("%d/%d/%d", tenantID, interviewID, questionID)
	result, err, _ := c.group.Do(key, func() (any, error) {
		// 进入临界区后复查：可能已被另一个请求写好。
		if payload, err := c.store.ReadQuestionAudio(tenantID, interviewID, questionID); err == nil {
			return payload, nil
		} else if !errors.Is(err, ErrAudioNotFound) {
			return nil, err
		}
		synthesized, err := c.synthesizer.Synthesize(ctx, SynthesisRequest{Text: text})
		if err != nil {
			return nil, err
		}
		if err := c.store.SaveQuestionAudio(tenantID, interviewID, questionID, synthesized.Audio); err != nil {
			return nil, err
		}
		return synthesized.Audio, nil
	})
	if err != nil {
		return nil, err
	}
	payload, ok := result.([]byte)
	if !ok {
		return nil, errors.New("speech: unexpected synthesis payload type")
	}
	return payload, nil
}

// Prewarm 逐题串行预合成题目语音。
//
// 串行而非并发：上游 mlx-audio 的推理 broker 是单 worker，串行提交能让
// 用户的前台请求最多等待一题，而不是一整批。
func (c *SynthesisCache) Prewarm(ctx context.Context, tenantID, interviewID int64, questions []PrewarmQuestion) error {
	for _, question := range questions {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := c.EnsureQuestionAudio(ctx, tenantID, interviewID, question.ID, question.Text); err != nil {
			return fmt.Errorf("speech: prewarm question %d: %w", question.ID, err)
		}
	}
	return nil
}
```

- [ ] **Step 5: 运行测试确认通过**

Run: `cd backend && go test ./internal/speech/... -v`
Expected: PASS —— 全部通过

- [ ] **Step 6: 提交**

```bash
cd backend && gofmt -w internal/speech/ && go test ./internal/speech/... && cd ..
git add backend/internal/speech/cache.go backend/internal/speech/cache_test.go backend/go.mod backend/go.sum
git commit -m "feat(speech): 新增题目语音合成去重与预合成"
```

### Task 6: HTTP 接口（朗读 / 录音回放 / 转写）

**Files:**
- Create: `backend/internal/speech/service.go`（用例层门面）
- Create: `backend/internal/httpapi/voice_handler.go`
- Modify: `backend/internal/httpapi/router.go`
- Test: `backend/internal/httpapi/voice_handler_test.go`

**Interfaces:**
- Consumes:
  - Task 2 的 `Synthesizer` / `Transcriber`
  - Task 3 的 `AudioStore`
  - Task 5 的 `SynthesisCache`
  - 既有 `interview.Service.Questions`（返回带 `AudioState` 的题目，Task 4 已加）
- Produces:
  - `type VoiceService struct { ... }` 与 `func NewVoiceService(synthesizer Synthesizer, transcriber Transcriber, store *AudioStore, cache *SynthesisCache) *VoiceService`
  - `func (s *VoiceService) Synthesizer() Synthesizer`
  - `func (s *VoiceService) Transcriber() Transcriber`
  - `func (s *VoiceService) Store() *AudioStore`
  - `func (s *VoiceService) Cache() *SynthesisCache`
  - `RouterConfig.VoiceService *speech.VoiceService`
  - `RouterConfig.VoiceEnabled bool`
  - 路由：
    - `GET /api/interviews/:id/questions/:qid/audio`
    - `GET /api/interviews/:id/questions/:qid/recording`
    - `POST /api/interviews/:id/questions/:qid/voice/transcribe`

**接口路径与设计文档的差异：** 设计文档写的是 `POST /api/interviews/:id/voice/transcribe`（不含 `:qid`）。录音必须归属到具体题目才能落盘与回放，因此实现采用含 `:qid` 的形式。**Task 12 会同步更新设计文档。**

- [ ] **Step 1: 先读现有 handler 测试的辅助函数**

Read: `backend/internal/httpapi/interview_handler_test.go`（全文）
记录下：路由构造方式、登录/会话辅助函数名、如何造带题目的面试。后续测试代码必须用**这些真实名称**。

- [ ] **Step 2: 写失败的 handler 测试**

创建 `backend/internal/httpapi/voice_handler_test.go`，用 Step 1 记下的真实辅助函数。必须覆盖：

| 用例 | 期望 |
|---|---|
| 未认证访问 audio | 401 |
| 未认证访问 recording | 401 |
| 未认证访问 transcribe | 401 |
| 跨租户访问 audio | 404 |
| 该题无录音时访问 recording | 404 |
| 上传非法音频（无 RIFF magic） | 400 |
| 语音服务未启用（VoiceEnabled=false） | 503 |
| 正常转写 | 200 且返回 `{"text":"..."}` |

测试骨架（辅助函数名按 Step 1 替换）：

```go
package httpapi

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"resumepro/backend/internal/speech"
)

// 语音 provider 的测试替身
type fakeVoiceProvider struct {
	transcript string
}

func (f *fakeVoiceProvider) Synthesize(ctx context.Context, req speech.SynthesisRequest) (speech.SynthesisResult, error) {
	return speech.SynthesisResult{Audio: []byte("RIFFfake"), ContentType: "audio/wav"}, nil
}

func (f *fakeVoiceProvider) Transcribe(ctx context.Context, req speech.TranscriptionRequest) (speech.TranscriptionResult, error) {
	return speech.TranscriptionResult{Text: f.transcript}, nil
}
```

- [ ] **Step 3: 运行测试确认失败**

Run: `cd backend && go test ./internal/httpapi/... -run 'Voice|QuestionAudio|Recording|Transcribe' -v`
Expected: FAIL —— handler 与路由尚不存在

- [ ] **Step 4: 实现 speech 用例层门面**

创建 `backend/internal/speech/service.go`：

```go
package speech

// VoiceService 聚合语音能力的用例层：provider（协议实现）、音频存储与合成缓存。
//
// httpapi 层只与本层对话，不直接持有 provider。
type VoiceService struct {
	synthesizer Synthesizer
	transcriber Transcriber
	store       *AudioStore
	cache       *SynthesisCache
}

// NewVoiceService 创建语音用例层。
func NewVoiceService(synthesizer Synthesizer, transcriber Transcriber, store *AudioStore, cache *SynthesisCache) *VoiceService {
	return &VoiceService{
		synthesizer: synthesizer,
		transcriber: transcriber,
		store:       store,
		cache:       cache,
	}
}

// Synthesizer 返回合成能力。
func (s *VoiceService) Synthesizer() Synthesizer { return s.synthesizer }

// Transcriber 返回转写能力。
func (s *VoiceService) Transcriber() Transcriber { return s.transcriber }

// Store 返回音频存储。
func (s *VoiceService) Store() *AudioStore { return s.store }

// Cache 返回合成缓存。
func (s *VoiceService) Cache() *SynthesisCache { return s.cache }
```

- [ ] **Step 5: 实现 voice_handler.go**

创建 `backend/internal/httpapi/voice_handler.go`：

```go
package httpapi

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"resumepro/backend/internal/interview"
	"resumepro/backend/internal/speech"
)

const (
	// maxVoiceUploadBytes 是单次录音上传的体积上限（含 multipart 信封）。
	maxVoiceUploadBytes = 20 << 20 // 20 MiB
	// maxVoiceAudioSeconds 是单次录音的时长上限。
	maxVoiceAudioSeconds = 10 * 60
	// voiceSampleRate 是期望的采样率；浏览器端已重采样到此。
	voiceSampleRate = 16000
)

type voiceHandler struct {
	service *interview.Service
	voice   *speech.VoiceService
	enabled bool
}

func newVoiceHandler(service *interview.Service, voice *speech.VoiceService, enabled bool) *voiceHandler {
	return &voiceHandler{service: service, voice: voice, enabled: enabled}
}

// questionAudio 返回某题的题目语音。
func (h *voiceHandler) questionAudio(ginCtx *gin.Context) {
	principal, ok := principalFromContext(ginCtx)
	if !ok {
		writeError(ginCtx, http.StatusUnauthorized, "authentication required")
		return
	}
	if !h.enabled || h.voice == nil {
		writeError(ginCtx, http.StatusServiceUnavailable, "voice features disabled")
		return
	}
	interviewID, ok := interviewRouteID(ginCtx)
	if !ok {
		return
	}
	questionID, ok := questionRouteID(ginCtx)
	if !ok {
		return
	}
	question, err := h.findQuestion(ginCtx, principal.TenantID, interviewID, questionID)
	if err != nil {
		writeError(ginCtx, http.StatusNotFound, "question not found")
		return
	}
	payload, err := h.voice.Cache().EnsureQuestionAudio(
		ginCtx.Request.Context(), principal.TenantID, interviewID, questionID, question.Question,
	)
	if err != nil {
		writeError(ginCtx, http.StatusServiceUnavailable, "could not synthesize question audio", err)
		return
	}
	ginCtx.Data(http.StatusOK, "audio/wav", payload)
}

// recording 返回某题的用户录音。
func (h *voiceHandler) recording(ginCtx *gin.Context) {
	principal, ok := principalFromContext(ginCtx)
	if !ok {
		writeError(ginCtx, http.StatusUnauthorized, "authentication required")
		return
	}
	if !h.enabled || h.voice == nil {
		writeError(ginCtx, http.StatusServiceUnavailable, "voice features disabled")
		return
	}
	interviewID, ok := interviewRouteID(ginCtx)
	if !ok {
		return
	}
	questionID, ok := questionRouteID(ginCtx)
	if !ok {
		return
	}
	if _, err := h.findQuestion(ginCtx, principal.TenantID, interviewID, questionID); err != nil {
		writeError(ginCtx, http.StatusNotFound, "question not found")
		return
	}
	payload, err := h.voice.Store().ReadRecording(principal.TenantID, interviewID, questionID)
	if err != nil {
		if errors.Is(err, speech.ErrAudioNotFound) {
			writeError(ginCtx, http.StatusNotFound, "recording not found")
			return
		}
		writeError(ginCtx, http.StatusInternalServerError, "could not read recording", err)
		return
	}
	ginCtx.Data(http.StatusOK, "audio/wav", payload)
}

// transcribe 接收录音，落盘并转写为文本。
func (h *voiceHandler) transcribe(ginCtx *gin.Context) {
	principal, ok := principalFromContext(ginCtx)
	if !ok {
		writeError(ginCtx, http.StatusUnauthorized, "authentication required")
		return
	}
	if !h.enabled || h.voice == nil {
		writeError(ginCtx, http.StatusServiceUnavailable, "voice features disabled")
		return
	}
	interviewID, ok := interviewRouteID(ginCtx)
	if !ok {
		return
	}
	questionID, ok := questionRouteID(ginCtx)
	if !ok {
		return
	}
	if _, err := h.findQuestion(ginCtx, principal.TenantID, interviewID, questionID); err != nil {
		writeError(ginCtx, http.StatusNotFound, "question not found")
		return
	}
	ginCtx.Request.Body = http.MaxBytesReader(ginCtx.Writer, ginCtx.Request.Body, maxVoiceUploadBytes)
	fileHeader, err := ginCtx.FormFile("file")
	if err != nil {
		writeError(ginCtx, http.StatusBadRequest, "recording file is required")
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		writeError(ginCtx, http.StatusBadRequest, "recording file is required")
		return
	}
	defer file.Close()
	payload, err := io.ReadAll(io.LimitReader(file, maxVoiceUploadBytes))
	if err != nil {
		writeError(ginCtx, http.StatusBadRequest, "could not read recording")
		return
	}
	if err := validateVoiceAudio(payload); err != nil {
		writeError(ginCtx, http.StatusBadRequest, "recording must be a 16kHz mono WAV file")
		return
	}
	if err := h.voice.Store().SaveRecording(principal.TenantID, interviewID, questionID, payload); err != nil {
		writeError(ginCtx, http.StatusInternalServerError, "could not store recording", err)
		return
	}
	result, err := h.voice.Transcriber().Transcribe(ginCtx.Request.Context(), speech.TranscriptionRequest{
		Audio:       payload,
		ContentType: "audio/wav",
	})
	if err != nil {
		writeError(ginCtx, http.StatusServiceUnavailable, "could not transcribe recording", err)
		return
	}
	writeJSON(ginCtx, http.StatusOK, map[string]string{"text": result.Text})
}

// findQuestion 读取并校验题目归属（跨租户一律 404）。
func (h *voiceHandler) findQuestion(ginCtx *gin.Context, tenantID, interviewID, questionID int64) (interview.Question, error) {
	questions, err := h.service.Questions(ginCtx.Request.Context(), tenantID, interviewID)
	if err != nil {
		return interview.Question{}, err
	}
	for _, question := range questions {
		if question.ID == questionID {
			return question, nil
		}
	}
	return interview.Question{}, interview.ErrNotFound
}

// questionRouteID 解析 :qid 路径参数。
func questionRouteID(context *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(context.Param("qid"), 10, 64)
	if err != nil || id <= 0 {
		writeError(context, http.StatusBadRequest, "question ID must be a positive integer")
		return 0, false
	}
	return id, true
}

// validateVoiceAudio 校验 WAV 头部：RIFF magic、采样率、单声道、时长上限。
//
// 后端不做任何转码，只做校验与字节透传。
func validateVoiceAudio(payload []byte) error {
	if len(payload) < 44 {
		return errors.New("voice: audio payload too short")
	}
	if !bytes.Equal(payload[0:4], []byte("RIFF")) || !bytes.Equal(payload[8:12], []byte("WAVE")) {
		return errors.New("voice: not a RIFF/WAVE payload")
	}
	if !bytes.Equal(payload[12:16], []byte("fmt ")) {
		return errors.New("voice: missing fmt chunk")
	}
	channels := binary.LittleEndian.Uint16(payload[22:24])
	sampleRate := binary.LittleEndian.Uint32(payload[24:28])
	if channels != 1 {
		return errors.New("voice: recording must be mono")
	}
	if sampleRate != voiceSampleRate {
		return errors.New("voice: recording must be 16kHz")
	}
	byteRate := binary.LittleEndian.Uint32(payload[28:32])
	if byteRate == 0 {
		return errors.New("voice: invalid byte rate")
	}
	duration := float64(len(payload)-44) / float64(byteRate)
	if duration > maxVoiceAudioSeconds {
		return errors.New("voice: recording too long")
	}
	return nil
}
```

- [ ] **Step 6: 注册路由**

在 `backend/internal/httpapi/router.go` 的 `RouterConfig` 增加：

```go
	// VoiceService is nil when voice features are disabled; the authenticated
	// routes remain registered and return an explicit 503.
	VoiceService *speech.VoiceService
	// VoiceEnabled reports whether the voice feature is turned on.
	VoiceEnabled bool
```

并 import `"resumepro/backend/internal/speech"`。

在 `interviews` 路由组内（`interviews.Use(requireAuthentication(service))` 之后）追加：

```go
	voiceHandler := newVoiceHandler(config.InterviewService, config.VoiceService, config.VoiceEnabled)
	interviews.GET("/:id/questions/:qid/audio", voiceHandler.questionAudio)
	interviews.GET("/:id/questions/:qid/recording", voiceHandler.recording)
	interviews.POST("/:id/questions/:qid/voice/transcribe", voiceHandler.transcribe)
```

**注意路由冲突：** 既有路由有 `interviews.GET("/:id/questions", ...)`。Gin 的路由树里 `/:id/questions` 与 `/:id/questions/:qid/audio` 是不同分支，可以共存；但要确认 Gin 版本允许在 `/:id/questions` 之后继续挂 `/:id/questions/:qid/*`。若报 panic，说明该路径段冲突 —— 改用 `interviews.GET("/:id/audio/:qid", ...)` 之类的等价路径并同步更新前端与本计划。

- [ ] **Step 7: 运行测试确认通过**

Run: `cd backend && go test ./internal/httpapi/... -v`
Expected: PASS —— 全部通过

- [ ] **Step 8: 提交**

```bash
cd backend && gofmt -w internal/httpapi/ internal/speech/ && go test ./... && cd ..
git add backend/internal/httpapi/voice_handler.go backend/internal/httpapi/voice_handler_test.go backend/internal/httpapi/router.go backend/internal/speech/service.go
git commit -m "feat(api): 新增面试语音朗读、回放与转写接口"
```

---

### Task 7: 面试结束清理钩子 + main.go 接线

**Files:**
- Modify: `backend/internal/interview/service.go`
- Modify: `backend/internal/interview/graph.go`
- Modify: `backend/cmd/server/main.go`
- Test: `backend/internal/interview/service_test.go`（追加清理用例）

**Interfaces:**
- Consumes:
  - Task 3 的 `speech.AudioStore.CleanupInterview`
  - Task 4 的 `interview.Repository.ClearAudioStates`
- Produces:
  - `type AudioCleaner interface { CleanupInterview(tenantID, interviewID int64) error }`（定义在 `backend/internal/interview` 包；`speech.AudioStore` 天然满足）
  - `func NewService(repo *Repository, store *RedisStore, gen *QuestionGenerator, graph *Graph, cleaner AudioCleaner) *Service`

**注意：** `NewService` 签名变更会影响所有调用点（含既有测试）。改完后必须 `cd backend && go build ./... && go test ./...` 确认无遗漏。

- [ ] **Step 1: 先读现有代码**

Read:
- `backend/internal/interview/service.go`（`Service` 结构体、`NewService`、`Abort`）
- `backend/internal/interview/graph.go`（`graphDeps` 结构、`NewGraph`、`finishNode`）
- `backend/internal/interview/service_test.go`（顶部：建面试辅助函数名、日志方式）
- `backend/cmd/server/main.go`（`buildRouterConfig`、`buildInterviewService`）

记录：`graphDeps` 的真实字段名、`NewGraph` 的真实签名、`interview` 包是否有 logrus 实例、测试里建面试的真实辅助函数名。**下面代码里的占位必须按实际替换。**

- [ ] **Step 2: 写失败的清理测试**

在 `backend/internal/interview/service_test.go` 追加：

```go
type recordingCleaner struct {
	mu      sync.Mutex
	cleaned []int64
	err     error
}

func (c *recordingCleaner) CleanupInterview(tenantID, interviewID int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cleaned = append(c.cleaned, interviewID)
	return c.err
}

func (c *recordingCleaner) cleanedInterviews() []int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]int64(nil), c.cleaned...)
}
```

再加用例（用 Step 1 记下的真实辅助函数建面试）：

```go
func TestAbortCleansUpAudioAndIgnoresCleanupFailure(t *testing.T) {
	// 用真实辅助函数建面试并拿到 service
	cleaner := &recordingCleaner{err: errors.New("cleanup failed")}
	// 把 cleaner 注入 service

	if err := service.Abort(ctx, tenantID, interviewID); err != nil {
		t.Fatalf("Abort error = %v, want nil despite cleanup failure", err)
	}
	if len(cleaner.cleanedInterviews()) != 1 {
		t.Errorf("cleaned = %v", cleaner.cleanedInterviews())
	}
}
```

- [ ] **Step 3: 运行测试确认失败**

Run: `cd backend && go test ./internal/interview/... -run 'TestAbortCleansUpAudio' -v`
Expected: FAIL —— cleaner 未被调用

- [ ] **Step 4: 实现 Service 侧清理**

在 `backend/internal/interview/service.go`：

4a. 新增接口（放在 Service 结构体定义之前）：

```go
// AudioCleaner 清理一场面试的全部音频文件。
//
// 由 speech.AudioStore 实现；interview 包只依赖这个窄接口，不直接 import speech
// 的存储实现（与本包不 import resume / targetjob 的既有分层一致）。
type AudioCleaner interface {
	CleanupInterview(tenantID, interviewID int64) error
}
```

4b. `Service` 加 `cleaner AudioCleaner` 字段，构造函数改为：

```go
func NewService(repo *Repository, store *RedisStore, gen *QuestionGenerator, graph *Graph, cleaner AudioCleaner) *Service {
	return &Service{repo: repo, store: store, gen: gen, graph: graph, cleaner: cleaner}
}
```

4c. 新增内部清理方法：

```go
// cleanupAudio 清理面试音频与数据库音频状态。
//
// 清理失败只记录日志：面试状态已是终态，不能因为删文件失败而让答题接口报错。
func (s *Service) cleanupAudio(ctx context.Context, tenantID, interviewID int64) {
	if s.cleaner == nil {
		return
	}
	if err := s.cleaner.CleanupInterview(tenantID, interviewID); err != nil {
		log.Warnf("interview: cleanup audio for %d: %v", interviewID, err)
	}
	if err := s.repo.ClearAudioStates(ctx, interviewID); err != nil {
		log.Warnf("interview: clear audio states for %d: %v", interviewID, err)
	}
}
```

（`log` 的来源按 Step 1 的结果定：若 `interview` 包已有 logrus 实例就用它；否则在包内加 `var log = logrus.New()`。）

4d. 在 `Abort` 中，`UpdateInterviewStatus(... "aborted" ...)` 成功之后调用：

```go
	s.cleanupAudio(ctx, tenantID, interviewID)
```

- [ ] **Step 5: 在 graph finishNode 中触发清理**

在 `backend/internal/interview/graph.go`：

5a. `graphDeps` 新增字段（按 Step 1 记下的真实结构体名调整）：

```go
	// cleaner 清理面试音频（可为 nil）。
	cleaner AudioCleaner
```

5b. `NewGraph` 的参数列表按现有风格加 `cleaner AudioCleaner`。

5c. `finishNode` 中，`UpdateInterviewStatus(... "completed" ...)` 成功之后：

```go
	if dep.cleaner != nil {
		if row, err := dep.repo.GetInterviewByID(ctx, interviewID); err == nil {
			if err := dep.cleaner.CleanupInterview(row.TenantID, interviewID); err != nil {
				log.Warnf("interview: cleanup audio for %d: %v", interviewID, err)
			}
			if err := dep.repo.ClearAudioStates(ctx, interviewID); err != nil {
				log.Warnf("interview: clear audio states for %d: %v", interviewID, err)
			}
		}
	}
```

（`GetInterviewByID` 不带租户过滤，正是给图节点这类内部组件用的；用它拿 TenantID。）

- [ ] **Step 6: main.go 接线**

在 `backend/cmd/server/main.go` 的 `buildRouterConfig` 中（AI service 装配附近）：

```go
	voiceConfig := speech.LoadConfigFromEnv()
	var voiceService *speech.VoiceService
	var cleaner interview.AudioCleaner
	var cache *speech.SynthesisCache
	if voiceConfig.Enabled {
		provider := speech.NewOpenAICompatibleProvider(voiceConfig.TTS, voiceConfig.ASR)
		audioStore := speech.NewAudioStore(voiceConfig.CacheDir)
		cache = speech.NewSynthesisCache(provider, audioStore)
		voiceService = speech.NewVoiceService(provider, provider, audioStore, cache)
		cleaner = audioStore
		config.VoiceService = voiceService
		config.VoiceEnabled = true
		logger.WithFields(logrus.Fields{
			"tts_base_url": voiceConfig.TTS.BaseURL,
			"asr_base_url": voiceConfig.ASR.BaseURL,
			"cache_dir":    voiceConfig.CacheDir,
		}).Info("voice features enabled")
	} else {
		logger.Warn("voice features disabled (set VOICE_ENABLED=true to enable)")
	}
```

并把 `cleaner` 传给 `buildInterviewService` → `interview.NewService(..., cleaner)`。`buildInterviewService` 的签名需要加 `cleaner interview.AudioCleaner` 参数（以及 Task 8 需要的 `cache`）。

- [ ] **Step 7: 运行测试确认通过**

Run: `cd backend && go build ./... && go test ./... -v`
Expected: PASS —— 全部通过

- [ ] **Step 8: 提交**

```bash
cd backend && gofmt -w internal/interview/ cmd/server/ && go test ./... && cd ..
git add backend/internal/interview/ backend/cmd/server/main.go
git commit -m "feat(interview): 面试结束清理音频并接入语音服务"
```

### Task 8: 预合成触发（创建面试 + 追问）

**Files:**
- Modify: `backend/internal/interview/service.go`
- Modify: `backend/internal/interview/graph.go`
- Modify: `backend/internal/speech/cache.go`
- Modify: `backend/cmd/server/main.go`
- Test: `backend/internal/interview/service_test.go`（追加预合成用例）

**Interfaces:**
- Consumes: Task 5 的 `SynthesisCache.Prewarm`
- Produces:
  - `type PrewarmQuestion struct { ID int64; Text string }`（定义在 `backend/internal/interview` 包，与 `speech.PrewarmQuestion` 同名但不同包，各自独立）
  - `type AudioPrewarmer interface { PrewarmQuestions(ctx context.Context, tenantID, interviewID int64, questions []PrewarmQuestion) error }`
  - `func NewService(repo *Repository, store *RedisStore, gen *QuestionGenerator, graph *Graph, cleaner AudioCleaner, prewarmer AudioPrewarmer) *Service`（在 Task 7 的 5 参数基础上再加第 6 个）
  - `func (c *SynthesisCache) PrewarmQuestions(ctx context.Context, tenantID, interviewID int64, questions []PrewarmQuestion) error`（speech 侧适配方法）

- [ ] **Step 1: 先读现有代码**

Read:
- `backend/internal/interview/service.go` 的 `CreateInterview`（看 `MainQuestionInput` 是否含 id、`CreateInterviewWithQuestions` 是否返回题目 id）
- `backend/internal/interview/repository.go` 的 `MainQuestions`（已确认存在，返回带 id 的题目）
- `backend/internal/interview/graph.go` 里 `InsertFollowUp` 的调用点（追问产生处）
- `backend/internal/interview/service_test.go` 顶部辅助函数

**关键：** `MainQuestionInput` 当前**没有 ID 字段**（题面落库前拿不到 id）。因此预合成的题目列表必须用 `repo.MainQuestions(ctx, interviewID)` 取 —— 该方法已存在，返回带真实 id 的题目。

- [ ] **Step 2: 写失败的预合成测试**

在 `backend/internal/interview/service_test.go` 追加：

```go
type recordingPrewarmer struct {
	mu       sync.Mutex
	requests [][]PrewarmQuestion
}

func (p *recordingPrewarmer) PrewarmQuestions(ctx context.Context, tenantID, interviewID int64, questions []PrewarmQuestion) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.requests = append(p.requests, questions)
	return nil
}

func (p *recordingPrewarmer) total() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	total := 0
	for _, batch := range p.requests {
		total += len(batch)
	}
	return total
}
```

用例：创建面试后，prewarmer 收到与主问题数一致的一批，且**预合成失败不影响创建成功**（用返回 error 的 prewarmer，断言 `CreateInterview` 仍返回 nil error）。

- [ ] **Step 3: 运行测试确认失败**

Run: `cd backend && go test ./internal/interview/... -run 'Prewarm' -v`
Expected: FAIL —— prewarmer 未被调用

- [ ] **Step 4: 实现 interview 侧触发**

在 `backend/internal/interview/service.go`：

4a. 加接口与类型：

```go
// AudioPrewarmer 触发题目语音预合成（可为 nil）。
type AudioPrewarmer interface {
	PrewarmQuestions(ctx context.Context, tenantID, interviewID int64, questions []PrewarmQuestion) error
}

// PrewarmQuestion 是预合成队列里的一道题。
type PrewarmQuestion struct {
	ID   int64
	Text string
}
```

4b. `Service` 加 `prewarmer AudioPrewarmer` 字段，构造函数改为：

```go
func NewService(repo *Repository, store *RedisStore, gen *QuestionGenerator, graph *Graph, cleaner AudioCleaner, prewarmer AudioPrewarmer) *Service {
	return &Service{repo: repo, store: store, gen: gen, graph: graph, cleaner: cleaner, prewarmer: prewarmer}
}
```

4c. 在 `CreateInterview` 成功返回前触发（**后台 goroutine，不阻塞创建**）：

```go
	if s.prewarmer != nil {
		go s.prewarmQuestions(context.WithoutCancel(ctx), tenantID, iv.ID)
	}
```

4d. 新增内部方法（用 `repo.MainQuestions` 取真实题目 id）：

```go
// prewarmQuestions 后台预合成题目语音。
//
// 逐题串行：上游推理服务是单 worker，串行能让用户点喇叭最多等一题。
// 失败只记日志，不影响面试创建。
func (s *Service) prewarmQuestions(ctx context.Context, tenantID, interviewID int64) {
	rows, err := s.repo.MainQuestions(ctx, interviewID)
	if err != nil {
		log.Warnf("interview: load questions for prewarm %d: %v", interviewID, err)
		return
	}
	questions := make([]PrewarmQuestion, 0, len(rows))
	for _, row := range rows {
		questions = append(questions, PrewarmQuestion{ID: row.ID, Text: row.Question})
	}
	if err := s.prewarmer.PrewarmQuestions(ctx, tenantID, interviewID, questions); err != nil {
		log.Warnf("interview: prewarm audio for %d: %v", interviewID, err)
	}
}
```

4e. 追问的预合成：在 `backend/internal/interview/graph.go` 里 `InsertFollowUp` 之后触发单题预合成（后台 goroutine，题面文本 = 追问文本）。

- [ ] **Step 5: speech 侧适配方法**

在 `backend/internal/speech/cache.go` 追加：

```go
// PrewarmQuestions 逐题串行预合成题目语音（interview 层窄接口的适配）。
func (c *SynthesisCache) PrewarmQuestions(ctx context.Context, tenantID, interviewID int64, questions []PrewarmQuestion) error {
	converted := make([]PrewarmQuestion, 0, len(questions))
	for _, question := range questions {
		converted = append(converted, PrewarmQuestion{ID: question.ID, Text: question.Text})
	}
	return c.Prewarm(ctx, tenantID, interviewID, converted)
}
```

- [ ] **Step 6: main.go 补接线**

把 Task 7 创建的 `cache`（`*speech.SynthesisCache`）作为 prewarmer 传给 `interview.NewService`：

```go
	interview.NewService(repo, redisStore, generator, graph, cleaner, cache)
```

（`cache` 需在 `buildInterviewService` 作用域内可见 —— 若它在 `buildRouterConfig` 里创建，作为参数传进 `buildInterviewService`。）

- [ ] **Step 7: 运行测试确认通过**

Run: `cd backend && go build ./... && go test ./... -v`
Expected: PASS —— 全部通过

- [ ] **Step 8: 提交**

```bash
cd backend && gofmt -w internal/interview/ internal/speech/ cmd/server/ && go test ./... && cd ..
git add backend/internal/interview/ backend/internal/speech/cache.go backend/cmd/server/main.go
git commit -m "feat(interview): 面试创建后后台预合成题目语音"
```

---

### Task 9: 前端 WAV 编码工具

**Files:**
- Create: `frontend/src/utils/wav.ts`
- Test: `frontend/src/utils/wav.test.ts`

**Interfaces:**
- Consumes: 无
- Produces:
  - `export function encodeWav(samples: Float32Array, sampleRate: number): ArrayBuffer`
  - `export function resampleToMono16k(channels: Float32Array[], sourceRate: number): Float32Array`
  - `export async function decodeToMono16kWav(blob: Blob): Promise<ArrayBuffer>`

- [ ] **Step 1: 写失败的 wav 测试**

创建 `frontend/src/utils/wav.test.ts`：

```ts
import { describe, expect, it } from 'vitest';

import { encodeWav, resampleToMono16k } from './wav';

describe('encodeWav', () => {
  it('writes a RIFF/WAVE header with mono 16-bit PCM', () => {
    const samples = new Float32Array([0, 0.5, -0.5, 1]);
    const buffer = encodeWav(samples, 16000);
    const view = new DataView(buffer);

    expect(new TextDecoder().decode(new Uint8Array(buffer, 0, 4))).toBe('RIFF');
    expect(new TextDecoder().decode(new Uint8Array(buffer, 8, 4))).toBe('WAVE');
    expect(view.getUint16(22, true)).toBe(1); // channels
    expect(view.getUint32(24, true)).toBe(16000); // sample rate
    expect(view.getUint16(34, true)).toBe(16); // bits per sample
    expect(view.getUint32(40, true)).toBe(samples.length * 2); // data size
  });

  it('clamps samples outside the -1..1 range', () => {
    const buffer = encodeWav(new Float32Array([2, -2]), 16000);
    const view = new DataView(buffer);
    expect(view.getInt16(44, true)).toBe(32767);
    expect(view.getInt16(46, true)).toBe(-32768);
  });

  it('encodes an empty buffer without throwing', () => {
    expect(() => encodeWav(new Float32Array(0), 16000)).not.toThrow();
  });
});

describe('resampleToMono16k', () => {
  it('downmixes channels to mono', () => {
    const left = new Float32Array([1, 1, 1, 1]);
    const right = new Float32Array([0, 0, 0, 0]);
    const mono = resampleToMono16k([left, right], 16000);
    expect(mono.length).toBe(4);
    expect(mono[0]).toBeCloseTo(0.5, 5);
  });

  it('resamples 48kHz input down to 16kHz', () => {
    const input = new Float32Array(480);
    const output = resampleToMono16k([input], 48000);
    expect(output.length).toBe(160);
  });

  it('passes 16kHz input through unchanged', () => {
    const input = new Float32Array(32);
    const output = resampleToMono16k([input], 16000);
    expect(output.length).toBe(32);
  });
});
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd frontend && npx vitest run src/utils/wav.test.ts`
Expected: FAIL —— `Cannot find module './wav'`

- [ ] **Step 3: 实现 wav.ts**

创建 `frontend/src/utils/wav.ts`：

```ts
/** 录音处理工具：把浏览器录到的音频转成 16kHz 单声道 16bit PCM WAV。
 *
 * 后端不做任何转码（服务器上没有 ffmpeg），因此格式转换全部在浏览器完成：
 * MediaRecorder 的 webm/opus 或 mp4/aac → 解码 → 重采样 → 封装 WAV。
 */

/** 目标采样率：ASR 模型要求 16kHz。 */
const TARGET_SAMPLE_RATE = 16000;

/** 把 Float32 采样编码成 16bit PCM WAV。 */
export function encodeWav(samples: Float32Array, sampleRate: number): ArrayBuffer {
  const bytesPerSample = 2;
  const dataSize = samples.length * bytesPerSample;
  const buffer = new ArrayBuffer(44 + dataSize);
  const view = new DataView(buffer);

  const writeText = (offset: number, text: string) => {
    for (let index = 0; index < text.length; index += 1) {
      view.setUint8(offset + index, text.charCodeAt(index));
    }
  };

  writeText(0, 'RIFF');
  view.setUint32(4, 36 + dataSize, true);
  writeText(8, 'WAVE');
  writeText(12, 'fmt ');
  view.setUint32(16, 16, true); // fmt chunk size
  view.setUint16(20, 1, true); // PCM
  view.setUint16(22, 1, true); // mono
  view.setUint32(24, sampleRate, true);
  view.setUint32(28, sampleRate * bytesPerSample, true); // byte rate
  view.setUint16(32, bytesPerSample, true); // block align
  view.setUint16(34, 16, true); // bits per sample
  writeText(36, 'data');
  view.setUint32(40, dataSize, true);

  let offset = 44;
  for (let index = 0; index < samples.length; index += 1) {
    const clamped = Math.max(-1, Math.min(1, samples[index]));
    view.setInt16(offset, clamped < 0 ? clamped * 0x8000 : clamped * 0x7fff, true);
    offset += bytesPerSample;
  }
  return buffer;
}

/** 多声道下混为单声道，并重采样到 16kHz。 */
export function resampleToMono16k(channels: Float32Array[], sourceRate: number): Float32Array {
  const mono = new Float32Array(channels[0]?.length ?? 0);
  for (let index = 0; index < mono.length; index += 1) {
    let sum = 0;
    for (const channel of channels) {
      sum += channel[index] ?? 0;
    }
    mono[index] = sum / channels.length;
  }
  if (sourceRate === TARGET_SAMPLE_RATE) {
    return mono;
  }
  const ratio = sourceRate / TARGET_SAMPLE_RATE;
  const output = new Float32Array(Math.round(mono.length / ratio));
  for (let index = 0; index < output.length; index += 1) {
    const source = index * ratio;
    const lower = Math.floor(source);
    const upper = Math.min(lower + 1, mono.length - 1);
    const weight = source - lower;
    output[index] = mono[lower] * (1 - weight) + mono[upper] * weight;
  }
  return output;
}

/** 解码浏览器录音 Blob 并转成 16kHz 单声道 WAV。 */
export async function decodeToMono16kWav(blob: Blob): Promise<ArrayBuffer> {
  const arrayBuffer = await blob.arrayBuffer();
  const audioContext = new AudioContext();
  try {
    const decoded = await audioContext.decodeAudioData(arrayBuffer);
    const channels: Float32Array[] = [];
    for (let index = 0; index < decoded.numberOfChannels; index += 1) {
      channels.push(decoded.getChannelData(index));
    }
    const mono = resampleToMono16k(channels, decoded.sampleRate);
    return encodeWav(mono, TARGET_SAMPLE_RATE);
  } finally {
    await audioContext.close();
  }
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `cd frontend && npx vitest run src/utils/wav.test.ts`
Expected: PASS —— 三个 describe 全部通过

- [ ] **Step 5: 提交**

```bash
git add frontend/src/utils/wav.ts frontend/src/utils/wav.test.ts
git commit -m "feat(voice): 新增浏览器端 WAV 编码与重采样工具"
```

### Task 10: 前端语音 API 层与录音 hook

**Files:**
- Create: `frontend/src/api/voice.ts`
- Create: `frontend/src/hooks/useVoiceRecorder.ts`
- Test: `frontend/src/api/voice.test.ts`
- Test: `frontend/src/hooks/useVoiceRecorder.test.tsx`

**Interfaces:**
- Consumes: Task 9 的 `decodeToMono16kWav`
- Produces:
  - `frontend/src/api/voice.ts`：
    - `export type AudioState = 'ready' | 'recorded' | null`
    - `export interface VoiceTranscriptionResult { text: string }`
    - `export function getQuestionAudio(interviewId: number, questionId: number): Promise<Blob>`
    - `export function getQuestionRecording(interviewId: number, questionId: number): Promise<Blob>`
    - `export function transcribeRecording(interviewId: number, questionId: number, wav: ArrayBuffer): Promise<VoiceTranscriptionResult>`
  - `frontend/src/hooks/useVoiceRecorder.ts`：
    - `export type RecorderStatus = 'idle' | 'recording' | 'transcribing'`
    - `export interface UseVoiceRecorderResult { status: RecorderStatus; elapsedSeconds: number; error: string; start: () => Promise<void>; stop: () => Promise<string | null> }`
    - `export function useVoiceRecorder(options: { interviewId: number; questionId: number | null; onTranscript: (text: string) => void }): UseVoiceRecorderResult`

- [ ] **Step 1: 写失败的 API 层测试**

创建 `frontend/src/api/voice.test.ts`：

```ts
import { afterEach, describe, expect, it, vi } from 'vitest';

import { getQuestionAudio, getQuestionRecording, transcribeRecording } from './voice';

afterEach(() => vi.unstubAllGlobals());

describe('voice API', () => {
  it('fetches question audio as a blob', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(new Blob(['wav']), { status: 200 }));
    vi.stubGlobal('fetch', fetchMock);

    const blob = await getQuestionAudio(3, 12);

    expect(await blob.text()).toBe('wav');
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/interviews/3/questions/12/audio',
      expect.objectContaining({ credentials: 'include' }),
    );
  });

  it('uploads the recording as multipart form data', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ text: '识别文本' }), { status: 200 }),
    );
    vi.stubGlobal('fetch', fetchMock);

    const result = await transcribeRecording(3, 12, new ArrayBuffer(8));

    expect(result.text).toBe('识别文本');
    const [, init] = fetchMock.mock.calls[0];
    expect(init.method).toBe('POST');
    expect(init.body).toBeInstanceOf(FormData);
    expect((init.body as FormData).get('file')).toBeInstanceOf(Blob);
    // multipart 不能手工设置 Content-Type，交给浏览器带 boundary。
    expect(init.headers).toBeUndefined();
  });

  it('propagates API errors', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ error: '语音服务不可用' }), { status: 503 }),
    ));
    await expect(getQuestionRecording(3, 12)).rejects.toThrow('语音服务不可用');
  });
});
```

- [ ] **Step 2: 写失败的 hook 测试**

创建 `frontend/src/hooks/useVoiceRecorder.test.tsx`。jsdom 没有 MediaRecorder / AudioContext，需要 mock：

```ts
import { act, renderHook, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { useVoiceRecorder } from './useVoiceRecorder';

const transcribeMock = vi.fn();

vi.mock('../api/voice', () => ({
  transcribeRecording: (...args: unknown[]) => transcribeMock(...args),
}));

class FakeMediaRecorder {
  static instances: FakeMediaRecorder[] = [];
  ondataavailable: ((event: { data: Blob }) => void) | null = null;
  onstop: (() => void) | null = null;
  mimeType = 'audio/webm';
  state = 'inactive';

  constructor() {
    FakeMediaRecorder.instances.push(this);
  }

  start() {
    this.state = 'recording';
  }

  stop() {
    this.state = 'inactive';
    this.ondataavailable?.({ data: new Blob(['audio']) });
    this.onstop?.();
  }
}

afterEach(() => {
  vi.unstubAllGlobals();
  transcribeMock.mockReset();
  FakeMediaRecorder.instances = [];
});

describe('useVoiceRecorder', () => {
  it('records, transcribes and reports the transcript', async () => {
    vi.stubGlobal('MediaRecorder', FakeMediaRecorder);
    transcribeMock.mockResolvedValue({ text: '识别文本' });
    const onTranscript = vi.fn();

    const { result } = renderHook(() =>
      useVoiceRecorder({ interviewId: 3, questionId: 12, onTranscript }),
    );

    await act(async () => {
      await result.current.start();
    });
    expect(result.current.status).toBe('recording');

    await act(async () => {
      await result.current.stop();
    });

    await waitFor(() => expect(result.current.status).toBe('idle'));
    expect(onTranscript).toHaveBeenCalledWith('识别文本');
  });

  it('surfaces a transcription failure without throwing', async () => {
    vi.stubGlobal('MediaRecorder', FakeMediaRecorder);
    transcribeMock.mockRejectedValue(new Error('识别失败'));

    const { result } = renderHook(() =>
      useVoiceRecorder({ interviewId: 3, questionId: 12, onTranscript: vi.fn() }),
    );

    await act(async () => {
      await result.current.start();
    });
    await act(async () => {
      await result.current.stop();
    });

    await waitFor(() => expect(result.current.error).toBe('识别失败，可继续打字'));
    expect(result.current.status).toBe('idle');
  });
});
```

**注意：** 上面的 hook 测试依赖 `decodeToMono16kWav`（需要 `AudioContext`）。jsdom 无 `AudioContext` —— 要么在测试里 stub `globalThis.AudioContext`（返回一个 `decodeAudioData` 解析出固定采样的假对象），要么在 `vi.mock('../utils/wav')` 里把它 mock 掉。**用 mock 更稳**：```ts
vi.mock('../utils/wav', () => ({
  decodeToMono16kWav: vi.fn().mockResolvedValue(new ArrayBuffer(44)),
}));
```

- [ ] **Step 3: 运行测试确认失败**

Run: `cd frontend && npx vitest run src/api/voice.test.ts src/hooks/useVoiceRecorder.test.tsx`
Expected: FAIL —— 模块不存在

- [ ] **Step 4: 实现 api/voice.ts**

创建 `frontend/src/api/voice.ts`：

```ts
import { ApiError } from './client';

/** 题目的音频状态：ready=题目语音就绪，recorded=用户录音已保存，null=无音频。 */
export type AudioState = 'ready' | 'recorded' | null;

export interface VoiceTranscriptionResult {
  text: string;
}

async function fetchBlob(path: string): Promise<Blob> {
  const response = await fetch(path, { credentials: 'include' });
  if (!response.ok) {
    const body = await response.json().catch(() => ({}));
    throw new ApiError(body.error ?? '请求失败，请稍后重试。', response.status);
  }
  return response.blob();
}

/** 取某题的题目语音。 */
export function getQuestionAudio(interviewId: number, questionId: number): Promise<Blob> {
  return fetchBlob(`/api/interviews/${interviewId}/questions/${questionId}/audio`);
}

/** 取某题的用户录音。 */
export function getQuestionRecording(interviewId: number, questionId: number): Promise<Blob> {
  return fetchBlob(`/api/interviews/${interviewId}/questions/${questionId}/recording`);
}

/** 上传录音并转写为文本。
 *
 * multipart 上传不能走 apiFetch（它固定 Content-Type: application/json），
 * 因此这里单独实现 fetch；Content-Type 交给浏览器带 boundary。
 */
export async function transcribeRecording(
  interviewId: number,
  questionId: number,
  wav: ArrayBuffer,
): Promise<VoiceTranscriptionResult> {
  const form = new FormData();
  form.append('file', new Blob([wav], { type: 'audio/wav' }), 'recording.wav');
  const response = await fetch(
    `/api/interviews/${interviewId}/questions/${questionId}/voice/transcribe`,
    { method: 'POST', body: form, credentials: 'include' },
  );
  if (!response.ok) {
    const body = await response.json().catch(() => ({}));
    throw new ApiError(body.error ?? '识别失败，可继续打字。', response.status);
  }
  return (await response.json()) as VoiceTranscriptionResult;
}
```

- [ ] **Step 5: 实现 useVoiceRecorder.ts**

创建 `frontend/src/hooks/useVoiceRecorder.ts`：

```ts
import { useCallback, useEffect, useRef, useState } from 'react';

import { transcribeRecording } from '../api/voice';
import { decodeToMono16kWav } from '../utils/wav';

export type RecorderStatus = 'idle' | 'recording' | 'transcribing';

export interface UseVoiceRecorderOptions {
  interviewId: number;
  questionId: number | null;
  onTranscript: (text: string) => void;
}

export interface UseVoiceRecorderResult {
  status: RecorderStatus;
  elapsedSeconds: number;
  error: string;
  start: () => Promise<void>;
  stop: () => Promise<string | null>;
}

/** 录音 → 16k 单声道 WAV → 上传转写。
 *
 * 状态机：idle → recording → transcribing → idle。
 * 转写文本通过 onTranscript 回填输入框，绝不自动提交。
 */
export function useVoiceRecorder(options: UseVoiceRecorderOptions): UseVoiceRecorderResult {
  const { interviewId, questionId, onTranscript } = options;
  const [status, setStatus] = useState<RecorderStatus>('idle');
  const [elapsedSeconds, setElapsedSeconds] = useState(0);
  const [error, setError] = useState('');

  const recorderRef = useRef<MediaRecorder | null>(null);
  const chunksRef = useRef<Blob[]>([]);
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const clearTimer = useCallback(() => {
    if (timerRef.current !== null) {
      clearInterval(timerRef.current);
      timerRef.current = null;
    }
  }, []);

  useEffect(() => clearTimer, [clearTimer]);

  const start = useCallback(async () => {
    if (status !== 'idle') return;
    setError('');
    if (!navigator.mediaDevices?.getUserMedia) {
      setError('当前浏览器不支持录音，可继续打字。');
      return;
    }
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      const recorder = new MediaRecorder(stream);
      chunksRef.current = [];
      recorder.ondataavailable = (event) => {
        if (event.data.size > 0) chunksRef.current.push(event.data);
      };
      recorder.onstop = () => {
        stream.getTracks().forEach((track) => track.stop());
      };
      recorder.start();
      recorderRef.current = recorder;
      setStatus('recording');
      setElapsedSeconds(0);
      timerRef.current = setInterval(() => {
        setElapsedSeconds((value) => value + 1);
      }, 1000);
    } catch {
      setError('麦克风权限被拒，可继续打字。');
    }
  }, [status]);

  const stop = useCallback(async (): Promise<string | null> => {
    const recorder = recorderRef.current;
    if (!recorder || status !== 'recording') return null;
    clearTimer();
    recorder.stop();
    recorderRef.current = null;
    setStatus('transcribing');

    try {
      const blob = new Blob(chunksRef.current, { type: recorder.mimeType || 'audio/webm' });
      chunksRef.current = [];
      if (questionId == null) {
        setStatus('idle');
        return null;
      }
      const wav = await decodeToMono16kWav(blob);
      const result = await transcribeRecording(interviewId, questionId, wav);
      setStatus('idle');
      if (!result.text) {
        setError('没听清，可重新录音或打字。');
        return null;
      }
      onTranscript(result.text);
      return result.text;
    } catch {
      setStatus('idle');
      setError('识别失败，可继续打字。');
      return null;
    }
  }, [clearTimer, interviewId, onTranscript, questionId, status]);

  return { status, elapsedSeconds, error, start, stop };
}
```

- [ ] **Step 6: 运行测试确认通过**

Run: `cd frontend && npx vitest run src/api/voice.test.ts src/hooks/useVoiceRecorder.test.tsx`
Expected: PASS —— 全部通过

- [ ] **Step 7: 提交**

```bash
git add frontend/src/api/voice.ts frontend/src/api/voice.test.ts frontend/src/hooks/useVoiceRecorder.ts frontend/src/hooks/useVoiceRecorder.test.tsx
git commit -m "feat(voice): 新增前端语音接口与录音 hook"
```

---

### Task 11: InterviewChat 集成语音交互

**Files:**
- Modify: `frontend/src/components/InterviewChat.tsx`
- Modify: `frontend/src/api/interviews.ts`（`InterviewQuestion` 加 `audioState`）
- Modify: `frontend/src/styles/global.css`
- Test: `frontend/src/components/InterviewChat.test.tsx`

**Interfaces:**
- Consumes: Task 10 的 `getQuestionAudio` / `getQuestionRecording` / `useVoiceRecorder` / `AudioState`
- Produces: `InterviewQuestion.audioState?: AudioState`

- [ ] **Step 1: 先读现有组件测试**

Read: `frontend/src/components/InterviewChat.test.tsx`（全文）与 `frontend/src/components/InterviewChat.tsx`（`renderTurn` 与 `interview-dock__actions` 两处）
记录：现有 `vi.mock` 的写法、`makeQuestion` 辅助函数、`renderTurn` 里题目头部的 JSX 结构、输入坞 actions 区的 JSX 结构。

- [ ] **Step 2: 写失败的组件测试**

在 `frontend/src/components/InterviewChat.test.tsx` 追加用例。加 `vi.mock('../api/voice')`：

```ts
const getQuestionAudio = vi.fn();
const getQuestionRecording = vi.fn();

vi.mock('../api/voice', () => ({
  getQuestionAudio: (...args: unknown[]) => getQuestionAudio(...args),
  getQuestionRecording: (...args: unknown[]) => getQuestionRecording(...args),
  transcribeRecording: vi.fn(),
}));
```

必须覆盖：

| 用例 | 期望 |
|---|---|
| `audioState='ready'` 时显示"播放题目语音"按钮 | 按钮存在 |
| `audioState=null` 时不显示播放按钮 | 按钮不存在 |
| `audioState='recorded'` 时显示"回放我的录音"按钮 | 按钮存在 |
| 点击播放按钮调用 `getQuestionAudio` | 被调用 |
| 语音未启用时不渲染麦克风按钮 | 麦克风按钮不存在 |

- [ ] **Step 3: 运行测试确认失败**

Run: `cd frontend && npx vitest run src/components/InterviewChat.test.tsx`
Expected: FAIL —— 按钮不存在

- [ ] **Step 4: 扩展题目类型**

在 `frontend/src/api/interviews.ts`：

```ts
import type { AudioState } from './voice';

// InterviewQuestion 内新增字段：
export interface InterviewQuestion {
  // ...既有字段...
  /** 该题的音频状态：ready=题目语音就绪，recorded=用户录音已保存，null=无音频。 */
  audioState?: AudioState;
}
```

**注意：** 后端 `questionDTO` 目前不输出 `audioState` —— Task 6 未包含 DTO 改动。在本任务里**同时**修改 `backend/internal/httpapi/interview_handler.go` 的 `questionDTO`，加上：

```go
	if q.AudioState != "" {
		dto["audioState"] = q.AudioState
	}
```

并在该文件里补一条对应测试。

- [ ] **Step 5: 在 InterviewChat 中集成**

在 `frontend/src/components/InterviewChat.tsx`：

5a. import：

```tsx
import { getQuestionAudio, getQuestionRecording } from '../api/voice';
import { useVoiceRecorder } from '../hooks/useVoiceRecorder';
```

5b. 状态与 hook：

```tsx
const [voiceEnabled, setVoiceEnabled] = useState(true);
const [playingQuestionId, setPlayingQuestionId] = useState<number | null>(null);
const [voiceError, setVoiceError] = useState('');

const handleTranscript = useCallback((text: string) => {
  setAnswer((previous) => (previous.trim() ? `${previous.trim()} ${text}` : text));
}, []);

const recorder = useVoiceRecorder({
  interviewId: interview.id,
  questionId: current?.id ?? null,
  onTranscript: handleTranscript,
});
```

5c. 播放函数（题目语音与用户录音共用，按状态选接口）：

```tsx
const playAudio = useCallback(async (questionId: number, source: 'audio' | 'recording') => {
  setVoiceError('');
  setPlayingQuestionId(questionId);
  try {
    const blob = source === 'recording'
      ? await getQuestionRecording(interview.id, questionId)
      : await getQuestionAudio(interview.id, questionId);
    const url = URL.createObjectURL(blob);
    const audio = new Audio(url);
    audio.addEventListener('ended', () => {
      URL.revokeObjectURL(url);
      setPlayingQuestionId(null);
    });
    await audio.play();
  } catch {
    setVoiceError('语音播放失败，可重试。');
    setPlayingQuestionId(null);
  }
}, [interview.id]);
```

5d. **两个独立按钮**（按设计文档语义）：

- 题目头部（`renderTurn` 内，`interview-msg__no` 之后）：**朗读**按钮，条件 `question.audioState === 'ready'`
- 「我的回答」气泡旁（`renderQuestionBody` 内）：**回放**按钮，条件 `question.audioState === 'recorded'`

题目头部按钮：

```tsx
{voiceEnabled && question.audioState === 'ready' ? (
  <button
    aria-label="播放题目语音"
    className="interview-msg__audio"
    disabled={playingQuestionId !== null}
    onClick={() => void playAudio(question.id, 'audio')}
    title="播放题目语音"
    type="button"
  >
    {playingQuestionId === question.id ? '播放中…' : '朗读'}
  </button>
) : null}
```

回答气泡旁的回放按钮（`renderQuestionBody` 内，`interview-bubble__text` 之后）：

```tsx
{voiceEnabled && question.audioState === 'recorded' ? (
  <button
    aria-label="回放我的录音"
    className="interview-msg__audio"
    disabled={playingQuestionId !== null}
    onClick={() => void playAudio(question.id, 'recording')}
    title="回放我的录音"
    type="button"
  >
    {playingQuestionId === question.id ? '播放中…' : '回放录音'}
  </button>
) : null}
```

5e. 输入坞麦克风按钮（`interview-dock__actions` 内）：

```tsx
{voiceEnabled ? (
  <button
    aria-label={recorder.status === 'recording' ? '停止录音' : '开始录音'}
    className="interview-dock__mic"
    disabled={recorder.status === 'transcribing' || current == null}
    onClick={() => {
      if (recorder.status === 'recording') {
        void recorder.stop();
      } else {
        void recorder.start();
      }
    }}
    title={recorder.status === 'recording' ? '停止录音' : '开始录音'}
    type="button"
  >
    {recorder.status === 'recording'
      ? `停止录音 ${recorder.elapsedSeconds}s`
      : recorder.status === 'transcribing'
        ? '识别中…'
        : '录音'}
  </button>
) : null}
```

5f. 错误提示行（`submitError` 附近）：

```tsx
{(voiceError || recorder.error) && (
  <p className="interview-chat__error" role="alert">{voiceError || recorder.error}</p>
)}
```

5g. `voiceEnabled` 的真实来源：后端 `GET /api/interviews/:id` 响应要带 `voice.enabled`。在 `frontend/src/api/interviews.ts` 的 `Interview` 接口加 `voice?: { enabled: boolean }`，并在 `backend/internal/httpapi/interview_handler.go` 的 `interviewDTO` 里加 `"voice": map[string]any{"enabled": <配置开关>}`。**具体如何把这个开关传到 handler 层，按现有 RouterConfig 模式实现**（例如 `RouterConfig.VoiceEnabled` 已有，handler 构造时传入即可）。`InterviewChat` 从 `interview.voice?.enabled ?? true` 取。

- [ ] **Step 6: 加样式**

在 `frontend/src/styles/global.css` 追加（沿用既有 `.interview-*` 变量）：

```css
.interview-msg__audio { background: #fff; border: 1px solid var(--int-line); border-radius: 999px; color: var(--int-ink); cursor: pointer; font: inherit; font-size: .68rem; padding: .15rem .55rem; }
.interview-msg__audio:hover:not(:disabled) { border-color: var(--int-accent); color: var(--int-accent); }
.interview-msg__audio:disabled { cursor: wait; opacity: .55; }
.interview-dock__mic { background: #fff; border: 1px solid #cbd5e1; border-radius: 999px; color: #42526a; cursor: pointer; font: inherit; font-size: .86rem; padding: .6rem .95rem; }
.interview-dock__mic:hover:not(:disabled) { border-color: var(--int-accent); color: var(--int-accent); }
.interview-dock__mic:disabled { cursor: wait; opacity: .55; }
```

- [ ] **Step 7: 运行测试确认通过**

Run: `cd frontend && npx vitest run src/components/InterviewChat.test.tsx`
Expected: PASS —— 全部通过

- [ ] **Step 8: 运行全量测试与构建**

Run: `cd frontend && npx vitest run && npm run build`
Expected: 全部测试通过；build 无 TypeScript 报错

同时跑后端：`cd backend && go test ./...`
Expected: PASS

- [ ] **Step 9: 提交**

```bash
git add frontend/src/components/InterviewChat.tsx frontend/src/components/InterviewChat.test.tsx frontend/src/api/interviews.ts frontend/src/styles/global.css backend/internal/httpapi/interview_handler.go
git commit -m "feat(interview): 面试页集成题目朗读与录音转写"
```

---

### Task 12: 端到端联调与文档

**Files:**
- Create: `tools/voice-server/README.md`
- Modify: `docs/plans/2026-11-05-voice-interview-design.md`
- Modify: `.env.example`
- Modify: `README.md`

**Interfaces:**
- Consumes: Task 1–11 全部产出
- Produces: 可运行的本地语音服务说明与配置样例

- [ ] **Step 1: 写 tools/voice-server/README.md**

```markdown
# 本地语音推理服务

面试语音能力（题目朗读 / 录音转写）依赖一个 OpenAI 兼容的推理服务。
本机（M 系列 Mac，无 CUDA）使用 mlx-audio 的内置 server。

## 启动

```bash
uv tool install --prerelease=allow mlx-audio
mlx_audio.server --host 127.0.0.1 --port 21100
```

首次调用会从 HuggingFace 下载模型（约 3.5GB）：

| 用途 | 模型 | 体积 |
| --- | --- | --- |
| ASR | `mlx-community/Qwen3-ASR-1.7B-8bit` | ~1.5 GB |
| TTS | `mlx-community/Qwen3-TTS-12Hz-0.6B-CustomVoice-8bit` | ~1.9 GB |

## 验证

```bash
curl -s http://127.0.0.1:21100/v1/models

# 合成一句中文
curl -s -X POST http://127.0.0.1:21100/v1/audio/speech \
  -H 'Content-Type: application/json' \
  -d '{"model":"mlx-community/Qwen3-TTS-12Hz-0.6B-CustomVoice-8bit","input":"你好","voice":"Vivian","response_format":"wav"}' \
  -o /tmp/tts.wav

# 转写一段 16k 单声道 WAV
curl -s -X POST http://127.0.0.1:21100/v1/audio/transcriptions \
  -F 'file=@/tmp/recording.wav' \
  -F 'model=mlx-community/Qwen3-ASR-1.7B-8bit' \
  -F 'response_format=json'
```

## 音色

CustomVoice 内置 9 个预置音色：Vivian、Serena、Uncle_Fu、Dylan、Eric、Ryan、Aiden、Ono_Anna、Sohee。
默认 Vivian（中文女声）。

## 与后端配合

后端通过 `VOICE_TTS_BASE_URL` / `VOICE_ASR_BASE_URL` 指向本服务。
两个地址可以相同，也可以分别指向不同服务（如本地出题 + 云端转写）。

## 注意事项

- 推理服务是单 worker 串行的：预合成与前台请求会排队。后端已按「前台最多等一题」设计。
- 端口 21100 避开常用段；换端口要同步改 `.env`。
```

- [ ] **Step 2: 更新 .env.example**

在 `.env.example` 末尾追加：

```bash
# Optional. AI 面试语音能力（题目朗读 / 录音转写）。
# 需要一个 OpenAI 兼容的音频服务；本机用 mlx-audio 内置 server（见 tools/voice-server/README.md）。
# TTS 与 ASR 的 base URL 各自独立，可分别指向本地或云端服务。
# VOICE_ENABLED=true
# VOICE_TTS_BASE_URL=http://127.0.0.1:21100/v1
# VOICE_TTS_API_KEY=
# VOICE_TTS_MODEL=mlx-community/Qwen3-TTS-12Hz-0.6B-CustomVoice-8bit
# VOICE_TTS_VOICE=Vivian
# VOICE_ASR_BASE_URL=http://127.0.0.1:21100/v1
# VOICE_ASR_API_KEY=
# VOICE_ASR_MODEL=mlx-community/Qwen3-ASR-1.7B-8bit
# VOICE_TIMEOUT=120s
# VOICE_CACHE_DIR=./uploads/voice
```

- [ ] **Step 3: 同步设计文档**

在 `docs/plans/2026-11-05-voice-interview-design.md` 的「后端接口」章节：

3a. 把转写路径改为实现采用的含 `:qid` 形式：

```
### `POST /api/interviews/:id/questions/:qid/voice/transcribe`
```

3b. 补一句说明：录音必须归属到具体题目才能落盘与回放，因此路径含 `:qid`。

3c. 在「题目 DTO 新增 `audioState`」小节补上 `"ready"` / `"recorded"` 两种取值的实际含义（朗读按钮 / 回放按钮），以及清空后为 `null`。

- [ ] **Step 4: 更新 README.md**

在 README 的「AI 模拟面试」小节补充一节，说明语音能力（题目朗读 / 录音转写 / 回放录音 / 音频在面试结束后清理），并指向 `tools/voice-server/README.md`。

- [ ] **Step 5: 端到端手测（必须做）**

按顺序执行并记录每一步的实际结果：

1. 启动推理服务：`mlx_audio.server --host 127.0.0.1 --port 21100`
2. 在项目根 `.env` 里设 `VOICE_ENABLED=true` 与两个 base URL
3. 启动后端：`cd backend && go run ./cmd/server`
4. 启动前端：`cd frontend && npm run dev`
5. 浏览器打开面试页 → 创建面试 → 确认：
   - 题目语音能播放（朗读按钮出现并可播）
   - 点麦克风能录音，停止后文字回填输入框
   - 「回放录音」按钮出现并可播
   - **重启后端**（Ctrl-C 后重新 `go run`）→ 重新进入面试页 → 朗读与回放按钮仍存在且可播
   - 答完题让面试结束 → 检查 `uploads/voice/{tenantID}/{interviewID}/` 目录已被删除
6. 把实际结果写进最终报告

- [ ] **Step 6: 提交**

```bash
git add tools/voice-server/README.md .env.example README.md docs/plans/2026-11-05-voice-interview-design.md
git commit -m "docs(voice): 补充本地语音服务说明与配置样例"
```

---

## 验收清单

全部任务完成后逐条确认：

- [ ] `cd backend && go test ./...` 全绿
- [ ] `cd frontend && npx vitest run` 全绿
- [ ] `cd frontend && npm run build` 无 TS 报错
- [ ] 本地 `mlx_audio.server` 起在 21100，`curl /v1/models` 有返回
- [ ] 面试中能听到题目语音
- [ ] 录音能转成文字并回填输入框
- [ ] 「回放录音」可播
- [ ] **后端重启后**面试页的朗读与回放按钮仍可播
- [ ] **面试结束后** `uploads/voice/{tenantID}/{interviewID}/` 目录消失
- [ ] 关闭 `VOICE_ENABLED` 后语音按钮消失、接口返回 503、面试主流程不受影响
- [ ] 切云端只改 env（TTS / ASR 各自独立），Go 代码零改动





