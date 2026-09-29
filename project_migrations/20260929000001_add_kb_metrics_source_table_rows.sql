-- +goose Up
-- openspec change table-row-context: MinerU emits a whole table as one input line, so
-- source_line_spans can only point at the whole table. source_table_rows records which
-- logical rows of that line a metric came from, e.g.
-- [{"line":116,"rows":["r1"],"row_hash":{"r1":"3fa1c09b7e22"}}].
-- source_line_spans / line_range are unchanged. Nullable: NULL means "no row reference"
-- (non-table metric, or not yet backfilled).
ALTER TABLE IF EXISTS kb.metrics
    ADD COLUMN IF NOT EXISTS source_table_rows JSONB;

-- +goose Down
ALTER TABLE IF EXISTS kb.metrics
    DROP COLUMN IF EXISTS source_table_rows;
