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

// Suggestion status values reported by the server for every suggestion.
const (
	// SuggestionStatusApplied marks a suggestion that passed every safety
	// check and is part of the derived rewrite.
	SuggestionStatusApplied = "applied"
	// SuggestionStatusRejected marks a suggestion that was dropped by a
	// server-side safety check and is therefore not part of the rewrite.
	SuggestionStatusRejected = "rejected"
)

// Suggestion severity levels derived by the server from the check code.
const (
	SuggestionSeverityHigh   = "high"
	SuggestionSeverityMedium = "medium"
	SuggestionSeverityLow    = "low"
)

// Suggestion codes the model may report. S1 (missing quantification) belongs
// to evidence requests and S7 (exposure of gaps) belongs to warnings, so
// neither may appear here.
const (
	SuggestionCodeVerbWeak       = "S2"
	SuggestionCodeDutyStack      = "S3"
	SuggestionCodeImpactVague    = "S4"
	SuggestionCodeKeywordMissing = "S5"
	SuggestionCodePlainSentence  = "S6"
	SuggestionCodeVerbose        = "S8"
	SuggestionCodeDensity        = "S10"
)

// SectionEdit is one model-proposed rewrite of a single field. The model
// returns the whole rewritten text; the server diffs it against the current
// content to obtain addressable suggestions.
type SectionEdit struct {
	Target  IssueTarget `json:"target"`
	NewText string      `json:"newText"`
	Issues  []EditIssue `json:"issues"`
}

// EditIssue explains one problem found in the edited field.
type EditIssue struct {
	Code      string `json:"code"`
	Label     string `json:"label"`
	Issue     string `json:"issue"`
	Rationale string `json:"rationale"`
	Quote     string `json:"quote"`
}

// Suggestion is an addressable single-span rewrite recommendation produced by
// the server. Start and End are rune offsets in the original field text and
// always address the exact slice kept in Anchor.
type Suggestion struct {
	ID              string      `json:"id"`
	Code            string      `json:"code"`
	Label           string      `json:"label,omitempty"`
	Severity        string      `json:"severity"`
	Target          IssueTarget `json:"target"`
	Issue           string      `json:"issue"`
	Anchor          string      `json:"anchor"`
	Replacement     string      `json:"replacement"`
	Rationale       string      `json:"rationale"`
	Start           int         `json:"start"`
	End             int         `json:"end"`
	Status          string      `json:"status"`
	RejectionReason string      `json:"rejectionReason,omitempty"`
}

type OptimizationAnalysis struct {
	JDAlignment      JDAlignment       `json:"jdAlignment"`
	Suggestions      []Suggestion      `json:"suggestions"`
	EvidenceRequests []EvidenceRequest `json:"evidenceRequests"`
	Warnings         []Warning         `json:"warnings"`
	Density          []DensityMetric   `json:"density,omitempty"`
}

type JDAlignment struct {
	Status           string   `json:"status"`
	MatchedKeywords  []string `json:"matchedKeywords,omitempty"`
	ExcludedKeywords []string `json:"excludedKeywords,omitempty"`
	MissingKeywords  []string `json:"missingKeywords,omitempty"`
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

// DensityMetric reports deterministic information-density measurements for one
// rewriteable field. It is computed by the server and never by the model.
type DensityMetric struct {
	ItemID         string `json:"itemId,omitempty"`
	Field          string `json:"field"`
	Chars          int    `json:"chars"`
	Lines          int    `json:"lines"`
	Bullets        int    `json:"bullets"`
	MaxBulletChars int    `json:"maxBulletChars"`
	OverLimit      bool   `json:"overLimit"`
}
