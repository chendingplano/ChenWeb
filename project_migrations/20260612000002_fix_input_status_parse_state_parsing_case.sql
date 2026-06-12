-- +goose Up
--
-- The original 20260609000002 migration was deployed without the 'parsing'
-- WHEN clause in kb.input_status_parse_state. Records with proc_status='active'
-- (currently being parsed) were incorrectly mapped to 'parsed_failed' instead of
-- 'parsing'. This migration re-deploys the corrected function and backfills any
-- stale parse_state values.

-- +goose StatementBegin
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

-- Backfill any rows whose parse_state was computed by the old (broken) function.
UPDATE kb.inputs
SET parse_state = kb.input_status_parse_state(status)
WHERE parse_state <> kb.input_status_parse_state(status);

-- +goose Down
-- Restore the broken version (pre-fix) — only needed for rollback completeness.
-- +goose StatementBegin
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
        ELSE 'parsed_failed'
    END
$$;
-- +goose StatementEnd
