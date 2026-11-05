package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"resumepro/backend/internal/logging"
)

// errProbe 是测试用的内部错误。
var errProbe = errors.New("probe failure")

// captureLogger 收集日志条目，用来断言「5xx 必须落日志」。
type captureLogger struct {
	levels   []string
	messages []string
	fields   []logging.Fields
}

func (c *captureLogger) add(level, message string, fields logging.Fields) {
	c.levels = append(c.levels, level)
	c.messages = append(c.messages, message)
	c.fields = append(c.fields, fields)
}

func (c *captureLogger) Debug(args ...any) { c.add("debug", fmt.Sprint(args...), nil) }
func (c *captureLogger) Info(args ...any)  { c.add("info", fmt.Sprint(args...), nil) }
func (c *captureLogger) Warn(args ...any)  { c.add("warn", fmt.Sprint(args...), nil) }
func (c *captureLogger) Error(args ...any) { c.add("error", fmt.Sprint(args...), nil) }
func (c *captureLogger) Fatal(args ...any) { c.add("fatal", fmt.Sprint(args...), nil) }

func (c *captureLogger) Debugf(format string, args ...any) {
	c.add("debug", fmt.Sprintf(format, args...), nil)
}
func (c *captureLogger) Infof(format string, args ...any) {
	c.add("info", fmt.Sprintf(format, args...), nil)
}
func (c *captureLogger) Warnf(format string, args ...any) {
	c.add("warn", fmt.Sprintf(format, args...), nil)
}
func (c *captureLogger) Errorf(format string, args ...any) {
	c.add("error", fmt.Sprintf(format, args...), nil)
}
func (c *captureLogger) Fatalf(format string, args ...any) {
	c.add("fatal", fmt.Sprintf(format, args...), nil)
}

func (c *captureLogger) WithError(err error) logging.Logger {
	return &captureWithError{parent: c, err: err}
}

func (c *captureLogger) WithFields(fields logging.Fields) logging.Logger {
	return &captureWithFields{parent: c, fields: fields}
}

type captureWithError struct {
	parent *captureLogger
	err    error
}

func (d *captureWithError) Debug(args ...any) {
	d.parent.add("debug", fmt.Sprint(args...), logging.Fields{"error": d.err.Error()})
}
func (d *captureWithError) Info(args ...any) {
	d.parent.add("info", fmt.Sprint(args...), logging.Fields{"error": d.err.Error()})
}
func (d *captureWithError) Warn(args ...any) {
	d.parent.add("warn", fmt.Sprint(args...), logging.Fields{"error": d.err.Error()})
}
func (d *captureWithError) Error(args ...any) {
	d.parent.add("error", fmt.Sprint(args...), logging.Fields{"error": d.err.Error()})
}
func (d *captureWithError) Fatal(args ...any) {
	d.parent.add("fatal", fmt.Sprint(args...), logging.Fields{"error": d.err.Error()})
}

func (d *captureWithError) Debugf(format string, args ...any) {
	d.parent.add("debug", fmt.Sprintf(format, args...), logging.Fields{"error": d.err.Error()})
}
func (d *captureWithError) Infof(format string, args ...any) {
	d.parent.add("info", fmt.Sprintf(format, args...), logging.Fields{"error": d.err.Error()})
}
func (d *captureWithError) Warnf(format string, args ...any) {
	d.parent.add("warn", fmt.Sprintf(format, args...), logging.Fields{"error": d.err.Error()})
}
func (d *captureWithError) Errorf(format string, args ...any) {
	d.parent.add("error", fmt.Sprintf(format, args...), logging.Fields{"error": d.err.Error()})
}
func (d *captureWithError) Fatalf(format string, args ...any) {
	d.parent.add("fatal", fmt.Sprintf(format, args...), logging.Fields{"error": d.err.Error()})
}

func (d *captureWithError) WithError(err error) logging.Logger {
	return &captureWithError{parent: d.parent, err: err}
}
func (d *captureWithError) WithFields(fields logging.Fields) logging.Logger {
	merged := logging.Fields{"error": d.err.Error()}
	for key, value := range fields {
		merged[key] = value
	}
	return &captureWithFields{parent: d.parent, fields: merged}
}

type captureWithFields struct {
	parent *captureLogger
	fields logging.Fields
}

func (d *captureWithFields) Debug(args ...any) { d.parent.add("debug", fmt.Sprint(args...), d.fields) }
func (d *captureWithFields) Info(args ...any)  { d.parent.add("info", fmt.Sprint(args...), d.fields) }
func (d *captureWithFields) Warn(args ...any)  { d.parent.add("warn", fmt.Sprint(args...), d.fields) }
func (d *captureWithFields) Error(args ...any) { d.parent.add("error", fmt.Sprint(args...), d.fields) }
func (d *captureWithFields) Fatal(args ...any) { d.parent.add("fatal", fmt.Sprint(args...), d.fields) }

func (d *captureWithFields) Debugf(format string, args ...any) {
	d.parent.add("debug", fmt.Sprintf(format, args...), d.fields)
}
func (d *captureWithFields) Infof(format string, args ...any) {
	d.parent.add("info", fmt.Sprintf(format, args...), d.fields)
}
func (d *captureWithFields) Warnf(format string, args ...any) {
	d.parent.add("warn", fmt.Sprintf(format, args...), d.fields)
}
func (d *captureWithFields) Errorf(format string, args ...any) {
	d.parent.add("error", fmt.Sprintf(format, args...), d.fields)
}
func (d *captureWithFields) Fatalf(format string, args ...any) {
	d.parent.add("fatal", fmt.Sprintf(format, args...), d.fields)
}

func (d *captureWithFields) WithError(err error) logging.Logger {
	merged := logging.Fields{}
	for key, value := range d.fields {
		merged[key] = value
	}
	merged["error"] = err.Error()
	return &captureWithFields{parent: d.parent, fields: merged}
}

func (d *captureWithFields) WithFields(fields logging.Fields) logging.Logger {
	merged := logging.Fields{}
	for key, value := range d.fields {
		merged[key] = value
	}
	for key, value := range fields {
		merged[key] = value
	}
	return &captureWithFields{parent: d.parent, fields: merged}
}

// newLoggingTestRouter 造一个只挂了请求日志中间件的最小路由。
func newLoggingTestRouter(t *testing.T, handler gin.HandlerFunc) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(requestLogger())
	router.GET("/api/probe", handler)
	return router
}

// TestRequestLoggerRecordsServerErrors 5xx 必须落日志，且带请求 ID 与内部错误。
//
// 这条断言守着「前端只看到一句泛化文案、线上却查不到」的根因：
// 漏记中间件或漏带 request_id，测试就会红。
func TestRequestLoggerRecordsServerErrors(t *testing.T) {
	capture := &captureLogger{}
	original := logging.L()
	t.Cleanup(func() { logging.SetLogger(original) })
	logging.SetLogger(capture)

	router := newLoggingTestRouter(t, func(context *gin.Context) {
		writeError(context, http.StatusInternalServerError, "interview operation failed", errProbe)
	})

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/probe", nil)
	router.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", response.Code)
	}

	// 响应头与响应体都要带请求 ID：用户报错时报这个 ID 就能定位到日志。
	requestID := response.Header().Get(requestIDHeader)
	if requestID == "" {
		t.Fatal("response header missing request id")
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["request_id"] != requestID {
		t.Errorf("body request_id = %v, want %v", body["request_id"], requestID)
	}

	if len(capture.levels) == 0 {
		t.Fatal("5xx must be logged")
	}
	last := len(capture.levels) - 1
	if capture.levels[last] != "error" {
		t.Errorf("log level = %q, want error", capture.levels[last])
	}
	fields := capture.fields[last]
	if fields["request_id"] != requestID {
		t.Errorf("log request_id = %v, want %v", fields["request_id"], requestID)
	}
	if fields["path"] != "/api/probe" {
		t.Errorf("log path = %v, want /api/probe", fields["path"])
	}
	if fields["method"] != http.MethodGet {
		t.Errorf("log method = %v, want GET", fields["method"])
	}
	if fields["error"] == nil || fields["error"] == "" {
		t.Error("log must carry the internal error detail")
	}
}

// TestRequestLoggerSkipsClientErrors 4xx 是客户端问题，不进错误日志。
func TestRequestLoggerSkipsClientErrors(t *testing.T) {
	capture := &captureLogger{}
	original := logging.L()
	t.Cleanup(func() { logging.SetLogger(original) })
	logging.SetLogger(capture)

	router := newLoggingTestRouter(t, func(context *gin.Context) {
		writeError(context, http.StatusNotFound, "interview not found")
	})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/probe", nil))

	if len(capture.levels) != 0 {
		t.Errorf("4xx must not be logged, got %v", capture.levels)
	}
	if response.Header().Get(requestIDHeader) == "" {
		t.Error("every response should carry a request id")
	}
}

// TestRequestLoggerUsesWarnForServiceUnavailable 503 是「能力未启用」的预期状态，
// 不能按故障（Error）刷日志。
func TestRequestLoggerUsesWarnForServiceUnavailable(t *testing.T) {
	capture := &captureLogger{}
	original := logging.L()
	t.Cleanup(func() { logging.SetLogger(original) })
	logging.SetLogger(capture)

	router := newLoggingTestRouter(t, func(context *gin.Context) {
		writeError(context, http.StatusServiceUnavailable, "AI features disabled")
	})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/probe", nil))

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", response.Code)
	}
	if len(capture.levels) != 1 {
		t.Fatalf("entries = %d, want 1", len(capture.levels))
	}
	if capture.levels[0] != "warn" {
		t.Errorf("log level = %q, want warn", capture.levels[0])
	}
}

// TestRequestIDsAreUnique 并发请求的 ID 必须能区分开，否则日志对不上号。
func TestRequestIDsAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		id := newRequestID()
		if id == "" {
			t.Fatal("empty request id")
		}
		if seen[id] {
			t.Fatalf("duplicated request id: %s", id)
		}
		seen[id] = true
	}
}
