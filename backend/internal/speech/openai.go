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
