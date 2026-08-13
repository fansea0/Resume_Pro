import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';

import { templateRegistry } from './registry';
import type { ResumeDocument } from '../types/resume';

const sampleResume: ResumeDocument = {
  title: '产品经理简历',
  templateId: 'minimal',
  profile: {
    fullName: '林晓',
    headline: '高级产品经理',
    email: 'lin@example.com',
    phone: '13800000000',
    location: '上海',
    website: 'https://example.com',
    summary: '专注互联网产品增长与用户体验。',
  },
  experiences: [{
    id: 'work-1',
    company: '星云科技',
    role: '产品经理',
    startDate: '2022.03',
    endDate: '至今',
    highlights: ['负责增长产品，月活提升 30%。'],
  }],
  projects: [],
  education: [],
  skills: [],
};

afterEach(cleanup);

describe('templateRegistry', () => {
  it('registers the minimal, business, creative, tech, editorial, redline, violet, and azure chronicle templates', () => {
    expect(templateRegistry.map((template) => template.id)).toEqual([
      'minimal',
      'business',
      'creative',
      'tech',
      'editorial',
      'redline',
      'violet',
      'azure-chronicle',
    ]);
  });

  it.each(templateRegistry)('defines a static preview cover for $id', (template) => {
    expect(template.previewImage).toBe(`/template-previews/${template.id}.png`);
  });

  it.each([
    ['tech', 'CAREER SIGNAL'],
    ['editorial', 'PROFILE'],
    ['redline', '个人总结'],
    ['violet', '个人总结'],
    ['azure-chronicle', '个人总结'],
  ])('renders the distinctive heading in %s', (templateId, heading) => {
    const template = templateRegistry.find(({ id }) => id === templateId);

    expect(template).toBeDefined();
    const Renderer = template!.Renderer;
    render(<Renderer document={sampleResume} />);

    expect(screen.getByText('林晓')).toBeInTheDocument();
    expect(screen.getByText('星云科技')).toBeInTheDocument();
    expect(screen.getByText(heading)).toBeInTheDocument();
  });

  it.each(templateRegistry)('renders profile and experience content in $id', ({ Renderer }) => {
    render(<Renderer document={sampleResume} />);

    expect(screen.getByText('林晓')).toBeInTheDocument();
    expect(screen.getByText('星云科技')).toBeInTheDocument();
    expect(screen.getByText('负责增长产品，月活提升 30%。')).toBeInTheDocument();
  });
});
