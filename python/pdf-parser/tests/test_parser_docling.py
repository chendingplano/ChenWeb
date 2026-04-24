import json
import os
import tempfile
from unittest.mock import patch

from parser_docling import DoclingParser


class TestDoclingParserInit:
    def test_init_succeeds_when_docling_installed(self):
        parser = DoclingParser()
        with patch("importlib.import_module", return_value=object()):
            parser.init()

    def test_init_raises_if_docling_missing(self):
        parser = DoclingParser()
        with patch("importlib.import_module", side_effect=ImportError("missing")):
            try:
                parser.init()
                assert False, "Should have raised"
            except RuntimeError as e:
                assert "docling" in str(e).lower()


class TestDoclingParserParse:
    def test_parse_writes_json_and_returns_summary(self):
        parser = DoclingParser()
        parser._initialized = True

        fake_dict = {
            "texts": [{"text": "hello"}],
            "pages": {
                "1": {"size": {"width": 100, "height": 200}},
                "2": {"size": {"width": 100, "height": 200}},
            },
        }

        class FakeDocument:
            def export_to_dict(self):
                return fake_dict

        class FakeResult:
            document = FakeDocument()

        class FakeConverter:
            def convert(self, pdf_path):
                assert pdf_path.endswith("test.pdf")
                return FakeResult()

        progress_calls = []

        with tempfile.TemporaryDirectory() as out_dir:
            pdf_path = os.path.join(out_dir, "test.pdf")
            with open(pdf_path, "wb") as f:
                f.write(b"%PDF-1.4\n")

            with patch.object(parser, "_create_converter", return_value=FakeConverter()):
                result = parser.parse(
                    pdf_path,
                    out_dir,
                    lambda p, t: progress_calls.append((p, t)),
                )

            output_path = os.path.join(out_dir, "test.docling.json")
            assert os.path.isfile(output_path)
            with open(output_path, "r", encoding="utf-8") as f:
                written = json.load(f)

            assert written == fake_dict
            assert result["engine"] == "docling"
            assert result["total_pages"] == 2
            assert result["pages"] == [
                {"page_number": 1, "items": []},
                {"page_number": 2, "items": []},
            ]
            assert result["raw"] == fake_dict
            assert progress_calls[0] == (0, 1)
            assert progress_calls[-1] == (1, 1)
