#!/usr/bin/env bash
# Usage: ./shell_server_build_rcp.sh "build_name" "target_machine" "os_info" "description"
# Requires PROJECT_WORKSPACE_DIR and PostgreSQL settings (PG_* or ChenWeb/.env).
# All kb.doc_group_members files and directories are staged relative to the workspace.

set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
cd "$SCRIPT_DIR"

# One marker line per step; the Deployment page (deploy-runner) turns them into step rows.
step() { if [ $# -gt 1 ]; then printf '::step::%s::%s\n' "$1" "$2"; else printf '::step::%s\n' "$1"; fi; }
step check

if [ "$#" -gt 4 ]; then
  echo "usage: $0 [build_name [target_machine [os_info [description]]]" >&2
  exit 2
fi

export BUILD_NAME="${1:-chenweb}"
export TARGET_MACHINE="${2:-unknown}"
export OS_INFO="${3:-linux/amd64}"
export BUILD_DESCRIPTION="${4:-}"

# Check the workspace before starting the potentially expensive server build.
: "${PROJECT_WORKSPACE_DIR:?PROJECT_WORKSPACE_DIR must name an existing workspace directory}"
[ -d "$PROJECT_WORKSPACE_DIR" ] || { echo "!! PROJECT_WORKSPACE_DIR is not a directory: $PROJECT_WORKSPACE_DIR" >&2; exit 1; }

# Direct scp to the China box (210.5.158.91) can be extremely slow. Instead,
# push to the Mac Mini (192.168.29.96), which the China box then pulls from
# over its own, faster path -- see shell_server_deploy.sh.
RELAY_HOST=192.168.29.96
RELAY_PORT=8822
RELAY_USER=cding
RELAY_DIR=/home/cding/Backups

# Release versioning (openspec release-versioning): a successful build is the next
# revision. Reserve it before compiling so the version is embedded in the binaries; the
# reservation takes every pending release item. Any failure from here on gives the
# version back and returns the items to pending (release_cleanup).
RELEASE_TOOL=/tmp/chenweb-release-tool
go build -o "$RELEASE_TOOL" ./server/cmd/release-tool
release_tool() { mise exec -- "$RELEASE_TOOL" "$@"; }
RELEASE_VERSION=$(release_tool reserve --pid $$ | sed -n 's/^RELEASE_VERSION=//p' | tail -n 1)
if [[ ! "$RELEASE_VERSION" =~ ^[0-9]{2}\.[0-9]{2}\.[0-9]{2}$ ]]; then
  echo "!! release-tool reserve did not report a release version" >&2
  exit 1
fi
export RELEASE_VERSION
echo "==> release $RELEASE_VERSION"
RELAY_UPLOADED=""
release_cleanup() {
  local status=$?
  if [ "$status" -ne 0 ]; then
    if [ -n "$RELAY_UPLOADED" ]; then
      echo "!! removing the uploaded files from the relay:$RELAY_UPLOADED" >&2
      # The relay's login shell is tcsh; run rm under bash (see shell_server_deploy.sh).
      ssh -o BatchMode=yes -p "$RELAY_PORT" "$RELAY_USER@$RELAY_HOST" bash -c "'rm -f$RELAY_UPLOADED'" \
        || echo "!! could not remove$RELAY_UPLOADED from the relay; delete it by hand" >&2
    fi
    release_tool abort --version "$RELEASE_VERSION" \
      || echo "!! could not give back release $RELEASE_VERSION; the next build reverts it" >&2
  fi
  exit "$status"
}
trap release_cleanup EXIT

mise run build-server-linux
BUILD_NUMBER=$(sed -n 's/^build_number: //p' /tmp/chenweb-deploy/MANIFEST)
if [[ ! "$BUILD_NUMBER" =~ ^[0-9]{14}$ ]]; then
  echo "!! Invalid build number in /tmp/chenweb-deploy/MANIFEST" >&2
  exit 1
fi

# The server build recreates /tmp/chenweb-deploy, so stage documents afterwards.
step documents
python3 "$SCRIPT_DIR/scripts/stage_document_members.py" /tmp/chenweb-deploy/document-workspace

# The PDF parser runs as the chenweb-pdf-parser image, which the China box builds
# from these sources (python/pdf-parser/container/README.md). Ship them in the
# same archive so the image is rebuilt from the code being deployed.
step pdf-parser
PDF_SRC="$SCRIPT_DIR/python/pdf-parser"
PDF_OUT=/tmp/chenweb-deploy/pdf-parser
mkdir -p "$PDF_OUT/container/mineru-v4"
cp "$PDF_SRC"/*.py "$PDF_OUT/"
cp -R "$PDF_SRC/container/." "$PDF_OUT/container/"
# The modules prepare-context.py copies from container/mineru-v4/ over the deployed worker.
cp "$PDF_SRC/parser_mineru.py" "$PDF_SRC/mineru_v4_adapter.py" "$PDF_SRC/mineru_table_repair.py" "$PDF_SRC/mineru_cell_repair.py" "$PDF_SRC/stacked_cells.py" "$PDF_SRC/table_geometry.py" "$PDF_OUT/container/mineru-v4/"

# release/: release.json (this release and the whole history) and the notes as text and
# PDF in English and Chinese. The deploy installs it; the server imports it at start.
step release
release_tool notes --version "$RELEASE_VERSION" --build-number "$BUILD_NUMBER" \
  --out /tmp/chenweb-deploy/release --description "$BUILD_DESCRIPTION"

# The deploy side reads the build number from this name (runshen-deploy-<build_number>).
archive="/tmp/runshen-deploy-${BUILD_NUMBER}.tar.gz"
# The release file also sits next to the archive, so the deploy machine can list
# releases and show their notes without downloading archives.
sidecar="/tmp/runshen-deploy-${BUILD_NUMBER}.release.json"

step archive
tar -czvf "$archive" /tmp/chenweb-deploy
cp /tmp/chenweb-deploy/release/release.json "$sidecar"
step upload
echo "uploading $(basename "$archive") and $(basename "$sidecar") to $RELAY_USER@$RELAY_HOST:$RELAY_DIR/"
RELAY_UPLOADED=" $RELAY_DIR/${archive##*/} $RELAY_DIR/${sidecar##*/}"
scp -P "$RELAY_PORT" "$archive" "$sidecar" "$RELAY_USER@$RELAY_HOST:$RELAY_DIR/"
rm -f "$archive" "$sidecar"

step release-commit
release_tool commit --version "$RELEASE_VERSION"
RELAY_UPLOADED=""
printf '::archive::%s\n' "${archive##*/}"
