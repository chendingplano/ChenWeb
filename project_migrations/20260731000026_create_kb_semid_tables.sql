-- +goose Up
-- Shared canonicalization-kernel tables (ADR DR15): the kernel owns, once,
-- normalizer -> candidates -> scoring -> adjudication -> merge/split -> audit,
-- instantiated per identity family. Families are objects, ontology terms,
-- keywords (P3), and categories (P4); the decision log, never_merge, and
-- snapshots are shared across all of them.
CREATE TABLE IF NOT EXISTS kb.semid_decision_log (
    id             BIGSERIAL PRIMARY KEY,
    family         TEXT NOT NULL,
    scope          TEXT,
    input          JSONB NOT NULL,
    output         JSONB NOT NULL,
    verdict        TEXT NOT NULL,
    model          TEXT,
    prompt_version TEXT,
    actor          TEXT,
    tokens         INT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_kb_semid_decision_log_family_time
    ON kb.semid_decision_log (family, created_at DESC);

CREATE TABLE IF NOT EXISTS kb.semid_never_merge (
    id         BIGSERIAL PRIMARY KEY,
    family     TEXT NOT NULL,
    node_a     TEXT NOT NULL,
    node_b     TEXT NOT NULL,
    reason     TEXT,
    actor      TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (family, node_a, node_b)
);

CREATE TABLE IF NOT EXISTS kb.semid_snapshots (
    id                BIGSERIAL PRIMARY KEY,
    family            TEXT NOT NULL,
    normalizer_version INT NOT NULL,
    counts            JSONB NOT NULL,
    promoted_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- +goose Down
DROP TABLE IF EXISTS kb.semid_decision_log;
DROP TABLE IF EXISTS kb.semid_never_merge;
DROP TABLE IF EXISTS kb.semid_snapshots;
