import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { MintCanvasTemplate } from './mint-canvas';
import type { ResumeDocument } from '../types/resume';

const document: ResumeDocument = {
  schemaVersion: 2,
  title: '薄荷绿卡样例',
  templateId: 'mint-canvas',
  profile: {
    fullName: '陈知微',
    headline: 'Java 研发｜校招',
    email: 'zhiwei.chen@example.com',
    phone: '138-0000-2026',
    location: '杭州',
    website: 'zhiwei.dev',
    avatarUrl: '',
    avatarStorageKey: '',
    showAvatar: true,
  },
  sections: [
    {
      id: 'internship',
      type: 'internship',
      title: '实习经历',
      visible: true,
      content: {
        items: [
          {
            id: 'intern-1',
            company: '新芽科技',
            role: '后端实习生',
            startDate: '2024.07',
            endDate: '2024.10',
            location: '杭州',
            achievements: '负责订单服务重构，接口平均响应时间下降 38%。',
            highlightsMarkdown:
              '- **独立设计**：负责订单中台架构方案，输出 4 份 RFC 文档。\n- **性能优化**：基于本地缓存 + 异步队列把 P99 从 1.2s 降至 240ms。',
            showLocation: true,
            showAchievements: true,
          },
        ],
      },
    },
    {
      id: 'project',
      type: 'project',
      title: '项目经历',
      visible: true,
      content: {
        items: [
          {
            id: 'project-1',
            name: '小芽 RAG 助手',
            role: '项目负责人',
            startDate: '2025.03',
            endDate: '2025.05',
            location: '',
            link: '',
            introduction: '基于 RAG 的内部文档问答机器人。',
            techStack: 'Spring Boot / Caffeine / MyBatis-Plus',
            achievements: '',
            highlightsMarkdown:
              '- **索引策略**：按文件类型定制分块，检索召回率提升 22%。\n- **二级缓存**：Spring Cache + Caffeine 减少非关键功能 60% 重复查询。',
            showLocation: false,
            showLink: false,
            showIntroduction: true,
            showTechStack: true,
            showAchievements: false,
          },
        ],
      },
    },
    {
      id: 'skills',
      type: 'skills',
      title: '专业技能',
      visible: true,
      content: {
        items: [
          { id: 'skill-java', name: 'Java', note: '熟练' },
          { id: 'skill-spring', name: 'Spring Boot', note: '熟练' },
        ],
      },
    },
    {
      id: 'honours',
      type: 'honours',
      title: '荣誉奖项',
      visible: true,
      content: { items: [{ id: 'h-1', name: '校级一等奖学金' }] },
    },
  ],
};

describe('MintCanvasTemplate', () => {
  it('wraps content in a rounded mint-bordered card and uses a green header block', () => {
    const { container } = render(<MintCanvasTemplate document={document} />);
    const root = container.querySelector('.template-mint-canvas');
    expect(root).toBeInTheDocument();
    expect(container.querySelector('.template-mint-canvas__header')).toBeInTheDocument();
    expect(container.querySelector('.template-mint-canvas__avatar')).toBeInTheDocument();
    expect(screen.getByText('陈知微')).toBeInTheDocument();
  });

  it('renders badge-style section headings and an inline keyword chip per bullet', () => {
    const { container } = render(<MintCanvasTemplate document={document} />);
    const badges = container.querySelectorAll('.template-mint-canvas__badge');
    // summary + work + project + skills + honours is 5 (no work here so 4 sections, but we still
    // expect 4 badge icons; assert >= 4 to be resilient to future section additions).
    expect(badges.length).toBeGreaterThanOrEqual(4);
    expect(container.querySelectorAll('.template-mint-canvas__badge svg').length).toBe(badges.length);
    // bullets from internship + project, with 关键词: 形式 should each render a chip
    const chips = container.querySelectorAll('.template-mint-canvas__chip');
    expect(chips.length).toBe(4);
    expect(chips[0]).toHaveTextContent('独立设计');
    expect(chips[2]).toHaveTextContent('索引策略');
  });

  it('puts the project role on the title row and the tech stack right under the introduction', () => {
    const { container } = render(<MintCanvasTemplate document={document} />);
    const projectEntry = Array.from(container.querySelectorAll('[data-resume-entry]'))
      .find((node) => node.textContent?.includes('小芽 RAG 助手'));
    expect(projectEntry).toBeDefined();

    const heading = projectEntry?.querySelector('.template-mint-canvas__entry-heading');
    const title = heading?.querySelector('h3')?.textContent;
    const role = heading?.querySelector('.template-mint-canvas__role')?.textContent;
    const date = heading?.querySelector('.template-mint-canvas__entry-date')?.textContent;
    expect(heading?.textContent).toContain('小芽 RAG 助手');
    expect(heading?.textContent).toContain('项目负责人');
    expect(heading?.textContent).toContain('2025.03');
    // Title leads, role sits in the middle slot, date closes the row.
    expect([title, role, date]).toEqual(['小芽 RAG 助手', '项目负责人', '2025.03 – 2025.05']);

    const subLines = Array.from(projectEntry?.querySelectorAll('.template-mint-canvas__entry-sub, .template-mint-canvas__tech') ?? [])
      .map((node) => node.textContent ?? '');
    expect(subLines[0]).toContain('项目介绍：');
    expect(subLines[0]).toContain('基于 RAG 的内部文档问答机器人。');
    expect(subLines[1]).toContain('相关技术：');

    const techChips = Array.from(projectEntry?.querySelectorAll('.template-mint-canvas__tech .template-mint-canvas__skill') ?? [])
      .map((node) => node.textContent ?? '');
    // A slash keeps a grouped technology inside one chip.
    expect(techChips).toEqual(['Spring Boot / Caffeine / MyBatis-Plus']);
  });

  it('splits tech-stack entries on Chinese and ASCII commas only', () => {
    const withCommas = {
      ...document,
      sections: document.sections.map((section) => (
        section.type === 'project'
          ? { ...section, content: { items: section.content.items.map((item) => ({ ...item, techStack: 'Go、Redis / Kafka，MySQL, PostgreSQL' })) } }
          : section
      )),
    };
    const { container } = render(<MintCanvasTemplate document={withCommas} />);
    const techChips = Array.from(container.querySelectorAll('.template-mint-canvas__tech .template-mint-canvas__skill'))
      .map((node) => node.textContent ?? '');
    expect(techChips).toEqual(['Go', 'Redis / Kafka', 'MySQL', 'PostgreSQL']);
  });

  it('hides the whole tech-stack row when its display switch is off', () => {
    const hidden = {
      ...document,
      sections: document.sections.map((section) => (
        section.type === 'project'
          ? { ...section, content: { items: section.content.items.map((item) => ({ ...item, showTechStack: false })) } }
          : section
      )),
    };
    const { container } = render(<MintCanvasTemplate document={hidden} />);
    expect(container.querySelectorAll('.template-mint-canvas__tech')).toHaveLength(0);
    // The introduction keeps its own visibility switch and stays visible.
    expect(container.querySelectorAll('.template-mint-canvas__meta-label')).toHaveLength(1);
  });

  it('omits the tech-stack line when the project has none', () => {
    const withoutTechStack = {
      ...document,
      sections: document.sections.map((section) => (
        section.type === 'project'
          ? { ...section, content: { items: section.content.items.map((item) => ({ ...item, techStack: '' })) } }
          : section
      )),
    };
    const { container } = render(<MintCanvasTemplate document={withoutTechStack} />);
    expect(container.querySelectorAll('.template-mint-canvas__tech')).toHaveLength(0);
  });

  it('renders inline Markdown in bullets, the introduction and achievements without leaking asterisks', () => {
    const rich = {
      ...document,
      sections: document.sections.map((section) => (
        section.type === 'project'
          ? {
            ...section,
            content: {
              items: section.content.items.map((item) => ({
                ...item,
                introduction: '负责 **检索链路** 与 *召回* 优化。',
                achievements: '耗时下降 **30%**。',
                highlightsMarkdown: '- **索引策略**：分块后 **召回率** 提升 22%。\n- 普通 **加粗** 与 *斜体* 文本',
                showAchievements: true,
              })),
            },
          }
          : section
      )),
    };
    const { container } = render(<MintCanvasTemplate document={rich} />);
    const projectEntry = Array.from(container.querySelectorAll('[data-resume-entry]'))
      .find((node) => node.textContent?.includes('小芽 RAG 助手')) as HTMLElement;
    const markdown = projectEntry.querySelector('.template-mint-canvas__markdown') as HTMLElement;
    // The chip keyword keeps its own treatment while the tail renders Markdown.
    expect(markdown.querySelector('.template-mint-canvas__chip')?.textContent).toBe('索引策略');
    expect(markdown.querySelector('li strong')?.textContent).toBe('召回率');
    expect(markdown.querySelector('li em')?.textContent).toBe('斜体');
    expect(markdown.textContent).not.toContain('*');

    expect(projectEntry.querySelector('.template-mint-canvas__entry-sub strong')?.textContent).toBe('检索链路');
    expect(projectEntry.querySelector('.template-mint-canvas__achievement strong')?.textContent).toBe('30%');
    expect(projectEntry.querySelector('.template-mint-canvas__entry-sub')?.textContent).not.toContain('*');
    expect(projectEntry.querySelector('.template-mint-canvas__achievement')?.textContent).not.toContain('*');
  });

  it('renders experience, project, skills, and honours content with the template landmark', () => {
    const { container } = render(<MintCanvasTemplate document={document} />);
    const entries = Array.from(container.querySelectorAll('.template-mint-canvas__entry')).map((node) => node.textContent ?? '');
    expect(entries.some((text) => text.includes('新芽科技'))).toBe(true);
    expect(entries.some((text) => text.includes('小芽 RAG 助手'))).toBe(true);
    const skillChips = Array.from(container.querySelectorAll('.template-mint-canvas__skill')).map((node) => node.textContent ?? '');
    expect(skillChips.some((text) => text.includes('Java'))).toBe(true);
    const honours = Array.from(container.querySelectorAll('.template-mint-canvas__honour')).map((node) => node.textContent ?? '');
    expect(honours.some((text) => text.includes('校级一等奖学金'))).toBe(true);
    const markdown = container.querySelector('.template-mint-canvas__markdown')?.textContent ?? '';
    expect(markdown).toContain('负责订单中台架构方案，输出 4 份 RFC 文档。');
  });
});
