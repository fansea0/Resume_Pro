# AI 模拟面试语音能力（题目朗读 + 录音转写）— 设计文档

> 日期：2026-11-05
> 状态：设计待审
> 影响范围：新增 `backend/internal/speech/` 包、`backend/internal/httpapi/voice_handler.go`、`frontend/src/api/voice.ts`、`frontend/src/hooks/useVoiceRecorder.ts`、`frontend/src/utils/wav.ts`，改动 `backend/cmd/server/main.go`、`backend/internal/httpapi/router.go`、`backend/internal/store/migrate.go`、`backend/internal/interview/`、`frontend/src/components/InterviewChat.tsx`。
> **数据库改动仅一列**：`interview_questions.audio_state`（幂等迁移，新增列可空）。转写文本不入库，走现有答题接口提交。

## 目标

AI 模拟面试目前是纯文字流程：面试官出题、用户打字回答。本功能给它加上声音：

- **题目朗读**：面试官提问后可以听语音，也可随时重播。
- **录音转写**：用户可以口述回答，识别成文字填入输入框，确认后提交。

**核心设计原则**：

- **抽象在协议层，不在厂商层**。语音能力对外只暴露两个 Go 接口，实现层统一走 OpenAI 音频协议（`/v1/audio/speech`、`/v1/audio/transcriptions`）。本地 MLX 推理服务和云端 API 因此是同一份代码，切换只改环境变量。若将来某家 API 不兼容 OpenAI 协议，新增一个 provider 实现即可，上层（interview / httpapi）零改动。
- **文字为主，语音是增强**。不改变现有打字答题流程，不做全语音模式。识别结果一定回填到输入框让用户过目，绝不自动提交。
- **后端零音频编解码依赖**。本机没有 ffmpeg，转码全部在浏览器完成（Web Audio API），Go 侧只搬运字节。
- **语音服务不可用时优雅降级**。语音是可选增强，任何失败都不能阻断面试主流程。

## 非目标

- 不做流式 TTS（边生成边播），不做播放打断。
- 不做语音克隆、不做音色设计、不做情感指令。
- 不做实时语音对话（Realtime API）。
- 不把转写结果落库、不做转写文本的后处理（去口语词、纠错）。
- 不做录音的长期归档 —— 面试结束即清理，报告页只保留文字。
- 不引入麦克风权限的额外管理界面。
- 不改动现有 LLM 评分与追问逻辑。

## 现有约束

调查现有代码后确认的、决定实现方式的事实：

- **后端**：Go 1.26 + Gin，路由统一在 `backend/internal/httpapi/router.go` 的 `NewRouter` 构造，`RouterConfig` 携带依赖，缺依赖时路由仍注册并返回 503（AI 功能的既有降级约定）。日志用 logrus 结构化字段。
- **面试状态**：面试进行态存在 Redis（`internal/interview/redis_store.go`），题面与答案存 SQLite。创建面试时**一次性生成全部主问题并落库**，追问在评分时才动态产生。
- **现有 REST 面**：`/api/interviews` 组下有 create / list / get / answer / questions / abort，全部走 `requireAuthentication` 中间件，租户隔离靠 service 层校验。
- **前端**：React 18 + Vite，`InterviewChat.tsx` 是进行中页面（逐题对话流 + 贴底输入坞），`api/interviews.ts` 是接口层，`api/client.ts` 的 `apiFetch` 固定 `Content-Type: application/json`（multipart 需另走 fetch）。
- **测试**：后端用 httptest + 内存 SQLite 做路由级黑盒测试；前端用 vitest + jsdom。
- **本机**：Apple M5 / 32GB RAM / macOS 26.4.1，**无 CUDA**（官方 vLLM 路线不可用），无 ffmpeg，已有 ollama（仅 embedding 模型）与 uv。

## 技术选型

### 模型与运行时

| 用途 | 模型 | 体积 | 说明 |
| --- | --- | --- | --- |
| ASR | `mlx-community/Qwen3-ASR-1.7B-8bit` | ~1.5 GB | 官方 mlx-audio 0.3.1 转换版 |
| TTS | `mlx-community/Qwen3-TTS-12Hz-0.6B-CustomVoice-8bit` | ~1.9 GB | 带 9 个预置音色 |

两模型同时常驻约 3.5 GB，32GB 内存无压力。

**选 MLX 而非 GGUF**：M5 的 MLX Metal 融合路径成熟，量化转换与推理代码都在 mlx-audio 内，Go 侧只需对接一个现成的 OpenAI 兼容 server，不必自建 llama.cpp 代理层。

**选 8bit 而非 4bit**：有实测依据 —— mlx-audio PR #398 记录 CustomVoice 8bit 效果良好，4bit "still okay, but kind of slow; a little weird intonation"。ASR 侧 4bit 量化 `embed_tokens` 在日语等语种会出现幻觉（实测 CER 劣化 172%）。省下的 1GB 不值得。

### 推理服务

`mlx-audio` 自带 OpenAI 兼容 API server（FastAPI + uvicorn），无需自写代理：

```bash
mlx_audio.server --host 127.0.0.1 --port 21100
```

端口 21100 避开常用段（本机已跑多个项目）。

## 架构

```
frontend/src/components/InterviewChat.tsx
  ├─ api/voice.ts            GET questions/:qid/audio / POST voice/transcribe
  ├─ hooks/useVoiceRecorder  MediaRecorder → WAV(16k mono) → 上传
  └─ utils/wav.ts            浏览器端重采样 + WAV 封装
        │ HTTP (JSON / multipart)
        ▼
backend/internal/httpapi/voice_handler.go
        │
        ▼
backend/internal/speech/          ← 抽象层
  ├─ speech.go    Synthesizer / Transcriber 接口 + 请求响应类型
  ├─ openai.go    OpenAICompatibleProvider（唯一实现）
  ├─ config.go    从 env 读配置（TTS / ASR 各自独立）
  ├─ store.go     音频落盘 + 目录清理
  └─ cache.go     合成去重 + singleflight
        │ HTTP (OpenAI 音频协议，两个 base URL 独立)
        ├──────────────────────────┐
        ▼                          ▼
TTS 服务 (VOICE_TTS_BASE_URL)   ASR 服务 (VOICE_ASR_BASE_URL)
  POST /v1/audio/speech          POST /v1/audio/transcriptions
        │
        └─ 本地默认：两者都指向 mlx_audio.server :21100
           可各自独立切到云端，无需改 Go 代码
```

### 抽象层接口

```go
package speech

type SynthesisRequest struct {
    Text     string
    Voice    string
    Language string
}
type SynthesisResult struct {
    Audio       []byte
    ContentType string
}

type TranscriptionRequest struct {
    Audio       []byte
    ContentType string
    Language    string
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
```

接口与传输实现分离，收益具体体现在：

- `VOICE_TTS_BASE_URL=http://127.0.0.1:21100/v1` → 本地 MLX 出题
- `VOICE_ASR_BASE_URL=https://dashscope.aliyuncs.com/compatible-mode/v1` + `VOICE_ASR_MODEL=qwen3-asr-flash` → 阿里云百炼转写

两者协议同源，是同一份 `OpenAICompatibleProvider` 代码，不存在"本地版/云端版"两套实现。且 TTS 与 ASR 的配置完全独立（各自的 base URL / api key / model / http.Client），可任意组合。

## 三个必须处理的实现坑

以下三点是从 `mlx_audio/server.py` 源码核实出来的真实行为，不是假设。若不显式处理，功能会以难以定位的方式失败。

### 坑 1：`response_format` 决定 MIME，但后端只产 WAV

`/v1/audio/speech` 用 `media_type=f"audio/{payload.response_format}"` 拼 Content-Type，而 TTS 内部固定产出 24kHz PCM。传 `response_format=mp3` 会得到一个**标着 audio/mp3、实则 WAV 字节**的响应，浏览器 `<audio>` 按 MIME 猜格式会拒绝播放。

**处理**：Go 侧请求固定用 `response_format=wav`，且**不信任**上游返回的 Content-Type —— `Synthesize` 直接把 Content-Type 定为 `audio/wav`，与实际字节一致。前端不依赖 MIME 猜测，直接按 `audio/wav` 播放。

### 坑 2：`/v1/audio/transcriptions` 默认返回 ndjson

OpenAI 协议要求 JSON，但 mlx-audio 的 `response_format` **默认值是 `ndjson`**（返回 `application/x-ndjson` 流）。不显式指定就拿到 ndjson，按 JSON 解析会失败。

**处理**：Go 侧请求**必须显式传** `response_format=json`，此时服务端返回 `{"text": "..."}`。同时 `Transcribe` 对响应做容错：若 Content-Type 是 ndjson，取第一行 JSON 的 `text` 字段作为兜底，防止服务端默认值变化导致整体失效。

### 坑 3：预合成与前台请求在服务端抢同一个 GPU worker

mlx-audio 的 `InferenceBroker` 是**单 worker 线程**（源码注释：`Keep all GPU work serialized`），所有 TTS/STT 请求在服务端串行执行。预合成 worker 若正在合成一批题，用户此时点喇叭，请求会在服务端队列里排队等前面的批次跑完。

关键细节：broker 支持 **batch 合并** —— 同 `endpoint_kind` + 同 `model_name` + 同 `batch_key` 的待处理请求会被凑成一批一次执行（TTS 默认 `max_batch_size=8`）。这既是机会也是风险：

- **机会**：预合成并发提交多题，服务端会自动合批，吞吐显著高于逐题串行。
- **风险**：一批 8 题合成期间，用户点喇叭的请求排在队尾，要等整批跑完。

**处理**：Go 侧不试图绕过服务端的串行化（那需要改 mlx-audio，不在我们的改动范围），而是在**请求排序与合批粒度**上做控制：

1. **前台优先**：预合成 worker 用**独立的 HTTP 客户端与独立的 semaphore（并发 1）**，与前台请求分属不同连接池。前台请求发出后立即到达服务端队列；预合成批次虽大，但用户请求排在队尾的等待时间 = 当前正在执行的那一批剩余时间。
2. **缩小预合成批次**：预合成**逐题串行提交**（不靠并发凑批），每题单独一个请求。这样服务端每次只跑 1 题，用户点喇叭最多等 1 题的时间，而不是一整批 8 题。代价是预合成总时长变长 —— 但预合成在后台进行，总时长不敏感，用户等待时间才是敏感指标。
3. **同一题不重复计算**：`speech.Cache` 用 `sync.Map` 记录每题状态（pending / ready / failed）+ singleflight。前台请求命中 pending 时**不重复发起**，等待该任务完成或超时；预合成失败标记 failed，前台请求改为自行发起一次同步合成（singleflight 保证不并发重复）。

这样三个目标同时成立：预合成在后台跑、不重复算、用户点喇叭最多等一题。

## 语音文件生命周期

题目语音（TTS 产物）与用户录音（上传的原始音频）统一存放在一个面试专属目录下，面试结束时整体删除。

### 目录布局

```
uploads/voice/{tenantID}/{interviewID}/
  q{questionID}.wav      题目语音（TTS 产物）
  r{questionID}.wav      用户录音（原始上传音频）
```

以 **interviewID 为清理单元**，而不是以内容 hash 为单元。这带来两个直接好处：

1. 面试结束 = 删除一个目录，逻辑简单，不需要跨目录反查引用。
2. 天然满足租户隔离 —— 路径第一层就是 tenantID，且只经鉴权接口读取，不通过静态目录裸暴露。

代价是同一段题目文本在不同面试间不再复用音频。但面试题目是一次性生成、几乎不重复的内容，而跨面试复用会带来"清理时无法判断是否仍被其他面试引用"的难题 —— 用 interviewID 作单元更简单也更安全。

### 存在性需要落库

后端重启后，内存态全部丢失，但磁盘文件还在。为了让前端知道"这题有没有音频"，以及让后端在重启后仍能正确回答，需要把存在性写入数据库。

`interview_questions` 表新增一列：

```sql
ALTER TABLE interview_questions ADD COLUMN audio_state TEXT;
```

取值：

| 值 | 含义 |
| --- | --- |
| `NULL` | 尚无音频（未合成 / 未录音 / 已清理） |
| `ready` | 题目语音已就绪，可播放 |
| `recorded` | 用户录音已保存，可回放 |

迁移沿用 `backend/internal/store/migrate.go` 的幂等 SQL 风格（`CREATE TABLE IF NOT EXISTS` 同款写法，重复执行安全）。

**为什么不靠目录扫描**：目录扫描需要遍历 `{tenantID}/{interviewID}/` 下所有文件再与题目 id 求交集，每次播放前都要做一次 IO；且无法区分"文件存在但内容损坏"与"文件不存在"。落库是 O(1) 查询，且状态语义明确。

### 三种音频状态与来源

| 文件 | 何时写入 | `audio_state` |
| --- | --- | --- |
| `q{questionID}.wav` | 预合成或前台合成题目语音成功后 | `ready` |
| `r{questionID}.wav` | 用户录音上传并转写成功后 | `recorded` |
| 两者 | 面试结束（completed / aborted）后整体删除 | 回到 `NULL` |

用户录音与题目语音是**两个独立文件**，可以共存（用户录了音，题目语音也合成了）。但 `audio_state` 只有一列 —— 因此该列表达的是"**该题是否有可播放内容**"，具体是题目语音还是用户录音，由前端按需请求对应文件并由 404 判定。

更精确的做法是拆两列（`tts_state` / `recording_state`），但当前需求下两列的组合只有 4 种、实际只会用到 2 种，一列足够且更省事。若后续需要"题目语音与用户录音同时可播且要分别标识"，再拆列。

### 清理时机

面试结束有两个触发点，**都要清理**：

| 触发点 | 代码位置 | 动作 |
| --- | --- | --- |
| 正常答完（生成总评） | `graph.go` finishNode（status=completed 落库后） | 删除目录 + 清 `audio_state` |
| 用户放弃 | `interview.Service.Abort`（status=aborted 落库后） | 同上 |

两者都在**状态落库之后**执行清理。清理失败**只记 logrus warn，不影响面试结果** —— 状态已经是终态，不能因为删文件失败而让答题接口报错。

清理实现在 `speech` 包内（`speech.AudioStore.CleanupInterview`），由 interview 层在上述两个时机调用。interview 包不直接碰文件系统，保持它现有的分层（service 不 import resume / targetjob 包，同理不 import speech 的存储实现，只依赖一个窄接口）。

### 重启后仍能播放

面试进行中重启后端：

- **文件还在磁盘** —— `uploads/voice/{tenantID}/{interviewID}/` 是持久目录，不随进程消失。
- **状态还在数据库** —— `audio_state` 列落库，重启后仍能查到哪题有音频。
- **面试进度还在 Redis** —— 面试图 checkpoint 由 Redis 持久化（既有机制），后端重启后 `interview.Service.Questions` 仍能读回题目与已答内容。

因此前端重新进入面试页时，`GET /api/interviews/:id/questions` 的每题 DTO 带上 `audioState` 字段，前端据此决定是否显示"播放题目语音"与"回放我的录音"按钮。**这就是"面试未结束重启还需要支持播放"的完整链路。**

需要注意的是：若 Redis 本身也重启导致 checkpoint 丢失，现有机制会返回 `ErrSessionExpired`（会话过期，需重新开始面试）—— 这是既有行为，语音功能不改变它。

## 数据流

### 题目朗读

```
创建面试成功
  → 后台 worker 按题号顺序取主问题，逐题串行提交（并发 1，见坑 3）
  → 对每题：Synthesize(text) → 落盘 uploads/voice/{tenantID}/{interviewID}/q{questionID}.wav
  → 更新 audio_state = 'ready'
  → 失败：记 logrus warn，标记 failed，不阻断创建流程

用户点某题喇叭
  → GET /api/interviews/:id/questions/:qid/audio
  → 校验租户归属
  → 命中缓存：直接返回文件字节
  → 未命中：发起同步合成（singleflight）→ 写缓存 → 更新 audio_state='ready' → 返回
```

缓存单元是 `{tenantID}/{interviewID}/q{questionID}.wav`，面试结束即随目录整体删除（见"语音文件生命周期"）。

### 录音转写

```
用户点麦克风 → MediaRecorder 录制（webm/opus 或 mp4/aac）
  → 停止后 decodeAudioData 解码
  → OfflineAudioContext 重采样到 16kHz 单声道
  → 手工写 16bit PCM WAV 头
  → POST /api/interviews/:id/voice/transcribe（multipart）
  → Go 侧落盘 uploads/voice/{tenantID}/{interviewID}/r{questionID}.wav
  → Go 侧转发给 provider（multipart，response_format=json）
  → 返回 {"text": "..."}
  → 更新 audio_state = 'recorded'
  → 前端追加进输入框（输入框为空则直接填），用户可编辑
  → 用户确认后走现有 POST /:id/answer 提交
```

**转写文本不入库**。它只是帮用户打字，提交路径与纯键盘完全一致。**但录音音频落盘**（`r{questionID}.wav`），用于回放与重启后恢复，面试结束时随目录清理。

### 回放我的录音

```
用户点"回放录音"
  → GET /api/interviews/:id/questions/:qid/recording
  → 校验租户归属 + audio_state
  → 命中：返回 r{questionID}.wav 字节
  → 未命中：404（前端隐藏按钮，不会触发）
```

## 后端接口

挂在现有 `/api/interviews` 组下，走 `requireAuthentication`，租户隔离在 service 层校验（与既有接口一致）。

### `GET /api/interviews/:id/questions/:qid/audio`

返回该题的**题目语音**（TTS 产物）。

- 200：`audio/wav` 二进制
- 404：面试或题目不存在、不属于当前租户，或该题无音频（与其他接口一致的 404 语义，不泄露存在性）
- 503：语音服务未启用或不可用

### `GET /api/interviews/:id/questions/:qid/recording`

返回该题的**用户录音**（原始上传音频）。

- 200：`audio/wav` 二进制
- 404：面试或题目不存在、不属于当前租户，或该题无录音
- 503：语音服务未启用

### `POST /api/interviews/:id/voice/transcribe`

multipart 上传录音。**落盘后再转写**，两者都在本接口内完成。

- 请求：`file` 字段，WAV 音频
- 200：`{"text": "识别文本"}`
- 400：音频格式非法或超限
- 404 / 503：同上

**后端校验**：RIFF magic、采样率、声道数、时长上限（10 分钟）、体积上限（20MB）。后端**不做任何转码**，只做校验与字节透传。

### 题目 DTO 新增 `audioState`

`GET /api/interviews/:id/questions` 的每题返回体增加：

```json
{ "id": 12, "question": "...", "audioState": "ready" }
```

取值 `ready` / `recorded` / `null`。前端据此显示"播放题目语音"与"回放我的录音"按钮。面试结束后清理会把该列置回 `null`，与文件删除保持一致。

**这是本设计唯一的 schema 改动**（`interview_questions.audio_state` 新增一列，幂等迁移）。转写文本仍不入库。

## 前端交互

### 题目朗读

每道面试官题目右侧加喇叭图标按钮，点击播放该题语音。可重播已答过的题。

- 图标按钮遵循仓库既有约定：有足够点击面积、键盘焦点样式、无障碍名称（`aria-label`）、`title` 悬浮提示。
- 播放中该按钮显示加载态（合成中）。
- 播放失败给出行内提示，不弹窗打断。

### 录音转写与回放

输入坞加麦克风按钮，三态：

| 状态 | 表现 |
| --- | --- |
| 空闲 | 麦克风图标，可点 |
| 录音中 | 红色停止图标 + 计时 |
| 转写中 | 转写动画 + "识别中…"，输入框禁用 |

- 停止录音后自动上传转写，识别文本**追加**进输入框（输入框为空则直接填）。
- 转写失败：保留输入框已有内容，给出行内提示"识别失败，可继续打字"。
- 麦克风权限被拒：提示用户可继续打字，不反复弹权限请求。
- 录音期间输入框仍可编辑（不影响现有打字路径）。

**回放**：题目已录音后，该题的"我的回答"气泡旁出现回放按钮，点击播放 `GET .../recording` 返回的音频。用户可借此核对识别是否准确，必要时重新录音。

回放按钮按题目 DTO 的 `audioState` 决定显隐：
- `ready` → 显示"播放题目语音"
- `recorded` → 显示"回放我的录音"
- `null` → 都不显示

**面试进行中重启前端或后端**：重新进入面试页时 `GET .../questions` 返回的 `audioState` 仍然有效，按钮照常显示并可播放（文件与状态都在磁盘 / 数据库里）。

### 降级

`GET /api/interviews/:id` 响应增加 `voice: { enabled: bool }`。`VOICE_ENABLED=false` 或语音服务不可达时前端隐藏喇叭与麦克风按钮，退回纯文字面试；后端路由仍注册并返回 503（与既有 AI 降级约定一致）。

## 配置

新增环境变量（`backend/internal/speech/config.go` 读取，`cmd/server/main.go` 接线）：

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

VOICE_TIMEOUT=120s
VOICE_CACHE_DIR=./uploads/voice
```

- `VOICE_ENABLED` 未开 → 不创建 provider，`voice.enabled=false`，路由返回 503。
- `VOICE_TTS_API_KEY` / `VOICE_ASR_API_KEY` 本地服务留空，云端填 key。
- **TTS 与 ASR 的 base URL、API key、模型各自独立**，因此可以自由组合：本地 MLX 出题 + 云端 ASR 转写，或反之。两者互不影响，不需要同时指向同一个服务。
- `VOICE_TTS_BASE_URL` 与 `VOICE_ASR_BASE_URL` 默认值相同（都指向本地 mlx-audio），但**不共用一个变量** —— 需求明确要求分开配置，后续任一侧切换到云端不会牵动另一侧。

对应到抽象层：`Synthesizer` 只依赖 TTS 配置，`Transcriber` 只依赖 ASR 配置。实现仍是同一个 `OpenAICompatibleProvider` 代码路径，但两者各自持独立的 base URL / api key / model / http.Client，因此可以指向不同的服务实例。

## 错误处理

| 场景 | 行为 |
| --- | --- |
| 语音服务未启动 | 创建面试时预合成静默失败（logrus warn）；前台请求返回 503；前端隐藏语音按钮 |
| 单题合成失败 | 标记 failed，该题下次点喇叭重试一次；不影响其他题 |
| 预合成任务失败 | 不阻断创建面试，不影响文字答题 |
| 转写超时 | 返回 503，前端提示"识别超时，可继续打字" |
| 转写返回空文本 | 返回 200 + 空串，前端提示"没听清，可重新录音或打字" |
| 录音格式非法 | 后端 400，前端提示录音格式问题 |
| 面试结束清理失败 | 只记 logrus warn，不影响面试终态；残留文件由下次启动的清理扫描兜底 |
| 重启后某题无音频 | `audioState` 为 `null`，前端不显示播放按钮 |

## 测试

### 后端

- `speech/openai.go`：httptest 假服务验证 speech / transcriptions 请求的 method、path、query、multipart 字段、`response_format=json` 是否显式携带；验证 wav 场景下 Content-Type 被强制为 `audio/wav`；验证 ndjson 兜底解析。
- `speech/openai.go`：**TTS 与 ASR 分别指向不同 base URL** 时各自命中正确的服务（配置拆分的回归测试）。
- `speech/store.go`：落盘路径按 `{tenantID}/{interviewID}/q{questionID}.wav` 与 `r{questionID}.wav` 拼装；跨租户不串目录；`CleanupInterview` 删除整个面试目录且不影响其他面试。
- `speech/cache.go`：命中/未命中、singleflight 不重复发起、失败标记。
- `interview` 层清理钩子：面试 completed / aborted 后目录被删且 `audio_state` 置回 NULL；**清理失败不影响答题接口返回值**。
- `store/migrate.go`：`audio_state` 列迁移幂等 —— 迁移重复执行不报错（既有迁移测试风格）。
- `httpapi/voice_handler.go`：路由级黑盒测试 —— 成功路径、未认证 401、跨租户 404、非法音频 400、服务不可用 503；`GET .../recording` 无录音时 404。
- **重启恢复**：新建 service 实例（模拟重启）后仍能从磁盘读到音频、从数据库读到 `audio_state`。
- 遵守 `AGSIN.md` 门禁：改 `backend/**` 后 `cd backend && go test ./...` 必须通过。

### 前端

- `utils/wav.ts`：采样率转换与 WAV 头字节布局（单测，不依赖浏览器 API）。
- `hooks/useVoiceRecorder.ts`：vitest + jsdom，mock MediaRecorder / AudioContext，覆盖三态转换、权限拒绝、上传失败。
- `components/InterviewChat.tsx`：喇叭按钮渲染与点击、语音按钮在 `voice.enabled=false` 时隐藏；按 `audioState` 显示"播放题目语音"/"回放我的录音"。
- `api/voice.ts`：multipart 上传不走 `apiFetch`（后者固定 JSON Content-Type），单独覆盖。

## 落地文件

```
tools/voice-server/README.md          启动方式、模型下载、端口、env 说明
backend/internal/speech/speech.go     接口与类型
backend/internal/speech/openai.go     OpenAICompatibleProvider
backend/internal/speech/config.go     配置（TTS / ASR 各自的 base URL 等）
backend/internal/speech/store.go      音频落盘 + 目录清理
backend/internal/speech/cache.go      合成去重 + singleflight
backend/internal/httpapi/voice_handler.go
backend/internal/store/migrate.go     audio_state 列迁移
backend/internal/interview/           清理钩子（completed / aborted 时触发）
backend/cmd/server/main.go            接线
frontend/src/api/voice.ts
frontend/src/hooks/useVoiceRecorder.ts
frontend/src/utils/wav.ts
frontend/src/components/InterviewChat.tsx  集成
```

## 已知限制

- 本地推理服务需手动启动（`mlx_audio.server`），后端不代管其生命周期。
- 预合成速度受 Mac 实际推理速度限制；创建面试到全部题目就绪之间存在延迟期，此期间点喇叭会走前台同步合成路径。
- 预合成刻意采用逐题串行而非并发合批（见坑 3），因此预合成总时长长于合批，但用户点喇叭的等待上限更低。
- TTS 输出 24kHz WAV，体积约为 24KB/秒，单题数秒到数十秒，缓存占用可接受。
- 追问题目在评分时动态产生，无法预合成，改为评分返回时立即异步合成。
- **面试结束即清理音频**，因此报告页只能看文字与评分，不能回放题目语音或用户录音。
- 音频目录以 interviewID 为清理单元，不跨面试复用同一段题目文本的音频。
- 清理只覆盖"面试状态变终态"这一路径；若进程在清理前崩溃，残留文件由下次启动时的清理扫描兜底（按 `interview_questions.audio_state` 与文件存在性对账）。
