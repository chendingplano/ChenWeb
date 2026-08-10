-- +goose Up
-- ADR 2026081001 DR9: defer is retired as a runtime-recoverable gate effect.
-- ValidatePipelineVersion already rejects effect='defer' at creation time;
-- this makes the database itself refuse to store one. Confirmed zero live
-- rows have effect='defer' before this migration.
ALTER TABLE kb.pipeline_rules DROP CONSTRAINT IF EXISTS ck_pipeline_rules_effect;

-- +goose StatementBegin
DO $$
BEGIN
    ALTER TABLE kb.pipeline_rules
        ADD CONSTRAINT ck_pipeline_rules_effect
        CHECK (effect IN ('require', 'enable', 'skip'));
END $$;
-- +goose StatementEnd

-- +goose Down
ALTER TABLE kb.pipeline_rules DROP CONSTRAINT IF EXISTS ck_pipeline_rules_effect;
-- +goose StatementBegin
DO $$
BEGIN
    ALTER TABLE kb.pipeline_rules
        ADD CONSTRAINT ck_pipeline_rules_effect
        CHECK (effect IN ('require', 'enable', 'skip', 'defer'));
END $$;
-- +goose StatementEnd
