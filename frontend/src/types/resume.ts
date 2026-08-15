export type TemplateId =
  | 'minimal'
  | 'business'
  | 'creative'
  | 'tech'
  | 'editorial'
  | 'redline'
  | 'violet'
  | 'azure-chronicle';

export interface ResumeProfile {
  fullName: string;
  headline: string;
  email: string;
  phone: string;
  location: string;
  website: string;
}

export type ResumeSectionType =
  | 'summary'
  | 'work'
  | 'internship'
  | 'project'
  | 'education'
  | 'skills'
  | 'campus'
  | 'honours';

interface ResumeSectionBase<TType extends ResumeSectionType, TContent> {
  id: string;
  type: TType;
  title: string;
  visible: boolean;
  content: TContent;
}

export interface SummarySection
  extends ResumeSectionBase<'summary', { markdown: string }> {}

export interface ExperienceItem {
  id: string;
  company: string;
  role: string;
  startDate: string;
  endDate: string;
  location: string;
  achievements: string;
  highlightsMarkdown: string;
  showLocation: boolean;
  showAchievements: boolean;
}

export interface ExperienceSection<TType extends 'work' | 'internship'>
  extends ResumeSectionBase<TType, { items: ExperienceItem[] }> {}

export type WorkSection = ExperienceSection<'work'>;
export type InternshipSection = ExperienceSection<'internship'>;

export interface ProjectItem {
  id: string;
  name: string;
  role: string;
  startDate: string;
  endDate: string;
  location: string;
  link: string;
  introduction: string;
  achievements: string;
  highlightsMarkdown: string;
  showLocation: boolean;
  showLink: boolean;
  showIntroduction: boolean;
  showAchievements: boolean;
}

export interface ProjectSection
  extends ResumeSectionBase<'project', { items: ProjectItem[] }> {}

export interface EducationItem {
  id: string;
  school: string;
  startDate: string;
  endDate: string;
  degree: string;
  college: string;
  major: string;
}

export interface EducationSection
  extends ResumeSectionBase<'education', { items: EducationItem[] }> {}

export interface SkillItem {
  id: string;
  name: string;
  note: string;
}

export interface SkillsSection
  extends ResumeSectionBase<'skills', { items: SkillItem[] }> {}

export interface CampusItem {
  id: string;
  name: string;
  role: string;
  highlightsMarkdown: string;
}

export interface CampusSection
  extends ResumeSectionBase<'campus', { items: CampusItem[] }> {}

export interface HonourItem {
  id: string;
  name: string;
}

export interface HonoursSection
  extends ResumeSectionBase<'honours', { items: HonourItem[] }> {}

export type ResumeSection =
  | SummarySection
  | WorkSection
  | InternshipSection
  | ProjectSection
  | EducationSection
  | SkillsSection
  | CampusSection
  | HonoursSection;

export interface ResumeDocument {
  schemaVersion: 2;
  title: string;
  templateId: TemplateId;
  profile: ResumeProfile;
  sections: ResumeSection[];
}

export function createSummarySection(id = 'summary'): SummarySection {
  return { id, type: 'summary', title: '个人总结', visible: true, content: { markdown: '' } };
}

function createExperienceSection<TType extends 'work' | 'internship'>(
  id: string,
  type: TType,
  title: string,
): ExperienceSection<TType> {
  return { id, type, title, visible: true, content: { items: [] } };
}

export function createWorkSection(id = 'work'): WorkSection {
  return createExperienceSection(id, 'work', '工作经历');
}

export function createInternshipSection(id = 'internship'): InternshipSection {
  return createExperienceSection(id, 'internship', '实习经历');
}

export function createProjectSection(id = 'project'): ProjectSection {
  return { id, type: 'project', title: '项目经历', visible: true, content: { items: [] } };
}

export function createEducationSection(id = 'education'): EducationSection {
  return { id, type: 'education', title: '教育经历', visible: true, content: { items: [] } };
}

export function createSkillsSection(id = 'skills'): SkillsSection {
  return { id, type: 'skills', title: '专业技能', visible: true, content: { items: [] } };
}

export function createCampusSection(id = 'campus'): CampusSection {
  return { id, type: 'campus', title: '校园经历', visible: true, content: { items: [] } };
}

export function createHonoursSection(id = 'honours'): HonoursSection {
  return { id, type: 'honours', title: '荣誉奖项', visible: true, content: { items: [] } };
}

export function createEmptyResumeDocument(): ResumeDocument {
  return {
    schemaVersion: 2,
    title: '我的简历',
    templateId: 'minimal',
    profile: {
      fullName: '姓名',
      headline: '互联网从业者',
      email: '',
      phone: '',
      location: '',
      website: '',
    },
    sections: [],
  };
}

export function createSampleResumeDocument(): ResumeDocument {
  const summary = createSummarySection('sample-summary');
  summary.content.markdown =
    '3 年互联网产品经验，聚焦增长与 AI 应用。擅长将用户洞察转化为可落地的产品方案，主导从 0 到 1 的功能上线与持续迭代，曾推动核心转化率提升 18%。';

  const work = createWorkSection('sample-work');
  work.content.items = [
    {
      id: 'sample-work-nova',
      company: '新芽科技',
      role: '产品经理',
      startDate: '2023.07',
      endDate: '至今',
      location: '上海',
      achievements: '月活用户增长至 12 万',
      highlightsMarkdown:
        '- 负责智能内容助手的产品规划与迭代，梳理创作、审核、分发全链路。\n- 通过访谈、漏斗分析定位新手流失点，设计引导与模板推荐机制，首周留存率提升 18%。',
      showLocation: true,
      showAchievements: true,
    },
  ];

  const internship = createInternshipSection('sample-internship');
  internship.content.items = [
    {
      id: 'sample-internship-pixel',
      company: '像素互动',
      role: '产品运营实习生',
      startDate: '2021.07',
      endDate: '2023.06',
      location: '上海',
      achievements: '优质内容发布量提升 32%',
      highlightsMarkdown: '- 负责社区创作者成长体系，策划等级、任务与激励机制。',
      showLocation: true,
      showAchievements: true,
    },
  ];

  const project = createProjectSection('sample-project');
  project.content.items = [
    {
      id: 'sample-project-copilot',
      name: 'AI 创作副驾',
      role: '产品负责人',
      startDate: '2024.03',
      endDate: '2024.09',
      location: '上海',
      link: '',
      introduction: '面向内容创作者的 AI 灵感生成与改写工具，覆盖选题、提纲和初稿三个核心场景。',
      achievements: '工具周使用频次提升 26%，满意度达 4.7/5。',
      highlightsMarkdown: '- 完成竞品研究、用户访谈与 MVP 定义，制定按场景分层的提示词与交互方案。',
      showLocation: true,
      showLink: false,
      showIntroduction: true,
      showAchievements: true,
    },
  ];

  const education = createEducationSection('sample-education');
  education.content.items = [
    {
      id: 'sample-education-fudan',
      school: '复旦大学',
      startDate: '2017.09',
      endDate: '2021.06',
      degree: '本科',
      college: '管理学院',
      major: '信息管理与信息系统',
    },
  ];

  const skills = createSkillsSection('sample-skills');
  skills.content.items = [
    { id: 'sample-skill-product', name: '产品规划', note: '熟练' },
    { id: 'sample-skill-data', name: '数据分析', note: '熟练' },
    { id: 'sample-skill-figma', name: 'Figma', note: '熟练' },
  ];

  const campus = createCampusSection('sample-campus');
  campus.content.items = [
    {
      id: 'sample-campus-club',
      name: '复旦大学产品俱乐部',
      role: '活动负责人',
      highlightsMarkdown: '- 策划并组织 6 场产品分享活动，累计服务 300 余名学生。',
    },
  ];

  const honours = createHonoursSection('sample-honours');
  honours.content.items = [
    { id: 'sample-honour-scholarship', name: '校级一等奖学金' },
    { id: 'sample-honour-competition', name: '全国大学生创新创业大赛省级银奖' },
  ];

  return {
    schemaVersion: 2,
    title: '林知夏｜产品经理简历',
    templateId: 'redline',
    profile: {
      fullName: '林知夏',
      headline: '互联网产品经理｜增长与 AI 应用',
      email: 'zhixia.lin@example.com',
      phone: '138-0000-2026',
      location: '上海',
      website: 'zhixia-lin.dev',
    },
    sections: [summary, work, internship, project, education, skills, campus, honours],
  };
}
