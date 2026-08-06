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

# The in-container backend port is fixed: Caddy's reverse_proxy targets and the
# health polls all assume 8080. The host-facing knob is the port mapping to 80.
if [ -n "${GFTP_PORT:-}" ] && [ "${GFTP_PORT}" != "8080" ]; then
    echo "GFTP_PORT is fixed to 8080 inside this container (Caddy proxies it); ignoring ${GFTP_PORT}" >&2
fi
export GFTP_PORT=8080

# Start Go backend in background
/app/gftp &
backend_pid=$!

# Wait for backend to be ready (up to 5 seconds)
healthy=false
for i in $(seq 1 10); do
    if wget -q -O /dev/null http://localhost:8080/healthz 2>/dev/null; then
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
    echo "GoblinFTP backend did not become healthy within 5 seconds" >&2
    exit 1
fi

# Start Caddy once the backend is healthy
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
