-- +goose Up
-- Spike (kb.metrics only): make line spans a first-class, index-accelerated source of
-- truth for line-overlap connections. See
-- KnowledgeStore/Capsules/coding-capsules/llm-wiki/artifact-connections.md §2.2
-- ("Toward the purest form").
--
-- source_line_spans is a JSONB array of strings, each "N", "A:B", or "A-B" (end
-- inclusive), matching parseMetricLineSpan in extract-metrics.go. We derive an
-- int8multirange from it via a STORED generated column so the range can never drift
-- from source_line_spans (this subsumes any Go-side dual-write) and so existing rows
-- are backfilled automatically when the column is added.

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION kb.line_spans_to_int8multirange(spans jsonb)
RETURNS int8multirange
LANGUAGE plpgsql
IMMUTABLE
PARALLEL SAFE
AS $fn$
DECLARE
    elem text;
    sep  int;
    lo_t text;
    hi_t text;
    lo   bigint;
    hi   bigint;
    acc  int8multirange := '{}'::int8multirange;
BEGIN
    IF spans IS NULL OR jsonb_typeof(spans) <> 'array' THEN
        RETURN acc;
    END IF;
    FOR elem IN SELECT jsonb_array_elements_text(spans) LOOP
        elem := btrim(elem);
        CONTINUE WHEN elem = '';
        -- Separator is ':' (preferred, as in Go) or '-'.
        sep := position(':' in elem);
        IF sep = 0 THEN
            sep := position('-' in elem);
        END IF;
        IF sep > 0 THEN
            lo_t := btrim(substring(elem from 1 for sep - 1));
            hi_t := btrim(substring(elem from sep + 1));
        ELSE
            lo_t := elem;
            hi_t := elem;
        END IF;
        BEGIN
            lo := lo_t::bigint;
            hi := hi_t::bigint;
        EXCEPTION WHEN others THEN
            CONTINUE;  -- unparseable element: skip, matching parseMetricLineSpan
        END;
        -- parseMetricLineSpan requires start > 0 and end >= start.
        IF lo <= 0 OR hi < lo THEN
            CONTINUE;
        END IF;
        -- Inclusive [lo, hi] -> half-open int8range(lo, hi+1, '[)').
        acc := acc + int8multirange(int8range(lo, hi + 1, '[)'));
    END LOOP;
    RETURN acc;
END;
$fn$;
-- +goose StatementEnd

ALTER TABLE kb.metrics
    ADD COLUMN IF NOT EXISTS line_range int8multirange
    GENERATED ALWAYS AS (kb.line_spans_to_int8multirange(source_line_spans)) STORED;

CREATE INDEX IF NOT EXISTS idx_kb_metrics_line_range
    ON kb.metrics USING gist (line_range);

-- +goose Down
DROP INDEX IF EXISTS kb.idx_kb_metrics_line_range;
ALTER TABLE kb.metrics DROP COLUMN IF EXISTS line_range;
DROP FUNCTION IF EXISTS kb.line_spans_to_int8multirange(jsonb);
