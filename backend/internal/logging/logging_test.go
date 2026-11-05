package logging

import (
	"errors"
	"fmt"
	"testing"
)

// captureLogger 是测试用的收集器：把日志条目记在内存里。
type captureLogger struct {
	entries []capturedEntry
}

type capturedEntry struct {
	level  string
	msg    string
	fields Fields
}

func (c *captureLogger) add(level, msg string, fields Fields) {
	c.entries = append(c.entries, capturedEntry{level: level, msg: msg, fields: fields})
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

func (c *captureLogger) WithError(err error) Logger {
	return &derivedLogger{parent: c, err: err}
}

func (c *captureLogger) WithFields(fields Fields) Logger {
	return &derivedLogger{parent: c, fields: fields}
}

// derivedLogger 记录「派生」出来的 logger：只关心最终落点的 level/msg/fields。
type derivedLogger struct {
	parent *captureLogger
	err    error
	fields Fields
}

func (d *derivedLogger) merged() Fields {
	merged := Fields{}
	for key, value := range d.fields {
		merged[key] = value
	}
	if d.err != nil {
		merged["error"] = d.err.Error()
	}
	return merged
}

func (d *derivedLogger) add(level, msg string) {
	d.parent.add(level, msg, d.merged())
}

func (d *derivedLogger) Debug(args ...any) { d.add("debug", fmt.Sprint(args...)) }
func (d *derivedLogger) Info(args ...any)  { d.add("info", fmt.Sprint(args...)) }
func (d *derivedLogger) Warn(args ...any)  { d.add("warn", fmt.Sprint(args...)) }
func (d *derivedLogger) Error(args ...any) { d.add("error", fmt.Sprint(args...)) }
func (d *derivedLogger) Fatal(args ...any) { d.add("fatal", fmt.Sprint(args...)) }

func (d *derivedLogger) Debugf(format string, args ...any) {
	d.add("debug", fmt.Sprintf(format, args...))
}
func (d *derivedLogger) Infof(format string, args ...any) {
	d.add("info", fmt.Sprintf(format, args...))
}
func (d *derivedLogger) Warnf(format string, args ...any) {
	d.add("warn", fmt.Sprintf(format, args...))
}
func (d *derivedLogger) Errorf(format string, args ...any) {
	d.add("error", fmt.Sprintf(format, args...))
}
func (d *derivedLogger) Fatalf(format string, args ...any) {
	d.add("fatal", fmt.Sprintf(format, args...))
}

func (d *derivedLogger) WithError(err error) Logger {
	return &derivedLogger{parent: d.parent, err: err, fields: d.fields}
}

func (d *derivedLogger) WithFields(fields Fields) Logger {
	merged := Fields{}
	for key, value := range d.fields {
		merged[key] = value
	}
	for key, value := range fields {
		merged[key] = value
	}
	return &derivedLogger{parent: d.parent, err: d.err, fields: merged}
}

func TestSetLoggerSwitchesGlobalImplementation(t *testing.T) {
	capture := &captureLogger{}
	original := L()
	t.Cleanup(func() { SetLogger(original) })

	SetLogger(capture)
	Info("hello")
	Warnf("count=%d", 3)

	if len(capture.entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(capture.entries))
	}
	if capture.entries[0].level != "info" || capture.entries[0].msg != "hello" {
		t.Errorf("first entry = %+v", capture.entries[0])
	}
	if capture.entries[1].msg != "count=3" {
		t.Errorf("second entry = %+v", capture.entries[1])
	}
}

func TestSetLoggerIgnoresNil(t *testing.T) {
	original := L()
	t.Cleanup(func() { SetLogger(original) })

	SetLogger(nil)
	if L() != original {
		t.Error("SetLogger(nil) must not replace the current logger")
	}
}

func TestWithFieldsAndWithErrorCarryContext(t *testing.T) {
	capture := &captureLogger{}
	original := L()
	t.Cleanup(func() { SetLogger(original) })
	SetLogger(capture)

	WithFields(Fields{"interview_id": 42}).WithError(errors.New("boom")).Error("answer failed")

	if len(capture.entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(capture.entries))
	}
	entry := capture.entries[0]
	if entry.level != "error" {
		t.Errorf("level = %q, want error", entry.level)
	}
	if entry.fields["interview_id"] != 42 {
		t.Errorf("interview_id = %v, want 42", entry.fields["interview_id"])
	}
	if entry.fields["error"] != "boom" {
		t.Errorf("error = %v, want boom", entry.fields["error"])
	}
}

// TestLogrusAdapterIsUsable 保证默认实现（logrus 适配器）不是空壳：
// 接口方法齐全才能保证换实现时行为一致。
func TestLogrusAdapterIsUsable(t *testing.T) {
	logger := NewLogrus(nil)
	if logger == nil {
		t.Fatal("NewLogrus(nil) must return a usable logger")
	}
	logger.Debug("debug")
	logger.Info("info")
	logger.Warn("warn")
	logger.Error("error")
	logger.WithFields(Fields{"k": "v"}).Info("with fields")
	logger.WithError(errors.New("e")).Warn("with error")
	logger.Debugf("d %d", 1)
	logger.Infof("i %d", 1)
	logger.Warnf("w %d", 1)
	logger.Errorf("e %d", 1)
}
