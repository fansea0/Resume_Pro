package ai_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"resumepro/backend/internal/ai"
)

const emptyAnalysis = `"analysis":{"jdAlignment":{"status":"skipped","matchedKeywords":[],"excludedKeywords":[],"missingKeywords":[]},"evidenceRequests":[],"warnings":[]}`

func TestOptimizationOutputDerivesSuggestionsFromRewrittenText(t *testing.T) {
	rewrite, analysis, ok, err := ai.ValidateOptimizationOutput(
		"summary",
		json.RawMessage(`{"markdown":"负责产品交付"}`),
		[]byte(`{
			"edits":[{
				"target":{"itemId":null,"field":"markdown"},
				"newText":"主导产品交付",
				"issues":[{"code":"S2","label":"弱动词","issue":"用「负责」开头","rationale":"动词开头更醒目","quote":"主导产品交付"}]
			}],
			"analysis":{"jdAlignment":{"status":"applied","matchedKeywords":["产品交付"],"excludedKeywords":[],"missingKeywords":["A/B 测试"]},"evidenceRequests":[],"warnings":[]}
		}`),
	)

	require.NoError(t, err)
	assert.True(t, ok)
	assert.JSONEq(t, `{"markdown":"主导产品交付"}`, string(rewrite))
	assert.Contains(t, string(analysis), `"anchor":"负责"`)
	assert.Contains(t, string(analysis), `"replacement":"主导"`)
	assert.Contains(t, string(analysis), `"severity":"high"`)
	assert.Contains(t, string(analysis), `"code":"S2"`)
	assert.Contains(t, string(analysis), `"missingKeywords":["A/B 测试"]`)
}

func TestOptimizationOutputAppliesSuggestionsToMatchingItemOnly(t *testing.T) {
	rewrite, analysis, ok, err := ai.ValidateOptimizationOutput(
		"work",
		json.RawMessage(`{"items":[
			{"id":"work-1","achievements":"负责产品交付","highlightsMarkdown":"- 负责交付\n- 优化流程"},
			{"id":"work-2","achievements":"负责运营","highlightsMarkdown":"- 负责活动"}
		]}`),
		[]byte(`{
			"edits":[{
				"target":{"itemId":"work-1","field":"highlightsMarkdown"},
				"newText":"- 主导交付\n- 优化流程",
				"issues":[{"code":"S2","label":"弱动词","issue":"弱动词开头","rationale":"动作更明确","quote":"主导交付"}]
			}],
			`+emptyAnalysis+`
		}`),
	)

	require.NoError(t, err)
	assert.True(t, ok)
	assert.JSONEq(t, `{"items":[{"id":"work-1","achievements":"负责产品交付","highlightsMarkdown":"- 主导交付\n- 优化流程"}]}`, string(rewrite))
	assert.Contains(t, string(analysis), `"anchor":"负责"`)
	assert.Contains(t, string(analysis), `"start":2`)
	assert.Contains(t, string(analysis), `"end":4`)
}

func TestOptimizationOutputRejectsModelInsertedContent(t *testing.T) {
	rewrite, analysis, ok, err := ai.ValidateOptimizationOutput(
		"summary",
		json.RawMessage(`{"markdown":"负责产品交付"}`),
		[]byte(`{"edits":[{"target":{"itemId":null,"field":"markdown"},"newText":"负责产品交付并新增了服务","issues":[]}],`+emptyAnalysis+`}`),
	)

	require.NoError(t, err)
	assert.True(t, ok)
	assert.JSONEq(t, `{"markdown":"负责产品交付"}`, string(rewrite))
	assert.Contains(t, string(analysis), "模型只追加了原文没有的内容")
}

func TestOptimizationOutputRejectsSuggestionIntroducingNewNumber(t *testing.T) {
	rewrite, analysis, _, err := ai.ValidateOptimizationOutput(
		"summary",
		json.RawMessage(`{"markdown":"首周留存率提升 18%"}`),
		[]byte(`{"edits":[{"target":{"itemId":null,"field":"markdown"},"newText":"首周留存率提升 32%","issues":[{"code":"S4","label":"成果模糊","issue":"数字存疑","rationale":"保留原数据","quote":"首周留存率提升 32%"}]}],`+emptyAnalysis+`}`),
	)

	require.NoError(t, err)
	assert.JSONEq(t, `{"markdown":"首周留存率提升 18%"}`, string(rewrite))
	assert.Contains(t, string(analysis), `"status":"rejected"`)
	assert.Contains(t, string(analysis), "建议引入了原文中没有的数据")
}

func TestOptimizationOutputRejectsSuggestionWithPlaceholder(t *testing.T) {
	rewrite, analysis, _, err := ai.ValidateOptimizationOutput(
		"summary",
		json.RawMessage(`{"markdown":"首周留存率提升 18%"}`),
		[]byte(`{"edits":[{"target":{"itemId":null,"field":"markdown"},"newText":"首周留存率提升 XX%","issues":[]}],`+emptyAnalysis+`}`),
	)

	require.NoError(t, err)
	assert.JSONEq(t, `{"markdown":"首周留存率提升 18%"}`, string(rewrite))
	assert.Contains(t, string(analysis), "建议内容包含占位符")
}

func TestOptimizationOutputRejectsDeletionRemovingData(t *testing.T) {
	rewrite, analysis, _, err := ai.ValidateOptimizationOutput(
		"work",
		json.RawMessage(`{"items":[{"id":"work-1","achievements":"负责产品交付","highlightsMarkdown":"- 优化流程，节省 20% 成本"}]}`),
		[]byte(`{"edits":[{"target":{"itemId":"work-1","field":"highlightsMarkdown"},"newText":"- 优化流程","issues":[]}],`+emptyAnalysis+`}`),
	)

	require.NoError(t, err)
	assert.JSONEq(t, `{"items":[]}`, string(rewrite))
	assert.Contains(t, string(analysis), "建议删除了原文中的数据")
}

func TestOptimizationOutputAcceptsDeletionWithoutData(t *testing.T) {
	rewrite, _, ok, err := ai.ValidateOptimizationOutput(
		"work",
		json.RawMessage(`{"items":[{"id":"work-1","achievements":"负责产品交付","highlightsMarkdown":"- 优化流程，顺便做了很多其它无关的事情"}]}`),
		[]byte(`{"edits":[{"target":{"itemId":"work-1","field":"highlightsMarkdown"},"newText":"- 优化流程","issues":[{"code":"S8","label":"冗长啰嗦","issue":"尾句无关","rationale":"删掉无关细节","quote":"- 优化流程"}]}],`+emptyAnalysis+`}`),
	)

	require.NoError(t, err)
	assert.True(t, ok)
	assert.JSONEq(t, `{"items":[{"id":"work-1","achievements":"负责产品交付","highlightsMarkdown":"- 优化流程"}]}`, string(rewrite))
}

func TestOptimizationOutputIgnoresEditTargetingFrozenField(t *testing.T) {
	rewrite, analysis, ok, err := ai.ValidateOptimizationOutput(
		"work",
		json.RawMessage(`{"items":[{"id":"work-1","company":"Example Co","achievements":"负责产品交付","highlightsMarkdown":"- 负责交付"}]}`),
		[]byte(`{"edits":[{"target":{"itemId":"work-1","field":"company"},"newText":"Other Co","issues":[]}],`+emptyAnalysis+`}`),
	)

	require.NoError(t, err)
	assert.True(t, ok)
	assert.JSONEq(t, `{"items":[]}`, string(rewrite))
	assert.Contains(t, string(analysis), "指向不可改写的字段")
}

func TestOptimizationOutputIgnoresEditTargetingUnknownItem(t *testing.T) {
	rewrite, analysis, ok, err := ai.ValidateOptimizationOutput(
		"work",
		json.RawMessage(`{"items":[{"id":"work-1","achievements":"负责产品交付","highlightsMarkdown":"- 负责交付"}]}`),
		[]byte(`{"edits":[{"target":{"itemId":"work-9","field":"achievements"},"newText":"主导产品交付","issues":[]}],`+emptyAnalysis+`}`),
	)

	require.NoError(t, err)
	assert.True(t, ok)
	assert.JSONEq(t, `{"items":[]}`, string(rewrite))
	assert.Contains(t, string(analysis), "定位不到目标条目")
}

func TestOptimizationOutputAssignsUniqueSuggestionIDsAcrossEdits(t *testing.T) {
	_, analysis, _, err := ai.ValidateOptimizationOutput(
		"work",
		json.RawMessage(`{"items":[{"id":"work-1","achievements":"负责产品交付","highlightsMarkdown":"- 负责交付"}]}`),
		[]byte(`{
			"edits":[
				{"target":{"itemId":"work-1","field":"highlightsMarkdown"},"newText":"- 主导交付","issues":[]},
				{"target":{"itemId":"work-1","field":"achievements"},"newText":"主导产品交付","issues":[]}
			],
			`+emptyAnalysis+`
		}`),
	)

	require.NoError(t, err)
	assert.Contains(t, string(analysis), `"id":"sg-1"`)
	assert.Contains(t, string(analysis), `"id":"sg-2"`)
}

func TestOptimizationOutputSortsSuggestionsBySeverity(t *testing.T) {
	rewrite, analysis, ok, err := ai.ValidateOptimizationOutput(
		"work",
		json.RawMessage(`{"items":[{"id":"work-1","achievements":"负责产品交付","highlightsMarkdown":"- 负责交付"}]}`),
		[]byte(`{
			"edits":[
				{"target":{"itemId":"work-1","field":"highlightsMarkdown"},"newText":"- 主导交付","issues":[{"code":"S10","label":"信息密度","issue":"偏长","rationale":"压缩表达","quote":"主导交付"}]},
				{"target":{"itemId":"work-1","field":"achievements"},"newText":"主导产品交付","issues":[{"code":"S2","label":"弱动词","issue":"弱动词","rationale":"动作明确","quote":"主导产品交付"}]}
			],
			`+emptyAnalysis+`
		}`),
	)

	require.NoError(t, err)
	assert.True(t, ok)
	assert.JSONEq(t, `{"items":[{"id":"work-1","achievements":"主导产品交付","highlightsMarkdown":"- 主导交付"}]}`, string(rewrite))

	highIndex := indexOf(t, string(analysis), `"severity":"high"`)
	lowIndex := indexOf(t, string(analysis), `"severity":"low"`)
	assert.Less(t, highIndex, lowIndex)
}

func TestOptimizationOutputReportsDensityMetrics(t *testing.T) {
	_, analysis, _, err := ai.ValidateOptimizationOutput(
		"work",
		json.RawMessage(`{"items":[{"id":"work-1","achievements":"负责产品交付","highlightsMarkdown":"- 负责交付"}]}`),
		[]byte(`{"edits":[],`+emptyAnalysis+`}`),
	)

	require.NoError(t, err)
	assert.Contains(t, string(analysis), `"field":"achievements"`)
	assert.Contains(t, string(analysis), `"overLimit":false`)
}

func TestOptimizationOutputFlagsOverlongFieldAsOverLimit(t *testing.T) {
	_, analysis, _, err := ai.ValidateOptimizationOutput(
		"work",
		json.RawMessage(`{"items":[{"id":"work-1","achievements":"负责产品交付并推动团队完成年度目标同时协同多个部门达成关键指标，覆盖全部核心业务线的日常需求与紧急诉求，确保最终结果可衡量并且过程透明可追溯","highlightsMarkdown":"- 负责交付"}]}`),
		[]byte(`{"edits":[],`+emptyAnalysis+`}`),
	)

	require.NoError(t, err)
	assert.Contains(t, string(analysis), `"overLimit":true`)
}

func TestOptimizationOutputRejectsContractViolations(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{name: "missing edits", raw: `{"analysis":{"jdAlignment":{"status":"skipped"},"evidenceRequests":[],"warnings":[]}}`},
		{name: "edits not an array", raw: `{"edits":{},"analysis":{"jdAlignment":{"status":"skipped"},"evidenceRequests":[],"warnings":[]}}`},
		{name: "unexpected top level key", raw: `{"rewrite":{},"edits":[],"analysis":{"jdAlignment":{"status":"skipped"},"evidenceRequests":[],"warnings":[]}}`},
		{name: "trailing json", raw: `{"edits":[],"analysis":{"jdAlignment":{"status":"skipped"},"evidenceRequests":[],"warnings":[]}} {}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, ok, err := ai.ValidateOptimizationOutput(
				"summary",
				json.RawMessage(`{"markdown":"负责产品交付"}`),
				[]byte(tt.raw),
			)

			require.Error(t, err)
			assert.False(t, ok)
		})
	}
}

func TestOptimizationOutputMarksInvalidCurrentContentAsUnrepairable(t *testing.T) {
	_, _, ok, err := ai.ValidateOptimizationOutput(
		"summary",
		json.RawMessage(`{"markdown":123}`),
		[]byte(`{"edits":[],`+emptyAnalysis+`}`),
	)

	require.Error(t, err)
	assert.False(t, ok)
	assert.True(t, errors.Is(err, ai.ErrInvalidCurrentContent))
}

func TestOptimizationOutputDowngradesAnalysisWhenEvidencePlaceholderLeaks(t *testing.T) {
	rewrite, analysis, ok, err := ai.ValidateOptimizationOutput(
		"summary",
		json.RawMessage(`{"markdown":"负责产品交付"}`),
		[]byte(`{
			"edits":[],
			"analysis":{
				"jdAlignment":{"status":"skipped","matchedKeywords":[],"excludedKeywords":[],"missingKeywords":[]},
				"evidenceRequests":[{"code":"S1","target":{"field":"markdown"},"reason":"还需 XX 名用户规模","exampleWithPlaceholder":"覆盖 XX 名用户"}],
				"warnings":[]
			}
		}`),
	)

	require.NoError(t, err)
	assert.False(t, ok)
	assert.JSONEq(t, `{"markdown":"负责产品交付"}`, string(rewrite))
	assert.NotContains(t, string(analysis), "XX")
}

func TestOptimizationOutputKeepsEvidenceRequestWithPlaceholderExample(t *testing.T) {
	_, analysis, ok, err := ai.ValidateOptimizationOutput(
		"summary",
		json.RawMessage(`{"markdown":"负责产品交付"}`),
		[]byte(`{
			"edits":[],
			"analysis":{
				"jdAlignment":{"status":"skipped","matchedKeywords":[],"excludedKeywords":[],"missingKeywords":[]},
				"evidenceRequests":[{"code":"S1","target":{"field":"markdown"},"reason":"缺少规模","exampleWithPlaceholder":"覆盖 XX 名用户"}],
				"warnings":[]
			}
		}`),
	)

	require.NoError(t, err)
	assert.True(t, ok)
	assert.Contains(t, string(analysis), `"exampleWithPlaceholder":"覆盖 XX 名用户"`)
}

func TestOptimizationOutputNormalizesS7Warning(t *testing.T) {
	_, analysis, ok, err := ai.ValidateOptimizationOutput(
		"summary",
		json.RawMessage(`{"markdown":"负责产品交付"}`),
		[]byte(`{
			"edits":[],
			"analysis":{
				"jdAlignment":{"status":"skipped","matchedKeywords":[],"excludedKeywords":[],"missingKeywords":[]},
				"evidenceRequests":[],
				"warnings":[{"code":"S7","resolution":"由于频繁跳槽，改写为稳定的职业叙事。"}]
			}
		}`),
	)

	require.NoError(t, err)
	assert.True(t, ok)
	assert.Contains(t, string(analysis), "已基于既有事实重组叙事，未推测空窗、离职或转行原因。")
	assert.NotContains(t, string(analysis), "由于频繁跳槽")
}

func TestOptimizationOutputKeepsOriginalContentWhenModelReturnsNoEdits(t *testing.T) {
	rewrite, analysis, ok, err := ai.ValidateOptimizationOutput(
		"work",
		json.RawMessage(`{"items":[{"id":"work-1","achievements":"负责产品交付","highlightsMarkdown":"- 负责交付"}]}`),
		[]byte(`{"edits":[],`+emptyAnalysis+`}`),
	)

	require.NoError(t, err)
	assert.True(t, ok)
	assert.JSONEq(t, `{"items":[]}`, string(rewrite))
	assert.Contains(t, string(analysis), `"suggestions":[]`)
}

func indexOf(t *testing.T, haystack, needle string) int {
	t.Helper()
	index := indexOfString(haystack, needle)
	require.GreaterOrEqual(t, index, 0, "expected %q to be present", needle)
	return index
}

func indexOfString(haystack, needle string) int {
	for index := 0; index+len(needle) <= len(haystack); index++ {
		if haystack[index:index+len(needle)] == needle {
			return index
		}
	}
	return -1
}
