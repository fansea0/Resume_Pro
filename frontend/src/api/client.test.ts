import { afterEach, describe, expect, it, vi } from 'vitest';
import { apiFetch, onUnauthorized } from './client';

afterEach(() => vi.unstubAllGlobals());

describe('apiFetch', () => {
  it('includes cookies and notifies the application when a session expires', async () => {
    const fetch = vi.fn().mockResolvedValue(new Response(JSON.stringify({ error: 'authentication required' }), { status: 401 }));
    vi.stubGlobal('fetch', fetch);
    const onExpired = vi.fn();
    const removeListener = onUnauthorized(onExpired);

    await expect(apiFetch('/api/resumes')).rejects.toEqual(expect.objectContaining({ status: 401 }));

    expect(fetch).toHaveBeenCalledWith('/api/resumes', expect.objectContaining({ credentials: 'include' }));
    expect(onExpired).toHaveBeenCalledOnce();
    removeListener();
  });
});
