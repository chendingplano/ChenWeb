#!/usr/bin/env bash
# Install supervision policies and independent timer tasks on the China box.
# Run as root AFTER installing instrumented binaries. Does not restart workers.
set -euo pipefail
[ "$(id -u)" = 0 ] || { echo 'Run as root' >&2; exit 1; }
CHENWEB_DIR=${CHENWEB_DIR:-/home/gui/Workspace/ChenWeb}
SOURCE_DIR=$(cd "$(dirname "$0")" && pwd)
HEALTH_ROOT="$CHENWEB_DIR/Data/ProcessHealth"
install -d -m 0755 -o gui -g gui "$HEALTH_ROOT" "$HEALTH_ROOT/reports"
install -d -m 0755 -o root -g root "$HEALTH_ROOT/monitor" /etc/chenweb
install -m 0644 "$SOURCE_DIR/process-monitor.json" /etc/chenweb/process-monitor.json
# The supervisor's logger must not depend on application configuration files.
if [ ! -e /etc/chenweb/process-monitor-libconfig.toml ]; then
  printf '# Independent monitor logger; defaults are sufficient.\n' > /etc/chenweb/process-monitor-libconfig.toml
  chmod 0644 /etc/chenweb/process-monitor-libconfig.toml
fi
for task in process-monitor doc-recovery; do
  [ -x "$CHENWEB_DIR/$task-linux" ] || { echo "Missing $task-linux" >&2; exit 1; }
  install -m 0644 "$SOURCE_DIR/$task.service" "$SOURCE_DIR/$task.timer" /etc/systemd/system/
done
for service in chenweb doc-processor doc-service parser-result-converter pdf-parser; do
  install -d -m 0755 "/etc/systemd/system/$service.service.d"
  cat > "/etc/systemd/system/$service.service.d/process-health.conf" <<EOF
[Unit]
StartLimitIntervalSec=0
[Service]
Environment=PROCESS_HEALTH_DIR=$HEALTH_ROOT/reports
Environment=PROCESS_HEALTH_MONITOR_STATE=$HEALTH_ROOT/monitor/state.json
Restart=always
RestartSec=10
RestartPreventExitStatus=78
EOF
done
# These services have native ownership; no competing container restart policies.
for service in nats-server kratos nginx; do
  install -d -m 0755 "/etc/systemd/system/$service.service.d"
  cat > "/etc/systemd/system/$service.service.d/process-health.conf" <<EOF
[Unit]
StartLimitIntervalSec=0
[Service]
Restart=always
RestartSec=10
EOF
done
cat > /etc/logrotate.d/chenweb-process-health <<EOF
$HEALTH_ROOT/monitor/alarms.jsonl {
  daily
  rotate 14
  compress
  missingok
  notifempty
  create 0644 root root
}
EOF
systemctl daemon-reload
# Start monitoring after workers have been restarted with the new environment.
systemctl enable process-monitor.timer doc-recovery.timer
echo 'Installed. Restart instrumented workers, verify health reports, then start process-monitor.timer and doc-recovery.timer.'
