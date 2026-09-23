#!/usr/bin/env bash

set -euo pipefail

CHENWEB_DIR=/home/gui/Workspace/ChenWeb
DEPLOY_DIR="$CHENWEB_DIR/chenweb-deploy"
STAGING_DIR="$CHENWEB_DIR/deploy_staging"

if [ "$#" -gt 0 ]; then
  archive=$1
else
  archive=$(find "$STAGING_DIR" -maxdepth 1 -type f -name 'runshen-*.tar.gz' -print0 \
    | xargs -0 ls -t \
    | head -n 1)
fi

[ -n "$archive" ] || { echo "No runshen archive found in $STAGING_DIR" >&2; exit 1; }
[ -f "$archive" ] || { echo "Archive not found: $archive" >&2; exit 1; }

rm -rf "$DEPLOY_DIR"
mkdir -p "$CHENWEB_DIR"
tar -xzf "$archive" -C "$CHENWEB_DIR" --strip-components=1
bash "$DEPLOY_DIR/deploy-server-china.sh" "$DEPLOY_DIR"
