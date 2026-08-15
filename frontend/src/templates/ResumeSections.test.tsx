import { render, screen, within } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { ResumeSections } from './ResumeSections';
import type { ResumeSection } from '../types/resume';

const sections: ResumeSection[] = [
  {
    id: 'honours', type: 'honours', title: '荣誉奖项', visible: true,
    content: { items: [{ id: 'honour-1', name: '一等奖学金' }] },
  },
  {
    id: 'project', type: 'project', title: '项目经历', visible: true,
    content: { items: [{
      id: 'project-1', name: '简历平台', role: '项目负责人', startDate: '2025.01', endDate: '2025.06',
      location: '杭州', link: 'https://example.com', introduction: '这是项目介绍', achievements: '获校赛一等奖',
      highlightsMarkdown: '- **实现**项目编辑', showLocation: false, showLink: false,
      showIntroduction: false, showAchievements: false,
    }] },
  },
  {
    id: 'summary', type: 'summary', title: '个人总结', visible: true,
    content: { markdown: '**有经验的**开发者' },
  },
  {
    id: 'work', type: 'work', title: '工作经历', visible: true,
    content: { items: [{
      id: 'work-1', company: '示例公司', role: '后端开发', startDate: '2024.01', endDate: '至今',
      location: '上海', achievements: '增长 50%', highlightsMarkdown: '1. **交付**核心功能',
      showLocation: false, showAchievements: false,
    }] },
  },
  {
    id: 'internship', type: 'internship', title: '实习经历', visible: true,
    content: { items: [{
      id: 'internship-1', company: '实习公司', role: '实习生', startDate: '2023.01', endDate: '2023.06',
      location: '北京', achievements: '发布 3 个功能', highlightsMarkdown: '- 支持用户',
      showLocation: true, showAchievements: true,
    }] },
  },
  {
    id: 'education', type: 'education', title: '教育经历', visible: true,
    content: { items: [{
      id: 'education-1', school: '示例大学', startDate: '2019.09', endDate: '2023.06', degree: '本科',
      college: '计算机学院', major: '软件工程',
    }] },
  },
  {
    id: 'skills', type: 'skills', title: '专业技能', visible: true,
    content: { items: [{ id: 'skill-1', name: 'Go', note: '熟悉' }] },
  },
  {
    id: 'campus', type: 'campus', title: '校园经历', visible: true,
    content: { items: [{ id: 'campus-1', name: '学生会', role: '部长', highlightsMarkdown: '- 组织 **活动**' }] },
  },
  {
    id: 'hidden', type: 'summary', title: '不应显示的区块', visible: false,
    content: { markdown: '不应显示的内容' },
  },
];

describe('ResumeSections', () => {
  it('按 sections 数组顺序渲染全部可见区块及其内容', () => {
    const { container } = render(<ResumeSections sections={sections} />);

    const headings = [...container.querySelectorAll('[data-resume-section-title]')]
      .map((element) => element.textContent);
    expect(headings).toEqual(['荣誉奖项', '项目经历', '个人总结', '工作经历', '实习经历', '教育经历', '专业技能', '校园经历']);

    expect(screen.getByText('一等奖学金')).toBeInTheDocument();
    expect(screen.getByText('简历平台')).toBeInTheDocument();
    expect(screen.getByText('有经验的').tagName).toBe('STRONG');
    expect(screen.getByText('示例公司')).toBeInTheDocument();
    expect(screen.getByText('实习公司')).toBeInTheDocument();
    expect(screen.getByText('示例大学')).toBeInTheDocument();
    expect(screen.getByText('本科 · 计算机学院 · 软件工程')).toBeInTheDocument();
    expect(screen.getByText('Go')).toBeInTheDocument();
    expect(screen.getByText('熟悉')).toBeInTheDocument();
    expect(screen.getByText('学生会')).toBeInTheDocument();
    expect(screen.getByText('部长')).toBeInTheDocument();
    expect(screen.getByText('活动').tagName).toBe('STRONG');
    expect(screen.getByText('交付').tagName).toBe('STRONG');
  });

  it('不渲染隐藏区块或关闭显示开关的字段', () => {
    const { container } = render(<ResumeSections sections={sections} />);
    const view = within(container);

    expect(view.queryByText('不应显示的区块')).not.toBeInTheDocument();
    expect(view.queryByText('不应显示的内容')).not.toBeInTheDocument();
    expect(view.queryByText('杭州')).not.toBeInTheDocument();
    expect(view.queryByText('https://example.com')).not.toBeInTheDocument();
    expect(view.queryByText('这是项目介绍')).not.toBeInTheDocument();
    expect(view.queryByText('获校赛一等奖')).not.toBeInTheDocument();
    expect(view.queryByText('上海')).not.toBeInTheDocument();
    expect(view.queryByText('增长 50%')).not.toBeInTheDocument();
    expect(view.getByText('北京')).toBeInTheDocument();
    expect(view.getByText('发布 3 个功能')).toBeInTheDocument();
  });

  it('允许模板用 slots 和类名控制区块的展示容器', () => {
    const { container } = render(
      <ResumeSections
        sections={[sections[0]]}
        classNames={{ section: 'template-section', honours: 'template-honours' }}
        slots={{
          title: ({ children }) => <h4 data-testid="template-title">{children}</h4>,
          honours: ({ children, className }) => <ul className={className} data-testid="template-honours">{children}</ul>,
        }}
      />,
    );

    expect(container.querySelector('.template-section')).toBeInTheDocument();
    expect(screen.getByTestId('template-title')).toHaveTextContent('荣誉奖项');
    expect(screen.getByTestId('template-honours')).toHaveClass('template-honours');
    expect(within(screen.getByTestId('template-honours')).getByText('一等奖学金')).toBeInTheDocument();
  });
});
