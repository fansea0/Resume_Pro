package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"resumepro/backend/internal/resume"
)

type resumeHandler struct{ repository *resume.Repository }

type resumeInput struct {
	Title      string          `json:"title"`
	TemplateID string          `json:"templateId"`
	Content    json.RawMessage `json:"content"`
}

func newResumeHandler(repository *resume.Repository) *resumeHandler {
	return &resumeHandler{repository: repository}
}

func (h *resumeHandler) list(context *gin.Context) {
	principal, ok := principalFromContext(context)
	if !ok {
		writeError(context, http.StatusUnauthorized, "authentication required")
		return
	}
	items, err := h.repository.List(context.Request.Context(), principal.TenantID)
	if err != nil {
		writeError(context, http.StatusInternalServerError, "could not list resumes")
		return
	}
	writeJSON(context, http.StatusOK, items)
}

func (h *resumeHandler) create(context *gin.Context) {
	input, ok := decodeInput(context)
	if !ok {
		return
	}
	principal, ok := principalFromContext(context)
	if !ok {
		writeError(context, http.StatusUnauthorized, "authentication required")
		return
	}
	item, err := h.repository.Create(context.Request.Context(), resume.CreateInput{TenantID: principal.TenantID, Title: input.Title, TemplateID: input.TemplateID, Content: string(input.Content)})
	if err != nil {
		writeError(context, http.StatusInternalServerError, "could not create resume")
		return
	}
	writeJSON(context, http.StatusCreated, item)
}

func (h *resumeHandler) get(context *gin.Context) {
	id, ok := routeID(context)
	if !ok {
		return
	}
	principal, ok := principalFromContext(context)
	if !ok {
		writeError(context, http.StatusUnauthorized, "authentication required")
		return
	}
	item, err := h.repository.Get(context.Request.Context(), principal.TenantID, id)
	if !handleRepositoryError(context, err) {
		return
	}
	writeJSON(context, http.StatusOK, item)
}

func (h *resumeHandler) update(context *gin.Context) {
	id, ok := routeID(context)
	if !ok {
		return
	}
	input, ok := decodeInput(context)
	if !ok {
		return
	}
	principal, ok := principalFromContext(context)
	if !ok {
		writeError(context, http.StatusUnauthorized, "authentication required")
		return
	}
	item, err := h.repository.Update(context.Request.Context(), principal.TenantID, id, resume.UpdateInput{Title: input.Title, TemplateID: input.TemplateID, Content: string(input.Content)})
	if !handleRepositoryError(context, err) {
		return
	}
	writeJSON(context, http.StatusOK, item)
}

func (h *resumeHandler) delete(context *gin.Context) {
	id, ok := routeID(context)
	if !ok {
		return
	}
	principal, ok := principalFromContext(context)
	if !ok {
		writeError(context, http.StatusUnauthorized, "authentication required")
		return
	}
	if !handleRepositoryError(context, h.repository.Delete(context.Request.Context(), principal.TenantID, id)) {
		return
	}
	context.Status(http.StatusNoContent)
}

func decodeInput(context *gin.Context) (resumeInput, bool) {
	decoder := json.NewDecoder(context.Request.Body)
	decoder.DisallowUnknownFields()
	var input resumeInput
	if err := decoder.Decode(&input); err != nil || decoder.Decode(&struct{}{}) != io.EOF || !validInput(input) {
		writeError(context, http.StatusBadRequest, "title, templateId, and object content are required")
		return resumeInput{}, false
	}
	return input, true
}

func validInput(input resumeInput) bool {
	if strings.TrimSpace(input.Title) == "" || strings.TrimSpace(input.TemplateID) == "" || len(input.Content) == 0 {
		return false
	}
	var object map[string]json.RawMessage
	return json.Unmarshal(input.Content, &object) == nil && object != nil
}

func routeID(context *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(context.Param("id"), 10, 64)
	if err != nil || id < 1 {
		writeError(context, http.StatusNotFound, "resume not found")
		return 0, false
	}
	return id, true
}

func handleRepositoryError(context *gin.Context, err error) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, resume.ErrNotFound) {
		writeError(context, http.StatusNotFound, "resume not found")
	} else {
		writeError(context, http.StatusInternalServerError, "resume operation failed")
	}
	return false
}

func writeJSON(context *gin.Context, status int, value any) {
	context.Header("Content-Type", "application/json")
	context.Status(status)
	_ = json.NewEncoder(context.Writer).Encode(value)
}

func writeError(context *gin.Context, status int, message string) {
	writeJSON(context, status, map[string]string{"error": message})
}
