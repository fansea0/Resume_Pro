package llm

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponseFormatExtraFields_UsesStrictOpenAIJSONSchemaShape(t *testing.T) {
	extraFields := responseFormatExtraFields(&JSONSchemaResponseFormat{
		Name:        "summary_rewrite",
		Description: "summary output",
		Strict:      true,
		Schema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
		},
	})

	encoded, err := json.Marshal(extraFields)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"response_format": {
			"type": "json_schema",
			"json_schema": {
				"name": "summary_rewrite",
				"description": "summary output",
				"strict": true,
				"schema": {"type": "object", "additionalProperties": false}
			}
		}
	}`, string(encoded))
}
