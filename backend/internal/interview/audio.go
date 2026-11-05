package interview

import "context"

// PendingAudio 是一条待生成语音的题目：重启重载与失败补齐时按它重建队列。
//
// db tag 与仓储查询的列名一一对应（sqlx 默认按字段名小写映射，读不到 tenant_id 这类列）。
type PendingAudio struct {
	TenantID    int64  `db:"tenant_id"`
	InterviewID int64  `db:"interview_id"`
	QuestionID  int64  `db:"question_id"`
	Text        string `db:"text"`
}

// AudioHooks 是面试层对语音能力的窄依赖，实现由 speech.Generator 提供（main.go 接线）。
//
// 约定：所有方法都必须安全失败 —— 语音不可用只影响语音，不影响面试主流程；
// 实现方不得把错误抛回给调用方的答题链路。
type AudioHooks interface {
	// Enqueue 把题目加入后台串行生成队列（创建面试后 / 追问落库后调用）。
	Enqueue(ctx context.Context, tenantID, interviewID, questionID int64, text string)
	// CleanupInterview 清理一场面试的音频目录（面试终态时调用）。
	CleanupInterview(ctx context.Context, tenantID, interviewID int64)
}
