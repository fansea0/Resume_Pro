import { templateRegistry } from '../templates/registry';
import type { TemplateId } from '../types/resume';

interface TemplateGalleryProps {
  onSelect: (templateId: TemplateId) => void;
}

export function TemplateGallery({ onSelect }: TemplateGalleryProps) {
  return (
    <section className="template-gallery" aria-labelledby="template-gallery-title">
      <div className="template-gallery__heading">
        <p className="template-gallery__eyebrow">TEMPLATE LIBRARY</p>
        <h2 id="template-gallery-title">选择简历模板</h2>
        <p>先选一个版式，再使用你的内容完成简历。</p>
      </div>
      <div className="template-gallery__grid">
        {templateRegistry.map((template) => (
          <article className="template-card" key={template.id}>
            <img alt={`${template.name} 模板预览`} className="template-card__image" src={template.previewImage} />
            <div className="template-card__body">
              <h3>{template.name}</h3>
              <p>{template.description}</p>
              <button aria-label={`使用此模板：${template.name}`} onClick={() => onSelect(template.id)} type="button">
                使用此模板
              </button>
            </div>
          </article>
        ))}
      </div>
    </section>
  );
}
