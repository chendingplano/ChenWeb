-- +goose Up
-- Rolling summary of earlier Knowledge Desk turns. It covers every complete
-- turn whose user message has sequence_no <= history_summary_through_seq.
ALTER TABLE kb.agentic_conversations ADD COLUMN IF NOT EXISTS history_summary TEXT NOT NULL DEFAULT '';
ALTER TABLE kb.agentic_conversations ADD COLUMN IF NOT EXISTS history_summary_through_seq INTEGER NOT NULL DEFAULT 0;
ALTER TABLE kb.agentic_conversations ADD COLUMN IF NOT EXISTS history_summary_updated_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE kb.agentic_conversations DROP COLUMN IF EXISTS history_summary_updated_at;
ALTER TABLE kb.agentic_conversations DROP COLUMN IF EXISTS history_summary_through_seq;
ALTER TABLE kb.agentic_conversations DROP COLUMN IF EXISTS history_summary;
