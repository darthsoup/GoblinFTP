# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

GoblinFTP is a self-hosted web-based FTP/SFTP client: Go + Echo v4 backend, Nuxt 4 SPA frontend, deployed as a single Docker container (Caddy + Go binary).

## Commands

All commands run via [just](https://just.systems). `.env` is auto-loaded. The frontend lives in a pnpm workspace rooted at the repo root (single `pnpm-lock.yaml` + `pnpm-workspace.yaml` there): run `pnpm install` at the root; package scripts work both via `cd frontend && pnpm …` and `pnpm --filter goblinftp-frontend …`.

```bash
just dev          # concurrently: frontend (3000) + backend (8080) hot-reload
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
just sso-link     # generate a one-time SSO login link (examples/sso/ has Node + PHP generators)
just build        # build-fe (nuxt generate) + build-be (go build → bin/gftp)
```

Running the app (e.g. for visual verification): prefer `just dev` for the full stack — but check first whether the dev servers are already up (the user often has them running: frontend :3000, backend :8080). If only one half is missing, start just that half (`just dev-fe` / `just dev-be`) to avoid port conflicts; never kill the user's processes. For a backend with special env (custom port, SSO, S3), run `cd backend && GFTP_PORT=… go run ./cmd/gftp` directly on a free port instead.

## Architecture

```
backend/
  cmd/gftp/main.go        # entry: config load → newApp() → e.Start()
  cmd/gftp-sso-link/      # CLI: generate one-time SSO login links (reuses internal/sso)
  internal/
    api/                  # all HTTP handlers + routing
    auth/                 # in-memory session store (TTL) + CSRF token gen
    config/               # env + settings.json loading
    errors/               # GFTPError with machine-readable codes
    ftp/                  # jlaffaye/ftp adapter   → implements transfer.Client
    logging/              # slog Init (stdout + optional lumberjack file sink) + SafeLogAttrs redaction
    metrics/              # Prometheus registry, collectors, CountingReader (opt-in /metrics listener)
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
- **Lint**: `just lint-be` runs golangci-lint v2 (config: `backend/.golangci.yml`; install: `brew install golangci-lint`; CI pins the same version). `nolint` directives must be specific and carry an explanation (`//nolint:gosec // G101: …`) — nolintlint enforces this. `just fmt` formats the backend via `golangci-lint fmt` (gofmt + goimports with local-prefix grouping).
- **Metrics**: `internal/metrics` owns the Prometheus registry; the `Metrics` instance lives on `Handler` (default-constructed in `newHandler`, override via `WithMetrics(...)` — main.go shares its registry with the dedicated `/metrics` listener, served only when `GFTP_METRICS_ENABLED=true`, never on the main echo). Gauges (`sessions_active`, `connections_active{protocol}`) are scrape-time snapshots of `auth.Store` via a custom collector (`SetConnectionSnapshot`) — no inc/dec drift. Counters increment at call sites: connect results in `connect.go`/`sso.go`, transfer bytes via `metrics.CountingReader` wraps in `download.go`/`archive.go`/`upload.go` (chunk staging writes are NOT counted — only bytes to/from the FTP/SFTP server), frontend reports in `frontendlog.go`. `metricsMiddleware` sits between `RequestID` and `requestLogger` and must NOT call `c.Error` (the logger owns that); it labels by `c.Path()` route template (`"unmatched"` when empty) and skips `/healthz`.
- **Logging**: one structured access line per request via `requestLogger` middleware (`api/middleware_logging.go`). `Fail()` stashes the GFTPError in the echo context (`LoggedErrorKey`) for that line — handlers return nil after Fail, so the error can't travel via the return value. Attach root causes with `gftperrors.New(...).WithCause(err)` (logged as `cause`, never serialized into the envelope). Never log passwords/tokens/full session IDs (8-char prefix only; dynamic attrs go through `logging.SafeLogAttrs`). Tests assert log output via `newTestAppWithLog(t, cfg, &buf)` + `logLines`. Browser errors arrive at `POST /api/log/frontend` (public, CSRF-exempt, per-IP-throttled; SPA side: `plugins/error-reporter.client.ts`, gated by systemVars `frontendLogEnabled`). Streaming endpoints log the committed status — a mid-stream failure still shows 200.

### Frontend conventions

- **Design system**: dual-theme — "Goblin Tech-Dark" (`DESIGN.md`) and "Goblin Tech-Light" (`DESIGN-Gobin-Tech-Light.md`), switched via colorMode (settings modal: Light/Dark/Automatic, default Automatic). Nuxt UI tokens are overridden in `app/assets/css/main.css`: light tokens on `:root, .light`, dark on `.dark`; primary alias is the custom `goblin` green scale (light uses goblin-700, dark goblin-400); the `neutral` scale is overridden to charcoal-navy. Style with token utilities (`bg-default/muted/elevated/accented`, `text-muted/dimmed/highlighted`, `border-default/accented`) — never `gray-*` or `dark:` variants; the tokens flip per mode, components stay mode-agnostic. Mode-specific values that tokens don't cover live in `--gftp-*` custom properties (popover, editor-bg, scrollbar, selection).
- Fonts are self-hosted via `@fontsource-variable` (Inter = `font-sans`, JetBrains Mono = `font-mono`). Mono is used for all data: paths, sizes, dates, permissions, breadcrumbs, status bar. `label-caps` utility (defined in main.css) for column headers / field labels.
- Icons: `i-lucide-*` (plus `i-simple-icons-*` for file types in `FileRow.vue`) — both icon sets installed locally; do not use heroicons.
- All API calls go through `useApi()` — never `$fetch` directly, except `authStore.init()` / `authStore.connect()` which intentionally bypass CSRF.
- **VueUse** is available via `@vueuse/nuxt` (auto-imported `use*` composables — `useMediaQuery`, `useEventListener`, `useIntervalFn`, …); prefer it over hand-rolled listeners/timers/media queries. The module is listed first in `nuxt.config` so Nuxt's `useColorMode` (from `@nuxtjs/color-mode`) wins the name collision — don't reorder it.
- **White-label branding**: `useBranding()` exposes `appName`/`logoUrl`/`tagline`/`hideAttribution` (from `systemVars.branding`, with `'GoblinFTP'`/green fallbacks); the runtime accent color is injected by `plugins/auth.client.ts` via `utils/branding.ts` `applyBrandColor()` (overrides the `--color-goblin-*` scale). Document title/favicon are set with `useHead` in that plugin. New brand surfaces must read `useBranding()`, never hardcode "GoblinFTP".
- Components use `pathPrefix: false` — `Auth/LoginForm.vue` is `<LoginForm />`, not `<AuthLoginForm />`.
- Pinia stores use **Composition API style**: `defineStore('id', () => { ... })`.
- End-user preferences are browser-side only (the backend never reads them): `stores/settings.ts` persists to localStorage `gftp_settings` (incl. language); theme via colorMode localStorage. Preferences with an admin default in `settings.json` (dotfiles, language) follow "user override wins, otherwise admin default from systemVars" — the user value stays `null` until explicitly changed.
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
| `GFTP_LOG_LEVEL` / `GFTP_LOG_FORMAT` | Log level (`info`) and format (`json`\|`text`) |
| `GFTP_LOG_FILE` (+ `GFTP_LOG_FILE_MAX_SIZE_MB` / `_MAX_BACKUPS` / `_MAX_AGE_DAYS`) | Optional rotating file sink in addition to stdout |
| `GFTP_LOG_FRONTEND` | Browser-error forwarding endpoint (default on; `false` disables) |
| `GFTP_METRICS_ENABLED` / `GFTP_METRICS_PORT` | Prometheus /metrics on a dedicated port (default off / `9091`) |
| `GFTP_S3_ENABLED` + `GFTP_S3_ENDPOINT` / `GFTP_S3_BUCKET` / `GFTP_S3_ACCESS_KEY` / `GFTP_S3_SECRET_KEY` (+ optional `GFTP_S3_REGION`, `GFTP_S3_USE_PATH_STYLE`, `GFTP_S3_PREFIX`, `GFTP_S3_TIMEOUT_SECS`) | Optional S3 chunk staging — env-only, never in `settings.json` |

The FTP test container and MinIO are on docker compose profile `testing` — only `just ftp-up/ftp-down` and `just s3-up/s3-down` activate them.

## Release

Push a `v*` tag (`git tag v0.2.0 && git push --tags`) → `.github/workflows/release.yml` runs the shared gates (`checks.yml`, also used by ci.yml), publishes a multi-arch (amd64+arm64) image to `ghcr.io/darthsoup/goblinftp` with semver tags + `latest`, and creates a GitHub Release with commit-grouped notes (feat/fix/chore — commits land directly on main, so PR-based auto-notes alone would be empty). The tag is injected via the `VERSION` build-arg → `main.version` → startup log, `/healthz`, `/api/system/vars` (settings-modal footer), and the default `GFTP_SENTRY_RELEASE`. `latest` tracks releases, not `main`; branch builds report version `dev`. Prerelease tags (`v1.0.0-rc1`) skip `latest` and are marked prerelease.
