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
	var created resume.Resume
	cases := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
		assert     func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name:       "create",
			method:     http.MethodPost,
			path:       "/api/resumes",
			body:       `{"title":"Backend Engineer","templateId":"minimal","content":{"profile":{"name":"Ada"}}}`,
			wantStatus: http.StatusCreated,
			assert: func(t *testing.T, response *httptest.ResponseRecorder) {
				created = decodeResume(t, response)
				if created.Title != "Backend Engineer" || created.TemplateID != "minimal" || created.ID == 0 {
					t.Fatalf("created item = %#v", created)
				}
			},
		},
		{
			name:       "list",
			method:     http.MethodGet,
			path:       "/api/resumes",
			wantStatus: http.StatusOK,
			assert: func(t *testing.T, response *httptest.ResponseRecorder) {
				var items []resume.Resume
				if err := json.NewDecoder(response.Body).Decode(&items); err != nil {
					t.Fatal(err)
				}
				if len(items) != 1 || items[0].ID != created.ID {
					t.Fatalf("list = %#v", items)
				}
			},
		},
		{
			name:       "get",
			method:     http.MethodGet,
			path:       "/api/resumes/1",
			wantStatus: http.StatusOK,
			assert: func(t *testing.T, response *httptest.ResponseRecorder) {
				if item := decodeResume(t, response); item.Title != "Backend Engineer" {
					t.Fatalf("got item = %#v", item)
				}
			},
		},
		{
			name:       "update",
			method:     http.MethodPut,
			path:       "/api/resumes/1",
			body:       `{"title":"Staff Engineer","templateId":"business","content":{"profile":{"name":"Ada Lovelace"}}}`,
			wantStatus: http.StatusOK,
			assert: func(t *testing.T, response *httptest.ResponseRecorder) {
				if item := decodeResume(t, response); item.Title != "Staff Engineer" {
					t.Fatalf("updated item = %#v", item)
				}
			},
		},
		{
			name:       "delete",
			method:     http.MethodDelete,
			path:       "/api/resumes/1",
			wantStatus: http.StatusNoContent,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			response := requestJSON(t, router, tc.method, tc.path, tc.body)
			if response.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d: %s", response.Code, tc.wantStatus, response.Body.String())
			}
			if tc.assert != nil {
				tc.assert(t, response)
			}
		})
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
