-- +goose Up
-- kb.ontology_active_releases is a history, not a pointer: a release can be
-- activated, deactivated and activated again, which is why
-- 20260731000025 dropped UNIQUE (module_id, release_id). That pair therefore
-- cannot be the natural key for deployment sync. An activation is identified
-- by when it happened as well: (module_id, release_id, activated_at).
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'ontology_active_releases_sync_natural_key'
          AND conrelid = 'kb.ontology_active_releases'::regclass
    ) THEN
        ALTER TABLE kb.ontology_active_releases
            ADD CONSTRAINT ontology_active_releases_sync_natural_key
            UNIQUE (module_id, release_id, activated_at);
    END IF;
END
$$;
-- +goose StatementEnd

-- Replacing a release deactivates the current row and inserts the new one in a
-- single transaction. The deactivation takes the transaction time (the
-- touch_modify_time_if_unchanged trigger uses now()); the new row takes the
-- actual insert time, which is later. A sync pull orders rows by modify_time,
-- so the deactivation is always applied before the activation. With one shared
-- value the order would fall to the release id, and re-activating an older
-- release would apply the new active row first and be rejected, silently, by
-- the one-active-release-per-module index.
ALTER TABLE kb.ontology_active_releases
    ALTER COLUMN modify_time SET DEFAULT clock_timestamp();

-- +goose Down
ALTER TABLE kb.ontology_active_releases
    ALTER COLUMN modify_time SET DEFAULT now();
ALTER TABLE kb.ontology_active_releases
    DROP CONSTRAINT IF EXISTS ontology_active_releases_sync_natural_key;
