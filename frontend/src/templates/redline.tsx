import { ResumeSections } from './ResumeSections';
import type { ResumeDocument } from '../types/resume';
import { ResumeAvatar } from './ResumeAvatar';
import type { LocateHandler } from '../components/ResumeLocate';

export function RedlineTemplate({ document, onLocate }: { document: ResumeDocument; onLocate?: LocateHandler }) {
  const { profile, sections } = document;
  const contact = [profile.phone, profile.email, profile.website].filter(Boolean).join('  |  ');
  return <article className="resume-template template-redline"><header className="template-redline__header"><div><h1>{profile.fullName}</h1>{contact && <p className="template-redline__contact">{contact}</p>}<p className="template-redline__headline">{profile.headline}</p></div><ResumeAvatar className="template-redline__avatar" profile={profile} /></header><ResumeSections sections={sections} onLocate={onLocate} slots={{ honours: ({ children, className }) => <ul data-template-honours className={className} style={{ display: 'flex', flexWrap: 'wrap', gap: '0.35rem 0.7rem', listStyle: 'none', padding: 0 }}>{children}</ul> }} classNames={{ title: 'template-redline__section-title', entry: 'template-redline__entry', entryHeading: 'template-redline__entry-heading', meta: 'template-redline__meta', role: 'template-redline__role', summary: 'template-redline__summary', skills: 'template-redline__skills', honours: 'template-redline__honours', honour: 'template-redline__honour' }} /></article>;
}
