import { afterEach, describe, expect, it, vi } from 'vitest';

import {
  createTargetJob,
  deleteTargetJob,
  getTargetJob,
  listTargetJobs,
  updateTargetJob,
} from './targetJobs';

const targetJob = {
  id: 7,
  company: 'OpenAI',
  position: 'Engineer',
  requirements: 'Build reliable products for people.',
  createdAt: '2026-08-19T00:00:00Z',
  updatedAt: '2026-08-19T00:00:00Z',
};

const input = {
  company: 'OpenAI',
  position: 'Engineer',
  requirements: 'Build reliable products for people.',
};

afterEach(() => vi.unstubAllGlobals());

describe('target jobs API', () => {
  it('lists target jobs with an authenticated GET request', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify([targetJob]), { status: 200 }));
    vi.stubGlobal('fetch', fetchMock);

    await expect(listTargetJobs()).resolves.toEqual([targetJob]);

    expect(fetchMock).toHaveBeenCalledWith('/api/target-jobs', expect.objectContaining({
      method: 'GET', credentials: 'include',
    }));
  });

  it('gets one target job with an authenticated GET request', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(targetJob), { status: 200 }));
    vi.stubGlobal('fetch', fetchMock);

    await expect(getTargetJob(7)).resolves.toEqual(targetJob);

    expect(fetchMock).toHaveBeenCalledWith('/api/target-jobs/7', expect.objectContaining({
      method: 'GET', credentials: 'include',
    }));
  });

  it('creates a target job with a JSON POST body', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(targetJob), { status: 201 }));
    vi.stubGlobal('fetch', fetchMock);

    await expect(createTargetJob(input)).resolves.toEqual(targetJob);

    expect(fetchMock).toHaveBeenCalledWith('/api/target-jobs', expect.objectContaining({
      method: 'POST',
      credentials: 'include',
      body: JSON.stringify(input),
      headers: expect.objectContaining({ 'Content-Type': 'application/json' }),
    }));
  });

  it('updates a target job with a JSON PUT body', async () => {
    const updated = { ...targetJob, company: 'OpenAI Research' };
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(updated), { status: 200 }));
    vi.stubGlobal('fetch', fetchMock);

    await expect(updateTargetJob(7, { ...input, company: 'OpenAI Research' })).resolves.toEqual(updated);

    expect(fetchMock).toHaveBeenCalledWith('/api/target-jobs/7', expect.objectContaining({
      method: 'PUT',
      credentials: 'include',
      body: JSON.stringify({ ...input, company: 'OpenAI Research' }),
      headers: expect.objectContaining({ 'Content-Type': 'application/json' }),
    }));
  });

  it('deletes a target job and returns its deleted reference count', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ deletedReferences: 3 }), { status: 200 }));
    vi.stubGlobal('fetch', fetchMock);

    await expect(deleteTargetJob(7)).resolves.toEqual({ deletedReferences: 3 });

    expect(fetchMock).toHaveBeenCalledWith('/api/target-jobs/7', expect.objectContaining({
      method: 'DELETE', credentials: 'include',
    }));
  });

  it('propagates API errors from failed target job requests', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ error: 'target job not found' }), { status: 404 })));

    await expect(getTargetJob(7)).rejects.toMatchObject({
      name: 'ApiError', message: 'target job not found', status: 404,
    });
  });
});
