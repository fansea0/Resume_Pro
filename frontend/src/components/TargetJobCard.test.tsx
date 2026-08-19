import { useState } from 'react';
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { TargetJobCard } from './TargetJobCard';
import type { TargetJob } from '../types/targetJob';

const listTargetJobs = vi.fn();
const createTargetJob = vi.fn();

vi.mock('../api/targetJobs', () => ({
  listTargetJobs: (...args: unknown[]) => listTargetJobs(...args),
  createTargetJob: (...args: unknown[]) => createTargetJob(...args),
}));

const targetJob: TargetJob = {
  id: 7,
  company: '字节跳动',
  position: '高级后端工程师',
  requirements: '负责 ToB 营销中台后端架构设计，并推动 AI 能力在营销场景落地。',
  createdAt: '2026-08-18T01:00:00Z',
  updatedAt: '2026-08-19T02:00:00Z',
};

const alternateTargetJob: TargetJob = {
  ...targetJob,
  id: 9,
  company: '新公司',
  position: '平台工程师',
};

function StatefulTargetJobCard({ initiallyBound = false }: { initiallyBound?: boolean }) {
  const [selectedId, setSelectedId] = useState<number | null>(initiallyBound ? targetJob.id : null);
  const selected = [targetJob, alternateTargetJob].find((job) => job.id === selectedId);
  return <TargetJobCard onChange={setSelectedId} targetJob={selected} />;
}

afterEach(cleanup);

describe('TargetJobCard', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    listTargetJobs.mockResolvedValue([targetJob, alternateTargetJob]);
  });

  it('opens the picker from the unbound state and reports the selected targetJobId', async () => {
    const onChange = vi.fn();
    render(<TargetJobCard onChange={onChange} />);

    fireEvent.click(screen.getByRole('button', { name: '+ 选择目标岗位' }));
    const picker = await screen.findByRole('dialog', { name: '选择目标岗位' });
    fireEvent.click(await within(picker).findByRole('button', { name: '选择：字节跳动 · 高级后端工程师' }));

    expect(onChange).toHaveBeenCalledWith(7);
    expect(screen.queryByRole('dialog', { name: '选择目标岗位' })).not.toBeInTheDocument();
  });

  it('shows the current binding and expands the full JD preview', () => {
    render(<TargetJobCard onChange={vi.fn()} targetJob={targetJob} />);

    expect(screen.getByText('当前：字节跳动 · 高级后端工程师')).toBeInTheDocument();
    const toggle = screen.getByRole('button', { name: '展开岗位要求' });
    expect(toggle).toHaveAttribute('aria-expanded', 'false');
    fireEvent.click(toggle);
    expect(screen.getByText(targetJob.requirements)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: '收起岗位要求' })).toHaveAttribute('aria-expanded', 'true');
  });

  it('requires an accessible confirmation before changing the current binding', async () => {
    const onChange = vi.fn();
    render(<TargetJobCard onChange={onChange} targetJob={targetJob} />);

    fireEvent.click(screen.getByRole('button', { name: '更换目标岗位' }));
    const confirmation = screen.getByRole('alertdialog', { name: '更换目标岗位？' });
    expect(within(confirmation).getByText(/先保留当前绑定/)).toBeInTheDocument();
    fireEvent.click(within(confirmation).getByRole('button', { name: '继续更换' }));

    const picker = await screen.findByRole('dialog', { name: '选择目标岗位' });
    expect(await within(picker).findByText('当前绑定')).toBeInTheDocument();
    fireEvent.click(within(picker).getByRole('button', { name: '选择：新公司 · 平台工程师' }));
    expect(onChange).toHaveBeenCalledWith(9);
  });

  it('only clears targetJobId after the clear confirmation is accepted', () => {
    const onChange = vi.fn();
    render(<TargetJobCard onChange={onChange} targetJob={targetJob} />);

    fireEvent.click(screen.getByRole('button', { name: '清除目标岗位绑定' }));
    const confirmation = screen.getByRole('alertdialog', { name: '清除目标岗位绑定？' });
    fireEvent.click(within(confirmation).getByRole('button', { name: '取消' }));
    expect(onChange).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole('button', { name: '清除目标岗位绑定' }));
    fireEvent.click(within(screen.getByRole('alertdialog')).getByRole('button', { name: '清除绑定' }));
    expect(onChange).toHaveBeenCalledWith(null);
  });

  it('moves focus to the connected change control after binding replaces the select trigger', async () => {
    render(<StatefulTargetJobCard />);

    const selectTrigger = screen.getByRole('button', { name: '+ 选择目标岗位' });
    selectTrigger.focus();
    fireEvent.click(selectTrigger);
    const picker = await screen.findByRole('dialog', { name: '选择目标岗位' });
    fireEvent.click(await within(picker).findByRole('button', { name: '选择：字节跳动 · 高级后端工程师' }));

    const changeControl = screen.getByRole('button', { name: '更换目标岗位' });
    await waitFor(() => expect(changeControl).toHaveFocus());
    expect(selectTrigger.isConnected).toBe(false);
  });

  it('moves focus to the connected select control after clearing replaces the bound controls', async () => {
    render(<StatefulTargetJobCard initiallyBound />);

    const clearTrigger = screen.getByRole('button', { name: '清除目标岗位绑定' });
    clearTrigger.focus();
    fireEvent.click(clearTrigger);
    fireEvent.click(within(screen.getByRole('alertdialog')).getByRole('button', { name: '清除绑定' }));

    const selectControl = screen.getByRole('button', { name: '+ 选择目标岗位' });
    await waitFor(() => expect(selectControl).toHaveFocus());
    expect(clearTrigger.isConnected).toBe(false);
  });

  it('moves focus to the connected change control after creating and binding a job', async () => {
    createTargetJob.mockResolvedValueOnce(alternateTargetJob);
    render(<StatefulTargetJobCard />);

    const selectTrigger = screen.getByRole('button', { name: '+ 选择目标岗位' });
    selectTrigger.focus();
    fireEvent.click(selectTrigger);
    const picker = await screen.findByRole('dialog', { name: '选择目标岗位' });
    fireEvent.click(await within(picker).findByRole('button', { name: '+ 新建目标岗位' }));
    const createDialog = screen.getByRole('dialog', { name: '新建并选择目标岗位' });
    fireEvent.change(within(createDialog).getByLabelText('岗位要求'), {
      target: { value: alternateTargetJob.requirements },
    });
    fireEvent.click(within(createDialog).getByRole('button', { name: '保存并选择' }));

    const changeControl = await screen.findByRole('button', { name: '更换目标岗位' });
    await waitFor(() => expect(changeControl).toHaveFocus());
  });
});
