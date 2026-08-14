import { useCallback, useEffect, useRef, useState } from 'react';
import { listResumes, type StoredResume } from '../api/resumes';
import { templateRegistry } from '../templates/registry';

interface ResumeDraftsProps {
  onCreate: () => void;
  onSelect: (resume: StoredResume) => void;
}

const dateFormatter = new Intl.DateTimeFormat('zh-CN', {
  dateStyle: 'medium',
  timeStyle: 'short',
});

function templateName(templateId: StoredResume['templateId']) {
  return templateRegistry.find((template) => template.id === templateId)?.name ?? '未知模板';
}

export function ResumeDrafts({ onCreate, onSelect }: ResumeDraftsProps) {
  const [drafts, setDrafts] = useState<StoredResume[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);
  const isMounted = useRef(false);
  const isLoading = useRef(false);
  const requestVersion = useRef(0);

  const load = useCallback(async () => {
    if (isLoading.current) return;
    const version = ++requestVersion.current;
    isLoading.current = true;
    setLoading(true);
    try {
      const items = await listResumes();
      if (isMounted.current && version === requestVersion.current) {
        setDrafts(items);
        setError(false);
      }
    } catch {
      if (isMounted.current && version === requestVersion.current) setError(true);
    } finally {
      if (isMounted.current && version === requestVersion.current) {
        isLoading.current = false;
        setLoading(false);
      }
    }
  }, []);

  useEffect(() => {
    isMounted.current = true;
    void load();
    return () => {
      isMounted.current = false;
      isLoading.current = false;
      requestVersion.current += 1;
    };
  }, [load]);

  if (loading && !error) return <p>正在加载草稿…</p>;
  if (error) return <div><p role="alert">无法加载草稿，请稍后重试。</p>{loading && <p>正在重试…</p>}<button disabled={loading} onClick={load} type="button">重试</button></div>;

  return (
    <section aria-labelledby="resume-drafts-title" className="resume-drafts">
      <div className="resume-drafts__header">
        <p className="resume-drafts__eyebrow">简历工作台</p>
        <h2 id="resume-drafts-title">我的草稿</h2>
        <p>查看并继续编辑你已保存的简历。</p>
      </div>
      {drafts.length === 0 ? (
        <aside aria-labelledby="resume-drafts-empty-title" className="resume-drafts__empty">
          <div className="resume-drafts__empty-copy">
            <h3 id="resume-drafts-empty-title">开始你的第一份简历</h3>
            <p>还没有保存的简历草稿。</p>
          </div>
          <ol aria-label="创建简历的步骤" className="resume-drafts__steps">
            <li>选择适合你的简历模板</li>
            <li>填写并完善你的经历</li>
            <li>随时返回继续编辑</li>
          </ol>
          <button onClick={onCreate} type="button">去选择模板</button>
        </aside>
      ) : (
        <ul aria-label="已保存的简历草稿" className="resume-drafts__list">
          {drafts.map((draft, index) => (
            <li className="resume-drafts__item" key={draft.id}>
              <span aria-hidden="true" className="resume-drafts__index">{String(index + 1).padStart(2, '0')}</span>
              <article className="resume-drafts__item-content">
                <h3>{draft.title}</h3>
                <dl className="resume-drafts__metadata">
                  <div>
                    <dt>模板</dt>
                    <dd className="resume-drafts__template-name">{templateName(draft.templateId)}</dd>
                  </div>
                  <div>
                    <dt>最后更新</dt>
                    <dd><time className="resume-drafts__updated-at" dateTime={draft.updatedAt}>{dateFormatter.format(new Date(draft.updatedAt))}</time></dd>
                  </div>
                </dl>
              </article>
              <button aria-label={`继续编辑：${draft.title}`} onClick={() => onSelect(draft)} type="button">继续编辑</button>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
