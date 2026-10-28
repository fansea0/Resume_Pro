import { describe, expect, it } from 'vitest';

import { applySuggestionsToText } from './applySuggestions';

describe('applySuggestionsToText', () => {
  it('replaces a single span', () => {
    expect(applySuggestionsToText('负责产品交付。', [{ start: 0, end: 2, replacement: '主导' }]))
      .toBe('主导产品交付。');
  });

  it('replaces several spans from the back so earlier offsets stay valid', () => {
    const original = '负责交付，负责运营';
    const result = applySuggestionsToText(original, [
      { start: 0, end: 2, replacement: '主导' },
      { start: 5, end: 7, replacement: '推动' },
    ]);
    expect(result).toBe('主导交付，推动运营');
  });

  it('ignores the order of the given suggestions', () => {
    const original = '负责交付，负责运营';
    const result = applySuggestionsToText(original, [
      { start: 5, end: 7, replacement: '推动' },
      { start: 0, end: 2, replacement: '主导' },
    ]);
    expect(result).toBe('主导交付，推动运营');
  });

  it('removes a span when the replacement is empty', () => {
    expect(applySuggestionsToText('负责交付，负责运营', [{ start: 5, end: 7, replacement: '' }]))
      .toBe('负责交付，运营');
  });

  it('inserts text when the span is empty', () => {
    expect(applySuggestionsToText('负责交付', [{ start: 2, end: 2, replacement: '并推动留存' }]))
      .toBe('负责并推动留存交付');
  });

  it('keeps the original when there is no suggestion', () => {
    expect(applySuggestionsToText('负责产品交付。', [])).toBe('负责产品交付。');
  });

  it('counts runes instead of UTF-16 units', () => {
    const original = '负责🚀交付';
    const result = applySuggestionsToText(original, [{ start: 0, end: 2, replacement: '主导' }]);
    expect(result).toBe('主导🚀交付');
  });

  it('ignores suggestions pointing outside the text', () => {
    expect(applySuggestionsToText('负责交付', [{ start: 40, end: 42, replacement: '越界' }]))
      .toBe('负责交付');
  });
});
