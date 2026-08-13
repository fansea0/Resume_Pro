package httpapi

import (
	"encoding/json"
	"net/http"

	"resumepro/backend/internal/resume"
)

// NewRouter creates the HTTP API router.
func NewRouter(repositories ...*resume.Repository) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]string{"status": "ok"})
	})
	if len(repositories) > 0 && repositories[0] != nil {
		handler := newResumeHandler(repositories[0])
		mux.HandleFunc("GET /api/resumes", handler.list)
		mux.HandleFunc("POST /api/resumes", handler.create)
		mux.HandleFunc("GET /api/resumes/{id}", handler.get)
		mux.HandleFunc("PUT /api/resumes/{id}", handler.update)
		mux.HandleFunc("DELETE /api/resumes/{id}", handler.delete)
	}

	return mux
}
