#!/usr/bin/env python3
"""Render a PDF page by page and parse it with WeVisDoc-4B via MLX-VLM."""

from __future__ import annotations

import argparse
import contextlib
import json
import os
import subprocess
import sys
import tempfile
import time
from pathlib import Path


SCRIPT_PATH = Path(__file__).resolve()
CHENWEB_ROOT = SCRIPT_PATH.parents[2]
WORKSPACE_ROOT = SCRIPT_PATH.parents[3]
PROMPT_PATH = CHENWEB_ROOT / "prompts" / "prompt-wevisdoc-page-to-markdown-v1.md"
DEFAULT_MLX_PYTHON = WORKSPACE_ROOT / "ThirdParty" / "mlx-vlm" / ".venv" / "bin" / "python"
MODEL_ID = "Tencent/WeVisDoc-4B"


def _render_pdf(pdf_path: Path, image_dir: Path, dpi: int) -> list[Path]:
    try:
        import fitz
    except ImportError as exc:
        raise RuntimeError(
            "PyMuPDF is required. Run this script with "
            "ChenWeb/python/pdf-parser/.venv/bin/python."
        ) from exc

    try:
        document = fitz.open(pdf_path)
    except Exception as exc:
        raise RuntimeError(f"Could not open PDF {pdf_path}: {exc}") from exc

    if document.needs_pass:
        raise RuntimeError(f"PDF is password-protected: {pdf_path}")
    if len(document) == 0:
        raise RuntimeError(f"PDF has no pages: {pdf_path}")

    scale = dpi / 72
    matrix = fitz.Matrix(scale, scale)
    image_paths = []
    for page_index, page in enumerate(document):
        image_path = image_dir / f"page-{page_index + 1:04d}.png"
        page.get_pixmap(matrix=matrix, alpha=False).save(image_path)
        image_paths.append(image_path)
    document.close()
    return image_paths


def _mlx_worker(request_path: Path) -> int:
    from mlx_vlm import generate, load

    request = json.loads(request_path.read_text(encoding="utf-8"))
    prompt = request["prompt"].strip()
    chat_prompt = (
        "<|im_start|>user\n"
        "<|vision_start|><|image_pad|><|vision_end|>\n"
        f"{prompt}<|im_end|>\n"
        "<|im_start|>assistant\n"
    )

    # The HF checkpoint has a tied lm_head tensor that MLX-VLM does not consume.
    with contextlib.redirect_stdout(sys.stderr):
        load_started = time.perf_counter()
        model, processor = load(MODEL_ID, strict=False)
        model_load_seconds = time.perf_counter() - load_started

        pages = []
        for page_number, image_path in enumerate(request["images"], start=1):
            started = time.perf_counter()
            result = generate(
                model,
                processor,
                chat_prompt,
                image=[image_path],
                max_tokens=request["max_tokens"],
                temperature=0.0,
                verbose=False,
            )
            pages.append(
                {
                    "page_number": page_number,
                    "text": result.text if hasattr(result, "text") else str(result),
                    "seconds": time.perf_counter() - started,
                }
            )

    print(
        json.dumps(
            {"model_load_seconds": model_load_seconds, "pages": pages},
            ensure_ascii=False,
        ),
        file=sys.__stdout__,
    )
    return 0


def _run_cli() -> int:
    parser = argparse.ArgumentParser(
        description="Parse each page of a PDF with WeVisDoc-4B on Apple silicon."
    )
    parser.add_argument("pdf", type=Path, help="PDF file to parse")
    parser.add_argument("-o", "--output", type=Path, help="Markdown output path")
    parser.add_argument(
        "--dpi", type=int, default=200, help="PDF rendering resolution (default: 200)"
    )
    parser.add_argument(
        "--max-tokens",
        type=int,
        default=5000,
        help="Maximum generated tokens per page (default: 5000)",
    )
    parser.add_argument(
        "--mlx-python",
        type=Path,
        default=DEFAULT_MLX_PYTHON,
        help=f"Python executable with mlx-vlm installed (default: {DEFAULT_MLX_PYTHON})",
    )
    args = parser.parse_args()

    pdf_path = args.pdf.expanduser().resolve()
    if not pdf_path.is_file():
        raise RuntimeError(f"PDF file not found: {pdf_path}")
    if pdf_path.suffix.lower() != ".pdf":
        raise RuntimeError(f"Input must be a PDF file: {pdf_path}")
    if args.dpi < 36:
        raise RuntimeError("--dpi must be at least 36")
    if args.max_tokens < 1:
        raise RuntimeError("--max-tokens must be positive")
    if not args.mlx_python.is_file() or not os.access(args.mlx_python, os.X_OK):
        raise RuntimeError(
            f"MLX-VLM Python not found at {args.mlx_python}. "
            "Use --mlx-python to select its virtual-environment interpreter."
        )

    output_path = args.output.expanduser().resolve() if args.output else pdf_path.with_name(
        f"{pdf_path.stem}.wevisdoc.md"
    )
    prompt = PROMPT_PATH.read_text(encoding="utf-8")

    with tempfile.TemporaryDirectory(prefix="wevisdoc-") as temp_name:
        temp_dir = Path(temp_name)
        image_dir = temp_dir / "pages"
        image_dir.mkdir()
        image_paths = _render_pdf(pdf_path, image_dir, args.dpi)
        request_path = temp_dir / "request.json"
        request_path.write_text(
            json.dumps(
                {
                    "images": [str(path) for path in image_paths],
                    "prompt": prompt,
                    "max_tokens": args.max_tokens,
                },
                ensure_ascii=False,
            ),
            encoding="utf-8",
        )

        completed = subprocess.run(
            [str(args.mlx_python), str(SCRIPT_PATH), "--_mlx_worker", str(request_path)],
            capture_output=True,
            text=True,
            check=False,
        )
        if completed.returncode != 0:
            detail = completed.stderr.strip() or completed.stdout.strip()
            raise RuntimeError(f"MLX-VLM failed (exit {completed.returncode}):\n{detail}")
        try:
            result = json.loads(completed.stdout)
        except json.JSONDecodeError as exc:
            raise RuntimeError(
                "MLX-VLM returned invalid output. Check its environment and logs:\n"
                f"{completed.stderr.strip()}"
            ) from exc

    markdown_pages = []
    for page in result["pages"]:
        markdown_pages.append(f"<!-- Page {page['page_number']} -->\n\n{page['text'].strip()}")

    output_path.parent.mkdir(parents=True, exist_ok=True)
    output_path.write_text("\n\n---\n\n".join(markdown_pages) + "\n", encoding="utf-8")

    print(f"Model load: {result['model_load_seconds']:.1f}s", file=sys.stderr)
    for page in result["pages"]:
        print(f"Page {page['page_number']}: {page['seconds']:.1f}s", file=sys.stderr)
    total_inference = sum(page["seconds"] for page in result["pages"])
    print(
        f"Saved {len(markdown_pages)} pages to {output_path} "
        f"({total_inference:.1f}s inference total)",
        file=sys.stderr,
    )
    return 0


def main() -> int:
    if len(sys.argv) > 1 and sys.argv[1] == "--_mlx_worker":
        if len(sys.argv) != 3:
            print("Worker mode expects one request JSON path.", file=sys.stderr)
            return 2
        return _mlx_worker(Path(sys.argv[2]))

    try:
        return _run_cli()
    except (OSError, RuntimeError, ValueError) as exc:
        print(f"Error: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
