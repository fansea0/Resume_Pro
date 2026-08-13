package httpapi

import (
	"context"
	"net/http"

	"resumepro/backend/internal/auth"
)

const sessionCookieName = "resume_session"

type principalContextKey struct{}

func requireAuthentication(service *auth.Service, next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		cookie, err := request.Cookie(sessionCookieName)
		if err != nil {
			writeError(writer, http.StatusUnauthorized, "authentication required")
			return
		}
		principal, err := service.VerifyToken(cookie.Value)
		if err != nil {
			writeError(writer, http.StatusUnauthorized, "authentication required")
			return
		}
		next.ServeHTTP(writer, request.WithContext(context.WithValue(request.Context(), principalContextKey{}, principal)))
	})
}

func principalFromRequest(request *http.Request) (auth.Principal, bool) {
	principal, ok := request.Context().Value(principalContextKey{}).(auth.Principal)
	return principal, ok
}
