import { ResumeSections, type ItemSlotProps } from './ResumeSections';
import { ResumeMarkdown } from '../components/ResumeMarkdown';
import type { ResumeDocument } from '../types/resume';

export function TechTemplate({ document }: { document: ResumeDocument }) {
  const { profile, sections } = document;
  const contact = [profile.email, profile.phone, profile.location, profile.website].filter(Boolean);
  const Entry = ({ children, className, section }: ItemSlotProps) => <article className={section.type === 'education' ? 'template-tech__education' : className}>{children}</article>;
  return <article className="resume-template template-tech"><header className="template-tech__header"><p className="template-tech__eyebrow">CAREER SIGNAL</p><h1>{profile.fullName}</h1><p className="template-tech__headline">{profile.headline}</p>{contact.length > 0 && <p className="template-tech__contact">{contact.join(' / ')}</p>}</header><main className="template-tech__content"><ResumeSections sections={sections} slots={{ entry: Entry, item: (section, item) => {
    if (section.type === 'education') { const education = item as { id: string; school: string; degree: string; college: string; major: string; startDate: string; endDate: string }; return <div className="template-tech__education" key={education.id}><strong>{education.school}</strong><span>{[education.degree, education.college, education.major].filter(Boolean).join(' · ')}</span><time>{education.startDate} — {education.endDate}</time></div>; }
    const value = item as { id: string; name?: string; company?: string; role?: string; startDate?: string; endDate?: string; highlightsMarkdown?: string }; return <div className="template-tech__entry" key={value.id}><p className="template-tech__date">{value.startDate} — {value.endDate}</p><h3>{value.name ?? value.company}</h3>{value.role && <p className="template-tech__company">{value.role}</p>}{value.highlightsMarkdown && <ResumeMarkdown markdown={value.highlightsMarkdown} />}</div>;
  }, honours: ({ children, className }) => <ul data-template-honours className={className} style={{ display: 'flex', flexWrap: 'wrap', gap: '0.45rem', listStyle: 'none', padding: 0 }}>{children}</ul> }} classNames={{ entry: 'template-tech__entry', meta: 'template-tech__date', role: 'template-tech__company', summary: 'template-tech__summary', skills: 'template-tech__skills', honours: 'template-tech__honours', honour: 'template-tech__honour' }} /></main></article>;
}
