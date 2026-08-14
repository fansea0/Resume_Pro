import { apiFetch } from './client';
import type { ResumeDocument, TemplateId } from '../types/resume';

export interface StoredResume {
  id: number;
  title: string;
  templateId: ResumeDocument['templateId'];
  content: string;
  createdAt: string;
  updatedAt: string;
}

type ResumePayload = Pick<StoredResume, 'title' | 'templateId'> & { content: ResumeDocument };

const profileFields = ['fullName', 'headline', 'email', 'phone', 'location', 'website', 'summary'] as const;
const templateIds = new Set<TemplateId>(['minimal', 'business', 'creative', 'tech', 'editorial', 'redline', 'violet', 'azure-chronicle']);

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null;
}

function hasStringFields(value: Record<string, unknown>, fields: readonly string[]) {
  return fields.every((field) => typeof value[field] === 'string');
}

function isStringArray(value: unknown): value is string[] {
  return Array.isArray(value) && value.every((item) => typeof item === 'string');
}

function isTemplateId(value: unknown): value is TemplateId {
  return typeof value === 'string' && templateIds.has(value as TemplateId);
}

function isExperience(value: unknown) {
  return isRecord(value)
    && hasStringFields(value, ['id', 'company', 'role', 'startDate', 'endDate'])
    && isStringArray(value.highlights);
}

function isProject(value: unknown) {
  return isRecord(value)
    && hasStringFields(value, ['id', 'name', 'role', 'startDate', 'endDate'])
    && hasStringFields(value, ['link', 'description'])
    && isStringArray(value.highlights);
}

function isEducation(value: unknown) {
  return isRecord(value) && hasStringFields(value, ['id', 'school', 'degree', 'field', 'startDate', 'endDate']);
}

function isSkill(value: unknown) {
  return isRecord(value) && hasStringFields(value, ['id', 'name', 'level']);
}

function isResumeContent(value: unknown): value is Record<string, unknown> {
  if (!isRecord(value)) return false;
  const profile = value.profile;
  if (!isRecord(profile)) return false;
  return isTemplateId(value.templateId)
    && profileFields.every((field) => typeof profile[field] === 'string')
    && Array.isArray(value.experiences) && value.experiences.every(isExperience)
    && Array.isArray(value.projects) && value.projects.every(isProject)
    && Array.isArray(value.education) && value.education.every(isEducation)
    && Array.isArray(value.skills) && value.skills.every(isSkill);
}

function payload(document: ResumeDocument): ResumePayload {
  return { title: document.title, templateId: document.templateId, content: document };
}

export function listResumes() {
  return apiFetch<StoredResume[]>('/api/resumes');
}

export function createResume(document: ResumeDocument) {
  return apiFetch<StoredResume>('/api/resumes', { method: 'POST', body: JSON.stringify(payload(document)) });
}

export function updateResume(id: number, document: ResumeDocument) {
  return apiFetch<StoredResume>(`/api/resumes/${id}`, { method: 'PUT', body: JSON.stringify(payload(document)) });
}

export function deleteResume(id: number) {
  return apiFetch<void>(`/api/resumes/${id}`, { method: 'DELETE' });
}

export function documentFromStored(resume: StoredResume): ResumeDocument {
  const content = typeof resume.content === 'string' ? JSON.parse(resume.content) : resume.content;
  if (!isResumeContent(content) || !isTemplateId(resume.templateId)) {
    throw new Error('invalid stored resume content');
  }
  return { ...content, title: resume.title, templateId: resume.templateId } as ResumeDocument;
}
