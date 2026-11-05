import { afterEach, describe, expect, it, vi } from 'vitest';

import { fetchQuestionAudio, transcribeRecording } from './voice';

afterEach(() => vi.unstubAllGlobals());

describe('voice API', () => {
  it('fetches question audio as a blob', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(new Blob(['RIFF']), { status: 200, headers: { 'Content-Type': 'audio/wav' } }),
    );
    vi.stubGlobal('fetch', fetchMock);

    const blob = await fetchQuestionAudio(3, 12);

    expect(blob.size).toBeGreaterThan(0);
    expect(fetchMock).toHaveBeenCalledWith('/api/interviews/3/questions/12/audio',
      expect.objectContaining({ method: 'GET', credentials: 'include' }));
  });

  it('rejects with the pending state when audio is not ready', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ state: 'pending' }), { status: 409 }),
    );
    vi.stubGlobal('fetch', fetchMock);

    await expect(fetchQuestionAudio(3, 12)).rejects.toMatchObject({ status: 409 });
  });

  it('reports the disabled state when voice is not enabled', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ error: '语音未启用' }), { status: 503 }),
    );
    vi.stubGlobal('fetch', fetchMock);

    await expect(fetchQuestionAudio(3, 12)).rejects.toMatchObject({ status: 503 });
  });

  it('uploads the recording as multipart without a json content type', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ text: '识别文本' }), { status: 200 }),
    );
    vi.stubGlobal('fetch', fetchMock);

    const text = await transcribeRecording(3, 12, new Blob(['RIFF'], { type: 'audio/wav' }));

    expect(text).toBe('识别文本');
    const [, init] = fetchMock.mock.calls[0];
    expect(init.body).toBeInstanceOf(FormData);
    // multipart 的 Content-Type 必须由浏览器带 boundary，接口层不能自己设。
    expect(init.headers?.['Content-Type']).toBeUndefined();
  });
});
