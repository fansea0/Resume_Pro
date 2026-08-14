import { useCallback, useEffect, useState, type RefObject } from 'react';
import { deleteResume, documentFromStored, listResumes, type StoredResume } from '../api/resumes';
import { useResumeAutosave } from '../hooks/useResumeAutosave';
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
      autosave.cancel();
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
      <div className="resume-library__heading"><h2>简历管理</h2><div><button disabled={exporting} onClick={() => window.print()} type="button">导出 PDF</button><button disabled={exporting} onClick={exportPng} type="button">{exporting ? '正在导出 PNG…' : '导出 PNG'}</button></div></div>
      <label className="editor-field" htmlFor="resume-title"><span>简历名称</span><input id="resume-title" onChange={(event) => onChange({ ...document, title: event.target.value })} value={document.title} /></label>
      <div className="resume-library__actions">
        <button disabled={autosave.status === 'saving' || deleting} onClick={() => { onSelectedIdChange(null); void autosave.saveNow({ newDraft: true }); }} type="button">新建并保存</button>
        <button disabled={autosave.status === 'saving' || deleting} onClick={() => void autosave.saveNow()} type="button">保存更改</button>
        <button disabled={autosave.status === 'saving' || deleting} onClick={() => void autosave.saveNow()} type="button">立即保存</button>
        <button disabled={deleting || selectedId === null} onClick={remove} type="button">删除</button>
      </div>
      <div className="resume-library__sample">
        <button onClick={onRestoreSample} type="button">恢复示例内容</button>
        <p>恢复后的内容会按自动保存规则写入当前草稿。</p>
      </div>
      {loading ? <p>正在加载简历…</p> : <ul className="resume-library__list">{items.map((item) => <li key={item.id}><button aria-pressed={item.id === selectedId} onClick={() => select(item)} type="button">{item.title}</button></li>)}</ul>}
      {autosave.status === 'dirty' && <p className="resume-library__status" aria-live="polite">待保存</p>}
      {autosave.status === 'saving' && <p className="resume-library__status" aria-live="polite">正在保存…</p>}
      {autosave.status === 'saved' && <p className="resume-library__status" aria-live="polite">已保存</p>}
      {(error || autosave.error) && <p className="resume-library__error" role="alert">{error ?? autosave.error}</p>}
    </section>
  );
}
