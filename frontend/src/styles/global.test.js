import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

const stylesheet = readFileSync('src/styles/global.css', 'utf8');

describe('drafts workbench styles', () => {
  it('keeps saved-draft cards scoped away from empty-state guide steps', () => {
    expect(stylesheet).toMatch(/\.resume-drafts__item\s*\{[\s\S]*?background:/);
    expect(stylesheet).toMatch(/\.resume-drafts__steps\s+li\s*\{[\s\S]*?background:\s*transparent/);
    expect(stylesheet).not.toMatch(/\.resume-drafts\s+li\s*\{/);
  });

  it('defines the responsive, accessible workbench visual system', () => {
    expect(stylesheet).toMatch(/\.resume-drafts\s*\{[\s\S]*?max-width:\s*1040px/);
    expect(stylesheet).toMatch(/\.resume-drafts__index\s*\{[\s\S]*?font-variant-numeric:\s*tabular-nums/);
    expect(stylesheet).toMatch(/\.resume-drafts button:focus-visible/);
    expect(stylesheet).toMatch(/@media \(prefers-reduced-motion: reduce\)/);
    expect(stylesheet).toMatch(/@media \(max-width: 640px\)/);
  });
});
