import json
import os
import tempfile
from unittest.mock import patch, MagicMock
from parser_opendata import OpenDataParser


class TestOpenDataParserInit:
    def test_init_finds_java(self):
        with patch("shutil.which", return_value="/usr/bin/java"):
            parser = OpenDataParser()
            parser._jar_path = "/fake/jar.jar"
            parser.init()
            # Should not raise

    def test_init_raises_if_no_java(self):
        with patch("shutil.which", return_value=None):
            parser = OpenDataParser()
            try:
                parser.init()
                assert False, "Should have raised"
            except RuntimeError as e:
                assert "java" in str(e).lower()


class TestOpenDataParserParse:
    def test_parse_calls_subprocess_and_reads_json(self):
        parser = OpenDataParser()
        parser._jar_path = "/fake/jar.jar"
        parser._initialized = True

        with tempfile.TemporaryDirectory() as out_dir:
            pdf_path = "/fake/test.pdf"

            # Create fake output that opendataloader would produce
            output_json = {
                "file name": "test.pdf",
                "number of pages": 2,
                "kids": [
                    {"type": "text", "id": 1, "page number": 1},
                    {"type": "text", "id": 2, "page number": 2},
                ],
            }
            json_path = os.path.join(out_dir, "test.json")
            with open(json_path, "w") as f:
                json.dump(output_json, f)

            progress_calls = []

            mock_proc = MagicMock()
            mock_proc.poll.return_value = 0  # already finished, loop body never runs
            mock_proc.wait.return_value = 0
            mock_proc.stdout.read.return_value = ""

            with patch("subprocess.Popen", return_value=mock_proc):
                result = parser.parse(pdf_path, out_dir, lambda p, t: progress_calls.append((p, t)))

            assert result["engine"] == "opendata"
            assert result["total_pages"] == 2
            assert progress_calls[0] == (0, 1)
            assert progress_calls[-1] == (1, 1)

    def test_parse_raises_on_subprocess_failure(self):
        parser = OpenDataParser()
        parser._jar_path = "/fake/jar.jar"
        parser._initialized = True

        with tempfile.TemporaryDirectory() as out_dir:
            mock_proc = MagicMock()
            mock_proc.poll.return_value = 1
            mock_proc.wait.return_value = 1
            mock_proc.stdout.read.return_value = "boom"

            with patch("subprocess.Popen", return_value=mock_proc):
                try:
                    parser.parse("/fake/test.pdf", out_dir, lambda p, t: None)
                    assert False, "Should have raised"
                except RuntimeError as e:
                    assert "boom" in str(e)

    def test_parse_terminates_subprocess_on_abort(self):
        """When on_progress raises (abort requested), the subprocess is
        terminated and the exception propagates instead of the parse
        silently running to completion."""

        class Aborted(Exception):
            pass

        parser = OpenDataParser()
        parser._jar_path = "/fake/jar.jar"
        parser._initialized = True

        with tempfile.TemporaryDirectory() as out_dir:
            mock_proc = MagicMock()
            # Still running on the loop's first check, so the loop body
            # executes and calls on_progress, which raises.
            mock_proc.poll.return_value = None
            mock_proc.wait.return_value = -15
            mock_proc.stdout.read.return_value = ""

            calls = {"n": 0}

            def on_progress(p, t):
                calls["n"] += 1
                if calls["n"] == 2:  # first call is the initial on_progress(0, 1)
                    raise Aborted()

            with patch("subprocess.Popen", return_value=mock_proc):
                try:
                    parser.parse("/fake/test.pdf", out_dir, on_progress)
                    assert False, "Should have raised"
                except Aborted:
                    pass

            mock_proc.terminate.assert_called_once()
