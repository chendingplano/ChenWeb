-- +goose Up
ALTER TABLE llm_balance_snapshot ADD COLUMN IF NOT EXISTS total_spending NUMERIC(20,6) NOT NULL DEFAULT 0;

-- Backfill total_spending for every (account_id, currency_code) chain, starting at the point
-- hourly provider-balance polling began (2026-09-20 12:00 local). Rows captured before that
-- point keep the column default (0).
--
-- Per row, in captured_at order:
--   - entry_kind = 'set-total-spending': total_spending is reset to that row's deposit_amount
--     (the manually-set total).
--   - entry_kind = 'provider_balance': total_spending = previous total_spending + max(0,
--     previous provider_balance's balance_amount - this row's balance_amount). Only
--     provider_balance rows participate in the balance chain; 'deposit' rows are skipped when
--     looking for the previous balance reference.
--   - entry_kind = 'deposit': total_spending carries forward unchanged (a deposit is not spend).
-- +goose StatementBegin
DO $$
DECLARE
    combo RECORD;
    snap RECORD;
    last_provider_balance NUMERIC(20,6);
    running_total NUMERIC(20,6);
    hourly_spend NUMERIC(20,6);
    cutoff TIMESTAMPTZ := '2026-09-20 12:00:00-05';
BEGIN
    FOR combo IN
        SELECT DISTINCT account_id, currency_code FROM llm_balance_snapshot
    LOOP
        running_total := 0;

        -- Seed the chain with the latest provider_balance reading strictly before the cutoff,
        -- if any, so the first in-scope row's spending is computed against a real prior balance.
        SELECT balance_amount INTO last_provider_balance
        FROM llm_balance_snapshot
        WHERE account_id = combo.account_id
          AND currency_code = combo.currency_code
          AND entry_kind = 'provider_balance'
          AND captured_at < cutoff
        ORDER BY captured_at DESC
        LIMIT 1;

        FOR snap IN
            SELECT id, entry_kind, balance_amount, deposit_amount
            FROM llm_balance_snapshot
            WHERE account_id = combo.account_id
              AND currency_code = combo.currency_code
              AND captured_at >= cutoff
            ORDER BY captured_at ASC, id ASC
        LOOP
            IF snap.entry_kind = 'set-total-spending' THEN
                running_total := snap.deposit_amount;
            ELSIF snap.entry_kind = 'provider_balance' THEN
                IF last_provider_balance IS NOT NULL THEN
                    hourly_spend := GREATEST(last_provider_balance - snap.balance_amount, 0);
                    running_total := running_total + hourly_spend;
                END IF;
                last_provider_balance := snap.balance_amount;
            END IF;
            -- entry_kind = 'deposit': running_total unchanged, and it is not used to update
            -- last_provider_balance, so it never becomes a spending-chain reference point.

            UPDATE llm_balance_snapshot SET total_spending = running_total WHERE id = snap.id;
        END LOOP;
    END LOOP;
END $$;
-- +goose StatementEnd

-- +goose Down
ALTER TABLE llm_balance_snapshot DROP COLUMN IF EXISTS total_spending;
