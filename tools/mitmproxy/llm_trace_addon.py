import json
import os
import pathlib
import urllib.error
import urllib.request
from datetime import datetime, timezone

from mitmproxy import ctx
from mitmproxy import http


def _env(name: str, default: str = "") -> str:
    return os.environ.get(name, default).strip()


def _allowed_hosts() -> set[str]:
    raw = _env("MITMTRACE_ALLOWED_HOSTS", "api.anthropic.com,api.openai.com")
    return {item.strip().lower() for item in raw.split(",") if item.strip()}


def _max_body_bytes() -> int:
    raw = _env("MITMTRACE_MAX_BODY_BYTES", "16384")
    try:
        value = int(raw)
    except ValueError:
        return 16384
    return max(value, 1024)


def _truncate(text: str) -> str:
    limit = _max_body_bytes()
    if len(text) <= limit:
        return text
    return text[:limit] + "\n...[truncated]"


def _message_text(message) -> str:
    if message is None:
        return ""
    try:
        text = message.get_text(strict=False)
        if text is not None:
            return _truncate(text)
    except Exception:
        pass

    raw = getattr(message, "raw_content", None)
    if raw is None:
        raw = getattr(message, "content", None)
    if not raw:
        return ""
    if isinstance(raw, bytes):
        return _truncate(raw.decode("utf-8", "replace"))
    return _truncate(str(raw))


def _header_map(headers) -> dict[str, str]:
    include = _env("MITMTRACE_INCLUDE_HEADERS", "").lower() in {"1", "true", "yes"}
    if not include:
        return {}
    out: dict[str, str] = {}
    for key, value in headers.items(multi=True):
        lower = key.lower()
        if lower in {"authorization", "x-api-key", "cookie", "set-cookie"}:
            out[key] = "[redacted]"
            continue
        out[key] = value
    return out


def _duration_ms(flow: http.HTTPFlow) -> int:
    start = getattr(flow.request, "timestamp_start", None)
    end = None
    if flow.response is not None:
        end = getattr(flow.response, "timestamp_end", None) or getattr(flow.response, "timestamp_start", None)
    if end is None and flow.error is not None:
        end = getattr(flow.error, "timestamp", None)
    if not start or not end or end < start:
        return 0
    return int((end - start) * 1000)


def _started_at(flow: http.HTTPFlow) -> str:
    start = getattr(flow.request, "timestamp_start", None)
    if not start:
        return ""
    return datetime.fromtimestamp(start, tz=timezone.utc).isoformat()


class LLMTraceCapture:
    def __init__(self) -> None:
        self.ingest_url = _env(
            "CHENWEB_MITM_INGEST_URL",
            "http://127.0.0.1:8080/api/internal/mitmproxy/ingest",
        )
        self.ingest_token = _env("MITM_TRACE_INGEST_TOKEN")
        self.jsonl_path = _env("MITMTRACE_JSONL_PATH")
        self.allowed_hosts = _allowed_hosts()

    def load(self, loader) -> None:
        if not self.ingest_token:
            ctx.log.warn("MITM_TRACE_INGEST_TOKEN is empty; addon will log locally only.")
        ctx.log.info(f"LLM trace capture enabled for hosts: {sorted(self.allowed_hosts)}")

    def response(self, flow: http.HTTPFlow) -> None:
        self._capture(flow)

    def error(self, flow: http.HTTPFlow) -> None:
        self._capture(flow)

    def _capture(self, flow: http.HTTPFlow) -> None:
        host = (flow.request.host or "").lower()
        if host not in self.allowed_hosts:
            return
        if self._is_ingest_call(flow):
            return

        payload = {
            "source": "mitmproxy",
            "agent_kind": _env("MITMTRACE_AGENT_KIND"),
            "agent_name": _env("MITMTRACE_AGENT_NAME"),
            "agent_session": _env("MITMTRACE_AGENT_SESSION"),
            "session_id": flow.id,
            "method": flow.request.method,
            "url": flow.request.pretty_url,
            "host": flow.request.host,
            "path": flow.request.path,
            "status_code": flow.response.status_code if flow.response is not None else 0,
            "started_at": _started_at(flow),
            "duration_ms": _duration_ms(flow),
            "request_headers": _header_map(flow.request.headers),
            "response_headers": _header_map(flow.response.headers) if flow.response is not None else {},
            "request_body": _message_text(flow.request),
            "response_body": _message_text(flow.response),
            "error": flow.error.msg if flow.error is not None else "",
        }

        self._write_jsonl(payload)
        self._forward(payload)

    def _is_ingest_call(self, flow: http.HTTPFlow) -> bool:
        return flow.request.host in {"127.0.0.1", "localhost"} and flow.request.path.startswith("/api/internal/mitmproxy/ingest")

    def _write_jsonl(self, payload: dict) -> None:
        if not self.jsonl_path:
            return
        target = pathlib.Path(self.jsonl_path).expanduser()
        target.parent.mkdir(parents=True, exist_ok=True)
        with target.open("a", encoding="utf-8") as fh:
            fh.write(json.dumps(payload, ensure_ascii=True) + "\n")

    def _forward(self, payload: dict) -> None:
        if not self.ingest_token:
            return
        data = json.dumps(payload, ensure_ascii=True).encode("utf-8")
        req = urllib.request.Request(
            self.ingest_url,
            data=data,
            headers={
                "Content-Type": "application/json",
                "Authorization": f"Bearer {self.ingest_token}",
            },
            method="POST",
        )
        try:
            with urllib.request.urlopen(req, timeout=2):
                return
        except urllib.error.HTTPError as exc:
            ctx.log.warn(f"trace forward failed with HTTP {exc.code}")
        except Exception as exc:  # pragma: no cover - runtime safety path
            ctx.log.warn(f"trace forward failed: {exc}")


addons = [LLMTraceCapture()]
