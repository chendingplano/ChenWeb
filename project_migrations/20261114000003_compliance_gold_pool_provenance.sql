-- +goose Up
-- Compliance Review gold set (openspec compliance-reviewer-phase2, design D11/D12, second revision
-- 2026-11-14). queries and cited_docs are what the agent gave the pool for an activity (extra
-- queries searched as written; documents the text cites, searched within). found_by names the pool
-- list that first found a judged passage (q2/lexical, cited:<citation>, expand:<record>,
-- review:<doc-review run>). An augmented run copies its base run and adds the passages a review run
-- reported that the base run never judged.
ALTER TABLE testbed.compliance_gold_activities ADD COLUMN IF NOT EXISTS queries TEXT[] NOT NULL DEFAULT '{}';
ALTER TABLE testbed.compliance_gold_activities ADD COLUMN IF NOT EXISTS cited_docs TEXT[] NOT NULL DEFAULT '{}';
ALTER TABLE testbed.compliance_gold_judgements ADD COLUMN IF NOT EXISTS found_by TEXT NOT NULL DEFAULT '';
ALTER TABLE testbed.compliance_gold_runs ADD COLUMN IF NOT EXISTS base_run_id BIGINT
    REFERENCES testbed.compliance_gold_runs(id) ON DELETE RESTRICT;
ALTER TABLE testbed.compliance_gold_runs ADD COLUMN IF NOT EXISTS review_run_id BIGINT
    REFERENCES kb.doc_review_runs(id) ON DELETE SET NULL;
ALTER TABLE testbed.compliance_gold_runs DROP CONSTRAINT IF EXISTS compliance_gold_runs_augmentation_check;
ALTER TABLE testbed.compliance_gold_runs ADD CONSTRAINT compliance_gold_runs_augmentation_check
    CHECK (review_run_id IS NULL OR base_run_id IS NOT NULL);

-- +goose Down
ALTER TABLE testbed.compliance_gold_runs DROP CONSTRAINT IF EXISTS compliance_gold_runs_augmentation_check;
ALTER TABLE testbed.compliance_gold_runs DROP COLUMN IF EXISTS review_run_id;
ALTER TABLE testbed.compliance_gold_runs DROP COLUMN IF EXISTS base_run_id;
ALTER TABLE testbed.compliance_gold_judgements DROP COLUMN IF EXISTS found_by;
ALTER TABLE testbed.compliance_gold_activities DROP COLUMN IF EXISTS cited_docs;
ALTER TABLE testbed.compliance_gold_activities DROP COLUMN IF EXISTS queries;
