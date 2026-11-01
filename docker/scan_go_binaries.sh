#!/bin/sh
set -eu
image=${1:-goblinftp:smoke}
scan_dir=$(mktemp -d)
container_id=$(docker create "$image")
cleanup() {
    docker rm -v "$container_id" >/dev/null || true
    rm -rf "$scan_dir"
}
trap cleanup EXIT INT TERM
for executable in /app/gftp /usr/bin/caddy; do
    name=$(basename "$executable")
    docker cp "$container_id:$executable" "$scan_dir/$name"
    go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 -mode=binary "$scan_dir/$name"
done
