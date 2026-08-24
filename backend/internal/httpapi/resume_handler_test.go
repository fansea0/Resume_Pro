package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"

	"resumepro/backend/internal/auth"
	"resumepro/backend/internal/files"
	"resumepro/backend/internal/resume"
	"resumepro/backend/internal/resumesvc"
	"resumepro/backend/internal/store"
	"resumepro/backend/internal/targetjob"
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

func TestResumeLifecycleRetainsNewAvatarAndReleasesReplacedOrDeletedAvatar(t *testing.T) {
	storage := &lifecycleFileStorage{}
	router, session := testRouterWithStorage(t, storage)
	firstKey := "550e8400-e29b-41d4-a716-446655440000"
	secondKey := "550e8400-e29b-41d4-a716-446655440001"
	profile := func(key string) string {
		return `{"fullName":"Ada","headline":"Engineer","email":"ada@example.com","phone":"","location":"","website":"","avatarUrl":"/uploads/avatars/avatar.avatar","avatarStorageKey":"` + key + `"}`
	}
	create := `{"title":"Avatar","templateId":"minimal","content":` + validV2ContentWithProfile("Avatar", "minimal", profile(firstKey)) + `}`
	if response := requestJSON(t, router, http.MethodPost, "/api/resumes", create, session); response.Code != http.StatusCreated {
		t.Fatalf("create status = %d: %s", response.Code, response.Body.String())
	}
	if !storage.retained[firstKey] {
		t.Fatalf("create did not retain %q", firstKey)
	}
	update := `{"title":"Avatar updated","templateId":"minimal","content":` + validV2ContentWithProfile("Avatar updated", "minimal", profile(secondKey)) + `}`
	if response := requestJSON(t, router, http.MethodPut, "/api/resumes/1", update, session); response.Code != http.StatusOK {
		t.Fatalf("update status = %d: %s", response.Code, response.Body.String())
	}
	if !storage.retained[secondKey] || !storage.released[firstKey] {
		t.Fatalf("update lifecycle = retained:%v released:%v", storage.retained, storage.released)
	}
	if response := requestJSON(t, router, http.MethodDelete, "/api/resumes/1", "", session); response.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d: %s", response.Code, response.Body.String())
	}
	if !storage.released[secondKey] {
		t.Fatalf("delete did not release %q", secondKey)
	}
}

func TestResumeCreateDoesNotPersistAvatarWhenRetentionFails(t *testing.T) {
	storage := &lifecycleFileStorage{retainErr: errors.New("storage unavailable")}
	router, session := testRouterWithStorage(t, storage)
	key := "550e8400-e29b-41d4-a716-446655440000"
	profile := `{"fullName":"Ada","headline":"Engineer","email":"ada@example.com","phone":"","location":"","website":"","avatarUrl":"/uploads/avatars/avatar.avatar","avatarStorageKey":"` + key + `"}`
	body := `{"title":"Avatar","templateId":"minimal","content":` + validV2ContentWithProfile("Avatar", "minimal", profile) + `}`
	response := requestJSON(t, router, http.MethodPost, "/api/resumes", body, session)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("create status = %d: %s", response.Code, response.Body.String())
	}
	if response.Body.String() != "{\"error\":\"could not retain avatar image\"}\n" {
		t.Fatalf("create body = %q", response.Body.String())
	}
	response = requestJSON(t, router, http.MethodGet, "/api/resumes", "", session)
	var items []resume.Resume
	if err := json.NewDecoder(response.Body).Decode(&items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("resume persisted despite retain failure: %#v", items)
	}
}

func TestResumeUpdatePreservesAvatarRetentionFailureResponse(t *testing.T) {
	storage := &lifecycleFileStorage{}
	router, session := testRouterWithStorage(t, storage)
	created := requestJSON(t, router, http.MethodPost, "/api/resumes", validResumeRequest("Existing", "minimal"), session)
	if created.Code != http.StatusCreated {
		t.Fatalf("setup create status = %d: %s", created.Code, created.Body.String())
	}

	storage.retainErr = errors.New("storage unavailable")
	key := "550e8400-e29b-41d4-a716-446655440000"
	profile := `{"fullName":"Ada","headline":"Engineer","email":"ada@example.com","phone":"","location":"","website":"","avatarUrl":"/uploads/avatars/avatar.avatar","avatarStorageKey":"` + key + `"}`
	body := `{"title":"Updated","templateId":"minimal","content":` + validV2ContentWithProfile("Updated", "minimal", profile) + `}`
	response := requestJSON(t, router, http.MethodPut, "/api/resumes/1", body, session)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("update status = %d: %s", response.Code, response.Body.String())
	}
	if response.Body.String() != "{\"error\":\"could not retain avatar image\"}\n" {
		t.Fatalf("update body = %q", response.Body.String())
	}
}

type lifecycleFileStorage struct {
	retained  map[string]bool
	released  map[string]bool
	retainErr error
}

func (s *lifecycleFileStorage) Store(context.Context, string, string, files.File) (files.StoredFile, error) {
	return files.StoredFile{}, nil
}

func (s *lifecycleFileStorage) PublicURL(string, string) string {
	return "/uploads/avatars/avatar.avatar"
}

func (s *lifecycleFileStorage) Retain(_ context.Context, _ string, key string) error {
	if s.retainErr != nil {
		return s.retainErr
	}
	if s.retained == nil {
		s.retained = map[string]bool{}
	}
	s.retained[key] = true
	return nil
}

func (s *lifecycleFileStorage) Release(_ context.Context, _ string, key string) error {
	if s.released == nil {
		s.released = map[string]bool{}
	}
	s.released[key] = true
	return nil
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

func TestResumeCreateAcceptsEverySelectableTemplate(t *testing.T) {
	router, session := testRouter(t)
	for _, templateID := range []string{
		"minimal", "business", "creative", "tech", "editorial", "redline", "violet", "azure-chronicle",
		"mint-canvas", "scholarly-serif", "blue-sidebar",
	} {
		t.Run(templateID, func(t *testing.T) {
			response := requestJSON(t, router, http.MethodPost, "/api/resumes", validResumeRequest("Valid "+templateID, templateID), session)
			if response.Code != http.StatusCreated {
				t.Fatalf("create status = %d, want %d: %s", response.Code, http.StatusCreated, response.Body.String())
			}
		})
	}
}

func TestValidResumeContentAllowsOnlyPositiveIntegralTargetJobID(t *testing.T) {
	withTargetJobID := func(value string) json.RawMessage {
		content := strings.TrimSuffix(validV2ResumeContent("Valid", "minimal"), "}") + `,"targetJobId":` + value + `}`
		return json.RawMessage(content)
	}

	cases := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "omitted", value: "", want: true},
		{name: "positive integer", value: "42", want: true},
		{name: "zero", value: "0", want: false},
		{name: "negative integer", value: "-1", want: false},
		{name: "fraction", value: "1.5", want: false},
		{name: "string", value: `"42"`, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			content := json.RawMessage(validV2ResumeContent("Valid", "minimal"))
			if tc.value != "" {
				content = withTargetJobID(tc.value)
			}
			if got := validResumeContent(content, "Valid", "minimal"); got != tc.want {
				t.Fatalf("validResumeContent(targetJobId=%s) = %t, want %t", tc.value, got, tc.want)
			}
		})
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
	}{{http.MethodPost, "/api/resumes", http.StatusCreated}, {http.MethodPut, "/api/resumes/1", http.StatusConflict}} {
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

func TestResumeCreateAndUpdateRejectUnpairedOrForeignAvatarURLs(t *testing.T) {
	storage := &lifecycleFileStorage{}
	router, session := testRouterWithStorage(t, storage)
	if response := requestJSON(t, router, http.MethodPost, "/api/resumes", validResumeRequest("Existing", "minimal"), session); response.Code != http.StatusCreated {
		t.Fatalf("create existing status = %d: %s", response.Code, response.Body.String())
	}
	key := "550e8400-e29b-41d4-a716-446655440000"
	badURLProfile := `{"fullName":"Ada","headline":"Engineer","email":"","phone":"","location":"","website":"","avatarUrl":"/uploads/avatars/another-owner.avatar","avatarStorageKey":"` + key + `"}`
	badURL := `{"title":"Invalid","templateId":"minimal","content":` + validV2ContentWithProfile("Invalid", "minimal", badURLProfile) + `}`
	for _, target := range []struct{ method, path string }{{http.MethodPost, "/api/resumes"}, {http.MethodPut, "/api/resumes/1"}} {
		response := requestJSON(t, router, target.method, target.path, badURL, session)
		if response.Code != http.StatusBadRequest {
			t.Errorf("%s bad avatar URL status = %d, want %d: %s", target.method, response.Code, http.StatusBadRequest, response.Body.String())
		}
	}
	unpairedProfile := `{"fullName":"Ada","headline":"Engineer","email":"","phone":"","location":"","website":"","avatarUrl":"","avatarStorageKey":"` + key + `"}`
	unpaired := `{"title":"Invalid","templateId":"minimal","content":` + validV2ContentWithProfile("Invalid", "minimal", unpairedProfile) + `}`
	if response := requestJSON(t, router, http.MethodPost, "/api/resumes", unpaired, session); response.Code != http.StatusBadRequest {
		t.Fatalf("unpaired avatar status = %d, want %d: %s", response.Code, http.StatusBadRequest, response.Body.String())
	}
}

func TestResumeCreateAndUpdateRejectUnknownTargetJob(t *testing.T) {
	router, session := testRouter(t)
	content := strings.TrimSuffix(validV2ResumeContent("Targeted", "minimal"), "}") + `,"targetJobId":999}`
	body := `{"title":"Targeted","templateId":"minimal","content":` + content + `}`

	create := requestJSON(t, router, http.MethodPost, "/api/resumes", body, session)
	if create.Code != http.StatusBadRequest {
		t.Fatalf("create status = %d, want %d: %s", create.Code, http.StatusBadRequest, create.Body.String())
	}
	if create.Body.String() != "{\"error\":\"target job 999 not found or not owned by tenant\"}\n" {
		t.Fatalf("create body = %q", create.Body.String())
	}

	created := requestJSON(t, router, http.MethodPost, "/api/resumes", validResumeRequest("Existing", "minimal"), session)
	if created.Code != http.StatusCreated {
		t.Fatalf("setup create status = %d: %s", created.Code, created.Body.String())
	}
	update := requestJSON(t, router, http.MethodPut, "/api/resumes/1", body, session)
	if update.Code != http.StatusBadRequest {
		t.Fatalf("update status = %d, want %d: %s", update.Code, http.StatusBadRequest, update.Body.String())
	}
	if update.Body.String() != "{\"error\":\"target job 999 not found or not owned by tenant\"}\n" {
		t.Fatalf("update body = %q", update.Body.String())
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
	return testRouterWithStorage(t, nil)
}

func testRouterWithStorage(t *testing.T, fileStorage files.FileStorage) (http.Handler, *http.Cookie) {
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
	targetJobService := targetjob.NewService(targetjob.NewRepository(db))
	router := NewRouter(auth.NewRepository(db), resumeRepository, templates.NewRepository(db), RouterConfig{
		JWTSecret:        "resume-handler-test-secret",
		FileStorage:      fileStorage,
		ResumesvcService: resumesvc.NewService(resumeRepository, fileStorage, targetJobService),
	})
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
