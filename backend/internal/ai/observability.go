package ai

import (
	"context"

	"github.com/sirupsen/logrus"
)

type CallInfo struct {
	TenantID    int64
	Operation   string
	SectionType string
}

type CallResult struct {
	DurationMS   int64
	TokensIn     int
	TokensOut    int
	RetryCount   int
	SuggestionOK bool
}

type Hooks struct {
	OnCallStarted  func(context.Context, CallInfo)
	OnCallFinished func(context.Context, CallInfo, CallResult)
	OnCallFailed   func(context.Context, CallInfo, error)
}

func NewLogrusHooks(logger *logrus.Logger) *Hooks {
	return &Hooks{
		OnCallStarted: func(_ context.Context, info CallInfo) {
			logger.WithFields(logrus.Fields{
				"event":        "ai_call_started",
				"tenant_id":    info.TenantID,
				"operation":    info.Operation,
				"section_type": info.SectionType,
			}).Info("AI call started")
		},
		OnCallFinished: func(_ context.Context, info CallInfo, result CallResult) {
			logger.WithFields(logrus.Fields{
				"event":         "ai_call_finished",
				"tenant_id":     info.TenantID,
				"operation":     info.Operation,
				"section_type":  info.SectionType,
				"duration_ms":   result.DurationMS,
				"tokens_in":     result.TokensIn,
				"tokens_out":    result.TokensOut,
				"retry_count":   result.RetryCount,
				"suggestion_ok": result.SuggestionOK,
			}).Info("AI call finished")
		},
		OnCallFailed: func(_ context.Context, info CallInfo, err error) {
			logger.WithFields(logrus.Fields{
				"event":        "ai_call_failed",
				"tenant_id":    info.TenantID,
				"operation":    info.Operation,
				"section_type": info.SectionType,
				"error":        err.Error(),
			}).Warn("AI call failed")
		},
	}
}
