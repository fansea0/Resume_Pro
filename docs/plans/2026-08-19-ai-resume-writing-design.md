# AI 写简历功能 — 设计文档

> 日期：2026-08-19
> 状态：设计待审
> 影响范围：`backend/internal/{ai,llm,httpapi}/`、`backend/cmd/server/main.go`、`frontend/src/{api,components,types}/`

## 目标

为 Resume Pro 增加 AI 辅助写简历能力，覆盖两类用户场景：

1. **节选润色**：用户在编辑器中针对某节（如项目经历）点击「AI 润色」，AI 输出改写后的结构化建议，用户在侧边面板中比对、采纳或拒绝。
2. **从零生成**：用户粘贴一段「自我介绍/过往经历」文本（可选附上 JD），AI 一次性输出一份完整 v2 `ResumeDocument`，用户落库后进入编辑器继续打磨。

两路径在客户端均以**流式**呈现 AI 思考与建议，避免长耗时阻塞；服务端通过 eino 的**强类型 tool 调用**确保 AI 输出可直接合入现有 schema 校验，零侵入。

v1 范围：纯文本输入（必填）+ JD（可选）。PDF/Word 解析推迟至 v2。

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
└─────────┼─────────────────────────┼─────────────────────────────┘
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
│  │ (SSE 写, 鉴权, │   │ (拼 prompt,    │   │ (OpenAI 兼容   │  │
│  │  租户闭包注入) │   │  调 eino graph) │   │  client)       │  │
│  └────────────────┘   └────────┬───────┘   └────────────────┘  │
│                                │                                 │
│                  ┌─────────────┴─────────────┐                  │
│                  ▼                           ▼                  │
│         ┌─────────────────┐         ┌──────────────────┐        │
│         │ resume 包        │         │ eino tool 池     │        │
│         │ (写回, 严守      │         │ propose_section_ │        │
│         │  tenant 过滤)   │         │  rewrite         │        │
│         └─────────────────┘         │ propose_resume_  │        │
│                                    │  document        │        │
│                                    └──────────────────┘        │
└─────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
                       ┌────────────────────────┐
                       │  OpenAI 兼容云 LLM     │
                       │  (DeepSeek/Qwen/etc)   │
                       └────────────────────────┘
```

## 后端设计

### 新增包

| 包路径 | 职责 |
| --- | --- |
| `internal/llm` | eino ChatModel 工厂：读 env → 构造 OpenAI 兼容 client → 暴露 `ChatModel` 接口 |
| `internal/ai` | 业务核心：tool 定义、prompt 模板、service、限流器 |
| `internal/httpapi/ai_handler.go` | SSE handler，handler 负责从 gin ctx 提取 principal 并以闭包形式注入 tool |

### 依赖变更

`backend/go.mod` 新增：

- `github.com/cloudwego/eino` — 核心框架
- `github.com/cloudwego/eino/schema` — message / tool schema
- `github.com/cloudwego/eino-ext/components/model/openai` — OpenAI 兼容 ChatModel 实现

具体版本在实施阶段从 eino 官方 release 锁定。遵循 AGSIN.md「未涉及 AI 能力的普通业务逻辑不得为了使用 Eino 而增加依赖」——只在 AI 相关代码路径 import。

### 配置项（env 驱动，与现有风格一致）

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `OPENAI_API_KEY` | 无（启动 fail-fast 提示） | 必需 |
| `OPENAI_BASE_URL` | `https://api.deepseek.com/v1` | OpenAI 兼容 base URL |
| `OPENAI_MODEL` | `deepseek-chat` | 模型名 |
| `AI_MAX_TOKENS` | `2048` | 单次响应 token 上限 |
| `AI_REQUEST_TIMEOUT` | `30s` | 上游超时 |
| `AI_RATE_LIMIT_PER_MIN` | `10` | 每租户每分钟调用上限 |
| `AI_MAX_RETRIES` | `1` | tool 校验失败重试次数 |

未设置 `OPENAI_API_KEY` 时：开发环境 logger.Warn 后**禁用** AI 路由（返回 503 + `{"error":"AI features disabled"}`），其他路由不受影响；生产环境 fail-fast 退出。`APP_ENV=production` 时缺失 key 直接 Fatalf。

### 鉴权与租户安全（核心安全设计）

**铁律**：LLM 永远不接触 `tenantID` / `userID` / `resumeID`。这些信息在 handler 层从 gin ctx 取出后，通过闭包绑定到 tool 实例。

伪代码：

```go
// httpapi/ai_handler.go
func (h *aiHandler) rewriteSection(ginCtx *gin.Context) {
    principal, _ := auth.PrincipalFromContext(ginCtx) // 中间件已验签
    resumeID, _ := strconv.ParseInt(ginCtx.Param("id"), 10, 64)

    // 提前校验：resume 属于该租户
    if _, err := h.resumeRepo.Get(ginCtx.Request.Context(), principal.TenantID, resumeID); err != nil {
        // 复用现有 handleRepositoryError, 对外统一 404 防止枚举
        handleRepositoryError(ginCtx, err); return
    }

    // 把 principal + resumeID 闭包到 tool
    rewriteTool := ai.NewProposeSectionRewriteTool(ai.SectionToolDeps{
        Repository:   h.resumeRepo,
        Storage:      h.storage,
        TenantID:     principal.TenantID,
        ResumeID:     resumeID,
        PrincipalID:  principal.UserID,
    })

    sse := newSSEWriter(ginCtx)
    ai.RunRewriteGraph(ginCtx.Request.Context(), ai.RewriteGraphInput{
        SectionType:    req.SectionType,
        CurrentContent: req.CurrentContent,
        Instructions:   req.Instructions,
        Tool:           rewriteTool,
    }, sse)
}
```

LLM 看到的 `propose_section_rewrite` tool 入参 schema：

```jsonc
{
  "name": "propose_section_rewrite",
  "parameters": {
    "type": "object",
    "properties": {
      "sectionType":   { "type": "string", "enum": ["summary","work",...] },
      "newContent":    { "type": "object" }, // 由 sectionType 决定具体子结构
      "rationale":     { "type": "string" }   // AI 自述改了什么
    },
    "required": ["sectionType", "newContent"]
  }
}
```

**注意：`newContent` 的具体子结构**（items 数组、字段集）由 eino 在 tool 构造时基于 Go struct 自动生成（用 `eino/schema.NewTool` + `json.RawMessage` 或强类型 struct tag）。每种 section type 对应一个独立 tool 注册：

- `propose_summary_rewrite`
- `propose_work_rewrite`
- `propose_project_rewrite`
- `propose_education_rewrite`
- `propose_skills_rewrite`
- `propose_campus_rewrite`
- `propose_honours_rewrite`

**为什么不合并成一个 tool**：每种 section 的 items 字段集不同（work 有 `company/role/achievements`，project 有 `name/role/link/introduction/...`），分开注册可以让 eino 生成精确的 per-tool JSON schema，LLM 难以"串台"。

`propose_resume_document` tool 用于"从零生成"，入参是**完整 v2 `ResumeDocument` JSON schema**（通过 `schema.Reflect(ResumeDocument{})` 自动生成）。

### Tool 实现的安全约束

`internal/ai/tools/section_rewrite.go`：

```go
type SectionToolDeps struct {
    Repository  *resume.Repository
    Storage     files.FileStorage
    TenantID    int64  // 闭包注入，LLM 不可见
    ResumeID    int64  // 同上
    PrincipalID int64  // 同上
}

func (t *SectionRewriteTool) Invoke(ctx context.Context, args SectionRewriteArgs) (SectionRewriteResult, error) {
    // 二次校验：即使 LLM 传了 resumeID 也忽略
    item, err := t.Repository.Get(ctx, t.TenantID, t.ResumeID)
    if err != nil { return SectionRewriteResult{}, err }

    // 业务：把 newContent 暂存为"建议"，不直接落库
    // 仅返回给前端 SSE 流，由用户"采纳"时走 PUT /api/resumes/:id
    return SectionRewriteResult{
        SectionType:   args.SectionType,
        ProposedContent: args.NewContent,
        Rationale:     args.Rationale,
    }, nil
}
```

**关键不变量**：
- tool 永远不返回 `TenantID` / `ResumeID` / `UserID` 给 LLM（防止上下文泄露）
- 错误信息对外是 generic 的（"无权访问"），不打具体 ID
- 日志在 tool 内打 `t.TenantID` 供运维，但不出现在 SSE 流
- DB 写入路径（`Repository.Create` / `Update`）已经按 `WHERE tenant_id = ?` 过滤，是兜底防线

### Prompt 模板

`internal/ai/prompts/*.tmpl`，编译进二进制（`embed.FS`）。

每个 section type 一个改写模板，外加：
- `system_common.tmpl` — 通用角色/约束（"事实不增删、用动词量化、不要编造数字、必须调用 tool"）
- `generate_full.tmpl` — 从零生成专用（含 JD 拼接逻辑）
- 用户 `instructions` 字段用 XML 标签包裹：` <user_instructions>...</user_instructions>`，明确标记为数据非指令，防御 prompt 注入

模板示例（节选）：

```
你是一名资深中文简历顾问。用户正在编辑一份简历的「{section_title}」节，请基于以下原文给出改写建议。

约束：
1. 不得编造事实、数字、日期、雇主名称。
2. 优先用动词开头、量化成果。
3. 必须调用 propose_{section_type}_rewrite tool，不要直接输出文本。
4. 返回前请自检 newContent 是否符合 tool 的参数 schema。

原文：
{current_content_json}

用户附加指引：
<user_instructions>
{user_instructions}
</user_instructions>
```

### 限流器

`internal/ai/ratelimit.go`：内存 token bucket，每租户 1 个 bucket。

```go
type Limiter struct {
    mu       sync.Mutex
    buckets  map[int64]*bucket  // key = tenantID
    rate     int                // tokens per minute
}

func (l *Limiter) Allow(tenantID int64) bool { ... }
```

v1 单实例假设，spec 中明确标注"多实例部署需替换为 Redis 限流（`redigo`）"——已在 AGSIN.md 列出 redigo 的合法用途。**测试不得依赖外部 Redis**，保留内存实现以满足 CI。

### 路由

在 `internal/httpapi/router.go` 中：

```go
aiHandler := newAiHandler(aiService, resumeRepository, files, llmClient, limiter)
protected := router.Group("/api/ai")
protected.Use(requireAuthentication(service))
protected.POST("/resumes/generate", aiHandler.generateResume)
protected.POST("/resumes/:id/section/rewrite", aiHandler.rewriteSection)
```

`/api/ai` 路径前缀明确语义，且与 `/api/resumes` 的写操作路径解耦。

### SSE 协议

`Content-Type: text/event-stream`，每行以 `\n\n` 结尾。

事件序列（节选润色场景）：

```
event: thinking
data: {"text": "我注意到用户在 highlightsMarkdown 中用了过多被动语态..."}

event: thinking
data: {"text": "我准备把每条改写成「动词 + 量化」结构..."}

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

## 前端设计

### 新增模块

| 路径 | 职责 |
| --- | --- |
| `src/api/ai.ts` | SSE client 封装（基于 `fetch` + ReadableStream，不用 EventSource 以便支持 POST body） |
| `src/components/AiSuggestionPanel.tsx` | 通用侧边/底部面板：流式展示 + diff 高亮 + 采纳/拒绝 |
| `src/components/AiGenerateModal.tsx` | 从零生成入口（textarea + JD textarea + 模板下拉） |
| `src/types/ai.ts` | AI 相关 TS 类型：Suggestion、SSEEvent、SectionRewriteRequest 等 |

### 状态管理

- `AiSuggestionPanel` 接收 `document` + `onAccept(newSectionContent)` 回调
- `ResumeEditor` 维护 `suggestions: Record<sectionId, Suggestion>` 状态（建议面板打开时持有）
- "采纳" → 调用 `onAccept` 把 `newContent` 合入 `ResumeDocument` → 现有 autosave 触发 PUT
- "拒绝" → 清掉对应 `suggestion`，文档不动

### UX 细节（遵循 AGSIN.md 编辑器规范）

- 按钮放在 section header，与"添加/删除"图标同一行；icon + tooltip，文案"AI 润色"
- 按钮 disabled 条件：section 为空 / 正在流式 / 限流被拒
- 面板打开时主内容区加 `aria-busy="true"`，键盘焦点自动 trap 在面板内
- 移动端：面板从底部抽屉上滑（已有 CSS 体系）
- 错误展示走现有 toast/error UI 风格，不另起样式

### 从零生成 UX

`TemplateGallery` 顶部加按钮「AI 快速生成」→ 打开 `AiGenerateModal`：

- 必填：原始文本（≥ 30 字）
- 可选：JD（textarea）
- 必填：模板下拉
- 提交后：模态内容切换为流式面板，实时显示生成的 `ResumeDocument` 字段
- 完成：按钮变为「应用到新建简历」→ `POST /api/resumes` → 跳到 `/editor/:id`

## 数据流

### 节选润色

1. 用户在 `ResumeEditor` 选中一个 section，点「AI 润色」
2. `AiSuggestionPanel` 打开，状态 `loading`
3. 前端 POST `/api/ai/resumes/:id/section/rewrite`，body 含 `sectionType / currentContent / instructions?`
4. 后端 handler 提取 principal、校验 resume 归属、构造闭包 tool
5. eino graph 调 LLM → SSE 流回 thinking → tool call → suggestion → done
6. 前端逐 event 更新面板状态；最终展示 diff（旧 content vs newContent）
7. 用户点「采纳」→ `AiSuggestionPanel.onAccept(newContent)` → `ResumeEditor` 更新 `document` → autosave 走 `PUT /api/resumes/:id`
8. 用户点「拒绝」→ 关闭面板，文档不变

### 从零生成

1. `TemplateGallery` 点「AI 快速生成」→ 打开 `AiGenerateModal`
2. 用户填表提交
3. POST `/api/ai/resumes/generate`
4. 后端构造 `propose_resume_document` tool（TenantID 闭包）
5. eino graph 调 LLM → 流式返回 ResumeDocument
6. 前端实时渲染生成中的字段（用 `ResumePreview` 现有组件）
7. 完成 → 「应用到新建简历」按钮 → `POST /api/resumes` → 跳到编辑器

## 错误处理

| 场景 | 行为 |
| --- | --- |
| Tool 校验失败 | eino 自动重试 `AI_MAX_RETRIES` 次（默认 1），仍失败 → SSE `error` 事件 + HTTP 422 |
| LLM 30s 超时 | SSE `error` 事件 + HTTP 504 |
| 限流 | HTTP 429 + 提示「操作过于频繁，请稍后再试」 |
| `OPENAI_API_KEY` 未配置 | HTTP 503 + `{"error":"AI features disabled"}`（仅 dev mode；prod 启动 fail-fast） |
| 网络抖动/上游 5xx | SSE `error` 事件 + HTTP 502，提示「AI 服务暂不可用」 |
| LLM 返回的 newContent 无法通过 v2 schema 二次校验（兜底） | SSE `error` 事件 + HTTP 422，不写库 |

前端错误展示：复用现有 `writeError` 风格，面板不消失，用户可点"重试"按钮重发请求。

## 测试

### 后端（必须保证 `go test ./...` 全绿）

| 层 | 文件 | 重点 |
| --- | --- | --- |
| 单元 | `internal/ai/prompts/*_test.go` | 模板渲染、占位符、XML 包裹正确性 |
| 单元 | `internal/ai/ratelimit_test.go` | 令牌桶行为、并发安全 |
| 单元 | `internal/ai/tools/*_test.go` | tool 闭包 tenant 隔离：传错误 resumeID 也不能越权 |
| 单元 | `internal/llm/client_test.go` | env 解析、缺 key 行为 |
| 集成 | `internal/httpapi/ai_handler_test.go` | SSE 顺序、错误事件、auth 中间件、限流 429 |
| 集成 | `internal/httpapi/router_test.go` | `/api/ai/...` 路由注册、`APP_ENV=production` 缺 key 启动 fail |
| 现有 | 全部保持通过 | 现有 182 个测试 file 全绿，不修改 validResumeContent 等核心校验逻辑 |

**LLM mock**：eino ChatModel 实现一个 `llmtesting.FakeChatModel`（`internal/llm/testing/`），按录制脚本返回 tool call。前述所有测试**不连真实 LLM**。

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

1. 加 eino 依赖，写 `internal/llm` 工厂
2. 写 `internal/ai/prompts/*.tmpl` + embed.FS
3. 写 `internal/ai/ratelimit.go` + 单测
4. 写 `internal/ai/tools/*.go`（7 个 section tool + 1 个 generate tool）+ tenant 闭包
5. 写 `internal/ai/service.go`：构造 eino graph、调 tool、产出 SSE 事件
6. 写 `internal/httpapi/ai_handler.go` + SSE writer
7. router 注册 `/api/ai/...`
8. `cmd/server/main.go` 注入 LLM config + AI service；prod 缺 key fail-fast
9. 前端 `api/ai.ts`（SSE client）
10. 前端 `types/ai.ts`
11. 前端 `AiSuggestionPanel.tsx` + 单测
12. 前端 `AiGenerateModal.tsx` + 单测
13. `ResumeEditor` 集成润色按钮
14. `TemplateGallery` 集成生成入口
15. README 增加 env / 配置 / 限制章节
16. 跑全量测试 + 冒烟

## 范围边界

**v1 不做**：
- PDF/Word 解析（v2 单独 spec）
- 异步任务队列（v1 同步 SSE）
- AI 调用历史/审计表（v1 只 logrus 日志）
- 多语言 prompt（v1 中文 only）
- 自由对话/多轮追问
- 多个 AI 提供商切换 UI（env 切换即可）
- 分布式限流（v1 内存；多实例 → v2 切 Redis/redigo）

**v1 必做**：
- v2 `ResumeDocument` schema 不动
- 现有 182 个测试 file 全绿
- 鉴权/租户安全按本 spec 铁律实现
- 至少 1 个真实 LLM 冒烟通过（手动）
