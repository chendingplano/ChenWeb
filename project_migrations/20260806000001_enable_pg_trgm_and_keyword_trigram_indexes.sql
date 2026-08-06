-- +goose Up
-- Spec 2026080403 §19 step 11: pg_trgm backs Tier 5's fuzzy blocking query
-- (online, KeywordFamily.tier5FuzzyMatch) and the reconciler's lexical
-- blocking pass (offline, keywords.Reconciler) — the codebase had no
-- pg_trgm before this (2026080601-bug's inventory confirmed no existing
-- enablement). Indexes are on norm_key (surfaces) and pref_label (concepts)
-- because both blocking queries compare against those columns, not the
-- verbatim `surface` column.

CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX IF NOT EXISTS idx_keyword_surfaces_norm_key_trgm
    ON kb.keyword_surfaces USING gin (norm_key gin_trgm_ops);

CREATE INDEX IF NOT EXISTS idx_keyword_concepts_pref_label_trgm
    ON kb.keyword_concepts USING gin (pref_label gin_trgm_ops);

-- +goose Down
DROP INDEX IF EXISTS kb.idx_keyword_concepts_pref_label_trgm;
DROP INDEX IF EXISTS kb.idx_keyword_surfaces_norm_key_trgm;
-- pg_trgm itself is not dropped: other objects in the database may depend
-- on it, and CREATE EXTENSION IF NOT EXISTS on Up is idempotent regardless.
