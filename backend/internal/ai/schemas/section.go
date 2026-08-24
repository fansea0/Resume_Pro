package schemas

type SummarySectionContent struct {
	Markdown string `json:"markdown" jsonschema:"description=个人总结 markdown"`
}

type ExperienceItem struct {
	ID                 string `json:"id" jsonschema:"description=经历条目 ID"`
	Company            string `json:"company" jsonschema:"description=公司名称"`
	Role               string `json:"role" jsonschema:"description=职位名称"`
	StartDate          string `json:"startDate" jsonschema:"description=开始日期"`
	EndDate            string `json:"endDate" jsonschema:"description=结束日期"`
	Location           string `json:"location" jsonschema:"description=工作地点"`
	Achievements       string `json:"achievements" jsonschema:"description=业绩贡献"`
	HighlightsMarkdown string `json:"highlightsMarkdown" jsonschema:"description=经历亮点 markdown"`
	ShowLocation       bool   `json:"showLocation" jsonschema:"description=是否显示工作地点"`
	ShowAchievements   bool   `json:"showAchievements" jsonschema:"description=是否显示业绩贡献"`
}

type WorkSectionContent struct {
	Items []ExperienceItem `json:"items" jsonschema:"description=工作或实习经历条目"`
}

type ProjectItem struct {
	ID                 string `json:"id" jsonschema:"description=项目条目 ID"`
	Name               string `json:"name" jsonschema:"description=项目名称"`
	Role               string `json:"role" jsonschema:"description=项目角色"`
	StartDate          string `json:"startDate" jsonschema:"description=开始日期"`
	EndDate            string `json:"endDate" jsonschema:"description=结束日期"`
	Location           string `json:"location" jsonschema:"description=项目地点"`
	Link               string `json:"link" jsonschema:"description=项目链接"`
	Introduction       string `json:"introduction" jsonschema:"description=项目介绍"`
	Achievements       string `json:"achievements" jsonschema:"description=项目成绩"`
	HighlightsMarkdown string `json:"highlightsMarkdown" jsonschema:"description=项目亮点 markdown"`
	ShowLocation       bool   `json:"showLocation" jsonschema:"description=是否显示项目地点"`
	ShowLink           bool   `json:"showLink" jsonschema:"description=是否显示项目链接"`
	ShowIntroduction   bool   `json:"showIntroduction" jsonschema:"description=是否显示项目介绍"`
	ShowAchievements   bool   `json:"showAchievements" jsonschema:"description=是否显示项目成绩"`
}

type ProjectSectionContent struct {
	Items []ProjectItem `json:"items" jsonschema:"description=项目经历条目"`
}
