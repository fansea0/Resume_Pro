package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"resumepro/backend/internal/auth"
)

const (
	sessionCookieName   = "resume_session"
	principalContextKey = "authenticated_principal"
)

func requireAuthentication(service *auth.Service) gin.HandlerFunc {
	return func(context *gin.Context) {
		cookie, err := context.Request.Cookie(sessionCookieName)
		if err != nil {
			writeError(context, http.StatusUnauthorized, "authentication required")
			context.Abort()
			return
		}
		principal, err := service.VerifyToken(cookie.Value)
		if err != nil {
			writeError(context, http.StatusUnauthorized, "authentication required")
			context.Abort()
			return
		}
		context.Set(principalContextKey, principal)
	}
}

func requireAdministrator(repository *auth.Repository) gin.HandlerFunc {
	return func(context *gin.Context) {
		principal, ok := principalFromContext(context)
		if !ok {
			writeError(context, http.StatusForbidden, "administrator access required")
			context.Abort()
			return
		}
		user, err := repository.GetUserByID(context.Request.Context(), principal.UserID)
		if err != nil || !user.IsAdmin {
			writeError(context, http.StatusForbidden, "administrator access required")
			context.Abort()
			return
		}
	}
}

func principalFromContext(context *gin.Context) (auth.Principal, bool) {
	principal, ok := context.Get(principalContextKey)
	if !ok {
		return auth.Principal{}, false
	}
	value, ok := principal.(auth.Principal)
	return value, ok
}
