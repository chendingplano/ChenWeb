"""Local, atomic functional health reports shared with the independent monitor.

No database or broker is needed to report a blocked process. Callers supply only
credential-safe diagnoses and refresh reports after meaningful checks complete.
"""
import json
import os
import tempfile
import threading
import time
from datetime import datetime, timezone
from pathlib import Path


class Reporter:
    def __init__(self, service="pdf-parser"):
        directory = os.environ.get("PROCESS_HEALTH_DIR", "").strip()
        self.path = Path(directory) / f"{service}.json" if directory else None
        self.lock = threading.Lock()
        pid = int(os.environ.get("PROCESS_HEALTH_PID", str(os.getpid())))
        now = self._now()
        self.report = {"service": service, "pid": pid,
                       "generation": f"{pid}-{time.time_ns()}", "state": "starting",
                       "recovery": "none", "since": now, "observed_at": now}
        self._write()

    @staticmethod
    def _now():
        return datetime.now(timezone.utc).isoformat()

    def set(self, state, reason="", resource="", message="", recovery="none"):
        with self.lock:
            now = self._now()
            values = {"state": state, "reason": reason, "resource": resource,
                      "message": message, "recovery": recovery}
            if any(self.report.get(k) != values[k] for k in ("state", "reason", "resource", "recovery")):
                self.report["since"] = now
            self.report.update(values, observed_at=now)
            self._write()

    def _write(self):
        if self.path is None:
            return
        self.path.parent.mkdir(parents=True, exist_ok=True, mode=0o750)
        fd, name = tempfile.mkstemp(prefix=".health-", dir=self.path.parent)
        try:
            with os.fdopen(fd, "w") as handle:
                os.fchmod(handle.fileno(), 0o640)
                json.dump(self.report, handle)
                handle.flush()
                os.fsync(handle.fileno())
            os.replace(name, self.path)
        finally:
            if os.path.exists(name):
                os.unlink(name)


def writable_directory(directory):
    """Exercise directory read/write capability without retaining probe files."""
    list(Path(directory).iterdir())
    with tempfile.TemporaryFile(dir=directory):
        pass
