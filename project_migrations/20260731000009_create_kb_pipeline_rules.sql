-- +goose Up
CREATE TABLE IF NOT EXISTS kb.pipeline_rules (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(128) NOT NULL,
    priority INT NOT NULL DEFAULT 0,
    match_input_doc_type VARCHAR(128),
    match_source_language VARCHAR(32),
    match_knowledge_store_binding VARCHAR(32),
    pipeline_id BIGINT NOT NULL REFERENCES kb.pipelines(id) ON DELETE RESTRICT,
    active BOOLEAN NOT NULL DEFAULT true,
    create_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modify_time TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- NULL in a match_* column means "wildcard" (matches any facet value), not
-- "no rules match" — so no index can substitute for scanning active rules,
-- but this table is small (authored policy data, not per-document rows).
CREATE INDEX IF NOT EXISTS idx_kb_pipeline_rules_active_priority
    ON kb.pipeline_rules (active, priority DESC);

CREATE INDEX IF NOT EXISTS idx_kb_pipeline_rules_pipeline_id
    ON kb.pipeline_rules (pipeline_id);

-- +goose Down
DROP TABLE IF EXISTS kb.pipeline_rules;
