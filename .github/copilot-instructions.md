# GoblinFTP — Copilot Instructions

GoblinFTP is a self-hosted web-based FTP/SFTP client (rewrite of MonstaFTP). Go backend + Nuxt 4 SPA frontend, deployed as a single Docker container.

## Commands

All commands run via [just](https://just.systems). `.env` is auto-loaded by `set dotenv-load`.

```bash
# Development
just dev          # overmind: frontend + backend hot-reload (requires overmind)
just dev-fe       # frontend only (pnpm run dev → http://localhost:3000)
just dev-be       # backend only (go run → http://localhost:8080)

# Test
just test-be                                    # all Go tests
cd backend && go test ./internal/api/...        # single Go package
cd backend && go test -run TestConnectSuccess . # single Go test
just test-fe                                    # vitest run (passWithNoTests)
cd frontend && pnpm test:watch                  # vitest watch mode

# Build
just build-fe     # nuxt generate → frontend/.output/public/
just build-be     # go build → bin/gftp

# Lint / format
just lint-fe      # eslint + nuxt typecheck
just lint-be      # golangci-lint
just fmt          # prettier (frontend) + gofmt (backend)

# Utilities
just ftp-up       # start garethflowers/ftp-server on 20-21 (docker compose --profile testing)
just i18n-check   # verify de.json has all keys from en.json
```

## Architecture

```
goblinftp/
├── backend/               # Go 1.26, Echo v4
│   ├── cmd/gftp/main.go  # entry point: config load → newApp() → e.Start()
│   └── internal/
│       ├── api/           # all HTTP handlers + routing
│       ├── auth/          # session store (in-memory, TTL) + CSRF token generation
│       ├── config/        # env + settings.json loading
│       ├── errors/        # GFTPError type with machine-readable codes
│       ├── ftp/           # jlaffaye/ftp adapter → implements transfer.Client
│       ├── sftp/          # pkg/sftp adapter → implements transfer.Client
│       ├── sentry/        # custom Echo v4 Sentry middleware (sentry-go/echo is v5 only)
│       ├── sso/           # SSO token validation + one-time-use set
│       └── transfer/      # Client interface, chunked upload engine, download tokens
├── frontend/              # Nuxt 4 SPA (ssr: false), Nuxt UI v4, Tailwind v4
│   └── app/
│       ├── pages/index.vue        # single page — all app state from Pinia stores
│       ├── stores/                # auth, files, editor, upload, modal
│       ├── components/            # Auth/, Editor/, FileBrowser/, Layout/, Modals/, Upload/
│       ├── composables/useApi.ts  # wraps $fetch, injects CSRF header, unwraps API envelope
│       └── types/api.ts           # shared TypeScript interfaces matching backend JSON
├── docker/                # Dockerfile (Caddy + Go binary)
├── justfile               # task runner
└── settings.example.json  # runtime settings schema reference
```

### Request lifecycle

1. Browser → Vite proxy `/api/*` → Go Echo backend (dev only; in prod Caddy handles routing)
2. All API responses use the `Response` envelope: `{ success: bool, data?, errors?: [{code, message}] }`
3. CSRF: backend issues a token in `data.csrfToken` on connect; frontend sends it as `X-CSRF-Token` on every mutating request via `useApi`
4. Session stored in `gftp_session` cookie (HTTP-only); `transfer.Client` lives in `session.Data["client"]`
5. FTP/SFTP operations call through `transfer.Client` interface — same handler code for both protocols

### API envelope pattern

All backend handlers use two helpers — never write raw `c.JSON`:

```go
return api.OK(c, someData)                        // 200 { success: true, data: ... }
return api.Fail(c, gftperrors.New(errors.ErrX, "msg")) // appropriate HTTP status
```

`GFTPError` codes map to HTTP status via `errors.go:HTTPStatus()`.

### Frontend API calls

All API calls go through `useApi()`. Never call `$fetch` directly in components/stores (except `authStore.init()` and `authStore.connect()` which intentionally bypass CSRF):

```ts
const api = useApi()
const data = await api.get<FileInfo[]>('/api/files?path=...')
await api.patch('/api/files/rename', { from, to })
```

### Adding a new modal

1. Add the type to `ModalType` in `stores/modal.ts`
2. Create `components/Modals/YourModal.vue` — use `<UModal :open="modalStore.active === 'yourType'" @update:open="modalStore.close()">`
3. Add `<YourModal />` to `pages/index.vue`
4. Add i18n keys to both `i18n/locales/en.json` and `i18n/locales/de.json`

## Key conventions

### Backend

- **Error codes**: use constants from `internal/errors/errors.go` — never invent string literals
- **Testing**: all API tests live in `package api_test` (black-box). Use `newTestApp(t, defaultTestConfig())` + `testutil.MockClient` to avoid real FTP/SFTP connections. Use `WithDial(...)` handler option to inject the mock.
- **`transfer.Client`** is stored in `session.Data["client"]`; retrieve via `clientFromContext(c)` (returns `(Client, bool)`)
- `internal/ftp` and `internal/sftp` are integration-level; unit tests there require a real server — use `just ftp-up` for that
- **No test for** `internal/sentry` (intentional — Sentry SDK is not unit-testable in isolation)

### Frontend

- Components are registered with `pathPrefix: false` — `Auth/LoginForm.vue` is used as `<LoginForm />`, not `<AuthLoginForm />`
- **Pinia stores** use the Composition API style (`defineStore('id', () => { ... })`) — not the Options style
- `FileInfo` fields are `name`, `size`, `isDir`, `modified` (RFC3339 string), `mode` (e.g. `"drwxr-xr-x"`) — the backend `transfer.FileInfo` uses different field names internally
- `UProgress` uses `:model-value` not `:value`
- TypeScript strict mode is on including `noUncheckedIndexedAccess` — array/object index access returns `T | undefined`; use `!` after a length guard or optional chaining
- i18n: `en.json` is the source of truth; `just i18n-check` verifies `de.json` parity

### Configuration

Backend config is layered: env vars override `settings.json`. Key env vars:

| Env | Purpose |
|-----|---------|
| `GFTP_SESSION_SECRET` | Session cookie signing (auto-gen if unset) |
| `GFTP_DOWNLOAD_TOKEN_SECRET` | Download HMAC token signing |
| `GFTP_SSO_ENABLED` / `GFTP_SSO_SECRET` | SSO link validation |
| `GFTP_SENTRY_DSN` | Backend Sentry (optional) |
| `NUXT_PUBLIC_SENTRY_DSN` | Frontend Sentry (optional) |
| `GFTP_SETTINGS_PATH` | Path to `settings.json` (default `/app/data/settings.json`) |

### Docker / local FTP testing

```bash
just ftp-up   # starts garethflowers/ftp-server on ports 20-21
              # FTP_USER=ftpuser FTP_PASS=ftppass (set in .env or docker-compose)
just ftp-down
```

The FTP container is on profile `testing` — only `just ftp-up/ftp-down` activate it.
