import { describe, expectTypeOf, it } from 'vitest';

import type {
  ExperienceRewriteContent,
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
          newContent: { markdown: string };
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
        }
    >();
    expectTypeOf<SSEEvent>().toEqualTypeOf<
      | { kind: 'thinking'; text: string }
      | SuggestionEvent
      | { kind: 'error'; code: string; message: string }
      | { kind: 'done' }
    >();
  });
});
