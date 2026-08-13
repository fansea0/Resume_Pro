import type { ComponentType } from 'react';
import type { ResumeDocument, TemplateId } from '../types/resume';
import { BusinessTemplate } from './business';
import { CreativeTemplate } from './creative';
import { MinimalTemplate } from './minimal';
import './templates.css';

export interface ResumeTemplate {
  id: TemplateId;
  name: string;
  description: string;
  Renderer: ComponentType<{ document: ResumeDocument }>;
}

export const templateRegistry: ResumeTemplate[] = [
  { id: 'minimal', name: '极简专业', description: '清晰克制，适合技术与产品岗位。', Renderer: MinimalTemplate },
  { id: 'business', name: '现代商务', description: '稳重有序，突出职业经历与成果。', Renderer: BusinessTemplate },
  { id: 'creative', name: '创意双栏', description: '个性醒目，适合设计与创意岗位。', Renderer: CreativeTemplate },
];

export function getTemplate(templateId: TemplateId): ResumeTemplate {
  return templateRegistry.find((template) => template.id === templateId) ?? templateRegistry[0];
}
