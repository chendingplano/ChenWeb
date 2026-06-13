-- +goose Up
-- ADR 2026061302: entity-linked relations (D1) + provisional-entity marking (D4).
ALTER TABLE kb.relations ADD COLUMN IF NOT EXISTS subject_entity_id TEXT;
ALTER TABLE kb.relations ADD COLUMN IF NOT EXISTS object_entity_id TEXT;
ALTER TABLE kb.entities ADD COLUMN IF NOT EXISTS entity_status TEXT NOT NULL DEFAULT 'extracted';

CREATE INDEX IF NOT EXISTS idx_kb_relations_subject_entity_id ON kb.relations (subject_entity_id);
CREATE INDEX IF NOT EXISTS idx_kb_relations_object_entity_id ON kb.relations (object_entity_id);
CREATE INDEX IF NOT EXISTS idx_kb_entities_entity_status ON kb.entities (entity_status);

-- +goose Down
DROP INDEX IF EXISTS kb.idx_kb_entities_entity_status;
DROP INDEX IF EXISTS kb.idx_kb_relations_object_entity_id;
DROP INDEX IF EXISTS kb.idx_kb_relations_subject_entity_id;
ALTER TABLE kb.entities DROP COLUMN IF EXISTS entity_status;
ALTER TABLE kb.relations DROP COLUMN IF EXISTS object_entity_id;
ALTER TABLE kb.relations DROP COLUMN IF EXISTS subject_entity_id;
