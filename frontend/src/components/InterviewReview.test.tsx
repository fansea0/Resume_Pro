import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import type { Interview, InterviewQuestion } from '../api/interviews';
import { InterviewReview } from './InterviewReview';

const getInterviewQuestions = vi.fn();

vi.mock('../api/interviews', () => ({
  getInterviewQuestions: (...args: unknown[]) => getInterviewQuestions(...args),
}));

const completedInterview: Interview = {
  id: 3,
  targetJobId: 7,
  resumeId: 12,
  configSnapshot: {
    questionCount: 4,
    labelCounts: { '0': 2, '1': 1, '2': 1 },
    followUpBudget: 2,
    perQuestionFollowUpLimit: 1,
  },
  status: 'completed',
  overallGrade: 'B+',
  overallReview: '并发原语扎实，存储引擎底层需专项复习。',
  createdAt: '2026-11-02T10:00:00Z',
  updatedAt: '2026-11-02T10:34:00Z',
  completedAt: '2026-11-02T10:34:00Z',
};

function question(overrides: Partial<InterviewQuestion>): InterviewQuestion {
  return {
    id: 1,
    interviewId: 3,
    parentId: null,
    followUpRound: 0,
    label: '0',
    question: 'Go 的 channel 底层数据结构？',
    standardAnswer: 'hchan = 环形队列 + 等待队列 + lock。',
    userAnswer: 'hchan 结构体',
    extra: { mustPoints: ['hchan 结构组成', '发送阻塞条件'], bonusPoints: ['sudog 挂起与 gopark'] },
    grade: 'A',
    createdAt: '2026-11-02T10:01:00Z',
    ...overrides,
  };
}

afterEach(cleanup);

describe('InterviewReview', () => {
  it('renders the overall grade, the review text and the grade distribution', async () => {
    getInterviewQuestions.mockResolvedValue([
      question({ id: 1, grade: 'A' }),
      question({ id: 2, grade: 'B+', question: 'MySQL 索引为什么选 B+ 树？' }),
      question({ id: 3, grade: 'S', question: '三色标记如何解决漏标？' }),
      question({ id: 4, grade: 'C', userAnswer: '', question: 'RDB 与 AOF 如何取舍？' }),
    ]);

    render(<InterviewReview interview={completedInterview} onBack={vi.fn()} />);

    expect(await screen.findByText('总评等级')).toBeInTheDocument();
    expect(screen.getByText('B+', { selector: '.interview-hero__letter' })).toBeInTheDocument();
    expect(screen.getByText('并发原语扎实，存储引擎底层需专项复习。')).toBeInTheDocument();
    expect(screen.getByText('S × 1')).toBeInTheDocument();
    expect(screen.getByText('A × 1')).toBeInTheDocument();
    expect(screen.getByText('B+ × 1')).toBeInTheDocument();
    expect(screen.getByText('C × 1')).toBeInTheDocument();
  });

  it('shows must points, bonus points and follow-up chains per question', async () => {
    getInterviewQuestions.mockResolvedValue([
      question({ id: 1, grade: 'A' }),
      question({ id: 9, parentId: 1, followUpRound: 1, question: 'sendx / recvx 什么时候会阻塞？', userAnswer: '缓冲满时', grade: 'B+', standardAnswer: null, extra: null }),
    ]);

    render(<InterviewReview interview={completedInterview} onBack={vi.fn()} />);

    expect(await screen.findByText('必答点')).toBeInTheDocument();
    expect(screen.getByText('hchan 结构组成')).toBeInTheDocument();
    expect(screen.getByText('sudog 挂起与 gopark')).toBeInTheDocument();
    expect(screen.getByText('追问 1')).toBeInTheDocument();
    expect(screen.getByText('sendx / recvx 什么时候会阻塞？')).toBeInTheDocument();
    expect(screen.getByText('缓冲满时')).toBeInTheDocument();
  });

  it('renders the overall review markdown as bullets and bold', async () => {
    getInterviewQuestions.mockResolvedValue([question({ id: 1 })]);

    render(
      <InterviewReview
        interview={{
          ...completedInterview,
          overallReview: '结论：**并发扎实，存储层偏弱**。\n- 第 2 题只背了 **B+ 树** 结论\n- 第 4 题缺 **混合持久化**\n\n下一步：读 Redis 实战。',
        }}
        onBack={vi.fn()}
      />,
    );

    expect(await screen.findByText('混合持久化')).toBeInTheDocument();
    expect(screen.getByText('混合持久化').tagName).toBe('STRONG');
    // 紧贴段落的列表在 CommonMark 里会被吞成续行，这里必须解析成两条要点。
    expect(document.querySelectorAll('.interview-card__review li')).toHaveLength(2);
  });

  it('keeps inline code and headings instead of dropping their text', async () => {
    getInterviewQuestions.mockResolvedValue([question({ id: 1 })]);

    render(
      <InterviewReview
        interview={{
          ...completedInterview,
          overallReview: '用 `context.WithTimeout` 取消查询。\n\n## 短板',
        }}
        onBack={vi.fn()}
      />,
    );

    expect(await screen.findByText('context.WithTimeout')).toBeInTheDocument();
    expect(screen.getByText('context.WithTimeout').tagName).toBe('CODE');
    // 未列入白名单的标签要摊平成正文，不能让整句话凭空消失。
    expect(screen.getByText('短板')).toBeInTheDocument();
    expect(document.querySelector('.interview-card__review h2')).toBeNull();
  });

  it('marks skipped questions instead of pretending they were answered', async () => {
    getInterviewQuestions.mockResolvedValue([
      question({ id: 4, grade: 'C', userAnswer: '', question: 'RDB 与 AOF 如何取舍？' }),
    ]);

    render(<InterviewReview interview={completedInterview} onBack={vi.fn()} />);

    expect(await screen.findByText('已跳过')).toBeInTheDocument();
  });
});
