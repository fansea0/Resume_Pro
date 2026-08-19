// Package testing provides offline test doubles for LLM integrations.
package testing

import (
	"context"
	"sync"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// FakeChatModel is a deterministic, in-memory model for AI tests.
// It never performs network I/O.
type FakeChatModel struct {
	mu             sync.Mutex
	RecordedInputs [][]*schema.Message
	StreamChunks   []string
	FinalJSON      string
	ThinkingText   string
	ErrToReturn    error
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
func (f *FakeChatModel) Generate(_ context.Context, input []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	finalJSON, thinkingText, err := f.recordAndSnapshot(input)
	if err != nil {
		return nil, err
	}
	return &schema.Message{
		Role:             schema.Assistant,
		Content:          finalJSON,
		ReasoningContent: thinkingText,
	}, nil
}

// Stream records the request and returns configured thinking chunks followed by
// the configured final response.
func (f *FakeChatModel) Stream(_ context.Context, input []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	finalJSON, thinkingText, err := f.recordAndSnapshot(input)
	if err != nil {
		return nil, err
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
	messages = append(messages, &schema.Message{Role: schema.Assistant, Content: finalJSON})

	return schema.StreamReaderFromArray(messages), nil
}

// BindTools satisfies Eino's deprecated ChatModel interface. Tool definitions
// do not affect deterministic fake responses.
func (f *FakeChatModel) BindTools(_ []*schema.ToolInfo) error {
	return nil
}

func (f *FakeChatModel) recordAndSnapshot(input []*schema.Message) (finalJSON, thinkingText string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.RecordedInputs = append(f.RecordedInputs, input)
	return f.FinalJSON, f.ThinkingText, f.ErrToReturn
}

var _ model.ChatModel = (*FakeChatModel)(nil)
