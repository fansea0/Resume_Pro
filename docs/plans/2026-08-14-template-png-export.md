# Real Template PNG Previews and Export Plan

> **For implementer:** Use TDD throughout. Write failing test first. Watch it fail. Then implement.

**Goal:** Show real, full-content sample-template PNGs in the gallery and let users export their currently edited resume as a PNG.

**Architecture:** Gallery paths point only to pre-rendered A4 PNG snapshots made from `createSampleResumeDocument`, not SVG mockups. `ResumePreview` exposes its paper node by ref; an isolated export helper uses `html2canvas` to rasterize that node and triggers a named PNG download. PDF remains browser-print based.

**Tech Stack:** React, TypeScript, html2canvas, Vitest, React Testing Library, static PNG assets.

---

### Task 1: Use real static PNG snapshots in the gallery

**Files:**
- Modify: `frontend/src/templates/registry.tsx`
- Modify: `frontend/src/templates/registry.test.tsx`
- Modify: `frontend/src/components/TemplateGallery.test.tsx`
- Create: `frontend/public/template-previews/<id>.png` for each registered template

**Step 1:** Add a failing registry assertion that each preview path exactly equals `/template-previews/<id>.png`.

**Step 2:** Run `cd frontend && npm test -- --run src/templates/registry.test.tsx`; confirm it fails while paths end in `.svg`.

**Step 3:** Replace each gallery preview with a full static PNG generated from the fixed sample document and matching template layout. Update registry paths; do not use user data or live rendering in gallery.

**Step 4:** Run registry and gallery tests; expect PASS.

**Step 5:** Commit real preview assets and registry update.

### Task 2: Export the active resume preview as PNG

**Files:**
- Modify: `frontend/package.json`
- Modify: `frontend/package-lock.json`
- Create: `frontend/src/utils/exportResumePng.ts`
- Create: `frontend/src/utils/exportResumePng.test.ts`
- Modify: `frontend/src/components/ResumePreview.tsx`
- Modify: `frontend/src/components/ResumeLibrary.tsx`
- Modify: `frontend/src/components/ResumeLibrary.test.tsx`

**Step 1:** Add a failing unit test that, given a preview element and title, calls the canvas renderer with scale 2 and invokes a PNG download named from the document title.

**Step 2:** Run `cd frontend && npm test -- --run src/utils/exportResumePng.test.ts`; confirm it fails because the helper is absent.

**Step 3:** Add `html2canvas`; implement export helper with white background, scale 2, CORS-safe handling, and URL cleanup. Give `ResumePreview` a forwarded ref to the template paper node. Add an `导出 PNG` control beside `导出 PDF`; disable while exporting and show a recoverable error on failure.

**Step 4:** Run `cd frontend && npm test -- --run && npm run build`; expect PASS.

**Step 5:** Commit PNG export implementation.
