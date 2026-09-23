#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
cd "$SCRIPT_DIR"

mise run build-server-linux

timestamp=$(date +%Y%m%d-%H%M)
archive="/tmp/runshen-depoly-${timestamp}.tar.gz"

# Direct scp to the China box (210.5.158.91) can be extremely slow. Instead,
# push to the Mac Mini (192.168.29.96), which the China box then pulls from
# over its own, faster path -- see shell_server_deploy.sh.
RELAY_HOST=192.168.29.96
RELAY_PORT=8822
RELAY_USER=cding
RELAY_DIR=/home/cding/Backups

tar -czvf "$archive" /tmp/chenweb-deploy
scp -P "$RELAY_PORT" "$archive" "$RELAY_USER@$RELAY_HOST:$RELAY_DIR/"
rm -f "$archive"
