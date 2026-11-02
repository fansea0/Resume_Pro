# AI 模拟面试设计

> 日期：2026-11-02
> 状态：待评审
> 范围：基于「目标 JD + 简历」的 AI 模拟面试。生题 → 逐题提问 → 逐题评分（等级 C ~ S，6 级）→ 追问链 → 总评，全程落库可回看。
> 分支：feat/ai-interview
> 变更：v2 补齐总评分算法（中位数定档 + 比例门槛，等级 6 级去掉 C-，无 0-100 分数）；配置默认值用全局变量，创建面试时快照落库。

## 背景

用户想做一个 AI 模拟面试功能：机器人根据目标 JD 和简历预先生成面试题（覆盖 Go 基础、数据库、Redis、简历项目），逐题提问并对回答评分（等级制 C, B-, B, B+, A, S），回答模糊/偏移时可追问，面试结束给出总评等级和整体评价。所有作答可追踪，每个问题可回看标准答案。

## 已确认的决策

1. **题目/标准答案/作答数据存 SQLite**（面试表 + 问题表）。
2. **面试状态用 Eino graph 的 `WithGenLocalState` 管理**，checkpoint 持久化到 Redis（redigo 实现 `CheckPointStore`），checkpoint key = 面试 id。
3. **不引入外部知识库**，模型自带知识 + 简历 + JD 进上下文即可。
4. **动态题目 = 对当前题的深化追问**（父问题 id 关联，`follow_up_round` 递增），不改变题目总数与 index 体系。
5. **同步交互**：回答一次 → 返回评分 + 追问（或下一题 / 面试结束）。
6. **同一用户同时只能有一个进行中的面试**；发起新的时提示先结束/放弃当前的。
7. **追问总预算用尽后全局禁止追问**（预算默认 = 题目数一半，每用一次扣 1）。
8. **单问追问次数限制，默认 1**。
9. **追问等级 >= 父问题等级**：由代码 clamp 保证（取 max），不信任模型。
10. **A 及以上不追问**；用户回答「不会 / 没有补充」时跳过追问（模型输出 `userSkip` 标志 + 回答过短规则兜底）。
11. **评分等级 6 级**：`C B- B B+ A S`（去掉 C-）。
12. **总评分算法**：中位数定基础档 + 比例门槛定高档，只输出等级档位，无 0-100 分数（见「评分与总评算法」节）。
13. **面试配置**：题目总数、追问预算、单问上限、各标签数量都有全局默认值（代码常量）；用户可在面试开始前修改；创建面试时把生效配置快照进 `config_snapshot` 落库。配置只在快照存一份，运行时进度（当前 index、剩余预算、追问轮次）放 Eino state（Redis checkpoint），不冗余进 SQLite。
14. **每道题一个独立消息**（前端聊天式展示，追问折叠在父题下）。
15. 先不接 TTS。

## 技术选型验证（Eino checkpoint + Redis）

Eino v0.9.15（已在 go.mod）支持跨请求的图运行恢复：

- `WithGenLocalState(func(ctx) *State)`：图编译选项，每次运行生成类型化内存态。
- `WithStatePreHandler / WithStatePostHandler`：节点读写该状态，线程安全。
- `WithCheckPointStore(store)` + `WithSerializer(serializer)`：图编译选项，运行中断/结束时把 state 与执行位置持久化。
- `CheckPointStore` 接口仅两个方法：`Get(ctx, checkPointID) ([]byte, bool, error)`、`Set(ctx, checkPointID, checkPoint []byte) error`；可选 `CheckPointDeleter` 接口 `Delete`。
- `WithCheckPointID(id)`：指定 checkpoint 读写键；`WithWriteToCheckPointID` / `WithForceNewRun` 辅助。
- `Interrupt(ctx, info)` / `StatefulInterrupt(ctx, info, state)`：节点内发出中断信号，引擎保存 checkpoint 后返回。
- `ResumeWithData(ctx, checkPointID, data)`：带数据恢复，被中断节点通过 `GetResumeContext[T]` 取到恢复数据（本场景即用户回答）。
- `GetInterruptState[T](ctx)`：节点判断自己是否处于恢复流。

因此「提问 → 等用户回答 → 评分 → 追问/下一题」的跨请求状态机 = 一个图，通过 interrupt/resume 驱动，checkpoint 存 Redis（redigo），序列化用 JSON。问题数据放 SQLite，state 只保存最小进度，提问节点按 Index 读库取题：

```go
// InterviewState 是 Eino graph 的本地状态（WithGenLocalState），
// 每次运行由 gen 函数创建，经 checkpoint 序列化到 Redis（JSON）。
// 只存「进度指针」，不存任何对话内容——题目/答案/追问链都在 SQLite，
// 评分节点按需从库中重建上下文。State 丢失可从 SQLite 完全重建进度。
type InterviewState struct {
    // InterviewID 面试 id，等于 checkpoint id，用于 Redis key。
    InterviewID int64 `json:"interview_id"`
    // Index 当前主问题索引（0-based），指向 interviews.questions 中
    // parent_id IS NULL 的第 Index 条（按 id 升序 = 生成顺序）。
    Index int `json:"index"`
    // FollowUpRound 当前主问题已发生的追问轮次。
    // 0 = 正等主问题回答；N = 正等第 N 轮追问回答（该追问行已写入 SQLite）。
    FollowUpRound int `json:"follow_up_round"`
    // FollowUpBudgetLeft 剩余追问预算，每生成一次追问扣 1，0 则全局禁止。
    FollowUpBudgetLeft int `json:"follow_up_budget_left"`
}
```

> 设计说明：面试整体状态（active/completed/aborted、总评、配置快照）由 `interviews` 表持久化，state 只承载图运行进度；`Status` 不进 state，避免双真源。节点读写 state 用 `WithStatePreHandler / WithStatePostHandler`（线程安全），index/预算的推进在追问判定节点内完成。

**redigo**：`github.com/gomodule/redigo` 新增依赖，`REDIS_ADDR` 环境变量配置，`REDIS_PASSWORD`（可选）。本地无 Redis 时面试功能返回 503，不影响其它功能。

**Responses API（评分节点专用）**：新增 `github.com/cloudwego/eino-ext/components/model/agenticopenai@v0.2.4` 依赖，eino 升级到 v0.9.21（v0.9.15 → v0.9.21 接口兼容，已实测可编译）。评分节点用 `agenticopenai.NewResponsesModel` 走 Responses API，而非现有 ChatModel：

- 每个主问题（含追问链）= 一个独立会话，服务端维护 response 链，**不存历史消息**；
- 首次评分调用：只传当前问题上下文 + 用户回答，拿到响应后把 `ResponseMeta.OpenAIExtension.ID` 存入 SQLite 问题表主问题行（新增 `response_id` 列）；
- 后续追问评分：只传用户本次回答 + `WithHeadPreviousResponseID(已存 id)`，上下文由服务端自动续上，响应后更新 `response_id`；
- 已实测 MiniMax `https://api.minimax.cn/v1/responses` 端点返回 200（标准 Responses 结构，且响应含 `cached_tokens`，多轮续聊自动缓存省 token）。

## 状态机设计

### 生题图（创建面试时跑一次）

输入：JD（target_jobs 行）+ 简历纯文本（resumes.content JSON 提取）+ 配置（题目总数、各标签比例、追问预算）。
输出：题目数组，逐条含 `question / standard_answer / label / must_points(必答点) / bonus_points(加分点)`。
校验：JSON schema 校验 + 重试（复用现有 maxRetries 机制）；项目标签题无标准答案（`standard_answer` 为空）。
落库：面试表一行 + 问题表按 JSON 顺序 batch 插入（id 自增即顺序）。然后进入面试图。

### 面试图（每轮回答驱动，跨请求 interrupt/resume）

```
START
  → 提问节点：从 SQLite 按 Index 取题，把问题原文作为 assistant 消息输出，
    再 Interrupt(问题消息) 挂起等用户回答
  ← ResumeWithData(用户回答)
  → 评分节点（LLM）：(问题 + 标准答案 + 必答点/加分点 + 用户回答 + 追问历史)
      输出 { grade, followUp }
  → 追问判定（规则节点，非 LLM）：
      若 followUp ≠ "" 且 FollowUpRound < 单问上限 且 FollowUpBudgetLeft > 0
        且 grade < A
        → FollowUpBudgetLeft--，FollowUpRound++
        → 追问写入问题表（parent_id = 主问题 id，follow_up_round = FollowUpRound）
        → 回到提问节点：输出追问原文（assistant 消息），Interrupt 挂起等回答
      否则
        → 主问题等级 = 追问链最终等级（只升不降，代码 clamp）
        → Index+1，FollowUpRound 归零
        → 若 Index >= N：总评节点（LLM 生成 overall_review，服务端按算法定 overall_grade）→ 落库 → 结束
        → 否则回到提问节点：输出下一题原文（assistant 消息），Interrupt 挂起等回答
```

要点：

- **等级只升不降**：追问评分若低于父等级，`finalGrade = max(parentGrade, newGrade)`，并更新父问题行与该追问行。
- **「不会 / 没补充」**：由前端识别（跳过按钮），以 `skip=true` 传参；服务端不调 LLM，grade=C、index+1，见「用户跳过问题」节。
- **结束语义**：`status=completed` 落库，删除 Redis checkpoint；`abort` 则 `status=aborted`，保留已答数据。
- **每个问题独立 msg**：新问题的原文直接作为一条 assistant 消息发出（`Interrupt` 的 info 携带），前端按消息渲染，追问折叠在父题下；标准答案答完当前题后折叠可看。

### 如何等待用户回答并恢复面试

核心：一次面试 = 同一个 Eino graph 的多次运行，提问节点每次 `Interrupt` 都会保存 checkpoint，下一次回答用 `ResumeWithData` 恢复。详细时序：

1. **创建面试**：生题图跑完落库 → 首次运行面试图 → 提问节点输出第 1 题（assistant 消息）→ `Interrupt` → 引擎把执行位置 + `InterviewState` 序列化写入 Redis（key = 面试 id）→ 图运行返回，HTTP 层把第 1 题 + 等待状态返回给前端。
2. **等待**：前端把问题渲染为一条 assistant 消息，显示输入框等用户回答。期间服务端无任何常驻 goroutine，面试「挂起」在 Redis 的 checkpoint 里，随时可恢复（可跨分钟/小时/天）。
3. **恢复**：用户提交回答 → `POST /api/interviews/:id/answer` → 服务端调用 `graph.ResumeWithData(ctx, checkPointID=面试id, data=用户回答)` → 引擎从 Redis 读 checkpoint，从上次中断的提问节点之后继续：评分节点经 `GetResumeContext` 拿到用户回答 → 评分 → 追问判定 → 下一轮（新问题或追问再次 `Interrupt`，或结束）。
4. **并发防护**：同一面试同一时刻只允许一个活跃回答请求。`answer` 进入时校验 `status=active` 且无进行中请求（Redis 锁 or 状态位），重复提交返回 409。
5. **恢复失败**：checkpoint 不存在或已过期（Redis TTL 7 天）→ 返回明确错误「会话已过期，请重新开始面试」，已答数据仍在 SQLite 可回看。
6. **面试结束**：总评节点跑完 → 删除 Redis checkpoint（`CheckPointDeleter.Delete`）→ `status=completed`。

## 数据契约

```sql
CREATE TABLE interviews (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  tenant_id INTEGER NOT NULL REFERENCES tenants(id),
  target_job_id INTEGER NOT NULL REFERENCES target_jobs(id),
  resume_id INTEGER NOT NULL REFERENCES resumes(id),
  config_snapshot TEXT NOT NULL,          -- JSON: 生效配置快照（题目数/各标签数量/追问预算/单问上限）
  status TEXT NOT NULL,                   -- active / completed / aborted
  overall_grade TEXT,                     -- 总评等级（C/B-/B/B+/A/S）
  overall_review TEXT,                    -- 整体评价文本
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  completed_at DATETIME
);
CREATE TABLE interview_questions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,   -- id 决定顺序，按 JSON 顺序 batch 插入
  interview_id INTEGER NOT NULL REFERENCES interviews(id) ON DELETE CASCADE,
  parent_id INTEGER REFERENCES interview_questions(id),  -- NULL = 主问题；追问挂父
  follow_up_round INTEGER NOT NULL DEFAULT 0,            -- 追问轮次，0 = 主问题
  label INTEGER NOT NULL,                 -- 枚举: 0=八股 1=算法 2=项目（Go 代码里定义 QuestionLabel 枚举）
  question TEXT NOT NULL,
  standard_answer TEXT,                   -- 项目题可为空
  user_answer TEXT,
  extra TEXT,                             -- JSON: { must_points: [], bonus_points: [] }
  grade TEXT,                             -- 评分等级
  response_id TEXT,                       -- Responses API 会话 id（仅主问题行使用，追问行沿用父题）
  created_at DATETIME NOT NULL
);
```

> 说明：所有配置只存 `config_snapshot` 一份，不在 interviews 表冗余 `question_count` / `follow_up_budget` / `per_question_follow_up_limit` 列；这些值的「运行时版本」（当前 index、剩余预算、当前追问轮次）由 Eino `WithGenLocalState` 的 state 管理，经 checkpoint 持久化到 Redis，不进 SQLite。`label` 用 int + Go 枚举。不加联合索引（数据量小，无必要），查询按 `interview_id` 单列即可。

「父问题 id 和追问轮次一张表」的结论：**合适**。树形结构单表，回看时一次查全部按 `parent_id` 组树，代码层处理。

## API 契约（后端）

```
POST   /api/interviews                   创建面试（生题 + 落库），body 可带面试配置，返回面试 + 第一题
GET    /api/interviews                   面试列表（状态、总评摘要）
GET    /api/interviews/:id               详情（状态、配置快照、进度）
POST   /api/interviews/:id/answer        { answer, skip? } → { grade, followUp?, nextQuestion? | finished }
GET    /api/interviews/:id/questions     全部题目（追问树、答案、评分）——回看
POST   /api/interviews/:id/abort         终止面试（保留已答数据，不生成总评）
```

创建请求体（可选字段，未传用全局默认值）：

```json
{
  "targetJobId": 3,
  "resumeId": 7,
  "config": {
    "questionCount": 12,
    "labelCounts": { "0": 5, "1": 2, "2": 5 },
    "followUpBudget": 6,
    "perQuestionFollowUpLimit": 1
  }
}
```

约束：

- 全部走 `requireAuthentication`，按 tenant 隔离（复用现有模型）。
- 创建面试时若该 tenant 已有 `status=active` 面试 → 409，提示先结束当前面试。
- 创建接口幂等：前端禁点 + 后端校验（可选：幂等键）。
- 简历输入：`resumes.content` 是结构化 JSON，先提取纯文本（复用 optimization 的 section 提取逻辑）。

### 用户跳过问题 / 终止面试的操作

**跳过当前题（前端「跳过 / 不会 / 没补充」按钮）**：

- 调 `POST /api/interviews/:id/answer`，body 传 `{ "skip": true }`（不传 answer，前端需二次确认）。
- 服务端**不调 LLM**（省一次调用），直接把当前主问题 `grade` 记为 `C`（最低档）、`user_answer` 记为空，`index+1` 进入下一题；若已是最后一题则走总评。
- 不消耗追问预算；跳过 = 最差作答，总评算法分母 N 不变，无需特判。
- 回看时：`grade=C` 且 `user_answer` 为空 ⇒ 前端展示「已跳过」。
- 响应结构与「答完跳题」一致：`{ grade: "C", skip: true, nextQuestion | finished }`。

**终止面试（`POST /api/interviews/:id/abort`）**：

- 校验面试存在、属于该 tenant、`status=active`；已结束的返回 409。
- 置 `status=aborted`、`completed_at=now`；删除 Redis checkpoint（`CheckPointDeleter.Delete`）。
- 保留已答数据（问题表全部保留），可回看；**不生成总评**（未答完，总评无意义）。
- 之后该面试不能再 resume/answer（status 非 active → 409）。

### POST /api/interviews/:id/answer 完整实现

```
鉴权 → 校验面试存在、属于该 tenant、status=active（否则 404/409）
→ 并发防护：Redis SETNX 锁（key = interview:id, TTL 60s），拿不到锁 → 409「面试正在处理中」；完成后释放
→ 读 checkpoint（checkPointID = 面试 id）：
    不存在 / 已过期 → 410「会话已过期，请重新开始面试」（已答数据仍在 SQLite 可回看）
→ 分支：
  A. skip=true：
      不跑图。主问题 grade='C'、user_answer=''；Index+1
      → Index >= N：跑总评 → status=completed → 删 checkpoint → 返回 finished
      → 否则：读下一题原文 → 返回 nextQuestion（前端渲染为新 assistant 消息）
  B. answer 非空：
      graph.ResumeWithData(ctx, 面试id, answer) 恢复图运行
      → 提问节点从 GetResumeContext 取用户回答，转评分节点
      → 评分节点（LLM）输出 { grade, followUp }
      → 追问判定（规则，任一条件满足则「不追问」）：
          ① followUp == ""（AI 判定无需追问）
          ② grade >= A（答得够好）
          ③ FollowUpRound >= PerQuestionFollowUpLimit（单问追问次数到顶）
          ④ FollowUpBudgetLeft <= 0（全局预算用尽）
      → 不追问：
          clamp grade（追问链场景取 max(父等级, 本次)）；回写主问题行 grade + user_answer
          Index+1，FollowUpRound 归零
          → Index >= N：跑总评 → completed → 删 checkpoint → 返回 finished
          → 否则：读下一题原文 → 返回 nextQuestion
      → 追问（以上 4 条都不满足）：
          FollowUpBudgetLeft--；FollowUpRound++
          插入追问行（parent_id=主问题 id，question=followUp 原文，grade 暂空）
          回写主问题行 grade（clamp 后，父等级先行更新）
          返回 followUp 原文（前端渲染为折叠在父题下的新 assistant 消息，继续等回答）
→ 统一响应：
  { "grade": "B", "followUp": "追问原文", "finished": false }                // 追问
  { "grade": "B", "followUp": "", "nextQuestion": { "id":13, "text":"…", "label":0 }, "finished": false }  // 跳题
  { "finished": true, "overallGrade": "B+", "overallReview": "…" }           // 结束
```

要点：

- **「追问字段 AI 输出为空 → index+1 跳题目」**由判定条件 ① 保证：`followUp == ""` 直接走不追问分支跳下一题。
- **落库时机**：评分/追问判定节点内部直接依赖 repo 写 SQLite（题目本来就在库里，节点要读题、回写答案与等级）；图节点通过构造函数注入 repository，保持图与存储解耦。
- **并发锁 TTL 60s**：正常一次 answer（LLM 评分）秒级完成；超长锁自动过期避免死锁，过期后重复提交可能绕过防重，可接受（本地单用户场景）。
- **异常**：LLM 调用失败 → 图运行报错 → 返回 502，checkpoint 不写（中断前的 state 未推进），用户可原样重试；前端提示「评分失败，请重试」。

## 模型输出契约（评分节点）

```json
{
  "grade": "B+",
  "followUp": "你说的「缓存穿透」具体是怎么解决的？追问：有没有考虑布隆过滤器？"
}
```

- `grade` 枚举：`C B- B B+ A S`；不在枚举内 → 契约修复重试一次。
- `followUp` 空串 = 不需要追问。
- `grade` 由代码 clamp 保证 >= 父等级。
- **无 `userSkip`**：用户「不会 / 没补充」由前端识别（按钮）并以 `skip=true` 传参，不依赖模型判断，避免误判。

### 评分节点 LLM 提示词（简写）

```
你是资深 Go 后端面试官，正在用「目标 JD + 简历」做模拟面试。
请评估候选人对本题的回答。

【题目】{{.Question}}
【标签】{{.LabelName}}
【标准答案】{{.StandardAnswer}}
【必答点】{{.MustPoints}}
【加分点】{{.BonusPoints}}
【追问链】{{.FollowUpChain}}
【本次回答】{{.Answer}}

评分规则：
- 对照标准答案与必答点评估完整性、准确性、深度。
- 等级只输出一个：C / B- / B / B+ / A / S（从差到优）。
- 回答完整到位（约 A 及以上）时 followUp 必须为空串，不再追问。
- 回答模糊、偏移、漏必答点，或需要深挖时，生成一个 followUp 追问问题。

只输出 JSON（不要 markdown 代码块）：
{"grade":"B+","followUp":"追问问题原文或空串"}
```

模板字段说明：`Question` / `StandardAnswer` / `MustPoints` / `BonusPoints` 来自当前行；`FollowUpChain` 是追问链上下文（见下节）；`LabelName` 是枚举中文名（八股/算法/项目）。「不会 / 没补充」不进模型判定，由前端按钮以 `skip=true` 处理。输入为 `[]*schema.AgenticMessage`（Responses API），首次调用带完整题目上下文，后续追问只带用户新回答 + `WithHeadPreviousResponseID`。

### 每个问题独立会话（Responses API 续聊）

**每个主问题（含追问链）= 一个独立的 Responses API 会话，服务端维护上下文，不存历史消息**：

- 评分节点用 `agenticopenai.ResponsesModel`（Responses API），支持 `WithHeadPreviousResponseID(id)` 续聊——每次调用只需传「新增内容」，服务端自动带上之前全部上下文。
- 会话 id（`ResponseMeta.OpenAIExtension.ID`）存入 SQLite 问题表主问题行的 `response_id` 列，持久化跨请求；追问行沿用父题的会话，不单独存。
- 调用序列（以主问题 + 2 轮追问为例）：

```
第 1 次（答主问题）：input = [system 评分规则 + user 题目/标准答案/必答加分点 + user 回答]
    → 响应 resp_1，存 response_id = resp_1.id
第 2 次（答追问 1）：input = [user 追问 1 的回答]，
    opts = WithHeadPreviousResponseID(resp_1.id) → 响应 resp_2，更新 response_id = resp_2.id
第 3 次（答追问 2）：input = [user 追问 2 的回答]，
    opts = WithHeadPreviousResponseID(resp_2.id) → 响应 resp_3，更新 response_id = resp_3.id
```

- 模型上下文始终是「本主问题的题目 + 完整追问链」，不跨题；前端聊天流/回看仍用 `/questions` 从库重建，不依赖 response 链。
- 优势：服务端缓存（实测响应含 `cached_tokens`），多轮追问不重复计费前缀；`response_id` 丢了可从 SQLite 的追问链重建一个全新会话（退化为「重建 messages」路径），已答数据不受影响。
- 约束：如果切换模型/改写 history，响应缓存失效，需重新开会话（`InvalidateMessageCaches`），当前设计每主问题独立会话天然规避。

## 评分与总评算法

### 等级转序数（只用于排序，不直接相加）

| 等级 | 序数 |
| --- | --- |
| C | 0 |
| B- | 1 |
| B | 2 |
| B+ | 3 |
| A | 4 |
| S | 5 |

等距序数避免人为放大 S 的权重；不做加总平均。

### 中位数定基础档

把 N 个序数排序取中位数：

- N 为奇数：取正中间，`orders[N/2]`；
- N 为偶数：取中间两题中较低的那个，`orders[N/2-1]`（保守，避免总评虚高）。

中位数落在哪档，基础档就是哪档。总评由「大多数题的水平」决定，个别 S 拉不动。

### 比例门槛定高档

比例全部以 N 为分母，从高到低判断，命中即返回：

| 档 | 条件 |
| --- | --- |
| S | 基础档 ≥ A，且 S 题严格过半（`sCount*2 > N`） |
| A | 基础档 ≥ B+，且 A 及以上 ≥ 一半（`aOrAbove*2 >= N`） |
| B+ | 基础档 ≥ B，且 B+ 及以上 ≥ 一半（`bPlusOrAbove*2 >= N`） |

都不满足 → 返回基础档。

- 严格过半用 `> N`，至少一半用 `>= N`，全部整数比较，避免浮点，N 奇偶都正确。
- 高档判断先看基础档：中位数只有 B 时，S 再多也不会跳到 A（基础档未到 B+）。

### 执行流程

1. 遍历 N 个主问题的最终等级，转序数，同时统计 `sCount`、`aOrAbove`、`bPlusOrAbove`；
2. 排序取中位数 → 基础档；
3. 按 S → A → B+ 顺序套比例门槛，命中即返回；
4. 都不命中 → 返回基础档。

### 典型结果

| 分布 | 中位数 | 比例门槛 | 总评 |
| --- | --- | --- | --- |
| 5S7A | A | S 未过半 | A |
| 8S4A | S | S 过半 | S |
| 全 A | A | A 及以上过半 | A |
| 3S9B+ | B+ | A 及以上不足一半 | B+ |
| 3S9B | B | 基础档仅 B | B |
| 6S6A | A（偶数取下档） | S 未过半 | A |

### 整体评价

LLM 基于每题表现生成一段评价文本（优点、不足、建议），与总评等级一起落库 `overall_review` / `overall_grade`。无 0-100 分数。

### 配置默认值（全局变量 + 快照）

代码中定义全局默认配置常量（`DefaultInterviewConfig`），创建面试时快照进 `config_snapshot`：

```go
var DefaultInterviewConfig = InterviewConfig{
  QuestionCount:            12, // 题目总数默认
  LabelCounts:              map[QuestionLabel]int{LabelOctopus: 5, LabelAlgorithm: 2, LabelProject: 5}, // 各标签题目数，总和=QuestionCount
  FollowUpBudget:           -1, // 默认 N/2，-1 表示跟随题目数
  PerQuestionFollowUpLimit: 1,  // 单问追问上限默认 1
}

type QuestionLabel int // 数据库 label 列存该枚举值
const (
  LabelOctopus   QuestionLabel = iota // 0 八股（Go 基础/数据库/Redis）
  LabelAlgorithm                      // 1 算法
  LabelProject                        // 2 项目
)
```

用户可在面试开始前修改（题目总数、各标签数量、追问预算、单问上限），创建接口接收配置，未传字段用默认值，最终生效配置完整写入 `config_snapshot`（JSON）。面试进行中配置不可变。

## 前端交互（只讲做什么和长什么样）

1. 入口：目标 JD 卡片上「开始 AI 面试」。
2. 配置页：预填全局默认值（题目总数 12、各标签数量、追问预算 = 题目数一半、单问追问上限 1），可修改后开始面试。
3. 面试页：聊天式。服务端把每道新问题原文作为一条 assistant 消息返回，前端直接渲染为消息块，随后显示输入框等用户回答；答完展示评分等级徽章；追问折叠在父题下；标准答案折叠展示。底部输入框 + 提交（防抖防重复）。
4. 结束：总评卡片（总评等级 + 整体评价）+ 逐题明细（答案/标准答案/等级/追问链）。
5. 历史面试列表：状态、总评摘要、可回看、可放弃进行中的。
6. 同一用户仅一个进行中面试：发起新面试时若存在 active，提示先结束/放弃。

## 隐藏考虑点（已纳入设计）

1. **「不会 / 没补充」识别**：由前端按钮以 `skip=true` 显式传参，不依赖模型判断（避免 AI 误判用户意图）。
2. **并发/重复提交**：Redis 锁 or 状态校验防重；前端防抖。
3. **Redis 不可用**：面试功能 503 优雅降级，已答数据在 SQLite 不丢，可回看；checkpoint 丢失 → 明确错误「会话已过期」。
4. **checkpoint TTL**：Redis key 设 7 天；超期视为过期面试，无法续跑但可回看。
5. **生题失败**：schema 校验重试；部分题目非法 → 剔除或整批重试。
6. **token 成本**：生题一次大调用（题目多时分批，控制 MaxTokens）；每轮一次 LLM 调用；15 题约 15~25 次调用。
7. **简历输入**：JSON → 纯文本提取复用现有逻辑。
8. **标准答案展示时机**：答完当前题（含追问链）后折叠可看；历史回看全部。
9. **「每个问题独立 msg」结论**：需要，独立消息块便于回看与定位。

## 不做

- 不做 TTS（用户明确先不接）。
- 不做知识库。
- 不做流式（同步请求）。
- 不做算法题在线判题（算法题只给标准答案思路）。
- 不做自动判负/超时终止（用户可主动 abort）。
- 不做多面试并行（同一用户仅一个进行中面试）。

## 测试

- 后端 state 机：interrupt → resume 恢复位置正确；等级 clamp 只升不降；追问预算扣减与全局禁止；index 推进与结束判定。
- 后端追问判定：followUp 为空 / grade>=A / 单问到顶 / 预算用尽 → 均跳下一题；追问插入父题下。
- 后端跳过与终止：skip 不调 LLM、grade=C、index+1；abort 置状态、删 checkpoint、保留已答、不生成总评；非 active 面试 answer/abort 返回 409。
- 后端总评算法：中位数奇数/偶数取档、S 严格过半、A/B+ 至少一半、3S9B → B、5S7A → A、6S6A → A、8S4A → S。
- 后端 checkpoint：Redis store 的 Get/Set/Delete；TTL；checkpoint 丢失报「会话已过期」；并发锁 SETNX 防重。
- 后端契约：评分输出 schema 校验 + 修复重试；grade 枚举外值拒绝。
- 后端 Responses 会话：首次调用拿 response_id 存主问题行；追问带 previous id 续聊并更新 id；response_id 丢失时从追问链重建会话。
- 后端并发：同一面试重复 answer 被拒；active 面试存在时创建新面试 409。
- 后端存储：问题表 batch 插入顺序 = id 顺序；追问树组装；config_snapshot 快照与默认值合并。
- 前端：配置页（默认值预填、可修改）→ 面试页流程；防抖；跳过/终止按钮与二次确认；回看页；历史列表。
- 回归：现有 AI 功能（章节润色、诊断）不受影响。

## 分阶段计划（待批准后细化）

1. 后端基础设施：redigo Redis store（CheckPointStore 实现）、面试表/问题表迁移（含 response_id 列）、eino 升级 v0.9.21 + agenticopenai 依赖、配置加载（REDIS_ADDR）。
2. 生题图：简历文本提取 + 生题节点 + schema 校验 + 落库。
3. 面试图：提问节点 interrupt/resume + 评分节点 + 追问判定 + 总评节点 + checkpoint 生命周期。
4. API：interview handler + 路由 + 并发防护。
5. 前端：配置页、面试页、回看页、历史列表。
6. 联调与测试。
