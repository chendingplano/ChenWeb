-- +goose Up
-- Relations between semantic assertions -- conflict and supersession (ADR
-- 2026072901 DR8; spec 2026072702 §8.3: "Assertions must survive
-- processor-specific index rebuilds and support adjudication, supersession,
-- and conflict relationships.") A pair may be related more than one way over
-- time but not duplicated for the same relation_kind.
CREATE TABLE IF NOT EXISTS kb.assertion_relations (
    id                    BIGSERIAL PRIMARY KEY,
    assertion_id          BIGINT NOT NULL REFERENCES kb.semantic_assertions(id),
    related_assertion_id  BIGINT NOT NULL REFERENCES kb.semantic_assertions(id),
    relation_kind         TEXT NOT NULL CHECK (relation_kind IN ('conflicts_with', 'supersedes', 'superseded_by')),
    rationale             TEXT,
    create_time           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    create_by             TEXT,

    CONSTRAINT chk_assertion_relations_not_self CHECK (assertion_id <> related_assertion_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_kb_assertion_relations_pair
    ON kb.assertion_relations (assertion_id, related_assertion_id, relation_kind);
CREATE INDEX IF NOT EXISTS idx_kb_assertion_relations_related
    ON kb.assertion_relations (related_assertion_id);

-- +goose Down
DROP TABLE IF EXISTS kb.assertion_relations;
