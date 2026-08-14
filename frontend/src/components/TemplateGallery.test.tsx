import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import App from '../app';
import { templateRegistry } from '../templates/registry';

vi.mock('../api/auth', () => ({
  getCurrentUser: vi.fn().mockResolvedValue({ id: 1, email: 'ada@example.com', tenantId: 1 }),
  login: vi.fn(), register: vi.fn(), logout: vi.fn(),
}));

afterEach(cleanup);

describe('template gallery workflow', () => {
  it('uses one native button per card and selects exactly once for click or keyboard activation', async () => {
    const { TemplateGallery } = await import('./TemplateGallery');
    const onSelect = vi.fn();
    render(<TemplateGallery onSelect={onSelect} />);

    const card = screen.getByRole('button', { name: '选择模板：现代商务' });
    expect(card.tagName).toBe('BUTTON');
    expect(screen.queryByRole('button', { name: '使用此模板：现代商务' })).not.toBeInTheDocument();

    fireEvent.click(card);
    expect(onSelect).toHaveBeenCalledTimes(1);
    expect(onSelect).toHaveBeenLastCalledWith('business');

    fireEvent.keyDown(card, { key: 'Enter' });
    fireEvent.click(card);
    expect(onSelect).toHaveBeenCalledTimes(2);
  });

  it('does not show disabled templates', async () => {
    const { TemplateGallery } = await import('./TemplateGallery');
    render(<TemplateGallery disabledTemplateIds={['minimal']} onSelect={vi.fn()} />);

    expect(screen.queryByRole('img', { name: '极简专业 模板预览' })).not.toBeInTheDocument();
    expect(screen.getByRole('img', { name: '现代商务 模板预览' })).toBeInTheDocument();
  });
  it('shows static cards first, enters the selected editor without resetting content, and returns to the gallery', async () => {
    render(<App />);

    await screen.findByRole('heading', { name: '从一个好模板开始' });

    expect(screen.queryByTestId('resume-preview')).not.toBeInTheDocument();
    for (const template of templateRegistry) {
      expect(screen.getByRole('img', { name: `${template.name} 模板预览` })).toHaveAttribute('src', template.previewImage);
      expect(screen.getByRole('button', { name: `选择模板：${template.name}` }).tagName).toBe('BUTTON');
      expect(screen.getByText(template.styleDescription)).toBeInTheDocument();
      for (const tag of template.tags) {
        expect(screen.getAllByText(tag).length).toBeGreaterThan(0);
      }
    }

    fireEvent.click(screen.getByRole('button', { name: '选择模板：现代商务' }));

    expect(screen.getByTestId('resume-preview')).toHaveTextContent('林知夏');
    expect(screen.getByText('当前模板：现代商务')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: '返回模板主页' }));

    expect(screen.queryByTestId('resume-preview')).not.toBeInTheDocument();
    expect(screen.getByRole('heading', { name: '从一个好模板开始' })).toBeInTheDocument();
  });

  it('gives every static cover a descriptive alternative text', async () => {
    render(<App />);

    await screen.findByRole('heading', { name: '从一个好模板开始' });

    for (const template of templateRegistry) {
      expect(screen.getByRole('img', { name: `${template.name} 模板预览` })).toHaveAttribute(
        'alt',
        `${template.name} 模板预览`,
      );
    }
  });
});
