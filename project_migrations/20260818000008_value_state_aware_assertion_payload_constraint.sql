-- +goose Up
-- ADR 2026081801 DR6: "The current kb.semantic_assertions constraint requiring
-- an object reference or literal must be revised so a genuine missing-value
-- instance can exist."
--
-- chk_semantic_assertions_object_ref_or_literal makes the DR6 `missing` state
-- unrepresentable: a source that names a metric but supplies no value has
-- neither an object_ref_id nor an object_literal, so the row is rejected and
-- the claim is lost -- precisely the loss this ADR exists to stop. Fabricating
-- a placeholder literal to satisfy the old constraint would be worse: it would
-- put a value in the knowledge base that no source ever stated.
--
-- The replacement keys the required payload off the governed value state.
--
-- Backfill first: every pre-existing row satisfied the old check by
-- construction (it was enforced), so every row with an object reference or
-- literal is `present` by definition. Anything that does not match is left
-- NULL and reported by the verification query below rather than guessed at.
UPDATE kb.semantic_assertions
SET value_state_term_id = 'semantic:value_present'
WHERE value_state_term_id IS NULL
  AND (object_ref_id IS NOT NULL OR object_literal IS NOT NULL);

ALTER TABLE kb.semantic_assertions
    DROP CONSTRAINT IF EXISTS chk_semantic_assertions_object_ref_or_literal;

-- NOT VALID + VALIDATE keeps the exclusive lock short on a large table: the
-- ADD acquires only a brief lock and skips the full scan, and VALIDATE scans
-- under a weaker SHARE UPDATE EXCLUSIVE lock that does not block reads or
-- writes.
--
-- A NULL value_state_term_id is permitted so legacy rows written before Phase 1
-- and rows written by the legacy writer still satisfy the constraint: Phase 1
-- is additive and must not break the running writer. The lossless writer always
-- sets the state, and the completeness projection reports NULLs.
ALTER TABLE kb.semantic_assertions
    ADD CONSTRAINT chk_semantic_assertions_value_state_payload
    CHECK (
        value_state_term_id IS NULL
        -- present: a normalized literal or reference is required.
        OR (value_state_term_id = 'semantic:value_present'
            AND (object_ref_id IS NOT NULL OR object_literal IS NOT NULL))
        -- unparsed / datatype_mismatch / unknown: the raw payload is what
        -- carries the claim, so it is required and a normalized object is not.
        OR (value_state_term_id IN (
                'semantic:value_state_unparsed',
                'semantic:value_state_datatype_mismatch',
                'semantic:value_state_unknown')
            AND (raw_payload IS NOT NULL OR raw_text IS NOT NULL))
        -- missing: no fabricated object. Subject and class are required;
        -- applicability and evidence are enforced by the writer transaction and
        -- the completeness projection, not by a row-level CHECK, because
        -- evidence lives in another table.
        OR (value_state_term_id = 'semantic:value_state_missing'
            AND object_ref_id IS NULL
            AND object_literal IS NULL
            AND subject_ref_id IS NOT NULL)
        -- not_applicable: explicit non-applicability context is required, and
        -- qualifiers is where applicability is recorded.
        OR (value_state_term_id = 'semantic:value_state_not_applicable'
            AND qualifiers IS NOT NULL)
    ) NOT VALID;

ALTER TABLE kb.semantic_assertions
    VALIDATE CONSTRAINT chk_semantic_assertions_value_state_payload;

-- +goose Down
ALTER TABLE kb.semantic_assertions
    DROP CONSTRAINT IF EXISTS chk_semantic_assertions_value_state_payload;

-- Restoring the old constraint requires the table to satisfy it again. Rows
-- written under the new model with a missing/unparsed value legitimately do
-- not, so the rollback drops them out of the constraint's scope by refusing to
-- re-add it when such rows exist -- a rollback must not silently delete
-- committed raw-preserved claims (ADR section 6).
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM kb.semantic_assertions
               WHERE object_ref_id IS NULL AND object_literal IS NULL) THEN
        RAISE NOTICE
            'chk_semantic_assertions_object_ref_or_literal not restored: % raw-preserved row(s) without an object payload exist',
            (SELECT count(*) FROM kb.semantic_assertions
             WHERE object_ref_id IS NULL AND object_literal IS NULL);
    ELSE
        ALTER TABLE kb.semantic_assertions
            ADD CONSTRAINT chk_semantic_assertions_object_ref_or_literal
            CHECK (object_ref_id IS NOT NULL OR object_literal IS NOT NULL);
    END IF;
END $$;
-- +goose StatementEnd
