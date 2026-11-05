package speech

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
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

// assertRequest 断言请求的方法与路径，避免端点写错时测试仍然全绿。
func assertRequest(t *testing.T, r *http.Request, method, path string) {
	t.Helper()
	if r.Method != method || r.URL.Path != path {
		t.Errorf("request = %s %s, want %s %s", r.Method, r.URL.Path, method, path)
	}
}

func TestSynthesizeForcesWavContentType(t *testing.T) {
	var gotFormat string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertRequest(t, r, http.MethodPost, "/audio/speech")
		var body struct {
			ResponseFormat string `json:"response_format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode json body: %v", err)
		}
		gotFormat = body.ResponseFormat
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

// TestSynthesizeSendsJSONBody 请求体必须是 JSON。
//
// 踩过的坑：mlx-audio 0.6.4 的 /v1/audio/speech 是 FastAPI 的 Pydantic 端点，
// 只接受 application/json；发 form-urlencoded 会拿到 422，TTS 全部失败。
// OpenAI 官方的同名端点用的也是 JSON body。
func TestSynthesizeSendsJSONBody(t *testing.T) {
	var gotContentType string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode json body: %v", err)
		}
		_, _ = w.Write([]byte("RIFFfake"))
	}))
	defer server.Close()

	cfg := testConfig(server.URL, server.URL)
	cfg.TTSModel = "tts-model"
	cfg.TTSVoice = "Vivian"
	provider := NewOpenAICompatibleProvider(cfg)
	if _, err := provider.Synthesize(context.Background(), SynthesisRequest{Text: "你好"}); err != nil {
		t.Fatalf("synthesize: %v", err)
	}

	if gotContentType != "application/json" {
		t.Errorf("content type = %q, want application/json", gotContentType)
	}
	if gotBody["model"] != "tts-model" {
		t.Errorf("model = %v, want tts-model", gotBody["model"])
	}
	if gotBody["input"] != "你好" {
		t.Errorf("input = %v, want 你好", gotBody["input"])
	}
	if gotBody["response_format"] != "wav" {
		t.Errorf("response_format = %v, want wav", gotBody["response_format"])
	}
	if gotBody["voice"] != "Vivian" {
		t.Errorf("voice = %v, want Vivian", gotBody["voice"])
	}
}

// TestSynthesizeOmitsEmptyVoice voice 为空时不应该传空字符串字段：
// 上游会把空 voice 当成「指定发音人」而不是「用默认」。
func TestSynthesizeOmitsEmptyVoice(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode json body: %v", err)
		}
		_, _ = w.Write([]byte("RIFFfake"))
	}))
	defer server.Close()

	cfg := testConfig(server.URL, server.URL)
	cfg.TTSVoice = ""
	provider := NewOpenAICompatibleProvider(cfg)
	if _, err := provider.Synthesize(context.Background(), SynthesisRequest{Text: "你好"}); err != nil {
		t.Fatalf("synthesize: %v", err)
	}
	if _, ok := gotBody["voice"]; ok {
		t.Errorf("voice should be omitted when empty, got %v", gotBody["voice"])
	}
}

func TestTranscribeSendsJSONResponseFormat(t *testing.T) {
	var gotFormat string
	var gotFileBytes int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertRequest(t, r, http.MethodPost, "/audio/transcriptions")
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
		assertRequest(t, r, http.MethodPost, "/audio/transcriptions")
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

// 上游忽略 response_format=json，却仍用 application/json 头返回 ndjson 体：
// 不能因此直接报错，应回退取首行。
func TestTranscribeHandlesNdjsonBodyWithJSONContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
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

// text/plain 头 + JSON 体是既有上游的常见返回，必须照常解析。
func TestTranscribeHandlesPlainTextContentTypeWithJSONBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(`{"text":"识别结果"}`))
	}))
	defer server.Close()

	provider := NewOpenAICompatibleProvider(testConfig(server.URL, server.URL))
	result, err := provider.Transcribe(context.Background(), TranscriptionRequest{Audio: []byte("RIFFdata")})
	if err != nil {
		t.Fatalf("transcribe: %v", err)
	}
	if result.Text != "识别结果" {
		t.Errorf("text = %q, want 识别结果", result.Text)
	}
}

// 没有 text 字段（含空串）必须报错，不能把空串当成转写成功。
func TestTranscribeRejectsPayloadWithoutText(t *testing.T) {
	cases := []struct {
		name        string
		contentType string
		body        string
	}{
		{name: "JSON 缺 text 字段", contentType: "application/json", body: `{"foo":1}`},
		{name: "JSON text 为空", contentType: "application/json", body: `{"text":""}`},
		{name: "ndjson 首行缺 text 字段", contentType: "application/x-ndjson", body: "{\"foo\":1}\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tc.contentType)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()

			provider := NewOpenAICompatibleProvider(testConfig(server.URL, server.URL))
			result, err := provider.Transcribe(context.Background(), TranscriptionRequest{Audio: []byte("RIFFdata")})
			if err == nil {
				t.Fatalf("expected error, got text %q", result.Text)
			}
			if !strings.Contains(err.Error(), "without text field") {
				t.Errorf("error = %v, want without text field error", err)
			}
		})
	}
}

// 上传文件的扩展名要跟着 ContentType 走，不能把 webm 字节标成 .wav。
func TestTranscribeUsesContentTypeFileExtension(t *testing.T) {
	cases := []struct {
		contentType string
		wantFile    string
	}{
		{contentType: "audio/webm", wantFile: "recording.webm"},
		{contentType: "audio/webm;codecs=opus", wantFile: "recording.webm"},
		{contentType: "audio/ogg", wantFile: "recording.ogg"},
		{contentType: "audio/mpeg", wantFile: "recording.mp3"},
		{contentType: "audio/mp4", wantFile: "recording.m4a"},
		{contentType: "audio/x-m4a", wantFile: "recording.m4a"},
		{contentType: "audio/m4a", wantFile: "recording.m4a"},
		{contentType: "audio/wav", wantFile: "recording.wav"},
		{contentType: "", wantFile: "recording.wav"},
		{contentType: "application/octet-stream", wantFile: "recording.wav"},
	}
	for _, tc := range cases {
		t.Run(tc.contentType, func(t *testing.T) {
			var gotFile string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := r.ParseMultipartForm(1 << 20); err != nil {
					t.Errorf("parse multipart: %v", err)
					return
				}
				file, header, err := r.FormFile("file")
				if err != nil {
					t.Errorf("form file: %v", err)
					return
				}
				defer func() { _ = file.Close() }()
				gotFile = header.Filename
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"text":"x"}`))
			}))
			defer server.Close()

			provider := NewOpenAICompatibleProvider(testConfig(server.URL, server.URL))
			_, err := provider.Transcribe(context.Background(), TranscriptionRequest{
				Audio:       []byte("RIFFdata"),
				ContentType: tc.contentType,
			})
			if err != nil {
				t.Fatalf("transcribe: %v", err)
			}
			if gotFile != tc.wantFile {
				t.Errorf("filename = %q, want %q", gotFile, tc.wantFile)
			}
		})
	}
}

// base URL 上带的 query 密钥不能进错误信息。
func TestRequestErrorDoesNotLeakBaseURLQuery(t *testing.T) {
	cfg := testConfig("http://127.0.0.1:1/v1?api_key=SUPERSECRET", "http://127.0.0.1:1/v1?api_key=SUPERSECRET")
	cfg.TTSTimeout = 2 * time.Second
	cfg.ASRTimeout = 2 * time.Second
	provider := NewOpenAICompatibleProvider(cfg)

	_, err := provider.Synthesize(context.Background(), SynthesisRequest{Text: "x"})
	if err == nil {
		t.Fatal("expected request error")
	}
	if strings.Contains(err.Error(), "SUPERSECRET") {
		t.Errorf("error leaks api key: %v", err)
	}
	if !strings.Contains(err.Error(), "speech: tts request failed") {
		t.Errorf("error = %v, want tts request failed error", err)
	}
}

// base URL 里带账号密码（userinfo）时，错误信息里不能出现凭据。
func TestRequestErrorDoesNotLeakBaseURLUserInfo(t *testing.T) {
	cfg := testConfig("http://user:SUPERSECRET@127.0.0.1:1/v1", "http://user:SUPERSECRET@127.0.0.1:1/v1")
	cfg.TTSTimeout = 2 * time.Second
	cfg.ASRTimeout = 2 * time.Second
	provider := NewOpenAICompatibleProvider(cfg)

	_, err := provider.Synthesize(context.Background(), SynthesisRequest{Text: "x"})
	if err == nil {
		t.Fatal("expected request error")
	}
	if strings.Contains(err.Error(), "SUPERSECRET") {
		t.Errorf("error leaks userinfo: %v", err)
	}

	_, err = provider.Transcribe(context.Background(), TranscriptionRequest{Audio: []byte("x")})
	if err == nil {
		t.Fatal("expected request error")
	}
	if strings.Contains(err.Error(), "SUPERSECRET") {
		t.Errorf("error leaks userinfo: %v", err)
	}
}

// base URL 非法（例如 query 里带控制字符）时，请求构造阶段的错误同样不能带出密钥。
func TestBuildRequestErrorDoesNotLeakBaseURLQuery(t *testing.T) {
	bad := "http://127.0.0.1:21100/v1?api_key=SUPERSECRET\u007f"
	cfg := testConfig(bad, bad)
	provider := NewOpenAICompatibleProvider(cfg)

	_, err := provider.Synthesize(context.Background(), SynthesisRequest{Text: "x"})
	if err == nil {
		t.Fatal("expected build error")
	}
	if !strings.Contains(err.Error(), "build tts request") {
		t.Errorf("error = %v, want build tts request error", err)
	}
	if strings.Contains(err.Error(), "SUPERSECRET") {
		t.Errorf("error leaks api key: %v", err)
	}

	_, err = provider.Transcribe(context.Background(), TranscriptionRequest{Audio: []byte("x")})
	if err == nil {
		t.Fatal("expected build error")
	}
	if !strings.Contains(err.Error(), "build asr request") {
		t.Errorf("error = %v, want build asr request error", err)
	}
	if strings.Contains(err.Error(), "SUPERSECRET") {
		t.Errorf("error leaks api key: %v", err)
	}
}

func TestProviderUsesSeparateBaseURLs(t *testing.T) {
	var ttsHits, asrHits atomic.Int64
	tts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertRequest(t, r, http.MethodPost, "/audio/speech")
		ttsHits.Add(1)
		_, _ = w.Write([]byte("RIFF"))
	}))
	defer tts.Close()
	asr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertRequest(t, r, http.MethodPost, "/audio/transcriptions")
		asrHits.Add(1)
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
	if ttsHits.Load() != 1 || asrHits.Load() != 1 {
		t.Errorf("hits: tts=%d asr=%d, want 1/1", ttsHits.Load(), asrHits.Load())
	}
}
