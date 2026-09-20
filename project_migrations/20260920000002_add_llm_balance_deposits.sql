-- +goose Up
ALTER TABLE llm_balance_snapshot ADD COLUMN IF NOT EXISTS entry_kind TEXT NOT NULL DEFAULT 'provider_balance';
ALTER TABLE llm_balance_snapshot ADD COLUMN IF NOT EXISTS deposit_amount NUMERIC(20,6) NOT NULL DEFAULT 0;
ALTER TABLE llm_balance_snapshot ADD COLUMN IF NOT EXISTS note TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS llm_balance_snapshot_kind_idx ON llm_balance_snapshot(account_id, entry_kind, captured_at DESC);

-- +goose Down
DROP INDEX IF EXISTS llm_balance_snapshot_kind_idx;
ALTER TABLE llm_balance_snapshot DROP COLUMN IF EXISTS note;
ALTER TABLE llm_balance_snapshot DROP COLUMN IF EXISTS deposit_amount;
ALTER TABLE llm_balance_snapshot DROP COLUMN IF EXISTS entry_kind;
