-- +goose Up
-- Prepares the QUDT-derived `quantity` ontology module for deployment sync
-- (server/api/datasync). Terms, mappings, releases and modules already carry a
-- modify_time that application code bumps on every update, and already have a
-- unique key on their business identity. Two things are missing.
--
-- 1. kb.ontology_term_labels had only its surrogate id. The sync upsert uses
--    ON CONFLICT DO NOTHING, which can only skip a row that violates a unique
--    constraint; without one a repeated pull would insert every label again.
-- +goose StatementBegin
DO $$
DECLARE
    duplicate_groups BIGINT;
BEGIN
    SELECT count(*) INTO duplicate_groups FROM (
        SELECT 1 FROM kb.ontology_term_labels
        GROUP BY term_id, version, lang, label_role, label
        HAVING count(*) > 1
    ) d;
    IF duplicate_groups <> 0 THEN
        RAISE EXCEPTION 'kb.ontology_term_labels has % duplicate (term_id, version, lang, label_role, label) groups; deduplicate them before adding the sync natural key', duplicate_groups;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'ontology_term_labels_sync_natural_key'
          AND conrelid = 'kb.ontology_term_labels'::regclass
    ) THEN
        ALTER TABLE kb.ontology_term_labels
            ADD CONSTRAINT ontology_term_labels_sync_natural_key
            UNIQUE (term_id, version, lang, label_role, label);
    END IF;
END
$$;
-- +goose StatementEnd

-- 2. kb.ontology_active_releases is updated in place when a release is
--    replaced (deactivated_at / deactivated_by), and nothing records when. A
--    target that only ever saw the old row would keep it active and then
--    reject the new active row (one active release per module), so the
--    pointer needs a cursor that moves on deactivation.
--
--    The trigger bumps modify_time only when the UPDATE did not set it. Sync
--    writes the source's modify_time explicitly, and overwriting that with the
--    target's clock could hide a later source change from the next pull.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION kb.touch_modify_time_if_unchanged()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.modify_time IS NOT DISTINCT FROM OLD.modify_time THEN
        NEW.modify_time := now();
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

ALTER TABLE kb.ontology_active_releases
    ADD COLUMN IF NOT EXISTS modify_time TIMESTAMPTZ NOT NULL DEFAULT now();

DROP TRIGGER IF EXISTS trg_kb_ontology_active_releases_touch_modify_time ON kb.ontology_active_releases;
CREATE TRIGGER trg_kb_ontology_active_releases_touch_modify_time
BEFORE UPDATE ON kb.ontology_active_releases
FOR EACH ROW EXECUTE FUNCTION kb.touch_modify_time_if_unchanged();

-- +goose Down
DROP TRIGGER IF EXISTS trg_kb_ontology_active_releases_touch_modify_time ON kb.ontology_active_releases;
ALTER TABLE kb.ontology_active_releases DROP COLUMN IF EXISTS modify_time;
DROP FUNCTION IF EXISTS kb.touch_modify_time_if_unchanged();
ALTER TABLE kb.ontology_term_labels
    DROP CONSTRAINT IF EXISTS ontology_term_labels_sync_natural_key;
