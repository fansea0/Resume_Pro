# Resume Pro

Resume Pro is a local-first web application for creating, previewing, and exporting resumes. The Go API stores each account's resumes in an isolated personal tenant; the React client provides the template gallery and editor.

## Requirements

- Go 1.22 or later
- Node.js 20 or later
- npm

## Secure local configuration

The API reads the following environment variables when it starts.

| Variable | Local value | Behaviour |
| --- | --- | --- |
| `JWT_SECRET` | a unique random value of at least 32 characters | Required in production. In local development an unset or short value falls back to a non-production secret. Changing the secret invalidates existing sessions. |
| `APP_ENV` | `development` | Enables the local-only development login endpoint only when set exactly to `development`. Use `production` in production. |
| `COOKIE_SECURE` | `false` | Enables the cookie `Secure` attribute only when set exactly to `true`. Keep it `false` for local HTTP development; set it to `true` behind HTTPS. |
| `RESUME_DB_PATH` | `./resume.db` | Optional SQLite database path. |

Generate a local secret without committing it to source control:

```bash
export JWT_SECRET="$(openssl rand -base64 48)"
export APP_ENV=development
export COOKIE_SECURE=false
export RESUME_DB_PATH="./resume.db"
```

`JWT_SECRET` is held only by the server. The browser receives a signed JWT in the `resume_session` HttpOnly cookie; it is not stored in local storage. The cookie uses `SameSite=Lax` and a seven-day lifetime.

Administrator access is controlled only by the `users.is_admin` database field. To grant it locally, update an existing user's row, for example: `UPDATE users SET is_admin = 1 WHERE email = 'admin@example.com';`. No environment variable can grant or revoke administrator access.

## Start locally

Start the API in one terminal:

```bash
cd backend
APP_ENV="$APP_ENV" JWT_SECRET="$JWT_SECRET" COOKIE_SECURE="$COOKIE_SECURE" RESUME_DB_PATH="$RESUME_DB_PATH" go run ./cmd/server
```

For local development, `./scripts/restart.sh`, `./scripts/restart.sh backend`, `./scripts/restart.sh frontend`, and direct `go run ./cmd/server` use a non-production development secret when `JWT_SECRET` is unset. Set `APP_ENV=production` in production; startup then refuses a missing or short secret.

Start the web application in a second terminal:

```bash
cd frontend
npm install
npm run dev
```

Open the Vite address shown in the terminal (normally `http://localhost:5173`). Its development proxy sends `/api` requests to the API at `http://localhost:8080`.

## Development API login

When the API starts with `APP_ENV=development`, `POST /api/dev/login` provides a passwordless local testing login. It is registered only in that exact environment and accepts only TCP loopback requests (`127.0.0.1` or `::1`); it is not available in test, staging, or production.

The request body contains an already registered email. A successful response uses the exact same `resume_session` JWT cookie and `{ "user": ... }` response as ordinary login, so subsequent curl requests exercise the normal authentication middleware.

```bash
cookie_jar="$(mktemp)"
curl --fail --silent --show-error --cookie-jar "$cookie_jar" \
  --header 'Content-Type: application/json' \
  --data '{"email":"existing-user@example.com"}' \
  http://127.0.0.1:8080/api/dev/login

curl --fail --silent --show-error --cookie "$cookie_jar" \
  http://127.0.0.1:8080/api/auth/me
rm -f "$cookie_jar"
```

The endpoint never creates users or tenants. Create a local test user through the normal registration API when needed; do not expose the development server beyond the local machine.

## Authentication and tenant isolation

- Registering with email and password creates a user, a password identity, and a personal tenant.
- Passwords are stored as bcrypt hashes; passwords must contain at least 12 characters.
- All resume create, list, read, update, and delete operations are scoped to the authenticated tenant. A resume from another account is returned as not found.
- The `user_identities` model keeps provider and provider-subject fields, so a future provider such as WeChat can attach to the same user without changing resume ownership.

## Smoke-test checklist

1. Register a new account using a valid email and a password of at least 12 characters; confirm the template gallery opens.
2. Choose a template, create and save a resume, then reload the page and confirm it is still present.
3. Log out, register or log in with a second account, and confirm the first account's resume is absent. Switching back to the first account should show only its own resume.
4. Log out and refresh. Confirm the application shows the login screen and direct resume API requests without the session cookie return `401 Unauthorized`.

## Production notes

Deploy the API and client over HTTPS, set `COOKIE_SECURE=true`, and use a high-entropy, unique `JWT_SECRET` provided by your deployment secret manager. Do not reuse the local secret or commit it to the repository. This MVP assumes same-origin browser/API requests; before allowing credentialed requests across different origins, add explicit CORS allow-lists and CSRF protection.

## AI 写简历功能

当前 AI 能力支持对 `summary`、`work`、`internship`、`project` 四类简历区块进行流式润色。界面和 API 已预留从零生成简历的入口，但后端目前会返回 SSE `NOT_IMPLEMENTED`，该能力尚不可用。

`internal/ai` 只负责提示词、LLM 调用和输出校验，不直接访问认证或数据库；HTTP 与持久化层负责读取当前租户的简历和目标岗位，并按 `tenant_id` 隔离数据。

### 配置

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `OPENAI_API_KEY` | 无 | 必需；填写所用 OpenAI 兼容服务的 API key。 |
| `OPENAI_BASE_URL` | `https://api.deepseek.com/v1` | OpenAI 兼容服务的 base URL。 |
| `OPENAI_MODEL` | `deepseek-chat` | 模型名。 |
| `AI_MAX_TOKENS` | `2048` | 单次响应 token 上限；无效或非正整数会使用默认值。 |
| `AI_REQUEST_TIMEOUT` | `30s` | 上游请求超时；无效或非正 duration 会使用默认值。 |
| `AI_MAX_RETRIES` | `1` | LLM 输出未通过 schema 校验后的重试次数，允许 `0` 到 `10`；无效值属于配置错误。 |

非生产环境未设置 `OPENAI_API_KEY` 时，服务仍可启动，需登录的 AI 路由返回 `503` 和 `{"error":"AI features disabled"}`，其他路由不受影响。无效的 `AI_MAX_RETRIES` 在非生产环境也会禁用 AI；`APP_ENV=production` 时缺少 key 或该变量无效都会导致启动失败。LLM 客户端构造失败则在所有环境中阻止启动。

### 字段级改写权限

- AI 润色仅支持 `summary`、`work`、`internship`、`project`；`education`、`skills`、`campus`、`honours` 不显示润色入口。
- `summary` 输出只包含 `markdown`；`work` 和 `internship` 的条目输出只包含 `id`、`achievements`、`highlightsMarkdown`；`project` 条目还可包含 `introduction`。
- 公司、职位、日期、学校等事实字段不在改写输出 schema 中；接受建议时仅合并上述可改字段。

### 目标岗位

- 目标岗位是当前用户的资产，可维护多份；`requirements` 去除首尾空白后按当前后端的字节长度校验至少为 `10`，`company` 和 `position` 可留空。
- 简历可通过 `targetJobId` 绑定一份目标岗位；创建或更新简历时会校验该目标岗位属于当前租户。

## Verification

```bash
cd backend && go test ./...
cd frontend && npm test -- --run && npm run build
```
