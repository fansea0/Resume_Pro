package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"resumepro/backend/internal/ai"
	"resumepro/backend/internal/auth"
	"resumepro/backend/internal/httpapi"
	llmtesting "resumepro/backend/internal/llm/testing"
	"resumepro/backend/internal/resume"
	"resumepro/backend/internal/resumesvc"
	"resumepro/backend/internal/store"
	"resumepro/backend/internal/targetjob"
	"resumepro/backend/internal/templates"
)

type aiAPIFixture struct {
	router         *gin.Engine
	authRepository *auth.Repository
	resumeRepo     *resume.Repository
	targetService  *targetjob.Service
}

func setupAIAPI(t *testing.T, service *ai.Service) aiAPIFixture {
	t.Helper()
	db, err := sqlx.Open("sqlite", filepath.Join(t.TempDir(), "ai-handler.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, store.Migrate(context.Background(), db))

	authRepository := auth.NewRepository(db)
	resumeRepository := resume.NewRepository(db)
	targetService := targetjob.NewService(targetjob.NewRepository(db))
	router := httpapi.NewRouter(authRepository, resumeRepository, templates.NewRepository(db), httpapi.RouterConfig{
		JWTSecret:        "test-secret-32-chars-minimum-len-padding",
		TargetJobService: targetService,
		ResumesvcService: resumesvc.NewService(resumeRepository, nil, targetService),
		AIService:        service,
	})
	return aiAPIFixture{
		router:         router,
		authRepository: authRepository,
		resumeRepo:     resumeRepository,
		targetService:  targetService,
	}
}

func TestAIRewriteRequiresAuthentication(t *testing.T) {
	fake := llmtesting.NewFake()
	fake.FinalJSON = `{"markdown":"new"}`
	fixture := setupAIAPI(t, ai.NewService(fake, nil, ai.WithMaxRetries(0)))

	response := aiRequest(t, fixture.router, "/api/ai/resumes/1/section/rewrite", `{"sectionType":"summary"}`, nil)

	assert.Equal(t, http.StatusUnauthorized, response.Code)
	assert.JSONEq(t, `{"error":"authentication required"}`, response.Body.String())
	assert.Empty(t, fake.RecordedInputs)
}

func TestAIRoutesReturnServiceUnavailableWhenDisabled(t *testing.T) {
	fixture := setupAIAPI(t, nil)
	cookie := loginAs(t, fixture.router, "ai-disabled@example.com")

	for _, tc := range []struct {
		name string
		path string
		body string
	}{
		{name: "rewrite", path: "/api/ai/resumes/1/section/rewrite", body: `{"sectionType":"summary"}`},
		{name: "generate", path: "/api/ai/resumes/generate", body: `{}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := aiRequest(t, fixture.router, tc.path, tc.body, &http.Cookie{Name: "resume_session", Value: cookie})

			assert.Equal(t, http.StatusServiceUnavailable, response.Code)
			assert.Equal(t, "application/json", response.Header().Get("Content-Type"))
			assert.JSONEq(t, `{"error":"AI features disabled"}`, response.Body.String())
		})
	}
}

func TestAIRewriteHidesCrossTenantResume(t *testing.T) {
	fake := llmtesting.NewFake()
	fake.FinalJSON = `{"markdown":"new"}`
	fixture := setupAIAPI(t, ai.NewService(fake, nil, ai.WithMaxRetries(0)))
	loginAs(t, fixture.router, "ai-owner@example.com")
	attackerCookie := loginAs(t, fixture.router, "ai-attacker@example.com")
	owned := createAIResume(t, fixture, "ai-owner@example.com", resumeContentForAI(nil, "summary", `{"markdown":"private"}`))

	response := aiRequest(t, fixture.router, "/api/ai/resumes/"+itoa(owned.ID)+"/section/rewrite", `{"sectionType":"summary"}`, &http.Cookie{Name: "resume_session", Value: attackerCookie})

	assert.Equal(t, http.StatusNotFound, response.Code)
	assert.JSONEq(t, `{"error":"resume not found"}`, response.Body.String())
	assert.Empty(t, fake.RecordedInputs)
}

func TestAIRewriteStreamsStoredSectionAndMappedTargetJobContext(t *testing.T) {
	fake := llmtesting.NewFake()
	fake.ThinkingText = "先分析岗位要求"
	fake.FinalJSON = `{"markdown":"量化交付成果"}`
	fixture := setupAIAPI(t, ai.NewService(fake, nil, ai.WithMaxRetries(0)))
	cookie := loginAs(t, fixture.router, "ai-context@example.com")
	user, err := fixture.authRepository.GetUserByEmail(context.Background(), "ai-context@example.com")
	require.NoError(t, err)
	target, err := fixture.targetService.Create(context.Background(), user.TenantID, targetjob.CreateInput{
		Company:      "示例公司",
		Position:     "后端工程师",
		Requirements: "负责高并发系统架构和稳定性交付",
	})
	require.NoError(t, err)
	item := createAIResume(t, fixture, user.Email, resumeContentForAI(&target.ID, "summary", `{"markdown":"旧总结"}`))

	response := aiRequest(t, fixture.router, "/api/ai/resumes/"+itoa(item.ID)+"/section/rewrite", `{"sectionType":"summary","instructions":"突出稳定性"}`, &http.Cookie{Name: "resume_session", Value: cookie})

	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assertSSEHeaders(t, response)
	assert.True(t, response.Flushed, "each SSE event should flush buffered bytes")
	frames := parseSSEFrames(t, response.Body.String())
	require.Len(t, frames, 3)
	assert.Equal(t, []string{"thinking", "suggestion", "done"}, frameNames(frames))
	assert.JSONEq(t, `{"text":"先分析岗位要求"}`, frames[0].Data)
	assert.JSONEq(t, `{"sectionType":"summary","newContent":{"markdown":"量化交付成果"}}`, frames[1].Data)
	assert.JSONEq(t, `{}`, frames[2].Data)

	require.Len(t, fake.RecordedInputs, 1)
	require.Len(t, fake.RecordedInputs[0], 2)
	var promptInput struct {
		SectionType    string               `json:"sectionType"`
		CurrentContent json.RawMessage      `json:"currentContent"`
		TargetJob      *ai.TargetJobContext `json:"targetJob"`
		Instructions   string               `json:"instructions"`
	}
	require.NoError(t, json.Unmarshal([]byte(fake.RecordedInputs[0][1].Content), &promptInput))
	assert.Equal(t, "summary", promptInput.SectionType)
	assert.JSONEq(t, `{"markdown":"旧总结"}`, string(promptInput.CurrentContent))
	require.NotNil(t, promptInput.TargetJob)
	assert.Equal(t, &ai.TargetJobContext{
		Company:      "示例公司",
		Position:     "后端工程师",
		Requirements: "负责高并发系统架构和稳定性交付",
	}, promptInput.TargetJob)
	assert.Equal(t, "突出稳定性", promptInput.Instructions)
}

func TestAIRewriteTreatsMissingOrForeignTargetJobAsNoContext(t *testing.T) {
	for _, tc := range []struct {
		name        string
		targetJobID func(*testing.T, aiAPIFixture) int64
	}{
		{
			name: "deleted target job",
			targetJobID: func(_ *testing.T, _ aiAPIFixture) int64 {
				return 999_999
			},
		},
		{
			name: "another tenant target job",
			targetJobID: func(t *testing.T, fixture aiAPIFixture) int64 {
				loginAs(t, fixture.router, "foreign-target@example.com")
				foreign, err := fixture.authRepository.GetUserByEmail(context.Background(), "foreign-target@example.com")
				require.NoError(t, err)
				target, err := fixture.targetService.Create(context.Background(), foreign.TenantID, targetjob.CreateInput{
					Company:      "不可见公司",
					Position:     "不可见职位",
					Requirements: "该岗位上下文绝不能跨租户泄露",
				})
				require.NoError(t, err)
				return target.ID
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := llmtesting.NewFake()
			fake.FinalJSON = `{"markdown":"new"}`
			fixture := setupAIAPI(t, ai.NewService(fake, nil, ai.WithMaxRetries(0)))
			cookie := loginAs(t, fixture.router, "missing-target@example.com")
			user, err := fixture.authRepository.GetUserByEmail(context.Background(), "missing-target@example.com")
			require.NoError(t, err)
			targetJobID := tc.targetJobID(t, fixture)
			item := createAIResume(t, fixture, user.Email, resumeContentForAI(&targetJobID, "summary", `{"markdown":"old"}`))

			response := aiRequest(t, fixture.router, "/api/ai/resumes/"+itoa(item.ID)+"/section/rewrite", `{"sectionType":"summary"}`, &http.Cookie{Name: "resume_session", Value: cookie})

			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			assert.Equal(t, []string{"suggestion", "done"}, frameNames(parseSSEFrames(t, response.Body.String())))
			require.Len(t, fake.RecordedInputs, 1)
			var promptInput struct {
				TargetJob *ai.TargetJobContext `json:"targetJob"`
			}
			require.NoError(t, json.Unmarshal([]byte(fake.RecordedInputs[0][1].Content), &promptInput))
			assert.Nil(t, promptInput.TargetJob)
			assert.NotContains(t, fake.RecordedInputs[0][1].Content, "不可见")
		})
	}
}

func TestAIRewriteStreamsErrorThenDoneForAsyncLLMFailure(t *testing.T) {
	fake := llmtesting.NewFake()
	fake.ErrToReturn = errors.New("upstream unavailable")
	fixture := setupAIAPI(t, ai.NewService(fake, nil, ai.WithMaxRetries(0)))
	cookie := loginAs(t, fixture.router, "ai-error@example.com")
	item := createAIResume(t, fixture, "ai-error@example.com", resumeContentForAI(nil, "summary", `{"markdown":"old"}`))

	response := aiRequest(t, fixture.router, "/api/ai/resumes/"+itoa(item.ID)+"/section/rewrite", `{"sectionType":"summary"}`, &http.Cookie{Name: "resume_session", Value: cookie})

	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assertSSEHeaders(t, response)
	frames := parseSSEFrames(t, response.Body.String())
	require.Len(t, frames, 2)
	assert.Equal(t, []string{"error", "done"}, frameNames(frames))
	assert.JSONEq(t, `{"code":"LLM_ERROR","message":"upstream unavailable"}`, frames[0].Data)
	assert.JSONEq(t, `{}`, frames[1].Data)
}

func TestAIRewriteReturnsJSONWhenStreamCannotStart(t *testing.T) {
	fixture := setupAIAPI(t, ai.NewService(nil, nil))
	cookie := loginAs(t, fixture.router, "ai-start-error@example.com")
	item := createAIResume(t, fixture, "ai-start-error@example.com", resumeContentForAI(nil, "summary", `{"markdown":"old"}`))

	response := aiRequest(t, fixture.router, "/api/ai/resumes/"+itoa(item.ID)+"/section/rewrite", `{"sectionType":"summary"}`, &http.Cookie{Name: "resume_session", Value: cookie})

	assert.Equal(t, http.StatusInternalServerError, response.Code)
	assert.Equal(t, "application/json", response.Header().Get("Content-Type"))
	assert.JSONEq(t, `{"error":"could not start AI rewrite"}`, response.Body.String())
	assert.NotContains(t, response.Body.String(), "chat model")
}

func TestAIGenerateStreamsNotImplementedErrorAndDone(t *testing.T) {
	fixture := setupAIAPI(t, ai.NewService(llmtesting.NewFake(), nil))
	cookie := loginAs(t, fixture.router, "ai-generate@example.com")

	response := aiRequest(t, fixture.router, "/api/ai/resumes/generate", `{}`, &http.Cookie{Name: "resume_session", Value: cookie})

	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assertSSEHeaders(t, response)
	frames := parseSSEFrames(t, response.Body.String())
	require.Len(t, frames, 2)
	assert.Equal(t, []string{"error", "done"}, frameNames(frames))
	assert.JSONEq(t, `{"code":"NOT_IMPLEMENTED","message":"from-scratch generate is TODO"}`, frames[0].Data)
	assert.JSONEq(t, `{}`, frames[1].Data)
}

func TestAIRewriteRejectsInvalidRequestsBeforeCallingAI(t *testing.T) {
	fake := llmtesting.NewFake()
	fake.FinalJSON = `{"markdown":"new"}`
	fixture := setupAIAPI(t, ai.NewService(fake, nil, ai.WithMaxRetries(0)))
	cookie := loginAs(t, fixture.router, "ai-validation@example.com")
	item := createAIResume(t, fixture, "ai-validation@example.com", resumeContentForAI(nil, "summary", `{"markdown":"old"}`))
	session := &http.Cookie{Name: "resume_session", Value: cookie}

	for _, tc := range []struct {
		name       string
		path       string
		body       string
		wantStatus int
	}{
		{name: "malformed body", path: "/api/ai/resumes/" + itoa(item.ID) + "/section/rewrite", body: `{`, wantStatus: http.StatusBadRequest},
		{name: "unsupported section", path: "/api/ai/resumes/" + itoa(item.ID) + "/section/rewrite", body: `{"sectionType":"education"}`, wantStatus: http.StatusBadRequest},
		{name: "invalid resume id", path: "/api/ai/resumes/not-an-id/section/rewrite", body: `{"sectionType":"summary"}`, wantStatus: http.StatusNotFound},
		{name: "section absent", path: "/api/ai/resumes/" + itoa(item.ID) + "/section/rewrite", body: `{"sectionType":"project"}`, wantStatus: http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := aiRequest(t, fixture.router, tc.path, tc.body, session)
			assert.Equal(t, tc.wantStatus, response.Code, response.Body.String())
			assert.NotContains(t, response.Header().Get("Content-Type"), "text/event-stream")
		})
	}
	assert.Empty(t, fake.RecordedInputs)
}

func TestAIRewriteRejectsEmptySupportedSections(t *testing.T) {
	fake := llmtesting.NewFake()
	fake.FinalJSON = `{"markdown":"new"}`
	fixture := setupAIAPI(t, ai.NewService(fake, nil, ai.WithMaxRetries(0)))
	cookie := loginAs(t, fixture.router, "ai-empty-section@example.com")
	session := &http.Cookie{Name: "resume_session", Value: cookie}

	for _, tc := range []struct {
		sectionType string
		content     string
	}{
		{sectionType: "summary", content: `{"markdown":"  "}`},
		{sectionType: "work", content: `{"items":[]}`},
		{sectionType: "internship", content: `{"items":[]}`},
		{sectionType: "project", content: `{"items":[]}`},
	} {
		t.Run(tc.sectionType, func(t *testing.T) {
			item := createAIResume(t, fixture, "ai-empty-section@example.com", resumeContentForAI(nil, tc.sectionType, tc.content))
			response := aiRequest(t, fixture.router, "/api/ai/resumes/"+itoa(item.ID)+"/section/rewrite", `{"sectionType":"`+tc.sectionType+`"}`, session)

			assert.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
			assert.JSONEq(t, `{"error":"section not found or empty"}`, response.Body.String())
		})
	}
	assert.Empty(t, fake.RecordedInputs)
}

func createAIResume(t *testing.T, fixture aiAPIFixture, email, content string) resume.Resume {
	t.Helper()
	user, err := fixture.authRepository.GetUserByEmail(context.Background(), email)
	require.NoError(t, err)
	item, err := fixture.resumeRepo.Create(context.Background(), resume.CreateInput{
		TenantID:   user.TenantID,
		Title:      "AI Resume",
		TemplateID: "minimal",
		Content:    content,
	})
	require.NoError(t, err)
	return item
}

func resumeContentForAI(targetJobID *int64, sectionType, content string) string {
	target := ""
	if targetJobID != nil {
		target = `,"targetJobId":` + itoa(*targetJobID)
	}
	return `{"schemaVersion":2,"sections":[{"id":"` + sectionType + `","type":"` + sectionType + `","content":` + content + `}]` + target + `}`
}

func aiRequest(t *testing.T, router http.Handler, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

type sseFrame struct {
	Event string
	Data  string
}

func parseSSEFrames(t *testing.T, body string) []sseFrame {
	t.Helper()
	require.NotEmpty(t, body)
	require.True(t, strings.HasSuffix(body, "\n\n"), "SSE stream must end each event with a blank line: %q", body)
	parts := strings.Split(strings.TrimSuffix(body, "\n\n"), "\n\n")
	frames := make([]sseFrame, 0, len(parts))
	for _, part := range parts {
		lines := strings.Split(part, "\n")
		require.Len(t, lines, 2, "SSE frame must have exactly event and data lines: %q", part)
		require.True(t, strings.HasPrefix(lines[0], "event: "), "missing event line: %q", part)
		require.True(t, strings.HasPrefix(lines[1], "data: "), "missing data line: %q", part)
		data := strings.TrimPrefix(lines[1], "data: ")
		assert.True(t, json.Valid([]byte(data)), "SSE data must be one JSON value: %q", data)
		frames = append(frames, sseFrame{Event: strings.TrimPrefix(lines[0], "event: "), Data: data})
	}
	return frames
}

func frameNames(frames []sseFrame) []string {
	names := make([]string, len(frames))
	for index, frame := range frames {
		names[index] = frame.Event
	}
	return names
}

func assertSSEHeaders(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	assert.Equal(t, "text/event-stream", response.Header().Get("Content-Type"))
	assert.Equal(t, "no-cache", response.Header().Get("Cache-Control"))
	assert.Equal(t, "keep-alive", response.Header().Get("Connection"))
	assert.Equal(t, "no", response.Header().Get("X-Accel-Buffering"))
}
