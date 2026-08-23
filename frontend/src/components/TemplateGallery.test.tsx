import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import App from '../app';
import { templateRegistry } from '../templates/registry';

vi.mock('../api/auth', () => ({
  getCurrentUser: vi.fn().mockResolvedValue({ id: 1, email: 'ada@example.com', tenantId: 1 }),
  login: vi.fn(), register: vi.fn(), logout: vi.fn(),
}));

afterEach(() => {
  cleanup();
  window.history.replaceState({}, '', '/');
});

describe('template gallery workflow', () => {
  it('uses a compact gallery heading without the removed hero copy', async () => {
    const { TemplateGallery } = await import('./TemplateGallery');
    render(<TemplateGallery onSelect={vi.fn()} />);

    expect(screen.getByRole('heading', { name: '选择简历模板' })).toBeInTheDocument();
    expect(screen.queryByText('从一个好模板开始')).not.toBeInTheDocument();
    expect(screen.queryByText('选择适合你的版式，下一步就能专注呈现你的经历。')).not.toBeInTheDocument();
  });

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

  it('marks the active template and supports canceling replacement mode', async () => {
    const { TemplateGallery } = await import('./TemplateGallery');
    const onCancel = vi.fn();
    render(<TemplateGallery currentTemplateId="business" onCancelReplacement={onCancel} onSelect={vi.fn()} replacementMode />);

    expect(screen.getByRole('heading', { name: '替换简历模板' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: '当前模板：现代商务' })).toBeDisabled();
    fireEvent.click(screen.getByRole('button', { name: '取消替换' }));
    expect(onCancel).toHaveBeenCalledOnce();
  });

  it('opens AI quick generation without starting a model request', async () => {
    const { TemplateGallery } = await import('./TemplateGallery');
    render(<TemplateGallery onSelect={vi.fn()} />);

    fireEvent.click(screen.getByRole('button', { name: 'AI 快速生成' }));

    expect(screen.getByRole('dialog', { name: 'AI 快速生成' })).toBeInTheDocument();
  });

  it('navigates from the gallery to target-job management', async () => {
    render(<App />);

    await screen.findByRole('heading', { name: '选择简历模板' });

    fireEvent.click(screen.getByRole('button', { name: '管理目标岗位' }));

    expect(await screen.findByRole('heading', { name: '目标岗位' })).toBeInTheDocument();
    expect(window.location.pathname).toBe('/target-jobs');
  });

  it('shows static cards first, enters the selected editor without resetting content, and returns to the gallery', async () => {
    render(<App />);

    await screen.findByRole('heading', { name: '选择简历模板' });

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
    expect(screen.getByRole('heading', { name: '选择简历模板' })).toBeInTheDocument();
  });

  it('gives every static cover a descriptive alternative text', async () => {
    render(<App />);

    await screen.findByRole('heading', { name: '选择简历模板' });

    for (const template of templateRegistry) {
      expect(screen.getByRole('img', { name: `${template.name} 模板预览` })).toHaveAttribute(
        'alt',
        `${template.name} 模板预览`,
      );
    }
  });
});
