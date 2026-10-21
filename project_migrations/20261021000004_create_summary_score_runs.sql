-- +goose Up
CREATE TABLE IF NOT EXISTS kb.summary_score_runs (
    id BIGSERIAL PRIMARY KEY,
    input_record_id BIGINT NOT NULL REFERENCES kb.inputs(id) ON DELETE RESTRICT,
    title TEXT NOT NULL DEFAULT '',
    algorithm TEXT NOT NULL,
    lang TEXT NOT NULL CHECK (lang IN ('en', 'zh-cn')),
    status TEXT NOT NULL CHECK (status IN ('running', 'done', 'failed')),
    model_name TEXT NOT NULL,
    prompt_name TEXT NOT NULL,
    created_by TEXT NOT NULL DEFAULT '',
    request JSONB NOT NULL,
    input_snapshot JSONB,
    judgments_snapshot JSONB,
    score_snapshot JSONB,
    report TEXT NOT NULL DEFAULT '',
    error_msg TEXT NOT NULL DEFAULT '',
    error_code TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS summary_score_runs_history ON kb.summary_score_runs (input_record_id, id DESC);
CREATE UNIQUE INDEX IF NOT EXISTS summary_score_runs_one_running ON kb.summary_score_runs (input_record_id) WHERE status = 'running';

INSERT INTO kb.page_config (page_key, entry_key, language, content, access_role)
VALUES ('development', 'sysadmin-llm-summaries-benchmark', 'en', '{}'::jsonb, '["admin","root"]'::jsonb),
       ('development', 'sysadmin-llm-summaries-benchmark', 'zh-cn', '{}'::jsonb, '["admin","root"]'::jsonb)
ON CONFLICT (page_key, entry_key, language) DO NOTHING;

-- +goose Down
DELETE FROM kb.page_config WHERE page_key = 'development' AND entry_key = 'sysadmin-llm-summaries-benchmark';
DROP INDEX IF EXISTS kb.summary_score_runs_one_running;
DROP INDEX IF EXISTS kb.summary_score_runs_history;
DROP TABLE IF EXISTS kb.summary_score_runs;
