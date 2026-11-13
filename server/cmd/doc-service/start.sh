#!/bin/sh
set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
cd "$SCRIPT_DIR"

. ../../../scripts/system-monitor-env.sh
export LOG_FILE_DIR="$DOC_SERVICE_LOG_DIR"
mise run build-doc-service
exec ../../../.cache/doc-service.exe
