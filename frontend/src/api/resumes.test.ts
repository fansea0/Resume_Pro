import { describe, expect, it } from 'vitest';
import { documentFromStored } from './resumes';

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
});
