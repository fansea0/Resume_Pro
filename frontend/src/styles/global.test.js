import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

const stylesheet = readFileSync('src/styles/global.css', 'utf8');

describe('drafts workbench styles', () => {
  it('hides the application header when printing a resume', () => {
    expect(stylesheet).toMatch(/@media print\s*\{[\s\S]*?\.app-header\s*\{[\s\S]*?display:\s*none\s*!important/);
  });

  it('adds a top margin to continuation pages without shifting the first page', () => {
    expect(stylesheet).toMatch(/@page\s*\{\s*size:\s*A4;\s*margin:\s*12mm 0 0;/);
    expect(stylesheet).toMatch(/@page :first\s*\{\s*margin-top:\s*0;/);
  });

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

  it('keeps the empty-state action compact instead of spanning the panel', () => {
    expect(stylesheet).toMatch(/\.resume-drafts__empty button\s*\{[\s\S]*?justify-self:\s*start/);
    expect(stylesheet).toMatch(/\.resume-drafts__empty button\s*\{[\s\S]*?width:\s*auto/);
  });

  it('uses the approved workbench tokens and type roles', () => {
    expect(stylesheet).toMatch(/--draft-paper:\s*#F8FAFC/);
    expect(stylesheet).toMatch(/--draft-ink:\s*#162235/);
    expect(stylesheet).toMatch(/--draft-accent:\s*#155EEF/);
    expect(stylesheet).toMatch(/--draft-wash:\s*#EAF1FF/);
    expect(stylesheet).toMatch(/--draft-muted:\s*#6B778C/);
    expect(stylesheet).toMatch(/--draft-body-font:\s*"PingFang SC",\s*"Microsoft YaHei",\s*sans-serif/);
    expect(stylesheet).toMatch(/\.resume-drafts\s*\{[\s\S]*?font-family:\s*var\(--draft-body-font\)/);
    expect(stylesheet).toMatch(/\.resume-drafts__(?:header h2|empty h3|item h3)\s*\{[\s\S]*?font-family:\s*var\(--draft-display-font\)/);
    expect(stylesheet).toMatch(/\.resume-drafts__template-name\s*\{[\s\S]*?font-family:\s*var\(--draft-body-font\)/);
    expect(stylesheet).toMatch(/\.resume-drafts__updated-at\s*\{[\s\S]*?font-family:\s*var\(--draft-mono-font\)/);
    expect(stylesheet).toMatch(/\.resume-drafts button:focus-visible\s*\{[\s\S]*?outline:\s*3px solid #155EEF/);
  });

  it('keeps portal target-job primary actions visibly styled outside page-scoped tokens', () => {
    const backdropRule = stylesheet.match(/\.target-job-dialog__backdrop\s*\{[^}]+\}/)?.[0] ?? '';
    const primaryActionRule = stylesheet.match(/\.target-jobs__create,\s*\.target-job-form__actions button:last-child\s*\{[^}]+\}/)?.[0] ?? '';
    const style = document.createElement('style');
    style.textContent = `${backdropRule}\n${primaryActionRule}`;
    document.head.appendChild(style);

    const backdrop = document.createElement('div');
    backdrop.className = 'target-job-dialog__backdrop';
    backdrop.innerHTML = '<form class="target-job-form"><div class="target-job-form__actions"><button>返回</button><button>保存并选择</button></div></form>';
    document.body.appendChild(backdrop);

    const save = backdrop.querySelector('button:last-child');
    expect(getComputedStyle(save).backgroundColor).toBe('rgb(21, 94, 239)');
    expect(getComputedStyle(save).color).toBe('rgb(255, 255, 255)');
    expect(getComputedStyle(backdrop).getPropertyValue('--target-accent').trim()).toBe('#155eef');

    backdrop.remove();
    style.remove();
  });

  it('lays out the AI suggestion as an accessible desktop drawer and mobile bottom sheet', () => {
    expect(stylesheet).toMatch(/\.ai-suggestion-panel__backdrop\s*\{[^}]*position:\s*fixed/);
    expect(stylesheet).toMatch(/\.ai-suggestion-panel\s*\{[^}]*max-width:\s*720px/);
    expect(stylesheet).toMatch(/\.ai-suggestion-panel button:focus-visible[^\{]*\{[^}]*outline:\s*3px solid/);
    expect(stylesheet).toMatch(/\.ai-suggestion-panel__sr-only\s*\{[^}]*clip:\s*rect\(0, 0, 0, 0\)/);
    expect(stylesheet).toMatch(/@media \(max-width: 640px\)\s*\{[\s\S]*?\.ai-suggestion-panel__backdrop\s*\{[^}]*align-items:\s*end/);
    expect(stylesheet).toMatch(/@media \(prefers-reduced-motion: reduce\)\s*\{[\s\S]*?\.ai-suggestion-panel__pulse/);
  });
});
