# 语音推理服务（mlx-audio）操作手册

本目录是 AI 模拟面试语音能力的**推理侧**工具：

| 文件 | 用途 |
| --- | --- |
| `README.md` | 本手册：启动真实推理服务、后端环境变量、端到端验证清单、常见问题 |
| `stub_server.py` | 零依赖联调桩：没有 GPU / 没装 mlx-audio 时用它打通链路 |

后端不内置 TTS / ASR，而是以 OpenAI 兼容协议访问外部服务。本地默认指向
`http://127.0.0.1:21100/v1`（端口 21100 是刻意避开常用段的，本机项目多、端口常被占）。

---

## 一、启动真实推理服务（mlx-audio）

### 安装（mlx-audio 0.6.4 / Apple Silicon）

`pip install mlx-audio` **不足以启动 server**：mlx-audio 0.6.4 没把自己的运行时依赖
（`uvicorn` / `fastapi` / `python-multipart` / `webrtcvad`）写进依赖表，缺一个就直接
`ModuleNotFoundError`。完整装法：

```bash
# 1) 独立 venv：系统 Python 与 Homebrew Python 都被 PEP 668 保护，不能直接装
uv venv --python 3.12 ~/.venvs/mlx-audio

# 2) 装 mlx-audio 与 server 缺的运行时依赖
UV_DEFAULT_INDEX=https://pypi.tuna.tsinghua.edu.cn/simple \
uv pip install --python ~/.venvs/mlx-audio/bin/python \
  mlx-audio uvicorn fastapi python-multipart webrtcvad 'setuptools<81'

# 3) 验证 GPU 可用（应为 True）
~/.venvs/mlx-audio/bin/python -c "import mlx.core as mx; print(mx.metal.is_available())"
```

> `setuptools<81` 是硬约束：`webrtcvad` 还在 `import pkg_resources`，而 setuptools 81+
> 已经把它删了。装成新版会在启动时抛 `No module named 'pkg_resources'`。

启动：

```bash
~/.venvs/mlx-audio/bin/mlx_audio.server --host 127.0.0.1 --port 21100
```

### 模型

首次请求会自动下载（合计约 3.5GB），下载完才有响应：

- TTS：`mlx-community/Qwen3-TTS-12Hz-0.6B-CustomVoice-8bit`
- ASR：`mlx-community/Qwen3-ASR-1.7B-8bit`（`Qwen3-ASR-0.6B-8bit` 更小，约 1.0GB，识别精度略低）

TTS 与 ASR 各自独立配置，可以分别指向不同服务（本地 mlx-audio 与云端 API 混用也支持）。

### 模型下载：国内网络会卡死，必须绕一下

本网络下 **HF 上单个大 safetensors 会下到 0 字节就卡死**（xet 与普通 HTTP 都一样），
而同一仓库里的小文件正常。ASR 1.7B 的 `model.safetensors` 是 2.46GB，必卡。

绕法（实测有效，1.0GB 约 6 分钟）：

1. 用 `hf-mirror` + range 请求分片并行下载：

   ```bash
   export URL='https://hf-mirror.com/mlx-community/Qwen3-ASR-0.6B-8bit/resolve/main/model.safetensors'
   # 按 16MB 一片、16 并发下载到 /tmp/asr_parts/part_N
   # 每片用 curl -r <start>-<end>
   ```

2. **按数字序**合并（`cat part_*` 是字典序，`part_10` 会排在 `part_2` 前面，合并后校验必然失败）：

   ```bash
   for i in $(seq 0 59); do cat /tmp/asr_parts/part_$i; done > /tmp/model.safetensors
   echo '<sha256>  /tmp/model.safetensors' | shasum -a 256 -c -
   ```

3. 放进 HF 缓存并建符号链接（blob 文件名就是 sha256，snapshot 目录里要软链过去）：

   ```bash
   B=~/.cache/huggingface/hub/models--mlx-community--Qwen3-ASR-0.6B-8bit
   cp /tmp/model.safetensors $B/blobs/<sha256>
   ln -sf ../../blobs/<sha256> $B/snapshots/<commit>/model.safetensors
   ```

4. 模型已经在本地时，用**离线模式**启动，避免服务端再次尝试联网拉取而卡住：

   ```bash
   HF_HUB_OFFLINE=1 ~/.venvs/mlx-audio/bin/mlx_audio.server --host 127.0.0.1 --port 21100
   ```

### 单独验证服务

> **请求体格式**：mlx-audio 0.6.4 的 `/v1/audio/speech` 只吃 **JSON body**（FastAPI 的
> `SpeechRequest`），发 `application/x-www-form-urlencoded` 会直接 **422**。
> `/v1/audio/transcriptions` 是 **multipart**（`file` + `model` + `response_format`），与后端一致。

```bash
# TTS：应产出一个 WAV 文件
curl -s -X POST http://127.0.0.1:21100/v1/audio/speech \
  -H 'Content-Type: application/json' \
  -d '{"model":"mlx-community/Qwen3-TTS-12Hz-0.6B-CustomVoice-8bit","input":"你好","response_format":"wav","voice":"Vivian"}' \
  -o /tmp/tts.wav && file /tmp/tts.wav

# ASR：应返回 {"text":"..."}
curl -s -X POST http://127.0.0.1:21100/v1/audio/transcriptions \
  -F 'file=@/tmp/tts.wav' \
  -F 'model=mlx-community/Qwen3-ASR-0.6B-8bit' \
  -F 'response_format=json'
```

实测产物：`file` 显示 `RIFF (little-endian) data, WAVE audio, Microsoft PCM, 16 bit, mono 24000 Hz`
（**24kHz**，不是 16kHz；后端 `ValidateWAV` 不硬卡采样率，能正常放行）。

---

## 二、后端环境变量

全部通过环境变量或 `.env` 文件配置。后端启动时会自动加载工作目录下的 `.env`；
工作目录没有就找上一级目录的 `.env`（只加载第一个找到的）。
已经在环境里显式 export 的变量优先于文件里的值。

| 变量 | 默认值 | 含义 |
| --- | --- | --- |
| `VOICE_ENABLED` | `false` | 语音总开关。**默认关闭**（opt-in）：没装 mlx-audio 的部署启动即报错太吵。关着时语音路由仍然注册，接口返回 503，面试照常 |
| `VOICE_TTS_BASE_URL` | `http://127.0.0.1:21100/v1` | TTS 服务基址。后端请求 `POST {base}/audio/speech` |
| `VOICE_TTS_API_KEY` | 空 | TTS 鉴权头（`Authorization: Bearer ...`）。本地 mlx-audio 不用填，云端 API 才需要 |
| `VOICE_TTS_MODEL` | `mlx-community/Qwen3-TTS-12Hz-0.6B-CustomVoice-8bit` | TTS 模型名，原样透传给上游 |
| `VOICE_TTS_VOICE` | `Vivian` | 发音人（voice）。空字符串表示不传该字段 |
| `VOICE_TTS_TIMEOUT` | `120s` | 单次 TTS 请求超时。真实推理可能几十秒，别调太小 |
| `VOICE_ASR_BASE_URL` | `http://127.0.0.1:21100/v1` | ASR 服务基址。后端请求 `POST {base}/audio/transcriptions` |
| `VOICE_ASR_API_KEY` | 空 | ASR 鉴权头 |
| `VOICE_ASR_MODEL` | `mlx-community/Qwen3-ASR-1.7B-8bit` | ASR 模型名，原样透传给上游 |
| `VOICE_ASR_TIMEOUT` | `60s` | 单次 ASR 请求超时 |
| `VOICE_CORRECT_TIMEOUT` | `30s` | 错别字修复（LLM 校对）的单次调用超时 |
| `VOICE_GENERATOR_MAX_ATTEMPTS` | `3` | 单题语音生成的尝试次数（含首次）。用尽仍失败 → `audioState=failed` |
| `VOICE_GENERATOR_BACKOFF` | `1s,3s,10s` | 重试退避序列（逗号分隔）。非法值整段回落到默认值 |
| `VOICE_CACHE_DIR` | `./uploads/voice` | 题目音频落盘根目录。**相对路径按后端进程的工作目录解析**（在 `backend/` 下启动就是 `backend/uploads/voice`）。目录结构：`{root}/{tenantID}/{interviewID}/q{questionID}.wav` |

最小可用配置：

```bash
VOICE_ENABLED=true
VOICE_TTS_BASE_URL=http://127.0.0.1:21100/v1
VOICE_ASR_BASE_URL=http://127.0.0.1:21100/v1
VOICE_CACHE_DIR=./uploads/voice
```

> 面试功能本身还依赖 **Redis**（checkpoint）与真实 LLM key（`OPENAI_API_KEY`），
> 与语音开关无关：`REDIS_ADDR` 未配置或连不上时，面试接口整体返回 503。

### 服务未启动时的行为（设计约束，不是故障）

推理服务没起 / 端口写错 / 模型没下载完时：

1. 后台生成器按 `VOICE_GENERATOR_MAX_ATTEMPTS` 重试，耗尽后该题 `audioState=failed`；
2. 播放接口对该题返回 `503 {"state":"failed"}`；
3. 前端只在 `audioState === 'ready'` 时显示喇叭按钮 → **不显示喇叭**；
4. **文字面试完全不受影响**：建面试、答题、评分、总评一切照旧。

也就是说：语音是附加层，挂了只影响语音。

---

## 三、端到端人工验证清单

按顺序做，每一步都要实际跑过。标注说明依赖：
**【LLM】** = 需要真实 LLM key；**【浏览器】** = 需要浏览器；**【GPU】** = 需要 mlx-audio。

| # | 步骤 | 期望 | 依赖 |
| --- | --- | --- | --- |
| ① | `cd backend && go test ./... -count=1` | 全绿 | 无 |
| ② | `cd frontend && npx vitest run` | 全绿 | 无 |
| ③ | 起 mlx-audio（见第一节） | 端口 21100 可访问 | 【GPU】 |
| ④ | `VOICE_ENABLED=true` 起后端（Redis 与 LLM key 也要就绪） | 日志出现 `voice features enabled` | 【LLM】 |
| ⑤ | 建一个面试 | `POST /api/interviews` 返回 201；随后 `GET /api/interviews/:id/questions` 里 `audioState` 依次变 `ready` | 【LLM】【GPU】 |
| ⑥ | `GET /api/interviews/:id/questions/:qid/audio` | 200，`Content-Type: audio/wav`，响应体是合法 WAV | 无 |
| ⑦ | 进面试页 | 当前题自动朗读；被浏览器拦截时点喇叭可播 | 【浏览器】 |
| ⑧ | 录音回答 | 停止后识别文本插入输入框光标处，且错别字被修正 | 【浏览器】【LLM】【GPU】 |
| ⑨ | 停掉 mlx-audio 再答题 | `answer` 仍返回 200，题面与评分正常；该题 `audioState` 变 `failed`，前端不显示喇叭 | 【LLM】 |
| ⑩ | 重启后端 | 进行中的面试语音仍可播（已 ready 的文件还在）；未生成的题目被补齐 | 【LLM】【GPU】 |

### 没有 GPU 时怎么验

用本目录的零依赖桩服务替代 mlx-audio，只验证链路，不验证识别质量：

```bash
python3 tools/voice-server/stub_server.py --port 21100
# 另开一个终端
export VOICE_ENABLED=true
export VOICE_TTS_BASE_URL=http://127.0.0.1:21100/v1
export VOICE_ASR_BASE_URL=http://127.0.0.1:21100/v1
cd backend && go run ./cmd/server
```

桩服务的验证：

```bash
curl -s -X POST http://127.0.0.1:21100/v1/audio/speech \
  -d 'model=stub&input=你好&response_format=wav' -o /tmp/stub.wav &&
python3 -c "import wave;w=wave.open('/tmp/stub.wav');print(w.getnchannels(),w.getframerate(),w.getsampwidth())"
# 期望输出：1 16000 2
```

> 桩服务**不是真实识别**：ASR 结果固定为「这是桩服务返回的识别结果」，TTS 返回一段固定音调。

---

## 四、常见问题

**题目语音一直不生成（`audioState` 停在 `pending`）**

- 后端是不是用 `VOICE_ENABLED=true` 起的？没开就不会建生成器，题目根本不入队（`audioState` 会是 `null` 而不是 `pending`）。
- 生成 worker 卡在上一题：串行 worker，一题要等到上一题合成完（真实 TTS 单题可能几十秒）。看后端日志里 `voice: question audio generation failed` 的时间点。
- 进程上次崩溃留下的 `pending` 残留：**重启后端会自动归零并补齐**（启动阶段 `Reload`）。若仍卡住，检查是否有别的进程占着同一个 `VOICE_CACHE_DIR`。
- 队列积压：看日志里 `generation queue and backlog full, skip`。

**`audioState` 变成 `failed`**

- 重试次数用尽。看后端日志 `voice: question audio generation failed` 后面的错误：
  `connection refused` = 服务没起 / 端口不对；`tts status 404` = base URL 少了 `/v1` 或路径不对；
  模型没下载完时上游可能直接 500。
- 修好上游后不会自动重试该题：`Reload` 只在启动时跑一次。**重启后端**即可补齐（`failed` 的题也会被重新入队）。

**播放接口返回 `409`（未就绪）**

- 正常现象：语音还在后台生成中。`409 {"state":"pending"}` 表示「稍后重试」。
- 一直 409 说明生成器没在跑（见上面第一条）。

**播放接口返回 `503`**

- 两种含义，看响应体：
  - `{"error":"voice features disabled"}` → **语音未启用**（`VOICE_ENABLED` 没开，或装配失败）。路由仍然注册，这不是「接口不存在」。
  - `{"state":"failed"}` → 这题的语音生成失败。
- 前提是整条链路都返回 503（播放与转写都是），说明 `VoiceService` 为 nil。

**转写接口返回 `503 {"error":"transcription unavailable"}`**

- ASR 上游不可用或超时（看 `VOICE_ASR_TIMEOUT`）。录音不会落盘，也不会被回放。

**转写接口返回 `400`**

- 上传的音频不是合法 WAV（前端产出的是 16kHz 单声道 PCM16）。后端只校验不转码。

**音频文件没被清理**

- 面试结束（答完最后一题）或 Abort 时会删掉整场面试的音频目录。若目录还在，看日志里
  `reset interview audio state failed` —— 状态归零失败时后端**故意不删目录**（避免出现「状态 ready 但文件没了」）。
