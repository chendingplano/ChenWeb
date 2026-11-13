#!/usr/bin/env bash
set -e
SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
. "$SCRIPT_DIR/scripts/system-monitor-env.sh"
mkdir -p "$(dirname "$CADDY_LOG_FILE")"
exec caddy run --config Caddyfile > >(tee -a "$CADDY_LOG_FILE") 2>&1
