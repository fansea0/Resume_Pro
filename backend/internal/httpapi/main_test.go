package httpapi

import (
	"os"
	"testing"

	"resumepro/backend/internal/logging"
)

// TestMain 静默日志。
//
// requestLogger 中间件会对每个 5xx 落日志，而本包的测试里有大量用例故意触发 500/503：
// 不静默的话测试输出会被日志刷爆，真正的失败断言反而被淹没。
// 需要断言日志行为的用例自己 SetLogger 到收集器，用例结束再恢复。
func TestMain(m *testing.M) {
	logging.SetLogger(logging.Discard())
	os.Exit(m.Run())
}
