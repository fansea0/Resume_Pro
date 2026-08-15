import { ResumeSections } from './ResumeSections';
import type { ResumeDocument } from '../types/resume';

export function AzureChronicleTemplate({ document }: { document: ResumeDocument }) {
  const { profile, sections } = document;
  const contact = [profile.phone, profile.email, profile.website].filter(Boolean).join('  |  ');
  const initial = profile.fullName.trim().slice(0, 1) || '简';
  return <article className="resume-template template-azure-chronicle"><header className="template-azure-chronicle__header"><div className="template-azure-chronicle__identity"><h1>{profile.fullName}</h1>{contact && <p>{contact}</p>}<p>{profile.headline}</p></div><div aria-label="候选人头像占位" className="template-azure-chronicle__avatar">{initial}</div></header><ResumeSections sections={sections} classNames={{ title: 'template-azure-chronicle__section-heading', entry: 'template-azure-chronicle__entry', meta: 'template-azure-chronicle__entry-meta', role: 'template-azure-chronicle__role', summary: 'template-azure-chronicle__summary', skills: 'template-azure-chronicle__skills', honours: 'template-azure-chronicle__honours', honour: 'template-azure-chronicle__honour' }} /></article>;
}
