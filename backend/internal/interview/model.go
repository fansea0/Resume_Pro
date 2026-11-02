// Package interview 提供 AI 模拟面试的领域模型：等级档位、题目标签与面试配置。
package interview

// Grade 是单题评分等级，共 6 档：C < B- < B < B+ < A < S。
type Grade string

const (
	GradeC      Grade = "C"
	GradeBMinus Grade = "B-"
	GradeB      Grade = "B"
	GradeBPlus  Grade = "B+"
	GradeA      Grade = "A"
	GradeS      Grade = "S"
)

// QuestionLabel 标识题目的考察方向。
type QuestionLabel int

const (
	// LabelOctopus 八股/基础题。
	LabelOctopus QuestionLabel = iota
	// LabelAlgorithm 算法题。
	LabelAlgorithm
	// LabelProject 项目深挖题。
	LabelProject
)

// InterviewConfig 是一次面试的生效配置快照（创建面试时落库到 config_snapshot）。
type InterviewConfig struct {
	// QuestionCount 主问题总数。
	QuestionCount int
	// LabelCounts 各标签的题目数量。
	LabelCounts map[QuestionLabel]int
	// FollowUpBudget 全场追问预算；< 0 表示按 QuestionCount/2 自动折半（见 Resolved）。
	FollowUpBudget int
	// PerQuestionFollowUpLimit 单题追问轮次上限。
	PerQuestionFollowUpLimit int
}

// DefaultInterviewConfig 是全局默认配置；FollowUpBudget = -1 表示自动折半。
var DefaultInterviewConfig = InterviewConfig{
	QuestionCount: 12,
	LabelCounts: map[QuestionLabel]int{
		LabelOctopus:   5,
		LabelAlgorithm: 2,
		LabelProject:   5,
	},
	FollowUpBudget:           -1,
	PerQuestionFollowUpLimit: 1,
}

// Resolved 返回 FollowUpBudget 已落定的配置副本：FollowUpBudget < 0 时取 QuestionCount/2。
// 接收者是值拷贝，不会改动全局默认配置或调用方传入的原始配置。
func (c InterviewConfig) Resolved() InterviewConfig {
	if c.FollowUpBudget < 0 {
		c.FollowUpBudget = c.QuestionCount / 2
	}
	return c
}
