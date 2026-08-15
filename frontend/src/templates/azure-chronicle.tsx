import { ResumeSections, type SkillsSlotProps, type TitleSlotProps } from './ResumeSections';
import type { ResumeDocument } from '../types/resume';

const icons: Record<string, string> = { summary: '●', work: '▣', internship: '▣', project: '◇', education: '◆', skills: '◌', campus: '★', honours: '✦' };

function AzureTitle({ children, className, section }: TitleSlotProps) {
  return <h2 className={className} data-resume-section-title><span aria-hidden="true" className="template-azure-chronicle__icon">{icons[section.type]}</span><span>{children}</span></h2>;
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
