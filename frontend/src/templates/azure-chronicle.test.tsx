import { render } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { AzureChronicleTemplate } from './azure-chronicle';
import type { ResumeDocument } from '../types/resume';

const document: ResumeDocument = {
  title: '达钦海｜Go 后端开发',
  templateId: 'azure-chronicle',
  profile: { fullName: '达钦海', headline: 'Go 后端开发', email: '', phone: '', location: '', website: '', summary: '' },
  experiences: [],
  projects: [],
  education: [],
  skills: [
    { id: 'go', name: 'Go 后端开发', level: '有实际工作经验' },
    { id: 'java', name: 'Java / Spring Boot', level: '熟悉' },
  ],
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
});
