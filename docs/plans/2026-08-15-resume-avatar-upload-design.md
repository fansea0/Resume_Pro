# Resume Avatar Upload Implementation Plan

> **For implementer:** Use TDD throughout. Write failing test first. Watch it fail. Then implement.

**Goal:** Let a user upload a profile avatar, persist its URL in the v2 resume document, and display it on every resume template while retaining an interchangeable storage backend.

**Architecture:** The backend exposes an authenticated avatar upload endpoint backed by a `FileStorage` interface. `LocalFileStorage` is the initial adapter and saves validated images under a configured project-local upload directory; an OSS adapter can later implement the same interface without changing the handler or frontend. The v2 profile receives an optional `avatarUrl` string, and templates render it when present while retaining their existing textual fallback.

**Tech Stack:** Go, Gin, multipart uploads, project-local file storage, React, TypeScript, Vitest, existing template registry.

---

### Task 1: Define the storage boundary and local adapter

**Files:**
- Create: `backend/internal/files/storage.go`
- Create: `backend/internal/files/local.go`
- Create: `backend/internal/files/local_test.go`

**Step 1: Write the failing test**

Test that `LocalFileStorage.Store` saves a PNG byte stream below the supplied directory, returns a stable `/uploads/avatars/...` URL, and rejects unsupported extension/path traversal input.

**Step 2: Run test — confirm it fails**

Command: `cd backend && go test ./internal/files`

**Step 3: Write minimal implementation**

Define `FileStorage` with `Store(ctx, File) (StoredFile, error)`; implement local storage with generated names, atomic creation, and a public URL prefix.

**Step 4: Run test — confirm it passes**

Command: `cd backend && go test ./internal/files`

**Step 5: Commit**

`git add backend/internal/files && git commit -m "feat: add local avatar file storage"`

### Task 2: Add an authenticated, validated upload route

**Files:**
- Modify: `backend/internal/httpapi/router.go`
- Create: `backend/internal/httpapi/upload_handler.go`
- Create: `backend/internal/httpapi/upload_handler_test.go`
- Modify: `backend/cmd/server/main.go`

**Step 1: Write the failing test**

Cover successful PNG/JPEG upload, missing authentication, invalid MIME/signature, and the size limit; assert returned URL and on-disk location via the fake storage.

**Step 2: Run test — confirm it fails**

Command: `cd backend && go test ./internal/httpapi -run AvatarUpload`

**Step 3: Write minimal implementation**

Register `POST /api/uploads/avatar`, limit multipart body to 5 MiB, verify decoded image type/dimensions, invoke `FileStorage`, and return `{ "url": "..." }`. Serve the configured local upload directory at `/uploads`.

**Step 4: Run test — confirm it passes**

Command: `cd backend && go test ./internal/httpapi -run AvatarUpload`

**Step 5: Commit**

`git add backend/cmd/server/main.go backend/internal/httpapi && git commit -m "feat: add authenticated avatar upload API"`

### Task 3: Extend the resume profile contract

**Files:**
- Modify: `frontend/src/types/resume.ts`
- Modify: `frontend/src/api/resumes.ts`
- Modify: `frontend/src/api/resumes.test.ts`
- Modify: `backend/internal/httpapi/resume_handler.go`
- Modify: `backend/internal/httpapi/resume_handler_test.go`

**Step 1: Write failing tests**

Add `avatarUrl: ''` to v2 factory expectations, assert old stored documents migrate with an empty URL, and assert backend write validation requires a bounded string profile field.

**Step 2: Run tests — confirm they fail**

Commands: `cd frontend && npm test -- --run src/api/resumes.test.ts`; `cd backend && go test ./internal/httpapi -run Resume`

**Step 3: Write minimal implementation**

Add `avatarUrl` to frontend profile models/factories/migration validators and backend v2 content validation.

**Step 4: Run tests — confirm they pass**

Commands: `cd frontend && npm test -- --run src/api/resumes.test.ts`; `cd backend && go test ./internal/httpapi -run Resume`

**Step 5: Commit**

`git add frontend/src/types/resume.ts frontend/src/api/resumes.ts frontend/src/api/resumes.test.ts backend/internal/httpapi/resume_handler.go backend/internal/httpapi/resume_handler_test.go && git commit -m "feat: persist resume avatar URLs"`

### Task 4: Build the editor upload affordance and template rendering

**Files:**
- Create: `frontend/src/api/uploads.ts`
- Create: `frontend/src/api/uploads.test.ts`
- Modify: `frontend/src/components/ResumeEditor.tsx`
- Modify: `frontend/src/components/ResumeEditor.test.tsx`
- Modify: `frontend/src/templates/*.tsx`
- Modify: `frontend/src/templates/registry.test.tsx`
- Modify: `frontend/src/styles/global.css`
- Modify: `frontend/src/templates/templates.css`

**Step 1: Write failing tests**

Test multipart API request handling, successful editor upload updates `profile.avatarUrl`, invalid upload reports a clear error, and each template renders the profile image with meaningful alt text when a URL exists.

**Step 2: Run tests — confirm they fail**

Commands: `cd frontend && npm test -- --run src/api/uploads.test.ts src/components/ResumeEditor.test.tsx src/templates/registry.test.tsx`

**Step 3: Write minimal implementation**

Add a compact image picker in 基本信息 with preview, replacement control, loading/error states, and `accept="image/png,image/jpeg,image/webp"`. Render image in each template header without image layout shifts; use existing initials/placeholders when no URL is present.

**Step 4: Run tests — confirm they pass**

Command: `cd frontend && npm test -- --run src/api/uploads.test.ts src/components/ResumeEditor.test.tsx src/templates/registry.test.tsx && npm run build`

**Step 5: Commit**

`git add frontend/src && git commit -m "feat: upload and render resume avatars"`

### Task 5: Run complete verification

**Files:**
- No new files.

**Step 1: Run frontend suite**

Command: `cd frontend && npm test -- --run && npm run build`

**Step 2: Run backend suite**

Command: `cd backend && go test ./...`

**Step 3: Confirm upload artifacts are ignored**

Add `backend/uploads/` to `.gitignore` if the chosen runtime upload location lives under the repository.

**Step 4: Commit ignore rule if required**

`git add .gitignore && git commit -m "chore: ignore local avatar uploads"`
