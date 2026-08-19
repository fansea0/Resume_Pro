import { useEffect, useId, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import type { TargetJob } from '../types/targetJob';
import { TargetJobPicker } from './TargetJobPicker';

export interface TargetJobCardProps {
  targetJob?: TargetJob;
  onChange: (targetJobId: number | null) => void;
}

interface ConfirmationDialogProps {
  confirmLabel: string;
  description: string;
  destructive?: boolean;
  onCancel: () => void;
  onConfirm: () => void;
  title: string;
}

function ConfirmationDialog({ confirmLabel, description, destructive, onCancel, onConfirm, title }: ConfirmationDialogProps) {
  const dialogRef = useRef<HTMLDivElement>(null);
  const cancelRef = useRef<HTMLButtonElement>(null);
  const previousFocusRef = useRef<HTMLElement | null>(null);
  const titleId = useId();
  const descriptionId = useId();
  const onCancelRef = useRef(onCancel);
  onCancelRef.current = onCancel;

  useEffect(() => {
    const dialog = dialogRef.current;
    if (!dialog) return;
    previousFocusRef.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const buttons = () => Array.from(dialog.querySelectorAll<HTMLElement>('button:not(:disabled)'));
    const handleFocusIn = (event: FocusEvent) => {
      if (!dialog.contains(event.target as Node)) cancelRef.current?.focus();
    };
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault();
        onCancelRef.current();
        return;
      }
      if (event.key !== 'Tab') return;
      const elements = buttons();
      const first = elements[0];
      const last = elements[elements.length - 1];
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last?.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first?.focus();
      }
    };

    document.addEventListener('focusin', handleFocusIn);
    document.addEventListener('keydown', handleKeyDown);
    cancelRef.current?.focus();
    return () => {
      document.removeEventListener('focusin', handleFocusIn);
      document.removeEventListener('keydown', handleKeyDown);
      previousFocusRef.current?.focus();
    };
  }, []);

  return createPortal(
    <div className="target-job-dialog__backdrop target-job-confirm__backdrop">
      <div
        aria-describedby={descriptionId}
        aria-labelledby={titleId}
        aria-modal="true"
        className="target-job-confirm"
        ref={dialogRef}
        role="alertdialog"
      >
        <div className="target-job-confirm__mark" aria-hidden="true">!</div>
        <div>
          <h3 id={titleId}>{title}</h3>
          <p id={descriptionId}>{description}</p>
        </div>
        <div className="target-job-confirm__actions">
          <button onClick={onCancel} ref={cancelRef} type="button">取消</button>
          <button className={destructive ? 'target-job-confirm__danger' : ''} onClick={onConfirm} type="button">{confirmLabel}</button>
        </div>
      </div>
    </div>,
    document.body,
  );
}

function targetJobName(targetJob: TargetJob) {
  return `${targetJob.company.trim() || '未指定公司'} · ${targetJob.position.trim() || '未指定职位'}`;
}

function collapsedRequirements(requirements: string) {
  const normalized = requirements.trim();
  return normalized.length > 80 ? `${normalized.slice(0, 80)}…` : normalized;
}

export function TargetJobCard({ targetJob, onChange }: TargetJobCardProps) {
  const [pickerOpen, setPickerOpen] = useState(false);
  const [expanded, setExpanded] = useState(false);
  const [confirmation, setConfirmation] = useState<'change' | 'clear' | null>(null);
  const headingId = useId();
  const requirementsId = useId();

  useEffect(() => setExpanded(false), [targetJob?.id]);

  const selectTargetJob = (id: number) => {
    if (id !== targetJob?.id) onChange(id);
  };

  const confirmChange = () => {
    setConfirmation(null);
    setPickerOpen(true);
  };

  const confirmClear = () => {
    setConfirmation(null);
    onChange(null);
  };

  return (
    <section aria-labelledby={headingId} className="target-job-card">
      <div className="target-job-card__heading">
        <div className="target-job-card__icon" aria-hidden="true">◎</div>
        <div>
          <p className="target-job-card__eyebrow">AI WRITING CONTEXT</p>
          <h2 id={headingId}>目标岗位</h2>
        </div>
      </div>

      {targetJob ? (
        <div className="target-job-card__bound">
          <div className="target-job-card__current-row">
            <strong>当前：{targetJobName(targetJob)}</strong>
            <button onClick={() => setConfirmation('change')} type="button">更换目标岗位</button>
          </div>
          <div className="target-job-card__requirements">
            <div className="target-job-card__requirements-heading">
              <span>JD 预览</span>
              <button
                aria-controls={requirementsId}
                aria-expanded={expanded}
                aria-label={expanded ? '收起岗位要求' : '展开岗位要求'}
                onClick={() => setExpanded((value) => !value)}
                title={expanded ? '收起岗位要求' : '展开岗位要求'}
                type="button"
              >
                {expanded ? '收起' : '展开'}
              </button>
            </div>
            <p id={requirementsId}>{expanded ? targetJob.requirements.trim() : collapsedRequirements(targetJob.requirements)}</p>
          </div>
          <button className="target-job-card__clear" onClick={() => setConfirmation('clear')} type="button">清除目标岗位绑定</button>
        </div>
      ) : (
        <div className="target-job-card__empty">
          <p>选择一份岗位简报，让后续 AI 润色更贴近你的求职方向。</p>
          <button onClick={() => setPickerOpen(true)} type="button">+ 选择目标岗位</button>
        </div>
      )}

      <TargetJobPicker
        onClose={() => setPickerOpen(false)}
        onSelect={selectTargetJob}
        open={pickerOpen}
        selectedId={targetJob?.id}
      />

      {confirmation === 'change' && (
        <ConfirmationDialog
          confirmLabel="继续更换"
          description="选择新岗位前会先保留当前绑定；只有选中新岗位后，简历才会更新并进入自动保存。"
          onCancel={() => setConfirmation(null)}
          onConfirm={confirmChange}
          title="更换目标岗位？"
        />
      )}
      {confirmation === 'clear' && (
        <ConfirmationDialog
          confirmLabel="清除绑定"
          description="清除后，这份简历的 AI 润色将不再使用当前岗位要求。"
          destructive
          onCancel={() => setConfirmation(null)}
          onConfirm={confirmClear}
          title="清除目标岗位绑定？"
        />
      )}
    </section>
  );
}
