import { describe, expect, it } from 'vitest';

import { createSampleResumeDocument } from '../types/resume';
import { sanitizeGeneratedResumeDocument } from './sanitizeGeneratedResume';

describe('sanitizeGeneratedResumeDocument', () => {
  it('rebuilds a generated document from allowlisted fields and trusted selections', () => {
    const sample = createSampleResumeDocument();
    const generated = {
      ...sample,
      templateId: 'unknown-template',
      targetJobId: 999,
      injectedRoot: 'drop me',
      profile: {
        ...sample.profile,
        showAvatar: undefined,
        avatarUrl: 'https://attacker.example/avatar.png',
        avatarStorageKey: '550e8400-e29b-41d4-a716-446655440000',
        injectedProfile: 'drop me',
      },
      sections: sample.sections.map((section, index) => index === 0
        ? {
            ...section,
            injectedSection: 'drop me',
            content: { ...section.content, injectedContent: 'drop me' },
          }
        : section.type === 'work'
          ? {
              ...section,
              content: {
                items: section.content.items.map((item) => ({ ...item, injectedItem: 'drop me' })),
              },
            }
        : section),
    };

    const sanitized = sanitizeGeneratedResumeDocument(generated, 'business', 7);

    expect(sanitized).toBeDefined();
    expect(sanitized).toMatchObject({
      schemaVersion: 2,
      title: sample.title,
      templateId: 'business',
      targetJobId: 7,
      profile: { ...sample.profile, avatarUrl: '', avatarStorageKey: '', showAvatar: true },
    });
    expect(Object.keys(sanitized ?? {})).toEqual([
      'schemaVersion', 'title', 'templateId', 'targetJobId', 'profile', 'sections',
    ]);
    expect(Object.keys(sanitized?.profile ?? {})).toEqual([
      'fullName', 'headline', 'email', 'phone', 'location', 'website',
      'avatarUrl', 'avatarStorageKey', 'showAvatar',
    ]);
    expect(sanitized?.sections[0]).toEqual(sample.sections[0]);
    expect(sanitized?.sections[1]).toEqual(sample.sections[1]);
    expect(sanitizeGeneratedResumeDocument(generated, 'business')).not.toHaveProperty('targetJobId');
  });

  it('preserves an explicit boolean showAvatar and rejects a wrong type', () => {
    const sample = createSampleResumeDocument();
    expect(sanitizeGeneratedResumeDocument({
      ...sample,
      profile: { ...sample.profile, showAvatar: false },
    }, 'minimal')?.profile.showAvatar).toBe(false);

    expect(sanitizeGeneratedResumeDocument({
      ...sample,
      profile: { ...sample.profile, showAvatar: 'yes' },
    }, 'minimal')).toBeUndefined();
  });

  it('rejects strings and collections outside the backend persistence limits', () => {
    const sample = createSampleResumeDocument();
    const summary = sample.sections.find((section) => section.type === 'summary');
    const skills = sample.sections.find((section) => section.type === 'skills');
    if (!summary || !skills) throw new Error('sample sections are required');

    expect(sanitizeGeneratedResumeDocument({
      ...sample,
      title: 'x'.repeat(20_001),
    }, 'minimal')).toBeUndefined();
    expect(sanitizeGeneratedResumeDocument({
      ...sample,
      title: '中'.repeat(6_667),
    }, 'minimal')).toBeUndefined();
    expect(sanitizeGeneratedResumeDocument({
      ...sample,
      sections: Array.from({ length: 51 }, (_, index) => ({ ...summary, id: `summary-${index}` })),
    }, 'minimal')).toBeUndefined();
    expect(sanitizeGeneratedResumeDocument({
      ...sample,
      sections: [{
        ...skills,
        content: {
          items: Array.from({ length: 101 }, (_, index) => ({ id: `skill-${index}`, name: 'Go', note: '' })),
        },
      }],
    }, 'minimal')).toBeUndefined();
  });

  it('rejects unsafe target ids and documents over the backend request limit', () => {
    const sample = createSampleResumeDocument();
    const skills = sample.sections.find((section) => section.type === 'skills');
    if (!skills) throw new Error('sample skills section is required');

    expect(sanitizeGeneratedResumeDocument(sample, 'minimal', 0)).toBeUndefined();
    expect(sanitizeGeneratedResumeDocument({
      ...sample,
      sections: Array.from({ length: 50 }, (_, sectionIndex) => ({
        ...skills,
        id: `skills-${sectionIndex}`,
        content: {
          items: Array.from({ length: 100 }, (_, itemIndex) => ({
            id: `skill-${sectionIndex}-${itemIndex}`,
            name: 'n'.repeat(220),
            note: '熟练',
          })),
        },
      })),
    }, 'minimal')).toBeUndefined();
  });
});
