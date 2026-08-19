package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"resumepro/backend/internal/auth"
	"resumepro/backend/internal/resume"
	"resumepro/backend/internal/store"
	"resumepro/backend/internal/targetjob"
	"resumepro/backend/internal/templates"
)

func TestHealth(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/health", nil)

	NewRouter(nil, nil, nil, RouterConfig{}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}

	if body := recorder.Body.String(); body != "{\"status\":\"ok\"}\n" {
		t.Fatalf("body = %q, want %q", body, "{\"status\":\"ok\"}\n")
	}
}

func TestNewRouterUsesGinEngine(t *testing.T) {
	var router *gin.Engine = NewRouter(nil, nil, nil, RouterConfig{})
	if router == nil {
		t.Fatal("router is nil")
	}
}

func TestNewRouterRegistersTargetJobRoutes(t *testing.T) {
	db, err := sqlx.Open("sqlite", filepath.Join(t.TempDir(), "router.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, store.Migrate(context.Background(), db))

	router := NewRouter(auth.NewRepository(db), resume.NewRepository(db), templates.NewRepository(db), RouterConfig{
		JWTSecret:        "test-secret-32-chars-minimum-len-padding",
		TargetJobService: targetjob.NewService(targetjob.NewRepository(db)),
	})

	routes := make(map[string]bool)
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = true
	}
	for _, route := range []string{
		"GET /api/target-jobs",
		"POST /api/target-jobs",
		"GET /api/target-jobs/:id",
		"PUT /api/target-jobs/:id",
		"DELETE /api/target-jobs/:id",
	} {
		assert.True(t, routes[route], "missing route %s", route)
	}
}
