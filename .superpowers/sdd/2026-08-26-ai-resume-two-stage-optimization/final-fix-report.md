# Final review fixes — 2026-08-26

## Scope and root cause

1. `rewriting_strategy.tmpl` retained an obsolete S3–S8 mapping. It now uses the approved ordered taxonomy exactly: S1 量化缺失, S2 动词贫瘠, S3 职责堆砌/SAR, S4 成果模糊/价值翻译, S5 关键词缺失/JD 术语注入, S6 句式平庸/结构化并列, S7 劣势暴露, S8 冗长啰嗦/信息降噪. S7 remains warnings-only and keeps the existing server-side safety constraints.
2. The panel selected `label ?? resolution`, so a valid labeled issue hid its resolution. It now renders the issue code/label and `如何修复：<resolution>`.
3. `hasSafeRewrite` treated any allowed field present in the wrapper as adoptable, including a value equal to the original. It now compares summary markdown and each allowed work/internship/project field against the original item value.

## Regression coverage and TDD evidence

- Added ordered prompt-taxonomy assertions in `backend/internal/ai/prompts/embed_test.go`.
- Extended the panel analysis case with S2/S5 labels and resolutions, asserting both are visible.
- Added table-driven no-op adoption cases for `summary`, `work`, `internship`, and `project`; each requires the “保留原文” state and a disabled adoption button.
- Red verification: before the frontend implementation, `npm test -- --run src/components/AiSuggestionPanel.test.tsx` failed 5 assertions: the label-only display omitted “替换弱动词”, and all four no-op wrappers remained adoptable. Before the prompt implementation, `go test ./internal/ai/prompts -count=1` failed because the approved taxonomy was absent.

## Verification

All commands exited 0 after the fixes:

```text
cd backend && go test ./internal/ai/prompts -count=1
cd frontend && npm test -- --run src/components/AiSuggestionPanel.test.tsx
cd backend && go test ./internal/ai ./internal/ai/schemas ./internal/httpapi -count=1
cd frontend && npm test -- --run src/api/ai.test.ts src/components/AiSuggestionPanel.test.tsx
cd frontend && npm run build
cd backend && go test ./...
cd frontend && npm test -- --run
git diff --check
```

Focused frontend verification: 18 panel tests passed; focused API plus panel verification: 27 tests passed. The full backend suite passed. The full frontend suite passed; it emitted existing React duplicate-key warnings in `ResumeSections` from unrelated blue-sidebar and mint-canvas template tests, but had no test failures. Frontend production build passed. `git diff --check` passed.
