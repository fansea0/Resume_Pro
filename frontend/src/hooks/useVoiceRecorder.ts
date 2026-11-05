import { useCallback, useEffect, useRef, useState } from 'react';

import { transcribeRecording } from '../api/voice';
import { encodeWavPcm16, mixToMono, resample, TARGET_SAMPLE_RATE } from '../utils/wav';

export type RecorderState = 'idle' | 'recording' | 'transcribing';

interface UseVoiceRecorderOptions {
  interviewId: number;
  questionId: number;
  onText: (text: string) => void;
  onError: (message: string) => void;
}

/** 读取 Blob 字节。老浏览器没有 Blob.arrayBuffer，退回 FileReader。 */
async function readBlobBytes(blob: Blob): Promise<ArrayBuffer> {
  if (typeof blob.arrayBuffer === 'function') return blob.arrayBuffer();
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result as ArrayBuffer);
    reader.onerror = () => reject(reader.error ?? new Error('读取录音失败'));
    reader.readAsArrayBuffer(blob);
  });
}

/** 录音 → 16kHz 单声道 WAV → 上传转写。 */
export function useVoiceRecorder({ interviewId, questionId, onText, onError }: UseVoiceRecorderOptions) {
  const [state, setState] = useState<RecorderState>('idle');
  const [seconds, setSeconds] = useState(0);
  const recorderRef = useRef<MediaRecorder | null>(null);
  const streamRef = useRef<MediaStream | null>(null);
  const chunksRef = useRef<Blob[]>([]);
  const timerRef = useRef<number | null>(null);
  const disposedRef = useRef(false);

  const stopTimer = useCallback(() => {
    if (timerRef.current !== null) {
      window.clearInterval(timerRef.current);
      timerRef.current = null;
    }
  }, []);

  // 组件卸载时必须释放麦克风：否则录音指示灯一直亮着，别的页面也拿不到设备。
  useEffect(() => {
    disposedRef.current = false;
    return () => {
      disposedRef.current = true;
      stopTimer();
      const recorder = recorderRef.current;
      recorderRef.current = null;
      if (recorder) {
        // 置空 onstop：卸载后不再触发上传。
        recorder.onstop = null;
        if (recorder.state !== 'inactive') recorder.stop();
      }
      streamRef.current?.getTracks().forEach((track) => track.stop());
      streamRef.current = null;
    };
  }, [stopTimer]);

  const upload = useCallback(async (chunks: Blob[], mimeType: string) => {
    if (disposedRef.current) return;
    setState('transcribing');
    let context: AudioContext | null = null;
    try {
      const blob = new Blob(chunks, { type: mimeType });
      const arrayBuffer = await readBlobBytes(blob);
      // 用 globalThis 取构造器：浏览器里它就是 window，测试环境里也便于替换。
      const scope = globalThis as unknown as {
        AudioContext?: new () => AudioContext;
        webkitAudioContext?: new () => AudioContext;
      };
      const AudioContextClass = scope.AudioContext ?? scope.webkitAudioContext;
      if (!AudioContextClass) throw new Error('浏览器不支持 AudioContext');
      context = new AudioContextClass();
      const decoded = await context.decodeAudioData(arrayBuffer);
      const channels = Array.from({ length: decoded.numberOfChannels }, (_, index) =>
        decoded.getChannelData(index),
      );
      const mono = mixToMono(channels);
      const samples = resample(mono, decoded.sampleRate);
      const wav = new Blob([encodeWavPcm16(samples, TARGET_SAMPLE_RATE)], { type: 'audio/wav' });
      const text = await transcribeRecording(interviewId, questionId, wav);
      if (disposedRef.current) return;
      if (text.trim()) onText(text);
      else onError('没听清，可重新录音或打字');
    } catch {
      if (!disposedRef.current) onError('识别失败，可继续打字');
    } finally {
      if (context) void context.close();
      if (!disposedRef.current) setState('idle');
    }
  }, [interviewId, questionId, onText, onError]);

  const start = useCallback(async () => {
    if (state !== 'idle') return;
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      if (disposedRef.current) {
        stream.getTracks().forEach((track) => track.stop());
        return;
      }
      const recorder = new MediaRecorder(stream);
      chunksRef.current = [];
      recorder.ondataavailable = (event) => {
        if (event.data.size > 0) chunksRef.current.push(event.data);
      };
      recorder.onstop = () => {
        stream.getTracks().forEach((track) => track.stop());
        streamRef.current = null;
        void upload(chunksRef.current, recorder.mimeType || 'audio/webm');
      };
      recorderRef.current = recorder;
      streamRef.current = stream;
      recorder.start();
      setState('recording');
      setSeconds(0);
      timerRef.current = window.setInterval(() => setSeconds((value) => value + 1), 1000);
    } catch {
      onError('无法使用麦克风，可继续打字');
      setState('idle');
    }
  }, [state, onError, upload]);

  const stop = useCallback(() => {
    stopTimer();
    const recorder = recorderRef.current;
    if (recorder && recorder.state !== 'inactive') recorder.stop();
  }, [stopTimer]);

  return { state, seconds, start, stop };
}
