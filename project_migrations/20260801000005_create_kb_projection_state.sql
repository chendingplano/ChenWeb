-- +goose Up
-- Derived-projection provenance (ADR 2026072901 DR8/DR11 seam 7; spec
-- 2026072702 §10.8). Every projection row references its authoritative
-- source record and revision, so staleness is a comparison ("does this
-- projection's authoritative_revision match the source's current
-- revision, and does the materialized value match what a fresh build would
-- produce"), and repair is always deterministic replay from that source --
-- never a hand patch (spec §16.2 item 6, §16.3 item 14).
--
-- One row per (projection_kind, projection_target_table, projection_target_id):
-- the current provenance of the projection currently materialized at that
-- target. Rebuilding overwrites the row rather than accumulating history --
-- the authoritative history lives in the source table (e.g.
-- kb.semantic_assertions' own revision chain), not here.
CREATE TABLE IF NOT EXISTS kb.projection_state (
    id                       BIGSERIAL PRIMARY KEY,
    projection_kind          TEXT NOT NULL,
    projection_target_table  TEXT NOT NULL,
    projection_target_id     TEXT NOT NULL,

    authoritative_table      TEXT NOT NULL,
    authoritative_id         BIGINT NOT NULL,
    authoritative_revision   INT NOT NULL,

    projection_version       INT NOT NULL DEFAULT 1,
    stale                    BOOLEAN NOT NULL DEFAULT FALSE,
    stale_reason             TEXT,

    last_built_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    create_time               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modify_time                TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_kb_projection_state_target
    ON kb.projection_state (projection_kind, projection_target_table, projection_target_id);
CREATE INDEX IF NOT EXISTS idx_kb_projection_state_stale
    ON kb.projection_state (stale) WHERE stale;
CREATE INDEX IF NOT EXISTS idx_kb_projection_state_authoritative
    ON kb.projection_state (authoritative_table, authoritative_id);

-- +goose Down
DROP TABLE IF EXISTS kb.projection_state;
