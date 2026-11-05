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
	"time"
)

const (
	// defaultTTSTimeout 是 TTS 请求的兜底超时：配置里没给时不能变成「永不超时」。
	defaultTTSTimeout = 120 * time.Second
	// defaultASRTimeout 是 ASR 请求的兜底超时。
	defaultASRTimeout = 60 * time.Second
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
	ttsTimeout := cfg.TTSTimeout
	if ttsTimeout <= 0 {
		ttsTimeout = defaultTTSTimeout
	}
	asrTimeout := cfg.ASRTimeout
	if asrTimeout <= 0 {
		asrTimeout = defaultASRTimeout
	}
	return &OpenAICompatibleProvider{
		ttsBaseURL: strings.TrimRight(cfg.TTSBaseURL, "/"),
		ttsAPIKey:  cfg.TTSAPIKey,
		ttsModel:   cfg.TTSModel,
		ttsVoice:   cfg.TTSVoice,
		ttsClient:  &http.Client{Timeout: ttsTimeout},
		asrBaseURL: strings.TrimRight(cfg.ASRBaseURL, "/"),
		asrAPIKey:  cfg.ASRAPIKey,
		asrModel:   cfg.ASRModel,
		asrClient:  &http.Client{Timeout: asrTimeout},
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
		return SynthesisResult{}, fmt.Errorf("speech: build tts request: %w", sanitizeRequestError(err, p.ttsBaseURL))
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if p.ttsAPIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.ttsAPIKey)
	}

	resp, err := p.ttsClient.Do(httpReq)
	if err != nil {
		return SynthesisResult{}, fmt.Errorf("speech: tts request failed: %w", sanitizeRequestError(err, p.ttsBaseURL))
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
	part, err := writer.CreateFormFile("file", audioFileName(req.ContentType))
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
		return TranscriptionResult{}, fmt.Errorf("speech: build asr request: %w", sanitizeRequestError(err, p.asrBaseURL))
	}
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())
	if p.asrAPIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.asrAPIKey)
	}

	resp, err := p.asrClient.Do(httpReq)
	if err != nil {
		return TranscriptionResult{}, fmt.Errorf("speech: asr request failed: %w", sanitizeRequestError(err, p.asrBaseURL))
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

// parseTranscription 解析 ASR 响应：标准 JSON {"text":"..."}。
//
// 不完全依赖上游的 Content-Type：有些部署会忽略 response_format=json，却仍用
// application/json 头返回 ndjson 体，因此这里先整体解析、失败再回退取首行 JSON。
// text 字段缺失或为空一律报错，避免把空串当成「转写成功」。
func parseTranscription(payload []byte, contentType string) (string, error) {
	firstLine := firstNonEmptyLine(payload)
	var candidates [][]byte
	if strings.Contains(strings.ToLower(contentType), "ndjson") {
		candidates = [][]byte{firstLine, payload}
	} else {
		candidates = [][]byte{payload, firstLine}
	}

	var parseErr error
	for _, candidate := range candidates {
		if len(candidate) == 0 {
			continue
		}
		text, ok, err := extractTranscriptionText(candidate)
		if err != nil {
			if parseErr == nil {
				parseErr = err
			}
			continue
		}
		if ok {
			return text, nil
		}
	}
	if parseErr != nil {
		return "", fmt.Errorf("speech: invalid transcription payload: %w", parseErr)
	}
	return "", errors.New("speech: transcription payload without text field")
}

// extractTranscriptionText 从一段 JSON 里取 text 字段。
// 返回 ok=false 表示这是合法 JSON 但没有非空的 text 字段。
func extractTranscriptionText(payload []byte) (string, bool, error) {
	var wrapper map[string]json.RawMessage
	if err := json.Unmarshal(payload, &wrapper); err != nil {
		return "", false, err
	}
	raw, ok := wrapper["text"]
	if !ok {
		return "", false, nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return "", false, nil
	}
	if strings.TrimSpace(text) == "" {
		return "", false, nil
	}
	return text, true, nil
}

// firstNonEmptyLine 取第一行非空内容（ndjson 兜底用）。
func firstNonEmptyLine(payload []byte) []byte {
	for _, line := range bytes.Split(payload, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) > 0 {
			return line
		}
	}
	return nil
}

// audioFileName 按音频 MIME 推导上传文件名；未知或空则按 wav 处理。
// 上游靠扩展名判断编码格式，固定 .wav 会把 webm 字节错标成 wav。
func audioFileName(contentType string) string {
	mediaType := strings.TrimSpace(strings.ToLower(contentType))
	if idx := strings.Index(mediaType, ";"); idx >= 0 {
		mediaType = strings.TrimSpace(mediaType[:idx])
	}
	switch mediaType {
	case "audio/webm":
		return "recording.webm"
	case "audio/ogg":
		return "recording.ogg"
	case "audio/mpeg", "audio/mp3":
		return "recording.mp3"
	case "audio/mp4", "audio/x-m4a", "audio/m4a":
		return "recording.m4a"
	default:
		return "recording.wav"
	}
}

// sanitizeRequestError 把 *url.Error 里的完整请求 URL 换成本服务的基础 URL（去掉 query）。
// 上游地址上若带了 api key 之类的 query 参数，原样拼进错误信息会进日志。
func sanitizeRequestError(err error, baseURL string) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		urlErr.URL = safeBaseURL(baseURL)
	}
	return err
}

// safeBaseURL 去掉 base URL 上的 query、fragment 与 userinfo（http://user:pass@host），
// 只保留可用于定位服务的信息，避免 api key / 账号密码随错误信息进日志。
//
// 这里手工裁剪而不是 url.Parse：base URL 本身可能就是非法的（例如含控制字符），
// 而它恰恰是要出现在错误信息里的那段内容，不能因为解析失败就整个丢掉。
func safeBaseURL(raw string) string {
	sanitized := raw
	if idx := strings.Index(sanitized, "#"); idx >= 0 {
		sanitized = sanitized[:idx]
	}
	if idx := strings.Index(sanitized, "?"); idx >= 0 {
		sanitized = sanitized[:idx]
	}
	if idx := strings.Index(sanitized, "//"); idx >= 0 {
		rest := sanitized[idx+2:]
		if at := strings.Index(rest, "@"); at >= 0 {
			sanitized = sanitized[:idx+2] + rest[at+1:]
		}
	}
	return sanitized
}

var (
	_ Synthesizer = (*OpenAICompatibleProvider)(nil)
	_ Transcriber = (*OpenAICompatibleProvider)(nil)
)
