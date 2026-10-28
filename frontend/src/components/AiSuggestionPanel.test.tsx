import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { streamRewrite } from '../api/ai';
import type { AiSuggestion, OptimizationAnalysis, SSEEvent } from '../types/ai';
import type { InternshipSection, ProjectSection, SummarySection, WorkSection } from '../types/resume';
import { AiSuggestionPanel } from './AiSuggestionPanel';

vi.mock('../api/ai', () => ({ streamRewrite: vi.fn() }));

const streamRewriteMock = vi.mocked(streamRewrite);

const emptyAnalysis: OptimizationAnalysis = {
  jdAlignment: { status: 'skipped' },
  suggestions: [],
  evidenceRequests: [],
  warnings: [],
  density: [],
};

const summarySection: SummarySection = {
  id: 'summary-42',
  type: 'summary',
  title: '个人总结',
  visible: true,
  content: { markdown: '负责产品交付。' },
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
        techStack: '',
        achievements: '提高了效率。',
        highlightsMarkdown: '- 完成系统交付',
        showLocation: true,
        showLink: true,
        showIntroduction: true,
        showTechStack: true,
        showAchievements: true,
      },
    ],
  },
};

const workSection: WorkSection = {
  id: 'work-42',
  type: 'work',
  title: '工作经历',
  visible: true,
  content: {
    items: [{
      id: 'work-alpha', company: '示例公司', role: '产品经理', startDate: '2024.01', endDate: '至今', location: '上海',
      achievements: '负责产品交付。', highlightsMarkdown: '- 参与需求梳理', showLocation: true, showAchievements: true,
    }],
  },
};

const internshipSection: InternshipSection = {
  ...workSection,
  id: 'internship-42',
  type: 'internship',
  title: '实习经历',
};

const summaryVerbSuggestion: AiSuggestion = {
  id: 'sg-1',
  code: 'S2',
  label: '弱动词',
  severity: 'high',
  target: { field: 'markdown' },
  issue: '用「负责」开头，没有体现你的主导范围',
  anchor: '负责',
  replacement: '主导',
  rationale: '招聘方只读前 3 秒，需要一眼看到动作强度',
  start: 0,
  end: 2,
  status: 'applied',
};

const workHighlightsSuggestion: AiSuggestion = {
  id: 'sg-2',
  code: 'S2',
  label: '弱动词',
  severity: 'high',
  target: { itemId: 'work-alpha', field: 'highlightsMarkdown' },
  issue: '用「参与」开头',
  anchor: '参与',
  replacement: '主导',
  rationale: '动作更明确',
  start: 2,
  end: 4,
  status: 'applied',
};

const workAchievementsSuggestion: AiSuggestion = {
  id: 'sg-3',
  code: 'S4',
  label: '成果模糊',
  severity: 'medium',
  target: { itemId: 'work-alpha', field: 'achievements' },
  issue: '只写了职责',
  anchor: '负责',
  replacement: '主导',
  rationale: '突出成果',
  start: 0,
  end: 2,
  status: 'applied',
};

afterEach(() => {
  streamRewriteMock.mockReset();
  document.body.innerHTML = '';
});

describe('AiSuggestionPanel', () => {
  it('keeps the first message for a repeated progress stage', async () => {
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

    act(() => {
      emit?.({ kind: 'progress', stage: 'context_loaded', message: '已读取上下文。' });
      emit?.({ kind: 'progress', stage: 'context_loaded', message: '不应覆盖已读取上下文。' });
    });

    expect(screen.getAllByText('已读取上下文。')).toHaveLength(2);
    expect(screen.queryByText('不应覆盖已读取上下文。')).not.toBeInTheDocument();
  });

  it('renders the diagnosis overview, keyword alignment and suggestion cards', async () => {
    streamRewriteMock.mockImplementation(async (_resumeId, _request, onEvent) => {
      onEvent({
        kind: 'suggestion',
        sectionId: 'summary-42',
        sectionType: 'summary',
        newContent: {
          rewrite: { markdown: '主导产品交付。' },
          analysis: {
            jdAlignment: { status: 'applied', matchedKeywords: ['用户增长'], excludedKeywords: ['SQL'], missingKeywords: ['A/B 实验'] },
            suggestions: [summaryVerbSuggestion],
            evidenceRequests: [{ code: 'S1', target: { field: 'markdown' }, reason: '缺少规模', exampleWithPlaceholder: '覆盖 XX 名用户' }],
            warnings: [{ code: 'S7', resolution: '已基于既有事实重组叙事。' }],
            density: [{ field: 'markdown', chars: 7, lines: 1, bullets: 0, maxBulletChars: 0, overLimit: true }],
          },
        },
      } as SSEEvent);
      onEvent({ kind: 'done' });
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

    const overview = await screen.findByRole('region', { name: '诊断总览' });
    expect(overview).toHaveTextContent('动作与结构');
    expect(overview).toHaveTextContent('1 处建议');
    expect(overview).toHaveTextContent('待补 1 项数据');

    const alignment = screen.getByRole('region', { name: 'JD 对齐' });
    expect(alignment).toHaveTextContent('用户增长');
    expect(alignment).toHaveTextContent('A/B 实验');

    const suggestions = screen.getByRole('region', { name: '改写建议' });
    expect(suggestions).toHaveTextContent('用「负责」开头，没有体现你的主导范围');
    expect(suggestions).toHaveTextContent('招聘方只读前 3 秒，需要一眼看到动作强度');
    expect(suggestions).toHaveTextContent('负责');
    expect(suggestions).toHaveTextContent('主导');

    const evidence = screen.getByRole('region', { name: '待补充数据' });
    expect(evidence).toHaveTextContent('覆盖 XX 名用户');
    expect(evidence).toHaveTextContent('不会随采纳写入简历');
    expect(within(evidence).queryByRole('button')).not.toBeInTheDocument();

    expect(screen.getByRole('region', { name: '提示' })).toHaveTextContent('已基于既有事实重组叙事。');
  });

  it('applies a single suggestion to the draft and reverts it', async () => {
    streamRewriteMock.mockImplementation(async (_resumeId, _request, onEvent) => {
      onEvent({
        kind: 'suggestion',
        sectionId: 'work-42',
        sectionType: 'work',
        newContent: {
          rewrite: { items: [{ id: 'work-alpha', highlightsMarkdown: '- 主导需求梳理' }] },
          analysis: {
            jdAlignment: { status: 'skipped' },
            suggestions: [workHighlightsSuggestion],
            evidenceRequests: [],
            warnings: [],
            density: [],
          },
        },
      } as SSEEvent);
      onEvent({ kind: 'done' });
    });
    const onAccept = vi.fn();

    render(
      <AiSuggestionPanel
        onAccept={onAccept}
        onClose={vi.fn()}
        originalSection={workSection}
        resumeId={7}
        sectionType="work"
      />,
    );

    const applyButton = await screen.findByRole('button', { name: '应用' });
    expect(screen.getByRole('button', { name: '应用到简历' })).toBeDisabled();

    fireEvent.click(applyButton);
    expect(screen.getByRole('button', { name: '撤销' })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: '应用到简历' }));

    expect(onAccept).toHaveBeenCalledWith({
      items: [{ id: 'work-alpha', highlightsMarkdown: '- 主导需求梳理' }],
    });
  });

  it('reverts the draft when the suggestion is undone', async () => {
    streamRewriteMock.mockImplementation(async (_resumeId, _request, onEvent) => {
      onEvent({
        kind: 'suggestion',
        sectionId: 'summary-42',
        sectionType: 'summary',
        newContent: {
          rewrite: { markdown: '主导产品交付。' },
          analysis: {
            jdAlignment: { status: 'skipped' },
            suggestions: [summaryVerbSuggestion],
            evidenceRequests: [],
            warnings: [],
            density: [],
          },
        },
      } as SSEEvent);
      onEvent({ kind: 'done' });
    });
    const onAccept = vi.fn();

    render(
      <AiSuggestionPanel
        onAccept={onAccept}
        onClose={vi.fn()}
        originalSection={summarySection}
        resumeId={7}
        sectionType="summary"
      />,
    );

    fireEvent.click(await screen.findByRole('button', { name: '应用' }));
    fireEvent.click(screen.getByRole('button', { name: '撤销' }));

    expect(screen.getByRole('button', { name: '应用到简历' })).toBeDisabled();
    fireEvent.click(screen.getByRole('button', { name: '应用到简历' }));
    expect(onAccept).not.toHaveBeenCalled();
  });

  it('applies every available suggestion at once and commits only the changed fields', async () => {
    streamRewriteMock.mockImplementation(async (_resumeId, _request, onEvent) => {
      onEvent({
        kind: 'suggestion',
        sectionId: 'work-42',
        sectionType: 'work',
        newContent: {
          rewrite: { items: [{ id: 'work-alpha', achievements: '主导产品交付。', highlightsMarkdown: '- 主导需求梳理' }] },
          analysis: {
            jdAlignment: { status: 'skipped' },
            suggestions: [workHighlightsSuggestion, workAchievementsSuggestion],
            evidenceRequests: [],
            warnings: [],
            density: [],
          },
        },
      } as SSEEvent);
      onEvent({ kind: 'done' });
    });
    const onAccept = vi.fn();

    render(
      <AiSuggestionPanel
        onAccept={onAccept}
        onClose={vi.fn()}
        originalSection={workSection}
        resumeId={7}
        sectionType="work"
      />,
    );

    fireEvent.click(await screen.findByRole('button', { name: '全部应用' }));
    fireEvent.click(screen.getByRole('button', { name: '应用到简历' }));

    expect(onAccept).toHaveBeenCalledWith({
      items: [{ id: 'work-alpha', achievements: '主导产品交付。', highlightsMarkdown: '- 主导需求梳理' }],
    });
  });

  it('lists rejected suggestions with their reason', async () => {
    streamRewriteMock.mockImplementation(async (_resumeId, _request, onEvent) => {
      onEvent({
        kind: 'suggestion',
        sectionId: 'summary-42',
        sectionType: 'summary',
        newContent: {
          rewrite: { markdown: '负责产品交付。' },
          analysis: {
            jdAlignment: { status: 'skipped' },
            suggestions: [{
              ...summaryVerbSuggestion,
              id: 'sg-2',
              status: 'rejected',
              rejectionReason: '建议引入了原文中没有的数据',
            }],
            evidenceRequests: [],
            warnings: [],
            density: [],
          },
        },
      } as SSEEvent);
      onEvent({ kind: 'done' });
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

    expect(await screen.findByText('已跳过 1 条建议')).toBeInTheDocument();
    expect(screen.getByText('建议引入了原文中没有的数据')).toBeInTheDocument();
    expect(screen.getByRole('region', { name: '改写建议' })).toHaveTextContent('本次没有可安全采纳的改写，保留原文。');
  });

  it('shows the target job alignment state and preserves the original when there is no safe rewrite', async () => {
    streamRewriteMock.mockImplementation(async (_resumeId, _request, onEvent) => {
      onEvent({
        kind: 'suggestion',
        sectionId: 'summary-42',
        sectionType: 'summary',
        newContent: { rewrite: { markdown: '' }, analysis: emptyAnalysis },
      } as SSEEvent);
      onEvent({ kind: 'done' });
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

    expect(await screen.findByRole('region', { name: 'JD 对齐' })).toHaveTextContent('未绑定目标岗位');
    expect(screen.getByRole('region', { name: '改写建议' })).toHaveTextContent('保留原文');
    expect(screen.getByRole('button', { name: '应用到简历' })).toBeDisabled();
  });

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
        newContent: { rewrite: { markdown: '不应显示的内容' }, analysis: emptyAnalysis },
      });
      emit?.({ kind: 'thinking', text: '岗位要求' });
    });
    expect(screen.queryByText('岗位要求')).not.toBeInTheDocument();
    act(() => {
      emit?.({
        kind: 'suggestion',
        sectionId: 'summary-42',
        sectionType: 'summary',
        newContent: {
          rewrite: { markdown: '主导产品交付。' },
          analysis: {
            jdAlignment: { status: 'skipped' },
            suggestions: [summaryVerbSuggestion],
            evidenceRequests: [],
            warnings: [],
            density: [],
          },
        },
      });
      emit?.({ kind: 'done' });
    });
    expect(await screen.findByRole('region', { name: '改写建议' })).toHaveTextContent('负责');
    expect(screen.queryByText('不应显示的内容')).not.toBeInTheDocument();
    act(() => {
      emit?.({ kind: 'thinking', text: '过期分析' });
    });
    expect(screen.queryByText('过期分析')).not.toBeInTheDocument();
    expect(streamRewriteMock).toHaveBeenCalledWith(
      7,
      { sectionId: 'summary-42', sectionType: 'summary' },
      expect.any(Function),
      expect.any(AbortSignal),
    );
  });

  it('commits the draft and closes without changing it', async () => {
    streamRewriteMock.mockImplementation(async (_resumeId, _request, onEvent) => {
      onEvent({
        kind: 'suggestion',
        sectionId: 'summary-42',
        sectionType: 'summary',
        newContent: {
          rewrite: { markdown: '主导产品交付。' },
          analysis: {
            jdAlignment: { status: 'skipped' },
            suggestions: [summaryVerbSuggestion],
            evidenceRequests: [],
            warnings: [],
            density: [],
          },
        },
      } as SSEEvent);
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

    fireEvent.click(await screen.findByRole('button', { name: '应用' }));
    fireEvent.click(screen.getByRole('button', { name: '应用到简历' }));
    expect(onAccept).toHaveBeenCalledWith({ markdown: '主导产品交付。' });

    rerender(
      <AiSuggestionPanel
        onAccept={onAccept}
        onClose={onClose}
        originalSection={summarySection}
        resumeId={7}
        sectionType="summary"
      />,
    );
    fireEvent.click(screen.getByRole('button', { name: '关闭' }));
    expect(onClose).toHaveBeenCalledOnce();
    expect(onAccept).toHaveBeenCalledOnce();
  });

  it('keeps a completed suggestion ready when the reader fails after done', async () => {
    let rejectStream: ((reason: Error) => void) | undefined;
    streamRewriteMock.mockImplementation((_resumeId, _request, onEvent) => {
      onEvent({
        kind: 'suggestion',
        sectionId: 'summary-42',
        sectionType: 'summary',
        newContent: {
          rewrite: { markdown: '主导产品交付。' },
          analysis: {
            jdAlignment: { status: 'skipped' },
            suggestions: [summaryVerbSuggestion],
            evidenceRequests: [],
            warnings: [],
            density: [],
          },
        },
      } as SSEEvent);
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
    expect(await screen.findByRole('region', { name: '改写建议' })).toHaveTextContent('负责');

    await act(async () => {
      rejectStream?.(new Error('reader failed after done'));
    });

    expect(screen.getByRole('region', { name: '改写建议' })).toHaveTextContent('负责');
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
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
        newContent: {
          rewrite: { markdown: '主导产品交付。' },
          analysis: {
            jdAlignment: { status: 'skipped' },
            suggestions: [summaryVerbSuggestion],
            evidenceRequests: [],
            warnings: [],
            density: [],
          },
        },
      } as SSEEvent);
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
    fireEvent.click(await screen.findByRole('button', { name: '应用' }));
    fireEvent.click(screen.getByRole('button', { name: '应用到简历' }));

    expect(onAccept).toHaveBeenCalledOnce();
    expect(callbackObservedAbort).toBe(true);
    expect(requestSignal?.aborted).toBe(true);
    expect(screen.queryByText('采纳后的过期错误')).not.toBeInTheDocument();
  });

  it.each([
    { action: '关闭', accessibleName: '关闭' },
    { action: '关闭按钮', accessibleName: '关闭 AI 润色建议' },
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
        newContent: { rewrite: { markdown: '过期建议' }, analysis: emptyAnalysis },
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
    expect(screen.getByRole('button', { name: '应用到简历' })).toBeDisabled();
  });

  it('explains a truncated model output with an actionable message', async () => {
    streamRewriteMock.mockImplementation(async (_resumeId, _request, onEvent) => {
      onEvent({ kind: 'error', code: 'TRUNCATED_OUTPUT', message: 'invalid character' });
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

    expect(await screen.findByRole('alert')).toHaveTextContent('AI 输出被截断');
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

    const dialog = screen.getByRole('dialog', { name: 'AI 诊断与润色' });
    expect(dialog).toContainElement(document.activeElement as HTMLElement);
    expect(background).toHaveAttribute('aria-hidden', 'true');
    expect(background).toHaveAttribute('inert');

    opener.focus();
    expect(dialog).toContainElement(document.activeElement as HTMLElement);
    const close = within(dialog).getByRole('button', { name: '关闭 AI 润色建议' });
    const apply = within(dialog).getByRole('button', { name: '应用到简历' });
    apply.focus();
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

  it.each([
    { section: summarySection, sectionType: 'summary' as const },
    { section: workSection, sectionType: 'work' as const },
    { section: internshipSection, sectionType: 'internship' as const },
    { section: projectSection, sectionType: 'project' as const },
  ])('renders a $sectionType section without an adoptable rewrite', async ({ section, sectionType }) => {
    streamRewriteMock.mockImplementation(async (_resumeId, _request, onEvent) => {
      onEvent({
        kind: 'suggestion',
        sectionId: section.id,
        sectionType,
        newContent: { rewrite: { markdown: '原文' }, analysis: emptyAnalysis },
      } as unknown as SSEEvent);
      onEvent({ kind: 'done' });
    });

    render(
      <AiSuggestionPanel
        onAccept={vi.fn()}
        onClose={vi.fn()}
        originalSection={section}
        resumeId={7}
        sectionType={sectionType}
      />,
    );

    expect(await screen.findByRole('region', { name: '改写建议' })).toHaveTextContent('保留原文');
    expect(screen.getByRole('button', { name: '应用到简历' })).toBeDisabled();
  });
});
