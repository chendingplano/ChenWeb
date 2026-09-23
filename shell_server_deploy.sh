#!/usr/bin/env bash

set -euo pipefail

CHENWEB_DIR=/home/gui/Workspace/ChenWeb
DEPLOY_DIR="$CHENWEB_DIR/chenweb-deploy"
STAGING_DIR="$CHENWEB_DIR/deploy_staging"

# The Mac pushes the archive to the Mac Mini's LAN address (192.168.29.96)
# instead of scp'ing directly here, since the direct Mac->China link can be
# extremely slow. Pull from there instead, over the Mac Mini's port forwarded
# to the public internet -- 47.189.245.217:8896 is the same machine as
# 192.168.29.96:8822 (verified: identical host keys), just reached from the
# China side instead of the Mac's LAN side.
RELAY_HOST=47.189.245.217
RELAY_PORT=8896
RELAY_USER=cding
RELAY_DIR=/home/cding/Backups

if [ "$#" -gt 0 ]; then
  archive=$1
else
  rm -rf "$STAGING_DIR"
  mkdir -p "$STAGING_DIR"
  # cding's login shell on the relay is tcsh, and plain `ssh host "cmd"` runs
  # "cmd" under that login shell, not bash -- tcsh doesn't understand bash's
  # `2>/dev/null` redirect syntax and fails with "Ambiguous output redirect."
  # Force bash explicitly for the remote command instead.
  latest=$(ssh -p "$RELAY_PORT" "$RELAY_USER@$RELAY_HOST" \
    bash -c "'ls -t $RELAY_DIR/runshen-*.tar.gz 2>/dev/null | head -n 1'")
  [ -n "$latest" ] || { echo "No runshen archive found on $RELAY_USER@$RELAY_HOST:$RELAY_DIR" >&2; exit 1; }
  echo "Pulling $latest from $RELAY_USER@$RELAY_HOST:$RELAY_PORT ..."
  scp -P "$RELAY_PORT" "$RELAY_USER@$RELAY_HOST:$latest" "$STAGING_DIR/"
  archive="$STAGING_DIR/$(basename "$latest")"
fi

[ -n "$archive" ] || { echo "No runshen archive found in $STAGING_DIR" >&2; exit 1; }
[ -f "$archive" ] || { echo "Archive not found: $archive" >&2; exit 1; }

rm -rf "$DEPLOY_DIR"
mkdir -p "$CHENWEB_DIR"
tar -xzf "$archive" -C "$CHENWEB_DIR" --strip-components=1
bash "$DEPLOY_DIR/deploy-server-china.sh" "$DEPLOY_DIR"
