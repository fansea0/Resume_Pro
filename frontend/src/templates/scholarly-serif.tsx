import type { ReactNode } from 'react';
import {
  ResumeSections,
  type ContentSlotProps,
  type ItemSlotProps,
  type SectionSlotProps,
  type TitleSlotProps,
} from './ResumeSections';
import type {
  CampusItem,
  EducationItem,
  ExperienceItem,
  ProjectItem,
  ResumeDocument,
  ResumeSection,
  SkillItem,
} from '../types/resume';
import { ResumeAvatar } from './ResumeAvatar';
import { ResumeMarkdown } from '../components/ResumeMarkdown';
import { SECTION_ANCHOR_ATTR, EntryLocateButton, ITEM_ANCHOR_ATTR, SectionLocateButton, type LocateHandler } from '../components/ResumeLocate';

/**
 * Scholarly Serif — Times-family single-column resume.
 *
 * Visual reference: classic Word/LaTeX academic resume.
 * - Centered italic serif name; single-line bold-label contact row underneath.
 * - All-caps bold section titles with a 1.5px black rule below.
 * - Company / school on the left of each entry, dates flush right on the same row.
 * - Italic role / position underneath; body content uses filled-disc bullets.
 * - Pure black on white, no accent colour, high information density.
 */

function ScholarlyHeader({ document }: { document: ResumeDocument }) {
  const { profile } = document;
  const contactBits: Array<{ label: string; value: string }> = [
    { label: 'TEL', value: profile.phone },
    { label: 'E-Mail', value: profile.email },
  ];
  if (profile.location) contactBits.push({ label: 'Location', value: profile.location });
  if (profile.website) contactBits.push({ label: 'Website', value: profile.website });
  const filtered = contactBits.filter((bit) => bit.value);
  return (
    <header className="template-scholarly-serif__header">
      <ResumeAvatar className="template-scholarly-serif__avatar" profile={profile} />
      <h1 className="template-scholarly-serif__name">{profile.fullName}</h1>
      {filtered.length > 0 && (
        <p className="template-scholarly-serif__contact">
          {filtered.map((bit, index) => (
            <span key={bit.label} className="template-scholarly-serif__contact-item">
              <strong>{bit.label}:</strong>
              <span> {bit.value}</span>
              {index < filtered.length - 1 && <span className="template-scholarly-serif__contact-sep">  ·  </span>}
            </span>
          ))}
        </p>
      )}
      {profile.headline && <p className="template-scholarly-serif__headline">{profile.headline}</p>}
    </header>
  );
}

function ScholarlyTitle({ children, className, onLocate, section }: TitleSlotProps) {
  return (
    <h2 className={className} data-resume-section-title>
      <span className="template-scholarly-serif__title-text"><SectionLocateButton label={section.title} onLocate={onLocate} sectionId={section.id}>{children}</SectionLocateButton></span>
    </h2>
  );
}

function ScholarlySection({ children, className, section }: SectionSlotProps) {
  return (
    <section className={className} data-resume-section={section.type} {...{ [SECTION_ANCHOR_ATTR]: section.id }}>
      {children}
    </section>
  );
}

function dateRange(start: string, end: string): ReactNode {
  if (!start && !end) return null;
  return (
    <time className="template-scholarly-serif__date">
      {start}
      {start && end ? ' – ' : ''}
      {end}
    </time>
  );
}

function ScholarlyEntry({ children, className, item }: ItemSlotProps) {
  return (
    <article className={className} data-resume-entry {...(item.id ? { [ITEM_ANCHOR_ATTR]: item.id } : {})}>
      {children}
    </article>
  );
}

function ScholarlyEntryHeading({ children, className }: ContentSlotProps) {
  return <div className={className}>{children}</div>;
}

function ScholarlyMeta({ children, className }: ContentSlotProps) {
  return <div className={className}>{children}</div>;
}

function ScholarlyRole({ children, className }: ContentSlotProps) {
  return <p className={className}>{children}</p>;
}

function ScholarlySummary({ children, className }: ContentSlotProps) {
  return <div className={className}>{children}</div>;
}

function ScholarlyMarkdown({ children, className }: ContentSlotProps) {
  return <div className={className}>{children}</div>;
}

function ScholarlyExperienceItem(section: ResumeSection, item: ExperienceItem, onLocate?: LocateHandler) {
  return (
    <ScholarlyEntry className="template-scholarly-serif__entry" item={item} section={section}>
      <ScholarlyEntryHeading className="template-scholarly-serif__entry-heading">
        <h3 className="template-scholarly-serif__entry-title"><EntryLocateButton itemId={item.id} label={item.company} onLocate={onLocate} sectionId={section.id}>{item.company}</EntryLocateButton></h3>
        {dateRange(item.startDate, item.endDate)}
      </ScholarlyEntryHeading>
      {item.role && <ScholarlyRole className="template-scholarly-serif__role">{item.role}</ScholarlyRole>}
      {item.showLocation && item.location && (
        <ScholarlyMeta className="template-scholarly-serif__location">{item.location}</ScholarlyMeta>
      )}
      {item.showAchievements && item.achievements && (
        <p className="template-scholarly-serif__achievements">{item.achievements}</p>
      )}
      {item.highlightsMarkdown && (
        <ScholarlyMarkdown className="template-scholarly-serif__markdown">
          <ResumeMarkdown markdown={item.highlightsMarkdown} />
        </ScholarlyMarkdown>
      )}
    </ScholarlyEntry>
  );
}

function ScholarlyProjectItem(section: ResumeSection, item: ProjectItem, onLocate?: LocateHandler) {
  return (
    <ScholarlyEntry className="template-scholarly-serif__entry" item={item} section={section}>
      <ScholarlyEntryHeading className="template-scholarly-serif__entry-heading">
        <h3 className="template-scholarly-serif__entry-title"><EntryLocateButton itemId={item.id} label={item.name} onLocate={onLocate} sectionId={section.id}>{item.name}</EntryLocateButton></h3>
        {dateRange(item.startDate, item.endDate)}
      </ScholarlyEntryHeading>
      {item.role && <ScholarlyRole className="template-scholarly-serif__role">{item.role}</ScholarlyRole>}
      {item.showLocation && item.location && (
        <ScholarlyMeta className="template-scholarly-serif__location">{item.location}</ScholarlyMeta>
      )}
      {item.showIntroduction && item.introduction && (
        <p className="template-scholarly-serif__introduction">{item.introduction}</p>
      )}
      {item.showAchievements && item.achievements && (
        <p className="template-scholarly-serif__achievements">{item.achievements}</p>
      )}
      {item.highlightsMarkdown && (
        <ScholarlyMarkdown className="template-scholarly-serif__markdown">
          <ResumeMarkdown markdown={item.highlightsMarkdown} />
        </ScholarlyMarkdown>
      )}
    </ScholarlyEntry>
  );
}

function ScholarlyEducationItem(section: ResumeSection, item: EducationItem, onLocate?: LocateHandler) {
  return (
    <ScholarlyEntry className="template-scholarly-serif__entry" item={item} section={section}>
      <ScholarlyEntryHeading className="template-scholarly-serif__entry-heading">
        <h3 className="template-scholarly-serif__entry-title"><EntryLocateButton itemId={item.id} label={item.school} onLocate={onLocate} sectionId={section.id}>{item.school}</EntryLocateButton></h3>
        {dateRange(item.startDate, item.endDate)}
      </ScholarlyEntryHeading>
      {[item.degree, item.college, item.major].filter(Boolean).length > 0 && (
        <ScholarlyRole className="template-scholarly-serif__role">
          {[item.degree, item.college, item.major].filter(Boolean).join(' · ')}
        </ScholarlyRole>
      )}
    </ScholarlyEntry>
  );
}

function ScholarlyCampusItem(section: ResumeSection, item: CampusItem, onLocate?: LocateHandler) {
  return (
    <ScholarlyEntry className="template-scholarly-serif__entry" item={item} section={section}>
      <ScholarlyEntryHeading className="template-scholarly-serif__entry-heading">
        <h3 className="template-scholarly-serif__entry-title"><EntryLocateButton itemId={item.id} label={item.name} onLocate={onLocate} sectionId={section.id}>{item.name}</EntryLocateButton></h3>
      </ScholarlyEntryHeading>
      {item.role && <ScholarlyRole className="template-scholarly-serif__role">{item.role}</ScholarlyRole>}
      {item.highlightsMarkdown && (
        <ScholarlyMarkdown className="template-scholarly-serif__markdown">
          <ResumeMarkdown markdown={item.highlightsMarkdown} />
        </ScholarlyMarkdown>
      )}
    </ScholarlyEntry>
  );
}

function ScholarlySkills({ className, section }: { className?: string; section: Extract<ResumeSection, { type: 'skills' }> }) {
  return (
    <ul className={className} data-template-skills>
      {section.content.items.map((item: SkillItem) => (
        <li className="template-scholarly-serif__skill" key={item.id}>
          <span className="template-scholarly-serif__skill-name">{item.name}</span>
          {item.note && <span className="template-scholarly-serif__skill-note"> — {item.note}</span>}
        </li>
      ))}
    </ul>
  );
}

function ScholarlyHonours({ children, className }: ContentSlotProps) {
  return (
    <ul className={className} data-template-honours>
      {children}
    </ul>
  );
}

export function ScholarlySerifTemplate({ document, onLocate }: { document: ResumeDocument; onLocate?: LocateHandler }) {
  const { sections } = document;

  const slots: Parameters<typeof ResumeSections>[0]['slots'] = {
    section: ScholarlySection,
    title: ScholarlyTitle,
    entry: ScholarlyEntry,
    entryHeading: ScholarlyEntryHeading,
    meta: ScholarlyMeta,
    role: ScholarlyRole,
    honours: ScholarlyHonours,
    skills: ScholarlySkills,
    item: (section, item, onLocate) => {
      switch (section.type) {
        case 'work':
        case 'internship':
          return ScholarlyExperienceItem(section, item as ExperienceItem, onLocate);
        case 'project':
          return ScholarlyProjectItem(section, item as ProjectItem, onLocate);
        case 'education':
          return ScholarlyEducationItem(section, item as EducationItem, onLocate);
        case 'campus':
          return ScholarlyCampusItem(section, item as CampusItem, onLocate);
        default:
          return null;
      }
    },
  };

  return (
    <article className="resume-template template-scholarly-serif">
      <ScholarlyHeader document={document} />
      <ResumeSections
        sections={sections}
        slots={slots}
        onLocate={onLocate}
        classNames={{
          section: 'template-scholarly-serif__section',
          title: 'template-scholarly-serif__section-title',
          entry: 'template-scholarly-serif__entry',
          entryHeading: 'template-scholarly-serif__entry-heading',
          meta: 'template-scholarly-serif__meta',
          role: 'template-scholarly-serif__role',
          summary: 'template-scholarly-serif__summary',
          markdown: 'template-scholarly-serif__markdown',
          skills: 'template-scholarly-serif__skills',
          skill: 'template-scholarly-serif__skill',
          campus: 'template-scholarly-serif__campus',
          honours: 'template-scholarly-serif__honours',
          honour: 'template-scholarly-serif__honour',
        }}
      />
    </article>
  );
}
