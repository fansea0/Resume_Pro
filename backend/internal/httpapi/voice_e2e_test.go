package httpapi

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"

	"resumepro/backend/internal/auth"
	"resumepro/backend/internal/interview"
	"resumepro/backend/internal/resume"
	"resumepro/backend/internal/resumesvc"
	"resumepro/backend/internal/speech"
	"resumepro/backend/internal/store"
	"resumepro/backend/internal/targetjob"
	"resumepro/backend/internal/templates"
)

// 本文件是语音链路的端到端验证：除 LLM / TTS / ASR 三个外部依赖换成桩之外，
// 其余全部走真实组件——真实 HTTP 路由、真实 interview.Service、真实
// interview.Repository（文件型 SQLite）、真实 speech.Generator、真实
// speech.AudioStore（临时目录）、真实 speech.OpenAICompatibleProvider。
//
// 桩只负责「外部世界」：
//   - TTS：httptest 服务返回一段真实合法的 16kHz 单声道 PCM16 WAV；
//   - ASR：httptest 服务返回固定文本；
//   - LLM：复用既有的 stubInterviewModel（生题 / 评分）。
//
// 本文件不修改任何既有测试辅助，只新增自己的装配与等待辅助。

const (
	// e2eStubASRText 是桩 ASR 固定返回的识别结果。
	e2eStubASRText = "这是桩服务返回的识别结果"
	// e2eStubTTSModel / e2eStubASRModel 只是桩服务收到的模型名，后端不校验模型名。
	e2eStubTTSModel = "stub-tts"
	e2eStubASRModel = "stub-asr"
	// e2eAudioWaitTimeout 是等待后台生成完成的超时上限（桩服务是本机 HTTP，正常是毫秒级）。
	e2eAudioWaitTimeout = 20 * time.Second
)

// e2eStubWAV 生成一段合法的 16kHz 单声道 PCM16 WAV（440Hz 正弦，durationMS 毫秒）。
//
// 必须是真 WAV：生成器落盘前会用 speech.ValidateWAV 校验，
// 播放接口也会把字节按 audio/wav 直接下发给浏览器。
func e2eStubWAV(durationMS int) []byte {
	const sampleRate = 16000
	samples := sampleRate * durationMS / 1000
	dataSize := samples * 2
	buf := &bytes.Buffer{}
	_ = binary.Write(buf, binary.LittleEndian, []byte("RIFF"))
	_ = binary.Write(buf, binary.LittleEndian, uint32(36+dataSize))
	_ = binary.Write(buf, binary.LittleEndian, []byte("WAVE"))
	_ = binary.Write(buf, binary.LittleEndian, []byte("fmt "))
	_ = binary.Write(buf, binary.LittleEndian, uint32(16))
	_ = binary.Write(buf, binary.LittleEndian, uint16(1)) // PCM
	_ = binary.Write(buf, binary.LittleEndian, uint16(1)) // 单声道
	_ = binary.Write(buf, binary.LittleEndian, uint32(sampleRate))
	_ = binary.Write(buf, binary.LittleEndian, uint32(sampleRate*2))
	_ = binary.Write(buf, binary.LittleEndian, uint16(2))  // block align
	_ = binary.Write(buf, binary.LittleEndian, uint16(16)) // 16 bit
	_ = binary.Write(buf, binary.LittleEndian, []byte("data"))
	_ = binary.Write(buf, binary.LittleEndian, uint32(dataSize))
	for i := 0; i < samples; i++ {
		value := int16(3000 * math.Sin(2*math.Pi*440*float64(i)/sampleRate))
		_ = binary.Write(buf, binary.LittleEndian, value)
	}
	return buf.Bytes()
}

// e2eStubVoice 是桩 TTS/ASR 服务，附带调用计数用于断言「确实打过上游」。
type e2eStubVoice struct {
	server   *httptest.Server
	ttsCalls atomic.Int64
	asrCalls atomic.Int64
}

// e2eStartStubVoiceServer 起一个 OpenAI 兼容的桩服务：
// POST /v1/audio/speech 返回合法 WAV；POST /v1/audio/transcriptions 返回 {"text":...}。
func e2eStartStubVoiceServer(t *testing.T) *e2eStubVoice {
	t.Helper()
	stub := &e2eStubVoice{}
	stub.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/audio/speech":
			stub.ttsCalls.Add(1)
			// 请求体是 JSON，读掉它，避免长连接在下一个请求上错位。
			_, _ = io.Copy(io.Discard, r.Body)
			w.Header().Set("Content-Type", "audio/wav")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(e2eStubWAV(200))
		case "/v1/audio/transcriptions":
			stub.asrCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"text":"` + e2eStubASRText + `"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"unknown path"}`))
		}
	}))
	t.Cleanup(stub.server.Close)
	return stub
}

// voiceE2EEnv 是端到端用例的运行环境。
type voiceE2EEnv struct {
	router    *gin.Engine
	cookie    *http.Cookie
	db        *sqlx.DB
	repo      *interview.Repository
	store     *speech.AudioStore
	generator *speech.Generator
	stub      *e2eStubVoice
}

// setupVoiceE2E 装配端到端环境。
//
// voiceEnabled=true 时接入桩 TTS/ASR 与真实生成器（题目语音会真的在后台生成）；
// voiceEnabled=false 时整条语音链路缺席（VoiceService 为 nil、音频钩子为 nil）。
func setupVoiceE2E(t *testing.T, voiceEnabled bool) *voiceE2EEnv {
	t.Helper()
	// 文件型 SQLite（临时目录）而不是 :memory:：
	// 生成器 worker 与 HTTP 请求处理会并发访问数据库，:memory: 库按连接隔离，
	// 多连接下会落到不同的库上（与 internal/interview/repository_test.go 的单连接做法不同）。
	db, err := store.OpenSQLite(filepath.Join(t.TempDir(), "voice_e2e.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}

	authRepo := auth.NewRepository(db)
	resumeRepo := resume.NewRepository(db)
	templateRepo := templates.NewRepository(db)
	targetService := targetjob.NewService(targetjob.NewRepository(db))
	interviewRepo := interview.NewRepository(db)

	mr := miniredis.RunT(t)
	redisStore, err := interview.NewRedisStore(mr.Addr(), "", 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = redisStore.Close() })

	// LLM 用既有桩：生题出两道主问题、评分固定 B+ 且不追问。
	generator := interview.NewQuestionGenerator(&stubInterviewModel{payload: stubQuestionPayload}, 1)
	graph, err := interview.BuildGraph(interviewRepo, &stubInterviewModel{payload: stubGradePayload}, redisStore)
	if err != nil {
		t.Fatal(err)
	}

	env := &voiceE2EEnv{db: db, repo: interviewRepo}
	var (
		voiceService *speech.VoiceService
		audioHooks   interview.AudioHooks
	)
	if voiceEnabled {
		env.stub = e2eStartStubVoiceServer(t)
		cacheDir := t.TempDir()
		config := speech.Config{
			Enabled:     true,
			TTSBaseURL:  env.stub.server.URL + "/v1",
			ASRBaseURL:  env.stub.server.URL + "/v1",
			TTSModel:    e2eStubTTSModel,
			ASRModel:    e2eStubASRModel,
			TTSVoice:    "stub",
			MaxAttempts: 2,
			Backoff:     []time.Duration{10 * time.Millisecond},
			CacheDir:    cacheDir,
		}
		provider := speech.NewOpenAICompatibleProvider(config)
		env.store = speech.NewAudioStore(cacheDir)
		env.generator = speech.NewGenerator(provider, env.store, interviewRepo, config)
		env.generator.Start(context.Background())
		// 关闭生成器：避免 worker goroutine 泄漏影响后续用例。
		t.Cleanup(env.generator.Close)
		audioHooks = env.generator
		// Corrector 传 nil：本用例只验证 ASR 原文透传（错别字修复需要真实 LLM）。
		voiceService = speech.NewVoiceService(env.store, provider, nil)
	}

	interviewService := interview.NewService(interviewRepo, redisStore, generator, graph,
		interview.WithAudioHooks(audioHooks))

	env.router = NewRouter(authRepo, resumeRepo, templateRepo, RouterConfig{
		JWTSecret:        "voice-e2e-test-secret",
		TargetJobService: targetService,
		ResumesvcService: resumesvc.NewService(resumeRepo, nil, targetService),
		InterviewService: interviewService,
		VoiceService:     voiceService,
	})
	env.cookie = registerAuthUser(t, env.router, "voice-e2e@example.com")
	return env
}

// e2eQuestions 读取面试题目列表（含 audioState）。
func e2eQuestions(t *testing.T, env *voiceE2EEnv, interviewID int64) []map[string]any {
	t.Helper()
	response := requestJSON(t, env.router, http.MethodGet,
		"/api/interviews/"+jsonNumber(interviewID)+"/questions", "", env.cookie)
	if response.Code != http.StatusOK {
		t.Fatalf("questions: %d %s", response.Code, response.Body.String())
	}
	var rows []map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &rows); err != nil {
		t.Fatalf("decode questions: %v", err)
	}
	return rows
}

// e2eWaitForAudioState 轮询题目列表，直到出现期望的 audioState。
//
// 后台生成是异步的：用条件等待而不是固定 sleep 判定「已经发生」。
func e2eWaitForAudioState(t *testing.T, env *voiceE2EEnv, interviewID int64, want string, timeout time.Duration) []map[string]any {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last []map[string]any
	for {
		last = e2eQuestions(t, env, interviewID)
		for _, row := range last {
			if row["audioState"] == want {
				return last
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("audioState %q not reached within %s, last = %v", want, timeout, last)
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// e2eWaitFor 轮询直到条件成立（带超时）。
func e2eWaitFor(t *testing.T, timeout time.Duration, message string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if condition() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for %s", message)
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// e2eQuestionIDWithAudioState 从题目列表里挑出指定语音状态的题目 id。
func e2eQuestionIDWithAudioState(rows []map[string]any, state string) int64 {
	for _, row := range rows {
		if row["audioState"] != state {
			continue
		}
		if id, ok := row["id"].(float64); ok {
			return int64(id)
		}
	}
	return 0
}

// e2eGetAudio 请求题目语音。
func e2eGetAudio(t *testing.T, env *voiceE2EEnv, cookie *http.Cookie, interviewID, questionID int64) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet,
		"/api/interviews/"+jsonNumber(interviewID)+"/questions/"+jsonNumber(questionID)+"/audio", nil)
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	env.router.ServeHTTP(recorder, request)
	return recorder
}

// e2ePostAudio 上传录音到转写接口（multipart，字段名 file）。
func e2ePostAudio(t *testing.T, env *voiceE2EEnv, cookie *http.Cookie, interviewID, questionID int64, filename string, payload []byte) *httptest.ResponseRecorder {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost,
		"/api/interviews/"+jsonNumber(interviewID)+"/questions/"+jsonNumber(questionID)+"/transcribe", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	env.router.ServeHTTP(recorder, request)
	return recorder
}

// e2eAudioDir 返回一场面试的音频目录路径。
func e2eAudioDir(t *testing.T, env *voiceE2EEnv, interviewID int64) string {
	t.Helper()
	tenantID := interviewTenantID(t, env.db, interviewID)
	return filepath.Join(env.store.Root(), strconv.FormatInt(tenantID, 10), strconv.FormatInt(interviewID, 10))
}

// TestVoiceE2ECreateInterviewGeneratesAudioAndServesWAV 链路一：
// 建面试 → 后台生成 → audioState 变 ready → 播放接口返回合法 WAV。
func TestVoiceE2ECreateInterviewGeneratesAudioAndServesWAV(t *testing.T) {
	env := setupVoiceE2E(t, true)
	_, interviewID := createInterviewFixture(t, env.router, env.cookie, env.db)

	rows := e2eWaitForAudioState(t, env, interviewID, "ready", e2eAudioWaitTimeout)
	questionID := e2eQuestionIDWithAudioState(rows, "ready")
	if questionID == 0 {
		t.Fatal("no question reached audioState=ready")
	}
	if calls := env.stub.ttsCalls.Load(); calls == 0 {
		t.Error("stub tts was never called")
	}

	recorder := e2eGetAudio(t, env, env.cookie, interviewID, questionID)
	if recorder.Code != http.StatusOK {
		t.Fatalf("audio status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Type"); got != "audio/wav" {
		t.Errorf("content type = %q, want audio/wav", got)
	}
	if _, err := speech.ValidateWAV(recorder.Body.Bytes()); err != nil {
		t.Fatalf("downloaded body is not a valid wav: %v", err)
	}
}

// TestVoiceE2ETranscribeReturnsStubText 链路二：上传合法 WAV → 转写返回桩 ASR 的文本。
//
// Corrector 为 nil 时应当原样返回，不做任何加工。
func TestVoiceE2ETranscribeReturnsStubText(t *testing.T) {
	env := setupVoiceE2E(t, true)
	_, interviewID := createInterviewFixture(t, env.router, env.cookie, env.db)
	questionID := firstQuestionID(t, env.router, env.cookie, interviewID)

	recorder := e2ePostAudio(t, env, env.cookie, interviewID, questionID, "recording.wav", e2eStubWAV(200))
	if recorder.Code != http.StatusOK {
		t.Fatalf("transcribe status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode transcribe: %v", err)
	}
	if payload.Text != e2eStubASRText {
		t.Errorf("text = %q, want %q", payload.Text, e2eStubASRText)
	}
	if calls := env.stub.asrCalls.Load(); calls != 1 {
		t.Errorf("stub asr calls = %d, want 1", calls)
	}
}

// TestVoiceE2ETranscribeRejectsInvalidAudio 链路三：上传非 WAV 字节 → 400。
func TestVoiceE2ETranscribeRejectsInvalidAudio(t *testing.T) {
	env := setupVoiceE2E(t, true)
	_, interviewID := createInterviewFixture(t, env.router, env.cookie, env.db)
	questionID := firstQuestionID(t, env.router, env.cookie, interviewID)

	recorder := e2ePostAudio(t, env, env.cookie, interviewID, questionID, "recording.wav",
		[]byte("this is definitely not a riff wave payload"))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("transcribe status = %d, want 400: %s", recorder.Code, recorder.Body.String())
	}
	if calls := env.stub.asrCalls.Load(); calls != 0 {
		t.Errorf("stub asr calls = %d, want 0 (非法音频不应打到上游)", calls)
	}
}

// TestVoiceE2EAudioRejectsOtherTenant 链路四：跨租户访问 → 404（不泄露资源是否存在）。
func TestVoiceE2EAudioRejectsOtherTenant(t *testing.T) {
	env := setupVoiceE2E(t, true)
	_, interviewID := createInterviewFixture(t, env.router, env.cookie, env.db)
	questionID := firstQuestionID(t, env.router, env.cookie, interviewID)
	other := registerAuthUser(t, env.router, "voice-e2e-other@example.com")

	audio := e2eGetAudio(t, env, other, interviewID, questionID)
	if audio.Code != http.StatusNotFound {
		t.Fatalf("audio status = %d, want 404: %s", audio.Code, audio.Body.String())
	}
	transcribe := e2ePostAudio(t, env, other, interviewID, questionID, "recording.wav", e2eStubWAV(200))
	if transcribe.Code != http.StatusNotFound {
		t.Fatalf("transcribe status = %d, want 404: %s", transcribe.Code, transcribe.Body.String())
	}
}

// TestVoiceE2EDisabledVoiceKeepsRoutesAndReturns503 链路五：
// 语音未启用时两个接口都返回 503，且路由仍然注册（不是 404「路由不存在」）。
func TestVoiceE2EDisabledVoiceKeepsRoutesAndReturns503(t *testing.T) {
	env := setupVoiceE2E(t, false)

	registered := map[string]bool{}
	for _, route := range env.router.Routes() {
		registered[route.Method+" "+route.Path] = true
	}
	for _, want := range []string{
		"GET /api/interviews/:id/questions/:qid/audio",
		"POST /api/interviews/:id/questions/:qid/transcribe",
	} {
		if !registered[want] {
			t.Errorf("route %q is not registered", want)
		}
	}

	audio := e2eGetAudio(t, env, env.cookie, 1, 1)
	if audio.Code != http.StatusServiceUnavailable {
		t.Fatalf("audio status = %d, want 503: %s", audio.Code, audio.Body.String())
	}
	transcribe := e2ePostAudio(t, env, env.cookie, 1, 1, "recording.wav", e2eStubWAV(200))
	if transcribe.Code != http.StatusServiceUnavailable {
		t.Fatalf("transcribe status = %d, want 503: %s", transcribe.Code, transcribe.Body.String())
	}
}

// TestVoiceE2EFinishedInterviewCleansUpAudio 链路六：
// 答完全部题目（res.Finished）后，音频目录被删除、audioState 被清空。
func TestVoiceE2EFinishedInterviewCleansUpAudio(t *testing.T) {
	env := setupVoiceE2E(t, true)
	_, interviewID := createInterviewFixture(t, env.router, env.cookie, env.db)
	e2eWaitForAudioState(t, env, interviewID, "ready", e2eAudioWaitTimeout)

	dir := e2eAudioDir(t, env, interviewID)
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("audio dir should exist before the interview finishes: %v", err)
	}

	finished := false
	for attempt := 0; attempt < 5 && !finished; attempt++ {
		response := requestJSON(t, env.router, http.MethodPost,
			"/api/interviews/"+jsonNumber(interviewID)+"/answer", `{"answer":"我的回答内容"}`, env.cookie)
		if response.Code != http.StatusOK {
			t.Fatalf("answer: %d %s", response.Code, response.Body.String())
		}
		var result struct {
			Finished bool `json:"finished"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatalf("decode answer: %v", err)
		}
		finished = result.Finished
	}
	if !finished {
		t.Fatal("interview did not finish after answering every question")
	}

	e2eWaitFor(t, 5*time.Second, "interview audio directory to be removed", func() bool {
		_, err := os.Stat(dir)
		return os.IsNotExist(err)
	})
	for _, row := range e2eQuestions(t, env, interviewID) {
		if row["audioState"] != nil {
			t.Errorf("question %v audioState = %v, want null after the interview finished", row["id"], row["audioState"])
		}
	}
}

// TestVoiceE2EAbortedInterviewCleansUpAudio 链路六的另一个分支：
// Abort 之后音频同样被清理。
func TestVoiceE2EAbortedInterviewCleansUpAudio(t *testing.T) {
	env := setupVoiceE2E(t, true)
	_, interviewID := createInterviewFixture(t, env.router, env.cookie, env.db)
	e2eWaitForAudioState(t, env, interviewID, "ready", e2eAudioWaitTimeout)

	dir := e2eAudioDir(t, env, interviewID)
	response := requestJSON(t, env.router, http.MethodPost,
		"/api/interviews/"+jsonNumber(interviewID)+"/abort", "", env.cookie)
	if response.Code != http.StatusOK {
		t.Fatalf("abort: %d %s", response.Code, response.Body.String())
	}

	e2eWaitFor(t, 5*time.Second, "aborted interview audio directory to be removed", func() bool {
		_, err := os.Stat(dir)
		return os.IsNotExist(err)
	})
	for _, row := range e2eQuestions(t, env, interviewID) {
		if row["audioState"] != nil {
			t.Errorf("question %v audioState = %v, want null after abort", row["id"], row["audioState"])
		}
	}
}

// TestVoiceE2EMainFlowWorksWithoutVoiceHooks 链路七：
// 语音缺席（WithAudioHooks(nil)）时，面试主链路完全正常。
func TestVoiceE2EMainFlowWorksWithoutVoiceHooks(t *testing.T) {
	env := setupVoiceE2E(t, false)
	_, interviewID := createInterviewFixture(t, env.router, env.cookie, env.db)

	if enabled := interviewVoiceEnabled(t, env.router, env.cookie, interviewID); enabled {
		t.Error("voice.enabled = true, want false when the voice service is absent")
	}
	response := requestJSON(t, env.router, http.MethodPost,
		"/api/interviews/"+jsonNumber(interviewID)+"/answer", `{"answer":"我的回答内容"}`, env.cookie)
	if response.Code != http.StatusOK {
		t.Fatalf("answer: %d %s", response.Code, response.Body.String())
	}
	for _, row := range e2eQuestions(t, env, interviewID) {
		if row["audioState"] != nil {
			t.Errorf("question %v audioState = %v, want null when voice is absent", row["id"], row["audioState"])
		}
	}
}
