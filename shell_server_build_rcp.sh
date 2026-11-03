#!/usr/bin/env bash
# Usage: ./shell_server_build_rcp.sh "build_name" "target_machine" "os_info" "description"

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

# The PDF parser runs as the chenweb-pdf-parser image, which the China box builds
# from these sources (python/pdf-parser/container/README.md). Ship them in the
# same archive so the image is rebuilt from the code being deployed.
PDF_SRC="$SCRIPT_DIR/python/pdf-parser"
PDF_OUT=/tmp/chenweb-deploy/pdf-parser
mkdir -p "$PDF_OUT/container/mineru-v4"
cp "$PDF_SRC"/*.py "$PDF_OUT/"
cp -R "$PDF_SRC/container/." "$PDF_OUT/container/"
# The modules prepare-context.py copies from container/mineru-v4/ over the deployed worker.
cp "$PDF_SRC/parser_mineru.py" "$PDF_SRC/mineru_v4_adapter.py" "$PDF_SRC/mineru_table_repair.py" "$PDF_SRC/mineru_cell_repair.py" "$PDF_SRC/stacked_cells.py" "$PDF_SRC/table_geometry.py" "$PDF_OUT/container/mineru-v4/"

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
