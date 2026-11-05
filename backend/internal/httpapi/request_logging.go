package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"resumepro/backend/internal/logging"
)

// requestIDHeader 是回传请求 ID 的响应头名。
//
// 前端只拿到一句 "interview operation failed" 时，线上根本无法定位是哪次请求出的问题。
// 带上一个短 ID，用户把截图里的 ID 报过来就能直接翻到后端日志。
const requestIDHeader = "X-Request-ID"

// requestIDContextKey 是请求 ID 在 gin.Context 里的键。
const requestIDContextKey = "request_id"

// serverErrorContextKey 是内部错误在 gin.Context 里的键：writeError 放进来，
// requestLogger 中间件读取后与请求元信息一起落成一条日志。
const serverErrorContextKey = "server_error"

// requestLogger 为每个请求分配 ID，并统一记录服务端错误（5xx）。
//
// 为什么放在中间件而不是每个 handler 自己记：
//   - 漏记是这类问题的根源 —— handler 各自 return 500 很容易忘了打日志；
//   - 中间件是唯一能看到「最终真的返回了 5xx」的地方，比 handler 自己判断可靠。
//
// 只记 5xx：4xx 是客户端问题（参数错、未登录、越权），量大且不需要逐条追。
// 请求体与查询参数一律不记 —— 里面有简历内容、录音、鉴权信息。
func requestLogger() gin.HandlerFunc {
	return func(context *gin.Context) {
		requestID := newRequestID()
		context.Set(requestIDContextKey, requestID)
		context.Header(requestIDHeader, requestID)

		start := time.Now()
		context.Next()

		status := context.Writer.Status()
		if status < http.StatusInternalServerError {
			return
		}
		fields := logging.Fields{
			"request_id":  requestID,
			"method":      context.Request.Method,
			"path":        context.Request.URL.Path,
			"status":      status,
			"duration_ms": time.Since(start).Milliseconds(),
			"client_ip":   context.ClientIP(),
		}
		// writeError 放进来的内部错误：没有它就只有状态码，看不出到底哪一步炸了。
		if value, ok := context.Get(serverErrorContextKey); ok {
			if err, ok := value.(error); ok && err != nil {
				fields["error"] = err.Error()
			}
		}
		// 503 是「该能力未启用/不可用」，属于预期状态而非故障：
		// 用 Warn 记录，避免配置未开时把错误日志刷爆。
		if status == http.StatusServiceUnavailable {
			logging.WithFields(fields).Warn("http request returned service unavailable")
			return
		}
		logging.WithFields(fields).Error("http request failed with server error")
	}
}

// requestIDFrom 取当前请求的 ID；没有中间件时返回空串。
func requestIDFrom(context *gin.Context) string {
	if context == nil {
		return ""
	}
	value, ok := context.Get(requestIDContextKey)
	if !ok {
		return ""
	}
	id, _ := value.(string)
	return id
}

// newRequestID 生成一个短的请求 ID（8 字节随机数的十六进制）。
//
// 用 crypto/rand 而不是时间戳：同一秒内的并发请求需要能区分开。
func newRequestID() string {
	buffer := make([]byte, 8)
	if _, err := rand.Read(buffer); err != nil {
		// 随机源不可用时退化成时间戳：ID 唯一性变差，但不影响功能。
		return hex.EncodeToString([]byte(time.Now().Format("150405.000000")))[:16]
	}
	return hex.EncodeToString(buffer)
}
