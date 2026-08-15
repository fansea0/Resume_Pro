import { ResumeSections } from './ResumeSections';
import type { ResumeDocument } from '../types/resume';

export function RedlineTemplate({ document }: { document: ResumeDocument }) {
  const { profile, sections } = document;
  const contact = [profile.phone, profile.email, profile.website].filter(Boolean).join('  |  ');
  const initial = profile.fullName.trim().slice(0, 1) || '简';
  return <article className="resume-template template-redline"><header className="template-redline__header"><div><h1>{profile.fullName}</h1>{contact && <p className="template-redline__contact">{contact}</p>}<p className="template-redline__headline">{profile.headline}</p></div><div aria-label="候选人头像占位" className="template-redline__avatar">{initial}</div></header><ResumeSections sections={sections} classNames={{ title: 'template-redline__section-title', entry: 'template-redline__entry', meta: 'template-redline__meta', role: 'template-redline__role', summary: 'template-redline__summary', skills: 'template-redline__skills', honours: 'template-redline__honours', honour: 'template-redline__honour' }} /></article>;
}
