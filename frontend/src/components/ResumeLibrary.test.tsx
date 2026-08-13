import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { createRef } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ResumeLibrary } from './ResumeLibrary';
import { createEmptyResumeDocument } from '../types/resume';
import App from '../app';

const createResume = vi.fn();
const listResumes = vi.fn().mockResolvedValue([]);
const updateResume = vi.fn();
const deleteResume = vi.fn();
const exportResumePng = vi.fn();

vi.mock('../api/auth', () => ({
  getCurrentUser: vi.fn().mockResolvedValue({ id: 1, email: 'ada@example.com', tenantId: 1 }),
  login: vi.fn(), register: vi.fn(), logout: vi.fn(),
}));

vi.mock('../api/resumes', () => ({
  createResume: (...args: unknown[]) => createResume(...args),
  listResumes: (...args: unknown[]) => listResumes(...args),
  updateResume: (...args: unknown[]) => updateResume(...args),
  deleteResume: (...args: unknown[]) => deleteResume(...args),
}));

vi.mock('../utils/exportResumePng', () => ({
  exportResumePng: (...args: unknown[]) => exportResumePng(...args),
}));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe('ResumeLibrary', () => {
  it('saves the current document and renders the returned resume title', async () => {
    const document = { ...createEmptyResumeDocument(), title: '前端工程师简历' };
    createResume.mockResolvedValueOnce({
      id: 7,
      title: '已保存的前端工程师简历',
      templateId: document.templateId,
      content: JSON.stringify(document),
      createdAt: '2026-08-13T00:00:00Z',
      updatedAt: '2026-08-13T00:00:00Z',
    });

    render(<ResumeLibrary document={document} onChange={vi.fn()} onRestoreSample={vi.fn()} onSelect={vi.fn()} />);
    fireEvent.click(screen.getByRole('button', { name: '新建并保存' }));

    await waitFor(() => expect(createResume).toHaveBeenCalledWith(document));
    expect(await screen.findByText('已保存的前端工程师简历')).toBeInTheDocument();
  });

  it('exports with the browser print dialog', () => {
    const print = vi.spyOn(window, 'print').mockImplementation(() => undefined);
    render(<ResumeLibrary document={createEmptyResumeDocument()} onChange={vi.fn()} onRestoreSample={vi.fn()} onSelect={vi.fn()} />);

    fireEvent.click(screen.getByRole('button', { name: '导出 PDF' }));
    expect(print).toHaveBeenCalledOnce();
  });

  it('exports the active preview as a PNG', async () => {
    const preview = document.createElement('article');
    const previewRef = createRef<HTMLElement>();
    Object.defineProperty(previewRef, 'current', { value: preview });
    exportResumePng.mockResolvedValueOnce(undefined);

    render(<ResumeLibrary document={createEmptyResumeDocument()} onChange={vi.fn()} onRestoreSample={vi.fn()} onSelect={vi.fn()} previewRef={previewRef} />);
    fireEvent.click(screen.getByRole('button', { name: '导出 PNG' }));

    await waitFor(() => expect(exportResumePng).toHaveBeenCalledWith(preview, '我的简历'));
  });

  it('shows a recoverable error when PNG export fails', async () => {
    const preview = document.createElement('article');
    const previewRef = createRef<HTMLElement>();
    Object.defineProperty(previewRef, 'current', { value: preview });
    exportResumePng.mockRejectedValueOnce(new Error('render failed'));

    render(<ResumeLibrary document={createEmptyResumeDocument()} onChange={vi.fn()} onRestoreSample={vi.fn()} onSelect={vi.fn()} previewRef={previewRef} />);
    fireEvent.click(screen.getByRole('button', { name: '导出 PNG' }));

    expect(await screen.findByRole('alert')).toHaveTextContent('导出 PNG 失败，请稍后重试。');
  });

  it('shows sample content by default and restores it without saving', async () => {
    render(<App />);
    fireEvent.click(await screen.findByRole('button', { name: '使用此模板：赤线单栏' }));

    expect(await screen.findByText('林知夏')).toBeInTheDocument();
    const title = screen.getByLabelText('简历名称');
    fireEvent.change(title, { target: { value: '我的自定义简历' } });
    expect(title).toHaveValue('我的自定义简历');

    fireEvent.click(screen.getByRole('button', { name: '恢复示例内容' }));

    expect(title).toHaveValue('林知夏｜产品经理简历');
    expect(createResume).not.toHaveBeenCalled();
    expect(updateResume).not.toHaveBeenCalled();
  });
});
