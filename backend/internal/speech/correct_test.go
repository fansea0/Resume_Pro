package speech

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// stubTextModel 返回固定文本或错误。
type stubTextModel struct {
	text string
	err  error
}

func (s *stubTextModel) Generate(_ context.Context, _ []*schema.AgenticMessage, _ ...model.Option) (*schema.AgenticMessage, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &schema.AgenticMessage{
		Role:          schema.AgenticRoleTypeAssistant,
		ContentBlocks: []*schema.ContentBlock{schema.NewContentBlock(&schema.AssistantGenText{Text: s.text})},
	}, nil
}

func TestCorrectorReturnsCorrectedText(t *testing.T) {
	corrector := NewCorrector(&stubTextModel{text: "我做过 Golang 的后台服务"}, time.Second)
	got := corrector.Correct(context.Background(), "我做过 go lang 的后台服务")
	if got != "我做过 Golang 的后台服务" {
		t.Fatalf("corrected = %q", got)
	}
}

func TestCorrectorFallsBackToRawTextOnError(t *testing.T) {
	corrector := NewCorrector(&stubTextModel{err: errors.New("llm down")}, time.Second)
	raw := "识别出来的原话"
	if got := corrector.Correct(context.Background(), raw); got != raw {
		t.Fatalf("corrected = %q, want raw %q", got, raw)
	}
}

func TestCorrectorFallsBackWhenModelMissing(t *testing.T) {
	corrector := NewCorrector(nil, time.Second)
	raw := "原话"
	if got := corrector.Correct(context.Background(), raw); got != raw {
		t.Fatalf("corrected = %q, want raw", got)
	}
}

func TestCorrectorFallsBackOnEmptyModelOutput(t *testing.T) {
	corrector := NewCorrector(&stubTextModel{text: "  "}, time.Second)
	raw := "原话"
	if got := corrector.Correct(context.Background(), raw); got != raw {
		t.Fatalf("corrected = %q, want raw %q", got, raw)
	}
}

func TestCorrectorPromptForbidsRewriting(t *testing.T) {
	if !strings.Contains(correctSystemPrompt, "不得改变原意") {
		t.Error("system prompt must forbid semantic rewriting")
	}
}
