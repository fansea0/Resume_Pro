package ai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"resumepro/backend/internal/ai/schemas"
)

// densityLimit describes the deterministic information-density budget of one
// rewriteable field.
type densityLimit struct {
	maxChars       int
	maxLines       int
	maxBullets     int
	maxBulletChars int
}

// densityLimits are the section-level information-density budgets. They are
// proxies for "one line, one point" and never claim to predict real page
// counts.
var densityLimits = map[string]densityLimit{
	"markdown":           {maxChars: 150, maxLines: 4},
	"achievements":       {maxChars: 60, maxLines: 2},
	"introduction":       {maxChars: 60, maxLines: 1},
	"highlightsMarkdown": {maxBullets: 5, maxBulletChars: 40},
}

var numberPattern = regexp.MustCompile(`\d+(?:\.\d+)?`)

// sectionItem is one rewriteable entry of the section under review.
type sectionItem struct {
	id     string
	fields map[string]string
}

// sectionView is a read-only projection of the current section content.
type sectionView struct {
	sectionType string
	items       []sectionItem
}

// ErrInvalidCurrentContent marks a failure caused by the stored resume
// content rather than by the model output. Such failures are not repairable
// by asking the model again.
var ErrInvalidCurrentContent = errors.New("invalid current content")

// ValidateOptimizationOutput validates a model response without changing
// resume state. The server keeps ownership of the final text: it diffs the
// rewritten text proposed by the model against the current content, keeps only
// the suggestions that are grounded in the original facts, and derives the
// rewrite by applying those suggestions.
func ValidateOptimizationOutput(sectionType string, currentContent json.RawMessage, raw []byte) (rewrite json.RawMessage, analysis json.RawMessage, analysisOK bool, err error) {
	modelOutput, err := decodeModelOutput(raw)
	if err != nil {
		return nil, nil, false, err
	}
	current, err := decodeJSONObject(currentContent, "current content")
	if err != nil {
		return nil, nil, false, fmt.Errorf("%w: %v", ErrInvalidCurrentContent, err)
	}
	view, err := newSectionView(sectionType, current)
	if err != nil {
		return nil, nil, false, fmt.Errorf("%w: %v", ErrInvalidCurrentContent, err)
	}

	suggestions, editWarnings := view.resolveEdits(modelOutput.Edits)
	rewriteValue := view.deriveRewrite(suggestions)
	rewriteBytes, err := json.Marshal(rewriteValue)
	if err != nil {
		return nil, nil, false, fmt.Errorf("encode derived rewrite: %w", err)
	}
	rewriteSchemaValue, err := schemas.RewriteSchemaFor(sectionType)
	if err != nil {
		return nil, nil, false, err
	}
	rewriteSchema, ok := rewriteSchemaValue.(map[string]any)
	if !ok {
		return nil, nil, false, fmt.Errorf("invalid rewrite schema type: %T", rewriteSchemaValue)
	}
	if err := validateRewriteResponse(rewriteBytes, rewriteSchema); err != nil {
		return nil, nil, false, fmt.Errorf("derived rewrite is invalid: %w", err)
	}
	if containsUnsafePlaceholder(rewriteBytes) {
		return nil, nil, false, errors.New("derived rewrite contains an evidence placeholder")
	}

	analysisValue, analysisOK := view.buildAnalysis(modelOutput.Analysis, suggestions, editWarnings)
	analysisBytes, err := json.Marshal(analysisValue)
	if err != nil {
		return rewriteBytes, fallbackAnalysisJSON(), false, nil
	}
	return rewriteBytes, analysisBytes, analysisOK, nil
}

type modelOutput struct {
	Edits    []json.RawMessage
	Analysis json.RawMessage
}

func decodeModelOutput(raw []byte) (modelOutput, error) {
	object, err := decodeJSONObject(raw, "optimization output")
	if err != nil {
		return modelOutput{}, err
	}
	if err := requireExactKeys(object, "optimization output", "edits", "analysis"); err != nil {
		return modelOutput{}, err
	}
	items, ok := object["edits"].([]any)
	if !ok {
		return modelOutput{}, errors.New("optimization output edits must be an array")
	}
	edits := make([]json.RawMessage, 0, len(items))
	for index, item := range items {
		encoded, err := json.Marshal(item)
		if err != nil {
			return modelOutput{}, fmt.Errorf("encode edit %d: %w", index, err)
		}
		edits = append(edits, encoded)
	}
	analysis, err := marshalJSONValue(object["analysis"])
	if err != nil {
		return modelOutput{}, fmt.Errorf("encode analysis: %w", err)
	}
	return modelOutput{Edits: edits, Analysis: analysis}, nil
}

func newSectionView(sectionType string, current map[string]any) (sectionView, error) {
	switch sectionType {
	case "summary":
		markdown, ok := current["markdown"].(string)
		if !ok {
			return sectionView{}, errors.New("summary content markdown must be a string")
		}
		return sectionView{
			sectionType: sectionType,
			items:       []sectionItem{{id: "", fields: map[string]string{"markdown": markdown}}},
		}, nil
	case "work", "internship", "project":
		rawItems, ok := current["items"].([]any)
		if !ok {
			return sectionView{}, errors.New("current content items must be an array")
		}
		fields := rewriteableFields(sectionType)
		items := make([]sectionItem, 0, len(rawItems))
		for index, rawItem := range rawItems {
			item, ok := rawItem.(map[string]any)
			if !ok {
				return sectionView{}, fmt.Errorf("current content item %d must be an object", index)
			}
			id, ok := item["id"].(string)
			if !ok {
				return sectionView{}, fmt.Errorf("current content item %d id must be a string", index)
			}
			itemFields := make(map[string]string, len(fields))
			for _, field := range fields {
				text, _ := item[field].(string)
				itemFields[field] = text
			}
			items = append(items, sectionItem{id: id, fields: itemFields})
		}
		return sectionView{sectionType: sectionType, items: items}, nil
	default:
		return sectionView{}, fmt.Errorf("unsupported section type for AI rewrite: %s", sectionType)
	}
}

func rewriteableFields(sectionType string) []string {
	switch sectionType {
	case "summary":
		return []string{"markdown"}
	case "project":
		return []string{"introduction", "achievements", "highlightsMarkdown"}
	default:
		return []string{"achievements", "highlightsMarkdown"}
	}
}

func (view sectionView) item(id string) (sectionItem, bool) {
	for _, item := range view.items {
		if item.id == id {
			return item, true
		}
	}
	return sectionItem{}, false
}

// resolveEdits turns the rewritten texts proposed by the model into
// addressable suggestions. Every suggestion is checked against the original
// content; unsafe ones are rejected with a reason instead of failing the whole
// response.
func (view sectionView) resolveEdits(rawEdits []json.RawMessage) ([]schemas.Suggestion, []schemas.Warning) {
	suggestions := make([]schemas.Suggestion, 0)
	warnings := make([]schemas.Warning, 0)

	for _, raw := range rawEdits {
		edit, err := decodeEdit(raw)
		if err != nil {
			warnings = append(warnings, schemas.Warning{Code: "S8", Resolution: "有一条改写建议无法解析，已忽略。"})
			continue
		}
		if !allowedTargetField(view.sectionType, edit.Target.Field) {
			warnings = append(warnings, schemas.Warning{Code: "S8", Resolution: "有一条改写建议指向不可改写的字段，已忽略。"})
			continue
		}
		target, err := view.validateTargetValues(edit.Target.ItemID, edit.Target.Field)
		if err != nil {
			warnings = append(warnings, schemas.Warning{Code: "S8", Resolution: "有一条改写建议定位不到目标条目，已忽略。"})
			continue
		}
		item, found := view.item(target.ItemID)
		if !found {
			warnings = append(warnings, schemas.Warning{Code: "S8", Resolution: "有一条改写建议定位不到目标条目，已忽略。"})
			continue
		}
		original := item.fields[target.Field]
		if edit.NewText == original {
			continue
		}

		hunks := diffText(original, edit.NewText)
		if onlyInsertions(hunks) {
			warnings = append(warnings, schemas.Warning{
				Code:       "S8",
				Resolution: "模型只追加了原文没有的内容，该字段的建议已忽略。",
			})
			continue
		}

		fieldSuggestions := view.buildFieldSuggestions(target, original, edit, hunks)
		for _, suggestion := range fieldSuggestions {
			suggestions = append(suggestions, suggestion)
		}
	}

	sortSuggestionsBySeverity(suggestions)
	for index := range suggestions {
		suggestions[index].ID = fmt.Sprintf("sg-%d", index+1)
	}
	return suggestions, warnings
}

// buildFieldSuggestions converts the diff hunks of one edited field into
// suggestions and attaches the model-provided issues to them.
func (view sectionView) buildFieldSuggestions(
	target schemas.IssueTarget,
	original string,
	edit schemas.SectionEdit,
	hunks []diffHunk,
) []schemas.Suggestion {
	revisedRunes := []rune(edit.NewText)
	suggestions := make([]schemas.Suggestion, 0, len(hunks))
	acceptedIndexes := make([]int, 0, len(hunks))

	for index, hunk := range hunks {
		if hunk.Original == hunk.Revised {
			continue
		}
		suggestion := schemas.Suggestion{
			Target:      target,
			Issue:       "",
			Anchor:      hunk.Original,
			Replacement: hunk.Revised,
			Rationale:   "",
			Start:       hunk.OriginalStart,
			End:         hunk.OriginalEnd,
			Status:      schemas.SuggestionStatusApplied,
		}
		if reason := view.checkHunkSafety(hunk, original); reason != "" {
			suggestion.Status = schemas.SuggestionStatusRejected
			suggestion.RejectionReason = reason
			suggestion.Severity = schemas.SuggestionSeverityMedium
			suggestions = append(suggestions, suggestion)
			continue
		}
		acceptedIndexes = append(acceptedIndexes, index)
		suggestions = append(suggestions, suggestion)
	}

	attachIssues(suggestions, acceptedIndexes, hunks, edit.Issues, revisedRunes)
	return suggestions
}

// checkHunkSafety rejects a diff hunk that would introduce or remove data the
// original content does not support.
func (view sectionView) checkHunkSafety(hunk diffHunk, original string) string {
	if containsUnsafePlaceholderText(hunk.Revised) {
		return "建议内容包含占位符"
	}
	if hunk.Revised == "" && containsNumber(hunk.Original) {
		return "建议删除了原文中的数据"
	}
	if !numbersGrounded(hunk.Revised, original) {
		return "建议引入了原文中没有的数据"
	}
	return ""
}

// attachIssues maps every issue reported by the model to the suggestion whose
// revised text overlaps the quoted fragment.
func attachIssues(
	suggestions []schemas.Suggestion,
	acceptedIndexes []int,
	hunks []diffHunk,
	issues []schemas.EditIssue,
	revisedRunes []rune,
) {
	for _, issue := range issues {
		code := strings.TrimSpace(issue.Code)
		if !isSuggestionCode(code) {
			continue
		}
		quoteRunes := []rune(issue.Quote)
		if len(quoteRunes) == 0 {
			continue
		}
		start, ok := uniqueIndexOf(revisedRunes, quoteRunes)
		if !ok {
			continue
		}
		end := start + len(quoteRunes)
		bestIndex := -1
		bestOverlap := 0
		for _, index := range acceptedIndexes {
			hunk := hunks[index]
			overlap := min(end, hunk.RevisedEnd) - max(start, hunk.RevisedStart)
			if overlap > bestOverlap {
				bestOverlap = overlap
				bestIndex = index
			}
		}
		if bestIndex < 0 || bestOverlap <= 0 {
			continue
		}
		for position := range suggestions {
			if suggestions[position].Start != hunks[bestIndex].OriginalStart ||
				suggestions[position].End != hunks[bestIndex].OriginalEnd {
				continue
			}
			if suggestions[position].Code != "" {
				break
			}
			suggestions[position].Code = code
			suggestions[position].Label = issue.Label
			suggestions[position].Issue = issue.Issue
			suggestions[position].Rationale = issue.Rationale
			suggestions[position].Severity = severityForCode(code)
			break
		}
	}
}

// deriveRewrite applies the accepted suggestions to the original content and
// returns the resulting section content.
func (view sectionView) deriveRewrite(suggestions []schemas.Suggestion) map[string]any {
	type fieldEdit struct {
		start       int
		end         int
		replacement string
	}
	byField := map[string][]fieldEdit{}
	for _, suggestion := range suggestions {
		if suggestion.Status != schemas.SuggestionStatusApplied {
			continue
		}
		key := targetKey(suggestion.Target.ItemID, suggestion.Target.Field)
		byField[key] = append(byField[key], fieldEdit{
			start:       suggestion.Start,
			end:         suggestion.End,
			replacement: suggestion.Replacement,
		})
	}

	derived := map[string]string{}
	for key, edits := range byField {
		item, found := view.itemByKey(key)
		if !found {
			continue
		}
		sort.SliceStable(edits, func(left, right int) bool {
			return edits[left].start > edits[right].start
		})
		runes := []rune(item.fields[fieldOfKey(key)])
		for _, edit := range edits {
			if edit.start < 0 || edit.end > len(runes) || edit.start > edit.end {
				continue
			}
			replacement := []rune(edit.replacement)
			updated := make([]rune, 0, len(runes)-len(replacement)+len(replacement))
			updated = append(updated, runes[:edit.start]...)
			updated = append(updated, replacement...)
			updated = append(updated, runes[edit.end:]...)
			runes = updated
		}
		derived[key] = string(runes)
	}

	if view.sectionType == "summary" {
		text, ok := derived[targetKey("", "markdown")]
		if !ok {
			item, _ := view.item("")
			text = item.fields["markdown"]
		}
		return map[string]any{"markdown": text}
	}

	items := make([]any, 0, len(view.items))
	for _, item := range view.items {
		changed := false
		entry := map[string]any{"id": item.id}
		for _, field := range rewriteableFields(view.sectionType) {
			text, ok := derived[targetKey(item.id, field)]
			if !ok {
				text = item.fields[field]
			} else {
				changed = true
			}
			entry[field] = text
		}
		if changed {
			items = append(items, entry)
		}
	}
	return map[string]any{"items": items}
}

func (view sectionView) itemByKey(key string) (sectionItem, bool) {
	itemID, field := splitTargetKey(key)
	item, found := view.item(itemID)
	if !found {
		return sectionItem{}, false
	}
	if _, ok := item.fields[field]; !ok {
		return sectionItem{}, false
	}
	return item, true
}

func (view sectionView) buildAnalysis(
	rawAnalysis json.RawMessage,
	suggestions []schemas.Suggestion,
	editWarnings []schemas.Warning,
) (clientAnalysis, bool) {
	analysis := clientAnalysis{
		JDAlignment:      schemas.JDAlignment{Status: "skipped"},
		Suggestions:      suggestions,
		EvidenceRequests: make([]schemas.EvidenceRequest, 0),
		Warnings:         editWarnings,
		Density:          view.densityMetrics(),
	}
	degraded := false
	object, err := decodeJSONObject(rawAnalysis, "analysis")
	if err != nil {
		degraded = true
		analysis.Warnings = append(analysis.Warnings, schemas.Warning{
			Code:       "S8",
			Resolution: "未能生成完整分析说明。",
		})
		return analysis, false
	}
	if err := requireExactKeys(object, "analysis", "jdAlignment", "evidenceRequests", "warnings"); err != nil {
		degraded = true
		analysis.Warnings = append(analysis.Warnings, schemas.Warning{
			Code:       "S8",
			Resolution: "未能生成完整分析说明。",
		})
		return analysis, false
	}
	alignment, err := validateJDAlignment(object["jdAlignment"])
	if err != nil {
		degraded = true
		analysis.Warnings = append(analysis.Warnings, schemas.Warning{
			Code:       "S8",
			Resolution: "未能生成完整分析说明。",
		})
		return analysis, false
	}
	analysis.JDAlignment = alignment
	if requests, err := view.validateEvidenceRequests(object["evidenceRequests"]); err == nil {
		analysis.EvidenceRequests = requests
	} else {
		degraded = true
	}
	if warnings, err := validateWarnings(object["warnings"]); err == nil {
		analysis.Warnings = append(analysis.Warnings, warnings...)
	} else {
		degraded = true
	}
	return analysis, !degraded
}

func (view sectionView) validateEvidenceRequests(value any) ([]schemas.EvidenceRequest, error) {
	requests, ok := value.([]any)
	if !ok {
		return nil, errors.New("analysis evidenceRequests must be an array")
	}
	result := make([]schemas.EvidenceRequest, 0, len(requests))
	for index, value := range requests {
		request, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("analysis evidenceRequests %d must be an object", index)
		}
		if err := requireExactKeys(request, fmt.Sprintf("analysis evidenceRequests %d", index), "code", "target", "reason", "exampleWithPlaceholder"); err != nil {
			return nil, err
		}
		code, ok := request["code"].(string)
		if !ok || code != "S1" {
			return nil, fmt.Errorf("analysis evidenceRequests %d code must be S1", index)
		}
		target, err := view.validateTarget(request["target"])
		if err != nil {
			return nil, fmt.Errorf("analysis evidenceRequests %d: %w", index, err)
		}
		reason, ok := request["reason"].(string)
		if !ok {
			return nil, fmt.Errorf("analysis evidenceRequests %d reason must be a string", index)
		}
		if containsUnsafePlaceholderText(reason) {
			return nil, fmt.Errorf("analysis evidenceRequests %d reason contains an evidence placeholder", index)
		}
		example, ok := request["exampleWithPlaceholder"].(string)
		if !ok || !strings.Contains(example, "XX") {
			return nil, fmt.Errorf("analysis evidenceRequests %d exampleWithPlaceholder must contain XX", index)
		}
		result = append(result, schemas.EvidenceRequest{
			Code:                   code,
			Target:                 target,
			Reason:                 reason,
			ExampleWithPlaceholder: example,
		})
	}
	return result, nil
}

func (view sectionView) validateTarget(value any) (schemas.IssueTarget, error) {
	target, ok := value.(map[string]any)
	if !ok {
		return schemas.IssueTarget{}, errors.New("target must be an object")
	}
	if err := requireAllowedAndRequiredKeys(target, "target", []string{"field"}, []string{"itemId", "field"}); err != nil {
		return schemas.IssueTarget{}, err
	}
	field, _ := target["field"].(string)
	itemID := ""
	if rawItemID, present := target["itemId"]; present && rawItemID != nil {
		itemID, ok = rawItemID.(string)
		if !ok {
			return schemas.IssueTarget{}, errors.New("target itemId must be a string")
		}
	}
	return view.validateTargetValues(itemID, field)
}

// validateTargetValues checks that a target points at a rewriteable field of
// an existing item.
func (view sectionView) validateTargetValues(itemID, field string) (schemas.IssueTarget, error) {
	if !allowedTargetField(view.sectionType, field) {
		return schemas.IssueTarget{}, fmt.Errorf("target field %q is not rewriteable", field)
	}
	if view.sectionType == "summary" {
		if itemID != "" {
			return schemas.IssueTarget{}, errors.New("summary target must not have itemId")
		}
		return schemas.IssueTarget{Field: field}, nil
	}
	if itemID == "" {
		return schemas.IssueTarget{}, errors.New("item target must have an itemId string")
	}
	if _, found := view.item(itemID); !found {
		return schemas.IssueTarget{}, fmt.Errorf("target itemId %q is not in current content", itemID)
	}
	return schemas.IssueTarget{ItemID: itemID, Field: field}, nil
}

// densityMetrics reports deterministic measurements for every rewriteable
// field of the current content.
func (view sectionView) densityMetrics() []schemas.DensityMetric {
	metrics := make([]schemas.DensityMetric, 0)
	for _, item := range view.items {
		for _, field := range rewriteableFields(view.sectionType) {
			metrics = append(metrics, measureDensity(item.id, field, item.fields[field]))
		}
	}
	return metrics
}

func measureDensity(itemID, field, text string) schemas.DensityMetric {
	limit := densityLimits[field]
	metric := schemas.DensityMetric{ItemID: itemID, Field: field}
	for _, line := range splitLines(text) {
		metric.Chars += countRunes(line)
		if strings.TrimSpace(line) != "" {
			metric.Lines++
		}
		if bullet, ok := bulletText(line); ok {
			metric.Bullets++
			if size := countRunes(bullet); size > metric.MaxBulletChars {
				metric.MaxBulletChars = size
			}
		}
	}
	if limit.maxChars > 0 && metric.Chars > limit.maxChars {
		metric.OverLimit = true
	}
	if limit.maxLines > 0 && metric.Lines > limit.maxLines {
		metric.OverLimit = true
	}
	if limit.maxBullets > 0 && metric.Bullets > limit.maxBullets {
		metric.OverLimit = true
	}
	if limit.maxBulletChars > 0 && metric.MaxBulletChars > limit.maxBulletChars {
		metric.OverLimit = true
	}
	return metric
}

func splitLines(text string) []string {
	return strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
}

func bulletText(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	for _, prefix := range []string{"- ", "* ", "• ", "· "} {
		if strings.HasPrefix(trimmed, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, prefix)), true
		}
	}
	return "", false
}

func countRunes(text string) int {
	return utf8.RuneCountInString(strings.TrimSpace(text))
}

type clientAnalysis struct {
	JDAlignment      schemas.JDAlignment       `json:"jdAlignment"`
	Suggestions      []schemas.Suggestion      `json:"suggestions"`
	EvidenceRequests []schemas.EvidenceRequest `json:"evidenceRequests"`
	Warnings         []schemas.Warning         `json:"warnings"`
	Density          []schemas.DensityMetric   `json:"density"`
}

func decodeEdit(raw json.RawMessage) (schemas.SectionEdit, error) {
	object, err := decodeJSONObject(raw, "edit")
	if err != nil {
		return schemas.SectionEdit{}, err
	}
	if err := requireExactKeys(object, "edit", "target", "newText", "issues"); err != nil {
		return schemas.SectionEdit{}, err
	}
	newText, ok := object["newText"].(string)
	if !ok {
		return schemas.SectionEdit{}, errors.New("edit newText must be a string")
	}
	target, ok := object["target"].(map[string]any)
	if !ok {
		return schemas.SectionEdit{}, errors.New("edit target must be an object")
	}
	issueTarget := schemas.IssueTarget{}
	if rawItemID, present := target["itemId"]; present && rawItemID != nil {
		itemID, ok := rawItemID.(string)
		if !ok {
			return schemas.SectionEdit{}, errors.New("edit target itemId must be a string")
		}
		issueTarget.ItemID = itemID
	}
	field, _ := target["field"].(string)
	issueTarget.Field = field

	issues := make([]schemas.EditIssue, 0)
	if rawIssues, ok := object["issues"].([]any); ok {
		for _, rawIssue := range rawIssues {
			issueObject, ok := rawIssue.(map[string]any)
			if !ok {
				continue
			}
			code, _ := issueObject["code"].(string)
			label, _ := issueObject["label"].(string)
			issue, _ := issueObject["issue"].(string)
			rationale, _ := issueObject["rationale"].(string)
			quote, _ := issueObject["quote"].(string)
			issues = append(issues, schemas.EditIssue{
				Code:      code,
				Label:     label,
				Issue:     issue,
				Rationale: rationale,
				Quote:     quote,
			})
		}
	}
	return schemas.SectionEdit{Target: issueTarget, NewText: newText, Issues: issues}, nil
}

func validateJDAlignment(value any) (schemas.JDAlignment, error) {
	alignment, ok := value.(map[string]any)
	if !ok {
		return schemas.JDAlignment{}, errors.New("analysis jdAlignment must be an object")
	}
	if err := requireAllowedAndRequiredKeys(alignment, "analysis jdAlignment", []string{"status"}, []string{"status", "matchedKeywords", "excludedKeywords", "missingKeywords"}); err != nil {
		return schemas.JDAlignment{}, err
	}
	status, ok := alignment["status"].(string)
	if !ok || (status != "applied" && status != "skipped") {
		return schemas.JDAlignment{}, errors.New("analysis jdAlignment status must be applied or skipped")
	}
	result := schemas.JDAlignment{Status: status}
	for _, field := range []string{"matchedKeywords", "excludedKeywords", "missingKeywords"} {
		value, present := alignment[field]
		if !present {
			continue
		}
		if !isStringArray(value) {
			return schemas.JDAlignment{}, fmt.Errorf("analysis jdAlignment %s must be an array of strings", field)
		}
		if containsUnsafePlaceholderValue(value) {
			return schemas.JDAlignment{}, fmt.Errorf("analysis jdAlignment %s contains an evidence placeholder", field)
		}
		items := value.([]any)
		switch field {
		case "matchedKeywords":
			result.MatchedKeywords = toStringSlice(items)
		case "excludedKeywords":
			result.ExcludedKeywords = toStringSlice(items)
		case "missingKeywords":
			result.MissingKeywords = toStringSlice(items)
		}
	}
	return result, nil
}

func toStringSlice(items []any) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		if text, ok := item.(string); ok {
			result = append(result, text)
		}
	}
	return result
}

func validateWarnings(value any) ([]schemas.Warning, error) {
	warnings, ok := value.([]any)
	if !ok {
		return nil, errors.New("analysis warnings must be an array")
	}
	result := make([]schemas.Warning, 0, len(warnings))
	for index, value := range warnings {
		warning, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("analysis warnings %d must be an object", index)
		}
		if err := requireExactKeys(warning, fmt.Sprintf("analysis warnings %d", index), "code", "resolution"); err != nil {
			return nil, err
		}
		code, ok := warning["code"].(string)
		if !ok || !isIssueCode(code) || code == "S1" {
			return nil, fmt.Errorf("analysis warnings %d has invalid code", index)
		}
		resolution, ok := warning["resolution"].(string)
		if !ok {
			return nil, fmt.Errorf("analysis warnings %d resolution must be a string", index)
		}
		if code == "S7" {
			resolution = normalizedS7WarningResolution
		} else if containsUnsafePlaceholderText(resolution) {
			return nil, fmt.Errorf("analysis warnings %d resolution contains an evidence placeholder", index)
		}
		result = append(result, schemas.Warning{Code: code, Resolution: resolution})
	}
	return result, nil
}

func targetKey(itemID, field string) string {
	return itemID + "|" + field
}

func splitTargetKey(key string) (string, string) {
	index := strings.Index(key, "|")
	if index < 0 {
		return "", key
	}
	return key[:index], key[index+1:]
}

func fieldOfKey(key string) string {
	_, field := splitTargetKey(key)
	return field
}

// onlyInsertions reports whether the model only appended content instead of
// rewriting the existing text. A rewrite always keeps at least one changed
// span, so an edit made of pure insertions means the model invented content.
func onlyInsertions(hunks []diffHunk) bool {
	if len(hunks) == 0 {
		return false
	}
	for _, hunk := range hunks {
		if !hunk.isInsertion() {
			return false
		}
		if strings.TrimSpace(hunk.Revised) != "" {
			return true
		}
	}
	return false
}

// severityForCode maps a check code to the severity shown in the UI.
func severityForCode(code string) string {
	switch code {
	case schemas.SuggestionCodeVerbWeak, schemas.SuggestionCodeDutyStack, schemas.SuggestionCodeImpactVague:
		return schemas.SuggestionSeverityHigh
	case schemas.SuggestionCodeDensity:
		return schemas.SuggestionSeverityLow
	default:
		return schemas.SuggestionSeverityMedium
	}
}

// sortSuggestionsBySeverity orders suggestions by severity while keeping the
// original order inside each severity group.
func sortSuggestionsBySeverity(suggestions []schemas.Suggestion) {
	rank := map[string]int{
		schemas.SuggestionSeverityHigh:   0,
		schemas.SuggestionSeverityMedium: 1,
		schemas.SuggestionSeverityLow:    2,
	}
	sort.SliceStable(suggestions, func(left, right int) bool {
		return rank[suggestions[left].Severity] < rank[suggestions[right].Severity]
	})
}

// uniqueIndexOf returns the start offset of needle inside haystack when it
// occurs exactly once.
func uniqueIndexOf(haystack, needle []rune) (int, bool) {
	if len(needle) == 0 || len(needle) > len(haystack) {
		return 0, false
	}
	found := -1
	count := 0
	for index := 0; index+len(needle) <= len(haystack); index++ {
		if runesEqual(haystack[index:index+len(needle)], needle) {
			found = index
			count++
			if count > 1 {
				return 0, false
			}
		}
	}
	if count != 1 {
		return 0, false
	}
	return found, true
}

func runesEqual(left, right []rune) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

// numbersGrounded reports whether every number in the replacement already
// exists in the original text. Thousands separators are ignored so that
// "1,200" and "1200" count as the same figure.
func numbersGrounded(replacement, original string) bool {
	originalNumbers := extractNumbers(original)
	for number := range extractNumbers(replacement) {
		if _, ok := originalNumbers[number]; !ok {
			return false
		}
	}
	return true
}

func extractNumbers(text string) map[string]struct{} {
	cleaned := strings.ReplaceAll(text, ",", "")
	numbers := map[string]struct{}{}
	for _, number := range numberPattern.FindAllString(cleaned, -1) {
		numbers[number] = struct{}{}
	}
	return numbers
}

func containsNumber(text string) bool {
	return numberPattern.MatchString(text)
}

func isSuggestionCode(code string) bool {
	switch code {
	case schemas.SuggestionCodeVerbWeak,
		schemas.SuggestionCodeDutyStack,
		schemas.SuggestionCodeImpactVague,
		schemas.SuggestionCodeKeywordMissing,
		schemas.SuggestionCodePlainSentence,
		schemas.SuggestionCodeVerbose,
		schemas.SuggestionCodeDensity:
		return true
	default:
		return false
	}
}

func allowedTargetField(sectionType, field string) bool {
	switch sectionType {
	case "summary":
		return field == "markdown"
	case "work", "internship":
		return field == "achievements" || field == "highlightsMarkdown"
	case "project":
		return field == "introduction" || field == "achievements" || field == "highlightsMarkdown"
	default:
		return false
	}
}

func isIssueCode(code string) bool {
	return len(code) == 2 && code[0] == 'S' && code[1] >= '1' && code[1] <= '8'
}

func containsUnsafePlaceholder(raw json.RawMessage) bool {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return true
	}
	return hasUnsafePlaceholder(value)
}

func hasUnsafePlaceholder(value any) bool {
	switch value := value.(type) {
	case string:
		return containsUnsafePlaceholderText(value)
	case []any:
		for _, item := range value {
			if hasUnsafePlaceholder(item) {
				return true
			}
		}
	case map[string]any:
		for _, item := range value {
			if hasUnsafePlaceholder(item) {
				return true
			}
		}
	}
	return false
}

func fallbackAnalysisJSON() json.RawMessage {
	return json.RawMessage(`{"jdAlignment":{"status":"skipped"},"suggestions":[],"evidenceRequests":[],"warnings":[{"code":"S8","resolution":"未能生成完整分析说明。"}]}`)
}

const normalizedS7WarningResolution = "已基于既有事实重组叙事，未推测空窗、离职或转行原因。"

func containsUnsafePlaceholderText(value string) bool {
	return strings.Contains(value, "XX") || strings.Contains(value, "待补充")
}

func containsUnsafePlaceholderValue(value any) bool {
	return hasUnsafePlaceholder(value)
}

func decodeJSONObject(raw []byte, name string) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("decode %s: %w", name, err)
	}
	if decoder.More() {
		return nil, fmt.Errorf("decode %s: trailing JSON value", name)
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an object", name)
	}
	return object, nil
}

func requireExactKeys(object map[string]any, name string, keys ...string) error {
	return requireAllowedAndRequiredKeys(object, name, keys, keys)
}

func requireAllowedAndRequiredKeys(object map[string]any, name string, required, allowed []string) error {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		allowedSet[key] = struct{}{}
	}
	for key := range object {
		if _, ok := allowedSet[key]; !ok {
			return fmt.Errorf("%s contains unsupported field %q", name, key)
		}
	}
	for _, key := range required {
		if _, ok := object[key]; !ok {
			return fmt.Errorf("%s is missing required field %q", name, key)
		}
	}
	return nil
}

func isStringArray(value any) bool {
	items, ok := value.([]any)
	if !ok {
		return false
	}
	for _, item := range items {
		if _, ok := item.(string); !ok {
			return false
		}
	}
	return true
}

func marshalJSONValue(value any) (json.RawMessage, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(raw), nil
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func max(left, right int) int {
	if left > right {
		return left
	}
	return right
}
