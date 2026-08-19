package ai_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"resumepro/backend/internal/ai"
	lltesting "resumepro/backend/internal/llm/testing"
)

func TestService_RewriteSection_StreamsThinkingSuggestionAndDone(t *testing.T) {
	fake := lltesting.NewFake()
	fake.ThinkingText = "先分析岗位要求"
	fake.StreamChunks = []string{"，再压缩表达"}
	fake.FinalJSON = `{"items":[{"id":"a","introduction":null,"achievements":"newA","highlightsMarkdown":null}]}`
	svc := ai.NewService(fake, ai.NewLogrusHooks(logrus.New()))

	ch, err := svc.RewriteSection(context.Background(), ai.RewriteInput{
		TenantID:       1,
		ResumeID:       2,
		SectionType:    "project",
		CurrentContent: json.RawMessage(`{"items":[{"id":"a","name":"X","achievements":"old"}]}`),
	})
	require.NoError(t, err)

	events := collectEvents(t, ch)
	require.Len(t, events, 4)
	assert.Equal(t, ai.Event{Kind: "thinking", ThinkingText: "先分析岗位要求"}, events[0])
	assert.Equal(t, ai.Event{Kind: "thinking", ThinkingText: "，再压缩表达"}, events[1])
	assert.Equal(t, "suggestion", events[2].Kind)
	assert.JSONEq(t, fake.FinalJSON, string(events[2].Suggestion))
	assert.Equal(t, ai.Event{Kind: "done"}, events[3])
}

func TestService_RewriteSection_UpstreamErrorTerminatesWithErrorAndDone(t *testing.T) {
	fake := lltesting.NewFake()
	fake.ErrToReturn = errors.New("upstream down")
	svc := ai.NewService(fake, ai.NewLogrusHooks(logrus.New()))

	ch, err := svc.RewriteSection(context.Background(), ai.RewriteInput{
		TenantID:       1,
		SectionType:    "summary",
		CurrentContent: json.RawMessage(`{"markdown":"old"}`),
	})
	require.NoError(t, err)

	events := collectEvents(t, ch)
	require.Len(t, events, 2)
	assert.Equal(t, "error", events[0].Kind)
	assert.Equal(t, "LLM_ERROR", events[0].ErrorCode)
	assert.Contains(t, events[0].ErrorMessage, "upstream down")
	assert.Equal(t, "done", events[1].Kind)
}

func TestService_RewriteSection_ComposesTargetJobSchemaAndUserContext(t *testing.T) {
	fake := lltesting.NewFake()
	fake.FinalJSON = `{"markdown":"new"}`
	svc := ai.NewService(fake, ai.NewLogrusHooks(logrus.New()))

	ch, err := svc.RewriteSection(context.Background(), ai.RewriteInput{
		TenantID:       1,
		SectionType:    "summary",
		CurrentContent: json.RawMessage(`{"markdown":"old"}`),
		TargetJob: &ai.TargetJobContext{
			Company:      "字节",
			Position:     "后端",
			Requirements: "负责 ToB 营销中台后端架构设计",
		},
		Instructions: "突出 Go 经验",
	})
	require.NoError(t, err)
	collectEvents(t, ch)

	require.Len(t, fake.RecordedInputs, 1)
	require.Len(t, fake.RecordedInputs[0], 2)
	systemMessage := fake.RecordedInputs[0][0]
	userMessage := fake.RecordedInputs[0][1]
	assert.Equal(t, schema.System, systemMessage.Role)
	assert.Contains(t, systemMessage.Content, "字节")
	assert.Contains(t, systemMessage.Content, "ToB 营销中台")
	assert.Contains(t, systemMessage.Content, `"additionalProperties":false`)
	assert.Contains(t, systemMessage.Content, `"required":["markdown"]`)
	assert.Equal(t, schema.User, userMessage.Role)
	assert.Contains(t, userMessage.Content, `{"markdown":"old"}`)
	assert.Contains(t, userMessage.Content, "<user_instructions>\n突出 Go 经验\n</user_instructions>")
}

func TestService_RewriteSection_RetriesSchemaFailureThenEmitsValidSuggestion(t *testing.T) {
	fake := lltesting.NewFake()
	cm := &sequenceChatModel{
		FakeChatModel: fake,
		responses: []string{
			`{"items":[{"id":"a","achievements":"missing strict fields"}]}`,
			`{"items":[{"id":"a","introduction":null,"achievements":"fixed","highlightsMarkdown":null}]}`,
		},
	}
	svc := ai.NewService(cm, ai.NewLogrusHooks(logrus.New()))

	ch, err := svc.RewriteSection(context.Background(), ai.RewriteInput{
		TenantID:       1,
		SectionType:    "project",
		CurrentContent: json.RawMessage(`{"items":[{"id":"a"}]}`),
	})
	require.NoError(t, err)

	events := collectEvents(t, ch)
	require.Len(t, events, 2)
	assert.Equal(t, "suggestion", events[0].Kind)
	assert.JSONEq(t, cm.responses[1], string(events[0].Suggestion))
	assert.Equal(t, "done", events[1].Kind)
	require.Len(t, fake.RecordedInputs, 2)
	assert.Len(t, fake.RecordedInputs[1], 4)
	assert.Equal(t, schema.Assistant, fake.RecordedInputs[1][2].Role)
	assert.Equal(t, cm.responses[0], fake.RecordedInputs[1][2].Content)
	assert.Equal(t, schema.User, fake.RecordedInputs[1][3].Role)
	assert.Contains(t, fake.RecordedInputs[1][3].Content, "不符合 JSON schema")
}

func TestService_RewriteSection_RejectsSchemaInvalidResponseAfterOneRetry(t *testing.T) {
	fake := lltesting.NewFake()
	fake.FinalJSON = `{"markdown":"new","invented":"not allowed"}`
	svc := ai.NewService(fake, ai.NewLogrusHooks(logrus.New()))

	ch, err := svc.RewriteSection(context.Background(), ai.RewriteInput{
		TenantID:       1,
		SectionType:    "summary",
		CurrentContent: json.RawMessage(`{"markdown":"old"}`),
	})
	require.NoError(t, err)

	events := collectEvents(t, ch)
	require.Len(t, events, 2)
	assert.Equal(t, "error", events[0].Kind)
	assert.Equal(t, "INVALID_RESPONSE", events[0].ErrorCode)
	assert.NotEmpty(t, events[0].ErrorMessage)
	assert.Equal(t, "done", events[1].Kind)
	assert.Len(t, fake.RecordedInputs, 2)
}

func TestService_RewriteSection_RejectsUnsupportedSectionSynchronously(t *testing.T) {
	svc := ai.NewService(lltesting.NewFake(), nil)

	ch, err := svc.RewriteSection(context.Background(), ai.RewriteInput{
		SectionType:    "education",
		CurrentContent: json.RawMessage(`{}`),
	})

	assert.Error(t, err)
	assert.Nil(t, ch)
}

func TestService_GenerateResume_ReturnsTerminalNotImplementedEvents(t *testing.T) {
	svc := ai.NewService(lltesting.NewFake(), nil)

	ch, err := svc.GenerateResume(context.Background(), ai.GenerateInput{})
	require.NoError(t, err)

	events := collectEvents(t, ch)
	require.Len(t, events, 2)
	assert.Equal(t, "NOT_IMPLEMENTED", events[0].ErrorCode)
	assert.Equal(t, "done", events[1].Kind)
}

func collectEvents(t *testing.T, ch <-chan ai.Event) []ai.Event {
	t.Helper()
	var events []ai.Event
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for {
		select {
		case event, ok := <-ch:
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
	*lltesting.FakeChatModel
	mu        sync.Mutex
	responses []string
	next      int
}

func (m *sequenceChatModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	m.mu.Lock()
	index := m.next
	if index >= len(m.responses) {
		index = len(m.responses) - 1
	}
	m.next++
	m.FinalJSON = m.responses[index]
	m.mu.Unlock()
	return m.FakeChatModel.Stream(ctx, input, opts...)
}
