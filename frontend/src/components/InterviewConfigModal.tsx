import { useEffect, useId, useRef, useState } from 'react';
import { createPortal } from 'react-dom';

import type { InterviewConfig, InterviewLabel } from '../api/interviews';

export const DEFAULT_INTERVIEW_CONFIG: InterviewConfig = {
  questionCount: 12,
  labelCounts: { '0': 5, '1': 2, '2': 5 },
  followUpBudget: 6,
  perQuestionFollowUpLimit: 1,
};

const BUDGET_UNTOUCHED = Symbol('budget-untouched');

interface StepperProps {
  label: string;
  max: number;
  min: number;
  onStep: (delta: number) => void;
  value: number;
}

function Stepper({ label, max, min, onStep, value }: StepperProps) {
  return (
    <div className="interview-config__stepper">
      <button aria-label={`减少${label}`} disabled={value <= min} onClick={() => onStep(-1)} type="button">−</button>
      <input
        aria-label={label}
        inputMode="numeric"
        max={max}
        min={min}
        onChange={(event) => {
          const parsed = Number.parseInt(event.target.value, 10);
          onStep((Number.isNaN(parsed) ? min : parsed) - value);
        }}
        value={value}
      />
      <button aria-label={`增加${label}`} disabled={value >= max} onClick={() => onStep(1)} type="button">＋</button>
    </div>
  );
}

interface InterviewConfigModalProps {
  error?: string;
  onCancel: () => void;
  onConfirm: (config: InterviewConfig) => void;
  submitting: boolean;
  targetJobName: string;
}

export function InterviewConfigModal({ error, onCancel, onConfirm, submitting, targetJobName }: InterviewConfigModalProps) {
  const [questionCount, setQuestionCount] = useState(DEFAULT_INTERVIEW_CONFIG.questionCount);
  const [labelCounts, setLabelCounts] = useState<InterviewConfig['labelCounts']>({ ...DEFAULT_INTERVIEW_CONFIG.labelCounts });
  const [budget, setBudget] = useState<number | typeof BUDGET_UNTOUCHED>(BUDGET_UNTOUCHED);
  const [perQuestionLimit, setPerQuestionLimit] = useState(DEFAULT_INTERVIEW_CONFIG.perQuestionFollowUpLimit);
  const [localError, setLocalError] = useState('');
  const dialogRef = useRef<HTMLDivElement>(null);
  const onCancelRef = useRef(onCancel);
  const submittingRef = useRef(submitting);
  const titleId = useId();
  onCancelRef.current = onCancel;
  submittingRef.current = submitting;

  useEffect(() => {
    const dialog = dialogRef.current;
    if (!dialog) return;
    const previousFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const focusableSelector = 'button:not(:disabled), input:not(:disabled), textarea:not(:disabled)';
    const focusableElements = () => Array.from(dialog.querySelectorAll<HTMLElement>(focusableSelector));
    const handleFocusIn = (event: FocusEvent) => {
      if (!dialog.contains(event.target as Node)) focusableElements()[0]?.focus();
    };
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && !submittingRef.current) {
        event.preventDefault();
        onCancelRef.current();
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
    focusableElements()[0]?.focus();
    return () => {
      document.removeEventListener('focusin', handleFocusIn);
      document.removeEventListener('keydown', handleKeyDown);
      previousFocus?.focus();
    };
  }, []);

  const clamp = (value: number, min: number, max: number) => Math.min(max, Math.max(min, value));

  const stepQuestionCount = (delta: number) => {
    const next = clamp(questionCount + delta, 1, 20);
    setQuestionCount(next);
    if (budget === BUDGET_UNTOUCHED) setBudget(Math.ceil(next / 2));
    setLocalError('');
  };

  const stepLabelCount = (label: InterviewLabel) => (delta: number) => {
    setLabelCounts((counts) => ({ ...counts, [label]: clamp(counts[label] + delta, 0, 20) }));
    setLocalError('');
  };

  const labelSum = labelCounts['0'] + labelCounts['1'] + labelCounts['2'];

  const handleSubmit = () => {
    if (labelSum !== questionCount) {
      setLocalError('各标签数量之和应等于题目总数');
      return;
    }
    onConfirm({
      questionCount,
      labelCounts: { ...labelCounts },
      followUpBudget: budget === BUDGET_UNTOUCHED ? Math.ceil(questionCount / 2) : budget,
      perQuestionFollowUpLimit: perQuestionLimit,
    });
  };

  return createPortal(
    <div className="interview-dialog__backdrop">
      <div aria-labelledby={titleId} aria-modal="true" className="interview-dialog" ref={dialogRef} role="dialog">
        <div className="interview-dialog__heading">
          <div>
            <p className="interview-dialog__eyebrow">AI MOCK INTERVIEW</p>
            <h3 id={titleId}>开始 AI 面试</h3>
          </div>
          <button aria-label="关闭" disabled={submitting} onClick={onCancel} type="button">×</button>
        </div>
        <p className="interview-dialog__guidance">
          将基于「{targetJobName}」与当前简历生成题目并逐题评分（C ~ S 六级）。开始后配置将生成快照并锁定，面试中不可修改。
        </p>
        <div className="interview-config">
          <div className="interview-config__row">
            <div className="interview-config__label">
              <strong>题目总数</strong>
              <span>覆盖 Go 基础、数据库、Redis 与简历项目</span>
            </div>
            <Stepper label="题目总数" max={20} min={1} onStep={stepQuestionCount} value={questionCount} />
          </div>
          <div className="interview-config__row">
            <div className="interview-config__label">
              <strong>各标签数量</strong>
              <span>
                八股（Go / 数据库 / Redis）· 算法 · 项目，当前合计 {labelSum} / {questionCount}
              </span>
            </div>
            <div className="interview-config__labels">
              <div className="interview-config__label-count">
                <span className="interview-chip interview-chip--octopus">八股</span>
                <Stepper label="八股题数量" max={20} min={0} onStep={stepLabelCount('0')} value={labelCounts['0']} />
              </div>
              <div className="interview-config__label-count">
                <span className="interview-chip interview-chip--algorithm">算法</span>
                <Stepper label="算法题数量" max={20} min={0} onStep={stepLabelCount('1')} value={labelCounts['1']} />
              </div>
              <div className="interview-config__label-count">
                <span className="interview-chip interview-chip--project">项目</span>
                <Stepper label="项目题数量" max={20} min={0} onStep={stepLabelCount('2')} value={labelCounts['2']} />
              </div>
            </div>
          </div>
          <div className="interview-config__row">
            <div className="interview-config__label">
              <strong>追问总预算</strong>
              <span>默认 = 题目数 ÷ 2；用尽后全局禁止追问</span>
            </div>
            <Stepper
              label="追问总预算"
              max={20}
              min={0}
              onStep={(delta) => { setBudget((current) => clamp((current === BUDGET_UNTOUCHED ? Math.ceil(questionCount / 2) : current) + delta, 0, 20)); }}
              value={budget === BUDGET_UNTOUCHED ? Math.ceil(questionCount / 2) : budget}
            />
          </div>
          <div className="interview-config__row">
            <div className="interview-config__label">
              <strong>单问追问上限</strong>
              <span>同一道主问题最多被追问次数</span>
            </div>
            <Stepper label="单问追问上限" max={3} min={0} onStep={(delta) => setPerQuestionLimit((current) => clamp(current + delta, 0, 3))} value={perQuestionLimit} />
          </div>
        </div>
        {(localError || error) && <p className="interview-dialog__error" role="alert">{localError || error}</p>}
        <div className="interview-dialog__actions">
          <button disabled={submitting} onClick={onCancel} type="button">取消</button>
          <button disabled={submitting} onClick={handleSubmit} type="button">{submitting ? '正在生题，约需 10~20 秒…' : '开始面试'}</button>
        </div>
      </div>
    </div>,
    document.body,
  );
}
