-- +goose Up
-- Compliance Review gold set (openspec compliance-reviewer-phase2, design D8 revised 2026-11-14): the
-- covered-docs and uncovered-docs lists were merged into one list, [compliance-review.gold-docs].
-- Whether an activity has grounding comes from its labels (a governs judgement or none), so the
-- per-activity copy of the list a document was in is dropped.
ALTER TABLE testbed.compliance_gold_activities DROP COLUMN IF EXISTS covered;

-- +goose Down
-- The list a document was in is not recoverable; restored rows read false.
ALTER TABLE testbed.compliance_gold_activities ADD COLUMN IF NOT EXISTS covered BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE testbed.compliance_gold_activities ALTER COLUMN covered DROP DEFAULT;
