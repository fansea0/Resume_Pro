export type TemplateId = 'minimal' | 'business' | 'creative';

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
