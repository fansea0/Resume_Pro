import { ResumeSections } from './ResumeSections';
import type { ResumeDocument } from '../types/resume';
import { ResumeAvatar } from './ResumeAvatar';
import type { LocateHandler } from '../components/ResumeLocate';

export function VioletTemplate({ document, onLocate }: { document: ResumeDocument; onLocate?: LocateHandler }) {
  const { profile, sections } = document;
  const contact = [profile.phone, profile.email, profile.website].filter(Boolean).join('  |  ');
  return <article className="resume-template template-violet"><header className="template-violet__header"><div className="template-violet__identity"><h1>{profile.fullName}</h1>{contact && <p>{contact}</p>}<p>{profile.headline}</p></div><ResumeAvatar className="template-violet__avatar" profile={profile} /></header><ResumeSections sections={sections} onLocate={onLocate} slots={{ honours: ({ children, className }) => <ul data-template-honours className={className} style={{ display: 'flex', flexWrap: 'wrap', gap: '0.35rem 0.7rem', listStyle: 'none', padding: 0 }}>{children}</ul> }} classNames={{ title: 'template-violet__section-heading', entry: 'template-violet__entry', entryHeading: 'template-violet__entry-top', meta: 'template-violet__entry-meta', role: 'template-violet__role', summary: 'template-violet__summary', skills: 'template-violet__skills', honours: 'template-violet__honours', honour: 'template-violet__honour' }} /></article>;
}
