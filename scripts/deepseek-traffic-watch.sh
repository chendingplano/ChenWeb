#!/usr/bin/env bash
# Logs each new TCP connection from a local process to api.deepseek.com, with
# the process id and command line. It samples lsof once per interval, so a
# connection that opens and closes between samples can be missed. Requests
# are long enough (seconds) that normal LLM calls are caught.
#
# Usage: scripts/deepseek-traffic-watch.sh   (Ctrl-C to stop)
# Env:   DEEPSEEK_WATCH_LOG       log file (default: $SYSTEM_MONITOR_LOG_ROOT/deepseek-traffic.log)
#        DEEPSEEK_WATCH_INTERVAL  seconds between samples (default: 1)
#        DEEPSEEK_WATCH_EXCLUDE   regex of process names to skip (default: ^Google Chrome)
#                                 Chrome shares CloudFront edge IPs with DeepSeek, so its
#                                 unrelated connections would be logged as DeepSeek traffic.
set -euo pipefail

HOST="api.deepseek.com"
SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
. "$SCRIPT_DIR/system-monitor-env.sh"
LOG="$DEEPSEEK_WATCH_LOG"
INTERVAL="${DEEPSEEK_WATCH_INTERVAL:-1}"
EXCLUDE="${DEEPSEEK_WATCH_EXCLUDE:-^Google Chrome}"

mkdir -p "$(dirname "$LOG")"
seen_file="$(mktemp)"
trap 'rm -f "$seen_file"' EXIT

resolve() { dig +short "$HOST" | grep -E '^[0-9.]+$' || true; }

ips="$(resolve)"
refreshed=$SECONDS
echo "watching $HOST ($(echo "$ips" | tr '\n' ' ')) -> $LOG" >&2

sample() {
  for ip in $ips; do
    lsof -nP -iTCP@"$ip" -sTCP:ESTABLISHED -F pcn 2>/dev/null || true
  done | awk '
    /^p/ { pid = substr($0, 2) }
    /^c/ { cmd = substr($0, 2) }
    /^n/ {
      s = substr($0, 2)
      if (index(s, "->")) {
        split(s, parts, "->")
        print pid "\t" cmd "\t" parts[1] "\t" parts[2]
      }
    }'
}

while true; do
  # Re-resolve every five minutes in case DeepSeek changes addresses.
  if (( SECONDS - refreshed >= 300 )); then
    ips="$(resolve)"
    refreshed=$SECONDS
  fi

  sample | while IFS=$'\t' read -r pid cmd lport remote; do
    if [[ $cmd =~ $EXCLUDE ]]; then
      continue
    fi
    key="$pid $lport $remote"
    if ! grep -qxF "$key" "$seen_file"; then
      echo "$key" >> "$seen_file"
      # Redact API keys that a tool passed on its command line.
      args="$(ps -o args= -p "$pid" 2>/dev/null | cut -c1-200 | sed -E 's/sk-[A-Za-z0-9_-]+/sk-<redacted>/g' || true)"
      echo "$(date '+%Y-%m-%d %H:%M:%S') pid=$pid cmd=$cmd $lport -> $remote args=$args" | tee -a "$LOG"
    fi
  done

  sleep "$INTERVAL"
done
