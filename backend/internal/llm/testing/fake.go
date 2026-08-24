// Package testing provides offline test doubles for LLM integrations.
package testing

import (
	"context"
	"sync"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"resumepro/backend/internal/llm"
)

// FakeChatModel is a deterministic, in-memory model for AI tests.
// It never performs network I/O.
type FakeChatModel struct {
	mu                         sync.Mutex
	RecordedInputs             [][]*schema.Message
	RecordedCallOptions        []llm.CallOptions
	StreamChunks               []string
	FinalJSON                  string
	ThinkingText               string
	ResponseUsage              *schema.TokenUsage
	ErrToReturn                error
	WaitForContextCancellation bool
}

// FakeOption configures a FakeChatModel returned by NewFake.
type FakeOption func(*FakeChatModel)

// NewFake constructs an offline FakeChatModel.
func NewFake(opts ...FakeOption) *FakeChatModel {
	fake := &FakeChatModel{}
	for _, opt := range opts {
		if opt != nil {
			opt(fake)
		}
	}
	return fake
}

// Generate records the request and returns the configured final response.
func (f *FakeChatModel) Generate(ctx context.Context, input []*schema.Message, options ...model.Option) (*schema.Message, error) {
	finalJSON, thinkingText, usage, waitForCancellation, err := f.recordAndSnapshot(input, options)
	if err != nil {
		return nil, err
	}
	if waitForCancellation {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return &schema.Message{
		Role:             schema.Assistant,
		Content:          finalJSON,
		ReasoningContent: thinkingText,
		ResponseMeta:     responseMeta(usage),
	}, nil
}

// Stream records the request and returns configured thinking chunks followed by
// the configured final response.
func (f *FakeChatModel) Stream(ctx context.Context, input []*schema.Message, options ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	finalJSON, thinkingText, usage, waitForCancellation, err := f.recordAndSnapshot(input, options)
	if err != nil {
		return nil, err
	}
	if waitForCancellation {
		<-ctx.Done()
		return nil, ctx.Err()
	}

	f.mu.Lock()
	chunks := append([]string(nil), f.StreamChunks...)
	f.mu.Unlock()

	messages := make([]*schema.Message, 0, len(chunks)+2)
	if thinkingText != "" {
		messages = append(messages, &schema.Message{Role: schema.Assistant, ReasoningContent: thinkingText})
	}
	for _, chunk := range chunks {
		messages = append(messages, &schema.Message{Role: schema.Assistant, ReasoningContent: chunk})
	}
	messages = append(messages, &schema.Message{Role: schema.Assistant, Content: finalJSON, ResponseMeta: responseMeta(usage)})

	return schema.StreamReaderFromArray(messages), nil
}

// BindTools satisfies Eino's deprecated ChatModel interface. Tool definitions
// do not affect deterministic fake responses.
func (f *FakeChatModel) BindTools(_ []*schema.ToolInfo) error {
	return nil
}

func (f *FakeChatModel) recordAndSnapshot(input []*schema.Message, options []model.Option) (finalJSON, thinkingText string, usage *schema.TokenUsage, waitForCancellation bool, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.RecordedInputs = append(f.RecordedInputs, input)
	f.RecordedCallOptions = append(f.RecordedCallOptions, llm.GetCallOptions(options...))
	if f.ResponseUsage != nil {
		usageCopy := *f.ResponseUsage
		usage = &usageCopy
	}
	return f.FinalJSON, f.ThinkingText, usage, f.WaitForContextCancellation, f.ErrToReturn
}

func responseMeta(usage *schema.TokenUsage) *schema.ResponseMeta {
	if usage == nil {
		return nil
	}
	return &schema.ResponseMeta{Usage: usage}
}

var _ model.ChatModel = (*FakeChatModel)(nil)
