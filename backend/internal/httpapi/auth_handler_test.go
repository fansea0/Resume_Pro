package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"

	"resumepro/backend/internal/auth"
	"resumepro/backend/internal/resume"
	"resumepro/backend/internal/resumesvc"
	"resumepro/backend/internal/store"
	"resumepro/backend/internal/templates"
)

func TestAuthRegisterLoginMeAndLogout(t *testing.T) {
	router := newAuthTestRouter(t)

	register := authJSONRequest(t, router, http.MethodPost, "/api/auth/register", `{"email":"ada@example.com","password":"correct horse battery"}`, nil)
	if register.Code != http.StatusCreated {
		t.Fatalf("register status = %d: %s", register.Code, register.Body.String())
	}
	if register.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("register cache control = %q", register.Header().Get("Cache-Control"))
	}
	cookie := responseCookie(t, register, sessionCookieName)
	if !cookie.HttpOnly || cookie.Value == "" || cookie.Path != "/" || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("unexpected session cookie: %#v", cookie)
	}

	me := authJSONRequest(t, router, http.MethodGet, "/api/auth/me", "", cookie)
	if me.Code != http.StatusOK {
		t.Fatalf("me status = %d: %s", me.Code, me.Body.String())
	}
	var current struct {
		User publicUser `json:"user"`
	}
	if err := json.NewDecoder(me.Body).Decode(&current); err != nil {
		t.Fatal(err)
	}
	if current.User.Email != "ada@example.com" || current.User.ID == 0 || current.User.TenantID == 0 || current.User.IsAdmin {
		t.Fatalf("unexpected me user: %#v", current.User)
	}

	logout := authJSONRequest(t, router, http.MethodPost, "/api/auth/logout", "", cookie)
	if logout.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d: %s", logout.Code, logout.Body.String())
	}
	cleared := responseCookie(t, logout, sessionCookieName)
	if cleared.MaxAge >= 0 {
		t.Fatalf("logout cookie max age = %d, want negative", cleared.MaxAge)
	}

	login := authJSONRequest(t, router, http.MethodPost, "/api/auth/login", `{"email":"ADA@example.com","password":"correct horse battery"}`, nil)
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d: %s", login.Code, login.Body.String())
	}
}

func TestAuthLoginHidesInvalidCredentials(t *testing.T) {
	router := newAuthTestRouter(t)
	for _, body := range []string{
		`{"email":"missing@example.com","password":"correct horse battery"}`,
		`{"email":"missing@example.com","password":"wrong password value"}`,
		`{"email":"not-an-email","password":"correct horse battery"}`,
	} {
		response := authJSONRequest(t, router, http.MethodPost, "/api/auth/login", body, nil)
		if response.Code != http.StatusUnauthorized || response.Body.String() != "{\"error\":\"invalid email or password\"}\n" {
			t.Errorf("login body %s: %d %s", body, response.Code, response.Body.String())
		}
	}
}

func TestAuthMeReflectsCurrentAdministratorRole(t *testing.T) {
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
	router := NewRouter(authRepository, resume.NewRepository(db), templates.NewRepository(db), RouterConfig{JWTSecret: "test-secret-that-is-long-enough"})
	cookie := registerAuthUser(t, router, "role-state@example.com")
	user, err := authRepository.GetUserByEmail(ctx, "role-state@example.com")
	if err != nil {
		t.Fatal(err)
	}

	assertCurrentAdminRole := func(want bool) {
		t.Helper()
		response := authJSONRequest(t, router, http.MethodGet, "/api/auth/me", "", cookie)
		if response.Code != http.StatusOK {
			t.Fatalf("me status = %d: %s", response.Code, response.Body.String())
		}
		var current struct {
			User publicUser `json:"user"`
		}
		if err := json.NewDecoder(response.Body).Decode(&current); err != nil {
			t.Fatal(err)
		}
		if current.User.IsAdmin != want {
			t.Fatalf("isAdmin = %t, want %t", current.User.IsAdmin, want)
		}
	}

	if err := authRepository.SetAdministrator(ctx, user.ID, true); err != nil {
		t.Fatalf("promote administrator: %v", err)
	}
	assertCurrentAdminRole(true)

	if err := authRepository.SetAdministrator(ctx, user.ID, false); err != nil {
		t.Fatalf("revoke administrator: %v", err)
	}
	assertCurrentAdminRole(false)
}

func TestDevelopmentLoginIssuesNormalSessionForLoopbackRequest(t *testing.T) {
	router := newDevelopmentAuthTestRouter(t)
	registeredCookie := registerAuthUser(t, router, "dev-login@example.com")

	response := developmentLoginRequest(t, router, `{"email":"DEV-LOGIN@example.com"}`, "127.0.0.1:45678")
	if response.Code != http.StatusOK {
		t.Fatalf("development login status = %d: %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("development login cache control = %q", response.Header().Get("Cache-Control"))
	}
	developmentCookie := responseCookie(t, response, sessionCookieName)
	if developmentCookie.Value == "" || developmentCookie.HttpOnly != registeredCookie.HttpOnly || developmentCookie.Path != registeredCookie.Path || developmentCookie.SameSite != registeredCookie.SameSite || developmentCookie.Secure != registeredCookie.Secure || developmentCookie.MaxAge <= 0 {
		t.Fatalf("unexpected development session cookie: %#v", developmentCookie)
	}

	me := authJSONRequest(t, router, http.MethodGet, "/api/auth/me", "", developmentCookie)
	if me.Code != http.StatusOK {
		t.Fatalf("development session /me status = %d: %s", me.Code, me.Body.String())
	}
	var current struct {
		User publicUser `json:"user"`
	}
	if err := json.NewDecoder(me.Body).Decode(&current); err != nil {
		t.Fatal(err)
	}
	if current.User.Email != "dev-login@example.com" || current.User.ID == 0 || current.User.TenantID == 0 {
		t.Fatalf("unexpected development login principal: %#v", current.User)
	}
}

func TestDevelopmentLoginRejectsInvalidRequests(t *testing.T) {
	enabled := newDevelopmentAuthTestRouter(t)
	for _, tc := range []struct {
		name       string
		body       string
		remoteAddr string
		wantStatus int
	}{
		{name: "non-loopback", body: `{"email":"any@example.com"}`, remoteAddr: "192.0.2.1:1234", wantStatus: http.StatusForbidden},
		{name: "missing email", body: `{}`, remoteAddr: "127.0.0.1:1234", wantStatus: http.StatusBadRequest},
		{name: "invalid email", body: `{"email":"not-an-email"}`, remoteAddr: "127.0.0.1:1234", wantStatus: http.StatusBadRequest},
		{name: "unknown field", body: `{"email":"any@example.com","role":"admin"}`, remoteAddr: "127.0.0.1:1234", wantStatus: http.StatusBadRequest},
		{name: "unknown user", body: `{"email":"missing@example.com"}`, remoteAddr: "127.0.0.1:1234", wantStatus: http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := developmentLoginRequest(t, enabled, tc.body, tc.remoteAddr)
			if response.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d: %s", response.Code, tc.wantStatus, response.Body.String())
			}
		})
	}

	disabled := newAuthTestRouter(t)
	response := developmentLoginRequest(t, disabled, `{"email":"any@example.com"}`, "127.0.0.1:1234")
	if response.Code != http.StatusNotFound {
		t.Fatalf("disabled development login status = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestProtectedResumeRequiresSessionAndHidesCrossTenantResume(t *testing.T) {
	router := newAuthTestRouter(t)
	unauthenticated := authJSONRequest(t, router, http.MethodGet, "/api/resumes", "", nil)
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated list status = %d", unauthenticated.Code)
	}

	first := registerAuthUser(t, router, "first@example.com")
	created := authJSONRequest(t, router, http.MethodPost, "/api/resumes", validResumeRequest("Private", "minimal"), first)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status = %d: %s", created.Code, created.Body.String())
	}
	var owned resume.Resume
	if err := json.NewDecoder(created.Body).Decode(&owned); err != nil {
		t.Fatal(err)
	}

	second := registerAuthUser(t, router, "second@example.com")
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		response := authJSONRequest(t, router, method, "/api/resumes/"+jsonNumber(owned.ID), validResumeRequest("Stolen", "minimal"), second)
		if response.Code != http.StatusNotFound {
			t.Errorf("cross-tenant %s status = %d: %s", method, response.Code, response.Body.String())
		}
	}
}

func newAuthTestRouter(t *testing.T) http.Handler {
	t.Helper()
	db, err := sqlx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	resumeRepository := resume.NewRepository(db)
	return NewRouter(auth.NewRepository(db), resumeRepository, templates.NewRepository(db), RouterConfig{
		JWTSecret:        "test-secret-that-is-long-enough",
		ResumesvcService: resumesvc.NewService(resumeRepository, nil, nil),
	})
}

func newDevelopmentAuthTestRouter(t *testing.T) http.Handler {
	t.Helper()
	db, err := sqlx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	return NewRouter(auth.NewRepository(db), resume.NewRepository(db), templates.NewRepository(db), RouterConfig{JWTSecret: "test-secret-that-is-long-enough", EnableDevLogin: true})
}

func developmentLoginRequest(t *testing.T, router http.Handler, body, remoteAddr string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/dev/login", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	request.RemoteAddr = remoteAddr
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func authJSONRequest(t *testing.T, router http.Handler, method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	return response
}

func responseCookie(t *testing.T, recorder *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}
	t.Fatalf("response did not include %q cookie", name)
	return nil
}

func registerAuthUser(t *testing.T, router http.Handler, email string) *http.Cookie {
	t.Helper()
	response := authJSONRequest(t, router, http.MethodPost, "/api/auth/register", `{"email":"`+email+`","password":"correct horse battery"}`, nil)
	if response.Code != http.StatusCreated {
		t.Fatalf("register %s: %d %s", email, response.Code, response.Body.String())
	}
	return responseCookie(t, response, sessionCookieName)
}

func jsonNumber(value int64) string { return json.Number(stringifyInt(value)).String() }

func stringifyInt(value int64) string {
	if value == 0 {
		return "0"
	}
	result := ""
	for value > 0 {
		result = string(rune('0'+value%10)) + result
		value /= 10
	}
	return result
}
