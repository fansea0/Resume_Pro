# Premium Resume Templates Implementation Plan

> **For implementer:** Use TDD throughout. Write failing test first. Watch it fail. Then implement.

**Goal:** Add visually distinctive dark-technology and editorial resume templates that work with the existing editor, data model, and A4 print export.

**Architecture:** Two independent React template renderers consume the existing `ResumeDocument` and enter the application solely through `templateRegistry`. CSS adds isolated template namespaces without altering the shared data model or API.

**Tech Stack:** React, TypeScript, CSS, Vitest, React Testing Library.

---

### Task 1: Add dark technology and editorial template renderers

**Files:**
- Create: `frontend/src/templates/tech.tsx`
- Create: `frontend/src/templates/editorial.tsx`
- Modify: `frontend/src/templates/registry.tsx`
- Modify: `frontend/src/templates/templates.css`
- Modify: `frontend/src/templates/registry.test.tsx`

**Step 1: Write the failing test**

Extend the registry test so it expects five ordered IDs (`minimal`, `business`, `creative`, `tech`, `editorial`) and renders `tech` and `editorial`, asserting the sample full name, sample company, and each template's distinctive heading text.

**Step 2: Run test — confirm it fails**

Command: `cd frontend && npm test -- --run src/templates/registry.test.tsx`

Expected: FAIL because `tech` and `editorial` are not registered.

**Step 3: Write minimal implementation**

Create a `TechTemplate` with a dark navy identity header, cyan accent timeline, compact skill chips, and all existing resume sections. Create an `EditorialTemplate` with large type-led identity, warm neutral canvas, fine dividers, and sidebar content. Register IDs `tech` and `editorial` with Chinese names and concise descriptions. Add isolated responsive and `@media print` CSS; use existing document fields only.

**Step 4: Run test — confirm it passes**

Command: `cd frontend && npm test -- --run src/templates/registry.test.tsx && npm run build`

Expected: PASS and production build succeeds.

**Step 5: Commit**

`git add frontend/src/templates && git commit -m "feat: add premium resume templates"`

### Task 2: Verify all template regressions and print integration

**Files:**
- Modify: `frontend/src/templates/registry.test.tsx`
- Modify: `frontend/src/styles/global.css`

**Step 1: Write the failing test**

Add an assertion that the registry has five unique IDs and each registered renderer can render the sample document without errors.

**Step 2: Run test — confirm it fails**

Command: `cd frontend && npm test -- --run src/templates/registry.test.tsx`

Expected: FAIL before the generalized assertion exists.

**Step 3: Write minimal implementation**

Ensure the shared A4 print rule permits both premium template roots to use their essential visual accents without displaying editor-only layout. Keep template content blocks together using `break-inside: avoid`.

**Step 4: Run test — confirm it passes**

Command: `cd frontend && npm test -- --run && npm run build`

Expected: all frontend tests pass and the production build succeeds.

**Step 5: Commit**

`git add frontend/src/templates/registry.test.tsx frontend/src/styles/global.css && git commit -m "test: verify premium template rendering"`
