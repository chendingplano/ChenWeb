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

-- Repair the persistence constraints for databases that applied the original
-- benchmark migration before the workspace ownership extension.
-- Legacy terminal triggers reject spelling-only lifecycle updates, so remove
-- them transactionally and recreate them after stored rows are normalized.
DROP TRIGGER IF EXISTS trg_benchmark_attempt_terminal ON kb.benchmark_case_attempts;
DROP TRIGGER IF EXISTS trg_benchmark_run_terminal ON kb.benchmark_runs;

ALTER TABLE kb.benchmark_workspaces
  DROP CONSTRAINT IF EXISTS benchmark_workspaces_cleanup_state_check;
UPDATE kb.benchmark_workspaces SET cleanup_state='error' WHERE cleanup_state='failed';
ALTER TABLE kb.benchmark_workspaces
  ADD CONSTRAINT benchmark_workspaces_cleanup_state_check
    CHECK (cleanup_state IN ('pending','active','error','db_pending','files_pending','cleaned'));

ALTER TABLE kb.benchmark_runs
  DROP CONSTRAINT IF EXISTS benchmark_runs_lifecycle_check;
UPDATE kb.benchmark_runs SET lifecycle='canceled' WHERE lifecycle='cancelled';
ALTER TABLE kb.benchmark_runs
  ADD CONSTRAINT benchmark_runs_lifecycle_check
    CHECK (lifecycle IN ('queued','running','succeeded','failed','canceled'));

ALTER TABLE kb.benchmark_case_attempts
  DROP CONSTRAINT IF EXISTS benchmark_case_attempts_lifecycle_check,
  DROP CONSTRAINT IF EXISTS ck_failure_kind;
UPDATE kb.benchmark_case_attempts SET failure_kind='canceled' WHERE failure_kind='cancelled';
-- The original failure-kind CHECK required NULL for the legacy cancelled
-- lifecycle; populate its canonical failure before replacing that lifecycle.
UPDATE kb.benchmark_case_attempts SET failure_kind='canceled' WHERE lifecycle='cancelled' AND failure_kind IS NULL;
UPDATE kb.benchmark_case_attempts SET lifecycle='canceled' WHERE lifecycle='cancelled';
ALTER TABLE kb.benchmark_case_attempts
  ADD CONSTRAINT benchmark_case_attempts_lifecycle_check
    CHECK (lifecycle IN ('queued','leased','running','succeeded','failed','canceled')),
  ADD CONSTRAINT ck_failure_kind
    CHECK ((lifecycle NOT IN ('failed','canceled') AND failure_kind IS NULL)
      OR lifecycle='failed'
      OR (lifecycle='canceled' AND failure_kind='canceled'));

CREATE OR REPLACE FUNCTION kb.benchmark_terminal_guard() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF TG_TABLE_NAME='benchmark_case_attempts' THEN IF OLD.input_record_id_snapshot IS NOT NULL AND OLD.input_record_id_snapshot IS DISTINCT FROM NEW.input_record_id_snapshot THEN RAISE EXCEPTION 'input snapshot is immutable'; END IF; END IF; IF OLD.lifecycle IN ('success','processor_failed','timed_out','invalid_output','infrastructure_failed','scorer_failed','canceled','succeeded','failed') AND (NEW IS DISTINCT FROM OLD) THEN RAISE EXCEPTION 'terminal benchmark row is immutable'; END IF; RETURN NEW; END $$;

CREATE TRIGGER trg_benchmark_attempt_terminal BEFORE UPDATE ON kb.benchmark_case_attempts FOR EACH ROW EXECUTE FUNCTION kb.benchmark_terminal_guard();
CREATE TRIGGER trg_benchmark_run_terminal BEFORE UPDATE ON kb.benchmark_runs FOR EACH ROW EXECUTE FUNCTION kb.benchmark_terminal_guard();

CREATE OR REPLACE FUNCTION kb.benchmark_score_guard() RETURNS trigger LANGUAGE plpgsql AS $$ DECLARE l text; s uuid; case_run uuid; BEGIN IF OLD.attempt_id IS NOT NULL THEN SELECT case_run_id INTO case_run FROM kb.benchmark_case_attempts WHERE id=OLD.attempt_id; SELECT selected_attempt_id INTO s FROM kb.benchmark_case_runs WHERE id=case_run FOR UPDATE; SELECT lifecycle INTO l FROM kb.benchmark_case_attempts WHERE id=OLD.attempt_id FOR UPDATE; IF s=OLD.attempt_id THEN RAISE EXCEPTION 'scores immutable after selected attempt'; END IF; ELSE SELECT lifecycle INTO l FROM kb.benchmark_runs WHERE id=OLD.run_id FOR UPDATE; END IF; IF l IN ('success','processor_failed','timed_out','invalid_output','infrastructure_failed','scorer_failed','canceled','succeeded','failed') THEN RAISE EXCEPTION 'scores immutable after terminal owner'; END IF; IF TG_OP='DELETE' THEN RETURN OLD; END IF; RETURN NEW; END $$;

-- +goose Down
-- Restore the exact pre-extension constraints and guards before removing the
-- ownership columns. Canonical cleanup states without a legacy equivalent are
-- conservatively returned to pending so the legacy CHECK remains satisfiable.
DROP TRIGGER IF EXISTS trg_benchmark_attempt_terminal ON kb.benchmark_case_attempts;
DROP TRIGGER IF EXISTS trg_benchmark_run_terminal ON kb.benchmark_runs;

ALTER TABLE kb.benchmark_workspaces
  DROP CONSTRAINT IF EXISTS benchmark_workspaces_cleanup_state_check;
UPDATE kb.benchmark_workspaces SET cleanup_state='failed' WHERE cleanup_state='error';
UPDATE kb.benchmark_workspaces SET cleanup_state='pending' WHERE cleanup_state IN ('active','db_pending','files_pending');
ALTER TABLE kb.benchmark_workspaces
  ADD CONSTRAINT benchmark_workspaces_cleanup_state_check
    CHECK (cleanup_state IN ('pending','cleaned','failed'));

ALTER TABLE kb.benchmark_runs
  DROP CONSTRAINT IF EXISTS benchmark_runs_lifecycle_check;
UPDATE kb.benchmark_runs SET lifecycle='cancelled' WHERE lifecycle='canceled';
ALTER TABLE kb.benchmark_runs
  ADD CONSTRAINT benchmark_runs_lifecycle_check
    CHECK (lifecycle IN ('queued','running','succeeded','failed','cancelled'));

ALTER TABLE kb.benchmark_case_attempts
  DROP CONSTRAINT IF EXISTS benchmark_case_attempts_lifecycle_check,
  DROP CONSTRAINT IF EXISTS ck_failure_kind;
UPDATE kb.benchmark_case_attempts SET failure_kind='cancelled' WHERE failure_kind='canceled' AND lifecycle<>'canceled';
-- The legacy failure CHECK required NULL for a cancelled lifecycle.
UPDATE kb.benchmark_case_attempts SET failure_kind=NULL WHERE lifecycle='canceled' AND failure_kind='canceled';
UPDATE kb.benchmark_case_attempts SET lifecycle='cancelled' WHERE lifecycle='canceled';
ALTER TABLE kb.benchmark_case_attempts
  ADD CONSTRAINT benchmark_case_attempts_lifecycle_check
    CHECK (lifecycle IN ('queued','leased','running','succeeded','failed','cancelled')),
  ADD CONSTRAINT ck_failure_kind
    CHECK ((lifecycle NOT IN ('failed') AND failure_kind IS NULL) OR lifecycle='failed');

CREATE OR REPLACE FUNCTION kb.benchmark_terminal_guard() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF TG_TABLE_NAME='benchmark_case_attempts' THEN IF OLD.input_record_id_snapshot IS DISTINCT FROM NEW.input_record_id_snapshot THEN RAISE EXCEPTION 'input snapshot is immutable'; END IF; END IF; IF OLD.lifecycle IN ('success','processor_failed','timed_out','invalid_output','infrastructure_failed','scorer_failed','canceled','succeeded','failed','cancelled') AND (NEW IS DISTINCT FROM OLD) THEN RAISE EXCEPTION 'terminal benchmark row is immutable'; END IF; RETURN NEW; END $$;

CREATE TRIGGER trg_benchmark_attempt_terminal BEFORE UPDATE ON kb.benchmark_case_attempts FOR EACH ROW EXECUTE FUNCTION kb.benchmark_terminal_guard();
CREATE TRIGGER trg_benchmark_run_terminal BEFORE UPDATE ON kb.benchmark_runs FOR EACH ROW EXECUTE FUNCTION kb.benchmark_terminal_guard();

CREATE OR REPLACE FUNCTION kb.benchmark_score_guard() RETURNS trigger LANGUAGE plpgsql AS $$ DECLARE l text; s uuid; case_run uuid; BEGIN IF OLD.attempt_id IS NOT NULL THEN SELECT case_run_id INTO case_run FROM kb.benchmark_case_attempts WHERE id=OLD.attempt_id; SELECT selected_attempt_id INTO s FROM kb.benchmark_case_runs WHERE id=case_run FOR UPDATE; SELECT lifecycle INTO l FROM kb.benchmark_case_attempts WHERE id=OLD.attempt_id FOR UPDATE; IF s=OLD.attempt_id THEN RAISE EXCEPTION 'scores immutable after selected attempt'; END IF; ELSE SELECT lifecycle INTO l FROM kb.benchmark_runs WHERE id=OLD.run_id FOR UPDATE; END IF; IF l IN ('success','processor_failed','timed_out','invalid_output','infrastructure_failed','scorer_failed','canceled','succeeded','failed','cancelled') THEN RAISE EXCEPTION 'scores immutable after terminal owner'; END IF; IF TG_OP='DELETE' THEN RETURN OLD; END IF; RETURN NEW; END $$;

ALTER TABLE kb.benchmark_workspaces
  DROP COLUMN IF EXISTS verified_marker,
  DROP COLUMN IF EXISTS verified_marker_hash,
  DROP COLUMN IF EXISTS verified_size,
  DROP COLUMN IF EXISTS verified_hash,
  DROP COLUMN IF EXISTS verified,
  DROP COLUMN IF EXISTS evidence_root,
  DROP COLUMN IF EXISTS evidence_path,
  DROP COLUMN IF EXISTS work_root;
