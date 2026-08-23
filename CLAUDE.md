# CLAUDE.md

Guidance for Claude Code (claude.ai/code) when working in this repository.

GoblinFTP is a self-hosted, web-based FTP/FTPS/SFTP client. Go + Echo v4 backend, Nuxt 4 SPA frontend, shipped as a single Docker container (Caddy serves the SPA, the Go binary serves `/api`). It is stateless by design: no database, no user store, credentials live only in an in-memory session.

## Commands

Everything runs through [just](https://just.systems); `.env` is auto-loaded. The frontend is a pnpm workspace rooted at the repo root (one `pnpm-lock.yaml` + `pnpm-workspace.yaml` there): run `pnpm install` at the root. Scripts work both via `cd frontend && pnpm …` and `pnpm --filter goblinftp-frontend …`.

```bash
just dev          # frontend :3000 + backend :8080 with hot reload (dev-fe / dev-be for one half)

just test         # test-fe + test-be
just test-be      # go test ./...
just test-fe      # vitest run
cd backend && go test ./internal/api/...                    # one Go package
cd backend && go test -run TestConnectSuccess ./internal/api/...
cd frontend && pnpm test:watch                              # vitest watch

just lint         # eslint + nuxt typecheck + golangci-lint
just fmt          # eslint --fix + golangci-lint fmt
just i18n-check   # locale key + placeholder parity against en.json
just confgen      # regenerate .env.example and the docs config tables

just ftp-up / ftps-up / sftp-up / s3-up   # local test servers, matching *-down stops them
just sso-link     # one-time SSO login link (generators in examples/sso/)
just build        # nuxt generate + go build → bin/gftp
```

`just --list` covers the rest (docker stacks, `analyze-fe`, `clean`). Setup and integration-test walkthroughs live in `docs/development.md`.

Running the app for a visual check: prefer `just dev`, but first look whether the user already has :3000 / :8080 running and start only the missing half. Never kill their processes. For a backend that needs special env, run `cd backend && GFTP_DATA_DIR=data GFTP_PORT=… go run ./cmd/gftp` on a free port. Without a writable `GFTP_DATA_DIR`, SFTP connects fail trying to create `/app/data`.

## Layout

```
backend/
  cmd/gftp/main.go        # entry: config load → newApp() → e.Start()
  cmd/gftp-sso-link/      # CLI: generate one-time SSO login links (reuses internal/sso)
  cmd/gftp-confgen/       # CLI: regenerate .env.example + doc config tables from the registry
  internal/
    api/                  # all HTTP handlers, middleware, routing
    auth/                 # in-memory session store (TTL), CSRF tokens, login throttle
    config/               # env config: key registry, loader, artifact generator (gen/)
    errors/               # GFTPError with machine-readable codes
    ftp/                  # jlaffaye/ftp adapter   → implements transfer.Client
    logging/              # slog Init (stdout + optional lumberjack file sink), SafeLogAttrs redaction
    metrics/              # Prometheus registry, collectors, CountingReader (opt-in /metrics listener)
    sftp/                 # pkg/sftp adapter       → implements transfer.Client
    sentry/               # custom Echo v4 Sentry middleware (sentry-go/echo is v5-only)
    sso/                  # SSO token validation + one-time-use set
    staging/              # ChunkStore interface: local disk (default) or S3 chunk staging
    transfer/             # Client interface, chunked upload engine, download tokens, testutil.MockClient
frontend/app/
  pages/                  # login.vue (boot + SSO landing), index.vue (workspace), edit.vue (editor)
  middleware/auth.global.ts  # redirects on authStore.connected
  layouts/default.vue     # app chrome plus every overlay/modal mount point
  components/             # Auth, Editor, FileBrowser, Layout, Modals, Upload (pathPrefix: false)
  stores/                 # auth, files, editor, upload, modal, settings (Composition API style)
  composables/useApi.ts   # wraps $fetch, injects CSRF, unwraps the API envelope
  utils/, types/api.ts    # pure helpers; TS interfaces mirroring backend JSON
  tests/                  # vitest specs for stores, utils, components
```

### Request lifecycle

1. Browser → (dev: Vite proxy `/api/*`, prod: Caddy) → Go Echo backend.
2. Every response uses the `Response` envelope: `{ success, data?, errors?: [{code, message}] }`.
3. CSRF: the backend returns a token in `data.csrfToken` on connect; the frontend sends it as `X-CSRF-Token` on every mutating request via `useApi`.
4. Session lives in the `gftp_session` cookie (HTTP-only); the `transfer.Client` sits in `session.Data["client"]`.
5. FTP and SFTP share all handler code through the `transfer.Client` interface.

## Code style

Two rules matter more than the rest. Both apply to backend and frontend alike.

### 1. Comments: few, short, and only where they earn it

Keep comments to a minimum. Prefer self-explanatory names and structure. Comment only what is genuinely relevant and not visible in the code itself: a tricky invariant, a workaround, a gotcha, the reason behind a surprising choice.

- **One to two lines per comment, hard cap.** If an explanation needs more, either the code needs restructuring or the text belongs in `docs/`.
- **No header banners, no restating what the code does, no step-by-step narration** of an obvious flow.
- Deleting a comment should lose information. Otherwise it should not exist.
- Write the "why", never the "what". `// increment the counter` above `count++` is noise.
- The same applies to comments in generated examples, tests, and config files.

### 2. Never use em dashes or en dashes as punctuation

The characters `—` (em dash, U+2014) and `–` (en dash, U+2013) must not appear as punctuation anywhere in this repo: code comments, documentation, README, i18n strings, commit messages, PR descriptions.

Rephrase with a period, comma, colon, semicolon, or parentheses instead. A plain hyphen in a compound word (`right-to-left`) or a CLI flag (`--fix`) is fine; what is banned is the dash used to join clauses.

## Backend conventions

- **Never** write raw `c.JSON`. Always `OK(c, data)` or `Fail(c, gftperrors.New(gftperrors.ErrX, "msg"))` from `api/response.go`. Codes map to HTTP status via `GFTPError.HTTPStatus()`.
- **Error codes** come from constants in `internal/errors/errors.go`. Never invent string literals.
- Raw FTP/SFTP errors never reach the client: `errclass.go:classify()` maps them to a stable code plus a safe message, and the original is attached via `WithCause` for logs only.
- `transfer.Client` is read from the session with `clientFromContext(c)` → `(Client, bool)`.
- Chunk staging sits behind `staging.ChunkStore` (local disk default, S3 via `GFTP_S3_ENABLED`). The aws-sdk-go-v2 dependency lives only in `internal/staging`.
- **API tests** are black-box (`package api_test`): `newTestApp(t, defaultTestConfig(), opts...)` plus `transfer/testutil.MockClient`. Handler options inject test doubles: `WithDial`, `WithChunkStore`, `WithLogger`, `WithMetrics`, `WithVersion`.
- `internal/ftp` and `internal/sftp` are integration-level. Real-server tests are gated by `GFTP_TEST_FTP_HOST` / `GFTP_TEST_FTPS_HOST` / `GFTP_TEST_SFTP_HOST`, S3 tests in `internal/staging` by `GFTP_TEST_S3_ENDPOINT`. Start the servers with the matching `just *-up` recipe.
- `internal/sentry` is intentionally not unit-tested.
- **Lint**: golangci-lint v2, config in `backend/.golangci.yml`, version pinned in `.github/workflows/checks.yml` (keep the local `brew install golangci-lint` in sync). `nolint` directives must be specific and carry a reason (`//nolint:gosec // G101: …`); nolintlint enforces it.
- **Logging** (details in `docs/logging.md`): one structured access line per request from `requestLogger` (`api/middleware_logging.go`). `Fail()` stashes the GFTPError in the echo context (`LoggedErrorKey`) for that line, because handlers return nil after Fail. Never log passwords, tokens, or full session IDs (8-char prefix only; dynamic attrs go through `logging.SafeLogAttrs`). Tests assert output via `newTestAppWithLog(t, cfg, &buf)` + `logLines`. Streaming endpoints log the committed status, so a mid-stream failure still shows 200.
- **Metrics** (details in `docs/metrics.md`): `internal/metrics` owns the registry; the instance hangs off `Handler`. The `/metrics` listener is separate and only served when `GFTP_METRICS_ENABLED=true`, never on the main echo. Gauges are scrape-time snapshots of `auth.Store` via a custom collector, so there is no inc/dec drift; counters increment at call sites (`connect.go`, `sso.go`, `download.go`, `archive.go`, `upload.go`, `frontendlog.go`). Chunk staging writes are not counted, only bytes to and from the FTP/SFTP server. `metricsMiddleware` sits between `RequestID` and `requestLogger` and must not call `c.Error` (the logger owns that); it labels by the `c.Path()` route template and skips `/healthz`.

## Frontend conventions

- **Design system**: dual theme (dark and light) switched by colorMode, default Automatic. Nuxt UI tokens are overridden in `app/assets/css/main.css` (light on `:root, .light`, dark on `.dark`); `primary` aliases the custom `goblin` green scale, `neutral` is a charcoal navy. Style with token utilities (`bg-default/muted/elevated/accented`, `text-muted/dimmed/highlighted`, `border-default/accented`), never `gray-*` or `dark:` variants, so components stay mode-agnostic. Values tokens cannot express live in `--gftp-*` custom properties (input, popover, editor-bg, scrollbar, selection, primary-text). `label-caps` is a custom utility for column headers and field labels.
- **Component sizing** is centralized in `app/app.config.ts`: `lg` is the default size for buttons and every input, meaning 48px tall controls with 14px text. Reach for `size="sm"` only on dense surfaces such as the file toolbar. Change the shared look there, not per component.
- Fonts are self-hosted via `@fontsource-variable`: Plus Jakarta Sans (`font-sans`) is the default for everything, UI chrome and data alike (paths, sizes, dates, permissions, breadcrumbs, status bar). JetBrains Mono is reserved for code: the CodeMirror editor and the `<pre>` preview in `FilePreviewPanel.vue`. Do not add `font-mono` to data or UI surfaces.
- Icons: `i-lucide-*`, plus `i-simple-icons-*` for file types in `FileRow.vue`. Both sets are installed locally. Do not use heroicons.
- All API calls go through `useApi()`. The only intentional exceptions are `authStore.init()` / `connect()` / the status ping, which hit public, CSRF-exempt endpoints.
- **VueUse** is available via `@vueuse/nuxt` (auto-imported `use*` composables). Prefer it over hand-rolled listeners, timers, and media queries. The module is listed first in `nuxt.config.ts` so `@nuxt/ui`'s `useColorMode` wins the name collision. Do not reorder it.
- **White-label branding**: `useBranding()` exposes `appName` / `logoUrl` / `hideAttribution` from `systemVars.branding`. `plugins/auth.client.ts` injects the runtime accent (`utils/branding.ts` `applyBrandColor()`), the `--gftp-primary-text` button-text token, the document title, and the favicon; both color injections are skipped when a tenant `themeCssUrl` is present, since that theme's `config.css` owns those tokens. `logoUrl` is colorMode-aware and returns `logoDarkUrl` in dark mode when set. New brand surfaces must read `useBranding()` and never hardcode "GoblinFTP". Per-tenant themes are documented in `docs/theming.md`.
- Pinia stores use **Composition API style**: `defineStore('id', () => { … })`.
- End-user preferences are browser-side only, the backend never reads them: `stores/settings.ts` persists to localStorage `gftp_settings`, theme via colorMode. Preferences with an admin default (dotfiles, language) follow "user override wins, otherwise the systemVars default", so the user value stays `null` until explicitly changed.
- `FileInfo` JSON fields are `name`, `size`, `isDir`, `modified` (RFC3339), `mode` (`"drwxr-xr-x"`). The backend's internal `transfer.FileInfo` uses different field names.
- TypeScript strict mode including `noUncheckedIndexedAccess`: index access is `T | undefined`, so use `!` after a length guard or optional chaining.
- `UProgress` takes `:model-value`, not `:value`.
- i18n: `en.json` is the source of truth and `just i18n-check` enforces recursive key parity plus identical `{…}` placeholders (gated in CI). See `docs/i18n.md` before adding or renaming a locale.

### Adding a new modal

1. Add the type to `ModalType` in `stores/modal.ts`.
2. Create `components/Modals/YourModal.vue` with `<UModal :open="modalStore.active === 'yourType'" @update:open="modalStore.close()">`.
3. Mount `<YourModal />` in `layouts/default.vue` alongside the other overlays, not in `pages/index.vue`. A modal mounted on the page never renders on `/edit`.
4. Add the keys to `i18n/locales/en.json`, then to every other locale file so `just i18n-check` stays green. `de.json` is the reference for tone.

### Adding a config key

Define it once in `backend/internal/config/registry.go`. The loader, `.env.example`, and the docs tables are all generated from that registry. Run `just confgen` afterwards; backend tests fail on drift.

## Configuration

Backend config is env-only and loaded once at startup. `backend/internal/config` is the source of truth for every key, default, and validation rule; the operator-facing reference is `docs/configuration.md`. Do not enumerate config keys here, they drift. There is no `settings.json`; a mounted one fails startup with a migration hint.

Gotchas worth knowing beyond the reference:

- `GFTP_DATA_DIR` (default `/app/data`, a container path) is the writable dir for SFTP `known_hosts`, local chunk staging, and themes. `just dev-be` defaults it to `data` and resolves relative values against the repo root, so the dev data dir is `<repo>/data`.
- `GFTP_FRAME_ANCESTORS` is read by Go, by Caddy (which serves the framed `index.html`), and by `nuxt.config.ts` route rules for dev parity. Setting it flips the session cookie to `SameSite=None; Secure; Partitioned` instance-wide (see `api/cookie.go`).
- `GFTP_ACCESS_TRUSTED_PROXIES` decides whether forwarded headers are trusted at all (`api/proxy.go`). Unset means the direct peer is the client, which is what throttling and logging then key on.
- `docker-compose.yml` passes `.env` into the container via `env_file`, so any `GFTP_*` var set there is live configuration under compose.
- `NUXT_PUBLIC_SENTRY_DSN` is the one build-time variable, baked into the static SPA at `nuxt generate`. All `GFTP_*` vars are read at process start.

The FTP/FTPS/SFTP test containers and MinIO sit on the docker compose profile `testing`, activated only by the `just ftp-up` / `ftps-up` / `sftp-up` / `s3-up` recipes. MinIO's root credentials are `MINIO_ROOT_USER` / `MINIO_ROOT_PASSWORD` / `MINIO_TEST_BUCKET`, deliberately not the app-side `GFTP_S3_*` names.

## Release

Push a `v*` tag (`git tag v0.2.0 && git push --tags`). `.github/workflows/release.yml` runs the shared gates in `checks.yml` (also used by `ci.yml`), publishes a multi-arch (amd64 + arm64) image to `ghcr.io/darthsoup/goblinftp` with semver tags plus `latest`, and creates a GitHub Release with commit-grouped notes (feat/fix/chore, since commits land directly on main and PR-based auto-notes would be empty). The tag flows through the `VERSION` build-arg into `main.version`, and from there into the startup log, `/healthz`, `/api/system/vars` (settings-modal footer), and the default `GFTP_SENTRY_RELEASE`. `latest` tracks releases, not `main`; branch builds report version `dev`. Prerelease tags such as `v1.0.0-rc1` skip `latest` and are marked prerelease.

## Docs map

`docs/configuration.md` (every env key), `development.md` (setup, integration tests), `installation.md`, `system-requirements.md`, `embedding.md` (iframe), `theming.md` (branding and tenant themes), `i18n.md`, `logging.md`, `metrics.md`, `sentry.md` (error tracking), `s3-staging.md`.
