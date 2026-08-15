import { ResumeSections } from './ResumeSections';
import type { ResumeDocument } from '../types/resume';

export function MinimalTemplate({ document }: { document: ResumeDocument }) {
  const { profile, sections } = document;
  return <article className="resume-template template-minimal">
    <header className="template-minimal__header"><h1>{profile.fullName}</h1><p className="template-minimal__headline">{profile.headline}</p><p className="template-minimal__contact">{[profile.email, profile.phone, profile.location, profile.website].filter(Boolean).join(' · ')}</p></header>
    <ResumeSections sections={sections} classNames={{ entry: 'template-minimal__entry', summary: 'template-minimal__summary', skills: 'template-minimal__skills', honour: 'template-minimal__honour', honours: 'template-minimal__honours' }} />
  </article>;
}
