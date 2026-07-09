-- +goose Up
ALTER TABLE kb.object_audit_log
    DROP CONSTRAINT IF EXISTS chk_kb_object_audit_log_action;

ALTER TABLE kb.object_audit_log
    ADD CONSTRAINT chk_kb_object_audit_log_action
        CHECK (action IN ('resolve_object_id', 'edit_fields', 'create_node', 'merge_nodes'));

-- +goose Down
ALTER TABLE kb.object_audit_log
    DROP CONSTRAINT IF EXISTS chk_kb_object_audit_log_action;

ALTER TABLE kb.object_audit_log
    ADD CONSTRAINT chk_kb_object_audit_log_action
        CHECK (action IN ('resolve_object_id', 'edit_fields'));
