# 预览定位到编辑器 — 设计文档

> 日期：2026-10-13
> 状态：设计待审
> 分支：`feat/preview-editor-section-sync`（自 `main` 签出）
> 影响范围：`frontend/src/components/{ResumePreview,ResumeEditor,ResumeLocate}.tsx`、`frontend/src/hooks/useLocateTarget.ts`、`frontend/src/app.tsx`、`frontend/src/templates/ResumeSections.tsx`、7 个模板文件、`frontend/src/styles/global.css` 及对应测试。**不涉及后端、不改 `ResumeDocument` schema。**

## 目标

简历内容变长后，左侧编辑区很难定位到目标区块或条目。本功能让用户在右侧简历预览上点击区块标题或条目标题，左侧编辑区自动滚动到对应位置并短暂高亮，让"看预览 → 改内容"这一步不需要手动来回滚动。

**核心设计原则**：

- **单向**。只有"预览 → 编辑器"一条链路，不做反向同步，不做拖拽式双向滚动跟随。双向滚动同步在两个独立滚动容器里极易抖动，且收益远小于成本。
- **用 `section.id` / `item.id` 定位，不用索引、不用 `type`**。同一份简历可以有两个"项目经历"区块，`section.type` 无法区分，必须用稳定 id。
- **两侧各自持有自己的 DOM**。预览只负责发出定位信号，编辑器只在自己的 DOM 里查找并滚动。`app.tsx` 仅做信号路由，不跨组件 ref 穿透到对方内部结构。
- **锚点靠测试强制，不靠约定**。模板是 slot 架构，新加模板的人极易忘记补锚点属性。因此用 `registry.test.tsx` 的跨模板不变量断言把"记得补"变成 CI 门禁。

## 非目标

- 不做编辑器 → 预览的反向定位。
- 不做滚动跟随（滚动编辑器时预览自动跟随）。
- 不记忆或恢复"上次编辑位置"。
- 不为「专业技能」「荣誉奖项」提供条目级定位（见"已知限制"）。
- 不改后端接口、数据库与 `ResumeDocument` schema。
- 不改动任何模板的视觉排版。

## 现有约束

调查现有代码后确认的、决定实现方式的事实：

- **区块锚点目前只有 type，没有 id**。`ResumeSections.tsx` 的 `DefaultSection` 输出 `data-resume-section={section.type}`；`business` / `navy-gutter` / `blue-sidebar` / `scholarly-serif` 各自重写了 `section` 槽并重复输出同一属性。现有 `registry.test.tsx` 有多处依赖 `[data-resume-section="skills"]` 选择器，因此**新增 id 属性必须与 type 属性并存，不能替换**。
- **条目渲染分两条路径**。`slots.item` 一旦提供，就完全绕过 `entry` 槽。提供 `item` 槽的有 `business` / `tech` / `blue-sidebar` / `mint-canvas` / `navy-gutter` / `scholarly-serif`；只重写 `entry` 槽的有 `blue-sidebar` / `navy-gutter` / `scholarly-serif` / `tech`；其余 `minimal` / `creative` / `editorial` / `redline` / `violet` / `azure-chronicle` 走 `DefaultEntry`。
- **标题槽同样分叉**。重写 `title` 槽的有 `azure-chronicle` / `mint-canvas` / `scholarly-serif` / `blue-sidebar` / `navy-gutter`，其余 7 个走 `DefaultTitle`。
- **部分模板把区块拆到两栏**。`creative` / `editorial` 用 `visible.slice(0, 2)` 与 `visible.slice(2)` 把区块分到 sidebar 与 main。因此预览 DOM 顺序不等于视觉顺序，定位**必须按 id 查找节点，不能依赖文档顺序或 `slice` 分界**。
- **PNG 导出走 html2canvas**。`exportResumePng` 对 `previewRef.current` 调用 `html2canvas`，未设置 `ignoreElements`。任何新增的可视元素都会进画布。
- **打印时编辑器整块隐藏**。`@media print` 下 `.resume-workspace__editor` 为 `display: none`，因此定位高亮不会污染打印产物。
- **桌面端两栏各自独立滚动**。`.resume-workspace__editor` 与 `.resume-preview` 均为 `overflow-y: auto`（`min-width: 901px`）；`≤900px` 时改为整页滚动。前者用 `scrollIntoView` 可同时覆盖两种情况，因为该 API 会滚动所有祖先滚动容器。
- **`SectionCard` 的 `<details>` 是 `open` 常量**。React 仅在 prop 变化时写属性，因此用户手动折叠后不会被后续渲染重新打开。定位到被折叠的区块时需要主动展开。
- **测试环境缺 API**。`src/test/setup.ts` 仅引入 `jest-dom`。jsdom 不实现 `scrollIntoView`，也不实现 `window.matchMedia`。实现侧必须对两者做存在性防御，测试侧再显式打桩以断言行为。

## 方案选型

条目级定位必须让每个条目 DOM 带上 `item.id`，而条目渲染分散在 8 个模板文件中（见上）。两个选项：

- **A. 逐模板补属性**。显式、可读，但把正确性寄托在"改模板的人记得补"这一约定上。新模板静默失效。
- **B. A + 跨模板不变量测试（采纳）**。在 `registry.test.tsx` 增加断言：遍历全部 12 个模板，校验每个可见区块都有 `data-resume-section-id`、每个可见条目都有 `data-resume-item-id`、每个区块标题内都有可聚焦的定位按钮。成本与 A 相同，多一层防漂移保护。

**采纳 B**。现有 `registry.test.tsx` 已经在做同类的跨模板遍历断言（校验各模板的 `data-resume-section` / `data-resume-section-title` 结构），沿用该模式是顺理成章的。

## 交互契约

### 信号链路

```
预览标题按钮 onClick
  → onLocate({ sectionId, itemId? })
  → app.tsx 存入 pendingLocate（附递增 nonce）
  → ResumeEditor 的 useEffect 读取请求
  → 在编辑器自身 root 内按属性查找目标 → 展开 details → scrollIntoView → 高亮 → 1.6s 后移除高亮
```

预览侧的 `onLocate` 只发出 `{ sectionId, itemId? }`，**不含 nonce**；nonce 由 `app.tsx` 在写入 state 时赋予，保证连续点击同一目标也能重新触发。

### 定位请求类型

```ts
export interface LocateRequest {
  sectionId: string;
  itemId?: string;   // 省略时定位到区块级
  nonce: number;     // 每次请求自增，强制重放同一目标
}
```

### 锚点属性契约

| 位置 | 元素 | 属性 | 值 |
|---|---|---|---|
| 预览 | 区块根节点 | `data-resume-section-id` | `section.id` |
| 预览 | 条目根节点 | `data-resume-item-id` | `item.id` |
| 编辑器 | 区块 `<details>` | `data-editor-section-id` | `section.id` |
| 编辑器 | 条目 `.editor-card` | `data-editor-item-id` | `item.id` |

四个属性名以常量形式从共享模块导出，模板与编辑器共用同一份定义，避免字符串漂移。

## 组件改动

### 新增 `frontend/src/components/ResumeLocate.tsx`

共享契约模块，供预览与编辑器同时引用：

- 导出四个锚点属性名常量。
- `SectionLocateButton`：包裹区块标题文本的按钮，`onClick` 触发区块级定位。
- `EntryLocateButton`：包裹条目标题文本的按钮，`onClick` 触发条目级定位，同时携带所属 `sectionId`。
- 二者均接受 `label`（用于 `aria-label`）与 `children`（标题文本）。

### 新增 `frontend/src/hooks/useLocateTarget.ts`

编辑器侧消费定位请求的自定义 hook。入参为请求与编辑器 root ref；命中时依次完成：按 `data-editor-item-id` 优先查找条目、否则按 `data-editor-section-id` 查找区块 → 若目标在 `<details>` 内且处于折叠则展开 → `scrollIntoView` → 追加高亮 class → 超时移除。

必须对 `scrollIntoView` 与 `window.matchMedia` 做存在性防御（jsdom 与极旧浏览器环境）。

### `ResumeSections.tsx`

- `DefaultSection` 增加 `data-resume-section-id`。
- `DefaultEntry` 增加 `data-resume-item-id`；`ItemSlotProps.item` 当前类型为 `unknown`，需收窄为 `{ id?: string }` 并仅在 id 存在时输出属性。
- `DefaultTitle` 改为 `<h2 data-resume-section-title>{<SectionLocateButton ...>{children}</SectionLocateButton>}</h2>`，标题语义保留。
- `ExperienceEntry` / `ProjectEntry` / `CampusEntry` 及 education 条目的 `<h3>` 同样包 `EntryLocateButton`。
- `ResumeSectionsProps` 增加可选的 `onLocate` 回调，与各槽组件透传。

### 7 个模板文件

| 文件 | 需补 `section` | 需补 `title` | 需补 `entry` / `item` |
|---|---|---|---|
| `business.tsx` | 是（内联槽） | 否（走默认） | 是（`item` 槽） |
| `tech.tsx` | 否 | 否 | 是（`entry` + `item` 槽） |
| `blue-sidebar.tsx` | 是 | 是 | 是（`entry` + `item` 槽） |
| `navy-gutter.tsx` | 是 | 是 | 是（`entry` + `item` 槽） |
| `scholarly-serif.tsx` | 是 | 是 | 是（`entry` + `item` 槽） |
| `mint-canvas.tsx` | 否 | 是 | 是（`item` 槽） |
| `azure-chronicle.tsx` | 否 | 是（`AzureTitle`） | 否 |
| `minimal` / `creative` / `editorial` / `redline` / `violet` | 否 | 否 | 否 |

各模板需把 `onLocate` 从 props 透传到自己的槽组件与条目标题。

### `ResumeEditor.tsx`

- `SectionCard` 的 `<details>` 增加 `data-editor-section-id`。
- 7 个条目编辑器（`ExperienceItemsEditor` / `ProjectItemsEditor` / `EducationItemsEditor` / `SkillsItemsEditor` / `CampusItemsEditor` / `HonoursItemsEditor`，以及 `SimpleItemsEditor` 公共路径）产出的 `.editor-card` 增加 `data-editor-item-id`。
- `ResumeEditor` 接收 `locateRequest` 与自身 `rootRef`，交由 `useLocateTarget` 处理。

### `ResumePreview.tsx` / `app.tsx`

- `ResumePreview` 接收 `onLocate` 并透传给模板 `Renderer`。
- `app.tsx` 持有 `pendingLocate` state，接收预览的 `onLocate` 后写入带自增 `nonce` 的请求，传给 `ResumeEditor`。

## 滚动与高亮细节

- **滚动**：`scrollIntoView({ behavior, block: 'start' })`。`behavior` 取 `smooth`，但当 `matchMedia('(prefers-reduced-motion: reduce)').matches` 为真时取 `auto`。`block: 'start'` 同时适配桌面独立滚动容器与移动端整页滚动。
- **移动端留白**：`.editor-section` 与 `.editor-card` 增加 `scroll-margin-top`，避免移动端滚动后目标被吸顶 header 遮挡。
- **折叠处理**：命中目标若位于折叠的 `<details>` 内，先设 `open = true` 再滚动，否则会停在折叠标题上。
- **高亮**：命中元素追加 `is-located` class，1600ms 后移除；在 `prefers-reduced-motion: reduce` 下改为静态描边而非动画。连续请求会先清理上一次的高亮定时器，避免 class 提前消失。

## 无障碍

- 区块与条目定位使用原生 `<button>`，置于原 `<h2>` / `<h3>` 内部：既保留标题语义（读屏仍按标题朗读），又获得原生按钮语义与键盘可达性。
- 按钮 `aria-label` 形如「定位到编辑器的『项目经历』」「定位到编辑器的『AI 创作副驾』」。
- 按钮保持 `background: none; border: 0; padding: 0; font: inherit; color: inherit`，视觉与原标题完全一致。
- `focus-visible` 提供与项目一致的 3px 焦点描边（对齐 `global.css` 现有 `.editor-navigation button:focus-visible` 规则），满足 `AGSIN.md` 对焦点样式与无障碍名称的要求。
- 命中数量为 0 时静默无操作，不报错、不打断用户。

## 导出与打印

这是本方案唯一的真实风险点，按"实测而非假设"处理。

- 定位按钮**不得**加 `data-html2canvas-ignore`——那会把标题文字一并从 PNG 画布中抹掉。正确做法是保证按钮透明、无边框、继承字体与颜色，使其在 html2canvas 下的绘制结果与普通标题文字一致。
- 必须补一条 `exportResumePng` 测试，断言导出后画布中标题文字仍然存在，防止后续样式调整把按钮外观带进 PNG。
- `@media print` 下编辑器整块隐藏，高亮不会出现在打印产物中；按钮本身在打印时按普通标题渲染。
- 项目链接点击不受影响：定位控件只包裹标题文本，不覆盖区块正文，因此预览正文的文字选中与链接点击行为保持不变。

## 已知限制（明确接受）

- 「专业技能」的条目渲染为并排 chip、「荣誉奖项」的条目渲染为 `<li>`，均无独立标题层级。在 chip / `<li>` 上挂按钮会带来过小的点击区域，违反 `AGSIN.md` 对点击面积的要求，因此这两类**只提供区块级定位**，不提供条目级定位。
- 「个人总结」为单一 textarea，无条目概念，同样只有区块级定位。
- 条目级定位不跨区块：预览中某个条目点击后，只会定位到其所属区块内的该条目。

## 测试策略

- **`ResumeSections.test.tsx`**：默认槽输出 `data-resume-section-id` / `data-resume-item-id`；标题内包含可聚焦的定位按钮并带正确 `aria-label`。
- **`registry.test.tsx`（跨模板不变量，方案 B 的核心）**：遍历全部 12 个模板，断言每个可见区块都有 `data-resume-section-id`、每个可见条目都有 `data-resume-item-id`、每个区块标题内都有定位按钮。此断言使新模板漏补锚点在 CI 即刻失败。
- **`ResumeEditor.test.tsx`**：编辑侧锚点属性正确输出；定位命中折叠区块时自动展开。
- **`app.test.tsx`（集成）**：点击预览中的区块标题 → 编辑器收到带自增 `nonce` 的定位请求；点击条目标题 → 请求携带正确 `sectionId` 与 `itemId`。
- **`useLocateTarget` 测试**：jsdom 不实现 `scrollIntoView` 与 `matchMedia`，测试需显式打桩；覆盖 reduced-motion 走 `auto`、普通情况走 `smooth`、目标不存在时静默返回三条路径。
- **`global.test.js`**：沿用现有 CSS 断言模式，校验 `scroll-margin-top`、`is-located` 高亮与 reduced-motion 规则存在。

## 交付验证

- `cd frontend && npx vitest run` 全部通过。
- `cd frontend && npm run build`（`tsc -b` + `vite build`）通过。
- 手工核对 12 个模板在区块级与条目级定位下均正常滚动，且模板视觉排版无变化。
- 手工核对 PNG 导出产物的标题文字与排版与改动前一致。
