package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"resumepro/backend/internal/templates"
)

type templateHandler struct{ repository *templates.Repository }

func newTemplateHandler(repository *templates.Repository) *templateHandler {
	return &templateHandler{repository: repository}
}

func (h *templateHandler) listDisabled(context *gin.Context) {
	templateIDs, err := h.repository.ListDisabled(context.Request.Context())
	if err != nil {
		writeError(context, http.StatusInternalServerError, "could not list disabled templates")
		return
	}
	writeJSON(context, http.StatusOK, map[string][]string{"disabledTemplateIds": templateIDs})
}

func (h *templateHandler) disable(context *gin.Context) {
	templateID := strings.TrimSpace(context.Param("templateID"))
	if templateID == "" {
		writeError(context, http.StatusBadRequest, "template ID is required")
		return
	}
	if err := h.repository.Disable(context.Request.Context(), templateID); err != nil {
		if errors.Is(err, templates.ErrInvalidTemplateID) {
			writeError(context, http.StatusBadRequest, "template ID is required")
			return
		}
		writeError(context, http.StatusInternalServerError, "could not disable template")
		return
	}
	context.Status(http.StatusNoContent)
}
