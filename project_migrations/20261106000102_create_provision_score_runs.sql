-- +goose Up
-- Scoring runs of extract_provisions against the gold provisions (testbed.provisions), separate
-- from the independently authored gold benchmark runs. Same shape as kb.reference_score_runs.
CREATE TABLE IF NOT EXISTS kb.provision_score_runs (
    id BIGSERIAL PRIMARY KEY,
    input_record_id BIGINT NOT NULL REFERENCES kb.inputs(id) ON DELETE RESTRICT,
    title TEXT NOT NULL DEFAULT '',
    lang TEXT NOT NULL CHECK (lang IN ('en', 'zh-cn')),
    status TEXT NOT NULL CHECK (status IN ('running', 'done', 'failed')),
    model_name TEXT NOT NULL,
    prompt_name TEXT NOT NULL,
    created_by TEXT NOT NULL DEFAULT '',
    request JSONB NOT NULL,
    input_snapshot JSONB,
    matches_snapshot JSONB,
    score_snapshot JSONB,
    report TEXT NOT NULL DEFAULT '',
    error_msg TEXT NOT NULL DEFAULT '',
    error_code TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS provision_score_runs_history ON kb.provision_score_runs (input_record_id, id DESC);
CREATE UNIQUE INDEX IF NOT EXISTS provision_score_runs_one_running ON kb.provision_score_runs (input_record_id) WHERE status = 'running';

-- System Admin > Benchmark > Provisions on the development page: Gold Provisions and Benchmark,
-- admin/root only like the other benchmark entries. A missing row would make an entry visible to all.
INSERT INTO kb.page_config (page_key, entry_key, language, content, access_role)
VALUES ('development', 'sysadmin-llm-provisions-gold', 'en', '{}'::jsonb, '["admin","root"]'::jsonb),
       ('development', 'sysadmin-llm-provisions-gold', 'zh-cn', '{}'::jsonb, '["admin","root"]'::jsonb),
       ('development', 'sysadmin-llm-provisions-benchmark', 'en', '{}'::jsonb, '["admin","root"]'::jsonb),
       ('development', 'sysadmin-llm-provisions-benchmark', 'zh-cn', '{}'::jsonb, '["admin","root"]'::jsonb)
ON CONFLICT (page_key, entry_key, language) DO NOTHING;

-- +goose Down
DELETE FROM kb.page_config WHERE page_key = 'development'
  AND entry_key IN ('sysadmin-llm-provisions-gold', 'sysadmin-llm-provisions-benchmark');
DROP INDEX IF EXISTS kb.provision_score_runs_one_running;
DROP INDEX IF EXISTS kb.provision_score_runs_history;
DROP TABLE IF EXISTS kb.provision_score_runs;
