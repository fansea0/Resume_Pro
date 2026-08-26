import { describe, expectTypeOf, it } from 'vitest';

import type {
  ExperienceRewriteContent,
  OptimizationSuggestion,
  ProjectRewriteContent,
  RewriteRequest,
  SSEEvent,
  SuggestionEvent,
} from './ai';

describe('AI type contracts', () => {
  it('identifies the exact resume section a rewrite targets', () => {
    expectTypeOf<RewriteRequest>().toEqualTypeOf<{
      sectionId: string;
      sectionType: 'summary' | 'work' | 'internship' | 'project';
      instructions?: string;
    }>();
  });

  it('preserves omitted rewrite fields while allowing an empty string to clear a field', () => {
    expectTypeOf<ExperienceRewriteContent>().toEqualTypeOf<{
      items: Array<{
        id: string;
        achievements?: string;
        highlightsMarkdown?: string;
      }>;
    }>();
    expectTypeOf<ProjectRewriteContent>().toEqualTypeOf<{
      items: Array<{
        id: string;
        introduction?: string;
        achievements?: string;
        highlightsMarkdown?: string;
      }>;
    }>();
  });

  it('uses backend SSE event names and section-specific suggestion payloads', () => {
    expectTypeOf<SuggestionEvent>().toEqualTypeOf<
      | {
          kind: 'suggestion';
          sectionId: string;
          sectionType: 'summary';
          newContent: OptimizationSuggestion<{ markdown: string }>;
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
        }
    >();
    expectTypeOf<SSEEvent>().toEqualTypeOf<
      | { kind: 'thinking'; text: string }
      | { kind: 'progress'; stage: 'context_loaded' | 'jd_alignment_started' | 'jd_alignment_completed' | 'optimization_started' | 'optimization_completed'; message: string }
      | SuggestionEvent
      | { kind: 'error'; code: string; message: string }
      | { kind: 'done' }
    >();
  });
});
