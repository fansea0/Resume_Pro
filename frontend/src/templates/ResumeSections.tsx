import type { ComponentType, ReactNode } from 'react';
import { ResumeMarkdown, safeUrl } from '../components/ResumeMarkdown';
import type {
  CampusItem,
  ExperienceItem,
  HonourItem,
  ProjectItem,
  ResumeSection,
} from '../types/resume';

type ClassNameKey =
  | 'section'
  | 'title'
  | 'entry'
  | 'entryHeading'
  | 'meta'
  | 'role'
  | 'summary'
  | 'markdown'
  | 'location'
  | 'link'
  | 'introduction'
  | 'achievements'
  | 'skills'
  | 'skill'
  | 'campus'
  | 'honours'
  | 'honour';

export interface SectionSlotProps {
  children: ReactNode;
  className?: string;
  section: ResumeSection;
}

export interface TitleSlotProps {
  children: ReactNode;
  className?: string;
  section: ResumeSection;
}

export interface ItemSlotProps {
  children: ReactNode;
  className?: string;
  section: ResumeSection;
  item: unknown;
}

export interface ContentSlotProps {
  children: ReactNode;
  className?: string;
}

export interface HonoursSlotProps extends ContentSlotProps {
  section: Extract<ResumeSection, { type: 'honours' }>;
}

export interface SkillsSlotProps extends ContentSlotProps {
  section: Extract<ResumeSection, { type: 'skills' }>;
}

/**
 * Wrapper slots let every template keep ownership of its visual hierarchy while
 * this component preserves the resume document's semantic content and order.
 */
export interface ResumeSectionsSlots {
  section: ComponentType<SectionSlotProps>;
  title: ComponentType<TitleSlotProps>;
  entry: ComponentType<ItemSlotProps>;
  entryHeading: ComponentType<ContentSlotProps>;
  meta: ComponentType<ContentSlotProps>;
  role: ComponentType<ContentSlotProps>;
  honours: ComponentType<HonoursSlotProps>;
  skills: ComponentType<SkillsSlotProps>;
  item: (section: ResumeSection, item: ExperienceItem | ProjectItem | CampusItem | Extract<ResumeSection, { type: 'education' }>['content']['items'][number]) => ReactNode;
}

export interface ResumeSectionsProps {
  sections: ResumeSection[];
  classNames?: Partial<Record<ClassNameKey, string>>;
  slots?: Partial<ResumeSectionsSlots>;
}

function joinClassNames(...classNames: Array<string | undefined>): string | undefined {
  const value = classNames.filter(Boolean).join(' ');
  return value || undefined;
}

function assertNever(value: never): never {
  throw new Error(`Unsupported resume section: ${JSON.stringify(value)}`);
}

function DefaultSection({ children, className, section }: SectionSlotProps) {
  return <section className={className} data-resume-section={section.type}>{children}</section>;
}

function DefaultTitle({ children, className }: TitleSlotProps) {
  return <h2 className={className} data-resume-section-title>{children}</h2>;
}

function DefaultEntry({ children, className }: ItemSlotProps) {
  return <article className={className}>{children}</article>;
}

function DefaultMeta({ children, className }: ContentSlotProps) {
  return <div className={className}>{children}</div>;
}

function DefaultRole({ children, className }: ContentSlotProps) {
  return <p className={className}>{children}</p>;
}

function DefaultHonours({ children, className }: HonoursSlotProps) {
  return <ul className={className}>{children}</ul>;
}

function DefaultSkills({ children, className }: SkillsSlotProps) {
  return <div className={className}>{children}</div>;
}

function DefaultEntryHeading({ children, className }: ContentSlotProps) {
  return <div className={className}>{children}</div>;
}

function Metadata({
  startDate,
  endDate,
  location,
  classNames,
  Meta,
}: {
  startDate: string;
  endDate: string;
  location?: string;
  classNames: ResumeSectionsProps['classNames'];
  Meta: ComponentType<ContentSlotProps>;
}) {
  return (
    <Meta className={classNames?.meta}>
      {(startDate || endDate) && <time>{startDate}{startDate && endDate ? ' — ' : ''}{endDate}</time>}
      {location && <span className={classNames?.location}>{location}</span>}
    </Meta>
  );
}

function ExperienceEntry({
  item,
  section,
  classNames,
  Entry,
  Meta,
  Role,
  EntryHeading,
}: {
  item: ExperienceItem;
  section: Extract<ResumeSection, { type: 'work' | 'internship' }>;
  classNames: ResumeSectionsProps['classNames'];
  Entry: ComponentType<ItemSlotProps>;
  Meta: ComponentType<ContentSlotProps>;
  Role: ComponentType<ContentSlotProps>;
  EntryHeading: ComponentType<ContentSlotProps>;
}) {
  return (
    <Entry className={classNames?.entry} item={item} section={section}>
      <EntryHeading className={classNames?.entryHeading}><h3>{item.company}</h3><Metadata
        Meta={Meta}
        classNames={classNames}
        endDate={item.endDate}
        location={item.showLocation ? item.location : undefined}
        startDate={item.startDate}
      /></EntryHeading>
      {item.role && <Role className={classNames?.role}>{item.role}</Role>}
      {item.showAchievements && item.achievements && <p className={classNames?.achievements}>{item.achievements}</p>}
      {item.highlightsMarkdown && <div className={classNames?.markdown}><ResumeMarkdown markdown={item.highlightsMarkdown} /></div>}
    </Entry>
  );
}

function ProjectEntry({
  item,
  section,
  classNames,
  Entry,
  Meta,
  Role,
  EntryHeading,
}: {
  item: ProjectItem;
  section: Extract<ResumeSection, { type: 'project' }>;
  classNames: ResumeSectionsProps['classNames'];
  Entry: ComponentType<ItemSlotProps>;
  Meta: ComponentType<ContentSlotProps>;
  Role: ComponentType<ContentSlotProps>;
  EntryHeading: ComponentType<ContentSlotProps>;
}) {
  const safeLink = safeUrl(item.link);

  return (
    <Entry className={classNames?.entry} item={item} section={section}>
      <EntryHeading className={classNames?.entryHeading}><h3>{item.name}</h3><Metadata
        Meta={Meta}
        classNames={classNames}
        endDate={item.endDate}
        location={item.showLocation ? item.location : undefined}
        startDate={item.startDate}
      /></EntryHeading>
      {item.role && <Role className={classNames?.role}>{item.role}</Role>}
      {item.showLink && safeLink && <a className={classNames?.link} href={safeLink}>{item.link}</a>}
      {item.showIntroduction && item.introduction && <p className={classNames?.introduction}>{item.introduction}</p>}
      {item.showAchievements && item.achievements && <p className={classNames?.achievements}>{item.achievements}</p>}
      {item.highlightsMarkdown && <div className={classNames?.markdown}><ResumeMarkdown markdown={item.highlightsMarkdown} /></div>}
    </Entry>
  );
}

function CampusEntry({
  item,
  section,
  classNames,
  Entry,
  Role,
}: {
  item: CampusItem;
  section: Extract<ResumeSection, { type: 'campus' }>;
  classNames: ResumeSectionsProps['classNames'];
  Entry: ComponentType<ItemSlotProps>;
  Role: ComponentType<ContentSlotProps>;
}) {
  return (
    <Entry className={joinClassNames(classNames?.entry, classNames?.campus)} item={item} section={section}>
      <h3>{item.name}</h3>
      {item.role && <Role className={classNames?.role}>{item.role}</Role>}
      {item.highlightsMarkdown && <div className={classNames?.markdown}><ResumeMarkdown markdown={item.highlightsMarkdown} /></div>}
    </Entry>
  );
}

/** Renders visible resume sections strictly in their persisted array order. */
export function ResumeSections({ sections, classNames, slots }: ResumeSectionsProps) {
  const Section = slots?.section ?? DefaultSection;
  const Title = slots?.title ?? DefaultTitle;
  const Entry = slots?.entry ?? DefaultEntry;
  const EntryHeading = slots?.entryHeading ?? DefaultEntryHeading;
  const Meta = slots?.meta ?? DefaultMeta;
  const Role = slots?.role ?? DefaultRole;
  const Honours = slots?.honours ?? DefaultHonours;
  const Skills = slots?.skills ?? DefaultSkills;

  return (
    <>
      {sections.filter((section) => section.visible).map((section) => {
        let content: ReactNode;

        switch (section.type) {
          case 'summary':
            content = section.content.markdown && (
              <div className={joinClassNames(classNames?.summary, classNames?.markdown)}>
                <ResumeMarkdown markdown={section.content.markdown} />
              </div>
            );
            break;
          case 'work':
          case 'internship':
            content = section.content.items.map((item) => (
              slots?.item ? slots.item(section, item) : <ExperienceEntry
                Entry={Entry}
                Meta={Meta}
                Role={Role}
                EntryHeading={EntryHeading}
                classNames={classNames}
                item={item}
                key={item.id}
                section={section}
              />
            ));
            break;
          case 'project':
            content = section.content.items.map((item) => (
              slots?.item ? slots.item(section, item) : <ProjectEntry
                Entry={Entry}
                Meta={Meta}
                Role={Role}
                EntryHeading={EntryHeading}
                classNames={classNames}
                item={item}
                key={item.id}
                section={section}
              />
            ));
            break;
          case 'education':
            content = section.content.items.map((item) => (
              slots?.item ? slots.item(section, item) : <Entry className={classNames?.entry} item={item} key={item.id} section={section}>
                <EntryHeading className={classNames?.entryHeading}><h3>{item.school}</h3><Metadata Meta={Meta} classNames={classNames} endDate={item.endDate} startDate={item.startDate} /></EntryHeading>
                {[item.degree, item.college, item.major].filter(Boolean).length > 0 && (
                  <Role className={classNames?.role}>{[item.degree, item.college, item.major].filter(Boolean).join(' · ')}</Role>
                )}
              </Entry>
            ));
            break;
          case 'skills':
            content = <Skills className={classNames?.skills} section={section}>{section.content.items.map((item) => (
              <span className={classNames?.skill} key={item.id}>{item.name}{item.note && <small>{item.note}</small>}</span>
            ))}</Skills>;
            break;
          case 'campus':
            content = section.content.items.map((item) => (
              slots?.item ? slots.item(section, item) : <CampusEntry Entry={Entry} Role={Role} classNames={classNames} item={item} key={item.id} section={section} />
            ));
            break;
          case 'honours':
            content = (
              <Honours className={classNames?.honours} section={section}>
                {section.content.items.map((item: HonourItem) => (
                  <li className={classNames?.honour} key={item.id}>{item.name}</li>
                ))}
              </Honours>
            );
            break;
          default:
            content = assertNever(section);
        }

        return (
          <Section className={classNames?.section} key={section.id} section={section}>
            <Title className={classNames?.title} section={section}>{section.title}</Title>
            {content}
          </Section>
        );
      })}
    </>
  );
}
