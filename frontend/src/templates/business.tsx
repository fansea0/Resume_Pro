import { ResumeSections } from './ResumeSections';
import { ResumeMarkdown } from '../components/ResumeMarkdown';
import type { ExperienceItem } from '../types/resume';
import type { ResumeDocument } from '../types/resume';

export function BusinessTemplate({ document }: { document: ResumeDocument }) {
  const { profile, sections } = document;
  return <article className="resume-template template-business">
    <header className="template-business__header"><div><p className="template-business__eyebrow">CURRICULUM VITAE</p><h1>{profile.fullName}</h1><p>{profile.headline}</p></div><address>{[profile.email, profile.phone, profile.location, profile.website].filter(Boolean).map((item) => <span key={item}>{item}</span>)}</address></header>
    <ResumeSections sections={sections} slots={{ section: ({ children, className, section }) => <section className={section.type === 'summary' ? 'template-business__intro' : className} data-resume-section={section.type}>{children}</section>, item: (section, item) => {
      if (section.type === 'work' || section.type === 'internship') { const experience = item as ExperienceItem; return <div className="template-business__entry" key={experience.id}><div className="template-business__period">{experience.startDate}<br />{experience.endDate}</div><div><h3>{experience.role}</h3><p className="template-business__company">{experience.company}</p>{experience.showLocation && experience.location && <p>{experience.location}</p>}{experience.showAchievements && experience.achievements && <p>{experience.achievements}</p>}{experience.highlightsMarkdown && <ResumeMarkdown markdown={experience.highlightsMarkdown} />}</div></div>; }
      const value = item as { id: string; name?: string; school?: string; company?: string; role?: string; startDate?: string; endDate?: string; highlightsMarkdown?: string };
      return <div className="template-business__entry" key={value.id}><div className="template-business__period">{value.startDate}<br />{value.endDate}</div><div><h3>{value.name ?? value.school ?? value.company}</h3>{value.role && <p>{value.role}</p>}{value.highlightsMarkdown && <ResumeMarkdown markdown={value.highlightsMarkdown} />}</div></div>;
    }, honours: ({ children, className }) => <ul data-template-honours className={className} style={{ columns: 2, gap: '1.2rem', margin: 0 }}>{children}</ul> }} classNames={{ entry: 'template-business__entry', meta: 'template-business__period', role: 'template-business__company', summary: 'template-business__summary', skills: 'template-business__skills', honours: 'template-business__honours', honour: 'template-business__honour' }} />
  </article>;
}
