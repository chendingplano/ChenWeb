-- +goose Up
-- openspec change extract-references-processor: a metric reference that resolves to a metric of
-- another document adds a row <rec>_rfm_<n> to the referencing document's kb.metrics. These
-- columns mark such rows (ref_mention_id non-null) and point at the original metric. They are null
-- on every row extract_metrics writes. Deleting the mention deletes its resolved rows.
ALTER TABLE kb.metrics ADD COLUMN IF NOT EXISTS ref_mention_id BIGINT REFERENCES kb.ref_mentions(id) ON DELETE CASCADE;
ALTER TABLE kb.metrics ADD COLUMN IF NOT EXISTS ref_input_record_id BIGINT;
ALTER TABLE kb.metrics ADD COLUMN IF NOT EXISTS ref_metric_id TEXT;
ALTER TABLE kb.metrics ADD COLUMN IF NOT EXISTS ref_metric_key TEXT;

CREATE INDEX IF NOT EXISTS idx_kb_metrics_ref_mention_id ON kb.metrics(ref_mention_id) WHERE ref_mention_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS kb.idx_kb_metrics_ref_mention_id;
ALTER TABLE kb.metrics DROP COLUMN IF EXISTS ref_metric_key;
ALTER TABLE kb.metrics DROP COLUMN IF EXISTS ref_metric_id;
ALTER TABLE kb.metrics DROP COLUMN IF EXISTS ref_input_record_id;
ALTER TABLE kb.metrics DROP COLUMN IF EXISTS ref_mention_id;
