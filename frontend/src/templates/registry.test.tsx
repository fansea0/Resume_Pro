import { useState } from 'react';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { getTemplate, templateRegistry } from './registry';
import { ITEM_ANCHOR_ATTR, SECTION_ANCHOR_ATTR, type LocateHandler } from '../components/ResumeLocate';
import {
  createSampleResumeDocument,
  type CampusItem,
  type EducationItem,
  type ExperienceItem,
  type ProjectItem,
  type ResumeDocument,
  type ResumeSection,
  type TemplateId,
} from '../types/resume';

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

function Renderer({ id, onLocate }: { id: TemplateId; onLocate?: LocateHandler }) {
  const Entry = getTemplate(id).Renderer;
  return <Entry document={createSampleResumeDocument()} onLocate={onLocate} />;
}

const sampleSections = createSampleResumeDocument().sections;
const visibleSections = sampleSections.filter((section) => section.visible);
/**
 * 条目级不变量覆盖哪些区块，同样从文档推导，与 `ResumeEditor.test.tsx` 的 `'items' in section.content`
 * 同一思路：`content` 带 `items` 的区块天然是条目区块，将来新增第六种条目区块会自动纳入锚点与
 * 按钮断言，不会像写死类型列表那样静默漏掉。
 *
 * 唯一的例外是「个人总结」「专业技能」「荣誉奖项」按设计决策只提供区块级定位：总结是单个 textarea，
 * 技能与荣誉渲染为内联 chip / `<li>`，点击区域过小，套按钮会违反 AGSIN.md 的可用性约束。
 */
const SECTION_LEVEL_ONLY_TYPES: ReadonlySet<ResumeSection['type']> = new Set(['summary', 'skills', 'honours']);
type ItemSection = Extract<ResumeSection, { content: { items: unknown[] } }>;
const itemSections = sampleSections.filter(
  (section): section is ItemSection => section.visible && 'items' in section.content && !SECTION_LEVEL_ONLY_TYPES.has(section.type),
);
const locateEntryCount = itemSections.reduce((total, section) => total + section.content.items.length, 0);

/**
 * 每个模板的条目标题取文档里的哪个字段。这是实现事实而非约定：12 个模板的 `EntryLocateButton`
 * 调用点已逐一核对，唯一分歧是 `business` 的经历类条目取岗位名（role），其余 11 个取公司名。
 * 断言必须写死到这一层，否则「所有条目按钮都播报区块标题」这种回归仍能过——读屏用户靠的
 * 就是这个可访问名称来区分条目。
 */
function entryLabel(templateId: TemplateId, section: ItemSection, item: unknown): string {
  if (templateId === 'business' && (section.type === 'work' || section.type === 'internship')) {
    return (item as ExperienceItem).role;
  }
  switch (section.type) {
    case 'work':
    case 'internship':
      return (item as ExperienceItem).company;
    case 'project':
      return (item as ProjectItem).name;
    case 'education':
      return (item as EducationItem).school;
    case 'campus':
      return (item as CampusItem).name;
    default:
      return '';
  }
}

/**
 * 条目标题元素：多数模板用 `<h3>`，tech 的教育分支用 `<strong>`。
 * 条目标题不带 `data-resume-section-title`，因此不能按区块标题那样用全局选择器查找——
 * 必须从条目锚点元素内部向下定位。
 */
const ENTRY_HEADING_SELECTOR = 'h1,h2,h3,h4,h5,h6,strong';

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

describe('全部模板的区块与标题定位不变量', () => {
  it.each(templateRegistry.map((t) => t.id))('%s 的每个可见区块都有 section id 锚点', (id) => {
    const { container } = render(<Renderer id={id} />);
    // 与下面两条不变量一样先按 visible 过滤：隐藏区块根本不渲染，
    // 不过滤的话这里会去要求一个永远不存在的锚点。
    for (const section of visibleSections) {
      expect(container.querySelector(`[data-resume-section="${section.type}"][${SECTION_ANCHOR_ATTR}="${section.id}"]`)).not.toBeNull();
    }
  });

  // 期望值全部从文档与 DOM 推导，不写死区块个数：将来新增区块类型时，
  // 本用例不会因为「个数变了」这种错误原因失败。
  it.each(templateRegistry.map((t) => t.id))('%s 的每个区块标题内都有定位按钮', (id) => {
    const { container } = render(<Renderer id={id} onLocate={vi.fn()} />);
    const headings = Array.from(container.querySelectorAll('[data-resume-section-title]'));

    // 姓名所在的 <h1> 没有 data-resume-section-title，因此这里只数区块标题。
    expect(headings).toHaveLength(visibleSections.length);
    for (const [index, section] of visibleSections.entries()) {
      const heading = headings[index];
      // 从标题元素内部查找，天然保证按钮在 <h2> 之内——按钮被挪到 <h2> 之外时这里会变成 0 个。
      const buttonsInHeading = Array.from(heading.querySelectorAll('button'));
      expect(buttonsInHeading).toHaveLength(1);
      expect(buttonsInHeading[0].getAttribute('aria-label')).toBe(`定位到编辑器的「${section.title}」`);
      // 按钮只包裹标题文字：标题内不得有落在按钮之外的文本或图标。
      expect(heading.textContent).toBe(buttonsInHeading[0].textContent);
    }
    // 无障碍兜底：必须是带可访问名称的真实 button。条目按钮与区块按钮共用同一命名模式，
    // 因此这里断言全等：区块标题数 + 经历类条目数，两者都从文档推导，不写死。
    // 实测 12 个模板（含 creative / editorial 双栏拆分）均为 8 + 5 = 13 个按钮。
    expect(screen.getAllByRole('button', { name: /^定位到编辑器的「.+」$/ })).toHaveLength(visibleSections.length + locateEntryCount);
  });
});

describe('全部模板的条目定位不变量', () => {
  /**
   * 复现真实链路：点击预览里的定位按钮 → App 写 state → 预览整体重渲染。
   * 槽位是被当作组件渲染的（`<Section .../>`），槽位身份一变 React 就卸载重建该子树，
   * 正在聚焦的节点被摘出文档，键盘用户的焦点就此丢失（只能重新 Tab 找回来）。
   */
  function StatefulRenderer({ id }: { id: TemplateId }) {
    const [locateCount, setLocateCount] = useState(0);
    const Entry = getTemplate(id).Renderer;
    return (
      <>
        <Entry document={createSampleResumeDocument()} onLocate={() => setLocateCount((count) => count + 1)} />
        <output data-testid="locate-count">{locateCount}</output>
      </>
    );
  }

  // 全模板门禁，而不是只盯 business：槽位写成内联箭头是随时可能重犯的错误，
  // 断言写在不变量这一层，新模板漏改会立刻在 CI 失败。
  it.each(templateRegistry.map((t) => t.id))('%s 的条目定位按钮在触发重渲染后仍保留键盘焦点', (id) => {
    render(<StatefulRenderer id={id} />);
    const button = screen.getByRole('button', { name: '定位到编辑器的「AI 创作副驾」' });

    button.focus();
    expect(document.activeElement).toBe(button);
    fireEvent.click(button);

    expect(screen.getByTestId('locate-count')).toHaveTextContent('1');
    // 断言节点同一性：子树没被卸载重建，聚焦的节点才还在文档里。
    // 焦点掉回 body 时这里失败（沙箱里重渲染后没有任何元素会被自动聚焦）。
    expect(document.activeElement).toBe(button);
  });

  it.each(templateRegistry.map((t) => t.id))('%s 的每个经历类条目都有 item id 锚点', (id) => {
    const { container } = render(<Renderer id={id} />);
    for (const section of itemSections) {
      for (const item of section.content.items) {
        expect(container.querySelector(`[${ITEM_ANCHOR_ATTR}="${(item as { id: string }).id}"]`)).not.toBeNull();
      }
    }
  });

  // 条目按钮的无障碍兜底：数量、位置、可访问名称与定位载荷四件事都要钉住。
  // 只有区块标题有这一层断言时，把条目按钮挪出 <h3> 或改掉 itemId 都还能保持总数不变。
  it.each(templateRegistry.map((t) => t.id))('%s 的每个经历类条目标题内恰有一个定位按钮', (id) => {
    const onLocate = vi.fn();
    const { container } = render(<Renderer id={id} onLocate={onLocate} />);
    for (const section of itemSections) {
      for (const item of section.content.items) {
        const itemId = (item as { id: string }).id;
        const entry = container.querySelector(`[${ITEM_ANCHOR_ATTR}="${itemId}"]`)!;
        const buttons = Array.from(entry.querySelectorAll('button'));
        const expectedLabel = entryLabel(id, section, item);

        // 无标题的条目按设计降级为纯文本：`business` 对空标题走 `{label ? <Button/> : label}`，
        // 此时 <h3> 里是空字符串，套按钮只会给读屏播报一个空名字。因此期望个数从文档推导：
        // 有标题则恰一个（重复渲染会 >1、漏渲染会 =0），无标题则 0 个。
        expect(buttons).toHaveLength(expectedLabel ? 1 : 0);
        if (!expectedLabel) continue;
        const button = buttons[0];

        // 按钮必须在条目自己的标题元素内部。从按钮向上找最近的标题元素，且不能越过条目根：
        // 按钮仍在 <h3> / <strong> 里时能找到；被挪到标题之外（挪进外层容器或挪成兄弟节点）
        // 时就会一路走到条目根仍无命中，断言随之失败。全程不依赖全局标题选择器。
        let node: HTMLElement | null = button;
        let heading: HTMLElement | null = null;
        while (node && node !== entry) {
          if (node.matches(ENTRY_HEADING_SELECTOR)) {
            heading = node;
            break;
          }
          node = node.parentElement;
        }
        expect(heading).not.toBeNull();

        // 可访问名称必须等于「文档里这个条目的标题」，而不是从按钮自身的 DOM 反推。
        // 后者对「所有条目按钮都播报同一个名字」这种回归完全免疫，而那恰好是读屏用户
        // 区分条目的唯一线索。可见文字也一并对齐，两个字符串任一错都会被抓住。
        expect(button.getAttribute('aria-label')).toBe(`定位到编辑器的「${expectedLabel}」`);
        expect(button.textContent).toBe(expectedLabel);

        // 载荷必须指向这个条目本身：sectionId / itemId 与文档中的条目对应。
        // ResumeSections.test.tsx 只覆盖默认槽路径，6 个自带 item 槽的模板靠这里保证。
        onLocate.mockClear();
        fireEvent.click(button);
        expect(onLocate).toHaveBeenCalledTimes(1);
        expect(onLocate).toHaveBeenCalledWith({ sectionId: section.id, itemId: itemId });
      }
    }
  });
});
