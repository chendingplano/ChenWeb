-- +goose Up
-- Metric occurrences permit only one active supports link. NULLS NOT DISTINCT
-- ensures legacy metric evidence without input_record_id cannot bypass the
-- cardinality rule. The predicate deliberately leaves non-metric artifacts,
-- contradictions, and retired evidence unrestricted.
CREATE UNIQUE INDEX IF NOT EXISTS uq_assertion_evidence_current_metric_support
    ON kb.assertion_evidence (artifact_type, artifact_id, input_record_id) NULLS NOT DISTINCT
    WHERE artifact_type = 'metric'
      AND evidence_role = 'supports'
      AND NOT deleted;

-- +goose Down
DROP INDEX IF EXISTS kb.uq_assertion_evidence_current_metric_support;
