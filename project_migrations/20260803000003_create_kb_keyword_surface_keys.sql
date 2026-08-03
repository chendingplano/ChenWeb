-- +goose Up
-- P3 Track B: keyword lexicon — derived surface keys (DR16 spec §3.2).
-- Stores the 4 alternate key kinds (alnum, sorted, phonetic, initials)
-- for each surface row. Keys are derived data — always written alongside
-- the surface, never authored independently.
-- CASCADE on surface delete so removing a surface removes its keys atomically.

CREATE TABLE IF NOT EXISTS kb.keyword_surface_keys (
    surface_id      TEXT NOT NULL REFERENCES kb.keyword_surfaces(surface_id)
                    ON DELETE CASCADE,
    key_kind        TEXT NOT NULL CHECK (key_kind IN ('alnum', 'sorted', 'phonetic', 'initials')),
    key_value       TEXT NOT NULL,
    norm_version    INT NOT NULL,
    PRIMARY KEY (surface_id, key_kind)
);

CREATE INDEX IF NOT EXISTS idx_keyword_surface_keys_lookup
    ON kb.keyword_surface_keys (key_kind, key_value, norm_version);

-- +goose Down
DROP TABLE IF EXISTS kb.keyword_surface_keys;
