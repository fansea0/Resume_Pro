---
name: dev-api-test
description: Analyze and test only the Go HTTP API endpoint or route group explicitly selected by a user in this repository. Use when a user selects or names a backend handler, route, DTO, or API test and asks to test, debug, validate, smoke-test, or produce a report. Infer that target's contract, exercise it locally with a persistent Cookie session when required, and return target-specific results plus actionable repair suggestions.
---

# Dev API Test

Test only the user-requested backend API target against a local development server. Use the development-login fixture only as an authentication prerequisite; it is not part of the report unless it blocks the selected target. Inspect code before calling the target, then report evidence rather than claiming unexecuted coverage.

## Scope

- Treat the method and path explicitly named or selected by the user as the target. Do not test neighboring routes, unrelated authentication endpoints, or the whole service.
- When the user selects a route group such as `/api/resumes`, test every method registered for that group. When the user selects one method such as `PUT /api/resumes/:id`, test only that method and its applicable success and failure cases.
- State the resolved target before executing requests. If the selection does not identify a method or route, inspect the selected router/handler code and ask for clarification only when more than one route remains plausible.

## Workflow

1. Read the selected code and its surrounding route registration, request/response types, authentication checks, repository calls, and existing tests. Infer only the target's HTTP method, path, required headers, JSON body, status codes, ownership rules, and side effects.
2. Confirm the target is a loopback URL (`127.0.0.1` or `::1`) and that the server runs with `APP_ENV=development`. Refuse to call a remote, staging, or production target. Do not use `X-Forwarded-For` to reason about eligibility.
3. If the endpoint requires authentication, use `dev@test` unless the user explicitly supplies another local test email. Establish a **persistent Cookie session** through `POST /api/dev/login` with that email. This intentionally bypasses password entry but returns the same signed `resume_session` Cookie used by normal authentication. Use that same session for all protected requests; it must capture `Set-Cookie` and resend the Cookie automatically. In Python, use one `requests.Session()`; with curl, use one temporary cookie jar with `-c` on development login and `-b` on subsequent requests. Never print a cookie, JWT, password, or complete sensitive response body.
4. Test the target's happy path plus only its relevant missing/invalid input, unauthenticated, not-found, and cross-tenant cases. Do not call unrelated routes except setup, cleanup, or session establishment. Derive payloads from validation and existing tests instead of guessing fields. Use unique `dev-api-test-<timestamp>` data for write operations.
5. Delete created resources whenever the API exposes deletion. Do not delete or recreate the fixed `dev@test` account. Remove the temporary Cookie jar/session state. State any intentional or unavoidable test-data residue in the report.
6. Return a concise target-only report with: resolved target; inferred contract; redacted request shapes; status/assertion matrix; cleanup result; target-specific coverage gaps; and prioritized repair suggestions. Mention Dev login only if it prevented the target test. Do not modify production code unless the user separately authorizes a change.

## Repair suggestions

For every failed assertion or target-specific coverage gap, provide:

1. Evidence: the expected and actual status or behavior, without sensitive data.
2. Likely cause: the relevant handler, middleware, validation, repository, or route location.
3. Minimal repair: a concrete code or test change that resolves the issue without broad unrelated refactors.
4. Verification: the focused test or curl scenario that should pass after the repair.

Separate confirmed defects from optional improvements. Do not present untested speculation as a defect.

## Cookie-session contract

- Authenticate protected-route tests through `POST /api/dev/login` with `dev@test` by default. Use a different email only when the user explicitly supplies one. Send `Content-Type: application/json` and `{"email":"dev@test"}`.
- Treat the response's `Set-Cookie: resume_session=...` as opaque. Do not parse, decode, log, or copy its JWT value.
- Send protected requests through the same persistent session/cookie jar. Do not use `Authorization: Bearer`; this API authenticates from the `resume_session` Cookie.
- Use manual Cookie injection only for narrow authentication-negative tests: omit the Cookie to assert `401`, or pass a synthetic invalid value such as `resume_session=invalid-token` to assert rejection. Never reuse or expose a real JWT.
- Test logout and post-logout behavior only when `POST /api/auth/logout` is the selected target or the user explicitly requests session-lifecycle coverage.

## Development-login guardrails

- Use `POST /api/dev/login` only for local protected-route testing, never to test password validation itself.
- Confirm loopback targeting and `APP_ENV=development`; treat `404` as disabled and `403` as a non-loopback rejection.
- Do not use this fixture for a login endpoint test. Validate `POST /api/auth/login` separately with its own credentials and temporary cookie jar when the user selects that endpoint.

## Safety and test-data rules

- Reuse the fixed local `dev@test` account. Create only disposable records beneath it, prefer API-supported cleanup, and do not delete data that existed before the test.
- Never invoke this skill against non-loopback targets, even if a Dev login endpoint responds.
- Redact credentials, cookies, JWTs, full resume content, and personally identifiable data from commands and reports.
