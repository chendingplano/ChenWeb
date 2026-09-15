-- +goose Up
-- Reusable BEFORE UPDATE trigger function for tables with an `update_time`
-- column that should track the most recent row change. Needed by the
-- production-data-sync feature (openspec/changes/production-data-sync),
-- which uses kb.product_names.update_time as a "changed since" sync cursor --
-- today that column only defaults on INSERT and is never bumped on UPDATE.
-- Written generically so a future table can attach the same trigger.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION kb.set_update_time()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.update_time = NOW();
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS trg_kb_product_names_set_update_time ON kb.product_names;
CREATE TRIGGER trg_kb_product_names_set_update_time
    BEFORE UPDATE ON kb.product_names
    FOR EACH ROW
    EXECUTE FUNCTION kb.set_update_time();

-- +goose Down
-- If a later migration attaches kb.set_update_time() to another table,
-- rolling back only this migration would break that trigger -- same
-- dependent-order caveat as any shared function.
DROP TRIGGER IF EXISTS trg_kb_product_names_set_update_time ON kb.product_names;
DROP FUNCTION IF EXISTS kb.set_update_time();
