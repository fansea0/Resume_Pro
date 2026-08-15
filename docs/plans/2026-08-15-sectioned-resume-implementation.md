# Sectioned Resume Implementation Plan

> **For implementer:** Use TDD throughout. Write failing test first. Watch it fail. Then implement.

**Goal:** Make a resume a persistable, ordered collection of typed, hideable sections that every template can render without reducing the fields the user may provide.

**Architecture:** The `ResumeDocument` v2 schema owns semantic sections and ordering. `documentFromStored` accepts canonical v2 content and migrates the legacy fixed-array content in memory. The editor edits v2 sections; a shared renderer converts each visible section to markup while individual templates supply visual primitives and CSS.

**Tech Stack:** React 18, TypeScript, Vitest, Testing Library, `react-markdown` with `remark-gfm` (without raw HTML support).

---

### Task 1: Define and create v2 section documents

**Files:**

- Modify: `frontend/src/types/resume.ts`
- Modify: `frontend/src/types/resume.test.ts`

**Step 1: Write the failing test**

Add tests for `createEmptyResumeDocument()` returning `schemaVersion: 2` and
`sections: []`, and `createSampleResumeDocument()` containing each of the eight
section types with a stable unique id and default `visible: true`.

**Step 2: Run test — confirm it fails**

Command: `cd frontend && npm test -- src/types/resume.test.ts`

Expected: FAIL because `schemaVersion` and `sections` do not exist.

**Step 3: Write minimal implementation**

Replace the legacy top-level content arrays with the discriminated types below;
retain `ResumeProfile` contact fields only (summary moves into a section).

```ts
type ResumeSection = SummarySection | ExperienceSection | ProjectSection |
  EducationSection | SkillsSection | CampusSection | HonoursSection;
// Each variant extends { id, type, title, visible } and owns { items } or
// { markdown }. Work and internship use the same ExperienceSection shape.
```

Add section factories for all eight types. They must initialise every string,
visibility flag and items array, so editing never relies on `undefined`.

**Step 4: Run test — confirm it passes**

Command: `cd frontend && npm test -- src/types/resume.test.ts`

Expected: PASS.

**Step 5: Commit**

`git add frontend/src/types/resume.ts frontend/src/types/resume.test.ts && git commit -m "feat: define sectioned resume document"`

### Task 2: Validate and migrate stored content

**Files:**

- Modify: `frontend/src/api/resumes.ts`
- Modify: `frontend/src/api/resumes.test.ts`

**Step 1: Write the failing test**

Add one test that restores a fully populated v2 document, one that restores
each v2 section type, and one that restores an existing legacy document and
asserts the generated order is summary, work, project, education, skills.
Also assert that a malformed variant is rejected.

**Step 2: Run test — confirm it fails**

Command: `cd frontend && npm test -- src/api/resumes.test.ts`

Expected: FAIL because the existing validator only recognises fixed arrays.

**Step 3: Write minimal implementation**

Implement narrow runtime guards for every v2 section variant, including all
required strings and boolean display flags. Add `migrateLegacyDocument()` that
recognises the current profile/experiences/projects/education/skills shape,
maps it into v2 factories, moves `profile.summary` into the summary section,
maps legacy highlights to Markdown bullet text, and returns `null` for invalid
legacy data. `documentFromStored()` must prefer valid v2 content, otherwise
attempt migration, then overlay the envelope title/template id.

**Step 4: Run test — confirm it passes**

Command: `cd frontend && npm test -- src/api/resumes.test.ts`

Expected: PASS.

**Step 5: Commit**

`git add frontend/src/api/resumes.ts frontend/src/api/resumes.test.ts && git commit -m "feat: migrate legacy resume content"`

### Task 3: Add a safe shared Markdown renderer

**Files:**

- Modify: `frontend/package.json`
- Modify: `frontend/package-lock.json`
- Create: `frontend/src/components/ResumeMarkdown.tsx`
- Create: `frontend/src/components/ResumeMarkdown.test.tsx`

**Step 1: Write the failing test**

Test bold, emphasis, a link, line breaks, unordered lists, and ordered lists.
Test that an HTML tag is rendered as text rather than executed or inserted as
raw markup.

**Step 2: Run test — confirm it fails**

Command: `cd frontend && npm test -- src/components/ResumeMarkdown.test.tsx`

Expected: FAIL because the component does not exist.

**Step 3: Write minimal implementation**

Install `react-markdown` and `remark-gfm`. Implement `ResumeMarkdown` with
only the GFM plugin; do not install or configure `rehype-raw`. Give generated
paragraphs and lists stable semantic class names for template CSS hooks.

**Step 4: Run test — confirm it passes**

Command: `cd frontend && npm test -- src/components/ResumeMarkdown.test.tsx`

Expected: PASS.

**Step 5: Commit**

`git add frontend/package.json frontend/package-lock.json frontend/src/components/ResumeMarkdown.tsx frontend/src/components/ResumeMarkdown.test.tsx && git commit -m "feat: render safe resume markdown"`

### Task 4: Build the section editor

**Files:**

- Modify: `frontend/src/components/ResumeEditor.tsx`
- Modify: `frontend/src/components/ResumeEditor.test.tsx`
- Modify: `frontend/src/styles/global.css` (only if necessary; preserve pre-existing user edits)

**Step 1: Write the failing test**

Add tests covering: add a work and internship section; move a section;
toggle a section's visibility; edit a work entry's location/achievement
visibility; add a project with all four optional visibility switches; add a
campus entry and honour; and update Markdown content.

**Step 2: Run test — confirm it fails**

Command: `cd frontend && npm test -- src/components/ResumeEditor.test.tsx`

Expected: FAIL because the old editor only edits fixed arrays.

**Step 3: Write minimal implementation**

Replace fixed section forms with reusable section-card controls: add by type,
edit heading, move up/down, show/hide, and delete. Dispatch to type-specific
content forms. Reuse one `ExperienceItemsEditor` for work and internship;
introduce project, education, skills, campus, honours, and summary forms.
Use immutable updates keyed by section and item id. A hidden optional field
stays editable and preserved but is not previewed.

**Step 4: Run test — confirm it passes**

Command: `cd frontend && npm test -- src/components/ResumeEditor.test.tsx`

Expected: PASS.

**Step 5: Commit**

`git add frontend/src/components/ResumeEditor.tsx frontend/src/components/ResumeEditor.test.tsx && git commit -m "feat: edit ordered resume sections"`

### Task 5: Provide shared semantic section rendering

**Files:**

- Create: `frontend/src/templates/ResumeSections.tsx`
- Create: `frontend/src/templates/ResumeSections.test.tsx`

**Step 1: Write the failing test**

Render a document that contains all eight section types in a deliberately
nonstandard order. Assert that the visible headings and content follow the
array order, and that hidden sections and hidden optional fields are absent.

**Step 2: Run test — confirm it fails**

Command: `cd frontend && npm test -- src/templates/ResumeSections.test.tsx`

Expected: FAIL because no shared semantic renderer exists.

**Step 3: Write minimal implementation**

Implement `ResumeSections` as an ordered discriminated-union switch. Accept
template-owned slots for heading, entry, metadata, role, and honours wrappers.
Use `ResumeMarkdown` for summary, bullet descriptions, and campus content.
Never hard-code a section order; only `section.visible` and field visibility
flags decide whether material is rendered.

**Step 4: Run test — confirm it passes**

Command: `cd frontend && npm test -- src/templates/ResumeSections.test.tsx`

Expected: PASS.

**Step 5: Commit**

`git add frontend/src/templates/ResumeSections.tsx frontend/src/templates/ResumeSections.test.tsx && git commit -m "feat: render semantic resume sections"`

### Task 6: Migrate every template to the shared renderer

**Files:**

- Modify: `frontend/src/templates/minimal.tsx`
- Modify: `frontend/src/templates/business.tsx`
- Modify: `frontend/src/templates/creative.tsx`
- Modify: `frontend/src/templates/tech.tsx`
- Modify: `frontend/src/templates/editorial.tsx`
- Modify: `frontend/src/templates/redline.tsx`
- Modify: `frontend/src/templates/violet.tsx`
- Modify: `frontend/src/templates/azure-chronicle.tsx`
- Modify: `frontend/src/templates/registry.test.tsx`
- Modify: `frontend/src/templates/azure-chronicle.test.tsx`
- Modify: `frontend/src/templates/templates.css` (only if necessary; preserve pre-existing user edits)

**Step 1: Write the failing test**

Extend registry tests to use a v2 fixture with all eight types and a reordered
section array. For each template, assert a representative item from every
visible type is rendered, honours are compactly rendered, and the first and
last headings honour the specified order.

**Step 2: Run test — confirm it fails**

Command: `cd frontend && npm test -- src/templates/registry.test.tsx src/templates/azure-chronicle.test.tsx`

Expected: FAIL because templates destructure removed legacy arrays.

**Step 3: Write minimal implementation**

Keep each existing template's header and visual identity. Replace its
hard-coded body sections with `ResumeSections` configured with that template's
existing classes and layout choices. Add CSS only where needed for Markdown
and each template's honour layout; do not modify unrelated existing CSS.

**Step 4: Run test — confirm it passes**

Command: `cd frontend && npm test -- src/templates/registry.test.tsx src/templates/azure-chronicle.test.tsx`

Expected: PASS.

**Step 5: Commit**

`git add frontend/src/templates && git commit -m "feat: render sections in every resume template"`

### Task 7: Run regression verification

**Files:**

- Modify only files required to repair a verified regression.

**Step 1: Run frontend test suite**

Command: `cd frontend && npm test -- --run`

Expected: PASS.

**Step 2: Run production build**

Command: `cd frontend && npm run build`

Expected: PASS with no TypeScript errors.

**Step 3: Manually inspect every template**

Start the frontend, open a v2 sample containing every section, reorder two
sections, hide one section and one optional field, then inspect the eight
previews. Confirm styles change by template while data availability and order
do not.

**Step 4: Commit any regression repair**

`git add <verified-files> && git commit -m "fix: verify sectioned resume rendering"`
