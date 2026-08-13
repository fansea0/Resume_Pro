# Authentication and Tenant Isolation Implementation Plan

> **For implementer:** Use TDD throughout. Write failing test first. Watch it fail. Then implement.

**Goal:** Add password/JWT authentication and enforce tenant-scoped resume persistence, with a provider identity model ready for WeChat login.

**Architecture:** Go auth middleware verifies an HttpOnly-cookie JWT and supplies a tenant principal to handlers. SQLite owns users, tenants, identities, and tenant-owned resumes. React calls `/api/auth/me` and gates the template library behind a small auth screen; browser cookies carry sessions automatically.

**Tech Stack:** Go, SQLite, sqlx, bcrypt, golang-jwt/jwt, React, TypeScript, Vitest, React Testing Library.

---

### Task 1: Migrate tenant, user, and identity tables

**Files:**
- Modify: `backend/go.mod`
- Modify: `backend/internal/store/migrate.go`
- Create: `backend/internal/auth/model.go`
- Create: `backend/internal/auth/repository.go`
- Create: `backend/internal/auth/repository_test.go`

**Step 1:** Write a temporary-SQLite test that migrates, creates tenant/user/password identity, checks normalized unique email, and verifies resume rows have a tenant ID.

**Step 2:** Run `cd backend && go test ./internal/auth -run TestRepositoryCreateUser -v`; confirm it fails because the package/migration is absent.

**Step 3:** Add idempotent migration/backfill logic, auth repository methods, and bcrypt hashing helpers using cost 12. Add `users`, `tenants`, `user_identities`, tenant indexes, and safely backfill pre-existing resumes/versions to a bootstrap tenant.

**Step 4:** Run `cd backend && go test ./internal/auth -run TestRepositoryCreateUser -v`; expect PASS.

**Step 5:** Commit `git add backend && git commit -m "feat: add tenant and user persistence"`.

### Task 2: Scope resume repository by tenant

**Files:**
- Modify: `backend/internal/resume/model.go`
- Modify: `backend/internal/resume/repository.go`
- Modify: `backend/internal/resume/repository_test.go`

**Step 1:** Add a failing test creating two tenants and resumes, then assert each list/get/update/delete can access only its own data and cross-tenant lookup is `ErrNotFound`.

**Step 2:** Run `cd backend && go test ./internal/resume -run TestRepositoryTenantIsolation -v`; confirm it fails.

**Step 3:** Require tenant ID in all repository methods/inputs, filter every read/write by it, and store tenant ID on versions.

**Step 4:** Run `cd backend && go test ./internal/resume -v`; expect PASS.

**Step 5:** Commit `git add backend/internal/resume && git commit -m "feat: isolate resumes by tenant"`.

### Task 3: Add JWT cookie auth API and protected middleware

**Files:**
- Modify: `backend/go.mod`
- Create: `backend/internal/auth/service.go`
- Create: `backend/internal/httpapi/auth_handler.go`
- Create: `backend/internal/httpapi/auth_middleware.go`
- Create: `backend/internal/httpapi/auth_handler_test.go`
- Modify: `backend/internal/httpapi/router.go`
- Modify: `backend/internal/httpapi/resume_handler.go`
- Modify: `backend/internal/httpapi/resume_handler_test.go`
- Modify: `backend/cmd/server/main.go`

**Step 1:** Write failing `httptest` cases for register/login/me/logout, protected-resume 401, invalid credentials generic failure, and cross-tenant resume 404.

**Step 2:** Run `cd backend && go test ./internal/httpapi -run 'TestAuth|TestProtectedResume' -v`; confirm it fails.

**Step 3:** Implement JWT issuance/verification, cookie configuration, request principal middleware, register/login/logout/me handlers, and inject authenticated tenant ID into resume handlers. Configure `JWT_SECRET` and `COOKIE_SECURE` in main.

**Step 4:** Run `cd backend && go test ./...`; expect PASS.

**Step 5:** Commit `git add backend && git commit -m "feat: add JWT login and protected resume API"`.

### Task 4: Add frontend auth gate and session-aware API client

**Files:**
- Create: `frontend/src/api/auth.ts`
- Modify: `frontend/src/api/client.ts`
- Create: `frontend/src/components/AuthScreen.tsx`
- Create: `frontend/src/components/AuthScreen.test.tsx`
- Modify: `frontend/src/app.tsx`
- Modify: `frontend/src/styles/global.css`

**Step 1:** Write failing tests for unauthenticated startup showing login/register, successful login showing the gallery, logout returning to auth, and 401 clearing authenticated state.

**Step 2:** Run `cd frontend && npm test -- --run src/components/AuthScreen.test.tsx`; confirm it fails.

**Step 3:** Add credential-including client requests, typed auth calls, auth gate/loading state, validation/error UI, account header, logout action, and 401 listener callback. Do not store JWT in JavaScript.

**Step 4:** Run `cd frontend && npm test -- --run && npm run build`; expect PASS.

**Step 5:** Commit `git add frontend && git commit -m "feat: add password login experience"`.

### Task 5: Document secure local configuration and final verification

**Files:**
- Create: `README.md`
- Modify: `backend/cmd/server/main.go`

**Step 1:** Add a smoke-test checklist for registration, private resume create, account switch isolation, and logout/401 behavior.

**Step 2:** Run `cd backend && go test ./...` and `cd frontend && npm test -- --run && npm run build`; expect PASS.

**Step 3:** Document `JWT_SECRET`, `COOKIE_SECURE`, database path, local startup, and warning that production needs an HTTPS deployment and a strong secret.

**Step 4:** Repeat all verification commands; expect PASS.

**Step 5:** Commit `git add README.md backend frontend && git commit -m "docs: add authentication setup"`.
