-- +goose Up
-- Rollback re-points activation at an older release (ADR DR2), which is
-- legitimately a re-activation of a release that was active before: that must
-- insert a new activation row, so UNIQUE(module_id, release_id) from
-- 20260731000023 is too strict. The partial unique index (one active per
-- module) remains the real invariant.
ALTER TABLE kb.ontology_active_releases DROP CONSTRAINT IF EXISTS ontology_active_releases_module_id_release_id_key;

-- +goose Down
ALTER TABLE kb.ontology_active_releases
    ADD CONSTRAINT ontology_active_releases_module_id_release_id_key UNIQUE (module_id, release_id);
