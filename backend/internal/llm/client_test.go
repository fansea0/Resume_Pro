package llm_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"resumepro/backend/internal/llm"
)

func TestLoadConfigFromEnv_MissingAPIKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")

	_, err := llm.LoadConfigFromEnv()

	assert.ErrorIs(t, err, llm.ErrMissingAPIKey)
}

func TestLoadConfigFromEnv_Defaults(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("OPENAI_BASE_URL", "")
	t.Setenv("OPENAI_MODEL", "")
	t.Setenv("AI_MAX_TOKENS", "")
	t.Setenv("AI_REQUEST_TIMEOUT", "")
	t.Setenv("AI_MAX_RETRIES", "")

	cfg, err := llm.LoadConfigFromEnv()

	require.NoError(t, err)
	assert.Equal(t, "test-key", cfg.APIKey)
	assert.Equal(t, "https://api.deepseek.com/v1", cfg.BaseURL)
	assert.Equal(t, "deepseek-chat", cfg.Model)
	assert.Equal(t, 2048, cfg.MaxTokens)
	assert.Equal(t, 30*time.Second, cfg.Timeout)
	assert.Equal(t, 1, cfg.MaxRetries)
}

func TestLoadConfigFromEnv_OverridesDefaults(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "custom-key")
	t.Setenv("OPENAI_BASE_URL", "https://llm.example.test/v1")
	t.Setenv("OPENAI_MODEL", "custom-model")
	t.Setenv("AI_MAX_TOKENS", "4096")
	t.Setenv("AI_REQUEST_TIMEOUT", "45s")
	t.Setenv("AI_MAX_RETRIES", "3")

	cfg, err := llm.LoadConfigFromEnv()

	require.NoError(t, err)
	assert.Equal(t, "custom-key", cfg.APIKey)
	assert.Equal(t, "https://llm.example.test/v1", cfg.BaseURL)
	assert.Equal(t, "custom-model", cfg.Model)
	assert.Equal(t, 4096, cfg.MaxTokens)
	assert.Equal(t, 45*time.Second, cfg.Timeout)
	assert.Equal(t, 3, cfg.MaxRetries)
}

func TestLoadConfigFromEnv_RejectsInvalidMaxRetries(t *testing.T) {
	for _, value := range []string{"not-a-number", "-1", "11"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("OPENAI_API_KEY", "test-key")
			t.Setenv("AI_MAX_RETRIES", value)

			_, err := llm.LoadConfigFromEnv()

			assert.ErrorIs(t, err, llm.ErrInvalidMaxRetries)
		})
	}
}

func TestLoadConfigFromEnv_IgnoresNonPositiveOrMalformedTuningValues(t *testing.T) {
	testCases := []struct {
		name      string
		maxTokens string
		timeout   string
	}{
		{name: "malformed", maxTokens: "not-a-number", timeout: "not-a-duration"},
		{name: "zero", maxTokens: "0", timeout: "0s"},
		{name: "negative", maxTokens: "-1", timeout: "-1s"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv("OPENAI_API_KEY", "test-key")
			t.Setenv("AI_MAX_TOKENS", testCase.maxTokens)
			t.Setenv("AI_REQUEST_TIMEOUT", testCase.timeout)

			cfg, err := llm.LoadConfigFromEnv()

			require.NoError(t, err)
			assert.Equal(t, 2048, cfg.MaxTokens)
			assert.Equal(t, 30*time.Second, cfg.Timeout)
		})
	}
}

func TestNewChatModel_ConstructsWithoutCallingProvider(t *testing.T) {
	cfg := llm.Config{
		APIKey:    "test-key",
		BaseURL:   "https://llm.example.test/v1",
		Model:     "test-model",
		MaxTokens: 128,
		Timeout:   time.Second,
	}

	chatModel, err := llm.NewChatModel(cfg)

	require.NoError(t, err)
	assert.NotNil(t, chatModel)
}

func TestNewChatModel_SerializesStrictJSONSchemaResponseFormat(t *testing.T) {
	requests := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/v1/chat/completions" {
			http.Error(response, "unexpected request", http.StatusNotFound)
			return
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			http.Error(response, err.Error(), http.StatusBadRequest)
			return
		}
		requests <- body
		response.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(response, `{
			"id":"chatcmpl-local",
			"object":"chat.completion",
			"created":1,
			"model":"test-model",
			"choices":[{
				"index":0,
				"message":{"role":"assistant","content":"{\"markdown\":\"new\"}"},
				"finish_reason":"stop"
			}],
			"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}
		}`)
	}))
	t.Cleanup(server.Close)

	chatModel, err := llm.NewChatModel(llm.Config{
		APIKey:    "test-key",
		BaseURL:   server.URL + "/v1",
		Model:     "test-model",
		MaxTokens: 128,
		Timeout:   time.Second,
	})
	require.NoError(t, err)

	response, err := chatModel.Generate(context.Background(), []*schema.Message{
		{Role: schema.User, Content: "rewrite"},
	}, llm.WithJSONSchemaResponse(llm.JSONSchemaResponseFormat{
		Name:   "summary_rewrite",
		Strict: true,
		Schema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"markdown": map[string]any{"type": "string"},
			},
			"required": []string{"markdown"},
		},
	}))
	require.NoError(t, err)
	require.NotNil(t, response)

	var requestPayload struct {
		ResponseFormat struct {
			Type       string `json:"type"`
			JSONSchema struct {
				Name   string         `json:"name"`
				Strict bool           `json:"strict"`
				Schema map[string]any `json:"schema"`
			} `json:"json_schema"`
		} `json:"response_format"`
	}
	require.NoError(t, json.Unmarshal(<-requests, &requestPayload))
	assert.Equal(t, "json_schema", requestPayload.ResponseFormat.Type)
	assert.Equal(t, "summary_rewrite", requestPayload.ResponseFormat.JSONSchema.Name)
	assert.True(t, requestPayload.ResponseFormat.JSONSchema.Strict)
	assert.Equal(t, "object", requestPayload.ResponseFormat.JSONSchema.Schema["type"])
	assert.Equal(t, false, requestPayload.ResponseFormat.JSONSchema.Schema["additionalProperties"])
	properties, ok := requestPayload.ResponseFormat.JSONSchema.Schema["properties"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, map[string]any{"type": "string"}, properties["markdown"])
	assert.Equal(t, []any{"markdown"}, requestPayload.ResponseFormat.JSONSchema.Schema["required"])
}
