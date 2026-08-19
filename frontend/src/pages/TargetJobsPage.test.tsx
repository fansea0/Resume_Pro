import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { TargetJobsPage } from './TargetJobsPage';
import type { TargetJob } from '../types/targetJob';

const listTargetJobs = vi.fn();
const createTargetJob = vi.fn();
const updateTargetJob = vi.fn();
const deleteTargetJob = vi.fn();

vi.mock('../api/targetJobs', () => ({
  listTargetJobs: (...args: unknown[]) => listTargetJobs(...args),
  createTargetJob: (...args: unknown[]) => createTargetJob(...args),
  updateTargetJob: (...args: unknown[]) => updateTargetJob(...args),
  deleteTargetJob: (...args: unknown[]) => deleteTargetJob(...args),
}));

const job: TargetJob = {
  id: 7,
  company: '字节跳动',
  position: '高级后端工程师',
  requirements: '负责 ToB 营销中台后端架构设计，并推动 AI 能力在营销场景落地。',
  createdAt: '2026-08-18T01:00:00Z',
  updatedAt: '2026-08-19T02:00:00Z',
};

afterEach(cleanup);

describe('TargetJobsPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    listTargetJobs.mockResolvedValue([]);
    vi.stubGlobal('confirm', vi.fn(() => true));
    vi.stubGlobal('alert', vi.fn());
  });

  it('lists target jobs with fallback company labels and a 50-character JD preview', async () => {
    const longRequirements = '一'.repeat(55);
    listTargetJobs.mockResolvedValueOnce([
      job,
      { ...job, id: 8, company: '', position: '产品经理', requirements: longRequirements },
    ]);

    render(<TargetJobsPage />);

    const list = await screen.findByRole('list', { name: '目标岗位列表' });
    expect(within(list).getByText('字节跳动')).toBeInTheDocument();
    expect(within(list).getByText('未指定公司')).toBeInTheDocument();
    expect(within(list).getByText(`${'一'.repeat(50)}…`)).toBeInTheDocument();
  });

  it('validates trimmed requirements before creating a target job', async () => {
    createTargetJob.mockResolvedValueOnce({ ...job, id: 9 });
    render(<TargetJobsPage />);
    await screen.findByText('还没有目标岗位。');

    fireEvent.click(screen.getByRole('button', { name: '+ 新建' }));
    const dialog = screen.getByRole('dialog', { name: '新建目标岗位' });
    expect(within(dialog).getByText('不知道公司 / 职位也能保存——requirements 是 AI 润色的核心依据')).toBeInTheDocument();
    fireEvent.change(within(dialog).getByLabelText('岗位要求'), { target: { value: '  不足十字  ' } });
    fireEvent.click(within(dialog).getByRole('button', { name: '保存目标岗位' }));

    expect(within(dialog).getByRole('alert')).toHaveTextContent('岗位要求至少需要 10 个字符。');
    expect(createTargetJob).not.toHaveBeenCalled();

    fireEvent.change(within(dialog).getByLabelText('公司（可选）'), { target: { value: '  字节跳动  ' } });
    fireEvent.change(within(dialog).getByLabelText('职位（可选）'), { target: { value: '  高级后端工程师  ' } });
    fireEvent.change(within(dialog).getByLabelText('岗位要求'), { target: { value: '  负责营销中台后端架构并推动团队交付  ' } });
    fireEvent.click(within(dialog).getByRole('button', { name: '保存目标岗位' }));

    await waitFor(() => expect(createTargetJob).toHaveBeenCalledWith({
      company: '字节跳动',
      position: '高级后端工程师',
      requirements: '负责营销中台后端架构并推动团队交付',
    }));
    expect(await screen.findByText('字节跳动')).toBeInTheDocument();
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('prefills and saves edits to a target job', async () => {
    listTargetJobs.mockResolvedValueOnce([job]);
    updateTargetJob.mockResolvedValueOnce({ ...job, company: '新公司' });
    render(<TargetJobsPage />);
    await screen.findByText('字节跳动');

    fireEvent.click(screen.getByRole('button', { name: '编辑：字节跳动 · 高级后端工程师' }));
    const dialog = screen.getByRole('dialog', { name: '编辑目标岗位' });
    expect(within(dialog).getByLabelText('岗位要求')).toHaveValue(job.requirements);
    fireEvent.change(within(dialog).getByLabelText('公司（可选）'), { target: { value: '新公司' } });
    fireEvent.click(within(dialog).getByRole('button', { name: '保存更改' }));

    await waitFor(() => expect(updateTargetJob).toHaveBeenCalledWith(7, {
      company: '新公司',
      position: job.position,
      requirements: job.requirements,
    }));
    expect(await screen.findByText('新公司')).toBeInTheDocument();
  });

  it('confirms deletion and reports cleared resume references', async () => {
    listTargetJobs.mockResolvedValueOnce([job]);
    deleteTargetJob.mockResolvedValueOnce({ deletedReferences: 2 });
    const confirm = vi.mocked(window.confirm);
    confirm.mockReturnValueOnce(false).mockReturnValueOnce(true);
    render(<TargetJobsPage />);
    await screen.findByText('字节跳动');

    const deleteButton = screen.getByRole('button', { name: '删除：字节跳动 · 高级后端工程师' });
    fireEvent.click(deleteButton);
    expect(deleteTargetJob).not.toHaveBeenCalled();
    fireEvent.click(deleteButton);

    await waitFor(() => expect(deleteTargetJob).toHaveBeenCalledWith(7));
    expect(confirm).toHaveBeenCalledWith(expect.stringContaining('同时清空这些简历的引用'));
    expect(window.alert).toHaveBeenCalledWith('已清空 2 份简历的引用');
    expect(screen.queryByText('字节跳动')).not.toBeInTheDocument();
  });

  it('offers a retry when target jobs cannot be loaded', async () => {
    listTargetJobs.mockRejectedValueOnce(new Error('network failed')).mockResolvedValueOnce([job]);
    render(<TargetJobsPage />);

    expect(await screen.findByRole('alert')).toHaveTextContent('无法加载目标岗位，请稍后重试。');
    fireEvent.click(screen.getByRole('button', { name: '重试' }));
    expect(await screen.findByText('字节跳动')).toBeInTheDocument();
    expect(listTargetJobs).toHaveBeenCalledTimes(2);
  });

  it('traps focus inside the create dialog and restores it after Escape', async () => {
    render(<TargetJobsPage />);
    await screen.findByText('还没有目标岗位。');
    const trigger = screen.getByRole('button', { name: '+ 新建' });

    trigger.focus();
    fireEvent.click(trigger);
    const dialog = screen.getByRole('dialog', { name: '新建目标岗位' });
    const company = within(dialog).getByLabelText('公司（可选）');
    expect(company).toHaveFocus();

    trigger.focus();
    expect(company).toHaveFocus();
    const save = within(dialog).getByRole('button', { name: '保存目标岗位' });
    save.focus();
    fireEvent.keyDown(dialog, { key: 'Tab' });
    expect(within(dialog).getByRole('button', { name: '关闭' })).toHaveFocus();

    fireEvent.keyDown(dialog, { key: 'Escape' });
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(trigger).toHaveFocus();
  });

  it('disables every delete action while a deletion is pending', async () => {
    const secondJob = { ...job, id: 8, company: '第二家公司' };
    listTargetJobs.mockResolvedValueOnce([job, secondJob]);
    let resolveDelete!: (result: { deletedReferences: number }) => void;
    deleteTargetJob.mockReturnValueOnce(new Promise((resolve) => { resolveDelete = resolve; }));
    render(<TargetJobsPage />);
    await screen.findByText('字节跳动');

    fireEvent.click(screen.getByRole('button', { name: '删除：字节跳动 · 高级后端工程师' }));

    expect(screen.getByRole('button', { name: '删除：字节跳动 · 高级后端工程师' })).toBeDisabled();
    expect(screen.getByRole('button', { name: '删除：第二家公司 · 高级后端工程师' })).toBeDisabled();
    expect(deleteTargetJob).toHaveBeenCalledTimes(1);

    resolveDelete({ deletedReferences: 0 });
    await waitFor(() => expect(screen.getByRole('button', { name: '删除：第二家公司 · 高级后端工程师' })).toBeEnabled());
  });
});
