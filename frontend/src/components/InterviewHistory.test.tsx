import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { Interview } from '../api/interviews';
import { InterviewHistory } from './InterviewHistory';

const listInterviews = vi.fn();
const abortInterview = vi.fn();

vi.mock('../api/interviews', () => ({
  listInterviews: (...args: unknown[]) => listInterviews(...args),
  abortInterview: (...args: unknown[]) => abortInterview(...args),
}));

function interview(overrides: Partial<Interview>): Interview {
  return {
    id: 3,
    targetJobId: 7,
    resumeId: 12,
    configSnapshot: {
      questionCount: 12,
      labelCounts: { '0': 5, '1': 2, '2': 5 },
      followUpBudget: 6,
      perQuestionFollowUpLimit: 1,
    },
    status: 'completed',
    overallGrade: 'B+',
    overallReview: '整体扎实。',
    createdAt: '2026-11-02T10:00:00Z',
    updatedAt: '2026-11-02T10:34:00Z',
    completedAt: '2026-11-02T10:34:00Z',
    ...overrides,
  };
}

afterEach(cleanup);

describe('InterviewHistory', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders interview records with status and overall grade', async () => {
    listInterviews.mockResolvedValue([
      interview({ id: 3, status: 'completed', overallGrade: 'B+' }),
      interview({ id: 5, status: 'aborted', overallGrade: null, overallReview: null, completedAt: '2026-10-21T09:00:00Z', updatedAt: '2026-10-21T09:00:00Z' }),
    ]);

    render(<InterviewHistory onOpen={vi.fn()} onStart={vi.fn()} />);

    expect(await screen.findByText('面试 #3')).toBeInTheDocument();
    expect(screen.getByText('面试 #5')).toBeInTheDocument();
    const completedItem = screen.getByRole('listitem', { name: /面试 #3/ });
    expect(completedItem).toHaveTextContent('已完成');
    expect(completedItem).toHaveTextContent('B+');
    const abortedItem = screen.getByRole('listitem', { name: /面试 #5/ });
    expect(abortedItem).toHaveTextContent('已放弃');
    expect(abortedItem).toHaveTextContent('未生成总评');
  });

  it('promotes the active interview into a banner and opens it on continue', async () => {
    const onOpen = vi.fn();
    const active = interview({ id: 8, status: 'active', overallGrade: null, overallReview: null, completedAt: null });
    listInterviews.mockResolvedValue([active]);

    render(<InterviewHistory onOpen={onOpen} onStart={vi.fn()} />);

    fireEvent.click(await screen.findByRole('button', { name: '继续面试' }));
    expect(onOpen).toHaveBeenCalledWith(active);
  });

  it('aborts an active interview after an explicit confirmation', async () => {
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true);
    const active = interview({ id: 8, status: 'active', overallGrade: null, overallReview: null, completedAt: null });
    listInterviews.mockResolvedValue([active]);
    abortInterview.mockResolvedValue({ ...active, status: 'aborted' });

    render(<InterviewHistory onOpen={vi.fn()} onStart={vi.fn()} />);
    fireEvent.click(await screen.findByRole('button', { name: '放弃' }));

    await waitFor(() => expect(abortInterview).toHaveBeenCalledWith(8));
    expect(await screen.findByText('已放弃')).toBeInTheDocument();
    confirmSpy.mockRestore();
  });

  it('opens completed interviews as review reports', async () => {
    const onOpen = vi.fn();
    listInterviews.mockResolvedValue([interview({ id: 3 })]);

    render(<InterviewHistory onOpen={onOpen} onStart={vi.fn()} />);
    fireEvent.click(await screen.findByRole('button', { name: '查看报告' }));

    expect(onOpen).toHaveBeenCalledWith(expect.objectContaining({ id: 3 }));
  });

  it('shows an empty state with the start entry when no interviews exist', async () => {
    listInterviews.mockResolvedValue([]);
    const onStart = vi.fn();

    render(<InterviewHistory onOpen={vi.fn()} onStart={onStart} />);

    fireEvent.click(await screen.findByRole('button', { name: '去目标岗位页开始' }));
    expect(onStart).toHaveBeenCalledTimes(1);
  });
});
