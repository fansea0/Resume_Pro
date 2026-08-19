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
            achievements: '',
            highlightsMarkdown:
              '- **索引策略**：按文件类型定制分块，检索召回率提升 22%。\n- **二级缓存**：Spring Cache + Caffeine 减少非关键功能 60% 重复查询。',
            showLocation: false,
            showLink: false,
            showIntroduction: true,
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
