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

export type OptimizationStage =
  | 'context_loaded'
  | 'jd_alignment_started'
  | 'jd_alignment_completed'
  | 'optimization_started'
  | 'optimization_completed'
  | 'contract_repair';

export interface OptimizationTarget {
  itemId?: string;
  field: string;
}

export type SuggestionSeverity = 'high' | 'medium' | 'low';

/** 一条可寻址的改写建议，锚点由服务端 diff 计算得出。 */
export interface AiSuggestion {
  id: string;
  code: string;
  label?: string;
  severity: SuggestionSeverity;
  target: OptimizationTarget;
  issue: string;
  anchor: string;
  replacement: string;
  rationale: string;
  /** 原文该字段中的 rune 偏移，纯插入时 start === end。 */
  start: number;
  end: number;
  status: 'applied' | 'rejected';
  rejectionReason?: string;
}

export interface DensityMetric {
  itemId?: string;
  field: string;
  chars: number;
  lines: number;
  bullets: number;
  maxBulletChars: number;
  overLimit: boolean;
}

export interface OptimizationAnalysis {
  jdAlignment: {
    status: 'applied' | 'skipped';
    matchedKeywords?: string[];
    excludedKeywords?: string[];
    missingKeywords?: string[];
  };
  suggestions: AiSuggestion[];
  evidenceRequests: Array<{
    code: 'S1';
    target: OptimizationTarget;
    reason: string;
    exampleWithPlaceholder: string;
  }>;
  warnings: Array<{ code: string; resolution: string }>;
  density: DensityMetric[];
}

export interface OptimizationSuggestion<TRewrite> {
  rewrite: TRewrite;
  analysis: OptimizationAnalysis;
}

export type SuggestionEvent =
  | {
      kind: 'suggestion';
      sectionId: string;
      sectionType: 'summary';
      newContent: OptimizationSuggestion<SummaryRewriteContent>;
    }
  | {
      kind: 'suggestion';
      sectionId: string;
      sectionType: 'work' | 'internship';
      newContent: OptimizationSuggestion<ExperienceRewriteContent>;
    }
  | {
      kind: 'suggestion';
      sectionId: string;
      sectionType: 'project';
      newContent: OptimizationSuggestion<ProjectRewriteContent>;
    };

export type ThinkingEvent = { kind: 'thinking'; text: string };
export type ProgressEvent = { kind: 'progress'; stage: OptimizationStage; message: string };
export type ErrorEvent = { kind: 'error'; code: string; message: string };
export type DoneEvent = { kind: 'done' };

export type SSEEvent = ThinkingEvent | ProgressEvent | SuggestionEvent | ErrorEvent | DoneEvent;

export type GenerateSuggestionEvent = {
  kind: 'suggestion';
  newContent: unknown;
};

export type GenerateSSEEvent = ThinkingEvent | GenerateSuggestionEvent | ErrorEvent | DoneEvent;
