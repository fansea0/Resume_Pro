# AI 写简历功能 — 设计文档

> 日期：2026-08-19
> 状态：设计待审
> 影响范围：`backend/internal/{ai,llm,httpapi}/`、`backend/internal/resume/service.go`（新增）、`backend/cmd/server/main.go`、`frontend/src/{api,components,types}/`

## 目标

为 Resume Pro 增加 AI 辅助写简历能力，覆盖两类用户场景：

1. **节选润色**：用户在编辑器中针对某节（如项目经历）点击「AI 润色」，AI 输出改写后的结构化建议，用户在侧边面板中比对、采纳或拒绝。
2. **从零生成**：用户粘贴一段「自我介绍/过往经历」文本（可选附上 JD），AI 一次性输出一份完整 v2 `ResumeDocument`，用户落库后进入编辑器继续打磨。

**核心设计原则（v1 确认）**：

- AI 是**纯文本→结构化 JSON 的转换器**，**不读写数据库**。所有写库动作仍走现有 `POST/PUT /api/resumes/:id` 端点。
- 租户安全由 `internal/resume` 仓储层（`WHERE tenant_id = ?`）统一保证，AI 层无需额外包一层闭包注入。
- AI 输出用 eino 的**结构化输出**（JSON schema 约束）保证合法，无需抽象成"tool"。
- v1 范围：纯文本输入（必填）+ JD（可选）。PDF/Word 解析推迟至 v2。

## 架构总览

```
┌─────────────────────────────────────────────────────────────────┐
│  Frontend (React + Vite)                                        │
│  ┌──────────────────┐   ┌────────────────────┐   ┌──────────┐  │
│  │ ResumeEditor     │──▶│ AiSuggestionPanel  │──▶│ 主文档   │  │
│  │  + "AI 润色" 按钮 │   │  - 流式 token      │   │ state    │  │
│  │  + "AI 生成" 入口 │   │  - diff 展示       │   │          │  │
│  └──────────────────┘   │  - 采纳/拒绝       │   └──────────┘  │
│         │   SSE / POST   └─────────┬──────────┘                  │
│         │                         │                              │
│         │                         │   POST /api/resumes/:id      │
│         │                         │   (采纳时)                   │
└─────────┼─────────────────────────┼──────────────────────────────┘
          │                         │
          ▼                         ▼
┌─────────────────────────────────────────────────────────────────┐
│  Backend (Go + Gin)                                              │
│  POST /api/ai/resumes/:id/section/rewrite   (SSE)                │
│  POST /api/ai/resumes/generate              (SSE)                │
│                                                                  │
│  ┌────────────────┐   ┌────────────────┐   ┌────────────────┐  │
│  │ httpapi        │   │ ai             │   │ llm (eino)     │  │
│  │ ai_handler.go  │──▶│ service.go     │──▶│ chatmodel.go   │  │
│  │ (SSE 写, 鉴权) │   │ (拼 prompt,    │   │ (OpenAI 兼容   │  │
│  │                │   │  调 eino, JSON │   │  client)       │  │
│  │                │   │  schema 约束)  │   │                │  │
│  └────────────────┘   └────────┬───────┘   └────────────────┘  │
│                                │                                 │
│              纯 LLM 包装，零 DB 访问                              │
│                                                                  │
│  ─────────────────────  独立业务边界  ─────────────────────       │
│                                                                  │
│  ┌────────────────┐   ┌────────────────┐                         │
│  │ httpapi        │──▶│ resume.Service │──▶ resume.Repository    │
│  │ resume_handler │   │ (新建, 抽出)   │    WHERE tenant_id = ?  │
│  └────────────────┘   └────────────────┘                         │
│                                                                  │
│  采纳建议 → 走 resume.Service.Create/Update → 仓储层兜底租户隔离 │
└─────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
                       ┌────────────────────────┐
                       │  OpenAI 兼容云 LLM     │
                       │  (DeepSeek/Qwen/etc)   │
                       └────────────────────────┘
```

**关键不变量**：

- `internal/ai` 包**不 import** `internal/resume` 或 `internal/auth`。它只跟 LLM 打交道。
- 现有 `resume.Repository` 的 `WHERE tenant_id = ?` 过滤是租户安全的唯一边界，AI 层无需重复保证。
- 采纳建议仍然走 `PUT /api/resumes/:id`，复用现有 schema 校验、avatar retain 逻辑。

## 后端设计

### 新增包

| 包路径 | 职责 |
| --- | --- |
| `internal/llm` | eino ChatModel 工厂：读 env → 构造 OpenAI 兼容 client → 暴露 `ChatModel` 接口 |
| `internal/ai` | AI 业务核心：prompt 模板、JSON schema 定义、service、限流器 + 观测 hooks。**不 import 任何 store/auth 包** |
| `internal/httpapi/ai_handler.go` | SSE handler，鉴权后调 `ai.Service` 拿流，转发为 SSE |
| `internal/resume/service.go` | **新增**：从 `resume_handler` 抽出 create/update 业务逻辑（avatar retain、validation、错误归一）。HTTP handler 调它 |

### 依赖变更

`backend/go.mod` 新增：

- `github.com/cloudwego/eino` — 核心框架
- `github.com/cloudwego/eino/schema` — message / tool schema
- `github.com/cloudwego/eino-ext/components/model/openai` — OpenAI 兼容 ChatModel

遵循 AGSIN.md「未涉及 AI 能力的普通业务逻辑不得为了使用 Eino 而增加依赖」——只在 `internal/ai` 和 `internal/llm` 中 import。

### 配置项（env 驱动）

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `OPENAI_API_KEY` | 无 | 必需 |
| `OPENAI_BASE_URL` | `https://api.deepseek.com/v1` | OpenAI 兼容 base URL |
| `OPENAI_MODEL` | `deepseek-chat` | 模型名 |
| `AI_MAX_TOKENS` | `2048` | 单次响应 token 上限 |
| `AI_REQUEST_TIMEOUT` | `30s` | 上游超时 |
| `AI_RATE_LIMIT_PER_MIN` | `10` | 每租户每分钟调用上限 |
| `AI_MAX_RETRIES` | `1` | LLM 输出 schema 校验失败时重试次数 |

未设置 `OPENAI_API_KEY` 时：开发环境 logger.Warn 后**禁用** AI 路由（返回 503 + `{"error":"AI features disabled"}`），其他路由不受影响；`APP_ENV=production` 时 fail-fast 退出。

### eino 结构化输出（核心）

不用 tool 抽象。直接用 eino 的**结构化输出**机制，约束 LLM 返回符合 JSON schema 的内容。

```go
// internal/ai/service.go (伪代码)

type RewriteRequest struct {
    SectionType    string          // "summary" / "work" / "project" / ...
    CurrentContent json.RawMessage // 当前 section.content
    Instructions   string          // 用户附加指引
}

type RewriteEvent struct {
    Kind         string          // "thinking" | "suggestion" | "error" | "done"
    ThinkingText string          // Kind=thinking
    Suggestion   *SectionPatch   // Kind=suggestion
    ErrorCode    string          // Kind=error
    ErrorMessage string          // Kind=error
}

type SectionPatch struct {
    SectionType string          // "summary" / "work" / ...
    NewContent  json.RawMessage // 新的 section.content（按 sectionType 走不同子结构）
    Rationale   string          // AI 自述改了什么
}

func (s *Service) RewriteSection(ctx context.Context, req RewriteRequest) (<-chan RewriteEvent, error) {
    // 1. 拼 prompt
    // 2. 选 JSON schema（按 sectionType）
    // 3. 调 eino ChatModel.Stream, 配 response_format=json_schema
    // 4. 流式产出 thinking token + 最终 suggestion
    // 5. 校验最终 suggestion 通过对应 sectionType 的 JSON schema
}
```

**JSON schema 来源**：每种 section type 对应一个独立的 `*SectionContent` Go struct（定义在 `internal/ai/schemas/`），用 eino 提供的 schema 反射（`eino/schema` + `reflect`）生成 OpenAI 兼容的 JSON schema 描述，喂给 LLM 的 `response_format` 参数。

七种 section 各一份 schema：`summary.json`、`work.json`、`project.json`、`education.json`、`skills.json`、`campus.json`、`honours.json`。结构与 `frontend/src/types/resume.ts` 严格对应。

`GenerateResume` 类似，schema 是完整 v2 `ResumeDocument`。

### 校验防御

LLM 输出到达时做**两道校验**：

1. **JSON schema 校验**：eino 在解析 `response_format` 时已做一轮（eino 内部用 `encoding/json.Unmarshal` 校验结构）。
2. **业务 schema 校验**：复用 `internal/resume` 包的 `validResumeContent` / `validResumeSection`，把 `newContent` 嵌入完整 `ResumeDocument` 试解析，确保与现有前端类型完全兼容。

任一校验失败 → 重试 `AI_MAX_RETRIES` 次，仍失败 → SSE `error` event + HTTP 422。

### Resume Service 抽取（顺手清理）

新增 `internal/resume/service.go`，把 `resume_handler.go` 里 `create` / `update` 的业务逻辑抽出来：

```go
// internal/resume/service.go
type Service struct {
    repository *Repository
    storage    files.FileStorage
}

type ServiceCreateInput struct {
    Principal auth.Principal
    Title     string
    TemplateID string
    Content   json.RawMessage
}

func (s *Service) Create(ctx context.Context, input ServiceCreateInput) (*Resume, error) {
    // 1. 校验 avatar binding
    // 2. retain avatar
    // 3. 调 repository.Create
    // 4. 出错时 release avatar
    // 5. 返回
}

// Update / Delete 同理
```

`resume_handler.go` 改造为薄壳：

```go
func (h *resumeHandler) create(ginCtx *gin.Context) {
    input, _ := decodeInput(ginCtx)
    principal, _ := principalFromContext(ginCtx)
    item, err := h.service.Create(ginCtx.Request.Context(), resume.ServiceCreateInput{
        Principal:   principal,
        Title:       input.Title,
        TemplateID: input.TemplateID,
        Content:     input.Content,
    })
    if !handleServiceError(ginCtx, err) { return }
    writeJSON(ginCtx, http.StatusCreated, item)
}
```

**好处**：
- HTTP handler 只做参数提取 + 响应格式化，业务逻辑集中可测
- 未来任何"创建简历"的调用方（CLI、批处理、admin tool）都能用同一个 service
- 测试可以从 service 层级覆盖业务逻辑，不必每次起 httptest

**不做的事**：service 不感知 AI、不感知 SSE、不感知 LLM。它是纯粹的"租户上下文 + 简历数据 → 持久化"封装。

### 限流器 + 观测 hooks

`internal/ai/ratelimit.go`：

```go
type Limiter struct {
    mu      sync.Mutex
    buckets map[int64]*bucket  // key = tenantID
    rate    int                // tokens per minute

    // Observability hooks (zero-cost when not wired)
    OnAllowed   func(tenantID int64, tokensIn, tokensOut int)
    OnThrottled func(tenantID int64)
    OnError     func(tenantID int64, code string)
}
```

v1 实现：
- 基础限流：每租户每分钟 `AI_RATE_LIMIT_PER_MIN` 次（默认 10），超限 HTTP 429
- 观测 hooks 默认接 logrus，记录 `tenant_id / tokens_in / tokens_out / call_count / error_count`
- **不落 DB**，但日志结构化（JSON 字段）方便后续接 Loki/ES 做统计
- 留口子：未来要做"按月 token 用量统计"、"用户级额度"，只需把 hooks 接到持久化层，limiter 主体不动

**多实例部署**：spec 中明确"v1 单实例假设，内存 limiter。多实例部署需切换为 `redigo` + Redis（AGSIN.md 已列出 redigo 合法用途），接口已抽象，切换零侵入"。

### Prompt 模板

`internal/ai/prompts/*.tmpl`，`embed.FS` 编译进二进制：

- `system_common.tmpl` — 通用角色/约束（"事实不增删、用动词量化、不要编造数字、必须输出符合 schema 的 JSON"）
- `system_generate.tmpl` — 从零生成专用（含 JD 拼接逻辑）
- `section_rewrite.tmpl` — 节选润色（按 sectionType 注入对应字段说明）
- 用户 `instructions` 字段用 XML 标签包裹：` <user_instructions>...</user_instructions>`，防御 prompt 注入

### 路由

在 `internal/httpapi/router.go` 中：

```go
aiHandler := newAiHandler(aiService, limiter)
protected := router.Group("/api/ai")
protected.Use(requireAuthentication(service))
protected.POST("/resumes/generate", aiHandler.generateResume)
protected.POST("/resumes/:id/section/rewrite", aiHandler.rewriteSection)
```

`/api/ai` 路径前缀明确语义，且与 `/api/resumes` 写操作路径解耦。`/api/ai/resumes/:id` 里的 `id` 用来定位"原简历"——rewrite 时确认它属于当前租户、把它作为上下文传给 LLM，但 AI service 自己不读它。

### SSE 协议

`Content-Type: text/event-stream`，每行以 `\n\n` 结尾。

事件序列（节选润色）：

```
event: thinking
data: {"text": "我注意到用户在 highlightsMarkdown 中用了过多被动语态..."}

event: suggestion
data: {"sectionType": "project", "newContent": {...}, "rationale": "..."}

event: done
data: {}
```

错误时：

```
event: error
data: {"code": "VALIDATION_FAILED", "message": "AI 输出的内容无法通过 schema 校验，请重试"}
```

`event: done` 始终作为最后一帧发送（无论成功失败），让前端能可靠关闭连接。

### 鉴权

复用 `requireAuthentication` 中间件 + `auth.PrincipalFromContext`。handler 仅做：

1. 提取 principal
2. 提取 URL/body 参数
3. 调 `ai.Service.RewriteSection` / `GenerateResume`
4. 把 channel 里的 event 翻译成 SSE 帧

## 前端设计

### 新增模块

| 路径 | 职责 |
| --- | --- |
| `src/api/ai.ts` | SSE client 封装（基于 `fetch` + `ReadableStream`） |
| `src/components/AiSuggestionPanel.tsx` | 通用侧边/底部面板：流式展示 + diff 高亮 + 采纳/拒绝 |
| `src/components/AiGenerateModal.tsx` | 从零生成入口（textarea + JD textarea + 模板下拉） |
| `src/types/ai.ts` | AI 相关 TS 类型：Suggestion、SSEEvent、SectionRewriteRequest 等 |

### 状态管理

- `AiSuggestionPanel` 接收 `document` + `onAccept(newSectionContent)` 回调
- `ResumeEditor` 维护 `suggestions: Record<sectionId, Suggestion>` 状态
- "采纳" → `onAccept` → `ResumeEditor` 更新 `document` → 现有 autosave 触发 `PUT /api/resumes/:id`（走 `resume.Service.Update`）
- "拒绝" → 清掉 suggestion，文档不变

### UX 细节

- 按钮放在 section header，icon + tooltip，文案"AI 润色"
- 按钮 disabled 条件：section 为空 / 正在流式 / 限流被拒
- 面板打开时主内容区加 `aria-busy="true"`，键盘焦点 trap
- 移动端：面板从底部抽屉上滑
- 错误展示走现有 toast 风格

### 从零生成 UX

`TemplateGallery` 顶部加按钮「AI 快速生成」→ 打开 `AiGenerateModal`：

- 必填：原始文本（≥ 30 字）
- 可选：JD（textarea）
- 必填：模板下拉
- 提交后：模态切换为流式面板，实时显示生成中的 `ResumeDocument` 字段
- 完成：按钮变为「应用到新建简历」→ `POST /api/resumes`（走 `resume.Service.Create`）→ 跳到 `/editor/:id`

## 数据流

### 节选润色

1. 用户在 `ResumeEditor` 选中一个 section，点「AI 润色」
2. `AiSuggestionPanel` 打开，状态 `loading`
3. 前端 POST `/api/ai/resumes/:id/section/rewrite`，body 含 `sectionType / currentContent / instructions?`
4. 后端 handler 鉴权后调 `ai.Service.RewriteSection`
5. `ai.Service` 拼 prompt、配 JSON schema、调 eino、产 `RewriteEvent` 流
6. handler 把流翻译成 SSE 帧
7. 前端逐 event 更新面板；最终展示 diff
8. 用户点「采纳」→ `onAccept(newContent)` → `ResumeEditor` 更新 `document` → autosave 触发 `PUT /api/resumes/:id` → `resume.Service.Update` → 仓储层兜底租户隔离
9. 用户点「拒绝」→ 关闭面板，文档不变

### 从零生成

1. `TemplateGallery` 点「AI 快速生成」→ 打开 `AiGenerateModal`
2. 用户填表提交
3. POST `/api/ai/resumes/generate`
4. 后端 handler 鉴权后调 `ai.Service.GenerateResume`
5. `ai.Service` 拼 prompt（含 JD）、配 `ResumeDocument` schema、调 eino、产 `RewriteEvent` 流
6. SSE 推回前端，模态实时展示生成中的字段
7. 完成 → 「应用到新建简历」→ `POST /api/resumes` → `resume.Service.Create` → 跳到编辑器

## 错误处理

| 场景 | 行为 |
| --- | --- |
| LLM 输出无法通过 JSON schema 校验 | eino 重试 `AI_MAX_RETRIES` 次（默认 1），仍失败 → SSE `error` 事件 + HTTP 422 |
| LLM 输出无法通过业务 schema 校验（兜底） | SSE `error` 事件 + HTTP 422，不返回脏数据 |
| LLM 30s 超时 | SSE `error` 事件 + HTTP 504 |
| 限流 | HTTP 429 + 提示「操作过于频繁，请稍后再试」 |
| `OPENAI_API_KEY` 未配置 | HTTP 503 + `{"error":"AI features disabled"}`（仅 dev mode；prod 启动 fail-fast） |
| 网络抖动 / 上游 5xx | SSE `error` 事件 + HTTP 502 |

前端错误展示：复用现有 error 风格，面板不消失，用户可点"重试"按钮重发请求。

## 测试

### 后端（必须保证 `go test ./...` 全绿）

| 层 | 文件 | 重点 |
| --- | --- | --- |
| 单元 | `internal/ai/prompts/*_test.go` | 模板渲染、占位符、XML 包裹正确性 |
| 单元 | `internal/ai/ratelimit_test.go` | 令牌桶行为、并发安全、hooks 触发 |
| 单元 | `internal/ai/schemas/*_test.go` | JSON schema 反射生成正确、覆盖每种 section type |
| 单元 | `internal/llm/client_test.go` | env 解析、缺 key 行为 |
| 集成 | `internal/httpapi/ai_handler_test.go` | SSE 顺序、错误事件、auth 中间件、限流 429 |
| 集成 | `internal/httpapi/router_test.go` | `/api/ai/...` 路由注册、`APP_ENV=production` 缺 key 启动 fail |
| 新增 | `internal/resume/service_test.go` | service 层 create/update 业务逻辑（avatar retain、错误归一） |
| 现有 | 全部保持通过 | 现有 182 个测试 file 全绿，**不修改 `validResumeContent` 等核心校验** |

**LLM mock**：`internal/llm/testing/` 写一个 `FakeChatModel`（实现 eino `model.ChatModel` 接口），按录制脚本返回 JSON 字符串。前述所有测试**不连真实 LLM**。

### 前端

| 文件 | 重点 |
| --- | --- |
| `src/api/ai.test.ts` | SSE 解析、error 事件、重连 |
| `src/components/AiSuggestionPanel.test.tsx` | 流式状态机、diff 展示、采纳/拒绝回调 |
| `src/components/AiGenerateModal.test.tsx` | 表单校验、流式渲染、提交 |
| `src/components/ResumeEditor.test.tsx` | 新增"AI 润色"按钮可访问、点击触发回调（不改现有断言） |

### 冒烟（不进 CI）

docs 中给出本地手动冒烟步骤：起 backend、设 `OPENAI_API_KEY`、用 `dev/login` 登录、走两次「润色」+ 一次「生成」，确认 SSE 顺序和落库内容。

## 实施步骤（高层，详细 plan 由 writing-plans 产出）

1. 加 eino 依赖，写 `internal/llm` 工厂 + `FakeChatModel` 测试替身
2. 抽 `internal/resume/service.go`，把 `resume_handler` 里的 create/update 业务逻辑搬过去；handler 改薄
3. 给 `resume.Service` 写单测（avatar retain、错误归一）
4. 写 `internal/ai/prompts/*.tmpl` + embed.FS
5. 写 `internal/ai/schemas/*.go`：7 种 section schema + ResumeDocument schema
6. 写 `internal/ai/ratelimit.go`（含 hooks）+ 单测
7. 写 `internal/ai/service.go`：拼 prompt、配 schema、调 eino、产 event 流
8. 写 `internal/httpapi/ai_handler.go` + SSE writer
9. router 注册 `/api/ai/...`
10. `cmd/server/main.go` 注入 LLM config + AI service；prod 缺 key fail-fast
11. 前端 `api/ai.ts`（SSE client）
12. 前端 `types/ai.ts`
13. 前端 `AiSuggestionPanel.tsx` + 单测
14. 前端 `AiGenerateModal.tsx` + 单测
15. `ResumeEditor` 集成润色按钮
16. `TemplateGallery` 集成生成入口
17. README 增加 env / 配置 / 限制章节
18. 跑全量测试 + 冒烟

## 范围边界

**v1 不做**：
- PDF/Word 解析（v2 单独 spec）
- 异步任务队列（v1 同步 SSE）
- AI 调用历史/审计表（v1 只 logrus 结构化日志 + hooks）
- 多语言 prompt（v1 中文 only）
- 自由对话/多轮追问
- 多个 AI 提供商切换 UI（env 切换即可）
- 分布式限流（v1 内存；多实例 → v2 切 Redis/redigo）
- AI service 写库（保持纯 LLM 包装，副作用全在 `resume.Service`）

**v1 必做**：
- v2 `ResumeDocument` schema 不动
- 现有 182 个测试 file 全绿
- 抽 `resume.Service` 作为唯一简历创建/更新入口
- 至少 1 个真实 LLM 冒烟通过（手动）
- 限流 + token/call 计数 hooks 留好
