-- +goose Up

ALTER TABLE kb.doc_proc_logs
    ADD COLUMN IF NOT EXISTS prompt_cache_hit_tokens  BIGINT,
    ADD COLUMN IF NOT EXISTS prompt_cache_miss_tokens BIGINT;

-- +goose Down

ALTER TABLE kb.doc_proc_logs
    DROP COLUMN IF EXISTS prompt_cache_miss_tokens,
    DROP COLUMN IF EXISTS prompt_cache_hit_tokens;
