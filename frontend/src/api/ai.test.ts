import { afterEach, describe, expect, it, vi } from 'vitest';

import type { SSEEvent } from '../types/ai';
import { streamGenerate, streamRewrite } from './ai';

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
  it('decodes split progress, wrapped suggestion, and done frames in stream order', async () => {
    const source = [
      'event: progress\n',
      'data: {"stage":"jd_alignment_started","message":"正在对齐目标岗位关键词。"}\n\n',
      'event: suggestion\n',
      'data: {"sectionId":"summary-42","sectionType":"summary","newContent":{"rewrite":{"markdown":"主导产品交付。"},"analysis":{"jdAlignment":{"status":"skipped"},"matchedIssues":[],"evidenceRequests":[],"warnings":[]}}}\n\n',
      'event: done\n',
      'data: {}\n\n',
    ].join('');
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(streamedResponse(source, [7, 39, source.length - 4])));
    const events: SSEEvent[] = [];

    await streamRewrite(7, { sectionId: 'summary-42', sectionType: 'summary' }, (event) => events.push(event));

    expect(events).toEqual([
      { kind: 'progress', stage: 'jd_alignment_started', message: '正在对齐目标岗位关键词。' },
      {
        kind: 'suggestion',
        sectionId: 'summary-42',
        sectionType: 'summary',
        newContent: {
          rewrite: { markdown: '主导产品交付。' },
          analysis: { jdAlignment: { status: 'skipped' }, matchedIssues: [], evidenceRequests: [], warnings: [] },
        },
      },
      { kind: 'done' },
    ]);
  });

  it('posts the stable section identity once and parses chunked CRLF SSE frames', async () => {
    const source = [
      ': heartbeat\r\n',
      'retry: 1000\r\n',
      'event:thinking\r\n',
      'data: {"text":\r\n',
      'data: "先分析岗位"}\r\n',
      '\r\n',
      'event: suggestion\r\n',
      'data: {"kind":"untrusted","sectionId":"section-42","sectionType":"project","newContent":{"rewrite":{"items":[]},"analysis":{"jdAlignment":{"status":"skipped"},"matchedIssues":[],"evidenceRequests":[],"warnings":[]}}}\r\n',
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
      {
        kind: 'suggestion', sectionId: 'section-42', sectionType: 'project', newContent: {
          rewrite: { items: [] },
          analysis: { jdAlignment: { status: 'skipped' }, matchedIssues: [], evidenceRequests: [], warnings: [] },
        },
      },
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

describe('streamGenerate', () => {
  it('posts generation inputs once and parses a streamed resume document', async () => {
    const generated = {
      schemaVersion: 2,
      title: '张三｜后端工程师',
      templateId: 'minimal',
      profile: {
        fullName: '张三', headline: '后端工程师', email: '', phone: '', location: '', website: '',
        avatarUrl: '', avatarStorageKey: '', showAvatar: true,
      },
      sections: [],
    };
    const source = [
      'event: thinking\n',
      'data: {"text":"正在梳理经历"}\n\n',
      'event: suggestion\n',
      `data: ${JSON.stringify({ newContent: generated })}\n\n`,
      'event: done\n',
      'data: {}\n\n',
    ].join('');
    const fetchMock = vi.fn().mockResolvedValue(streamedResponse(source, [23, source.length - 4]));
    vi.stubGlobal('fetch', fetchMock);
    const onEvent = vi.fn();
    const request = {
      rawText: '我有五年 Go 后端经验，负责过支付平台与稳定性治理。',
      targetJobId: 7,
      templateId: 'minimal' as const,
    };

    await streamGenerate(request, onEvent);

    expect(fetchMock).toHaveBeenCalledOnce();
    expect(fetchMock).toHaveBeenCalledWith('/api/ai/resumes/generate', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(request),
      credentials: 'include',
      signal: undefined,
    });
    expect(onEvent.mock.calls.map(([event]) => event)).toEqual([
      { kind: 'thinking', text: '正在梳理经历' },
      { kind: 'suggestion', newContent: generated },
      { kind: 'done' },
    ]);
  });
});
