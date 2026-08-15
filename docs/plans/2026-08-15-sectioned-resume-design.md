# Sectioned Resume Design

## Goal

Replace the fixed resume arrays with a stable, section-based document format. A
resume's content and order belong to its data; a template is responsible only
for the visual presentation of that content.

## Product principles

- The database stores the sectioned JSON format as the canonical resume
  content format.
- Every template supports every section type and renders visible sections in
  the document's order.
- Templates may choose their own typography, spacing, background, columns, and
  compact treatment of each section, especially honours; they must not remove
  data capabilities from the document model.
- Sections can be reordered and hidden. Fields explicitly marked optional by
  the product brief have independent visibility switches.
- Existing saved drafts remain readable through a one-way in-memory migration
  from the legacy fixed-array shape. The next save persists the canonical
  sectioned shape.

## Canonical schema

`ResumeDocument` remains the client-side representation stored in `content`.
The API envelope and database columns continue to hold `title` and
`templateId` separately for list views, while the inner content has the same
values for a self-contained document.

```ts
type ResumeDocument = {
  schemaVersion: 2;
  title: string;
  templateId: TemplateId;
  profile: ResumeProfile;
  sections: ResumeSection[];
};

type ResumeSection = {
  id: string;
  type: ResumeSectionType;
  title: string;
  visible: boolean;
  content: SectionContent;
};
```

`ResumeSectionType` is a closed discriminated union: `summary`, `work`,
`internship`, `project`, `education`, `skills`, `campus`, and `honours`.
`title` defaults to the corresponding Chinese heading but is retained in the
document so the displayed heading can be customised without compromising the
semantic type.

The content variants are:

| Type | Content |
| --- | --- |
| `summary` | `{ markdown: string }` |
| `work`, `internship` | `{ items: ExperienceItem[] }` — company, role, date range, location, achievement contribution, and Markdown bullet text. `showLocation` and `showAchievements` control their optional fields. |
| `project` | `{ items: ProjectItem[] }` — name, role, date range, location, link, introduction, achievement, and Markdown bullet text. `showLocation`, `showLink`, `showIntroduction`, and `showAchievements` control optional fields. |
| `education` | `{ items: EducationItem[] }` — school, date range, degree, college, and major. |
| `skills` | `{ items: SkillItem[] }` — skill name and note. |
| `campus` | `{ items: CampusItem[] }` — activity/department name, role, and Markdown bullet text. |
| `honours` | `{ items: HonourItem[] }` — short award names only. |

Dates remain display strings so users can enter `2025.08`, `至今`, or another
locale-specific form without a formatting conversion.

## Rendering architecture

Introduce one shared semantic section renderer and a small safe Markdown
renderer. Each template provides presentation primitives (section heading,
entry wrapper, metadata placement, and honours layout), then calls the shared
renderer for the section contents. The renderer traverses `sections` in array
order after filtering `visible === true`; it never relies on a fixed section
ordering.

Markdown is deliberately limited to paragraphs/line breaks, emphasis, strong
text, links, unordered lists, and ordered lists. It never renders raw HTML,
images, tables, or arbitrary embeds. List ordering is the source order of the
user's text; switching list syntax is treated as content editing, not a
separate ordering model.

## Editing and migration

The editor becomes a list of section cards. Users can add a section of any
supported type, move it up/down, toggle its section visibility, edit its
heading, and delete it. Type-specific item editors implement the fields above;
the explicitly optional fields expose their own visible switch.

`documentFromStored` first validates a v2 document. If the stored object is a
valid legacy document, it transforms `profile.summary`, `experiences`,
`projects`, `education`, and `skills` into default v2 sections, preserving
their IDs and ordering. A malformed payload still fails safely rather than
being guessed at.

## Testing

- Unit-test creation and legacy migration of v2 documents.
- Unit-test saved-content validation for all section variants.
- Exercise editor add, reorder, section hiding, optional field visibility, and
  all item types.
- Test safe Markdown output and the supported list syntax.
- Render every registered template with a document containing every section;
  assert that hidden sections are absent and visible sections follow supplied
  order.
