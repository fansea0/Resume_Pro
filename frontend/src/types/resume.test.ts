import { describe, expect, it } from 'vitest';

import { createEmptyResumeDocument, createSampleResumeDocument } from './resume';

describe('createEmptyResumeDocument', () => {
  it('creates an internet-job-ready document with meaningful defaults', () => {
    const document = createEmptyResumeDocument();

    expect(document).toMatchObject({
      title: '我的简历',
      templateId: 'minimal',
      profile: {
        fullName: '姓名',
        headline: '互联网从业者',
      },
    });
  });

  it('initializes every repeatable section as an empty array', () => {
    const document = createEmptyResumeDocument();

    expect(document.experiences).toEqual([]);
    expect(document.projects).toEqual([]);
    expect(document.education).toEqual([]);
    expect(document.skills).toEqual([]);
  });
});

describe('createSampleResumeDocument', () => {
  it('creates a complete, realistic redline resume with populated unique IDs', () => {
    const document = createSampleResumeDocument();
    const identifiers = [
      ...document.experiences.map((experience) => experience.id),
      ...document.projects.map((project) => project.id),
      ...document.education.map((education) => education.id),
      ...document.skills.map((skill) => skill.id),
    ];

    expect(document.templateId).toBe('redline');
    expect(document.profile.fullName).not.toBe('姓名');
    expect(document.profile.summary).not.toBe('');
    expect(document.profile.email).not.toBe('');
    expect(document.profile.phone).not.toBe('');
    expect(document.profile.location).not.toBe('');
    expect(document.experiences.length).toBeGreaterThan(0);
    expect(document.projects.length).toBeGreaterThan(0);
    expect(document.education.length).toBeGreaterThan(0);
    expect(document.skills.length).toBeGreaterThan(0);
    expect(identifiers.every(Boolean)).toBe(true);
    expect(new Set(identifiers).size).toBe(identifiers.length);
  });
});
