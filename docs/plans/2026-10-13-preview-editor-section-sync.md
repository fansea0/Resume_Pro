# 预览定位到编辑器 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让用户在右侧简历预览上点击区块标题或条目标题时，左侧编辑区自动滚动到对应位置并短暂高亮。

**Architecture:** 预览侧标题按钮发出 `{ sectionId, itemId? }` 信号 → `app.tsx` 存入带自增 nonce 的状态 → `ResumeEditor` 通过 `useLocateTarget` 在自己的 DOM 内按属性查找、展开折叠的 `<details>`、滚动并高亮。锚点用 `section.id` / `item.id`，不用 type 或索引。

**Tech Stack:** React 18 + TypeScript + Vite、vitest + @testing-library/react + jsdom、纯 CSS（`global.css`）。

**设计文档：** `docs/plans/2026-10-13-preview-editor-section-sync-design.md`

## Global Constraints

- 工作目录：`/Users/Admin/code/resume_pro`，分支 `feat/preview-editor-section-sync`。**不要切分支，不要提交到 main。**
- 不涉及后端。**不修改任何 `backend/**` 文件**，因此 `go test ./...` 门禁不适用。
- 不改 `ResumeDocument` schema（`schemaVersion: 2` 保持不变），不新增依赖。
- **必须保留现有的 `data-resume-section={section.type}` 属性**。`registry.test.tsx`、`ResumeSections.test.tsx` 多处依赖该选择器，替换会打破现有测试。新增 id 属性必须与它并存。
- 提交信息使用中文 tag：`feat:` / `fix:` / `refactor:` / `test:` / `docs:`（见 `AGSIN.md`）。
- 测试命令：`cd frontend && npx vitest run`；构建校验：`cd frontend && npm run build`。
- **测试里用 `fireEvent`（来自 `@testing-library/react`），不要用 `@testing-library/user-event`——该包未安装，且不得新增依赖。**
- **vitest 没开 `globals`，所以 RTL 的自动 cleanup 不生效。** 每个新增/修改的测试文件都要显式写 `afterEach(cleanup)`，否则 DOM 会在用例间累积导致莫名其妙的失败。仓库既有测试文件都是这么做的，沿用即可。
- **任何新组件的 props 必须在 Interfaces 与测试代码中保持一致。** 写测试时逐个核对必填 prop，不要凭直觉省略。
- 代码风格沿用现有前端：单引号、无分号结尾、2 空格缩进、JSX 属性按字母序（现有代码普遍如此）。
- 无障碍要求（`AGSIN.md`）：按钮必须有 `aria-label`、键盘焦点样式、足够点击面积。
- **不要把定位控件做成覆盖区块的透明遮罩**——那会拦截预览正文的文字选中与项目链接点击。只包裹标题文本。

---

## 关键调查结论（实现前必读）

这一节是调查现有代码后得到的、会改变实现方式的事实。**不要凭直觉重做这些调查。**

1. **`entry` 槽在 4 个模板中当前不可达。** `ResumeSections.tsx` 中每个条目分支都是 `slots?.item ? slots.item(...) : <Entry .../>` 的形态。`blue-sidebar`、`navy-gutter`、`scholarly-serif`、`tech` 同时提供了 `item` 槽，因此它们的 `entry` 覆盖组件（`BlueSidebarEntry` / `NavyGutterEntry` / `ScholarlyEntry` / `tech` 内联 `Entry`）**根本不会被渲染**。
   → 结论：**不要去改这 4 个死代码 `entry` 槽**，锚点加在它们的 `item` 渲染器上即可。删死代码不在本计划范围内。

2. **12 个模板精确分布：**
   - 走 `DefaultEntry`（只需改 `ResumeSections.tsx` 一个文件）：`minimal`、`creative`、`editorial`、`redline`、`violet`、`azure-chronicle`
   - 自带 `item` 槽（需逐个改）：`business`、`tech`、`blue-sidebar`、`mint-canvas`、`navy-gutter`、`scholarly-serif`
   - 覆盖 `section` 槽（需补区块 id）：`business`(内联)、`blue-sidebar`、`navy-gutter`、`scholarly-serif`
   - 覆盖 `title` 槽（需把定位按钮包进标题文本）：`azure-chronicle`、`mint-canvas`、`blue-sidebar`、`navy-gutter`、`scholarly-serif`

3. **`data-resume-entry` 语义已不一致。** `mint-canvas` 输出 `data-resume-entry={item.id}`（携带 id），而 `blue-sidebar` / `navy-gutter` / `scholarly-serif` 输出裸 `data-resume-entry`（布尔）。`mint-canvas.test.tsx` 依赖 `[data-resume-entry]` 选择器。
   → 结论：**不要改动 `data-resume-entry`**，新增语义明确的 `data-resume-item-id`。

4. **jsdom 未实现 `scrollIntoView` 和 `window.matchMedia`。** `src/test/setup.ts` 只引入了 `jest-dom`。
   → 结论：实现侧必须做存在性防御；测试侧显式打桩以断言行为。

5. **`<details open>` 是常量 prop。** React 仅在 prop 变化时写 DOM 属性，所以用户手动折叠后不会被后续渲染重新打开。定位到折叠区块时必须主动设 `details.open = true`。

---

## File Structure

**新建：**
- `frontend/src/components/ResumeLocate.tsx` — 四个锚点属性名常量 + `SectionLocateButton` + `EntryLocateButton`。预览与编辑器共用的唯一契约来源。
- `frontend/src/components/ResumeLocate.test.tsx` — 上述组件的测试。
- `frontend/src/hooks/useLocateTarget.ts` — 编辑器侧消费定位请求的自定义 hook。
- `frontend/src/hooks/useLocateTarget.test.tsx` — 上述 hook 的测试。

**修改：**
- `frontend/src/templates/ResumeSections.tsx` — 默认槽锚点 + 标题按钮 + `onLocate` 透传 + `slots.item` 签名扩展。
- `frontend/src/templates/ResumeSections.test.tsx` — 默认槽的新断言。
- `frontend/src/templates/registry.tsx` — `Renderer` 组件类型增加可选 `onLocate`。
- `frontend/src/templates/registry.test.tsx` — **跨模板不变量测试（方案 B 核心）**。
- `frontend/src/templates/{business,tech,blue-sidebar,navy-gutter,scholarly-serif,mint-canvas,azure-chronicle}.tsx` — 补锚点与定位按钮。
- `frontend/src/components/ResumeEditor.tsx` — 编辑侧锚点属性 + 接收 `locateRequest`。
- `frontend/src/components/ResumeEditor.test.tsx` — 编辑侧锚点测试。
- `frontend/src/components/ResumePreview.tsx` — 透传 `onLocate`。
- `frontend/src/app.tsx` — 持有 `locateRequest` state 与 nonce 路由。
- `frontend/src/app.test.tsx` — 集成测试。
- `frontend/src/styles/global.css` — 定位按钮样式、`scroll-margin-top`、`is-located` 高亮、reduced-motion、print。
- `frontend/src/styles/global.test.js` — CSS 规则断言。

---

### Task 1: 共享锚点契约与定位按钮组件

**Files:**
- Create: `frontend/src/components/ResumeLocate.tsx`
- Test: `frontend/src/components/ResumeLocate.test.tsx`

**Interfaces:**
- Produces:
  ```ts
  export const SECTION_ANCHOR_ATTR: 'data-resume-section-id';
  export const ITEM_ANCHOR_ATTR: 'data-resume-item-id';
  export const EDITOR_SECTION_ANCHOR_ATTR: 'data-editor-section-id';
  export const EDITOR_ITEM_ANCHOR_ATTR: 'data-editor-item-id';
  export const LOCATED_CLASS: 'is-located';
  export interface LocateTarget { sectionId: string; itemId?: string }
  export interface LocateRequest extends LocateTarget { nonce: number }
  export function SectionLocateButton(props: { label: string; onLocate?: (t: LocateTarget) => void; children: ReactNode }): JSX.Element;
  export function EntryLocateButton(props: { label: string; sectionId: string; itemId: string; onLocate?: (t: LocateTarget) => void; children: ReactNode }): JSX.Element;
  ```
  后续所有任务都从这里 import，不要在别处重复写属性名字符串。

- [ ] **Step 1: 写失败测试**

创建 `frontend/src/components/ResumeLocate.test.tsx`：

```tsx
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import {
  EDITOR_ITEM_ANCHOR_ATTR,
  EDITOR_SECTION_ANCHOR_ATTR,
  EntryLocateButton,
  ITEM_ANCHOR_ATTR,
  LOCATED_CLASS,
  SECTION_ANCHOR_ATTR,
  SectionLocateButton,
} from './ResumeLocate';

describe('ResumeLocate', () => {
  it('导出四个锚点属性名与高亮 class 常量', () => {
    expect(SECTION_ANCHOR_ATTR).toBe('data-resume-section-id');
    expect(ITEM_ANCHOR_ATTR).toBe('data-resume-item-id');
    expect(EDITOR_SECTION_ANCHOR_ATTR).toBe('data-editor-section-id');
    expect(EDITOR_ITEM_ANCHOR_ATTR).toBe('data-editor-item-id');
    expect(LOCATED_CLASS).toBe('is-located');
  });

  it('区块按钮点击后只发出区块级目标', async () => {
    const onLocate = vi.fn();
    render(<h2><SectionLocateButton label="项目经历" onLocate={onLocate}>项目经历</SectionLocateButton></h2>);
    const button = screen.getByRole('button', { name: '定位到编辑器的「项目经历」' });
    expect(button.tagName).toBe('BUTTON');
    expect(button).toHaveAttribute('type', 'button');
    await userEvent.click(button);
    expect(onLocate).toHaveBeenCalledWith({ sectionId: 'section-1' });
  });

  it('条目按钮点击后同时发出区块与条目 id', async () => {
    const onLocate = vi.fn();
    render(<h3><EntryLocateButton itemId="item-7" label="AI 创作副驾" sectionId="section-1" onLocate={onLocate}>AI 创作副驾</EntryLocateButton></h3>);
    await userEvent.click(screen.getByRole('button', { name: '定位到编辑器的「AI 创作副驾」' }));
    expect(onLocate).toHaveBeenCalledWith({ sectionId: 'section-1', itemId: 'item-7' });
  });

  it('缺少 onLocate 时不渲染按钮，标题保持纯文本', () => {
    render(<h2><SectionLocateButton label="项目经历">项目经历</SectionLocateButton></h2>);
    expect(screen.queryByRole('button')).toBeNull();
    expect(screen.getByRole('heading', { level: 2 })).toHaveTextContent('项目经历');
  });

  it('保留标题语义层级：按钮在 h2 / h3 内部', () => {
    render(<h2><SectionLocateButton label="A" onLocate={vi.fn()}>A</SectionLocateButton></h2>);
    expect(screen.getByRole('heading', { level: 2 }).querySelector('button')).not.toBeNull();
  });
});
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd frontend && npx vitest run src/components/ResumeLocate.test.tsx`
Expected: FAIL，报 `Failed to resolve import "./ResumeLocate"`。

- [ ] **Step 3: 实现组件**

创建 `frontend/src/components/ResumeLocate.tsx`：

```tsx
import type { ReactNode } from 'react';

/** 预览侧区块锚点。与既有 `data-resume-section={type}` 并存，不可互相替代。 */
export const SECTION_ANCHOR_ATTR = 'data-resume-section-id';
/** 预览侧条目锚点。语义独立于既有的 `data-resume-entry`（后者语义已不一致，勿动）。 */
export const ITEM_ANCHOR_ATTR = 'data-resume-item-id';
/** 编辑器侧区块锚点。 */
export const EDITOR_SECTION_ANCHOR_ATTR = 'data-editor-section-id';
/** 编辑器侧条目锚点。 */
export const EDITOR_ITEM_ANCHOR_ATTR = 'data-editor-item-id';
/** 定位命中时短暂施加的高亮 class。 */
export const LOCATED_CLASS = 'is-located';

export interface LocateTarget {
  sectionId: string;
  itemId?: string;
}

/** `app.tsx` 持有的完整请求；nonce 保证重复点击同一目标也能重放。 */
export interface LocateRequest extends LocateTarget {
  nonce: number;
}

export type LocateHandler = (target: LocateTarget) => void;

function locateLabel(label: string) {
  return `定位到编辑器的「${label}」`;
}

/**
 * 区块标题定位按钮。必须放在原 `<h2>` 内部：既保留标题语义（读屏仍按标题朗读），
 * 又获得原生按钮语义与键盘可达性。不要改成覆盖整个区块的透明遮罩——
 * 那会拦截预览正文的文字选中与项目链接点击。
 */
export function SectionLocateButton({ children, label, onLocate, sectionId }: {
  children: ReactNode;
  label: string;
  onLocate?: LocateHandler;
  sectionId: string;
}) {
  if (!onLocate) return <>{children}</>;
  return (
    <button aria-label={locateLabel(label)} className="resume-locate" onClick={() => onLocate({ sectionId })} title={locateLabel(label)} type="button">
      {children}
    </button>
  );
}

/** 条目标题定位按钮，同样包裹在原 `<h3>` 内部。 */
export function EntryLocateButton({ children, itemId, label, onLocate, sectionId }: {
  children: ReactNode;
  itemId: string;
  label: string;
  onLocate?: LocateHandler;
  sectionId: string;
}) {
  if (!onLocate) return <>{children}</>;
  return (
    <button aria-label={locateLabel(label)} className="resume-locate" onClick={() => onLocate({ sectionId, itemId })} title={locateLabel(label)} type="button">
      {children}
    </button>
  );
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `cd frontend && npx vitest run src/components/ResumeLocate.test.tsx`
Expected: PASS，5 个用例全绿。

- [ ] **Step 5: 提交**

```bash
git add frontend/src/components/ResumeLocate.tsx frontend/src/components/ResumeLocate.test.tsx
git commit -m "feat: 新增预览定位共享契约与标题定位按钮"
```

---

### Task 2: ResumeSections 默认槽锚点、标题按钮与 onLocate 透传

**Files:**
- Modify: `frontend/src/templates/ResumeSections.tsx`
- Test: `frontend/src/templates/ResumeSections.test.tsx`

**Interfaces:**
- Consumes: Task 1 的 `SECTION_ANCHOR_ATTR`、`ITEM_ANCHOR_ATTR`、`SectionLocateButton`、`EntryLocateButton`、`LocateHandler`。
- Produces:
  ```ts
  // SectionSlotProps / TitleSlotProps / ItemSlotProps 均新增可选字段：
  onLocate?: LocateHandler;
  // 三个槽的 item 上下文收窄为可取 id 的形状：
  export interface ItemSlotProps { children: ReactNode; className?: string; onLocate?: LocateHandler; section: ResumeSection; item: { id?: string } }
  // ResumeSectionsProps 新增：
  onLocate?: LocateHandler;
  // ResumeSectionsSlots.item 签名扩展为三参数：
  item: (section, item, onLocate?: LocateHandler) => ReactNode;
  ```

- [ ] **Step 1: 写失败测试**

在 `frontend/src/templates/ResumeSections.test.tsx` 末尾追加：

```tsx
describe('ResumeSections 定位锚点', () => {
  it('默认区块槽输出 section id 锚点，并保留既有 type 属性', () => {
    const { container } = render(<ResumeSections sections={sampleSections} />);
    const node = container.querySelector('[data-resume-section="project"]');
    expect(node).toHaveAttribute(SECTION_ANCHOR_ATTR, 'sample-project');
    expect(node).toHaveAttribute('data-resume-section', 'project');
  });

  it('默认条目槽输出 item id 锚点', () => {
    const { container } = render(<ResumeSections sections={sampleSections} />);
    expect(container.querySelector(`[${ITEM_ANCHOR_ATTR}="sample-project-copilot"]`)).not.toBeNull();
  });

  it('标题内是原生按钮，点击发出区块级目标', () => {
    const onLocate = vi.fn();
    render(<ResumeSections onLocate={onLocate} sections={sampleSections} />);
    fireEvent.click(screen.getByRole('button', { name: '定位到编辑器的「项目经历」' }));
    expect(onLocate).toHaveBeenCalledWith({ sectionId: 'sample-project' });
  });

  it('条目标题内是原生按钮，点击发出条目级目标', () => {
    const onLocate = vi.fn();
    render(<ResumeSections onLocate={onLocate} sections={sampleSections} />);
    fireEvent.click(screen.getByRole('button', { name: '定位到编辑器的「AI 创作副驾」' }));
    expect(onLocate).toHaveBeenCalledWith({ sectionId: 'sample-project', itemId: 'sample-project-copilot' });
  });

  it('没有 onLocate 时标题保持纯文本，布局与改动前一致', () => {
    render(<ResumeSections sections={sampleSections} />);
    expect(screen.queryByRole('button')).toBeNull();
  });

  it('summary 区块无条目锚点', () => {
    const { container } = render(<ResumeSections sections={sampleSections} />);
    const summary = container.querySelector('[data-resume-section="summary"]');
    expect(summary?.querySelectorAll(`[${ITEM_ANCHOR_ATTR}]`).length).toBe(0);
  });
});
```

在该文件顶部补齐 import（若尚未引入）：

```tsx
import { fireEvent, render, screen } from '@testing-library/react';
import { afterEach, cleanup } from 'vitest';
import { ITEM_ANCHOR_ATTR, SECTION_ANCHOR_ATTR } from '../components/ResumeLocate';

afterEach(cleanup);
```

并确保 `sampleSections` 变量已存在（若测试文件用的是别的名字，改为实际变量名；建议直接用 `createSampleResumeDocument().sections`）。

- [ ] **Step 2: 运行测试确认失败**

Run: `cd frontend && npx vitest run src/templates/ResumeSections.test.tsx`
Expected: FAIL，多条断言因缺少属性与按钮。

- [ ] **Step 3: 实现默认槽改造**

修改 `frontend/src/templates/ResumeSections.tsx`：

1. 顶部 import：

```tsx
import { EntryLocateButton, SectionLocateButton, ITEM_ANCHOR_ATTR, SECTION_ANCHOR_ATTR, type LocateHandler } from '../components/ResumeLocate';
```

2. 三个槽 props 接口各加 `onLocate?: LocateHandler;`（`SectionSlotProps`、`TitleSlotProps`、`ItemSlotProps`），并把 `ItemSlotProps` 的 `item: unknown` 改为 `item: { id?: string }`。

3. `ResumeSectionsProps` 加 `onLocate?: LocateHandler;`。

4. `ResumeSectionsSlots.item` 签名改为：

```tsx
item: (section: ResumeSection, item: ExperienceItem | ProjectItem | CampusItem | Extract<ResumeSection, { type: 'education' }>['content']['items'][number], onLocate?: LocateHandler) => ReactNode;
```

5. `DefaultSection` → `DefaultTitle` → `DefaultEntry` 依次改为：

```tsx
function DefaultSection({ children, className, section }: SectionSlotProps) {
  return <section className={className} data-resume-section={section.type} {...{ [SECTION_ANCHOR_ATTR]: section.id }}>{children}</section>;
}

function DefaultTitle({ children, className, onLocate, section }: TitleSlotProps) {
  return <h2 className={className} data-resume-section-title><SectionLocateButton label={section.title} onLocate={onLocate} sectionId={section.id}>{children}</SectionLocateButton></h2>;
}

function DefaultEntry({ children, className, item }: ItemSlotProps) {
  return <article className={className} {...(item.id ? { [ITEM_ANCHOR_ATTR]: item.id } : {})}>{children}</article>;
}
```

6. 四个条目渲染函数（`ExperienceEntry`、`ProjectEntry`、`CampusEntry`，以及 `education` 分支的内联 `Entry`）的 `<h3>` 改为包 `EntryLocateButton`：

```tsx
<EntryHeading className={classNames?.entryHeading}>
  <h3><EntryLocateButton itemId={item.id} label={item.company} onLocate={onLocate} sectionId={section.id}>{item.company}</EntryLocateButton></h3>
  <Metadata … />
</EntryHeading>
```

其中 `label` 依次取 `item.company`（work/internship）、`item.name`（project / campus）、`item.school`（education）。`ExperienceEntry` / `ProjectEntry` / `CampusEntry` 需要新增 `onLocate` 参数并在 props 类型中声明。

7. `ResumeSections` 主体：解构出 `onLocate`，并在三处调用点透传——
   - `<Section … section={section} onLocate={onLocate}>`
   - `<Title … section={section} onLocate={onLocate}>`
   - `slots?.item ? slots.item(section, item, onLocate) : <ExperienceEntry … onLocate={onLocate} />`（`work`/`internship`/`project`/`education`/`campus` 五个分支都要改）

- [ ] **Step 4: 运行测试确认通过**

Run: `cd frontend && npx vitest run src/templates/ResumeSections.test.tsx`
Expected: PASS，既有测试与新增 6 条断言全绿。

- [ ] **Step 5: 全量回归**

Run: `cd frontend && npx vitest run`
Expected: 全绿。Task 2 尚未接线到 `app.tsx`，因此 `app.test.tsx` 应仍通过。

- [ ] **Step 6: 提交**

```bash
git add frontend/src/templates/ResumeSections.tsx frontend/src/templates/ResumeSections.test.tsx
git commit -m "feat: 默认简历区块输出定位锚点与标题定位按钮"
```

---

### Task 3: 模板的区块锚点与标题定位按钮

**Files:**
- Modify: `frontend/src/templates/business.tsx`（区块锚点）
- Modify: `frontend/src/templates/blue-sidebar.tsx`、`navy-gutter.tsx`、`scholarly-serif.tsx`（区块锚点 + 标题按钮）
- Modify: `frontend/src/templates/mint-canvas.tsx`、`azure-chronicle.tsx`（标题按钮）
- Modify: `frontend/src/templates/registry.tsx`（`Renderer` 类型）

**Interfaces:**
- Consumes: Task 1 常量与按钮、Task 2 扩展后的槽 props。
- Produces: 全部 12 个模板的 `Renderer` 接受可选 `onLocate?: LocateHandler`，并写入 `document.sections`。

- [ ] **Step 1: 写失败测试**

在 `frontend/src/templates/registry.test.tsx` 追加**跨模板不变量测试**（方案 B 的核心）：

```tsx
describe('全部模板的区块与标题定位不变量', () => {
  it.each(templateRegistry.map((t) => t.id))('%s 的每个可见区块都有 section id 锚点', (id) => {
    const { container } = render(<Renderer id={id} />);
    for (const section of createSampleResumeDocument().sections) {
      expect(container.querySelector(`[data-resume-section="${section.type}"][${SECTION_ANCHOR_ATTR}="${section.id}"]`)).not.toBeNull();
    }
  });

  it.each(templateRegistry.map((t) => t.id))('%s 的每个区块标题内都有定位按钮', (id) => {
    render(<Renderer id={id} onLocate={vi.fn()} />);
    const buttons = screen.getAllByRole('button', { name: /^定位到编辑器的「.+」$/ });
    expect(buttons.length).toBeGreaterThan(0);
  });
});
```

> **条目锚点不变量在 Task 4 才加入。** 本任务结束时整个测试套件必须保持全绿，不允许提交一个红的套件——CI 在 `backend/**` 变更时跑测试，但前端套件的绿色同样是项目门禁。

在该测试文件顶部补齐 import：

```tsx
import { ITEM_ANCHOR_ATTR, SECTION_ANCHOR_ATTR } from '../components/ResumeLocate';
import { createSampleResumeDocument, type TemplateId } from '../types/resume';
```

> 若该测试文件已有自己的 `Renderer` 辅助组件（按 id 渲染对应模板），直接复用；否则按上述 `templateRegistry` 写一个：
>
> ```tsx
> function Renderer({ id, onLocate }: { id: TemplateId; onLocate?: LocateHandler }) {
>   const Entry = getTemplate(id).Renderer;
>   return <Entry document={createSampleResumeDocument()} onLocate={onLocate} />;
> }
> ```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd frontend && npx vitest run src/templates/registry.test.tsx`
Expected: FAIL，`section` 槽覆盖的 4 个模板与 `title` 槽覆盖的 5 个模板缺少锚点或按钮。

- [ ] **Step 3: 扩展 registry 的 Renderer 类型**

`frontend/src/templates/registry.tsx`：

```tsx
import type { LocateHandler } from '../components/ResumeLocate';
// …
export interface ResumeTemplate {
  id: TemplateId;
  name: string;
  description: string;
  tags: string[];
  styleDescription: string;
  previewImage: string;
  Renderer: ComponentType<{ document: ResumeDocument; onLocate?: LocateHandler }>;
}
```

- [ ] **Step 4: 给 7 个模板的组件签名加上 `onLocate` 并透传**

每个模板的导出组件从 `({ document }: { document: ResumeDocument })` 改为：

```tsx
({ document, onLocate }: { document: ResumeDocument; onLocate?: LocateHandler })
```

并在其 `<ResumeSections …>` 上加 `onLocate={onLocate}`。`creative` 与 `editorial` 各有两处 `<ResumeSections>`，两处都要加。`onLocate` 为 `undefined` 时行为与改动前完全一致。

- [ ] **Step 5: 补区块锚点（4 个模板）**

`business.tsx` 第 11 行内联 `section` 槽：

```tsx
section: ({ children, className, section }) => <section className={section.type === 'summary' ? 'template-business__intro' : className} data-resume-section={section.type} {...{ [SECTION_ANCHOR_ATTR]: section.id }}>{children}</section>,
```

`blue-sidebar.tsx` 的 `BlueSidebarSection`：

```tsx
<section className={className} data-resume-section={section.type} {...{ [SECTION_ANCHOR_ATTR]: section.id }}>
```

`navy-gutter.tsx` 的 `NavyGutterSection`：在既有 `data-resume-section={section.type}` 下一行加 `{...{ [SECTION_ANCHOR_ATTR]: section.id }}`。

`scholarly-serif.tsx` 的 `ScholarlySection`：同上。

- [ ] **Step 6: 补标题定位按钮（5 个模板）**

每个模板的标题槽把 `{children}` 包进 `SectionLocateButton`，并传入 `label={section.title}`、`sectionId={section.id}`、`onLocate`：

`azure-chronicle.tsx` 的 `AzureTitle`：

```tsx
function AzureTitle({ children, className, onLocate, section }: TitleSlotProps) {
  return <h2 className={className} data-resume-section-title><span aria-hidden="true" className="template-azure-chronicle__icon"><svg viewBox="0 0 20 20"><path d={icons[section.type]} /></svg></span><span><SectionLocateButton label={section.title} onLocate={onLocate} sectionId={section.id}>{children}</SectionLocateButton></span></h2>;
}
```

> 注意：`AzureTitle` 当前没有解构 `section` 之外的东西，加上 `onLocate` 与已有的 `section` 一起解构即可。

`mint-canvas.tsx` 的 `MintTitle`：`<span className="template-mint-canvas__title-text">{children}</span>` → 内部包 `SectionLocateButton`（badge 图标保持在按钮外，不要把 svg 塞进按钮）。

`blue-sidebar.tsx` 的 `BlueSidebarTitle`：`BlueSidebarTitle` 当前未解构 `section`，需补上。

`navy-gutter.tsx` 的 `NavyGutterTitle`：`<span className="template-navy-gutter__label">{children}</span>` → 内部包 `SectionLocateButton`。该槽已解构 `section`。

`scholarly-serif.tsx` 的 `ScholarlyTitle`：`<span className="template-scholarly-serif__title-text">{children}</span>` → 内部包 `SectionLocateButton`。**该槽当前没有解构 `section`，必须补上** `<span>` 内的文本用 `{section.title}` 作 label 来源。

- [ ] **Step 7: 运行测试确认通过**

Run: `cd frontend && npx vitest run`
Expected: **全绿**。本任务结束时整个套件必须是绿的。

- [ ] **Step 8: 提交**

```bash
git add frontend/src/templates/{business,blue-sidebar,navy-gutter,scholarly-serif,mint-canvas,azure-chronicle}.tsx frontend/src/templates/registry.tsx frontend/src/templates/registry.test.tsx
git commit -m "feat: 模板补齐区块定位锚点与标题定位按钮"
```

---

### Task 4: 模板的条目锚点与条目标题按钮

**Files:**
- Modify: `frontend/src/templates/business.tsx`、`tech.tsx`、`blue-sidebar.tsx`、`mint-canvas.tsx`、`navy-gutter.tsx`、`scholarly-serif.tsx`

**Interfaces:**
- Consumes: Task 1 的 `ITEM_ANCHOR_ATTR` 与 `EntryLocateButton`；Task 2 扩展后的 `slots.item(section, item, onLocate)` 签名。
- Produces: 6 个自带 `item` 槽的模板全部输出条目锚点与条目标题按钮。

> **只改 `item` 渲染路径。** `blue-sidebar` / `navy-gutter` / `scholarly-serif` / `tech` 的 `entry` 槽当前不可达（它们都提供了 `item`，`ResumeSections` 里 `slots?.item ? … : <Entry/>` 永远走前者）。不要去改那四个死代码组件。

- [ ] **Step 1: 加入条目锚点不变量测试并确认失败**

在 `frontend/src/templates/registry.test.tsx` 追加（Task 3 已建立的 `describe` 块内或新开一个同名的）：

```tsx
describe('全部模板的条目定位不变量', () => {
  const itemSections = createSampleResumeDocument().sections.filter(
    (section): section is Extract<typeof section, { type: 'work' | 'internship' | 'project' | 'education' | 'campus' }> =>
      section.type === 'work' || section.type === 'internship' || section.type === 'project' || section.type === 'education' || section.type === 'campus',
  );

  it.each(templateRegistry.map((t) => t.id))('%s 的每个经历类条目都有 item id 锚点', (id) => {
    const { container } = render(<Renderer id={id} />);
    for (const section of itemSections) {
      for (const item of section.content.items) {
        expect(container.querySelector(`[${ITEM_ANCHOR_ATTR}="${item.id}"]`)).not.toBeNull();
      }
    }
  });
});
```

> 「专业技能」「荣誉奖项」**不在**这个不变量里：设计文档已明确这两类只提供区块级定位（chip / `<li>` 点击区域过小，违反 `AGSIN.md`）。

Run: `cd frontend && npx vitest run src/templates/registry.test.tsx`
Expected: FAIL，6 个自带 `item` 槽的模板报缺失。

- [ ] **Step 2: `business.tsx`**

该文件的 `item` 槽是内联三元返回三个分支，逐个加锚点与标题按钮：

```tsx
// work / internship 分支
<div className="template-business__entry" key={experience.id} {...{ [ITEM_ANCHOR_ATTR]: experience.id }}>
  <div className="template-business__period">{experience.startDate}<br />{experience.endDate}</div>
  <div><h3><EntryLocateButton itemId={experience.id} label={experience.role} onLocate={onLocate} sectionId={section.id}>{experience.role}</EntryLocateButton></h3>…

// project 分支
<div className="template-business__entry" key={project.id} {...{ [ITEM_ANCHOR_ATTR]: project.id }}>
  …<div><h3><EntryLocateButton itemId={project.id} label={project.name} onLocate={onLocate} sectionId={section.id}>{project.name}</EntryLocateButton></h3>…

// 兜底分支
<div className="template-business__entry" key={value.id} {...{ [ITEM_ANCHOR_ATTR]: value.id }}>
  …<div><h3><EntryLocateButton itemId={value.id} label={value.name ?? value.school ?? value.company ?? ''} onLocate={onLocate} sectionId={section.id}>{value.name ?? value.school ?? value.company}</EntryLocateButton></h3>…
```

`item` 槽回调签名改为 `(section, item, onLocate) => {`。

> `business` 的 work 分支标题用的是 `item.role`，但编辑器里该条目的锚点卡以公司名为主。**统一用 `experience.role` 作为按钮 label**（与标题文字一致即可，label 只用于读屏与 tooltip）。

- [ ] **Step 3: `tech.tsx`**

`item` 槽三个分支同样处理。education 分支标题是 `<strong>{education.school}</strong>`（不是 `<h3>`），把 `<strong>` 内容包进 `EntryLocateButton`：

```tsx
<div className="template-tech__education" key={education.id} {...{ [ITEM_ANCHOR_ATTR]: education.id }}>
  <strong><EntryLocateButton itemId={education.id} label={education.school} onLocate={onLocate} sectionId={section.id}>{education.school}</EntryLocateButton></strong>…
```

project 分支 `<h3>{project.name}</h3>` 与兜底分支 `<h3>{label}</h3>` 同理包裹。回调签名改为 `(section, item, onLocate) => {`。

- [ ] **Step 4: `mint-canvas.tsx`**

该模板的条目渲染在 `MintExperienceItem` / `MintProjectItem` / `MintCampusItem` / education 内联分支中，且已输出 `data-resume-entry={id}`。**保留 `data-resume-entry` 不动**，在其外层或同一元素上追加 `ITEM_ANCHOR_ATTR`，并包裹 `<h3>`。

各条目函数新增 `onLocate?: LocateHandler` 参数，`slots.item` 回调改为 `(section, item, onLocate) => …` 并向下传递。

- [ ] **Step 5: `navy-gutter.tsx` / `scholarly-serif.tsx` / `blue-sidebar.tsx`**

三个模板结构一致：条目渲染在 `NavyGutterExperienceItem` / `…ProjectItem` / `…EducationItem` / `…CampusItem`（`scholarly-serif`、`blue-sidebar` 同名模式）这类独立函数里。

每个函数新增 `onLocate?: LocateHandler` 参数，在其根 `<article>` 上追加 `{...{ [ITEM_ANCHOR_ATTR]: item.id }}`，并把根节点内的标题元素（`<h3>` 或模板自定义的标题标签）内容包进 `EntryLocateButton`。`slots.item` 回调改为 `(section, item, onLocate) => …` 并向下传递。

- [ ] **Step 6: 运行测试确认通过**

Run: `cd frontend && npx vitest run src/templates/registry.test.tsx src/templates/ResumeSections.test.tsx`
Expected: 全绿。此时方案 B 的三组跨模板不变量全部通过。

- [ ] **Step 7: 全量回归**

Run: `cd frontend && npx vitest run`
Expected: 全绿。

- [ ] **Step 8: 提交**

```bash
git add frontend/src/templates/{business,tech,blue-sidebar,navy-gutter,scholarly-serif,mint-canvas}.tsx
git commit -m "feat: 模板补齐条目定位锚点与条目定位按钮"
```

---

### Task 5: useLocateTarget hook

**Files:**
- Create: `frontend/src/hooks/useLocateTarget.ts`
- Test: `frontend/src/hooks/useLocateTarget.test.tsx`

**Interfaces:**
- Consumes: Task 1 的 `EDITOR_ITEM_ANCHOR_ATTR`、`EDITOR_SECTION_ANCHOR_ATTR`、`LOCATED_CLASS`、`LocateRequest`。
- Produces:
  ```ts
  export const HIGHLIGHT_DURATION_MS: 1600;
  export function useLocateTarget(request: LocateRequest | undefined, rootRef: RefObject<HTMLElement | null>): void;
  ```

- [ ] **Step 1: 写失败测试**

创建 `frontend/src/hooks/useLocateTarget.test.tsx`：

```tsx
import { act, render, waitFor } from '@testing-library/react';
import { useRef } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useLocateTarget, HIGHLIGHT_DURATION_MS } from './useLocateTarget';
import type { LocateRequest } from '../components/ResumeLocate';

function Panel({ request }: { request?: LocateRequest }) {
  const ref = useRef<HTMLDivElement>(null);
  useLocateTarget(request, ref);
  return (
    <div ref={ref}>
      {/* 刻意不加 open：验证 hook 会主动展开折叠的区块 */}
      <details data-editor-section-id="section-1"><summary>项目经历</summary>
        <div className="editor-card" data-editor-item-id="item-7">条目</div>
      </details>
    </div>
  );
}

let scrollIntoView: ReturnType<typeof vi.fn>;

beforeEach(() => {
  vi.useFakeTimers();
  scrollIntoView = vi.fn();
  Element.prototype.scrollIntoView = scrollIntoView;
});

afterEach(() => {
  vi.useRealTimers();
  delete (Element.prototype as unknown as { scrollIntoView?: unknown }).scrollIntoView;
});

describe('useLocateTarget', () => {
  it('定位到区块：滚动到区块根节点并加高亮', async () => {
    const { container } = render(<Panel request={{ sectionId: 'section-1', nonce: 1 }} />);
    await waitFor(() => expect(scrollIntoView).toHaveBeenCalledWith({ behavior: 'smooth', block: 'start' }));
    expect(container.querySelector('details')?.classList.contains('is-located')).toBe(true);
  });

  it('目标区块折叠时先展开再滚动', async () => {
    const { container } = render(<Panel request={{ sectionId: 'section-1', nonce: 1 }} />);
    const details = container.querySelector('details') as HTMLDetailsElement;
    expect(details.open).toBe(true);
    expect(scrollIntoView).toHaveBeenCalled();
  });

  it('定位到条目时命中条目节点而不是区块节点', async () => {
    const { container } = render(<Panel request={{ sectionId: 'section-1', itemId: 'item-7', nonce: 1 }} />);
    const card = container.querySelector('[data-editor-item-id="item-7"]');
    await waitFor(() => expect(card?.classList.contains('is-located')).toBe(true));
    expect(container.querySelector('details')?.classList.contains('is-located')).toBe(false);
  });

  it('1600ms 后移除高亮', async () => {
    const { container } = render(<Panel request={{ sectionId: 'section-1', nonce: 1 }} />);
    const details = container.querySelector('details') as HTMLElement;
    await waitFor(() => expect(details.classList.contains('is-located')).toBe(true));
    act(() => { vi.advanceTimersByTime(HIGHLIGHT_DURATION_MS + 10); });
    expect(details.classList.contains('is-located')).toBe(false);
  });

  it('目标不存在时静默不报错、不滚动', async () => {
    render(<Panel request={{ sectionId: 'missing', nonce: 1 }} />);
    await waitFor(() => expect(scrollIntoView).not.toHaveBeenCalled());
  });

  it('request 为 undefined 时不做任何事', async () => {
    render(<Panel />);
    await waitFor(() => expect(scrollIntoView).not.toHaveBeenCalled());
  });

  it('scrollIntoView 不存在时静默降级，不抛异常', async () => {
    delete (Element.prototype as unknown as { scrollIntoView?: unknown }).scrollIntoView;
    const { container } = render(<Panel request={{ sectionId: 'section-1', nonce: 1 }} />);
    await waitFor(() => expect(container.querySelector('details')?.classList.contains('is-located')).toBe(true));
  });
});
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd frontend && npx vitest run src/hooks/useLocateTarget.test.tsx`
Expected: FAIL，`Failed to resolve import "./useLocateTarget"`。

- [ ] **Step 3: 实现 hook**

创建 `frontend/src/hooks/useLocateTarget.ts`：

```tsx
import { useEffect, type RefObject } from 'react';
import {
  EDITOR_ITEM_ANCHOR_ATTR,
  EDITOR_SECTION_ANCHOR_ATTR,
  LOCATED_CLASS,
  type LocateRequest,
} from '../components/ResumeLocate';

export const HIGHLIGHT_DURATION_MS = 1600;

/**
 * 按属性取值查节点，而不是拼进 CSS 选择器：id 来自持久化简历，可能包含
 * 任意字符，拼选择器既有转义风险又有 CSS 注入面。
 */
function findByAttr(root: HTMLElement, attr: string, value: string): HTMLElement | undefined {
  return Array.from(root.querySelectorAll<HTMLElement>(`[${attr}]`))
    .find((node) => node.getAttribute(attr) === value);
}

/** jsdom 与部分旧环境没有 matchMedia；缺失时按"不减弱动效"处理。 */
function prefersReducedMotion(): boolean {
  return typeof window !== 'undefined'
    && typeof window.matchMedia === 'function'
    && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
}

/**
 * 消费 app.tsx 路由过来的定位请求：在编辑器自己的 DOM 内查找目标，
 * 展开折叠的 <details>，滚动并短暂高亮。目标不存在时静默返回。
 */
export function useLocateTarget(request: LocateRequest | undefined, rootRef: RefObject<HTMLElement | null>): void {
  useEffect(() => {
    if (!request) return;
    const root = rootRef.current;
    if (!root) return;

    const target = (request.itemId ? findByAttr(root, EDITOR_ITEM_ANCHOR_ATTR, request.itemId) : undefined)
      ?? findByAttr(root, EDITOR_SECTION_ANCHOR_ATTR, request.sectionId);
    if (!target) return;

    const details = target.closest('details');
    if (details && !details.open) details.open = true;

    if (typeof target.scrollIntoView === 'function') {
      target.scrollIntoView({ behavior: prefersReducedMotion() ? 'auto' : 'smooth', block: 'start' });
    }

    target.classList.add(LOCATED_CLASS);
    const timer = window.setTimeout(() => target.classList.remove(LOCATED_CLASS), HIGHLIGHT_DURATION_MS);
    return () => {
      window.clearTimeout(timer);
      target.classList.remove(LOCATED_CLASS);
    };
  }, [request, rootRef]);
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `cd frontend && npx vitest run src/hooks/useLocateTarget.test.tsx`
Expected: PASS。

> 若"折叠时先展开"这条用例失败：确认 `Panel` 初始渲染时 `details` 是闭合的（`<details>` 不写 `open` 属性即为闭合）。`Panel` 中不要给 `<details>` 加 `open`。

- [ ] **Step 5: 提交**

```bash
git add frontend/src/hooks/useLocateTarget.ts frontend/src/hooks/useLocateTarget.test.tsx
git commit -m "feat: 新增编辑器定位滚动 hook"
```

---

### Task 6: ResumeEditor 输出编辑侧锚点并消费定位

**Files:**
- Modify: `frontend/src/components/ResumeEditor.tsx`
- Test: `frontend/src/components/ResumeEditor.test.tsx`

**Interfaces:**
- Consumes: Task 1 的 `EDITOR_SECTION_ANCHOR_ATTR`、`EDITOR_ITEM_ANCHOR_ATTR`；Task 5 的 `useLocateTarget`。
- Produces:
  ```ts
  export interface ResumeEditorProps {
    document: ResumeDocument;
    locateRequest?: LocateRequest;   // 新增，可选
    resumeId?: number;
    onBeforeAiRewrite?: (snapshot: ResumeDocument) => Promise<boolean>;
    onChange: (document: ResumeDocument) => void;
  }
  ```
  `onLocate` 在本任务**不需要**出现在 props 上——编辑器只消费请求，不发出请求。

- [ ] **Step 1: 写失败测试**

在 `frontend/src/components/ResumeEditor.test.tsx` 追加：

```tsx
describe('ResumeEditor 定位锚点', () => {
  it('每个区块 details 输出 editor 锚点', () => {
    const { container } = render(<ResumeEditor document={createSampleResumeDocument()} onChange={vi.fn()} />);
    const details = Array.from(container.querySelectorAll('details'));
    const sectionIds = details.map((node) => node.getAttribute('data-editor-section-id')).filter(Boolean);
    expect(sectionIds).toEqual(expect.arrayContaining(['sample-project', 'sample-work', 'sample-education']));
  });

  it('每个条目卡片输出 editor item 锚点', () => {
    const { container } = render(<ResumeEditor document={createSampleResumeDocument()} onChange={vi.fn()} />);
    expect(container.querySelector('[data-editor-item-id="sample-project-copilot"]')).not.toBeNull();
  });

  it('收到定位请求时对目标卡片调用 scrollIntoView', async () => {
    Element.prototype.scrollIntoView = vi.fn();
    render(<ResumeEditor document={createSampleResumeDocument()} locateRequest={{ sectionId: 'sample-project', nonce: 1 }} onChange={vi.fn()} />);
    await waitFor(() => expect(Element.prototype.scrollIntoView).toHaveBeenCalled());
    delete (Element.prototype as unknown as { scrollIntoView?: unknown }).scrollIntoView;
  });

  it('locateRequest 缺省时不报错', () => {
    expect(() => render(<ResumeEditor document={createSampleResumeDocument()} onChange={vi.fn()} />)).not.toThrow();
  });
});
```

补齐 import：`import { waitFor } from '@testing-library/react';`、`createSampleResumeDocument`。

- [ ] **Step 2: 运行测试确认失败**

Run: `cd frontend && npx vitest run src/components/ResumeEditor.test.tsx`
Expected: FAIL，锚点属性不存在。

- [ ] **Step 3: 实现**

`ResumeEditor.tsx` 改动：

1. import：

```tsx
import { useRef } from 'react';   // 已存在，确认
import { EDITOR_ITEM_ANCHOR_ATTR, EDITOR_SECTION_ANCHOR_ATTR, type LocateRequest } from './ResumeLocate';
import { useLocateTarget } from '../hooks/useLocateTarget';
```

2. props 接口加 `locateRequest?: LocateRequest;`。

3. `ResumeEditor` 内新增 root ref 并调用 hook：

```tsx
const rootRef = useRef<HTMLFormElement>(null);
useLocateTarget(locateRequest, rootRef);
```

4. `<form>` 上加 `ref={rootRef}`。

5. `SectionCard` 的 `<details>`：给 `DetailsSection` 增加一个可选 `anchorId` prop，渲染为 `<details … {...{ [EDITOR_SECTION_ANCHOR_ATTR]: anchorId }}>`；`SectionCard` 调用时传 `section.id`。「基本信息」与「添加区块」两个 `DetailsSection` 不传 `anchorId`。

6. 条目卡片：`ExperienceItemsEditor` / `ProjectItemsEditor` 的 `<div className="editor-card" key={item.id}>`，以及 `SimpleItemsEditor` 里的 `${cardClassName}` 版本，统一改为：

```tsx
<div className="editor-card" key={item.id} {...{ [EDITOR_ITEM_ANCHOR_ATTR]: item.id }}>
```

`SkillsItemsEditor` 的 `<div className="editor-card editor-card--skill">` 同样加锚点（编辑器侧保留锚点无害；只是预览侧不为技能提供条目按钮，见设计文档"已知限制"）。

- [ ] **Step 4: 运行测试确认通过**

Run: `cd frontend && npx vitest run src/components/ResumeEditor.test.tsx`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add frontend/src/components/ResumeEditor.tsx frontend/src/components/ResumeEditor.test.tsx
git commit -m "feat: 编辑器输出定位锚点并响应预览定位请求"
```

---

### Task 7: app.tsx 路由与 ResumePreview 透传

**Files:**
- Modify: `frontend/src/components/ResumePreview.tsx`、`frontend/src/app.tsx`
- Test: `frontend/src/app.test.tsx`

**Interfaces:**
- Consumes: Task 1 的 `LocateHandler`、`LocateRequest`。
- Produces: `ResumePreviewProps` 新增 `onLocate?: LocateHandler`。

- [ ] **Step 1: 写失败测试**

在 `frontend/src/app.test.tsx` 追加集成用例（沿用该文件已有的 mock 装配方式，先 `getCurrentUser.mockResolvedValue(...)` 进入编辑器视图）：

```tsx
describe('预览定位路由', () => {
  it('点击预览区块标题后编辑器把对应区块滚入视口', async () => {
    Element.prototype.scrollIntoView = vi.fn();
    await renderAuthenticatedEditor();
    const button = await screen.findByRole('button', { name: '定位到编辑器的「项目经历」' });
    fireEvent.click(button);
    await waitFor(() => expect(Element.prototype.scrollIntoView).toHaveBeenCalled());
    delete (Element.prototype as unknown as { scrollIntoView?: unknown }).scrollIntoView;
  });

  it('连续点击同一区块两次都会重新触发滚动（nonce 自增）', async () => {
    Element.prototype.scrollIntoView = vi.fn();
    await renderAuthenticatedEditor();
    const button = await screen.findByRole('button', { name: '定位到编辑器的「项目经历」' });
    fireEvent.click(button);
    await waitFor(() => expect(Element.prototype.scrollIntoView).toHaveBeenCalledTimes(1));
    fireEvent.click(button);
    await waitFor(() => expect(Element.prototype.scrollIntoView).toHaveBeenCalledTimes(2));
    delete (Element.prototype as unknown as { scrollIntoView?: unknown }).scrollIntoView;
  });
});
```

> `renderAuthenticatedEditor()` 是本测试文件的辅助函数：完成 `getCurrentUser` / `getTemplateStatus` / `listResumes` / `listTargetJobs` 的 mock 返回并渲染 `<App />`，等待进入编辑器视图。若该文件已有等价的进入编辑器的流程，直接复用，**不要另起一套 mock**。

- [ ] **Step 2: 运行测试确认失败**

Run: `cd frontend && npx vitest run src/app.test.tsx`
Expected: FAIL，找不到定位按钮。

- [ ] **Step 3: `ResumePreview.tsx` 透传**

```tsx
import { forwardRef } from 'react';
import { getTemplate } from '../templates/registry';
import type { ResumeDocument } from '../types/resume';
import type { LocateHandler } from './ResumeLocate';

interface ResumePreviewProps {
  document: ResumeDocument;
  onLocate?: LocateHandler;
}

export const ResumePreview = forwardRef<HTMLDivElement, ResumePreviewProps>(function ResumePreview({ document, onLocate }, ref) {
  const { Renderer } = getTemplate(document.templateId);
  return (
    <section className="resume-preview" aria-label="简历预览" data-testid="resume-preview">
      <div ref={ref}><Renderer document={document} onLocate={onLocate} /></div>
    </section>
  );
});
```

- [ ] **Step 4: `app.tsx` 路由**

在 `App` 内新增：

```tsx
import type { LocateRequest } from './components/ResumeLocate';
// …
const [locateRequest, setLocateRequest] = useState<LocateRequest>();
const locateNonce = useRef(0);
const handleLocate = useCallback((target: { sectionId: string; itemId?: string }) => {
  setLocateRequest({ ...target, nonce: ++locateNonce.current });
}, []);
```

接线：

```tsx
<ResumeEditor
  document={resume}
  locateRequest={locateRequest}
  resumeId={selectedResumeId ?? undefined}
  onBeforeAiRewrite={(snapshot) => resumeAutosaveRef.current?.flush(snapshot) ?? Promise.resolve(false)}
  onChange={changeResumeForSession}
/>
```

```tsx
<ResumePreview document={resume} onLocate={handleLocate} ref={previewRef} />
```

预览模式（`?preview=<template-id>`，用于生成模板封面 PNG）保持 `<Renderer document={sample} />` 不变——`onLocate` 是可选的。

- [ ] **Step 5: 运行测试确认通过**

Run: `cd frontend && npx vitest run src/app.test.tsx`
Expected: PASS。

- [ ] **Step 6: 全量回归**

Run: `cd frontend && npx vitest run`
Expected: 全绿。

- [ ] **Step 7: 提交**

```bash
git add frontend/src/app.tsx frontend/src/app.test.tsx frontend/src/components/ResumePreview.tsx
git commit -m "feat: 接通预览定位到编辑器滚动链路"
```

---

### Task 8: 样式与导出保护

**Files:**
- Modify: `frontend/src/styles/global.css`
- Test: `frontend/src/styles/global.test.js`

**Interfaces:**
- Consumes: Task 1 的 `LOCATED_CLASS`（字符串 `is-located`）、按钮 class `resume-locate`。

- [ ] **Step 1: 写失败测试**

在 `frontend/src/styles/global.test.js` 追加：

```js
describe('预览定位样式', () => {
  it('定位按钮复用继承字体且无边框背景', () => {
    expect(stylesheet).toMatch(/\.resume-locate\s*\{[^}]*background:\s*none/);
    expect(stylesheet).toMatch(/\.resume-locate\s*\{[^}]*font:\s*inherit/);
    expect(stylesheet).toMatch(/\.resume-locate\s*\{[^}]*border:\s*0/);
  });

  it('定位按钮具备可见焦点样式', () => {
    expect(stylesheet).toMatch(/\.resume-locate:focus-visible\s*\{[^}]*outline:/);
  });

  it('编辑器锚点留出滚动留白', () => {
    expect(stylesheet).toMatch(/scroll-margin-top/);
  });

  it('定位高亮有 is-located 样式', () => {
    expect(stylesheet).toMatch(/\.is-located\s*\{/);
  });

  it('减弱动效时定位按钮不做过渡且高亮为静态描边', () => {
    expect(stylesheet).toMatch(/@media \(prefers-reduced-motion: reduce\)[\s\S]*?\.resume-locate[^{]*\{[^}]*transition:\s*none/);
  });

  it('打印时复位定位按钮，标题文字不被抹掉', () => {
    expect(stylesheet).toMatch(/@media print[\s\S]*?\.resume-locate\s*\{/);
  });
});
```

> 该测试文件顶部已有读取样式表的 `stylesheet` 变量，复用即可。

- [ ] **Step 2: 运行测试确认失败**

Run: `cd frontend && npx vitest run src/styles/global.test.js`
Expected: FAIL，六条断言全部未命中。

- [ ] **Step 3: 实现样式**

在 `frontend/src/styles/global.css` 追加：

```css
/* 预览定位：标题按钮化，但视觉必须与原标题完全一致。
   刻意不加 data-html2canvas-ignore —— 那会把标题文字一并从 PNG 画布中抹掉。 */
.resume-locate {
  background: none;
  border: 0;
  color: inherit;
  cursor: pointer;
  font: inherit;
  letter-spacing: inherit;
  line-height: inherit;
  margin: 0;
  padding: 0;
  text-align: inherit;
  text-decoration: inherit;
}
.resume-locate:hover { text-decoration: underline; text-underline-offset: .18em; }
.resume-locate:focus-visible { outline: 3px solid #0071e3; outline-offset: 2px; }
[data-editor-section-id], [data-editor-item-id] { scroll-margin-top: 1rem; }
.is-located { animation: editor-located-flash 1.6s ease-out; border-radius: 8px; }
@keyframes editor-located-flash {
  0%, 100% { background-color: transparent; box-shadow: none; }
  15%, 60% { background-color: #eaf2ff; box-shadow: 0 0 0 3px #0071e3; }
}
```

在既有 `@media (prefers-reduced-motion: reduce)` 块（`global.css:505` 与 `:567`）内追加：

```css
  .resume-locate { transition: none; }
  .resume-locate:hover { text-decoration: none; }
  .is-located { animation: none; background-color: #eaf2ff; box-shadow: 0 0 0 3px #0071e3; }
```

在既有 `@media print` 块内追加：

```css
  /* 打印时把定位按钮复位成普通标题文字：不能隐藏，否则标题会消失 */
  .resume-locate { all: unset; cursor: default; }
```

- [ ] **Step 4: 运行测试确认通过**

Run: `cd frontend && npx vitest run src/styles/global.test.js`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add frontend/src/styles/global.css frontend/src/styles/global.test.js
git commit -m "feat: 新增预览定位按钮与高亮样式"
```

---

### Task 9: 全量验证与手工核对

**Files:** 无新增；仅在发现问题时回改前述文件。

- [ ] **Step 1: 全量测试**

Run: `cd frontend && npx vitest run`
Expected: 全部通过，无 skipped、无 unhandled error。

- [ ] **Step 2: 类型与构建校验**

Run: `cd frontend && npm run build`
Expected: `tsc -b` 无类型错误，`vite build` 成功。

- [ ] **Step 3: 确认后端未被触碰**

Run: `git diff main --stat -- backend/`
Expected: 空输出。

- [ ] **Step 4: 手工核对清单**

按下列顺序逐项确认，每项都要实际看到结果：

1. `cd frontend && npm run dev`，进入编辑器视图，样例简历默认模板为 `redline`。
2. 点击右侧预览的「项目经历」标题 → 左侧滚动到项目经历卡片，且有约 1.6s 的蓝色高亮。
3. 点击右侧预览中「AI 创作副驾」这条的项目名 → 左侧滚动到该条目的表单卡片（不是整个区块）。
4. 把左侧项目经历区块手动折叠，再从预览点击该区块标题 → 确认区块被自动展开后再滚动。
5. **切换全部 12 个模板**，重复第 2、3 步，确认定位均生效且**模板排版与改动前无差异**。
6. **导出 PNG**：确认产出的 PNG 中区块标题与条目标题文字完整可见，没有按钮边框或多余底色，且排版与改动前一致。这是本方案唯一的高风险项，务必肉眼比对。
7. **导出 PDF**（`window.print()`）：确认打印预览中标题正常、编辑器与高亮不出现。
8. 浏览器窗口缩到 ≤900px 宽度，确认单栏布局下点击预览同样能滚动到编辑器目标，且目标不被吸顶 header 遮挡。
9. 键盘 Tab 聚焦到预览标题按钮，确认有 3px 蓝色焦点描边，按 Enter 能触发定位。

- [ ] **Step 5: 提交验证结论（如有修复）**

```bash
git status --short
# 有改动时
git commit -m "fix: 修正预览定位手工核对发现的问题"
```

---

## 完成标准

- `cd frontend && npx vitest run` 全绿。
- `cd frontend && npm run build` 通过。
- 12 个模板的区块级与条目级定位均生效，模板排版零变化。
- PNG / PDF 导出产物与改动前视觉一致。
- `registry.test.tsx` 的跨模板不变量测试存在并通过——这是"新模板漏补锚点"的 CI 防线。
- 未修改任何 `backend/**` 文件。
