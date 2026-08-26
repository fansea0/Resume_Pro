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
