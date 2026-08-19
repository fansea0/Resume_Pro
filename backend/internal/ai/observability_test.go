package ai_test

import (
	"context"
	"errors"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	"resumepro/backend/internal/ai"
)

func TestLogrusHooks_FiresAllCallbacks(t *testing.T) {
	var buf []byte
	logger := logrus.New()
	logger.SetOutput(&byteSliceWriter{p: &buf})
	logger.SetFormatter(&logrus.JSONFormatter{})

	hooks := ai.NewLogrusHooks(logger)
	info := ai.CallInfo{TenantID: 1, Operation: "section_rewrite", SectionType: "project"}
	hooks.OnCallStarted(context.Background(), info)
	hooks.OnCallFinished(context.Background(), info, ai.CallResult{
		DurationMS:   1234,
		TokensIn:     100,
		TokensOut:    200,
		RetryCount:   0,
		SuggestionOK: true,
	})
	hooks.OnCallFailed(context.Background(), info, errors.New("oops"))

	s := string(buf)
	assert.Contains(t, s, `"event":"ai_call_started"`)
	assert.Contains(t, s, `"event":"ai_call_finished"`)
	assert.Contains(t, s, `"duration_ms":1234`)
	assert.Contains(t, s, `"tokens_in":100`)
	assert.Contains(t, s, `"event":"ai_call_failed"`)
	assert.Contains(t, s, `"error":"oops"`)
}

type byteSliceWriter struct{ p *[]byte }

func (w *byteSliceWriter) Write(p []byte) (int, error) {
	*w.p = append(*w.p, p...)
	return len(p), nil
}
