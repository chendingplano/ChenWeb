-- +goose Up
CREATE SCHEMA IF NOT EXISTS kb;

CREATE TABLE IF NOT EXISTS kb.events (
    id BIGSERIAL PRIMARY KEY,
    event_name VARCHAR(128) NOT NULL,
    event_id VARCHAR(128) NOT NULL UNIQUE,
    event_time TIMESTAMPTZ,
    event_subject TEXT NOT NULL DEFAULT '',
    event_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    event_status JSONB NOT NULL DEFAULT '[]'::jsonb,
    event_source VARCHAR(64) NOT NULL DEFAULT '',
    event_notes TEXT NOT NULL DEFAULT '',
    error_msg TEXT,
    create_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modify_time TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_kb_events_event_name
    ON kb.events(event_name);

CREATE INDEX IF NOT EXISTS idx_kb_events_event_time
    ON kb.events(event_time);

CREATE INDEX IF NOT EXISTS idx_kb_events_create_time
    ON kb.events(create_time);

-- +goose Down
DROP TABLE IF EXISTS kb.events;
