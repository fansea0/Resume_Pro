# Resume Generator Design

## Goal

Build a browser-based resume editor for internet-industry job seekers. Users can maintain structured resume data, select a polished template, preview changes immediately, save multiple resumes, and export a print-ready PDF.

## Architecture

The application is a React + TypeScript + Vite single-page application backed by a Go HTTP API. The client owns the editing experience and renders templates locally for immediate preview. The API persists a versioned JSON resume document in SQLite, keeping its schema flexible while templates evolve independently.

## Frontend

- React, TypeScript, Vite, React Router, and TanStack Query.
- A single `ResumeDocument` TypeScript model drives editing, persistence, preview, and export.
- The editor has section forms for profile, summary, work experience, projects, education, skills, and awards.
- A template registry maps a stable template ID to a metadata entry and an independent React renderer.
- Initial template IDs are `minimal`, `business`, and `creative`.
- Browser print CSS exports the preview area only, using A4 dimensions and page-break controls; no fragile screenshot/PDF conversion dependency is needed for the MVP.

## Backend

- Go standard library HTTP server with a small JSON API.
- `sqlx` manages SQLite queries and migrations; `logrus` provides structured request/error logs.
- Main endpoints: `GET /api/health`, `GET /api/resumes`, `POST /api/resumes`, `GET /api/resumes/{id}`, `PUT /api/resumes/{id}`, and `DELETE /api/resumes/{id}`.
- `GET /api/templates` returns template metadata; content rendering remains client-side.
- An `internal/ai` interface is present but has no provider implementation. Eino can be connected later without changing resume persistence or frontend contracts.

## Data model

`resumes` stores `id`, `title`, `template_id`, `content_json`, `created_at`, and `updated_at`. `content_json` is the canonical structured `ResumeDocument`, allowing additive fields without migration churn. A `resume_versions` table records a JSON snapshot whenever a persisted resume is updated, ready for later history/restore UI.

## Data flow

1. User opens or creates a resume.
2. The editor changes in-memory `ResumeDocument` state and preview rerenders synchronously.
3. The user saves explicitly; the client sends title, template ID, and document JSON to the Go API.
4. The API validates the request, writes the resume and (on update) a version snapshot, then returns the saved record.
5. Export uses the selected template rendered under dedicated print CSS.

## Error handling

- API validation returns concise JSON error messages with 400/404/500 status codes.
- The client displays save/load errors without discarding unsaved editor changes.
- Invalid stored JSON is treated as a server error and logged with the resume ID.

## Testing

- Go unit/integration tests cover migrations, repository CRUD, HTTP validation, and API response codes using a temporary SQLite database.
- Vitest + Testing Library cover document defaults, template registry, form updates, and rendering each initial template.
- A final production build plus Go test suite verifies the complete MVP.

## Scope boundaries

- No authentication or multi-user collaboration in MVP.
- No AI provider or token management in MVP; only an integration boundary is reserved.
- No cloud storage, sharing links, DOCX generation, or automated content generation in MVP.
