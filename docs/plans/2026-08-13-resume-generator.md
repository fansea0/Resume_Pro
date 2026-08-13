# Resume Generator Implementation Plan

> **For implementer:** Use TDD throughout. Write failing test first. Watch it fail. Then implement.

**Goal:** Build a local full-stack resume generator with editable structured data, three extensible templates, persistence, and PDF-ready printing.

**Architecture:** A React/Vite SPA holds editor state and renders templates from a registry. A Go `net/http` API stores one JSON document per resume in SQLite using `sqlx`; templates are frontend renderers, so visual additions do not change persistence.

**Tech Stack:** Go, `net/http`, SQLite, `sqlx`, `logrus`, React, TypeScript, Vite, Vitest, React Testing Library, TanStack Query.

---

### Task 1: Initialize Go module and health API

**Files:**
- Create: `backend/go.mod`
- Create: `backend/cmd/server/main.go`
- Create: `backend/internal/httpapi/router.go`
- Test: `backend/internal/httpapi/router_test.go`

**Step 1:** Write a test using `httptest.NewRecorder` that calls `NewRouter(nil).ServeHTTP` with `GET /api/health` and expects status 200 plus JSON body `{\"status\":\"ok\"}`.

**Step 2:** Run `cd backend && go test ./internal/httpapi -run TestHealth -v`; confirm it fails because the router is absent.

**Step 3:** Implement `NewRouter` using `http.NewServeMux`; register `GET /api/health` and encode the status JSON. Add a `main` package that listens on `:8080` and initializes a `logrus` logger.

**Step 4:** Re-run the command; expect PASS.

**Step 5:** Commit `git add backend && git commit -m \"feat: add Go health API\"`.

### Task 2: Add SQLite migrations and resume repository

**Files:**
- Modify: `backend/go.mod`
- Create: `backend/internal/resume/model.go`
- Create: `backend/internal/resume/repository.go`
- Create: `backend/internal/resume/repository_test.go`
- Create: `backend/internal/store/migrate.go`

**Step 1:** Write a repository test against a temporary SQLite database: migrate, create a resume with `title`, `template_id`, and JSON content; fetch it by ID; assert round-trip equality; update it and assert one version snapshot exists.

**Step 2:** Run `cd backend && go test ./internal/resume -run TestRepositoryCreateUpdate -v`; confirm it fails before repository implementation.

**Step 3:** Add `github.com/jmoiron/sqlx` and a SQLite driver. Create migrations for `resumes` and `resume_versions`; implement `Create`, `Get`, `List`, `Update`, and `Delete` methods with UTC timestamps and a transaction for update/version insertion.

**Step 4:** Re-run the command; expect PASS.

**Step 5:** Commit `git add backend && git commit -m \"feat: persist resumes in SQLite\"`.

### Task 3: Expose validated resume CRUD API

**Files:**
- Modify: `backend/internal/httpapi/router.go`
- Create: `backend/internal/httpapi/resume_handler.go`
- Create: `backend/internal/httpapi/resume_handler_test.go`
- Modify: `backend/cmd/server/main.go`

**Step 1:** Write table-driven API tests for create/list/get/update/delete plus invalid create content returning 400 and missing ID returning 404.

**Step 2:** Run `cd backend && go test ./internal/httpapi -run TestResume -v`; confirm it fails.

**Step 3:** Implement strict JSON decoding, title/template/content validation, route ID parsing, and response helpers. Wire a SQLite database path from `RESUME_DB_PATH` (default `./resume.db`) through migrations and repository into the router.

**Step 4:** Re-run `cd backend && go test ./...`; expect PASS.

**Step 5:** Commit `git add backend && git commit -m \"feat: add resume CRUD API\"`.

### Task 4: Scaffold frontend and resume document model

**Files:**
- Create: `frontend/package.json`
- Create: `frontend/vite.config.ts`
- Create: `frontend/tsconfig.json`
- Create: `frontend/index.html`
- Create: `frontend/src/main.tsx`
- Create: `frontend/src/app.tsx`
- Create: `frontend/src/types/resume.ts`
- Create: `frontend/src/types/resume.test.ts`
- Create: `frontend/src/styles/global.css`

**Step 1:** Write Vitest tests for `createEmptyResumeDocument()` ensuring meaningful defaults and empty arrays for every repeatable section.

**Step 2:** Run `cd frontend && npm test -- --run src/types/resume.test.ts`; confirm it fails.

**Step 3:** Add Vite, React, TypeScript, Vitest, and Testing Library configuration. Define the shared resume document types and factory. Render a minimal app root.

**Step 4:** Re-run the test; expect PASS.

**Step 5:** Commit `git add frontend && git commit -m \"feat: scaffold resume editor frontend\"`.

### Task 5: Create template registry and initial visual templates

**Files:**
- Create: `frontend/src/templates/registry.tsx`
- Create: `frontend/src/templates/minimal.tsx`
- Create: `frontend/src/templates/business.tsx`
- Create: `frontend/src/templates/creative.tsx`
- Create: `frontend/src/templates/templates.css`
- Create: `frontend/src/templates/registry.test.tsx`

**Step 1:** Test that the registry exposes `minimal`, `business`, and `creative`, and that each renderer displays sample profile name and experience content.

**Step 2:** Run `cd frontend && npm test -- --run src/templates/registry.test.tsx`; confirm it fails.

**Step 3:** Implement template metadata and three isolated renderers. Share only small presentational helpers; retain separate layout/style classes so later templates can be added by registering a single component.

**Step 4:** Re-run the test; expect PASS.

**Step 5:** Commit `git add frontend && git commit -m \"feat: add extensible resume templates\"`.

### Task 6: Build editor and live preview

**Files:**
- Create: `frontend/src/components/ResumeEditor.tsx`
- Create: `frontend/src/components/ResumePreview.tsx`
- Create: `frontend/src/components/TemplatePicker.tsx`
- Create: `frontend/src/components/ResumeEditor.test.tsx`
- Modify: `frontend/src/app.tsx`
- Modify: `frontend/src/styles/global.css`

**Step 1:** Test editing the profile name updates preview text, selecting `business` changes the active template, and adding/removing one work experience updates state.

**Step 2:** Run `cd frontend && npm test -- --run src/components/ResumeEditor.test.tsx`; confirm it fails.

**Step 3:** Implement controlled section forms, reusable add/remove controls, a template picker, and responsive editor/preview layout. Keep all state in the app component until API wiring is added.

**Step 4:** Re-run the test; expect PASS.

**Step 5:** Commit `git add frontend && git commit -m \"feat: add live resume editing\"`.

### Task 7: Connect save/load and print export

**Files:**
- Create: `frontend/src/api/client.ts`
- Create: `frontend/src/api/resumes.ts`
- Create: `frontend/src/components/ResumeLibrary.tsx`
- Create: `frontend/src/components/ResumeLibrary.test.tsx`
- Modify: `frontend/src/app.tsx`
- Modify: `frontend/src/styles/global.css`

**Step 1:** Test a mocked create request saves the current document and renders returned resume title; test clicking export calls `window.print`.

**Step 2:** Run `cd frontend && npm test -- --run src/components/ResumeLibrary.test.tsx`; confirm it fails.

**Step 3:** Add typed fetch client, a small resume library (create/select/save/delete), loading/error UI, and `@media print` rules that hide controls and constrain preview to A4. Add a Vite proxy for `/api` during development.

**Step 4:** Re-run frontend tests and `npm run build`; expect PASS.

**Step 5:** Commit `git add frontend && git commit -m \"feat: save resumes and export PDF\"`.

### Task 8: Document local development and verify MVP

**Files:**
- Create: `README.md`
- Modify: `backend/cmd/server/main.go`
- Modify: `frontend/src/app.tsx`

**Step 1:** Add a smoke test/checklist that creates a resume through the API, changes templates, saves, reloads, and prints preview.

**Step 2:** Run `cd backend && go test ./...` and `cd frontend && npm test -- --run && npm run build`; confirm all pass.

**Step 3:** Add README setup instructions: start backend, start frontend, set database location, and export via browser print dialog. Ensure CORS/proxy development path is documented and the default page has a clear empty-state action.

**Step 4:** Re-run all commands; expect PASS.

**Step 5:** Commit `git add README.md backend frontend && git commit -m \"docs: add resume generator setup\"`.
