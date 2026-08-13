# Template Snapshot Previews Implementation Plan

> **For implementer:** Use TDD throughout. Write failing test first. Watch it fail. Then implement.

**Goal:** Make every gallery preview a complete static A4 rendering of the shared sample resume in its actual template layout.

**Architecture:** A repeatable local snapshot script renders each registered template with `createSampleResumeDocument` and saves a static image under `frontend/public/template-previews`. The registry points to those image assets; the gallery only loads files and never renders a candidate's live document.

**Tech Stack:** React, Vite, local browser screenshot tooling, PNG/WebP assets, Vitest.

---

### Task 1: Establish preview image contract

**Files:**
- Modify: `frontend/src/templates/registry.test.tsx`
- Modify: `frontend/src/templates/registry.tsx`
- Create: `frontend/scripts/generate-template-previews.mjs`

**Step 1:** Add a failing assertion that every `previewImage` points to `/template-previews/<id>.png` and the basename matches its template ID.

**Step 2:** Run `cd frontend && npm test -- --run src/templates/registry.test.tsx`; confirm it fails while registry paths use SVG covers.

**Step 3:** Add the snapshot script. It starts/uses the local app preview, selects each registered renderer with the fixed sample document, waits for fonts/layout, and writes one A4-ratio PNG per ID. Keep all source data static and never include a user's saved document.

**Step 4:** Update registry preview paths to PNG and re-run the test; expect PASS.

**Step 5:** Commit the script and contract update.

### Task 2: Generate and use full A4 template snapshots

**Files:**
- Create: `frontend/public/template-previews/<id>.png` for every registered ID
- Modify: `frontend/src/components/TemplateGallery.test.tsx`
- Modify: `frontend/src/styles/global.css`

**Step 1:** Add a failing gallery test requiring preview images to use static PNG sources.

**Step 2:** Run `cd frontend && npm test -- --run src/components/TemplateGallery.test.tsx`; confirm it fails before paths change.

**Step 3:** Run the generator, inspect representative screenshots, and retain complete A4 content previews. Adjust gallery cover styling only to show the entire image rather than crop it.

**Step 4:** Run `cd frontend && npm test -- --run && npm run build`; expect all tests and the build to pass.

**Step 5:** Commit generated previews and gallery validation.
