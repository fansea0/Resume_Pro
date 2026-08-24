import type { TargetJob, TargetJobInput } from '../types/targetJob';
import { apiFetch } from './client';

export function listTargetJobs(): Promise<TargetJob[]> {
  return apiFetch<TargetJob[]>('/api/target-jobs', { method: 'GET' });
}

export function getTargetJob(id: number): Promise<TargetJob> {
  return apiFetch<TargetJob>(`/api/target-jobs/${id}`, { method: 'GET' });
}

export function createTargetJob(input: TargetJobInput): Promise<TargetJob> {
  return apiFetch<TargetJob>('/api/target-jobs', {
    method: 'POST',
    body: JSON.stringify(input),
  });
}

export function updateTargetJob(id: number, input: TargetJobInput): Promise<TargetJob> {
  return apiFetch<TargetJob>(`/api/target-jobs/${id}`, {
    method: 'PUT',
    body: JSON.stringify(input),
  });
}

export function deleteTargetJob(id: number): Promise<{ deletedReferences: number }> {
  return apiFetch<{ deletedReferences: number }>(`/api/target-jobs/${id}`, { method: 'DELETE' });
}
