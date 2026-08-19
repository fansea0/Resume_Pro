package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

type sseWriter struct {
	w       io.Writer
	flusher http.Flusher
}

func newSSEWriter(ginCtx *gin.Context) *sseWriter {
	ginCtx.Header("Content-Type", "text/event-stream")
	ginCtx.Header("Cache-Control", "no-cache")
	ginCtx.Header("Connection", "keep-alive")
	ginCtx.Header("X-Accel-Buffering", "no")
	ginCtx.Status(http.StatusOK)

	flusher, _ := ginCtx.Writer.(http.Flusher)
	return &sseWriter{w: ginCtx.Writer, flusher: flusher}
}

func (s *sseWriter) WriteEvent(event string, data any) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", event, jsonData); err != nil {
		return err
	}
	if s.flusher != nil {
		s.flusher.Flush()
	}
	return nil
}
