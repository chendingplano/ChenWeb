#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ADDON_PATH="${ROOT_DIR}/tools/mitmproxy/llm_trace_addon.py"
LISTEN_HOST="${MITMTRACE_LISTEN_HOST:-127.0.0.1}"
LISTEN_PORT="${MITMTRACE_LISTEN_PORT:-8081}"

if ! command -v mitmdump >/dev/null 2>&1; then
  echo "mitmdump not found. Install mitmproxy first: brew install --cask mitmproxy" >&2
  exit 1
fi

if [[ -z "${MITM_TRACE_INGEST_TOKEN:-}" ]]; then
  echo "MITM_TRACE_INGEST_TOKEN is required." >&2
  exit 1
fi

if [[ ! -f "${ADDON_PATH}" ]]; then
  echo "addon script not found: ${ADDON_PATH}" >&2
  exit 1
fi

echo "Starting mitmproxy LLM trace capture on ${LISTEN_HOST}:${LISTEN_PORT}"
echo "Forwarding to ${CHENWEB_MITM_INGEST_URL:-http://127.0.0.1:8080/api/internal/mitmproxy/ingest}"

exec mitmdump \
  --listen-host "${LISTEN_HOST}" \
  --listen-port "${LISTEN_PORT}" \
  -s "${ADDON_PATH}"
