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

The cover preview is a real rendering of the template, not a hand-drawn SVG. Step 7 of `SKILL.md` and `references/preview-snapshot.md` cover the full recipe; keep these constraints in mind when planning the visual brief so the renderer can produce a clean A4 snapshot.

- The brief must end at a state that fits one A4 page with the fixed `createSampleResumeDocument` sample — usually identity, summary, two experience entries, one project, education, and skills.
- Use the template palette and layout exactly as it will appear in the live export, so the snapshot script and the user's `exportResumePng` output look the same.
- Never include live user data or the source-image candidate's personal data. The committed sample is the only data source.
- The committed PNG (not SVG) becomes `previewImage` in the registry; the gallery only loads static assets and never renders a candidate's live document.
