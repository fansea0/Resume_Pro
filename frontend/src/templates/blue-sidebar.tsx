import type { ReactNode } from 'react';
import {
  ResumeSections,
  type ContentSlotProps,
  type ItemSlotProps,
  type SectionSlotProps,
  type SkillsSlotProps,
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
import { SECTION_ANCHOR_ATTR, SectionLocateButton, type LocateHandler } from '../components/ResumeLocate';

/**
 * Blue Sidebar — single-column, ATS-clean Chinese resume with a vivid blue
 * accent. Inspired by a typical 互联网技术岗 求职 layout (white background,
 * bold black section titles, blue inline links, and a left-rule "callout"
 * sidebar used to highlight summary statements and core project highlights).
 *
 * Key visual choices:
 *  - Top "sidebar" header block: 求职意向 + 联系方式 with a left blue rule.
 *  - Section titles: bold black, ~1.05rem, no uppercase, with a subtle bottom
 *    rule that extends only as wide as the heading text.
 *  - Skills render as a bullet list where "熟悉：", "了解：" (or any leading
 *    label) are bolded; non-Chinese labels stay in the body text.
 *  - Inline links (email / GitHub / personal site) pick up the blue accent.
 *  - Compact spacing keeps the whole resume inside a single A4 page.
 */

function BlueSidebarSection({ children, className, section }: SectionSlotProps) {
  return (
    <section className={className} data-resume-section={section.type} {...{ [SECTION_ANCHOR_ATTR]: section.id }}>
      {children}
    </section>
  );
}

function BlueSidebarTitle({ children, className, onLocate, section }: TitleSlotProps) {
  return (
    <h2 className={className} data-resume-section-title>
      <span className="template-blue-sidebar__title-text"><SectionLocateButton label={section.title} onLocate={onLocate} sectionId={section.id}>{children}</SectionLocateButton></span>
    </h2>
  );
}

function BlueSidebarEntry({ children, className }: ItemSlotProps) {
  return (
    <article className={className} data-resume-entry>
      {children}
    </article>
  );
}

function BlueSidebarEntryHeading({ children, className }: ContentSlotProps) {
  return <div className={className}>{children}</div>;
}

function BlueSidebarMeta({ children, className }: ContentSlotProps) {
  return <div className={className}>{children}</div>;
}

function BlueSidebarRole({ children, className }: ContentSlotProps) {
  return <p className={className}>{children}</p>;
}

/**
 * Render a "X：Y" prefix where X is bolded. Falls back to the original text
 * when the bullet does not match the label pattern.
 */
function renderSkillWithLabel(text: string): ReactNode {
  const match = text.match(/^\s*([^：:\n]{1,8})\s*[：:]\s*(.*)$/);
  if (!match) return text;
  const label = (match[1] ?? '').trim();
  const rest = match[2];
  if (!label || !rest) return text;
  return (
    <>
      <strong className="template-blue-sidebar__skill-label">{label}：</strong>
      <span>{rest}</span>
    </>
  );
}

function BlueSidebarSkills({ className, section }: SkillsSlotProps) {
  return (
    <ul className={className} data-template-skills>
      {section.content.items.map((item: SkillItem) => (
        <li className="template-blue-sidebar__skill" key={item.id}>
          <span className="template-blue-sidebar__skill-name">{item.name}</span>
          {item.note && <span className="template-blue-sidebar__skill-note">：{item.note}</span>}
        </li>
      ))}
    </ul>
  );
}

function BlueSidebarHonours({ children, className }: ContentSlotProps) {
  return (
    <ul className={className} data-template-honours>
      {children}
    </ul>
  );
}

function BlueSidebarExperienceItem(_section: ResumeSection, item: ExperienceItem) {
  return (
    <BlueSidebarEntry className="template-blue-sidebar__entry" item={item} section={_section}>
      <BlueSidebarEntryHeading className="template-blue-sidebar__entry-heading">
        <h3 className="template-blue-sidebar__entry-title">
          {item.company}
          {item.role && (
            <span className="template-blue-sidebar__entry-role-inline"> · {item.role}</span>
          )}
        </h3>
        {(item.startDate || item.endDate) && (
          <time className="template-blue-sidebar__date">
            {item.startDate}
            {item.startDate && item.endDate ? ' – ' : ''}
            {item.endDate}
          </time>
        )}
      </BlueSidebarEntryHeading>
      {item.showLocation && item.location && (
        <BlueSidebarMeta className="template-blue-sidebar__location">{item.location}</BlueSidebarMeta>
      )}
      {item.showAchievements && item.achievements && (
        <p className="template-blue-sidebar__achievements">
          <strong>核心亮点：</strong>
          <span className="template-blue-sidebar__sidebar">
            {item.achievements}
          </span>
        </p>
      )}
      {item.highlightsMarkdown && (
        <div className="template-blue-sidebar__markdown">
          <ResumeMarkdown markdown={item.highlightsMarkdown} />
        </div>
      )}
    </BlueSidebarEntry>
  );
}

function BlueSidebarProjectItem(_section: ResumeSection, item: ProjectItem) {
  return (
    <BlueSidebarEntry className="template-blue-sidebar__entry" item={item} section={_section}>
      <BlueSidebarEntryHeading className="template-blue-sidebar__entry-heading">
        <h3 className="template-blue-sidebar__entry-title">{item.name}</h3>
        {(item.startDate || item.endDate) && (
          <time className="template-blue-sidebar__date">
            {item.startDate}
            {item.startDate && item.endDate ? ' – ' : ''}
            {item.endDate}
          </time>
        )}
      </BlueSidebarEntryHeading>
      {item.role && <BlueSidebarRole className="template-blue-sidebar__role">{item.role}</BlueSidebarRole>}
      {item.showIntroduction && item.introduction && (
        <p className="template-blue-sidebar__introduction">{item.introduction}</p>
      )}
      {item.showAchievements && item.achievements && (
        <p className="template-blue-sidebar__achievements">
          <strong>项目成绩：</strong>
          {item.achievements}
        </p>
      )}
      {item.highlightsMarkdown && (
        <div className="template-blue-sidebar__markdown">
          <ResumeMarkdown markdown={item.highlightsMarkdown} />
        </div>
      )}
    </BlueSidebarEntry>
  );
}

function BlueSidebarEducationItem(_section: ResumeSection, item: EducationItem) {
  const meta = [item.startDate && item.endDate ? `${item.startDate} – ${item.endDate}` : (item.startDate || item.endDate)]
    .filter(Boolean)
    .join('');
  return (
    <BlueSidebarEntry className="template-blue-sidebar__entry" item={item} section={_section}>
      <BlueSidebarEntryHeading className="template-blue-sidebar__entry-heading">
        <h3 className="template-blue-sidebar__entry-title">{item.school}</h3>
        {meta && <time className="template-blue-sidebar__date">{meta}</time>}
      </BlueSidebarEntryHeading>
      {[item.degree, item.college, item.major].filter(Boolean).length > 0 && (
        <BlueSidebarRole className="template-blue-sidebar__role">
          {[item.degree, item.college, item.major].filter(Boolean).join(' · ')}
        </BlueSidebarRole>
      )}
    </BlueSidebarEntry>
  );
}

function BlueSidebarCampusItem(_section: ResumeSection, item: CampusItem) {
  return (
    <BlueSidebarEntry className="template-blue-sidebar__entry" item={item} section={_section}>
      <BlueSidebarEntryHeading className="template-blue-sidebar__entry-heading">
        <h3 className="template-blue-sidebar__entry-title">{item.name}</h3>
        {item.role && <span className="template-blue-sidebar__role-inline">{item.role}</span>}
      </BlueSidebarEntryHeading>
      {item.highlightsMarkdown && (
        <div className="template-blue-sidebar__markdown">
          <ResumeMarkdown markdown={item.highlightsMarkdown} />
        </div>
      )}
    </BlueSidebarEntry>
  );
}

function BlueSidebarHeader({ document }: { document: ResumeDocument }) {
  const { profile } = document;
  const contactItems: Array<{ label: string; value: string; href?: string }> = [
    { label: '电话', value: profile.phone, href: profile.phone ? `tel:${profile.phone}` : undefined },
    { label: '邮箱', value: profile.email, href: profile.email ? `mailto:${profile.email}` : undefined },
    { label: '主页', value: profile.website, href: profile.website ? (/^https?:/i.test(profile.website) ? profile.website : `https://${profile.website}`) : undefined },
    { label: '所在地', value: profile.location },
  ].filter((item) => item.value);

  return (
    <header className="template-blue-sidebar__header">
      <div className="template-blue-sidebar__identity">
        <h1 className="template-blue-sidebar__name">{profile.fullName}</h1>
        {profile.headline && (
          <p className="template-blue-sidebar__headline">
            <span className="template-blue-sidebar__headline-label">求职意向</span>
            <span className="template-blue-sidebar__headline-sep">｜</span>
            <span className="template-blue-sidebar__headline-value">{profile.headline}</span>
          </p>
        )}
        {contactItems.length > 0 && (
          <p className="template-blue-sidebar__contact">
            {contactItems.map((item, index) => (
              <span key={item.label} className="template-blue-sidebar__contact-item">
                {item.href ? (
                  <a className="template-blue-sidebar__contact-value" href={item.href}>{item.value}</a>
                ) : (
                  <span className="template-blue-sidebar__contact-value">{item.value}</span>
                )}
                {index < contactItems.length - 1 && (
                  <span className="template-blue-sidebar__contact-sep" aria-hidden="true">·</span>
                )}
              </span>
            ))}
          </p>
        )}
      </div>
      <ResumeAvatar className="template-blue-sidebar__avatar" profile={profile} />
    </header>
  );
}

export function BlueSidebarTemplate({ document, onLocate }: { document: ResumeDocument; onLocate?: LocateHandler }) {
  const { sections } = document;

  const slots: Parameters<typeof ResumeSections>[0]['slots'] = {
    section: BlueSidebarSection,
    title: BlueSidebarTitle,
    entry: BlueSidebarEntry,
    entryHeading: BlueSidebarEntryHeading,
    meta: BlueSidebarMeta,
    role: BlueSidebarRole,
    skills: BlueSidebarSkills,
    honours: BlueSidebarHonours,
    item: (section, item) => {
      switch (section.type) {
        case 'work':
        case 'internship':
          return BlueSidebarExperienceItem(section, item as ExperienceItem);
        case 'project':
          return BlueSidebarProjectItem(section, item as ProjectItem);
        case 'education':
          return BlueSidebarEducationItem(section, item as EducationItem);
        case 'campus':
          return BlueSidebarCampusItem(section, item as CampusItem);
        default:
          return null;
      }
    },
  };

  return (
    <article className="resume-template template-blue-sidebar">
      <BlueSidebarHeader document={document} />
      <ResumeSections
        sections={sections}
        slots={slots}
        onLocate={onLocate}
        classNames={{
          section: 'template-blue-sidebar__section',
          title: 'template-blue-sidebar__section-title',
          entry: 'template-blue-sidebar__entry',
          entryHeading: 'template-blue-sidebar__entry-heading',
          meta: 'template-blue-sidebar__meta',
          role: 'template-blue-sidebar__role',
          summary: 'template-blue-sidebar__summary',
          markdown: 'template-blue-sidebar__markdown',
          skills: 'template-blue-sidebar__skills',
          skill: 'template-blue-sidebar__skill',
          campus: 'template-blue-sidebar__campus',
          honours: 'template-blue-sidebar__honours',
          honour: 'template-blue-sidebar__honour',
        }}
      />
    </article>
  );
}
