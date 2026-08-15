import { ResumeSections, type SkillsSlotProps, type TitleSlotProps } from './ResumeSections';
import type { ResumeDocument } from '../types/resume';

const icons: Record<string, string> = { summary: 'M5 4h10v9H9l-3 3V7a3 3 0 0 1 3-3h6', work: 'M5 7h10v8H5zm3 0V5h4v2m-4 4h4', internship: 'M5 7h10v8H5zm3 0V5h4v2m-4 4h4', project: 'M5 5h10v10H5zm3 3h4m-4 3h4', education: 'm4 8 6-3 6 3-6 3zm2 2v3c2 1 4 1 6 0v-3', skills: 'M10 4.5a5.5 5.5 0 1 0 5.5 5.5M10 7v3h3', campus: 'm10 4 1.6 3.3 3.6.5-2.6 2.5.6 3.6-3.2-1.7-3.2 1.7.6-3.6L4.8 7.8l3.6-.5z', honours: 'm10 4 1.3 3 3.2.3-2.4 2.1.7 3.2-2.8-1.6-2.8 1.6.7-3.2-2.4-2.1 3.2-.3z' };

function AzureTitle({ children, className, section }: TitleSlotProps) {
  return <h2 className={className} data-resume-section-title><span aria-hidden="true" className="template-azure-chronicle__icon"><svg viewBox="0 0 20 20"><path d={icons[section.type]} /></svg></span><span>{children}</span></h2>;
}

function AzureSkills({ className, section }: SkillsSlotProps) {
  return <dl className={className}>{section.content.items.map((item) => <div key={item.id}><dt>{item.name}</dt>{item.note && <dd>{item.note}</dd>}</div>)}</dl>;
}

export function AzureChronicleTemplate({ document }: { document: ResumeDocument }) {
  const { profile, sections } = document;
  const contact = [profile.phone, profile.email, profile.website].filter(Boolean).join('  |  ');
  const initial = profile.fullName.trim().slice(0, 1) || '简';
  return <article className="resume-template template-azure-chronicle"><header className="template-azure-chronicle__header"><div className="template-azure-chronicle__identity"><h1>{profile.fullName}</h1>{contact && <p>{contact}</p>}<p>{profile.headline}</p></div><div aria-label="候选人头像占位" className="template-azure-chronicle__avatar">{initial}</div></header><ResumeSections sections={sections} slots={{ title: AzureTitle, skills: AzureSkills, honours: ({ children, className }) => <ul data-template-honours className={className} style={{ columns: 2, gap: '1rem', listStyle: 'none', margin: 0, padding: 0 }}>{children}</ul> }} classNames={{ title: 'template-azure-chronicle__section-heading', entry: 'template-azure-chronicle__entry', entryHeading: 'template-azure-chronicle__entry-heading', meta: 'template-azure-chronicle__entry-meta', role: 'template-azure-chronicle__role', summary: 'template-azure-chronicle__summary', skills: 'template-azure-chronicle__skills', honours: 'template-azure-chronicle__honours', honour: 'template-azure-chronicle__honour' }} /></article>;
}
