package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"

	"resumepro/backend/internal/auth"
	"resumepro/backend/internal/resume"
	"resumepro/backend/internal/store"
	"resumepro/backend/internal/templates"
)

func TestResumeCRUD(t *testing.T) {
	router, session := testRouter(t)
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
			body:       validResumeRequest("Backend Engineer", "minimal"),
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
			body:       validResumeRequest("Staff Engineer", "business"),
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
			response := requestJSON(t, router, tc.method, tc.path, tc.body, session)
			if response.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d: %s", response.Code, tc.wantStatus, response.Body.String())
			}
			if tc.assert != nil {
				tc.assert(t, response)
			}
		})
	}
}

func TestResumeResponsesUseFrontendFieldNames(t *testing.T) {
	router, session := testRouter(t)
	response := requestJSON(t, router, http.MethodPost, "/api/resumes", validResumeRequest("Frontend Engineer", "minimal"), session)
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusCreated, response.Body.String())
	}

	var payload map[string]json.RawMessage
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if _, ok := payload["templateId"]; !ok {
		t.Fatalf("response keys = %#v, want templateId", payload)
	}
	if _, ok := payload["template_id"]; ok {
		t.Fatalf("response must not expose storage field template_id")
	}
}

func TestResumeInvalidCreateReturnsBadRequest(t *testing.T) {
	router, session := testRouter(t)
	for _, body := range []string{
		`{"title":"","templateId":"minimal","content":{}}`,
		`{"title":"Valid","templateId":"","content":{}}`,
		`{"title":"Valid","templateId":"minimal","content":"not an object"}`,
		`{"title":"Valid","templateId":"minimal","content":{},"unexpected":true}`,
		`{"title":"Valid","templateId":"minimal","content":{}} {}`,
	} {
		response := requestJSON(t, router, http.MethodPost, "/api/resumes", body, session)
		if response.Code != http.StatusBadRequest {
			t.Errorf("body %s: status = %d, want %d", body, response.Code, http.StatusBadRequest)
		}
	}
}

func TestResumeCreateAndUpdateRequireCompleteV2Content(t *testing.T) {
	router, session := testRouter(t)
	created := requestJSON(t, router, http.MethodPost, "/api/resumes", validResumeRequest("Valid", "minimal"), session)
	if created.Code != http.StatusCreated {
		t.Fatalf("create valid v2 status = %d: %s", created.Code, created.Body.String())
	}

	invalidContents := []string{
		`{}`,
		`{"title":"Valid","templateId":"minimal","profile":{"fullName":"","headline":"","email":"","phone":"","location":"","website":""},"sections":[]}`,
		`{"schemaVersion":2,"title":"Valid","templateId":"minimal","profile":{"fullName":"","headline":"","email":"","phone":"","location":"","website":""},"sections":[]}`,
		`{"schemaVersion":2,"title":"Valid","templateId":"minimal","profile":{"fullName":"","headline":"","email":"","phone":"","location":"","website":"","avatarUrl":""},"sections":[]}`,
		`{"schemaVersion":2,"title":"Valid","templateId":"minimal","profile":{"fullName":"","headline":"","email":"","phone":"","location":""},"sections":[]}`,
		`{"schemaVersion":2,"title":"Valid","templateId":"minimal","profile":{"fullName":"","headline":"","email":"","phone":"","location":"","website":""},"sections":[{"id":"bad","type":"unknown","title":"Bad","visible":true,"content":{}}]}`,
		`{"schemaVersion":2,"title":"Valid","templateId":"minimal","profile":{"fullName":"","headline":"","email":"","phone":"","location":"","website":""},"sections":[{"id":"work","type":"work","title":"工作经历","visible":true,"content":{"items":[{"id":"work-1","company":"公司","role":"开发","startDate":"2024.01","endDate":"至今","location":"","achievements":"","highlightsMarkdown":""}]}}]}`,
		`{"schemaVersion":2,"title":"Valid","templateId":"minimal","profile":{"fullName":"","headline":"","email":"","phone":"","location":"","website":""},"sections":[{"id":"project","type":"project","title":"项目经历","visible":"true","content":{"items":[]}}]}`,
	}
	for _, content := range invalidContents {
		body := `{"title":"Valid","templateId":"minimal","content":` + content + `}`
		for _, target := range []struct{ method, path string }{{http.MethodPost, "/api/resumes"}, {http.MethodPut, "/api/resumes/1"}} {
			response := requestJSON(t, router, target.method, target.path, body, session)
			if response.Code != http.StatusBadRequest {
				t.Errorf("%s content %s: status = %d, want %d", target.method, content, response.Code, http.StatusBadRequest)
			}
		}
	}
}

func TestResumeCreateAndUpdateRequireMatchingMetadataAndBoundedContent(t *testing.T) {
	router, session := testRouter(t)
	created := requestJSON(t, router, http.MethodPost, "/api/resumes", validResumeRequest("Valid", "minimal"), session)
	if created.Code != http.StatusCreated {
		t.Fatalf("create valid v2 status = %d: %s", created.Code, created.Body.String())
	}
	canonicalStorageKey := "550e8400-e29b-41d4-a716-446655440000"
	canonicalAvatarRequest := `{"title":"Valid","templateId":"minimal","content":` + validV2ContentWithProfile("Valid", "minimal", `{"fullName":"Ada","headline":"Engineer","email":"ada@example.com","phone":"","location":"","website":"","avatarUrl":"/uploads/avatars/avatar.png","avatarStorageKey":"`+canonicalStorageKey+`"}`) + `}`
	for _, target := range []struct {
		method, path string
		wantStatus   int
	}{{http.MethodPost, "/api/resumes", http.StatusCreated}, {http.MethodPut, "/api/resumes/1", http.StatusOK}} {
		response := requestJSON(t, router, target.method, target.path, canonicalAvatarRequest, session)
		if response.Code != target.wantStatus {
			t.Errorf("canonical avatar storage key %s: status = %d, want %d", target.method, response.Code, target.wantStatus)
		}
	}

	tooManySections := `[` + strings.TrimSuffix(strings.Repeat(`{"id":"summary","type":"summary","title":"个人总结","visible":true,"content":{"markdown":""}},`, 51), ",") + `]`
	tooManyItems := `[` + strings.TrimSuffix(strings.Repeat(`{"id":"skill","name":"Go","note":"熟悉"},`, 101), ",") + `]`
	largeSkillItems := `[` + strings.TrimSuffix(strings.Repeat(`{"id":"skill","name":"`+strings.Repeat("n", 220)+`","note":"熟悉"},`, 100), ",") + `]`
	largeSections := `[` + strings.TrimSuffix(strings.Repeat(`{"id":"skills","type":"skills","title":"专业技能","visible":true,"content":{"items":`+largeSkillItems+`}},`, 50), ",") + `]`
	largeContent := validV2ContentWithSections("Valid", "minimal", largeSections)
	largeRequest := `{"title":"Valid","templateId":"minimal","content":` + largeContent + `}`
	if len(largeRequest) <= 1<<20 {
		t.Fatalf("large request is %d bytes, want more than 1 MiB", len(largeRequest))
	}
	if !validResumeContent(json.RawMessage(largeContent), "Valid", "minimal") {
		t.Fatal("large content should satisfy the v2 schema and collection/string limits")
	}
	invalidBodies := []string{
		`{"title":"Outer","templateId":"minimal","content":` + validV2ResumeContent("Inner", "minimal") + `}`,
		`{"title":"Valid","templateId":"minimal","content":` + validV2ResumeContent("Valid", "business") + `}`,
		validResumeRequest("Valid", "unknown"),
		`{"title":"Valid","templateId":"minimal","content":` + validV2ContentWithSections("Valid", "minimal", tooManySections) + `}`,
		`{"title":"Valid","templateId":"minimal","content":` + validV2ContentWithSections("Valid", "minimal", `[{"id":"skills","type":"skills","title":"专业技能","visible":true,"content":{"items":`+tooManyItems+`}}]`) + `}`,
		`{"title":"Valid","templateId":"minimal","content":` + validV2ContentWithSections("Valid", "minimal", `[{"id":"summary","type":"summary","title":"个人总结","visible":true,"content":{"markdown":"`+strings.Repeat("x", 20_001)+`"}}]`) + `}`,
		`{"title":"Valid","templateId":"minimal","content":` + validV2ContentWithProfile("Valid", "minimal", `{"fullName":"Ada","headline":"Engineer","email":"ada@example.com","phone":"","location":"","website":"","avatarUrl":"`+strings.Repeat("x", 20_001)+`","avatarStorageKey":""}`) + `}`,
		`{"title":"Valid","templateId":"minimal","content":` + validV2ContentWithProfile("Valid", "minimal", `{"fullName":"Ada","headline":"Engineer","email":"ada@example.com","phone":"","location":"","website":"","avatarUrl":false,"avatarStorageKey":""}`) + `}`,
		`{"title":"Valid","templateId":"minimal","content":` + validV2ContentWithProfile("Valid", "minimal", `{"fullName":"Ada","headline":"Engineer","email":"ada@example.com","phone":"","location":"","website":"","avatarUrl":"","avatarStorageKey":false}`) + `}`,
		`{"title":"Valid","templateId":"minimal","content":` + validV2ContentWithProfile("Valid", "minimal", `{"fullName":"Ada","headline":"Engineer","email":"ada@example.com","phone":"","location":"","website":"","avatarUrl":"","avatarStorageKey":"avatar-key"}`) + `}`,
		`{"title":"Valid","templateId":"minimal","content":` + validV2ContentWithProfile("Valid", "minimal", `{"fullName":"Ada","headline":"Engineer","email":"ada@example.com","phone":"","location":"","website":"","avatarUrl":"","avatarStorageKey":"`+strings.Repeat("x", 20_001)+`"}`) + `}`,
		largeRequest,
	}
	for _, body := range invalidBodies {
		for _, target := range []struct{ method, path string }{{http.MethodPost, "/api/resumes"}, {http.MethodPut, "/api/resumes/1"}} {
			response := requestJSON(t, router, target.method, target.path, body, session)
			if response.Code != http.StatusBadRequest {
				t.Errorf("%s: status = %d, want %d", target.method, response.Code, http.StatusBadRequest)
			}
		}
	}
}

func TestResumeMissingReturnsNotFound(t *testing.T) {
	router, session := testRouter(t)
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		response := requestJSON(t, router, method, "/api/resumes/999", validResumeRequest("Valid", "minimal"), session)
		if response.Code != http.StatusNotFound {
			t.Errorf("%s status = %d, want %d", method, response.Code, http.StatusNotFound)
		}
	}
}

func validResumeRequest(title, templateID string) string {
	return `{"title":"` + title + `","templateId":"` + templateID + `","content":` + validV2ResumeContent(title, templateID) + `}`
}

func validV2ResumeContent(title, templateID string) string {
	return validV2ContentWithSections(title, templateID, `[`+
		`{"id":"summary","type":"summary","title":"个人总结","visible":true,"content":{"markdown":"总结"}},`+
		`{"id":"work","type":"work","title":"工作经历","visible":true,"content":{"items":[{"id":"work-1","company":"公司","role":"工程师","startDate":"2024.01","endDate":"至今","location":"上海","achievements":"交付","highlightsMarkdown":"- 说明","showLocation":true,"showAchievements":true}]}},`+
		`{"id":"internship","type":"internship","title":"实习经历","visible":true,"content":{"items":[{"id":"intern-1","company":"实习公司","role":"实习生","startDate":"2023.01","endDate":"2023.06","location":"上海","achievements":"交付","highlightsMarkdown":"- 说明","showLocation":false,"showAchievements":false}]}},`+
		`{"id":"project","type":"project","title":"项目经历","visible":true,"content":{"items":[{"id":"project-1","name":"项目","role":"负责人","startDate":"2024.01","endDate":"至今","location":"上海","link":"https://example.com","introduction":"介绍","achievements":"成绩","highlightsMarkdown":"- 说明","showLocation":true,"showLink":true,"showIntroduction":true,"showAchievements":true}]}},`+
		`{"id":"education","type":"education","title":"教育经历","visible":true,"content":{"items":[{"id":"education-1","school":"学校","startDate":"2020.09","endDate":"2024.06","degree":"本科","college":"学院","major":"专业"}]}},`+
		`{"id":"skills","type":"skills","title":"专业技能","visible":true,"content":{"items":[{"id":"skill-1","name":"Go","note":"熟悉"}]}},`+
		`{"id":"campus","type":"campus","title":"校园经历","visible":true,"content":{"items":[{"id":"campus-1","name":"社团","role":"负责人","highlightsMarkdown":"- 说明"}]}},`+
		`{"id":"honours","type":"honours","title":"荣誉奖项","visible":true,"content":{"items":[{"id":"honour-1","name":"一等奖"}]}}]`)
}

func validV2ContentWithSections(title, templateID, sections string) string {
	return validV2ContentWithProfile(title, templateID, `{"fullName":"Ada","headline":"Engineer","email":"ada@example.com","phone":"","location":"","website":"","avatarUrl":"","avatarStorageKey":""}`, sections)
}

func validV2ContentWithProfile(title, templateID, profile string, sections ...string) string {
	contentSections := `[]`
	if len(sections) > 0 {
		contentSections = sections[0]
	}
	return `{"schemaVersion":2,"title":"` + title + `","templateId":"` + templateID + `","profile":` + profile + `,"sections":` + contentSections + `}`
}

func testRouter(t *testing.T) (http.Handler, *http.Cookie) {
	t.Helper()
	db, err := sqlx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	router := NewRouter(auth.NewRepository(db), resume.NewRepository(db), templates.NewRepository(db), RouterConfig{JWTSecret: "resume-handler-test-secret"})
	return router, registerAuthUser(t, router, "resume-handler@example.com")
}

func requestJSON(t *testing.T, router http.Handler, method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		request.AddCookie(cookie)
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
