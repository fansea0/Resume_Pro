import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { TemplateAdmin } from './TemplateAdmin';
import { templateRegistry } from '../templates/registry';

const listAdminTemplates = vi.fn();
const disableTemplate = vi.fn();

vi.mock('../api/templates', () => ({
  listAdminTemplates: (...args: unknown[]) => listAdminTemplates(...args),
  disableTemplate: (...args: unknown[]) => disableTemplate(...args),
}));

afterEach(cleanup);

describe('TemplateAdmin', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.stubGlobal('confirm', vi.fn(() => true));
  });

  it('disables a template after confirmation', async () => {
    listAdminTemplates.mockResolvedValue({ disabledTemplateIds: [] });
    disableTemplate.mockResolvedValue(undefined);
    render(<TemplateAdmin />);

    expect(await screen.findByRole('heading', { name: '模板管理' })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: '删除模板：极简专业' }));

    await waitFor(() => expect(disableTemplate).toHaveBeenCalledWith('minimal'));
    expect(screen.getByText('已禁用')).toBeInTheDocument();
  });

  it('shows each template\'s job tags and style description', async () => {
    listAdminTemplates.mockResolvedValue({ disabledTemplateIds: [] });
    render(<TemplateAdmin />);

    await screen.findByRole('heading', { name: '模板管理' });
    for (const template of templateRegistry) {
      expect(screen.getByText(template.styleDescription)).toBeInTheDocument();
      for (const tag of template.tags) {
        expect(screen.getAllByText(tag).length).toBeGreaterThan(0);
      }
    }
  });

  it('presents template administration as a labelled settings list', async () => {
    listAdminTemplates.mockResolvedValue({ disabledTemplateIds: [] });
    render(<TemplateAdmin />);

    expect(await screen.findByRole('heading', { name: '模板管理' })).toBeInTheDocument();
    expect(screen.getByRole('list', { name: '模板设置' }).children).toHaveLength(templateRegistry.length);
  });
});
