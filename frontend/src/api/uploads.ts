import { ApiError } from './client';

export interface AvatarUploadResponse {
  url: string;
}

export async function uploadResumeAvatar(file: File, key: string): Promise<AvatarUploadResponse> {
  const body = new FormData();
  body.append('avatar', file);
  body.append('key', key);
  const response = await fetch('/api/uploads/avatar', { method: 'POST', credentials: 'include', body });
  if (!response.ok) {
    const payload = await response.json().catch(() => ({}));
    throw new ApiError(payload.error ?? '头像上传失败，请稍后重试。', response.status);
  }
  return response.json() as Promise<AvatarUploadResponse>;
}
