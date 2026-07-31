-- +goose Up
-- The live chenweb_test environment already has these constraints (added by
-- the Go-based idempotent migrations in migrations.go), but they were not
-- explicitly defined in the original kb.doc_process_plans creation migration
-- (20260731000001). This migration makes them explicit so a fresh goose-only
-- install gets them too, using idempotent IF NOT EXISTS / DO $$ patterns to
-- be safe on systems that already have them.
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'fk_doc_process_plans_run'
          AND conrelid = 'kb.doc_process_plans'::regclass
    ) THEN
        ALTER TABLE kb.doc_process_plans
            ADD CONSTRAINT fk_doc_process_plans_run
            FOREIGN KEY (run_id) REFERENCES kb.doc_process_runs(id) ON DELETE CASCADE;
    END IF;
END $$;
-- +goose StatementEnd

-- The original 20260731000001 migration created idx_kb_doc_process_plans_run_id
-- as a regular index, but the correct semantics for the P1 design (one plan
-- row per run) require it to be UNIQUE. This re-creates it as a unique
-- index if the old non-unique version exists.
DROP INDEX IF EXISTS idx_kb_doc_process_plans_run_id;
CREATE UNIQUE INDEX IF NOT EXISTS idx_kb_doc_process_plans_run_id
    ON kb.doc_process_plans (run_id);

-- +goose Down
ALTER TABLE kb.doc_process_plans DROP CONSTRAINT IF EXISTS fk_doc_process_plans_run;
DROP INDEX IF EXISTS idx_kb_doc_process_plans_run_id;
CREATE INDEX IF NOT EXISTS idx_kb_doc_process_plans_run_id
    ON kb.doc_process_plans (run_id);
