import type { ResumeDocument, ResumeSection } from '../types/resume';

function filenameFromTitle(title: string) {
  const normalized = title.trim().replace(/\s+/g, '-').replace(/[\\/:*?"<>|]/g, '');
  return `${normalized || '简历'}.md`;
}

function dateRange(startDate: string, endDate: string) {
  return [startDate, endDate].filter(Boolean).join(' - ');
}

function contactLine(resume: ResumeDocument) {
  const { email, phone, location, website } = resume.profile;
  return [email, phone, location, website].filter(Boolean).join(' ｜ ');
}

function trimmed(text: string) {
  return text.trim();
}

function experienceItemMarkdown(item: {
  company: string;
  role: string;
  startDate: string;
  endDate: string;
  location: string;
  achievements: string;
  highlightsMarkdown: string;
  showLocation: boolean;
  showAchievements: boolean;
}) {
  const lines: string[] = [];
  const heading = [item.company, item.role].filter(Boolean).join('｜');
  lines.push(`### ${heading}`);
  const meta = [dateRange(item.startDate, item.endDate), item.showLocation ? item.location : '']
    .filter(Boolean)
    .join(' ｜ ');
  if (meta) lines.push(`*${meta}*`);
  if (item.showAchievements && trimmed(item.achievements)) lines.push(`**${trimmed(item.achievements)}**`);
  const highlights = trimmed(item.highlightsMarkdown);
  if (highlights) lines.push(highlights);
  return lines.join('\n\n');
}

function projectItemMarkdown(item: {
  name: string;
  role: string;
  startDate: string;
  endDate: string;
  location: string;
  link: string;
  introduction: string;
  techStack: string;
  achievements: string;
  highlightsMarkdown: string;
  showLocation: boolean;
  showLink: boolean;
  showIntroduction: boolean;
  showTechStack: boolean;
  showAchievements: boolean;
}) {
  const lines: string[] = [];
  const name = item.showLink && item.link ? `[${item.name}](${item.link})` : item.name;
  const heading = [name, item.role].filter(Boolean).join('｜');
  lines.push(`### ${heading}`);
  const meta = [dateRange(item.startDate, item.endDate), item.showLocation ? item.location : '']
    .filter(Boolean)
    .join(' ｜ ');
  if (meta) lines.push(`*${meta}*`);
  if (item.showIntroduction && trimmed(item.introduction)) lines.push(trimmed(item.introduction));
  if (item.showTechStack && trimmed(item.techStack)) lines.push(`**技术栈**：${trimmed(item.techStack)}`);
  if (item.showAchievements && trimmed(item.achievements)) lines.push(`**${trimmed(item.achievements)}**`);
  const highlights = trimmed(item.highlightsMarkdown);
  if (highlights) lines.push(highlights);
  return lines.join('\n\n');
}

function educationItemMarkdown(item: { school: string; startDate: string; endDate: string; degree: string; college: string; major: string }) {
  const lines: string[] = [];
  const heading = [item.school, item.degree].filter(Boolean).join('｜');
  lines.push(`### ${heading}`);
  const meta = [dateRange(item.startDate, item.endDate), [item.college, item.major].filter(Boolean).join(' ')]
    .filter(Boolean)
    .join(' ｜ ');
  if (meta) lines.push(`*${meta}*`);
  return lines.join('\n\n');
}

function campusItemMarkdown(item: { name: string; role: string; highlightsMarkdown: string }) {
  const lines: string[] = [];
  const heading = [item.name, item.role].filter(Boolean).join('｜');
  lines.push(`### ${heading}`);
  const highlights = trimmed(item.highlightsMarkdown);
  if (highlights) lines.push(highlights);
  return lines.join('\n\n');
}

function sectionMarkdown(section: ResumeSection): string {
  switch (section.type) {
    case 'summary': {
      const markdown = trimmed(section.content.markdown);
      return markdown ? `## ${section.title}\n\n${markdown}` : '';
    }
    case 'work':
    case 'internship': {
      const blocks = section.content.items.map(experienceItemMarkdown).filter(Boolean);
      return blocks.length ? `## ${section.title}\n\n${blocks.join('\n\n')}` : '';
    }
    case 'project': {
      const blocks = section.content.items.map(projectItemMarkdown).filter(Boolean);
      return blocks.length ? `## ${section.title}\n\n${blocks.join('\n\n')}` : '';
    }
    case 'education': {
      const blocks = section.content.items.map(educationItemMarkdown).filter(Boolean);
      return blocks.length ? `## ${section.title}\n\n${blocks.join('\n\n')}` : '';
    }
    case 'skills': {
      const lines = section.content.items
        .map((item) => {
          const name = trimmed(item.name);
          if (!name) return '';
          const note = trimmed(item.note);
          return note ? `- ${name}（${note}）` : `- ${name}`;
        })
        .filter(Boolean);
      return lines.length ? `## ${section.title}\n\n${lines.join('\n')}` : '';
    }
    case 'campus': {
      const blocks = section.content.items.map(campusItemMarkdown).filter(Boolean);
      return blocks.length ? `## ${section.title}\n\n${blocks.join('\n\n')}` : '';
    }
    case 'honours': {
      const lines = section.content.items
        .map((item) => {
          const name = trimmed(item.name);
          return name ? `- ${name}` : '';
        })
        .filter(Boolean);
      return lines.length ? `## ${section.title}\n\n${lines.join('\n')}` : '';
    }
  }
}

export function resumeDocumentToMarkdown(resume: ResumeDocument): string {
  const { profile } = resume;
  const lines: string[] = [];
  const heading = [profile.fullName, profile.headline].filter(Boolean).join('｜');
  lines.push(`# ${heading || resume.title}`);
  const contact = contactLine(resume);
  if (contact) lines.push(contact);

  for (const section of resume.sections) {
    if (!section.visible) continue;
    const markdown = sectionMarkdown(section);
    if (markdown) lines.push(markdown);
  }
  return `${lines.join('\n\n')}\n`;
}

export function exportResumeMarkdown(resume: ResumeDocument): void {
  const markdown = resumeDocumentToMarkdown(resume);
  const blob = new Blob([markdown], { type: 'text/markdown;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.download = filenameFromTitle(resume.title);
  link.href = url;
  document.body.append(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
}
