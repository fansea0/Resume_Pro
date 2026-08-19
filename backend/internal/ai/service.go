package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/template"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"resumepro/backend/internal/ai/prompts"
	"resumepro/backend/internal/ai/schemas"
	"resumepro/backend/internal/llm"
)

const (
	defaultMaxSchemaRetries = 1
	maxSchemaRetries        = 10
)

// TargetJobContext is the AI-facing subset of a target job. HTTP adapters map
// persistence models to this DTO so the AI package remains independent of
// authentication and persistence concerns.
type TargetJobContext struct {
	Company      string `json:"company,omitempty"`
	Position     string `json:"position,omitempty"`
	Requirements string `json:"requirements"`
}

type Service struct {
	cm         model.ChatModel
	hooks      *Hooks
	tmpl       *template.Template
	maxRetries int
}

type ServiceOption func(*Service)

func WithMaxRetries(maxRetries int) ServiceOption {
	return func(service *Service) {
		if maxRetries >= 0 && maxRetries <= maxSchemaRetries {
			service.maxRetries = maxRetries
		}
	}
}

func NewService(cm model.ChatModel, hooks *Hooks, options ...ServiceOption) *Service {
	service := &Service{
		cm:         cm,
		hooks:      normalizedHooks(hooks),
		tmpl:       template.Must(template.ParseFS(prompts.FS, "*.tmpl")),
		maxRetries: defaultMaxSchemaRetries,
	}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

type RewriteInput struct {
	TenantID       int64
	ResumeID       int64
	SectionType    string
	CurrentContent json.RawMessage
	TargetJob      *TargetJobContext
	Instructions   string
}

type GenerateInput struct {
	TenantID       int64
	RawText        string
	TargetJob      *TargetJobContext
	JobDescription string
	TemplateID     string
}

type Event struct {
	Kind         string          `json:"kind"`
	ThinkingText string          `json:"thinkingText,omitempty"`
	Suggestion   json.RawMessage `json:"suggestion,omitempty"`
	ErrorCode    string          `json:"errorCode,omitempty"`
	ErrorMessage string          `json:"errorMessage,omitempty"`
}

func (s *Service) RewriteSection(ctx context.Context, in RewriteInput) (<-chan Event, error) {
	if s.cm == nil {
		return nil, errors.New("AI chat model is required")
	}

	outputSchemaValue, err := schemas.RewriteSchemaFor(in.SectionType)
	if err != nil {
		return nil, err
	}
	outputSchema, ok := outputSchemaValue.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid rewrite schema type: %T", outputSchemaValue)
	}
	sectionTemplate, err := sectionTemplateName(in.SectionType)
	if err != nil {
		return nil, err
	}

	ch := make(chan Event, 16)
	go s.rewrite(ctx, ch, in, sectionTemplate, outputSchema)
	return ch, nil
}

func (s *Service) rewrite(ctx context.Context, ch chan<- Event, in RewriteInput, sectionTemplate string, outputSchema map[string]any) {
	defer close(ch)

	startedAt := time.Now()
	callInfo := CallInfo{
		TenantID:    in.TenantID,
		Operation:   "section_rewrite",
		SectionType: in.SectionType,
	}
	s.hooks.OnCallStarted(ctx, callInfo)

	messages, err := s.buildRewriteMessages(in, sectionTemplate, outputSchema)
	if err != nil {
		s.fail(ctx, ch, callInfo, "PROMPT_BUILD_FAILED", err)
		return
	}

	tokensIn := 0
	tokensOut := 0
	responseFormat := llm.WithJSONSchemaResponse(llm.JSONSchemaResponseFormat{
		Name:   in.SectionType + "_rewrite",
		Strict: true,
		Schema: outputSchema,
	})
	for attempt := 0; attempt <= s.maxRetries; attempt++ {
		response, err := s.streamResponse(ctx, ch, messages, responseFormat)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			s.fail(ctx, ch, callInfo, "LLM_ERROR", err)
			return
		}
		tokensIn += response.tokensIn
		tokensOut += response.tokensOut

		if err := validateRewriteResponse([]byte(response.content), outputSchema); err != nil {
			if attempt < s.maxRetries {
				messages = repairMessages(messages, response.content, err)
				continue
			}
			s.fail(ctx, ch, callInfo, "INVALID_RESPONSE", err)
			return
		}

		s.hooks.OnCallFinished(ctx, callInfo, CallResult{
			DurationMS:   time.Since(startedAt).Milliseconds(),
			TokensIn:     tokensIn,
			TokensOut:    tokensOut,
			RetryCount:   attempt,
			SuggestionOK: true,
		})
		s.sendTerminal(ctx, ch, Event{Kind: "suggestion", Suggestion: json.RawMessage(response.content)})
		return
	}
}

func (s *Service) GenerateResume(ctx context.Context, _ GenerateInput) (<-chan Event, error) {
	ch := make(chan Event, 2)
	go func() {
		defer close(ch)
		s.sendTerminal(ctx, ch, Event{
			Kind:         "error",
			ErrorCode:    "NOT_IMPLEMENTED",
			ErrorMessage: "from-scratch generate is TODO",
		})
	}()
	return ch, nil
}

type streamResponse struct {
	content   string
	tokensIn  int
	tokensOut int
}

func (s *Service) streamResponse(ctx context.Context, ch chan<- Event, messages []*schema.Message, options ...model.Option) (streamResponse, error) {
	reader, err := s.cm.Stream(ctx, messages, options...)
	if err != nil {
		return streamResponse{}, err
	}
	defer reader.Close()

	var content strings.Builder
	var tokensIn int
	var tokensOut int
	for {
		message, err := reader.Recv()
		if errors.Is(err, io.EOF) {
			return streamResponse{content: content.String(), tokensIn: tokensIn, tokensOut: tokensOut}, nil
		}
		if err != nil {
			return streamResponse{}, err
		}
		if message == nil {
			return streamResponse{}, errors.New("LLM stream returned a nil message")
		}
		if err := ctx.Err(); err != nil {
			return streamResponse{}, err
		}
		if message.ResponseMeta != nil && message.ResponseMeta.Usage != nil {
			usage := message.ResponseMeta.Usage
			if usage.PromptTokens > tokensIn {
				tokensIn = usage.PromptTokens
			}
			if usage.CompletionTokens > tokensOut {
				tokensOut = usage.CompletionTokens
			}
		}
		if message.ReasoningContent != "" {
			if !s.emit(ctx, ch, Event{Kind: "thinking", ThinkingText: message.ReasoningContent}) {
				return streamResponse{}, ctx.Err()
			}
		}
		content.WriteString(message.Content)
	}
}

func (s *Service) buildRewriteMessages(in RewriteInput, sectionTemplate string, outputSchema map[string]any) ([]*schema.Message, error) {
	var rendered bytes.Buffer
	render := func(name string, data any) (string, error) {
		rendered.Reset()
		if err := s.tmpl.ExecuteTemplate(&rendered, name, data); err != nil {
			return "", err
		}
		return rendered.String(), nil
	}

	commonText, err := render("system_common.tmpl", nil)
	if err != nil {
		return nil, err
	}
	strategyText, err := render("rewriting_strategy.tmpl", nil)
	if err != nil {
		return nil, err
	}
	sectionText, err := render(sectionTemplate, map[string]string{"Title": in.SectionType})
	if err != nil {
		return nil, err
	}

	systemParts := []string{commonText, strategyText, sectionText}
	systemParts = append(systemParts, "输出必须严格符合以下 JSON schema：\n"+schemas.SchemaJSONString(outputSchema))

	userPayload, err := json.Marshal(struct {
		SectionType    string            `json:"sectionType"`
		CurrentContent json.RawMessage   `json:"currentContent"`
		TargetJob      *TargetJobContext `json:"targetJob,omitempty"`
		Instructions   string            `json:"instructions,omitempty"`
	}{
		SectionType:    in.SectionType,
		CurrentContent: in.CurrentContent,
		TargetJob:      in.TargetJob,
		Instructions:   in.Instructions,
	})
	if err != nil {
		return nil, fmt.Errorf("serialize untrusted rewrite context: %w", err)
	}

	return []*schema.Message{
		{Role: schema.System, Content: strings.Join(systemParts, "\n")},
		{Role: schema.User, Content: string(userPayload)},
	}, nil
}

func repairMessages(messages []*schema.Message, invalidContent string, validationErr error) []*schema.Message {
	repaired := append([]*schema.Message(nil), messages...)
	return append(repaired,
		&schema.Message{Role: schema.Assistant, Content: invalidContent},
		&schema.Message{
			Role: schema.User,
			Content: fmt.Sprintf(
				"上一次输出不符合 JSON schema：%s。请只返回修正后的 JSON，不要解释或使用 markdown 包裹。",
				validationErr,
			),
		},
	)
}

func sectionTemplateName(sectionType string) (string, error) {
	switch sectionType {
	case "summary":
		return "section_summary.tmpl", nil
	case "work", "internship":
		return "section_work.tmpl", nil
	case "project":
		return "section_project.tmpl", nil
	default:
		return "", fmt.Errorf("unsupported section type for AI rewrite: %s", sectionType)
	}
}

func (s *Service) fail(ctx context.Context, ch chan<- Event, info CallInfo, code string, err error) {
	s.hooks.OnCallFailed(ctx, info, err)
	s.sendTerminal(ctx, ch, Event{Kind: "error", ErrorCode: code, ErrorMessage: err.Error()})
}

func (s *Service) emit(ctx context.Context, ch chan<- Event, event Event) bool {
	if ctx.Err() != nil {
		return false
	}
	// Thinking is best-effort telemetry. Reserve two buffered slots for the
	// atomic terminal outcome so a slow consumer cannot stall completion.
	if cap(ch) >= 2 && len(ch) >= cap(ch)-2 {
		return true
	}
	select {
	case <-ctx.Done():
		return false
	case ch <- event:
		return true
	default:
		return true
	}
}

// sendTerminal makes the terminal outcome atomic from the service's point of
// view: cancellation observed before this call wins and emits nothing;
// otherwise the outcome and done events are emitted together in order.
func (s *Service) sendTerminal(ctx context.Context, ch chan<- Event, outcome Event) {
	if ctx.Err() != nil {
		return
	}
	ch <- outcome
	ch <- Event{Kind: "done"}
}

func normalizedHooks(hooks *Hooks) *Hooks {
	normalized := &Hooks{}
	if hooks != nil {
		*normalized = *hooks
	}
	if normalized.OnCallStarted == nil {
		normalized.OnCallStarted = func(context.Context, CallInfo) {}
	}
	if normalized.OnCallFinished == nil {
		normalized.OnCallFinished = func(context.Context, CallInfo, CallResult) {}
	}
	if normalized.OnCallFailed == nil {
		normalized.OnCallFailed = func(context.Context, CallInfo, error) {}
	}
	return normalized
}

// validateRewriteResponse validates only the JSON Schema keyword subset
// generated by internal/ai/schemas for rewrite responses: object, array,
// string and null types plus properties, required and additionalProperties.
// It is deliberately not a general-purpose JSON Schema validator.
func validateRewriteResponse(raw []byte, outputSchema map[string]any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	if err := ensureJSONEnd(decoder); err != nil {
		return err
	}
	return validateRewriteSchemaValue("$", value, outputSchema)
}

func ensureJSONEnd(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return fmt.Errorf("invalid trailing JSON: %w", err)
	}
	return errors.New("response contains more than one JSON value")
}

func validateRewriteSchemaValue(path string, value any, definition map[string]any) error {
	if !matchesRewriteSchemaType(value, definition["type"]) {
		return fmt.Errorf("%s: expected %v, got %T", path, definition["type"], value)
	}

	switch typed := value.(type) {
	case map[string]any:
		return validateRewriteSchemaObject(path, typed, definition)
	case []any:
		itemDefinition, ok := definition["items"].(map[string]any)
		if !ok {
			return nil
		}
		for index, item := range typed {
			if err := validateRewriteSchemaValue(fmt.Sprintf("%s[%d]", path, index), item, itemDefinition); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateRewriteSchemaObject(path string, value map[string]any, definition map[string]any) error {
	properties, _ := definition["properties"].(map[string]any)
	for _, required := range schemaStrings(definition["required"]) {
		if _, ok := value[required]; !ok {
			return fmt.Errorf("%s: required property %q is missing", path, required)
		}
	}
	if additional, ok := definition["additionalProperties"].(bool); ok && !additional {
		for name := range value {
			if _, ok := properties[name]; !ok {
				return fmt.Errorf("%s: additional property %q is not allowed", path, name)
			}
		}
	}
	for name, propertyValue := range value {
		propertyDefinition, ok := properties[name].(map[string]any)
		if !ok {
			continue
		}
		if err := validateRewriteSchemaValue(path+"."+name, propertyValue, propertyDefinition); err != nil {
			return err
		}
	}
	return nil
}

func matchesRewriteSchemaType(value any, definition any) bool {
	allowed := schemaStrings(definition)
	if len(allowed) == 0 {
		return true
	}
	for _, expected := range allowed {
		switch expected {
		case "null":
			if value == nil {
				return true
			}
		case "object":
			_, ok := value.(map[string]any)
			if ok {
				return true
			}
		case "array":
			_, ok := value.([]any)
			if ok {
				return true
			}
		case "string":
			_, ok := value.(string)
			if ok {
				return true
			}
		}
	}
	return false
}

func schemaStrings(value any) []string {
	switch typed := value.(type) {
	case string:
		return []string{typed}
	case []string:
		return typed
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				result = append(result, text)
			}
		}
		return result
	default:
		return nil
	}
}
