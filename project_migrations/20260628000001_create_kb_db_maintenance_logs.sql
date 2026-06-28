-- +goose Up

CREATE TABLE IF NOT EXISTS kb.db_maintenance_logs (
    id           BIGSERIAL    PRIMARY KEY,
    operation    TEXT         NOT NULL,
    result_data  JSONB        NOT NULL DEFAULT '{}',
    performed_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_db_maintenance_logs_operation    ON kb.db_maintenance_logs (operation);
CREATE INDEX IF NOT EXISTS idx_db_maintenance_logs_performed_at ON kb.db_maintenance_logs (performed_at DESC);

-- +goose Down

DROP INDEX IF EXISTS idx_db_maintenance_logs_performed_at;
DROP INDEX IF EXISTS idx_db_maintenance_logs_operation;
DROP TABLE IF EXISTS kb.db_maintenance_logs;
