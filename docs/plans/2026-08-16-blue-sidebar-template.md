# Blue Sidebar Template Implementation Plan

> **For implementer:** Use TDD throughout. Write failing test first. Watch it fail. Then implement.

**Goal:** Add the editable and print-safe `blue-sidebar` React resume template with an original A4 gallery cover.

**Architecture:** The renderer delegates every persisted resume section to `ResumeSections`, preserving user-controlled order and visibility. Template-specific classes own the blue identity sidebar, wine-red rules, compact entries and responsive behavior. A standalone SVG cover supplies the gallery preview without using live resume data.

**Tech Stack:** React, TypeScript, Vitest, CSS, static SVG assets.

---

### Task 1: Register the template contract

**Files:**
- Modify: `frontend/src/types/resume.ts`
- Modify: `frontend/src/templates/registry.tsx`
- Modify: `frontend/src/templates/registry.test.tsx`

**Step 1: Write the failing test**

Add `blue-sidebar` to the expected registry order and assert it renders the landmark `.template-blue-sidebar__sidebar`, shared profile name, and work content.

**Step 2: Run test — confirm it fails**

Command: `cd frontend && npm test -- --run src/templates/registry.test.tsx`

Expected: FAIL because `blue-sidebar` does not exist in the `TemplateId` union or registry.

**Step 3: Write minimal implementation**

Add the stable template ID and a registry entry with Chinese name, three role tags, style description, cover path, and renderer import.

**Step 4: Run test — confirm it passes**

Command: `cd frontend && npm test -- --run src/templates/registry.test.tsx`

Expected: PASS after Task 2 implementation is present.

### Task 2: Build the renderer and CSS

**Files:**
- Create: `frontend/src/templates/blue-sidebar.tsx`
- Modify: `frontend/src/templates/templates.css`

**Step 1: Keep the registry test failing**

The new test must remain red until the renderer is imported and outputs the landmark class.

**Step 2: Write minimal implementation**

Render `ResumeAvatar`, profile identity/contact fields and `ResumeSections` within a two-column A4 article. Use class names for all section, entry, metadata, skills and honours surfaces. Add scoped CSS variables, print-safe entry breaks, and the small-screen stacked layout.

**Step 3: Run test — confirm it passes**

Command: `cd frontend && npm test -- --run src/templates/registry.test.tsx`

Expected: PASS.

### Task 3: Add the static gallery cover and verify

**Files:**
- Create: `frontend/public/template-previews/blue-sidebar.svg`

**Step 1: Add the static asset**

Create an original 210 × 297 SVG A4 snapshot with fictional identity, summary, two experience entries, a project, education and skills. Use a neutral monogram rather than a portrait.

**Step 2: Verify**

Commands:

```bash
cd frontend && npm test -- --run src/templates/registry.test.tsx
cd frontend && npm run build
```

Expected: Tests and TypeScript/Vite build pass.

### Task 4: Visual review

**Files:**
- Review only: `frontend/src/templates/blue-sidebar.tsx`
- Review only: `frontend/src/templates/templates.css`
- Review only: `frontend/public/template-previews/blue-sidebar.svg`

**Step 1: Inspect**

Open the static cover and a browser-rendered template. Confirm full sidebar contrast, no fixed-height clipping, readable metadata, and suitable A4 density.

**Step 2: Adjust measurable mismatch only**

Correct hierarchy, contrast, overflow, or responsive stacking as needed, then repeat the verification commands.
