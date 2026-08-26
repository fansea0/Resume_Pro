import type {
  GenerateRequest,
  GenerateSSEEvent,
  RewriteRequest,
  SSEEvent,
} from '../types/ai';
import { ApiError } from './client';

type SSEEventKind = SSEEvent['kind'];

const supportedEventKinds = new Set<SSEEventKind>([
  'thinking',
  'progress',
  'suggestion',
  'error',
  'done',
]);

export async function streamRewrite(
  resumeId: number,
  request: RewriteRequest,
  onEvent: (event: SSEEvent) => void,
  signal?: AbortSignal,
): Promise<void> {
  // A rewrite is a non-idempotent POST. Deliberately make one attempt and let
  // the caller decide whether an interrupted request is safe to repeat.
  const response = await fetch(`/api/ai/resumes/${resumeId}/section/rewrite`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(request),
    credentials: 'include',
    signal,
  });

  if (!response.ok) {
    throw await responseError(response);
  }
  if (!response.body) {
    throw new Error('AI response did not include an event stream');
  }

  const reader = response.body.getReader();
  const textDecoder = new TextDecoder();
  const eventDecoder = new SSEDecoder(onEvent);

  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) {
        eventDecoder.push(textDecoder.decode(), true);
        return;
      }
      eventDecoder.push(textDecoder.decode(value, { stream: true }));
    }
  } finally {
    reader.releaseLock();
  }
}

export async function streamGenerate(
  request: GenerateRequest,
  onEvent: (event: GenerateSSEEvent) => void,
  signal?: AbortSignal,
): Promise<void> {
  // Generation is also a non-idempotent POST. A retry is always initiated by
  // an explicit user action in the modal.
  const response = await fetch('/api/ai/resumes/generate', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(request),
    credentials: 'include',
    signal,
  });

  if (!response.ok) {
    throw await responseError(response);
  }
  if (!response.body) {
    throw new Error('AI response did not include an event stream');
  }

  const reader = response.body.getReader();
  const textDecoder = new TextDecoder();
  const eventDecoder = new SSEDecoder(onEvent);

  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) {
        eventDecoder.push(textDecoder.decode(), true);
        return;
      }
      eventDecoder.push(textDecoder.decode(value, { stream: true }));
    }
  } finally {
    reader.releaseLock();
  }
}

async function responseError(response: Response): Promise<ApiError> {
  let message = `AI request failed: ${response.status}`;
  try {
    const payload: unknown = await response.json();
    if (
      typeof payload === 'object'
      && payload !== null
      && 'error' in payload
      && typeof payload.error === 'string'
      && payload.error.trim() !== ''
    ) {
      message = payload.error;
    }
  } catch {
    // Some proxies return an empty or non-JSON error body. Preserve a useful
    // status-based error in that case.
  }
  return new ApiError(message, response.status);
}

class SSEDecoder<TEvent extends { kind: SSEEventKind }> {
  private buffer = '';
  private eventName = '';
  private dataLines: string[] = [];

  constructor(private readonly onEvent: (event: TEvent) => void) {}

  push(chunk: string, endOfStream = false): void {
    this.buffer += chunk;

    while (true) {
      const boundary = findLineBoundary(this.buffer, endOfStream);
      if (!boundary) break;
      const line = this.buffer.slice(0, boundary.lineEnd);
      this.buffer = this.buffer.slice(boundary.nextLineStart);
      this.processLine(line);
    }

    if (endOfStream) {
      if (this.buffer !== '') {
        this.processLine(this.buffer);
        this.buffer = '';
      }
      this.dispatch();
    }
  }

  private processLine(line: string): void {
    if (line === '') {
      this.dispatch();
      return;
    }
    if (line.startsWith(':')) return;

    const colon = line.indexOf(':');
    const field = colon < 0 ? line : line.slice(0, colon);
    let value = colon < 0 ? '' : line.slice(colon + 1);
    if (value.startsWith(' ')) value = value.slice(1);

    if (field === 'event') {
      this.eventName = value;
    } else if (field === 'data') {
      this.dataLines.push(value);
    }
  }

  private dispatch(): void {
    const eventName = this.eventName;
    const data = this.dataLines.join('\n');
    this.eventName = '';
    this.dataLines = [];

    if (data === '' || !isSupportedEventKind(eventName)) return;

    try {
      const payload: unknown = JSON.parse(data);
      if (typeof payload !== 'object' || payload === null || Array.isArray(payload)) {
        throw new TypeError('SSE data must be a JSON object');
      }
      this.onEvent({ ...payload, kind: eventName } as TEvent);
    } catch (error) {
      console.error('Failed to parse SSE data:', data, error);
    }
  }
}

function isSupportedEventKind(value: string): value is SSEEventKind {
  return supportedEventKinds.has(value as SSEEventKind);
}

function findLineBoundary(buffer: string, endOfStream: boolean): {
  lineEnd: number;
  nextLineStart: number;
} | undefined {
  for (let index = 0; index < buffer.length; index += 1) {
    const character = buffer[index];
    if (character === '\n') {
      return { lineEnd: index, nextLineStart: index + 1 };
    }
    if (character === '\r') {
      if (index + 1 === buffer.length && !endOfStream) return undefined;
      const hasLineFeed = buffer[index + 1] === '\n';
      return { lineEnd: index, nextLineStart: index + (hasLineFeed ? 2 : 1) };
    }
  }
  return undefined;
}
