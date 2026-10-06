-- +goose Up
-- ADR 2026100603 DR2: link a metric row to the provision clause whose criterion
-- it states. Not filled by any processor yet; existing rows stay NULL.
-- References the surrogate kb.provisions.id (prov_id is reused on
-- re-extraction). ON DELETE SET NULL so provision re-extraction neither
-- deletes metrics nor fails. The constraint is added separately because
-- MetricsSQLStore.ensureMetricsTable may already have added the bare column.
ALTER TABLE kb.metrics ADD COLUMN IF NOT EXISTS provision_id BIGINT;

-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'fk_metrics_provision_id'
          AND conrelid = 'kb.metrics'::regclass
    ) THEN
        ALTER TABLE kb.metrics
            ADD CONSTRAINT fk_metrics_provision_id
            FOREIGN KEY (provision_id) REFERENCES kb.provisions (id) ON DELETE SET NULL;
    END IF;
END
$$;
-- +goose StatementEnd

CREATE INDEX IF NOT EXISTS idx_metrics_provision_id
    ON kb.metrics (provision_id) WHERE provision_id IS NOT NULL;

COMMENT ON COLUMN kb.metrics.provision_id IS
    'Provision clause this metric row is a criterion of (ADR 2026100603 DR2). NULL until linked.';

-- +goose Down
DROP INDEX IF EXISTS kb.idx_metrics_provision_id;
ALTER TABLE kb.metrics DROP CONSTRAINT IF EXISTS fk_metrics_provision_id;
ALTER TABLE kb.metrics DROP COLUMN IF EXISTS provision_id;
