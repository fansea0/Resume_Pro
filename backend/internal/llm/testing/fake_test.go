package testing

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

func TestNewFakeAppliesOptions(t *testing.T) {
	fake := NewFake(func(f *FakeChatModel) {
		f.FinalJSON = `{"ok":true}`
	})

	response, err := fake.Generate(context.Background(), nil)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if response.Content != `{"ok":true}` {
		t.Fatalf("Generate() content = %q", response.Content)
	}
}

func TestFakeChatModelGenerateRecordsInputAndReturnsConfiguredResponse(t *testing.T) {
	fake := NewFake()
	fake.FinalJSON = `{"summary":"improved"}`
	fake.ThinkingText = "checking keywords"
	input := []*schema.Message{{Role: schema.User, Content: "Improve this"}}

	response, err := fake.Generate(context.Background(), input, model.WithMaxTokens(64))

	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if response.Role != schema.Assistant || response.Content != fake.FinalJSON || response.ReasoningContent != fake.ThinkingText {
		t.Fatalf("Generate() response = %#v", response)
	}
	if len(fake.RecordedInputs) != 1 || len(fake.RecordedInputs[0]) != 1 || fake.RecordedInputs[0][0] != input[0] {
		t.Fatalf("RecordedInputs = %#v, want input %#v", fake.RecordedInputs, input)
	}
}

func TestFakeChatModelGenerateReturnsConfiguredErrorAfterRecordingInput(t *testing.T) {
	fake := NewFake()
	fake.ErrToReturn = errors.New("upstream unavailable")
	input := []*schema.Message{{Role: schema.User, Content: "Improve this"}}

	response, err := fake.Generate(context.Background(), input)

	if !errors.Is(err, fake.ErrToReturn) {
		t.Fatalf("Generate() error = %v, want %v", err, fake.ErrToReturn)
	}
	if response != nil {
		t.Fatalf("Generate() response = %#v, want nil", response)
	}
	if len(fake.RecordedInputs) != 1 || fake.RecordedInputs[0][0] != input[0] {
		t.Fatalf("RecordedInputs = %#v, want input %#v", fake.RecordedInputs, input)
	}
}

func TestFakeChatModelStreamEmitsThinkingChunksThenFinalJSON(t *testing.T) {
	fake := NewFake()
	fake.StreamChunks = []string{"checking role", "rewriting achievements"}
	fake.FinalJSON = `{"items":[]}`
	input := []*schema.Message{{Role: schema.User, Content: "Improve this"}}

	stream, err := fake.Stream(context.Background(), input)
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	t.Cleanup(stream.Close)

	for index, want := range fake.StreamChunks {
		chunk, recvErr := stream.Recv()
		if recvErr != nil {
			t.Fatalf("stream chunk %d error = %v", index, recvErr)
		}
		if chunk.Role != schema.Assistant || chunk.ReasoningContent != want || chunk.Content != "" {
			t.Fatalf("stream chunk %d = %#v", index, chunk)
		}
	}
	final, recvErr := stream.Recv()
	if recvErr != nil {
		t.Fatalf("final stream chunk error = %v", recvErr)
	}
	if final.Role != schema.Assistant || final.Content != fake.FinalJSON {
		t.Fatalf("final stream chunk = %#v", final)
	}
	if _, recvErr = stream.Recv(); !errors.Is(recvErr, io.EOF) {
		t.Fatalf("stream completion error = %v, want EOF", recvErr)
	}
	if len(fake.RecordedInputs) != 1 || fake.RecordedInputs[0][0] != input[0] {
		t.Fatalf("RecordedInputs = %#v, want input %#v", fake.RecordedInputs, input)
	}
}
