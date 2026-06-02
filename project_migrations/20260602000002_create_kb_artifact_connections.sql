-- +goose Up
CREATE SCHEMA IF NOT EXISTS kb;

-- Canonical cross-artifact connection (edge) table for the Deep Wiki artifact graph.
-- Partitioned by relation_method so the three families (llm | line_overlap | structural)
-- — which have very different volume and lifecycle — live in isolated partitions while
-- sharing one schema and one traversal surface. See
-- KnowledgeStore/Capsules/coding-capsules/deep-wiki/artifact-graph-creation.md
CREATE TABLE IF NOT EXISTS kb.artifact_connections (
    id                 BIGSERIAL,
    input_record_id    BIGINT      NOT NULL REFERENCES kb.inputs(id) ON DELETE CASCADE,
    source_type        TEXT        NOT NULL,
    source_id          TEXT        NOT NULL,
    target_type        TEXT        NOT NULL,
    target_id          TEXT        NOT NULL,
    relation_name      TEXT        NOT NULL,
    relation_method    TEXT        NOT NULL,
    confidence         DOUBLE PRECISION,
    overlap            JSONB,
    provenance         JSONB,
    semantic_signature TEXT,
    extra_info         JSONB,
    create_time        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (relation_method, id),
    UNIQUE (relation_method, source_type, source_id, target_type, target_id, relation_name)
) PARTITION BY LIST (relation_method);

CREATE TABLE IF NOT EXISTS kb.artifact_connections_llm
    PARTITION OF kb.artifact_connections FOR VALUES IN ('llm');
CREATE TABLE IF NOT EXISTS kb.artifact_connections_line_overlap
    PARTITION OF kb.artifact_connections FOR VALUES IN ('line_overlap');
CREATE TABLE IF NOT EXISTS kb.artifact_connections_structural
    PARTITION OF kb.artifact_connections FOR VALUES IN ('structural');
CREATE TABLE IF NOT EXISTS kb.artifact_connections_manual
    PARTITION OF kb.artifact_connections FOR VALUES IN ('manual');

-- Indexes created on the partitioned parent cascade to every partition (current and future).
CREATE INDEX IF NOT EXISTS idx_kb_artifact_connections_record
    ON kb.artifact_connections (input_record_id);
CREATE INDEX IF NOT EXISTS idx_kb_artifact_connections_source
    ON kb.artifact_connections (input_record_id, source_type, source_id);
CREATE INDEX IF NOT EXISTS idx_kb_artifact_connections_target
    ON kb.artifact_connections (input_record_id, target_type, target_id);
CREATE INDEX IF NOT EXISTS idx_kb_artifact_connections_relation
    ON kb.artifact_connections (input_record_id, relation_name);

-- +goose Down
DROP TABLE IF EXISTS kb.artifact_connections_manual;
DROP TABLE IF EXISTS kb.artifact_connections_structural;
DROP TABLE IF EXISTS kb.artifact_connections_line_overlap;
DROP TABLE IF EXISTS kb.artifact_connections_llm;
DROP TABLE IF EXISTS kb.artifact_connections;
