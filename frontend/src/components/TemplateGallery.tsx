import { templateRegistry } from '../templates/registry';
import type { TemplateId } from '../types/resume';

interface TemplateGalleryProps {
  onSelect: (templateId: TemplateId) => void;
  disabledTemplateIds?: TemplateId[];
}

export function TemplateGallery({ disabledTemplateIds = [], onSelect }: TemplateGalleryProps) {
  const disabled = new Set(disabledTemplateIds);
  return (
    <section className="template-gallery" aria-labelledby="template-gallery-title">
      <div className="template-gallery__heading">
        <p className="template-gallery__eyebrow">RESUME TEMPLATES</p>
        <h2 id="template-gallery-title">从一个好模板开始</h2>
        <p>选择适合你的版式，下一步就能专注呈现你的经历。</p>
      </div>
      <div className="template-gallery__grid">
        {templateRegistry.filter((template) => !disabled.has(template.id)).map((template) => (
          <article
            aria-label={`选择模板：${template.name}`}
            className="template-card"
            key={template.id}
            onClick={() => onSelect(template.id)}
            onKeyDown={(event) => {
              if (event.key === 'Enter' || event.key === ' ') {
                event.preventDefault();
                onSelect(template.id);
              }
            }}
            role="button"
            tabIndex={0}
          >
            <img alt={`${template.name} 模板预览`} className="template-card__image" src={template.previewImage} />
            <div className="template-card__body">
              <h3>{template.name}</h3>
              <ul aria-label={`${template.name} 适用岗位`} className="template-card__tags">
                {template.tags.map((tag) => <li key={tag}>{tag}</li>)}
              </ul>
              <p>{template.styleDescription}</p>
              <button aria-label={`使用此模板：${template.name}`} onClick={(event) => { event.stopPropagation(); onSelect(template.id); }} type="button">
                使用此模板
              </button>
            </div>
          </article>
        ))}
      </div>
    </section>
  );
}
