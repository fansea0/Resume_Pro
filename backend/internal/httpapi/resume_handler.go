package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

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

func (h *resumeHandler) list(writer http.ResponseWriter, request *http.Request) {
	principal, ok := principalFromRequest(request)
	if !ok {
		writeError(writer, http.StatusUnauthorized, "authentication required")
		return
	}
	items, err := h.repository.List(request.Context(), principal.TenantID)
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "could not list resumes")
		return
	}
	writeJSON(writer, http.StatusOK, items)
}

func (h *resumeHandler) create(writer http.ResponseWriter, request *http.Request) {
	input, ok := decodeInput(writer, request)
	if !ok {
		return
	}
	principal, ok := principalFromRequest(request)
	if !ok {
		writeError(writer, http.StatusUnauthorized, "authentication required")
		return
	}
	item, err := h.repository.Create(request.Context(), resume.CreateInput{
		TenantID: principal.TenantID, Title: input.Title, TemplateID: input.TemplateID, Content: string(input.Content),
	})
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "could not create resume")
		return
	}
	writeJSON(writer, http.StatusCreated, item)
}

func (h *resumeHandler) get(writer http.ResponseWriter, request *http.Request) {
	id, ok := routeID(writer, request)
	if !ok {
		return
	}
	principal, ok := principalFromRequest(request)
	if !ok {
		writeError(writer, http.StatusUnauthorized, "authentication required")
		return
	}
	item, err := h.repository.Get(request.Context(), principal.TenantID, id)
	if !handleRepositoryError(writer, err) {
		return
	}
	writeJSON(writer, http.StatusOK, item)
}

func (h *resumeHandler) update(writer http.ResponseWriter, request *http.Request) {
	id, ok := routeID(writer, request)
	if !ok {
		return
	}
	input, ok := decodeInput(writer, request)
	if !ok {
		return
	}
	principal, ok := principalFromRequest(request)
	if !ok {
		writeError(writer, http.StatusUnauthorized, "authentication required")
		return
	}
	item, err := h.repository.Update(request.Context(), principal.TenantID, id, resume.UpdateInput{
		Title: input.Title, TemplateID: input.TemplateID, Content: string(input.Content),
	})
	if !handleRepositoryError(writer, err) {
		return
	}
	writeJSON(writer, http.StatusOK, item)
}

func (h *resumeHandler) delete(writer http.ResponseWriter, request *http.Request) {
	id, ok := routeID(writer, request)
	if !ok {
		return
	}
	principal, ok := principalFromRequest(request)
	if !ok {
		writeError(writer, http.StatusUnauthorized, "authentication required")
		return
	}
	if !handleRepositoryError(writer, h.repository.Delete(request.Context(), principal.TenantID, id)) {
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func decodeInput(writer http.ResponseWriter, request *http.Request) (resumeInput, bool) {
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	var input resumeInput
	if err := decoder.Decode(&input); err != nil || decoder.Decode(&struct{}{}) != io.EOF || !validInput(input) {
		writeError(writer, http.StatusBadRequest, "title, templateId, and object content are required")
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

func routeID(writer http.ResponseWriter, request *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(request.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeError(writer, http.StatusNotFound, "resume not found")
		return 0, false
	}
	return id, true
}

func handleRepositoryError(writer http.ResponseWriter, err error) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, resume.ErrNotFound) {
		writeError(writer, http.StatusNotFound, "resume not found")
	} else {
		writeError(writer, http.StatusInternalServerError, "resume operation failed")
	}
	return false
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func writeError(writer http.ResponseWriter, status int, message string) {
	writeJSON(writer, status, map[string]string{"error": message})
}
