package httpapi

import (
	"encoding/json"
	"net/http"

	"resumepro/backend/internal/auth"
	"resumepro/backend/internal/resume"
)

type RouterConfig struct {
	JWTSecret      string
	CookieSecure   bool
	EnableDevLogin bool
}

// NewRouter creates the HTTP API router.
func NewRouter(authRepository *auth.Repository, resumeRepository *resume.Repository, config RouterConfig) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]string{"status": "ok"})
	})
	if authRepository == nil || resumeRepository == nil {
		return mux
	}
	service, err := auth.NewService(authRepository, config.JWTSecret)
	if err != nil {
		panic(err)
	}
	authHandler := newAuthHandler(service, config.CookieSecure)
	mux.HandleFunc("POST /api/auth/register", authHandler.register)
	mux.HandleFunc("POST /api/auth/login", authHandler.login)
	mux.HandleFunc("POST /api/auth/logout", authHandler.logout)
	if config.EnableDevLogin {
		mux.HandleFunc("POST /api/dev/login", authHandler.developmentLogin)
	}
	mux.Handle("GET /api/auth/me", requireAuthentication(service, http.HandlerFunc(authHandler.me)))
	resumeHandler := newResumeHandler(resumeRepository)
	protected := requireAuthentication(service, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.Method {
		case http.MethodGet:
			if request.PathValue("id") == "" {
				resumeHandler.list(writer, request)
			} else {
				resumeHandler.get(writer, request)
			}
		case http.MethodPost:
			resumeHandler.create(writer, request)
		case http.MethodPut:
			resumeHandler.update(writer, request)
		case http.MethodDelete:
			resumeHandler.delete(writer, request)
		}
	}))
	mux.Handle("GET /api/resumes", protected)
	mux.Handle("POST /api/resumes", protected)
	mux.Handle("GET /api/resumes/{id}", protected)
	mux.Handle("PUT /api/resumes/{id}", protected)
	mux.Handle("DELETE /api/resumes/{id}", protected)

	return mux
}
