# Premium Resume Templates Design

## Goal

Add two polished, print-safe resume templates that help internet-industry candidates differentiate themselves without compromising clarity: a dark technology design and an editorial design.

## Template: Dark Technology

- Stable ID: `tech`.
- Header uses a deep navy field with the candidate name, role, and contact details; cyan accents highlight role and section markers.
- Main content remains predominantly white with dark text to preserve print readability and avoid excessive ink usage.
- Skills render as compact outline chips; experience and project entries use a slim cyan timeline rule.
- Dense but legible two-column desktop layout collapses safely in the editor panel; printed content stays in a single A4-oriented document flow.

## Template: Editorial

- Stable ID: `editorial`.
- Warm off-white canvas, charcoal typography, dark forest-green accents, substantial whitespace, and fine hairline dividers.
- A large typographic identity block makes the candidate name prominent; experience is chronologically presented in an airy single-column reading flow.
- A narrow sidebar holds contact details, education, skills, and awards on large screens, while print remains a balanced two-column page.
- This template avoids photos and decorative images so it remains ATS-conscious and complete with existing structured data.

## Extensibility and testing

- Both templates are separate React renderers registered through the existing `templateRegistry`.
- Styling uses template-specific class prefixes (`tech-`, `editorial-`) plus the shared base styling, preserving isolation.
- Registry tests render every entry using the existing sample document; new assertions confirm both IDs, names, profile content, and a representative experience are rendered.
- No data-model, API, editor-form, or database changes are necessary because both templates consume the existing `ResumeDocument`.

## Print behavior

- Print CSS overrides dark fills where needed: the tech header keeps a high-contrast dark color and explicitly permits background-color printing; content avoids full-page colored backgrounds.
- Both templates use existing A4 print constraints and avoid page-breaks inside individual experience/project blocks.
