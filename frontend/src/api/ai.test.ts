import { afterEach, describe, expect, it, vi } from 'vitest';

import type { SSEEvent } from '../types/ai';
import { streamRewrite } from './ai';

const encoder = new TextEncoder();

function streamedResponse(source: string, splitAt: number[] = []): Response {
  const encoded = encoder.encode(source);
  const boundaries = [...splitAt, encoded.length];
  let start = 0;
  const body = new ReadableStream<Uint8Array>({
    start(controller) {
      for (const end of boundaries) {
        controller.enqueue(encoded.slice(start, end));
        start = end;
      }
      controller.close();
    },
  });
  return new Response(body, {
    status: 200,
    headers: { 'Content-Type': 'text/event-stream' },
  });
}

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe('streamRewrite', () => {
  it('posts the stable section identity once and parses chunked CRLF SSE frames', async () => {
    const source = [
      ': heartbeat\r\n',
      'retry: 1000\r\n',
      'event:thinking\r\n',
      'data: {"text":\r\n',
      'data: "先分析岗位"}\r\n',
      '\r\n',
      'event: suggestion\r\n',
      'data: {"kind":"untrusted","sectionId":"section-42","sectionType":"project","newContent":{"items":[]}}\r\n',
      '\r\n',
      'event:done\r\n',
      'data: {}\r\n',
      '\r\n',
    ].join('');
    const unicodeByte = encoder.encode(source).findIndex((byte) => byte > 0x7f);
    const fetchMock = vi.fn().mockResolvedValue(streamedResponse(source, [1, 12, unicodeByte + 1, unicodeByte + 2, source.length - 3]));
    vi.stubGlobal('fetch', fetchMock);
    const events: SSEEvent[] = [];
    const request = {
      sectionId: 'section-42',
      sectionType: 'project' as const,
      instructions: '突出成果',
    };

    await streamRewrite(7, request, (event) => events.push(event));

    expect(fetchMock).toHaveBeenCalledOnce();
    expect(fetchMock).toHaveBeenCalledWith('/api/ai/resumes/7/section/rewrite', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(request),
      credentials: 'include',
      signal: undefined,
    });
    expect(events).toEqual([
      { kind: 'thinking', text: '先分析岗位' },
      { kind: 'suggestion', sectionId: 'section-42', sectionType: 'project', newContent: { items: [] } },
      { kind: 'done' },
    ]);
  });

  it('dispatches a backend SSE error followed by done without treating HTTP 200 as a request failure', async () => {
    const source = [
      'event: error\n',
      'data: {"code":"VALIDATION_FAILED","message":"AI 输出无效"}\n\n',
      'event: done\n',
      'data: {}\n\n',
    ].join('');
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(streamedResponse(source)));
    const onEvent = vi.fn();

    await expect(streamRewrite(7, { sectionId: 'summary', sectionType: 'summary' }, onEvent)).resolves.toBeUndefined();

    expect(onEvent.mock.calls.map(([event]) => event)).toEqual([
      { kind: 'error', code: 'VALIDATION_FAILED', message: 'AI 输出无效' },
      { kind: 'done' },
    ]);
  });

  it('uses the backend JSON error for a pre-stream 503 response', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(
      JSON.stringify({ error: 'AI features disabled' }),
      { status: 503, headers: { 'Content-Type': 'application/json' } },
    ));
    vi.stubGlobal('fetch', fetchMock);
    const onEvent = vi.fn();

    await expect(streamRewrite(7, { sectionId: 'summary', sectionType: 'summary' }, onEvent)).rejects.toMatchObject({
      name: 'ApiError',
      message: 'AI features disabled',
      status: 503,
    });
    expect(onEvent).not.toHaveBeenCalled();
    expect(fetchMock).toHaveBeenCalledOnce();
  });

  it('ignores malformed and unknown events while continuing with later valid frames', async () => {
    const source = [
      'event: thinking\n',
      'data: not-json\n\n',
      'event: heartbeat\n',
      'data: {}\n\n',
      'event: done\n',
      'data: {}',
    ].join('');
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(streamedResponse(source, [17, 31])));
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => undefined);
    const onEvent = vi.fn();

    await streamRewrite(7, { sectionId: 'summary', sectionType: 'summary' }, onEvent);

    expect(consoleError).toHaveBeenCalledOnce();
    expect(onEvent).toHaveBeenCalledOnce();
    expect(onEvent).toHaveBeenCalledWith({ kind: 'done' });
  });

  it('forwards aborts and never retries the rewrite POST', async () => {
    const controller = new AbortController();
    controller.abort();
    const abortError = new DOMException('The operation was aborted.', 'AbortError');
    const fetchMock = vi.fn().mockImplementation((_path: string, init: RequestInit) => {
      expect(init.signal).toBe(controller.signal);
      return Promise.reject(abortError);
    });
    vi.stubGlobal('fetch', fetchMock);

    await expect(streamRewrite(
      7,
      { sectionId: 'summary', sectionType: 'summary' },
      vi.fn(),
      controller.signal,
    )).rejects.toBe(abortError);
    expect(fetchMock).toHaveBeenCalledOnce();
  });

  it('propagates response stream failures without retrying the rewrite POST', async () => {
    const readError = new Error('connection reset');
    const body = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.error(readError);
      },
    });
    const fetchMock = vi.fn().mockResolvedValue(new Response(body, { status: 200 }));
    vi.stubGlobal('fetch', fetchMock);

    await expect(streamRewrite(
      7,
      { sectionId: 'summary', sectionType: 'summary' },
      vi.fn(),
    )).rejects.toBe(readError);
    expect(fetchMock).toHaveBeenCalledOnce();
  });

  it('rejects a successful response that has no readable body', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 200 }));
    vi.stubGlobal('fetch', fetchMock);

    await expect(streamRewrite(
      7,
      { sectionId: 'summary', sectionType: 'summary' },
      vi.fn(),
    )).rejects.toThrow('AI response did not include an event stream');
    expect(fetchMock).toHaveBeenCalledOnce();
  });
});
