-- +goose Up
-- The ADR P2 object-node extension (DR10): ontological_level, identity_scope,
-- external_identifiers, and primary_class_term_id (a DERIVED projection).
-- primary_class_term_id is maintained by classification assertions and
-- project_semantics, both of which arrive in P3 with the assertion store; in
-- P2 the column exists but is only populated when a consumer writes it. The
-- ADR lists both identity_scope and scope_key (DR15.1, chunk D) as the
-- identity-scope columns; both are carried here.
ALTER TABLE kb.object_nodes ADD COLUMN IF NOT EXISTS ontological_level TEXT
    CHECK (ontological_level IN ('individual', 'type', 'collection', 'occurrence', 'concept'));
ALTER TABLE kb.object_nodes ADD COLUMN IF NOT EXISTS identity_scope TEXT;
ALTER TABLE kb.object_nodes ADD COLUMN IF NOT EXISTS external_identifiers JSONB;
ALTER TABLE kb.object_nodes ADD COLUMN IF NOT EXISTS primary_class_term_id TEXT;

CREATE INDEX IF NOT EXISTS idx_kb_object_nodes_ontological_level
    ON kb.object_nodes (ontological_level);
CREATE INDEX IF NOT EXISTS idx_kb_object_nodes_primary_class
    ON kb.object_nodes (primary_class_term_id);

-- +goose Down
ALTER TABLE kb.object_nodes DROP COLUMN IF EXISTS primary_class_term_id;
ALTER TABLE kb.object_nodes DROP COLUMN IF EXISTS external_identifiers;
ALTER TABLE kb.object_nodes DROP COLUMN IF EXISTS identity_scope;
ALTER TABLE kb.object_nodes DROP COLUMN IF EXISTS ontological_level;
