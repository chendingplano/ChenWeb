-- +goose Up
CREATE TABLE IF NOT EXISTS kb.object_audit_log (
    id          BIGSERIAL    PRIMARY KEY,
    table_name  TEXT         NOT NULL,
    row_key     TEXT         NOT NULL,
    action      TEXT         NOT NULL,
    changes     JSONB        NOT NULL,
    actor       TEXT,
    create_time TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_kb_object_audit_log_table_name
        CHECK (table_name IN ('kb.artifact_objects', 'kb.object_nodes')),
    CONSTRAINT chk_kb_object_audit_log_action
        CHECK (action IN ('resolve_object_id', 'edit_fields'))
);

CREATE INDEX IF NOT EXISTS idx_object_audit_log_row  ON kb.object_audit_log (table_name, row_key);
CREATE INDEX IF NOT EXISTS idx_object_audit_log_time ON kb.object_audit_log (create_time);

-- +goose Down
DROP TABLE IF EXISTS kb.object_audit_log;
