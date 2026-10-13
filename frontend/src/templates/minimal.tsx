import { ResumeSections } from './ResumeSections';
import type { ResumeDocument } from '../types/resume';
import { ResumeAvatar } from './ResumeAvatar';
import type { LocateHandler } from '../components/ResumeLocate';

export function MinimalTemplate({ document, onLocate }: { document: ResumeDocument; onLocate?: LocateHandler }) {
  const { profile, sections } = document;
  return <article className="resume-template template-minimal">
    <header className="template-minimal__header"><div className="template-minimal__identity"><h1>{profile.fullName}</h1><p className="template-minimal__headline">{profile.headline}</p><p className="template-minimal__contact">{[profile.email, profile.phone, profile.location, profile.website].filter(Boolean).join(' · ')}</p></div><ResumeAvatar className="template-minimal__avatar" profile={profile} /></header>
    <ResumeSections sections={sections} onLocate={onLocate} slots={{ honours: ({ children, className }) => <ul data-template-honours className={className} style={{ display: 'flex', flexWrap: 'wrap', gap: '0.35rem 0.8rem', listStyle: 'none', padding: 0 }}>{children}</ul> }} classNames={{ entry: 'template-minimal__entry', summary: 'template-minimal__summary', skills: 'template-minimal__skills', honour: 'template-minimal__honour', honours: 'template-minimal__honours' }} />
  </article>;
}
