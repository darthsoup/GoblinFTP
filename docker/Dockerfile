# syntax=docker/dockerfile:1

# Runs on the build platform: the generated SPA is static and arch-independent.
FROM --platform=$BUILDPLATFORM node:24-alpine@sha256:44fce696f38ba8c987c4db89954051000bbd05ffd8d8659ba15ed4a8602e937f AS frontend-builder
WORKDIR /build
RUN corepack enable
# Workspace root + the frontend manifest first (layer-cached install)
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY frontend/package.json frontend/
RUN pnpm install --frozen-lockfile
COPY frontend/ frontend/
# NUXT_PUBLIC_* are baked into the static SPA here, unlike GFTP_* which the
# backend reads at process start.
ARG NUXT_PUBLIC_SENTRY_DSN=
ARG NUXT_PUBLIC_SENTRY_ENVIRONMENT=
ARG NUXT_PUBLIC_SENTRY_RELEASE=
ARG VERSION=dev
RUN pnpm --filter goblinftp-frontend run generate
# Client source maps are emitted hidden (nuxt.config.ts) for upload to Sentry,
# never for shipping. Drop the upload step in before this line to keep them.
RUN find frontend/.output/public -name '*.map' -delete

# Runs on the build platform and cross-compiles for the target, so no emulation.
FROM --platform=$BUILDPLATFORM golang:1.26-alpine@sha256:3082400e369fa24d5fc60bca20edab3f6d604e0c5a690ec66b295eff4dd87ade AS backend-builder
WORKDIR /build/backend
ARG TARGETOS
ARG TARGETARCH
# VERSION is the release tag (e.g. v0.2.0); "dev" outside release builds.
ARG VERSION=dev
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-w -X main.version=${VERSION}" -o /app/gftp ./cmd/gftp

FROM --platform=$BUILDPLATFORM golang:1.26-alpine@sha256:3082400e369fa24d5fc60bca20edab3f6d604e0c5a690ec66b295eff4dd87ade AS caddy-builder
WORKDIR /build/caddy
ARG TARGETOS
ARG TARGETARCH
COPY docker/caddy/go.mod docker/caddy/go.sum ./
RUN go mod download
COPY docker/caddy/main.go ./
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-w" -o /out/caddy .

FROM caddy:2-alpine@sha256:d8542f48d34a9cf4e4c11a478865229840e87e4c96ea3f439101f31a5d35f75f

RUN addgroup -g 10001 goblinftp && adduser -D -H -u 10001 -G goblinftp goblinftp \
    && mkdir -p /app/data && chown 10001:10001 /app/data \
    && setcap -r /usr/bin/caddy

ENV XDG_CONFIG_HOME=/tmp/caddy/config XDG_DATA_HOME=/tmp/caddy/data

COPY --from=frontend-builder /build/frontend/.output/public /app/public
COPY --from=backend-builder /app/gftp /app/gftp
COPY --from=caddy-builder /out/caddy /usr/bin/caddy
COPY docker/Caddyfile /etc/caddy/Caddyfile
COPY --chmod=0755 docker/entrypoint.sh /entrypoint.sh

USER 10001:10001

EXPOSE 8080

# Polls through Caddy so the check covers the full serving chain, not just the
# Go process (the entrypoint already delays Caddy until the backend is up).
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -T 2 -q -O /dev/null http://127.0.0.1:8080/healthz || exit 1

ENTRYPOINT ["/entrypoint.sh"]
