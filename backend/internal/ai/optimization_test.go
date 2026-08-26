package ai_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"resumepro/backend/internal/ai"
)

func TestOptimizationOutputAcceptsSafeRewriteWithS2AndS5Analysis(t *testing.T) {
	rewrite, analysis, ok, err := ai.ValidateOptimizationOutput(
		"summary",
		json.RawMessage(`{"markdown":"负责产品交付"}`),
		[]byte(`{
			"rewrite":{"markdown":"主导产品交付"},
			"analysis":{
				"jdAlignment":{"status":"applied","matchedKeywords":["产品交付"],"excludedKeywords":[]},
				"matchedIssues":[
					{"code":"S2","target":{"field":"markdown"},"resolution":"替换弱动词","status":"fixed"},
					{"code":"S5","target":{"field":"markdown"},"resolution":"突出既有交付经验","status":"fixed"}
				],
				"evidenceRequests":[],
				"warnings":[]
			}
		}`),
	)

	require.NoError(t, err)
	assert.True(t, ok)
	assert.JSONEq(t, `{"markdown":"主导产品交付"}`, string(rewrite))
	assert.Contains(t, string(analysis), `"code":"S2"`)
	assert.Contains(t, string(analysis), `"code":"S5"`)
}

func TestOptimizationOutputRejectsEvidencePlaceholderInRewrite(t *testing.T) {
	_, _, ok, err := ai.ValidateOptimizationOutput(
		"summary",
		json.RawMessage(`{"markdown":"负责产品交付"}`),
		[]byte(`{
			"rewrite":{"markdown":"主导产品交付，覆盖 XX 名用户"},
			"analysis":{"jdAlignment":{"status":"skipped"},"matchedIssues":[],"evidenceRequests":[],"warnings":[]}
		}`),
	)

	require.Error(t, err)
	assert.False(t, ok)
}

func TestOptimizationOutputAllowsS1PlaceholderOnlyInEvidenceRequest(t *testing.T) {
	rewrite, analysis, ok, err := ai.ValidateOptimizationOutput(
		"summary",
		json.RawMessage(`{"markdown":"负责产品交付"}`),
		[]byte(`{
			"rewrite":{"markdown":"主导产品交付"},
			"analysis":{
				"jdAlignment":{"status":"skipped"},
				"matchedIssues":[],
				"evidenceRequests":[{"code":"S1","target":{"field":"markdown"},"reason":"缺少规模","exampleWithPlaceholder":"覆盖 XX 名用户"}],
				"warnings":[]
			}
		}`),
	)

	require.NoError(t, err)
	assert.True(t, ok)
	assert.JSONEq(t, `{"markdown":"主导产品交付"}`, string(rewrite))
	assert.Contains(t, string(analysis), `"exampleWithPlaceholder":"覆盖 XX 名用户"`)
}

func TestOptimizationOutputDowngradesAnalysisWhenEvidencePlaceholderLeaksOutsideS1Example(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
	}{
		{
			name: "matched issue resolution",
			raw:  []byte(`{"rewrite":{"markdown":"主导产品交付"},"analysis":{"jdAlignment":{"status":"skipped"},"matchedIssues":[{"code":"S2","target":{"field":"markdown"},"resolution":"覆盖 XX 名用户","status":"fixed"}],"evidenceRequests":[],"warnings":[]}}`),
		},
		{
			name: "matched issue label",
			raw:  []byte(`{"rewrite":{"markdown":"主导产品交付"},"analysis":{"jdAlignment":{"status":"skipped"},"matchedIssues":[{"code":"S2","label":"XX 量化缺失","target":{"field":"markdown"},"resolution":"替换弱动词","status":"fixed"}],"evidenceRequests":[],"warnings":[]}}`),
		},
		{
			name: "evidence request reason",
			raw:  []byte(`{"rewrite":{"markdown":"主导产品交付"},"analysis":{"jdAlignment":{"status":"skipped"},"matchedIssues":[],"evidenceRequests":[{"code":"S1","target":{"field":"markdown"},"reason":"还需 XX 名用户规模","exampleWithPlaceholder":"覆盖 XX 名用户"}],"warnings":[]}}`),
		},
		{
			name: "warning resolution",
			raw:  []byte(`{"rewrite":{"markdown":"主导产品交付"},"analysis":{"jdAlignment":{"status":"skipped"},"matchedIssues":[],"evidenceRequests":[],"warnings":[{"code":"S8","resolution":"请补充 XX 数据"}]}}`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, analysis, ok, err := ai.ValidateOptimizationOutput(
				"summary",
				json.RawMessage(`{"markdown":"负责产品交付"}`),
				tt.raw,
			)

			require.NoError(t, err)
			assert.False(t, ok)
			assert.NotContains(t, string(analysis), "XX")
		})
	}
}

func TestOptimizationOutputDowngradesAnalysisForFrozenTarget(t *testing.T) {
	rewrite, analysis, ok, err := ai.ValidateOptimizationOutput(
		"work",
		json.RawMessage(`{"items":[{"id":"work-1","company":"Example Co","achievements":"负责产品交付","highlightsMarkdown":"- 负责交付"}]}`),
		[]byte(`{
			"rewrite":{"items":[{"id":"work-1","achievements":"主导产品交付","highlightsMarkdown":"- 主导交付"}]},
			"analysis":{
				"jdAlignment":{"status":"skipped"},
				"matchedIssues":[{"code":"S2","target":{"itemId":"work-1","field":"company"},"resolution":"替换弱动词","status":"fixed"}],
				"evidenceRequests":[],
				"warnings":[]
			}
		}`),
	)

	require.NoError(t, err)
	assert.False(t, ok)
	assert.JSONEq(t, `{"items":[{"id":"work-1","achievements":"主导产品交付","highlightsMarkdown":"- 主导交付"}]}`, string(rewrite))
	assert.Contains(t, string(analysis), "未能生成完整分析说明")
}

func TestOptimizationOutputDowngradesFixedIssueWithoutActualDiff(t *testing.T) {
	rewrite, analysis, ok, err := ai.ValidateOptimizationOutput(
		"summary",
		json.RawMessage(`{"markdown":"负责产品交付"}`),
		[]byte(`{
			"rewrite":{"markdown":"负责产品交付"},
			"analysis":{
				"jdAlignment":{"status":"skipped"},
				"matchedIssues":[{"code":"S2","target":{"field":"markdown"},"resolution":"替换弱动词","status":"fixed"}],
				"evidenceRequests":[],
				"warnings":[]
			}
		}`),
	)

	require.NoError(t, err)
	assert.False(t, ok)
	assert.JSONEq(t, `{"markdown":"负责产品交付"}`, string(rewrite))
	assert.Contains(t, string(analysis), "未能生成完整分析说明")
}

func TestOptimizationOutputDowngradesMatchedIssueWithNonFixedStatus(t *testing.T) {
	tests := []struct {
		name   string
		status string
	}{
		{name: "evidence marker", status: "XX"},
		{name: "unknown semantic status", status: "pending"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, analysis, ok, err := ai.ValidateOptimizationOutput(
				"summary",
				json.RawMessage(`{"markdown":"负责产品交付"}`),
				[]byte(`{"rewrite":{"markdown":"主导产品交付"},"analysis":{"jdAlignment":{"status":"skipped"},"matchedIssues":[{"code":"S2","target":{"field":"markdown"},"resolution":"替换弱动词","status":"`+tt.status+`"}],"evidenceRequests":[],"warnings":[]}}`),
			)

			require.NoError(t, err)
			assert.False(t, ok)
			assert.NotContains(t, string(analysis), tt.status)
		})
	}
}

func TestOptimizationOutputDowngradesS7RewriteTargetedFixedIssue(t *testing.T) {
	_, analysis, ok, err := ai.ValidateOptimizationOutput(
		"summary",
		json.RawMessage(`{"markdown":"负责产品交付"}`),
		[]byte(`{
			"rewrite":{"markdown":"主导产品交付"},
			"analysis":{
				"jdAlignment":{"status":"skipped"},
				"matchedIssues":[{"code":"S7","target":{"field":"markdown"},"resolution":"已重组既有事实","status":"fixed"}],
				"evidenceRequests":[],
				"warnings":[]
			}
		}`),
	)

	require.NoError(t, err)
	assert.False(t, ok)
	assert.Contains(t, string(analysis), "未能生成完整分析说明")
}

func TestOptimizationOutputNormalizesS7WarningWithoutPreservingInventedReason(t *testing.T) {
	_, analysis, ok, err := ai.ValidateOptimizationOutput(
		"summary",
		json.RawMessage(`{"markdown":"负责产品交付"}`),
		[]byte(`{
			"rewrite":{"markdown":"主导产品交付"},
			"analysis":{
				"jdAlignment":{"status":"skipped"},
				"matchedIssues":[],
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

func TestOptimizationOutputDowngradesFixedIssueForNullRewriteField(t *testing.T) {
	rewrite, analysis, ok, err := ai.ValidateOptimizationOutput(
		"work",
		json.RawMessage(`{"items":[{"id":"work-1","achievements":"负责产品交付","highlightsMarkdown":"- 负责交付"}]}`),
		[]byte(`{
			"rewrite":{"items":[{"id":"work-1","achievements":null,"highlightsMarkdown":"- 主导交付"}]},
			"analysis":{
				"jdAlignment":{"status":"skipped"},
				"matchedIssues":[{"code":"S2","target":{"itemId":"work-1","field":"achievements"},"resolution":"替换弱动词","status":"fixed"}],
				"evidenceRequests":[],
				"warnings":[]
			}
		}`),
	)

	require.NoError(t, err)
	assert.False(t, ok)
	assert.JSONEq(t, `{"items":[{"id":"work-1","achievements":null,"highlightsMarkdown":"- 主导交付"}]}`, string(rewrite))
	assert.Contains(t, string(analysis), "未能生成完整分析说明")
}

func TestOptimizationOutputRejectsFrozenFieldsInRewrite(t *testing.T) {
	_, _, ok, err := ai.ValidateOptimizationOutput(
		"work",
		json.RawMessage(`{"items":[{"id":"work-1","company":"Example Co","achievements":"负责产品交付","highlightsMarkdown":"- 负责交付"}]}`),
		[]byte(`{
			"rewrite":{"items":[{"id":"work-1","company":"Other Co","achievements":"主导产品交付","highlightsMarkdown":"- 主导交付"}]},
			"analysis":{"jdAlignment":{"status":"skipped"},"matchedIssues":[],"evidenceRequests":[],"warnings":[]}
		}`),
	)

	require.Error(t, err)
	assert.False(t, ok)
}

func TestOptimizationOutputPreservesNarrowRewriteEmptyStringIDSemantics(t *testing.T) {
	rewrite, _, ok, err := ai.ValidateOptimizationOutput(
		"work",
		json.RawMessage(`{"items":[{"id":"","achievements":"负责产品交付","highlightsMarkdown":"- 负责交付"}]}`),
		[]byte(`{
			"rewrite":{"items":[{"id":"","achievements":"主导产品交付","highlightsMarkdown":"- 主导交付"}]},
			"analysis":{"jdAlignment":{"status":"skipped"},"matchedIssues":[],"evidenceRequests":[],"warnings":[]}
		}`),
	)

	require.NoError(t, err)
	assert.True(t, ok)
	assert.JSONEq(t, `{"items":[{"id":"","achievements":"主导产品交付","highlightsMarkdown":"- 主导交付"}]}`, string(rewrite))
}

func TestOptimizationOutputRejectsTrailingJSONValue(t *testing.T) {
	_, _, ok, err := ai.ValidateOptimizationOutput(
		"summary",
		json.RawMessage(`{"markdown":"负责产品交付"}`),
		[]byte(`{"rewrite":{"markdown":"主导产品交付"},"analysis":{"jdAlignment":{"status":"skipped"},"matchedIssues":[],"evidenceRequests":[],"warnings":[]}} {}`),
	)

	require.Error(t, err)
	assert.False(t, ok)
}

func TestOptimizationOutputFallbackUsesSchemaAllowedWarningCode(t *testing.T) {
	_, analysis, ok, err := ai.ValidateOptimizationOutput(
		"summary",
		json.RawMessage(`{"markdown":"负责产品交付"}`),
		[]byte(`{"rewrite":{"markdown":"主导产品交付"},"analysis":{"jdAlignment":{"status":"skipped"},"matchedIssues":[],"evidenceRequests":[]}}`),
	)

	require.NoError(t, err)
	assert.False(t, ok)
	assert.JSONEq(t, `{"jdAlignment":{"status":"skipped"},"matchedIssues":[],"evidenceRequests":[],"warnings":[{"code":"S8","resolution":"未能生成完整分析说明。"}]}`, string(analysis))
}
