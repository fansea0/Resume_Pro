import type { ReactNode } from 'react';
import { ResumeSections, type ContentSlotProps, type SkillsSlotProps, type TitleSlotProps } from './ResumeSections';
import type {
  CampusItem,
  ExperienceItem,
  ProjectItem,
  ResumeDocument,
  ResumeSection,
} from '../types/resume';
import { ResumeAvatar } from './ResumeAvatar';
import { ResumeMarkdown } from '../components/ResumeMarkdown';

/**
 * Mint Canvas — thin border, mint header block, badge section titles and
 * inline keyword chips. Aimed at fresh graduates and technical roles.
 */

const sectionIcons: Record<string, string> = {
  summary: 'M5 4h10v9H9l-3 3V7a3 3 0 0 1 3-3h6',
  work: 'M5 7h10v8H5zm3 0V5h4v2m-4 4h4',
  internship: 'M5 7h10v8H5zm3 0V5h4v2m-4 4h4',
  project: 'M5 5h10v10H5zm3 3h4m-4 3h4',
  education: 'm4 8 6-3 6 3-6 3zm2 2v3c2 1 4 1 6 0v-3',
  skills: 'M10 4.5a5.5 5.5 0 1 0 5.5 5.5M10 7v3h3',
  campus: 'm10 4 1.6 3.3 3.6.5-2.6 2.5.6 3.6-3.2-1.7-3.2 1.7.6-3.6L4.8 7.8l3.6-.5z',
  honours: 'm10 4 1.3 3 3.2.3-2.4 2.1.7 3.2-2.8-1.6-2.8 1.6.7-3.2-2.4-2.1 3.2-.3z',
};

function MintTitle({ children, className, section }: TitleSlotProps) {
  const iconPath = sectionIcons[section.type];
  return (
    <h2 className={className} data-resume-section-title>
      <span aria-hidden="true" className="template-mint-canvas__badge">
        <svg viewBox="0 0 20 20"><path d={iconPath} /></svg>
      </span>
      <span className="template-mint-canvas__title-text">{children}</span>
    </h2>
  );
}

function MintSkills({ className, section }: SkillsSlotProps) {
  return (
    <div className={className}>
      {section.content.items.map((item) => (
        <span className="template-mint-canvas__skill" key={item.id}>
          {item.name}
          {item.note && <small>{item.note}</small>}
        </span>
      ))}
    </div>
  );
}

function MintHonours({ children, className }: ContentSlotProps) {
  return (
    <ul data-template-honours className={className} style={{ display: 'flex', flexWrap: 'wrap', gap: '0.35rem 0.7rem', listStyle: 'none', padding: 0 }}>
      {children}
    </ul>
  );
}

/**
 * Render a highlightsMarkdown block while turning the first "keyword：value"
 * chunk of each top-level bullet into a mint chip. Falls back to plain
 * markdown when the bullet does not start with that pattern.
 */
function renderHighlightsWithChips(markdown: string): ReactNode {
  if (!markdown) return null;
  const lines = markdown.split(/\r?\n/);
  const blocks: Array<{ type: 'bullet' | 'ordered' | 'text'; depth: number; lines: string[] }> = [];
  let current: { type: 'bullet' | 'ordered' | 'text'; depth: number; lines: string[] } | null = null;

  const bulletRe = /^(\s*)([-*+])\s+(.*)$/;
  const orderedRe = /^(\s*)(\d+)\.\s+(.*)$/;

  for (const raw of lines) {
    const line = raw.replace(/\s+$/, '');
    if (!line.trim()) {
      if (current) {
        blocks.push(current);
        current = null;
      }
      blocks.push({ type: 'text', depth: 0, lines: [''] });
      continue;
    }
    const bulletMatch = line.match(bulletRe);
    const orderedMatch = line.match(orderedRe);
    if (bulletMatch) {
      const [, indent, , text] = bulletMatch;
      if (!current || current.type !== 'bullet' || current.depth !== indent.length) {
        if (current) blocks.push(current);
        current = { type: 'bullet', depth: indent.length, lines: [text] };
      } else {
        current.lines.push(text);
      }
    } else if (orderedMatch) {
      const [, indent, , text] = orderedMatch;
      if (!current || current.type !== 'ordered' || current.depth !== indent.length) {
        if (current) blocks.push(current);
        current = { type: 'ordered', depth: indent.length, lines: [text] };
      } else {
        current.lines.push(text);
      }
    } else if (current && (current.type === 'bullet' || current.type === 'ordered') && /^\s+/.test(raw)) {
      current.lines.push(line.trim());
    } else {
      if (current) {
        blocks.push(current);
        current = null;
      }
      blocks.push({ type: 'text', depth: 0, lines: [line] });
    }
  }
  if (current) blocks.push(current);

  return (
    <>
      {blocks.map((block, blockIndex) => {
        if (block.type === 'text') {
          if (!block.lines[0]) return <div key={`gap-${blockIndex}`} className="template-mint-canvas__gap" />;
          return (
            <p key={`p-${blockIndex}`} className="template-mint-canvas__para">
              {block.lines.map((line, lineIndex) => (
                <span key={lineIndex}>{renderInlineChips(line)}</span>
              ))}
            </p>
          );
        }
        const Tag = block.type === 'bullet' ? 'ul' : 'ol';
        return (
          <Tag
            key={`${block.type}-${blockIndex}`}
            className={`template-mint-canvas__list template-mint-canvas__list--${block.type}`}
            style={block.depth ? { marginLeft: `${block.depth * 0.6}rem` } : undefined}
          >
            {block.lines.map((item, itemIndex) => (
              <li key={itemIndex}>{renderInlineChips(item)}</li>
            ))}
          </Tag>
        );
      })}
    </>
  );
}

/** Render inline content, splitting the first "xxx：yyy" or "xxx: yyy" into a chip. */
function renderInlineChips(text: string): ReactNode {
  // Match either **markdown-bold** keyword or a plain keyword, followed by
  // a Chinese (：) or ASCII (:) colon, then the rest of the line.
  const match = text.match(/^\s*(?:\*\*([^*\n]{1,16})\*\*|([^：:\n]{1,16}))\s*[：:]\s*(.*)$/);
  if (!match) return text;
  const keyword = (match[1] ?? match[2] ?? '').trim();
  const rest = match[3];
  // Only treat as chip when there's actual follow-up text and the keyword
  // looks like a label rather than a URL or numeric.
  if (!keyword || !rest || /^[\d./-]+$/.test(keyword)) return text;
  return (
    <>
      <span className="template-mint-canvas__chip">{keyword}</span>
      <span className="template-mint-canvas__chip-sep">：</span>
      <span>{rest}</span>
    </>
  );
}

function MintExperienceItem(_section: ResumeSection, item: ExperienceItem) {
  return (
    <article className="template-mint-canvas__entry" data-resume-entry={item.id}>
      <div className="template-mint-canvas__entry-heading">
        <h3>{item.company}</h3>
        {(item.startDate || item.endDate) && (
          <time className="template-mint-canvas__entry-date">
            {item.startDate}
            {item.startDate && item.endDate ? ' – ' : ''}
            {item.endDate}
          </time>
        )}
      </div>
      {item.role && <p className="template-mint-canvas__role">{item.role}</p>}
      {item.showAchievements && item.achievements && (
        <p className="template-mint-canvas__achievement">{item.achievements}</p>
      )}
      {item.highlightsMarkdown && (
        <div className="template-mint-canvas__markdown">{renderHighlightsWithChips(item.highlightsMarkdown)}</div>
      )}
    </article>
  );
}

function MintProjectItem(_section: ResumeSection, item: ProjectItem) {
  return (
    <article className="template-mint-canvas__entry" data-resume-entry={item.id}>
      <div className="template-mint-canvas__entry-heading">
        <h3>{item.name}</h3>
        {(item.startDate || item.endDate) && (
          <time className="template-mint-canvas__entry-date">
            {item.startDate}
            {item.startDate && item.endDate ? ' – ' : ''}
            {item.endDate}
          </time>
        )}
      </div>
      <div className="template-mint-canvas__entry-sub">
        {item.role && <span className="template-mint-canvas__role">{item.role}</span>}
        {item.role && item.introduction && <span className="template-mint-canvas__sep"> · </span>}
        {item.showIntroduction && item.introduction && <span>{item.introduction}</span>}
      </div>
      {item.showAchievements && item.achievements && (
        <p className="template-mint-canvas__achievement">{item.achievements}</p>
      )}
      {item.highlightsMarkdown && (
        <div className="template-mint-canvas__markdown">{renderHighlightsWithChips(item.highlightsMarkdown)}</div>
      )}
    </article>
  );
}

function MintCampusItem(_section: ResumeSection, item: CampusItem) {
  return (
    <article className="template-mint-canvas__entry" data-resume-entry={item.id}>
      <div className="template-mint-canvas__entry-heading">
        <h3>{item.name}</h3>
        {item.role && <span className="template-mint-canvas__role">{item.role}</span>}
      </div>
      {item.highlightsMarkdown && (
        <div className="template-mint-canvas__markdown">
          <ResumeMarkdown markdown={item.highlightsMarkdown} />
        </div>
      )}
    </article>
  );
}

export function MintCanvasTemplate({ document }: { document: ResumeDocument }) {
  const { profile, sections } = document;
  const headerContact = [profile.phone, profile.email, profile.website].filter(Boolean).join('  |  ');

  // Custom item renderer to attach chip-styled highlight rendering.
  const slots: Parameters<typeof ResumeSections>[0]['slots'] = {
    title: MintTitle,
    skills: MintSkills,
    honours: MintHonours,
    item: (section, item) => {
      if (section.type === 'work' || section.type === 'internship') {
        return MintExperienceItem(section, item as ExperienceItem);
      }
      if (section.type === 'project') {
        return MintProjectItem(section, item as ProjectItem);
      }
      if (section.type === 'campus') {
        return MintCampusItem(section, item as CampusItem);
      }
      if (section.type === 'education') {
        const edu = item as { id: string; school: string; startDate: string; endDate: string; degree: string; college: string; major: string };
        return (
          <article className="template-mint-canvas__entry" data-resume-entry={edu.id}>
            <div className="template-mint-canvas__entry-heading">
              <h3>{edu.school}</h3>
              {(edu.startDate || edu.endDate) && (
                <time className="template-mint-canvas__entry-date">
                  {edu.startDate}
                  {edu.startDate && edu.endDate ? ' – ' : ''}
                  {edu.endDate}
                </time>
              )}
            </div>
            {[edu.degree, edu.college, edu.major].filter(Boolean).length > 0 && (
              <p className="template-mint-canvas__role">{[edu.degree, edu.college, edu.major].filter(Boolean).join(' · ')}</p>
            )}
          </article>
        );
      }
      return null;
    },
  };

  return (
    <article className="resume-template template-mint-canvas">
      <header className="template-mint-canvas__header">
        <div className="template-mint-canvas__identity">
          <h1>{profile.fullName}</h1>
          {headerContact && <p className="template-mint-canvas__contact">{headerContact}</p>}
          {profile.headline && <p className="template-mint-canvas__headline">{profile.headline}</p>}
        </div>
        <ResumeAvatar className="template-mint-canvas__avatar" profile={profile} />
      </header>
      <div className="template-mint-canvas__body">
        <ResumeSections
          sections={sections}
          slots={slots}
          classNames={{
            section: 'template-mint-canvas__section',
            title: 'template-mint-canvas__section-title',
            summary: 'template-mint-canvas__summary',
            skills: 'template-mint-canvas__skills',
            honour: 'template-mint-canvas__honour',
          }}
        />
      </div>
    </article>
  );
}
