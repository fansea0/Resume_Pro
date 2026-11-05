import { ApiError } from './client';

/** 题目语音状态：pending=生成中，ready=已就绪，failed=生成失败。 */
export type AudioState = 'pending' | 'ready' | 'failed';

/**
 * 语音接口错误。与仓库既有的 ApiError 同形（带 status），
 * 这样「409 还没生成好 / 503 语音没启用」可以按状态码统一判断。
 */
export class VoiceApiError extends ApiError {
  constructor(message: string, status: number) {
    super(message, status);
    this.name = 'VoiceApiError';
  }
}

/** 取题目语音。服务端返回 409 表示还在生成中。 */
export async function fetchQuestionAudio(interviewId: number, questionId: number): Promise<Blob> {
  const response = await fetch(`/api/interviews/${interviewId}/questions/${questionId}/audio`, {
    method: 'GET',
    credentials: 'include',
  });
  if (!response.ok) {
    const body = await response.json().catch(() => ({}));
    throw new VoiceApiError(body.error ?? '语音暂不可用', response.status);
  }
  return response.blob();
}

/** 上传录音并返回识别文本。multipart 不能设置 Content-Type，交给浏览器带 boundary。 */
export async function transcribeRecording(
  interviewId: number,
  questionId: number,
  recording: Blob,
): Promise<string> {
  const form = new FormData();
  form.append('file', recording, 'recording.wav');
  const response = await fetch(`/api/interviews/${interviewId}/questions/${questionId}/transcribe`, {
    method: 'POST',
    credentials: 'include',
    body: form,
  });
  if (!response.ok) {
    const body = await response.json().catch(() => ({}));
    throw new VoiceApiError(body.error ?? '识别失败，可继续打字', response.status);
  }
  const payload = await response.json();
  return payload.text ?? '';
}
