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

            with patch("subprocess.run") as mock_run:
                mock_run.return_value = MagicMock(returncode=0)
                result = parser.parse(pdf_path, out_dir, lambda p, t: progress_calls.append((p, t)))

            assert result["engine"] == "opendata"
            assert result["total_pages"] == 2
            assert progress_calls[0] == (0, 1)
            assert progress_calls[-1] == (1, 1)

    def test_parse_raises_on_subprocess_failure(self):
        import subprocess

        parser = OpenDataParser()
        parser._jar_path = "/fake/jar.jar"
        parser._initialized = True

        with tempfile.TemporaryDirectory() as out_dir:
            with patch("subprocess.run", side_effect=subprocess.CalledProcessError(1, "java")):
                try:
                    parser.parse("/fake/test.pdf", out_dir, lambda p, t: None)
                    assert False, "Should have raised"
                except subprocess.CalledProcessError:
                    pass
