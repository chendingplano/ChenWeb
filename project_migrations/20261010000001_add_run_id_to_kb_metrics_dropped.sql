-- +goose Up
-- kb.metrics_dropped.run_id: the kb.doc_process_runs row of the extract_metrics run that set the
-- row aside, the same id kb.doc_proc_logs.run_id carries. NULL when the run had no
-- kb.doc_process_runs row (a direct processor call). Existing rows are backfilled through
-- event_id when it names exactly one run of the same record.
ALTER TABLE kb.metrics_dropped ADD COLUMN IF NOT EXISTS run_id BIGINT REFERENCES kb.doc_process_runs (id);
CREATE INDEX IF NOT EXISTS idx_kb_metrics_dropped_run_id ON kb.metrics_dropped (run_id);

UPDATE kb.metrics_dropped d
SET run_id = r.id
FROM kb.doc_process_runs r
WHERE d.run_id IS NULL
  AND r.event_id = d.event_id
  AND r.record_id = d.input_record_id
  AND (SELECT count(*) FROM kb.doc_process_runs r2
       WHERE r2.event_id = d.event_id AND r2.record_id = d.input_record_id) = 1;

-- +goose Down
DROP INDEX IF EXISTS kb.idx_kb_metrics_dropped_run_id;
ALTER TABLE kb.metrics_dropped DROP COLUMN IF EXISTS run_id;
