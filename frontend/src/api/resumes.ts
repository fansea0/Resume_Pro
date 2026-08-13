import { apiFetch } from './client';
import type { ResumeDocument } from '../types/resume';

export interface StoredResume {
  id: number;
  title: string;
  templateId: ResumeDocument['templateId'];
  content: string;
  createdAt: string;
  updatedAt: string;
}

type ResumePayload = Pick<StoredResume, 'title' | 'templateId'> & { content: ResumeDocument };

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
  return { ...content, title: resume.title, templateId: resume.templateId } as ResumeDocument;
}
