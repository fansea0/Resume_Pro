package llm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

var (
	ErrMissingAPIKey     = errors.New("OPENAI_API_KEY is required for AI features")
	ErrInvalidMaxRetries = errors.New("AI_MAX_RETRIES must be an integer from 0 to 10")
	defaultBaseURL       = "https://api.deepseek.com/v1"
	defaultModel         = "deepseek-chat"
	defaultMaxTokens     = 2048
	defaultMaxRetries    = 1
	maxRetriesLimit      = 10
	defaultTimeout       = 30 * time.Second
)

type Config struct {
	APIKey     string
	BaseURL    string
	Model      string
	MaxTokens  int
	MaxRetries int
	Timeout    time.Duration
}

func LoadConfigFromEnv() (Config, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return Config{}, ErrMissingAPIKey
	}

	cfg := Config{
		APIKey:     apiKey,
		BaseURL:    envOrDefault("OPENAI_BASE_URL", defaultBaseURL),
		Model:      envOrDefault("OPENAI_MODEL", defaultModel),
		MaxTokens:  defaultMaxTokens,
		MaxRetries: defaultMaxRetries,
		Timeout:    defaultTimeout,
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
	if value := os.Getenv("AI_MAX_RETRIES"); value != "" {
		maxRetries, err := strconv.Atoi(value)
		if err != nil || maxRetries < 0 || maxRetries > maxRetriesLimit {
			return Config{}, fmt.Errorf("%w: %q", ErrInvalidMaxRetries, value)
		}
		cfg.MaxRetries = maxRetries
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
	chatModel, err := openai.NewChatModel(context.Background(), &openai.ChatModelConfig{
		BaseURL:   cfg.BaseURL,
		APIKey:    cfg.APIKey,
		Model:     cfg.Model,
		MaxTokens: &cfg.MaxTokens,
		Timeout:   cfg.Timeout,
	})
	if err != nil {
		return nil, err
	}
	return &structuredOutputChatModel{ChatModel: chatModel}, nil
}

type structuredOutputChatModel struct {
	model.ChatModel
}

func (m *structuredOutputChatModel) Generate(ctx context.Context, input []*schema.Message, options ...model.Option) (*schema.Message, error) {
	return m.ChatModel.Generate(ctx, input, translateCallOptions(options)...)
}

func (m *structuredOutputChatModel) Stream(ctx context.Context, input []*schema.Message, options ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return m.ChatModel.Stream(ctx, input, translateCallOptions(options)...)
}

func translateCallOptions(options []model.Option) []model.Option {
	callOptions := GetCallOptions(options...)
	if callOptions.ResponseFormat == nil {
		return options
	}
	translated := append([]model.Option(nil), options...)
	return append(translated, openai.WithExtraFields(responseFormatExtraFields(callOptions.ResponseFormat)))
}

func responseFormatExtraFields(format *JSONSchemaResponseFormat) map[string]any {
	return map[string]any{
		"response_format": map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":        format.Name,
				"description": format.Description,
				"strict":      format.Strict,
				"schema":      format.Schema,
			},
		},
	}
}

var _ model.ChatModel = (*structuredOutputChatModel)(nil)
