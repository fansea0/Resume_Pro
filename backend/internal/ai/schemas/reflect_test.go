package schemas_test

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"resumepro/backend/internal/ai/schemas"
)

func TestRewriteSchemaForReturnsSchemaForSupportedSectionTypes(t *testing.T) {
	tests := []struct {
		sectionType string
		want        map[string]any
	}{
		{
			sectionType: "summary",
			want: map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"markdown": map[string]any{"type": "string", "description": "改写后的个人总结 markdown"},
				},
				"required": []string{"markdown"},
			},
		},
		{
			sectionType: "work",
			want:        experienceSchema(),
		},
		{
			sectionType: "internship",
			want:        experienceSchema(),
		},
		{
			sectionType: "project",
			want: map[string]any{
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
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.sectionType, func(t *testing.T) {
			got, err := schemas.RewriteSchemaFor(tt.sectionType)
			if err != nil {
				t.Fatalf("RewriteSchemaFor(%q) error: %v", tt.sectionType, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("RewriteSchemaFor(%q) = %#v, want %#v", tt.sectionType, got, tt.want)
			}
		})
	}
}

func TestRewriteSchemaForRejectsUnsupportedSectionType(t *testing.T) {
	got, err := schemas.RewriteSchemaFor("education")
	if err == nil {
		t.Fatal("RewriteSchemaFor(education) error = nil, want unsupported section error")
	}
	if got != nil {
		t.Fatalf("RewriteSchemaFor(education) schema = %#v, want nil", got)
	}
}

func TestGenerateJSONSchemaRejectsUnsupportedType(t *testing.T) {
	got, err := schemas.GenerateJSONSchema(struct{}{})
	if err == nil {
		t.Fatal("GenerateJSONSchema(struct{}{}) error = nil, want unsupported type error")
	}
	if got != nil {
		t.Fatalf("GenerateJSONSchema(struct{}{}) schema = %#v, want nil", got)
	}
}

func TestRewriteSchemasExcludeFrozenFields(t *testing.T) {
	tests := []struct {
		name      string
		value     any
		forbidden []string
	}{
		{name: "summary", value: schemas.SummaryRewriteOutput{}},
		{
			name:  "work",
			value: schemas.ExperienceRewriteOutput{},
			forbidden: []string{
				"company", "role", "startDate", "endDate", "location",
				"showLocation", "showAchievements",
			},
		},
		{
			name:  "project",
			value: schemas.ProjectRewriteOutput{},
			forbidden: []string{
				"name", "role", "startDate", "endDate", "location", "link",
				"showLocation", "showLink", "showIntroduction", "showAchievements",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema, err := schemas.GenerateJSONSchema(tt.value)
			if err != nil {
				t.Fatalf("GenerateJSONSchema(%T) error: %v", tt.value, err)
			}
			encoded := schemas.SchemaJSONString(schema)
			for _, field := range tt.forbidden {
				if strings.Contains(encoded, `"`+field+`"`) {
					t.Errorf("rewrite schema for %s contains frozen field %q: %s", tt.name, field, encoded)
				}
			}
		})
	}
}

func TestAllRewriteSchemaObjectsRejectAdditionalProperties(t *testing.T) {
	values := []any{
		schemas.SummaryRewriteOutput{},
		schemas.ExperienceRewriteOutput{},
		schemas.ProjectRewriteOutput{},
	}
	for _, value := range values {
		schema, err := schemas.GenerateJSONSchema(value)
		if err != nil {
			t.Fatalf("GenerateJSONSchema(%T) error: %v", value, err)
		}
		assertObjectSchemasAreStrict(t, "$", schema)
	}
}

func TestSchemaJSONStringUsesJSONEncoding(t *testing.T) {
	got := schemas.SchemaJSONString(map[string]any{"description": "line\n\"quoted\""})
	if !json.Valid([]byte(got)) {
		t.Fatalf("SchemaJSONString returned invalid JSON: %q", got)
	}
	if got != `{"description":"line\n\"quoted\""}` {
		t.Fatalf("SchemaJSONString() = %q, want encoding/json output", got)
	}
}

func experienceSchema() map[string]any {
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

func assertObjectSchemasAreStrict(t *testing.T, path string, node any) {
	t.Helper()

	object, ok := node.(map[string]any)
	if !ok {
		return
	}
	if object["type"] == "object" && object["additionalProperties"] != false {
		t.Errorf("object schema at %s has additionalProperties = %#v, want false", path, object["additionalProperties"])
	}
	for key, child := range object {
		switch value := child.(type) {
		case map[string]any:
			assertObjectSchemasAreStrict(t, path+"."+key, value)
		case []any:
			for i, item := range value {
				assertObjectSchemasAreStrict(t, path+"."+key+"["+strconv.Itoa(i)+"]", item)
			}
		}
	}
}
