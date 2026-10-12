import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';

import { templateRegistry } from './registry';
import type { ResumeDocument } from '../types/resume';

const sampleResume: ResumeDocument = {
  schemaVersion: 2,
  title: '产品经理简历',
  templateId: 'minimal',
  profile: {
    fullName: '林晓',
    headline: '高级产品经理',
    email: 'lin@example.com',
    phone: '13800000000',
    location: '上海',
    website: 'https://example.com',
    avatarUrl: '/uploads/avatars/test.webp',
    avatarStorageKey: '123e4567-e89b-42d3-a456-426614174000',
    showAvatar: true,
  },
  // Deliberately non-standard: every template must honor this stored order.
  sections: [
    { id: 'honours', type: 'honours', title: '荣誉奖项', visible: true, content: { items: [{ id: 'award', name: '年度创新奖' }] } },
    { id: 'campus', type: 'campus', title: '校园经历', visible: true, content: { items: [{ id: 'campus-item', name: '产品社', role: '负责人', highlightsMarkdown: '- 组织活动。' }] } },
    { id: 'skills', type: 'skills', title: '专业技能', visible: true, content: { items: [{ id: 'skill', name: '用户研究', note: '熟练' }] } },
    { id: 'education', type: 'education', title: '教育经历', visible: true, content: { items: [{ id: 'education-item', school: '复旦大学', startDate: '2018.09', endDate: '2022.06', degree: '本科', college: '管理学院', major: '信息管理' }] } },
    { id: 'project', type: 'project', title: '项目经历', visible: true, content: { items: [{ id: 'project-item', name: '增长平台', role: '负责人', startDate: '2023.01', endDate: '2023.06', location: '上海', link: 'https://example.com', introduction: '项目介绍', techStack: 'Go / Redis', achievements: '项目成绩', highlightsMarkdown: '- 项目说明', showLocation: true, showLink: true, showIntroduction: true, showTechStack: true, showAchievements: true }] } },
    { id: 'internship', type: 'internship', title: '实习经历', visible: true, content: { items: [{ id: 'internship-item', company: '实习公司', role: '实习生', startDate: '2021.01', endDate: '2021.06', location: '上海', achievements: '实习成绩', highlightsMarkdown: '- 实习说明', showLocation: true, showAchievements: true }] } },
    { id: 'work', type: 'work', title: '工作经历', visible: true, content: { items: [{ id: 'work-1', company: '星云科技', role: '产品经理', startDate: '2022.03', endDate: '至今', location: '上海', achievements: '月活提升 30%。', highlightsMarkdown: '- 负责增长产品，月活提升 30%。', showLocation: true, showAchievements: true }] } },
    { id: 'summary', type: 'summary', title: '个人总结', visible: true, content: { markdown: '专注互联网产品增长与用户体验。' } },
  ],
};

afterEach(cleanup);

describe('templateRegistry', () => {
  it('registers the minimal, business, creative, tech, editorial, redline, violet, azure chronicle, mint canvas, scholarly serif, blue sidebar, and navy gutter templates', () => {
    expect(templateRegistry.map((template) => template.id)).toEqual([
      'minimal',
      'business',
      'creative',
      'tech',
      'editorial',
      'redline',
      'violet',
      'azure-chronicle',
      'mint-canvas',
      'scholarly-serif',
      'blue-sidebar',
      'navy-gutter',
    ]);
  });

  it('renders the navy gutter label rail, section card, and resume content', () => {
    const template = templateRegistry.find(({ id }) => id === 'navy-gutter');

    expect(template).toBeDefined();
    const Renderer = template!.Renderer;
    const { container } = render(<Renderer document={sampleResume} />);

    const root = container.querySelector('.template-navy-gutter');
    expect(root).toBeInTheDocument();
    // Section labels are navy blocks pinned to the top-left of each section card.
    const skillsSection = container.querySelector('[data-resume-section="skills"]');
    expect(skillsSection?.classList.contains('template-navy-gutter__section')).toBe(true);
    const skillsLabel = skillsSection?.querySelector('[data-resume-section-title] .template-navy-gutter__label');
    expect(skillsLabel?.textContent).toBe('专业技能');
    // The first section extends a navy rule out of its label, as in the reference.
    const summarySection = container.querySelector('[data-resume-section="summary"]');
    expect(summarySection?.classList.contains('template-navy-gutter__section--summary')).toBe(true);
    // Skills render as chips with a highlighted core item.
    const chips = container.querySelectorAll('[data-resume-section="skills"] .template-navy-gutter__skill');
    expect(chips).toHaveLength(1);
    expect(chips[0]?.classList.contains('template-navy-gutter__skill--core')).toBe(true);
    expect(container.querySelector('[data-template-honours]')).toBeInTheDocument();

    expect(screen.getByText('林晓')).toBeInTheDocument();
    expect(screen.getByText('星云科技')).toBeInTheDocument();
  });

  it('renders the blue sidebar identity landmark and resume content', () => {
    const template = templateRegistry.find(({ id }) => id === 'blue-sidebar');

    expect(template).toBeDefined();
    const Renderer = template!.Renderer;
    const { container } = render(<Renderer document={sampleResume} />);

    expect(container.querySelector('.template-blue-sidebar__sidebar')).toBeInTheDocument();
    expect(screen.getByText('林晓')).toBeInTheDocument();
    expect(screen.getByText('星云科技')).toBeInTheDocument();
  });

  it('renders the scholarly serif identity landmark, all-caps section title, and resume content', () => {
    const template = templateRegistry.find(({ id }) => id === 'scholarly-serif');

    expect(template).toBeDefined();
    const Renderer = template!.Renderer;
    const { container } = render(<Renderer document={sampleResume} />);

    const root = container.querySelector('.template-scholarly-serif');
    expect(root).toBeInTheDocument();
    // The full name renders inside a centered italic serif header.
    expect(container.querySelector('.template-scholarly-serif__name')?.textContent).toBe('林晓');
    // All-caps underlined section title is the visual landmark of the template.
    const honoursTitle = container.querySelector('[data-resume-section="honours"] .template-scholarly-serif__section-title');
    expect(honoursTitle?.textContent?.trim()).toBe('荣誉奖项');
    // Standard persisted content still flows through the template.
    expect(screen.getByText('星云科技')).toBeInTheDocument();
    expect(screen.getByText('年度创新奖')).toBeInTheDocument();
  });

  it.each(templateRegistry)('defines a static preview cover for $id', (template) => {
    expect(template.previewImage).toBe(`/template-previews/${template.id}.png`);
  });

  it.each(templateRegistry)('renders the resume photo when one is provided in $id', ({ Renderer }) => {
    render(<Renderer document={sampleResume} />);
    expect(screen.getByAltText('林晓的简历照片')).toHaveAttribute('src', '/uploads/avatars/test.webp');
  });

  it.each(templateRegistry)('defines Chinese job tags and a style description for $id', (template) => {
    expect(template.tags.length).toBeGreaterThanOrEqual(2);
    expect(template.tags.length).toBeLessThanOrEqual(3);
    expect(template.tags.every((tag) => /[\u4e00-\u9fff]/.test(tag))).toBe(true);
    expect(template.styleDescription).toMatch(/[\u4e00-\u9fff]/);
  });

  it('uses role or job-context tags for the azure chronicle template', () => {
    const azureChronicle = templateRegistry.find(({ id }) => id === 'azure-chronicle');

    expect(azureChronicle?.tags).toContain('运营');
    expect(azureChronicle?.tags).not.toContain('中文');
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

  it.each(templateRegistry)('renders all visible section types in persisted order in $id', ({ Renderer }) => {
    render(<Renderer document={sampleResume} />);

    expect(screen.getByText('林晓')).toBeInTheDocument();
    expect(screen.getByText('星云科技')).toBeInTheDocument();
    expect(screen.getByText('负责增长产品，月活提升 30%。')).toBeInTheDocument();
    expect(screen.getByText('年度创新奖')).toBeInTheDocument();
    expect(screen.getByText('产品社')).toBeInTheDocument();
    expect(screen.getByText('增长平台')).toBeInTheDocument();
    const titles = Array.from(document.querySelectorAll('[data-resume-section-title]')).map((node) => node.lastElementChild?.textContent ?? node.textContent);
    expect(titles).toEqual(['荣誉奖项', '校园经历', '专业技能', '教育经历', '项目经历', '实习经历', '工作经历', '个人总结']);
    expect(document.querySelector('[data-template-honours]')).toBeInTheDocument();
  });

  it('keeps the business intro, tech education grid, and redline entry heading contracts', () => {
    const Business = templateRegistry.find(({ id }) => id === 'business')!.Renderer;
    const { container: business } = render(<Business document={sampleResume} />);
    expect(business.querySelector('.template-business__intro')).toBeInTheDocument();
    const businessEntry = business.querySelector('.template-business__entry');
    expect(businessEntry?.children).toHaveLength(2);
    expect(businessEntry?.firstElementChild).toHaveClass('template-business__period');

    const Tech = templateRegistry.find(({ id }) => id === 'tech')!.Renderer;
    const { container: tech } = render(<Tech document={sampleResume} />);
    expect(tech.querySelector('.template-tech__education')).toBeInTheDocument();
    const techEducation = tech.querySelector('.template-tech__education');
    expect(techEducation?.querySelector(':scope > strong')).toBeInTheDocument();
    expect(techEducation?.querySelector(':scope > span')).toBeInTheDocument();
    expect(techEducation?.querySelector(':scope > time')).toBeInTheDocument();

    const Redline = templateRegistry.find(({ id }) => id === 'redline')!.Renderer;
    const { container: redline } = render(<Redline document={sampleResume} />);
    expect(redline.querySelector('.template-redline__entry-heading')).toBeInTheDocument();
  });

  it('keeps visible section content in both creative and editorial columns', () => {
    const Creative = templateRegistry.find(({ id }) => id === 'creative')!.Renderer;
    const { container: creative } = render(<Creative document={sampleResume} />);
    expect(creative.querySelector('.template-creative__sidebar [data-resume-section]')).toBeInTheDocument();

    const Editorial = templateRegistry.find(({ id }) => id === 'editorial')!.Renderer;
    const { container: editorial } = render(<Editorial document={sampleResume} />);
    expect(editorial.querySelector('.template-editorial__sidebar [data-resume-section]')).toBeInTheDocument();
  });

  it.each(['business', 'tech'] as const)('renders optional item fields only while enabled in %s', (templateId) => {
    const Renderer = templateRegistry.find(({ id }) => id === templateId)!.Renderer;
    const { rerender } = render(<Renderer document={sampleResume} />);
    expect(screen.getAllByText('上海').length).toBeGreaterThan(0);
    expect(screen.getByText('项目介绍')).toBeInTheDocument();
    expect(screen.getByText('项目成绩')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'https://example.com' })).toHaveAttribute('href', 'https://example.com');

    const hidden = {
      ...sampleResume,
      sections: sampleResume.sections.map((section) => section.type === 'project'
        ? { ...section, content: { items: section.content.items.map((item) => ({ ...item, showLocation: false, showLink: false, showIntroduction: false, showTechStack: false, showAchievements: false })) } }
        : section.type === 'work'
          ? { ...section, content: { items: section.content.items.map((item) => ({ ...item, showLocation: false, showAchievements: false })) } }
          : section),
    } as ResumeDocument;
    rerender(<Renderer document={hidden} />);
    expect(screen.queryByText('项目介绍')).not.toBeInTheDocument();
    expect(screen.queryByText('项目成绩')).not.toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'https://example.com' })).not.toBeInTheDocument();
  });
});
