import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { InterviewConfigModal, DEFAULT_INTERVIEW_CONFIG } from './InterviewConfigModal';

afterEach(cleanup);

describe('InterviewConfigModal', () => {
  it('prefills the global default config values', () => {
    render(<InterviewConfigModal onCancel={vi.fn()} onConfirm={vi.fn()} submitting={false} targetJobName="字节跳动 · Go 后端工程师" />);

    expect(screen.getByRole('dialog', { name: '开始 AI 面试' })).toBeInTheDocument();
    expect(screen.getByText(/字节跳动 · Go 后端工程师/)).toBeInTheDocument();
    expect(screen.getByLabelText('题目总数')).toHaveValue(String(DEFAULT_INTERVIEW_CONFIG.questionCount));
    expect(screen.getByLabelText('八股题数量')).toHaveValue(String(DEFAULT_INTERVIEW_CONFIG.labelCounts['0']));
    expect(screen.getByLabelText('算法题数量')).toHaveValue(String(DEFAULT_INTERVIEW_CONFIG.labelCounts['1']));
    expect(screen.getByLabelText('项目题数量')).toHaveValue(String(DEFAULT_INTERVIEW_CONFIG.labelCounts['2']));
    expect(screen.getByLabelText('追问总预算')).toHaveValue('6');
    expect(screen.getByLabelText('单问追问上限')).toHaveValue('1');
  });

  it('keeps the follow-up budget at half of the question count until the user edits it', () => {
    render(<InterviewConfigModal onCancel={vi.fn()} onConfirm={vi.fn()} submitting={false} targetJobName="字节跳动 · Go 后端工程师" />);

    fireEvent.change(screen.getByLabelText('题目总数'), { target: { value: '16' } });
    expect(screen.getByLabelText('追问总预算')).toHaveValue('8');

    fireEvent.change(screen.getByLabelText('追问总预算'), { target: { value: '3' } });
    fireEvent.change(screen.getByLabelText('题目总数'), { target: { value: '18' } });
    expect(screen.getByLabelText('追问总预算')).toHaveValue('3');
  });

  it('reports the submitted config and warns when label counts do not sum to the question count', () => {
    const onConfirm = vi.fn();
    render(<InterviewConfigModal onCancel={vi.fn()} onConfirm={onConfirm} submitting={false} targetJobName="字节跳动 · Go 后端工程师" />);

    fireEvent.change(screen.getByLabelText('题目总数'), { target: { value: '8' } });
    fireEvent.change(screen.getByLabelText('八股题数量'), { target: { value: '3' } });
    fireEvent.change(screen.getByLabelText('算法题数量'), { target: { value: '1' } });
    fireEvent.change(screen.getByLabelText('项目题数量'), { target: { value: '4' } });

    fireEvent.click(screen.getByRole('button', { name: '开始面试' }));
    expect(onConfirm).toHaveBeenCalledWith({
      questionCount: 8,
      labelCounts: { '0': 3, '1': 1, '2': 4 },
      followUpBudget: 4,
      perQuestionFollowUpLimit: 1,
    });

    fireEvent.change(screen.getByLabelText('八股题数量'), { target: { value: '9' } });
    fireEvent.click(screen.getByRole('button', { name: '开始面试' }));
    expect(onConfirm).toHaveBeenCalledTimes(1);
    expect(screen.getByRole('alert')).toHaveTextContent('各标签数量之和应等于题目总数');
  });

  it('cancels through the cancel button', () => {
    const onCancel = vi.fn();
    render(<InterviewConfigModal onCancel={onCancel} onConfirm={vi.fn()} submitting={false} targetJobName="字节跳动 · Go 后端工程师" />);

    fireEvent.click(screen.getByRole('button', { name: '取消' }));
    expect(onCancel).toHaveBeenCalledTimes(1);
  });

  it('reports progress while the questions are being generated', () => {
    render(<InterviewConfigModal onCancel={vi.fn()} onConfirm={vi.fn()} submitting targetJobName="字节跳动 · Go 后端工程师" />);

    expect(screen.getByText('正在生成 12 道题')).toBeInTheDocument();
    expect(screen.getByText(/已用 0s/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: '正在生题…' })).toBeDisabled();
    expect(screen.queryByRole('button', { name: '取消' })).toBeDisabled();
  });
});
