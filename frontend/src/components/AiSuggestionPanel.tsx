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
  ProjectRewriteContent,
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
  suggestion?: SuggestionEvent;
  error?: string;
}

const initialState: PanelState = { phase: 'loading', thinking: '' };
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

function ItemFields({ children, title }: { children: ReactNode; title: string }) {
  return (
    <article className="ai-suggestion-panel__item">
      <h4>{title}</h4>
      <dl>{children}</dl>
    </article>
  );
}

function SuggestionDiff({ originalSection, suggestion }: {
  originalSection: RewriteableResumeSection;
  suggestion: SuggestionEvent;
}) {
  if (originalSection.type === 'summary' && suggestion.sectionType === 'summary') {
    return (
      <div className="ai-suggestion-panel__groups">
        <FieldGroup frozen>
          <ItemFields title="章节设置">
            <FrozenField label="章节标题" value={originalSection.title} />
            <FrozenField label="显示状态" value={originalSection.visible ? '显示' : '隐藏'} />
          </ItemFields>
        </FieldGroup>
        <FieldGroup>
          <ItemFields title="个人总结">
            <RewriteableField
              label="总结内容"
              newValue={suggestion.newContent.markdown}
              oldValue={originalSection.content.markdown}
            />
          </ItemFields>
        </FieldGroup>
      </div>
    );
  }

  if (
    (originalSection.type === 'work' || originalSection.type === 'internship')
    && (suggestion.sectionType === 'work' || suggestion.sectionType === 'internship')
  ) {
    const rewriteById = new Map(suggestion.newContent.items.map((item) => [item.id, item]));
    return (
      <div className="ai-suggestion-panel__groups">
        <FieldGroup frozen>
          {originalSection.content.items.map((item, index) => (
            <ItemFields key={item.id} title={`${index + 1}. ${item.company || item.role || '未命名经历'}`}>
              {frozenExperienceFields.map(([field, label]) => (
                <FrozenField key={field} label={label} value={item[field]} />
              ))}
            </ItemFields>
          ))}
        </FieldGroup>
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

  if (originalSection.type === 'project' && suggestion.sectionType === 'project') {
    const rewriteById = new Map(suggestion.newContent.items.map((item) => [item.id, item]));
    return (
      <div className="ai-suggestion-panel__groups">
        <FieldGroup frozen>
          {originalSection.content.items.map((item, index) => (
            <ItemFields key={item.id} title={`${index + 1}. ${item.name || item.role || '未命名项目'}`}>
              {frozenProjectFields.map(([field, label]) => (
                <FrozenField key={field} label={label} value={item[field]} />
              ))}
            </ItemFields>
          ))}
        </FieldGroup>
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
      setState({ phase: 'error', thinking: '', error: '章节类型不匹配，无法生成建议。' });
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
    const rewrite = normalizeRewrite(sectionType, state.suggestion.newContent);
    cancelActiveRequest();
    if (!rewrite) {
      setState((current) => ({ ...current, phase: 'error', error: 'AI 返回的建议格式无效。', suggestion: undefined }));
      return;
    }
    onAccept(rewrite);
  };

  const statusLabel = state.phase === 'loading'
    ? '正在读取目标岗位与当前简历…'
    : state.phase === 'streaming'
      ? '正在分析可优化的表达…'
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
            <div className={`ai-suggestion-panel__progress-step${state.phase !== 'loading' ? ' is-complete' : ' is-active'}`}>
              <span className="ai-suggestion-panel__progress-node" aria-hidden="true">{state.phase === 'loading' ? <i /> : '✓'}</span>
              <strong>理解上下文</strong><small>读取 JD 与当前简历</small>
            </div>
            <div className={`ai-suggestion-panel__progress-step${state.phase === 'ready' ? ' is-complete' : state.phase === 'streaming' ? ' is-active' : ''}`}>
              <span className="ai-suggestion-panel__progress-node" aria-hidden="true">{state.phase === 'streaming' ? <i /> : state.phase === 'ready' ? '✓' : '2'}</span>
              <strong>分析问题</strong><small>提炼可优化的表达</small>
            </div>
            <div className={`ai-suggestion-panel__progress-step${state.phase === 'ready' ? ' is-complete' : ''}`}>
              <span className="ai-suggestion-panel__progress-node" aria-hidden="true">{state.phase === 'ready' ? '✓' : '3'}</span>
              <strong>汇总建议</strong><small>生成可采纳的修改</small>
            </div>
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
            {state.suggestion.rationale && <p className="ai-suggestion-panel__rationale">{state.suggestion.rationale}</p>}
            <SuggestionDiff originalSection={originalSection} suggestion={state.suggestion} />
          </div>
        )}

        <footer className="ai-suggestion-panel__actions">
          <button onClick={closePanel} type="button">拒绝</button>
          <button onClick={runRewrite} type="button">重新生成</button>
          <button disabled={state.phase !== 'ready' || !state.suggestion} onClick={accept} type="button">采纳</button>
        </footer>
      </aside>
    </div>,
    document.body,
  );
}
