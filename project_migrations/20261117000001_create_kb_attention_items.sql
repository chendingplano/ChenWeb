-- kb.attention_items holds data problems that need a human decision, one row per
-- problem. item_type classifies the problem (validated in server/api/attention, so a
-- new type needs no migration). (item_type, dedupe_key) identifies the problem across
-- all statuses: a repeat updates the row, and a repeat after resolve/dismiss reopens it.
-- See openspec change attention-items-ambiguous-objects.

-- +goose Up
CREATE TABLE IF NOT EXISTS kb.attention_items (
    id               BIGSERIAL   PRIMARY KEY,
    item_type        TEXT        NOT NULL,
    status           TEXT        NOT NULL DEFAULT 'open',
    dedupe_key       TEXT        NOT NULL,
    title            TEXT        NOT NULL DEFAULT '',
    details          JSONB       NOT NULL DEFAULT '{}'::jsonb,
    input_record_id  BIGINT,
    occurrence_count INTEGER     NOT NULL DEFAULT 1,
    reopen_count     INTEGER     NOT NULL DEFAULT 0,
    first_seen       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_by      TEXT,
    resolved_at      TIMESTAMPTZ,
    resolution       TEXT,
    create_time      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modify_time      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_kb_attention_items_status
        CHECK (status IN ('open', 'resolved', 'dismissed'))
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_kb_attention_items_problem
    ON kb.attention_items (item_type, dedupe_key);
CREATE INDEX IF NOT EXISTS idx_kb_attention_items_type_status
    ON kb.attention_items (item_type, status, last_seen DESC);

-- +goose Down
DROP INDEX IF EXISTS kb.idx_kb_attention_items_type_status;
DROP INDEX IF EXISTS kb.uq_kb_attention_items_problem;
DROP TABLE IF EXISTS kb.attention_items;
