# 模板主页与我的草稿 Implementation Plan

> **For implementer:** Use TDD throughout. Write failing test first. Watch it fail. Then implement.

**Goal:** 登录用户可在模板主页与个人草稿页间导航，并点击自己的草稿直接继续编辑。

**Architecture:** `App` 保持单页状态，新增 `drafts` 视图和 `ResumeDrafts` 组件。草稿组件只调用现有、已由 JWT 租户隔离的 `listResumes()`，再将所选 `StoredResume` 交回 `App` 解码并打开编辑器。

**Tech Stack:** React 18、TypeScript、Vite、Vitest、React Testing Library、现有 Fetch API。

---

### Task 1: 实现我的草稿列表组件

**Files:**
- Create: `frontend/src/components/ResumeDrafts.tsx`
- Create: `frontend/src/components/ResumeDrafts.test.tsx`

**Step 1: Write the failing test**

在 `ResumeDrafts.test.tsx` mock `listResumes` 并添加：

```tsx
it('lists only returned drafts and continues editing the selected draft', async () => {
  listResumes.mockResolvedValueOnce([{ id: 7, title: '产品经理简历', templateId: 'business', content: '{}', createdAt: '2026-08-14T00:00:00Z', updatedAt: '2026-08-14T01:00:00Z' }]);
  const onSelect = vi.fn();
  render(<ResumeDrafts onCreate={() => undefined} onSelect={onSelect} />);
  expect(await screen.findByText('产品经理简历')).toBeInTheDocument();
  fireEvent.click(screen.getByRole('button', { name: '继续编辑：产品经理简历' }));
  expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ id: 7 }));
});
```

另写空状态（`listResumes` 返回空数组）与失败状态（reject）测试。

**Step 2: Run test — confirm it fails**

Command: `cd frontend && npm test -- --run src/components/ResumeDrafts.test.tsx`

Expected: FAIL — cannot resolve `./ResumeDrafts`.

**Step 3: Write minimal implementation**

实现组件：挂载时加载 `listResumes()`，展示标题、由 `templateRegistry` 找到的模板名、用 `Intl.DateTimeFormat('zh-CN')` 格式化的更新时间；加载、空、错误状态各有清晰文案。空状态按钮调用 `onCreate`；每个草稿的“继续编辑”按钮调用 `onSelect(item)`。

**Step 4: Run test — confirm it passes**

Command: `cd frontend && npm test -- --run src/components/ResumeDrafts.test.tsx`

Expected: PASS.

**Step 5: Commit**

`git add frontend/src/components/ResumeDrafts.tsx frontend/src/components/ResumeDrafts.test.tsx && git commit -m "feat: add personal resume drafts page"`

### Task 2: 将草稿页接入应用导航

**Files:**
- Modify: `frontend/src/app.tsx`
- Modify: `frontend/src/app.test.tsx`
- Modify: `frontend/src/components/TemplateGallery.test.tsx`
- Modify: `frontend/src/styles/global.css`

**Step 1: Write the failing integration test**

在 `app.test.tsx` mock `listResumes`，新增测试：认证后存在“模板主页”和“我的草稿”导航；点击“我的草稿”显示“我的草稿”；点击“继续编辑：产品经理简历”后出现 resume preview 和“当前模板：现代商务”。再增加空草稿的“去选择模板”返回模板主页测试。

**Step 2: Run test — confirm it fails**

Command: `cd frontend && npm test -- --run src/app.test.tsx`

Expected: FAIL — missing navigation or drafts heading.

**Step 3: Write minimal implementation**

将 `view` 联合类型扩展为 `gallery | drafts | editor`。在认证后的 header 添加两个导航按钮（使用 `aria-current="page"` 标识当前页）。渲染 `ResumeDrafts`：其 `onCreate` 设置示例简历并进入模板主页；其 `onSelect` 通过 `documentFromStored` 设置简历并进入编辑器，解析失败则保持草稿页。编辑器导航提供“返回模板主页”和“我的草稿”。为导航、草稿卡、空状态补充响应式样式，不修改模板预览画面。

**Step 4: Run test — confirm it passes**

Command: `cd frontend && npm test -- --run src/app.test.tsx src/components/TemplateGallery.test.tsx`

Expected: PASS.

**Step 5: Commit**

`git add frontend/src/app.tsx frontend/src/app.test.tsx frontend/src/components/TemplateGallery.test.tsx frontend/src/styles/global.css && git commit -m "feat: navigate between template home and drafts"`

### Task 3: 全量验证与回归修复

**Files:**
- Modify only if required by a failing test: `frontend/src/components/ResumeLibrary.test.tsx`

**Step 1: Run full frontend verification**

Command: `cd frontend && npm test -- --run && npm run build`

Expected: all Vitest tests pass and Vite production build completes.

**Step 2: Fix only discovered expectation drift**

若既有测试依赖旧的无导航结构，先让其失败，再仅调整断言到确认后的交互；不得改变已定义的行为或扩大范围。

**Step 3: Re-run verification**

Command: `cd frontend && npm test -- --run && npm run build`

Expected: PASS.

**Step 4: Commit**

`git add <only regression test files changed> && git commit -m "test: cover drafts navigation workflow"`
