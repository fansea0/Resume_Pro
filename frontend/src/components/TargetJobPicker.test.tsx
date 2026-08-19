import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { TargetJobPicker } from './TargetJobPicker';
import type { TargetJob } from '../types/targetJob';

const listTargetJobs = vi.fn();
const createTargetJob = vi.fn();

vi.mock('../api/targetJobs', () => ({
  listTargetJobs: (...args: unknown[]) => listTargetJobs(...args),
  createTargetJob: (...args: unknown[]) => createTargetJob(...args),
}));

const jobs: TargetJob[] = [
  {
    id: 7,
    company: '字节跳动',
    position: '高级后端工程师',
    requirements: '负责 ToB 营销中台后端架构设计，并推动 AI 能力在营销场景落地。',
    createdAt: '2026-08-18T01:00:00Z',
    updatedAt: '2026-08-19T02:00:00Z',
  },
  {
    id: 8,
    company: '',
    position: '',
    requirements: '负责业务分析、跨团队协作和产品交付，持续改善用户体验。',
    createdAt: '2026-08-18T03:00:00Z',
    updatedAt: '2026-08-19T04:00:00Z',
  },
];

afterEach(cleanup);

describe('TargetJobPicker', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    listTargetJobs.mockResolvedValue(jobs);
  });

  it('loads jobs when opened, identifies the current binding, and selects a job', async () => {
    const onClose = vi.fn();
    const onSelect = vi.fn();
    render(<TargetJobPicker onClose={onClose} onSelect={onSelect} open selectedId={7} />);

    const dialog = screen.getByRole('dialog', { name: '选择目标岗位' });
    expect(within(dialog).getByText('正在加载目标岗位…')).toBeInTheDocument();
    expect(await within(dialog).findByText('当前绑定')).toBeInTheDocument();
    expect(within(dialog).getByText('未指定公司 · 未指定职位')).toBeInTheDocument();

    fireEvent.click(within(dialog).getByRole('button', { name: '选择：未指定公司 · 未指定职位' }));

    expect(onSelect).toHaveBeenCalledWith(8);
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('validates trimmed requirements and selects a newly created job', async () => {
    const onClose = vi.fn();
    const onSelect = vi.fn();
    const created = { ...jobs[0], id: 9, company: '新公司', position: '平台工程师' };
    createTargetJob.mockResolvedValueOnce(created);
    render(<TargetJobPicker onClose={onClose} onSelect={onSelect} open />);
    await screen.findByText('字节跳动 · 高级后端工程师');

    fireEvent.click(screen.getByRole('button', { name: '+ 新建目标岗位' }));
    const dialog = screen.getByRole('dialog', { name: '新建并选择目标岗位' });
    fireEvent.change(within(dialog).getByLabelText('岗位要求'), { target: { value: '  不足十字  ' } });
    fireEvent.click(within(dialog).getByRole('button', { name: '保存并选择' }));

    expect(within(dialog).getByRole('alert')).toHaveTextContent('岗位要求至少需要 10 个字符。');
    expect(createTargetJob).not.toHaveBeenCalled();

    fireEvent.change(within(dialog).getByLabelText('公司（可选）'), { target: { value: '  新公司  ' } });
    fireEvent.change(within(dialog).getByLabelText('职位（可选）'), { target: { value: '  平台工程师  ' } });
    fireEvent.change(within(dialog).getByLabelText('岗位要求'), { target: { value: '  负责平台建设、稳定性治理与研发效能提升  ' } });
    fireEvent.click(within(dialog).getByRole('button', { name: '保存并选择' }));

    await waitFor(() => expect(createTargetJob).toHaveBeenCalledWith({
      company: '新公司',
      position: '平台工程师',
      requirements: '负责平台建设、稳定性治理与研发效能提升',
    }));
    expect(onSelect).toHaveBeenCalledWith(9);
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('offers retry after a load failure and restores focus when closed with Escape', async () => {
    listTargetJobs.mockRejectedValueOnce(new Error('network')).mockResolvedValueOnce(jobs);
    const onClose = vi.fn();
    const trigger = document.createElement('button');
    trigger.textContent = '打开选择器';
    document.body.appendChild(trigger);
    trigger.focus();

    const { rerender } = render(<TargetJobPicker onClose={onClose} onSelect={vi.fn()} open />);
    expect(await screen.findByRole('alert')).toHaveTextContent('无法加载目标岗位');
    fireEvent.click(screen.getByRole('button', { name: '重试' }));
    expect(await screen.findByText('字节跳动 · 高级后端工程师')).toBeInTheDocument();

    const dialog = screen.getByRole('dialog', { name: '选择目标岗位' });
    fireEvent.keyDown(dialog, { key: 'Escape' });
    expect(onClose).toHaveBeenCalledTimes(1);
    rerender(<TargetJobPicker onClose={onClose} onSelect={vi.fn()} open={false} />);
    expect(trigger).toHaveFocus();
    trigger.remove();
  });
});
