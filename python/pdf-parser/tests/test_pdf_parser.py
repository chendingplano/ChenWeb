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
    _repo_dirs,
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


class TestConfig:
    def test_repo_dirs_use_data_home_dir_directly(self, monkeypatch):
        monkeypatch.setenv("DATA_HOME_DIR", "/Users/cding/Apps/SemOS")
        assert _repo_dirs() == ["/Users/cding/Apps/SemOS"]


class TestThrottledProgress:
    def test_first_call_always_fires(self):
        db_calls = []

        def db_update(ms_used, pct, total_pages):
            db_calls.append((ms_used, pct))

        throttled = make_throttled_progress(db_update, min_interval=3.0)
        throttled(1, 10)
        assert len(db_calls) == 1

    def test_suppresses_rapid_calls(self):
        db_calls = []

        def db_update(ms_used, pct, total_pages):
            db_calls.append(pct)

        throttled = make_throttled_progress(db_update, min_interval=3.0)
        throttled(1, 10)  # fires (first call)
        throttled(2, 10)  # suppressed (< 3s)
        throttled(3, 10)  # suppressed (< 3s)
        assert len(db_calls) == 1

    def test_fires_after_interval(self):
        db_calls = []

        def db_update(ms_used, pct, total_pages):
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
    def test_resolve_input_file_supports_relative_repo_file_name(self, tmp_path):
        repo_root = tmp_path / "SemOS"
        repo_pdf = repo_root / "Artifacts" / "0" / "76" / "stdGk_517071.pdf"
        repo_pdf.parent.mkdir(parents=True)
        repo_pdf.write_bytes(b"%PDF-1.4\n")

        source_path, from_staging = _resolve_input_file(
            {
                "id": 76,
                "name": "stdGk_517071.pdf",
                "file_name": "Artifacts/0/76/stdGk_517071.pdf",
                "result_filename": "",
            },
            staging_dir=str(tmp_path / "staging"),
            repo_dirs=[str(repo_root)],
        )

        assert source_path == str(repo_pdf)
        assert from_staging is False

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
        monkeypatch.setattr(pdf_parser_module, "record_parse_active", lambda *args: args[2])
        monkeypatch.setattr(pdf_parser_module, "record_parsed_success", lambda *args: args[2])
        monkeypatch.setattr(
            pdf_parser_module,
            "copy_file",
            lambda *args: (_ for _ in ()).throw(AssertionError("repo file should not be copied")),
        )
        monkeypatch.setattr(os, "remove", lambda *args: (_ for _ in ()).throw(AssertionError("repo file should not be removed")))
        monkeypatch.setenv("DATA_HOME_DIR", str(tmp_path / "SemOS"))

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
        assert result["result_filename"] == str(Path("Artifacts") / "0" / "76" / "stdGk_517071_opendata.json")

    def test_process_record_persists_relative_paths(self, tmp_path, monkeypatch):
        repo_root = tmp_path / "SemOS"
        repo_pdf = repo_root / "Artifacts" / "0" / "76" / "stdGk_517071.pdf"
        repo_pdf.parent.mkdir(parents=True)
        repo_pdf.write_bytes(b"%PDF-1.4\n")

        persisted = {}

        class FakeBackend:
            def init(self):
                pass

            def parse(self, pdf_path, output_dir, on_progress):
                on_progress(1, 1)
                return {"total_pages": 1, "pages": [{"text": "ok"}], "engine": "opendata"}

        def fake_record_parsed_success(conn, rec_id, raw_status, start_time, ms_used, result_filename, file_name, backup_filename, parser_name, num_pages):
            persisted["result_filename"] = result_filename
            persisted["file_name"] = file_name
            persisted["backup_filename"] = backup_filename
            return raw_status

        monkeypatch.setitem(pdf_parser_module.PARSER_REGISTRY, "opendata", FakeBackend)
        monkeypatch.setattr(pdf_parser_module, "find_duplicate_processed_record", lambda *args: None)
        monkeypatch.setattr(pdf_parser_module, "record_parse_active", lambda *args: args[2])
        monkeypatch.setattr(pdf_parser_module, "record_parsed_success", fake_record_parsed_success)

        old_home = os.environ.get("DATA_HOME_DIR")
        os.environ["DATA_HOME_DIR"] = str(repo_root)
        try:
            result = _process_record(
                conn=object(),
                rec={
                    "id": 76,
                    "name": "stdGk_517071.pdf",
                    "file_name": "Artifacts/0/76/stdGk_517071.pdf",
                    "backup_filename": "",
                    "result_filename": "",
                    "parser_name": "opendata",
                    "status": "[]",
                    "md5": "known-md5",
                },
                backend_cache={},
                repo_dirs=[str(repo_root)],
                backup_dir=str(tmp_path / "backup"),
                staging_dir=str(tmp_path / "staging"),
                default_parser="opendata",
            )
        finally:
            if old_home is None:
                os.environ.pop("DATA_HOME_DIR", None)
            else:
                os.environ["DATA_HOME_DIR"] = old_home

        assert persisted["file_name"] == "Artifacts/0/76/stdGk_517071.pdf"
        assert persisted["result_filename"] == "Artifacts/0/76/stdGk_517071_opendata.json"
        assert persisted["backup_filename"] == ""
        assert result["result_filename"] == "Artifacts/0/76/stdGk_517071_opendata.json"

    def test_process_record_persists_relative_backup_path(self, tmp_path, monkeypatch):
        repo_root = tmp_path / "SemOS"
        backup_root = tmp_path / "Backup"
        staging_root = tmp_path / "staging"
        source_pdf = staging_root / "std_20039.pdf"
        source_pdf.parent.mkdir(parents=True)
        source_pdf.write_bytes(b"%PDF-1.4\n")

        persisted = {}

        class FakeBackend:
            def init(self):
                pass

            def parse(self, pdf_path, output_dir, on_progress):
                on_progress(1, 1)
                return {"total_pages": 1, "pages": [{"text": "ok"}], "engine": "opendata"}

        def fake_record_parsed_success(conn, rec_id, raw_status, start_time, ms_used, result_filename, file_name, backup_filename, parser_name, num_pages):
            persisted["result_filename"] = result_filename
            persisted["file_name"] = file_name
            persisted["backup_filename"] = backup_filename
            return raw_status

        monkeypatch.setitem(pdf_parser_module.PARSER_REGISTRY, "opendata", FakeBackend)
        monkeypatch.setattr(pdf_parser_module, "find_duplicate_processed_record", lambda *args: None)
        monkeypatch.setattr(pdf_parser_module, "record_parse_active", lambda *args: args[2])
        monkeypatch.setattr(pdf_parser_module, "record_parsed_success", fake_record_parsed_success)
        monkeypatch.setenv("DATA_HOME_DIR", str(repo_root))
        monkeypatch.setenv("DATA_BACKUP_DIR", str(backup_root))

        _process_record(
            conn=object(),
            rec={
                "id": 87,
                "name": "std_20039.pdf",
                "file_name": str(source_pdf),
                "backup_filename": "",
                "result_filename": "",
                "parser_name": "opendata",
                "status": "[]",
                "md5": "known-md5",
            },
            backend_cache={},
            repo_dirs=[str(repo_root)],
            backup_dir=str(backup_root),
            staging_dir=str(staging_root),
            default_parser="opendata",
        )

        assert persisted["file_name"] == "Artifacts/0/87/std_20039.pdf"
        assert persisted["result_filename"] == "Artifacts/0/87/std_20039_opendata.json"
        assert persisted["backup_filename"] == "Backup/std_20039.pdf"
