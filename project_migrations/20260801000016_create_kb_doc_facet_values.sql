-- +goose Up
CREATE TABLE IF NOT EXISTS kb.doc_facet_values (
    id BIGSERIAL PRIMARY KEY,
    record_id BIGINT NOT NULL REFERENCES kb.inputs(id) ON DELETE CASCADE,
    path TEXT NOT NULL,
    value JSONB,
    state TEXT NOT NULL DEFAULT 'known'
        CHECK (state IN ('known', 'missing', 'conflicting', 'invalid')),
    method TEXT NOT NULL
        CHECK (method IN ('deterministic', 'metadata', 'classifier')),
    confidence DOUBLE PRECISION,
    evidence JSONB NOT NULL DEFAULT '{}'::jsonb,
    source_fingerprint TEXT NOT NULL DEFAULT '',
    decision_attempt_id TEXT NOT NULL DEFAULT '',
    invocation_id TEXT NOT NULL DEFAULT '',
    vocabulary_release_id BIGINT NOT NULL DEFAULT 0,
    create_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modify_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (record_id, path, decision_attempt_id, invocation_id)
);

CREATE INDEX IF NOT EXISTS idx_kb_doc_facet_values_record_release
    ON kb.doc_facet_values (record_id, vocabulary_release_id, path);

CREATE INDEX IF NOT EXISTS idx_kb_doc_facet_values_path_method
    ON kb.doc_facet_values (path, method, create_time DESC);

-- +goose Down
DROP TABLE IF EXISTS kb.doc_facet_values;
