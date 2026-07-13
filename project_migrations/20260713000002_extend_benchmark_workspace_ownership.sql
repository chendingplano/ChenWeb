-- +goose Up
ALTER TABLE kb.benchmark_workspaces
  ADD COLUMN IF NOT EXISTS work_root text,
  ADD COLUMN IF NOT EXISTS evidence_path text,
  ADD COLUMN IF NOT EXISTS evidence_root text,
  ADD COLUMN IF NOT EXISTS verified boolean NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS verified_hash text,
  ADD COLUMN IF NOT EXISTS verified_size bigint,
  ADD COLUMN IF NOT EXISTS verified_marker_hash text,
  ADD COLUMN IF NOT EXISTS verified_marker jsonb;

-- +goose Down
ALTER TABLE kb.benchmark_workspaces
  DROP COLUMN IF EXISTS verified_marker,
  DROP COLUMN IF EXISTS verified_marker_hash,
  DROP COLUMN IF EXISTS verified_size,
  DROP COLUMN IF EXISTS verified_hash,
  DROP COLUMN IF EXISTS verified,
  DROP COLUMN IF EXISTS evidence_root,
  DROP COLUMN IF EXISTS evidence_path,
  DROP COLUMN IF EXISTS work_root;
