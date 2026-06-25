-- +goose Up

ALTER TABLE llm_usage_event
    ADD COLUMN IF NOT EXISTS prompt_cache_hit_tokens BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS prompt_cache_miss_tokens BIGINT NOT NULL DEFAULT 0;

-- +goose Down

ALTER TABLE llm_usage_event
    DROP COLUMN IF EXISTS prompt_cache_miss_tokens,
    DROP COLUMN IF EXISTS prompt_cache_hit_tokens;
