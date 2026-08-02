-- +goose Up
ALTER TABLE alarms_errors ADD COLUMN IF NOT EXISTS run_id BIGINT;
ALTER TABLE alarms_errors ADD COLUMN IF NOT EXISTS kind TEXT;
ALTER TABLE alarms_errors ADD COLUMN IF NOT EXISTS record_id BIGINT;

-- Partial unique index: only alarms that carry both a run correlator and a
-- stable machine kind participate in dedup. Alarms without either (most
-- existing alarm producers) are unaffected and always insert normally.
CREATE UNIQUE INDEX IF NOT EXISTS uq_alarms_errors_run_id_kind
    ON alarms_errors (run_id, kind)
    WHERE run_id IS NOT NULL AND kind IS NOT NULL;

-- Second, narrower dedup key for alarms raised before a kb.doc_process_runs
-- row exists (e.g. a block-mode DR7 conflict that fails processing before
-- dispatch, per spec 2026080102 section 11 -- there is no run_id yet at
-- that call site). record_id is stable across redeliveries/retries of the
-- same input record, so it is the next-best correlator when run_id is
-- unavailable. Scoped to run_id IS NULL so it never competes with the
-- run_id-based index above once a run row does exist.
CREATE UNIQUE INDEX IF NOT EXISTS uq_alarms_errors_record_id_kind
    ON alarms_errors (record_id, kind)
    WHERE run_id IS NULL AND record_id IS NOT NULL AND kind IS NOT NULL;

COMMENT ON COLUMN alarms_errors.run_id IS
    'Optional correlator (e.g. kb.doc_process_runs.id) for machine-raised alarms that must be '
    'deduplicated per run, per ADR 2026072901 DR3 / spec 2026080102 section 11. NULL for alarms '
    'without a run context (e.g. raised before a run row exists).';
COMMENT ON COLUMN alarms_errors.kind IS
    'Optional stable machine-readable classifier (e.g. RoutingAlarm.Kind) used together with '
    'run_id (or record_id when run_id is unavailable) for dedup via uq_alarms_errors_run_id_kind '
    '/ uq_alarms_errors_record_id_kind. NULL for alarms without a stable kind.';
COMMENT ON COLUMN alarms_errors.record_id IS
    'Optional correlator (e.g. kb.doc_metadata_input.id) for machine-raised alarms that fire '
    'before a kb.doc_process_runs row exists, so they can still be deduplicated per record via '
    'uq_alarms_errors_record_id_kind. NULL for alarms without a record context or once a run_id '
    'is available.';

-- +goose Down
DROP INDEX IF EXISTS uq_alarms_errors_record_id_kind;
DROP INDEX IF EXISTS uq_alarms_errors_run_id_kind;
ALTER TABLE alarms_errors DROP COLUMN IF EXISTS record_id;
ALTER TABLE alarms_errors DROP COLUMN IF EXISTS kind;
ALTER TABLE alarms_errors DROP COLUMN IF EXISTS run_id;
