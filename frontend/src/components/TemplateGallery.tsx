import { useState } from 'react';
import { templateRegistry } from '../templates/registry';
import type { TemplateId } from '../types/resume';
import { AiGenerateModal } from './AiGenerateModal';

interface TemplateGalleryProps {
  onSelect: (templateId: TemplateId) => void;
  disabledTemplateIds?: TemplateId[];
}

export function TemplateGallery({ disabledTemplateIds = [], onSelect }: TemplateGalleryProps) {
  const [aiGenerateOpen, setAiGenerateOpen] = useState(false);
  const disabled = new Set(disabledTemplateIds);

  const manageTargetJobs = () => {
    const state = { appView: 'targetJobs' };
    window.history.pushState(state, '', '/target-jobs');
    window.dispatchEvent(new PopStateEvent('popstate', { state }));
  };

  return (
    <>
      <section className="template-gallery" aria-labelledby="template-gallery-title">
        <div className="template-gallery__heading">
          <div>
            <p className="template-gallery__eyebrow">RESUME TEMPLATES</p>
            <h2 id="template-gallery-title">选择简历模板</h2>
          </div>
          <div className="template-gallery__actions">
            <button onClick={manageTargetJobs} type="button">管理目标岗位</button>
            <button onClick={() => setAiGenerateOpen(true)} type="button">AI 快速生成</button>
          </div>
        </div>
        <div className="template-gallery__grid">
          {templateRegistry.filter((template) => !disabled.has(template.id)).map((template) => (
            <button
              aria-label={`选择模板：${template.name}`}
              className="template-card"
              key={template.id}
              onClick={() => onSelect(template.id)}
              type="button"
            >
              <img alt={`${template.name} 模板预览`} className="template-card__image" src={template.previewImage} />
              <span className="template-card__body">
                <strong className="template-card__title">{template.name}</strong>
                <span aria-label={`${template.name} 适用岗位`} className="template-card__tags" role="list">
                  {template.tags.map((tag) => <span key={tag} role="listitem">{tag}</span>)}
                </span>
                <span className="template-card__description">{template.styleDescription}</span>
              </span>
            </button>
          ))}
        </div>
      </section>
      <AiGenerateModal
        onClose={() => setAiGenerateOpen(false)}
        onCreated={() => setAiGenerateOpen(false)}
        open={aiGenerateOpen}
      />
    </>
  );
}
