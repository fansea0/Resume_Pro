import type { ComponentType } from 'react';
import type { ResumeDocument, TemplateId } from '../types/resume';
import { BusinessTemplate } from './business';
import { CreativeTemplate } from './creative';
import { MinimalTemplate } from './minimal';
import { TechTemplate } from './tech';
import { EditorialTemplate } from './editorial';
import { RedlineTemplate } from './redline';
import { VioletTemplate } from './violet';
import { AzureChronicleTemplate } from './azure-chronicle';
import './templates.css';

export interface ResumeTemplate {
  id: TemplateId;
  name: string;
  description: string;
  tags: string[];
  styleDescription: string;
  previewImage: string;
  Renderer: ComponentType<{ document: ResumeDocument }>;
}

export const templateRegistry: ResumeTemplate[] = [
  { id: 'minimal', name: '极简专业', description: '清晰克制，适合技术与产品岗位。', tags: ['产品', '技术', '社招'], styleDescription: '克制留白，信息层级清晰。', previewImage: '/template-previews/minimal.png', Renderer: MinimalTemplate },
  { id: 'business', name: '现代商务', description: '稳重有序，突出职业经历与成果。', tags: ['管理', '咨询', '社招'], styleDescription: '稳重有序，强调经历与成果。', previewImage: '/template-previews/business.png', Renderer: BusinessTemplate },
  { id: 'creative', name: '创意双栏', description: '个性醒目，适合设计与创意岗位。', tags: ['设计', '品牌', '创意'], styleDescription: '鲜明双栏，兼顾个性与秩序。', previewImage: '/template-previews/creative.png', Renderer: CreativeTemplate },
  { id: 'tech', name: '深色科技感', description: '深色信号感，突出技术实力与项目成果。', tags: ['开发', '算法', '技术'], styleDescription: '深色信号感，突出技术成果。', previewImage: '/template-previews/tech.png', Renderer: TechTemplate },
  { id: 'editorial', name: '高级杂志风', description: '留白与大字排版，适合产品、设计与运营岗位。', tags: ['产品', '运营', '设计'], styleDescription: '大字留白，呈现编辑感节奏。', previewImage: '/template-previews/editorial.png', Renderer: EditorialTemplate },
  { id: 'redline', name: '赤线单栏', description: '红色章节线与紧凑单栏，适合中文互联网求职。', tags: ['互联网', '产品', '运营'], styleDescription: '红色章节线，紧凑利落。', previewImage: '/template-previews/redline.png', Renderer: RedlineTemplate },
  { id: 'violet', name: '紫韵单栏', description: '淡紫章节与居中信息区，适合校园与技术求职。', tags: ['校招', '技术', '学生'], styleDescription: '淡紫章节，气质轻盈专业。', previewImage: '/template-previews/violet.png', Renderer: VioletTemplate },
  { id: 'azure-chronicle', name: '澄蓝履历', description: '蓝色图标章节与双端信息排版，清晰呈现中文履历。', tags: ['产品', '项目', '中文'], styleDescription: '澄蓝图标章节，信息阅读流畅。', previewImage: '/template-previews/azure-chronicle.png', Renderer: AzureChronicleTemplate },
];

export function getTemplate(templateId: TemplateId): ResumeTemplate {
  return templateRegistry.find((template) => template.id === templateId) ?? templateRegistry[0];
}
