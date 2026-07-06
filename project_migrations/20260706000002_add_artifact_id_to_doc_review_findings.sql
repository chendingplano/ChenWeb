-- +goose Up
-- Identifies which artifact-under-review (metric_id / prov_id /
-- inventory_item_id) a finding is about, for the per-artifact reviewers
-- (metrics, provisions, inventory_items). Distinct from the existing
-- related_artifact_id/related_record_id in metadata (ADR 2026070201 AR5),
-- which identify the matched cross-document artifact instead.

ALTER TABLE kb.doc_review_findings
    ADD COLUMN artifact_id TEXT;

CREATE INDEX IF NOT EXISTS idx_doc_review_findings_artifact
    ON kb.doc_review_findings (input_record_id, artifact_id);

-- +goose Down
DROP INDEX IF EXISTS idx_doc_review_findings_artifact;
ALTER TABLE kb.doc_review_findings
    DROP COLUMN IF EXISTS artifact_id;
