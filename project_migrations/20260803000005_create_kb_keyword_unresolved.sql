-- +goose Up
-- P3 Track B: keyword lexicon — unresolved backlog (DR16 spec §3.2).
-- Tracks surface forms that could not be resolved to a keyword concept.
-- Keyed on (norm_key, scope) — the natural dedup unit. Accumulates
-- distinct raw surfaces (capped) and context snippets (reservoir sample)
-- for later batch adjudication. Negative-cached: an unchanged item is
-- never re-sent to a model (last_attempt).

CREATE TABLE IF NOT EXISTS kb.keyword_unresolved (
    norm_key        TEXT NOT NULL,
    scope           TEXT NOT NULL DEFAULT '_',
    surfaces        JSONB NOT NULL DEFAULT '[]'::jsonb,
    contexts        JSONB,
    hits            INT NOT NULL DEFAULT 1,
    status          TEXT NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'batched', 'needs_human',
                          'resolved', 'junk', 'insufficient_context')),
    attempts        INT NOT NULL DEFAULT 0,
    last_attempt    TEXT,
    priority        DOUBLE PRECISION NOT NULL DEFAULT 0,
    first_seen      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (norm_key, scope)
);

CREATE INDEX IF NOT EXISTS idx_keyword_unresolved_status
    ON kb.keyword_unresolved (status, last_seen);

-- +goose Down
DROP TABLE IF EXISTS kb.keyword_unresolved;
