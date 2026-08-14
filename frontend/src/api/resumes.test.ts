import { describe, expect, it } from 'vitest';
import { documentFromStored } from './resumes';
import { createSampleResumeDocument } from '../types/resume';

describe('documentFromStored', () => {
  it('rejects parseable persisted content that is not a resume document', () => {
    expect(() => documentFromStored({
      id: 9,
      title: '损坏的简历',
      templateId: 'business',
      content: JSON.stringify({ profile: {} }),
      createdAt: '2026-08-14T01:00:00Z',
      updatedAt: '2026-08-14T02:00:00Z',
    })).toThrow();
  });

  it('rejects nested entries that would crash template rendering', () => {
    const document = createSampleResumeDocument();
    expect(() => documentFromStored({
      id: 10,
      title: document.title,
      templateId: document.templateId,
      content: JSON.stringify({ ...document, experiences: [{}] }),
      createdAt: '2026-08-14T01:00:00Z',
      updatedAt: '2026-08-14T02:00:00Z',
    })).toThrow();
  });

  it('rejects unknown template identifiers from persisted data', () => {
    const document = createSampleResumeDocument();
    expect(() => documentFromStored({
      id: 11,
      title: document.title,
      templateId: 'unknown' as typeof document.templateId,
      content: JSON.stringify({ ...document, templateId: 'unknown' }),
      createdAt: '2026-08-14T01:00:00Z',
      updatedAt: '2026-08-14T02:00:00Z',
    })).toThrow();
  });
});
