#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
cd "$SCRIPT_DIR"

if [ "$#" -gt 4 ]; then
  echo "usage: $0 [build_name [target_machine [os_info [description]]]" >&2
  exit 2
fi

export BUILD_NAME="${1:-chenweb}"
export TARGET_MACHINE="${2:-unknown}"
export OS_INFO="${3:-linux/amd64}"
export BUILD_DESCRIPTION="${4:-}"

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
