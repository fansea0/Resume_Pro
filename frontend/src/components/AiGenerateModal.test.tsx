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
  document.querySelectorAll('[data-ai-generate-test-background]').forEach((element) => element.remove());
  vi.useRealTimers();
});

describe('AiGenerateModal', () => {
  it('renders only while open and keeps validation errors linked until each field is corrected', async () => {
    const { rerender } = render(<AiGenerateModal onClose={vi.fn()} onCreated={vi.fn()} open={false} />);
    expect(screen.queryByRole('dialog', { name: 'AI 快速生成' })).not.toBeInTheDocument();

    rerender(<AiGenerateModal onClose={vi.fn()} onCreated={vi.fn()} open />);
    const dialog = screen.getByRole('dialog', { name: 'AI 快速生成' });
    const rawTextInput = within(dialog).getByLabelText('你的经历与优势');
    const templateSelect = within(dialog).getByLabelText('简历模板');
    fireEvent.change(rawTextInput, { target: { value: '   字数太少   ' } });
    fireEvent.click(within(dialog).getByRole('button', { name: '开始生成' }));

    const rawTextError = within(dialog).getByText('你的经历与优势至少 30 个字符。');
    const templateError = within(dialog).getByText('请选择简历模板。');
    expect(rawTextInput).toHaveAttribute('aria-invalid', 'true');
    expect(rawTextInput.getAttribute('aria-describedby')).toContain(rawTextError.id);
    expect(templateSelect).toHaveAttribute('aria-invalid', 'true');
    expect(templateSelect.getAttribute('aria-describedby')).toContain(templateError.id);
    expect(streamGenerateMock).not.toHaveBeenCalled();

    fireEvent.change(rawTextInput, { target: { value: rawText } });
    expect(within(dialog).queryByText('你的经历与优势至少 30 个字符。')).not.toBeInTheDocument();
    expect(within(dialog).getByText('请选择简历模板。')).toBeInTheDocument();

    fireEvent.change(templateSelect, { target: { value: 'business' } });
    expect(within(dialog).queryByText('请选择简历模板。')).not.toBeInTheDocument();
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
    }, expect.any(AbortSignal));
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

  it('hides and inerts app-level background content, then restores its previous state', () => {
    const appBackground = document.createElement('main');
    appBackground.dataset.aiGenerateTestBackground = '';
    appBackground.setAttribute('aria-hidden', 'false');
    document.body.appendChild(appBackground);

    const { unmount } = render(<AiGenerateModal onClose={vi.fn()} onCreated={vi.fn()} open />);

    expect(appBackground).toHaveAttribute('aria-hidden', 'true');
    expect(appBackground).toHaveAttribute('inert');
    unmount();
    expect(appBackground).toHaveAttribute('aria-hidden', 'false');
    expect(appBackground).not.toHaveAttribute('inert');
    appBackground.remove();
  });

  it('moves focus to the completed draft and announces that it is ready', async () => {
    const generated = generatedDocument();
    streamGenerateMock.mockImplementation(async (_request, onEvent) => {
      onEvent({ kind: 'suggestion', newContent: generated });
      onEvent({ kind: 'done' });
    });
    render(<AiGenerateModal onClose={vi.fn()} onCreated={vi.fn()} open />);
    fillRequiredFields();

    fireEvent.click(screen.getByRole('button', { name: '开始生成' }));

    const heading = await screen.findByRole('heading', { name: generated.title });
    await waitFor(() => expect(heading).toHaveFocus());
    expect(screen.getByRole('status')).toHaveTextContent('简历初稿已生成，可以应用到新建简历');
  });

  it('keeps cancel and close focusable while saving and aborts without stale callbacks', async () => {
    const generated = generatedDocument();
    streamGenerateMock.mockImplementation(async (_request, onEvent) => {
      onEvent({ kind: 'suggestion', newContent: generated });
      onEvent({ kind: 'done' });
    });
    let saveSignal: AbortSignal | undefined;
    createResumeMock.mockImplementation((_document, signal) => {
      saveSignal = signal;
      return new Promise((_resolve, reject) => {
        signal?.addEventListener('abort', () => reject(new DOMException('aborted', 'AbortError')), { once: true });
      });
    });
    const onClose = vi.fn();
    const onCreated = vi.fn();
    render(<AiGenerateModal onClose={onClose} onCreated={onCreated} open />);
    fillRequiredFields();
    fireEvent.click(screen.getByRole('button', { name: '开始生成' }));
    fireEvent.click(await screen.findByRole('button', { name: '应用到新建简历' }));

    expect(screen.getByRole('button', { name: '取消' })).toBeEnabled();
    expect(screen.getByRole('button', { name: '关闭 AI 快速生成' })).toBeEnabled();
    fireEvent.click(screen.getByRole('button', { name: '取消' }));

    expect(saveSignal?.aborted).toBe(true);
    expect(onClose).toHaveBeenCalledOnce();
    await act(async () => Promise.resolve());
    expect(onCreated).not.toHaveBeenCalled();
    expect(createResumeMock).toHaveBeenCalledOnce();
  });

  it('aborts a timed-out create and returns to a retryable ready state', async () => {
    const generated = generatedDocument();
    streamGenerateMock.mockImplementation(async (_request, onEvent) => {
      onEvent({ kind: 'suggestion', newContent: generated });
      onEvent({ kind: 'done' });
    });
    let saveSignal: AbortSignal | undefined;
    createResumeMock.mockImplementation((_document, signal) => {
      saveSignal = signal;
      return new Promise((_resolve, reject) => {
        signal?.addEventListener('abort', () => reject(new DOMException('aborted', 'AbortError')), { once: true });
      });
    });
    render(<AiGenerateModal onClose={vi.fn()} onCreated={vi.fn()} open />);
    fillRequiredFields();
    fireEvent.click(screen.getByRole('button', { name: '开始生成' }));
    const apply = await screen.findByRole('button', { name: '应用到新建简历' });
    vi.useFakeTimers();
    fireEvent.click(apply);

    await act(async () => {
      vi.advanceTimersByTime(15_000);
      await Promise.resolve();
    });

    expect(saveSignal?.aborted).toBe(true);
    expect(screen.getByRole('alert')).toHaveTextContent('保存超时');
    expect(screen.getByRole('button', { name: '应用到新建简历' })).toBeEnabled();
  });
});
