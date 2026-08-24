import type {
  CampusItem,
  EducationItem,
  ExperienceItem,
  HonourItem,
  ProjectItem,
  ResumeDocument,
  ResumeSection,
  SkillItem,
  TemplateId,
} from '../types/resume';

const MAX_RESUME_REQUEST_BYTES = 1 << 20;
const MAX_RESUME_SECTIONS = 50;
const MAX_RESUME_ITEMS_PER_SECTION = 100;
const MAX_RESUME_STRING_BYTES = 20_000;
const encoder = new TextEncoder();
const templateIds = new Set<TemplateId>([
  'minimal', 'business', 'creative', 'tech', 'editorial', 'redline', 'violet', 'azure-chronicle',
  'mint-canvas', 'scholarly-serif', 'blue-sidebar',
]);

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function boundedString(value: unknown): string | undefined {
  return typeof value === 'string' && encoder.encode(value).byteLength <= MAX_RESUME_STRING_BYTES
    ? value
    : undefined;
}

function requiredStrings(
  value: Record<string, unknown>,
  fields: readonly string[],
): Record<string, string> | undefined {
  const result: Record<string, string> = {};
  for (const field of fields) {
    const normalized = boundedString(value[field]);
    if (normalized === undefined) return undefined;
    result[field] = normalized;
  }
  return result;
}

function requiredBooleans(
  value: Record<string, unknown>,
  fields: readonly string[],
): Record<string, boolean> | undefined {
  const result: Record<string, boolean> = {};
  for (const field of fields) {
    if (typeof value[field] !== 'boolean') return undefined;
    result[field] = value[field];
  }
  return result;
}

function sanitizeItems<T>(
  content: Record<string, unknown>,
  sanitize: (value: unknown) => T | undefined,
): T[] | undefined {
  if (!Array.isArray(content.items) || content.items.length > MAX_RESUME_ITEMS_PER_SECTION) return undefined;
  const items: T[] = [];
  for (const item of content.items) {
    const sanitized = sanitize(item);
    if (sanitized === undefined) return undefined;
    items.push(sanitized);
  }
  return items;
}

function sanitizeExperienceItem(value: unknown): ExperienceItem | undefined {
  if (!isRecord(value)) return undefined;
  const strings = requiredStrings(value, [
    'id', 'company', 'role', 'startDate', 'endDate', 'location', 'achievements', 'highlightsMarkdown',
  ]);
  const booleans = requiredBooleans(value, ['showLocation', 'showAchievements']);
  return strings && booleans ? { ...strings, ...booleans } as unknown as ExperienceItem : undefined;
}

function sanitizeProjectItem(value: unknown): ProjectItem | undefined {
  if (!isRecord(value)) return undefined;
  const strings = requiredStrings(value, [
    'id', 'name', 'role', 'startDate', 'endDate', 'location', 'link',
    'introduction', 'achievements', 'highlightsMarkdown',
  ]);
  const booleans = requiredBooleans(value, [
    'showLocation', 'showLink', 'showIntroduction', 'showAchievements',
  ]);
  return strings && booleans ? { ...strings, ...booleans } as unknown as ProjectItem : undefined;
}

function sanitizeEducationItem(value: unknown): EducationItem | undefined {
  if (!isRecord(value)) return undefined;
  const strings = requiredStrings(value, ['id', 'school', 'startDate', 'endDate', 'degree', 'college', 'major']);
  return strings as unknown as EducationItem | undefined;
}

function sanitizeSkillItem(value: unknown): SkillItem | undefined {
  if (!isRecord(value)) return undefined;
  const strings = requiredStrings(value, ['id', 'name', 'note']);
  return strings as unknown as SkillItem | undefined;
}

function sanitizeCampusItem(value: unknown): CampusItem | undefined {
  if (!isRecord(value)) return undefined;
  const strings = requiredStrings(value, ['id', 'name', 'role', 'highlightsMarkdown']);
  return strings as unknown as CampusItem | undefined;
}

function sanitizeHonourItem(value: unknown): HonourItem | undefined {
  if (!isRecord(value)) return undefined;
  const strings = requiredStrings(value, ['id', 'name']);
  return strings as unknown as HonourItem | undefined;
}

function sanitizeSection(value: unknown): ResumeSection | undefined {
  if (!isRecord(value) || !isRecord(value.content)) return undefined;
  const baseStrings = requiredStrings(value, ['id', 'title']);
  if (!baseStrings || typeof value.visible !== 'boolean') return undefined;
  const base = { id: baseStrings.id, title: baseStrings.title, visible: value.visible };

  switch (value.type) {
    case 'summary': {
      const markdown = boundedString(value.content.markdown);
      return markdown === undefined ? undefined : { ...base, type: 'summary', content: { markdown } };
    }
    case 'work':
    case 'internship': {
      const items = sanitizeItems(value.content, sanitizeExperienceItem);
      return items === undefined ? undefined : { ...base, type: value.type, content: { items } };
    }
    case 'project': {
      const items = sanitizeItems(value.content, sanitizeProjectItem);
      return items === undefined ? undefined : { ...base, type: 'project', content: { items } };
    }
    case 'education': {
      const items = sanitizeItems(value.content, sanitizeEducationItem);
      return items === undefined ? undefined : { ...base, type: 'education', content: { items } };
    }
    case 'skills': {
      const items = sanitizeItems(value.content, sanitizeSkillItem);
      return items === undefined ? undefined : { ...base, type: 'skills', content: { items } };
    }
    case 'campus': {
      const items = sanitizeItems(value.content, sanitizeCampusItem);
      return items === undefined ? undefined : { ...base, type: 'campus', content: { items } };
    }
    case 'honours': {
      const items = sanitizeItems(value.content, sanitizeHonourItem);
      return items === undefined ? undefined : { ...base, type: 'honours', content: { items } };
    }
    default:
      return undefined;
  }
}

export function sanitizeGeneratedResumeDocument(
  value: unknown,
  templateId: TemplateId,
  targetJobId?: number,
): ResumeDocument | undefined {
  if (
    !isRecord(value)
    || value.schemaVersion !== 2
    || !isRecord(value.profile)
    || !Array.isArray(value.sections)
    || value.sections.length > MAX_RESUME_SECTIONS
    || !templateIds.has(templateId)
    || (targetJobId !== undefined && (!Number.isSafeInteger(targetJobId) || targetJobId <= 0))
  ) return undefined;

  const rawTitle = boundedString(value.title);
  const profileStrings = requiredStrings(value.profile, [
    'fullName', 'headline', 'email', 'phone', 'location', 'website',
  ]);
  const showAvatar = value.profile.showAvatar === undefined
    ? true
    : typeof value.profile.showAvatar === 'boolean'
      ? value.profile.showAvatar
      : undefined;
  if (rawTitle === undefined || !profileStrings || showAvatar === undefined) return undefined;

  const sections: ResumeSection[] = [];
  for (const section of value.sections) {
    const sanitized = sanitizeSection(section);
    if (!sanitized) return undefined;
    sections.push(sanitized);
  }

  const title = rawTitle.trim() || 'AI 生成简历';
  const document: ResumeDocument = {
    schemaVersion: 2,
    title,
    templateId,
    ...(targetJobId === undefined ? {} : { targetJobId }),
    profile: {
      fullName: profileStrings.fullName,
      headline: profileStrings.headline,
      email: profileStrings.email,
      phone: profileStrings.phone,
      location: profileStrings.location,
      website: profileStrings.website,
      avatarUrl: '',
      avatarStorageKey: '',
      showAvatar,
    },
    sections,
  };

  const payload = JSON.stringify({ title, templateId, content: document });
  return encoder.encode(payload).byteLength <= MAX_RESUME_REQUEST_BYTES ? document : undefined;
}
