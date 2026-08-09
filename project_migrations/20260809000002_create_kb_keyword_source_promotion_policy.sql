-- +goose Up
-- Per-resource override for whether an approved external resource's staged
-- catalog entries auto-promote into keyword concepts (keyword-catalog-auto-
-- promotion openspec change). Deliberately mutable, unlike kb.keyword_sources
-- (which is immutable by trigger) -- this is a live admin setting, not
-- provenance evidence. Absence of a row means enabled (the column default);
-- an admin only needs to write a row to opt a resource out.
CREATE TABLE IF NOT EXISTS kb.keyword_source_promotion_policy (
    source               TEXT NOT NULL PRIMARY KEY,
    auto_promote_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    set_by               TEXT,
    set_at               TIMESTAMPTZ,
    create_time          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modify_time          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- +goose Down
DROP TABLE IF EXISTS kb.keyword_source_promotion_policy;
