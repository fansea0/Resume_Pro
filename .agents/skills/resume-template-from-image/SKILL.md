---
name: resume-template-from-image
description: Generate a new editable, print-safe React resume template from an uploaded reference image or screenshot. Use when the user provides a resume design image and asks to reproduce, adapt, turn into a template, match its visual style, or add it to the resume generator's template picker.
---

# Resume Template From Image

Turn a reference design into an original, editable template for this repository's resume generator. Preserve the visual hierarchy and layout language; do not copy branding, copyrighted imagery, or personal data from the source.

## Workflow

1. Inspect the supplied image with `view_image` before editing code. If it is not available locally, ask the user to attach it again.
2. Read `references/visual-analysis.md`. Extract a compact design brief: page grid, header, typography hierarchy, palette, section order, reusable motifs, density, and A4/page-break risks.
3. Read the current `ResumeDocument` type and `frontend/src/templates/registry.tsx`. Reuse every existing structured section where sensible; do not add database/API fields solely to imitate decoration.
4. Propose the template ID, Chinese display name, 2–3 Chinese job tags, concise Chinese style description, and the design brief. Get approval before implementation when the request is a new visual direction; proceed directly only when the user has already approved it.
5. Implement a separate renderer in `frontend/src/templates/<id>.tsx`, add a namespaced CSS block in `templates.css`, and register it in `templateRegistry` with its ID, Chinese name, job tags, and style description.
6. Ensure the renderer has no hard-coded candidate details. Render all applicable profile, summary, experience, project, education, skills, and award data from `ResumeDocument`.
7. Generate an original static PNG cover at `frontend/public/template-previews/<id>.png`. It must be a full A4 sample-resume snapshot: show a fixed fictional name, contact line, summary, at least two experience entries, a project, education, and skills in the same layout, palette, and hierarchy as the renderer. It must not render live user data or include source-image personal data. Add its `/template-previews/<id>.png` path as `previewImage` in the registry entry.
8. Add a registry test that renders the template with the shared sample document and checks its ID, a distinctive template landmark, profile name, and representative experience content. Assert the new registry entry has 2–3 nonempty Chinese job tags, a nonempty concise Chinese style description, and a static `previewImage` beginning with `/template-previews/`. Add or update gallery/admin tests so tags and style description are visible on cards.
9. Verify with:

   ```bash
   cd frontend && npm test -- --run src/templates/registry.test.tsx && npm run build
   ```

10. Inspect the result in the running app or a rendered browser screenshot. Check the gallery cover as well as the editor result. Adjust only measurable mismatches in hierarchy, spacing, contrast, overflow, or print behavior. Keep A4 rules and use `break-inside: avoid` for experience/project blocks.

## Design constraints

- Favor readable, ATS-conscious typography over decorative effects.
- Use CSS variables scoped beneath the template root for its colors, spacing, and typography.
- Give every template a stable lowercase `TemplateId`; never rename an existing ID.
- Always include one static PNG gallery cover per template. Keep its A4-like aspect ratio, use a fixed fictional sample resume with complete text content (not only placeholder lines), and never read live user data.
- Every template registration must include a stable lowercase ID, a Chinese display name, 2–3 nonempty Chinese job tags, and one concise Chinese style description. The tags describe suitable roles or job contexts; the description states the visual style rather than repeating the name.
- Keep editor controls and data models template-agnostic.
- Use original shapes and icons. Do not reproduce logos, portrait photos, or brand marks from the reference.
- Preserve contrast in print; avoid full-page dark backgrounds unless the user explicitly wants them.

## Output

Report the added template ID, Chinese display name, job tags, style description, files changed, visual choices derived from the reference, and verification results. If the reference cannot support a faithful editable implementation (for example, text is unreadable or it depends on a proprietary asset), explain the limitation and offer an original adaptation.
