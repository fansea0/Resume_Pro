package schemas

import (
	"encoding/json"
	"fmt"
)

// GenerateJSONSchema returns the controlled JSON schema for a supported
// rewrite output type.
func GenerateJSONSchema(v any) (map[string]any, error) {
	switch v.(type) {
	case SummaryRewriteOutput:
		return summaryRewriteSchema(), nil
	case ExperienceRewriteOutput:
		return experienceRewriteSchema(), nil
	case ProjectRewriteOutput:
		return projectRewriteSchema(), nil
	default:
		return nil, fmt.Errorf("unsupported schema type: %T", v)
	}
}

// RewriteSchemaFor returns the rewrite schema for a supported resume section.
func RewriteSchemaFor(sectionType string) (any, error) {
	v, err := rewriteStructFor(sectionType)
	if err != nil {
		return nil, err
	}
	return GenerateJSONSchema(v)
}

func rewriteStructFor(sectionType string) (any, error) {
	switch sectionType {
	case "summary":
		return SummaryRewriteOutput{}, nil
	case "work", "internship":
		return ExperienceRewriteOutput{}, nil
	case "project":
		return ProjectRewriteOutput{}, nil
	default:
		return nil, fmt.Errorf("unsupported section type for AI rewrite: %s", sectionType)
	}
}

func summaryRewriteSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"markdown": map[string]any{
				"type":        "string",
				"description": "改写后的个人总结 markdown",
			},
		},
		"required": []string{"markdown"},
	}
}

func experienceRewriteSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"items": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"properties": map[string]any{
						"id":                 map[string]any{"type": "string"},
						"achievements":       map[string]any{"type": "string"},
						"highlightsMarkdown": map[string]any{"type": "string"},
					},
					"required": []string{"id"},
				},
			},
		},
		"required": []string{"items"},
	}
}

func projectRewriteSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"items": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"properties": map[string]any{
						"id":                 map[string]any{"type": "string"},
						"introduction":       map[string]any{"type": "string"},
						"achievements":       map[string]any{"type": "string"},
						"highlightsMarkdown": map[string]any{"type": "string"},
					},
					"required": []string{"id"},
				},
			},
		},
		"required": []string{"items"},
	}
}

// SchemaJSONString serializes a schema with the standard JSON encoder.
func SchemaJSONString(v map[string]any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}
