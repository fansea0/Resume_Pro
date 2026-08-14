import { useCallback, useEffect, useRef, useState } from 'react';
import { disableTemplate, listAdminTemplates } from '../api/templates';
import { templateRegistry } from '../templates/registry';
import type { TemplateId } from '../types/resume';

interface TemplateAdminProps {
  onDisabledTemplateIdsChange?: (templateIds: TemplateId[]) => void;
}

export function TemplateAdmin({ onDisabledTemplateIdsChange }: TemplateAdminProps) {
  const [disabledTemplateIds, setDisabledTemplateIds] = useState<TemplateId[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const mounted = useRef(false);

  const load = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const status = await listAdminTemplates();
      if (!mounted.current) return;
      setDisabledTemplateIds(status.disabledTemplateIds);
      onDisabledTemplateIdsChange?.(status.disabledTemplateIds);
    } catch {
      if (mounted.current) setError('无法加载模板状态，请稍后重试。');
    } finally {
      if (mounted.current) setLoading(false);
    }
  }, [onDisabledTemplateIdsChange]);

  useEffect(() => {
    mounted.current = true;
    void load();
    return () => { mounted.current = false; };
  }, [load]);

  const handleDisable = async (templateId: TemplateId, templateName: string) => {
    if (!window.confirm(`确定要禁用“${templateName}”吗？已保存的简历不会被删除。`)) return;
    setError('');
    try {
      await disableTemplate(templateId);
      const next = [...new Set([...disabledTemplateIds, templateId])];
      setDisabledTemplateIds(next);
      onDisabledTemplateIdsChange?.(next);
    } catch {
      setError('禁用模板失败，请稍后重试。');
    }
  };

  return (
    <section aria-labelledby="template-admin-title" className="template-admin">
      <div className="template-admin__heading">
        <p className="template-gallery__eyebrow">ADMINISTRATION</p>
        <h2 id="template-admin-title">模板管理</h2>
        <p>禁用后模板不会再出现在用户的模板主页；已保存的简历不受影响。</p>
      </div>
      {error && <p role="alert">{error}</p>}
      {loading ? <p>正在加载模板状态…</p> : (
        <div className="template-admin__grid">
          {templateRegistry.map((template) => {
            const isDisabled = disabledTemplateIds.includes(template.id);
            return (
              <article className="template-admin__card" key={template.id}>
                <img alt={`${template.name} 模板预览`} src={template.previewImage} />
                <div>
                  <h3>{template.name}</h3>
                  <ul aria-label={`${template.name} 适用岗位`} className="template-admin__tags">
                    {template.tags.map((tag) => <li key={tag}>{tag}</li>)}
                  </ul>
                  <p>{template.styleDescription}</p>
                  {isDisabled ? <span className="template-admin__status">已禁用</span> : (
                    <button aria-label={`删除模板：${template.name}`} onClick={() => void handleDisable(template.id, template.name)} type="button">删除模板</button>
                  )}
                </div>
              </article>
            );
          })}
        </div>
      )}
    </section>
  );
}
