export class ApiError extends Error {
  constructor(message: string, public readonly status: number) {
    super(message);
    this.name = 'ApiError';
  }
}

export async function apiFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...init,
    headers: { 'Content-Type': 'application/json', ...init?.headers },
  });

  if (!response.ok) {
    const body = await response.json().catch(() => ({}));
    throw new ApiError(body.error ?? '请求失败，请稍后重试。', response.status);
  }

  if (response.status === 204) return undefined as T;
  return response.json() as Promise<T>;
}
