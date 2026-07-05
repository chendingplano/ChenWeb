-- +goose Up
-- account_id and profile_id are made nullable so events are always recorded
-- even when the sink cannot resolve the account/profile at call time.
-- Cost-rollup queries should use WHERE account_id IS NOT NULL.

ALTER TABLE llm_usage_event
    ALTER COLUMN account_id DROP NOT NULL,
    ALTER COLUMN profile_id DROP NOT NULL;

-- +goose Down
-- Requires all existing rows to already have non-null values.
UPDATE llm_usage_event SET account_id = '' WHERE account_id IS NULL;
UPDATE llm_usage_event SET profile_id = '' WHERE profile_id IS NULL;
ALTER TABLE llm_usage_event
    ALTER COLUMN account_id SET NOT NULL,
    ALTER COLUMN profile_id SET NOT NULL;
