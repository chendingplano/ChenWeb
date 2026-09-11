-- +goose Up
DROP TABLE IF EXISTS kb.chad_messages;
DROP TABLE IF EXISTS kb.chad_sessions;

CREATE TABLE IF NOT EXISTS kb.harness_sessions (
    id           BIGSERIAL PRIMARY KEY,
    harness_name TEXT NOT NULL,
    session_id   TEXT NOT NULL,
    title        TEXT NOT NULL DEFAULT '',
    updated      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    turns        INTEGER NOT NULL DEFAULT 0,
    create_time  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    directory    TEXT NOT NULL,
    harness_version TEXT NOT NULL DEFAULT '',
    model_name   TEXT NOT NULL DEFAULT '',
    mode         TEXT NOT NULL DEFAULT '',
    meta         JSONB NOT NULL DEFAULT '{}'::jsonb,
    UNIQUE (harness_name, session_id)
);

CREATE INDEX IF NOT EXISTS idx_harness_sessions_updated
    ON kb.harness_sessions (updated DESC);
CREATE INDEX IF NOT EXISTS idx_harness_sessions_harness
    ON kb.harness_sessions (harness_name, updated DESC);

CREATE TABLE IF NOT EXISTS kb.harness_messages (
    id                    BIGSERIAL PRIMARY KEY,
    harness_name          TEXT NOT NULL,
    session_id            TEXT NOT NULL,
    message_index         INTEGER NOT NULL,
    role                  TEXT NOT NULL DEFAULT '',
    message_json          JSONB NOT NULL,
    content_text          TEXT NOT NULL DEFAULT '',
    tool_call_command     TEXT NOT NULL DEFAULT '',
    tool_call_parameters  JSONB,
    create_time           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (harness_name, session_id, message_index),
    FOREIGN KEY (harness_name, session_id)
        REFERENCES kb.harness_sessions(harness_name, session_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_harness_messages_session
    ON kb.harness_messages (harness_name, session_id, message_index);

-- +goose Down
DROP TABLE IF EXISTS kb.harness_messages;
DROP TABLE IF EXISTS kb.harness_sessions;
