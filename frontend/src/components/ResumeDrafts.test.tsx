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
    expect(screen.getByText('还没有保存的简历草稿。').parentElement).toHaveClass('resume-drafts__empty');
    fireEvent.click(screen.getByRole('button', { name: '去选择模板' }));
    expect(onCreate).toHaveBeenCalledOnce();
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
