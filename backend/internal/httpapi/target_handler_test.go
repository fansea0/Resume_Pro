package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	"resumepro/backend/internal/httpapi"
	"resumepro/backend/internal/resume"
	"resumepro/backend/internal/store"
	"resumepro/backend/internal/targetjob"
	"resumepro/backend/internal/templates"
)

func setupTargetAPI(t *testing.T) (*gin.Engine, *sqlx.DB) {
	t.Helper()
	db, err := sqlx.Open("sqlite", filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, store.Migrate(context.Background(), db))

	authRepo := auth.NewRepository(db)
	resumeRepo := resume.NewRepository(db)
	tmplRepo := templates.NewRepository(db)
	targetSvc := targetjob.NewService(targetjob.NewRepository(db))

	secret := "test-secret-32-chars-minimum-len-padding"
	_, err = auth.NewService(authRepo, secret)
	require.NoError(t, err)

	router := httpapi.NewRouter(authRepo, resumeRepo, tmplRepo, httpapi.RouterConfig{
		JWTSecret:        secret,
		CookieSecure:     false,
		EnableDevLogin:   false,
		TargetJobService: targetSvc,
	})
	return router, db
}

func loginAs(t *testing.T, router *gin.Engine, email string) string {
	t.Helper()
	body, err := json.Marshal(map[string]string{"email": email, "password": "test-password-12chars"})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == "resume_session" {
			return cookie.Value
		}
	}
	t.Fatal("no session cookie set")
	return ""
}

func targetRequest(t *testing.T, router *gin.Engine, method, path string, body []byte, cookie string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: "resume_session", Value: cookie})
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func targetJobPayload(t *testing.T, company, position, requirements string) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]string{
		"company": company, "position": position, "requirements": requirements,
	})
	require.NoError(t, err)
	return body
}

func TestTargetJobCRUD(t *testing.T) {
	router, _ := setupTargetAPI(t)
	cookie := loginAs(t, router, "user@example.com")

	createdResponse := targetRequest(t, router, http.MethodPost, "/api/target-jobs", targetJobPayload(t, "字节", "后端", "负责 ToB 营销中台后端架构设计"), cookie)
	require.Equal(t, http.StatusCreated, createdResponse.Code, createdResponse.Body.String())
	var created targetjob.TargetJob
	require.NoError(t, json.Unmarshal(createdResponse.Body.Bytes(), &created))
	assert.Equal(t, "字节", created.Company)
	require.NotZero(t, created.ID)

	listResponse := targetRequest(t, router, http.MethodGet, "/api/target-jobs", nil, cookie)
	require.Equal(t, http.StatusOK, listResponse.Code, listResponse.Body.String())
	var list []targetjob.TargetJob
	require.NoError(t, json.Unmarshal(listResponse.Body.Bytes(), &list))
	require.Len(t, list, 1)

	path := "/api/target-jobs/" + itoa(created.ID)
	getResponse := targetRequest(t, router, http.MethodGet, path, nil, cookie)
	require.Equal(t, http.StatusOK, getResponse.Code, getResponse.Body.String())

	updatedResponse := targetRequest(t, router, http.MethodPut, path, targetJobPayload(t, "新公司", "", "更新后的 JD 内容，超过十字"), cookie)
	require.Equal(t, http.StatusOK, updatedResponse.Code, updatedResponse.Body.String())
	var updated targetjob.TargetJob
	require.NoError(t, json.Unmarshal(updatedResponse.Body.Bytes(), &updated))
	assert.Equal(t, "新公司", updated.Company)
	assert.Equal(t, "", updated.Position)

	deleteResponse := targetRequest(t, router, http.MethodDelete, path, nil, cookie)
	require.Equal(t, http.StatusOK, deleteResponse.Code, deleteResponse.Body.String())
	assert.Equal(t, "0", deleteResponse.Header().Get("X-Deleted-References"))
	var deleted struct {
		DeletedReferences int64 `json:"deletedReferences"`
	}
	require.NoError(t, json.Unmarshal(deleteResponse.Body.Bytes(), &deleted))
	assert.EqualValues(t, 0, deleted.DeletedReferences)
}

func TestTargetJobCreateValidationError(t *testing.T) {
	router, _ := setupTargetAPI(t)
	cookie := loginAs(t, router, "user@example.com")

	response := targetRequest(t, router, http.MethodPost, "/api/target-jobs", targetJobPayload(t, "x", "y", "short"), cookie)
	assert.Equal(t, http.StatusBadRequest, response.Code)
}

func TestTargetJobCreateRequiresAuthentication(t *testing.T) {
	router, _ := setupTargetAPI(t)

	response := targetRequest(t, router, http.MethodPost, "/api/target-jobs", targetJobPayload(t, "", "", "valid requirement here"), "")
	assert.Equal(t, http.StatusUnauthorized, response.Code)
}

func TestTargetJobTenantIsolation(t *testing.T) {
	router, _ := setupTargetAPI(t)
	firstCookie := loginAs(t, router, "first@example.com")
	secondCookie := loginAs(t, router, "second@example.com")

	createdResponse := targetRequest(t, router, http.MethodPost, "/api/target-jobs", targetJobPayload(t, "第一家公司", "工程师", "first tenant target job"), firstCookie)
	require.Equal(t, http.StatusCreated, createdResponse.Code, createdResponse.Body.String())
	var created targetjob.TargetJob
	require.NoError(t, json.Unmarshal(createdResponse.Body.Bytes(), &created))
	path := "/api/target-jobs/" + itoa(created.ID)

	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		response := targetRequest(t, router, method, path, nil, secondCookie)
		assert.Equal(t, http.StatusNotFound, response.Code, method)
	}
	updateResponse := targetRequest(t, router, http.MethodPut, path, targetJobPayload(t, "窃取公司", "工程师", "cross tenant update must fail"), secondCookie)
	assert.Equal(t, http.StatusNotFound, updateResponse.Code)

	listResponse := targetRequest(t, router, http.MethodGet, "/api/target-jobs", nil, secondCookie)
	require.Equal(t, http.StatusOK, listResponse.Code, listResponse.Body.String())
	var list []targetjob.TargetJob
	require.NoError(t, json.Unmarshal(listResponse.Body.Bytes(), &list))
	assert.JSONEq(t, "[]", listResponse.Body.String())
	assert.NotNil(t, list)
	assert.Empty(t, list)
}

func itoa(i int64) string { return fmt.Sprintf("%d", i) }
