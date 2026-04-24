import time
from unittest.mock import MagicMock, patch
from pdf_parser import (
    get_backend,
    make_throttled_progress,
    PARSER_REGISTRY,
    should_drop_stage_event,
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
