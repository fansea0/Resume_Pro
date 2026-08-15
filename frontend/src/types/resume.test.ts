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

  it('creates a version 2 document with no content sections', () => {
    const document = createEmptyResumeDocument();

    expect(document.schemaVersion).toBe(2);
    expect(document.sections).toEqual([]);
  });
});

describe('createSampleResumeDocument', () => {
  it('creates a complete v2 document with every visible section type and stable unique IDs', () => {
    const document = createSampleResumeDocument();
    const sectionTypes = document.sections.map((section) => section.type);
    const identifiers = document.sections.flatMap((section) => [
      section.id,
      ...('items' in section.content ? section.content.items.map((item) => item.id) : []),
    ]);

    expect(document.templateId).toBe('redline');
    expect(document.profile.fullName).not.toBe('姓名');
    expect(document.profile.email).not.toBe('');
    expect(document.profile.phone).not.toBe('');
    expect(document.profile.location).not.toBe('');
    expect(document.schemaVersion).toBe(2);
    expect(sectionTypes).toEqual([
      'summary',
      'work',
      'internship',
      'project',
      'education',
      'skills',
      'campus',
      'honours',
    ]);
    expect(document.sections.every((section) => section.visible)).toBe(true);
    expect(identifiers.every(Boolean)).toBe(true);
    expect(new Set(identifiers).size).toBe(identifiers.length);
  });
});
