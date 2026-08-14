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
| `COOKIE_SECURE` | `false` | Enables the cookie `Secure` attribute only when set exactly to `true`. Keep it `false` for local HTTP development; set it to `true` behind HTTPS. |
| `RESUME_DB_PATH` | `./resume.db` | Optional SQLite database path. |

Generate a local secret without committing it to source control:

```bash
export JWT_SECRET="$(openssl rand -base64 48)"
export COOKIE_SECURE=false
export RESUME_DB_PATH="./resume.db"
```

`JWT_SECRET` is held only by the server. The browser receives a signed JWT in the `resume_session` HttpOnly cookie; it is not stored in local storage. The cookie uses `SameSite=Lax` and a seven-day lifetime.

## Start locally

Start the API in one terminal:

```bash
cd backend
JWT_SECRET="$JWT_SECRET" COOKIE_SECURE="$COOKIE_SECURE" RESUME_DB_PATH="$RESUME_DB_PATH" go run ./cmd/server

For local development, `./scripts/restart.sh`, `./scripts/restart.sh backend`, `./scripts/restart.sh frontend`, and direct `go run ./cmd/server` use a non-production development secret when `JWT_SECRET` is unset. Set `APP_ENV=production` in production; startup then refuses a missing or short secret.
```

Start the web application in a second terminal:

```bash
cd frontend
npm install
npm run dev
```

Open the Vite address shown in the terminal (normally `http://localhost:5173`). Its development proxy sends `/api` requests to the API at `http://localhost:8080`.

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

## Verification

```bash
cd backend && go test ./...
cd frontend && npm test -- --run && npm run build
```
