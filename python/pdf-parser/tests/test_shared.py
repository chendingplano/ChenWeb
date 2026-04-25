import json
import os
import tempfile
from shared import decode_status, has_operation, upsert_status, replace_status_entry
from shared import file_md5, copy_file, unique_path, resolve_source_path, choose_repo_dir
from shared import fetch_candidates, fetch_record_by_id, find_duplicate_processed_record, has_md5_record, record_duplicated


class TestDecodeStatus:
    def test_empty_string(self):
        assert decode_status("") == []

    def test_null_string(self):
        assert decode_status("null") == []

    def test_none(self):
        assert decode_status(None) == []

    def test_valid_array(self):
        raw = json.dumps([{"operation": "parsing"}])
        assert decode_status(raw) == [{"operation": "parsing"}]

    def test_single_dict_wrapped_in_list(self):
        raw = json.dumps({"operation": "parsing"})
        assert decode_status(raw) == [{"operation": "parsing"}]

    def test_invalid_json(self):
        assert decode_status("not json") == []


class TestHasOperation:
    def test_no_operations(self):
        assert has_operation("[]", "parsing") is False

    def test_has_parsing(self):
        raw = json.dumps([{"operation": "parsing"}])
        assert has_operation(raw, "parsing") is True

    def test_has_parsed(self):
        raw = json.dumps([{"operation": "parsed"}])
        assert has_operation(raw, "parsed") is True

    def test_multiple_operations_check(self):
        raw = json.dumps([{"operation": "parsed"}])
        assert has_operation(raw, "parsing", "parsed") is True

    def test_case_insensitive(self):
        raw = json.dumps([{"operation": "PARSING"}])
        assert has_operation(raw, "parsing") is True


class TestUpsertStatus:
    def test_append_to_empty(self):
        result = json.loads(upsert_status("[]", "parsing", {"progress": "0%"}))
        assert len(result) == 1
        assert result[0]["operation"] == "parsing"
        assert result[0]["progress"] == "0%"

    def test_update_existing(self):
        raw = json.dumps([{"operation": "parsing", "progress": "0%"}])
        result = json.loads(upsert_status(raw, "parsing", {"progress": "50%"}))
        assert len(result) == 1
        assert result[0]["progress"] == "50%"

    def test_preserves_other_entries(self):
        raw = json.dumps([{"operation": "other"}, {"operation": "parsing", "progress": "0%"}])
        result = json.loads(upsert_status(raw, "parsing", {"progress": "50%"}))
        assert len(result) == 2
        assert result[0]["operation"] == "other"
        assert result[1]["progress"] == "50%"


class TestReplaceStatusEntry:
    def test_replace_parsing_with_parsed(self):
        raw = json.dumps([{"operation": "parsing", "progress": "80%"}])
        new_entry = {"operation": "parsed", "proc-status": "success", "ms-used": 5000}
        result = json.loads(replace_status_entry(raw, "parsing", new_entry))
        assert len(result) == 1
        assert result[0]["operation"] == "parsed"
        assert result[0]["proc-status"] == "success"
        assert "progress" not in result[0]

    def test_append_when_old_op_not_found(self):
        raw = json.dumps([{"operation": "other"}])
        new_entry = {"operation": "parsed", "proc-status": "failed"}
        result = json.loads(replace_status_entry(raw, "parsing", new_entry))
        assert len(result) == 2
        assert result[1]["operation"] == "parsed"

    def test_preserves_other_entries(self):
        raw = json.dumps([{"operation": "analyze"}, {"operation": "parsing"}])
        new_entry = {"operation": "parsed", "proc-status": "success"}
        result = json.loads(replace_status_entry(raw, "parsing", new_entry))
        assert len(result) == 2
        assert result[0]["operation"] == "analyze"
        assert result[1]["operation"] == "parsed"


class TestFileMd5:
    def test_computes_md5(self):
        with tempfile.NamedTemporaryFile(delete=False, suffix=".pdf") as f:
            f.write(b"hello world")
            f.flush()
            path = f.name
        try:
            result = file_md5(path)
            assert result == "5eb63bbbe01eeed093cb22bb8f5acdc3"
        finally:
            os.unlink(path)


class TestCopyFile:
    def test_copies_file(self):
        with tempfile.TemporaryDirectory() as d:
            src = os.path.join(d, "src.txt")
            dst = os.path.join(d, "sub", "dst.txt")
            with open(src, "w") as f:
                f.write("content")
            copy_file(src, dst)
            assert os.path.exists(dst)
            with open(dst) as f:
                assert f.read() == "content"


class TestUniquePath:
    def test_returns_original_when_no_conflict(self):
        with tempfile.TemporaryDirectory() as d:
            result = unique_path(d, "file.pdf")
            assert result == os.path.join(d, "file.pdf")

    def test_appends_suffix_on_conflict(self):
        with tempfile.TemporaryDirectory() as d:
            open(os.path.join(d, "file.pdf"), "w").close()
            result = unique_path(d, "file.pdf")
            assert result == os.path.join(d, "file_1.pdf")


class TestResolveSourcePath:
    def test_absolute_path_returned_as_is(self):
        assert resolve_source_path("/abs/path.pdf", "/staging") == "/abs/path.pdf"

    def test_relative_path_joined_with_staging(self):
        assert resolve_source_path("file.pdf", "/staging") == "/staging/file.pdf"

    def test_empty_returns_empty(self):
        assert resolve_source_path("", "/staging") == ""

    def test_no_staging_returns_filename(self):
        assert resolve_source_path("file.pdf", "") == "file.pdf"


class TestChooseRepoDir:
    def test_picks_emptiest_dir(self):
        with tempfile.TemporaryDirectory() as d:
            dir_a = os.path.join(d, "a")
            dir_b = os.path.join(d, "b")
            os.makedirs(dir_a)
            os.makedirs(dir_b)
            # Put a file in dir_a so dir_b is lighter
            with open(os.path.join(dir_a, "big.bin"), "wb") as f:
                f.write(b"x" * 1000)
            result = choose_repo_dir([dir_a, dir_b])
            assert result == dir_b


class _FakeCursor:
    def __init__(self, owner):
        self.owner = owner

    def __enter__(self):
        return self

    def __exit__(self, exc_type, exc, tb):
        return False

    def execute(self, sql, params):
        self.owner.executed_sql = sql
        self.owner.executed_params = params

    def fetchall(self):
        return list(self.owner.rows)

    def fetchone(self):
        return self.owner.row


class _FakeConn:
    def __init__(self):
        self.executed_sql = ""
        self.executed_params = ()
        self.commit_called = False
        self.rows = []
        self.row = None

    def cursor(self):
        return _FakeCursor(self)

    def commit(self):
        self.commit_called = True


class TestRecordDuplicated:
    def test_record_duplicated_includes_start_time(self):
        conn = _FakeConn()
        raw = json.dumps([{"operation": "parsing", "progress": "50%"}])
        updated = record_duplicated(conn, 21, raw, 20, "20260410 16:24:29")
        entries = json.loads(updated)

        assert conn.commit_called is True
        assert len(entries) == 1
        assert entries[0]["operation"] == "parsed"
        assert entries[0]["proc-status"] == "duplicated"
        assert entries[0]["dup_rcd_id"] == 20
        assert entries[0]["start_time"] == "20260410 16:24:29"


class TestReadQueriesCloseTransactions:
    def test_fetch_candidates_commits_after_select(self):
        conn = _FakeConn()
        conn.rows = [
            (11, "doc.pdf", "/tmp/doc.pdf", "docling", "[]", "md5", "", ""),
        ]

        records = fetch_candidates(conn, 25)

        assert conn.commit_called is True
        assert records[0]["id"] == 11

    def test_fetch_record_by_id_commits_after_select(self):
        conn = _FakeConn()
        conn.row = (12, "doc.pdf", "/tmp/doc.pdf", "docling", "[]", "md5", "", "")

        record = fetch_record_by_id(conn, 12)

        assert conn.commit_called is True
        assert record["id"] == 12

    def test_has_md5_record_commits_after_select(self):
        conn = _FakeConn()
        conn.row = (1,)

        assert has_md5_record(conn, "abc") is True
        assert conn.commit_called is True

    def test_find_duplicate_processed_record_commits_after_select(self):
        conn = _FakeConn()
        conn.row = (99,)

        assert find_duplicate_processed_record(conn, 12, "abc") == 99
        assert conn.commit_called is True
