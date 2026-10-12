package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"resumepro/backend/internal/auth"
	"resumepro/backend/internal/files"
	"resumepro/backend/internal/resume"
	"resumepro/backend/internal/resumesvc"
)

type resumeHandler struct {
	repository *resume.Repository
	service    *resumesvc.Service
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

func newResumeHandler(service *resumesvc.Service, repository *resume.Repository, storage files.FileStorage) *resumeHandler {
	return &resumeHandler{service: service, repository: repository, storage: storage}
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

func (h *resumeHandler) create(ginCtx *gin.Context) {
	input, ok := decodeInput(ginCtx)
	if !ok {
		return
	}
	principal, ok := principalFromContext(ginCtx)
	if !ok {
		writeError(ginCtx, http.StatusUnauthorized, "authentication required")
		return
	}
	if !validAvatarBinding(input.Content, principal, h.storage) {
		writeError(ginCtx, http.StatusBadRequest, "avatar URL and storage key must be a valid matching pair")
		return
	}
	item, err := h.service.Create(ginCtx.Request.Context(), resumesvc.CreateInput{
		Principal:  principal,
		Title:      input.Title,
		TemplateID: input.TemplateID,
		Content:    input.Content,
	})
	if err != nil {
		if strings.Contains(err.Error(), "target job") {
			writeError(ginCtx, http.StatusBadRequest, err.Error())
			return
		}
		if errors.Is(err, resumesvc.ErrAvatarRetention) {
			writeError(ginCtx, http.StatusInternalServerError, "could not retain avatar image")
			return
		}
		if errors.Is(err, resume.ErrAvatarKeyInUse) || errors.Is(err, resume.ErrAvatarKeyReserved) {
			writeError(ginCtx, http.StatusConflict, "avatar image is already used by another resume")
			return
		}
		writeError(ginCtx, http.StatusInternalServerError, "could not create resume")
		return
	}
	writeJSON(ginCtx, http.StatusCreated, item)
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

func (h *resumeHandler) update(ginCtx *gin.Context) {
	id, ok := routeID(ginCtx)
	if !ok {
		return
	}
	input, ok := decodeInput(ginCtx)
	if !ok {
		return
	}
	principal, ok := principalFromContext(ginCtx)
	if !ok {
		writeError(ginCtx, http.StatusUnauthorized, "authentication required")
		return
	}
	if !validAvatarBinding(input.Content, principal, h.storage) {
		writeError(ginCtx, http.StatusBadRequest, "avatar URL and storage key must be a valid matching pair")
		return
	}
	item, err := h.service.Update(ginCtx.Request.Context(), principal.TenantID, id, resumesvc.UpdateInput{
		Principal:  principal,
		Title:      input.Title,
		TemplateID: input.TemplateID,
		Content:    input.Content,
	})
	if err != nil && strings.Contains(err.Error(), "target job") {
		writeError(ginCtx, http.StatusBadRequest, err.Error())
		return
	}
	if errors.Is(err, resumesvc.ErrAvatarRetention) {
		writeError(ginCtx, http.StatusInternalServerError, "could not retain avatar image")
		return
	}
	if !handleRepositoryError(ginCtx, err) {
		return
	}
	writeJSON(ginCtx, http.StatusOK, item)
}

func validAvatarBinding(raw json.RawMessage, principal auth.Principal, storage files.FileStorage) bool {
	var document struct {
		Profile struct {
			AvatarURL        string `json:"avatarUrl"`
			AvatarStorageKey string `json:"avatarStorageKey"`
		} `json:"profile"`
	}
	if json.Unmarshal(raw, &document) != nil {
		return false
	}
	hasURL, hasKey := document.Profile.AvatarURL != "", document.Profile.AvatarStorageKey != ""
	if hasURL != hasKey {
		return false
	}
	return !hasURL || storage == nil || document.Profile.AvatarURL == storage.PublicURL(avatarOwner(principal), document.Profile.AvatarStorageKey)
}

func (h *resumeHandler) delete(ginCtx *gin.Context) {
	id, ok := routeID(ginCtx)
	if !ok {
		return
	}
	principal, ok := principalFromContext(ginCtx)
	if !ok {
		writeError(ginCtx, http.StatusUnauthorized, "authentication required")
		return
	}
	if !handleRepositoryError(ginCtx, h.service.Delete(ginCtx.Request.Context(), principal, id)) {
		return
	}
	ginCtx.Status(http.StatusNoContent)
}

func avatarOwner(principal auth.Principal) string {
	return "user-" + strconv.FormatInt(principal.UserID, 10)
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
	"mint-canvas": {}, "scholarly-serif": {}, "blue-sidebar": {}, "navy-gutter": {},
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
	if targetJobID, ok := document["targetJobId"]; ok {
		value, ok := targetJobID.(float64)
		if !ok || math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 || math.Trunc(value) != value {
			return false
		}
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
	return ok && hasStringFields(item, "id", "name", "role", "startDate", "endDate", "location", "link", "introduction", "techStack", "achievements", "highlightsMarkdown") && hasBooleanField(item, "showLocation") && hasBooleanField(item, "showLink") && hasBooleanField(item, "showIntroduction") && hasBooleanField(item, "showTechStack") && hasBooleanField(item, "showAchievements")
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
	} else if errors.Is(err, resume.ErrAvatarKeyInUse) || errors.Is(err, resume.ErrAvatarKeyReserved) {
		writeError(context, http.StatusConflict, "avatar image is already used by another resume")
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

func writeError(context *gin.Context, status int, message string, serverErrors ...error) {
	payload := map[string]string{"error": message}
	if len(serverErrors) > 0 && serverErrors[0] != nil {
		payload["server_err"] = serverErrors[0].Error()
	}
	writeJSON(context, status, payload)
}
