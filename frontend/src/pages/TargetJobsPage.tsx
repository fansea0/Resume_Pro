import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react';
import {
  createTargetJob,
  deleteTargetJob,
  listTargetJobs,
  updateTargetJob,
} from '../api/targetJobs';
import type { TargetJob, TargetJobInput } from '../types/targetJob';

const EMPTY_INPUT: TargetJobInput = { company: '', position: '', requirements: '' };

function targetJobName(targetJob: TargetJob) {
  return `${targetJob.company.trim() || '未指定公司'} · ${targetJob.position.trim() || '未指定职位'}`;
}

function requirementsPreview(requirements: string) {
  const normalized = requirements.trim();
  return normalized.length > 50 ? `${normalized.slice(0, 50)}…` : normalized;
}

interface TargetJobFormProps {
  editingTarget: TargetJob | null;
  input: TargetJobInput;
  error: string;
  saving: boolean;
  onCancel: () => void;
  onChange: (input: TargetJobInput) => void;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
}

function TargetJobForm({ editingTarget, input, error, saving, onCancel, onChange, onSubmit }: TargetJobFormProps) {
  const title = editingTarget ? '编辑目标岗位' : '新建目标岗位';
  const requirementsDescribedBy = error ? 'target-job-requirements-hint target-job-form-error' : 'target-job-requirements-hint';

  return (
    <div className="target-job-dialog__backdrop">
      <div aria-labelledby="target-job-dialog-title" aria-modal="true" className="target-job-dialog" role="dialog">
        <div className="target-job-dialog__heading">
          <div>
            <p className="target-jobs__eyebrow">JOB BRIEF</p>
            <h3 id="target-job-dialog-title">{title}</h3>
          </div>
          <button aria-label="关闭" disabled={saving} onClick={onCancel} type="button">×</button>
        </div>
        <p className="target-job-dialog__guidance">不知道公司 / 职位也能保存——requirements 是 AI 润色的核心依据</p>
        <form className="target-job-form" noValidate onSubmit={onSubmit}>
          <div className="target-job-form__optional-fields">
            <label>
              <span>公司（可选）</span>
              <input
                autoFocus
                maxLength={200}
                onChange={(event) => onChange({ ...input, company: event.target.value })}
                placeholder="例如：字节跳动"
                value={input.company}
              />
            </label>
            <label>
              <span>职位（可选）</span>
              <input
                maxLength={200}
                onChange={(event) => onChange({ ...input, position: event.target.value })}
                placeholder="例如：高级后端工程师"
                value={input.position}
              />
            </label>
          </div>
          <label>
            <span>岗位要求</span>
            <textarea
              aria-describedby={requirementsDescribedBy}
              aria-invalid={Boolean(error)}
              maxLength={20000}
              onChange={(event) => onChange({ ...input, requirements: event.target.value })}
              placeholder="粘贴岗位职责、任职要求或你关心的关键词。支持 Markdown。"
              required
              value={input.requirements}
            />
          </label>
          <p className="target-job-form__hint" id="target-job-requirements-hint">至少 10 个字符；支持 Markdown。</p>
          {error && <p className="target-job-form__error" id="target-job-form-error" role="alert">{error}</p>}
          <div className="target-job-form__actions">
            <button disabled={saving} onClick={onCancel} type="button">取消</button>
            <button disabled={saving} type="submit">{saving ? '正在保存…' : editingTarget ? '保存更改' : '保存目标岗位'}</button>
          </div>
        </form>
      </div>
    </div>
  );
}

export function TargetJobsPage() {
  const [targetJobs, setTargetJobs] = useState<TargetJob[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState(false);
  const [actionError, setActionError] = useState('');
  const [formOpen, setFormOpen] = useState(false);
  const [editingTarget, setEditingTarget] = useState<TargetJob | null>(null);
  const [input, setInput] = useState<TargetJobInput>(EMPTY_INPUT);
  const [formError, setFormError] = useState('');
  const [saving, setSaving] = useState(false);
  const [deletingId, setDeletingId] = useState<number | null>(null);
  const mounted = useRef(false);
  const requestVersion = useRef(0);

  const load = useCallback(async () => {
    const version = ++requestVersion.current;
    setLoading(true);
    setLoadError(false);
    try {
      const items = await listTargetJobs();
      if (mounted.current && version === requestVersion.current) setTargetJobs(items);
    } catch {
      if (mounted.current && version === requestVersion.current) setLoadError(true);
    } finally {
      if (mounted.current && version === requestVersion.current) setLoading(false);
    }
  }, []);

  useEffect(() => {
    mounted.current = true;
    void load();
    return () => {
      mounted.current = false;
      requestVersion.current += 1;
    };
  }, [load]);

  const openCreate = () => {
    setEditingTarget(null);
    setInput(EMPTY_INPUT);
    setFormError('');
    setFormOpen(true);
  };

  const openEdit = (targetJob: TargetJob) => {
    setEditingTarget(targetJob);
    setInput({
      company: targetJob.company,
      position: targetJob.position,
      requirements: targetJob.requirements,
    });
    setFormError('');
    setFormOpen(true);
  };

  const closeForm = () => {
    if (!saving) setFormOpen(false);
  };

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
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
      if (editingTarget) {
        const updated = await updateTargetJob(editingTarget.id, normalized);
        if (!mounted.current) return;
        setTargetJobs((items) => [updated, ...items.filter((item) => item.id !== updated.id)]);
      } else {
        const created = await createTargetJob(normalized);
        if (!mounted.current) return;
        setTargetJobs((items) => [created, ...items]);
      }
      setFormOpen(false);
    } catch {
      if (mounted.current) setFormError('保存目标岗位失败，请稍后重试。');
    } finally {
      if (mounted.current) setSaving(false);
    }
  };

  const handleDelete = async (targetJob: TargetJob) => {
    const confirmed = window.confirm(
      `确定删除“${targetJobName(targetJob)}”吗？如果这份目标岗位被简历引用，删除后会同时清空这些简历的引用。`,
    );
    if (!confirmed) return;

    setDeletingId(targetJob.id);
    setActionError('');
    try {
      const result = await deleteTargetJob(targetJob.id);
      if (!mounted.current) return;
      setTargetJobs((items) => items.filter((item) => item.id !== targetJob.id));
      if (result.deletedReferences > 0) {
        window.alert(`已清空 ${result.deletedReferences} 份简历的引用`);
      }
    } catch {
      if (mounted.current) setActionError('删除目标岗位失败，请稍后重试。');
    } finally {
      if (mounted.current) setDeletingId(null);
    }
  };

  return (
    <section aria-labelledby="target-jobs-title" className="target-jobs">
      <div className="target-jobs__header">
        <div>
          <p className="target-jobs__eyebrow">AI WRITING CONTEXT</p>
          <h2 id="target-jobs-title">目标岗位</h2>
          <p>保存岗位要求，让 AI 润色更贴近你的求职方向。</p>
        </div>
        <button className="target-jobs__create" onClick={openCreate} type="button">+ 新建</button>
      </div>

      {actionError && <p className="target-jobs__error" role="alert">{actionError}</p>}
      {loading ? (
        <p className="target-jobs__status" aria-live="polite">正在加载目标岗位…</p>
      ) : loadError ? (
        <div className="target-jobs__load-error">
          <p role="alert">无法加载目标岗位，请稍后重试。</p>
          <button onClick={() => void load()} type="button">重试</button>
        </div>
      ) : targetJobs.length === 0 ? (
        <div className="target-jobs__empty">
          <p>还没有目标岗位。</p>
          <span>创建一份岗位简报，后续可在简历里直接选择。</span>
        </div>
      ) : (
        <ul aria-label="目标岗位列表" className="target-jobs__list">
          {targetJobs.map((targetJob) => (
            <li className="target-jobs__item" key={targetJob.id}>
              <article>
                <div className="target-jobs__identity">
                  <h3>{targetJob.company.trim() || '未指定公司'}</h3>
                  <span aria-hidden="true">·</span>
                  <p>{targetJob.position.trim() || '未指定职位'}</p>
                </div>
                <p className="target-jobs__requirements">{requirementsPreview(targetJob.requirements)}</p>
              </article>
              <div className="target-jobs__actions">
                <button aria-label={`编辑：${targetJobName(targetJob)}`} onClick={() => openEdit(targetJob)} type="button">编辑</button>
                <button
                  aria-label={`删除：${targetJobName(targetJob)}`}
                  disabled={deletingId === targetJob.id}
                  onClick={() => void handleDelete(targetJob)}
                  type="button"
                >{deletingId === targetJob.id ? '删除中…' : '删除'}</button>
              </div>
            </li>
          ))}
        </ul>
      )}

      {formOpen && (
        <TargetJobForm
          editingTarget={editingTarget}
          error={formError}
          input={input}
          onCancel={closeForm}
          onChange={(nextInput) => { setInput(nextInput); if (formError) setFormError(''); }}
          onSubmit={(event) => void handleSubmit(event)}
          saving={saving}
        />
      )}
    </section>
  );
}
