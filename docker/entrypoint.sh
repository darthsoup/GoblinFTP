#!/bin/sh
set -e

cleanup() {
    if [ -n "${caddy_pid:-}" ] && kill -0 "$caddy_pid" 2>/dev/null; then
        kill "$caddy_pid" 2>/dev/null || true
    fi
    if [ -n "${backend_pid:-}" ] && kill -0 "$backend_pid" 2>/dev/null; then
        kill "$backend_pid" 2>/dev/null || true
    fi
    if [ -n "${caddy_pid:-}" ]; then
        wait "$caddy_pid" 2>/dev/null || true
    fi
    if [ -n "${backend_pid:-}" ]; then
        wait "$backend_pid" 2>/dev/null || true
    fi
}

shutdown() {
    trap - INT TERM EXIT
    cleanup
    exit 0
}
trap shutdown INT TERM
trap cleanup EXIT

# The backend is reachable only through the local Caddy process.
if [ -n "${GFTP_PORT:-}" ] && [ "${GFTP_PORT}" != "8081" ]; then
    echo "GFTP_PORT is fixed to 8081 inside this container; ignoring ${GFTP_PORT}" >&2
fi
export GFTP_PORT=8081
export GFTP_BIND_ADDRESS=127.0.0.1

# Caddy validates external hops; Go also trusts the loopback hop from Caddy.
GFTP_CADDY_TRUSTED_PROXIES="$(printf '%s' "${GFTP_ACCESS_TRUSTED_PROXIES:-}" | tr ',' ' ')"
export GFTP_CADDY_TRUSTED_PROXIES
GFTP_ACCESS_TRUSTED_PROXIES="${GFTP_ACCESS_TRUSTED_PROXIES:+${GFTP_ACCESS_TRUSTED_PROXIES},}127.0.0.1/32"
export GFTP_ACCESS_TRUSTED_PROXIES

/app/gftp &
backend_pid=$!

healthy=false
deadline=$(($(date +%s) + 15))
while [ "$(date +%s)" -lt "$deadline" ]; do
    if wget -T 1 -q -O /dev/null http://127.0.0.1:8081/healthz 2>/dev/null; then
        healthy=true
        break
    fi
    if ! kill -0 "$backend_pid" 2>/dev/null; then
        echo "GoblinFTP backend exited before becoming healthy" >&2
        exit 1
    fi
    sleep 0.5
done

if [ "$healthy" != "true" ]; then
    echo "GoblinFTP backend did not become healthy within 15 seconds" >&2
    exit 1
fi

caddy run --config /etc/caddy/Caddyfile --adapter caddyfile &
caddy_pid=$!

while kill -0 "$backend_pid" 2>/dev/null && kill -0 "$caddy_pid" 2>/dev/null; do
    sleep 1
done

if ! kill -0 "$backend_pid" 2>/dev/null; then
    echo "GoblinFTP backend exited unexpectedly" >&2
    exit 1
fi

if ! kill -0 "$caddy_pid" 2>/dev/null; then
    echo "Caddy exited unexpectedly" >&2
    exit 1
fi
