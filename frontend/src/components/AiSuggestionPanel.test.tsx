import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { streamRewrite } from '../api/ai';
import type { SSEEvent } from '../types/ai';
import type { ProjectSection, SummarySection } from '../types/resume';
import { AiSuggestionPanel } from './AiSuggestionPanel';

vi.mock('../api/ai', () => ({ streamRewrite: vi.fn() }));

const streamRewriteMock = vi.mocked(streamRewrite);

const summarySection: SummarySection = {
  id: 'summary-42',
  type: 'summary',
  title: '个人总结',
  visible: true,
  content: { markdown: '我负责产品交付。' },
};

const projectSection: ProjectSection = {
  id: 'project-42',
  type: 'project',
  title: '项目经历',
  visible: true,
  content: {
    items: [
      {
        id: 'project-alpha',
        name: '智能招聘平台',
        role: '项目负责人',
        startDate: '2024.01',
        endDate: '2024.09',
        location: '上海',
        link: 'https://example.com',
        introduction: '建设招聘平台。',
        achievements: '提高了效率。',
        highlightsMarkdown: '- 完成系统交付',
        showLocation: true,
        showLink: true,
        showIntroduction: true,
        showAchievements: true,
      },
    ],
  },
};

afterEach(() => {
  streamRewriteMock.mockReset();
  document.body.innerHTML = '';
});

describe('AiSuggestionPanel', () => {
  it('requests and renders only the suggestion correlated to the current section id and type', async () => {
    let emit: ((event: SSEEvent) => void) | undefined;
    streamRewriteMock.mockImplementation((_resumeId, _request, onEvent) => {
      emit = onEvent;
      return new Promise<void>(() => undefined);
    });

    render(
      <AiSuggestionPanel
        onAccept={vi.fn()}
        onClose={vi.fn()}
        originalSection={summarySection}
        resumeId={7}
        sectionType="summary"
      />,
    );

    expect(screen.getByText('理解上下文')).toBeInTheDocument();
    expect(screen.getByText('分析问题')).toBeInTheDocument();
    expect(screen.getByText('汇总建议')).toBeInTheDocument();
    act(() => {
      emit?.({ kind: 'thinking', text: '正在分析' });
    });
    expect(await screen.findByText('分析问题')).toBeInTheDocument();
    expect(screen.queryByText('正在分析')).not.toBeInTheDocument();
    act(() => {
      emit?.({
        kind: 'suggestion',
        sectionId: 'another-summary',
        sectionType: 'summary',
        newContent: { markdown: '不应显示的内容' },
      });
      emit?.({ kind: 'thinking', text: '岗位要求' });
    });
    expect(screen.queryByText('岗位要求')).not.toBeInTheDocument();
    act(() => {
      emit?.({
        kind: 'suggestion',
        sectionId: 'summary-42',
        sectionType: 'summary',
        newContent: { markdown: '主导产品交付，推动上线效率提升 30%。' },
      });
      emit?.({ kind: 'done' });
    });
    expect(await screen.findByText('主导产品交付，推动上线效率提升 30%。')).toBeInTheDocument();
    expect(screen.queryByText('不应显示的内容')).not.toBeInTheDocument();
    act(() => {
      emit?.({ kind: 'thinking', text: '过期分析' });
      emit?.({
        kind: 'suggestion',
        sectionId: 'summary-42',
        sectionType: 'summary',
        newContent: { markdown: '过期建议' },
      });
    });
    expect(screen.queryByText('过期建议')).not.toBeInTheDocument();
    expect(screen.getByText('主导产品交付，推动上线效率提升 30%。')).toBeInTheDocument();
    expect(streamRewriteMock).toHaveBeenCalledWith(
      7,
      { sectionId: 'summary-42', sectionType: 'summary' },
      expect.any(Function),
      expect.any(AbortSignal),
    );
    expect(screen.getByRole('button', { name: '采纳' })).toBeEnabled();
  });

  it('keeps frozen project fields neutral and highlights only changed rewriteable fields', async () => {
    streamRewriteMock.mockImplementation(async (_resumeId, _request, onEvent) => {
      onEvent({
        kind: 'suggestion',
        sectionId: 'project-42',
        sectionType: 'project',
        newContent: {
          items: [
            {
              id: 'project-alpha',
              introduction: '搭建面向校招的智能招聘平台。',
              achievements: '提高了效率。',
            },
            { id: 'unknown-item', introduction: '不应渲染的条目' },
          ],
        },
      });
      onEvent({ kind: 'done' });
    });

    render(
      <AiSuggestionPanel
        onAccept={vi.fn()}
        onClose={vi.fn()}
        originalSection={projectSection}
        resumeId={7}
        sectionType="project"
      />,
    );

    expect((await screen.findAllByText('分析问题')).length).toBeGreaterThan(0);
    expect(screen.queryByRole('group', { name: 'Frozen · 保持原样' })).not.toBeInTheDocument();
    expect(screen.getByText('「智能招聘平台」重写了项目介绍，突出业务目标和你的贡献。')).toBeInTheDocument();

    const rewriteable = screen.getByRole('group', { name: 'Rewriteable · AI 建议' });
    expect(within(rewriteable).getByText('建设招聘平台。')).toHaveClass('ai-suggestion-panel__old-value');
    expect(within(rewriteable).getByText('搭建面向校招的智能招聘平台。')).toHaveClass('ai-suggestion-panel__new-value');
    expect(within(rewriteable).getByText('提高了效率。')).toHaveClass('ai-suggestion-panel__unchanged-value');
    expect(screen.queryByText('不应渲染的条目')).not.toBeInTheDocument();
  });

  it('accepts the typed rewrite and rejects without changing it', async () => {
    const rewrite = { markdown: '量化产品交付成果。' };
    streamRewriteMock.mockImplementation(async (_resumeId, _request, onEvent) => {
      onEvent({ kind: 'suggestion', sectionId: 'summary-42', sectionType: 'summary', newContent: rewrite });
      onEvent({ kind: 'done' });
    });
    const onAccept = vi.fn();
    const onClose = vi.fn();
    const { rerender } = render(
      <AiSuggestionPanel
        onAccept={onAccept}
        onClose={onClose}
        originalSection={summarySection}
        resumeId={7}
        sectionType="summary"
      />,
    );

    fireEvent.click(await screen.findByRole('button', { name: '采纳' }));
    expect(onAccept).toHaveBeenCalledWith(rewrite);

    rerender(
      <AiSuggestionPanel
        onAccept={onAccept}
        onClose={onClose}
        originalSection={summarySection}
        resumeId={7}
        sectionType="summary"
      />,
    );
    fireEvent.click(screen.getByRole('button', { name: '拒绝' }));
    expect(onClose).toHaveBeenCalledOnce();
    expect(onAccept).toHaveBeenCalledOnce();
  });

  it('normalizes accepted project rewrites to the runtime allowlist while preserving explicit empty strings', async () => {
    streamRewriteMock.mockImplementation(async (_resumeId, _request, onEvent) => {
      onEvent({
        kind: 'suggestion',
        sectionId: 'project-42',
        sectionType: 'project',
        newContent: {
          items: [
            {
              id: 'project-alpha',
              introduction: '',
              achievements: '交付周期缩短 40%。',
              name: '恶意覆盖项目名',
              showIntroduction: false,
              arbitrary: '不在契约内',
            },
          ],
          title: '恶意覆盖章节标题',
        },
      } as unknown as SSEEvent);
      onEvent({ kind: 'done' });
    });
    const onAccept = vi.fn();

    render(
      <AiSuggestionPanel
        onAccept={onAccept}
        onClose={vi.fn()}
        originalSection={projectSection}
        resumeId={7}
        sectionType="project"
      />,
    );

    fireEvent.click(await screen.findByRole('button', { name: '采纳' }));
    expect(onAccept).toHaveBeenCalledWith({
      items: [{
        id: 'project-alpha',
        introduction: '',
        achievements: '交付周期缩短 40%。',
      }],
    });
  });

  it('keeps a completed suggestion ready when the reader fails after done', async () => {
    let rejectStream: ((reason: Error) => void) | undefined;
    streamRewriteMock.mockImplementation((_resumeId, _request, onEvent) => {
      onEvent({
        kind: 'suggestion',
        sectionId: 'summary-42',
        sectionType: 'summary',
        newContent: { markdown: '已完成的建议' },
      });
      onEvent({ kind: 'done' });
      return new Promise<void>((_resolve, reject) => {
        rejectStream = reject;
      });
    });

    render(
      <AiSuggestionPanel
        onAccept={vi.fn()}
        onClose={vi.fn()}
        originalSection={summarySection}
        resumeId={7}
        sectionType="summary"
      />,
    );
    expect(await screen.findByText('已完成的建议')).toBeInTheDocument();

    await act(async () => {
      rejectStream?.(new Error('reader failed after done'));
    });

    expect(screen.getByText('已完成的建议')).toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: '采纳' })).toBeEnabled();
  });

  it('aborts and invalidates the active request before invoking onAccept', async () => {
    let emit: ((event: SSEEvent) => void) | undefined;
    let requestSignal: AbortSignal | undefined;
    streamRewriteMock.mockImplementation((_resumeId, _request, onEvent, signal) => {
      emit = onEvent;
      requestSignal = signal;
      onEvent({
        kind: 'suggestion',
        sectionId: 'summary-42',
        sectionType: 'summary',
        newContent: { markdown: '可采纳建议' },
      });
      onEvent({ kind: 'done' });
      return new Promise<void>(() => undefined);
    });
    let callbackObservedAbort = false;
    const onAccept = vi.fn(() => {
      callbackObservedAbort = requestSignal?.aborted ?? false;
      emit?.({ kind: 'error', code: 'LATE', message: '采纳后的过期错误' });
    });

    render(
      <AiSuggestionPanel
        onAccept={onAccept}
        onClose={vi.fn()}
        originalSection={summarySection}
        resumeId={7}
        sectionType="summary"
      />,
    );
    fireEvent.click(await screen.findByRole('button', { name: '采纳' }));

    expect(onAccept).toHaveBeenCalledOnce();
    expect(callbackObservedAbort).toBe(true);
    expect(requestSignal?.aborted).toBe(true);
    expect(screen.queryByText('采纳后的过期错误')).not.toBeInTheDocument();
  });

  it.each([
    { action: '拒绝', accessibleName: '拒绝' },
    { action: '关闭', accessibleName: '关闭 AI 润色建议' },
  ])('aborts and invalidates before $action invokes onClose', async ({ accessibleName }) => {
    let emit: ((event: SSEEvent) => void) | undefined;
    let requestSignal: AbortSignal | undefined;
    streamRewriteMock.mockImplementation((_resumeId, _request, onEvent, signal) => {
      emit = onEvent;
      requestSignal = signal;
      return new Promise<void>(() => undefined);
    });
    let callbackObservedAbort = false;
    const onClose = vi.fn(() => {
      callbackObservedAbort = requestSignal?.aborted ?? false;
      emit?.({ kind: 'thinking', text: '关闭后的过期内容' });
    });

    render(
      <AiSuggestionPanel
        onAccept={vi.fn()}
        onClose={onClose}
        originalSection={summarySection}
        resumeId={7}
        sectionType="summary"
      />,
    );
    await waitFor(() => expect(streamRewriteMock).toHaveBeenCalledOnce());
    fireEvent.click(screen.getByRole('button', { name: accessibleName }));

    expect(onClose).toHaveBeenCalledOnce();
    expect(callbackObservedAbort).toBe(true);
    expect(requestSignal?.aborted).toBe(true);
    expect(screen.queryByText('关闭后的过期内容')).not.toBeInTheDocument();
  });

  it('aborts the previous request and ignores its late events when regenerating', async () => {
    const streams: Array<{ onEvent: (event: SSEEvent) => void; signal?: AbortSignal }> = [];
    streamRewriteMock.mockImplementation((_resumeId, _request, onEvent, signal) => {
      streams.push({ onEvent, signal });
      return new Promise<void>(() => undefined);
    });

    render(
      <AiSuggestionPanel
        onAccept={vi.fn()}
        onClose={vi.fn()}
        originalSection={summarySection}
        resumeId={7}
        sectionType="summary"
      />,
    );
    await waitFor(() => expect(streamRewriteMock).toHaveBeenCalledOnce());

    fireEvent.click(screen.getByRole('button', { name: '重新生成' }));
    await waitFor(() => expect(streamRewriteMock).toHaveBeenCalledTimes(2));
    expect(streams[0].signal?.aborted).toBe(true);

    act(() => {
      streams[0].onEvent({
        kind: 'suggestion',
        sectionId: 'summary-42',
        sectionType: 'summary',
        newContent: { markdown: '过期建议' },
      });
      streams[0].onEvent({ kind: 'done' });
      streams[1].onEvent({ kind: 'thinking', text: '新一轮分析' });
    });

    expect(screen.queryByText('过期建议')).not.toBeInTheDocument();
    expect(screen.getByText('正在分析问题')).toBeInTheDocument();
    expect(screen.queryByText('新一轮分析')).not.toBeInTheDocument();
  });

  it('shows stream errors and allows retrying', async () => {
    streamRewriteMock
      .mockRejectedValueOnce(new Error('网络中断'))
      .mockImplementationOnce(async (_resumeId, _request, onEvent) => {
        onEvent({ kind: 'error', code: 'INVALID_OUTPUT', message: 'AI 返回内容无效' });
        throw new Error('错误后的连接中断');
      });

    render(
      <AiSuggestionPanel
        onAccept={vi.fn()}
        onClose={vi.fn()}
        originalSection={summarySection}
        resumeId={7}
        sectionType="summary"
      />,
    );

    expect(await screen.findByRole('alert')).toHaveTextContent('网络中断');
    fireEvent.click(screen.getByRole('button', { name: '重新生成' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('AI 返回内容无效');
    expect(screen.getByRole('button', { name: '采纳' })).toBeDisabled();
  });

  it('traps focus, isolates the background, restores focus, and rejects on Escape', async () => {
    streamRewriteMock.mockImplementation(() => new Promise<void>(() => undefined));
    const background = document.createElement('main');
    const opener = document.createElement('button');
    opener.textContent = '打开面板';
    background.appendChild(opener);
    document.body.appendChild(background);
    opener.focus();
    const onClose = vi.fn();

    const { unmount } = render(
      <AiSuggestionPanel
        onAccept={vi.fn()}
        onClose={onClose}
        originalSection={summarySection}
        resumeId={7}
        sectionType="summary"
      />,
    );

    const dialog = screen.getByRole('dialog', { name: 'AI 润色建议' });
    expect(dialog).toContainElement(document.activeElement as HTMLElement);
    expect(background).toHaveAttribute('aria-hidden', 'true');
    expect(background).toHaveAttribute('inert');

    opener.focus();
    expect(dialog).toContainElement(document.activeElement as HTMLElement);
    const close = within(dialog).getByRole('button', { name: '关闭 AI 润色建议' });
    const accept = within(dialog).getByRole('button', { name: '采纳' });
    accept.focus();
    fireEvent.keyDown(dialog, { key: 'Tab' });
    expect(close).toHaveFocus();

    fireEvent.keyDown(dialog, { key: 'Escape' });
    expect(onClose).toHaveBeenCalledOnce();
    unmount();
    await act(async () => undefined);
    expect(background).not.toHaveAttribute('aria-hidden');
    expect(background).not.toHaveAttribute('inert');
    expect(opener).toHaveFocus();
  });
});
