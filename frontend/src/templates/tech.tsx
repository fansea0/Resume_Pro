import { ResumeSections } from './ResumeSections';
import type { ResumeDocument } from '../types/resume';

export function TechTemplate({ document }: { document: ResumeDocument }) {
  const { profile, sections } = document;
  const contact = [profile.email, profile.phone, profile.location, profile.website].filter(Boolean);
  return <article className="resume-template template-tech"><header className="template-tech__header"><p className="template-tech__eyebrow">CAREER SIGNAL</p><h1>{profile.fullName}</h1><p className="template-tech__headline">{profile.headline}</p>{contact.length > 0 && <p className="template-tech__contact">{contact.join(' / ')}</p>}</header><main className="template-tech__content"><ResumeSections sections={sections} classNames={{ entry: 'template-tech__entry', meta: 'template-tech__date', role: 'template-tech__company', summary: 'template-tech__summary', skills: 'template-tech__skills', honours: 'template-tech__honours', honour: 'template-tech__honour' }} /></main></article>;
}
