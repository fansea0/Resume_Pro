# 开发协作规范

## 后端技术基线

- 后端使用 Go 与 Gin；HTTP 接口保持在 `backend/internal/httpapi`，并通过 `NewRouter` 构造，以便在不启动真实服务的情况下测试。
- 数据访问统一使用 `sqlx`，SQL、迁移和事务边界必须可测试；当前 SQLite 实现继续保留。
- 日志统一走 `backend/internal/logging` 抽象层（默认实现是 `logrus` 适配器）；业务代码一律不直接 `import logrus`。详见下方「日志规范」。
- 需要编排 LLM、工具调用或工作流时使用 Eino。未涉及 AI 能力的普通业务逻辑不得为了使用 Eino 而增加依赖。
- 需要 Redis 缓存、会话辅助、限流或分布式协调时使用 `redigo`。Redis 连接与依赖须通过配置注入；本地开发和自动化测试不得依赖外部 Redis 服务。
- 用 Eino 的 Responses 模型（`agenticopenai`）接非 OpenAI 上游时，`previous_response_id` 会被**静默忽略**（MiniMax 的 Responses 兼容端点即如此）。任何依赖「服务端续上下文」的写法都不成立：system prompt 与题目上下文必须由调用方每次自带。2026-11-05 的追问轮评分就因为只发一句用户答案、指望服务端续上下文，模型把答案当聊天请求返回了技术方案，最终报 `invalid grade ""`。

## 日志规范

日志存在的意义是**出问题时能定位**。前端只会拿到一句「interview operation failed」这种泛化文案，
没有日志就等于线上无法排查 —— 这类事故在 2026-11-05 的语音追问提交失败上真实发生过一次，
根因（`grade_answer` 节点拿到空 grade）完全靠后端日志才定位到。

### 唯一入口

- 业务代码只 import `backend/internal/logging`，用 `logging.Info / Warn / Error / Warnf ...`
  与 `logging.WithError(err).WithFields(logging.Fields{...})`。
- **禁止在业务代码里直接 import `logrus`**。只有两个例外：`internal/logging` 自身（适配器实现）
  与 `cmd/server/main.go`（组装点，负责把 `logrus.New()` 包成 `logging.Logger`）。
- 换日志组件（zap / slog / 云厂商 SDK）时只改 `internal/logging` 一个包，必要时调一次
  `logging.SetLogger(...)`；业务代码零改动。
- `internal/ai` 的 eino 回调同样走 `logging`（`ai.NewHooks(logger)`），不直接依赖 logrus。

### 必须打日志的场景

1. **任何返回 5xx 的路径** —— 统一走 `httpapi.writeError` + `requestLogger` 中间件落日志，
   不要在各个 handler 里各写各的（漏一处就又变成「只有一句泛化文案」）。
2. **任何降级或兜底** —— 功能被禁用、跳过、重试耗尽、失败后回落到默认值。
3. **任何外部调用失败** —— LLM、TTS、ASR、Redis、数据库、文件存储。
4. **状态机的异常迁移** —— 抢占生成权失败、重复提交、清理失败、状态归零失败、队列丢弃。
5. **启动与关闭** —— 监听地址、各能力是否装配成功（语音/AI/面试）、配置生效情况。

### 怎么写

- 级别：`Error` = 需要人介入；`Warn` = 降级但主流程正常；`Info` = 关键节点；`Debug` = 排查细节。
- **必须带原始错误**：拿得到 `err` 就用 `WithError(err)`，不要只写一句「失败了」。
- 关键上下文用结构化字段（`WithFields`），不要拼进消息文本 —— 拼进文本就没法按字段检索。
- 消息用「模块: 发生了什么」的固定前缀，便于按模块过滤（如 `voice: ...`、`interview: ...`）。
- 新增或修改日志行为要有对应测试（见 `internal/logging/logging_test.go`、
  `internal/httpapi/request_logging_test.go`）。

### 请求 ID 与前端报障

- 每个请求分配一个短 `request_id`（`X-Request-ID` 响应头）。
- 5xx 响应体带 `request_id` 字段：前端只显示泛化文案时，用户报障报这个 ID 即可定位日志。
- 中间件在 `internal/httpapi/request_logging.go`，记录方法、路径、状态、耗时、客户端 IP、
  内部错误详情与 `request_id`。**只记 5xx**：4xx 是客户端问题（参数错、未登录、越权），量大且不必逐条追。

### 禁止

- 禁止记录请求体、查询参数、简历内容、录音、密钥、token、Cookie、密码。
- 禁止用 `log.Print` / `fmt.Println` 打日志。
- 禁止吞掉 `err`：要么处理，要么记日志。
- 禁止在 handler 里另起一套 5xx 记录逻辑（统一走 `writeError` + 中间件）。

## Dev API 自动化测试入口

- Dev API 测试以路由级黑盒测试为入口：使用 `httptest` 调用 `httpapi.NewRouter`，并为仓储创建独立的内存 SQLite 数据库。
- 新增或修改 API 时，至少覆盖成功路径、请求校验失败、未认证访问，以及资源不存在或越权访问；认证接口还必须验证 Cookie 和敏感错误信息。
- 测试不得连接开发、测试或生产的共享数据库、Redis 或第三方服务。外部依赖必须通过接口或配置替换为测试替身。
- 保持 `GET /api/health` 作为无认证、无副作用的服务健康检查。唯一允许的开发认证入口是 `POST /api/dev/login`：它必须仅在 `APP_ENV=development` 注册、仅接受回环来源，并使用已有用户邮箱签发与正常登录完全相同的会话 Cookie。
- `POST /api/dev/login` 不得创建用户、tenant 或测试专用权限；不得在 test、staging 或 production 环境注册，也不得通过代理来源头放宽回环限制。
- 使用 `.agents/skills/dev-api-test` 对选中接口执行 curl 测试时，必须使用临时 Cookie jar、隔离的测试数据和清理步骤，并输出测试报告及改进建议。

## 服务端提交门禁

1. 修改 `backend/**` 后，在提交前运行 `cd backend && go test ./...`。
2. 若修改路由、处理器、认证、仓储、迁移或配置，必须新增或更新对应的 API 测试；只修改实现而不调整失效的测试不可提交。
3. GitHub Actions 工作流会在推送或拉取请求涉及 `backend/**` 时运行同一命令。测试失败时不得合并。
4. 服务端代码提交说明应写明已执行的测试命令；无法执行时必须说明原因，且不得作为可合并状态。
5. 每项功能完成并验证无误后必须提交代码；提交使用中文 tag（如 `feat:`、`fix:`、`docs:` 后接中文说明），确保提交记录可追溯。

## 仓库卫生

- 不提交本地 IDE 配置、操作系统元数据、依赖目录、构建输出、日志、环境变量文件或本地 SQLite 数据库及其 WAL/SHM 文件。
- 提交前使用 `git status --short` 检查新增文件；只有源代码、测试、文档、受控配置和明确需要的静态资源可以纳入版本控制。

## 前端编辑器交互规范

- 预计包含较长、连续文本的字段（如成果、说明、摘要）必须独占一行，并使用足够高度的多行输入控件；不要将其挤入双列或单行输入布局。
- 可选内容的显示/隐藏控制应放在对应字段标签旁，以眼睛图标等直观控件就地切换；不得把大量与字段脱节的开关集中堆放在表单底部。
- 图标控件必须有足够的点击面积、键盘焦点样式、无障碍名称，以及 `title` 悬浮提示来说明当前操作。
- 对排序、显示/隐藏、删除、添加等高频操作，优先使用语义明确的图标按钮；若一组操作使用相同图标会降低辨识度（例如添加不同类型的内容），保留文字标签或提供额外的可见区分。
- 表单的布局、标签、操作提示和隐藏状态必须在移动端保持可读、可点、可通过键盘操作。
