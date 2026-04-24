"""PaddleOCR backend for the unified PDF parser service.

Ported from ThirdParty/paddleocr/pdf_parser_service.py.
All PaddleOCR-specific imports are deferred to init()/parse() so that this
module can be imported even when PaddleOCR is not installed.
"""

import importlib
import json
import logging
import os
import threading
import time
from collections import Counter
from typing import Any

from parser_base import ParserBackend

# ---------------------------------------------------------------------------
# Logging
# ---------------------------------------------------------------------------
log = logging.getLogger(__name__)

# ---------------------------------------------------------------------------
# Per-block timing accumulator (populated by VLM sub-step hooks, consumed by
# _timed_glpr).  threading.local so that future parallel workers stay isolated.
# ---------------------------------------------------------------------------
_timing_state = threading.local()

_QUERY_LABEL_MAP: dict[str, str] = {
    "table recognition:": "table",
    "formula recognition:": "formula",
    "chart recognition:": "chart",
    "seal recognition:": "seal",
    "spotting:": "spotting",
}


def _query_to_label(query: str) -> str:
    """Map a VLM query prefix to a short block-type label for timing logs."""
    return _QUERY_LABEL_MAP.get(query.strip().lower(), "ocr")


def _log_block_type_summary(blocks: list) -> None:
    """Emit one [TIMING] BlockType log line per label, sorted by total ms descending."""
    if not blocks:
        return
    totals: dict[str, dict] = {}
    for b in blocks:
        try:
            label = b["label"]
            if label not in totals:
                totals[label] = {"count": 0, "total_ms": 0.0, "total_px": 0, "total_chars": 0}
            t = totals[label]
            t["count"] += 1
            t["total_ms"] += b["pre_ms"] + b["xfer_ms"] + b["gen_ms"] + b["post_ms"]
            t["total_px"] += b["pixels"]
            t["total_chars"] += b["chars"]
        except (KeyError, TypeError):
            pass
    for label, t in sorted(totals.items(), key=lambda x: -x[1]["total_ms"]):
        n = t["count"]
        avg_ms = t["total_ms"] / n
        avg_px_k = t["total_px"] / n / 1000
        avg_chars = t["total_chars"] // n
        log.info(
            "[TIMING] BlockType %-22s %2d %s  avg=%5.0fms  avg_px=%5.1fK  avg_chars=%4d  total=%7.0fms",
            label + ":",
            n,
            "blocks" if n != 1 else "block ",
            avg_ms,
            avg_px_k,
            avg_chars,
            t["total_ms"],
        )


# ---------------------------------------------------------------------------
# Configuration helpers
# ---------------------------------------------------------------------------

def _env(key: str, default: str = "") -> str:
    return os.environ.get(key, default).strip()


def _use_vl() -> bool:
    return _env("PDF_USE_VL", "true").lower() == "true"


def _use_mps() -> bool:
    return _env("PDF_MPS", "false").lower() == "true"


def _use_timing() -> bool:
    return _env("PDF_TIMING", "false").lower() == "true"


def _use_quantization() -> bool:
    return _env("PDF_QUANTIZE_ENABLED", "false").lower() == "true"


def _quantize_bits() -> int:
    """Return 4 or 8. Any value other than '4' is treated as 8."""
    return 4 if _env("PDF_QUANTIZE_BITS", "8").strip() == "4" else 8


def _vlm_ocr_max_pixels() -> int:
    """Cap on min_pixels used for OCR blocks. Default 50176 (224²).
    Floor of 12544 (112²) prevents values too small to read text."""
    try:
        return max(12544, int(_env("PDF_VLM_OCR_MAX_PIXELS", "50176")))
    except ValueError:
        return 50176


# ---------------------------------------------------------------------------
# Quantization
# ---------------------------------------------------------------------------

def _enable_quantization(ocr_engine) -> None:
    """Quantize VLM model weights using torchao weight-only quantization.

    Must be called BEFORE _enable_mps_acceleration: torchao quantize_() operates
    on CPU tensors; moving the quantized model to MPS afterward is safe.

    PDF_QUANTIZE_BITS=8  → Int8WeightOnlyConfig  (recommended; 2× memory savings,
                           reliable on Apple Silicon CPU via NEON)
    PDF_QUANTIZE_BITS=4  → Int4WeightOnlyConfig  (4× memory savings; torchao INT4
                           GEMM is CUDA-optimized — verify timing on CPU before
                           relying on this)

    On any failure (import error, quantize_() raises), logs a warning and leaves
    the model unchanged — no crash.
    """
    try:
        quant_mod = importlib.import_module("torchao.quantization")
        Int4WeightOnlyConfig = getattr(quant_mod, "Int4WeightOnlyConfig")
        Int8WeightOnlyConfig = getattr(quant_mod, "Int8WeightOnlyConfig")
        quantize_ = getattr(quant_mod, "quantize_")
    except (ImportError, AttributeError):
        log.warning("[Quant] torchao not installed — quantization skipped")
        return

    outer = ocr_engine.paddlex_pipeline
    pipeline = getattr(outer, "_pipeline", outer)

    if not hasattr(pipeline, "vl_rec_model"):
        log.warning("[Quant] vl_rec_model not found — skipping")
        return

    vl_model = pipeline.vl_rec_model

    if getattr(vl_model, "_quantization_enabled", False):
        log.warning("[Quant] already enabled — skipping")
        return

    bits = _quantize_bits()
    config = Int8WeightOnlyConfig() if bits == 8 else Int4WeightOnlyConfig()
    label = f"INT{bits}"

    try:
        quantize_(vl_model.infer, config)
    except Exception as exc:
        log.warning("[Quant] quantize_() failed (%s): %s — model stays FP32", label, exc)
        return

    vl_model._quantization_enabled = True
    log.info("[Quant] model quantized to %s weight-only", label)


# ---------------------------------------------------------------------------
# OCR timing infrastructure
# ---------------------------------------------------------------------------

class _TimedModelWrapper:
    """Wraps a callable model to log wall-clock time and result count.

    Replaces the model attribute on the pipeline object so that Python's
    normal special-method dispatch (which bypasses instance __call__) is
    avoided.  Attribute access is delegated to the wrapped model so that
    callers accessing e.g. .batch_sampler still work.
    """

    def __init__(self, model, label: str) -> None:
        self._model = model
        self._label = label

    def __call__(self, *args, **kwargs):
        t0 = time.perf_counter()
        results = list(self._model(*args, **kwargs))
        ms = (time.perf_counter() - t0) * 1000
        if results and isinstance(results[0], dict) and "boxes" in results[0]:
            n = sum(len(r.get("boxes", [])) for r in results)
            log.info("[TIMING] %s: %.0fms, boxes=%d", self._label, ms, n)
        else:
            log.info("[TIMING] %s: %.0fms", self._label, ms)
        return results

    def __getattr__(self, name: str):
        return getattr(self._model, name)


def _install_timing_hooks(ocr_engine) -> None:
    """Monkey-patch key pipeline methods to emit [TIMING] log lines.

    Intended to be called only when PDF_TIMING=true.  All patches are applied to the
    live pipeline *instance* — the class and installed paddlex files are
    not modified.

    Log output per page (example):
        [TIMING] PageRender:   38ms
        [TIMING] PNGSave:      22ms
        [TIMING] ArrayBuild:    2ms
        [TIMING] DocPreproc:   12ms
        [TIMING] LayoutDet:   430ms, boxes=14
        [TIMING] Blocks: {'text': 9, 'table': 1, 'formula': 3, 'title': 1}
        [TIMING] Block label=ocr            pixels=142080  pre=   11ms xfer=   1ms gen= 2841ms post=   1ms chars=  183
        [TIMING] Block label=table          pixels=318400  pre=   18ms xfer=   2ms gen= 4120ms post=   2ms chars=  612
        [TIMING] BlockType ocr:             9 blocks  avg= 2950ms  avg_px= 120.0K  avg_chars= 150  total= 26550ms
        [TIMING] BlockType table:           1 block   avg= 4120ms  avg_px= 318.4K  avg_chars= 612  total=  4120ms
        [TIMING] Phases: vlm_loop=30670ms  other=215ms  total=30885ms
        [TIMING] VLM: 14 blocks, 30885ms total, 2206ms avg/block

    Object hierarchy:
        ocr_engine                  PaddleOCRVL
        └─ .paddlex_pipeline        PaddleOCRVL15Pipeline (AutoParallelSimpleInferencePipeline)
               └─ ._pipeline        _PaddleOCRVLPipeline  ← the actual processing object
    The outer pipeline delegates attribute *reads* via __getattr__ to ._pipeline, but
    attribute *writes* land on the outer object.  We must patch ._pipeline directly so
    that self.layout_det_model etc. resolve to our wrappers when the inner pipeline
    executes its predict() method.
    """
    import numpy as np

    outer = ocr_engine.paddlex_pipeline
    # Drill down to the actual inner pipeline (_PaddleOCRVLPipeline).
    # AutoParallelSimpleInferencePipeline stores it as ._pipeline (single-device mode).
    pipeline = getattr(outer, "_pipeline", outer)
    log.info("timing hooks target: %s", type(pipeline).__name__)

    # Guard against double-install. Check the instance __dict__ to avoid MagicMock auto-creation.
    try:
        if "_timing_hooks_installed" in pipeline.__dict__:
            log.warning("timing hooks already installed — skipping")
            return
    except AttributeError:
        pass
    pipeline._timing_hooks_installed = True

    if hasattr(pipeline, "doc_preprocessor_pipeline"):
        pipeline.doc_preprocessor_pipeline = _TimedModelWrapper(
            pipeline.doc_preprocessor_pipeline, "DocPreproc"
        )

    if hasattr(pipeline, "layout_det_model"):
        pipeline.layout_det_model = _TimedModelWrapper(
            pipeline.layout_det_model, "LayoutDet"
        )

    if hasattr(pipeline, "get_layout_parsing_results"):
        _orig_glpr = pipeline.get_layout_parsing_results

        def _timed_glpr(*args, **kwargs):
            det_results = kwargs.get("layout_det_results", [])
            label_counts = Counter(
                b.get("label", "?")
                for r in det_results
                for b in r.get("boxes", [])
            )
            n_blocks = sum(label_counts.values())
            log.info("[TIMING] Blocks: %s", dict(label_counts))

            # Reset per-block accumulator for this page
            _timing_state.blocks = []
            _timing_state.vlm_total_ms = 0.0

            t0 = time.perf_counter()
            result = _orig_glpr(*args, **kwargs)
            total_ms = (time.perf_counter() - t0) * 1000

            # Emit per-block-type summary (populated by _timed_process hooks)
            _log_block_type_summary(_timing_state.blocks)

            # Phases split: vlm_loop is the sum of all _timed_process durations;
            # other covers block prep + result assembly (not separately measurable
            # without modifying pipeline source)
            vlm_ms = _timing_state.vlm_total_ms
            other_ms = total_ms - vlm_ms
            log.info(
                "[TIMING] Phases: vlm_loop=%.0fms  other=%.0fms  total=%.0fms",
                vlm_ms, other_ms, total_ms,
            )

            if n_blocks:
                log.info(
                    "[TIMING] VLM: %d blocks, %.0fms total, %.0fms avg/block",
                    n_blocks, total_ms, total_ms / n_blocks,
                )
            else:
                log.info("[TIMING] VLM: 0 blocks, %.0fms total", total_ms)
            return result

        pipeline.get_layout_parsing_results = _timed_glpr

    # ------------------------------------------------------------------
    # vl_rec_model sub-step hooks
    # ------------------------------------------------------------------
    if not hasattr(pipeline, "vl_rec_model"):
        return

    vl_model = pipeline.vl_rec_model

    # Wrap each sub-step.  Each wrapper stores its elapsed ms into
    # _timing_state.current_block (a dict set up by _timed_process below).
    # We guard with hasattr so the function is silent if PaddleOCR internals change.

    if hasattr(vl_model, "processor") and hasattr(vl_model.processor, "preprocess"):
        _orig_pre = vl_model.processor.preprocess

        def _timed_preprocess(*args, **kwargs):
            t0 = time.perf_counter()
            result = _orig_pre(*args, **kwargs)
            if hasattr(_timing_state, "current_block"):
                _timing_state.current_block["pre_ms"] = (time.perf_counter() - t0) * 1000
            return result

        vl_model.processor.preprocess = _timed_preprocess

    if hasattr(vl_model, "_switch_inputs_to_device"):
        _orig_xfer = vl_model._switch_inputs_to_device

        def _timed_xfer(*args, **kwargs):
            t0 = time.perf_counter()
            result = _orig_xfer(*args, **kwargs)
            if hasattr(_timing_state, "current_block"):
                _timing_state.current_block["xfer_ms"] = (time.perf_counter() - t0) * 1000
            return result

        vl_model._switch_inputs_to_device = _timed_xfer

    if hasattr(vl_model, "infer") and hasattr(vl_model.infer, "generate"):
        _orig_gen = vl_model.infer.generate

        def _timed_generate(*args, **kwargs):
            t0 = time.perf_counter()
            result = _orig_gen(*args, **kwargs)
            if hasattr(_timing_state, "current_block"):
                _timing_state.current_block["gen_ms"] = (time.perf_counter() - t0) * 1000
            return result

        vl_model.infer.generate = _timed_generate

    if hasattr(vl_model, "processor") and hasattr(vl_model.processor, "postprocess"):
        _orig_post = vl_model.processor.postprocess

        def _timed_postprocess(*args, **kwargs):
            t0 = time.perf_counter()
            result = _orig_post(*args, **kwargs)
            if hasattr(_timing_state, "current_block"):
                _timing_state.current_block["post_ms"] = (time.perf_counter() - t0) * 1000
            return result

        vl_model.processor.postprocess = _timed_postprocess

    if hasattr(vl_model, "process"):
        _orig_process = vl_model.process

        def _timed_process(data, **kwargs):
            # Extract input metadata
            first = data[0] if data else {}
            img = first.get("image")
            query = first.get("query", "")
            if isinstance(img, np.ndarray) and img.ndim >= 2:
                pixels = img.shape[0] * img.shape[1]
            else:
                pixels = 0
            label = _query_to_label(str(query))

            # Reset sub-step accumulator for this block
            _timing_state.current_block = {
                "pre_ms": 0.0, "xfer_ms": 0.0, "gen_ms": 0.0, "post_ms": 0.0,
            }

            result = _orig_process(data, **kwargs)

            # Extract output char count
            try:
                chars = sum(
                    len(r) for r in result.get("result", []) if isinstance(r, str)
                )
            except (AttributeError, TypeError):
                chars = 0

            block: dict[str, Any] = dict(_timing_state.current_block)
            block.update({"label": label, "pixels": pixels, "chars": chars})
            total_block_ms = block["pre_ms"] + block["xfer_ms"] + block["gen_ms"] + block["post_ms"]

            log.info(
                "[TIMING] Block label=%-22s pixels=%7d"
                "  pre=%5.0fms xfer=%4.0fms gen=%6.0fms post=%4.0fms chars=%5d",
                label, pixels,
                block["pre_ms"], block["xfer_ms"], block["gen_ms"], block["post_ms"], chars,
            )

            if not hasattr(_timing_state, "blocks"):
                _timing_state.blocks = []
            if not hasattr(_timing_state, "vlm_total_ms"):
                _timing_state.vlm_total_ms = 0.0
            _timing_state.blocks.append(block)
            _timing_state.vlm_total_ms += total_block_ms

            return result

        vl_model.process = _timed_process


def _enable_mps_acceleration(ocr_engine) -> None:
    """Move the VLM PyTorch model to Apple Metal GPU (MPS).

    Patches two things on the live DocVLMPredictor instance:
      1. vl_model.infer        — moved to MPS via .to("mps")
      2. _switch_inputs_to_device — replaced to move torch.Tensor inputs to MPS

    The existing TemporaryDeviceChanger(self.device) context only sets the Paddle
    device and has no effect on PyTorch MPS computation.

    On failure (unsupported op, MPS unavailable), logs a warning and leaves the
    model on CPU — no crash.
    """
    try:
        import torch
    except ModuleNotFoundError:
        log.warning("[MPS] torch not installed — model stays on CPU")
        return

    if not (hasattr(torch.backends, "mps") and torch.backends.mps.is_available()):
        log.warning("[MPS] not available — model stays on CPU")
        return

    outer = ocr_engine.paddlex_pipeline
    pipeline = getattr(outer, "_pipeline", outer)

    if not hasattr(pipeline, "vl_rec_model"):
        log.warning("[MPS] vl_rec_model not found — skipping")
        return

    vl_model = pipeline.vl_rec_model

    # Guard against double-install
    if getattr(vl_model, "_mps_enabled", False):
        log.warning("[MPS] already enabled — skipping")
        return

    # Newer PaddleOCR-VL builds can expose a Paddle model here instead of a
    # PyTorch module. In that case, torch MPS offload is not applicable.
    infer_obj = getattr(vl_model, "infer", None)
    infer_module = type(infer_obj).__module__ if infer_obj is not None else ""
    if infer_obj is None:
        log.warning("[MPS] vl_rec_model.infer missing — skipping")
        return
    if infer_module.startswith("paddle") or infer_module.startswith("paddlex"):
        log.warning(
            "[MPS] unsupported backend for torch MPS offload: %s. "
            "This PaddleOCR-VL build keeps the VLM on Paddle tensors.",
            infer_module,
        )
        return

    try:
        vl_model.infer = infer_obj.to("mps")
    except Exception as exc:
        log.warning("[MPS] failed to move model to MPS: %s — staying on CPU", exc)
        return

    def _mps_switch_inputs(input_dict):
        return {
            k: v.to("mps") if isinstance(v, torch.Tensor) else v
            for k, v in input_dict.items()
        }

    vl_model._switch_inputs_to_device = _mps_switch_inputs
    vl_model._mps_enabled = True
    log.info("[MPS] VLM model and input routing moved to Metal GPU")


def _install_dynamic_pixels_hook(ocr_engine) -> None:
    """Wrap vl_rec_model.process to cap min_pixels for OCR blocks.

    Only OCR blocks (query prefix "OCR:") are affected. Table, formula, chart,
    seal, and spotting blocks keep their original min_pixels so their complex
    structured output is unaffected.

    If min_pixels is already <= cap (or None), the call is passed through
    unchanged — zero overhead.
    """
    outer = ocr_engine.paddlex_pipeline
    pipeline = getattr(outer, "_pipeline", outer)

    if not hasattr(pipeline, "vl_rec_model"):
        return

    vl_model = pipeline.vl_rec_model
    if not hasattr(vl_model, "process"):
        return

    if getattr(vl_model, "_dynamic_pixels_installed", False):
        log.warning("[DynPx] already installed — skipping")
        return

    _orig_process = vl_model.process
    cap = _vlm_ocr_max_pixels()

    def _dynamic_pixels_process(data, min_pixels=None, **kwargs):
        if min_pixels is not None and min_pixels > cap and data:
            query = (data[0].get("query", "") if isinstance(data[0], dict) else "")
            if query.upper().startswith("OCR:"):
                min_pixels = cap
        return _orig_process(data, min_pixels=min_pixels, **kwargs)

    vl_model.process = _dynamic_pixels_process
    vl_model._dynamic_pixels_installed = True
    log.info(
        "[DynPx] OCR min_pixels capped at %d (%dx%d)",
        cap, int(cap ** 0.5), int(cap ** 0.5),
    )


# ---------------------------------------------------------------------------
# Output helpers
# ---------------------------------------------------------------------------

def _simplify_page_output(page_data, page_number, image_filename):
    """Keep only minimal block fields and include page number per block."""
    if not isinstance(page_data, dict):
        return page_data
    blocks = page_data.get("parsing_res_list")
    if not isinstance(blocks, list):
        return page_data
    simplified = []
    for block in blocks:
        if not isinstance(block, dict):
            continue
        simplified.append({
            "block_label": block.get("block_label"),
            "block_content": block.get("block_content"),
            "block_bbox": block.get("block_bbox"),
            "page_number": page_number,
            "image": image_filename,
        })
    page_data["parsing_res_list"] = simplified
    return page_data


# ---------------------------------------------------------------------------
# PaddleParser backend
# ---------------------------------------------------------------------------

class PaddleParser(ParserBackend):
    name = "paddleocr"

    def __init__(self) -> None:
        self._ocr_engine = None
        self._initialized = False

    def init(self) -> None:
        if self._initialized:
            return

        import fitz  # noqa: F401
        import numpy as np  # noqa: F401
        from paddleocr import PaddleOCR, PaddleOCRVL

        if _use_vl():
            self._ocr_engine = PaddleOCRVL()
            log.info("PaddleParser: PaddleOCR-VL initialized")
        else:
            self._ocr_engine = PaddleOCR(
                use_doc_orientation_classify=False,
                use_doc_unwarping=False,
                use_textline_orientation=False,
            )
            log.info("PaddleParser: PaddleOCR initialized")

        if _use_quantization():
            _enable_quantization(self._ocr_engine)
        if _use_mps():
            _enable_mps_acceleration(self._ocr_engine)
        _install_dynamic_pixels_hook(self._ocr_engine)
        if _use_timing():
            _install_timing_hooks(self._ocr_engine)
            log.info("PaddleParser: timing hooks installed")

        self._initialized = True

    def parse(self, pdf_path, output_dir, on_progress):
        import fitz
        import numpy as np

        doc = fitz.open(pdf_path)
        total_pages = len(doc)
        pages = []
        tmp_json = os.path.join(output_dir, "_tmp_ocr.json")

        try:
            for page_num in range(total_pages):
                page = doc[page_num]

                if _use_timing():
                    _t0 = time.perf_counter()
                pix = page.get_pixmap(matrix=fitz.Matrix(2, 2), colorspace=fitz.csRGB)
                image_filename = f"page_{page_num + 1}.png"
                image_path = os.path.join(output_dir, image_filename)
                if _use_timing():
                    _t1 = time.perf_counter()
                    log.info("[TIMING] PageRender: %.0fms", (_t1 - _t0) * 1000)
                pix.save(image_path)
                if _use_timing():
                    _t2 = time.perf_counter()
                    log.info("[TIMING] PNGSave: %.0fms", (_t2 - _t1) * 1000)
                img_array = np.frombuffer(pix.samples, dtype=np.uint8).reshape(
                    pix.height, pix.width, 3
                )
                if _use_timing():
                    log.info("[TIMING] ArrayBuild: %.0fms", (time.perf_counter() - _t2) * 1000)

                results = self._ocr_engine.predict(input=img_array)
                page_data = []
                for res in results:
                    if hasattr(res, "json") and isinstance(res.json, dict):
                        page_data = res.json
                    else:
                        res.save_to_json(tmp_json)
                        with open(tmp_json, "r", encoding="utf-8") as f:
                            page_data = json.load(f)

                pages.append(_simplify_page_output(page_data, page_num + 1, image_filename))
                on_progress(page_num + 1, total_pages)
        finally:
            doc.close()
            if os.path.exists(tmp_json):
                os.remove(tmp_json)

        return {
            "pages": pages,
            "total_pages": total_pages,
            "engine": "paddleocr",
        }
