import {
  useCallback,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type ReactNode,
} from 'react';
import { createPortal } from 'react-dom';

import { streamRewrite } from '../api/ai';
import type {
  ExperienceRewriteContent,
  OptimizationAnalysis as OptimizationAnalysisPayload,
  ProjectRewriteContent,
  ProgressEvent,
  RewriteableSectionType,
  SSEEvent,
  SuggestionEvent,
  SummaryRewriteContent,
} from '../types/ai';
import type {
  InternshipSection,
  ProjectSection,
  SummarySection,
  WorkSection,
} from '../types/resume';

type PanelPhase = 'loading' | 'streaming' | 'ready' | 'error';
type RewriteableResumeSection = SummarySection | WorkSection | InternshipSection | ProjectSection;

interface SectionByType {
  summary: SummarySection;
  work: WorkSection;
  internship: InternshipSection;
  project: ProjectSection;
}

interface RewriteContentBySectionType {
  summary: SummaryRewriteContent;
  work: ExperienceRewriteContent;
  internship: ExperienceRewriteContent;
  project: ProjectRewriteContent;
}

export interface AiSuggestionPanelProps<T extends RewriteableSectionType = RewriteableSectionType> {
  sectionType: T;
  originalSection: SectionByType[T];
  onAccept: (rewrite: RewriteContentBySectionType[T]) => void;
  onClose: () => void;
  resumeId: number;
}

interface PanelState {
  phase: PanelPhase;
  thinking: string;
  progress: ProgressEvent[];
  suggestion?: SuggestionEvent;
  error?: string;
}

const initialState: PanelState = { phase: 'loading', thinking: '', progress: [] };
const focusableSelector = 'button:not(:disabled), [href], [tabindex]:not([tabindex="-1"])';

const frozenExperienceFields = [
  ['company', '公司'],
  ['role', '职位'],
  ['startDate', '开始时间'],
  ['endDate', '结束时间'],
  ['location', '地点'],
] as const;

const frozenProjectFields = [
  ['name', '项目名称'],
  ['role', '项目角色'],
  ['startDate', '开始时间'],
  ['endDate', '结束时间'],
  ['location', '地点'],
  ['link', '项目链接'],
] as const;

const rewriteableExperienceFields = [
  ['achievements', '业绩贡献'],
  ['highlightsMarkdown', '经历亮点'],
] as const;

const rewriteableProjectFields = [
  ['introduction', '项目介绍'],
  ['achievements', '项目成果'],
  ['highlightsMarkdown', '项目亮点'],
] as const;

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function normalizeRewriteItem(value: unknown, fields: readonly string[]) {
  if (!isRecord(value) || typeof value.id !== 'string') return undefined;
  const normalized: Record<string, string> = { id: value.id };
  for (const field of fields) {
    if (Object.prototype.hasOwnProperty.call(value, field) && typeof value[field] === 'string') {
      normalized[field] = value[field];
    }
  }
  return normalized;
}

function normalizeRewrite<T extends RewriteableSectionType>(
  sectionType: T,
  value: unknown,
): RewriteContentBySectionType[T] | undefined {
  if (!isRecord(value)) return undefined;
  if (sectionType === 'summary') {
    return (typeof value.markdown === 'string' ? { markdown: value.markdown } : undefined) as
      RewriteContentBySectionType[T] | undefined;
  }
  if (!Array.isArray(value.items)) return undefined;
  const fields = sectionType === 'project'
    ? rewriteableProjectFields.map(([field]) => field)
    : rewriteableExperienceFields.map(([field]) => field);
  const items = value.items
    .map((item) => normalizeRewriteItem(item, fields))
    .filter((item): item is Record<string, string> => item !== undefined);
  return { items } as unknown as RewriteContentBySectionType[T];
}

function displayValue(value: string) {
  return value === '' ? '（空）' : value;
}

function FrozenField({ label, value }: { label: string; value: string }) {
  return (
    <div className="ai-suggestion-panel__field ai-suggestion-panel__field--frozen">
      <dt>{label}</dt>
      <dd>{displayValue(value)}</dd>
    </div>
  );
}

function RewriteableField({ label, newValue, oldValue }: {
  label: string;
  newValue?: string;
  oldValue: string;
}) {
  if (newValue === undefined || newValue === oldValue) {
    return (
      <div className="ai-suggestion-panel__field">
        <dt>{label}</dt>
        <dd className="ai-suggestion-panel__unchanged-value">{displayValue(oldValue)}</dd>
      </div>
    );
  }

  return (
    <details className="ai-suggestion-panel__diff" open>
      <summary>{label}<span>已更新</span></summary>
      <div className="ai-suggestion-panel__diff-values">
        <del className="ai-suggestion-panel__old-value">
          <span className="ai-suggestion-panel__sr-only">原内容：</span>{displayValue(oldValue)}
        </del>
        <ins className="ai-suggestion-panel__new-value">
          <span className="ai-suggestion-panel__sr-only">新内容：</span>{displayValue(newValue)}
        </ins>
      </div>
    </details>
  );
}

function FieldGroup({ children, frozen }: { children: ReactNode; frozen?: boolean }) {
  const label = frozen ? 'Frozen · 保持原样' : 'Rewriteable · AI 建议';
  return (
    <section aria-label={label} className={`ai-suggestion-panel__group${frozen ? ' ai-suggestion-panel__group--frozen' : ''}`} role="group">
      <h3><span aria-hidden="true">{frozen ? '🔒' : '✎'}</span>{label}</h3>
      {children}
    </section>
  );
}

function OptimizationAnalysis({ analysis }: { analysis: OptimizationAnalysisPayload }) {
  const { jdAlignment, matchedIssues, evidenceRequests, warnings } = analysis;
  return (
    <section aria-label="分析过程" className="ai-suggestion-panel__analysis-result">
      <h3>分析过程</h3>
      <section className="ai-suggestion-panel__analysis-section">
        <h4>JD 对齐</h4>
        {jdAlignment.status === 'skipped' ? (
          <p>未绑定目标岗位，未进行 JD 对齐。</p>
        ) : (
          <>
            <p>已完成目标岗位关键词对齐。</p>
            {jdAlignment.matchedKeywords?.length ? <p>匹配关键词：{jdAlignment.matchedKeywords.join('、')}</p> : null}
            {jdAlignment.excludedKeywords?.length ? <p>未纳入关键词：{jdAlignment.excludedKeywords.join('、')}</p> : null}
          </>
        )}
      </section>
      {matchedIssues.length > 0 && (
        <section className="ai-suggestion-panel__analysis-section">
          <h4>已修复的问题</h4>
          <ul className="ai-suggestion-panel__analysis-list">
            {matchedIssues.map((issue, index) => (
              <li key={`${issue.code}-${issue.target.itemId ?? ''}-${issue.target.field}-${index}`}>
                <strong>{issue.code}</strong>
                <span>{issue.label ?? issue.resolution}</span>
              </li>
            ))}
          </ul>
        </section>
      )}
      {evidenceRequests.length > 0 && (
        <section className="ai-suggestion-panel__analysis-section">
          <h4>待补充数据</h4>
          <ul className="ai-suggestion-panel__evidence-cards">
            {evidenceRequests.map((request, index) => (
              <li key={`${request.target.itemId ?? ''}-${request.target.field}-${index}`}>
                <strong>{request.code} · {request.reason}</strong>
                <span>示例：{request.exampleWithPlaceholder}</span>
                <small>不会随采纳写入简历</small>
              </li>
            ))}
          </ul>
        </section>
      )}
      {warnings.length > 0 && (
        <section className="ai-suggestion-panel__analysis-section">
          <h4>提示</h4>
          <ul className="ai-suggestion-panel__analysis-list">
            {warnings.map((warning, index) => (
              <li key={`${warning.code}-${index}`}>
                <strong>{warning.code}</strong>
                <span>{warning.resolution}</span>
              </li>
            ))}
          </ul>
        </section>
      )}
    </section>
  );
}

function ItemFields({ children, title }: { children: ReactNode; title: string }) {
  return (
    <article className="ai-suggestion-panel__item">
      <h4>{title}</h4>
      <dl>{children}</dl>
    </article>
  );
}

function SuggestionDiff({ originalSection, rewrite: newContent }: {
  originalSection: RewriteableResumeSection;
  rewrite: unknown;
}) {
  if (originalSection.type === 'summary') {
    const rewrite = normalizeRewrite('summary', newContent);
    if (!rewrite) return <p role="alert">建议内容格式无效，请重新生成。</p>;
    return (
      <div className="ai-suggestion-panel__groups">
        <FieldGroup>
          <ItemFields title="个人总结">
            <RewriteableField
              label="总结内容"
              newValue={rewrite.markdown}
              oldValue={originalSection.content.markdown}
            />
          </ItemFields>
        </FieldGroup>
      </div>
    );
  }

  if (
    originalSection.type === 'work' || originalSection.type === 'internship'
  ) {
    const rewrite = normalizeRewrite(originalSection.type, newContent);
    if (!rewrite) return <p role="alert">建议内容格式无效，请重新生成。</p>;
    const rewriteById = new Map(rewrite.items.map((item) => [item.id, item]));
    return (
      <div className="ai-suggestion-panel__groups">
        <FieldGroup>
          {originalSection.content.items.map((item, index) => {
            const rewrite = rewriteById.get(item.id);
            return (
              <ItemFields key={item.id} title={`${index + 1}. ${item.company || item.role || '未命名经历'}`}>
                {rewriteableExperienceFields.map(([field, label]) => (
                  <RewriteableField key={field} label={label} newValue={rewrite?.[field]} oldValue={item[field]} />
                ))}
              </ItemFields>
            );
          })}
        </FieldGroup>
      </div>
    );
  }

  if (originalSection.type === 'project') {
    const rewrite = normalizeRewrite('project', newContent);
    if (!rewrite) return <p role="alert">建议内容格式无效，请重新生成。</p>;
    const rewriteById = new Map(rewrite.items.map((item) => [item.id, item]));
    return (
      <div className="ai-suggestion-panel__groups">
        <FieldGroup>
          {originalSection.content.items.map((item, index) => {
            const rewrite = rewriteById.get(item.id);
            return (
              <ItemFields key={item.id} title={`${index + 1}. ${item.name || item.role || '未命名项目'}`}>
                {rewriteableProjectFields.map(([field, label]) => (
                  <RewriteableField key={field} label={label} newValue={rewrite?.[field]} oldValue={item[field]} />
                ))}
              </ItemFields>
            );
          })}
        </FieldGroup>
      </div>
    );
  }

  return <p role="alert">建议与当前章节不匹配，请重新生成。</p>;
}

function errorMessage(error: unknown) {
  return error instanceof Error && error.message.trim() !== ''
    ? error.message
    : 'AI 润色失败，请稍后重试。';
}

const progressSteps: Array<{ stage: ProgressEvent['stage']; title: string; fallback: string }> = [
  { stage: 'context_loaded', title: '理解上下文', fallback: '读取 JD 与当前简历' },
  { stage: 'jd_alignment_started', title: '分析问题', fallback: '对齐目标岗位关键词' },
  { stage: 'jd_alignment_completed', title: '完成 JD 对齐', fallback: '已完成关键词对照' },
  { stage: 'optimization_started', title: '优化表达', fallback: '优化章节表达' },
  { stage: 'optimization_completed', title: '汇总建议', fallback: '生成可采纳的修改' },
];

function hasSafeRewrite(
  originalSection: RewriteableResumeSection,
  rewrite: SummaryRewriteContent | ExperienceRewriteContent | ProjectRewriteContent,
) {
  if (originalSection.type === 'summary') return 'markdown' in rewrite && rewrite.markdown.trim() !== '';
  if (!('items' in rewrite)) return false;
  const itemIds = new Set(originalSection.content.items.map((item) => item.id));
  const fields = originalSection.type === 'project'
    ? rewriteableProjectFields.map(([field]) => field)
    : rewriteableExperienceFields.map(([field]) => field);
  return rewrite.items.some((item) => itemIds.has(item.id) && fields.some((field) => field in item));
}

export function AiSuggestionPanel<T extends RewriteableSectionType>({
  onAccept,
  onClose,
  originalSection,
  resumeId,
  sectionType,
}: AiSuggestionPanelProps<T>) {
  const [state, setState] = useState<PanelState>(initialState);
  const titleId = useId();
  const statusId = useId();
  const backdropRef = useRef<HTMLDivElement>(null);
  const dialogRef = useRef<HTMLElement>(null);
  const closeButtonRef = useRef<HTMLButtonElement>(null);
  const previousFocusRef = useRef<HTMLElement | null>(null);
  const abortRef = useRef<AbortController>();
  const requestVersionRef = useRef(0);
  const onCloseRef = useRef(onClose);
  onCloseRef.current = onClose;

  const cancelActiveRequest = useCallback(() => {
    requestVersionRef.current += 1;
    abortRef.current?.abort();
  }, []);

  const closePanel = useCallback(() => {
    cancelActiveRequest();
    onCloseRef.current();
  }, [cancelActiveRequest]);

  const runRewrite = useCallback(() => {
    const requestVersion = ++requestVersionRef.current;
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;
    setState(initialState);

    if (originalSection.type !== sectionType) {
      setState({ phase: 'error', thinking: '', progress: [], error: '章节类型不匹配，无法生成建议。' });
      return;
    }

    let correlatedSuggestion: SuggestionEvent | undefined;
    let reachedTerminalEvent = false;
    let receivedError = false;

    const onEvent = (event: SSEEvent) => {
      if (requestVersion !== requestVersionRef.current || controller.signal.aborted) return;
      if (reachedTerminalEvent) return;

      if (event.kind === 'thinking') {
        if (receivedError) return;
        setState((current) => ({
          ...current,
          phase: 'streaming',
          thinking: '',
        }));
        return;
      }

      if (event.kind === 'progress') {
        if (receivedError) return;
        setState((current) => ({
          ...current,
          phase: 'streaming',
          thinking: event.message,
          progress: current.progress.some((progress) => progress.stage === event.stage)
            ? current.progress
            : [...current.progress, event],
        }));
        return;
      }

      if (event.kind === 'suggestion') {
        if (event.sectionId !== originalSection.id || event.sectionType !== sectionType || receivedError) return;
        correlatedSuggestion = event;
        setState((current) => ({ ...current, phase: 'streaming', suggestion: event }));
        return;
      }

      if (event.kind === 'error') {
        receivedError = true;
        setState((current) => ({ ...current, phase: 'error', error: event.message, suggestion: undefined }));
        return;
      }

      reachedTerminalEvent = true;
      if (receivedError) return;
      setState((current) => correlatedSuggestion
        ? { ...current, phase: 'ready', suggestion: correlatedSuggestion }
        : { ...current, phase: 'error', error: 'AI 未返回与当前章节匹配的建议。' });
    };

    void streamRewrite(
      resumeId,
      { sectionId: originalSection.id, sectionType },
      onEvent,
      controller.signal,
    ).then(() => {
      if (
        requestVersion !== requestVersionRef.current
        || controller.signal.aborted
        || reachedTerminalEvent
        || receivedError
      ) return;
      setState((current) => correlatedSuggestion
        ? { ...current, phase: 'ready', suggestion: correlatedSuggestion }
        : { ...current, phase: 'error', error: 'AI 响应提前结束，请重新生成。' });
    }).catch((error: unknown) => {
      if (
        requestVersion !== requestVersionRef.current
        || controller.signal.aborted
        || reachedTerminalEvent
        || receivedError
      ) return;
      setState((current) => ({ ...current, phase: 'error', error: errorMessage(error), suggestion: undefined }));
    });
  }, [originalSection.id, originalSection.type, resumeId, sectionType]);

  useLayoutEffect(() => {
    runRewrite();
    return cancelActiveRequest;
  }, [cancelActiveRequest, runRewrite]);

  useLayoutEffect(() => {
    const dialog = dialogRef.current;
    const backdrop = backdropRef.current;
    if (!dialog || !backdrop) return;

    previousFocusRef.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const backgroundElements = Array.from(document.body.children).filter(
      (element): element is HTMLElement => element instanceof HTMLElement && element !== backdrop,
    );
    const previousBackgroundState = backgroundElements.map((element) => ({
      element,
      ariaHidden: element.getAttribute('aria-hidden'),
      inert: element.hasAttribute('inert'),
    }));
    for (const element of backgroundElements) {
      element.setAttribute('aria-hidden', 'true');
      element.setAttribute('inert', '');
    }

    const focusableElements = () => Array.from(dialog.querySelectorAll<HTMLElement>(focusableSelector));
    const handleFocusIn = (event: FocusEvent) => {
      if (!dialog.contains(event.target as Node)) closeButtonRef.current?.focus();
    };
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault();
        closePanel();
        return;
      }
      if (event.key !== 'Tab') return;
      const elements = focusableElements();
      if (elements.length === 0) {
        event.preventDefault();
        return;
      }
      const first = elements[0];
      const last = elements[elements.length - 1];
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    };

    document.addEventListener('focusin', handleFocusIn);
    document.addEventListener('keydown', handleKeyDown);
    closeButtonRef.current?.focus();
    return () => {
      document.removeEventListener('focusin', handleFocusIn);
      document.removeEventListener('keydown', handleKeyDown);
      for (const { element, ariaHidden, inert } of previousBackgroundState) {
        if (ariaHidden === null) element.removeAttribute('aria-hidden');
        else element.setAttribute('aria-hidden', ariaHidden);
        if (!inert) element.removeAttribute('inert');
      }
      const previousFocus = previousFocusRef.current;
      queueMicrotask(() => {
        if (previousFocus?.isConnected) previousFocus.focus();
      });
    };
  }, [closePanel]);

  const accept = () => {
    if (state.phase !== 'ready' || !state.suggestion) return;
    const rewrite = normalizeRewrite(sectionType, state.suggestion.newContent.rewrite);
    if (!rewrite || !hasSafeRewrite(originalSection, rewrite)) {
      setState((current) => ({ ...current, phase: 'error', error: 'AI 返回的建议格式无效。', suggestion: undefined }));
      return;
    }
    cancelActiveRequest();
    onAccept(rewrite);
  };

  const latestProgress = state.progress[state.progress.length - 1];
  const normalizedRewrite = state.suggestion
    ? normalizeRewrite(sectionType, state.suggestion.newContent.rewrite)
    : undefined;
  const canAccept = state.phase === 'ready'
    && normalizedRewrite !== undefined
    && hasSafeRewrite(originalSection, normalizedRewrite);

  const statusLabel = state.phase === 'loading'
    ? '正在读取目标岗位与当前简历…'
    : state.phase === 'streaming'
      ? latestProgress?.message ?? '正在分析可优化的表达…'
      : state.phase === 'ready'
        ? '建议已生成，请检查更改。'
        : '本次生成未完成。';

  return createPortal(
    <div className="ai-suggestion-panel__backdrop" ref={backdropRef}>
      <aside
        aria-describedby={statusId}
        aria-labelledby={titleId}
        aria-modal="true"
        aria-busy={state.phase === 'loading' || state.phase === 'streaming'}
        className="ai-suggestion-panel"
        ref={dialogRef}
        role="dialog"
      >
        <header className="ai-suggestion-panel__header">
          <div>
            <p>AI WRITING REVIEW</p>
            <h2 id={titleId}>AI 润色建议</h2>
          </div>
          <button aria-label="关闭 AI 润色建议" onClick={closePanel} ref={closeButtonRef} title="关闭" type="button">×</button>
        </header>

        <p aria-live="polite" className="ai-suggestion-panel__status" id={statusId}>{statusLabel}</p>

        {(state.phase === 'loading' || state.phase === 'streaming' || state.phase === 'ready') && (
          <section className="ai-suggestion-panel__progress" aria-label="AI 润色进度">
            {progressSteps.map((step, index) => {
              const received = state.progress.find((progress) => progress.stage === step.stage);
              const complete = state.phase === 'ready' || received !== undefined;
              const active = !complete && (
                state.phase === 'loading'
                ? index === 0
                : state.phase === 'streaming' && index === state.progress.length
              );
              return (
                <div className={`ai-suggestion-panel__progress-step${complete ? ' is-complete' : active ? ' is-active' : ''}`} key={step.stage}>
                  <span className="ai-suggestion-panel__progress-node" aria-hidden="true">{complete ? '✓' : active ? <i /> : index + 1}</span>
                  <strong>{step.title}</strong><small>{received?.message ?? step.fallback}</small>
                </div>
              );
            })}
          </section>
        )}

        {(state.phase === 'loading' || state.phase === 'streaming') && (
          <div className="ai-suggestion-panel__analysis" aria-live="polite">
            <strong>{state.phase === 'loading' ? '正在理解上下文' : '正在分析问题'}</strong>
            <p>{state.phase === 'loading'
              ? '正在对照目标岗位要求与当前简历内容。'
              : '正在识别可强化的成果表达，并确保不补写任何事实。'}</p>
          </div>
        )}

        {state.phase === 'error' && (
          <div className="ai-suggestion-panel__error" role="alert">
            <strong>无法显示润色建议</strong>
            <span>{state.error}</span>
          </div>
        )}

        {state.suggestion && state.phase === 'ready' && (
          <div className="ai-suggestion-panel__content">
            <OptimizationAnalysis analysis={state.suggestion.newContent.analysis} />
            {canAccept ? (
              <SuggestionDiff originalSection={originalSection} rewrite={state.suggestion.newContent.rewrite} />
            ) : (
              <p className="ai-suggestion-panel__preserve-original">保留原文：本次没有可安全采纳的改写。</p>
            )}
          </div>
        )}

        <footer className="ai-suggestion-panel__actions">
          <button onClick={closePanel} type="button">拒绝</button>
          <button onClick={runRewrite} type="button">重新生成</button>
          <button disabled={!canAccept} onClick={accept} type="button">采纳</button>
        </footer>
      </aside>
    </div>,
    document.body,
  );
}
