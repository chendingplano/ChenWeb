-- +goose Up
-- Retire the organization profile of the Compliance Review gold set (research 2026111103, activity
-- context; openspec compliance-reviewer-phase2). Activity context is extracted from the evaluation
-- documents with provenance, so a gold run no longer stores a user-supplied organization profile.
ALTER TABLE testbed.compliance_gold_runs DROP COLUMN IF EXISTS organization;

-- +goose Down
ALTER TABLE testbed.compliance_gold_runs
    ADD COLUMN IF NOT EXISTS organization JSONB NOT NULL DEFAULT '{}'::jsonb
    CHECK (jsonb_typeof(organization) = 'object');
