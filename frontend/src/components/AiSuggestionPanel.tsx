import {
  useCallback,
  useId,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import { createPortal } from 'react-dom';

import { streamRewrite } from '../api/ai';
import type {
  AiSuggestion,
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
import { applySuggestionsToText } from '../utils/applySuggestions';

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

const fieldLabels: Record<string, string> = {
  introduction: '项目介绍',
  achievements: '业绩贡献',
  highlightsMarkdown: '经历亮点',
  markdown: '总结内容',
};

const severityLabels: Record<string, string> = {
  high: '高',
  medium: '中',
  low: '低',
};

const errorCodeMessages: Record<string, string> = {
  TRUNCATED_OUTPUT: 'AI 输出被截断，请重新生成或调大 AI_MAX_TOKENS。',
  INVALID_RESPONSE: 'AI 返回的结果不符合约定，请重新生成。',
  LLM_ERROR: 'AI 服务暂时不可用，请稍后重试。',
  PROMPT_BUILD_FAILED: '提示词构建失败，请稍后重试。',
};

const dimensionDefinitions = [
  { key: 'structure', label: '动作与结构', codes: ['S2', 'S3', 'S6'] },
  { key: 'impact', label: '结果与量化', codes: ['S4'] },
  { key: 'keyword', label: 'JD 关键词', codes: ['S5'] },
  { key: 'density', label: '信息密度', codes: ['S10'] },
] as const;

interface DraftField {
  itemId: string;
  field: string;
  next: string;
}

function fieldLabel(field: string): string {
  return fieldLabels[field] ?? field;
}

function originalFieldText(
  section: RewriteableResumeSection,
  target: { itemId?: string; field: string },
): string | undefined {
  if (section.type === 'summary') {
    return target.field === 'markdown' ? section.content.markdown : undefined;
  }
  const item = section.content.items.find((candidate) => candidate.id === target.itemId);
  if (!item) return undefined;
  if (target.field === 'introduction' && 'introduction' in item) return item.introduction;
  if (target.field === 'achievements') return item.achievements;
  if (target.field === 'highlightsMarkdown') return item.highlightsMarkdown;
  return undefined;
}

function itemLabel(section: RewriteableResumeSection, itemId: string): string {
  if (section.type === 'summary') return '个人总结';
  const index = section.content.items.findIndex((item) => item.id === itemId);
  if (index < 0) return '当前条目';
  const item = section.content.items[index];
  const name = 'company' in item
    ? item.company || item.role
    : 'name' in item
      ? item.name || item.role
      : '';
  return `第 ${index + 1} 条${name ? ` · ${name}` : ''}`;
}

function collectDraftFields(
  section: RewriteableResumeSection,
  suggestions: AiSuggestion[],
  selectedIds: string[],
): DraftField[] {
  const groups = new Map<string, { itemId: string; field: string; suggestions: AiSuggestion[] }>();
  for (const suggestion of suggestions) {
    if (suggestion.status !== 'applied') continue;
    if (!selectedIds.includes(suggestion.id)) continue;
    const itemId = suggestion.target.itemId ?? '';
    const key = `${itemId}|${suggestion.target.field}`;
    const group = groups.get(key) ?? { itemId, field: suggestion.target.field, suggestions: [] };
    group.suggestions.push(suggestion);
    groups.set(key, group);
  }

  const fields: DraftField[] = [];
  for (const group of groups.values()) {
    const original = originalFieldText(section, { itemId: group.itemId, field: group.field });
    if (original === undefined) continue;
    const next = applySuggestionsToText(original, group.suggestions);
    if (next === original) continue;
    fields.push({ itemId: group.itemId, field: group.field, next });
  }
  return fields;
}

function buildRewritePayload(
  section: RewriteableResumeSection,
  fields: DraftField[],
): SummaryRewriteContent | ExperienceRewriteContent | ProjectRewriteContent {
  if (section.type === 'summary') {
    const field = fields.find((candidate) => candidate.field === 'markdown');
    return { markdown: field?.next ?? section.content.markdown };
  }
  const items = new Map<string, Record<string, string>>();
  for (const field of fields) {
    const entry = items.get(field.itemId) ?? { id: field.itemId };
    entry[field.field] = field.next;
    items.set(field.itemId, entry);
  }
  return { items: [...items.values()] } as unknown as ExperienceRewriteContent | ProjectRewriteContent;
}

function DimensionOverview({ analysis, suggestions }: {
  analysis: OptimizationAnalysisPayload;
  suggestions: AiSuggestion[];
}) {
  const applied = suggestions.filter((suggestion) => suggestion.status === 'applied');
  const overLimit = analysis.density.filter((metric) => metric.overLimit).length;
  const skipped = analysis.jdAlignment.status === 'skipped';

  return (
    <section aria-label="诊断总览" className="ai-suggestion-panel__dimensions">
      <h3>诊断总览</h3>
      <ul>
        {dimensionDefinitions.map((dimension) => {
          const count = applied.filter((suggestion) => dimension.codes.includes(suggestion.code as never)).length;
          let status = '通过';
          if (dimension.key === 'keyword' && skipped) {
            status = '未检查（未绑定岗位）';
          } else if (count > 0) {
            status = `${count} 处建议`;
          } else if (dimension.key === 'impact' && analysis.evidenceRequests.length > 0) {
            status = `待补 ${analysis.evidenceRequests.length} 项数据`;
          } else if (dimension.key === 'keyword' && (analysis.jdAlignment.missingKeywords?.length ?? 0) > 0) {
            status = `${analysis.jdAlignment.missingKeywords?.length} 个关键词未覆盖`;
          } else if (dimension.key === 'density' && overLimit > 0) {
            status = `${overLimit} 处超限`;
          }
          return (
            <li key={dimension.key}>
              <span>{dimension.label}</span>
              <em className={status === '通过' ? 'is-passed' : 'is-flagged'}>{status}</em>
            </li>
          );
        })}
      </ul>
    </section>
  );
}

function KeywordAlignment({ analysis }: { analysis: OptimizationAnalysisPayload }) {
  const { jdAlignment } = analysis;
  if (jdAlignment.status === 'skipped') {
    return (
      <section aria-label="JD 对齐" className="ai-suggestion-panel__analysis-section">
        <h4>JD 对齐</h4>
        <p>未绑定目标岗位，未进行 JD 对齐。</p>
      </section>
    );
  }
  return (
    <section aria-label="JD 对齐" className="ai-suggestion-panel__analysis-section">
      <h4>JD 对齐</h4>
      {jdAlignment.matchedKeywords?.length ? <p>已覆盖：{jdAlignment.matchedKeywords.join('、')}</p> : null}
      {jdAlignment.missingKeywords?.length ? (
        <p>未覆盖：{jdAlignment.missingKeywords.join('、')}（本节没有事实支撑，AI 不能替你补）</p>
      ) : null}
      {jdAlignment.excludedKeywords?.length ? <p>未纳入：{jdAlignment.excludedKeywords.join('、')}</p> : null}
    </section>
  );
}

function SuggestionCard({
  suggestion,
  location,
  densityNote,
  selected,
  onToggle,
}: {
  suggestion: AiSuggestion;
  location: string;
  densityNote?: string;
  selected: boolean;
  onToggle: () => void;
}) {
  return (
    <li className={`ai-suggestion-panel__suggestion${selected ? ' is-selected' : ''}`}>
      <div className="ai-suggestion-panel__suggestion-head">
        <span className={`ai-suggestion-panel__severity ai-suggestion-panel__severity--${suggestion.severity}`}>
          {severityLabels[suggestion.severity] ?? suggestion.severity}
        </span>
        <span className="ai-suggestion-panel__code">{suggestion.code}{suggestion.label ? ` ${suggestion.label}` : ''}</span>
        <span className="ai-suggestion-panel__location">{location} · {fieldLabel(suggestion.target.field)}</span>
      </div>
      {suggestion.issue ? <p className="ai-suggestion-panel__problem">问题：{suggestion.issue}</p> : null}
      <div className="ai-suggestion-panel__diff-values">
        <del className="ai-suggestion-panel__old-value">
          <span className="ai-suggestion-panel__sr-only">原内容：</span>
          {suggestion.anchor || '（新增内容）'}
        </del>
        <ins className="ai-suggestion-panel__new-value">
          <span className="ai-suggestion-panel__sr-only">新内容：</span>
          {suggestion.replacement || '（删除该片段）'}
        </ins>
      </div>
      {suggestion.rationale ? <p className="ai-suggestion-panel__reason">理由：{suggestion.rationale}</p> : null}
      {densityNote ? <p className="ai-suggestion-panel__density">{densityNote}</p> : null}
      <button onClick={onToggle} type="button">{selected ? '撤销' : '应用'}</button>
    </li>
  );
}

function errorMessage(error: unknown) {
  if (error instanceof Error && error.message.trim() !== '') return error.message;
  return 'AI 润色失败，请稍后重试。';
}

const progressSteps: Array<{ stage: ProgressEvent['stage']; title: string; fallback: string }> = [
  { stage: 'context_loaded', title: '理解上下文', fallback: '读取 JD 与当前简历' },
  { stage: 'jd_alignment_started', title: '分析问题', fallback: '对齐目标岗位关键词' },
  { stage: 'jd_alignment_completed', title: '完成 JD 对齐', fallback: '已完成关键词对照' },
  { stage: 'contract_repair', title: '修复输出', fallback: '输出未符合约定，正在自动修复' },
  { stage: 'optimization_started', title: '优化表达', fallback: '优化章节表达' },
  { stage: 'optimization_completed', title: '汇总建议', fallback: '生成可采纳的修改' },
];

export function AiSuggestionPanel<T extends RewriteableSectionType>({
  onAccept,
  onClose,
  originalSection,
  resumeId,
  sectionType,
}: AiSuggestionPanelProps<T>) {
  const [state, setState] = useState<PanelState>(initialState);
  const [selectedIds, setSelectedIds] = useState<string[]>([]);
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
    setSelectedIds([]);

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
        setState((current) => ({
          ...current,
          phase: 'error',
          error: errorCodeMessages[event.code] ?? event.message,
          suggestion: undefined,
        }));
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

  const analysis = state.suggestion?.newContent.analysis;
  const suggestions = useMemo(() => analysis?.suggestions ?? [], [analysis]);
  const appliedSuggestions = useMemo(
    () => suggestions.filter((suggestion) => suggestion.status === 'applied'),
    [suggestions],
  );
  const rejectedSuggestions = useMemo(
    () => suggestions.filter((suggestion) => suggestion.status === 'rejected'),
    [suggestions],
  );
  const draftFields = useMemo(
    () => collectDraftFields(originalSection, appliedSuggestions, selectedIds),
    [appliedSuggestions, originalSection, selectedIds],
  );

  const toggleSuggestion = (id: string) => {
    setSelectedIds((current) => (
      current.includes(id) ? current.filter((candidate) => candidate !== id) : [...current, id]
    ));
  };

  const selectAll = () => {
    setSelectedIds(appliedSuggestions.map((suggestion) => suggestion.id));
  };

  const accept = () => {
    if (state.phase !== 'ready' || draftFields.length === 0) return;
    cancelActiveRequest();
    onAccept(buildRewritePayload(originalSection, draftFields) as RewriteContentBySectionType[T]);
  };

  const latestProgress = state.progress[state.progress.length - 1];
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
            <p>AI RESUME DIAGNOSIS</p>
            <h2 id={titleId}>AI 诊断与润色</h2>
          </div>
          <button aria-label="关闭 AI 润色建议" onClick={closePanel} ref={closeButtonRef} title="关闭" type="button">×</button>
        </header>

        <p aria-live="polite" className="ai-suggestion-panel__status" id={statusId}>{statusLabel}</p>

        {(state.phase === 'loading' || state.phase === 'streaming' || state.phase === 'ready') && (
          <section className="ai-suggestion-panel__progress" aria-label="AI 润色进度">
            {progressSteps.filter((step) => (
              step.stage !== 'contract_repair'
              || state.progress.some((progress) => progress.stage === 'contract_repair')
            )).map((step, index) => {
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

        {state.suggestion && state.phase === 'ready' && analysis && (
          <div className="ai-suggestion-panel__content">
            <DimensionOverview analysis={analysis} suggestions={suggestions} />
            <KeywordAlignment analysis={analysis} />

            <section aria-label="改写建议" className="ai-suggestion-panel__analysis-section">
              <h4>改写建议{selectedIds.length > 0 ? `（已选 ${selectedIds.length} 条）` : ''}</h4>
              {appliedSuggestions.length === 0 ? (
                <p>本次没有可安全采纳的改写，保留原文。</p>
              ) : (
                <ul className="ai-suggestion-panel__suggestions">
                  {appliedSuggestions.map((suggestion) => (
                    <SuggestionCard
                      densityNote={densityNoteFor(analysis, suggestion)}
                      key={suggestion.id}
                      location={itemLabel(originalSection, suggestion.target.itemId ?? '')}
                      onToggle={() => toggleSuggestion(suggestion.id)}
                      selected={selectedIds.includes(suggestion.id)}
                      suggestion={suggestion}
                    />
                  ))}
                </ul>
              )}
            </section>

            {rejectedSuggestions.length > 0 && (
              <details className="ai-suggestion-panel__rejected">
                <summary>已跳过 {rejectedSuggestions.length} 条建议</summary>
                <ul>
                  {rejectedSuggestions.map((suggestion) => (
                    <li key={suggestion.id}>
                      <strong>{suggestion.code || '建议'}</strong>
                      <span>{suggestion.rejectionReason ?? '未通过安全校验'}</span>
                    </li>
                  ))}
                </ul>
              </details>
            )}

            {analysis.evidenceRequests.length > 0 && (
              <section aria-label="待补充数据" className="ai-suggestion-panel__analysis-section">
                <h4>待补充数据</h4>
                <ul className="ai-suggestion-panel__evidence-cards">
                  {analysis.evidenceRequests.map((request, index) => (
                    <li key={`${request.target.itemId ?? ''}-${request.target.field}-${index}`}>
                      <strong>{request.code} · {request.reason}</strong>
                      <span>示例：{request.exampleWithPlaceholder}</span>
                      <small>不会随采纳写入简历</small>
                    </li>
                  ))}
                </ul>
              </section>
            )}

            {analysis.warnings.length > 0 && (
              <section aria-label="提示" className="ai-suggestion-panel__analysis-section">
                <h4>提示</h4>
                <ul className="ai-suggestion-panel__analysis-list">
                  {analysis.warnings.map((warning, index) => (
                    <li key={`${warning.code}-${index}`}>
                      <strong>{warning.code}</strong>
                      <span>{warning.resolution}</span>
                    </li>
                  ))}
                </ul>
              </section>
            )}
          </div>
        )}

        <footer className="ai-suggestion-panel__actions">
          <button onClick={closePanel} type="button">关闭</button>
          <button onClick={runRewrite} type="button">重新生成</button>
          <button disabled={appliedSuggestions.length === 0} onClick={selectAll} type="button">全部应用</button>
          <button disabled={draftFields.length === 0} onClick={accept} type="button">应用到简历</button>
        </footer>
      </aside>
    </div>,
    document.body,
  );
}

function densityNoteFor(analysis: OptimizationAnalysisPayload, suggestion: AiSuggestion): string | undefined {
  const metric = analysis.density.find((candidate) => (
    candidate.field === suggestion.target.field
    && (candidate.itemId ?? '') === (suggestion.target.itemId ?? '')
  ));
  if (!metric || !metric.overLimit) return undefined;
  if (suggestion.target.field === 'highlightsMarkdown') {
    return `信息密度：当前最长一条 ${metric.maxBulletChars} 字 / 上限 40 字，共 ${metric.bullets} 条 / 上限 5 条`;
  }
  return `信息密度：当前 ${metric.chars} 字 / 上限 60 字，共 ${metric.lines} 行 / 上限 2 行`;
}
