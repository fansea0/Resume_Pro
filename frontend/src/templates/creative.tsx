import { ResumeSections } from './ResumeSections';
import type { ResumeDocument } from '../types/resume';

export function CreativeTemplate({ document }: { document: ResumeDocument }) {
  const { profile, sections } = document;
  const visible = sections.filter((section) => section.visible);
  return <article className="resume-template template-creative">
    <aside className="template-creative__sidebar"><p className="template-creative__label">PORTFOLIO</p><h1>{profile.fullName}</h1><p className="template-creative__headline">{profile.headline}</p><section><h2>联系</h2>{[profile.email, profile.phone, profile.location, profile.website].filter(Boolean).map((item) => <p key={item}>{item}</p>)}</section><ResumeSections sections={visible.slice(0, 2)} slots={{ honours: ({ children, className }) => <ul data-template-honours className={className} style={{ listStyle: 'none', padding: 0, margin: 0 }}>{children}</ul> }} /></aside>
    <main className="template-creative__content"><ResumeSections sections={visible.slice(2)} slots={{ honours: ({ children, className }) => <ul data-template-honours className={className} style={{ display: 'flex', flexWrap: 'wrap', gap: '0.5rem', listStyle: 'none', padding: 0 }}>{children}</ul> }} classNames={{ entry: 'template-creative__entry', meta: 'template-creative__date', role: 'template-creative__role', summary: 'template-creative__summary', skills: 'template-creative__skills', honours: 'template-creative__honours', honour: 'template-creative__honour' }} /></main>
  </article>;
}
