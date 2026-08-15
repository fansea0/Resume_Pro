import { render } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { AzureChronicleTemplate } from './azure-chronicle';
import type { ResumeDocument } from '../types/resume';

const document: ResumeDocument = {
  schemaVersion: 2,
  title: '达钦海｜Go 后端开发',
  templateId: 'azure-chronicle',
  profile: { fullName: '达钦海', headline: 'Go 后端开发', email: '', phone: '', location: '', website: '' },
  sections: [{ id: 'skills', type: 'skills', title: '专业技能', visible: true, content: { items: [
    { id: 'go', name: 'Go 后端开发', note: '有实际工作经验' },
    { id: 'java', name: 'Java / Spring Boot', note: '熟悉' },
  ] } }],
};

describe('AzureChronicleTemplate skills', () => {
  it('renders skills as a semantic two-part capability list', () => {
    const { container } = render(<AzureChronicleTemplate document={document} />);

    const skills = container.querySelector('.template-azure-chronicle__skills');
    expect(skills?.tagName).toBe('DIV');
    expect(skills?.querySelectorAll('span')).toHaveLength(2);
    expect(skills?.querySelectorAll('small')).toHaveLength(2);
    expect(skills).toHaveTextContent('Go 后端开发');
    expect(skills).toHaveTextContent('有实际工作经验');
  });
});
