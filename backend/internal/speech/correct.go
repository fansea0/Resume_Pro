package speech

import (
	"context"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"

	"resumepro/backend/internal/logging"
)

// correctSystemPrompt 约束模型只做错别字校对。
// 思考强度固定最低档（调用方用 ReasoningEffort=low 的独立模型实例）：
// 这是纯校对任务，不需要长思考。
const correctSystemPrompt = `你是中文语音转写校对器。用户给你一段语音转写文本，你只需要修正错别字、同音字、明显的标点错误。
硬性要求：
1. 不得改变原意，不得润色、扩写、缩写或调整语序。
2. 不得增加原文没有的信息。
3. 只输出校对后的纯文本，不要任何解释、前缀或引号。`

// defaultCorrectTimeout 是校对调用的兜底超时：配置缺失时不能变成「永不超时」。
const defaultCorrectTimeout = 30 * time.Second

// Corrector 用 LLM 修复转写文本的错别字。
type Corrector struct {
	model   TextModel
	timeout time.Duration
}

// NewCorrector 创建校对器。model 为 nil 时 Correct 直接返回原文。
func NewCorrector(model TextModel, timeout time.Duration) *Corrector {
	return &Corrector{model: model, timeout: timeout}
}

// Correct 返回修复后的文本。
// 模型不可用、报错、超时或返回空时返回原文 —— 校对失败不阻断转写，
// 更不能返回空串把用户已经说出口的话吃掉。
func (c *Corrector) Correct(ctx context.Context, text string) string {
	if c == nil || c.model == nil {
		return text
	}
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return text
	}
	timeout := c.timeout
	if timeout <= 0 {
		timeout = defaultCorrectTimeout
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resp, err := c.model.Generate(callCtx, []*schema.AgenticMessage{
		schema.SystemAgenticMessage(correctSystemPrompt),
		schema.UserAgenticMessage(trimmed),
	})
	if err != nil {
		logging.WithError(err).Warn("voice: text correction failed, use raw transcript")
		return text
	}
	corrected := stripWrappers(strings.TrimSpace(agenticText(resp)))
	if corrected == "" {
		return text
	}
	return corrected
}

// stripWrappers 剥掉模型可能加的包裹标记。
//
// prompt 已经要求「只输出校对后的纯文本」，但模型不听话时经常带上 Markdown 围栏、
// 加粗标记或引号；这些标记进了输入框就是脏数据。
//
// 只剥「成对且包裹整个文本」的标记 —— 文本内部的引号属于正常内容，不能动。
func stripWrappers(text string) string {
	trimmed := strings.TrimSpace(text)
	wrappers := []struct{ open, close string }{
		{"```", "```"},
		{"**", "**"},
		{"\"\"", "\"\""},
		{"「", "」"},
		{"“", "”"},
		{"‘", "’"},
	}
	for _, wrapper := range wrappers {
		if len(trimmed) > len(wrapper.open)+len(wrapper.close) &&
			strings.HasPrefix(trimmed, wrapper.open) &&
			strings.HasSuffix(trimmed, wrapper.close) {
			trimmed = strings.TrimSpace(trimmed[len(wrapper.open) : len(trimmed)-len(wrapper.close)])
		}
	}
	return trimmed
}

// agenticText 拼接 assistant 响应中的全部生成文本块。
func agenticText(msg *schema.AgenticMessage) string {
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
