#!/usr/bin/env bash
#
# Restart ChenWeb services (+ their infra) on the China box without `su -`.
# gui has a scoped sudoers rule (/etc/sudoers.d/gui-chenweb-services, added
# 2026-09-23) granting passwordless `sudo systemctl {start,stop,restart,status}`
# on exactly these six units -- nothing else.
#
#   bash shell_server_restart.sh [unit ...]
#
#   unit   one or more of: nats-server kratos chenweb doc-processor
#          parser-result-converter doc-service
#          (default: all six, in dependency order -- see the start-system devdoc)

set -euo pipefail

ALL_UNITS="nats-server kratos chenweb doc-processor parser-result-converter doc-service"

if [ "$#" -gt 0 ]; then
  UNITS=("$@")
else
  # shellcheck disable=SC2206
  UNITS=($ALL_UNITS)
fi

for u in "${UNITS[@]}"; do
  case " $ALL_UNITS " in
    *" $u "*) : ;;
    *) echo "ERROR: unknown unit '$u' (valid: $ALL_UNITS)" >&2; exit 1 ;;
  esac
done

for u in "${UNITS[@]}"; do
  echo "== $u =="
  sudo -n /bin/systemctl restart "$u"
  sleep 1
  # is-active is a read-only query -- no sudo needed for it.
  if systemctl is-active --quiet "$u"; then
    echo "  active"
  else
    echo "  !! NOT active -- journalctl -u $u -n 30 --no-pager"
    journalctl -u "$u" -n 30 --no-pager || true
  fi
done
