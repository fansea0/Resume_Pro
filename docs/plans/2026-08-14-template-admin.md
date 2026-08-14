# 模板后台管理 Implementation Plan

> **For implementer:** Use TDD throughout. Write failing test first. Watch it fail. Then implement.

**Goal:** 管理员账号可在后台查看并禁用简历模板；普通用户无入口且无法调用管理 API。

**Architecture:** SQLite 保存用户管理员角色与禁用模板状态；管理员角色仅由 `users.is_admin` 管理。JWT 与 `/api/auth/me` 暴露只读角色。模板注册表保留在前端，前端依据受保护 API 返回的禁用模板 ID 过滤主页，并用管理 API 变更状态。

**Tech Stack:** Go、sqlx、SQLite、JWT、React、TypeScript、Vitest、React Testing Library。

---

### Task 1: 管理员角色迁移与启动同步

**Files:**
- Modify: `backend/internal/store/migrate.go`
- Modify: `backend/internal/auth/model.go`
- Modify: `backend/internal/auth/repository.go`
- Modify: `backend/internal/auth/repository_test.go`
- Modify: `backend/cmd/server/main.go`
- Modify: `README.md`

**Step 1: Write failing tests**

增加仓储测试：迁移后新用户默认 `IsAdmin == false`；`SetAdministrator` 可通过用户 ID 设置及撤销数据库角色。

**Step 2: Run test — confirm it fails**

Command: `cd backend && go test ./internal/auth -run 'Test.*Admin' -v`

Expected: FAIL — missing `IsAdmin`/administrator synchronization API.

**Step 3: Implement minimally**

在 `users` 表使用幂等列迁移添加 `is_admin INTEGER NOT NULL DEFAULT 0`；`User` 映射该字段，所有用户查询读出字段。仓储提供按用户 ID 设置数据库角色的方法；启动过程不读取管理员配置。README 记录数据库字段为唯一管理员权限来源。

**Step 4: Run tests — confirm pass**

Command: `cd backend && go test ./internal/auth -run 'Test.*Admin' -v && go test ./...`

Expected: PASS.

**Step 5: Commit**

`git add backend/internal/store/migrate.go backend/internal/auth/model.go backend/internal/auth/repository.go backend/internal/auth/repository_test.go backend/cmd/server/main.go README.md && git commit -m "feat: 添加管理员角色同步"`

### Task 2: JWT 角色和模板管理 API

**Files:**
- Create: `backend/internal/templates/repository.go`
- Create: `backend/internal/templates/repository_test.go`
- Modify: `backend/internal/auth/service.go`
- Modify: `backend/internal/httpapi/auth_handler.go`
- Modify: `backend/internal/httpapi/auth_middleware.go`
- Modify: `backend/internal/httpapi/router.go`
- Modify: `backend/internal/httpapi/auth_handler_test.go`
- Create: `backend/internal/httpapi/template_handler_test.go`

**Step 1: Write failing tests**

测试登录和 `/api/auth/me` 返回 `isAdmin`；测试普通用户访问 `GET /api/admin/templates` 与 `DELETE /api/admin/templates/minimal` 为 403；管理员可获取禁用 ID 列表，删除模板后列表含该 ID，重复删除仍成功。

**Step 2: Run test — confirm it fails**

Command: `cd backend && go test ./internal/httpapi -run 'Test.*(Admin|Template)' -v`

Expected: FAIL — missing role claim/routes/handlers.

**Step 3: Implement minimally**

增加 `disabled_templates` 迁移与简洁仓储（ListDisabled、Disable）。JWT Claims/Principal 添加 `IsAdmin`，只接受有效签名的声明。认证公开用户 JSON 加 `isAdmin`。添加 `requireAdministrator` 中间件与管理路由：`GET /api/admin/templates` 返回 `{ disabledTemplateIds: string[] }`，`DELETE /api/admin/templates/{templateID}` 仅接受非空 ID 并幂等禁用。不得提供公开角色写接口。

**Step 4: Run tests — confirm pass**

Command: `cd backend && go test ./internal/httpapi -run 'Test.*(Admin|Template)' -v && go test ./...`

Expected: PASS.

**Step 5: Commit**

`git add backend/internal/templates backend/internal/store/migrate.go backend/internal/auth/service.go backend/internal/httpapi && git commit -m "feat: 添加管理员模板管理接口"`

### Task 3: 前端管理页与模板主页过滤

**Files:**
- Create: `frontend/src/api/templates.ts`
- Create: `frontend/src/api/templates.test.ts`
- Create: `frontend/src/components/TemplateAdmin.tsx`
- Create: `frontend/src/components/TemplateAdmin.test.tsx`
- Modify: `frontend/src/api/auth.ts`
- Modify: `frontend/src/app.tsx`
- Modify: `frontend/src/app.test.tsx`
- Modify: `frontend/src/components/TemplateGallery.tsx`
- Modify: `frontend/src/components/TemplateGallery.test.tsx`
- Modify: `frontend/src/styles/global.css`

**Step 1: Write failing tests**

新增应用测试：普通用户不存在“后台管理”入口；管理员点击入口显示“模板管理”；管理页加载模板、点击“删除模板”后调用 delete API 并显示“已禁用”。模板主页测试：传入禁用 `minimal` 后不显示该模板。

**Step 2: Run test — confirm it fails**

Command: `cd frontend && npm test -- --run src/app.test.tsx src/components/TemplateGallery.test.tsx src/components/TemplateAdmin.test.tsx`

Expected: FAIL — missing admin user field/admin page/template status API.

**Step 3: Implement minimally**

`AuthUser` 增加 `isAdmin`，新 API 查询禁用状态与调用禁用请求。`App` 新增 `admin` 视图：仅管理员导航可达，加载失败时主页不阻塞且不隐藏模板。`TemplateGallery` 接收 `disabledTemplateIds` 并过滤。`TemplateAdmin` 以静态 registry 为目录显示卡片和状态；删除动作调用 `window.confirm`，成功后刷新局部状态，错误用 `role=alert` 呈现。管理页不直接删除静态源文件或已保存的简历。

**Step 4: Run tests — confirm pass**

Command: `cd frontend && npm test -- --run src/app.test.tsx src/components/TemplateGallery.test.tsx src/components/TemplateAdmin.test.tsx && npm run build`

Expected: PASS.

**Step 5: Commit**

`git add frontend/src/api/templates.ts frontend/src/api/templates.test.ts frontend/src/components/TemplateAdmin.tsx frontend/src/components/TemplateAdmin.test.tsx frontend/src/api/auth.ts frontend/src/app.tsx frontend/src/app.test.tsx frontend/src/components/TemplateGallery.tsx frontend/src/components/TemplateGallery.test.tsx frontend/src/styles/global.css && git commit -m "feat: 添加模板后台管理页面"`

### Task 4: 全量回归验证

**Files:**
- Modify only if a confirmed expectation drift requires it: corresponding test file.

**Step 1: Run full verification**

Command: `cd backend && go test ./... && cd ../frontend && npm test -- --run && npm run build`

Expected: all backend/frontend tests and production build pass.

**Step 2: Fix only an observed regression**

先写能证明回归的失败测试，再作最小修复；不得改变权限或删除语义。

**Step 3: Re-run full verification**

Command: `cd backend && go test ./... && cd ../frontend && npm test -- --run && npm run build`

Expected: PASS.

**Step 4: Commit if required**

`git add <only changed regression files> && git commit -m "test: 覆盖模板后台管理回归"`
