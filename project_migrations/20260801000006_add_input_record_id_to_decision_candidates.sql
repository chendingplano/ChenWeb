-- +goose Up
-- Association-run telemetry (spec §10.9, ADR P3 chunk F) needs to reconcile
-- every candidate examined for one input record. logical_identity_key
-- embeds the record id by convention (e.g. "metric:2:2_mtc_1") but parsing
-- that string back out is fragile; a real column is the honest fix, mirroring
-- the same gap chunk E already found and fixed on kb.assertion_evidence.
ALTER TABLE kb.semantic_decision_candidates ADD COLUMN IF NOT EXISTS input_record_id BIGINT;

CREATE INDEX IF NOT EXISTS idx_kb_semantic_decision_candidates_input_record
    ON kb.semantic_decision_candidates (input_record_id);

-- +goose Down
ALTER TABLE kb.semantic_decision_candidates DROP COLUMN IF EXISTS input_record_id;
