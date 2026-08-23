-- +goose Up
-- openspec change governed-property-normalization: a plain-string resolve-or-propose
-- bucket map, structured identically to kb.metric_value_range_type_map (ADR 2026081401)
-- but dimension-keyed so more than one closed-vocabulary classification field can
-- share it without a schema change. canonical_bucket is a plain TEXT, not a term_id --
-- deliberately: value_type/value_class-style fields are closed-ish classification
-- labels, not open-vocabulary entities that warrant their own governed-term catalog
-- identity (unlike kb.governed_property_value_map, which this table does not replace
-- or touch). Today only dimension='value_type' is populated -- value_class's raw
-- values were checked against the live corpus and found too clean (3 distinct values)
-- to need curation; it resolves via the "simple" method (semid.Normalizer) instead.
-- No seed data: no prior hardcoded classification exists for value_type to preserve.
CREATE TABLE IF NOT EXISTS kb.metric_value_bucket_map (
    dimension             TEXT NOT NULL,
    raw_value             TEXT NOT NULL,
    canonical_bucket      TEXT,
    status                TEXT NOT NULL DEFAULT 'proposed'
                               CHECK (status IN ('proposed', 'approved', 'ambiguous')),
    occurrence_count      BIGINT NOT NULL DEFAULT 0,
    first_seen_record_id  BIGINT,
    last_seen_record_id   BIGINT,
    note                  TEXT,
    create_time           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    create_by             TEXT,
    modify_time           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modify_by             TEXT,
    PRIMARY KEY (dimension, raw_value)
);

CREATE INDEX IF NOT EXISTS idx_kb_metric_value_bucket_map_status
    ON kb.metric_value_bucket_map (dimension, status);

-- +goose Down
DROP TABLE IF EXISTS kb.metric_value_bucket_map;
