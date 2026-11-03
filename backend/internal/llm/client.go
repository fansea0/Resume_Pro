package llm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/eino-ext/components/model/agenticopenai"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

var (
	ErrMissingAPIKey     = errors.New("OPENAI_API_KEY is required for AI features")
	ErrInvalidMaxRetries = errors.New("AI_MAX_RETRIES must be an integer from 0 to 10")
	ErrInvalidReasoning  = errors.New("AI_REASONING_EFFORT must be one of low, medium, high, xhigh, max")
	defaultBaseURL       = "https://api.deepseek.com/v1"
	defaultModel         = "deepseek-chat"
	defaultMaxTokens     = 4096
	defaultMaxRetries    = 1
	maxRetriesLimit      = 10
	defaultTimeout       = 60 * time.Second
)

// allowedReasoningEfforts lists the reasoning effort levels accepted by the
// OpenAI-compatible providers this application targets. Reasoning models
// otherwise spend their whole token budget on hidden thinking and return a
// truncated answer.
var allowedReasoningEfforts = map[string]struct{}{
	"low":    {},
	"medium": {},
	"high":   {},
	"xhigh":  {},
	"max":    {},
}

type Config struct {
	APIKey     string
	BaseURL    string
	Model      string
	MaxTokens  int
	MaxRetries int
	Timeout    time.Duration
	// ReasoningEffort is sent as the reasoning_effort request field. An empty
	// value leaves the provider default untouched.
	ReasoningEffort string
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
	if value := strings.TrimSpace(os.Getenv("AI_REASONING_EFFORT")); value != "" {
		if _, ok := allowedReasoningEfforts[value]; !ok {
			return Config{}, fmt.Errorf("%w: %q", ErrInvalidReasoning, value)
		}
		cfg.ReasoningEffort = value
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
	return &structuredOutputChatModel{ChatModel: chatModel, reasoningEffort: cfg.ReasoningEffort}, nil
}

// NewResponsesModel creates an agenticopenai Responses API model from config.
// The returned model satisfies model.AgenticModel (Generate/Stream over
// *schema.AgenticMessage). Compile-time wiring only for now; callers route
// interview question generation through it.
func NewResponsesModel(cfg Config) (model.AgenticModel, error) {
	responsesConfig := &agenticopenai.ResponsesConfig{
		APIKey:    cfg.APIKey,
		BaseURL:   cfg.BaseURL,
		Model:     cfg.Model,
		MaxTokens: &cfg.MaxTokens,
		Timeout:   &cfg.Timeout,
		// 不设则沿用 SDK 默认的 2 次静默重试，单次挂起会按超时时长翻倍放大。
		MaxRetries: &cfg.MaxRetries,
	}
	// MiniMax-M3.1-Flash 默认思考档位=max，会把 max_output_tokens 整份额度烧在隐藏
	// 思考上导致正文（生题 JSON）被截断，因此必须显式降档。
	//
	// Responses 端点只认嵌套的 reasoning.effort：顶层 reasoning_effort 是 Chat
	// Completions 的字段，MiniMax 会静默忽略并按默认最高档跑。实测同一批 12 题：
	// 顶层字段 171s 且 JSON 截断报错，嵌套字段 15s 且思考仅 50 token。
	if cfg.ReasoningEffort != "" {
		responsesConfig.ExtraFields = map[string]any{
			"reasoning": map[string]any{"effort": cfg.ReasoningEffort},
		}
	}
	return agenticopenai.NewResponsesModel(context.Background(), responsesConfig)
}

type structuredOutputChatModel struct {
	model.ChatModel
	// reasoningEffort is sent as the top-level reasoning_effort request field.
	// It is applied here rather than through the provider config because the
	// per-call extra fields replace the configured ones.
	reasoningEffort string
}

func (m *structuredOutputChatModel) Generate(ctx context.Context, input []*schema.Message, options ...model.Option) (*schema.Message, error) {
	return m.ChatModel.Generate(ctx, input, m.translateCallOptions(options)...)
}

func (m *structuredOutputChatModel) Stream(ctx context.Context, input []*schema.Message, options ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return m.ChatModel.Stream(ctx, input, m.translateCallOptions(options)...)
}

// translateCallOptions converts the application-owned response format into the
// provider request fields. The reasoning effort is merged into the same extra
// field map because the provider replaces extra fields as a whole.
func (m *structuredOutputChatModel) translateCallOptions(options []model.Option) []model.Option {
	callOptions := GetCallOptions(options...)
	extraFields := map[string]any{}
	if callOptions.ResponseFormat != nil {
		extraFields = responseFormatExtraFields(callOptions.ResponseFormat)
	}
	if m.reasoningEffort != "" {
		extraFields["reasoning_effort"] = m.reasoningEffort
	}
	if len(extraFields) == 0 {
		return options
	}
	translated := append([]model.Option(nil), options...)
	return append(translated, openai.WithExtraFields(extraFields))
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
