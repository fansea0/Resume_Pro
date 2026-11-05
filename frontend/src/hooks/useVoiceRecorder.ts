import { useCallback, useEffect, useRef, useState } from 'react';

import { transcribeRecording } from '../api/voice';
import { encodeWavPcm16, mixToMono, resample, TARGET_SAMPLE_RATE } from '../utils/wav';

export type RecorderState = 'idle' | 'starting' | 'recording' | 'transcribing';

interface UseVoiceRecorderOptions {
  interviewId: number;
  questionId: number;
  onText: (text: string) => void;
  onError: (message: string) => void;
}

/**
 * 录音时长软上限（秒）。
 * 16kHz 单声道 PCM16 约 32KB/秒，10 分钟约 19.2MB，紧贴后端 20MB 上限；
 * 这里卡 5 分钟（约 9.6MB），到点自动停止。
 */
const MAX_RECORDING_SECONDS = 300;

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
  // 连点保护：state 是渲染期快照，await getUserMedia 期间它还是 idle，
  // 只看 state 会让两次 start 各开一路麦克风，第一个 recorder 永远停不下来。
  const startingRef = useRef(false);
  const secondsRef = useRef(0);

  const stopTimer = useCallback(() => {
    if (timerRef.current !== null) {
      window.clearInterval(timerRef.current);
      timerRef.current = null;
    }
  }, []);

  /** 释放麦克风轨道：设备一直被独占的话录音指示灯会一直亮着。 */
  const releaseStream = useCallback(() => {
    streamRef.current?.getTracks().forEach((track) => track.stop());
    streamRef.current = null;
  }, []);

  /** 停掉 recorder 并断开上传回调（异常路径与卸载路径用）。 */
  const releaseRecorder = useCallback(() => {
    const recorder = recorderRef.current;
    recorderRef.current = null;
    if (!recorder) return;
    recorder.onstop = null;
    if (recorder.state !== 'inactive') recorder.stop();
  }, []);

  // 组件卸载时必须释放麦克风：否则录音指示灯一直亮着，别的页面也拿不到设备。
  useEffect(() => {
    disposedRef.current = false;
    return () => {
      disposedRef.current = true;
      startingRef.current = false;
      stopTimer();
      releaseRecorder();
      releaseStream();
    };
  }, [stopTimer, releaseRecorder, releaseStream]);

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
      // 采样为 0 的 WAV 会被后端判为非法（data 长度 0），提前给出更贴切的提示。
      if (samples.length === 0) {
        onError('录音太短，请重新录音或打字');
        return;
      }
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

  const stop = useCallback(() => {
    stopTimer();
    // 停止后计时归零：下次 start 重新计数。
    secondsRef.current = 0;
    setSeconds(0);
    const recorder = recorderRef.current;
    if (recorder && recorder.state !== 'inactive') recorder.stop();
  }, [stopTimer]);

  const start = useCallback(async () => {
    if (startingRef.current || state !== 'idle') return;
    startingRef.current = true;
    setState('starting');
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      streamRef.current = stream;
      if (disposedRef.current) {
        stream.getTracks().forEach((track) => track.stop());
        streamRef.current = null;
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
      recorder.start();
      setState('recording');
      secondsRef.current = 0;
      setSeconds(0);
      timerRef.current = window.setInterval(() => {
        secondsRef.current += 1;
        setSeconds(secondsRef.current);
        if (secondsRef.current >= MAX_RECORDING_SECONDS) {
          onError(`已达 ${MAX_RECORDING_SECONDS / 60} 分钟录音上限，已自动停止`);
          stop();
        }
      }, 1000);
    } catch {
      // 构造/启动失败同样要释放麦克风，否则设备一直被独占。
      releaseRecorder();
      releaseStream();
      onError('无法使用麦克风，可继续打字');
      setState('idle');
    } finally {
      startingRef.current = false;
    }
  }, [state, onError, upload, stop, releaseRecorder, releaseStream]);

  return { state, seconds, start, stop };
}
