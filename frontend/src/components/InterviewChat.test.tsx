import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { ApiError } from '../api/client';
import type { Interview, InterviewQuestion } from '../api/interviews';
import { InterviewChat } from './InterviewChat';

const getInterviewQuestions = vi.fn();
const answerInterview = vi.fn();

vi.mock('../api/interviews', () => ({
  getInterviewQuestions: (...args: unknown[]) => getInterviewQuestions(...args),
  answerInterview: (...args: unknown[]) => answerInterview(...args),
}));

const fetchQuestionAudio = vi.fn();
const transcribeRecording = vi.fn();

vi.mock('../api/voice', () => ({
  fetchQuestionAudio: (...args: unknown[]) => fetchQuestionAudio(...args),
  transcribeRecording: (...args: unknown[]) => transcribeRecording(...args),
}));

// 录音 hook 自己在 useVoiceRecorder.test.ts 里测；这里只关心它回调回来的识别文本。
const recorderOptions = vi.hoisted(() => ({
  current: null as { onText: (text: string) => void; onError: (message: string) => void } | null,
}));

vi.mock('../hooks/useVoiceRecorder', () => ({
  useVoiceRecorder: (options: { onText: (text: string) => void; onError: (message: string) => void }) => {
    recorderOptions.current = options;
    return { state: 'idle', seconds: 0, start: vi.fn(), stop: vi.fn() };
  },
}));

// jsdom 没有真正的媒体播放：Audio 与 URL.createObjectURL 都要打桩。
class FakeAudio {
  static instances: FakeAudio[] = [];
  /** 模拟浏览器拦截自动播放。 */
  static playBlocked = false;

  play = vi.fn(() => (FakeAudio.playBlocked
    ? Promise.reject(new Error('autoplay blocked'))
    : Promise.resolve()));

  addEventListener = vi.fn();

  constructor(public src: string) {
    FakeAudio.instances.push(this);
  }
}

const originalCreateObjectURL = (URL as unknown as { createObjectURL?: unknown }).createObjectURL;
const originalRevokeObjectURL = (URL as unknown as { revokeObjectURL?: unknown }).revokeObjectURL;

function stubAudio() {
  Object.defineProperty(URL, 'createObjectURL', {
    value: vi.fn(() => 'blob:mock'),
    configurable: true,
    writable: true,
  });
  Object.defineProperty(URL, 'revokeObjectURL', {
    value: vi.fn(),
    configurable: true,
    writable: true,
  });
  vi.stubGlobal('Audio', FakeAudio);
}

function restoreUrlMethod(name: 'createObjectURL' | 'revokeObjectURL', original: unknown) {
  if (original) {
    Object.defineProperty(URL, name, { value: original, configurable: true, writable: true });
  } else {
    delete (URL as unknown as Record<string, unknown>)[name];
  }
}
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
  vi.unstubAllGlobals();
  delete (Element.prototype as unknown as { scrollIntoView?: unknown }).scrollIntoView;
  restoreUrlMethod('createObjectURL', originalCreateObjectURL);
  restoreUrlMethod('revokeObjectURL', originalRevokeObjectURL);
  FakeAudio.instances = [];
  FakeAudio.playBlocked = false;
});

describe('InterviewChat', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // jsdom 没有 scrollIntoView，组件用它把当前题滚入视野。
    Element.prototype.scrollIntoView = vi.fn();
    stubAudio();
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

  it('keeps standard answers and scoring points out of the live interview', async () => {
    getInterviewQuestions.mockResolvedValue([
      makeQuestion({ id: 1, userAnswer: '答过了', grade: 'B' }),
      makeQuestion({ id: 2, question: '当前题：Redis 持久化怎么选？', userAnswer: null, grade: null }),
    ]);

    render(<InterviewChat interview={interview} onFinished={vi.fn()} onExit={vi.fn()} />);
    await screen.findByText('当前题：Redis 持久化怎么选？');

    expect(screen.queryByText('查看标准答案')).not.toBeInTheDocument();
    expect(screen.queryByText(/环形队列/)).not.toBeInTheDocument();
    expect(screen.queryByText('hchan 结构组成')).not.toBeInTheDocument();
  });

  it('shows the interviewer typing indicator while the answer is graded', async () => {
    let release: (value: unknown) => void = () => {};
    getInterviewQuestions.mockResolvedValue([makeQuestion({ id: 1, userAnswer: null, grade: null })]);
    answerInterview.mockReturnValue(new Promise((resolve) => { release = resolve; }));

    render(<InterviewChat interview={interview} onFinished={vi.fn()} onExit={vi.fn()} />);
    fireEvent.change(await screen.findByLabelText('回答输入'), { target: { value: '回答' } });
    fireEvent.click(screen.getByRole('button', { name: '提交回答' }));

    expect(await screen.findByText(/面试官正在评分/)).toBeInTheDocument();
    expect(screen.getByText(/约需 3~8 秒/)).toBeInTheDocument();
    expect(screen.getByLabelText('回答输入')).toHaveAttribute('readonly');
    release({ grade: 'B', followUp: '' });
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

  it('shows the backend request id when evaluation fails', async () => {
    // 5xx 时后端会带 request_id：用户报障时报这个编号就能定位到后端日志。
    getInterviewQuestions.mockResolvedValue([makeQuestion({ id: 1, userAnswer: null, grade: null })]);
    answerInterview.mockRejectedValueOnce(new ApiError('interview operation failed', 500, 'abc123def456'));

    render(<InterviewChat interview={interview} onFinished={vi.fn()} onExit={vi.fn()} />);
    fireEvent.change(await screen.findByLabelText('回答输入'), { target: { value: '回答' } });
    fireEvent.click(screen.getByRole('button', { name: '提交回答' }));

    expect(await screen.findByRole('alert')).toHaveTextContent('abc123def456');
  });

  it('surfaces an evaluation failure and allows retrying', async () => {
    getInterviewQuestions.mockResolvedValue([makeQuestion({ id: 1, userAnswer: null, grade: null })]);
    answerInterview.mockRejectedValueOnce(Object.assign(new Error('boom'), { name: 'ApiError', status: 502 }));

    render(<InterviewChat interview={interview} onFinished={vi.fn()} onExit={vi.fn()} />);
    fireEvent.change(await screen.findByLabelText('回答输入'), { target: { value: '回答' } });
    fireEvent.click(screen.getByRole('button', { name: '提交回答' }));

    expect(await screen.findByRole('alert')).toHaveTextContent('评分失败，请重试');
    expect(screen.getByRole('button', { name: '提交回答' })).toBeEnabled();
    expect(screen.getByLabelText('回答输入')).not.toHaveAttribute('readonly');
  });

  it('hides voice controls when voice is disabled', async () => {
    getInterviewQuestions.mockResolvedValue([makeQuestion({ id: 1 })]);
    render(
      <InterviewChat
        interview={{ ...interview, voice: { enabled: false } }}
        onFinished={vi.fn()}
        onExit={vi.fn()}
      />,
    );

    expect(await screen.findByText('Go 的 channel 底层数据结构是怎样的？')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /朗读题目/ })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /录音回答/ })).not.toBeInTheDocument();
  });

  it('shows a play button when the question audio is ready', async () => {
    getInterviewQuestions.mockResolvedValue([makeQuestion({ id: 1, audioState: 'ready' })]);
    render(
      <InterviewChat
        interview={{ ...interview, voice: { enabled: true } }}
        onFinished={vi.fn()}
        onExit={vi.fn()}
      />,
    );

    expect(await screen.findByRole('button', { name: /朗读题目/ })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /录音回答/ })).toBeInTheDocument();
  });

  it('shows a generating hint while the audio is pending', async () => {
    getInterviewQuestions.mockResolvedValue([makeQuestion({ id: 1, audioState: 'pending' })]);
    render(
      <InterviewChat
        interview={{ ...interview, voice: { enabled: true } }}
        onFinished={vi.fn()}
        onExit={vi.fn()}
      />,
    );

    expect(await screen.findByText('语音生成中…')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /朗读题目/ })).not.toBeInTheDocument();
  });

  it('inserts the recognised text at the cursor position', async () => {
    getInterviewQuestions.mockResolvedValue([makeQuestion({ id: 1, audioState: 'ready' })]);
    render(
      <InterviewChat
        interview={{ ...interview, voice: { enabled: true } }}
        onFinished={vi.fn()}
        onExit={vi.fn()}
      />,
    );

    const area = await screen.findByLabelText('回答输入') as HTMLTextAreaElement;
    fireEvent.change(area, { target: { value: '前缀后缀' } });
    area.setSelectionRange(2, 2);
    act(() => {
      recorderOptions.current?.onText('中间');
    });

    await waitFor(() => expect(screen.getByLabelText('回答输入')).toHaveValue('前缀中间后缀'));
    // 光标要落在插入文本之后（前 2 字 + 插入 2 字 = 4）。
    await waitFor(() => expect(area.selectionStart).toBe(4));
  });
  it('auto plays the current question when its audio is ready', async () => {
    getInterviewQuestions.mockResolvedValue([makeQuestion({ id: 7, audioState: 'ready' })]);
    render(
      <InterviewChat
        interview={{ ...interview, voice: { enabled: true } }}
        onFinished={vi.fn()}
        onExit={vi.fn()}
      />,
    );

    await waitFor(() => expect(fetchQuestionAudio).toHaveBeenCalledTimes(1));
    expect(fetchQuestionAudio).toHaveBeenCalledWith(3, 7);
    expect(FakeAudio.instances).toHaveLength(1);
  });

  it('does not request audio when voice is disabled', async () => {
    getInterviewQuestions.mockResolvedValue([makeQuestion({ id: 1, audioState: 'ready' })]);
    render(
      <InterviewChat
        interview={{ ...interview, voice: { enabled: false } }}
        onFinished={vi.fn()}
        onExit={vi.fn()}
      />,
    );

    expect(await screen.findByText('Go 的 channel 底层数据结构是怎样的？')).toBeInTheDocument();
    expect(fetchQuestionAudio).not.toHaveBeenCalled();
  });

  it('does not request audio when the question audio has failed', async () => {
    getInterviewQuestions.mockResolvedValue([makeQuestion({ id: 1, audioState: 'failed' })]);
    render(
      <InterviewChat
        interview={{ ...interview, voice: { enabled: true } }}
        onFinished={vi.fn()}
        onExit={vi.fn()}
      />,
    );

    expect(await screen.findByText('Go 的 channel 底层数据结构是怎样的？')).toBeInTheDocument();
    expect(fetchQuestionAudio).not.toHaveBeenCalled();
    expect(screen.queryByRole('button', { name: /朗读题目/ })).not.toBeInTheDocument();
  });

  it('gives the follow-up question its own speaker button', async () => {
    getInterviewQuestions.mockResolvedValue([
      makeQuestion({ id: 1, userAnswer: '答过了', grade: 'B' }),
      makeQuestion({
        id: 9,
        parentId: 1,
        followUpRound: 1,
        question: '追问：ctx.Err() 怎么区分？',
        audioState: 'ready',
      }),
    ]);
    render(
      <InterviewChat
        interview={{ ...interview, voice: { enabled: true } }}
        onFinished={vi.fn()}
        onExit={vi.fn()}
      />,
    );

    const followUpButton = await screen.findByRole('button', { name: '朗读题目 9' });
    fetchQuestionAudio.mockClear();
    fireEvent.click(followUpButton);

    await waitFor(() => expect(fetchQuestionAudio).toHaveBeenCalledWith(3, 9));
  });

  it('asks the user to click the speaker when autoplay is blocked', async () => {
    FakeAudio.playBlocked = true;
    getInterviewQuestions.mockResolvedValue([makeQuestion({ id: 1, audioState: 'ready' })]);
    render(
      <InterviewChat
        interview={{ ...interview, voice: { enabled: true } }}
        onFinished={vi.fn()}
        onExit={vi.fn()}
      />,
    );

    expect(await screen.findByText('点击喇叭播放本题')).toBeInTheDocument();
  });

  it('dismisses the transcription failure notice automatically', async () => {
    vi.useFakeTimers();
    try {
      getInterviewQuestions.mockResolvedValue([makeQuestion({ id: 1, audioState: 'ready' })]);
      render(
        <InterviewChat
          interview={{ ...interview, voice: { enabled: true } }}
          onFinished={vi.fn()}
          onExit={vi.fn()}
        />,
      );
      await act(async () => {
        await vi.advanceTimersByTimeAsync(0);
      });

      act(() => recorderOptions.current?.onError('识别失败，可继续打字'));
      expect(screen.getByText('识别失败，可继续打字')).toBeInTheDocument();

      // 提示只是辅助信息，几秒后必须自己消失，不能一直挂在页面上。
      await act(async () => {
        await vi.advanceTimersByTimeAsync(4500);
      });
      expect(screen.queryByText('识别失败，可继续打字')).not.toBeInTheDocument();
    } finally {
      vi.useRealTimers();
    }
  });

  it('limits polling when the question carries no audio state', async () => {
    vi.useFakeTimers();
    try {
      fetchQuestionAudio.mockRejectedValue(Object.assign(new Error('pending'), { status: 409 }));
      getInterviewQuestions.mockResolvedValue([makeQuestion({ id: 1 })]);
      render(
        <InterviewChat
          interview={{ ...interview, voice: { enabled: true } }}
          onFinished={vi.fn()}
          onExit={vi.fn()}
        />,
      );

      await act(async () => {
        await vi.advanceTimersByTimeAsync(0);
      });
      expect(fetchQuestionAudio).toHaveBeenCalledTimes(1);

      await act(async () => {
        await vi.advanceTimersByTimeAsync(10_000);
      });
      // 首次请求 + 最多 3 次轮询。
      expect(fetchQuestionAudio).toHaveBeenCalledTimes(4);
    } finally {
      vi.useRealTimers();
    }
  });
});