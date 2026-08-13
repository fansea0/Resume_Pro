export type TemplateId = 'minimal' | 'business' | 'creative' | 'tech' | 'editorial' | 'redline';

export interface ResumeProfile {
  fullName: string;
  headline: string;
  email: string;
  phone: string;
  location: string;
  website: string;
  summary: string;
}

export interface WorkExperience {
  id: string;
  company: string;
  role: string;
  startDate: string;
  endDate: string;
  highlights: string[];
}

export interface Project {
  id: string;
  name: string;
  role: string;
  link: string;
  startDate: string;
  endDate: string;
  description: string;
  highlights: string[];
}

export interface Education {
  id: string;
  school: string;
  degree: string;
  field: string;
  startDate: string;
  endDate: string;
}

export interface Skill {
  id: string;
  name: string;
  level: string;
}

export interface ResumeDocument {
  title: string;
  templateId: TemplateId;
  profile: ResumeProfile;
  experiences: WorkExperience[];
  projects: Project[];
  education: Education[];
  skills: Skill[];
}

export function createEmptyResumeDocument(): ResumeDocument {
  return {
    title: '我的简历',
    templateId: 'minimal',
    profile: {
      fullName: '姓名',
      headline: '互联网从业者',
      email: '',
      phone: '',
      location: '',
      website: '',
      summary: '',
    },
    experiences: [],
    projects: [],
    education: [],
    skills: [],
  };
}

export function createSampleResumeDocument(): ResumeDocument {
  return {
    title: '林知夏｜产品经理简历',
    templateId: 'redline',
    profile: {
      fullName: '林知夏',
      headline: '互联网产品经理｜增长与 AI 应用',
      email: 'zhixia.lin@example.com',
      phone: '138-0000-2026',
      location: '上海',
      website: 'zhixia-lin.dev',
      summary:
        '3 年互联网产品经验，聚焦增长与 AI 应用。擅长将用户洞察转化为可落地的产品方案，主导从 0 到 1 的功能上线与持续迭代，曾推动核心转化率提升 18%。',
    },
    experiences: [
      {
        id: 'sample-experience-nova',
        company: '新芽科技',
        role: '产品经理',
        startDate: '2023.07',
        endDate: '至今',
        highlights: [
          '负责智能内容助手的产品规划与迭代，梳理创作、审核、分发全链路，月活用户增长至 12 万。',
          '通过访谈、漏斗分析定位新手流失点，设计引导与模板推荐机制，首周留存率提升 18%。',
          '协同算法、研发与运营团队上线 20 余项需求，建立指标看板与复盘机制，需求按期交付率达 95%。',
        ],
      },
      {
        id: 'sample-experience-pixel',
        company: '像素互动',
        role: '产品运营',
        startDate: '2021.07',
        endDate: '2023.06',
        highlights: [
          '负责社区创作者成长体系，策划等级、任务与激励机制，优质内容发布量提升 32%。',
          '联合数据团队搭建活动效果评估模型，缩短复盘周期并沉淀可复用运营策略。',
        ],
      },
    ],
    projects: [
      {
        id: 'sample-project-copilot',
        name: 'AI 创作副驾',
        role: '产品负责人',
        link: '',
        startDate: '2024.03',
        endDate: '2024.09',
        description: '面向内容创作者的 AI 灵感生成与改写工具，覆盖选题、提纲和初稿三个核心场景。',
        highlights: [
          '完成竞品研究、用户访谈与 MVP 定义，制定按场景分层的提示词与交互方案。',
          '上线后通过 A/B 测试优化结果页，工具周使用频次提升 26%，满意度达 4.7/5。',
        ],
      },
      {
        id: 'sample-project-growth',
        name: '会员增长实验平台',
        role: '核心成员',
        link: '',
        startDate: '2023.10',
        endDate: '2024.02',
        description: '统一管理会员权益、活动配置与实验分流的内部增长工具。',
        highlights: [
          '设计实验配置流程和核心数据看板，支持运营同学自主发起活动，配置时长从 2 天缩短至 2 小时。',
        ],
      },
    ],
    education: [
      {
        id: 'sample-education-fudan',
        school: '复旦大学',
        degree: '本科',
        field: '信息管理与信息系统',
        startDate: '2017.09',
        endDate: '2021.06',
      },
    ],
    skills: [
      { id: 'sample-skill-product', name: '产品规划', level: '熟练' },
      { id: 'sample-skill-data', name: '数据分析', level: '熟练' },
      { id: 'sample-skill-figma', name: 'Figma', level: '熟练' },
      { id: 'sample-skill-sql', name: 'SQL', level: '熟练' },
      { id: 'sample-skill-english', name: '英语', level: 'CET-6' },
    ],
  };
}
