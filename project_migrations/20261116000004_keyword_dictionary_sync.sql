-- +goose Up
-- Makes the keyword dictionary itself syncable, so a deployment mirrors the
-- dev machine's dictionary instead of rebuilding it:
--   kb.keyword_concepts, kb.keyword_surfaces       the dictionary rows
--   kb.keyword_surface_keys                        derived lookup keys of a surface
--   kb.keyword_rewrite_rules                       normalization rules
--   kb.keyword_external_ids, kb.keyword_surface_evidence
--                                                  identity mappings from external sources
-- A sync item needs a natural key (a UNIQUE constraint other than the primary
-- key) and a timestamp cursor that moves on every update.

-- 1. Natural keys. concept_id, surface_id and rule_id are stable text ids (content
--    hashes / chosen ids), never database surrogates, so each key repeats the
--    primary key. kb.keyword_surface_evidence already has a suitable UNIQUE.
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint
                   WHERE conname = 'keyword_concepts_sync_natural_key'
                     AND conrelid = 'kb.keyword_concepts'::regclass) THEN
        ALTER TABLE kb.keyword_concepts
            ADD CONSTRAINT keyword_concepts_sync_natural_key UNIQUE (concept_id);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint
                   WHERE conname = 'keyword_surfaces_sync_natural_key'
                     AND conrelid = 'kb.keyword_surfaces'::regclass) THEN
        ALTER TABLE kb.keyword_surfaces
            ADD CONSTRAINT keyword_surfaces_sync_natural_key UNIQUE (surface_id);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint
                   WHERE conname = 'keyword_surface_keys_sync_natural_key'
                     AND conrelid = 'kb.keyword_surface_keys'::regclass) THEN
        ALTER TABLE kb.keyword_surface_keys
            ADD CONSTRAINT keyword_surface_keys_sync_natural_key UNIQUE (surface_id, key_kind);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint
                   WHERE conname = 'keyword_rewrite_rules_sync_natural_key'
                     AND conrelid = 'kb.keyword_rewrite_rules'::regclass) THEN
        ALTER TABLE kb.keyword_rewrite_rules
            ADD CONSTRAINT keyword_rewrite_rules_sync_natural_key UNIQUE (rule_id);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint
                   WHERE conname = 'keyword_external_ids_sync_natural_key'
                     AND conrelid = 'kb.keyword_external_ids'::regclass) THEN
        ALTER TABLE kb.keyword_external_ids
            ADD CONSTRAINT keyword_external_ids_sync_natural_key UNIQUE (source, external_id, release);
    END IF;
END
$$;
-- +goose StatementEnd

-- 2. Cursors. concepts, surfaces and rewrite rules already have modify_time, and
--    nearly every update sets it; the trigger below covers any path that does
--    not, because a change that does not move the cursor never reaches a
--    deployment. The trigger bumps modify_time only when the UPDATE left it
--    alone: sync writes the source's value explicitly and must keep it.
--    (kb.touch_modify_time_if_unchanged() comes from 20261116000002.)
DROP TRIGGER IF EXISTS trg_kb_keyword_concepts_touch_modify_time ON kb.keyword_concepts;
CREATE TRIGGER trg_kb_keyword_concepts_touch_modify_time
BEFORE UPDATE ON kb.keyword_concepts
FOR EACH ROW EXECUTE FUNCTION kb.touch_modify_time_if_unchanged();

DROP TRIGGER IF EXISTS trg_kb_keyword_surfaces_touch_modify_time ON kb.keyword_surfaces;
CREATE TRIGGER trg_kb_keyword_surfaces_touch_modify_time
BEFORE UPDATE ON kb.keyword_surfaces
FOR EACH ROW EXECUTE FUNCTION kb.touch_modify_time_if_unchanged();

DROP TRIGGER IF EXISTS trg_kb_keyword_rewrite_rules_touch_modify_time ON kb.keyword_rewrite_rules;
CREATE TRIGGER trg_kb_keyword_rewrite_rules_touch_modify_time
BEFORE UPDATE ON kb.keyword_rewrite_rules
FOR EACH ROW EXECUTE FUNCTION kb.touch_modify_time_if_unchanged();

--    kb.keyword_surface_keys has no timestamp. The application never edits a key
--    row: it deletes all keys of a surface and inserts them again
--    (SurfaceKeyStore), so an insert time is a complete cursor. clock_timestamp()
--    is volatile, so existing rows are rewritten with their own value.
ALTER TABLE kb.keyword_surface_keys
    ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp();

--    kb.keyword_external_ids and kb.keyword_surface_evidence have no timestamp
--    either and can be updated, so they get modify_time plus the trigger.
ALTER TABLE kb.keyword_external_ids
    ADD COLUMN IF NOT EXISTS modify_time TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp();
DROP TRIGGER IF EXISTS trg_kb_keyword_external_ids_touch_modify_time ON kb.keyword_external_ids;
CREATE TRIGGER trg_kb_keyword_external_ids_touch_modify_time
BEFORE UPDATE ON kb.keyword_external_ids
FOR EACH ROW EXECUTE FUNCTION kb.touch_modify_time_if_unchanged();

ALTER TABLE kb.keyword_surface_evidence
    ADD COLUMN IF NOT EXISTS modify_time TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp();
DROP TRIGGER IF EXISTS trg_kb_keyword_surface_evidence_touch_modify_time ON kb.keyword_surface_evidence;
CREATE TRIGGER trg_kb_keyword_surface_evidence_touch_modify_time
BEFORE UPDATE ON kb.keyword_surface_evidence
FOR EACH ROW EXECUTE FUNCTION kb.touch_modify_time_if_unchanged();

-- 3. A concept can point at the concept it was merged into. A sync page applies
--    rows in modify_time order, so a merged concept can arrive before its target.
--    Making the constraint deferrable lets the sync check it at commit instead
--    (the sync upsert runs SET CONSTRAINTS ALL DEFERRED); everywhere else it
--    stays IMMEDIATE and behaves as before.
ALTER TABLE kb.keyword_concepts
    ALTER CONSTRAINT keyword_concepts_merged_into_fkey DEFERRABLE INITIALLY IMMEDIATE;

-- +goose Down
ALTER TABLE kb.keyword_concepts
    ALTER CONSTRAINT keyword_concepts_merged_into_fkey NOT DEFERRABLE;

DROP TRIGGER IF EXISTS trg_kb_keyword_surface_evidence_touch_modify_time ON kb.keyword_surface_evidence;
ALTER TABLE kb.keyword_surface_evidence DROP COLUMN IF EXISTS modify_time;
DROP TRIGGER IF EXISTS trg_kb_keyword_external_ids_touch_modify_time ON kb.keyword_external_ids;
ALTER TABLE kb.keyword_external_ids DROP COLUMN IF EXISTS modify_time;
ALTER TABLE kb.keyword_surface_keys DROP COLUMN IF EXISTS created_at;
DROP TRIGGER IF EXISTS trg_kb_keyword_rewrite_rules_touch_modify_time ON kb.keyword_rewrite_rules;
DROP TRIGGER IF EXISTS trg_kb_keyword_surfaces_touch_modify_time ON kb.keyword_surfaces;
DROP TRIGGER IF EXISTS trg_kb_keyword_concepts_touch_modify_time ON kb.keyword_concepts;

ALTER TABLE kb.keyword_external_ids DROP CONSTRAINT IF EXISTS keyword_external_ids_sync_natural_key;
ALTER TABLE kb.keyword_rewrite_rules DROP CONSTRAINT IF EXISTS keyword_rewrite_rules_sync_natural_key;
ALTER TABLE kb.keyword_surface_keys DROP CONSTRAINT IF EXISTS keyword_surface_keys_sync_natural_key;
ALTER TABLE kb.keyword_surfaces DROP CONSTRAINT IF EXISTS keyword_surfaces_sync_natural_key;
ALTER TABLE kb.keyword_concepts DROP CONSTRAINT IF EXISTS keyword_concepts_sync_natural_key;
