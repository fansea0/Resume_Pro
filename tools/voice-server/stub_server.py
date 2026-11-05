#!/usr/bin/env python3
"""OpenAI 兼容的语音推理桩服务（联调用）。

注意：这是**联调桩**，不是真实识别，也不是真实语音合成。
- TTS 返回一段固定音调（440Hz）的 16kHz 单声道 PCM16 WAV，只用于打通链路；
- ASR 的识别结果**固定**为 "这是桩服务返回的识别结果"，不做任何真实识别。

用途：让没有 GPU / 没装 mlx-audio 的人也能手动联调整条语音链路
（题目语音后台生成 → 播放接口 → 录音转写）。

零依赖：只用 Python 标准库。

    python3 tools/voice-server/stub_server.py --port 21100
"""

from __future__ import annotations

import argparse
import array
import io
import json
import math
import sys
import wave
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

# 桩 ASR 的固定识别结果。
STUB_ASR_TEXT = "这是桩服务返回的识别结果"

# TTS 桩的默认参数：16kHz 单声道 PCM16。
DEFAULT_SAMPLE_RATE = 16000
DEFAULT_DURATION_MS = 800
DEFAULT_FREQUENCY = 440.0
DEFAULT_VOLUME = 8000


def build_wav(
    duration_ms: int = DEFAULT_DURATION_MS,
    sample_rate: int = DEFAULT_SAMPLE_RATE,
    frequency: float = DEFAULT_FREQUENCY,
    volume: int = DEFAULT_VOLUME,
) -> bytes:
    """生成一段合法的 16kHz 单声道 PCM16 WAV。

    用 wave 模块写头，保证 RIFF/WAVE magic、fmt 块与 data 块长度自洽，
    能被后端的 WAV 校验与 Python 的 wave 模块同时读回。
    """
    frame_count = max(1, int(sample_rate * duration_ms / 1000))
    samples = array.array("h")
    for index in range(frame_count):
        value = int(volume * math.sin(2 * math.pi * frequency * index / sample_rate))
        samples.append(value)
    if sys.byteorder != "little":
        samples.byteswap()

    buffer = io.BytesIO()
    with wave.open(buffer, "wb") as handle:
        handle.setnchannels(1)
        handle.setsampwidth(2)
        handle.setframerate(sample_rate)
        handle.writeframes(samples.tobytes())
    return buffer.getvalue()


class VoiceStubHandler(BaseHTTPRequestHandler):
    """实现 /v1/audio/speech 与 /v1/audio/transcriptions 两个端点。"""

    protocol_version = "HTTP/1.1"
    duration_ms = DEFAULT_DURATION_MS

    def log_message(self, fmt: str, *args) -> None:
        sys.stderr.write("[voice-stub] %s %s\n" % (self.address_string(), fmt % args))

    def _read_body(self) -> bytes:
        try:
            length = int(self.headers.get("Content-Length") or 0)
        except ValueError:
            length = 0
        if length <= 0:
            return b""
        return self.rfile.read(length)

    def _respond(self, status: int, content_type: str, payload: bytes) -> None:
        self.send_response(status)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)

    def do_POST(self) -> None:
        # 请求体一律读掉：不读会让 HTTP/1.1 的长连接在下一个请求上错位。
        self._read_body()
        path = self.path.split("?", 1)[0].rstrip("/")
        if path == "/v1/audio/speech":
            self._respond(200, "audio/wav", build_wav(self.duration_ms))
            return
        if path == "/v1/audio/transcriptions":
            body = json.dumps({"text": STUB_ASR_TEXT}, ensure_ascii=False).encode("utf-8")
            self._respond(200, "application/json", body)
            return
        self._respond(404, "application/json", b'{"error":"unknown path"}')

    def do_GET(self) -> None:
        path = self.path.split("?", 1)[0].rstrip("/")
        if path in ("/health", "/v1/models"):
            self._respond(200, "application/json", b'{"status":"ok","stub":true}')
            return
        self._respond(404, "application/json", b'{"error":"unknown path"}')


def print_hint(host: str, port: int) -> None:
    base = "http://%s:%d" % (host, port)
    print("[voice-stub] 桩服务已启动：%s" % base)
    print(
        "[voice-stub] 验证 TTS：curl -s -X POST %s/v1/audio/speech "
        "-d 'model=stub&input=你好&response_format=wav' -o /tmp/stub.wav && "
        "python3 -c \"import wave;w=wave.open('/tmp/stub.wav');"
        "print(w.getnchannels(),w.getframerate(),w.getsampwidth())\"" % base
    )
    print(
        "[voice-stub] 验证 ASR：curl -s -X POST %s/v1/audio/transcriptions -F 'file=@/tmp/stub.wav'" % base
    )
    print(
        "[voice-stub] 后端环境变量：VOICE_ENABLED=true "
        "VOICE_TTS_BASE_URL=%s/v1 VOICE_ASR_BASE_URL=%s/v1 VOICE_CACHE_DIR=./uploads/voice" % (base, base)
    )
    print("[voice-stub] 提醒：这是联调桩，识别结果固定，不是真实识别。")
    # stdout 非 TTY 时是块缓冲的：显式 flush，保证提示行立刻可见（进程被 kill 也不丢）。
    sys.stdout.flush()


def main() -> int:
    parser = argparse.ArgumentParser(description="OpenAI 兼容语音桩服务（联调用，非真实识别）")
    parser.add_argument("--host", default="127.0.0.1", help="监听地址，默认 127.0.0.1")
    parser.add_argument("--port", type=int, default=21100, help="监听端口，默认 21100")
    parser.add_argument(
        "--duration-ms",
        type=int,
        default=DEFAULT_DURATION_MS,
        help="TTS 桩音频时长（毫秒），默认 %d" % DEFAULT_DURATION_MS,
    )
    args = parser.parse_args()

    VoiceStubHandler.duration_ms = args.duration_ms
    server = ThreadingHTTPServer((args.host, args.port), VoiceStubHandler)
    server.daemon_threads = True
    print_hint(args.host, args.port)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        print("\n[voice-stub] 已停止")
    finally:
        server.server_close()
    return 0


if __name__ == "__main__":
    sys.exit(main())
