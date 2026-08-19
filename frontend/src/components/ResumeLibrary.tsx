import { useCallback, useEffect, useState, type ButtonHTMLAttributes, type ReactNode, type RefObject } from 'react';
import { deleteResume, documentFromStored, listResumes, type StoredResume } from '../api/resumes';
import { useResumeAutosave } from '../hooks/useResumeAutosave';
import { templateRegistry } from '../templates/registry';
import type { ResumeDocument } from '../types/resume';
import { exportResumePng } from '../utils/exportResumePng';

interface ResumeLibraryProps {
  document: ResumeDocument;
  selectedId: number | null;
  onSelectedIdChange: (id: number | null) => void;
  onChange: (document: ResumeDocument) => void;
  onRestoreSample: () => void;
  onSelect: (document: ResumeDocument) => void;
  previewRef?: RefObject<HTMLElement>;
}

type LibraryIconName = 'copy' | 'download' | 'image' | 'restore' | 'save' | 'trash';

function LibraryIcon({ name }: { name: LibraryIconName }) {
  const paths: Record<LibraryIconName, ReactNode> = {
    copy: <><rect height="12" rx="1.5" width="10" x="8" y="8" /><path d="M6 16H5.5A1.5 1.5 0 0 1 4 14.5v-9A1.5 1.5 0 0 1 5.5 4h9A1.5 1.5 0 0 1 16 5.5V6" /></>,
    download: <><path d="M12 3v11M8 10l4 4 4-4M5 19h14" /></>,
    image: <><rect height="15" rx="2" width="17" x="3.5" y="4.5" /><circle cx="8.5" cy="9" r="1.2" /><path d="m5.5 17 4.2-4 2.8 2.5 2.2-2 3.8 3.5" /></>,
    restore: <><path d="M4 8.5V4.5m0 0h4m-4 0 3 3A8 8 0 1 1 5.7 15" /></>,
    save: <><path d="M5 4h12l2 2v14H5zM8 4v6h8V4M8 20v-6h8v6" /></>,
    trash: <><path d="M5 7h14M10 11v5M14 11v5M8 7l1-2h6l1 2M7 7l.8 12h8.4L17 7" /></>,
  };
  return <svg aria-hidden="true" viewBox="0 0 24 24">{paths[name]}</svg>;
}

function LibraryAction({ icon, children, className, ...props }: Omit<ButtonHTMLAttributes<HTMLButtonElement>, 'children'> & { icon: LibraryIconName; children: ReactNode; className?: string }) {
  return <button className={`resume-library__action-button${className ? ` ${className}` : ''}`} type="button" {...props}><LibraryIcon name={icon} /><span>{children}</span></button>;
}

function templateName(templateId: StoredResume['templateId']) {
  return templateRegistry.find((template) => template.id === templateId)?.name ?? '未知模板';
}

export function ResumeLibrary({ document, selectedId, onSelectedIdChange, onChange, onRestoreSample, onSelect, previewRef }: ResumeLibraryProps) {
  const [items, setItems] = useState<StoredResume[]>([]);
  const [loading, setLoading] = useState(true);
  const [deleting, setDeleting] = useState(false);
  const [exporting, setExporting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let active = true;
    listResumes()
      .then((resumes) => active && setItems(resumes))
      .catch(() => active && setError('无法加载已保存的简历。'))
      .finally(() => active && setLoading(false));
    return () => { active = false; };
  }, []);

  const handleSaved = useCallback((id: number, item: StoredResume) => {
    onSelectedIdChange(id);
    setItems((current) => current.some((resume) => resume.id === item.id)
      ? current.map((resume) => resume.id === item.id ? item : resume)
      : [item, ...current]);
  }, [onSelectedIdChange]);
  const autosave = useResumeAutosave({ document, selectedId, onSaved: handleSaved });

  const select = (item: StoredResume) => {
    try {
      const next = documentFromStored(item);
      autosave.cancel({ skipNextDocument: true });
      onSelectedIdChange(item.id);
      onSelect(next);
      setError(null);
    } catch { setError('无法读取该简历内容。'); }
  };

  const remove = async () => {
    if (selectedId === null) return;
    autosave.cancel();
    setDeleting(true); setError(null);
    try {
      await deleteResume(selectedId);
      setItems((current) => current.filter((item) => item.id !== selectedId));
      onSelectedIdChange(null);
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

  return (
    <section className="resume-library" aria-label="简历管理">
      <div className="resume-library__heading"><h2>简历管理</h2><div className="resume-library__export-actions"><LibraryAction disabled={exporting} icon="download" onClick={() => window.print()}>导出 PDF</LibraryAction><LibraryAction disabled={exporting} icon="image" onClick={exportPng}>{exporting ? '正在导出…' : '导出 PNG'}</LibraryAction></div></div>
      <label className="editor-field" htmlFor="resume-title"><span>简历名称</span><input id="resume-title" onChange={(event) => onChange({ ...document, title: event.target.value })} value={document.title} /></label>
      <div className="resume-library__actions">
        <LibraryAction disabled={autosave.status === 'saving' || deleting} icon="copy" onClick={() => { onSelectedIdChange(null); void autosave.saveNow({ newDraft: true }); }}>新建并保存</LibraryAction>
        <LibraryAction disabled={autosave.status === 'saving' || deleting} icon="save" onClick={() => void autosave.saveNow()}>保存更改</LibraryAction>
        <LibraryAction disabled={deleting || selectedId === null} icon="trash" onClick={remove}>删除</LibraryAction>
      </div>
      <div className="resume-library__sample">
        <LibraryAction className="resume-library__restore" icon="restore" onClick={onRestoreSample}>恢复示例内容</LibraryAction>
        <p>恢复后的内容会按自动保存规则写入当前草稿。</p>
      </div>
      {loading ? <p>正在加载简历…</p> : <ul className="resume-library__list">{items.map((item) => <li key={item.id}><button aria-pressed={item.id === selectedId} onClick={() => select(item)} type="button"><span className="resume-library__draft-title">{item.title}</span><span className="resume-library__draft-template">{templateName(item.templateId)}</span></button></li>)}</ul>}
      {autosave.status === 'dirty' && <p className="resume-library__status" aria-live="polite">待保存</p>}
      {autosave.status === 'saving' && <p className="resume-library__status" aria-live="polite">正在保存…</p>}
      {autosave.status === 'saved' && <p className="resume-library__status" aria-live="polite">已保存</p>}
      {(error || autosave.error) && <p className="resume-library__error" role="alert">{error ?? autosave.error}</p>}
    </section>
  );
}
