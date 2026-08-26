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

// OptimizationOutputFor returns the strict rewrite-and-analysis response
// schema for a supported resume section.
func OptimizationOutputFor(sectionType string) (any, error) {
	rewrite, err := RewriteSchemaFor(sectionType)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"rewrite":  rewrite,
			"analysis": optimizationAnalysisSchema(),
		},
		"required": []string{"rewrite", "analysis"},
	}, nil
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
						"achievements":       map[string]any{"type": []string{"string", "null"}},
						"highlightsMarkdown": map[string]any{"type": []string{"string", "null"}},
					},
					"required": []string{"id", "achievements", "highlightsMarkdown"},
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
						"introduction":       map[string]any{"type": []string{"string", "null"}},
						"achievements":       map[string]any{"type": []string{"string", "null"}},
						"highlightsMarkdown": map[string]any{"type": []string{"string", "null"}},
					},
					"required": []string{"id", "introduction", "achievements", "highlightsMarkdown"},
				},
			},
		},
		"required": []string{"items"},
	}
}

func optimizationAnalysisSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"jdAlignment": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"status":           map[string]any{"type": "string", "enum": []string{"applied", "skipped"}},
					"matchedKeywords":  stringArraySchema(),
					"excludedKeywords": stringArraySchema(),
				},
				"required": []string{"status"},
			},
			"matchedIssues": map[string]any{
				"type":  "array",
				"items": issueSchema(),
			},
			"evidenceRequests": map[string]any{
				"type":  "array",
				"items": evidenceRequestSchema(),
			},
			"warnings": map[string]any{
				"type":  "array",
				"items": warningSchema(),
			},
		},
		"required": []string{"jdAlignment", "matchedIssues", "evidenceRequests", "warnings"},
	}
}

func stringArraySchema() map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{"type": "string"}}
}

func issueTargetSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"itemId": map[string]any{"type": "string"},
			"field":  map[string]any{"type": "string"},
		},
		"required": []string{"field"},
	}
}

func issueSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"code":       map[string]any{"type": "string", "enum": []string{"S2", "S3", "S4", "S5", "S6", "S7", "S8"}},
			"label":      map[string]any{"type": "string"},
			"target":     issueTargetSchema(),
			"resolution": map[string]any{"type": "string"},
			"status":     map[string]any{"type": "string"},
		},
		"required": []string{"code", "target", "resolution", "status"},
	}
}

func evidenceRequestSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"code":                   map[string]any{"type": "string", "enum": []string{"S1"}},
			"target":                 issueTargetSchema(),
			"reason":                 map[string]any{"type": "string"},
			"exampleWithPlaceholder": map[string]any{"type": "string"},
		},
		"required": []string{"code", "target", "reason", "exampleWithPlaceholder"},
	}
}

func warningSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"code":       map[string]any{"type": "string", "enum": []string{"S2", "S3", "S4", "S5", "S6", "S7", "S8"}},
			"resolution": map[string]any{"type": "string"},
		},
		"required": []string{"code", "resolution"},
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
