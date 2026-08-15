import { apiFetch } from './client';
import {
  createEducationSection,
  createProjectSection,
  createSkillsSection,
  createSummarySection,
  createWorkSection,
  type ResumeDocument,
  type ResumeSection,
  type TemplateId,
} from '../types/resume';

export interface StoredResume {
  id: number;
  title: string;
  templateId: ResumeDocument['templateId'];
  content: string;
  createdAt: string;
  updatedAt: string;
}

type ResumePayload = Pick<StoredResume, 'title' | 'templateId'> & { content: ResumeDocument };

const templateIds = new Set<TemplateId>(['minimal', 'business', 'creative', 'tech', 'editorial', 'redline', 'violet', 'azure-chronicle']);
const profileFields = ['fullName', 'headline', 'email', 'phone', 'location', 'website', 'avatarUrl', 'avatarStorageKey'] as const;
const legacyProfileFields = ['fullName', 'headline', 'email', 'phone', 'location', 'website', 'summary'] as const;

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null;
}

function hasStringFields(value: Record<string, unknown>, fields: readonly string[]): boolean {
  return fields.every((field) => typeof value[field] === 'string');
}

function isStringArray(value: unknown): value is string[] {
  return Array.isArray(value) && value.every((item) => typeof item === 'string');
}

function isTemplateId(value: unknown): value is TemplateId {
  return typeof value === 'string' && templateIds.has(value as TemplateId);
}

function isSectionBase(
  value: unknown,
  type: ResumeSection['type'],
): value is Record<string, unknown> & { content: Record<string, unknown> } {
  return isRecord(value)
    && value.type === type
    && hasStringFields(value, ['id', 'title'])
    && typeof value.visible === 'boolean'
    && isRecord(value.content);
}

function isSummarySection(value: unknown): boolean {
  return isSectionBase(value, 'summary') && typeof value.content.markdown === 'string';
}

function isExperienceItem(value: unknown): boolean {
  return isRecord(value)
    && hasStringFields(value, ['id', 'company', 'role', 'startDate', 'endDate', 'location', 'achievements', 'highlightsMarkdown'])
    && typeof value.showLocation === 'boolean'
    && typeof value.showAchievements === 'boolean';
}

function isExperienceSection(value: unknown, type: 'work' | 'internship'): boolean {
  return isSectionBase(value, type)
    && Array.isArray(value.content.items)
    && value.content.items.every(isExperienceItem);
}

function isProjectItem(value: unknown): boolean {
  return isRecord(value)
    && hasStringFields(value, ['id', 'name', 'role', 'startDate', 'endDate', 'location', 'link', 'introduction', 'achievements', 'highlightsMarkdown'])
    && typeof value.showLocation === 'boolean'
    && typeof value.showLink === 'boolean'
    && typeof value.showIntroduction === 'boolean'
    && typeof value.showAchievements === 'boolean';
}

function isProjectSection(value: unknown): boolean {
  return isSectionBase(value, 'project')
    && Array.isArray(value.content.items)
    && value.content.items.every(isProjectItem);
}

function isEducationItem(value: unknown): boolean {
  return isRecord(value) && hasStringFields(value, ['id', 'school', 'startDate', 'endDate', 'degree', 'college', 'major']);
}

function isEducationSection(value: unknown): boolean {
  return isSectionBase(value, 'education')
    && Array.isArray(value.content.items)
    && value.content.items.every(isEducationItem);
}

function isSkillItem(value: unknown): boolean {
  return isRecord(value) && hasStringFields(value, ['id', 'name', 'note']);
}

function isSkillsSection(value: unknown): boolean {
  return isSectionBase(value, 'skills')
    && Array.isArray(value.content.items)
    && value.content.items.every(isSkillItem);
}

function isCampusItem(value: unknown): boolean {
  return isRecord(value) && hasStringFields(value, ['id', 'name', 'role', 'highlightsMarkdown']);
}

function isCampusSection(value: unknown): boolean {
  return isSectionBase(value, 'campus')
    && Array.isArray(value.content.items)
    && value.content.items.every(isCampusItem);
}

function isHonourItem(value: unknown): boolean {
  return isRecord(value) && hasStringFields(value, ['id', 'name']);
}

function isHonoursSection(value: unknown): boolean {
  return isSectionBase(value, 'honours')
    && Array.isArray(value.content.items)
    && value.content.items.every(isHonourItem);
}

function isResumeSection(value: unknown): boolean {
  if (!isRecord(value)) return false;
  switch (value.type) {
    case 'summary': return isSummarySection(value);
    case 'work': return isExperienceSection(value, 'work');
    case 'internship': return isExperienceSection(value, 'internship');
    case 'project': return isProjectSection(value);
    case 'education': return isEducationSection(value);
    case 'skills': return isSkillsSection(value);
    case 'campus': return isCampusSection(value);
    case 'honours': return isHonoursSection(value);
    default: return false;
  }
}

function isResumeContent(value: unknown): value is ResumeDocument {
  if (!isRecord(value) || value.schemaVersion !== 2 || !isRecord(value.profile)) return false;
  return hasStringFields(value, ['title'])
    && isTemplateId(value.templateId)
    && hasStringFields(value.profile, profileFields)
    && Array.isArray(value.sections)
    && value.sections.every(isResumeSection);
}

function upgradeV2AvatarFields(value: unknown): unknown {
  if (!isRecord(value) || value.schemaVersion !== 2 || !isRecord(value.profile)) return value;
  const hasAvatarURL = Object.prototype.hasOwnProperty.call(value.profile, 'avatarUrl');
  const hasAvatarStorageKey = Object.prototype.hasOwnProperty.call(value.profile, 'avatarStorageKey');
  if (hasAvatarURL && hasAvatarStorageKey) return value;

  return {
    ...value,
    profile: {
      ...value.profile,
      ...(hasAvatarURL ? {} : { avatarUrl: '' }),
      ...(hasAvatarStorageKey ? {} : { avatarStorageKey: '' }),
    },
  };
}

function isLegacyExperience(value: unknown): value is Record<string, unknown> {
  return isRecord(value)
    && hasStringFields(value, ['id', 'company', 'role', 'startDate', 'endDate'])
    && isStringArray(value.highlights);
}

function isLegacyProject(value: unknown): value is Record<string, unknown> {
  return isRecord(value)
    && hasStringFields(value, ['id', 'name', 'role', 'link', 'startDate', 'endDate', 'description'])
    && isStringArray(value.highlights);
}

function isLegacyEducation(value: unknown): value is Record<string, unknown> {
  return isRecord(value) && hasStringFields(value, ['id', 'school', 'degree', 'field', 'startDate', 'endDate']);
}

function isLegacySkill(value: unknown): value is Record<string, unknown> {
  return isRecord(value) && hasStringFields(value, ['id', 'name', 'level']);
}

function highlightsToMarkdown(highlights: string[]): string {
  return highlights.map((highlight) => `- ${highlight}`).join('\n');
}

function migrateLegacyDocument(value: unknown): ResumeDocument | null {
  if (!isRecord(value) || !isRecord(value.profile) || !isTemplateId(value.templateId)) return null;
  if (!hasStringFields(value, ['title']) || !hasStringFields(value.profile, legacyProfileFields)) return null;
  if (!Array.isArray(value.experiences) || !value.experiences.every(isLegacyExperience)) return null;
  if (!Array.isArray(value.projects) || !value.projects.every(isLegacyProject)) return null;
  if (!Array.isArray(value.education) || !value.education.every(isLegacyEducation)) return null;
  if (!Array.isArray(value.skills) || !value.skills.every(isLegacySkill)) return null;
  const summary = createSummarySection('legacy-summary');
  summary.content.markdown = value.profile.summary as string;

  const work = createWorkSection('legacy-work');
  work.content.items = value.experiences.map((experience) => ({
    id: experience.id as string,
    company: experience.company as string,
    role: experience.role as string,
    startDate: experience.startDate as string,
    endDate: experience.endDate as string,
    location: '',
    achievements: '',
    highlightsMarkdown: highlightsToMarkdown(experience.highlights as string[]),
    showLocation: false,
    showAchievements: false,
  }));

  const project = createProjectSection('legacy-project');
  project.content.items = value.projects.map((item) => ({
    id: item.id as string,
    name: item.name as string,
    role: item.role as string,
    startDate: item.startDate as string,
    endDate: item.endDate as string,
    location: '',
    link: item.link as string,
    introduction: item.description as string,
    achievements: '',
    highlightsMarkdown: highlightsToMarkdown(item.highlights as string[]),
    showLocation: false,
    showLink: Boolean(item.link),
    showIntroduction: Boolean(item.description),
    showAchievements: false,
  }));

  const education = createEducationSection('legacy-education');
  education.content.items = value.education.map((item) => ({
    id: item.id as string,
    school: item.school as string,
    startDate: item.startDate as string,
    endDate: item.endDate as string,
    degree: item.degree as string,
    college: '',
    major: item.field as string,
  }));

  const skills = createSkillsSection('legacy-skills');
  skills.content.items = value.skills.map((item) => ({
    id: item.id as string,
    name: item.name as string,
    note: item.level as string,
  }));

  return {
    schemaVersion: 2,
    title: value.title as string,
    templateId: value.templateId,
    profile: {
      fullName: value.profile.fullName as string,
      headline: value.profile.headline as string,
      email: value.profile.email as string,
      phone: value.profile.phone as string,
      location: value.profile.location as string,
      website: value.profile.website as string,
      avatarUrl: '',
      avatarStorageKey: '',
    },
    sections: [summary, work, project, education, skills],
  };
}

function payload(document: ResumeDocument): ResumePayload {
  return { title: document.title, templateId: document.templateId, content: document };
}

export function listResumes() {
  return apiFetch<StoredResume[]>('/api/resumes');
}

export function createResume(document: ResumeDocument) {
  return apiFetch<StoredResume>('/api/resumes', { method: 'POST', body: JSON.stringify(payload(document)) });
}

export function updateResume(id: number, document: ResumeDocument) {
  return apiFetch<StoredResume>(`/api/resumes/${id}`, { method: 'PUT', body: JSON.stringify(payload(document)) });
}

export function deleteResume(id: number) {
  return apiFetch<void>(`/api/resumes/${id}`, { method: 'DELETE' });
}

export function documentFromStored(resume: StoredResume): ResumeDocument {
  let content: unknown;
  try {
    content = typeof resume.content === 'string' ? JSON.parse(resume.content) : resume.content;
  } catch {
    throw new Error('invalid stored resume content');
  }

  if (!isTemplateId(resume.templateId)) throw new Error('invalid stored resume content');
  const upgradedContent = upgradeV2AvatarFields(content);
  const document = isResumeContent(upgradedContent) ? upgradedContent : migrateLegacyDocument(content);
  if (!document) throw new Error('invalid stored resume content');

  return { ...document, title: resume.title, templateId: resume.templateId };
}
