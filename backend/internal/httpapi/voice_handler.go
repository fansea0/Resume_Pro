package httpapi

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"resumepro/backend/internal/interview"
	"resumepro/backend/internal/speech"
)

// voiceHandler 是语音能力的 HTTP 层：播放题目语音、转写录音。
//
// voice 为 nil（语音未启用）时所有端点返回 503，而不是 404：
// 路由始终注册，调用方能明确区分「接口不存在」与「语音没开」。
type voiceHandler struct {
	service     *interview.Service
	store       *speech.AudioStore
	transcriber speech.Transcriber
	corrector   *speech.Corrector
}

// newVoiceHandler 组装语音 handler。voice 为 nil 时所有端点走 503 降级。
func newVoiceHandler(service *interview.Service, voice *speech.VoiceService) *voiceHandler {
	handler := &voiceHandler{service: service}
	if voice != nil {
		handler.store = voice.Store
		handler.transcriber = voice.Transcriber
		handler.corrector = voice.Corrector
	}
	return handler
}

// audio 返回题目语音（TTS 产物）。
//
// 只读已生成的文件，绝不在请求内同步合成：单题合成数秒到数十秒，
// 放进请求会让用户干等，还会与后台生成队列抢 GPU。
//
// 状态语义（与设计文档「播放题目语音」一致）：
//   - ready   → 200 audio/wav
//   - pending → 409 {"state":"pending"}（前端显示生成中，稍后重试）
//   - failed  → 503 {"state":"failed"}
//   - NULL    → 409 {"state":"pending"}（尚未入队，按生成中处理）
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
		handleInterviewError(ginCtx, err)
		return
	}
	state := ""
	if question.AudioState != nil {
		state = *question.AudioState
	}
	switch state {
	case "ready":
		audio, err := h.store.ReadQuestionAudio(principal.TenantID, interviewID, questionID)
		if err != nil {
			// 状态说就绪但文件不在：按 404 处理（前端隐藏喇叭即可）。
			if errors.Is(err, speech.ErrAudioNotFound) {
				writeError(ginCtx, http.StatusNotFound, "question audio not found")
				return
			}
			writeError(ginCtx, http.StatusServiceUnavailable, "question audio unavailable")
			return
		}
		ginCtx.Data(http.StatusOK, "audio/wav", audio)
	case "failed":
		writeJSON(ginCtx, http.StatusServiceUnavailable, map[string]any{"state": "failed"})
	default:
		writeJSON(ginCtx, http.StatusConflict, map[string]any{"state": "pending"})
	}
}

// transcribe 上传录音 → ASR 转写 → AI 修错别字 → 返回文本。
//
// 请求体是 multipart/form-data，字段名 file。
// 录音只在请求内透传给 ASR，不落盘、不回放。
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
	// 归属校验先于读文件：不属于本租户的资源按「不存在」处理。
	if _, err := h.service.QuestionForAudio(ginCtx.Request.Context(), principal.TenantID, interviewID, questionID); err != nil {
		handleInterviewError(ginCtx, err)
		return
	}

	// 解析前先限体积：multipart 解析会把整个请求体读进内存，超出内存阈值的部分
	// 由 net/http 写成磁盘临时文件 —— 那会让用户录音短暂落盘（与「不存录音」冲突），
	// 超大请求体还会变成无上限的资源消耗。先用 Content-Length 快速拒绝，
	// 再用 MaxBytesReader 兜住 chunked 请求（ContentLength 为 -1 时没有快速判断）。
	const maxUploadBytes = speech.MaxAudioBytes + (1 << 20) // 额外留 1MB 给 multipart 头部
	if ginCtx.Request.ContentLength > maxUploadBytes {
		writeError(ginCtx, http.StatusBadRequest, "audio file is too large")
		return
	}
	ginCtx.Request.Body = http.MaxBytesReader(ginCtx.Writer, ginCtx.Request.Body, maxUploadBytes)

	header, err := ginCtx.FormFile("file")
	if err != nil {
		// MaxBytesReader 触发的错误要归到「太大」，而不是「缺文件」。
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(ginCtx, http.StatusBadRequest, "audio file is too large")
			return
		}
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
	if len(data) > speech.MaxAudioBytes {
		writeError(ginCtx, http.StatusBadRequest, "audio file is too large")
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
