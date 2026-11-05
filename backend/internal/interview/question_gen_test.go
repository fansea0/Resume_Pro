package interview

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// fakeResponses 按 payloads 顺序返回预设响应（超出次数时重复最后一个），
// 并记录每次收到的消息，便于断言生题与契约修复的消息结构。
type fakeResponses struct {
	payloads []string
	calls    int
	inputs   [][]*schema.AgenticMessage

	// requireAnyOf 非空时：模型输入里必须出现其中至少一个标记，否则返回空 grade。
	//
	// 用来模拟真实模型的行为 —— 拿不到用户答案时它会返回 {"grade":""}，
	// 而空 grade 会被输出 schema 校验拒绝（invalid grade ""）。
	// 固定 payload 的 fake 掩盖了这个真实故障，测不出「回放路径」的问题。
	requireAnyOf []string
}

func (f *fakeResponses) Generate(_ context.Context, in []*schema.AgenticMessage, _ ...model.Option) (*schema.AgenticMessage, error) {
	f.inputs = append(f.inputs, in)
	index := f.calls
	f.calls++
	if index >= len(f.payloads) {
		index = len(f.payloads) - 1
	}
	payload := f.payloads[index]
	if len(f.requireAnyOf) > 0 && !agenticMessagesContainAny(in, f.requireAnyOf) {
		payload = `{"grade":"","followUp":""}`
	}
	return assistantAgenticMessage(payload), nil
}

// agenticMessagesContainAny 判断模型输入里是否出现任一标记。
func agenticMessagesContainAny(in []*schema.AgenticMessage, markers []string) bool {
	var builder strings.Builder
	for _, message := range in {
		if message == nil {
			continue
		}
		for _, block := range message.ContentBlocks {
			if block == nil {
				continue
			}
			if block.UserInputText != nil {
				builder.WriteString(block.UserInputText.Text)
			}
			if block.AssistantGenText != nil {
				builder.WriteString(block.AssistantGenText.Text)
			}
		}
	}
	text := builder.String()
	for _, marker := range markers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func questionsPayload(t *testing.T, questions ...map[string]any) string {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"questions": questions})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return string(payload)
}

func octopusQuestion() map[string]any {
	return map[string]any{
		"question":       "Go 的 channel 底层结构？",
		"standardAnswer": "hchan 结构",
		"label":          0,
		"mustPoints":     []string{"hchan"},
		"bonusPoints":    []string{"sendq/recvq"},
	}
}

func algorithmQuestion() map[string]any {
	return map[string]any{
		"question":       "LRU 缓存如何用 O(1) 实现？",
		"standardAnswer": "哈希表 + 双向链表",
		"label":          1,
		"mustPoints":     []string{"哈希表定位"},
		"bonusPoints":    []string{"哨兵节点"},
	}
}

func projectQuestion() map[string]any {
	return map[string]any{
		"question":       "项目里的缓存怎么做？",
		"standardAnswer": "",
		"label":          2,
		"mustPoints":     []string{"场景"},
		"bonusPoints":    []string{"布隆过滤器"},
	}
}

func threeQuestionConfig() InterviewConfig {
	return InterviewConfig{
		QuestionCount:            3,
		LabelCounts:              map[QuestionLabel]int{LabelOctopus: 1, LabelAlgorithm: 1, LabelProject: 1},
		FollowUpBudget:           1,
		PerQuestionFollowUpLimit: 1,
	}
}

func TestGenerateQuestions(t *testing.T) {
	payload := questionsPayload(t, octopusQuestion(), algorithmQuestion(), projectQuestion())
	fake := &fakeResponses{payloads: []string{payload}}
	g := NewQuestionGenerator(fake, 1)

	got, err := g.GenerateQuestions(context.Background(), "简历文本", &JDContext{Company: "X", Position: "Go 开发", Requirements: "熟悉 Redis"}, threeQuestionConfig())
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d questions, want 3", len(got))
	}
	if got[0].Label != LabelOctopus || got[1].Label != LabelAlgorithm || got[2].Label != LabelProject {
		t.Errorf("labels = %d, %d, %d", got[0].Label, got[1].Label, got[2].Label)
	}
	if got[0].StandardAnswer == "" || got[1].StandardAnswer == "" {
		t.Error("八股/算法题应有标准答案")
	}
	if got[2].StandardAnswer != "" {
		t.Error("项目题标准答案应为空")
	}
	if len(got[0].MustPoints) != 1 || got[0].MustPoints[0] != "hchan" || len(got[0].BonusPoints) != 1 || got[0].BonusPoints[0] != "sendq/recvq" {
		t.Errorf("points not mapped: %+v", got[0])
	}
	if fake.calls != 1 {
		t.Fatalf("calls = %d, want 1", fake.calls)
	}
	if len(fake.inputs[0]) != 2 {
		t.Fatalf("messages = %d, want 2", len(fake.inputs[0]))
	}
	if fake.inputs[0][0].Role != schema.AgenticRoleTypeSystem || fake.inputs[0][1].Role != schema.AgenticRoleTypeUser {
		t.Errorf("roles = %s, %s", fake.inputs[0][0].Role, fake.inputs[0][1].Role)
	}
	userText := fake.inputs[0][1].ContentBlocks[0].UserInputText.Text
	if !strings.Contains(userText, "简历文本") || !strings.Contains(userText, "熟悉 Redis") {
		t.Error("user payload should contain resume text and JD requirements")
	}
}

func TestGenerateQuestionsRepairsOnceOnRetry(t *testing.T) {
	short := questionsPayload(t, octopusQuestion())
	good := questionsPayload(t, octopusQuestion(), algorithmQuestion(), projectQuestion())
	fake := &fakeResponses{payloads: []string{short, good}}
	g := NewQuestionGenerator(fake, 1)

	got, err := g.GenerateQuestions(context.Background(), "简历文本", &JDContext{}, threeQuestionConfig())
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d questions, want 3", len(got))
	}
	if fake.calls != 2 {
		t.Fatalf("calls = %d, want 2", fake.calls)
	}
	repaired := fake.inputs[1]
	if len(repaired) != 4 {
		t.Fatalf("repair messages = %d, want 4", len(repaired))
	}
	if repaired[2].Role != schema.AgenticRoleTypeAssistant || repaired[3].Role != schema.AgenticRoleTypeUser {
		t.Errorf("repair roles = %s, %s", repaired[2].Role, repaired[3].Role)
	}
}

func TestGenerateQuestionsFailsAfterFailedRepair(t *testing.T) {
	bad := questionsPayload(t, octopusQuestion())
	fake := &fakeResponses{payloads: []string{bad}}
	g := NewQuestionGenerator(fake, 1)

	_, err := g.GenerateQuestions(context.Background(), "简历文本", &JDContext{}, threeQuestionConfig())
	if err == nil {
		t.Fatal("expected error after failed repair")
	}
	if fake.calls != 2 {
		t.Fatalf("calls = %d, want 2", fake.calls)
	}
}

func TestGenerateQuestionsNoRetryWhenMaxRetriesZero(t *testing.T) {
	bad := questionsPayload(t, octopusQuestion())
	fake := &fakeResponses{payloads: []string{bad}}
	g := NewQuestionGenerator(fake, 0)

	_, err := g.GenerateQuestions(context.Background(), "简历文本", &JDContext{}, threeQuestionConfig())
	if err == nil {
		t.Fatal("expected error")
	}
	if fake.calls != 1 {
		t.Fatalf("calls = %d, want 1", fake.calls)
	}
}

func TestGenerateQuestionsValidationFailures(t *testing.T) {
	invalidLabel := func() map[string]any {
		q := octopusQuestion()
		q["label"] = 3
		return q
	}
	blankQuestion := func() map[string]any {
		q := octopusQuestion()
		q["question"] = "   "
		return q
	}
	octopusWithoutAnswer := func() map[string]any {
		q := octopusQuestion()
		q["standardAnswer"] = ""
		return q
	}
	algorithmWithoutAnswer := func() map[string]any {
		q := algorithmQuestion()
		q["standardAnswer"] = ""
		return q
	}

	cases := []struct {
		name      string
		questions []map[string]any
	}{
		{"label 越界", []map[string]any{invalidLabel(), algorithmQuestion(), projectQuestion()}},
		{"question 为空白", []map[string]any{blankQuestion(), algorithmQuestion(), projectQuestion()}},
		{"八股题缺标准答案", []map[string]any{octopusWithoutAnswer(), algorithmQuestion(), projectQuestion()}},
		{"算法题缺标准答案", []map[string]any{octopusQuestion(), algorithmWithoutAnswer(), projectQuestion()}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeResponses{payloads: []string{questionsPayload(t, tc.questions...)}}
			g := NewQuestionGenerator(fake, 1)

			_, err := g.GenerateQuestions(context.Background(), "简历文本", &JDContext{}, threeQuestionConfig())
			if err == nil {
				t.Fatalf("expected error for %s", tc.name)
			}
			if fake.calls != 2 {
				t.Fatalf("calls = %d, want 2 (修复重试一次后仍失败)", fake.calls)
			}
		})
	}
}

func TestGenerateQuestionsRejectsInvalidConfig(t *testing.T) {
	fake := &fakeResponses{payloads: []string{questionsPayload(t, octopusQuestion())}}
	g := NewQuestionGenerator(fake, 1)

	_, err := g.GenerateQuestions(context.Background(), "简历文本", &JDContext{}, InterviewConfig{QuestionCount: 0})
	if err == nil {
		t.Fatal("expected error for zero question count")
	}
	if fake.calls != 0 {
		t.Fatalf("calls = %d, want 0", fake.calls)
	}
}

func TestGenerateQuestionsRequiresModel(t *testing.T) {
	g := NewQuestionGenerator(nil, 1)

	if _, err := g.GenerateQuestions(context.Background(), "简历文本", &JDContext{}, threeQuestionConfig()); err == nil {
		t.Fatal("expected error for nil model")
	}
}
