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
