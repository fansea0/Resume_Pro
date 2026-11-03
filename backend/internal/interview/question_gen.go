package interview

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/template"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"resumepro/backend/internal/interview/prompts"
)

// questionGenerationMaxTokens 是生题单次调用的输出上限：N 道题含标准答案与
// 必答/加分点，12 题约需 5~7k token，给 8192 留足余量避免 JSON 被截断。
const questionGenerationMaxTokens = 8192

// ResponsesModel 是生题所需的 Agentic 模型窄接口（model.AgenticModel 的 Generate 半边），
// 收窄后便于测试替换 fake 实现。
type ResponsesModel interface {
	Generate(ctx context.Context, in []*schema.AgenticMessage, opts ...model.Option) (*schema.AgenticMessage, error)
}

// GeneratedQuestion 是 LLM 生成的一道面试主问题。落库转换（MainQuestionInput）由编排层负责。
type GeneratedQuestion struct {
	Question       string
	StandardAnswer string
	Label          QuestionLabel
	MustPoints     []string
	BonusPoints    []string
}

// JDContext 是生题所需的目标岗位上下文，语义与 ai.TargetJobContext 对齐。
type JDContext struct {
	Company      string
	Position     string
	Requirements string
}

// QuestionGenerator 通过 LLM 生成面试主问题：prompt 要求只输出 JSON，
// Go 侧做 schema 校验，失败时按契约修复模式重试一次。
type QuestionGenerator struct {
	cm         ResponsesModel
	tmpl       *template.Template
	maxRetries int
}

// NewQuestionGenerator 创建生题器；maxRetries > 0 时校验失败会追加纠正消息重试一次。
func NewQuestionGenerator(cm ResponsesModel, maxRetries int) *QuestionGenerator {
	return &QuestionGenerator{
		cm:         cm,
		tmpl:       template.Must(template.ParseFS(prompts.FS, "questions.tmpl")),
		maxRetries: maxRetries,
	}
}

// labelNames 把标签映射为 prompt 中的中文说明。
var labelNames = map[QuestionLabel]string{
	LabelOctopus:   "八股/基础题（Go 基础、数据库、Redis）",
	LabelAlgorithm: "算法题",
	LabelProject:   "项目深挖题",
}

type labelCount struct {
	Name  string
	Count int
}

// labelCountsForPrompt 按固定标签顺序输出非零的各标签数量，保证 prompt 稳定可读。
func labelCountsForPrompt(cfg InterviewConfig) []labelCount {
	labels := []QuestionLabel{LabelOctopus, LabelAlgorithm, LabelProject}
	counts := make([]labelCount, 0, len(labels))
	for _, label := range labels {
		if n := cfg.LabelCounts[label]; n > 0 {
			counts = append(counts, labelCount{Name: labelNames[label], Count: n})
		}
	}
	return counts
}

type questionUserPayload struct {
	Company       string       `json:"company,omitempty"`
	Position      string       `json:"position,omitempty"`
	Requirements  string       `json:"requirements,omitempty"`
	ResumeText    string       `json:"resumeText"`
	QuestionCount int          `json:"questionCount"`
	LabelCounts   []labelCount `json:"labelCounts"`
}

func (g *QuestionGenerator) buildMessages(resumeText string, jd *JDContext, cfg InterviewConfig) ([]*schema.AgenticMessage, error) {
	if jd == nil {
		jd = &JDContext{}
	}
	userPayload, err := json.Marshal(questionUserPayload{
		Company:       jd.Company,
		Position:      jd.Position,
		Requirements:  jd.Requirements,
		ResumeText:    resumeText,
		QuestionCount: cfg.QuestionCount,
		LabelCounts:   labelCountsForPrompt(cfg),
	})
	if err != nil {
		return nil, fmt.Errorf("serialize question prompt context: %w", err)
	}

	var system bytes.Buffer
	if err := g.tmpl.ExecuteTemplate(&system, "questions.tmpl", struct {
		QuestionCount int
		LabelCounts   []labelCount
	}{
		QuestionCount: cfg.QuestionCount,
		LabelCounts:   labelCountsForPrompt(cfg),
	}); err != nil {
		return nil, fmt.Errorf("render question prompt: %w", err)
	}

	return []*schema.AgenticMessage{
		schema.SystemAgenticMessage(system.String()),
		schema.UserAgenticMessage(string(userPayload)),
	}, nil
}

// GenerateQuestions 生成一批通过 schema 校验的主问题。
// 数量或字段约束不满足时最多按契约修复模式重试一次，仍失败则返回错误。
func (g *QuestionGenerator) GenerateQuestions(ctx context.Context, resumeText string, jd *JDContext, cfg InterviewConfig) ([]GeneratedQuestion, error) {
	if g.cm == nil {
		return nil, errors.New("question generator requires a responses model")
	}
	if cfg.QuestionCount <= 0 {
		return nil, fmt.Errorf("invalid interview config: question count is %d", cfg.QuestionCount)
	}

	messages, err := g.buildMessages(resumeText, jd, cfg)
	if err != nil {
		return nil, err
	}

	// 生题一次输出 N 道题（每题含标准答案、必答/加分点），默认 4096 token 会被截断，
	// 显式给足空间（12 题约 5~7k token，8192 留有余量）。
	response, err := g.cm.Generate(ctx, messages, model.WithMaxTokens(questionGenerationMaxTokens))
	if err != nil {
		return nil, fmt.Errorf("generate questions: %w", err)
	}
	content := agenticMessageText(response)

	questions, err := parseGeneratedQuestions(content, cfg)
	if err != nil && g.maxRetries > 0 {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		retryResponse, retryErr := g.cm.Generate(ctx, repairQuestionMessages(messages, content, err), model.WithMaxTokens(questionGenerationMaxTokens))
		if retryErr != nil {
			return nil, fmt.Errorf("generate questions: %w", retryErr)
		}
		questions, err = parseGeneratedQuestions(agenticMessageText(retryResponse), cfg)
	}
	if err != nil {
		return nil, err
	}
	return questions, nil
}

type generatedQuestionsPayload struct {
	Questions []generatedQuestionPayload `json:"questions"`
}

type generatedQuestionPayload struct {
	Question       string   `json:"question"`
	StandardAnswer string   `json:"standardAnswer"`
	Label          *int     `json:"label"`
	MustPoints     []string `json:"mustPoints"`
	BonusPoints    []string `json:"bonusPoints"`
}

// parseGeneratedQuestions 解析并校验生题输出：
// 单个 JSON 对象；数量等于 cfg.QuestionCount；label ∈ {0,1,2}；question 非空；
// 八股/算法题 standardAnswer 必须非空，项目题允许空。
func parseGeneratedQuestions(content string, cfg InterviewConfig) ([]GeneratedQuestion, error) {
	decoder := json.NewDecoder(strings.NewReader(content))
	var payload generatedQuestionsPayload
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if err := ensureJSONEnd(decoder); err != nil {
		return nil, err
	}
	if len(payload.Questions) != cfg.QuestionCount {
		return nil, fmt.Errorf("expected %d questions, got %d", cfg.QuestionCount, len(payload.Questions))
	}

	questions := make([]GeneratedQuestion, 0, len(payload.Questions))
	for i, q := range payload.Questions {
		if q.Label == nil || *q.Label < int(LabelOctopus) || *q.Label > int(LabelProject) {
			return nil, fmt.Errorf("questions[%d]: invalid label", i)
		}
		if strings.TrimSpace(q.Question) == "" {
			return nil, fmt.Errorf("questions[%d]: question is empty", i)
		}
		label := QuestionLabel(*q.Label)
		if label != LabelProject && strings.TrimSpace(q.StandardAnswer) == "" {
			return nil, fmt.Errorf("questions[%d]: label %d requires a standard answer", i, label)
		}
		questions = append(questions, GeneratedQuestion{
			Question:       q.Question,
			StandardAnswer: q.StandardAnswer,
			Label:          label,
			MustPoints:     q.MustPoints,
			BonusPoints:    q.BonusPoints,
		})
	}
	return questions, nil
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

// agenticMessageText 拼接 assistant 响应中的全部生成文本块。
func agenticMessageText(msg *schema.AgenticMessage) string {
	if msg == nil {
		return ""
	}
	var sb strings.Builder
	for _, block := range msg.ContentBlocks {
		if block == nil || block.AssistantGenText == nil {
			continue
		}
		sb.WriteString(block.AssistantGenText.Text)
	}
	return sb.String()
}

// assistantAgenticMessage 构造带生成文本的 assistant 消息。
// eino v0.9.21 只提供 SystemAgenticMessage/UserAgenticMessage，assistant 需自行组装。
func assistantAgenticMessage(text string) *schema.AgenticMessage {
	return &schema.AgenticMessage{
		Role:          schema.AgenticRoleTypeAssistant,
		ContentBlocks: []*schema.ContentBlock{schema.NewContentBlock(&schema.AssistantGenText{Text: text})},
	}
}

// repairQuestionMessages 复用 ai 包的契约修复模式：原消息 + 上一次无效输出 + 纠正指令。
func repairQuestionMessages(messages []*schema.AgenticMessage, invalidContent string, validationErr error) []*schema.AgenticMessage {
	repaired := append([]*schema.AgenticMessage(nil), messages...)
	return append(repaired,
		assistantAgenticMessage(invalidContent),
		schema.UserAgenticMessage(fmt.Sprintf(
			"上一次输出不符合约定：%s。请只返回修正后的 JSON，不要解释或使用 markdown 包裹。",
			validationErr,
		)),
	)
}
