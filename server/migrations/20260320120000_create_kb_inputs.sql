-- +goose Up
-- +goose StatementBegin
CREATE SCHEMA IF NOT EXISTS kb;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS kb.inputs (
    id              BIGSERIAL PRIMARY KEY,
    name            TEXT,
    type            VARCHAR(50)  NOT NULL,
    title           TEXT,
    doc_no          VARCHAR(255),
    source          TEXT,
    file_name       TEXT,
    backup_filename TEXT,
    result_filename TEXT,
    publish_date    DATE,
    authors         TEXT,
    owner           BIGINT,
    status          JSONB        NOT NULL DEFAULT '[]',
    create_time     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    modify_time     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    public_info     JSONB,
    private_info    JSONB,
    notes           TEXT,
    error_msg       TEXT
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS idx_kb_inputs_type        ON kb.inputs (type);
CREATE INDEX IF NOT EXISTS idx_kb_inputs_owner       ON kb.inputs (owner);
CREATE INDEX IF NOT EXISTS idx_kb_inputs_create_time ON kb.inputs (create_time DESC);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS kb.inputs;
-- +goose StatementEnd

-- +goose StatementBegin
DROP SCHEMA IF EXISTS kb;
-- +goose StatementEnd
