// Package logging 是项目统一的日志抽象层。
//
// 为什么要抽象：业务代码直接依赖 logrus（logrus.Warn / logrus.WithError(...)）
// 会把日志组件焊死在每个包里 —— 将来换 zap / slog / 云厂商 SDK 就要全仓改。
// 这里定义最小接口，业务代码只依赖它；默认实现是 logrus 适配器，
// 换实现只要改这一个包 + 调一次 SetLogger。
//
// 用法（包级，与 logrus 包级 API 一一对应，迁移时几乎不用改写法）：
//
//	logging.Info("server started")
//	logging.WithError(err).WithFields(logging.Fields{"interview_id": id}).Error("answer failed")
//
// 结构化字段一律用 Fields，不要把变量拼进消息文本 —— 拼进文本就没法按字段检索了。
package logging

import (
	"sync"

	"github.com/sirupsen/logrus"
)

// Fields 是结构化日志字段。
type Fields map[string]any

// Logger 是项目内业务代码唯一允许依赖的日志接口。
//
// 方法集合刻意与 logrus 对齐：迁移时把 logrus.Xxx 换成 logging.Xxx 即可，
// 少一个方法就多一次全仓改造。Fatal* 会结束进程，只在启动/关闭路径用。
type Logger interface {
	Debug(args ...any)
	Info(args ...any)
	Warn(args ...any)
	Error(args ...any)
	Fatal(args ...any)

	Debugf(format string, args ...any)
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)
	Fatalf(format string, args ...any)

	// WithError 附加错误；WithFields 附加结构化字段。两者都返回新实例，不改原实例。
	WithError(err error) Logger
	WithFields(fields Fields) Logger
}

// logrusAdapter 把 logrus 适配到 Logger 接口。
//
// 内部持有 *logrus.Entry 而不是 *logrus.Logger：Entry 才有 WithError/WithFields
// 的链式派生能力（Logger 没有），而链式派生正是本接口 WithError/WithFields 的语义。
type logrusAdapter struct {
	entry *logrus.Entry
}

// NewLogrus 用一个 logrus 实例构造 Logger。
//
// logger 为 nil 时回落到 logrus 标准实例（写到 stderr）。
func NewLogrus(logger *logrus.Logger) Logger {
	if logger == nil {
		logger = logrus.StandardLogger()
	}
	return &logrusAdapter{entry: logrus.NewEntry(logger)}
}

func (a *logrusAdapter) Debug(args ...any) { a.entry.Debug(args...) }
func (a *logrusAdapter) Info(args ...any)  { a.entry.Info(args...) }
func (a *logrusAdapter) Warn(args ...any)  { a.entry.Warn(args...) }
func (a *logrusAdapter) Error(args ...any) { a.entry.Error(args...) }
func (a *logrusAdapter) Fatal(args ...any) { a.entry.Fatal(args...) }

func (a *logrusAdapter) Debugf(format string, args ...any) { a.entry.Debugf(format, args...) }
func (a *logrusAdapter) Infof(format string, args ...any)  { a.entry.Infof(format, args...) }
func (a *logrusAdapter) Warnf(format string, args ...any)  { a.entry.Warnf(format, args...) }
func (a *logrusAdapter) Errorf(format string, args ...any) { a.entry.Errorf(format, args...) }
func (a *logrusAdapter) Fatalf(format string, args ...any) { a.entry.Fatalf(format, args...) }

func (a *logrusAdapter) WithError(err error) Logger {
	return &logrusAdapter{entry: a.entry.WithError(err)}
}

func (a *logrusAdapter) WithFields(fields Fields) Logger {
	return &logrusAdapter{entry: a.entry.WithFields(logrus.Fields(fields))}
}

var (
	mu      sync.RWMutex
	current Logger = NewLogrus(logrus.StandardLogger())
)

// L 返回当前生效的全局 Logger。
func L() Logger {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// SetLogger 替换全局 Logger：换日志组件、或测试里换成收集器时调用。
//
// 传 nil 会被忽略（避免把进程打成静默的）。
func SetLogger(logger Logger) {
	if logger == nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	current = logger
}

// Reset 恢复默认实现（logrus 标准实例）。测试收尾时用。
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	current = NewLogrus(logrus.StandardLogger())
}

// 包级便捷函数：直接打日志时用 L() 的派生版本。

func Debug(args ...any) { L().Debug(args...) }
func Info(args ...any)  { L().Info(args...) }
func Warn(args ...any)  { L().Warn(args...) }
func Error(args ...any) { L().Error(args...) }
func Fatal(args ...any) { L().Fatal(args...) }

func Debugf(format string, args ...any) { L().Debugf(format, args...) }
func Infof(format string, args ...any)  { L().Infof(format, args...) }
func Warnf(format string, args ...any)  { L().Warnf(format, args...) }
func Errorf(format string, args ...any) { L().Errorf(format, args...) }
func Fatalf(format string, args ...any) { L().Fatalf(format, args...) }

// WithError 返回附加了错误的 Logger。
func WithError(err error) Logger { return L().WithError(err) }

// WithFields 返回附加了结构化字段的 Logger。
func WithFields(fields Fields) Logger { return L().WithFields(fields) }

// discardLogger 丢弃所有日志：测试里大量用例会故意触发 5xx，
// 不静默的话输出被日志刷爆，真正的失败断言反而看不见。
type discardLogger struct{}

func (discardLogger) Debug(args ...any)                 {}
func (discardLogger) Info(args ...any)                  {}
func (discardLogger) Warn(args ...any)                  {}
func (discardLogger) Error(args ...any)                 {}
func (discardLogger) Fatal(args ...any)                 {}
func (discardLogger) Debugf(format string, args ...any) {}
func (discardLogger) Infof(format string, args ...any)  {}
func (discardLogger) Warnf(format string, args ...any)  {}
func (discardLogger) Errorf(format string, args ...any) {}
func (discardLogger) Fatalf(format string, args ...any) {}
func (discardLogger) WithError(err error) Logger        { return discardLogger{} }
func (discardLogger) WithFields(fields Fields) Logger   { return discardLogger{} }

// Discard 返回一个丢弃全部日志的 Logger，供测试静默使用。
func Discard() Logger { return discardLogger{} }
