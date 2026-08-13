import { describe, expect, it } from 'vitest';

import { createEmptyResumeDocument } from './resume';

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
