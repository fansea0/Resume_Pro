import { templateRegistry } from '../templates/registry';
import type { TemplateId } from '../types/resume';

interface TemplatePickerProps {
  selectedTemplateId: TemplateId;
  onSelect: (templateId: TemplateId) => void;
}

export function TemplatePicker({ selectedTemplateId, onSelect }: TemplatePickerProps) {
  return (
    <section className="template-picker" aria-labelledby="template-picker-title">
      <h2 id="template-picker-title">选择模板</h2>
      <div className="template-picker__options">
        {templateRegistry.map((template) => (
          <button
            aria-pressed={selectedTemplateId === template.id}
            className="template-picker__option"
            key={template.id}
            onClick={() => onSelect(template.id)}
            type="button"
          >
            <strong>{template.name}</strong>
            <span>{template.description}</span>
          </button>
        ))}
      </div>
    </section>
  );
}
