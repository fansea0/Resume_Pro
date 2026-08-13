# Sample Resume Experience Implementation Plan

> **For implementer:** Use TDD throughout. Write failing test first. Watch it fail. Then implement.

**Goal:** Show a realistic, complete sample resume at first launch while preserving every user edit and saved resume.

**Architecture:** Add a `createSampleResumeDocument` factory alongside the existing empty factory. Initialize only the in-memory app state with that factory; saving remains explicit. Expose a restore-sample action which replaces only the currently edited in-memory document and never modifies a saved record until the user explicitly saves.

**Tech Stack:** React, TypeScript, Vitest, React Testing Library.

---

### Task 1: Add a complete sample resume factory

**Files:**
- Modify: `frontend/src/types/resume.ts`
- Modify: `frontend/src/types/resume.test.ts`

**Step 1: Write the failing test**

Add a test for `createSampleResumeDocument` which expects a non-placeholder name, summary, contact details, at least one experience, one project, one education item, and skills. Assert IDs are non-empty and the default template ID is `redline`.

**Step 2: Run test — confirm it fails**

Command: `cd frontend && npm test -- --run src/types/resume.test.ts`

Expected: FAIL because `createSampleResumeDocument` is not exported.

**Step 3: Write minimal implementation**

Return one static, realistic internet-industry sample document with unique IDs. Keep the existing `createEmptyResumeDocument` unchanged for future blank-document use.

**Step 4: Run test — confirm it passes**

Command: `cd frontend && npm test -- --run src/types/resume.test.ts`

Expected: PASS.

**Step 5: Commit**

`git add frontend/src/types/resume.ts frontend/src/types/resume.test.ts && git commit -m "feat: add sample resume document"`

### Task 2: Initialize preview with sample data and add restore action

**Files:**
- Modify: `frontend/src/app.tsx`
- Modify: `frontend/src/components/ResumeLibrary.tsx`
- Modify: `frontend/src/components/ResumeLibrary.test.tsx`
- Modify: `frontend/src/styles/global.css`

**Step 1: Write the failing test**

Render `App` with mocked resume API. Assert sample candidate content appears in preview. Change the title, click `恢复示例`, and assert the sample title is restored; assert no POST/PUT API request occurs.

**Step 2: Run test — confirm it fails**

Command: `cd frontend && npm test -- --run src/components/ResumeLibrary.test.tsx`

Expected: FAIL because no restore action exists and the app starts with empty document content.

**Step 3: Write minimal implementation**

Initialize `App` with `createSampleResumeDocument`. Add `onRestoreSample` to `ResumeLibrary`, wire it to a clearly labelled `恢复示例内容` button, and set state to a fresh sample factory result. Do not call persistence APIs from this action. Explain near the button that the sample is saved only if the user chooses save.

**Step 4: Run test — confirm it passes**

Command: `cd frontend && npm test -- --run && npm run build`

Expected: all frontend tests pass and production build succeeds.

**Step 5: Commit**

`git add frontend/src/app.tsx frontend/src/components/ResumeLibrary.tsx frontend/src/components/ResumeLibrary.test.tsx frontend/src/styles/global.css && git commit -m "feat: show sample resume by default"`
