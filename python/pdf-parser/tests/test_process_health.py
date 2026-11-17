import json
import os
from pathlib import Path

from process_health import Reporter, writable_directory


def test_report_contract_survives_operator_fault(tmp_path, monkeypatch):
    monkeypatch.setenv("PROCESS_HEALTH_DIR", str(tmp_path))
    monkeypatch.setenv("PROCESS_HEALTH_PID", "1234")
    reporter = Reporter()
    reporter.set("blocked", "parser_configuration_invalid", "PDF_PARSER_NAME",
                 "Configured parser is unavailable", "operator_action")
    report = json.loads((tmp_path / "pdf-parser-1234.json").read_text())
    assert report["pid"] == 1234
    assert report["state"] == "blocked"
    assert report["recovery"] == "operator_action"
    since = report["since"]
    reporter.set("blocked", "parser_configuration_invalid", "PDF_PARSER_NAME",
                 "Configured parser is unavailable", "operator_action")
    assert json.loads((tmp_path / "pdf-parser-1234.json").read_text())["since"] == since
    reporter.set("healthy")
    assert json.loads((tmp_path / "pdf-parser-1234.json").read_text())["state"] == "healthy"
    assert list(tmp_path.glob(".health-*")) == []


def test_disabled_reporter_does_not_create_files(tmp_path, monkeypatch):
    monkeypatch.delenv("PROCESS_HEALTH_DIR", raising=False)
    monkeypatch.chdir(tmp_path)
    Reporter().set("healthy")
    assert list(tmp_path.iterdir()) == []


def test_directory_probe_leaves_no_files(tmp_path):
    writable_directory(tmp_path)
    assert list(tmp_path.iterdir()) == []


def test_missing_configured_parser_resource_is_operator_fault(tmp_path, monkeypatch):
    import pdf_parser
    monkeypatch.setenv("MINERU_CONFIG", str(tmp_path / "missing-model-config"))
    issue = pdf_parser._check_storage_and_backend([str(tmp_path)], str(tmp_path), {"mineru": object()})
    assert issue[0] == "blocked"
    assert issue[1] == "parser_configuration_missing"
    assert issue[-1] == "operator_action"


def test_parser_resource_recovery_requires_real_capability(tmp_path, monkeypatch):
    import pdf_parser
    config = tmp_path / "config.yaml"
    config.write_text("{}")
    monkeypatch.setenv("MINERU_CONFIG", str(config))
    assert pdf_parser._check_storage_and_backend([str(tmp_path)], str(tmp_path), {"mineru": object()}) is None


def test_database_health_error_redacts_client_exception(tmp_path, monkeypatch):
    import pdf_parser
    def fail(_dsn):
        raise RuntimeError("postgres://private-password@host")
    monkeypatch.setattr(pdf_parser, "pg_connect", fail)
    issue = pdf_parser._check_pdf_dependencies({}, [str(tmp_path)], str(tmp_path), {})
    assert issue[1] == "database_unavailable"
    assert issue[-1] == "restart"
    assert "private-password" not in str(issue)


def test_idle_jetstream_worker_reports_healthy_without_completed_jobs(tmp_path, monkeypatch):
    import asyncio
    import pdf_parser
    monkeypatch.setenv("PROCESS_HEALTH_DIR", str(tmp_path))
    monkeypatch.setattr(pdf_parser, "_health", Reporter())
    monkeypatch.setattr(pdf_parser, "_check_pdf_dependencies", lambda *args: None)
    async def scenario():
        stop = asyncio.Event()
        class Connection:
            is_connected = True
        class Bus:
            _nc = Connection()
        class Subscription:
            async def consumer_info(self):
                stop.set()
                return object()
        await pdf_parser._run_pdf_health(stop, Bus(), Subscription(), {}, [], "", {})
    asyncio.run(scenario())
    report = json.loads((tmp_path / f"pdf-parser-{os.getpid()}.json").read_text())
    assert report["state"] == "healthy"


def test_separate_process_reports_do_not_overwrite(tmp_path, monkeypatch):
    monkeypatch.setenv("PROCESS_HEALTH_DIR", str(tmp_path))
    monkeypatch.setenv("PROCESS_HEALTH_PID", "1234")
    first = Reporter()
    first.set("healthy")
    monkeypatch.setenv("PROCESS_HEALTH_PID", "5678")
    second = Reporter()
    second.set("blocked", "failed", "parser", "unavailable", "restart")
    assert json.loads((tmp_path / "pdf-parser-1234.json").read_text())["state"] == "healthy"
    assert json.loads((tmp_path / "pdf-parser-5678.json").read_text())["state"] == "blocked"
