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
// The server derives the rewrite from the accepted suggestions, so this schema
// describes the shape the client receives rather than the model output.
func RewriteSchemaFor(sectionType string) (any, error) {
	v, err := rewriteStructFor(sectionType)
	if err != nil {
		return nil, err
	}
	return GenerateJSONSchema(v)
}

// OptimizationOutputFor returns the strict response schema the model must
// follow. The model proposes the rewritten text of each field; the server
// diffs that text against the current content and stays the single source of
// truth for the resulting resume content.
func OptimizationOutputFor(sectionType string) (any, error) {
	if _, err := rewriteStructFor(sectionType); err != nil {
		return nil, err
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"edits": map[string]any{
				"type":  "array",
				"items": sectionEditSchema(),
			},
			"analysis": optimizationAnalysisSchema(),
		},
		"required": []string{"edits", "analysis"},
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

// sectionEditSchema is the model-facing shape of one proposed field rewrite.
func sectionEditSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"target":  editTargetSchema(),
			"newText": map[string]any{"type": "string"},
			"issues": map[string]any{
				"type":  "array",
				"items": editIssueSchema(),
			},
		},
		"required": []string{"target", "newText", "issues"},
	}
}

func editTargetSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"itemId": map[string]any{"type": []string{"string", "null"}},
			"field":  map[string]any{"type": "string"},
		},
		"required": []string{"itemId", "field"},
	}
}

func editIssueSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"code":      map[string]any{"type": "string", "enum": []string{"S2", "S3", "S4", "S5", "S6", "S8", "S10"}},
			"label":     map[string]any{"type": "string"},
			"issue":     map[string]any{"type": "string"},
			"rationale": map[string]any{"type": "string"},
			"quote":     map[string]any{"type": "string"},
		},
		"required": []string{"code", "label", "issue", "rationale", "quote"},
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
					"missingKeywords":  stringArraySchema(),
				},
				"required": []string{"status", "matchedKeywords", "excludedKeywords", "missingKeywords"},
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
		"required": []string{"jdAlignment", "evidenceRequests", "warnings"},
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
