import { useEffect, useState } from 'react';
import { createResume, deleteResume, documentFromStored, listResumes, type StoredResume, updateResume } from '../api/resumes';
import type { ResumeDocument } from '../types/resume';

interface ResumeLibraryProps {
  document: ResumeDocument;
  onChange: (document: ResumeDocument) => void;
  onSelect: (document: ResumeDocument) => void;
}

export function ResumeLibrary({ document, onChange, onSelect }: ResumeLibraryProps) {
  const [items, setItems] = useState<StoredResume[]>([]);
  const [selectedId, setSelectedId] = useState<number | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let active = true;
    listResumes()
      .then((resumes) => active && setItems(resumes))
      .catch(() => active && setError('无法加载已保存的简历。'))
      .finally(() => active && setLoading(false));
    return () => { active = false; };
  }, []);

  const saveNew = async () => {
    setSaving(true); setError(null);
    try {
      const item = await createResume(document);
      setItems((current) => [item, ...current]);
      setSelectedId(item.id);
      onChange(documentFromStored(item));
    } catch {
      setError('保存失败，请稍后重试。');
    } finally { setSaving(false); }
  };

  const saveCurrent = async () => {
    if (selectedId === null) return saveNew();
    setSaving(true); setError(null);
    try {
      const item = await updateResume(selectedId, document);
      setItems((current) => current.map((resume) => resume.id === item.id ? item : resume));
      onChange(documentFromStored(item));
    } catch { setError('保存失败，请稍后重试。'); } finally { setSaving(false); }
  };

  const select = (item: StoredResume) => {
    try {
      const next = documentFromStored(item);
      setSelectedId(item.id);
      onSelect(next);
      setError(null);
    } catch { setError('无法读取该简历内容。'); }
  };

  const remove = async () => {
    if (selectedId === null) return;
    setSaving(true); setError(null);
    try {
      await deleteResume(selectedId);
      setItems((current) => current.filter((item) => item.id !== selectedId));
      setSelectedId(null);
    } catch { setError('删除失败，请稍后重试。'); } finally { setSaving(false); }
  };

  return (
    <section className="resume-library" aria-label="简历管理">
      <div className="resume-library__heading"><h2>简历管理</h2><button onClick={() => window.print()} type="button">导出 PDF</button></div>
      <label className="editor-field" htmlFor="resume-title"><span>简历名称</span><input id="resume-title" onChange={(event) => onChange({ ...document, title: event.target.value })} value={document.title} /></label>
      <div className="resume-library__actions">
        <button disabled={saving} onClick={saveNew} type="button">新建并保存</button>
        <button disabled={saving} onClick={saveCurrent} type="button">保存更改</button>
        <button disabled={saving || selectedId === null} onClick={remove} type="button">删除</button>
      </div>
      {loading ? <p>正在加载简历…</p> : <ul className="resume-library__list">{items.map((item) => <li key={item.id}><button aria-pressed={item.id === selectedId} onClick={() => select(item)} type="button">{item.title}</button></li>)}</ul>}
      {error && <p className="resume-library__error" role="alert">{error}</p>}
    </section>
  );
}
