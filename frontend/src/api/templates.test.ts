import { afterEach, describe, expect, it, vi } from 'vitest';
import { disableTemplate, getTemplateStatus, listAdminTemplates } from './templates';

afterEach(() => vi.unstubAllGlobals());

describe('template status API', () => {
  it('reads public and administrator status and disables by identifier', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ disabledTemplateIds: ['redline'] }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ disabledTemplateIds: [] }), { status: 200 }))
      .mockResolvedValueOnce(new Response(null, { status: 204 }));
    vi.stubGlobal('fetch', fetchMock);

    await expect(getTemplateStatus()).resolves.toEqual({ disabledTemplateIds: ['redline'] });
    await expect(listAdminTemplates()).resolves.toEqual({ disabledTemplateIds: [] });
    await expect(disableTemplate('redline')).resolves.toBeUndefined();
    expect(fetchMock).toHaveBeenNthCalledWith(3, '/api/admin/templates/redline', expect.objectContaining({ method: 'DELETE', credentials: 'include' }));
  });
});
