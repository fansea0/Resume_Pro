package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/jmoiron/sqlx"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"resumepro/backend/internal/ai"
	"resumepro/backend/internal/auth"
	"resumepro/backend/internal/httpapi"
	"resumepro/backend/internal/llm"
	llmtesting "resumepro/backend/internal/llm/testing"
	"resumepro/backend/internal/resume"
	"resumepro/backend/internal/store"
	"resumepro/backend/internal/templates"
)

const testJWTSecret = "test-secret-32-chars-minimum-len-padding"

func TestBuildRouterConfigDevelopmentWithoutAPIKeyKeepsAuthenticatedAIRoutesAvailable(t *testing.T) {
	db := openServerTestDB(t)
	resumeRepository := resume.NewRepository(db)
	config, err := buildRouterConfig(db, resumeRepository, logrus.New(), serverOptions{
		JWTSecret:       testJWTSecret,
		AppEnv:          "development",
		UploadDirectory: t.TempDir(),
	}, aiBootstrap{
		LoadConfig: func() (llm.Config, error) { return llm.Config{}, llm.ErrMissingAPIKey },
		NewChatModel: func(llm.Config) (model.ChatModel, error) {
			t.Fatal("chat model factory called without an API key")
			return nil, nil
		},
	})
	require.NoError(t, err)

	authRepository := auth.NewRepository(db)
	router := httpapi.NewRouter(authRepository, resumeRepository, templates.NewRepository(db), config)
	cookie := authenticatedCookie(t, authRepository)
	request := httptest.NewRequest(http.MethodPost, "/api/ai/resumes/generate", bytes.NewBufferString(`{}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(cookie)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusServiceUnavailable, response.Code)
	assert.Equal(t, "application/json", response.Header().Get("Content-Type"))
	assert.JSONEq(t, `{"error":"AI features disabled"}`, response.Body.String())
}

func TestBuildRouterConfigProductionWithoutAPIKeyReturnsStartupError(t *testing.T) {
	db := openServerTestDB(t)
	resumeRepository := resume.NewRepository(db)

	_, err := buildRouterConfig(db, resumeRepository, logrus.New(), serverOptions{
		JWTSecret:       testJWTSecret,
		AppEnv:          "production",
		UploadDirectory: t.TempDir(),
	}, aiBootstrap{
		LoadConfig:   func() (llm.Config, error) { return llm.Config{}, llm.ErrMissingAPIKey },
		NewChatModel: func(llm.Config) (model.ChatModel, error) { return llmtesting.NewFake(), nil },
	})

	require.ErrorIs(t, err, llm.ErrMissingAPIKey)
	assert.Contains(t, err.Error(), "AI features required in production")
}

func TestBuildRouterConfigPassesConfiguredRetryLimitToAIService(t *testing.T) {
	db := openServerTestDB(t)
	resumeRepository := resume.NewRepository(db)
	chatModel := &sequenceChatModel{
		FakeChatModel: llmtesting.NewFake(),
		responses: []string{
			`{"markdown":"invalid","invented":true}`,
			`{"markdown":"still invalid","invented":true}`,
			`{"markdown":"valid on configured retry"}`,
		},
	}
	config, err := buildRouterConfig(db, resumeRepository, logrus.New(), serverOptions{
		JWTSecret:       testJWTSecret,
		AppEnv:          "development",
		UploadDirectory: t.TempDir(),
	}, aiBootstrap{
		LoadConfig: func() (llm.Config, error) {
			return llm.Config{APIKey: "offline-test-key", Model: "offline", MaxRetries: 2}, nil
		},
		NewChatModel: func(llm.Config) (model.ChatModel, error) { return chatModel, nil },
	})
	require.NoError(t, err)
	require.NotNil(t, config.AIService)

	stream, err := config.AIService.RewriteSection(context.Background(), ai.RewriteInput{
		SectionType:    "summary",
		CurrentContent: json.RawMessage(`{"markdown":"old"}`),
	})
	require.NoError(t, err)
	events := collectAIEvents(t, stream)

	require.Len(t, events, 2)
	assert.Equal(t, "suggestion", events[0].Kind)
	assert.JSONEq(t, `{"markdown":"valid on configured retry"}`, string(events[0].Suggestion))
	assert.Equal(t, "done", events[1].Kind)
}

func openServerTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	db, err := sqlx.Open("sqlite", filepath.Join(t.TempDir(), "server.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, store.Migrate(context.Background(), db))
	return db
}

func authenticatedCookie(t *testing.T, repository *auth.Repository) *http.Cookie {
	t.Helper()
	service, err := auth.NewService(repository, testJWTSecret)
	require.NoError(t, err)
	user, err := service.Register(context.Background(), auth.CreateUserInput{
		Email:    "server-ai-disabled@example.com",
		Password: "correct horse battery staple",
	})
	require.NoError(t, err)
	token, err := service.IssueToken(user)
	require.NoError(t, err)
	return &http.Cookie{Name: "resume_session", Value: token}
}

func collectAIEvents(t *testing.T, stream <-chan ai.Event) []ai.Event {
	t.Helper()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	var events []ai.Event
	for {
		select {
		case event, ok := <-stream:
			if !ok {
				return events
			}
			events = append(events, event)
		case <-timer.C:
			t.Fatal("timeout waiting for AI events")
		}
	}
}

type sequenceChatModel struct {
	*llmtesting.FakeChatModel
	responses []string
	next      int
}

func (m *sequenceChatModel) Stream(ctx context.Context, input []*schema.Message, options ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	if len(m.responses) == 0 {
		return nil, errors.New("sequence chat model has no responses")
	}
	index := m.next
	if index >= len(m.responses) {
		index = len(m.responses) - 1
	}
	m.next++
	m.FinalJSON = m.responses[index]
	return m.FakeChatModel.Stream(ctx, input, options...)
}
