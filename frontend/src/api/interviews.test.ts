import { afterEach, describe, expect, it, vi } from 'vitest';

import {
  abortInterview,
  answerInterview,
  createInterview,
  getInterview,
  getInterviewQuestions,
  listInterviews,
  type Interview,
} from './interviews';

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

afterEach(() => vi.unstubAllGlobals());

describe('interviews API', () => {
  it('creates an interview with a JSON POST body and omits an absent config', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ interview, firstQuestion: null }), { status: 201 }),
    );
    vi.stubGlobal('fetch', fetchMock);

    await expect(createInterview(7, 12)).resolves.toEqual({ interview, firstQuestion: null });

    expect(fetchMock).toHaveBeenCalledWith('/api/interviews', expect.objectContaining({
      method: 'POST',
      credentials: 'include',
      body: JSON.stringify({ targetJobId: 7, resumeId: 12 }),
      headers: expect.objectContaining({ 'Content-Type': 'application/json' }),
    }));
  });

  it('includes the interview config in the create body when provided', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ interview, firstQuestion: null }), { status: 201 }),
    );
    vi.stubGlobal('fetch', fetchMock);
    const config = {
      questionCount: 8,
      labelCounts: { '0': 4, '1': 1, '2': 3 },
      followUpBudget: 4,
      perQuestionFollowUpLimit: 1,
    };

    await createInterview(7, 12, config);

    expect(fetchMock).toHaveBeenCalledWith('/api/interviews', expect.objectContaining({
      body: JSON.stringify({ targetJobId: 7, resumeId: 12, config }),
    }));
  });

  it('lists interviews with an authenticated GET request', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify([interview]), { status: 200 }));
    vi.stubGlobal('fetch', fetchMock);

    await expect(listInterviews()).resolves.toEqual([interview]);

    expect(fetchMock).toHaveBeenCalledWith('/api/interviews', expect.objectContaining({
      method: 'GET', credentials: 'include',
    }));
  });

  it('gets one interview with an authenticated GET request', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(interview), { status: 200 }));
    vi.stubGlobal('fetch', fetchMock);

    await expect(getInterview(3)).resolves.toEqual(interview);

    expect(fetchMock).toHaveBeenCalledWith('/api/interviews/3', expect.objectContaining({
      method: 'GET', credentials: 'include',
    }));
  });

  it('answers a question with the answer text in the body', async () => {
    const result = { grade: 'B+', followUp: '', nextQuestion: null, finished: false };
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(result), { status: 200 }));
    vi.stubGlobal('fetch', fetchMock);

    await expect(answerInterview(3, 'hchan 结构体')).resolves.toEqual(result);

    expect(fetchMock).toHaveBeenCalledWith('/api/interviews/3/answer', expect.objectContaining({
      method: 'POST',
      credentials: 'include',
      body: JSON.stringify({ answer: 'hchan 结构体' }),
      headers: expect.objectContaining({ 'Content-Type': 'application/json' }),
    }));
  });

  it('sends a skip flag instead of an answer when the user skips', async () => {
    const result = { grade: 'C', skip: true, followUp: '', nextQuestion: null, finished: false };
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(result), { status: 200 }));
    vi.stubGlobal('fetch', fetchMock);

    await expect(answerInterview(3, undefined, true)).resolves.toEqual(result);

    expect(fetchMock).toHaveBeenCalledWith('/api/interviews/3/answer', expect.objectContaining({
      body: JSON.stringify({ skip: true }),
    }));
  });

  it('lists all questions of one interview for the review view', async () => {
    const questions = [{
      id: 11, interviewId: 3, parentId: null, followUpRound: 0, label: 0,
      question: 'Go 的 channel 底层数据结构？', standardAnswer: 'hchan',
      userAnswer: 'hchan 加环形队列', extra: { mustPoints: ['hchan'], bonusPoints: [] },
      grade: 'A', createdAt: '2026-11-02T10:01:00Z',
    }];
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(questions), { status: 200 }));
    vi.stubGlobal('fetch', fetchMock);

    await expect(getInterviewQuestions(3)).resolves.toEqual(questions);

    expect(fetchMock).toHaveBeenCalledWith('/api/interviews/3/questions', expect.objectContaining({
      method: 'GET', credentials: 'include',
    }));
  });

  it('aborts an active interview and returns the updated record', async () => {
    const aborted = { ...interview, status: 'aborted' as const };
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(aborted), { status: 200 }));
    vi.stubGlobal('fetch', fetchMock);

    await expect(abortInterview(3)).resolves.toEqual(aborted);

    expect(fetchMock).toHaveBeenCalledWith('/api/interviews/3/abort', expect.objectContaining({
      method: 'POST', credentials: 'include',
    }));
  });

  it('propagates API errors from failed interview requests', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ error: 'active interview already exists' }), { status: 409 })));

    await expect(createInterview(7, 12)).rejects.toMatchObject({
      name: 'ApiError', message: 'active interview already exists', status: 409,
    });
  });
});
