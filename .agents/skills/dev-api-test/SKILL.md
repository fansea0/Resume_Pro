---
name: dev-api-test
description: Analyze and test a selected Go HTTP API endpoint in this repository against a local server. Use when a user selects or names a backend handler, route, DTO, or API test and asks to test, debug, validate, smoke-test, or produce a report for that endpoint. Infer its contract from code, exercise protected routes with a persistent Cookie session that mirrors browser behavior, run isolated checks, and return results plus actionable improvement suggestions.
---

# Dev API Test

Test selected backend APIs against a local development server. Use the development-login fixture to bypass password entry while preserving the application's normal JWT-in-Cookie session behavior. Inspect code before calling the endpoint, then report evidence rather than claiming unexecuted coverage.

## Workflow

1. Read the selected code and its surrounding route registration, request/response types, authentication checks, repository calls, and existing tests. Infer the HTTP method, path, required headers, JSON body, status codes, ownership rules, and side effects.
2. Confirm the target is a loopback URL (`127.0.0.1` or `::1`) and that the server runs with `APP_ENV=development`. Refuse to call a remote, staging, or production target. Do not use `X-Forwarded-For` to reason about eligibility.
3. If the endpoint requires authentication, use `dev@test` unless the user explicitly supplies another local test email. Establish a **persistent Cookie session** through `POST /api/dev/login` with that email. This intentionally bypasses password entry but returns the same signed `resume_session` Cookie used by normal authentication. Use that same session for all protected requests; it must capture `Set-Cookie` and resend the Cookie automatically. In Python, use one `requests.Session()`; with curl, use one temporary cookie jar with `-c` on development login and `-b` on subsequent requests. Never print a cookie, JWT, password, or complete sensitive response body.
4. Test the happy path plus relevant missing/invalid input, unauthenticated, not-found, and cross-tenant cases. Derive test payloads from validation and existing tests instead of guessing fields. Use unique `dev-api-test-<timestamp>` data for write operations.
5. Delete created resources whenever the API exposes deletion. Do not delete or recreate the fixed `dev@test` account. Remove the temporary Cookie jar/session state. State any intentional or unavoidable test-data residue in the report.
6. Return a concise report with: inferred contract; commands or request shapes used with secrets redacted; status/assertion matrix; cleanup result; coverage gaps; and prioritized code or test improvements. Do not modify production code unless the user separately authorizes a change.

## Cookie-session contract

- Authenticate protected-route tests through `POST /api/dev/login` with `dev@test` by default. Use a different email only when the user explicitly supplies one. Send `Content-Type: application/json` and `{"email":"dev@test"}`.
- Treat the response's `Set-Cookie: resume_session=...` as opaque. Do not parse, decode, log, or copy its JWT value.
- Send protected requests through the same persistent session/cookie jar. Do not use `Authorization: Bearer`; this API authenticates from the `resume_session` Cookie.
- Use manual Cookie injection only for narrow authentication-negative tests: omit the Cookie to assert `401`, or pass a synthetic invalid value such as `resume_session=invalid-token` to assert rejection. Never reuse or expose a real JWT.
- Test logout by calling `POST /api/auth/logout` through the persistent session, then assert a protected request returns `401` using that same session.

## Development-login guardrails

- Use `POST /api/dev/login` only for local protected-route testing, never to test password validation itself.
- Confirm loopback targeting and `APP_ENV=development`; treat `404` as disabled and `403` as a non-loopback rejection.
- Do not use this fixture for a login endpoint test. Validate `POST /api/auth/login` separately with its own credentials and temporary cookie jar when the user selects that endpoint.

## Safety and test-data rules

- Reuse the fixed local `dev@test` account. Create only disposable records beneath it, prefer API-supported cleanup, and do not delete data that existed before the test.
- Never invoke this skill against non-loopback targets, even if a Dev login endpoint responds.
- Redact credentials, cookies, JWTs, full resume content, and personally identifiable data from commands and reports.
