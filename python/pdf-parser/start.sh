#!/usr/bin/env bash
set -e
SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
. "$SCRIPT_DIR/../../scripts/system-monitor-env.sh"
mkdir -p "$(dirname "$PDF_PARSER_LOG_FILE")"
export PYTHONUNBUFFERED=1
PYTHONPATH=. exec mise ocr-service-start-sync > >(tee -a "$PDF_PARSER_LOG_FILE") 2>&1
