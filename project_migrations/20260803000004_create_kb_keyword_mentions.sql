-- +goose Up
-- P3 Track B: keyword lexicon — mention evidence queue (DR16 spec §3.2).
-- Append-only: the observe-mode collector writes one row per candidate
-- keyword mention found in document text. Never updated or deleted.
-- This is the first-class evidence queue for the reconciliation pipeline.

CREATE TABLE IF NOT EXISTS kb.keyword_mentions (
    mention_id      BIGSERIAL PRIMARY KEY,
    artifact_ref    TEXT,
    chunk_ref       TEXT,
    context_text    TEXT,
    ks_id           TEXT,
    create_time     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_keyword_mentions_artifact
    ON kb.keyword_mentions (artifact_ref, chunk_ref);
CREATE INDEX IF NOT EXISTS idx_keyword_mentions_ks_id
    ON kb.keyword_mentions (ks_id);

-- +goose Down
DROP TABLE IF EXISTS kb.keyword_mentions;
