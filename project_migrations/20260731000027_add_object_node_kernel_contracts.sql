-- +goose Up
-- DR15.1 kernel contracts on the live object-node family: the columns exist,
-- but the object-node reconciler is NOT rewritten -- it adopts the kernel's
-- contracts (explicit scope, tombstone merges) while keeping its current
-- scorer and tie-break behavior until kernel fixtures prove parity (ADR test
-- 23). merged_into is the tombstone pointer; scope_key is the identity scope
-- (knowledge store, plant, org).
ALTER TABLE kb.object_nodes ADD COLUMN IF NOT EXISTS merged_into TEXT;
ALTER TABLE kb.object_nodes ADD COLUMN IF NOT EXISTS scope_key TEXT;

CREATE INDEX IF NOT EXISTS idx_kb_object_nodes_merged_into
    ON kb.object_nodes (merged_into);
CREATE INDEX IF NOT EXISTS idx_kb_object_nodes_scope_key
    ON kb.object_nodes (scope_key);

-- +goose Down
ALTER TABLE kb.object_nodes DROP COLUMN IF EXISTS merged_into;
ALTER TABLE kb.object_nodes DROP COLUMN IF EXISTS scope_key;
