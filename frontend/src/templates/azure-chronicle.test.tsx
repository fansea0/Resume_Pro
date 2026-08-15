import { render } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { AzureChronicleTemplate } from './azure-chronicle';
import type { ResumeDocument } from '../types/resume';

const document: ResumeDocument = {
  schemaVersion: 2,
  title: '达钦海｜Go 后端开发',
  templateId: 'azure-chronicle',
  profile: { fullName: '达钦海', headline: 'Go 后端开发', email: '', phone: '', location: '', website: '', avatarUrl: '', avatarStorageKey: '' },
  sections: [{ id: 'skills', type: 'skills', title: '专业技能', visible: true, content: { items: [
    { id: 'go', name: 'Go 后端开发', note: '有实际工作经验' },
    { id: 'java', name: 'Java / Spring Boot', note: '熟悉' },
  ] } }],
};

describe('AzureChronicleTemplate skills', () => {
  it('renders skills as a semantic two-part capability list', () => {
    const { container } = render(<AzureChronicleTemplate document={document} />);

    const skills = container.querySelector('.template-azure-chronicle__skills');
    expect(skills?.tagName).toBe('DL');
    expect(skills?.querySelectorAll('dt')).toHaveLength(2);
    expect(skills?.querySelectorAll('dd')).toHaveLength(2);
    expect(skills).toHaveTextContent('Go 后端开发');
    expect(skills).toHaveTextContent('有实际工作经验');
  });

  it('uses icon headings and a template-specific honour container', () => {
    const withHonours: ResumeDocument = {
      ...document,
      sections: [
        { id: 'honours', type: 'honours', title: '荣誉奖项', visible: true, content: { items: [{ id: 'award', name: '年度创新奖' }] } },
        ...document.sections,
      ],
    };
    const { container } = render(<AzureChronicleTemplate document={withHonours} />);
    expect(container.querySelectorAll('.template-azure-chronicle__icon svg')).not.toHaveLength(0);
    expect(container.querySelectorAll('.template-azure-chronicle__icon svg path')).not.toHaveLength(0);
    expect(container.querySelector('.template-azure-chronicle__honours[data-template-honours]')).toBeInTheDocument();
    expect(container.querySelector('.template-azure-chronicle__entry-heading')).toBeNull();
  });
});
