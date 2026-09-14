-- +goose Up
CREATE TABLE IF NOT EXISTS kb.agentic_conversations (
    id              VARCHAR(64) PRIMARY KEY DEFAULT gen_random_uuid()::text,
    owner_user_id   TEXT NOT NULL,
    service_slug    TEXT NOT NULL,
    profile_slug    TEXT NOT NULL,
    profile_version TEXT NOT NULL,
    model_name      TEXT NOT NULL,
    title           TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'active'
                    CHECK (status IN ('active', 'completed')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_agentic_conversations_owner_updated
    ON kb.agentic_conversations (owner_user_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS kb.agentic_response_attempts (
    id              VARCHAR(64) PRIMARY KEY DEFAULT gen_random_uuid()::text,
    conversation_id VARCHAR(64) NOT NULL REFERENCES kb.agentic_conversations(id) ON DELETE CASCADE,
    idempotency_key TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'running'
                    CHECK (status IN ('running', 'completed', 'stopped', 'failed', 'interrupted', 'limit')),
    error_code      TEXT NOT NULL DEFAULT '',
    error_message   TEXT NOT NULL DEFAULT '',
    input_tokens    BIGINT NOT NULL DEFAULT 0 CHECK (input_tokens >= 0),
    output_tokens   BIGINT NOT NULL DEFAULT 0 CHECK (output_tokens >= 0),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at    TIMESTAMPTZ,
    UNIQUE (conversation_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_agentic_attempts_conversation_created
    ON kb.agentic_response_attempts (conversation_id, created_at);

CREATE TABLE IF NOT EXISTS kb.agentic_messages (
    id              VARCHAR(64) PRIMARY KEY DEFAULT gen_random_uuid()::text,
    conversation_id VARCHAR(64) NOT NULL REFERENCES kb.agentic_conversations(id) ON DELETE CASCADE,
    attempt_id      VARCHAR(64) REFERENCES kb.agentic_response_attempts(id) ON DELETE CASCADE,
    sequence_no     INTEGER NOT NULL CHECK (sequence_no > 0),
    role            TEXT NOT NULL CHECK (role IN ('user', 'assistant', 'system')),
    content         TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'complete'
                    CHECK (status IN ('streaming', 'complete', 'incomplete')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (conversation_id, sequence_no),
    UNIQUE (id, attempt_id)
);

CREATE INDEX IF NOT EXISTS idx_agentic_messages_conversation_sequence
    ON kb.agentic_messages (conversation_id, sequence_no);

CREATE TABLE IF NOT EXISTS kb.agentic_tool_calls (
    id          VARCHAR(64) PRIMARY KEY DEFAULT gen_random_uuid()::text,
    attempt_id  VARCHAR(64) NOT NULL REFERENCES kb.agentic_response_attempts(id) ON DELETE CASCADE,
    tool_name   TEXT NOT NULL,
    status      TEXT NOT NULL CHECK (status IN ('running', 'complete', 'failed', 'stopped')),
    input_summary  JSONB NOT NULL DEFAULT '{}'::jsonb,
    output_summary JSONB NOT NULL DEFAULT '{}'::jsonb,
    duration_ms BIGINT NOT NULL DEFAULT 0 CHECK (duration_ms >= 0),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (id, attempt_id)
);

CREATE INDEX IF NOT EXISTS idx_agentic_tool_calls_attempt_created
    ON kb.agentic_tool_calls (attempt_id, created_at);

CREATE TABLE IF NOT EXISTS kb.agentic_sources (
    id             BIGSERIAL PRIMARY KEY,
    attempt_id     VARCHAR(64) NOT NULL REFERENCES kb.agentic_response_attempts(id) ON DELETE CASCADE,
    message_id     VARCHAR(64) NOT NULL,
    tool_call_id   VARCHAR(64) NOT NULL,
    document_id    TEXT NOT NULL,
    document_title TEXT NOT NULL DEFAULT '',
    source_version TEXT NOT NULL DEFAULT '',
    source_fingerprint TEXT NOT NULL CHECK (BTRIM(source_fingerprint) <> ''),
    artifact_type  TEXT NOT NULL DEFAULT '',
    artifact_id    TEXT NOT NULL DEFAULT '',
    line_start     INTEGER NOT NULL DEFAULT 0 CHECK (line_start >= 0),
    line_end       INTEGER NOT NULL DEFAULT 0 CHECK (line_end >= line_start),
    page_start     INTEGER NOT NULL DEFAULT 0 CHECK (page_start >= 0),
    page_end       INTEGER NOT NULL DEFAULT 0 CHECK (page_end >= page_start),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (tool_call_id, attempt_id)
        REFERENCES kb.agentic_tool_calls(id, attempt_id) ON DELETE CASCADE,
    FOREIGN KEY (message_id, attempt_id)
        REFERENCES kb.agentic_messages(id, attempt_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_agentic_sources_attempt
    ON kb.agentic_sources (attempt_id, id);

CREATE TABLE IF NOT EXISTS kb.agentic_feedback (
    id            VARCHAR(64) PRIMARY KEY DEFAULT gen_random_uuid()::text,
    message_id    VARCHAR(64) NOT NULL REFERENCES kb.agentic_messages(id) ON DELETE CASCADE,
    owner_user_id TEXT NOT NULL,
    rating        TEXT NOT NULL CHECK (rating IN ('helpful', 'unhelpful')),
    comment       TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (message_id, owner_user_id)
);

CREATE INDEX IF NOT EXISTS idx_agentic_feedback_owner_created
    ON kb.agentic_feedback (owner_user_id, created_at DESC);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION kb.touch_agentic_conversation() RETURNS TRIGGER AS $$
DECLARE
    target_conversation_id VARCHAR(64);
BEGIN
    IF TG_TABLE_NAME = 'agentic_feedback' THEN
        SELECT conversation_id INTO target_conversation_id
        FROM kb.agentic_messages WHERE id = NEW.message_id;
    ELSIF TG_TABLE_NAME = 'agentic_tool_calls' THEN
        SELECT conversation_id INTO target_conversation_id
        FROM kb.agentic_response_attempts WHERE id = NEW.attempt_id;
    ELSIF TG_TABLE_NAME = 'agentic_sources' THEN
        SELECT conversation_id INTO target_conversation_id
        FROM kb.agentic_messages WHERE id = NEW.message_id;
    ELSE
        target_conversation_id := NEW.conversation_id;
    END IF;
    UPDATE kb.agentic_conversations SET updated_at = NOW()
    WHERE id = target_conversation_id;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS trg_touch_agentic_conversation_messages ON kb.agentic_messages;
CREATE TRIGGER trg_touch_agentic_conversation_messages
AFTER INSERT OR UPDATE ON kb.agentic_messages
FOR EACH ROW EXECUTE FUNCTION kb.touch_agentic_conversation();
DROP TRIGGER IF EXISTS trg_touch_agentic_conversation_attempts ON kb.agentic_response_attempts;
CREATE TRIGGER trg_touch_agentic_conversation_attempts
AFTER INSERT OR UPDATE ON kb.agentic_response_attempts
FOR EACH ROW EXECUTE FUNCTION kb.touch_agentic_conversation();
DROP TRIGGER IF EXISTS trg_touch_agentic_conversation_tool_calls ON kb.agentic_tool_calls;
CREATE TRIGGER trg_touch_agentic_conversation_tool_calls
AFTER INSERT OR UPDATE ON kb.agentic_tool_calls
FOR EACH ROW EXECUTE FUNCTION kb.touch_agentic_conversation();
DROP TRIGGER IF EXISTS trg_touch_agentic_conversation_sources ON kb.agentic_sources;
CREATE TRIGGER trg_touch_agentic_conversation_sources
AFTER INSERT OR UPDATE ON kb.agentic_sources
FOR EACH ROW EXECUTE FUNCTION kb.touch_agentic_conversation();
DROP TRIGGER IF EXISTS trg_touch_agentic_conversation_feedback ON kb.agentic_feedback;
CREATE TRIGGER trg_touch_agentic_conversation_feedback
AFTER INSERT OR UPDATE ON kb.agentic_feedback
FOR EACH ROW EXECUTE FUNCTION kb.touch_agentic_conversation();

-- +goose Down
DROP TRIGGER IF EXISTS trg_touch_agentic_conversation_feedback ON kb.agentic_feedback;
DROP TRIGGER IF EXISTS trg_touch_agentic_conversation_sources ON kb.agentic_sources;
DROP TRIGGER IF EXISTS trg_touch_agentic_conversation_tool_calls ON kb.agentic_tool_calls;
DROP TRIGGER IF EXISTS trg_touch_agentic_conversation_attempts ON kb.agentic_response_attempts;
DROP TRIGGER IF EXISTS trg_touch_agentic_conversation_messages ON kb.agentic_messages;
DROP FUNCTION IF EXISTS kb.touch_agentic_conversation();
DROP INDEX IF EXISTS kb.idx_agentic_feedback_owner_created;
DROP TABLE IF EXISTS kb.agentic_feedback;
DROP INDEX IF EXISTS kb.idx_agentic_sources_attempt;
DROP TABLE IF EXISTS kb.agentic_sources;
DROP INDEX IF EXISTS kb.idx_agentic_tool_calls_attempt_created;
DROP TABLE IF EXISTS kb.agentic_tool_calls;
DROP INDEX IF EXISTS kb.idx_agentic_messages_conversation_sequence;
DROP TABLE IF EXISTS kb.agentic_messages;
DROP INDEX IF EXISTS kb.idx_agentic_attempts_conversation_created;
DROP TABLE IF EXISTS kb.agentic_response_attempts;
DROP INDEX IF EXISTS kb.idx_agentic_conversations_owner_updated;
DROP TABLE IF EXISTS kb.agentic_conversations;
