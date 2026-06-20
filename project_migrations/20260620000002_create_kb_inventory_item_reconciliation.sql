-- +goose Up
-- Corpus-level inventory item reconciliation — mirrors the entity reconciliation
-- schema added in 20260617000004, scoped to kb.inventory_items.
--
-- canonical_item_id: the surviving inventory_item_id this row folds into.
--   NULL or = inventory_item_id  => this row is its own canonical head.
ALTER TABLE kb.inventory_items ADD COLUMN IF NOT EXISTS canonical_item_id TEXT;

-- reconcile_status: lifecycle in the item dedup pipeline.
--   'pending'   : not yet reconciled
--   'clustered' : reconciled, is a canonical head
--   'merged'    : folded into canonical_item_id, no longer a head
ALTER TABLE kb.inventory_items ADD COLUMN IF NOT EXISTS reconcile_status TEXT NOT NULL DEFAULT 'pending';

ALTER TABLE kb.inventory_items ADD COLUMN IF NOT EXISTS reconciled_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_kb_inventory_items_canonical_item_id ON kb.inventory_items (canonical_item_id);
CREATE INDEX IF NOT EXISTS idx_kb_inventory_items_reconcile_status  ON kb.inventory_items (reconcile_status);

-- Reuse kb.touch_modify_time() created in 20260617000004.
DROP TRIGGER IF EXISTS trg_kb_inventory_items_touch_modify_time ON kb.inventory_items;
CREATE TRIGGER trg_kb_inventory_items_touch_modify_time
BEFORE UPDATE ON kb.inventory_items
FOR EACH ROW
EXECUTE FUNCTION kb.touch_modify_time();

-- Immutable provenance for applied item merges.
CREATE TABLE IF NOT EXISTS kb.inventory_item_merges (
    merge_id     BIGINT           GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    from_item_id TEXT             NOT NULL,
    into_item_id TEXT             NOT NULL,
    method       TEXT             NOT NULL,
    confidence   DOUBLE PRECISION,
    reason       TEXT,
    evidence     JSONB,
    decided_by   TEXT,
    decided_at   TIMESTAMPTZ      NOT NULL DEFAULT now(),
    undone_at    TIMESTAMPTZ,
    undone_by    TEXT
);
CREATE INDEX IF NOT EXISTS idx_kb_inventory_item_merges_from ON kb.inventory_item_merges (from_item_id);
CREATE INDEX IF NOT EXISTS idx_kb_inventory_item_merges_into ON kb.inventory_item_merges (into_item_id);

-- +goose Down
DROP INDEX  IF EXISTS kb.idx_kb_inventory_item_merges_into;
DROP INDEX  IF EXISTS kb.idx_kb_inventory_item_merges_from;
DROP TABLE  IF EXISTS kb.inventory_item_merges;
DROP TRIGGER IF EXISTS trg_kb_inventory_items_touch_modify_time ON kb.inventory_items;
DROP INDEX  IF EXISTS kb.idx_kb_inventory_items_reconcile_status;
DROP INDEX  IF EXISTS kb.idx_kb_inventory_items_canonical_item_id;
ALTER TABLE kb.inventory_items DROP COLUMN IF EXISTS reconciled_at;
ALTER TABLE kb.inventory_items DROP COLUMN IF EXISTS reconcile_status;
ALTER TABLE kb.inventory_items DROP COLUMN IF EXISTS canonical_item_id;
