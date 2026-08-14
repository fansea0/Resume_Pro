import { apiFetch } from './client';
import type { TemplateId } from '../types/resume';

export interface TemplateStatus {
  disabledTemplateIds: TemplateId[];
}

export function getTemplateStatus() {
  return apiFetch<TemplateStatus>('/api/templates');
}

export function listAdminTemplates() {
  return apiFetch<TemplateStatus>('/api/admin/templates');
}

export function disableTemplate(templateId: TemplateId) {
  return apiFetch<void>(`/api/admin/templates/${templateId}`, { method: 'DELETE' });
}
