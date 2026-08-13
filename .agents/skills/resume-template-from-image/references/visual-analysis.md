# Visual Analysis Checklist

Analyze the reference before coding. Capture only decisions that affect the generated template.

## Design brief

- **Grid:** A4 margins, columns, sidebar width, alignment anchors.
- **Identity block:** name scale, title position, contact treatment, divider/header treatment.
- **Typography:** approximate serif/sans personality, heading/body contrast, casing, line height.
- **Color:** base, accent, neutral surfaces, print-safe fallback.
- **Content hierarchy:** section order, date placement, company/role emphasis, bullet treatment.
- **Motifs:** timeline, rules, chips, cards, numbered sections, icons.
- **Density:** compact, balanced, or editorial; what may collapse on small screens.

## Implementation map

Map the visual system to the existing structured fields:

| Reference area | ResumeDocument source |
| --- | --- |
| identity/contact | `profile` |
| intro statement | `summary` |
| career chronology | `experiences` |
| portfolio/case studies | `projects` |
| education | `education` |
| expertise labels | `skills` |
| recognition | `awards` |

If a decorative area has no source field, omit it or use a neutral generated visual treatment. Never fabricate candidate content.

## Print checks

- Keep margins compatible with A4 output.
- Avoid clipping with fixed heights.
- Keep each experience/project entry intact where feasible.
- Test dark fills and low-contrast colors with browser print preview.

## Static gallery cover

Create `frontend/public/template-previews/<template-id>.svg` for every generated template.

- Use an A4-like SVG canvas: `viewBox="0 0 210 297"`.
- Reduce the reference to its visual system: blocks for header/sidebar, section rules, accent shapes, and neutral placeholder lines.
- Use the template palette and layout; do not use a renderer screenshot, user data, real names, contact details, or source-image portraits.
- Register `/template-previews/<template-id>.svg` as `previewImage` so the gallery loads a static asset without rendering the user's document.
