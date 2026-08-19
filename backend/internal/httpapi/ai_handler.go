package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"resumepro/backend/internal/ai"
	"resumepro/backend/internal/resume"
	"resumepro/backend/internal/targetjob"
)

type aiHandler struct {
	service       *ai.Service
	resumeRepo    *resume.Repository
	targetService *targetjob.Service
}

func newAIHandler(service *ai.Service, resumeRepo *resume.Repository, targetService *targetjob.Service) *aiHandler {
	return &aiHandler{service: service, resumeRepo: resumeRepo, targetService: targetService}
}

type rewriteRequest struct {
	SectionType  string `json:"sectionType"`
	Instructions string `json:"instructions"`
}

type aiResumeContext struct {
	TargetJobID *int64 `json:"targetJobId"`
	Sections    []struct {
		Type    string          `json:"type"`
		Content json.RawMessage `json:"content"`
	} `json:"sections"`
}

func (h *aiHandler) rewriteSection(ginCtx *gin.Context) {
	// TODO(ai-ratelimit): 接入 Limiter。
	principal, ok := principalFromContext(ginCtx)
	if !ok {
		writeError(ginCtx, http.StatusUnauthorized, "authentication required")
		return
	}
	if h.service == nil {
		writeError(ginCtx, http.StatusServiceUnavailable, "AI features disabled")
		return
	}
	resumeID, ok := routeID(ginCtx)
	if !ok {
		return
	}
	request, ok := decodeRewriteRequest(ginCtx)
	if !ok {
		return
	}
	if !isSupportedRewriteSection(request.SectionType) {
		writeError(ginCtx, http.StatusBadRequest, "unsupported section type for AI rewrite")
		return
	}

	item, err := h.resumeRepo.Get(ginCtx.Request.Context(), principal.TenantID, resumeID)
	if !handleRepositoryError(ginCtx, err) {
		return
	}
	resumeContext, err := parseAIResumeContext(item.Content)
	if err != nil {
		writeError(ginCtx, http.StatusInternalServerError, "invalid resume content")
		return
	}
	sectionContent, ok := resumeContext.sectionContent(request.SectionType)
	if !ok {
		writeError(ginCtx, http.StatusBadRequest, "section not found or empty")
		return
	}

	targetContext, err := h.loadTargetJobContext(ginCtx, principal.TenantID, resumeContext.TargetJobID)
	if err != nil {
		writeError(ginCtx, http.StatusInternalServerError, "load target job failed")
		return
	}
	stream, err := h.service.RewriteSection(ginCtx.Request.Context(), ai.RewriteInput{
		TenantID:       principal.TenantID,
		ResumeID:       resumeID,
		SectionType:    request.SectionType,
		CurrentContent: sectionContent,
		TargetJob:      targetContext,
		Instructions:   request.Instructions,
	})
	if err != nil {
		writeError(ginCtx, http.StatusInternalServerError, "could not start AI rewrite")
		return
	}
	writeAIStream(ginCtx, stream, request.SectionType)
}

func (h *aiHandler) generateResume(ginCtx *gin.Context) {
	// TODO(ai-ratelimit): 接入 Limiter。
	principal, ok := principalFromContext(ginCtx)
	if !ok {
		writeError(ginCtx, http.StatusUnauthorized, "authentication required")
		return
	}
	if h.service == nil {
		writeError(ginCtx, http.StatusServiceUnavailable, "AI features disabled")
		return
	}
	stream, err := h.service.GenerateResume(ginCtx.Request.Context(), ai.GenerateInput{TenantID: principal.TenantID})
	if err != nil {
		writeError(ginCtx, http.StatusInternalServerError, "could not start AI generation")
		return
	}
	writeAIStream(ginCtx, stream, "")
}

func decodeRewriteRequest(ginCtx *gin.Context) (rewriteRequest, bool) {
	ginCtx.Request.Body = http.MaxBytesReader(ginCtx.Writer, ginCtx.Request.Body, maxResumeRequestBodyBytes)
	decoder := json.NewDecoder(ginCtx.Request.Body)
	decoder.DisallowUnknownFields()
	var request rewriteRequest
	if err := decoder.Decode(&request); err != nil || decoder.Decode(&struct{}{}) != io.EOF || strings.TrimSpace(request.SectionType) == "" {
		writeError(ginCtx, http.StatusBadRequest, "sectionType is required")
		return rewriteRequest{}, false
	}
	return request, true
}

func parseAIResumeContext(content string) (aiResumeContext, error) {
	var resumeContext aiResumeContext
	if err := json.Unmarshal([]byte(content), &resumeContext); err != nil {
		return aiResumeContext{}, err
	}
	return resumeContext, nil
}

func (context aiResumeContext) sectionContent(sectionType string) (json.RawMessage, bool) {
	for _, section := range context.Sections {
		if section.Type != sectionType {
			continue
		}
		trimmed := strings.TrimSpace(string(section.Content))
		if trimmed == "" || trimmed == "null" || !json.Valid(section.Content) || !hasRewriteableContent(sectionType, section.Content) {
			return nil, false
		}
		return section.Content, true
	}
	return nil, false
}

func hasRewriteableContent(sectionType string, content json.RawMessage) bool {
	switch sectionType {
	case "summary":
		var summary struct {
			Markdown string `json:"markdown"`
		}
		return json.Unmarshal(content, &summary) == nil && strings.TrimSpace(summary.Markdown) != ""
	case "work", "internship", "project":
		var items struct {
			Items []json.RawMessage `json:"items"`
		}
		return json.Unmarshal(content, &items) == nil && len(items.Items) > 0
	default:
		return false
	}
}

func (h *aiHandler) loadTargetJobContext(ginCtx *gin.Context, tenantID int64, targetJobID *int64) (*ai.TargetJobContext, error) {
	if targetJobID == nil {
		return nil, nil
	}
	if h.targetService == nil {
		return nil, errors.New("target job service is required")
	}
	target, err := h.targetService.Get(ginCtx.Request.Context(), tenantID, *targetJobID)
	if errors.Is(err, targetjob.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ai.TargetJobContext{
		Company:      target.Company,
		Position:     target.Position,
		Requirements: target.Requirements,
	}, nil
}

func writeAIStream(ginCtx *gin.Context, stream <-chan ai.Event, sectionType string) {
	sse := newSSEWriter(ginCtx)
	for event := range stream {
		var err error
		switch event.Kind {
		case "thinking":
			err = sse.WriteEvent("thinking", map[string]string{"text": event.ThinkingText})
		case "suggestion":
			payload := map[string]any{"newContent": json.RawMessage(event.Suggestion)}
			if sectionType != "" {
				payload["sectionType"] = sectionType
			}
			err = sse.WriteEvent("suggestion", payload)
		case "error":
			err = sse.WriteEvent("error", map[string]string{"code": event.ErrorCode, "message": event.ErrorMessage})
		case "done":
			err = sse.WriteEvent("done", struct{}{})
		}
		if err != nil {
			return
		}
	}
}

func isSupportedRewriteSection(sectionType string) bool {
	switch sectionType {
	case "summary", "work", "internship", "project":
		return true
	default:
		return false
	}
}
