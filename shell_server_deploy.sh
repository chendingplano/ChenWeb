#!/usr/bin/env bash

set -euo pipefail

# Set PROJECT_WORKSPACE_DIR in the root environment to the existing target workspace.
# Packaged Document Builder members are copied there without deleting other files.

# Root is required to restart pdf-parser. Check before anything is changed.
if [ "$(id -u)" != 0 ]; then
  echo "!! shell_server_deploy.sh must be run as root (for example: sudo -i, then run it)." >&2
  exit 1
fi

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

# Validate before replacing the current deployment.
: "${PROJECT_WORKSPACE_DIR:?PROJECT_WORKSPACE_DIR must name an existing workspace directory}"
[ -d "$PROJECT_WORKSPACE_DIR" ] || { echo "!! PROJECT_WORKSPACE_DIR is not a directory: $PROJECT_WORKSPACE_DIR" >&2; exit 1; }

rm -rf "$DEPLOY_DIR"
mkdir -p "$CHENWEB_DIR"
tar -xzf "$archive" -C "$CHENWEB_DIR" --strip-components=1
# Preserve the workspace-relative member paths used by Document Builder.
if [ -d "$DEPLOY_DIR/document-workspace" ]; then
  rsync -a --chown=gui:gui "$DEPLOY_DIR/document-workspace/" "$PROJECT_WORKSPACE_DIR/"
fi
bash "$DEPLOY_DIR/deploy-server-china.sh" "$DEPLOY_DIR"

# --- pdf-parser -------------------------------------------------------------
# deploy-server-china.sh installs only the Go binaries. The PDF parser runs as the
# chenweb-pdf-parser image, so rebuild it from the sources in this archive, using
# the steps in python/pdf-parser/container/README.md.
WORKSPACE_DIR=/home/gui/Workspace
PDF_SRC="$DEPLOY_DIR/pdf-parser"
PDF_DIR="$CHENWEB_DIR/python/pdf-parser"
PDF_BUILD_DIR="$WORKSPACE_DIR/deploy/mineru-4.0.9-build"
PDF_IMAGE=chenweb-pdf-parser:mineru-4.0.9

if [ -d "$PDF_SRC" ]; then
  echo "== pdf-parser =="
  # The image snapshots this tree, so mirror the archive exactly: stale modules are removed.
  rsync -a --delete --chown=gui:gui --exclude .venv --exclude __pycache__ --exclude .pytest_cache --exclude tests \
    "$PDF_SRC"/ "$PDF_DIR"/
  rm -rf "$PDF_BUILD_DIR"
  python3 "$PDF_DIR/container/prepare-context.py" \
    --worker "$PDF_DIR" --adapters "$PDF_DIR/container/mineru-v4" \
    --mineru "$WORKSPACE_DIR/ThirdParty/mineru" --output "$PDF_BUILD_DIR"
  docker build --network=host -t "$PDF_IMAGE" "$PDF_BUILD_DIR"
  # Refuse to continue if the image lacks the table geometry code (the bug this section fixes).
  docker run --rm --entrypoint sh "$PDF_IMAGE" -c \
    'test -f /opt/pdf-parser/table_geometry.py && test -f /opt/pdf-parser/stacked_cells.py &&
     test -f /opt/pdf-parser/mineru_cell_repair.py && grep -q write_table_geometry /opt/pdf-parser/pdf_parser.py &&
     grep -q "use_layout=False" /opt/pdf-parser/table_geometry.py &&
     grep -q "use_layout=False" /opt/pdf-parser/mineru_cell_repair.py' \
    || { echo "!! $PDF_IMAGE is missing the table geometry code" >&2; exit 1; }
  echo "  image built and verified: $PDF_IMAGE"
  # Restarting the unit interrupts any parse in progress.
  systemctl restart pdf-parser
  # The deploy fails unless the running container is the image just built and has the geometry code.
  want=$(docker image inspect -f '{{.Id}}' "$PDF_IMAGE")
  ok=no
  for _ in $(seq 1 60); do
    if [ "$(docker inspect -f '{{.Image}}' chenweb-pdf-parser 2>/dev/null || true)" = "$want" ] &&
       systemctl is-active --quiet pdf-parser; then
      ok=yes
      break
    fi
    sleep 2
  done
  if [ "$ok" != yes ]; then
    echo "!! pdf-parser is not running $PDF_IMAGE" >&2
    journalctl -u pdf-parser -n 30 --no-pager >&2 || true
    exit 1
  fi
  docker exec chenweb-pdf-parser test -f /opt/pdf-parser/table_geometry.py ||
    { echo "!! running pdf-parser lacks table_geometry.py" >&2; exit 1; }
  echo "  pdf-parser running $PDF_IMAGE with table geometry: OK"
else
  echo "!! archive has no pdf-parser sources; the parser was NOT rebuilt (build it with the updated shell_server_build_rcp.sh)" >&2
fi
