import {
  useCallback,
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type FormEvent,
} from 'react';
import { createPortal } from 'react-dom';

import { streamGenerate } from '../api/ai';
import { createResume, documentFromStored } from '../api/resumes';
import { getTargetJob } from '../api/targetJobs';
import { templateRegistry } from '../templates/registry';
import type { GenerateSSEEvent } from '../types/ai';
import type { ResumeDocument, TemplateId } from '../types/resume';
import type { TargetJob } from '../types/targetJob';
import { TargetJobPicker } from './TargetJobPicker';

type ModalPhase = 'form' | 'generating' | 'ready' | 'error' | 'saving';

export interface AiGenerateModalProps {
  open: boolean;
  onClose: () => void;
  onCreated: (newResumeId: number) => void;
}

interface ModalState {
  phase: ModalPhase;
  thinking: string;
  error?: string;
  generated?: ResumeDocument;
  streamedTitle?: string;
}

const initialState: ModalState = { phase: 'form', thinking: '' };
const focusableSelector = 'button:not(:disabled), input:not(:disabled), textarea:not(:disabled), select:not(:disabled), [href], [tabindex]:not([tabindex="-1"])';

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function targetJobName(targetJob: TargetJob) {
  return `${targetJob.company.trim() || '未指定公司'} · ${targetJob.position.trim() || '未指定职位'}`;
}

function generationError(code: string, message: string) {
  if (code === 'NOT_IMPLEMENTED') return 'AI 快速生成正在准备中，请稍后重试。';
  return message.trim() || 'AI 生成失败，请稍后重试。';
}

function requestError(error: unknown) {
  return error instanceof Error && error.message.trim() !== ''
    ? error.message
    : 'AI 生成失败，请稍后重试。';
}

function normalizeGeneratedDocument(
  value: unknown,
  templateId: TemplateId,
  targetJobId?: number,
): ResumeDocument | undefined {
  if (!isRecord(value) || !isRecord(value.profile)) return undefined;
  const { targetJobId: _untrustedTargetJobId, ...withoutTargetJob } = value;
  const title = typeof value.title === 'string' && value.title.trim() !== ''
    ? value.title.trim()
    : 'AI 生成简历';
  const candidate = {
    ...withoutTargetJob,
    title,
    templateId,
    ...(targetJobId === undefined ? {} : { targetJobId }),
    profile: {
      ...value.profile,
      // A generated document must never claim an existing uploaded asset.
      avatarUrl: '',
      avatarStorageKey: '',
    },
  };

  try {
    return documentFromStored({
      id: 0,
      title,
      templateId,
      content: JSON.stringify(candidate),
      createdAt: '',
      updatedAt: '',
    });
  } catch {
    return undefined;
  }
}

export function AiGenerateModal({ open, onClose, onCreated }: AiGenerateModalProps) {
  const [rawText, setRawText] = useState('');
  const [templateId, setTemplateId] = useState<TemplateId | ''>('');
  const [targetJobId, setTargetJobId] = useState<number>();
  const [targetJob, setTargetJob] = useState<TargetJob>();
  const [targetLoading, setTargetLoading] = useState(false);
  const [pickerOpen, setPickerOpen] = useState(false);
  const [state, setState] = useState<ModalState>(initialState);
  const titleId = useId();
  const descriptionId = useId();
  const rawTextHintId = useId();
  const dialogRef = useRef<HTMLDivElement>(null);
  const closeButtonRef = useRef<HTMLButtonElement>(null);
  const targetButtonRef = useRef<HTMLButtonElement>(null);
  const previousFocusRef = useRef<HTMLElement | null>(null);
  const abortRef = useRef<AbortController>();
  const requestVersionRef = useRef(0);
  const targetVersionRef = useRef(0);
  const applyLockRef = useRef(false);
  const pickerOpenRef = useRef(pickerOpen);
  const phaseRef = useRef(state.phase);
  const onCloseRef = useRef(onClose);
  const onCreatedRef = useRef(onCreated);
  pickerOpenRef.current = pickerOpen;
  phaseRef.current = state.phase;
  onCloseRef.current = onClose;
  onCreatedRef.current = onCreated;

  const cancelGeneration = useCallback(() => {
    requestVersionRef.current += 1;
    abortRef.current?.abort();
    abortRef.current = undefined;
  }, []);

  const closeModal = useCallback(() => {
    if (phaseRef.current === 'saving') return;
    cancelGeneration();
    setPickerOpen(false);
    onCloseRef.current();
  }, [cancelGeneration]);

  useEffect(() => {
    if (!open) {
      cancelGeneration();
      targetVersionRef.current += 1;
      return;
    }
    setRawText('');
    setTemplateId('');
    setTargetJobId(undefined);
    setTargetJob(undefined);
    setTargetLoading(false);
    setPickerOpen(false);
    setState(initialState);
    applyLockRef.current = false;
  }, [cancelGeneration, open]);

  useEffect(() => () => {
    cancelGeneration();
    targetVersionRef.current += 1;
  }, [cancelGeneration]);

  useLayoutEffect(() => {
    if (!open) return;
    const dialog = dialogRef.current;
    if (!dialog) return;
    previousFocusRef.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;

    const focusableElements = () => Array.from(dialog.querySelectorAll<HTMLElement>(focusableSelector));
    const handleFocusIn = (event: FocusEvent) => {
      if (pickerOpenRef.current) return;
      if (!dialog.contains(event.target as Node)) closeButtonRef.current?.focus();
    };
    const handleKeyDown = (event: KeyboardEvent) => {
      if (pickerOpenRef.current) return;
      if (event.key === 'Escape') {
        event.preventDefault();
        closeModal();
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
      const previousFocus = previousFocusRef.current;
      queueMicrotask(() => {
        if (previousFocus?.isConnected) previousFocus.focus();
      });
      previousFocusRef.current = null;
    };
  }, [closeModal, open]);

  const selectTargetJob = (id: number) => {
    const version = ++targetVersionRef.current;
    setTargetJobId(id);
    setTargetJob(undefined);
    setTargetLoading(true);
    void getTargetJob(id).then((item) => {
      if (version === targetVersionRef.current) setTargetJob(item);
    }).catch(() => {
      // The selected id still came from the authenticated picker. Keep it and
      // fall back to an id label when the detail refresh fails.
    }).finally(() => {
      if (version === targetVersionRef.current) setTargetLoading(false);
    });
  };

  const clearTargetJob = () => {
    targetVersionRef.current += 1;
    setTargetJobId(undefined);
    setTargetJob(undefined);
    setTargetLoading(false);
  };

  const runGeneration = useCallback(() => {
    const normalizedRawText = rawText.trim();
    if (normalizedRawText.length < 30) {
      setState({ phase: 'form', thinking: '', error: '你的经历与优势至少 30 个字符。' });
      return;
    }
    if (templateId === '') {
      setState({ phase: 'form', thinking: '', error: '请选择简历模板。' });
      return;
    }

    const requestVersion = ++requestVersionRef.current;
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;
    applyLockRef.current = false;
    setState({ phase: 'generating', thinking: '' });
    let pendingContent: unknown;
    let reachedTerminalEvent = false;
    let receivedError = false;

    const onEvent = (event: GenerateSSEEvent) => {
      if (requestVersion !== requestVersionRef.current || controller.signal.aborted || reachedTerminalEvent) return;
      if (event.kind === 'thinking') {
        if (receivedError) return;
        setState((current) => ({ ...current, phase: 'generating', thinking: current.thinking + event.text }));
        return;
      }
      if (event.kind === 'suggestion') {
        if (receivedError) return;
        pendingContent = event.newContent;
        const streamedTitle = isRecord(event.newContent) && typeof event.newContent.title === 'string'
          ? event.newContent.title
          : undefined;
        setState((current) => ({ ...current, streamedTitle }));
        return;
      }
      if (event.kind === 'error') {
        receivedError = true;
        setState({ phase: 'error', thinking: '', error: generationError(event.code, event.message) });
        return;
      }

      reachedTerminalEvent = true;
      if (receivedError) return;
      const generated = normalizeGeneratedDocument(pendingContent, templateId, targetJobId);
      setState(generated
        ? { phase: 'ready', thinking: '', generated }
        : { phase: 'error', thinking: '', error: 'AI 返回的简历格式无效，请重试生成。' });
    };

    void streamGenerate({
      rawText: normalizedRawText,
      ...(targetJobId === undefined ? {} : { targetJobId }),
      templateId,
    }, onEvent, controller.signal).then(() => {
      if (
        requestVersion !== requestVersionRef.current
        || controller.signal.aborted
        || reachedTerminalEvent
        || receivedError
      ) return;
      const generated = normalizeGeneratedDocument(pendingContent, templateId, targetJobId);
      setState(generated
        ? { phase: 'ready', thinking: '', generated }
        : { phase: 'error', thinking: '', error: 'AI 响应提前结束，请重试生成。' });
    }).catch((error: unknown) => {
      if (
        requestVersion !== requestVersionRef.current
        || controller.signal.aborted
        || reachedTerminalEvent
        || receivedError
      ) return;
      setState({ phase: 'error', thinking: '', error: requestError(error) });
    });
  }, [rawText, targetJobId, templateId]);

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    runGeneration();
  };

  const applyGenerated = async () => {
    if (state.phase !== 'ready' || !state.generated || applyLockRef.current) return;
    applyLockRef.current = true;
    const version = requestVersionRef.current;
    const generated = state.generated;
    setState((current) => ({ ...current, phase: 'saving', error: undefined }));

    let created;
    try {
      created = await createResume(generated);
    } catch {
      if (version !== requestVersionRef.current) return;
      applyLockRef.current = false;
      setState((current) => ({
        ...current,
        phase: 'ready',
        error: '新建简历保存失败，请重试。',
      }));
      return;
    }

    if (version !== requestVersionRef.current) return;
    try {
      onCreatedRef.current(created.id);
    } finally {
      onCloseRef.current();
    }
  };

  if (!open) return null;

  const busy = state.phase === 'generating' || state.phase === 'saving';
  const selectedTargetLabel = targetLoading
    ? '正在读取目标岗位…'
    : targetJob
      ? targetJobName(targetJob)
      : targetJobId === undefined
        ? '未选择，将按通用求职标准生成'
        : `目标岗位 #${targetJobId}`;

  return createPortal(
    <>
      <div className="ai-generate-modal__backdrop">
        <div
          aria-busy={busy}
          aria-describedby={descriptionId}
          aria-hidden={pickerOpen || undefined}
          aria-labelledby={titleId}
          aria-modal="true"
          className="ai-generate-modal"
          ref={dialogRef}
          role="dialog"
        >
          <header className="ai-generate-modal__header">
            <div>
              <p>AI RESUME DRAFT</p>
              <h2 id={titleId}>AI 快速生成</h2>
            </div>
            <button
              aria-label="关闭 AI 快速生成"
              disabled={state.phase === 'saving'}
              onClick={closeModal}
              ref={closeButtonRef}
              title="关闭"
              type="button"
            >×</button>
          </header>

          <p className="ai-generate-modal__description" id={descriptionId}>
            把原始经历交给 AI 梳理，完成后由你确认再新建简历。
          </p>

          {(state.phase === 'form') && (
            <form className="ai-generate-modal__form" onSubmit={submit}>
              <label>
                你的经历与优势
                <textarea
                  aria-describedby={rawTextHintId}
                  aria-invalid={Boolean(state.error && rawText.trim().length < 30)}
                  disabled={busy}
                  onChange={(event) => setRawText(event.target.value)}
                  placeholder="例如：我有 5 年 Go 后端经验，负责过支付平台与稳定性治理…"
                  rows={8}
                  value={rawText}
                />
              </label>
              <p className="ai-generate-modal__hint" id={rawTextHintId}>至少 30 个字符；可以包含经历、项目、教育背景和可量化成果。</p>

              <div className="ai-generate-modal__field-group">
                <span className="ai-generate-modal__label">目标岗位（可选）</span>
                <div className="ai-generate-modal__target-row">
                  <span>{selectedTargetLabel}</span>
                  <div>
                    {targetJobId !== undefined && <button disabled={busy} onClick={clearTargetJob} type="button">跳过</button>}
                    <button disabled={busy} onClick={() => setPickerOpen(true)} ref={targetButtonRef} type="button">
                      {targetJobId === undefined ? '选择目标岗位' : '更换目标岗位'}
                    </button>
                  </div>
                </div>
              </div>

              <label>
                简历模板
                <select disabled={busy} onChange={(event) => setTemplateId(event.target.value as TemplateId | '')} value={templateId}>
                  <option value="">请选择模板</option>
                  {templateRegistry.map((template) => <option key={template.id} value={template.id}>{template.name}</option>)}
                </select>
              </label>

              {state.error && <p className="ai-generate-modal__error" role="alert">{state.error}</p>}

              <footer className="ai-generate-modal__actions">
                <button onClick={closeModal} type="button">取消</button>
                <button type="submit">开始生成</button>
              </footer>
            </form>
          )}

          {state.phase === 'generating' && (
            <section className="ai-generate-modal__progress" aria-live="polite">
              <span aria-hidden="true" className="ai-suggestion-panel__pulse" />
              <div>
                <strong>正在生成简历…</strong>
                <p>{state.thinking || '正在阅读你的经历并规划简历结构。'}</p>
                {state.streamedTitle && <span>已起草：{state.streamedTitle}</span>}
              </div>
            </section>
          )}

          {state.phase === 'error' && (
            <section className="ai-generate-modal__result ai-generate-modal__result--error">
              <div className="ai-generate-modal__error-card" role="alert">
                <strong>本次生成未完成</strong>
                <span>{state.error}</span>
              </div>
              <footer className="ai-generate-modal__actions">
                <button onClick={closeModal} type="button">取消</button>
                <button onClick={() => setState(initialState)} type="button">修改输入</button>
                <button onClick={runGeneration} type="button">重试生成</button>
              </footer>
            </section>
          )}

          {(state.phase === 'ready' || state.phase === 'saving') && state.generated && (
            <section className="ai-generate-modal__result">
              <div className="ai-generate-modal__resume-summary">
                <p>初稿已完成</p>
                <h3>{state.generated.title}</h3>
                <dl>
                  <div><dt>姓名</dt><dd>{state.generated.profile.fullName || '待补充'}</dd></div>
                  <div><dt>章节</dt><dd>{state.generated.sections.length} 个</dd></div>
                  <div><dt>模板</dt><dd>{templateRegistry.find((item) => item.id === state.generated?.templateId)?.name}</dd></div>
                </dl>
              </div>
              {state.error && <p className="ai-generate-modal__error" role="alert">{state.error}</p>}
              <footer className="ai-generate-modal__actions">
                <button disabled={state.phase === 'saving'} onClick={closeModal} type="button">取消</button>
                <button disabled={state.phase === 'saving'} onClick={() => void applyGenerated()} type="button">
                  {state.phase === 'saving' ? '正在新建简历…' : '应用到新建简历'}
                </button>
              </footer>
            </section>
          )}
        </div>
      </div>

      <TargetJobPicker
        onClose={() => setPickerOpen(false)}
        onSelect={selectTargetJob}
        open={pickerOpen}
        returnFocusRef={targetButtonRef}
        selectedId={targetJobId}
      />
    </>,
    document.body,
  );
}
