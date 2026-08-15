import { ResumeSections } from './ResumeSections';
import type { ResumeDocument } from '../types/resume';

export function BusinessTemplate({ document }: { document: ResumeDocument }) {
  const { profile, sections } = document;
  return <article className="resume-template template-business">
    <header className="template-business__header"><div><p className="template-business__eyebrow">CURRICULUM VITAE</p><h1>{profile.fullName}</h1><p>{profile.headline}</p></div><address>{[profile.email, profile.phone, profile.location, profile.website].filter(Boolean).map((item) => <span key={item}>{item}</span>)}</address></header>
    <ResumeSections sections={sections} slots={{ section: ({ children, className, section }) => <section className={section.type === 'summary' ? 'template-business__intro' : className} data-resume-section={section.type}>{children}</section>, honours: ({ children, className }) => <ul data-template-honours className={className} style={{ columns: 2, gap: '1.2rem', margin: 0 }}>{children}</ul> }} classNames={{ entry: 'template-business__entry', meta: 'template-business__period', role: 'template-business__company', summary: 'template-business__summary', skills: 'template-business__skills', honours: 'template-business__honours', honour: 'template-business__honour' }} />
  </article>;
}
