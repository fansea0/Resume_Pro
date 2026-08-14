import { afterEach, describe, expect, it, vi } from 'vitest';
import { disableTemplate, getTemplateStatus, listAdminTemplates } from './templates';

afterEach(() => vi.unstubAllGlobals());

describe('template status API', () => {
  it('reads public and administrator status and disables by identifier', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ disabledTemplateIds: ['minimal'] }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ disabledTemplateIds: [] }), { status: 200 }))
      .mockResolvedValueOnce(new Response(null, { status: 204 }));
    vi.stubGlobal('fetch', fetchMock);

    await expect(getTemplateStatus()).resolves.toEqual({ disabledTemplateIds: ['minimal'] });
    await expect(listAdminTemplates()).resolves.toEqual({ disabledTemplateIds: [] });
    await expect(disableTemplate('minimal')).resolves.toBeUndefined();
    expect(fetchMock).toHaveBeenNthCalledWith(3, '/api/admin/templates/minimal', expect.objectContaining({ method: 'DELETE', credentials: 'include' }));
  });
});
