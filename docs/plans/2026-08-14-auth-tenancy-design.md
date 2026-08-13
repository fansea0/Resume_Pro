# Authentication and Tenant Isolation Design

## Goal

Add password registration/login, JWT-backed sessions, and strict personal-tenant isolation so each account can access only its own resumes, while keeping a clean path for future WeChat and other identity providers.

## Session model

- Registration and login issue a signed HS256 JWT with `sub` (user ID), `tenant_id`, `iat`, and `exp` claims.
- The token is stored only in a `resume_session` HttpOnly cookie with `SameSite=Lax`, `Path=/`, and a 7-day expiry. `Secure` is enabled when `COOKIE_SECURE=true`; local development remains HTTP-compatible.
- `JWT_SECRET` is required in production; development may use an explicit documented local-only default. The API validates issuer, signature, expiry, user ID, and tenant ID before protected routes run.
- Logout clears the cookie. This MVP does not implement token revocation; rotating `JWT_SECRET` invalidates all sessions. Add a session table only if revocation/device management is later required.

## Data model

- `tenants`: personal workspace row with an ID, display name, and timestamps.
- `users`: email (normalized unique), bcrypt password hash, default tenant ID, timestamps.
- `user_identities`: links a user to a provider identifier. Password registration creates `provider=password`, `provider_subject=<normalized email>`; future WeChat OAuth adds `provider=wechat` with its OpenID without changing users or resume ownership.
- `resumes` gets a non-null `tenant_id`; existing pre-auth rows migrate to a bootstrap tenant so migration is safe. `resume_versions` gets `tenant_id` too for scoped history queries.

## Authorization boundary

- Auth middleware creates a typed request context principal `{userID, tenantID, email}`.
- Every resume repository method accepts `tenantID` and includes it in `WHERE` clauses and inserts. A resume belonging to another tenant returns the same `404` as an absent resume.
- Only `/api/health`, `/api/auth/register`, `/api/auth/login`, and `/api/auth/logout` are public. `/api/auth/me` and all `/api/resumes` routes require a valid session.

## Frontend

- Startup requests `/api/auth/me`; while resolving, show a loading state. Unauthenticated users see a compact email/password login/register screen.
- Credentials are sent with `credentials: 'include'`; no token is stored in JavaScript or browser storage.
- Authenticated users enter the existing template gallery. The header shows their email and a logout control. A 401 response clears local auth state and returns to the login screen.

## Error handling and security

- Validate email format, password minimum length (12 characters), duplicate email, malformed JSON, and invalid/expired session.
- Return generic `invalid email or password` for login failures; never expose whether an email exists.
- Apply `Cache-Control: no-store` on auth responses. Use bcrypt cost 12.
- This single-page/local deployment has no cross-origin credential flow; if deployment later separates domains, add explicit CORS origins and CSRF protection before enabling credentials cross-origin.

## Testing

- Backend tests: password hash/verify, JWT validation/expiry, registration creates tenant/user/identity, login/logout/me, protected route 401, and cross-tenant CRUD attempts return 404 without altering data.
- Frontend tests: unauthenticated login screen, login success shows gallery, logout returns login screen, and 401 transitions out of the authenticated UI.
