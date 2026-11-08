-- +goose Up
-- Scoring runs of extract_products against the gold product relations (testbed.products),
-- separate from the independently authored gold benchmark runs. Same shape as
-- testbed.provision_score_runs, created in testbed from the start and with llm_run_id
-- (see 20261108000001_shared_llm_run_ids.sql).
CREATE TABLE IF NOT EXISTS testbed.product_score_runs (
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
    llm_run_id BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at TIMESTAMPTZ
);
COMMENT ON COLUMN testbed.product_score_runs.llm_run_id IS 'LLM usage correlation ID; 0 means allocation failed; NULL means legacy execution';
CREATE INDEX IF NOT EXISTS product_score_runs_history ON testbed.product_score_runs (input_record_id, id DESC);
CREATE UNIQUE INDEX IF NOT EXISTS product_score_runs_one_running ON testbed.product_score_runs (input_record_id) WHERE status = 'running';

-- System Admin > Benchmark > Products on the development page: Gold Products and Benchmark,
-- admin/root only like the other benchmark entries. A missing row would make an entry visible to all.
INSERT INTO kb.page_config (page_key, entry_key, language, content, access_role)
VALUES ('development', 'sysadmin-llm-products-gold', 'en', '{}'::jsonb, '["admin","root"]'::jsonb),
       ('development', 'sysadmin-llm-products-gold', 'zh-cn', '{}'::jsonb, '["admin","root"]'::jsonb),
       ('development', 'sysadmin-llm-products-benchmark', 'en', '{}'::jsonb, '["admin","root"]'::jsonb),
       ('development', 'sysadmin-llm-products-benchmark', 'zh-cn', '{}'::jsonb, '["admin","root"]'::jsonb)
ON CONFLICT (page_key, entry_key, language) DO NOTHING;

-- +goose Down
DELETE FROM kb.page_config WHERE page_key = 'development'
  AND entry_key IN ('sysadmin-llm-products-gold', 'sysadmin-llm-products-benchmark');
DROP INDEX IF EXISTS testbed.product_score_runs_one_running;
DROP INDEX IF EXISTS testbed.product_score_runs_history;
DROP TABLE IF EXISTS testbed.product_score_runs;
