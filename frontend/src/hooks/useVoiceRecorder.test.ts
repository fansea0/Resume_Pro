import { act, renderHook, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { useVoiceRecorder } from './useVoiceRecorder';

const transcribeRecording = vi.fn();

vi.mock('../api/voice', () => ({
  transcribeRecording: (...args: unknown[]) => transcribeRecording(...args),
}));

class FakeTrack {
  stopped = false;

  stop() {
    this.stopped = true;
  }
}

class FakeStream {
  tracks = [new FakeTrack()];

  getTracks() {
    return this.tracks;
  }
}

class FakeMediaRecorder {
  static instances: FakeMediaRecorder[] = [];

  state: 'inactive' | 'recording' | 'paused' = 'inactive';
  mimeType = 'audio/webm';
  ondataavailable: ((event: { data: Blob }) => void) | null = null;
  onstop: (() => void) | null = null;

  constructor(public stream: FakeStream) {
    FakeMediaRecorder.instances.push(this);
  }

  start() {
    this.state = 'recording';
  }

  stop() {
    this.state = 'inactive';
    this.ondataavailable?.({ data: new Blob(['fake-webm'], { type: 'audio/webm' }) });
    this.onstop?.();
  }
}

class FakeAudioContext {
  async decodeAudioData() {
    return {
      numberOfChannels: 2,
      sampleRate: 48000,
      getChannelData: (index: number) => new Float32Array(480).fill(index === 0 ? 0.5 : -0.5),
    };
  }

  async close() {}
}

/** 读取 Blob 字节（jsdom 的 Blob 没有 arrayBuffer，只能走 FileReader）。 */
async function readBytes(blob: Blob): Promise<Uint8Array> {
  if (typeof blob.arrayBuffer === 'function') return new Uint8Array(await blob.arrayBuffer());
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(new Uint8Array(reader.result as ArrayBuffer));
    reader.onerror = () => reject(reader.error ?? new Error('读取失败'));
    reader.readAsArrayBuffer(blob);
  });
}

/** 装一套假的浏览器录音环境，返回可断言的麦克风流。 */
function stubBrowser() {
  const stream = new FakeStream();
  vi.stubGlobal('navigator', {
    mediaDevices: { getUserMedia: vi.fn().mockResolvedValue(stream) },
  });
  vi.stubGlobal('MediaRecorder', FakeMediaRecorder);
  vi.stubGlobal('AudioContext', FakeAudioContext);
  return stream;
}

afterEach(() => {
  vi.unstubAllGlobals();
  vi.clearAllMocks();
  vi.restoreAllMocks();
  FakeMediaRecorder.instances = [];
});

describe('useVoiceRecorder', () => {
  it('reports a permission error without throwing', async () => {
    const onError = vi.fn();
    vi.stubGlobal('navigator', {
      mediaDevices: {
        getUserMedia: vi.fn().mockRejectedValue(new Error('denied')),
      },
    });

    const { result } = renderHook(() =>
      useVoiceRecorder({ interviewId: 1, questionId: 2, onText: vi.fn(), onError }),
    );

    await act(async () => {
      await result.current.start();
    });

    expect(onError).toHaveBeenCalled();
    expect(result.current.state).toBe('idle');
  });

  it('uploads a 16k mono wav and reports the recognised text', async () => {
    const stream = stubBrowser();
    transcribeRecording.mockResolvedValue('识别文本');
    const onText = vi.fn();

    const { result } = renderHook(() =>
      useVoiceRecorder({ interviewId: 1, questionId: 2, onText, onError: vi.fn() }),
    );

    await act(async () => {
      await result.current.start();
    });
    expect(result.current.state).toBe('recording');

    await act(async () => {
      result.current.stop();
    });

    await waitFor(() => expect(transcribeRecording).toHaveBeenCalledTimes(1));
    const [interviewId, questionId, blob] = transcribeRecording.mock.calls[0];
    expect(interviewId).toBe(1);
    expect(questionId).toBe(2);
    expect(blob.type).toBe('audio/wav');
    const bytes = await readBytes(blob);
    const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
    expect(String.fromCharCode(...bytes.slice(0, 4))).toBe('RIFF');
    expect(view.getUint16(22, true)).toBe(1); // 单声道
    expect(view.getUint32(24, true)).toBe(16000); // 16kHz

    await waitFor(() => expect(onText).toHaveBeenCalledWith('识别文本'));
    // 停止录音后必须释放麦克风轨道。
    expect(stream.tracks[0].stopped).toBe(true);
    await waitFor(() => expect(result.current.state).toBe('idle'));
  });

  it('asks the user to retype when nothing was recognised', async () => {
    stubBrowser();
    transcribeRecording.mockResolvedValue('   ');
    const onText = vi.fn();
    const onError = vi.fn();

    const { result } = renderHook(() =>
      useVoiceRecorder({ interviewId: 1, questionId: 2, onText, onError }),
    );

    await act(async () => {
      await result.current.start();
    });
    await act(async () => {
      result.current.stop();
    });

    await waitFor(() => expect(onError).toHaveBeenCalledWith('没听清，可重新录音或打字'));
    expect(onText).not.toHaveBeenCalled();
  });

  it('reports a transcription failure without breaking the page', async () => {
    stubBrowser();
    transcribeRecording.mockRejectedValue(new Error('boom'));
    const onError = vi.fn();

    const { result } = renderHook(() =>
      useVoiceRecorder({ interviewId: 1, questionId: 2, onText: vi.fn(), onError }),
    );

    await act(async () => {
      await result.current.start();
    });
    await act(async () => {
      result.current.stop();
    });

    await waitFor(() => expect(onError).toHaveBeenCalledWith('识别失败，可继续打字'));
    await waitFor(() => expect(result.current.state).toBe('idle'));
  });

  it('releases the microphone when the component unmounts', async () => {
    const stream = stubBrowser();
    const { result, unmount } = renderHook(() =>
      useVoiceRecorder({ interviewId: 1, questionId: 2, onText: vi.fn(), onError: vi.fn() }),
    );

    await act(async () => {
      await result.current.start();
    });
    unmount();

    expect(stream.tracks[0].stopped).toBe(true);
    expect(transcribeRecording).not.toHaveBeenCalled();
  });
});
