-- +goose Up
ALTER TABLE alarms_errors ADD COLUMN IF NOT EXISTS run_id BIGINT;
ALTER TABLE alarms_errors ADD COLUMN IF NOT EXISTS kind TEXT;

-- Partial unique index: only alarms that carry both a run correlator and a
-- stable machine kind participate in dedup. Alarms without either (most
-- existing alarm producers) are unaffected and always insert normally.
CREATE UNIQUE INDEX IF NOT EXISTS uq_alarms_errors_run_id_kind
    ON alarms_errors (run_id, kind)
    WHERE run_id IS NOT NULL AND kind IS NOT NULL;

COMMENT ON COLUMN alarms_errors.run_id IS
    'Optional correlator (e.g. kb.doc_process_runs.id) for machine-raised alarms that must be '
    'deduplicated per run, per ADR 2026072901 DR3 / spec 2026080102 section 11. NULL for alarms '
    'without a run context (e.g. raised before a run row exists).';
COMMENT ON COLUMN alarms_errors.kind IS
    'Optional stable machine-readable classifier (e.g. RoutingAlarm.Kind) used together with '
    'run_id for per-run dedup via uq_alarms_errors_run_id_kind. NULL for alarms without a stable '
    'kind.';

-- +goose Down
DROP INDEX IF EXISTS uq_alarms_errors_run_id_kind;
ALTER TABLE alarms_errors DROP COLUMN IF EXISTS kind;
ALTER TABLE alarms_errors DROP COLUMN IF EXISTS run_id;
