-- +goose Up
-- Every pipeline-policy version is minted from MAX(version)+1 in a
-- transaction that is not serializable, so two concurrent promotions can
-- compute the same next version and one insert silently wins while the other
-- overwrites. A unique constraint on (version) is the real guard: the loser's
-- insert fails loudly and its release rolls back (P5 review 2026080302 finding
-- P5-10).
CREATE UNIQUE INDEX IF NOT EXISTS idx_kb_pipeline_policies_unique_version
    ON kb.pipeline_policies (version);

-- +goose Down
DROP INDEX IF EXISTS idx_kb_pipeline_policies_unique_version;
