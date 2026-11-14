-- +goose Up
-- Compliance Review gold set (openspec compliance-reviewer-phase2, design D4/D7/D10 revised
-- 2026-11-14): the first iteration's gold is labelled by the coding agent alone (method 'agent');
-- the two-annotator model panel with an adjudicator (method 'model_panel') stays possible.
-- max_candidates (was pool_limit) is the cap on pooled candidates per activity, from
-- [reviewers.compliance].max_candidates in doc-review.local.toml.
ALTER TABLE testbed.compliance_gold_runs ADD COLUMN IF NOT EXISTS method TEXT NOT NULL DEFAULT 'model_panel';
ALTER TABLE testbed.compliance_gold_runs ALTER COLUMN method DROP DEFAULT;
ALTER TABLE testbed.compliance_gold_runs ADD COLUMN IF NOT EXISTS rules_version TEXT NOT NULL DEFAULT '';
ALTER TABLE testbed.compliance_gold_runs ADD COLUMN IF NOT EXISTS audit_notes TEXT NOT NULL DEFAULT '';
ALTER TABLE testbed.compliance_gold_runs RENAME COLUMN pool_limit TO max_candidates;
ALTER TABLE testbed.compliance_gold_runs ALTER COLUMN annotator_2 DROP NOT NULL;
ALTER TABLE testbed.compliance_gold_runs ALTER COLUMN adjudicator DROP NOT NULL;
ALTER TABLE testbed.compliance_gold_runs DROP CONSTRAINT IF EXISTS compliance_gold_runs_check;
ALTER TABLE testbed.compliance_gold_runs DROP CONSTRAINT IF EXISTS compliance_gold_runs_labellers_check;
ALTER TABLE testbed.compliance_gold_runs ADD CONSTRAINT compliance_gold_runs_labellers_check CHECK (
    (method = 'agent' AND annotator_2 IS NULL AND adjudicator IS NULL)
    OR (method = 'model_panel' AND annotator_2 IS NOT NULL AND adjudicator IS NOT NULL AND annotator_1 <> annotator_2));

-- An agent run has one labeller: every activity is 'single', its label is label_1 (which may be
-- 'ambiguous', with a reason) and label_2 is empty.
ALTER TABLE testbed.compliance_gold_activities DROP CONSTRAINT IF EXISTS compliance_gold_activities_agreement_check;
ALTER TABLE testbed.compliance_gold_activities ADD CONSTRAINT compliance_gold_activities_agreement_check
    CHECK (agreement IN ('both', 'adjudicated_kept', 'single'));
ALTER TABLE testbed.compliance_gold_judgements DROP CONSTRAINT IF EXISTS compliance_gold_judgements_label_1_check;
ALTER TABLE testbed.compliance_gold_judgements ADD CONSTRAINT compliance_gold_judgements_label_1_check
    CHECK (label_1 IN ('governs', 'topical', 'unrelated', 'ambiguous'));
ALTER TABLE testbed.compliance_gold_judgements ALTER COLUMN label_2 DROP NOT NULL;

-- +goose Down
-- Agent runs cannot be represented in the earlier two-annotator shape, so they are removed.
DELETE FROM testbed.compliance_gold_runs WHERE method = 'agent';
UPDATE testbed.compliance_gold_judgements SET label_2 = '' WHERE label_2 IS NULL;
ALTER TABLE testbed.compliance_gold_judgements ALTER COLUMN label_2 SET NOT NULL;
ALTER TABLE testbed.compliance_gold_judgements DROP CONSTRAINT IF EXISTS compliance_gold_judgements_label_1_check;
ALTER TABLE testbed.compliance_gold_judgements ADD CONSTRAINT compliance_gold_judgements_label_1_check
    CHECK (label_1 IN ('governs', 'topical', 'unrelated'));
ALTER TABLE testbed.compliance_gold_activities DROP CONSTRAINT IF EXISTS compliance_gold_activities_agreement_check;
ALTER TABLE testbed.compliance_gold_activities ADD CONSTRAINT compliance_gold_activities_agreement_check
    CHECK (agreement IN ('both', 'adjudicated_kept'));
ALTER TABLE testbed.compliance_gold_runs DROP CONSTRAINT IF EXISTS compliance_gold_runs_labellers_check;
ALTER TABLE testbed.compliance_gold_runs ADD CONSTRAINT compliance_gold_runs_check CHECK (annotator_1 <> annotator_2);
ALTER TABLE testbed.compliance_gold_runs ALTER COLUMN adjudicator SET NOT NULL;
ALTER TABLE testbed.compliance_gold_runs ALTER COLUMN annotator_2 SET NOT NULL;
ALTER TABLE testbed.compliance_gold_runs RENAME COLUMN max_candidates TO pool_limit;
ALTER TABLE testbed.compliance_gold_runs DROP COLUMN IF EXISTS audit_notes;
ALTER TABLE testbed.compliance_gold_runs DROP COLUMN IF EXISTS rules_version;
ALTER TABLE testbed.compliance_gold_runs DROP COLUMN IF EXISTS method;
