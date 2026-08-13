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
4. Propose the template ID, Chinese display name, and the design brief. Get approval before implementation when the request is a new visual direction; proceed directly only when the user has already approved it.
5. Implement a separate renderer in `frontend/src/templates/<id>.tsx`, add a namespaced CSS block in `templates.css`, and register it in `templateRegistry`.
6. Ensure the renderer has no hard-coded candidate details. Render all applicable profile, summary, experience, project, education, skills, and award data from `ResumeDocument`.
7. Create an original static SVG cover at `frontend/public/template-previews/<id>.svg`. It must communicate the new template's grid, palette, and visual motif without rendering a React template or including user/candidate data. Add its `/template-previews/<id>.svg` path as `previewImage` in the registry entry.
8. Add a registry test that renders the template with the shared sample document and checks its ID, a distinctive template landmark, profile name, and representative experience content. Also assert every registry entry has a static `previewImage` beginning with `/template-previews/`.
9. Verify with:

   ```bash
   cd frontend && npm test -- --run src/templates/registry.test.tsx && npm run build
   ```

10. Inspect the result in the running app or a rendered browser screenshot. Check the gallery cover as well as the editor result. Adjust only measurable mismatches in hierarchy, spacing, contrast, overflow, or print behavior. Keep A4 rules and use `break-inside: avoid` for experience/project blocks.

## Design constraints

- Favor readable, ATS-conscious typography over decorative effects.
- Use CSS variables scoped beneath the template root for its colors, spacing, and typography.
- Give every template a stable lowercase `TemplateId`; never rename an existing ID.
- Always include one static SVG gallery cover per template. Keep its A4-like aspect ratio, use `viewBox="0 0 210 297"`, omit personal data, and use shapes/placeholder lines rather than live resume text.
- Keep editor controls and data models template-agnostic.
- Use original shapes and icons. Do not reproduce logos, portrait photos, or brand marks from the reference.
- Preserve contrast in print; avoid full-page dark backgrounds unless the user explicitly wants them.

## Output

Report the added template ID, files changed, the visual choices derived from the reference, and the verification results. If the reference cannot support a faithful editable implementation (for example, text is unreadable or it depends on a proprietary asset), explain the limitation and offer an original adaptation.
