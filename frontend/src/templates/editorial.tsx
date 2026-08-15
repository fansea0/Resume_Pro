import { ResumeSections } from './ResumeSections';
import type { ResumeDocument } from '../types/resume';

export function EditorialTemplate({ document }: { document: ResumeDocument }) {
  const { profile, sections } = document;
  const contact = [profile.email, profile.phone, profile.location, profile.website].filter(Boolean);
  return <article className="resume-template template-editorial"><header className="template-editorial__header"><p>PROFILE</p><h1>{profile.fullName}</h1><div><span>{profile.headline}</span><span>{contact.join(' · ')}</span></div></header><div className="template-editorial__layout"><aside className="template-editorial__sidebar" /><main className="template-editorial__content"><ResumeSections sections={sections} classNames={{ entry: 'template-editorial__entry', meta: 'template-editorial__date', role: 'template-editorial__role', summary: 'template-editorial__summary', skills: 'template-editorial__skills', honours: 'template-editorial__honours', honour: 'template-editorial__honour' }} /></main></div></article>;
}
