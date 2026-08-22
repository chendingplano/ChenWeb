-- +goose Up
-- ADR 2026082203 DR3: generalizes kb.metric_value_range_type_map's existing resolve-or-propose
-- shape (ADR 2026081401) across every class-identity dimension (metric_name, metric_subject,
-- metric_object_name, metric_value_class, ...) instead of growing a new per-field table each time.
-- dimension is a plain discriminator column, not an enum or a term_kind -- which term_kind an
-- approved value's term_id belongs to is deliberately left to whoever approves the row (OD2), not
-- decided by this table or any code that reads it.
--
-- status: 'proposed' (auto-inserted on first sighting, not yet authoritative) / 'approved' (a human
-- confirmed or corrected term_id; used to resolve class-identity signature dimensions) / 'ambiguous'
-- (a human decided no governed term can be inferred from the raw value) / 'rejected' (a human
-- decided this raw value should never resolve, e.g. noise/garbage extraction). No seed data: unlike
-- kb.metric_value_range_type_map's DR5 seed (which preserved an existing hardcoded Go switch's
-- behavior), there is no prior classification behavior for these dimensions to preserve.
CREATE TABLE IF NOT EXISTS kb.governed_property_value_map (
    dimension             TEXT NOT NULL,
    raw_value             TEXT NOT NULL,
    term_id               TEXT REFERENCES kb.ontology_term_headers(term_id),
    status                TEXT NOT NULL DEFAULT 'proposed'
                               CHECK (status IN ('proposed', 'approved', 'ambiguous', 'rejected')),
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

CREATE INDEX IF NOT EXISTS idx_kb_governed_property_value_map_status
    ON kb.governed_property_value_map (dimension, status);

-- +goose Down
DROP TABLE IF EXISTS kb.governed_property_value_map;
