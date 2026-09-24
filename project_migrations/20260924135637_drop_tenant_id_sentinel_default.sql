-- +goose Up
-- The literal string '-' was used as a "no tenant assigned yet" sentinel for
-- kb.inputs.tenant_id and kb.knowledge_store.tenant_id. It is not NULL, so a
-- naive "is it set" check missed it, letting unattributed records slip
-- through undetected (see devdoc 2026092403). Application code now leaves
-- tenant_id NULL/empty instead of writing the sentinel, and treats NULL,
-- empty, and (for any row written before this migration) '-' as unset.
ALTER TABLE kb.inputs
    ALTER COLUMN tenant_id DROP DEFAULT,
    ALTER COLUMN tenant_id DROP NOT NULL;

ALTER TABLE kb.knowledge_store
    ALTER COLUMN tenant_id DROP DEFAULT,
    ALTER COLUMN tenant_id DROP NOT NULL;

-- +goose Down
ALTER TABLE kb.inputs
    ALTER COLUMN tenant_id SET DEFAULT '-',
    ALTER COLUMN tenant_id SET NOT NULL;

ALTER TABLE kb.knowledge_store
    ALTER COLUMN tenant_id SET DEFAULT '-',
    ALTER COLUMN tenant_id SET NOT NULL;
