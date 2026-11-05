package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"resumepro/backend/internal/interview"
	"resumepro/backend/internal/resume"
	"resumepro/backend/internal/targetjob"
)

// interviewHandler 是 AI 模拟面试的 HTTP 层：只做鉴权、入参解码、DTO 转换
// （interview 包类型 → 前端契约形状）与错误映射，业务全部委托 Service。
//
// service 为 nil 时所有端点返回 503（Redis / LLM 不可用时由 main 注入 nil，
// 与 aiHandler 的降级约定一致）。
type interviewHandler struct {
	service    *interview.Service
	resumeRepo *resume.Repository
	targetSvc  *targetjob.Service
	// voiceEnabled 表示语音能力是否启用（前端据此隐藏语音按钮）。
	voiceEnabled bool
}

func newInterviewHandler(service *interview.Service, resumeRepo *resume.Repository, targetSvc *targetjob.Service, voiceEnabled bool) *interviewHandler {
	return &interviewHandler{service: service, resumeRepo: resumeRepo, targetSvc: targetSvc, voiceEnabled: voiceEnabled}
}

type createInterviewInput struct {
	TargetJobID int64                      `json:"targetJobId"`
	ResumeID    int64                      `json:"resumeId"`
	Config      *interview.InterviewConfig `json:"config"`
}

func (h *interviewHandler) create(ginCtx *gin.Context) {
	principal, ok := principalFromContext(ginCtx)
	if !ok {
		writeError(ginCtx, http.StatusUnauthorized, "authentication required")
		return
	}
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
	var input createInterviewInput
	if err := ginCtx.ShouldBindJSON(&input); err != nil || input.TargetJobID < 1 || input.ResumeID < 1 {
		writeError(ginCtx, http.StatusBadRequest, "targetJobId and resumeId are required")
		return
	}

	// 简历输入：resumes.content 是结构化 JSON，提取纯文本作生题输入
	item, err := h.resumeRepo.Get(ginCtx.Request.Context(), principal.TenantID, input.ResumeID)
	if err != nil {
		if errors.Is(err, resume.ErrNotFound) {
			writeError(ginCtx, http.StatusNotFound, "resume not found")
			return
		}
		writeError(ginCtx, http.StatusInternalServerError, "could not load resume")
		return
	}
	resumeText := extractResumeText(item.Content)

	// 目标岗位上下文：target_jobs 行的 Company/Position/Requirements
	var jd *interview.JDContext
	if h.targetSvc != nil {
		target, err := h.targetSvc.Get(ginCtx.Request.Context(), principal.TenantID, input.TargetJobID)
		if errors.Is(err, targetjob.ErrNotFound) {
			writeError(ginCtx, http.StatusNotFound, "target job not found")
			return
		}
		if err != nil {
			writeError(ginCtx, http.StatusInternalServerError, "could not load target job")
			return
		}
		jd = &interview.JDContext{Company: target.Company, Position: target.Position, Requirements: target.Requirements}
	}

	iv, err := h.service.CreateInterview(ginCtx.Request.Context(), principal.TenantID, input.TargetJobID, input.ResumeID, resumeText, jd, input.Config)
	if err != nil {
		if errors.Is(err, interview.ErrActiveInterviewExists) {
			writeError(ginCtx, http.StatusConflict, err.Error())
			return
		}
		writeError(ginCtx, http.StatusInternalServerError, "could not create interview", err)
		return
	}
	first, err := h.service.FirstQuestion(ginCtx.Request.Context(), principal.TenantID, iv.ID)
	if err != nil {
		writeError(ginCtx, http.StatusInternalServerError, "could not load first question")
		return
	}
	// 新面试恒为 active：首题同样不下发标准答案与考察点，避免在面试开始前就透题。
	writeJSON(ginCtx, http.StatusCreated, map[string]any{
		"interview":     h.interviewDTO(iv),
		"firstQuestion": questionDTO(first, false),
	})
}

func (h *interviewHandler) list(ginCtx *gin.Context) {
	principal, ok := principalFromContext(ginCtx)
	if !ok {
		writeError(ginCtx, http.StatusUnauthorized, "authentication required")
		return
	}
	if h.service == nil {
		writeError(ginCtx, http.StatusServiceUnavailable, "AI interviews disabled (Redis required)")
		return
	}
	items, err := h.service.List(ginCtx.Request.Context(), principal.TenantID)
	if err != nil {
		writeError(ginCtx, http.StatusInternalServerError, "could not list interviews")
		return
	}
	dtos := make([]any, 0, len(items))
	for i := range items {
		dtos = append(dtos, h.interviewDTO(&items[i]))
	}
	writeJSON(ginCtx, http.StatusOK, dtos)
}

func (h *interviewHandler) get(ginCtx *gin.Context) {
	principal, ok := principalFromContext(ginCtx)
	if !ok {
		writeError(ginCtx, http.StatusUnauthorized, "authentication required")
		return
	}
	if h.service == nil {
		writeError(ginCtx, http.StatusServiceUnavailable, "AI interviews disabled (Redis required)")
		return
	}
	id, ok := interviewRouteID(ginCtx)
	if !ok {
		return
	}
	iv, err := h.service.Get(ginCtx.Request.Context(), principal.TenantID, id)
	if err != nil {
		if !handleInterviewError(ginCtx, err) {
			return
		}
	}
	writeJSON(ginCtx, http.StatusOK, h.interviewDTO(iv))
}

type answerInput struct {
	Answer string `json:"answer"`
	Skip   bool   `json:"skip"`
}

func (h *interviewHandler) answer(ginCtx *gin.Context) {
	principal, ok := principalFromContext(ginCtx)
	if !ok {
		writeError(ginCtx, http.StatusUnauthorized, "authentication required")
		return
	}
	if h.service == nil {
		writeError(ginCtx, http.StatusServiceUnavailable, "AI interviews disabled (Redis required)")
		return
	}
	id, ok := interviewRouteID(ginCtx)
	if !ok {
		return
	}
	var input answerInput
	if err := ginCtx.ShouldBindJSON(&input); err != nil {
		writeError(ginCtx, http.StatusBadRequest, "answer or skip is required")
		return
	}
	if !input.Skip && strings.TrimSpace(input.Answer) == "" {
		writeError(ginCtx, http.StatusBadRequest, "answer is required")
		return
	}
	result, err := h.service.Answer(ginCtx.Request.Context(), principal.TenantID, id, interview.AnswerPayload{Answer: input.Answer, Skip: input.Skip})
	if err != nil {
		if !handleInterviewError(ginCtx, err) {
			return
		}
	}
	// 未结束 = 仍在面试中：下一题同样隐去标准答案与考察点。
	writeJSON(ginCtx, http.StatusOK, answerResultDTO(result, input.Skip, !result.Finished))
}

func (h *interviewHandler) questions(ginCtx *gin.Context) {
	principal, ok := principalFromContext(ginCtx)
	if !ok {
		writeError(ginCtx, http.StatusUnauthorized, "authentication required")
		return
	}
	if h.service == nil {
		writeError(ginCtx, http.StatusServiceUnavailable, "AI interviews disabled (Redis required)")
		return
	}
	id, ok := interviewRouteID(ginCtx)
	if !ok {
		return
	}
	iv, err := h.service.Get(ginCtx.Request.Context(), principal.TenantID, id)
	if err != nil {
		if !handleInterviewError(ginCtx, err) {
			return
		}
	}
	items, err := h.service.Questions(ginCtx.Request.Context(), principal.TenantID, id)
	if err != nil {
		if !handleInterviewError(ginCtx, err) {
			return
		}
	}
	// 面试进行中不下发标准答案与必答/加分点：候选人不能通过接口提前拿到答案；
	// 面试结束后（completed/aborted）报告页按原逻辑回看。
	withSolutions := iv.Status != "active"
	dtos := make([]any, 0, len(items))
	for i := range items {
		dtos = append(dtos, questionDTO(&items[i], withSolutions))
	}
	writeJSON(ginCtx, http.StatusOK, dtos)
}

func (h *interviewHandler) abort(ginCtx *gin.Context) {
	principal, ok := principalFromContext(ginCtx)
	if !ok {
		writeError(ginCtx, http.StatusUnauthorized, "authentication required")
		return
	}
	if h.service == nil {
		writeError(ginCtx, http.StatusServiceUnavailable, "AI interviews disabled (Redis required)")
		return
	}
	id, ok := interviewRouteID(ginCtx)
	if !ok {
		return
	}
	if err := h.service.Abort(ginCtx.Request.Context(), principal.TenantID, id); err != nil {
		if !handleInterviewError(ginCtx, err) {
			return
		}
	}
	// 前端 abortInterview 期望返回更新后的面试对象（status=aborted）
	iv, err := h.service.Get(ginCtx.Request.Context(), principal.TenantID, id)
	if err != nil {
		writeError(ginCtx, http.StatusInternalServerError, "could not load interview after abort")
		return
	}
	writeJSON(ginCtx, http.StatusOK, h.interviewDTO(iv))
}

func interviewRouteID(context *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(context.Param("id"), 10, 64)
	if err != nil || id < 1 {
		writeError(context, http.StatusNotFound, "interview not found")
		return 0, false
	}
	return id, true
}

// handleInterviewError 把 interview 包的哨兵错误映射为 HTTP 状态码。
func handleInterviewError(context *gin.Context, err error) bool {
	if err == nil {
		return true
	}
	switch {
	case errors.Is(err, interview.ErrNotFound):
		writeError(context, http.StatusNotFound, "interview not found")
	case errors.Is(err, interview.ErrActiveInterviewExists),
		errors.Is(err, interview.ErrAlreadyEnded),
		errors.Is(err, interview.ErrConcurrentAnswer):
		writeError(context, http.StatusConflict, err.Error())
	case errors.Is(err, interview.ErrSessionExpired):
		writeError(context, http.StatusGone, err.Error())
	default:
		writeError(context, http.StatusInternalServerError, "interview operation failed")
	}
	return false
}

// ---- DTO 转换（interview 包类型 → 前端契约形状） ----

// interviewDTO 把落库 Interview 转成前端 Interview 形状：
// configSnapshot 由 JSON 字符串解析为对象（InterviewConfig 的 camelCase tag），
// 其余字段 json tag 已对齐。
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

// questionDTO 把问题行转成前端 InterviewQuestion 形状：
// label 由 int 转字符串（"0"/"1"/"2"），extra 由 JSON 字符串解析为
// {mustPoints, bonusPoints}，空串字段转 null。
//
// withSolutions=false 时整段省略 standardAnswer 与 extra：面试未结束前，
// 标准答案和考察点不得出现在接口响应里（前端也就无从提前拿到）。
func questionDTO(q *interview.Question, withSolutions bool) map[string]any {
	if q == nil {
		return nil
	}
	dto := map[string]any{
		"id":            q.ID,
		"interviewId":   q.InterviewID,
		"parentId":      q.ParentID,
		"followUpRound": q.FollowUpRound,
		"label":         strconv.Itoa(int(q.Label)),
		"question":      q.Question,
		"userAnswer":    nullableText(q.UserAnswer),
		"grade":         nullableText(q.Grade),
		"createdAt":     q.CreatedAt,
		// audioState 是题目语音状态：null=未入队、pending=生成中、ready=已就绪、failed=失败。
		"audioState": q.AudioState,
	}
	if withSolutions {
		dto["standardAnswer"] = nullableText(q.StandardAnswer)
		dto["extra"] = questionExtraDTO(q.Extra)
	}
	return dto
}

// questionExtraDTO 把 extra 列（{"must_points":[],"bonus_points":[]}）解析成前端
// 契约的 {mustPoints, bonusPoints}；无必答/加分点或非法 JSON 时返回 nil。
func questionExtraDTO(extra string) map[string]any {
	if extra == "" {
		return nil
	}
	var payload struct {
		MustPoints  []string `json:"must_points"`
		BonusPoints []string `json:"bonus_points"`
	}
	if err := json.Unmarshal([]byte(extra), &payload); err != nil {
		return nil
	}
	if len(payload.MustPoints) == 0 && len(payload.BonusPoints) == 0 {
		return nil
	}
	return map[string]any{
		"mustPoints":  payload.MustPoints,
		"bonusPoints": payload.BonusPoints,
	}
}

// answerResultDTO 把图运行结果转成前端 AnswerResult 形状。
func answerResultDTO(result interview.AnswerResult, skip, hideSolutions bool) map[string]any {
	dto := map[string]any{
		"grade":    result.Grade,
		"followUp": result.FollowUp,
		"finished": result.Finished,
	}
	if skip {
		dto["skip"] = true
	}
	if result.NextQuestion != nil {
		dto["nextQuestion"] = questionDTO(result.NextQuestion, !hideSolutions)
	}
	if result.Finished {
		dto["overallGrade"] = result.OverallGrade
		dto["overallReview"] = result.OverallReview
	}
	return dto
}

func nullableText(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

// extractResumeText 把 resumes.content（结构化 JSON）提取成纯文本，
// 供生题 LLM 使用。遍历各 section 的 content，递归收集字符串字段值。
func extractResumeText(content string) string {
	var document struct {
		Profile  map[string]any `json:"profile"`
		Sections []struct {
			Type    string         `json:"type"`
			Content map[string]any `json:"content"`
		} `json:"sections"`
	}
	if err := json.Unmarshal([]byte(content), &document); err != nil {
		return ""
	}
	var parts []string
	for _, profileField := range []string{"fullName", "headline", "email", "phone", "location"} {
		if value, ok := document.Profile[profileField].(string); ok && strings.TrimSpace(value) != "" {
			parts = append(parts, value)
		}
	}
	for _, section := range document.Sections {
		if section.Type == "summary" {
			if markdown, ok := section.Content["markdown"].(string); ok && strings.TrimSpace(markdown) != "" {
				parts = append(parts, markdown)
			}
			continue
		}
		items, ok := section.Content["items"].([]any)
		if !ok {
			continue
		}
		for _, raw := range items {
			item, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			parts = append(parts, collectStringFields(item)...)
		}
	}
	return strings.Join(parts, "\n")
}

// collectStringFields 递归收集 map 中所有非空字符串值（含嵌套数组元素）。
func collectStringFields(value map[string]any) []string {
	var out []string
	for _, field := range value {
		switch typed := field.(type) {
		case string:
			if trimmed := strings.TrimSpace(typed); trimmed != "" {
				out = append(out, trimmed)
			}
		case []any:
			for _, element := range typed {
				if nested, ok := element.(map[string]any); ok {
					out = append(out, collectStringFields(nested)...)
				} else if text, ok := element.(string); ok && strings.TrimSpace(text) != "" {
					out = append(out, strings.TrimSpace(text))
				}
			}
		}
	}
	return out
}
