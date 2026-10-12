import { cleanup, fireEvent, render, screen, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ResumeSections } from './ResumeSections';
import { createSampleResumeDocument, type ResumeSection } from '../types/resume';
import { ITEM_ANCHOR_ATTR, SECTION_ANCHOR_ATTR } from '../components/ResumeLocate';

// vitest 未开启 globals，@testing-library/react 不会自动清理，需显式 cleanup。
afterEach(cleanup);

const sampleSections = createSampleResumeDocument().sections;

const sections: ResumeSection[] = [
  {
    id: 'honours', type: 'honours', title: '荣誉奖项', visible: true,
    content: { items: [{ id: 'honour-1', name: '一等奖学金' }] },
  },
  {
    id: 'project', type: 'project', title: '项目经历', visible: true,
    content: { items: [{
      id: 'project-1', name: '简历平台', role: '项目负责人', startDate: '2025.01', endDate: '2025.06',
      location: '杭州', link: 'https://example.com', introduction: '这是项目介绍', techStack: 'React / Go', achievements: '获校赛一等奖',
      highlightsMarkdown: '- **实现**项目编辑', showLocation: false, showLink: false,
      showIntroduction: false, showTechStack: false, showAchievements: false,
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

  it('用清晰分隔符连接同时显示的时间与地点，并保持单独字段的布局', () => {
    const internship = sections[4];
    if (internship.type !== 'internship') throw new Error('test fixture must be an internship section');
    const onlyDate = { ...internship, id: 'only-date-section', content: { items: [{ ...internship.content.items[0], id: 'only-date', location: '', showLocation: false }] } };
    const onlyLocation = { ...internship, id: 'only-location-section', content: { items: [{ ...internship.content.items[0], id: 'only-location', startDate: '', endDate: '', location: '深圳', showLocation: true }] } };
    const { container } = render(<ResumeSections sections={[internship, onlyDate, onlyLocation]} />);
    const view = within(container);

    expect(view.getByText('北京').parentElement).toHaveTextContent('2023.01 — 2023.06 · 北京');
    const separator = container.querySelector('[data-resume-metadata-separator]');
    expect(separator).toHaveTextContent('·');
    expect(separator).not.toHaveAttribute('aria-hidden');
    expect(view.getAllByText('2023.01 — 2023.06')[1].parentElement).toHaveTextContent('2023.01 — 2023.06');
    expect(view.getByText('深圳').parentElement).toHaveTextContent('深圳');
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

  it('允许模板覆盖 section 和 entry 容器', () => {
    const { container } = render(
      <ResumeSections
        sections={[sections[3]]}
        slots={{
          section: ({ children, section }) => <div data-section-type={section.type}>{children}</div>,
          entry: ({ children, section }) => <div data-entry-section-type={section.type}>{children}</div>,
        }}
      />,
    );

    expect(container.querySelector('[data-section-type="work"]')).toBeInTheDocument();
    expect(container.querySelector('[data-entry-section-type="work"]')).toHaveTextContent('示例公司');
  });

  it.each(['javascript:alert(1)', 'data:text/html;base64,PHNjcmlwdD4=', ''])('不渲染不安全的项目链接：%s', (link) => {
    const project = sections[1];
    if (project.type !== 'project') throw new Error('test fixture must be a project section');
    const { container } = render(
      <ResumeSections sections={[{
        ...project,
        content: { items: [{ ...project.content.items[0], link, showLink: true }] },
      }]} />,
    );

    expect(container.querySelector('a')).not.toBeInTheDocument();
  });

  it('使用安全的项目链接', () => {
    const project = sections[1];
    if (project.type !== 'project') throw new Error('test fixture must be a project section');
    render(
      <ResumeSections sections={[{
        ...project,
        content: { items: [{ ...project.content.items[0], showLink: true }] },
      }]} />,
    );

    expect(screen.getByRole('link', { name: 'https://example.com' })).toHaveAttribute('href', 'https://example.com');
  });
});

describe('ResumeSections 定位锚点', () => {
  it('默认区块槽输出 section id 锚点，并保留既有 type 属性', () => {
    const { container } = render(<ResumeSections sections={sampleSections} />);
    const node = container.querySelector('[data-resume-section="project"]');
    expect(node).toHaveAttribute(SECTION_ANCHOR_ATTR, 'sample-project');
    expect(node).toHaveAttribute('data-resume-section', 'project');
  });

  it('默认条目槽输出 item id 锚点', () => {
    const { container } = render(<ResumeSections sections={sampleSections} />);
    expect(container.querySelector(`[${ITEM_ANCHOR_ATTR}="sample-project-copilot"]`)).not.toBeNull();
  });

  it('标题内是原生按钮，点击发出区块级目标', () => {
    const onLocate = vi.fn();
    render(<ResumeSections onLocate={onLocate} sections={sampleSections} />);
    fireEvent.click(screen.getByRole('button', { name: '定位到编辑器的「项目经历」' }));
    expect(onLocate).toHaveBeenCalledWith({ sectionId: 'sample-project' });
  });

  it('条目标题内是原生按钮，点击发出条目级目标', () => {
    const onLocate = vi.fn();
    render(<ResumeSections onLocate={onLocate} sections={sampleSections} />);
    fireEvent.click(screen.getByRole('button', { name: '定位到编辑器的「AI 创作副驾」' }));
    expect(onLocate).toHaveBeenCalledWith({ sectionId: 'sample-project', itemId: 'sample-project-copilot' });
  });

  it('没有 onLocate 时标题保持纯文本，布局与改动前一致', () => {
    render(<ResumeSections sections={sampleSections} />);
    expect(screen.queryByRole('button')).toBeNull();
  });

  it('summary 区块无条目锚点', () => {
    const { container } = render(<ResumeSections sections={sampleSections} />);
    const summary = container.querySelector('[data-resume-section="summary"]');
    expect(summary?.querySelectorAll(`[${ITEM_ANCHOR_ATTR}]`).length).toBe(0);
  });
});
