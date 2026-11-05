export class ApiError extends Error {
  /**
   * requestId 是后端在 5xx 响应里带的请求编号（X-Request-ID）。
   *
   * 服务端出错时用户只看到一句泛化文案，没有编号就没法定位到具体哪次请求、
   * 哪一步失败。把编号显示出来，用户报障时报这个编号即可在后端日志里查到。
   */
  constructor(message: string, public readonly status: number, public readonly requestId?: string) {
    super(message);
    this.name = 'ApiError';
  }
}

type UnauthorizedListener = () => void;
const unauthorizedListeners = new Set<UnauthorizedListener>();

export function onUnauthorized(listener: UnauthorizedListener) {
  unauthorizedListeners.add(listener);
  return () => unauthorizedListeners.delete(listener);
}

export async function apiFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...init,
    credentials: 'include',
    headers: { 'Content-Type': 'application/json', ...init?.headers },
  });

  if (!response.ok) {
    const body = await response.json().catch(() => ({}));
    if (response.status === 401) {
      unauthorizedListeners.forEach((listener) => listener());
    }
    throw new ApiError(body.error ?? '请求失败，请稍后重试。', response.status, body.request_id);
  }

  if (response.status === 204) return undefined as T;
  return response.json() as Promise<T>;
}
