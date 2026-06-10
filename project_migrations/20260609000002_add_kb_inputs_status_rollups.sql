-- +goose Up
--
-- Doc-processor status rollups.
--
-- kb.inputs.status is a JSON array holding one entry per processor (parsing,
-- converting, static_analyzer, extract_*, generate_*, plus the aggregate
-- "doc_processing" control entry). Filtering tens of millions of rows by status
-- forced a sequential scan + jsonb_array_elements per row. This migration adds:
--
--   1. Three derived scalar columns on kb.inputs (parse_state, pipeline_state,
--      has_failed_proc) with partial / btree indexes for the hot dashboard
--      queries.
--   2. A child table kb.input_proc_status with one row per (record, processor)
--      for per-processor status querying.
--
-- Both projections are maintained by DB triggers, so EVERY writer of
-- kb.inputs.status (pdf-parser, service-pdf-parser, file-converters,
-- doc-processor, stop handler, ...) keeps them consistent with no application
-- changes and no risk of a missed call site. status stays the source of truth.

-- ---------------------------------------------------------------------------
-- Derivation helpers (shared by the triggers and the backfill below).
-- ---------------------------------------------------------------------------

-- +goose StatementBegin
-- canonical_op mirrors canonicalOperationName in
-- ChenWeb/server/api/doc-processing/event.go: lowercases, maps '-' to '_', and
-- folds two legacy aliases onto their canonical names.
CREATE OR REPLACE FUNCTION kb.canonical_op(raw text)
RETURNS text
LANGUAGE sql
IMMUTABLE
AS $$
    SELECT CASE replace(lower(trim(coalesce(raw, ''))), '-', '_')
               WHEN 'chunked'          THEN 'chunking'
               WHEN 'extract_metadata' THEN 'extract_doc_metadata'
               ELSE replace(lower(trim(coalesce(raw, ''))), '-', '_')
           END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
-- input_status_parse_state derives the parse state of the single 'parsed' entry
-- the PDF parser maintains (proc_status transitions active -> success/fail/
-- duplicated):
--   'pending'        - no parse entry yet (never parsed)
--   'parsing'        - a parse is in progress (proc_status active/running)
--   'parsed_success' - parsed successfully
--   'parsed_failed'  - parse failed or the record is a duplicate
-- 'pending' and 'parsing' are kept distinct so the parser's claim query can map
-- "never parsed" vs "stale in-progress claim" onto the indexed column.
CREATE OR REPLACE FUNCTION kb.input_status_parse_state(s jsonb)
RETURNS text
LANGUAGE sql
IMMUTABLE
AS $$
    WITH p AS (
        SELECT lower(coalesce(nullif(elem->>'proc_status', ''),
                              nullif(elem->>'proc-status', ''),
                              elem->>'status', '')) AS st
        FROM jsonb_array_elements(coalesce(s, '[]'::jsonb)) AS elem
        WHERE kb.canonical_op(elem->>'operation') IN ('parsed', 'parsing', 'parse')
    )
    SELECT CASE
        WHEN NOT EXISTS (SELECT 1 FROM p)                  THEN 'pending'
        WHEN EXISTS (SELECT 1 FROM p WHERE st = 'success') THEN 'parsed_success'
        WHEN EXISTS (SELECT 1 FROM p WHERE st IN ('active', 'running', 'parsing', 'in_progress')) THEN 'parsing'
        ELSE 'parsed_failed'
    END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
-- input_status_pipeline_state derives the overall doc-processing pipeline state
-- from the aggregate "doc_processing" control entry: 'pending' (never started),
-- 'running', 'success', 'failed', or 'stopped'.
CREATE OR REPLACE FUNCTION kb.input_status_pipeline_state(s jsonb)
RETURNS text
LANGUAGE sql
IMMUTABLE
AS $$
    WITH d AS (
        SELECT lower(coalesce(nullif(elem->>'proc_status', ''),
                              nullif(elem->>'proc-status', ''),
                              elem->>'status', '')) AS st
        FROM jsonb_array_elements(coalesce(s, '[]'::jsonb)) AS elem
        WHERE kb.canonical_op(elem->>'operation') = 'doc_processing'
    )
    SELECT CASE
        WHEN NOT EXISTS (SELECT 1 FROM d)                  THEN 'pending'
        WHEN EXISTS (SELECT 1 FROM d WHERE st = 'failed')  THEN 'failed'
        WHEN EXISTS (SELECT 1 FROM d WHERE st = 'stopped') THEN 'stopped'
        WHEN EXISTS (SELECT 1 FROM d WHERE st = 'success')
             AND NOT EXISTS (SELECT 1 FROM d WHERE st <> 'success') THEN 'success'
        ELSE 'running'
    END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
-- input_status_has_failed_proc is true when any real doc processor failed.
-- parsing / converting failures are intentionally excluded (they are captured
-- by parse_state) to match the existing "failed-procs" selector semantics in
-- ChenWeb/server/api/doc-processing/control.go (failedDocProcessorNames).
CREATE OR REPLACE FUNCTION kb.input_status_has_failed_proc(s jsonb)
RETURNS boolean
LANGUAGE sql
IMMUTABLE
AS $$
    SELECT EXISTS (
        SELECT 1
        FROM jsonb_array_elements(coalesce(s, '[]'::jsonb)) AS elem
        WHERE lower(coalesce(nullif(elem->>'proc_status', ''),
                             nullif(elem->>'proc-status', ''),
                             elem->>'status', '')) = 'failed'
          AND kb.canonical_op(elem->>'operation') NOT IN (
              '', 'parsed', 'parsing', 'parse', 'converted', 'converting',
              'consumed', 'doc_processing', 'doc_processor', 'stop_requested'
          )
    )
$$;
-- +goose StatementEnd

-- ---------------------------------------------------------------------------
-- Rollup columns on kb.inputs (nullable + constant default => fast metadata-only
-- ALTER even on a very large table; real values are backfilled below).
-- ---------------------------------------------------------------------------

ALTER TABLE kb.inputs
    ADD COLUMN IF NOT EXISTS parse_state     TEXT    NOT NULL DEFAULT 'pending',
    ADD COLUMN IF NOT EXISTS pipeline_state  TEXT    NOT NULL DEFAULT 'pending',
    ADD COLUMN IF NOT EXISTS has_failed_proc BOOLEAN NOT NULL DEFAULT false;

-- ---------------------------------------------------------------------------
-- Child table: one row per (record, processor).
-- ---------------------------------------------------------------------------

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS kb.input_proc_status (
    record_id   BIGINT      NOT NULL REFERENCES kb.inputs(id) ON DELETE CASCADE,
    processor   TEXT        NOT NULL,
    proc_status TEXT        NOT NULL DEFAULT '',
    start_time  TEXT        NOT NULL DEFAULT '',
    ms_used     BIGINT      NULL,
    error       TEXT        NULL,
    modify_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT input_proc_status_pkey PRIMARY KEY (record_id, processor)
);
-- +goose StatementEnd

-- Per-processor status lookups: "records that failed on extract_metrics", etc.
CREATE INDEX IF NOT EXISTS idx_kb_input_proc_status_processor_status
    ON kb.input_proc_status (processor, proc_status);
-- "any failed processor for record" hot subset.
CREATE INDEX IF NOT EXISTS idx_kb_input_proc_status_failed
    ON kb.input_proc_status (record_id)
    WHERE proc_status = 'failed';

-- ---------------------------------------------------------------------------
-- Backfill existing rows from the canonical status array. These statements set
-- the rollup columns directly (the column-rollup trigger only fires on writes
-- to `status`, so backfilling the columns here does not re-trigger).
-- ---------------------------------------------------------------------------

-- +goose StatementBegin
UPDATE kb.inputs
SET parse_state     = kb.input_status_parse_state(status),
    pipeline_state  = kb.input_status_pipeline_state(status),
    has_failed_proc = kb.input_status_has_failed_proc(status)
WHERE status IS NOT NULL;
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO kb.input_proc_status (record_id, processor, proc_status, start_time, ms_used, error, modify_time)
SELECT DISTINCT ON (i.id, s.proc)
       i.id, s.proc, s.proc_status, s.start_time, s.ms_used, s.error, NOW()
FROM kb.inputs i
CROSS JOIN LATERAL (
    SELECT kb.canonical_op(elem->>'operation') AS proc,
           lower(coalesce(nullif(elem->>'proc_status', ''),
                          nullif(elem->>'proc-status', ''),
                          elem->>'status', '')) AS proc_status,
           coalesce(elem->>'start_time', '') AS start_time,
           nullif(coalesce(elem->>'ms_used', elem->>'ms-used'), '')::numeric::bigint AS ms_used,
           coalesce(elem->>'error', elem->>'error_msg') AS error,
           ord
    FROM jsonb_array_elements(coalesce(i.status, '[]'::jsonb)) WITH ORDINALITY AS t(elem, ord)
) s
WHERE s.proc NOT IN ('', 'doc_processing', 'doc_processor', 'consumed', 'stop_requested')
ORDER BY i.id, s.proc, s.ord DESC
ON CONFLICT (record_id, processor) DO NOTHING;
-- +goose StatementEnd

-- ---------------------------------------------------------------------------
-- Indexes on the rollup columns (created after backfill for speed).
-- ---------------------------------------------------------------------------

-- Active-pipeline dashboard query: only a tiny fraction of rows are ever
-- pending/running, so this partial index stays small regardless of table size.
CREATE INDEX IF NOT EXISTS idx_kb_inputs_pipeline_active
    ON kb.inputs (id)
    WHERE pipeline_state IN ('pending', 'running');
CREATE INDEX IF NOT EXISTS idx_kb_inputs_has_failed_proc
    ON kb.inputs (id)
    WHERE has_failed_proc;
CREATE INDEX IF NOT EXISTS idx_kb_inputs_parse_state
    ON kb.inputs (parse_state);
CREATE INDEX IF NOT EXISTS idx_kb_inputs_pipeline_state
    ON kb.inputs (pipeline_state);

-- ---------------------------------------------------------------------------
-- Triggers that keep the projections in sync with kb.inputs.status.
-- ---------------------------------------------------------------------------

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION kb.refresh_input_status_rollups()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.parse_state     := kb.input_status_parse_state(NEW.status);
    NEW.pipeline_state  := kb.input_status_pipeline_state(NEW.status);
    NEW.has_failed_proc := kb.input_status_has_failed_proc(NEW.status);
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS trg_refresh_input_status_rollups ON kb.inputs;
CREATE TRIGGER trg_refresh_input_status_rollups
BEFORE INSERT OR UPDATE OF status ON kb.inputs
FOR EACH ROW
EXECUTE FUNCTION kb.refresh_input_status_rollups();

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION kb.sync_input_proc_status()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    DELETE FROM kb.input_proc_status WHERE record_id = NEW.id;
    INSERT INTO kb.input_proc_status (record_id, processor, proc_status, start_time, ms_used, error, modify_time)
    SELECT DISTINCT ON (s.proc)
           NEW.id, s.proc, s.proc_status, s.start_time, s.ms_used, s.error, NOW()
    FROM (
        SELECT kb.canonical_op(elem->>'operation') AS proc,
               lower(coalesce(nullif(elem->>'proc_status', ''),
                              nullif(elem->>'proc-status', ''),
                              elem->>'status', '')) AS proc_status,
               coalesce(elem->>'start_time', '') AS start_time,
               nullif(coalesce(elem->>'ms_used', elem->>'ms-used'), '')::numeric::bigint AS ms_used,
               coalesce(elem->>'error', elem->>'error_msg') AS error,
               ord
        FROM jsonb_array_elements(coalesce(NEW.status, '[]'::jsonb)) WITH ORDINALITY AS t(elem, ord)
    ) s
    WHERE s.proc NOT IN ('', 'doc_processing', 'doc_processor', 'consumed', 'stop_requested')
    ORDER BY s.proc, s.ord DESC;
    RETURN NULL;
END;
$$;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS trg_sync_input_proc_status ON kb.inputs;
CREATE TRIGGER trg_sync_input_proc_status
AFTER INSERT OR UPDATE OF status ON kb.inputs
FOR EACH ROW
EXECUTE FUNCTION kb.sync_input_proc_status();

-- +goose Down
DROP TRIGGER IF EXISTS trg_sync_input_proc_status ON kb.inputs;
DROP TRIGGER IF EXISTS trg_refresh_input_status_rollups ON kb.inputs;
DROP FUNCTION IF EXISTS kb.sync_input_proc_status();
DROP FUNCTION IF EXISTS kb.refresh_input_status_rollups();
DROP INDEX IF EXISTS kb.idx_kb_inputs_pipeline_state;
DROP INDEX IF EXISTS kb.idx_kb_inputs_parse_state;
DROP INDEX IF EXISTS kb.idx_kb_inputs_has_failed_proc;
DROP INDEX IF EXISTS kb.idx_kb_inputs_pipeline_active;
DROP TABLE IF EXISTS kb.input_proc_status;
ALTER TABLE kb.inputs
    DROP COLUMN IF EXISTS has_failed_proc,
    DROP COLUMN IF EXISTS pipeline_state,
    DROP COLUMN IF EXISTS parse_state;
DROP FUNCTION IF EXISTS kb.input_status_has_failed_proc(jsonb);
DROP FUNCTION IF EXISTS kb.input_status_pipeline_state(jsonb);
DROP FUNCTION IF EXISTS kb.input_status_parse_state(jsonb);
DROP FUNCTION IF EXISTS kb.canonical_op(text);
