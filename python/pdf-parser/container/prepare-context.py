#!/usr/bin/env python3
"""Stage the deployed worker, replacing only its MinerU integration."""

import argparse
import shutil
from pathlib import Path

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("--worker", type=Path, required=True)
parser.add_argument("--mineru", type=Path, required=True)
parser.add_argument("--adapters", type=Path, required=True)
parser.add_argument("--output", type=Path, required=True)
args = parser.parse_args()
worker = args.output / "worker"
mineru = args.output / "mineru"
worker.mkdir(parents=True, exist_ok=True)
mineru.mkdir(parents=True, exist_ok=True)
for path in args.worker.glob("*.py"):
    shutil.copy2(str(path), str(worker / path.name))
for name in ("parser_mineru.py", "mineru_v4_adapter.py", "mineru_table_repair.py"):
    shutil.copy2(str(args.adapters / name), str(worker / name))
# The September worker passed a language argument removed by the MinerU 4 API.
main = worker / "pdf_parser.py"
main.write_text(main.read_text()
    .replace("make_ocr_parser(_ocr_fallback_lang())", "make_ocr_parser()")
    .replace('getattr(backend, "_backend", "") == "pipeline"',
             'getattr(backend, "_tier", "") == "basic"')
    .replace('"ocr" in getattr(backend, "_extra_args", [])',
             'getattr(backend, "_ocr_mode", "") == "ocr"')
    .replace("pipeline OCR", "basic OCR")
    .replace("mineru-pipeline-ocr", "mineru-basic-ocr"))
shutil.copytree(str(args.mineru / "mineru"), str(mineru / "mineru"),
                ignore=shutil.ignore_patterns("__pycache__", "*.pyc"))
for name in ("pyproject.toml", "README.md", "LICENSE.md"):
    shutil.copy2(str(args.mineru / name), str(mineru / name))
shutil.copy2(str(Path(__file__).parent / "Dockerfile"), str(args.output / "Dockerfile"))
