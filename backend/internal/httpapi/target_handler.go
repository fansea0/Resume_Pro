package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"resumepro/backend/internal/targetjob"
)

type targetHandler struct {
	service *targetjob.Service
}

func newTargetJobHandler(service *targetjob.Service) *targetHandler {
	return &targetHandler{service: service}
}

type targetJobInput struct {
	Company      string `json:"company"`
	Position     string `json:"position"`
	Requirements string `json:"requirements"`
}

func (h *targetHandler) create(context *gin.Context) {
	principal, ok := principalFromContext(context)
	if !ok {
		writeError(context, http.StatusUnauthorized, "authentication required")
		return
	}
	var input targetJobInput
	if err := context.ShouldBindJSON(&input); err != nil {
		writeError(context, http.StatusBadRequest, "company, position, requirements are required")
		return
	}
	target, err := h.service.Create(context.Request.Context(), principal.TenantID, targetjob.CreateInput(input))
	if err != nil {
		if errors.Is(err, targetjob.ErrValidation) {
			writeError(context, http.StatusBadRequest, err.Error())
			return
		}
		writeError(context, http.StatusInternalServerError, "could not create target job")
		return
	}
	writeJSON(context, http.StatusCreated, target)
}

func (h *targetHandler) list(context *gin.Context) {
	principal, ok := principalFromContext(context)
	if !ok {
		writeError(context, http.StatusUnauthorized, "authentication required")
		return
	}
	list, err := h.service.List(context.Request.Context(), principal.TenantID)
	if err != nil {
		writeError(context, http.StatusInternalServerError, "could not list target jobs")
		return
	}
	if list == nil {
		list = []targetjob.TargetJob{}
	}
	writeJSON(context, http.StatusOK, list)
}

func (h *targetHandler) get(context *gin.Context) {
	id, ok := targetJobRouteID(context)
	if !ok {
		return
	}
	principal, ok := principalFromContext(context)
	if !ok {
		writeError(context, http.StatusUnauthorized, "authentication required")
		return
	}
	target, err := h.service.Get(context.Request.Context(), principal.TenantID, id)
	if !handleTargetJobError(context, err) {
		return
	}
	writeJSON(context, http.StatusOK, target)
}

func (h *targetHandler) update(context *gin.Context) {
	id, ok := targetJobRouteID(context)
	if !ok {
		return
	}
	principal, ok := principalFromContext(context)
	if !ok {
		writeError(context, http.StatusUnauthorized, "authentication required")
		return
	}
	var input targetJobInput
	if err := context.ShouldBindJSON(&input); err != nil {
		writeError(context, http.StatusBadRequest, "company, position, requirements are required")
		return
	}
	target, err := h.service.Update(context.Request.Context(), principal.TenantID, id, targetjob.UpdateInput(input))
	if err != nil {
		if errors.Is(err, targetjob.ErrValidation) {
			writeError(context, http.StatusBadRequest, err.Error())
			return
		}
		if !handleTargetJobError(context, err) {
			return
		}
	}
	writeJSON(context, http.StatusOK, target)
}

func (h *targetHandler) delete(context *gin.Context) {
	id, ok := targetJobRouteID(context)
	if !ok {
		return
	}
	principal, ok := principalFromContext(context)
	if !ok {
		writeError(context, http.StatusUnauthorized, "authentication required")
		return
	}
	deletedReferences, err := h.service.Delete(context.Request.Context(), principal.TenantID, id)
	if !handleTargetJobError(context, err) {
		return
	}
	context.Header("X-Deleted-References", strconv.FormatInt(deletedReferences, 10))
	writeJSON(context, http.StatusOK, map[string]int64{"deletedReferences": deletedReferences})
}

func targetJobRouteID(context *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(context.Param("id"), 10, 64)
	if err != nil || id < 1 {
		writeError(context, http.StatusNotFound, "target job not found")
		return 0, false
	}
	return id, true
}

func handleTargetJobError(context *gin.Context, err error) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, targetjob.ErrNotFound) {
		writeError(context, http.StatusNotFound, "target job not found")
		return false
	}
	writeError(context, http.StatusInternalServerError, "target job operation failed")
	return false
}
