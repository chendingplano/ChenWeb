-- +goose Up
DELETE FROM kb.metrics a USING kb.metrics b
WHERE a.metric_id IS NOT NULL
  AND a.metric_id = b.metric_id
  AND a.input_record_id = b.input_record_id
  AND a.id > b.id;

CREATE UNIQUE INDEX IF NOT EXISTS kb_metrics_record_metric_id_uniq
    ON kb.metrics (input_record_id, metric_id)
    WHERE metric_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS kb_metrics_record_metric_id_uniq;
