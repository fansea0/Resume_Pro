import { describe, expect, it } from 'vitest';
import { documentFromStored } from './resumes';
import { createSampleResumeDocument } from '../types/resume';

function stored(content: unknown, title = '数据库标题', templateId = 'business') {
  return {
    id: 9,
    title,
    templateId: templateId as 'business',
    content: JSON.stringify(content),
    createdAt: '2026-08-14T01:00:00Z',
    updatedAt: '2026-08-14T02:00:00Z',
  };
}

describe('documentFromStored', () => {
  it('restores a complete v2 document and gives persisted metadata precedence', () => {
    const document = createSampleResumeDocument();

    expect(documentFromStored(stored(document))).toEqual({
      ...document,
      title: '数据库标题',
      templateId: 'business',
    });
  });

  it('restores every valid v2 section variant', () => {
    const document = createSampleResumeDocument();
    const restored = documentFromStored(stored(document));

    expect(restored.sections.map((section) => section.type)).toEqual([
      'summary', 'work', 'internship', 'project', 'education', 'skills', 'campus', 'honours',
    ]);
  });

  it('migrates a valid legacy document in the established section order', () => {
    const legacy = {
      title: '旧格式简历',
      templateId: 'redline',
      profile: {
        fullName: '达钦海', headline: 'Go 后端开发', email: 'a@example.com', phone: '13800000000',
        location: '武汉', website: 'https://example.com', summary: '擅长 **Go** 开发。',
      },
      experiences: [{
        id: 'work-1', company: '微派', role: '后端开发', startDate: '2025.08', endDate: '至今',
        highlights: ['交付后台需求', '保障稳定性'],
      }],
      projects: [{
        id: 'project-1', name: 'AI 客服', role: '后端开发', link: 'https://example.com/project',
        startDate: '2025.08', endDate: '至今', description: '客服系统', highlights: ['支持意图识别'],
      }],
      education: [{
        id: 'education-1', school: '湖南科技大学', degree: '本科', field: '机器人工程',
        startDate: '2021.09', endDate: '2025.06',
      }],
      skills: [{ id: 'skill-1', name: 'Go', level: '熟悉' }],
    };

    const restored = documentFromStored(stored(legacy));

    expect(restored).toMatchObject({
      schemaVersion: 2,
      title: '数据库标题',
      templateId: 'business',
      profile: {
        fullName: '达钦海', headline: 'Go 后端开发', email: 'a@example.com', phone: '13800000000',
        location: '武汉', website: 'https://example.com', avatarUrl: '', avatarStorageKey: '',
      },
    });
    expect(restored.sections.map((section) => section.type)).toEqual([
      'summary', 'work', 'project', 'education', 'skills',
    ]);
    expect(restored.sections[0]).toMatchObject({ content: { markdown: '擅长 **Go** 开发。' } });
    expect(restored.sections[1]).toMatchObject({
      content: { items: [{ location: '', showLocation: false, achievements: '', highlightsMarkdown: '- 交付后台需求\n- 保障稳定性' }] },
    });
    expect(restored.sections[2]).toMatchObject({
      content: { items: [{ introduction: '客服系统', achievements: '', highlightsMarkdown: '- 支持意图识别' }] },
    });
  });

  it('rejects malformed v2 variants and invalid legacy content', () => {
    const document = createSampleResumeDocument();
    const work = document.sections.find((section) => section.type === 'work');
    if (!work) throw new Error('sample document must include work');
    const malformedV2 = {
      ...document,
      sections: [{ ...work, content: { items: [{ ...work.content.items[0], showLocation: 'yes' }] } }],
    };
    const invalidLegacy = {
      ...document,
      schemaVersion: undefined,
      sections: undefined,
      profile: { ...document.profile, summary: '旧总结' },
      experiences: [{ id: 'bad', company: '坏数据' }],
      projects: [], education: [], skills: [],
    };

    expect(() => documentFromStored(stored(malformedV2))).toThrow('invalid stored resume content');
    expect(() => documentFromStored(stored(invalidLegacy))).toThrow('invalid stored resume content');
    expect(() => documentFromStored({ ...stored({}), content: '{' })).toThrow('invalid stored resume content');
  });

  it('upgrades earlier v2 content without avatar fields while legacy documents migrate them as empty', () => {
    const document = createSampleResumeDocument();
    const withoutAvatarFields = { ...document, profile: { ...document.profile } };
    delete (withoutAvatarFields.profile as Partial<typeof document.profile>).avatarUrl;
    delete (withoutAvatarFields.profile as Partial<typeof document.profile>).avatarStorageKey;

    const restored = documentFromStored(stored(withoutAvatarFields));
    expect(restored.profile.avatarUrl).toBe('');
    expect(restored.profile.avatarStorageKey).toBe('');
  });

  it('rejects malformed avatar field types in v2 content', () => {
    const document = createSampleResumeDocument();
    const corruptAvatarUrl = {
      ...document,
      profile: { ...document.profile, avatarUrl: 42 },
    };
    const corruptAvatarStorageKey = {
      ...document,
      profile: { ...document.profile, avatarStorageKey: 42 },
    };

    expect(() => documentFromStored(stored(corruptAvatarUrl))).toThrow('invalid stored resume content');
    expect(() => documentFromStored(stored(corruptAvatarStorageKey))).toThrow('invalid stored resume content');
  });

  it('accepts canonical avatar storage keys and rejects invalid nonempty values', () => {
    const document = createSampleResumeDocument();
    const canonicalStorageKey = '550e8400-e29b-41d4-a716-446655440000';
    const withCanonicalStorageKey = {
      ...document,
      profile: { ...document.profile, avatarStorageKey: canonicalStorageKey },
    };
    const withInvalidStorageKey = {
      ...document,
      profile: { ...document.profile, avatarStorageKey: 'avatar-key' },
    };

    expect(documentFromStored(stored(withCanonicalStorageKey)).profile.avatarStorageKey).toBe(canonicalStorageKey);
    expect(() => documentFromStored(stored(withInvalidStorageKey))).toThrow('invalid stored resume content');
  });

  it('rejects unknown outer template identifiers', () => {
    const document = createSampleResumeDocument();
    expect(() => documentFromStored({ ...stored(document), templateId: 'unknown' as 'business' })).toThrow();
  });

  it.each(['mint-canvas', 'scholarly-serif', 'blue-sidebar'] as const)(
    'accepts the selectable %s template when validating generated content',
    (templateId) => {
      const document = { ...createSampleResumeDocument(), templateId };
      expect(documentFromStored(stored(document, document.title, templateId))).toMatchObject({ templateId });
    },
  );
});
