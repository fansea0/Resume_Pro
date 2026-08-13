# Template Gallery Implementation Plan

> **For implementer:** Use TDD throughout. Write failing test first. Watch it fail. Then implement.

**Goal:** Replace the crowded inline template selector with a static-preview template gallery and a focused one-template editing workspace.

**Architecture:** `App` owns a two-state view: `gallery` or `editor`. The gallery maps template registry metadata to immutable preview-image URLs in `public/template-previews`, while the editor renders only the selected template using live resume data. Selection updates `templateId` without resetting document content.

**Tech Stack:** React, TypeScript, CSS, SVG static assets, Vitest, React Testing Library.

---

### Task 1: Add static template preview assets and gallery metadata

**Files:**
- Create: `frontend/public/template-previews/minimal.svg`
- Create: `frontend/public/template-previews/business.svg`
- Create: `frontend/public/template-previews/creative.svg`
- Create: `frontend/public/template-previews/tech.svg`
- Create: `frontend/public/template-previews/editorial.svg`
- Create: `frontend/public/template-previews/redline.svg`
- Create: `frontend/public/template-previews/violet.svg`
- Modify: `frontend/src/templates/registry.tsx`
- Modify: `frontend/src/templates/registry.test.tsx`

**Step 1:** Add a failing test requiring every registry entry to define a preview image path beginning `/template-previews/`.

**Step 2:** Run `cd frontend && npm test -- --run src/templates/registry.test.tsx`; confirm it fails because preview metadata is absent.

**Step 3:** Add `previewImage` to registry metadata and create original lightweight SVG covers that visually signal each template's color/layout without rendering resume components or user content.

**Step 4:** Re-run the test; expect PASS.

**Step 5:** Commit `git add frontend/public/template-previews frontend/src/templates && git commit -m "feat: add static template preview covers"`.

### Task 2: Build the gallery view and focused editor navigation

**Files:**
- Create: `frontend/src/components/TemplateGallery.tsx`
- Create: `frontend/src/components/TemplateGallery.test.tsx`
- Modify: `frontend/src/app.tsx`
- Modify: `frontend/src/styles/global.css`
- Delete: `frontend/src/components/TemplatePicker.tsx`

**Step 1:** Write a failing component/App test: gallery initially presents static image cards for every template; clicking `使用此模板` switches to editor and preserves sample document name; clicking `返回模板库` returns to gallery.

**Step 2:** Run `cd frontend && npm test -- --run src/components/TemplateGallery.test.tsx`; confirm it fails because gallery is absent.

**Step 3:** Implement accessible gallery cards with `<img>` preview images, name, description, and select CTA. Set app's initial view to gallery. In editor, remove the inline picker and add a compact return/change-template control. Do not render `ResumePreview` in gallery view.

**Step 4:** Run `cd frontend && npm test -- --run && npm run build`; expect all tests/build PASS.

**Step 5:** Commit `git add frontend/src && git commit -m "feat: add template gallery workflow"`.

### Task 3: Verify responsive layout and cleanup

**Files:**
- Modify: `frontend/src/styles/global.css`
- Modify: `frontend/src/components/TemplateGallery.test.tsx`

**Step 1:** Add a failing test checking preview image alt text and selected-template state label.

**Step 2:** Run `cd frontend && npm test -- --run src/components/TemplateGallery.test.tsx`; confirm it fails.

**Step 3:** Add responsive grid rules (one, two, three cards based on viewport), keyboard focus styles, fixed aspect ratio covers, and clear current-template context in editor. Ensure preview images are object-fit contained and not generated from live data.

**Step 4:** Run `cd frontend && npm test -- --run && npm run build`; expect PASS.

**Step 5:** Commit `git add frontend/src && git commit -m "test: verify template gallery accessibility"`.
