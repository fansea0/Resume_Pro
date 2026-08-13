import type { ComponentType } from 'react';
import type { ResumeDocument, TemplateId } from '../types/resume';
import { BusinessTemplate } from './business';
import { CreativeTemplate } from './creative';
import { MinimalTemplate } from './minimal';
import { TechTemplate } from './tech';
import { EditorialTemplate } from './editorial';
import { RedlineTemplate } from './redline';
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
  { id: 'tech', name: '深色科技感', description: '深色信号感，突出技术实力与项目成果。', Renderer: TechTemplate },
  { id: 'editorial', name: '高级杂志风', description: '留白与大字排版，适合产品、设计与运营岗位。', Renderer: EditorialTemplate },
  { id: 'redline', name: '赤线单栏', description: '红色章节线与紧凑单栏，适合中文互联网求职。', Renderer: RedlineTemplate },
];

export function getTemplate(templateId: TemplateId): ResumeTemplate {
  return templateRegistry.find((template) => template.id === templateId) ?? templateRegistry[0];
}
