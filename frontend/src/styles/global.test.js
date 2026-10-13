import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { HIGHLIGHT_DURATION_MS } from '../hooks/useLocateTarget';

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

describe('预览定位样式', () => {
  /** 取出单条规则文本，断言其中的具体声明，而不是只断言选择器出现过。 */
  const rule = (pattern) => stylesheet.match(pattern)?.[0] ?? '';

  const locateRule = rule(/\.resume-locate\s*\{[^}]*\}/);
  const focusRule = rule(/\.resume-locate:focus-visible\s*\{[^}]*\}/);
  const locatedRule = rule(/\.is-located\s*\{[^}]*\}/);
  const flashOn = rule(/@keyframes editor-located-flash\s*\{[\s\S]*?15%,\s*60%\s*\{[^}]*\}/);
  const flashOff = rule(/@keyframes editor-located-flash\s*\{[\s\S]*?0%,\s*100%\s*\{[^}]*\}/);
  const scrollRule = rule(/\[data-editor-section-id\][^{]*\[data-editor-item-id\]\s*\{[^}]*\}/);
  const reducedLocatedRule = rule(/@media \(prefers-reduced-motion: reduce\)\s*\{[\s\S]*?\.is-located\s*\{[^}]*\}/);
  const reducedLocateRule = rule(/@media \(prefers-reduced-motion: reduce\)\s*\{[\s\S]*?\.resume-locate\s*\{[^}]*\}/);
  const printLocateRule = rule(/@media print[\s\S]*?\.resume-locate\s*\{[^}]*\}/);

  it('定位按钮复用继承字体且无边框背景', () => {
    expect(locateRule).toMatch(/background:\s*none/);
    expect(locateRule).toMatch(/border:\s*0/);
    expect(locateRule).toMatch(/font:\s*inherit/);
    expect(locateRule).toMatch(/color:\s*inherit/);
    expect(locateRule).toMatch(/letter-spacing:\s*inherit/);
    expect(locateRule).toMatch(/line-height:\s*inherit/);
    expect(locateRule).toMatch(/text-align:\s*inherit/);
    expect(locateRule).toMatch(/margin:\s*0/);
    expect(locateRule).toMatch(/padding:\s*0/);
    // 原生按钮外观（appearance: auto）在部分浏览器会自己画一层底与边，必须显式去掉。
    expect(locateRule).toMatch(/appearance:\s*none/);
  });

  it('定位按钮保持指针手势', () => {
    expect(locateRule).toMatch(/cursor:\s*pointer/);
  });

  it('定位按钮在任何上下文都不被隐藏，否则导出的 PNG 会丢掉标题文字', () => {
    expect(locateRule).not.toMatch(/display:\s*none/);
    expect(stylesheet).not.toMatch(/\.resume-locate[^{]*\{[^}]*display:\s*none/);
    expect(stylesheet).not.toMatch(/\.resume-locate[^{]*\{[^}]*visibility:\s*hidden/);
    expect(stylesheet).not.toMatch(/\.resume-locate[^{]*\{[^}]*font-size:\s*0/);
  });

  it('定位按钮具备可见焦点样式', () => {
    expect(focusRule).toMatch(/outline:\s*3px solid/);
    expect(focusRule).toMatch(/outline-offset:\s*2px/);
    expect(focusRule).not.toMatch(/outline:\s*(none|0)\b/);
  });

  it('编辑器锚点留出滚动留白', () => {
    expect(scrollRule).toMatch(/\[data-editor-section-id\]/);
    expect(scrollRule).toMatch(/\[data-editor-item-id\]/);
    expect(scrollRule).toMatch(/scroll-margin-top:\s*1rem/);
  });

  it('定位高亮有 is-located 样式，且闪烁时长与 hook 的移除延时一致', () => {
    expect(locatedRule).toMatch(/animation:\s*editor-located-flash/);
    expect(locatedRule).toMatch(/border-radius:\s*8px/);
    expect(flashOn).toMatch(/background-color:\s*#eaf2ff/);
    expect(flashOn).toMatch(/box-shadow:\s*0 0 0 3px #0071e3/);
    // 动画必须回到透明态，否则 class 被移除后高亮会被"冻"在编辑器里。
    expect(flashOff).toMatch(/background-color:\s*transparent/);
    expect(flashOff).toMatch(/box-shadow:\s*none/);
    const seconds = locatedRule.match(/animation:\s*editor-located-flash\s+([\d.]+)s/)?.[1];
    expect(Number(seconds) * 1000).toBe(HIGHLIGHT_DURATION_MS);
  });

  it('减弱动效时定位按钮不做过渡且高亮为静态描边', () => {
    expect(reducedLocateRule).toMatch(/transition:\s*none/);
    expect(reducedLocatedRule).toMatch(/animation:\s*none/);
    expect(reducedLocatedRule).toMatch(/background-color:\s*#eaf2ff/);
    expect(reducedLocatedRule).toMatch(/box-shadow:\s*0 0 0 3px #0071e3/);
  });

  it('打印时复位定位按钮，标题文字不被抹掉', () => {
    expect(printLocateRule).toMatch(/all:\s*unset/);
    expect(printLocateRule).toMatch(/cursor:\s*default/);
    // `all: unset` 会清掉 font 简写，显式补回，保证打印出的标题仍是模板字体。
    expect(printLocateRule).toMatch(/font:\s*inherit/);
    expect(stylesheet).not.toMatch(/@media print[\s\S]*?\.resume-locate[^{]*\{[^}]*display:\s*none/);
  });

  it('打印时编辑器侧（含定位高亮）整体不参与打印', () => {
    expect(stylesheet).toMatch(/@media print\s*\{[\s\S]*?\.resume-workspace__editor[^{]*\{[^}]*display:\s*none\s*!important/);
  });
});
