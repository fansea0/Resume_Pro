import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { Interview, InterviewQuestion } from '../api/interviews';
import { InterviewChat } from './InterviewChat';

const getInterviewQuestions = vi.fn();
const answerInterview = vi.fn();

vi.mock('../api/interviews', () => ({
  getInterviewQuestions: (...args: unknown[]) => getInterviewQuestions(...args),
  answerInterview: (...args: unknown[]) => answerInterview(...args),
}));

const interview: Interview = {
  id: 3,
  targetJobId: 7,
  resumeId: 12,
  configSnapshot: {
    questionCount: 12,
    labelCounts: { '0': 5, '1': 2, '2': 5 },
    followUpBudget: 6,
    perQuestionFollowUpLimit: 1,
  },
  status: 'active',
  overallGrade: null,
  overallReview: null,
  createdAt: '2026-11-02T10:00:00Z',
  updatedAt: '2026-11-02T10:00:00Z',
  completedAt: null,
};

function makeQuestion(overrides: Partial<InterviewQuestion>): InterviewQuestion {
  return {
    id: 1,
    interviewId: 3,
    parentId: null,
    followUpRound: 0,
    label: '0',
    question: 'Go 的 channel 底层数据结构是怎样的？',
    standardAnswer: 'hchan = 环形队列 + sudog 等待队列 + lock。',
    userAnswer: null,
    extra: { mustPoints: ['hchan 结构组成'], bonusPoints: ['sudog 挂起与 gopark'] },
    grade: null,
    createdAt: '2026-11-02T10:01:00Z',
    ...overrides,
  };
}

afterEach(() => {
  cleanup();
  delete (Element.prototype as unknown as { scrollIntoView?: unknown }).scrollIntoView;
});

describe('InterviewChat', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // jsdom 没有 scrollIntoView，组件用它把当前题滚入视野。
    Element.prototype.scrollIntoView = vi.fn();
  });

  it('renders the conversation with grades, waiting marker and progress', async () => {
    getInterviewQuestions.mockResolvedValue([
      makeQuestion({ id: 1, userAnswer: 'hchan 结构体', grade: 'A' }),
      makeQuestion({ id: 2, question: 'MySQL 索引为什么选 B+ 树？', label: '0', userAnswer: null, grade: null }),
    ]);

    render(<InterviewChat interview={interview} onFinished={vi.fn()} onExit={vi.fn()} />);

    expect(await screen.findByText('Go 的 channel 底层数据结构是怎样的？')).toBeInTheDocument();
    expect(screen.getByText('hchan 结构体')).toBeInTheDocument();
    expect(screen.getByText('MySQL 索引为什么选 B+ 树？')).toBeInTheDocument();
    expect(screen.getByText('等待回答')).toBeInTheDocument();
    expect(screen.getByText('进度')).toHaveTextContent('1/12');
  });

  it('keeps the thread at the current question and tracks progress', async () => {
    getInterviewQuestions.mockResolvedValue([
      makeQuestion({ id: 1, userAnswer: '答过了', grade: 'B' }),
      makeQuestion({ id: 2, question: '当前题：Redis 持久化怎么选？', userAnswer: null, grade: null }),
      makeQuestion({ id: 3, question: '还没轮到的题', userAnswer: null, grade: null }),
    ]);

    const { container } = render(<InterviewChat interview={interview} onFinished={vi.fn()} onExit={vi.fn()} />);

    expect(await screen.findByText('当前题：Redis 持久化怎么选？')).toBeInTheDocument();
    expect(screen.queryByText('还没轮到的题')).not.toBeInTheDocument();
    expect(screen.getByText('第 2 / 12 题')).toBeInTheDocument();
    expect(container.querySelectorAll('.interview-progress__seg')).toHaveLength(12);
    expect(container.querySelector('.interview-progress__seg--b')).toHaveTextContent('B');
    expect(container.querySelector('.interview-progress__seg--current')).toHaveTextContent('2');
  });

  it('marks only the awaiting question and keeps the parent grade visible', async () => {
    getInterviewQuestions.mockResolvedValue([
      makeQuestion({ id: 1, userAnswer: '答过了', grade: 'B+' }),
      makeQuestion({ id: 9, parentId: 1, followUpRound: 1, question: '追问：ctx.Err() 怎么区分？', userAnswer: null, grade: null }),
      makeQuestion({ id: 2, question: '还没轮到的题', userAnswer: null, grade: null }),
    ]);

    const { container } = render(<InterviewChat interview={interview} onFinished={vi.fn()} onExit={vi.fn()} />);

    expect(await screen.findByText('追问：ctx.Err() 怎么区分？')).toBeInTheDocument();
    expect(screen.getAllByText('等待回答')).toHaveLength(1);
    expect(container.querySelector('.interview-msg__head .interview-grade')).toHaveTextContent('B+');
    expect(container.querySelector('.interview-msg--current')).toBeTruthy();
  });

  it('submits the typed answer, shows the grade and the follow-up question', async () => {
    getInterviewQuestions
      .mockResolvedValueOnce([makeQuestion({ id: 1, userAnswer: null, grade: null })])
      .mockResolvedValueOnce([
        makeQuestion({ id: 1, userAnswer: 'hchan 结构体', grade: 'B+' }),
        makeQuestion({ id: 9, parentId: 1, followUpRound: 1, question: 'sendx / recvx 什么时候会阻塞？', userAnswer: null, grade: null }),
      ]);
    answerInterview.mockResolvedValue({ grade: 'B+', followUp: 'sendx / recvx 什么时候会阻塞？' });

    render(<InterviewChat interview={interview} onFinished={vi.fn()} onExit={vi.fn()} />);
    fireEvent.change(await screen.findByLabelText('回答输入'), { target: { value: 'hchan 结构体' } });
    fireEvent.click(screen.getByRole('button', { name: '提交回答' }));

    await waitFor(() => expect(answerInterview).toHaveBeenCalledWith(3, 'hchan 结构体'));
    expect(await screen.findByText('sendx / recvx 什么时候会阻塞？')).toBeInTheDocument();
    expect(screen.getByText('追问 1')).toBeInTheDocument();
    expect(getInterviewQuestions).toHaveBeenCalledTimes(2);
  });

  it('reports the overall result and stops when the interview finishes', async () => {
    const onFinished = vi.fn();
    getInterviewQuestions.mockResolvedValue([makeQuestion({ id: 1, userAnswer: null, grade: null })]);
    answerInterview.mockResolvedValue({
      grade: 'A', followUp: '', finished: true, overallGrade: 'B+', overallReview: '整体扎实。',
    });

    render(<InterviewChat interview={interview} onFinished={onFinished} onExit={vi.fn()} />);
    fireEvent.change(await screen.findByLabelText('回答输入'), { target: { value: '最后一名答完' } });
    fireEvent.click(screen.getByRole('button', { name: '提交回答' }));

    await waitFor(() => expect(onFinished).toHaveBeenCalledWith({ overallGrade: 'B+', overallReview: '整体扎实。' }));
  });

  it('confirms before sending a skip flag without an answer', async () => {
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true);
    getInterviewQuestions.mockResolvedValue([makeQuestion({ id: 1, userAnswer: null, grade: null })]);
    answerInterview.mockResolvedValue({ grade: 'C', skip: true, followUp: '', nextQuestion: null, finished: false });

    render(<InterviewChat interview={interview} onFinished={vi.fn()} onExit={vi.fn()} />);
    fireEvent.click(await screen.findByRole('button', { name: '跳过 · 不会' }));

    expect(confirmSpy).toHaveBeenCalled();
    await waitFor(() => expect(answerInterview).toHaveBeenCalledWith(3, undefined, true));
    confirmSpy.mockRestore();
  });

  it('does not submit again while a request is in flight', async () => {
    let release: (value: unknown) => void = () => {};
    getInterviewQuestions.mockResolvedValue([makeQuestion({ id: 1, userAnswer: null, grade: null })]);
    answerInterview.mockReturnValue(new Promise((resolve) => { release = resolve; }));

    render(<InterviewChat interview={interview} onFinished={vi.fn()} onExit={vi.fn()} />);
    fireEvent.change(await screen.findByLabelText('回答输入'), { target: { value: '第一次提交' } });
    fireEvent.click(screen.getByRole('button', { name: '提交回答' }));

    expect(screen.getByRole('button', { name: '评分中…' })).toBeDisabled();
    fireEvent.click(screen.getByRole('button', { name: '评分中…' }));
    expect(answerInterview).toHaveBeenCalledTimes(1);
    release({ grade: 'B', followUp: '' });
  });

  it('surfaces an evaluation failure and allows retrying', async () => {
    getInterviewQuestions.mockResolvedValue([makeQuestion({ id: 1, userAnswer: null, grade: null })]);
    answerInterview.mockRejectedValueOnce(Object.assign(new Error('boom'), { name: 'ApiError', status: 502 }));

    render(<InterviewChat interview={interview} onFinished={vi.fn()} onExit={vi.fn()} />);
    fireEvent.change(await screen.findByLabelText('回答输入'), { target: { value: '回答' } });
    fireEvent.click(screen.getByRole('button', { name: '提交回答' }));

    expect(await screen.findByRole('alert')).toHaveTextContent('评分失败，请重试');
    expect(screen.getByRole('button', { name: '提交回答' })).toBeEnabled();
  });
});
