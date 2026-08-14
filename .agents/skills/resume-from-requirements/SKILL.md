---
name: resume-from-requirements
description: Create a polished, truthful resume from a user's raw job-search materials, select or recommend an available template in this resume platform, and optionally save it to the user's account. Use when a user asks to generate, draft, tailor, or save a resume and provides (or needs help providing) their experience, target role, template choice, or platform account.
---

# Resume From Requirements

Turn supplied career material into a complete, editable platform resume. Treat the platform's current template registry and the user's authenticated account as the source of truth.

## Discover the platform first

Before suggesting a template or saving anything, inspect the workspace:

1. Read `frontend/src/templates/registry.tsx` for current template IDs, Chinese names, descriptions, tags, and style descriptions.
2. Read `frontend/src/types/resume.ts` for the current `ResumeDocument` shape.
3. Read `frontend/src/api/resumes.ts` and `frontend/src/api/templates.ts` for the active save and disabled-template contracts.
4. If the app/API is running, obtain public disabled-template status before recommending. Do not recommend a disabled template.

Do not hard-code a template catalog in this skill; the registry changes over time.

## Intake and missing information

Extract supplied prose, notes, a previous resume, or job description into this minimum intake:

- Account email that should own the resume.
- Name, target role, target industry or job description, and city/work preference when relevant.
- Contact details the user wants displayed.
- Work experience: company, role, dates, responsibilities, outcomes, and metrics only when supplied.
- Projects, education, skills, and optional portfolio links.
- Template name, if the user has one.

Ask only for the missing information that blocks a credible result. If the template is absent, do **not** block on the entire intake: recommend templates first. Use a compact prompt such as:

> 我已收到你的经历。请选择模板：`极简专业`（产品/技术）、`现代商务`（管理/咨询）或 `赤线单栏`（中文互联网）。如需我推荐，请补充目标岗位和年限。

If experience detail is sparse, ask for the role, dates, 2–3 responsibilities or outcomes, and education rather than inventing achievements. Clearly mark optional gaps as “待补充”.

## Choose a template

1. Match an explicit user-provided template by its current Chinese name or ID. Confirm the match if more than one is plausible.
2. If it is unavailable or disabled, say so and show the available alternatives with their tags and style description.
3. If no template is specified, recommend at most two available templates based on target job, seniority, content volume, and the registry metadata. Give one short reason per recommendation.
4. Ask the user to select one before generating the final formatted resume. Do not silently fall back to a default template.

Use the registry's own wording. Never claim a template supports a role that its tags and description do not support.

## Account and session handling

- Confirm the account email with the user before saving.
- Use the platform's existing authenticated browser/API session. If there is no session or it belongs to another account, direct the user to log in through the platform, then re-check the current account.
- Never ask the user to paste a password, JWT, cookie, or other secret into chat or a skill file.
- Generate a preview-only document until the user explicitly requests “保存” / “生成并保存”, or otherwise grants clear save authority.

## Build the document

Create a `ResumeDocument` that exactly matches the current type contract:

- Use the selected `templateId` and a specific title, for example `王晨｜产品经理简历`.
- Use stable, unique IDs for every experience, project, education, and skill item.
- Preserve the user's language and factual claims. Rewrite for clarity and relevance, but never fabricate employers, credentials, dates, technologies, salary, rankings, or quantitative outcomes.
- Prefer concise action–context–outcome bullets. Keep a claim qualitative when the user did not provide a measurable result.
- Omit empty optional sections instead of adding filler. Keep editable empty strings only when the platform schema requires them.

Before saving, present a compact confirmation containing:

- Account email and resume title.
- Selected template name, tags, and style description.
- Sections included and every material assumption or “待补充” item.

## Save and verify

When explicit save authority exists:

1. Create a new resume through the platform's supported UI or current `POST /api/resumes` contract using the authenticated session.
2. Do not overwrite an existing resume unless the user named that draft or explicitly asked to replace it.
3. Verify the success response has an ID, title, template ID, and persisted content.
4. Reload the draft list or saved record and confirm it belongs to the active account and has the selected template.
5. Report the saved draft title and ID. If save or verification fails, report the failure plainly; never claim it was saved.

## Final response

For preview-only work, return the selected template and the generated resume content in a concise, editable structure, followed by the exact missing items if any.

For saved work, return the same summary plus confirmation that the draft was verified in the named account. Offer one next action: refine content, switch template, or open the draft for editing.
