import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

const stylesheet = readFileSync('src/templates/templates.css', 'utf8');

describe('resume template print pagination', () => {
  it('allows sections to flow across pages while keeping individual entries together', () => {
    expect(stylesheet).toMatch(/\.resume-template section\s*\{[\s\S]*?break-inside:\s*auto/);
    expect(stylesheet).toMatch(/\.template-redline__entry\s*\{[\s\S]*?break-inside:\s*avoid/);
  });
});
