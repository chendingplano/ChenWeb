-- The Product Relation Types review moved into System Admin -> Database Maintenance ->
-- Consistency Check (openspec change product-relation-types), so the standalone nav entry
-- seeded by 20261118000002 is removed. Idempotent.

-- +goose Up
DELETE FROM kb.page_config
 WHERE page_key = 'development' AND entry_key = 'sysadmin-db-product-relation-types';

-- +goose Down
-- Nothing to restore: 20261118000002 recreates the entry if it is re-run from scratch.
SELECT 1;
