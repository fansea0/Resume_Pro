package llm_test

import (
	"testing"
	"time"

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

	cfg, err := llm.LoadConfigFromEnv()

	require.NoError(t, err)
	assert.Equal(t, "test-key", cfg.APIKey)
	assert.Equal(t, "https://api.deepseek.com/v1", cfg.BaseURL)
	assert.Equal(t, "deepseek-chat", cfg.Model)
	assert.Equal(t, 2048, cfg.MaxTokens)
	assert.Equal(t, 30*time.Second, cfg.Timeout)
}

func TestLoadConfigFromEnv_OverridesDefaults(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "custom-key")
	t.Setenv("OPENAI_BASE_URL", "https://llm.example.test/v1")
	t.Setenv("OPENAI_MODEL", "custom-model")
	t.Setenv("AI_MAX_TOKENS", "4096")
	t.Setenv("AI_REQUEST_TIMEOUT", "45s")

	cfg, err := llm.LoadConfigFromEnv()

	require.NoError(t, err)
	assert.Equal(t, "custom-key", cfg.APIKey)
	assert.Equal(t, "https://llm.example.test/v1", cfg.BaseURL)
	assert.Equal(t, "custom-model", cfg.Model)
	assert.Equal(t, 4096, cfg.MaxTokens)
	assert.Equal(t, 45*time.Second, cfg.Timeout)
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
