-- +goose Up
ALTER TABLE alarms_errors ADD COLUMN IF NOT EXISTS scope_id TEXT;

-- Partial unique index: only alarms that carry both a scope correlator and a
-- stable machine kind participate in dedup. Alarms without either (all existing
-- producers) are unaffected and always insert normally. This is the DB-level
-- "one warning per scope id" guarantee for automatic review-profile-selection
-- indeterminacy (spec 2026080102 section 11), independent of the run_id/record_id
-- routing-alarm indexes from migration 20260801000019 because a review scope is
-- identified by its TEXT review_scope_id, not by a BIGINT run/record id.
CREATE UNIQUE INDEX IF NOT EXISTS uq_alarms_errors_scope_id_kind
    ON alarms_errors (scope_id, kind)
    WHERE scope_id IS NOT NULL AND kind IS NOT NULL;

COMMENT ON COLUMN alarms_errors.scope_id IS
    'Optional correlator (kb.ontology_review_scopes.review_scope_id, TEXT) for machine-raised alarms '
    'that must be deduplicated per review scope, per spec 2026080102 section 11 (automatic '
    'profile-selection indeterminacy raises one warning deduplicated by scope id). NULL for alarms '
    'without a scope context.';

-- +goose Down
DROP INDEX IF EXISTS uq_alarms_errors_scope_id_kind;
ALTER TABLE alarms_errors DROP COLUMN IF EXISTS scope_id;
