import { apiFetch } from './client';
import type { AudioState } from './voice';

export type InterviewGrade = 'C' | 'B-' | 'B' | 'B+' | 'A' | 'S';
export type InterviewStatus = 'active' | 'completed' | 'aborted';

/** 与后端 QuestionLabel 枚举一致：0=八股 1=算法 2=项目。 */
export type InterviewLabel = '0' | '1' | '2';

export interface InterviewConfig {
  questionCount: number;
  labelCounts: Record<InterviewLabel, number>;
  followUpBudget: number;
  perQuestionFollowUpLimit: number;
}

export interface Interview {
  id: number;
  targetJobId: number;
  resumeId: number;
  configSnapshot: InterviewConfig;
  status: InterviewStatus;
  /** 语音能力开关；缺省表示后端未启用语音。 */
  voice?: { enabled: boolean };
  overallGrade?: InterviewGrade | null;
  overallReview?: string | null;
  createdAt: string;
  updatedAt: string;
  completedAt?: string | null;
}

export interface InterviewQuestionExtra {
  mustPoints: string[];
  bonusPoints: string[];
}

export interface InterviewQuestion {
  id: number;
  interviewId: number;
  parentId?: number | null;
  followUpRound: number;
  label: InterviewLabel;
  question: string;
  standardAnswer?: string | null;
  userAnswer?: string | null;
  extra?: InterviewQuestionExtra | null;
  grade?: InterviewGrade | null;
  /** 题目语音状态；缺省表示后端未启用语音。 */
  audioState?: AudioState | null;
  createdAt: string;
}

export interface CreateInterviewResult {
  interview: Interview;
  firstQuestion: InterviewQuestion | null;
}

export interface AnswerResult {
  grade: InterviewGrade;
  skip?: boolean;
  followUp?: string;
  nextQuestion?: InterviewQuestion | null;
  finished?: boolean;
  overallGrade?: InterviewGrade;
  overallReview?: string;
}

export function createInterview(targetJobId: number, resumeId: number, config?: InterviewConfig): Promise<CreateInterviewResult> {
  return apiFetch<CreateInterviewResult>('/api/interviews', {
    method: 'POST',
    body: JSON.stringify(config ? { targetJobId, resumeId, config } : { targetJobId, resumeId }),
  });
}

export function listInterviews(): Promise<Interview[]> {
  return apiFetch<Interview[]>('/api/interviews', { method: 'GET' });
}

export function getInterview(id: number): Promise<Interview> {
  return apiFetch<Interview>(`/api/interviews/${id}`, { method: 'GET' });
}

export function answerInterview(id: number, answer?: string, skip?: boolean): Promise<AnswerResult> {
  const body = skip ? { skip: true } : { answer };
  return apiFetch<AnswerResult>(`/api/interviews/${id}/answer`, {
    method: 'POST',
    body: JSON.stringify(body),
  });
}

export function getInterviewQuestions(id: number): Promise<InterviewQuestion[]> {
  return apiFetch<InterviewQuestion[]>(`/api/interviews/${id}/questions`, { method: 'GET' });
}

export function abortInterview(id: number): Promise<Interview> {
  return apiFetch<Interview>(`/api/interviews/${id}/abort`, { method: 'POST' });
}
