# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

GoblinFTP is a self-hosted web-based FTP/SFTP client: Go + Echo v4 backend, Nuxt 4 SPA frontend, deployed as a single Docker container (Caddy + Go binary).

## Commands

All commands run via [just](https://just.systems). `.env` is auto-loaded.

```bash
just dev          # overmind: frontend (3000) + backend (8080) hot-reload
just dev-fe       # frontend only
just dev-be       # backend only

just test                                         # all tests
just test-be                                      # all Go tests
cd backend && go test ./internal/api/...          # single Go package
cd backend && go test -run TestConnectSuccess .   # single Go test
just test-fe                                      # vitest (passWithNoTests)
cd frontend && pnpm test:watch                    # vitest watch

just lint         # eslint + nuxt typecheck + golangci-lint
just fmt          # prettier (frontend) + gofmt (backend)
just i18n-check   # verify de.json has all keys from en.json
just ftp-up       # local FTP test server (ftpuser/ftppass on :21); ftp-down stops it
just s3-up        # local MinIO for S3 chunk staging (minioadmin/minioadmin on :9000); s3-down stops it
just build        # build-fe (nuxt generate) + build-be (go build → bin/gftp)
```

## Architecture

```
backend/
  cmd/gftp/main.go        # entry: config load → newApp() → e.Start()
  internal/
    api/                  # all HTTP handlers + routing
    auth/                 # in-memory session store (TTL) + CSRF token gen
    config/               # env + settings.json loading
    errors/               # GFTPError with machine-readable codes
    ftp/                  # jlaffaye/ftp adapter   → implements transfer.Client
    sftp/                 # pkg/sftp adapter       → implements transfer.Client
    sentry/               # custom Echo v4 Sentry middleware (sentry-go/echo is v5-only)
    sso/                  # SSO token validation + one-time-use set
    staging/              # ChunkStore interface: local-disk (default) + optional S3 chunk staging
    transfer/             # Client interface, chunked upload engine, download tokens
frontend/app/
  pages/index.vue         # single page — all state via Pinia stores
  stores/                 # auth, files, editor, upload, modal (Composition API style)
  composables/useApi.ts   # wraps $fetch, injects CSRF header, unwraps API envelope
  types/api.ts            # TS interfaces mirroring backend JSON
```

### Request lifecycle

1. Browser → (dev: Vite proxy `/api/*` | prod: Caddy) → Go Echo backend.
2. All responses use the `Response` envelope: `{ success, data?, errors?: [{code, message}] }`.
3. CSRF: backend returns a token in `data.csrfToken` on connect; frontend sends it as `X-CSRF-Token` on every mutating request via `useApi`.
4. Session in `gftp_session` cookie (HTTP-only); `transfer.Client` lives in `session.Data["client"]`.
5. FTP and SFTP share handler code via the `transfer.Client` interface.

### Backend conventions

- **Never** write raw `c.JSON` — always `api.OK(c, data)` or `api.Fail(c, gftperrors.New(errors.ErrX, "msg"))`. `GFTPError` codes map to HTTP status via `errors.go:HTTPStatus()`.
- **Error codes**: use constants from `internal/errors/errors.go` — never invent string literals.
- **API tests** live in `package api_test` (black-box). Use `newTestApp(t, defaultTestConfig())` + `testutil.MockClient` to avoid real FTP/SFTP. Inject the mock with the `WithDial(...)` handler option.
- `transfer.Client` is retrieved from session via `clientFromContext(c)` → `(Client, bool)`.
- Upload chunk staging is abstracted behind `staging.ChunkStore` (local disk default, S3 via `GFTP_S3_ENABLED`); inject mocks with the `WithChunkStore(...)` handler option. The aws-sdk-go-v2 dependency lives only in `internal/staging`.
- `internal/ftp` and `internal/sftp` are integration-level — real-server tests require `just ftp-up`. S3 integration tests in `internal/staging` are gated by `GFTP_TEST_S3_ENDPOINT` (requires `just s3-up`).
- `internal/sentry` is intentionally not unit-tested.

### Frontend conventions

- All API calls go through `useApi()` — never `$fetch` directly, except `authStore.init()` / `authStore.connect()` which intentionally bypass CSRF.
- Components use `pathPrefix: false` — `Auth/LoginForm.vue` is `<LoginForm />`, not `<AuthLoginForm />`.
- Pinia stores use **Composition API style**: `defineStore('id', () => { ... })`.
- `FileInfo` JSON fields: `name`, `size`, `isDir`, `modified` (RFC3339), `mode` (`"drwxr-xr-x"`). Backend's internal `transfer.FileInfo` uses different field names.
- TypeScript strict mode incl. `noUncheckedIndexedAccess` — index access is `T | undefined`; use `!` after a length guard or optional chaining.
- `UProgress` uses `:model-value`, not `:value`.
- i18n: `en.json` is source of truth; run `just i18n-check` to verify `de.json` parity.

### Adding a new modal

1. Add the type to `ModalType` in `stores/modal.ts`.
2. Create `components/Modals/YourModal.vue` using `<UModal :open="modalStore.active === 'yourType'" @update:open="modalStore.close()">`.
3. Mount `<YourModal />` in `pages/index.vue`.
4. Add i18n keys to both `i18n/locales/en.json` and `de.json`.

## Configuration

Backend config is layered: env vars override `settings.json` (schema in `settings.example.json`, default path `/app/data/settings.json`).

| Env | Purpose |
|-----|---------|
| `GFTP_SESSION_SECRET` | Session cookie signing (auto-gen if unset) |
| `GFTP_DOWNLOAD_TOKEN_SECRET` | Download HMAC token signing |
| `GFTP_SSO_ENABLED` / `GFTP_SSO_SECRET` | SSO link validation |
| `GFTP_SENTRY_DSN` / `NUXT_PUBLIC_SENTRY_DSN` | Sentry (optional) |
| `GFTP_SETTINGS_PATH` | Path to `settings.json` |
| `GFTP_S3_ENABLED` + `GFTP_S3_ENDPOINT` / `GFTP_S3_BUCKET` / `GFTP_S3_ACCESS_KEY` / `GFTP_S3_SECRET_KEY` (+ optional `GFTP_S3_REGION`, `GFTP_S3_USE_PATH_STYLE`, `GFTP_S3_PREFIX`, `GFTP_S3_TIMEOUT_SECS`) | Optional S3 chunk staging — env-only, never in `settings.json` |

The FTP test container and MinIO are on docker compose profile `testing` — only `just ftp-up/ftp-down` and `just s3-up/s3-down` activate them.
