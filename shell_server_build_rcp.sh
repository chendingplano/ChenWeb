#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
cd "$SCRIPT_DIR"

mise run build-server-linux

timestamp=$(date +%Y%m%d-%H%M)
archive="/tmp/runshen-depoly-${timestamp}.tar.gz"
staging_dir=/home/gui/Workspace/ChenWeb/deploy_staging

tar -czvf "$archive" /tmp/chenweb-deploy
ssh -p 8822 gui@210.5.158.91 "rm -rf '$staging_dir' && mkdir -p '$staging_dir'"
scp -P 8822 "$archive" "gui@210.5.158.91:$staging_dir/"
rm -f "$archive"
