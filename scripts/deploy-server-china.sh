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
# Run ON THE BOX, either as root (su -) or as `gui` directly -- `gui` has a
# scoped sudoers rule (/etc/sudoers.d/gui-chenweb-services, added 2026-09-23)
# granting passwordless `sudo systemctl {start,stop,restart,status}` on
# exactly chenweb/doc-processor/parser-result-converter/doc-service/kratos/
# nats-server, nothing else. Everything this script writes lives under
# ~/Workspace/ChenWeb, already owned by gui, so no other step needs root.
# First, syncs migration files (project_migrations/, shared_migrations/), then
# prompts/ (mirrored with --delete), then the *.local.toml reviewer configs,
# then config.toml + config/ + docs/doc-templates -- all read from disk at
# runtime, not compiled into the binaries, so a binaries-only payload leaves
# them stale or missing. Migrations must land before any unit starts, since
# every binary runs goose at startup.
# Then per binary: verify sha256, verify ELF/arch, back up the current binary,
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
#   SKIP_MIGRATIONS=1  leave migration files untouched (binaries only)
#   DRY_RUN=1     print privileged actions instead of running them

set -euo pipefail

CHENWEB_DIR=${CHENWEB_DIR:-/home/gui/Workspace/ChenWeb}
RUN_USER=${RUN_USER:-gui}
PORT=${PORT:-8090}
KEEP_BAKS=${KEEP_BAKS:-3}
SKIP_MIGRATIONS=${SKIP_MIGRATIONS:-0}
DRY_RUN=${DRY_RUN:-0}

die() { echo "ERROR: $*" >&2; exit 1; }
run() { if [ "$DRY_RUN" = 1 ]; then echo "  [dry-run] $*"; else "$@"; fi; }
# systemctl needs root; run it directly if we already are root (su -), else
# through the scoped sudoers rule for gui -- see the header comment above.
systemctl_cmd() { if [ "$IS_ROOT" = 1 ]; then systemctl "$@"; else sudo -n /bin/systemctl "$@"; fi; }
# sorted basenames of the *.sql in $1 (empty, not an error, if $1 has none/missing)
list_sql() { (cd "$1" 2>/dev/null && ls -1 ./*.sql 2>/dev/null) | sed 's|^\./||' | sort; }

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

IS_ROOT=0
[ "$(id -u)" -eq 0 ] && IS_ROOT=1
if [ "$DRY_RUN" != 1 ] && [ "$IS_ROOT" != 1 ]; then
  sudo -n /bin/systemctl status chenweb >/dev/null 2>&1 \
    || die "must run as root (su -), or as gui with the scoped sudoers rule (/etc/sudoers.d/gui-chenweb-services) for systemctl. Or set DRY_RUN=1."
fi
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

# --- migrations -------------------------------------------------------------
# Must land BEFORE any unit starts: config.RunMigrations runs at every binary's
# startup and reads these dirs from disk (os.DirFS), so shipping a binary without
# them leaves the schema behind and tables go missing at runtime.
# Additive on purpose -- never --delete, so a migration the box has already
# applied is never yanked out from under goose's tracking table.
# NOTE: the per-binary auto-rollback below does NOT roll migrations back; goose
# down-migrations are not run here. Review the "new file(s)" list before deploying.
echo "== migrations =="
if [ "$SKIP_MIGRATIONS" = 1 ]; then
  echo "  SKIP_MIGRATIONS=1 -- migration files left untouched"
else
  for d in project_migrations shared_migrations; do
    src="$SRC_DIR/$d"; dst="$CHENWEB_DIR/$d"
    if [ ! -d "$src" ]; then
      echo "  $d: not in payload -- skipped (build predates migration shipping?)"
      continue
    fi
    pending=$(comm -23 <(list_sql "$src") <(list_sql "$dst") || true)
    n=$(printf '%s\n' "$pending" | grep -c . || true)
    if [ "$n" -eq 0 ]; then
      echo "  $d: already current ($(list_sql "$dst" | grep -c . || true) files)"
    else
      echo "  $d: $n new file(s), will be applied at next service start:"
      printf '%s\n' "$pending" | sed 's/^/      /'
    fi
    run mkdir -p "$dst"
    run rsync -a "$src"/ "$dst"/
    run chown -R "$RUN_USER:$RUN_USER" "$dst"
  done
fi
echo

# --- prompts ------------------------------------------------------------------
# prompts/ is read from disk per-call (PROMPT_DIR, falling back to ./prompts),
# not go:embed'd, so a binaries-only payload leaves the box's prompts frozen
# wherever the last hand-rsync left them. Unlike migrations, mirrored with
# --delete: a prompt renamed/removed on the source should not keep being read
# on the box (there's no "already applied" tracking to protect, unlike goose).
# This bit onto.bzton.cn for real (2026-09-22: stale product-mention /
# product-extraction prompts after a rename went undeployed for days).
echo "== prompts =="
src="$SRC_DIR/prompts"; dst="$CHENWEB_DIR/prompts"
if [ ! -d "$src" ]; then
  echo "  prompts: not in payload -- skipped (build predates prompt shipping?)"
else
  before=$(ls -1 "$dst" 2>/dev/null | wc -l | tr -d " ")
  run mkdir -p "$dst"
  run rsync -a --delete "$src"/ "$dst"/
  run chown -R "$RUN_USER:$RUN_USER" "$dst"
  after=$(ls -1 "$src" 2>/dev/null | wc -l | tr -d " ")
  echo "  prompts: synced ($before -> $after files)"
fi
echo

# --- reviewer configs ---------------------------------------------------------
# doc-review.local.toml / product-review.local.toml are git-tracked repo-root
# files read from disk (not compiled into the binary), so a binaries-only
# payload leaves the box's copy stale or missing entirely -- see the runbook's
# Gotcha section (prompts/ hit the same class of bug, fixed 2026-09-22;
# product-review.local.toml was never deployed here at all until 2026-09-16).
# Verbatim copy, no per-box translation needed (only symbolic model/prompt refs
# inside). Always synced regardless of which binaries are named on the command
# line, same as migrations above.
echo "== reviewer configs =="
for f in doc-review.local.toml product-review.local.toml; do
  src="$SRC_DIR/$f"; dst="$CHENWEB_DIR/$f"
  if [ ! -f "$src" ]; then
    echo "  $f: not in payload -- skipped (build predates config shipping?)"
    continue
  fi
  if [ -f "$dst" ] && cmp -s "$src" "$dst"; then
    echo "  $f: already current"
  else
    echo "  $f: installing"
    run install -m 0644 -o "$RUN_USER" -g "$RUN_USER" "$src" "$dst"
  fi
done
echo

# --- app config -----------------------------------------------------------
# config.toml (root) and the config/ tree are read from disk at runtime
# (config.LoadConfig / /api/site-config's LoadSiteConfig), not compiled into
# the binary -- same class of bug as prompts/ and the reviewer configs above.
# Hit for real on onto.bzton.cn 2026-09-23: the box's config.toml predated the
# 2026-09-17 commit that moved [doc-processing] required_processors out of
# config.toml into config.local.toml-only, so the box's stale copy kept
# silently overriding every config.local.toml edit no matter how many times
# doc-processor was restarted. config/ is mirrored with --delete like
# prompts/: it's plain config data, no "already applied" tracking to protect.
# Always synced regardless of which binaries are named on the command line.
echo "== app config =="
f=config.toml
src="$SRC_DIR/$f"; dst="$CHENWEB_DIR/$f"
if [ ! -f "$src" ]; then
  echo "  $f: not in payload -- skipped (build predates config shipping?)"
elif [ -f "$dst" ] && cmp -s "$src" "$dst"; then
  echo "  $f: already current"
else
  echo "  $f: installing"
  run install -m 0644 -o "$RUN_USER" -g "$RUN_USER" "$src" "$dst"
fi

for d in config docs/doc-templates; do
  src="$SRC_DIR/$d"; dst="$CHENWEB_DIR/$d"
  if [ ! -d "$src" ]; then
    echo "  $d/: not in payload -- skipped (build predates config shipping?)"
    continue
  fi
  run mkdir -p "$dst"
  run rsync -a --delete "$src"/ "$dst"/
  run chown -R "$RUN_USER:$RUN_USER" "$dst"
  echo "  $d/: synced ($(find "$src" -type f | wc -l | tr -d ' ') files)"
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

  run systemctl_cmd stop "$svc"
  run install -m 0755 -o "$RUN_USER" -g "$RUN_USER" "$bin" "$dst"
  run systemctl_cmd start "$svc"

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
    run systemctl_cmd stop "$svc"
    if [ -n "$latest" ]; then cp -a "$latest" "$dst"; echo "  restored $latest"; fi
    run systemctl_cmd start "$svc"
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
