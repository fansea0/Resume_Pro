/** 题目语音状态：pending=生成中，ready=已就绪，failed=生成失败。 */
export type AudioState = 'pending' | 'ready' | 'failed';

/**
 * 语音接口错误。带状态码：409=还没生成好，503=语音没启用。
 * 这里不继承 api/client 的 ApiError：那份模块在部分测试里是整体 mock 的，
 * 独立定义可以避免语音接口被无关的 mock 牵连。
 */
export class VoiceApiError extends Error {
  constructor(message: string, public readonly status: number) {
    super(message);
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
