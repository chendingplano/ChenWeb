#!/usr/bin/env bash
# render-prod-env.sh — port env vars from ChenWeb/mise.local.toml to the China box.
#
# WHY THIS EXISTS
#   The dingbo/onto box (210.5.158.91) has no `mise`. Its systemd units read env
#   vars from plain files via `EnvironmentFile=`:
#       ~/Workspace/ChenWeb/.env        -> chenweb, doc-processor,
#                                          parser-result-converter, pdf-parser
#       ~/Workspace/Kratos/kratos.env   -> kratos
#   plus a few `Environment=` lines baked into the .service units
#   (SHARED_LIB_CONFIG_DIR; the MINERU_* block for pdf-parser).
#
#   Those two .env files are the PRODUCTION source of truth. They are maintained
#   by hand and hold prod values (prod domain, prod keys, /home/gui/... paths,
#   port 8090, https). `mise.local.toml` holds Mac/dev values
#   (macmini.deepdocs.me, /Users/cding/..., port 8080). The two are parallel, not
#   synced.
#
#   This script is a PORTING AID, not a deploy step. It renders the
#   `mise.local.toml` [env] table into dotenv/systemd form so you can `--diff` it
#   against the deployed prod .env and see exactly which keys are new, gone, or
#   changed — then hand-port the ones that should apply to prod. Never scp its
#   raw output onto the box: it would overwrite prod values with Mac ones.
#
# TRANSLATION RULES when you do port a var over:
#   * `KEY = "value"`  ->  `KEY=value`   (strip quotes, no spaces around `=` —
#     systemd 237's EnvironmentFile parser is strict and takes values literally)
#   * `/Users/cding/...`  ->  `/home/gui/...`
#   * drop Mac-only vars (PGDATA, TEST_DATABASE_URL, DATA_SYNC_CONFIG,
#     PG_BACKUP_REMOTE_*, MINERU_DEVICE_MODE=mps, ...)
#
# USAGE
#   scripts/render-prod-env.sh                         # normalized KEY=value to stdout
#   scripts/render-prod-env.sh --diff /path/to/prod.env
#       # keys ONLY in mise.local.toml (candidates to port), keys ONLY in the
#       # prod file (box-specific), and keys whose value differs
#   MISE_LOCAL_TOML=... scripts/render-prod-env.sh     # override input path

set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TOML="${MISE_LOCAL_TOML:-$here/../mise.local.toml}"
[ -f "$TOML" ] || { echo "no such file: $TOML" >&2; exit 1; }

render() {
  awk '
    /^[[:space:]]*\[env\][[:space:]]*$/ { inenv=1; next }
    /^[[:space:]]*\[/                   { inenv=0 }
    !inenv || /^[[:space:]]*#/ || /^[[:space:]]*$/ { next }
    {
      line=$0
      eq=index(line,"=")
      if (eq==0) next
      key=substr(line,1,eq-1); val=substr(line,eq+1)
      gsub(/^[[:space:]]+|[[:space:]]+$/,"",key)
      gsub(/^[[:space:]]+/,"",val)
      q=substr(val,1,1)
      if (q=="\"" || q=="'"'"'") {
        rest=substr(val,2); ci=index(rest,q)
        if (ci>0) val=substr(rest,1,ci-1); else val=rest
      } else {
        sub(/[[:space:]]+#.*$/,"",val)          # strip trailing comment on bare values
        gsub(/[[:space:]]+$/,"",val)
      }
      if (key ~ /^[A-Za-z_][A-Za-z0-9_]*$/) print key "=" val
    }
  ' "$TOML" | sort
}

if [ "${1:-}" = "--diff" ]; then
  target="${2:?usage: render-prod-env.sh --diff /path/to/prod.env}"
  [ -f "$target" ] || { echo "no such file: $target" >&2; exit 1; }
  tmp_mise="$(mktemp)"; tmp_prod="$(mktemp)"
  trap 'rm -f "$tmp_mise" "$tmp_prod"' EXIT
  render > "$tmp_mise"
  grep -E '^[A-Za-z_][A-Za-z0-9_]*=' "$target" | sort > "$tmp_prod"

  echo "### keys in mise.local.toml [env] but NOT in $(basename "$target")  (candidates to port) ###"
  comm -23 <(cut -d= -f1 "$tmp_mise") <(cut -d= -f1 "$tmp_prod") | sed 's/^/  + /'
  echo
  echo "### keys in $(basename "$target") but NOT in mise.local.toml  (box-specific — leave) ###"
  comm -13 <(cut -d= -f1 "$tmp_mise") <(cut -d= -f1 "$tmp_prod") | sed 's/^/  - /'
  echo
  echo "### keys in both with a DIFFERENT value  (Mac value first — most are expected: domain/paths/ports; secret-ish values masked) ###"
  join -t= <(sort "$tmp_mise") <(sort "$tmp_prod") | awk -F= '
    {
      k=$1; m=$2; p=$3
      if (m==p) next
      if (k ~ /(KEY|SECRET|PASSWORD|PASSWD|TOKEN|DSN)/) { m="<set>"; p="<set, differs>" }
      printf "  %s\n      mise: %s\n      prod: %s\n", k, m, p
    }'
else
  render
fi
