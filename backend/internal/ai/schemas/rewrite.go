package schemas

import "encoding/json"

// SummaryRewriteOutput is the rewrite output for a summary section.
type SummaryRewriteOutput struct {
	Markdown string `json:"markdown" jsonschema:"description=改写后的个人总结 markdown"`
}

// ExperienceRewriteOutput is the narrow rewrite output shared by work and
// internship sections.
type ExperienceRewriteOutput struct {
	Items []ExperienceRewriteItem `json:"items" jsonschema:"description=工作或实习经历改写条目"`
}

type ExperienceRewriteItem struct {
	ID                 string  `json:"id" jsonschema:"description=对应原经历条目的 ID,required"`
	Achievements       *string `json:"achievements,omitempty" jsonschema:"description=改写后的业绩贡献"`
	HighlightsMarkdown *string `json:"highlightsMarkdown,omitempty" jsonschema:"description=改写后的经历亮点 markdown"`
}

// ProjectRewriteOutput is the narrow rewrite output for a project section.
type ProjectRewriteOutput struct {
	Items []ProjectRewriteItem `json:"items" jsonschema:"description=项目经历改写条目"`
}

type ProjectRewriteItem struct {
	ID                 string  `json:"id" jsonschema:"description=对应原项目条目的 ID,required"`
	Introduction       *string `json:"introduction,omitempty" jsonschema:"description=改写后的项目介绍"`
	Achievements       *string `json:"achievements,omitempty" jsonschema:"description=改写后的项目成绩"`
	HighlightsMarkdown *string `json:"highlightsMarkdown,omitempty" jsonschema:"description=改写后的项目亮点 markdown"`
}

// OptimizationOutput is the strict response envelope for section optimization.
// Rewrite is the only content that may be adopted into a resume; analysis is
// explanatory metadata for the suggestion UI.
type OptimizationOutput struct {
	Rewrite  json.RawMessage      `json:"rewrite"`
	Analysis OptimizationAnalysis `json:"analysis"`
}

type OptimizationAnalysis struct {
	JDAlignment      JDAlignment       `json:"jdAlignment"`
	MatchedIssues    []Issue           `json:"matchedIssues"`
	EvidenceRequests []EvidenceRequest `json:"evidenceRequests"`
	Warnings         []Warning         `json:"warnings"`
}

type JDAlignment struct {
	Status           string   `json:"status"`
	MatchedKeywords  []string `json:"matchedKeywords,omitempty"`
	ExcludedKeywords []string `json:"excludedKeywords,omitempty"`
}

type Issue struct {
	Code       string      `json:"code"`
	Label      string      `json:"label,omitempty"`
	Target     IssueTarget `json:"target"`
	Resolution string      `json:"resolution"`
	Status     string      `json:"status"`
}

type EvidenceRequest struct {
	Code                   string      `json:"code"`
	Target                 IssueTarget `json:"target"`
	Reason                 string      `json:"reason"`
	ExampleWithPlaceholder string      `json:"exampleWithPlaceholder"`
}

type Warning struct {
	Code       string `json:"code"`
	Resolution string `json:"resolution"`
}

type IssueTarget struct {
	ItemID string `json:"itemId,omitempty"`
	Field  string `json:"field"`
}
