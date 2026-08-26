package ai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type optimizationOutput struct {
	Rewrite  json.RawMessage
	Analysis json.RawMessage
}

// ValidateOptimizationOutput validates a model response without changing
// resume state. Unsafe rewrites fail the whole response. Invalid explanatory
// analysis is replaced with a controlled fallback when the rewrite is safe.
func ValidateOptimizationOutput(sectionType string, currentContent json.RawMessage, raw []byte) (rewrite json.RawMessage, analysis json.RawMessage, analysisOK bool, err error) {
	output, err := decodeOptimizationOutput(raw)
	if err != nil {
		return nil, nil, false, err
	}
	current, err := decodeJSONObject(currentContent, "current content")
	if err != nil {
		return nil, nil, false, err
	}
	if err := validateNarrowRewrite(sectionType, current, output.Rewrite); err != nil {
		return nil, nil, false, err
	}
	if containsUnsafePlaceholder(output.Rewrite) {
		return nil, nil, false, errors.New("rewrite contains an evidence placeholder")
	}

	validatedAnalysis, err := validateAnalysis(sectionType, current, output.Rewrite, output.Analysis)
	if err != nil {
		return output.Rewrite, fallbackAnalysisJSON(), false, nil
	}
	return output.Rewrite, validatedAnalysis, true, nil
}

func decodeOptimizationOutput(raw []byte) (optimizationOutput, error) {
	output, err := decodeJSONObject(raw, "optimization output")
	if err != nil {
		return optimizationOutput{}, err
	}
	if err := requireExactKeys(output, "optimization output", "rewrite", "analysis"); err != nil {
		return optimizationOutput{}, err
	}
	rewrite, err := marshalJSONValue(output["rewrite"])
	if err != nil {
		return optimizationOutput{}, fmt.Errorf("encode rewrite: %w", err)
	}
	analysis, err := marshalJSONValue(output["analysis"])
	if err != nil {
		return optimizationOutput{}, fmt.Errorf("encode analysis: %w", err)
	}
	return optimizationOutput{Rewrite: rewrite, Analysis: analysis}, nil
}

func validateNarrowRewrite(sectionType string, current map[string]any, raw json.RawMessage) error {
	rewrite, err := decodeJSONObject(raw, "rewrite")
	if err != nil {
		return err
	}
	switch sectionType {
	case "summary":
		if err := requireExactKeys(rewrite, "summary rewrite", "markdown"); err != nil {
			return err
		}
		if _, ok := rewrite["markdown"].(string); !ok {
			return errors.New("summary rewrite markdown must be a string")
		}
		return nil
	case "work", "internship":
		return validateItemRewrite(rewrite, current, "experience")
	case "project":
		return validateItemRewrite(rewrite, current, "project")
	default:
		return fmt.Errorf("unsupported section type for AI rewrite: %s", sectionType)
	}
}

func validateItemRewrite(rewrite, current map[string]any, itemKind string) error {
	if err := requireExactKeys(rewrite, itemKind+" rewrite", "items"); err != nil {
		return err
	}
	items, ok := rewrite["items"].([]any)
	if !ok {
		return fmt.Errorf("%s rewrite items must be an array", itemKind)
	}
	currentIDs, err := currentItemIDs(current)
	if err != nil {
		return err
	}
	seenIDs := make(map[string]struct{}, len(items))
	for index, value := range items {
		item, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("%s rewrite item %d must be an object", itemKind, index)
		}
		fields := []string{"id", "achievements", "highlightsMarkdown"}
		if itemKind == "project" {
			fields = []string{"id", "introduction", "achievements", "highlightsMarkdown"}
		}
		if err := requireExactKeys(item, fmt.Sprintf("%s rewrite item %d", itemKind, index), fields...); err != nil {
			return err
		}
		id, ok := item["id"].(string)
		if !ok {
			return fmt.Errorf("%s rewrite item %d id must be a string", itemKind, index)
		}
		if _, ok := currentIDs[id]; !ok {
			return fmt.Errorf("%s rewrite item %d references unknown id %q", itemKind, index, id)
		}
		if _, duplicate := seenIDs[id]; duplicate {
			return fmt.Errorf("%s rewrite contains duplicate id %q", itemKind, id)
		}
		seenIDs[id] = struct{}{}
		for _, field := range fields[1:] {
			if value := item[field]; value != nil {
				if _, ok := value.(string); !ok {
					return fmt.Errorf("%s rewrite item %d %s must be a string or null", itemKind, index, field)
				}
			}
		}
	}
	return nil
}

func validateAnalysis(sectionType string, current map[string]any, rewrite json.RawMessage, raw json.RawMessage) (json.RawMessage, error) {
	analysis, err := decodeJSONObject(raw, "analysis")
	if err != nil {
		return nil, err
	}
	if err := requireExactKeys(analysis, "analysis", "jdAlignment", "matchedIssues", "evidenceRequests", "warnings"); err != nil {
		return nil, err
	}
	if err := validateJDAlignment(analysis["jdAlignment"]); err != nil {
		return nil, err
	}
	rewriteObject, err := decodeJSONObject(rewrite, "rewrite")
	if err != nil {
		return nil, err
	}
	if err := validateIssues(sectionType, current, rewriteObject, analysis["matchedIssues"]); err != nil {
		return nil, err
	}
	if err := validateEvidenceRequests(sectionType, current, analysis["evidenceRequests"]); err != nil {
		return nil, err
	}
	if err := validateWarnings(analysis["warnings"]); err != nil {
		return nil, err
	}
	return marshalJSONValue(analysis)
}

func validateJDAlignment(value any) error {
	alignment, ok := value.(map[string]any)
	if !ok {
		return errors.New("analysis jdAlignment must be an object")
	}
	if err := requireAllowedAndRequiredKeys(alignment, "analysis jdAlignment", []string{"status"}, []string{"status", "matchedKeywords", "excludedKeywords"}); err != nil {
		return err
	}
	status, ok := alignment["status"].(string)
	if !ok || (status != "applied" && status != "skipped") {
		return errors.New("analysis jdAlignment status must be applied or skipped")
	}
	for _, field := range []string{"matchedKeywords", "excludedKeywords"} {
		if value, present := alignment[field]; present {
			if !isStringArray(value) {
				return fmt.Errorf("analysis jdAlignment %s must be an array of strings", field)
			}
			if containsUnsafePlaceholderValue(value) {
				return fmt.Errorf("analysis jdAlignment %s contains an evidence placeholder", field)
			}
		}
	}
	return nil
}

func validateIssues(sectionType string, current, rewrite map[string]any, value any) error {
	issues, ok := value.([]any)
	if !ok {
		return errors.New("analysis matchedIssues must be an array")
	}
	for index, value := range issues {
		issue, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("analysis matchedIssues %d must be an object", index)
		}
		if err := requireAllowedAndRequiredKeys(issue, fmt.Sprintf("analysis matchedIssues %d", index), []string{"code", "target", "resolution", "status"}, []string{"code", "label", "target", "resolution", "status"}); err != nil {
			return err
		}
		code, ok := issue["code"].(string)
		if !ok || !isIssueCode(code) || code == "S1" || code == "S7" {
			return fmt.Errorf("analysis matchedIssues %d has invalid code", index)
		}
		resolution, ok := issue["resolution"].(string)
		if !ok {
			return fmt.Errorf("analysis matchedIssues %d resolution must be a string", index)
		}
		if containsUnsafePlaceholderText(resolution) {
			return fmt.Errorf("analysis matchedIssues %d resolution contains an evidence placeholder", index)
		}
		if label, present := issue["label"]; present {
			label, ok := label.(string)
			if !ok {
				return fmt.Errorf("analysis matchedIssues %d label must be a string", index)
			}
			if containsUnsafePlaceholderText(label) {
				return fmt.Errorf("analysis matchedIssues %d label contains an evidence placeholder", index)
			}
		}
		field, itemID, err := validateTarget(sectionType, current, issue["target"])
		if err != nil {
			return fmt.Errorf("analysis matchedIssues %d: %w", index, err)
		}
		status, ok := issue["status"].(string)
		if !ok || status != "fixed" {
			return fmt.Errorf("analysis matchedIssues %d status must be fixed", index)
		}
		if !targetChanged(sectionType, current, rewrite, itemID, field) {
			return fmt.Errorf("analysis matchedIssues %d marks an unchanged target as fixed", index)
		}
	}
	return nil
}

func validateEvidenceRequests(sectionType string, current map[string]any, value any) error {
	requests, ok := value.([]any)
	if !ok {
		return errors.New("analysis evidenceRequests must be an array")
	}
	for index, value := range requests {
		request, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("analysis evidenceRequests %d must be an object", index)
		}
		if err := requireExactKeys(request, fmt.Sprintf("analysis evidenceRequests %d", index), "code", "target", "reason", "exampleWithPlaceholder"); err != nil {
			return err
		}
		code, ok := request["code"].(string)
		if !ok || code != "S1" {
			return fmt.Errorf("analysis evidenceRequests %d code must be S1", index)
		}
		if _, _, err := validateTarget(sectionType, current, request["target"]); err != nil {
			return fmt.Errorf("analysis evidenceRequests %d: %w", index, err)
		}
		reason, ok := request["reason"].(string)
		if !ok {
			return fmt.Errorf("analysis evidenceRequests %d reason must be a string", index)
		}
		if containsUnsafePlaceholderText(reason) {
			return fmt.Errorf("analysis evidenceRequests %d reason contains an evidence placeholder", index)
		}
		example, ok := request["exampleWithPlaceholder"].(string)
		if !ok || !strings.Contains(example, "XX") {
			return fmt.Errorf("analysis evidenceRequests %d exampleWithPlaceholder must contain XX", index)
		}
	}
	return nil
}

func validateWarnings(value any) error {
	warnings, ok := value.([]any)
	if !ok {
		return errors.New("analysis warnings must be an array")
	}
	for index, value := range warnings {
		warning, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("analysis warnings %d must be an object", index)
		}
		if err := requireExactKeys(warning, fmt.Sprintf("analysis warnings %d", index), "code", "resolution"); err != nil {
			return err
		}
		code, ok := warning["code"].(string)
		if !ok || !isIssueCode(code) || code == "S1" {
			return fmt.Errorf("analysis warnings %d has invalid code", index)
		}
		resolution, ok := warning["resolution"].(string)
		if !ok {
			return fmt.Errorf("analysis warnings %d resolution must be a string", index)
		}
		if code == "S7" {
			warning["resolution"] = normalizedS7WarningResolution
			continue
		}
		if containsUnsafePlaceholderText(resolution) {
			return fmt.Errorf("analysis warnings %d resolution contains an evidence placeholder", index)
		}
	}
	return nil
}

func validateTarget(sectionType string, current map[string]any, value any) (field, itemID string, err error) {
	target, ok := value.(map[string]any)
	if !ok {
		return "", "", errors.New("target must be an object")
	}
	if err := requireAllowedAndRequiredKeys(target, "target", []string{"field"}, []string{"itemId", "field"}); err != nil {
		return "", "", err
	}
	field, ok = target["field"].(string)
	if !ok || !allowedTargetField(sectionType, field) {
		return "", "", fmt.Errorf("target field %q is not rewriteable", field)
	}
	if sectionType == "summary" {
		if _, hasItemID := target["itemId"]; hasItemID {
			return "", "", errors.New("summary target must not have itemId")
		}
		return field, "", nil
	}
	itemID, ok = target["itemId"].(string)
	if !ok {
		return "", "", errors.New("item target must have an itemId string")
	}
	ids, err := currentItemIDs(current)
	if err != nil {
		return "", "", err
	}
	if _, ok := ids[itemID]; !ok {
		return "", "", fmt.Errorf("target itemId %q is not in current content", itemID)
	}
	return field, itemID, nil
}

func targetChanged(sectionType string, current, rewrite map[string]any, itemID, field string) bool {
	if sectionType == "summary" {
		return current[field] != rewrite[field]
	}
	currentItems, currentOK := current["items"].([]any)
	rewriteItems, rewriteOK := rewrite["items"].([]any)
	if !currentOK || !rewriteOK {
		return false
	}
	var currentValue any
	for _, value := range currentItems {
		item, ok := value.(map[string]any)
		if ok && item["id"] == itemID {
			currentValue = item[field]
			break
		}
	}
	for _, value := range rewriteItems {
		item, ok := value.(map[string]any)
		if ok && item["id"] == itemID {
			rewrittenValue, present := item[field]
			return present && rewrittenValue != nil && currentValue != rewrittenValue
		}
	}
	return false
}

func currentItemIDs(current map[string]any) (map[string]struct{}, error) {
	items, ok := current["items"].([]any)
	if !ok {
		return nil, errors.New("current content items must be an array")
	}
	ids := make(map[string]struct{}, len(items))
	for index, value := range items {
		item, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("current content item %d must be an object", index)
		}
		id, ok := item["id"].(string)
		if !ok {
			return nil, fmt.Errorf("current content item %d id must be a string", index)
		}
		ids[id] = struct{}{}
	}
	return ids, nil
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
	return json.RawMessage(`{"jdAlignment":{"status":"skipped"},"matchedIssues":[],"evidenceRequests":[],"warnings":[{"code":"S8","resolution":"未能生成完整分析说明。"}]}`)
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
