-- +goose Up
-- Soft, wording-invariant dedup signal for kb.ontology_candidates (openspec
-- change ontology-candidate-dedup). Computed application-side in
-- candidates.IdentityKey from candidate_kind='term' payloads; deliberately
-- NOT unique — it flags a possible duplicate for human review via the
-- existing candidate_matches column, it never blocks an insert. No backfill:
-- existing rows keep identity_key = NULL until reprocessed.
ALTER TABLE kb.ontology_candidates ADD COLUMN IF NOT EXISTS identity_key TEXT;

CREATE INDEX IF NOT EXISTS idx_kb_ontology_candidates_identity_key
    ON kb.ontology_candidates (identity_key);

-- +goose Down
DROP INDEX IF EXISTS idx_kb_ontology_candidates_identity_key;
ALTER TABLE kb.ontology_candidates DROP COLUMN IF EXISTS identity_key;
