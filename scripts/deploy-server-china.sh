#!/usr/bin/env bash
#
# Deploy ChenWeb linux/amd64 binaries on the China box
# (onto.bzton.cn / 210.5.158.91, hostname rssvr19).
#
# Build side (Mac):   mise build-server-linux            -> /tmp/chenweb-deploy/
# Copy:               scp -P 8822 -r /tmp/chenweb-deploy gui@210.5.158.91:~/
# Deploy side (box):   su -
#                      bash ~/chenweb-deploy/deploy-server-china.sh ~/chenweb-deploy
#
# Run ON THE BOX, as root (systemctl needs it; `gui` is not a sudoer).
# Per binary: verify sha256, verify ELF/arch, back up the current binary,
# stop -> swap -> start the matching service, health-check, and auto-roll-back
# to the previous binary if the health-check fails.
#
#   bash deploy-server-china.sh <dir> [name ...]
#
#   <dir>   directory holding <name>-linux + <name>-linux.sha256
#           (default: the directory this script lives in)
#   name    one or more of:
#             server  doc-processor  parser-result-converter  doc-service  create-admin
#           (default: every *-linux found in <dir>)
#
# Env overrides:
#   CHENWEB_DIR   (default /home/gui/Workspace/ChenWeb)
#   RUN_USER      (default gui)          owner of the installed binaries
#   PORT          (default 8090)         chenweb health-check port
#   KEEP_BAKS     (default 3)            how many .bak-<ts> copies to retain
#   DRY_RUN=1     print privileged actions instead of running them

set -euo pipefail

CHENWEB_DIR=${CHENWEB_DIR:-/home/gui/Workspace/ChenWeb}
RUN_USER=${RUN_USER:-gui}
PORT=${PORT:-8090}
KEEP_BAKS=${KEEP_BAKS:-3}
DRY_RUN=${DRY_RUN:-0}

die() { echo "ERROR: $*" >&2; exit 1; }
run() { if [ "$DRY_RUN" = 1 ]; then echo "  [dry-run] $*"; else "$@"; fi; }

# --- valid binary -> systemd service (empty = CLI tool, install only) ---------
valid_names="server doc-processor parser-result-converter doc-service create-admin"
svc_for() {
  case "$1" in
    server)                   printf 'chenweb' ;;
    doc-processor)            printf 'doc-processor' ;;
    parser-result-converter)  printf 'parser-result-converter' ;;
    doc-service)              printf 'doc-service' ;;
    create-admin)             printf '' ;;
    *)                        printf '__invalid__' ;;
  esac
}

# --- args -------------------------------------------------------------------
SRC_DIR=${1:-$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)}
[ $# -gt 0 ] && shift || true
NAMES=("$@")

[ "$DRY_RUN" = 1 ] || [ "$(id -u)" -eq 0 ] || die "must run as root (su -); systemctl needs it. Or set DRY_RUN=1."
[ -d "$SRC_DIR" ] || die "source dir not found: $SRC_DIR"
[ "$DRY_RUN" = 1 ] || [ -d "$CHENWEB_DIR" ] || die "ChenWeb dir not found: $CHENWEB_DIR"

if [ ${#NAMES[@]} -eq 0 ]; then
  shopt -s nullglob
  for f in "$SRC_DIR"/*-linux; do NAMES+=("$(basename "$f" -linux)"); done
  shopt -u nullglob
  [ ${#NAMES[@]} -gt 0 ] || die "no *-linux files in $SRC_DIR"
fi

# validate all names up front (die-in-subshell would not abort the script)
for n in "${NAMES[@]}"; do
  case " $valid_names " in *" $n "*) : ;; *) die "unknown binary '$n' (valid: $valid_names)" ;; esac
done

echo "== plan =="
echo "  source : $SRC_DIR"
echo "  target : $CHENWEB_DIR"
for n in "${NAMES[@]}"; do
  s=$(svc_for "$n"); printf '  %-26s -> %s\n' "$n-linux" "${s:-<install only>}"
done
echo

ts=$(date +%Y%m%d-%H%M%S)
deployed=()

for n in "${NAMES[@]}"; do
  bin="$SRC_DIR/$n-linux"
  dst="$CHENWEB_DIR/$n-linux"
  svc=$(svc_for "$n")

  echo "== $n-linux =="
  [ -f "$bin" ] || die "missing $bin"
  [ -f "$SRC_DIR/$n-linux.sha256" ] || die "missing $SRC_DIR/$n-linux.sha256"

  ( cd "$SRC_DIR" && sha256sum -c "$n-linux.sha256" ) || die "checksum FAILED for $n-linux"
  finfo=$(file -b "$bin")
  case "$finfo" in
    *"ELF 64-bit"*) : ;; *) die "$n-linux is not ELF 64-bit: $finfo" ;;
  esac
  case "$finfo" in
    *x86-64*|*x86_64*) : ;; *) die "$n-linux is not x86-64: $finfo" ;;
  esac
  echo "  checksum ok; $finfo"

  if [ "$DRY_RUN" != 1 ] && [ -f "$dst" ]; then
    cp -a "$dst" "$dst.bak-$ts"
    echo "  backup -> $dst.bak-$ts"
    # shellcheck disable=SC2012
    ls -1t "$dst".bak-* 2>/dev/null | tail -n +$((KEEP_BAKS + 1)) | while read -r old; do rm -f "$old"; done
  fi

  if [ -z "$svc" ]; then
    run install -m 0755 -o "$RUN_USER" -g "$RUN_USER" "$bin" "$dst"
    echo "  installed (CLI tool, no service)"
    deployed+=("$n")
    continue
  fi

  run systemctl stop "$svc"
  run install -m 0755 -o "$RUN_USER" -g "$RUN_USER" "$bin" "$dst"
  run systemctl start "$svc"

  if [ "$DRY_RUN" = 1 ]; then echo "  [dry-run] skip health check"; deployed+=("$n"); continue; fi

  ok=no
  for _ in $(seq 1 30); do
    if systemctl is-active --quiet "$svc"; then
      if [ "$svc" = chenweb ]; then
        code=$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:$PORT/" || true)
        [ "$code" = 200 ] && { ok=yes; break; }
      else
        ok=yes; break
      fi
    fi
    sleep 1
  done

  if [ "$ok" != yes ]; then
    echo "  !! health check FAILED for $svc -- rolling back"
    latest=$(ls -1t "$dst".bak-* 2>/dev/null | head -1 || true)
    run systemctl stop "$svc"
    if [ -n "$latest" ]; then cp -a "$latest" "$dst"; echo "  restored $latest"; fi
    run systemctl start "$svc"
    echo "  ---- journalctl -u $svc -n 50 ----"
    journalctl -u "$svc" -n 50 --no-pager || true
    die "deploy of $n-linux failed; rolled back to ${latest:-<none>}"
  fi

  echo "  OK: $svc active$([ "$svc" = chenweb ] && echo ', http 200')"
  journalctl -u "$svc" -n 12 --no-pager || true
  deployed+=("$n")
done

echo
echo "== done: ${deployed[*]:-<none>} =="
if [ "$DRY_RUN" != 1 ] && printf '%s\n' "${deployed[@]:-}" | grep -qx server; then
  cfg=$(curl -s "http://127.0.0.1:$PORT/api/config" || true)
  echo "  /api/config enable_phone_login: $(printf '%s' "$cfg" | grep -o '"enable_phone_login":[^,}]*' || echo '(field not found)')"
fi
