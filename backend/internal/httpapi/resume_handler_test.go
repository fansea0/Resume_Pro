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

	"resumepro/backend/internal/resume"
	"resumepro/backend/internal/store"
)

func TestResumeCRUD(t *testing.T) {
	router := testRouter(t)
	created := requestJSON(t, router, http.MethodPost, "/api/resumes", `{"title":"Backend Engineer","templateId":"minimal","content":{"profile":{"name":"Ada"}}}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d: %s", created.Code, http.StatusCreated, created.Body.String())
	}
	item := decodeResume(t, created)
	if item.Title != "Backend Engineer" || item.TemplateID != "minimal" || item.ID == 0 {
		t.Fatalf("created item = %#v", item)
	}

	list := requestJSON(t, router, http.MethodGet, "/api/resumes", "")
	if list.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d", list.Code, http.StatusOK)
	}
	var items []resume.Resume
	if err := json.NewDecoder(list.Body).Decode(&items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != item.ID {
		t.Fatalf("list = %#v", items)
	}

	got := requestJSON(t, router, http.MethodGet, "/api/resumes/1", "")
	if got.Code != http.StatusOK || decodeResume(t, got).Title != "Backend Engineer" {
		t.Fatalf("get = %d %s", got.Code, got.Body.String())
	}

	updated := requestJSON(t, router, http.MethodPut, "/api/resumes/1", `{"title":"Staff Engineer","templateId":"business","content":{"profile":{"name":"Ada Lovelace"}}}`)
	if updated.Code != http.StatusOK || decodeResume(t, updated).Title != "Staff Engineer" {
		t.Fatalf("update = %d %s", updated.Code, updated.Body.String())
	}

	deleted := requestJSON(t, router, http.MethodDelete, "/api/resumes/1", "")
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d", deleted.Code, http.StatusNoContent)
	}
}

func TestResumeInvalidCreateReturnsBadRequest(t *testing.T) {
	router := testRouter(t)
	for _, body := range []string{
		`{"title":"","templateId":"minimal","content":{}}`,
		`{"title":"Valid","templateId":"","content":{}}`,
		`{"title":"Valid","templateId":"minimal","content":"not an object"}`,
		`{"title":"Valid","templateId":"minimal","content":{},"unexpected":true}`,
		`{"title":"Valid","templateId":"minimal","content":{}} {}`,
	} {
		response := requestJSON(t, router, http.MethodPost, "/api/resumes", body)
		if response.Code != http.StatusBadRequest {
			t.Errorf("body %s: status = %d, want %d", body, response.Code, http.StatusBadRequest)
		}
	}
}

func TestResumeMissingReturnsNotFound(t *testing.T) {
	router := testRouter(t)
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		response := requestJSON(t, router, method, "/api/resumes/999", `{"title":"Valid","templateId":"minimal","content":{}}`)
		if response.Code != http.StatusNotFound {
			t.Errorf("%s status = %d, want %d", method, response.Code, http.StatusNotFound)
		}
	}
}

func testRouter(t *testing.T) http.Handler {
	t.Helper()
	db, err := sqlx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	return NewRouter(resume.NewRepository(db))
}

func requestJSON(t *testing.T, router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func decodeResume(t *testing.T, response *httptest.ResponseRecorder) resume.Resume {
	t.Helper()
	var item resume.Resume
	if err := json.NewDecoder(response.Body).Decode(&item); err != nil {
		t.Fatal(err)
	}
	return item
}
