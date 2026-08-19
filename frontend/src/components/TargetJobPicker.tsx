import { useCallback, useEffect, useId, useRef, useState, type FormEvent } from 'react';
import { createPortal } from 'react-dom';
import { createTargetJob, listTargetJobs } from '../api/targetJobs';
import type { TargetJob, TargetJobInput } from '../types/targetJob';

const EMPTY_INPUT: TargetJobInput = { company: '', position: '', requirements: '' };

export interface TargetJobPickerProps {
  open: boolean;
  onClose: () => void;
  onSelect: (id: number) => void;
  selectedId?: number;
}

function targetJobName(targetJob: TargetJob) {
  return `${targetJob.company.trim() || '未指定公司'} · ${targetJob.position.trim() || '未指定职位'}`;
}

function requirementsPreview(requirements: string) {
  const normalized = requirements.trim();
  return normalized.length > 90 ? `${normalized.slice(0, 90)}…` : normalized;
}

export function TargetJobPicker({ open, onClose, onSelect, selectedId }: TargetJobPickerProps) {
  const [targetJobs, setTargetJobs] = useState<TargetJob[]>([]);
  const [loading, setLoading] = useState(false);
  const [loadError, setLoadError] = useState(false);
  const [mode, setMode] = useState<'list' | 'create'>('list');
  const [input, setInput] = useState<TargetJobInput>(EMPTY_INPUT);
  const [formError, setFormError] = useState('');
  const [saving, setSaving] = useState(false);
  const dialogRef = useRef<HTMLDivElement>(null);
  const companyInputRef = useRef<HTMLInputElement>(null);
  const previousFocusRef = useRef<HTMLElement | null>(null);
  const requestVersion = useRef(0);
  const titleId = useId();
  const hintId = useId();
  const errorId = useId();
  const onCloseRef = useRef(onClose);
  const savingRef = useRef(saving);
  onCloseRef.current = onClose;
  savingRef.current = saving;

  const load = useCallback(async () => {
    const version = ++requestVersion.current;
    setLoading(true);
    setLoadError(false);
    try {
      const jobs = await listTargetJobs();
      if (version === requestVersion.current) setTargetJobs(jobs);
    } catch {
      if (version === requestVersion.current) setLoadError(true);
    } finally {
      if (version === requestVersion.current) setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (!open) {
      requestVersion.current += 1;
      return;
    }
    setMode('list');
    setInput(EMPTY_INPUT);
    setFormError('');
    setSaving(false);
    void load();
  }, [load, open]);

  useEffect(() => {
    if (!open) return;
    const dialog = dialogRef.current;
    if (!dialog) return;
    previousFocusRef.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const focusableSelector = 'button:not(:disabled), input:not(:disabled), textarea:not(:disabled), [tabindex]:not([tabindex="-1"])';
    const focusableElements = () => Array.from(dialog.querySelectorAll<HTMLElement>(focusableSelector));
    const initialFocus = focusableElements()[0];

    const handleFocusIn = (event: FocusEvent) => {
      if (!dialog.contains(event.target as Node)) focusableElements()[0]?.focus();
    };
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && !savingRef.current) {
        event.preventDefault();
        onCloseRef.current();
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
    initialFocus?.focus();
    return () => {
      document.removeEventListener('focusin', handleFocusIn);
      document.removeEventListener('keydown', handleKeyDown);
      previousFocusRef.current?.focus();
      previousFocusRef.current = null;
    };
  }, [open]);

  useEffect(() => {
    if (open && mode === 'create') companyInputRef.current?.focus();
  }, [mode, open]);

  if (!open) return null;

  const close = () => {
    if (!saving) onClose();
  };

  const select = (id: number) => {
    onSelect(id);
    onClose();
  };

  const startCreate = () => {
    setInput(EMPTY_INPUT);
    setFormError('');
    setMode('create');
  };

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const normalized = {
      company: input.company.trim(),
      position: input.position.trim(),
      requirements: input.requirements.trim(),
    };
    if (normalized.requirements.length < 10) {
      setFormError('岗位要求至少需要 10 个字符。');
      return;
    }

    setSaving(true);
    setFormError('');
    try {
      const created = await createTargetJob(normalized);
      onSelect(created.id);
      onClose();
    } catch {
      setFormError('保存目标岗位失败，请稍后重试。');
      setSaving(false);
    }
  };

  return createPortal(
    <div className="target-job-dialog__backdrop target-job-picker__backdrop">
      <div
        aria-labelledby={titleId}
        aria-modal="true"
        className="target-job-dialog target-job-picker"
        ref={dialogRef}
        role="dialog"
      >
        <div className="target-job-dialog__heading">
          <div>
            <p className="target-jobs__eyebrow">RESUME DIRECTION</p>
            <h3 id={titleId}>{mode === 'list' ? '选择目标岗位' : '新建并选择目标岗位'}</h3>
          </div>
          <button aria-label="关闭目标岗位选择器" disabled={saving} onClick={close} title="关闭" type="button">×</button>
        </div>

        {mode === 'list' ? (
          <>
            <p className="target-job-picker__guidance">选择的岗位会写入简历，并在自动保存时同步。</p>
            {loading ? (
              <p aria-live="polite" className="target-job-picker__status">正在加载目标岗位…</p>
            ) : loadError ? (
              <div className="target-jobs__load-error target-job-picker__load-error">
                <p role="alert">无法加载目标岗位，请稍后重试。</p>
                <button onClick={() => void load()} type="button">重试</button>
              </div>
            ) : targetJobs.length === 0 ? (
              <div className="target-job-picker__empty">
                <strong>还没有可选的目标岗位</strong>
                <span>新建一份岗位简报后会直接绑定到当前简历。</span>
              </div>
            ) : (
              <ul aria-label="可选目标岗位" className="target-job-picker__list">
                {targetJobs.map((targetJob) => {
                  const name = targetJobName(targetJob);
                  const current = targetJob.id === selectedId;
                  return (
                    <li className="target-job-picker__item" key={targetJob.id}>
                      <div>
                        <div className="target-job-picker__identity">
                          <strong>{name}</strong>
                          {current && <span>当前绑定</span>}
                        </div>
                        <p>{requirementsPreview(targetJob.requirements)}</p>
                      </div>
                      <button
                        aria-label={`选择：${name}`}
                        disabled={current}
                        onClick={() => select(targetJob.id)}
                        type="button"
                      >
                        {current ? '已选择' : '选择'}
                      </button>
                    </li>
                  );
                })}
              </ul>
            )}
            <div className="target-job-picker__footer">
              <button className="target-job-picker__create" onClick={startCreate} type="button">+ 新建目标岗位</button>
              <button onClick={close} type="button">取消</button>
            </div>
          </>
        ) : (
          <form className="target-job-form" noValidate onSubmit={submit}>
            <p className="target-job-dialog__guidance">不知道公司 / 职位也能保存——岗位要求是 AI 润色的核心依据。</p>
            <div className="target-job-form__optional-fields">
              <label>
                <span>公司（可选）</span>
                <input
                  maxLength={200}
                  onChange={(event) => setInput({ ...input, company: event.target.value })}
                  placeholder="例如：字节跳动"
                  ref={companyInputRef}
                  value={input.company}
                />
              </label>
              <label>
                <span>职位（可选）</span>
                <input
                  maxLength={200}
                  onChange={(event) => setInput({ ...input, position: event.target.value })}
                  placeholder="例如：高级后端工程师"
                  value={input.position}
                />
              </label>
            </div>
            <label>
              <span>岗位要求</span>
              <textarea
                aria-describedby={formError ? `${hintId} ${errorId}` : hintId}
                aria-invalid={Boolean(formError)}
                maxLength={20000}
                onChange={(event) => setInput({ ...input, requirements: event.target.value })}
                placeholder="粘贴岗位职责、任职要求或你关心的关键词。支持 Markdown。"
                required
                value={input.requirements}
              />
            </label>
            <p className="target-job-form__hint" id={hintId}>至少 10 个字符；支持 Markdown。</p>
            {formError && <p className="target-job-form__error" id={errorId} role="alert">{formError}</p>}
            <div className="target-job-form__actions">
              <button disabled={saving} onClick={() => setMode('list')} type="button">返回列表</button>
              <button disabled={saving} type="submit">{saving ? '正在保存…' : '保存并选择'}</button>
            </div>
          </form>
        )}
      </div>
    </div>,
    document.body,
  );
}
