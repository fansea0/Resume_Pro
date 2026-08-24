package ai_test

import (
	"reflect"
	"testing"

	"resumepro/backend/internal/ai"
	"resumepro/backend/internal/ai/schemas"
)

func stringPointer(value string) *string {
	return &value
}

func TestApplySummaryRewriteUsesNewMarkdown(t *testing.T) {
	if got := ai.ApplySummaryRewrite("old summary", "new summary"); got != "new summary" {
		t.Fatalf("ApplySummaryRewrite() = %q, want %q", got, "new summary")
	}
}

func TestApplyExperienceItemsRewriteAppliesOnlyPresentRewriteableFields(t *testing.T) {
	originals := []schemas.ExperienceItem{
		{
			ID:                 "experience-1",
			Company:            "Original company",
			Role:               "Original role",
			StartDate:          "2024-01",
			EndDate:            "2025-01",
			Location:           "Shanghai",
			Achievements:       "Original achievement",
			HighlightsMarkdown: "Original highlights",
			ShowLocation:       true,
			ShowAchievements:   true,
		},
		{
			ID:                 "experience-2",
			Achievements:       "Preserved achievement",
			HighlightsMarkdown: "Preserved highlights",
		},
	}
	rewrites := []schemas.ExperienceRewriteItem{{
		ID:                 "experience-1",
		Achievements:       stringPointer(""),
		HighlightsMarkdown: stringPointer("Rewritten highlights"),
	}, {
		ID:                 "experience-2",
		Achievements:       nil,
		HighlightsMarkdown: nil,
	}}

	got := ai.ApplyExperienceItemsRewrite(originals, rewrites)
	want := []schemas.ExperienceItem{
		{
			ID:                 "experience-1",
			Company:            "Original company",
			Role:               "Original role",
			StartDate:          "2024-01",
			EndDate:            "2025-01",
			Location:           "Shanghai",
			Achievements:       "",
			HighlightsMarkdown: "Rewritten highlights",
			ShowLocation:       true,
			ShowAchievements:   true,
		},
		{
			ID:                 "experience-2",
			Achievements:       "Preserved achievement",
			HighlightsMarkdown: "Preserved highlights",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ApplyExperienceItemsRewrite() = %#v, want %#v", got, want)
	}
}

func TestApplyExperienceItemsRewritePreservesOriginalOrderAndDiscardsUnknownIDs(t *testing.T) {
	originals := []schemas.ExperienceItem{
		{ID: "c", Achievements: "original c"},
		{ID: "a", Achievements: "original a"},
		{ID: "b", Achievements: "original b"},
	}
	rewrites := []schemas.ExperienceRewriteItem{
		{ID: "ghost", Achievements: stringPointer("must not be added")},
		{ID: "b", Achievements: stringPointer("rewritten b")},
		{ID: "a", Achievements: stringPointer("rewritten a")},
	}

	got := ai.ApplyExperienceItemsRewrite(originals, rewrites)
	want := []schemas.ExperienceItem{
		{ID: "c", Achievements: "original c"},
		{ID: "a", Achievements: "rewritten a"},
		{ID: "b", Achievements: "rewritten b"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ApplyExperienceItemsRewrite() = %#v, want %#v", got, want)
	}
}

func TestApplyProjectItemsRewriteAppliesOnlyPresentRewriteableFields(t *testing.T) {
	originals := []schemas.ProjectItem{{
		ID:                 "project-1",
		Name:               "Original name",
		Role:               "Original role",
		StartDate:          "2024-01",
		EndDate:            "2025-01",
		Location:           "Beijing",
		Link:               "https://example.com/original",
		Introduction:       "Original introduction",
		Achievements:       "Original achievement",
		HighlightsMarkdown: "Original highlights",
		ShowLocation:       true,
		ShowLink:           true,
		ShowIntroduction:   true,
		ShowAchievements:   true,
	}}
	rewrites := []schemas.ProjectRewriteItem{{
		ID:                 "project-1",
		Introduction:       nil,
		Achievements:       stringPointer("Rewritten achievement"),
		HighlightsMarkdown: stringPointer(""),
	}}

	got := ai.ApplyProjectItemsRewrite(originals, rewrites)
	want := []schemas.ProjectItem{{
		ID:                 "project-1",
		Name:               "Original name",
		Role:               "Original role",
		StartDate:          "2024-01",
		EndDate:            "2025-01",
		Location:           "Beijing",
		Link:               "https://example.com/original",
		Introduction:       "Original introduction",
		Achievements:       "Rewritten achievement",
		HighlightsMarkdown: "",
		ShowLocation:       true,
		ShowLink:           true,
		ShowIntroduction:   true,
		ShowAchievements:   true,
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ApplyProjectItemsRewrite() = %#v, want %#v", got, want)
	}
}

func TestApplyProjectItemsRewritePreservesOriginalOrderAndDiscardsUnknownIDs(t *testing.T) {
	originals := []schemas.ProjectItem{
		{ID: "c", Introduction: "original c"},
		{ID: "a", Introduction: "original a"},
		{ID: "b", Introduction: "original b"},
	}
	rewrites := []schemas.ProjectRewriteItem{
		{ID: "ghost", Introduction: stringPointer("must not be added")},
		{ID: "b", Introduction: stringPointer("rewritten b")},
		{ID: "a", Introduction: stringPointer("rewritten a")},
	}

	got := ai.ApplyProjectItemsRewrite(originals, rewrites)
	want := []schemas.ProjectItem{
		{ID: "c", Introduction: "original c"},
		{ID: "a", Introduction: "rewritten a"},
		{ID: "b", Introduction: "rewritten b"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ApplyProjectItemsRewrite() = %#v, want %#v", got, want)
	}
}
