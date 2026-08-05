-- +goose Up
-- Spec 2026080403 §19 step 5 (K1/N3/K3): the singularized form joins the
-- derived key kinds. Singularization is an alternate key, never part of the
-- canonical key (spec §6.4 item d); the row is written even when it equals
-- the norm key so plural queries bridge to singular surfaces.

ALTER TABLE kb.keyword_surface_keys
    DROP CONSTRAINT IF EXISTS keyword_surface_keys_key_kind_check;

ALTER TABLE kb.keyword_surface_keys
    ADD CONSTRAINT keyword_surface_keys_key_kind_check
    CHECK (key_kind IN ('alnum', 'sorted', 'phonetic', 'initials', 'singular'));

-- +goose Down
ALTER TABLE kb.keyword_surface_keys
    DROP CONSTRAINT IF EXISTS keyword_surface_keys_key_kind_check;

ALTER TABLE kb.keyword_surface_keys
    ADD CONSTRAINT keyword_surface_keys_key_kind_check
    CHECK (key_kind IN ('alnum', 'sorted', 'phonetic', 'initials'));
