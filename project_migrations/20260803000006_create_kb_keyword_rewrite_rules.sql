-- +goose Up
-- P3 Track B: keyword lexicon — pattern-based rewrite rules (DR16 spec §3.2).
-- Rules are disabled by default; a human enables them per scope after
-- validation. Applied in tier 3 as a pre-normalization pass: the surface
-- is rewritten, then tiers 0-2 retry with the rewritten form.
-- Patterns are constrained: no capture groups, no backreferences —
-- simple literal replacement only.

CREATE TABLE IF NOT EXISTS kb.keyword_rewrite_rules (
    rule_id         TEXT PRIMARY KEY,
    pattern         TEXT NOT NULL,
    replacement     TEXT NOT NULL,
    scope           TEXT NOT NULL DEFAULT '_',
    enabled         BOOLEAN NOT NULL DEFAULT FALSE,
    provenance      TEXT NOT NULL DEFAULT 'human:',
    create_time     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modify_time     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_keyword_rewrite_rules_enabled_scope
    ON kb.keyword_rewrite_rules (enabled, scope);

-- +goose Down
DROP TABLE IF EXISTS kb.keyword_rewrite_rules;
