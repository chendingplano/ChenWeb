-- +goose Up
-- ADR 2026081801 DR6 (and ADR 2026081701 DR8): add the `represented` lifecycle
-- status and `unsupported_prior_status`.
--
-- Why a new status rather than reusing `candidate`: `candidate` means "selected
-- for governance", and the whole point of lossless ingestion is that admitting
-- a claim is NOT a governance act. Today associate_semantics drives every
-- successfully normalized metric straight to `accepted`
-- (associate_semantics.go processMetric), which conflates "we parsed it" with
-- "we endorse it". `represented` gives ingestion a status that carries no
-- endorsement, so `accepted` keeps its existing meaning for the consumers that
-- filter on it.
--
-- This migration deliberately runs while NO code writes `represented`: Phase 1
-- is additive only, and the writer gate stays off until Phase 2 reader
-- certification completes.
ALTER TABLE kb.semantic_assertions
    DROP CONSTRAINT IF EXISTS semantic_assertions_status_check;

ALTER TABLE kb.semantic_assertions
    ADD CONSTRAINT semantic_assertions_status_check
    CHECK (status IN (
        'represented', 'candidate', 'in_review', 'accepted', 'rejected',
        'deferred', 'superseded', 'unsupported'
    ));

-- DR6: records which status a claim held when its last supporting evidence
-- link was removed, so restoration returns it to exactly that status instead
-- of guessing. Restoration must never promote a represented claim to accepted
-- or advance an in-progress governance decision, which is only possible if the
-- prior status was recorded rather than inferred.
ALTER TABLE kb.semantic_assertions
    ADD COLUMN IF NOT EXISTS unsupported_prior_status TEXT;

-- DR6: "The database permits unsupported_prior_status only when
-- status = 'unsupported' ... and restricts its value to represented,
-- candidate, in_review, deferred, or accepted."
--
-- rejected and superseded are excluded on purpose: they are historical
-- decision states that do not transition merely because evidence changed, so
-- they can never be a restoration target.
ALTER TABLE kb.semantic_assertions
    ADD CONSTRAINT chk_semantic_assertions_unsupported_prior_status
    CHECK (
        (unsupported_prior_status IS NULL)
        OR (status = 'unsupported'
            AND unsupported_prior_status IN
                ('represented', 'candidate', 'in_review', 'deferred', 'accepted'))
    );

CREATE INDEX IF NOT EXISTS idx_kb_semantic_assertions_unsupported_prior
    ON kb.semantic_assertions (unsupported_prior_status)
    WHERE unsupported_prior_status IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS kb.idx_kb_semantic_assertions_unsupported_prior;
ALTER TABLE kb.semantic_assertions
    DROP CONSTRAINT IF EXISTS chk_semantic_assertions_unsupported_prior_status;
ALTER TABLE kb.semantic_assertions
    DROP COLUMN IF EXISTS unsupported_prior_status;
ALTER TABLE kb.semantic_assertions
    DROP CONSTRAINT IF EXISTS semantic_assertions_status_check;

-- +goose StatementBegin
-- Narrowing the status constraint again is only possible while no represented
-- row exists. ADR 2026081801 section 6 is explicit that a rollback "does not
-- delete committed raw-preserved assertions or outcome history", so when such
-- rows are present this leaves the widened vocabulary in place and says so,
-- rather than failing the rollback or deleting the claims to make the
-- constraint fit. The widened CHECK is harmless to the legacy writer: it never
-- emits 'represented'.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM kb.semantic_assertions WHERE status = 'represented') THEN
        RAISE NOTICE
            'status CHECK left widened: % represented assertion(s) exist and are not deleted by rollback',
            (SELECT count(*) FROM kb.semantic_assertions WHERE status = 'represented');
        ALTER TABLE kb.semantic_assertions
            ADD CONSTRAINT semantic_assertions_status_check
            CHECK (status IN (
                'represented', 'candidate', 'in_review', 'accepted', 'rejected',
                'deferred', 'superseded', 'unsupported'
            ));
    ELSE
        ALTER TABLE kb.semantic_assertions
            ADD CONSTRAINT semantic_assertions_status_check
            CHECK (status IN (
                'candidate', 'in_review', 'accepted', 'rejected',
                'deferred', 'superseded', 'unsupported'
            ));
    END IF;
END $$;
-- +goose StatementEnd
