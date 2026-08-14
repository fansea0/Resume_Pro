import type { ReactNode } from 'react';
import type { ResumeDocument } from '../types/resume';

interface TemplateProps { document: ResumeDocument; }

const icons: Record<string, ReactNode> = {
  summary: <path d="M7 3.5h7v10H9l-3 3v-10A3 3 0 0 1 9 3.5Zm2 3h3m-3 2.5h3" />,
  experience: <path d="M5 6.5h10v8H5Zm3 0V5h4v1.5M8 10h4" />,
  project: <path d="M5 5.5h10v10H5Zm3 3h4m-4 3h4" />,
  education: <path d="m4 8 6-3 6 3-6 3Zm2.2 1.8V13c1.8 1.3 5.8 1.3 7.6 0V9.8M16 8v4" />,
  skills: <path d="M10 4.5a5.5 5.5 0 1 0 5.5 5.5M10 7v3h3M4.7 15.3 3.5 16.5" />,
};

function SectionHeading({ icon, children }: { icon: keyof typeof icons; children: string }) {
  return <h2 className="template-azure-chronicle__section-heading"><span aria-hidden="true" className="template-azure-chronicle__icon"><svg viewBox="0 0 20 20">{icons[icon]}</svg></span><span>{children}</span></h2>;
}

function EntryMeta({ startDate, endDate, location }: { startDate: string; endDate: string; location?: string }) {
  return <span className="template-azure-chronicle__entry-meta"><span>{[startDate, endDate].filter(Boolean).join(' — ')}</span>{location && <span>{location}</span>}</span>;
}

export function AzureChronicleTemplate({ document }: TemplateProps) {
  const { profile, experiences, projects, education, skills } = document;
  const contact = [profile.phone, profile.email, profile.website].filter(Boolean).join('  |  ');
  const initial = profile.fullName.trim().slice(0, 1) || '简';

  return <article className="resume-template template-azure-chronicle">
    <header className="template-azure-chronicle__header">
      <div className="template-azure-chronicle__identity"><h1>{profile.fullName}</h1>{contact && <p>{contact}</p>}<p>{profile.headline}</p></div>
      <div aria-label="候选人头像占位" className="template-azure-chronicle__avatar">{initial}</div>
    </header>
    {profile.summary && <section><SectionHeading icon="summary">个人总结</SectionHeading><p className="template-azure-chronicle__summary">{profile.summary}</p></section>}
    {experiences.length > 0 && <section><SectionHeading icon="experience">工作经历</SectionHeading>{experiences.map((item) => <div className="template-azure-chronicle__entry" key={item.id}><div className="template-azure-chronicle__entry-heading"><h3>{item.company}</h3><EntryMeta endDate={item.endDate} location={profile.location} startDate={item.startDate} /></div><p className="template-azure-chronicle__role">{item.role}</p>{item.highlights.length > 0 && <ul>{item.highlights.map((highlight, index) => <li key={index}>{highlight}</li>)}</ul>}</div>)}</section>}
    {projects.length > 0 && <section><SectionHeading icon="project">项目经历</SectionHeading>{projects.map((item) => <div className="template-azure-chronicle__entry" key={item.id}><div className="template-azure-chronicle__entry-heading"><h3>{item.name}</h3><EntryMeta endDate={item.endDate} startDate={item.startDate} /></div><p className="template-azure-chronicle__role">{[item.role, item.link].filter(Boolean).join(' · ')}</p>{item.description && <p>{item.description}</p>}{item.highlights.length > 0 && <ul>{item.highlights.map((highlight, index) => <li key={index}>{highlight}</li>)}</ul>}</div>)}</section>}
    {education.length > 0 && <section><SectionHeading icon="education">教育经历</SectionHeading>{education.map((item) => <div className="template-azure-chronicle__entry" key={item.id}><div className="template-azure-chronicle__entry-heading"><h3>{item.school}</h3><EntryMeta endDate={item.endDate} startDate={item.startDate} /></div><p className="template-azure-chronicle__role">{item.degree}{item.field && ` · ${item.field}`}</p></div>)}</section>}
    {skills.length > 0 && <section><SectionHeading icon="skills">专业技能</SectionHeading><dl className="template-azure-chronicle__skills">{skills.map((skill) => <div key={skill.id}><dt>{skill.name}</dt>{skill.level && <dd>{skill.level}</dd>}</div>)}</dl></section>}
  </article>;
}
