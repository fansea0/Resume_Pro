import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ResumeDrafts } from './ResumeDrafts';

const listResumes = vi.fn();

vi.mock('../api/resumes', () => ({
  listResumes: (...args: unknown[]) => listResumes(...args),
}));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe('ResumeDrafts', () => {
  it('lists only returned drafts and continues editing the selected draft', async () => {
    listResumes.mockResolvedValueOnce([{
      id: 7,
      title: '产品经理简历',
      templateId: 'business',
      content: '{}',
      createdAt: '2026-08-14T00:00:00Z',
      updatedAt: '2026-08-14T01:00:00Z',
    }]);
    const onSelect = vi.fn();

    render(<ResumeDrafts onCreate={() => undefined} onSelect={onSelect} />);

    expect(await screen.findByText('产品经理简历')).toBeInTheDocument();
    expect(screen.getByText('现代商务')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: '继续编辑：产品经理简历' }));
    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ id: 7 }));
  });

  it('shows an empty state and lets the user choose a template', async () => {
    listResumes.mockResolvedValueOnce([]);
    const onCreate = vi.fn();

    render(<ResumeDrafts onCreate={onCreate} onSelect={vi.fn()} />);

    expect(await screen.findByText('还没有保存的简历草稿。')).toBeInTheDocument();
    expect(screen.getByRole('region', { name: '我的草稿' })).toHaveClass('resume-drafts');
    expect(screen.getByText('还没有保存的简历草稿。').closest('aside')).toHaveClass('resume-drafts__empty');
    expect(screen.getByRole('heading', { name: '开始你的第一份简历' })).toBeInTheDocument();
    expect(screen.getByRole('list', { name: '创建简历的步骤' })).toHaveTextContent('选择适合你的简历模板');
    expect(screen.getByRole('list', { name: '创建简历的步骤' })).toHaveTextContent('填写并完善你的经历');
    expect(screen.getByRole('list', { name: '创建简历的步骤' })).toHaveTextContent('随时返回继续编辑');
    fireEvent.click(screen.getByRole('button', { name: '去选择模板' }));
    expect(onCreate).toHaveBeenCalledOnce();
  });

  it('presents each saved draft as a workbench item with readable metadata', async () => {
    listResumes.mockResolvedValueOnce([{
      id: 7,
      title: '产品经理简历',
      templateId: 'business',
      content: '{}',
      createdAt: '2026-08-14T00:00:00Z',
      updatedAt: '2026-08-14T01:00:00Z',
    }]);

    render(<ResumeDrafts onCreate={vi.fn()} onSelect={vi.fn()} />);

    const draft = await screen.findByRole('listitem');
    expect(draft).toHaveClass('resume-drafts__item');
    expect(draft.querySelector('.resume-drafts__index')).toHaveAttribute('aria-hidden', 'true');
    expect(draft.querySelector('.resume-drafts__metadata')).toHaveTextContent('模板');
    expect(draft.querySelector('.resume-drafts__metadata')).toHaveTextContent('现代商务');
    expect(draft.querySelector('.resume-drafts__metadata')).toHaveTextContent('最后更新');
    expect(draft.querySelector('time')).toHaveAttribute('dateTime', '2026-08-14T01:00:00Z');
  });

  it('shows a recoverable error when drafts cannot be loaded', async () => {
    listResumes.mockRejectedValueOnce(new Error('network failed'));

    render(<ResumeDrafts onCreate={vi.fn()} onSelect={vi.fn()} />);

    expect(await screen.findByRole('alert')).toHaveTextContent('无法加载草稿，请稍后重试。');
  });

  it('retries loading drafts after an error', async () => {
    listResumes
      .mockRejectedValueOnce(new Error('network failed'))
      .mockResolvedValueOnce([{
        id: 8,
        title: '重试后的简历',
        templateId: 'minimal',
        content: '{}',
        createdAt: '2026-08-14T00:00:00Z',
        updatedAt: '2026-08-14T01:00:00Z',
      }]);

    render(<ResumeDrafts onCreate={vi.fn()} onSelect={vi.fn()} />);

    expect(await screen.findByRole('alert')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: '重试' }));
    expect(await screen.findByText('重试后的简历')).toBeInTheDocument();
    expect(listResumes).toHaveBeenCalledTimes(2);
  });

  it('disables retry while another draft request is loading', async () => {
    let resolveRetry: (value: Array<{
      id: number;
      title: string;
      templateId: 'minimal';
      content: string;
      createdAt: string;
      updatedAt: string;
    }>) => void;
    const retryRequest = new Promise<Array<{
      id: number;
      title: string;
      templateId: 'minimal';
      content: string;
      createdAt: string;
      updatedAt: string;
    }>>((resolve) => { resolveRetry = resolve; });
    listResumes.mockRejectedValueOnce(new Error('network failed')).mockReturnValueOnce(retryRequest);

    render(<ResumeDrafts onCreate={vi.fn()} onSelect={vi.fn()} />);

    expect(await screen.findByRole('alert')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: '重试' }));
    expect(screen.getByRole('button', { name: '重试' })).toBeDisabled();

    resolveRetry!([]);
  });
});
