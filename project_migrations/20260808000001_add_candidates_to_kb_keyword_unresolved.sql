-- +goose Up
-- Spec 2026080403 §13/§20.1.13-follow-up: ambiguity reconciliation.
-- `candidates` captures the tied concept_id/score/method set from an
-- `ambiguous` verdict (kernel.go's Adjudicate: top score tied across more
-- concepts than AutoAcceptPolicy.MaxCandidates) at the moment it is logged,
-- so an offline reconciliation pass can later re-rank the tie with stronger
-- evidence (query embedding vs. each tied concept) without re-deriving it
-- from decision-log JSON. NULL for a plain no-candidate miss (Deferred/
-- HumanReview) — only the Ambiguous branch in ObserveOccurrence populates
-- it (keywords/unresolved_store.go). Distinguishes the two cases the
-- backlog previously conflated under one undifferentiated row shape.

ALTER TABLE kb.keyword_unresolved
    ADD COLUMN IF NOT EXISTS candidates JSONB;

-- +goose Down
ALTER TABLE kb.keyword_unresolved
    DROP COLUMN IF EXISTS candidates;
