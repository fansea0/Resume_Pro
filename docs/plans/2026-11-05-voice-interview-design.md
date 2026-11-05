# AI 模拟面试语音能力（题目朗读 + 录音转写）— 设计文档

> 日期：2026-11-05
> 状态：设计待审
> 影响范围：新增 `backend/internal/speech/` 包、`backend/internal/httpapi/voice_handler.go`、`frontend/src/api/voice.ts`、`frontend/src/hooks/useVoiceRecorder.ts`、`frontend/src/utils/wav.ts`；改动 `backend/cmd/server/main.go`、`backend/internal/httpapi/router.go`、`backend/internal/httpapi/interview_handler.go`、`backend/internal/store/migrate.go`、`backend/internal/interview/`、`frontend/src/components/InterviewChat.tsx`。
> **数据库改动仅一列**：`interview_questions.audio_state`（幂等迁移，新增列可空）。转写文本不入库，走现有答题接口提交。

## 目标

AI 模拟面试目前是纯文字流程：面试官出题、用户打字回答。本功能给它加上声音：

- **题目朗读**：进入页面自动播放当前题语音，也可随时重播。
- **录音转写**：口述回答自动识别成文字，经 AI 修复错别字后填入输入框光标处，确认后提交。

**核心设计原则**：

- **抽象在协议层，不在厂商层**。语音能力对外只暴露 Go 接口，实现层统一走 OpenAI 音频协议（`/v1/audio/speech`、`/v1/audio/transcriptions`）。本地 MLX 推理服务与云端 API 是同一份代码，切换只改环境变量。
- **文字为主，语音是增强**。不改变现有打字答题流程，识别结果回填到输入框让用户过目，绝不自动提交。
- **音频只在后台生成**。播放接口只读已生成的文件，绝不在请求里同步合成。
- **后端零音频编解码依赖**。本机没有 ffmpeg，转码全部在浏览器完成（Web Audio API），Go 侧只搬运字节。
- **语音服务不可用时优雅降级**。任何失败都不能阻断面试主流程。

## 非目标

- 不做流式 TTS、不做播放打断。
- 不做语音克隆、音色设计、情感指令。
- 不做实时语音对话（Realtime API）。
- **不做录音回放与录音归档** —— 录音仅在转写时透传，不落盘、不回放。
- **不做转写文本的润色/改写**，只做错别字修复（最低思考强度）。
- 不引入麦克风权限的额外管理界面。
- 不改动现有 LLM 评分与追问逻辑。

## 现有约束

- **后端**：Go + Gin，路由统一在 `backend/internal/httpapi/router.go` 的 `NewRouter` 构造，`RouterConfig` 携带依赖，缺依赖时路由仍注册并返回 503。日志用 logrus。
- **面试状态**：进行态在 Redis（checkpoint），题面与答案在 SQLite。创建时一次性生成全部主问题并落库，追问在评分时动态产生。
- **现有 REST 面**：`/api/interviews` 组下有 create / list / get / answer / questions / abort，全部走 `requireAuthentication`，租户隔离在 service 层。
- **前端**：React 18 + Vite，`InterviewChat.tsx` 是进行中页面（逐题对话流 + 贴底输入坞），`api/interviews.ts` 是接口层，`api/client.ts` 的 `apiFetch` 固定 JSON Content-Type（multipart 需另走 fetch）。
- **测试**：后端 httptest + 内存 SQLite；前端 vitest + jsdom。
- **本机**：Apple M5 / 32GB RAM / macOS，**无 CUDA**，无 ffmpeg，已有 ollama（仅 embedding）与 uv。

## 技术选型

| 用途 | 模型 | 体积 | 说明 |
| --- | --- | --- | --- |
| ASR | `mlx-community/Qwen3-ASR-1.7B-8bit` | ~1.5 GB | 官方 mlx-audio 转换版 |
| TTS | `mlx-community/Qwen3-TTS-12Hz-0.6B-CustomVoice-8bit` | ~1.9 GB | 带 9 个预置音色 |
| 错别字修复 | 复用现有 `OPENAI_MODEL`（`llm.NewResponsesModel`） | — | 独立实例，`ReasoningEffort=low` |

两模型同时常驻约 3.5 GB，32GB 内存无压力。选 MLX 而非 GGUF：M5 的 Metal 融合路径成熟，Go 侧只需对接现成的 OpenAI 兼容 server。选 8bit 而非 4bit：mlx-audio PR #398 记录 CustomVoice 8bit 效果良好，4bit 音调异常；ASR 侧 4bit 在部分语种有幻觉。

推理服务用 `mlx-audio` 自带 server：

```bash
mlx_audio.server --host 127.0.0.1 --port 21100
```

端口 21100 避开常用段。

## 架构

```
InterviewChat.tsx
  ├─ api/voice.ts            GET questions/:qid/audio / POST questions/:qid/transcribe
  ├─ hooks/useVoiceRecorder  MediaRecorder → WAV(16k mono)
  └─ utils/wav.ts            浏览器端重采样 + WAV 封装
        │ HTTP
        ▼
httpapi/voice_handler.go
        ▼
speech/                     ← 抽象层
  ├─ speech.go   Synthesizer / Transcriber / TextModel 接口
  ├─ openai.go   OpenAICompatibleProvider（TTS + ASR 共用协议）
  ├─ config.go   env 配置（TTS / ASR 各自独立）
  ├─ store.go    题目音频落盘 + 目录清理
  ├─ generator.go 串行生成 worker + 重试 + 重启 reload
  ├─ correct.go  AI 错别字修复
  └─ hooks.go    实现 interview.AudioHooks
        │
        ├─ TTS 服务 (VOICE_TTS_BASE_URL)   POST /v1/audio/speech
        └─ ASR 服务 (VOICE_ASR_BASE_URL)   POST /v1/audio/transcriptions
```

### 抽象层接口

```go
package speech

type SynthesisRequest struct {
    Text  string
    Voice string
}
type SynthesisResult struct {
    Audio       []byte
    ContentType string
}

type TranscriptionRequest struct {
    Audio       []byte
    ContentType string
}
type TranscriptionResult struct {
    Text string
}

type Synthesizer interface {
    Synthesize(ctx context.Context, req SynthesisRequest) (SynthesisResult, error)
}
type Transcriber interface {
    Transcribe(ctx context.Context, req TranscriptionRequest) (TranscriptionResult, error)
}

// TextModel 是错别字修复所需的最小模型接口（与 interview.ResponsesModel 同形）。
type TextModel interface {
    Generate(ctx context.Context, in []*schema.AgenticMessage, opts ...model.Option) (*schema.AgenticMessage, error)
}
```

### interview 层依赖的窄接口

```go
// AudioHooks 是面试层对语音能力的窄依赖；实现由 speech 包提供，main.go 接线。
// 所有方法必须安全失败（只记日志，不把错误抛回答题链路）。
type AudioHooks interface {
    // Enqueue 把题目加入后台串行生成队列（创建面试 / 追问落库时调用）。
    Enqueue(ctx context.Context, interviewID, questionID int64, text string)
    // CleanupInterview 面试终态后删除音频目录并重置 audio_state。
    CleanupInterview(ctx context.Context, interviewID int64)
}
```

## 音频生成与重载

### 原则

- **生成只在后台发生**：播放接口只读已生成的文件，绝不在请求里同步合成。
- **串行 + 有序**：全局单 worker，按 `(interviewID, questionID)` 顺序处理，避免与评分/生题抢 GPU。
- **可重试**：单题失败按退避重试，耗尽后标记 `failed`。
- **可重载**：服务重启后从数据库重建队列，补齐未完成的部分。

### 触发点

| 触发 | 位置 | 入队内容 |
| --- | --- | --- |
| 创建面试成功 | `Service.CreateInterview`（`StartInterview` 之后） | 全部主问题，按题号顺序 |
| 追问落库 | `Service.Answer`（`NextQuestion != nil` 时） | 该追问题 |
| 服务重启 | `main.go` 启动 goroutine | active 面试中 `audio_state ∈ {NULL, failed}` 的题（`pending` 先由启动归零处理） |

### worker 与状态机

```go
type Job struct {
    InterviewID int64
    QuestionID  int64
    Text        string
}

// 单 worker 串行消费。
func (g *Generator) Run(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            return
        case job := <-g.jobs:
            g.process(ctx, job)
        }
    }
}

// 入队即抢占生成权（数据库原子操作，见「并发与竞争态」）。
func (g *Generator) Enqueue(ctx context.Context, job Job) {
    claimed, err := g.repo.ClaimAudioState(ctx, job.QuestionID)
    if err != nil || !claimed {
        return // 已被别的路径抢到，或已就绪
    }
    g.jobs <- job
}

func (g *Generator) process(ctx context.Context, job Job) {
    for attempt := 0; attempt <= maxAttempts; attempt++ {
        res, err := g.synth.Synthesize(ctx, SynthesisRequest{Text: job.Text})
        if err == nil {
            if err := g.saveAudio(ctx, job, res.Audio); err == nil {
                return
            } else if errors.Is(err, errInterviewClosed) {
                return // 面试已终止：丢弃结果
            }
        }
        if attempt < maxAttempts {
            select {
            case <-time.After(backoff(attempt)): // 1s / 3s / 10s
            case <-ctx.Done():
                return
            }
        }
    }
    _ = g.repo.UpdateAudioState(ctx, job.QuestionID, "failed")
    logrus.WithFields(logrus.Fields{"interview_id": job.InterviewID, "question_id": job.QuestionID}).Warn("question audio generation failed")
}
```

`audio_state` 状态机：

| 值 | 含义 | 前端表现 |
| --- | --- | --- |
| `NULL` | 未生成、未抢占（待生成） | 不显示喇叭 |
| `pending` | **已抢占生成权**（在队列中或正在合成） | 显示“生成中” |
| `ready` | 已就绪 | 显示喇叭，可播放 / 自动播放 |
| `failed` | 重试耗尽 | 不显示喇叭 |

### 重启重载

```go
func (g *Generator) Reload(ctx context.Context) error {
    // 启动时没有 worker 在跑，pending 一定是上次崩溃的残留：先归零。
    if _, err := g.repo.ResetStalePendingAudio(ctx); err != nil {
        return err
    }
    questions, err := g.repo.ActiveQuestionsNeedingAudio(ctx) // JOIN interviews 过滤 status=active
    if err != nil {
        return err
    }
    for _, q := range questions {
        g.Enqueue(ctx, q)
    }
    return nil
}
```

`main.go` 启动时执行 `go func() { _ = gen.Reload(ctx) }()`。`Reload` 只读数据库，不依赖内存队列，因此重启后能完整补齐。

### 并发与竞争态

生成环节有三条并发路径：**重启重载**（服务启动时扫库补齐）、**创建面试**（入队全部主问题）、**答题推进**（入队下一题）。它们可能同时命中同一道题，也可能与“面试结束清理”撞上。必须处理三类竞争：

**1. 同一题被并发入队 → 数据库抢占**

内存里的 `sync.Map` 去重只在单进程内有效，且存在“已出队但状态未更新”的窗口。更可靠的做法是让**数据库成为生成权的唯一真源**：

```sql
UPDATE interview_questions SET audio_state = 'pending'
WHERE id = ? AND (audio_state IS NULL OR audio_state = 'failed')
```

条件 UPDATE 是原子的：只有影响行数为 1 的那一方拿到生成权，另一方直接跳过，不会重复合成。`pending` 语义因此从“已入队”收紧为“**已抢占生成权**”，既表示排队中，也表示正在合成。

**2. 面试结束清理 vs 正在进行的合成 → 读写锁 + 终止标记**

清理先删目录，但合成可能刚好在写文件，清完又冒出一个孤儿文件；同时 `audio_state` 被写回 `ready`，与磁盘状态不一致。

处理：生成器持一把 `sync.RWMutex` 与一个“已终止面试”集合。

- `CleanupInterview`：取**写锁** → 标记该面试已终止 → 释放 → 删目录 → 清 `audio_state`。
- `saveAudio`：取**读锁** → 检查终止标记 → 未终止才落盘并写 `ready`。

写锁与读锁互斥，于是只有两种可能：**先落盘后清理**（文件被删掉，状态被清空，一致）或**先清理后落盘**（看到终止标记，直接丢弃结果）。不会出现孤儿文件，也不会出现“状态 ready 但文件不存在”。

**3. 进程崩溃留下 `pending` 残留 → 启动时归零**

抢占后进程崩溃，`pending` 会永久卡住，而 `pending` 又不会被自动扫描重领（避免抢正在进行的任务）。解法是启动时归零：服务刚起来时还没有 worker 在跑，此时所有 `pending` 必然是上次崩溃的残留。`Reload` 先 `ResetStalePendingAudio`（把进行中面试的 `pending` 置回 `NULL`），再扫描重新入队。

**兜底：入队失败要归还生成权**

队列满时任务不会真正被处理，`pending` 会卡住。因此**最终**没能入队的任务必须调用 `ReleaseAudioState` 把 `pending` 放回 `NULL`，让下次 `Reload` 能补上。

两级缓冲：队列（容量 256）满了先放进**进程内 backlog**（上限 4096），worker 每处理完一题就把 backlog 灌回队列——这样启动时一次性补齐上百道题也不会丢。只有 backlog 也满了，才真正丢弃并归还生成权。

> 为什么不用「worker 空闲时回扫数据库续拉」：那条路会反复捞起 `failed` 的题（扫描条件是 `audio_state IS NULL OR 'failed'`），变成对同一道坏题的无限重试。

**另外两处一致性约束**

- **`Reload` 必须在 `Start` 之前调用**：`ResetStalePendingAudio` 会把进行中面试的所有 `pending` 归零，而 `pending` 表示「已抢占生成权」；worker 一旦开始消费，归零就会打断在途任务、导致重复合成。生成器用 `ErrReloadAfterStart` 挡住误用。
- **`CleanupInterview` 先归零状态、再删目录**：归零失败就**跳过删目录**并记 Error 级日志。宁可留下文件，也不能留下「状态 `ready` 但文件不存在」的僵尸——那样前端按 `ready` 请求音频只会拿到 404。

**跨进程兜底**

生成器是进程内的，重启会丢队列，但**不会丢数据**：文件在磁盘、状态在数据库。重启后 `Reload` 从数据库重建队列，已经 `ready` 的题不会被重新合成（抢占条件里 `ready` 不满足）。

### 为什么不在播放接口里同步生成

TTS 单题数秒到数十秒，本地 GPU worker 又是串行的；在播放请求里同步合成会让用户点击后干等，还会与后台队列抢资源。改为“后台提前生成 + 播放只读”后，播放是 O(1) 文件读取，用户体验与系统吞吐都更好。

## 调用链路图

> 只画新增/变化的逻辑；既有评分、生题、Redis 锁、checkpoint 等不展开。

### 创建面试 + 生成触发

```
POST /api/interviews
  → interviewHandler.create
      1) HasActiveInterview(tenant) 预检        ← 新增：最前面，避免白跑
         已有 active → 409，不加载简历/JD
      2) 加载 resume / targetJob（既有）
      3) Service.CreateInterview（既有：生题 + 落库 + StartInterview）
      4) audio.Enqueue(全部主问题)               ← 新增
  → 201 { interview, firstQuestion }
  → 后台 generator 串行生成 → 落盘 → audio_state=ready
```

### 答题 + 追问触发

```
POST /api/interviews/:id/answer（既有图执行）
  → Service.Answer 拿到 graphResult
  → NextQuestion != nil → audio.Enqueue(nextQuestion)   ← 新增
  → Finished         → audio.CleanupInterview(id)      ← 新增
  → 200
```

### 播放题目语音

```
GET /api/interviews/:id/questions/:qid/audio
  → 校验面试/题目归属
  → 读 audio_state
      ready   → 200 audio/wav
      pending → 409 {"state":"pending"}
      failed  → 503 {"state":"failed"}
      NULL    → 409 {"state":"pending"}
```

### 录音转写

```
MediaRecorder → 停止 → decodeAudioData → 重采样 16k/mono → WAV 头
  → POST /api/interviews/:id/questions/:qid/transcribe (multipart)
  → 校验归属 + WAV 格式
  → ASR 转写
  → AI 错别字修复（最低思考强度；失败则用原文）
  → 200 {"text":"..."}
  → 前端插入到光标位置
```

### 前端（简）

```
InterviewChat.tsx
 ├─ load() → GET /questions（含 audioState）
 ├─ 当前题 audioState=ready → 自动播放（浏览器拦截时提示点击）
 ├─ [🔊] 播放 / 重播
 ├─ [🎤] 录音 → 转写 → 插入光标处
 └─ voice.enabled=false → 隐藏语音按钮
```

## 三个必须处理的实现坑

以下是从 `mlx_audio/server.py` 源码核实出来的真实行为，不显式处理会以难以定位的方式失败。

### 坑 1：`response_format` 决定 MIME，但后端只产 WAV

`/v1/audio/speech` 用 `response_format` 拼 Content-Type，而 TTS 内部固定产出 24kHz PCM。传 `mp3` 会得到标着 `audio/mp3`、实则 WAV 字节的响应，浏览器按 MIME 猜格式会拒绝播放。

**处理**：Go 侧固定用 `response_format=wav`，且**不信任**上游返回的 Content-Type —— `Synthesize` 直接定为 `audio/wav`。

### 坑 2：`/v1/audio/transcriptions` 默认返回 ndjson

mlx-audio 的 `response_format` 默认值是 `ndjson`（返回 `application/x-ndjson` 流），按 JSON 解析会失败。

**处理**：Go 侧**必须显式传** `response_format=json`；同时 `Transcribe` 做容错：若 Content-Type 是 ndjson，取第一行 JSON 的 `text` 字段兜底。

### 坑 3：生成队列与 GPU 串行

mlx-audio 的 `InferenceBroker` 是**单 worker 线程**（源码注释：`Keep all GPU work serialized`），所有 TTS/STT 请求在服务端串行执行。

**处理**：Go 侧的生成 worker 也固定并发 1、逐题串行，不与评分/生题抢 GPU；播放接口不生成，天然不抢。追问落库后入队，排在主问题之后。

## 语音文件生命周期

```
uploads/voice/{tenantID}/{interviewID}/q{questionID}.wav
```

以 **interviewID 为清理单元**：面试结束 = 删除一个目录，逻辑简单，且天然满足租户隔离。`audio_state` 状态机见「音频生成与重载」。

**不存录音**：转写音频只在请求内透传给 ASR，不落盘、不回放。

### 清理时机

| 触发点 | 位置 | 动作 |
| --- | --- | --- |
| 正常答完 | `Service.Answer`（`Finished=true`） | 标记该面试已终止 → 删除目录 → 清空该场全部 `audio_state` |
| 用户放弃 | `Service.Abort` | 同上 |

「标记已终止」与「落盘」由生成器的读写锁互斥，保证清理之后不会再冒出孤儿音频文件（见「并发与竞争态」）。

两者都在状态落库之后执行，清理失败只记 warn，不影响面试终态。

### 重启后仍能播放

- 文件在磁盘，`audio_state` 在数据库，重启后仍可读。
- 未生成完的题由 `Reload` 重新入队补齐。
- 面试进度在 Redis checkpoint，重启后仍可恢复（既有机制）。

## 接口文档

全部挂在 `/api/interviews` 组下，走 `requireAuthentication`，租户隔离在 service 层。

### `GET /api/interviews/:id/questions/:qid/audio`

返回该题的题目语音（TTS 产物）。

| 状态码 | 含义 | 响应体 |
| --- | --- | --- |
| 200 | 已就绪 | `audio/wav` 二进制 |
| 401 | 未认证 | `{"error":"authentication required"}` |
| 404 | 面试/题目不存在或不属于当前租户 | `{"error":"interview not found"}` |
| 409 | 未就绪（`pending` / `NULL`） | `{"state":"pending"}` |
| 503 | 语音未启用或不可用 | `{"error":"voice features disabled"}` |
| 503 | 生成失败（`audio_state=failed`） | `{"state":"failed"}` |
| 404 | 状态是 `ready` 但音频文件已不在 | `{"error":"question audio not found"}` |

前端：`ready` → 播放；`pending` → 显示“生成中”；`failed` → 不显示喇叭。

### `POST /api/interviews/:id/questions/:qid/transcribe`

multipart 上传录音，返回经 AI 修复错别字后的文本。

**请求**

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `file` | file | WAV 音频（16kHz 单声道最优，后端校验 RIFF/采样率/声道/时长/体积） |

`qid` 由路径携带。后端不做转码，只校验后透传给 ASR。

**响应**

| 状态码 | 含义 | 响应体 |
| --- | --- | --- |
| 200 | 成功 | `{"text":"修复后的文本"}` |
| 400 | 音频格式非法或超限 | `{"error":"invalid audio file"}` |
| 400 | 缺少文件字段 | `{"error":"audio file is required"}` |
| 400 | 体积超过 20MB | `{"error":"audio file is too large"}` |
| 401 | 未认证 | `{"error":"authentication required"}` |
| 404 | 面试/题目不存在或不属于当前租户 | `{"error":"interview not found"}` |
| 503 | ASR 不可用 | `{"error":"transcription unavailable"}` |

**处理链**：ASR 转写 → AI 错别字修复（`ReasoningEffort=low`，只改错别字/同音字/标点，不改语义、不润色）→ 返回。AI 修复失败时返回 ASR 原文，不阻断转写。

**后端校验**：RIFF magic、采样率、声道数、时长上限 10 分钟、体积上限 20MB。

### `GET /api/interviews/:id/questions`

每题 DTO 增加 `audioState` 字段：

```json
[
  { "id": 12, "question": "...", "audioState": "ready" },
  { "id": 13, "question": "...", "audioState": "pending" }
]
```

取值 `ready` / `pending` / `failed` / `null`。面试结束后清理会置回 `null`。

### `GET /api/interviews/:id`

响应增加语音开关：

```json
{ "id": 1, "status": "active", "voice": { "enabled": true } }
```

`enabled` 反映的是**语音能力是否装配成功**（即 `VoiceService != nil`）：`VOICE_ENABLED` 未开、
或 provider 装配失败时为 `false`，前端据此隐藏语音控件。比「只反映配置」更保守 —— 装配失败时前端
不会显示一个必然失败的喇叭按钮。

### 错误码汇总

| 码 | 场景 |
| --- | --- |
| 400 | 音频格式非法/超限、请求体缺失 |
| 401 | 未认证 |
| 404 | 面试/题目不存在或不属于当前租户 |
| 409 | 已有 active 面试（创建时）、音频未就绪 |
| 503 | 语音服务未启用或不可用、生成失败 |

## 前端交互

### 自动播放

- 进入面试页、每次答题后，若当前题 `audioState === "ready"`，自动播放。
- 若 `pending`，显示“生成中”，并用短轮询（1.5s × 最多 40 次）等待就绪后自动播放。
- **浏览器自动播放限制**：无用户手势时 `audio.play()` 会被拒绝。捕获 `NotAllowedError` 后显示“点击播放”提示，用户首次交互后恢复自动播放。

### 录音转写

输入坞麦克风按钮三态：空闲 / 录音中（红色停止 + 计时）/ 转写中（“识别中…”）。

- 停止录音后自动上传转写，结果**插入到当前光标位置**（不是追加到末尾）。
- 转写失败：保留输入框已有内容，行内提示“识别失败，可继续打字”。
- 麦克风权限被拒：提示可继续打字，不反复弹权限请求。
- 录音期间输入框仍可编辑。

### 降级

`voice.enabled=false` 时隐藏全部语音按钮，退回纯文字面试。运行时服务不可达由语音接口返回 503/409 + 行内提示处理。

## 配置

```
VOICE_ENABLED=true

# TTS 与 ASR 各自独立指向一个 OpenAI 兼容服务
VOICE_TTS_BASE_URL=http://127.0.0.1:21100/v1
VOICE_TTS_API_KEY=
VOICE_TTS_MODEL=mlx-community/Qwen3-TTS-12Hz-0.6B-CustomVoice-8bit
VOICE_TTS_VOICE=Vivian

VOICE_ASR_BASE_URL=http://127.0.0.1:21100/v1
VOICE_ASR_API_KEY=
VOICE_ASR_MODEL=mlx-community/Qwen3-ASR-1.7B-8bit

# 生成队列
VOICE_GENERATOR_MAX_ATTEMPTS=3
VOICE_GENERATOR_BACKOFF=1s,3s,10s
VOICE_TTS_TIMEOUT=120s
VOICE_ASR_TIMEOUT=60s
VOICE_CORRECT_TIMEOUT=30s
VOICE_CACHE_DIR=./uploads/voice
```

- `VOICE_ENABLED` 未开 → 不创建 provider，`voice.enabled=false`，路由返回 503。
- TTS / ASR 的 base URL、API key、模型各自独立，可自由组合。
- 错别字修复复用 `OPENAI_MODEL`，独立实例固定 `ReasoningEffort=low`、超时 30s。

## 降级设计

**核心原则**：语音是附加层。**`answer` 主链路在任何语音故障下都必须成功返回**。

| 层级 | 故障 | 兜底行为 | 用户感知 |
| --- | --- | --- | --- |
| 配置层 | `VOICE_ENABLED=false` | 不创建 provider；`voice.enabled=false`；路由仍注册 | 隐藏语音按钮，纯文字面试 |
| 创建面试 | 生成失败 | 后台重试 → 标记 `failed`，不阻断创建 | 无（创建照常 201） |
| **答题主链路** | 任何语音故障 | **answer 不调用 TTS/ASR**（生成在后台队列） | 评分/下一题延迟与语音无关 |
| 播放题目语音 | 未就绪 | 409 + 前端显示“生成中”，就绪后自动播放 | 可继续打字 |
| 录音转写 | ASR 不可达 / 超时 | 503 + 行内提示，输入框内容保留 | 可继续打字 |
| 错别字修复 | LLM 不可用 | 返回 ASR 原文，不阻断转写 | 无感知 |
| 面试结束清理 | 删文件失败 | 只 warn，不影响终态 | 无 |
| 后端重启 | 内存队列丢失 | 文件 + `audio_state` 持久；`Reload` 补齐未生成 | 重新进入仍可播放 |

### 实现约束（硬性）

1. **answer 主链路不得同步等待语音**。生成只发生在后台 worker。
2. **语音失败不污染答题状态**。`audio_state` 只表达音频存在性，与 `status` / `grade` 无关。
3. **前端语音按钮全部 local error 处理**，只设置行内提示，绝不阻断提交。
4. **`voice.enabled` 只反映配置**，不反映运行时可达性。

## 测试

### 后端

- `speech/openai.go`：假服务验证 speech / transcriptions 请求的 method、path、query、multipart 字段、`response_format=json`；验证 wav 场景 Content-Type 强制为 `audio/wav`；验证 ndjson 兜底解析。
- `speech/openai.go`：TTS 与 ASR 分别指向不同 base URL 时各自命中正确服务。
- `speech/generator.go`：串行顺序、`pending → ready` 状态流转、失败重试后 `failed`、`Reload` 只补齐未完成项、`Enqueue` 去重。
- `speech/store.go`：落盘路径按 `{tenantID}/{interviewID}/q{questionID}.wav`；`CleanupInterview` 删除整场目录且不影响其他面试。
- `speech/correct.go`：只改错别字、不改语义；LLM 失败时返回原文。
- `interview/repository.go`：`ActiveQuestionsNeedingAudio` 只返回 active 面试中未就绪的题。
- `interview` 层：创建面试后入队全部主问题；`Answer` 后入队 `NextQuestion`；`Finished` / `Abort` 后清理；**清理失败不影响答题返回值**。
- `store/migrate.go`：`audio_state` 列迁移幂等。
- `httpapi/voice_handler.go`：成功、401、跨租户 404、非法音频 400、未就绪 409、服务不可用 503。
- **降级（关键）**：provider 不可达时，`answer` 仍返回 200 且结果与无语音时逐字段一致；`CreateInterview` 仍返回 201。
- **重启恢复**：新建 service 实例后 `Reload` 能从 DB 重建队列并补齐音频。
- 遵守 `AGENTS.md` 门禁：改 `backend/**` 后 `cd backend && go test ./...` 必须通过。

### 前端

- `utils/wav.ts`：采样率转换与 WAV 头字节布局。
- `hooks/useVoiceRecorder.ts`：三态转换、权限拒绝、上传失败。
- `components/InterviewChat.tsx`：自动播放（含浏览器拦截兜底）、光标插入、`voice.enabled=false` 时隐藏按钮、`pending` 显示“生成中”。
- `api/voice.ts`：multipart 上传不走 `apiFetch`。

## 落地文件

```
backend/internal/speech/
  speech.go       Synthesizer / Transcriber / TextModel 接口与请求响应类型
  config.go       从 env 读配置（TTS / ASR 独立）
  openai.go       OpenAICompatibleProvider：TTS + ASR 共用协议
  store.go        题目音频落盘 + 目录清理（以 interviewID 为单元）
  generator.go    串行生成 worker + 退避重试 + 重启 Reload
  correct.go      AI 错别字修复（最低思考强度）
  hooks.go        实现 interview.AudioHooks（Enqueue / CleanupInterview）
backend/internal/httpapi/voice_handler.go   语音接口（播放 / 转写）
backend/internal/httpapi/interview_handler.go  创建面试入口预检
backend/internal/interview/repository.go     Question.AudioState + 3 个方法
backend/internal/interview/service.go        AudioHooks 注入 + 触发 Enqueue/Cleanup
backend/internal/store/migrate.go            audio_state 列迁移
backend/cmd/server/main.go                   接线 generator + 启动 Reload
frontend/src/api/voice.ts                    语音接口层
frontend/src/hooks/useVoiceRecorder.ts       录音 → WAV → 上传
frontend/src/utils/wav.ts                    浏览器端重采样 + WAV 封装
frontend/src/components/InterviewChat.tsx    自动播放 / 光标插入 / 按钮集成
```

`interview/repository.go` 新增方法：

- `QuestionByID(ctx, id)`：播放接口校验题目归属。
- `UpdateAudioState(ctx, questionID, state)`：生成 worker 回写状态。
- `ActiveQuestionsNeedingAudio(ctx)`：`Reload` 扫描 active 面试中 `audio_state ∈ {NULL, pending, failed}` 的题。


## 实现说明（落地后回写）

### 实际落地结构

```
backend/internal/interview/audio.go          AudioHooks 窄接口 + PendingAudio
backend/internal/interview/repository.go     audio_state 四态 + 抢占式 6 方法
backend/internal/interview/service.go        WithAudioHooks / HasActiveInterview / QuestionForAudio
backend/internal/interview/graph.go          追问落库（追问音频在 service 层统一入队）
backend/internal/speech/speech.go            Synthesizer / Transcriber / TextModel + ValidateWAV
backend/internal/speech/config.go            Config + LoadConfigFromEnv（VOICE_ENABLED 默认关闭）
backend/internal/speech/openai.go            OpenAI 兼容 provider（同时实现 TTS 与 ASR）
backend/internal/speech/store.go             AudioStore 落盘（临时文件 + rename 原子提交）
backend/internal/speech/generator.go         后台串行生成器（抢占去重 + 读写锁 + 启动归零）
backend/internal/speech/correct.go           错别字修复（独立模型实例，ReasoningEffort=low）
backend/internal/speech/service.go            VoiceService 门面
backend/internal/httpapi/voice_handler.go    播放 / 转写两个接口
backend/internal/httpapi/router.go           路由无条件注册（缺依赖 → 503）
backend/cmd/server/main.go                   buildVoiceServices + Reload → Start 接线
frontend/src/api/voice.ts                    语音接口层
frontend/src/utils/wav.ts                    重采样 + PCM16 WAV 封装
frontend/src/hooks/useVoiceRecorder.ts       录音 → WAV → 上传（麦克风释放 + 连点保护）
frontend/src/components/InterviewChat.tsx    自动播放 / 喇叭 / 麦克风 / 光标插入
tools/voice-server/README.md                 mlx-audio 操作手册 + 端到端验证清单
tools/voice-server/stub_server.py            零依赖 OpenAI 兼容桩服务（无 GPU 也能联调）
```

### 状态机与归属

| `audio_state` | 含义 | 谁写 | 前端表现 |
| --- | --- | --- | --- |
| `NULL` | 未生成（尚未入队，或已归还生成权） | `ReleaseAudioState` / `ResetAudioStateByInterview` | 不显示喇叭 |
| `pending` | 已抢占生成权 | `ClaimAudioState` | 显示「生成中」，轮询重试 |
| `ready` | 音频已落盘 | `UpdateAudioState` | 显示喇叭，自动播放 |
| `failed` | 重试耗尽 / 音频非法 | `UpdateAudioState` | 不显示喇叭 |

生成权的唯一真源是数据库条件 UPDATE（`ClaimAudioState`：仅 `NULL`/`failed` 可抢）。
进程内 `sync.Map` 去重只是省掉一次无谓 UPDATE，不是真源。

### 降级矩阵

| 场景 | 表现 |
| --- | --- |
| `VOICE_ENABLED` 未开 | 路由仍注册 → 503；前端隐藏语音控件；面试全程正常 |
| mlx-audio 未启动 | 该题 `failed`；其它题照常；面试全程正常 |
| 上游超时 / 报错 | 重试（退避 + `MaxAttempts`），耗尽后 `failed` |
| AI 错别字修复失败 | 返回 ASR 原文，不清空用户说的话 |
| 浏览器拒绝麦克风 | 提示「无法使用麦克风，可继续打字」，页面不崩 |
| 浏览器拦截自动播放 | 提示「点击喇叭播放本题」，主问题与追问都有喇叭 |
| TTS 产物不是合法 WAV | 判为合成失败，不落盘 |

### 关键实现约定（踩过的坑）

1. **`Reload` 必须在 `Start` 之前**：归零 pending 与 worker 消费在途任务互斥，生成器用 `ErrReloadAfterStart` 挡住误用。
2. **路由无条件注册**：语音缺席时返回 503 而不是 404「路由不存在」，调用方才能区分两者。
3. **播放只读文件**：绝不在请求内同步合成（TTS 单题数秒到数十秒）。
4. **两级缓冲**：队列（256）满了先进进程内 backlog（4096），worker 优先消费 backlog；两者都满才丢弃并归还生成权。
5. **worker 三层 recover**：provider 一次 panic 不能打挂整个后端进程。
6. **落盘前校验**：存储层不校验音频格式（那是它的契约），由生成器在落盘前 `ValidateWAV` 兜一次。
7. **上传体积在解析前限制**：multipart 解析会把整个 body 读进内存/临时文件，必须先 `Content-Length` 判断 + `MaxBytesReader` 兜底，否则用户录音会短暂落盘。
8. **前端只产出 PCM16 WAV**：`MediaRecorder` 默认 webm/opus，必须自己重采样到 16kHz 单声道并封装 RIFF/WAVE，否则后端 `ValidateWAV` 直接拒。

### 已知限制（实现后确认）

- 本地推理服务需手动启动（`mlx_audio.server`），后端不代管其生命周期；`tools/voice-server/stub_server.py` 可在无 GPU 时替代联调。
- 生成是串行队列，创建面试到全部题目就绪之间有延迟；前端用「生成中」状态与轮询覆盖。
- 追问题音在追问落库后入队，排在主问题之后；主问题多时追问音频可能延迟就绪。
- **被丢弃的 active 面试没有回收机制**：`ActiveQuestionsNeedingAudio` 只按 `status='active'` 过滤，没有时间上界。既没答完也没 abort 的面试会永远停在 `active`，其音频每次重启都被重新合成（TTS 成本重复消耗）。需要运维定期清理，或后续给该查询加时间上界。
- 错别字修复用**独立的模型实例**（`ReasoningEffort=low`、独立超时），不复用面试出题模型 —— 校对任务不需要长思考。
- **面试结束即清理音频**，报告页只能看文字与评分。
- 音频目录以 interviewID 为清理单元，不跨面试复用。
- 清理只覆盖「面试状态变终态」这一路径；进程崩溃残留由下次启动的 `Reload` + 清理对账兜底。
- 浏览器侧要求 HTTPS 才能用麦克风；Safari 的 `decodeAudioData` 解 MediaRecorder 产物可能失败（落到「识别失败」分支，页面不崩）。
