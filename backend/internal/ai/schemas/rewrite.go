package schemas

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
