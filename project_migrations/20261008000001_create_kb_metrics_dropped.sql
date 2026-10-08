-- +goose Up
-- openspec change metric-row-soft-drop-decision-model: rows extract_metrics sets aside (drop-reason
-- tag, pure-requirement statement kind, open-value decision) are kept here instead of discarded.
-- kb.metrics keeps only live rows, so its readers never see them. row_data is the full enriched row
-- as JSONB so this table does not drift when kb.metrics gains columns. drop_id is
-- <record_id>_drp_<seqno>; dropped rows get no metric_id.
CREATE TABLE IF NOT EXISTS kb.metrics_dropped (
    id              BIGSERIAL    PRIMARY KEY,
    input_record_id BIGINT       NOT NULL,
    drop_id         TEXT         NOT NULL UNIQUE,
    candidate_id    TEXT,
    drop_stage      TEXT         NOT NULL,
    drop_reason     TEXT         NOT NULL,
    decision        JSONB,
    row_data        JSONB        NOT NULL,
    event_id        TEXT,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_kb_metrics_dropped_input_record_id ON kb.metrics_dropped(input_record_id);

-- +goose Down
DROP INDEX IF EXISTS kb.idx_kb_metrics_dropped_input_record_id;
DROP TABLE IF EXISTS kb.metrics_dropped;
