-- +goose Up
CREATE SCHEMA IF NOT EXISTS kb;

CREATE TABLE IF NOT EXISTS kb.chad_sessions (
    id           BIGSERIAL PRIMARY KEY,
    session_id   TEXT NOT NULL UNIQUE,
    title        TEXT NOT NULL DEFAULT '',
    updated      DOUBLE PRECISION NOT NULL DEFAULT 0,
    turns        INTEGER NOT NULL DEFAULT 0,
    create_time  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    directory    TEXT NOT NULL,
    chad_version TEXT NOT NULL DEFAULT '',
    model_name   TEXT NOT NULL DEFAULT '',
    mode         TEXT NOT NULL DEFAULT '',
    meta         JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX IF NOT EXISTS idx_chad_sessions_updated
    ON kb.chad_sessions (updated DESC);

CREATE INDEX IF NOT EXISTS idx_chad_sessions_directory
    ON kb.chad_sessions (directory);

CREATE TABLE IF NOT EXISTS kb.chad_messages (
    id                    BIGSERIAL PRIMARY KEY,
    session_id            TEXT NOT NULL REFERENCES kb.chad_sessions(session_id) ON DELETE CASCADE,
    message_index         INTEGER NOT NULL,
    role                  TEXT NOT NULL DEFAULT '',
    message_json          JSONB NOT NULL,
    content_text          TEXT NOT NULL DEFAULT '',
    tool_call_command     TEXT NOT NULL DEFAULT '',
    tool_call_parameters  JSONB,
    create_time           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (session_id, message_index)
);

CREATE INDEX IF NOT EXISTS idx_chad_messages_session
    ON kb.chad_messages (session_id, message_index);

-- +goose Down
DROP TABLE IF EXISTS kb.chad_messages;
DROP TABLE IF EXISTS kb.chad_sessions;
