package ai

import "resumepro/backend/internal/ai/schemas"

// ApplySummaryRewrite replaces the original summary markdown with the rewrite.
func ApplySummaryRewrite(_, newMarkdown string) string {
	return newMarkdown
}

// ApplyExperienceItemsRewrite applies rewrites to matching experience items.
// Items and fields omitted from rewrites retain their original values.
func ApplyExperienceItemsRewrite(
	originals []schemas.ExperienceItem,
	rewrites []schemas.ExperienceRewriteItem,
) []schemas.ExperienceItem {
	rewriteByID := make(map[string]schemas.ExperienceRewriteItem, len(rewrites))
	for _, rewrite := range rewrites {
		rewriteByID[rewrite.ID] = rewrite
	}

	result := append([]schemas.ExperienceItem(nil), originals...)
	for index := range result {
		rewrite, ok := rewriteByID[result[index].ID]
		if !ok {
			continue
		}
		if rewrite.Achievements != nil {
			result[index].Achievements = *rewrite.Achievements
		}
		if rewrite.HighlightsMarkdown != nil {
			result[index].HighlightsMarkdown = *rewrite.HighlightsMarkdown
		}
	}

	return result
}

// ApplyProjectItemsRewrite applies rewrites to matching project items.
// Items and fields omitted from rewrites retain their original values.
func ApplyProjectItemsRewrite(
	originals []schemas.ProjectItem,
	rewrites []schemas.ProjectRewriteItem,
) []schemas.ProjectItem {
	rewriteByID := make(map[string]schemas.ProjectRewriteItem, len(rewrites))
	for _, rewrite := range rewrites {
		rewriteByID[rewrite.ID] = rewrite
	}

	result := append([]schemas.ProjectItem(nil), originals...)
	for index := range result {
		rewrite, ok := rewriteByID[result[index].ID]
		if !ok {
			continue
		}
		if rewrite.Introduction != nil {
			result[index].Introduction = *rewrite.Introduction
		}
		if rewrite.Achievements != nil {
			result[index].Achievements = *rewrite.Achievements
		}
		if rewrite.HighlightsMarkdown != nil {
			result[index].HighlightsMarkdown = *rewrite.HighlightsMarkdown
		}
	}

	return result
}
