package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"

	"resumepro/backend/internal/auth"
	"resumepro/backend/internal/resume"
	"resumepro/backend/internal/store"
	"resumepro/backend/internal/templates"
)

func TestTemplateAdminRejectsRegularUsers(t *testing.T) {
	router, userCookie, _, _ := newTemplateAdminTestRouter(t, false)
	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		path := "/api/admin/templates"
		if method == http.MethodDelete {
			path += "/minimal"
		}
		response := authJSONRequest(t, router, method, path, "", userCookie)
		if response.Code != http.StatusForbidden {
			t.Errorf("%s %s status = %d, want %d: %s", method, path, response.Code, http.StatusForbidden, response.Body.String())
		}
	}
}

func TestTemplateAdminListsAndDisablesTemplates(t *testing.T) {
	router, adminCookie, _, _ := newTemplateAdminTestRouter(t, true)

	initial := authJSONRequest(t, router, http.MethodGet, "/api/admin/templates", "", adminCookie)
	if initial.Code != http.StatusOK {
		t.Fatalf("list initial status = %d: %s", initial.Code, initial.Body.String())
	}

	for attempt := 0; attempt < 2; attempt++ {
		deleted := authJSONRequest(t, router, http.MethodDelete, "/api/admin/templates/minimal", "", adminCookie)
		if deleted.Code != http.StatusNoContent {
			t.Fatalf("disable attempt %d status = %d: %s", attempt+1, deleted.Code, deleted.Body.String())
		}
	}

	listed := authJSONRequest(t, router, http.MethodGet, "/api/admin/templates", "", adminCookie)
	if listed.Code != http.StatusOK {
		t.Fatalf("list disabled status = %d: %s", listed.Code, listed.Body.String())
	}
	var response struct {
		DisabledTemplateIDs []string `json:"disabledTemplateIds"`
	}
	if err := json.NewDecoder(listed.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if len(response.DisabledTemplateIDs) != 1 || response.DisabledTemplateIDs[0] != "minimal" {
		t.Fatalf("disabled template IDs = %#v, want [minimal]", response.DisabledTemplateIDs)
	}
}

func TestTemplateAdminRejectsSessionAfterAdministratorRoleIsRevoked(t *testing.T) {
	router, adminCookie, _, authRepository := newTemplateAdminTestRouter(t, true)
	user, err := authRepository.GetUserByEmail(context.Background(), "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if err := authRepository.SetAdministrator(context.Background(), user.ID, false); err != nil {
		t.Fatalf("revoke administrator role: %v", err)
	}

	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		path := "/api/admin/templates"
		if method == http.MethodDelete {
			path += "/minimal"
		}
		response := authJSONRequest(t, router, method, path, "", adminCookie)
		if response.Code != http.StatusForbidden {
			t.Errorf("revoked administrator %s status = %d, want %d: %s", method, response.Code, http.StatusForbidden, response.Body.String())
		}
	}
}

func TestPublicTemplatesListsDisabledTemplateIDs(t *testing.T) {
	router, adminCookie, _, _ := newTemplateAdminTestRouter(t, true)
	disabled := authJSONRequest(t, router, http.MethodDelete, "/api/admin/templates/minimal", "", adminCookie)
	if disabled.Code != http.StatusNoContent {
		t.Fatalf("disable status = %d: %s", disabled.Code, disabled.Body.String())
	}

	response := authJSONRequest(t, router, http.MethodGet, "/api/templates", "", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("public list status = %d: %s", response.Code, response.Body.String())
	}
	var body struct {
		DisabledTemplateIDs []string `json:"disabledTemplateIds"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.DisabledTemplateIDs) != 1 || body.DisabledTemplateIDs[0] != "minimal" {
		t.Fatalf("disabled template IDs = %#v, want [minimal]", body.DisabledTemplateIDs)
	}
}

func TestAuthenticatedResponsesExposeAdministratorRole(t *testing.T) {
	router, adminCookie, _, _ := newTemplateAdminTestRouter(t, true)
	response := authJSONRequest(t, router, http.MethodGet, "/api/auth/me", "", adminCookie)
	if response.Code != http.StatusOK {
		t.Fatalf("me status = %d: %s", response.Code, response.Body.String())
	}
	var current struct {
		User publicUser `json:"user"`
	}
	if err := json.NewDecoder(response.Body).Decode(&current); err != nil {
		t.Fatal(err)
	}
	if !current.User.IsAdmin {
		t.Fatalf("isAdmin = false, want true")
	}
}

func newTemplateAdminTestRouter(t *testing.T, administrator bool) (http.Handler, *http.Cookie, *templates.Repository, *auth.Repository) {
	t.Helper()
	db, err := sqlx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	if err := store.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	authRepository := auth.NewRepository(db)
	user, err := authRepository.CreateUser(ctx, auth.CreateUserInput{Email: "admin@example.com", Password: "correct horse battery"})
	if err != nil {
		t.Fatal(err)
	}
	if administrator {
		if err := authRepository.SetAdministrator(ctx, user.ID, true); err != nil {
			t.Fatal(err)
		}
	}
	router := NewRouter(authRepository, resume.NewRepository(db), templates.NewRepository(db), RouterConfig{JWTSecret: "test-secret-that-is-long-enough"})
	login := authJSONRequest(t, router, http.MethodPost, "/api/auth/login", `{"email":"admin@example.com","password":"correct horse battery"}`, nil)
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d: %s", login.Code, login.Body.String())
	}
	return router, responseCookie(t, login, sessionCookieName), templates.NewRepository(db), authRepository
}
