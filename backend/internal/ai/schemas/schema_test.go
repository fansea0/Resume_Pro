package schemas

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestOptionalRewriteStringsDistinguishAbsentFromExplicitEmpty(t *testing.T) {
	t.Run("absent fields stay nil and are omitted", func(t *testing.T) {
		var got ProjectRewriteItem
		if err := json.Unmarshal([]byte(`{"id":"project-1"}`), &got); err != nil {
			t.Fatalf("unmarshal omitted rewrite fields: %v", err)
		}
		if got.Introduction != nil || got.Achievements != nil || got.HighlightsMarkdown != nil {
			t.Fatalf("omitted rewrite fields = %#v, want all nil", got)
		}

		encoded, err := json.Marshal(got)
		if err != nil {
			t.Fatalf("marshal omitted rewrite fields: %v", err)
		}
		if string(encoded) != `{"id":"project-1"}` {
			t.Fatalf("marshal omitted rewrite fields = %s", encoded)
		}

		var experience ExperienceRewriteItem
		if err := json.Unmarshal([]byte(`{"id":"experience-1"}`), &experience); err != nil {
			t.Fatalf("unmarshal omitted experience rewrite fields: %v", err)
		}
		if experience.Achievements != nil || experience.HighlightsMarkdown != nil {
			t.Fatalf("omitted experience rewrite fields = %#v, want all nil", experience)
		}
	})

	t.Run("explicit empty fields stay present", func(t *testing.T) {
		var got ProjectRewriteItem
		if err := json.Unmarshal([]byte(`{"id":"project-1","introduction":""}`), &got); err != nil {
			t.Fatalf("unmarshal explicit empty rewrite field: %v", err)
		}
		if got.Introduction == nil || *got.Introduction != "" {
			t.Fatalf("explicit empty introduction = %#v, want pointer to empty string", got.Introduction)
		}

		encoded, err := json.Marshal(got)
		if err != nil {
			t.Fatalf("marshal explicit empty rewrite field: %v", err)
		}
		if string(encoded) != `{"id":"project-1","introduction":""}` {
			t.Fatalf("marshal explicit empty rewrite field = %s", encoded)
		}

		var experience ExperienceRewriteItem
		if err := json.Unmarshal([]byte(`{"id":"experience-1","achievements":""}`), &experience); err != nil {
			t.Fatalf("unmarshal explicit empty experience rewrite field: %v", err)
		}
		if experience.Achievements == nil || *experience.Achievements != "" {
			t.Fatalf("explicit empty achievements = %#v, want pointer to empty string", experience.Achievements)
		}
	})
}

func TestRewriteItemsExposeOnlyIdentifiersAndRewriteableFields(t *testing.T) {
	tests := []struct {
		name       string
		value      any
		wantFields map[string]bool
	}{
		{
			name:  "experience",
			value: ExperienceRewriteItem{},
			wantFields: map[string]bool{
				"id": true, "achievements": true, "highlightsMarkdown": true,
			},
		},
		{
			name:  "project",
			value: ProjectRewriteItem{},
			wantFields: map[string]bool{
				"id": true, "introduction": true, "achievements": true, "highlightsMarkdown": true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			typ := reflect.TypeOf(tt.value)
			gotFields := make(map[string]bool, typ.NumField())
			for i := 0; i < typ.NumField(); i++ {
				jsonName := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
				gotFields[jsonName] = true
			}
			if !reflect.DeepEqual(gotFields, tt.wantFields) {
				t.Fatalf("rewrite JSON fields = %v, want %v", gotFields, tt.wantFields)
			}
		})
	}
}

func TestSchemaFieldsHaveJSONAndDescriptionTags(t *testing.T) {
	types := []any{
		SummarySectionContent{},
		ExperienceItem{},
		WorkSectionContent{},
		ProjectItem{},
		ProjectSectionContent{},
		SummaryRewriteOutput{},
		ExperienceRewriteOutput{},
		ExperienceRewriteItem{},
		ProjectRewriteOutput{},
		ProjectRewriteItem{},
	}

	for _, value := range types {
		typ := reflect.TypeOf(value)
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			if field.Tag.Get("json") == "" {
				t.Errorf("%s.%s has no json tag", typ.Name(), field.Name)
			}
			if !strings.Contains(field.Tag.Get("jsonschema"), "description=") {
				t.Errorf("%s.%s has no jsonschema description", typ.Name(), field.Name)
			}
		}
	}
}
