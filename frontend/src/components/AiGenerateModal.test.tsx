import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { streamGenerate } from '../api/ai';
import { createResume } from '../api/resumes';
import { getTargetJob, listTargetJobs } from '../api/targetJobs';
import type { GenerateSSEEvent } from '../types/ai';
import { createSampleResumeDocument } from '../types/resume';
import type { TargetJob } from '../types/targetJob';
import { AiGenerateModal } from './AiGenerateModal';

vi.mock('../api/ai', () => ({ streamGenerate: vi.fn() }));
vi.mock('../api/resumes', async (importOriginal) => {
  const original = await importOriginal<typeof import('../api/resumes')>();
  return { ...original, createResume: vi.fn() };
});
vi.mock('../api/targetJobs', () => ({
  createTargetJob: vi.fn(),
  getTargetJob: vi.fn(),
  listTargetJobs: vi.fn(),
}));

const streamGenerateMock = vi.mocked(streamGenerate);
const createResumeMock = vi.mocked(createResume);
const getTargetJobMock = vi.mocked(getTargetJob);
const listTargetJobsMock = vi.mocked(listTargetJobs);

const rawText = '我有五年 Go 后端经验，负责过支付平台、稳定性治理与跨团队项目交付。';
const targetJob: TargetJob = {
  id: 7,
  company: '星海科技',
  position: '高级后端工程师',
  requirements: '负责交易平台架构设计、稳定性治理和研发效能提升。',
  createdAt: '2026-08-18T01:00:00Z',
  updatedAt: '2026-08-19T01:00:00Z',
};

function fillRequiredFields(template = 'business') {
  fireEvent.change(screen.getByLabelText('你的经历与优势'), { target: { value: `  ${rawText}  ` } });
  fireEvent.change(screen.getByLabelText('简历模板'), { target: { value: template } });
}

function generatedDocument() {
  const document = createSampleResumeDocument();
  return { ...document, title: '张三｜高级后端工程师', templateId: 'minimal' as const };
}

beforeEach(() => {
  vi.clearAllMocks();
  listTargetJobsMock.mockResolvedValue([targetJob]);
  getTargetJobMock.mockResolvedValue(targetJob);
  streamGenerateMock.mockImplementation(() => new Promise<void>(() => undefined));
});

afterEach(() => {
  cleanup();
});

describe('AiGenerateModal', () => {
  it('renders only while open and validates trimmed raw text and template selection', async () => {
    const { rerender } = render(<AiGenerateModal onClose={vi.fn()} onCreated={vi.fn()} open={false} />);
    expect(screen.queryByRole('dialog', { name: 'AI 快速生成' })).not.toBeInTheDocument();

    rerender(<AiGenerateModal onClose={vi.fn()} onCreated={vi.fn()} open />);
    const dialog = screen.getByRole('dialog', { name: 'AI 快速生成' });
    fireEvent.change(within(dialog).getByLabelText('你的经历与优势'), { target: { value: '   字数太少   ' } });
    fireEvent.click(within(dialog).getByRole('button', { name: '开始生成' }));

    expect(within(dialog).getByRole('alert')).toHaveTextContent('至少 30 个字符');
    expect(streamGenerateMock).not.toHaveBeenCalled();

    fireEvent.change(within(dialog).getByLabelText('你的经历与优势'), { target: { value: rawText } });
    fireEvent.click(within(dialog).getByRole('button', { name: '开始生成' }));
    expect(within(dialog).getByRole('alert')).toHaveTextContent('请选择简历模板');
    expect(streamGenerateMock).not.toHaveBeenCalled();
  });

  it('selects an existing target job through the picker and sends the normalized request', async () => {
    render(<AiGenerateModal onClose={vi.fn()} onCreated={vi.fn()} open />);
    fillRequiredFields();
    fireEvent.click(screen.getByRole('button', { name: '选择目标岗位' }));

    const picker = await screen.findByRole('dialog', { name: '选择目标岗位' });
    fireEvent.click(await within(picker).findByRole('button', { name: '选择：星海科技 · 高级后端工程师' }));
    expect(await screen.findByText('星海科技 · 高级后端工程师')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: '开始生成' }));

    expect(streamGenerateMock).toHaveBeenCalledWith({
      rawText,
      targetJobId: 7,
      templateId: 'business',
    }, expect.any(Function), expect.any(AbortSignal));
  });

  it('renders streamed thinking and exposes an explicit retry for the backend placeholder error', async () => {
    let emit: ((event: GenerateSSEEvent) => void) | undefined;
    streamGenerateMock
      .mockImplementationOnce((_request, onEvent) => {
        emit = onEvent;
        onEvent({ kind: 'thinking', text: '正在梳理项目经历…' });
        return new Promise<void>(() => undefined);
      })
      .mockImplementationOnce(() => new Promise<void>(() => undefined));
    render(<AiGenerateModal onClose={vi.fn()} onCreated={vi.fn()} open />);
    fillRequiredFields();

    fireEvent.click(screen.getByRole('button', { name: '开始生成' }));
    expect(await screen.findByText('正在梳理项目经历…')).toBeInTheDocument();
    act(() => {
      emit?.({ kind: 'error', code: 'NOT_IMPLEMENTED', message: 'from-scratch generate is TODO' });
    });
    expect(await screen.findByRole('alert')).toHaveTextContent('AI 快速生成正在准备中');
    expect(streamGenerateMock).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByRole('button', { name: '重试生成' }));
    expect(streamGenerateMock).toHaveBeenCalledTimes(2);
    expect(screen.getByText('正在生成简历…')).toBeInTheDocument();
  });

  it('validates the completed document, applies selected metadata, and creates exactly one resume', async () => {
    const generated = generatedDocument();
    streamGenerateMock.mockImplementation(async (_request, onEvent) => {
      onEvent({ kind: 'suggestion', newContent: generated });
      onEvent({ kind: 'done' });
    });
    createResumeMock.mockResolvedValue({
      id: 42,
      title: generated.title,
      templateId: 'business',
      content: JSON.stringify(generated),
      createdAt: '2026-08-19T01:00:00Z',
      updatedAt: '2026-08-19T01:00:00Z',
    });
    const onClose = vi.fn();
    const onCreated = vi.fn();
    render(<AiGenerateModal onClose={onClose} onCreated={onCreated} open />);
    fillRequiredFields();
    fireEvent.click(screen.getByRole('button', { name: '选择目标岗位' }));
    const picker = await screen.findByRole('dialog', { name: '选择目标岗位' });
    fireEvent.click(await within(picker).findByRole('button', { name: '选择：星海科技 · 高级后端工程师' }));
    await screen.findByText('星海科技 · 高级后端工程师');

    fireEvent.click(screen.getByRole('button', { name: '开始生成' }));
    expect(await screen.findByText(generated.title)).toBeInTheDocument();
    const apply = screen.getByRole('button', { name: '应用到新建简历' });
    fireEvent.click(apply);
    fireEvent.click(apply);

    await waitFor(() => expect(createResumeMock).toHaveBeenCalledOnce());
    expect(createResumeMock).toHaveBeenCalledWith({
      ...generated,
      templateId: 'business',
      targetJobId: 7,
    });
    await waitFor(() => expect(onCreated).toHaveBeenCalledWith(42));
    expect(onCreated).toHaveBeenCalledOnce();
    expect(onClose).toHaveBeenCalledOnce();
  });

  it('rejects malformed generated content and ignores stale stream events after close', async () => {
    let emit: ((event: GenerateSSEEvent) => void) | undefined;
    let streamSignal: AbortSignal | undefined;
    streamGenerateMock.mockImplementation((_request, onEvent, signal) => {
      emit = onEvent;
      streamSignal = signal;
      return new Promise<void>(() => undefined);
    });
    const onClose = vi.fn();
    const { rerender } = render(<AiGenerateModal onClose={onClose} onCreated={vi.fn()} open />);
    fillRequiredFields();
    fireEvent.click(screen.getByRole('button', { name: '开始生成' }));

    act(() => {
      emit?.({ kind: 'suggestion', newContent: { schemaVersion: 2, title: '伪造简历' } });
      emit?.({ kind: 'done' });
    });
    expect(await screen.findByRole('alert')).toHaveTextContent('格式无效');
    expect(screen.queryByRole('button', { name: '应用到新建简历' })).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: '取消' }));
    expect(onClose).toHaveBeenCalledOnce();
    rerender(<AiGenerateModal onClose={onClose} onCreated={vi.fn()} open={false} />);
    expect(streamSignal?.aborted).toBe(true);
    act(() => {
      emit?.({ kind: 'suggestion', newContent: generatedDocument() });
      emit?.({ kind: 'done' });
    });
    expect(screen.queryByText('张三｜高级后端工程师')).not.toBeInTheDocument();
  });
});
