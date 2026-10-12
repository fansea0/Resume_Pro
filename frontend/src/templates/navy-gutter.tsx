import {
  ResumeSections,
  type ContentSlotProps,
  type HonoursSlotProps,
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
import { ResumeMarkdown, safeUrl } from '../components/ResumeMarkdown';

/**
 * Navy Gutter — 技术岗单栏模板，沿用「左侧深蓝标签栏 + 右侧浅色分区」的
 * 版式语言：章节标题变成贴在分区左上角的深蓝色块，首个分区用一条深蓝细线
 * 横向延伸，技术栈以 chip 呈现并对核心项做高亮。
 *
 * 关键视觉决定：
 *  - 姓名占据最大字号并压在左侧，求职意向右对齐，形成与参考图一致的对角张力。
 *  - 每个分区是一块浅蓝灰底 + 细描边的「分区卡片」，标题色块压在卡片左上角。
 *  - 经历 / 项目标题行左公司、右日期，用「｜」分隔公司与角色，与参考图一致。
 *  - 项目简介放进内嵌浅色面板并配深蓝左边线，承接参考图的项目子模块。
 *  - 技能 chip 默认描边，标记为精通 / 核心 / 主力的项以及每组首项填充实心深蓝。
 */

const CORE_SKILL_PATTERN = /精通|核心|主力|擅长|重点/;

function NavyGutterSection({ children, className, section }: SectionSlotProps) {
  return (
    <section
      className={`${className ?? ''} template-navy-gutter__section--${section.type}`.trim()}
      data-resume-section={section.type}
    >
      {children}
    </section>
  );
}

function NavyGutterTitle({ children, className, section }: TitleSlotProps) {
  return (
    <h2
      className={`${className ?? ''} template-navy-gutter__section-title--${section.type}`.trim()}
      data-resume-section-title
    >
      <span className="template-navy-gutter__label">{children}</span>
    </h2>
  );
}

function NavyGutterEntry({ children, className }: ItemSlotProps) {
  return (
    <article className={className} data-resume-entry>
      {children}
    </article>
  );
}

function NavyGutterEntryHeading({ children, className }: ContentSlotProps) {
  return <div className={className}>{children}</div>;
}

function NavyGutterMeta({ children, className }: ContentSlotProps) {
  return <div className={className}>{children}</div>;
}

function NavyGutterRole({ children, className }: ContentSlotProps) {
  return <p className={className}>{children}</p>;
}

function NavyGutterRange(startDate: string, endDate: string) {
  if (!startDate && !endDate) return null;
  return `${startDate}${startDate && endDate ? ' – ' : ''}${endDate}`;
}

function NavyGutterSkills({ className, section }: SkillsSlotProps) {
  return (
    <ul className={className} data-template-skills>
      {section.content.items.map((item: SkillItem, index: number) => {
        const isCore = index === 0 || CORE_SKILL_PATTERN.test(item.note ?? '');
        // 单行内联渲染：跨行的 JSX 会在两个 span 之间插入空白文本节点，把 chip
        // 挤成两行（实测高度 25px → 45px），所以把空格放进 note 自己前面。
        return (
          <li
            className={`template-navy-gutter__skill${isCore ? ' template-navy-gutter__skill--core' : ''}`}
            key={item.id}
          >
            {item.name}
            {item.note ? <span className="template-navy-gutter__skill-note"> {item.note}</span> : null}
          </li>
        );
      })}
    </ul>
  );
}

function NavyGutterHonours({ children, className }: HonoursSlotProps) {
  return (
    <ul className={className} data-template-honours>
      {children}
    </ul>
  );
}

/** 紧凑 chip 行：用于项目技术栈这类以斜杠分隔的短标签。 */
function NavyGutterTechStack({ className, techStack }: { className?: string; techStack: string }) {
  const tags = techStack
    .split(/[\/|,，、]/)
    .map((tag) => tag.trim())
    .filter(Boolean);
  if (tags.length === 0) return null;
  return (
    <ul className={className} data-template-tech-stack>
      {tags.map((tag) => (
        <li className="template-navy-gutter__tech-tag" key={tag}>
          {tag}
        </li>
      ))}
    </ul>
  );
}

function NavyGutterExperienceItem(section: ResumeSection, item: ExperienceItem) {
  const range = NavyGutterRange(item.startDate, item.endDate);
  const location = item.showLocation ? item.location : '';
  return (
    <NavyGutterEntry className="template-navy-gutter__entry" item={item} section={section}>
      <NavyGutterEntryHeading className="template-navy-gutter__entry-heading">
        <h3 className="template-navy-gutter__entry-title">
          {item.company}
          {item.role && (
            <span className="template-navy-gutter__entry-role">
              <span className="template-navy-gutter__entry-sep" aria-hidden="true">|</span>
              {item.role}
            </span>
          )}
        </h3>
        {(range || location) && (
          <div className="template-navy-gutter__meta">
            {range && <time>{range}</time>}
            {range && location && <span aria-hidden="true"> · </span>}
            {location && <span className="template-navy-gutter__location">{location}</span>}
          </div>
        )}
      </NavyGutterEntryHeading>
      {item.showAchievements && item.achievements && (
        <p className="template-navy-gutter__achievements">
          <strong className="template-navy-gutter__tag">核心成果</strong>
          {item.achievements}
        </p>
      )}
      {item.highlightsMarkdown && (
        <div className="template-navy-gutter__markdown">
          <ResumeMarkdown markdown={item.highlightsMarkdown} />
        </div>
      )}
    </NavyGutterEntry>
  );
}

function NavyGutterProjectItem(section: ResumeSection, item: ProjectItem) {
  const range = NavyGutterRange(item.startDate, item.endDate);
  const location = item.showLocation ? item.location : '';
  const link = item.showLink ? safeUrl(item.link) : '';
  return (
    <NavyGutterEntry className="template-navy-gutter__entry" item={item} section={section}>
      <NavyGutterEntryHeading className="template-navy-gutter__entry-heading">
        <h3 className="template-navy-gutter__entry-title">
          {item.name}
          {item.role && (
            <span className="template-navy-gutter__entry-role">
              <span className="template-navy-gutter__entry-sep" aria-hidden="true">|</span>
              {item.role}
            </span>
          )}
        </h3>
        {(range || location) && (
          <div className="template-navy-gutter__meta">
            {range && <time>{range}</time>}
            {range && location && <span aria-hidden="true"> · </span>}
            {location && <span className="template-navy-gutter__location">{location}</span>}
          </div>
        )}
      </NavyGutterEntryHeading>
      {item.showTechStack && item.techStack && (
        <NavyGutterTechStack className="template-navy-gutter__tech-stack" techStack={item.techStack} />
      )}
      {item.showIntroduction && item.introduction && (
        <p className="template-navy-gutter__introduction">{item.introduction}</p>
      )}
      {item.showAchievements && item.achievements && (
        <p className="template-navy-gutter__achievements">
          <strong className="template-navy-gutter__tag">项目成果</strong>
          {item.achievements}
        </p>
      )}
      {link && (
        <a className="template-navy-gutter__link" href={link}>
          {item.link}
        </a>
      )}
      {item.highlightsMarkdown && (
        <div className="template-navy-gutter__markdown">
          <ResumeMarkdown markdown={item.highlightsMarkdown} />
        </div>
      )}
    </NavyGutterEntry>
  );
}

function NavyGutterEducationItem(section: ResumeSection, item: EducationItem) {
  const range = NavyGutterRange(item.startDate, item.endDate);
  const detail = [item.degree, item.college, item.major].filter(Boolean).join(' · ');
  return (
    <NavyGutterEntry className="template-navy-gutter__entry template-navy-gutter__entry--compact" item={item} section={section}>
      <NavyGutterEntryHeading className="template-navy-gutter__entry-heading">
        <h3 className="template-navy-gutter__entry-title">
          {item.school}
          {detail && (
            <span className="template-navy-gutter__entry-role">
              <span className="template-navy-gutter__entry-sep" aria-hidden="true">|</span>
              {detail}
            </span>
          )}
        </h3>
        {range && <time className="template-navy-gutter__date">{range}</time>}
      </NavyGutterEntryHeading>
    </NavyGutterEntry>
  );
}

function NavyGutterCampusItem(section: ResumeSection, item: CampusItem) {
  return (
    <NavyGutterEntry className="template-navy-gutter__entry" item={item} section={section}>
      <NavyGutterEntryHeading className="template-navy-gutter__entry-heading">
        <h3 className="template-navy-gutter__entry-title">
          {item.name}
          {item.role && (
            <span className="template-navy-gutter__entry-role">
              <span className="template-navy-gutter__entry-sep" aria-hidden="true">|</span>
              {item.role}
            </span>
          )}
        </h3>
      </NavyGutterEntryHeading>
      {item.highlightsMarkdown && (
        <div className="template-navy-gutter__markdown">
          <ResumeMarkdown markdown={item.highlightsMarkdown} />
        </div>
      )}
    </NavyGutterEntry>
  );
}

function NavyGutterHeader({ document }: { document: ResumeDocument }) {
  const { profile } = document;
  const contacts = [
    { value: profile.phone, href: profile.phone ? `tel:${profile.phone}` : '' },
    { value: profile.email, href: profile.email ? `mailto:${profile.email}` : '' },
    { value: profile.location, href: '' },
    {
      value: profile.website,
      href: profile.website ? (/^https?:/i.test(profile.website) ? profile.website : `https://${profile.website}`) : '',
    },
  ].filter((item) => item.value);

  return (
    <>
      <header className="template-navy-gutter__header">
        <h1 className="template-navy-gutter__name">{profile.fullName}</h1>
        <div className="template-navy-gutter__header-side">
          {profile.headline && (
            <p className="template-navy-gutter__headline">{profile.headline}</p>
          )}
          <ResumeAvatar className="template-navy-gutter__avatar" profile={profile} />
        </div>
      </header>
      {contacts.length > 0 && (
        <p className="template-navy-gutter__contact">
          {contacts.map((item, index) => (
            <span className="template-navy-gutter__contact-item" key={`${item.value}-${index}`}>
              {index > 0 && (
                <span className="template-navy-gutter__contact-sep" aria-hidden="true">|</span>
              )}
              {item.href ? (
                <a className="template-navy-gutter__contact-value" href={item.href}>
                  {item.value}
                </a>
              ) : (
                <span className="template-navy-gutter__contact-value">{item.value}</span>
              )}
            </span>
          ))}
        </p>
      )}
    </>
  );
}

export function NavyGutterTemplate({ document }: { document: ResumeDocument }) {
  const { sections } = document;

  const slots: Parameters<typeof ResumeSections>[0]['slots'] = {
    section: NavyGutterSection,
    title: NavyGutterTitle,
    entry: NavyGutterEntry,
    entryHeading: NavyGutterEntryHeading,
    meta: NavyGutterMeta,
    role: NavyGutterRole,
    skills: NavyGutterSkills,
    honours: NavyGutterHonours,
    item: (section, item) => {
      switch (section.type) {
        case 'work':
        case 'internship':
          return NavyGutterExperienceItem(section, item as ExperienceItem);
        case 'project':
          return NavyGutterProjectItem(section, item as ProjectItem);
        case 'education':
          return NavyGutterEducationItem(section, item as EducationItem);
        case 'campus':
          return NavyGutterCampusItem(section, item as CampusItem);
        default:
          return null;
      }
    },
  };

  return (
    <article className="resume-template template-navy-gutter">
      <NavyGutterHeader document={document} />
      <ResumeSections
        sections={sections}
        slots={slots}
        classNames={{
          section: 'template-navy-gutter__section',
          title: 'template-navy-gutter__section-title',
          entry: 'template-navy-gutter__entry',
          entryHeading: 'template-navy-gutter__entry-heading',
          meta: 'template-navy-gutter__meta',
          role: 'template-navy-gutter__role',
          summary: 'template-navy-gutter__summary',
          markdown: 'template-navy-gutter__markdown',
          skills: 'template-navy-gutter__skills',
          skill: 'template-navy-gutter__skill',
          campus: 'template-navy-gutter__campus',
          honours: 'template-navy-gutter__honours',
          honour: 'template-navy-gutter__honour',
          location: 'template-navy-gutter__location',
          link: 'template-navy-gutter__link',
          introduction: 'template-navy-gutter__introduction',
          achievements: 'template-navy-gutter__achievements',
        }}
      />
    </article>
  );
}
