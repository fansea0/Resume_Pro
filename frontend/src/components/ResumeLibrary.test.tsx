import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ResumeLibrary } from './ResumeLibrary';
import { createEmptyResumeDocument } from '../types/resume';
import App from '../app';

const createResume = vi.fn();
const listResumes = vi.fn().mockResolvedValue([]);
const updateResume = vi.fn();
const deleteResume = vi.fn();

vi.mock('../api/resumes', () => ({
  createResume: (...args: unknown[]) => createResume(...args),
  listResumes: (...args: unknown[]) => listResumes(...args),
  updateResume: (...args: unknown[]) => updateResume(...args),
  deleteResume: (...args: unknown[]) => deleteResume(...args),
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

  it('shows sample content by default and restores it without saving', async () => {
    render(<App />);

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
