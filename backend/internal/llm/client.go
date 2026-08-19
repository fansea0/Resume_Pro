package llm

import (
	"context"
	"errors"
	"os"
	"strconv"
	"time"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
)

var (
	ErrMissingAPIKey = errors.New("OPENAI_API_KEY is required for AI features")
	defaultBaseURL   = "https://api.deepseek.com/v1"
	defaultModel     = "deepseek-chat"
	defaultMaxTokens = 2048
	defaultTimeout   = 30 * time.Second
)

type Config struct {
	APIKey    string
	BaseURL   string
	Model     string
	MaxTokens int
	Timeout   time.Duration
}

func LoadConfigFromEnv() (Config, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return Config{}, ErrMissingAPIKey
	}

	cfg := Config{
		APIKey:    apiKey,
		BaseURL:   envOrDefault("OPENAI_BASE_URL", defaultBaseURL),
		Model:     envOrDefault("OPENAI_MODEL", defaultModel),
		MaxTokens: defaultMaxTokens,
		Timeout:   defaultTimeout,
	}
	if value := os.Getenv("AI_MAX_TOKENS"); value != "" {
		if maxTokens, err := strconv.Atoi(value); err == nil && maxTokens > 0 {
			cfg.MaxTokens = maxTokens
		}
	}
	if value := os.Getenv("AI_REQUEST_TIMEOUT"); value != "" {
		if timeout, err := time.ParseDuration(value); err == nil && timeout > 0 {
			cfg.Timeout = timeout
		}
	}

	return cfg, nil
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func NewChatModel(cfg Config) (model.ChatModel, error) {
	return openai.NewChatModel(context.Background(), &openai.ChatModelConfig{
		BaseURL:   cfg.BaseURL,
		APIKey:    cfg.APIKey,
		Model:     cfg.Model,
		MaxTokens: &cfg.MaxTokens,
		Timeout:   cfg.Timeout,
	})
}
