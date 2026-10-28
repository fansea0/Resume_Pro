import type { AiSuggestion } from '../types/ai';

export type AppliedSuggestion = Pick<AiSuggestion, 'start' | 'end' | 'replacement'>;

/**
 * 把建议应用到原文，生成改写后的文本。
 *
 * 与服务端 `backend/internal/ai/optimization.go` 的派生逻辑保持一致：
 * 按 rune 偏移从后往前替换，这样前面片段的偏移不会因为替换而失效。
 */
export function applySuggestionsToText(original: string, suggestions: AppliedSuggestion[]): string {
  if (suggestions.length === 0) return original;
  const codePoints = [...original];
  const ordered = [...suggestions].sort((left, right) => right.start - left.start);
  for (const suggestion of ordered) {
    if (suggestion.start < 0 || suggestion.end < suggestion.start) continue;
    if (suggestion.end > codePoints.length) continue;
    codePoints.splice(suggestion.start, suggestion.end - suggestion.start, ...[...suggestion.replacement]);
  }
  return codePoints.join('');
}
