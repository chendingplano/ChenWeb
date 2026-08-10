-- +goose Up
-- ADR 2026081001 DR1: kb.pipelines becomes immutable and versioned. Editing
-- processors[] will insert a new row (version = MAX(version WHERE name=$1)+1)
-- instead of UPDATEing in place; UNIQUE(name) alone can no longer hold once
-- multiple versions of the same name coexist.
ALTER TABLE kb.pipelines ADD COLUMN IF NOT EXISTS version INT NOT NULL DEFAULT 1;
ALTER TABLE kb.pipelines ADD COLUMN IF NOT EXISTS status VARCHAR(16) NOT NULL DEFAULT 'active';

-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'ck_pipelines_status'
          AND conrelid = 'kb.pipelines'::regclass
    ) THEN
        ALTER TABLE kb.pipelines
            ADD CONSTRAINT ck_pipelines_status
            CHECK (status IN ('active', 'superseded'));
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE kb.pipelines DROP CONSTRAINT IF EXISTS pipelines_name_key;
CREATE UNIQUE INDEX IF NOT EXISTS idx_kb_pipelines_name_version ON kb.pipelines (name, version);

-- +goose Down
DROP INDEX IF EXISTS idx_kb_pipelines_name_version;
ALTER TABLE kb.pipelines ADD CONSTRAINT pipelines_name_key UNIQUE (name);
ALTER TABLE kb.pipelines DROP CONSTRAINT IF EXISTS ck_pipelines_status;
ALTER TABLE kb.pipelines DROP COLUMN IF EXISTS status;
ALTER TABLE kb.pipelines DROP COLUMN IF EXISTS version;
