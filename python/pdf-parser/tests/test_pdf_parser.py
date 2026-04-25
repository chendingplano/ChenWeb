import time
from unittest.mock import MagicMock, patch
from pathlib import Path
import os

import pdf_parser as pdf_parser_module
from pdf_parser import (
    get_backend,
    make_throttled_progress,
    PARSER_REGISTRY,
    should_drop_stage_event,
    _process_record,
    _resolve_input_file,
)


class TestParserRegistry:
    def test_opendata_registered(self):
        assert "opendata" in PARSER_REGISTRY

    def test_paddleocr_registered(self):
        assert "paddleocr" in PARSER_REGISTRY

    def test_docling_registered(self):
        assert "docling" in PARSER_REGISTRY

    def test_unknown_parser_raises(self):
        try:
            get_backend("nonexistent", {})
            assert False, "Should have raised"
        except ValueError as e:
            assert "nonexistent" in str(e)

    def test_get_backend_caches_instance(self):
        cache = {}
        with patch.dict(PARSER_REGISTRY, {"opendata": MagicMock}):
            b1 = get_backend("opendata", cache)
            b2 = get_backend("opendata", cache)
            assert b1 is b2


class TestThrottledProgress:
    def test_first_call_always_fires(self):
        db_calls = []

        def db_update(ms_used, pct):
            db_calls.append((ms_used, pct))

        throttled = make_throttled_progress(db_update, min_interval=3.0)
        throttled(1, 10)
        assert len(db_calls) == 1

    def test_suppresses_rapid_calls(self):
        db_calls = []

        def db_update(ms_used, pct):
            db_calls.append(pct)

        throttled = make_throttled_progress(db_update, min_interval=3.0)
        throttled(1, 10)  # fires (first call)
        throttled(2, 10)  # suppressed (< 3s)
        throttled(3, 10)  # suppressed (< 3s)
        assert len(db_calls) == 1

    def test_fires_after_interval(self):
        db_calls = []

        def db_update(ms_used, pct):
            db_calls.append(pct)

        throttled = make_throttled_progress(db_update, min_interval=0.0)
        throttled(1, 10)
        throttled(2, 10)
        assert len(db_calls) == 2


class TestJetStreamErrorHandling:
    def test_drop_invalid_record_id_event(self):
        assert should_drop_stage_event(ValueError("missing/invalid record_id"))

    def test_drop_missing_db_record_event(self):
        assert should_drop_stage_event(ValueError("kb.inputs record not found id=25"))

    def test_retry_other_errors(self):
        assert not should_drop_stage_event(RuntimeError("postgres temporarily unavailable"))


class TestRepoFileProcessing:
    def test_resolve_input_file_treats_existing_file_name_as_repo_file(self, tmp_path):
        repo_pdf = tmp_path / "SemOS" / "Artifacts" / "0" / "76" / "stdGk_517071.pdf"
        repo_pdf.parent.mkdir(parents=True)
        repo_pdf.write_bytes(b"%PDF-1.4\n")

        source_path, from_staging = _resolve_input_file(
            {
                "id": 76,
                "name": "stdGk_517071.pdf",
                "file_name": str(repo_pdf),
                "result_filename": "",
            },
            staging_dir=str(tmp_path / "staging"),
            repo_dirs=[str(tmp_path / "SemOS")],
        )

        assert source_path == str(repo_pdf)
        assert from_staging is False

    def test_process_record_outputs_next_to_repo_pdf_without_moving_it(self, tmp_path, monkeypatch):
        repo_pdf = tmp_path / "SemOS" / "Artifacts" / "0" / "76" / "stdGk_517071.pdf"
        repo_pdf.parent.mkdir(parents=True)
        repo_pdf.write_bytes(b"%PDF-1.4\n")

        calls = {}

        class FakeBackend:
            def init(self):
                pass

            def parse(self, pdf_path, output_dir, on_progress):
                calls["pdf_path"] = pdf_path
                calls["output_dir"] = output_dir
                on_progress(1, 1)
                return {"total_pages": 1, "pages": [{"text": "ok"}], "engine": "opendata"}

        monkeypatch.setitem(pdf_parser_module.PARSER_REGISTRY, "opendata", FakeBackend)
        monkeypatch.setattr(pdf_parser_module, "find_duplicate_processed_record", lambda *args: None)
        monkeypatch.setattr(pdf_parser_module, "record_parsing", lambda *args: args[2])
        monkeypatch.setattr(pdf_parser_module, "record_parsed_success", lambda *args: args[2])
        monkeypatch.setattr(
            pdf_parser_module,
            "copy_file",
            lambda *args: (_ for _ in ()).throw(AssertionError("repo file should not be copied")),
        )
        monkeypatch.setattr(os, "remove", lambda *args: (_ for _ in ()).throw(AssertionError("repo file should not be removed")))

        result = _process_record(
            conn=object(),
            rec={
                "id": 76,
                "name": "stdGk_517071.pdf",
                "file_name": str(repo_pdf),
                "backup_filename": "",
                "result_filename": "",
                "parser_name": "opendata",
                "status": "[]",
                "md5": "known-md5",
            },
            backend_cache={},
            repo_dirs=[str(tmp_path / "SemOS")],
            backup_dir=str(tmp_path / "backup"),
            staging_dir=str(tmp_path / "staging"),
            default_parser="opendata",
        )

        assert calls["pdf_path"] == str(repo_pdf)
        assert calls["output_dir"] == str(repo_pdf.parent)
        assert repo_pdf.exists()
        assert result["status"] == "success"
        assert Path(result["result_filename"]).parent == repo_pdf.parent
