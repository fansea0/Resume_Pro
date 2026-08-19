# Task 18 Report: AI HTTP and SSE integration

## Scope

Task 18 adds the authenticated HTTP boundary between persisted resume context
and the persistence-free AI service.

Implemented files:

- `backend/internal/httpapi/sse.go`
- `backend/internal/httpapi/ai_handler.go`
- `backend/internal/httpapi/ai_handler_test.go`
- `backend/internal/httpapi/router.go`
- `backend/internal/httpapi/router_test.go`

The implementation provides:

- `POST /api/ai/resumes/:id/section/rewrite`
- `POST /api/ai/resumes/generate` as the v1 not-implemented stream
- SSE headers, JSON event framing, and per-event flushing
- authenticated route registration even when AI is disabled
- tenant-scoped resume and target-job context loading
- explicit mapping from `targetjob.TargetJob` to the AI-local
  `ai.TargetJobContext` DTO
- the required `TODO(ai-ratelimit)` marker at both AI entry points

## HTTP and stream contract

### Disabled and pre-stream errors

The AI routes remain registered when `RouterConfig.AIService` is nil. An
authenticated request then receives:

```http
HTTP/1.1 503 Service Unavailable
Content-Type: application/json

{"error":"AI features disabled"}
```

Authentication still runs first, so a disabled deployment does not turn the
AI endpoints into unauthenticated probes.

The handler asks `ai.Service` to create the stream before committing SSE
headers. Synchronous setup failures therefore remain ordinary JSON HTTP
errors. Once streaming starts, the status is 200 and asynchronous model or
schema failures are represented by an SSE `error` event followed by `done`.
The handler never attempts to change HTTP status after the stream has begun.

### SSE framing

Every event is encoded as one JSON value and ends with a blank line:

```text
event: thinking
data: {"text":"..."}

event: suggestion
data: {"newContent":{...},"sectionType":"project"}

event: done
data: {}

```

The response includes:

- `Content-Type: text/event-stream`
- `Cache-Control: no-cache`
- `Connection: keep-alive`
- `X-Accel-Buffering: no`

`sseWriter.WriteEvent` flushes after each complete frame. The Task 17 service
owns terminal ordering: a non-cancelled outcome emits
`suggestion|error`, then `done`.

## Authentication, tenancy, and AI package isolation

The handler derives `TenantID` only from the authenticated principal and uses
it for both reads:

1. `resume.Repository.Get(ctx, principal.TenantID, resumeID)`
2. `targetjob.Service.Get(ctx, principal.TenantID, targetJobID)`

A cross-tenant resume is indistinguishable from a missing resume and never
reaches the fake model. A missing, deleted, or foreign target job is treated
as absent optional context; no target-job fields are included in the AI user
message.

`internal/ai` still imports no auth, resume, targetjob, store, or resumesvc
package. Persistence models are converted at the HTTP boundary:

```go
&ai.TargetJobContext{
    Company:      target.Company,
    Position:     target.Position,
    Requirements: target.Requirements,
}
```

## Review fix: stable section identity

### P1 root cause

The initial rewrite request contained only `sectionType`. The handler scanned
`document.sections` and returned on the first matching type. This caused two
related defects:

1. Two sections of the same type were ambiguous, so the user could request the
   second section while the handler sent the first section to the AI.
2. If the first same-type section was empty, lookup returned an empty-section
   error immediately and never considered the requested later section.

This is a reachable product state. `ResumeEditor` can add multiple sections of
the same type, the stored schema allows them, and every section already has a
stable string `id`.

### Corrected request contract

`sectionId` and `sectionType` are now both required:

```json
{
  "sectionId": "section-42",
  "sectionType": "project",
  "instructions": "突出量化结果"
}
```

The persisted section must match both values before its `content` can cross
the AI boundary. The pre-stream JSON errors are deliberately distinct:

| Condition | Status | JSON error |
| --- | ---: | --- |
| malformed body or missing/blank identity field | 400 | `sectionId and sectionType are required` |
| unsupported rewrite type | 400 | `unsupported section type for AI rewrite` |
| requested ID is absent | 400 | `section not found` |
| requested ID exists with another type | 400 | `section type mismatch` |
| exact ID/type match has no rewriteable content | 400 | `section is empty` |

The regression stores two `summary` sections: the first is empty and the
second is populated. A request for the second ID succeeds, and the recorded AI
user message contains only the second section's content. This proves that an
empty first duplicate cannot block the requested section.

## Downstream frontend contract gap

The original frontend plan still constructs rewrite requests from
`sectionType` alone. Later frontend tasks must use the corrected contract:

- **Task 20:** `RewriteRequest` in `frontend/src/types/ai.ts` must add required
  `sectionId: string`.
- **Task 22:** `streamRewrite` tests must prove the POST body includes both
  `sectionId` and `sectionType`.
- **Task 25:** `AiSuggestionPanel` must receive the selected section identity
  and call `streamRewrite(resumeId, { sectionId, sectionType }, ...)`.
- **Task 27:** `ResumeEditor` must pass `originalSection.id` together with
  `originalSection.type` when opening the AI suggestion flow.

In particular, Task 22/27 must not derive identity from the type or array
position. The stable persisted `ResumeSection.id` is the server contract.
Frontend source was intentionally not changed in Task 18.

## Offline test boundary

All Task 18 AI calls use `internal/llm/testing.FakeChatModel`. HTTP tests use
Gin with `httptest.ResponseRecorder` and isolated temporary SQLite databases.
They do not start the application server, connect to a real LLM, or use an
external network service.

## TDD evidence

### Initial handler RED

The first Task 18 tests were written before router or handler production code.
The focused package failed to compile because the dependency-injection field
did not exist:

```console
$ cd backend
$ go test -count=1 ./internal/httpapi
internal/httpapi/ai_handler_test.go:52:3: unknown field AIService in struct literal of type httpapi.RouterConfig
FAIL    resumepro/backend/internal/httpapi [build failed]
```

After the initial handler implementation, the focused package passed.

### Empty-section RED/GREEN

Blank summary and zero-item experience/project regressions were added before
the handler validated rewriteable content. The RED run showed all four
requests incorrectly entered SSE with status 200:

```console
$ go test -count=1 ./internal/httpapi -run TestAIRewriteRejectsEmptySupportedSections -v
--- FAIL: TestAIRewriteRejectsEmptySupportedSections
    expected: 400
    actual  : 200
FAIL
```

After adding section-type-aware emptiness checks, the same focused test passed
for summary, work, internship, and project.

### Stable-section P1 RED/GREEN

The duplicate-section regression was written before adding `sectionId`. The
RED run failed because strict decoding rejected the new field and the request
never reached the requested second section:

```console
$ go test -count=1 ./internal/httpapi -run TestAIRewriteSelectsRequestedSectionByIDAndType -v
--- FAIL: TestAIRewriteSelectsRequestedSectionByIDAndType
    expected: 200
    actual  : 400
    {"error":"sectionType is required"}
FAIL
```

After adding required `sectionId` plus exact ID/type lookup, the same command
passed:

```console
$ go test -count=1 ./internal/httpapi -run TestAIRewriteSelectsRequestedSectionByIDAndType -v
--- PASS: TestAIRewriteSelectsRequestedSectionByIDAndType
PASS
ok      resumepro/backend/internal/httpapi
```

The complete HTTP API package then passed:

```console
$ go test -count=1 ./internal/httpapi
ok      resumepro/backend/internal/httpapi    21.035s
```

## Final verification

### Focused HTTP API package

```console
$ cd backend
$ go test -count=1 ./internal/httpapi
ok      resumepro/backend/internal/httpapi    22.050s
```

### Full backend suite

```console
$ go test -count=1 ./...
?       resumepro/backend/cmd/server                 [no test files]
ok      resumepro/backend/internal/ai                0.467s
ok      resumepro/backend/internal/ai/prompts        1.237s
ok      resumepro/backend/internal/ai/schemas        1.478s
ok      resumepro/backend/internal/auth              1.841s
ok      resumepro/backend/internal/files             0.997s
ok      resumepro/backend/internal/httpapi           22.894s
ok      resumepro/backend/internal/llm               1.935s
ok      resumepro/backend/internal/llm/testing       3.103s
ok      resumepro/backend/internal/resume            3.388s
ok      resumepro/backend/internal/resumesvc         2.792s
?       resumepro/backend/internal/store             [no test files]
ok      resumepro/backend/internal/targetjob         3.715s
ok      resumepro/backend/internal/templates         3.694s
```

### Build, vet, and architecture checks

```console
$ go vet ./...
# no output; exit 0

$ go build ./...
# no output; exit 0

$ go list -deps ./internal/ai | rg 'resumepro/backend/internal/(auth|resume|targetjob|store|resumesvc)'
# no output; no forbidden dependency

$ rg -n '^\s*"resumepro/backend/internal/(auth|resume|targetjob|store|resumesvc)' internal/ai
# no output; no forbidden import
```

## Commits

- `5bb4b77 feat: 添加 AI 改写 SSE 接口与生成占位`
- P1 stable-section review fix and this report: current follow-up commit
