package httpapi

import (
	"encoding/json"
	"net/http"
)

// NewRouter creates the HTTP API router.
func NewRouter(_ any) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]string{"status": "ok"})
	})

	return mux
}
