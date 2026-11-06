import { useCallback, useEffect, useImperativeHandle, useRef, useState, type ButtonHTMLAttributes, type ReactNode, type Ref, type RefObject } from 'react';
import { deleteResume, documentFromStored, listResumes, type StoredResume } from '../api/resumes';
import { useResumeAutosave, type ResumeAutosaveHandle } from '../hooks/useResumeAutosave';
import { templateRegistry } from '../templates/registry';
import type { ResumeDocument } from '../types/resume';
import { exportResumeMarkdown } from '../utils/exportResumeMarkdown';
import { exportResumePng } from '../utils/exportResumePng';

interface ResumeLibraryProps {
  autosaveHandleRef?: Ref<ResumeAutosaveHandle>;
  document: ResumeDocument;
  selectedId: number | null;
  onSelectedIdChange: (id: number | null) => void;
  onChange: (document: ResumeDocument) => void;
  onRestoreSample: () => void;
  onSelect: (document: ResumeDocument) => void;
  previewRef?: RefObject<HTMLElement>;
}

type LibraryIconName = 'chevronDown' | 'close' | 'copy' | 'download' | 'image' | 'restore' | 'save' | 'trash';

function LibraryIcon({ name }: { name: LibraryIconName }) {
  const paths: Record<LibraryIconName, ReactNode> = {
    chevronDown: <path d="m6 9 6 6 6-6" />,
    close: <path d="M6 6l12 12M18 6 6 18" />,
    copy: <><rect height="12" rx="1.5" width="10" x="8" y="8" /><path d="M6 16H5.5A1.5 1.5 0 0 1 4 14.5v-9A1.5 1.5 0 0 1 5.5 4h9A1.5 1.5 0 0 1 16 5.5V6" /></>,
    download: <><path d="M12 3v11M8 10l4 4 4-4M5 19h14" /></>,
    image: <><rect height="15" rx="2" width="17" x="3.5" y="4.5" /><circle cx="8.5" cy="9" r="1.2" /><path d="m5.5 17 4.2-4 2.8 2.5 2.2-2 3.8 3.5" /></>,
    restore: <><path d="M4 8.5V4.5m0 0h4m-4 0 3 3A8 8 0 1 1 5.7 15" /></>,
    save: <><path d="M5 4h12l2 2v14H5zM8 4v6h8V4M8 20v-6h8v6" /></>,
    trash: <><path d="M5 7h14M10 11v5M14 11v5M8 7l1-2h6l1 2M7 7l.8 12h8.4L17 7" /></>,
  };
  return <svg aria-hidden="true" viewBox="0 0 24 24">{paths[name]}</svg>;
}

function LibraryAction({ icon, children, className, ...props }: Omit<ButtonHTMLAttributes<HTMLButtonElement>, 'children'> & { icon: LibraryIconName; children?: ReactNode; className?: string }) {
  return <button className={`resume-library__action-button${className ? ` ${className}` : ''}`} type="button" {...props}><LibraryIcon name={icon} />{children !== undefined && <span>{children}</span>}</button>;
}

function templateName(templateId: StoredResume['templateId']) {
  return templateRegistry.find((template) => template.id === templateId)?.name ?? '未知模板';
}

function formatDateTime(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '';
  return date.toLocaleString('zh-CN', { day: '2-digit', hour: '2-digit', hour12: false, minute: '2-digit', month: '2-digit' });
}

export function ResumeLibrary({ autosaveHandleRef, document, selectedId, onSelectedIdChange, onChange, onRestoreSample, onSelect, previewRef }: ResumeLibraryProps) {
  const [items, setItems] = useState<StoredResume[]>([]);
  const [loading, setLoading] = useState(true);
  const [deleting, setDeleting] = useState(false);
  const [pendingDeleteId, setPendingDeleteId] = useState<number | null>(null);
  const [confirmRestore, setConfirmRestore] = useState(false);
  const [exporting, setExporting] = useState(false);
  const [exportMenuOpen, setExportMenuOpen] = useState(false);
  const exportAreaRef = useRef<HTMLDivElement | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let active = true;
    listResumes()
      .then((resumes) => active && setItems(resumes))
      .catch(() => active && setError('无法加载已保存的简历。'))
      .finally(() => active && setLoading(false));
    return () => { active = false; };
  }, []);

  useEffect(() => {
    if (!exportMenuOpen) return;
    const closeOnOutside = (event: MouseEvent) => {
      if (!exportAreaRef.current?.contains(event.target as Node)) setExportMenuOpen(false);
    };
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setExportMenuOpen(false);
    };
    window.addEventListener('mousedown', closeOnOutside);
    window.addEventListener('keydown', closeOnEscape);
    return () => {
      window.removeEventListener('mousedown', closeOnOutside);
      window.removeEventListener('keydown', closeOnEscape);
    };
  }, [exportMenuOpen]);

  const handleSaved = useCallback((id: number, item: StoredResume) => {
    onSelectedIdChange(id);
    setItems((current) => current.some((resume) => resume.id === item.id)
      ? current.map((resume) => resume.id === item.id ? item : resume)
      : [item, ...current]);
  }, [onSelectedIdChange]);
  const autosave = useResumeAutosave({ document, selectedId, onSaved: handleSaved });
  useImperativeHandle(autosaveHandleRef, () => ({ flush: autosave.flush }), [autosave.flush]);

  const select = (item: StoredResume) => {
    try {
      const next = documentFromStored(item);
      autosave.cancel({ skipNextDocument: true });
      onSelectedIdChange(item.id);
      onSelect(next);
      setError(null);
    } catch { setError('无法读取该简历内容。'); }
  };

  const remove = async (id: number) => {
    if (selectedId === id) autosave.cancel();
    setDeleting(true); setError(null);
    try {
      await deleteResume(id);
      setItems((current) => current.filter((item) => item.id !== id));
      setPendingDeleteId(null);
      if (selectedId === id) onSelectedIdChange(null);
    } catch { setError('删除失败，请稍后重试。'); } finally { setDeleting(false); }
  };

  const exportPng = async () => {
    if (!previewRef?.current) {
      setError('简历预览尚未准备好，请稍后重试。');
      return;
    }
    setExporting(true); setError(null);
    try {
      await exportResumePng(previewRef.current, document.title);
    } catch {
      setError('导出 PNG 失败，请稍后重试。');
    } finally { setExporting(false); }
  };

  const exportMarkdown = () => {
    try {
      exportResumeMarkdown(document);
    } catch {
      setError('导出 Markdown 失败，请稍后重试。');
    }
  };

  return (
    <section className="resume-library" aria-label="简历管理">
      <div className="resume-library__heading">
        <h2>简历管理</h2>
        <div className="resume-library__export" ref={exportAreaRef}>
          <div className="resume-library__export-primary">
            <LibraryAction disabled={exporting} icon="download" onClick={() => window.print()}>导出 PDF</LibraryAction>
            <LibraryAction aria-expanded={exportMenuOpen} aria-haspopup="menu" aria-label="更多导出格式" className="resume-library__export-toggle" disabled={exporting} icon="chevronDown" onClick={() => setExportMenuOpen((open) => !open)} />
          </div>
          {exportMenuOpen && (
            <div className="resume-library__export-menu" role="menu">
              <LibraryAction disabled={exporting} icon="image" onClick={() => { setExportMenuOpen(false); void exportPng(); }} role="menuitem">{exporting ? '正在导出…' : '导出 PNG'}</LibraryAction>
              <LibraryAction disabled={exporting} icon="download" onClick={() => { setExportMenuOpen(false); exportMarkdown(); }} role="menuitem">导出 Markdown</LibraryAction>
            </div>
          )}
        </div>
      </div>
      <label className="editor-field" htmlFor="resume-title"><span>简历名称</span><input id="resume-title" onChange={(event) => onChange({ ...document, title: event.target.value })} value={document.title} /></label>
      <div className="resume-library__actions">
        <LibraryAction disabled={autosave.status === 'saving'} icon="copy" onClick={() => { onSelectedIdChange(null); void autosave.saveNow({ newDraft: true }); }}>新建并保存</LibraryAction>
        <LibraryAction disabled={autosave.status === 'saving'} icon="save" onClick={() => void autosave.saveNow()}>保存更改</LibraryAction>
      </div>
      <div className="resume-library__sample">
        {confirmRestore ? (
          <>
            <span className="resume-library__confirm-text">恢复示例会覆盖当前简历内容，确认继续？</span>
            <div className="resume-library__confirm-actions">
              <LibraryAction disabled={autosave.status === 'saving'} icon="restore" onClick={() => { setConfirmRestore(false); onRestoreSample(); }}>确认恢复</LibraryAction>
              <LibraryAction icon="close" onClick={() => setConfirmRestore(false)}>取消</LibraryAction>
            </div>
          </>
        ) : (
          <>
            <LibraryAction className="resume-library__restore" icon="restore" onClick={() => setConfirmRestore(true)}>恢复示例内容</LibraryAction>
            <p>恢复后的内容会按自动保存规则写入当前草稿。</p>
          </>
        )}
      </div>
      {loading ? <p>正在加载简历…</p> : <ul className="resume-library__list">{items.map((item) => <li key={item.id}>{pendingDeleteId === item.id ? (
        <div className="resume-library__confirm">
          <span className="resume-library__confirm-text">删除「{item.title}」？</span>
          <div className="resume-library__confirm-actions">
            <LibraryAction disabled={deleting} icon="trash" onClick={() => void remove(item.id)}>{deleting ? '删除中…' : '确认删除'}</LibraryAction>
            <LibraryAction disabled={deleting} icon="close" onClick={() => setPendingDeleteId(null)}>取消</LibraryAction>
          </div>
        </div>
      ) : (
        <div className="resume-library__draft">
          <button aria-pressed={item.id === selectedId} onClick={() => select(item)} type="button"><span className="resume-library__draft-title">{item.title}</span><span className="resume-library__draft-meta"><span className="resume-library__draft-template">{templateName(item.templateId)}</span>{formatDateTime(item.updatedAt) && <span className="resume-library__draft-time">{formatDateTime(item.updatedAt)}</span>}</span></button>
          <button aria-label={`删除草稿：${item.title}`} className="resume-library__draft-delete" onClick={() => setPendingDeleteId(item.id)} title={`删除草稿：${item.title}`} type="button"><LibraryIcon name="trash" /></button>
        </div>
      )}</li>)}</ul>}
      {autosave.status === 'dirty' && <p className="resume-library__status" aria-live="polite">待保存</p>}
      {autosave.status === 'saving' && <p className="resume-library__status" aria-live="polite">正在保存…</p>}
      {autosave.status === 'saved' && <p className="resume-library__status" aria-live="polite">已保存</p>}
      {(error || autosave.error) && <p className="resume-library__error" role="alert">{error ?? autosave.error}</p>}
    </section>
  );
}
