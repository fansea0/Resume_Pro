export type RewriteableSectionType = 'summary' | 'work' | 'internship' | 'project';

export interface RewriteRequest {
  sectionId: string;
  sectionType: RewriteableSectionType;
  instructions?: string;
}

export interface GenerateRequest {
  rawText: string;
  targetJobId?: number;
  templateId: import('./resume').TemplateId;
}
export interface SummaryRewriteContent {
  markdown: string;
}

export interface ExperienceRewriteItem {
  id: string;
  // An omitted field keeps the original value; an empty string clears it.
  achievements?: string;
  highlightsMarkdown?: string;
}

export interface ExperienceRewriteContent {
  items: ExperienceRewriteItem[];
}

export interface ProjectRewriteItem {
  id: string;
  // An omitted field keeps the original value; an empty string clears it.
  introduction?: string;
  achievements?: string;
  highlightsMarkdown?: string;
}

export interface ProjectRewriteContent {
  items: ProjectRewriteItem[];
}

export type SuggestionEvent =
  | {
      kind: 'suggestion';
      sectionId: string;
      sectionType: 'summary';
      newContent: SummaryRewriteContent;
      rationale?: string;
    }
  | {
      kind: 'suggestion';
      sectionId: string;
      sectionType: 'work' | 'internship';
      newContent: ExperienceRewriteContent;
      rationale?: string;
    }
  | {
      kind: 'suggestion';
      sectionId: string;
      sectionType: 'project';
      newContent: ProjectRewriteContent;
      rationale?: string;
    };

export type ThinkingEvent = { kind: 'thinking'; text: string };
export type ErrorEvent = { kind: 'error'; code: string; message: string };
export type DoneEvent = { kind: 'done' };

export type SSEEvent = ThinkingEvent | SuggestionEvent | ErrorEvent | DoneEvent;

export type GenerateSuggestionEvent = {
  kind: 'suggestion';
  newContent: unknown;
};

export type GenerateSSEEvent = ThinkingEvent | GenerateSuggestionEvent | ErrorEvent | DoneEvent;
