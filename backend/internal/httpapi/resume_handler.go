package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"resumepro/backend/internal/auth"
	"resumepro/backend/internal/files"
	"resumepro/backend/internal/resume"
)

type resumeHandler struct {
	repository *resume.Repository
	storage    files.FileStorage
}

type resumeInput struct {
	Title      string          `json:"title"`
	TemplateID string          `json:"templateId"`
	Content    json.RawMessage `json:"content"`
}

const (
	maxResumeRequestBodyBytes = 1 << 20 // 1 MiB
	maxResumeSections         = 50
	maxResumeItemsPerSection  = 100
	maxResumeStringLength     = 20_000
)

func newResumeHandler(repository *resume.Repository, storage files.FileStorage) *resumeHandler {
	return &resumeHandler{repository: repository, storage: storage}
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
	if err := h.retainAvatar(context.Request.Context(), principal, input.Content); err != nil {
		writeError(context, http.StatusInternalServerError, "could not retain avatar image")
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
	previous, err := h.repository.Get(context.Request.Context(), principal.TenantID, id)
	if !handleRepositoryError(context, err) {
		return
	}
	item, err := h.repository.Update(context.Request.Context(), principal.TenantID, id, resume.UpdateInput{Title: input.Title, TemplateID: input.TemplateID, Content: string(input.Content)})
	if !handleRepositoryError(context, err) {
		return
	}
	if err := h.retainAvatar(context.Request.Context(), principal, input.Content); err != nil {
		writeError(context, http.StatusInternalServerError, "could not retain avatar image")
		return
	}
	if oldKey, newKey := avatarStorageKey(json.RawMessage(previous.Content)), avatarStorageKey(input.Content); h.storage != nil && oldKey != "" && oldKey != newKey {
		if err := h.storage.Release(context.Request.Context(), avatarOwner(principal), oldKey); err != nil {
			writeError(context, http.StatusInternalServerError, "could not release previous avatar image")
			return
		}
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
	previous, err := h.repository.Get(context.Request.Context(), principal.TenantID, id)
	if !handleRepositoryError(context, err) {
		return
	}
	if !handleRepositoryError(context, h.repository.Delete(context.Request.Context(), principal.TenantID, id)) {
		return
	}
	if oldKey := avatarStorageKey(json.RawMessage(previous.Content)); oldKey != "" && h.storage != nil {
		if err := h.storage.Release(context.Request.Context(), avatarOwner(principal), oldKey); err != nil {
			writeError(context, http.StatusInternalServerError, "could not release avatar image")
			return
		}
	}
	context.Status(http.StatusNoContent)
}

func (h *resumeHandler) retainAvatar(ctx context.Context, principal auth.Principal, content json.RawMessage) error {
	if h.storage == nil {
		return nil
	}
	if key := avatarStorageKey(content); key != "" {
		return h.storage.Retain(ctx, avatarOwner(principal), key)
	}
	return nil
}

func avatarOwner(principal auth.Principal) string {
	return "user-" + strconv.FormatInt(principal.UserID, 10)
}

func avatarStorageKey(raw json.RawMessage) string {
	var document struct {
		Profile struct {
			AvatarStorageKey string `json:"avatarStorageKey"`
		} `json:"profile"`
	}
	if json.Unmarshal(raw, &document) != nil {
		return ""
	}
	return document.Profile.AvatarStorageKey
}

func decodeInput(context *gin.Context) (resumeInput, bool) {
	context.Request.Body = http.MaxBytesReader(context.Writer, context.Request.Body, maxResumeRequestBodyBytes)
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
	if strings.TrimSpace(input.Title) == "" || strings.TrimSpace(input.TemplateID) == "" || !validString(input.Title) || !validTemplateID(input.TemplateID) || len(input.Content) == 0 {
		return false
	}
	return validResumeContent(input.Content, input.Title, input.TemplateID)
}

var validTemplateIDs = map[string]struct{}{
	"minimal": {}, "business": {}, "creative": {}, "tech": {},
	"editorial": {}, "redline": {}, "violet": {}, "azure-chronicle": {},
}

// validResumeContent mirrors the v2 ResumeDocument contract used by the editor.
// Existing v1 content remains readable so the frontend can migrate it, but all
// newly-created or updated documents must use this fixed storage format.
func validResumeContent(raw json.RawMessage, title, templateID string) bool {
	var document map[string]any
	if json.Unmarshal(raw, &document) != nil || document == nil {
		return false
	}
	if schemaVersion, ok := document["schemaVersion"].(float64); !ok || schemaVersion != 2 {
		return false
	}
	if !hasStringFields(document, "title", "templateId") || !validTemplateID(document["templateId"]) {
		return false
	}
	if document["title"] != title || document["templateId"] != templateID {
		return false
	}
	profile, ok := document["profile"].(map[string]any)
	if !ok || !hasStringFields(profile, "fullName", "headline", "email", "phone", "location", "website", "avatarUrl", "avatarStorageKey") || !validAvatarStorageKey(profile["avatarStorageKey"].(string)) {
		return false
	}
	sections, ok := document["sections"].([]any)
	if !ok || len(sections) > maxResumeSections {
		return false
	}
	for _, section := range sections {
		if !validResumeSection(section) {
			return false
		}
	}
	return true
}

func validTemplateID(value any) bool {
	templateID, ok := value.(string)
	if !ok || !validString(templateID) {
		return false
	}
	_, ok = validTemplateIDs[templateID]
	return ok
}

func validResumeSection(value any) bool {
	section, ok := value.(map[string]any)
	if !ok || !hasStringFields(section, "id", "title", "type") || !hasBooleanField(section, "visible") {
		return false
	}
	content, ok := section["content"].(map[string]any)
	if !ok {
		return false
	}
	switch section["type"] {
	case "summary":
		return hasStringFields(content, "markdown")
	case "work", "internship":
		return validItems(content, validExperienceItem)
	case "project":
		return validItems(content, validProjectItem)
	case "education":
		return validItems(content, validEducationItem)
	case "skills":
		return validItems(content, validSkillItem)
	case "campus":
		return validItems(content, validCampusItem)
	case "honours":
		return validItems(content, validHonourItem)
	default:
		return false
	}
}

func validItems(content map[string]any, validItem func(any) bool) bool {
	items, ok := content["items"].([]any)
	if !ok || len(items) > maxResumeItemsPerSection {
		return false
	}
	for _, item := range items {
		if !validItem(item) {
			return false
		}
	}
	return true
}

func validExperienceItem(value any) bool {
	item, ok := value.(map[string]any)
	return ok && hasStringFields(item, "id", "company", "role", "startDate", "endDate", "location", "achievements", "highlightsMarkdown") && hasBooleanField(item, "showLocation") && hasBooleanField(item, "showAchievements")
}

func validProjectItem(value any) bool {
	item, ok := value.(map[string]any)
	return ok && hasStringFields(item, "id", "name", "role", "startDate", "endDate", "location", "link", "introduction", "achievements", "highlightsMarkdown") && hasBooleanField(item, "showLocation") && hasBooleanField(item, "showLink") && hasBooleanField(item, "showIntroduction") && hasBooleanField(item, "showAchievements")
}

func validEducationItem(value any) bool {
	item, ok := value.(map[string]any)
	return ok && hasStringFields(item, "id", "school", "startDate", "endDate", "degree", "college", "major")
}

func validSkillItem(value any) bool {
	item, ok := value.(map[string]any)
	return ok && hasStringFields(item, "id", "name", "note")
}

func validCampusItem(value any) bool {
	item, ok := value.(map[string]any)
	return ok && hasStringFields(item, "id", "name", "role", "highlightsMarkdown")
}

func validHonourItem(value any) bool {
	item, ok := value.(map[string]any)
	return ok && hasStringFields(item, "id", "name")
}

func hasStringFields(value map[string]any, fields ...string) bool {
	for _, field := range fields {
		if stringValue, ok := value[field].(string); !ok || !validString(stringValue) {
			return false
		}
	}
	return true
}

func validString(value string) bool {
	return len(value) <= maxResumeStringLength
}

func validAvatarStorageKey(value string) bool {
	if value == "" {
		return true
	}
	storageKey, err := uuid.Parse(value)
	return err == nil && storageKey.String() == value
}

func hasBooleanField(value map[string]any, field string) bool {
	_, ok := value[field].(bool)
	return ok
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
