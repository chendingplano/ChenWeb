#!/usr/bin/env bash

set -euo pipefail

# Set PROJECT_WORKSPACE_DIR in the environment (ChenWeb/.env on the box) to the existing
# target workspace. Packaged Document Builder files are copied there without deleting other files.
#
#   shell_server_deploy.sh              deploy the newest runshen-*.tar.gz on the relay
#   shell_server_deploy.sh <archive>    deploy a local archive
#   shell_server_deploy.sh --newest     print "<name> <size> <mtime>" of the newest relay archive
#   RELAY_ARCHIVE=<name> shell_server_deploy.sh   deploy that relay archive (the Deployment page)
#
# Runs as gui (the Deployment page runs it from chenweb-deploy.service) or as root. As gui,
# systemctl goes through the passwordless sudoers rule /etc/sudoers.d/gui-chenweb-services,
# which must list pdf-parser too, and docker works through gui's docker group.

# One marker line per step; the Deployment page (deploy-runner) turns them into step rows.
step() { if [ $# -gt 1 ]; then printf '::step::%s::%s\n' "$1" "$2"; else printf '::step::%s\n' "$1"; fi; }
die() { echo "!! $*" >&2; exit 1; }

IS_ROOT=0
[ "$(id -u)" = 0 ] && IS_ROOT=1
systemctl_cmd() { if [ "$IS_ROOT" = 1 ]; then systemctl "$@"; else sudo -n /bin/systemctl "$@"; fi; }

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

# cding's login shell on the relay is tcsh, and plain `ssh host "cmd"` runs
# "cmd" under that login shell, not bash -- tcsh doesn't understand bash's
# `2>/dev/null` redirect syntax and fails with "Ambiguous output redirect."
# Force bash explicitly for the remote command instead, and keep `$` out of it
# (tcsh would expand it before bash sees it).
if [ "${1:-}" = --newest ]; then
  line=$(ssh -o BatchMode=yes -o ConnectTimeout=20 -p "$RELAY_PORT" "$RELAY_USER@$RELAY_HOST" \
    bash -c "'ls -lt --time-style=+%s $RELAY_DIR/runshen-*.tar.gz 2>/dev/null | head -n 1'")
  [ -n "$line" ] || die "No runshen archive found on $RELAY_USER@$RELAY_HOST:$RELAY_DIR"
  read -r _ _ _ _ size mtime path <<< "$line"
  echo "$(basename "$path") $size $mtime"
  exit 0
fi

step check
# The target workspace must exist before replacing the current deployment.
: "${PROJECT_WORKSPACE_DIR:?PROJECT_WORKSPACE_DIR must name an existing workspace directory}"
[ -d "$PROJECT_WORKSPACE_DIR" ] || die "PROJECT_WORKSPACE_DIR is not a directory: $PROJECT_WORKSPACE_DIR"
export PROJECT_WORKSPACE_DIR
if [ "$IS_ROOT" != 1 ]; then
  for unit in chenweb pdf-parser; do
    sudo -n -l /bin/systemctl restart "$unit" >/dev/null 2>&1 ||
      die "gui may not run 'systemctl restart $unit' without a password; add it to /etc/sudoers.d/gui-chenweb-services"
  done
  echo "  sudoers rule allows systemctl for chenweb and pdf-parser"
fi
echo "  running as $(id -un); workspace $PROJECT_WORKSPACE_DIR"

step fetch
if [ "$#" -gt 0 ]; then
  archive=$1
  echo "Using local archive $archive"
else
  rm -rf "$STAGING_DIR"
  mkdir -p "$STAGING_DIR"
  if [ -n "${RELAY_ARCHIVE:-}" ]; then
    [[ "$RELAY_ARCHIVE" =~ ^runshen-[A-Za-z0-9._-]+\.tar\.gz$ ]] || die "invalid RELAY_ARCHIVE: $RELAY_ARCHIVE"
    latest="$RELAY_DIR/$RELAY_ARCHIVE"
  else
    latest=$(ssh -p "$RELAY_PORT" "$RELAY_USER@$RELAY_HOST" \
      bash -c "'ls -t $RELAY_DIR/runshen-*.tar.gz 2>/dev/null | head -n 1'")
    [ -n "$latest" ] || die "No runshen archive found on $RELAY_USER@$RELAY_HOST:$RELAY_DIR"
  fi
  echo "Pulling $latest from $RELAY_USER@$RELAY_HOST:$RELAY_PORT ..."
  scp -P "$RELAY_PORT" "$RELAY_USER@$RELAY_HOST:$latest" "$STAGING_DIR/"
  archive="$STAGING_DIR/$(basename "$latest")"
fi
[ -f "$archive" ] || die "Archive not found: $archive"
ls -l "$archive"

step extract
rm -rf "$DEPLOY_DIR"
mkdir -p "$CHENWEB_DIR"
tar -xzf "$archive" -C "$CHENWEB_DIR" --strip-components=1
echo "  extracted into $DEPLOY_DIR"
# Install this script's packaged copy for the next deploy. mv replaces the file without
# touching the copy bash is reading now.
if [ -f "$DEPLOY_DIR/shell_server_deploy.sh" ]; then
  cp "$DEPLOY_DIR/shell_server_deploy.sh" "$CHENWEB_DIR/shell_server_deploy.sh.new"
  chmod 0755 "$CHENWEB_DIR/shell_server_deploy.sh.new"
  mv -f "$CHENWEB_DIR/shell_server_deploy.sh.new" "$CHENWEB_DIR/shell_server_deploy.sh"
  echo "  installed shell_server_deploy.sh from the archive"
fi

step documents
# Older packaged installers do not install Document Builder files themselves.
if [ -d "$DEPLOY_DIR/document-workspace" ]; then
  echo "== Document Builder files: $PROJECT_WORKSPACE_DIR =="
  if [ "$IS_ROOT" = 1 ]; then
    rsync -a --chown=gui:gui "$DEPLOY_DIR/document-workspace/" "$PROJECT_WORKSPACE_DIR/"
  else
    rsync -a --no-owner --no-group "$DEPLOY_DIR/document-workspace/" "$PROJECT_WORKSPACE_DIR/"
  fi
fi
bash "$DEPLOY_DIR/deploy-server-china.sh" "$DEPLOY_DIR"

# --- pdf-parser -------------------------------------------------------------
# deploy-server-china.sh installs only the Go binaries. The PDF parser runs as the
# chenweb-pdf-parser image, so rebuild it from the sources in this archive, using
# the steps in python/pdf-parser/container/README.md.
step pdf-parser
WORKSPACE_DIR=/home/gui/Workspace
PDF_SRC="$DEPLOY_DIR/pdf-parser"
PDF_DIR="$CHENWEB_DIR/python/pdf-parser"
PDF_BUILD_DIR="$WORKSPACE_DIR/deploy/mineru-4.0.9-build"
PDF_IMAGE=chenweb-pdf-parser:mineru-4.0.9

if [ -d "$PDF_SRC" ]; then
  echo "== pdf-parser =="
  # The image snapshots this tree, so mirror the archive exactly: stale modules are removed.
  owner=(); [ "$IS_ROOT" = 1 ] && owner=(--chown=gui:gui)
  rsync -a --delete ${owner[@]+"${owner[@]}"} --exclude .venv --exclude __pycache__ --exclude .pytest_cache --exclude tests \
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
    || die "$PDF_IMAGE is missing the table geometry code"
  echo "  image built and verified: $PDF_IMAGE"
  # Restarting the unit interrupts any parse in progress.
  systemctl_cmd restart pdf-parser
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
    die "running pdf-parser lacks table_geometry.py"
  echo "  pdf-parser running $PDF_IMAGE with table geometry: OK"
else
  echo "!! archive has no pdf-parser sources; the parser was NOT rebuilt (build it with the updated shell_server_build_rcp.sh)" >&2
fi
