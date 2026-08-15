import { afterEach, describe, expect, it, vi } from 'vitest';
import { uploadResumeAvatar } from './uploads';

afterEach(() => vi.unstubAllGlobals());

describe('uploadResumeAvatar', () => {
  it('sends the image and storage key as an authenticated multipart request', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ url: '/uploads/avatars/photo.webp' }), { status: 200 }));
    vi.stubGlobal('fetch', fetchMock);
    const file = new File(['portrait'], 'photo.webp', { type: 'image/webp' });

    await expect(uploadResumeAvatar(file, '123e4567-e89b-42d3-a456-426614174000')).resolves.toEqual({ url: '/uploads/avatars/photo.webp' });

    expect(fetchMock).toHaveBeenCalledWith('/api/uploads/avatar', expect.objectContaining({ method: 'POST', credentials: 'include' }));
    const init = fetchMock.mock.calls[0][1] as RequestInit;
    expect(init.headers).toBeUndefined();
    expect(init.body).toBeInstanceOf(FormData);
    expect((init.body as FormData).get('key')).toBe('123e4567-e89b-42d3-a456-426614174000');
    expect((init.body as FormData).get('avatar')).toBe(file);
    expect((init.body as FormData).get('file')).toBeNull();
  });

  it('returns the API error message when upload fails', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ error: '请选择 JPG、PNG 或 WebP 图片。' }), { status: 400 })));

    await expect(uploadResumeAvatar(new File(['bad'], 'portrait.gif', { type: 'image/gif' }), '123e4567-e89b-42d3-a456-426614174000')).rejects.toThrow('请选择 JPG、PNG 或 WebP 图片。');
  });
});
