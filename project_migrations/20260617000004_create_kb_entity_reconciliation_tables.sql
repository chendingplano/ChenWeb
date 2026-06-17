-- +goose Up
-- ADR 2026061701 — Corpus-level entity reconciliation (resolves ADR 2026061302
-- Open Question #3: provisional-entity hygiene / cross-document merge).
--
-- Per-document linking (ADR 2026061302 D1-D6) gives each entity a canonical
-- entity_id WITHIN one document. This migration adds the corpus-level identity
-- layer used by the periodic reconciliation job: a self-referential canonical
-- mapping on kb.entities plus three bookkeeping tables (candidates, merges,
-- runs). It does not touch extraction.

-- ---------------------------------------------------------------------------
-- 1. kb.entities: corpus canonical identity + reconciliation lifecycle.
-- ---------------------------------------------------------------------------
-- canonical_entity_id: the surviving entity_id this row folds into.
--   NULL or = entity_id  => this row is its own canonical head.
ALTER TABLE kb.entities ADD COLUMN IF NOT EXISTS canonical_entity_id TEXT;

-- reconcile_status: lifecycle in the dedup pipeline. Independent of
-- entity_status, which stays as provenance ('extracted' | 'provisional').
--   'pending'   : not yet reconciled (also re-set when content changes)
--   'clustered' : reconciled, is a canonical head
--   'merged'    : folded into canonical_entity_id, no longer a head
--   'held'      : low-confidence, parked for human review
ALTER TABLE kb.entities ADD COLUMN IF NOT EXISTS reconcile_status TEXT NOT NULL DEFAULT 'pending';

ALTER TABLE kb.entities ADD COLUMN IF NOT EXISTS reconciled_at TIMESTAMPTZ;

-- modify_time: watermark driver. kb.entities had only create_time; the
-- incremental reconciler scans rows with modify_time > last watermark.
ALTER TABLE kb.entities ADD COLUMN IF NOT EXISTS modify_time TIMESTAMPTZ NOT NULL DEFAULT now();

-- content_fingerprint: hash of identity-bearing fields (entity/entity_en/
-- aliases/type). When it changes, a re-extracted or edited entity is
-- re-queued for reconciliation.
ALTER TABLE kb.entities ADD COLUMN IF NOT EXISTS content_fingerprint TEXT;

CREATE INDEX IF NOT EXISTS idx_kb_entities_canonical_entity_id ON kb.entities (canonical_entity_id);
CREATE INDEX IF NOT EXISTS idx_kb_entities_reconcile_status    ON kb.entities (reconcile_status);
CREATE INDEX IF NOT EXISTS idx_kb_entities_modify_time         ON kb.entities (modify_time);

-- Keep modify_time current on any row update (mirrors kb.entity_names usage).
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION kb.touch_modify_time()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.modify_time := now();
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS trg_kb_entities_touch_modify_time ON kb.entities;
CREATE TRIGGER trg_kb_entities_touch_modify_time
BEFORE UPDATE ON kb.entities
FOR EACH ROW
EXECUTE FUNCTION kb.touch_modify_time();

-- ---------------------------------------------------------------------------
-- 2. kb.entity_merge_candidates: output of the blocking phase, input to
--    adjudication. UNIQUE(lo,hi) keeps the job idempotent across re-runs.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS kb.entity_merge_candidates (
    candidate_id   BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    lo_entity_id   TEXT        NOT NULL,
    hi_entity_id   TEXT        NOT NULL,
    signals        JSONB       NOT NULL DEFAULT '{}'::jsonb,
    score          DOUBLE PRECISION NOT NULL DEFAULT 0,
    decision       TEXT        NOT NULL DEFAULT 'pending',
        -- 'pending'|'auto_merge'|'auto_reject'|'needs_human'|'applied'|'dismissed'
    decided_method TEXT,       -- 'rule' | 'llm' | 'human'
    decided_by     TEXT,
    decided_at     TIMESTAMPTZ,
    run_id         BIGINT,
    create_time    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT entity_merge_candidates_pair_uniq UNIQUE (lo_entity_id, hi_entity_id),
    CONSTRAINT entity_merge_candidates_ordered CHECK (lo_entity_id < hi_entity_id)
);
CREATE INDEX IF NOT EXISTS idx_kb_emc_decision ON kb.entity_merge_candidates (decision);
CREATE INDEX IF NOT EXISTS idx_kb_emc_score    ON kb.entity_merge_candidates (score DESC);
CREATE INDEX IF NOT EXISTS idx_kb_emc_run_id   ON kb.entity_merge_candidates (run_id);

-- ---------------------------------------------------------------------------
-- 3. kb.entity_merges: immutable, reversible provenance for applied merges.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS kb.entity_merges (
    merge_id            BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    from_entity_id      TEXT        NOT NULL,   -- the absorbed entity_id
    into_entity_id      TEXT        NOT NULL,   -- the surviving canonical entity_id
    method              TEXT        NOT NULL,   -- 'rule' | 'llm' | 'human'
    confidence          DOUBLE PRECISION,
    reason              TEXT,
    evidence            JSONB,
    candidate_id        BIGINT,
    relations_repointed BIGINT      NOT NULL DEFAULT 0,
    run_id              BIGINT,
    decided_by          TEXT,
    decided_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    undone_at           TIMESTAMPTZ,            -- non-null => merge reverted
    undone_by           TEXT
);
CREATE INDEX IF NOT EXISTS idx_kb_entity_merges_from ON kb.entity_merges (from_entity_id);
CREATE INDEX IF NOT EXISTS idx_kb_entity_merges_into ON kb.entity_merges (into_entity_id);
CREATE INDEX IF NOT EXISTS idx_kb_entity_merges_run  ON kb.entity_merges (run_id);

-- ---------------------------------------------------------------------------
-- 4. kb.reconcile_runs: watermark + per-run audit. watermark_to advances only
--    when status='ok', so a failed run safely re-processes the same slice.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS kb.reconcile_runs (
    run_id         BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    phase          TEXT        NOT NULL,  -- 'block'|'adjudicate'|'apply'|'enrich'|'full'
    scope          JSONB,
    watermark_from TIMESTAMPTZ,
    watermark_to   TIMESTAMPTZ,
    status         TEXT        NOT NULL DEFAULT 'running', -- 'running'|'ok'|'failed'
    stats          JSONB,
    error          TEXT,
    started_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at    TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_kb_reconcile_runs_status ON kb.reconcile_runs (status);
CREATE INDEX IF NOT EXISTS idx_kb_reconcile_runs_started ON kb.reconcile_runs (started_at DESC);

-- ---------------------------------------------------------------------------
-- 5. kb.relations_resolved: view that traverses canonical edges, so consumers
--    don't re-implement the canonical hop. The Apply step also physically
--    re-points relations; this view is the safety net (and survives un-merges).
-- ---------------------------------------------------------------------------
-- +goose StatementBegin
CREATE OR REPLACE VIEW kb.relations_resolved AS
SELECT r.*,
       COALESCE(NULLIF(s.canonical_entity_id, ''), r.subject_entity_id) AS subject_canonical_id,
       COALESCE(NULLIF(o.canonical_entity_id, ''), r.object_entity_id)  AS object_canonical_id
FROM kb.relations r
LEFT JOIN kb.entities s ON s.entity_id = r.subject_entity_id
LEFT JOIN kb.entities o ON o.entity_id = r.object_entity_id;
-- +goose StatementEnd

-- +goose Down
DROP VIEW IF EXISTS kb.relations_resolved;

DROP INDEX IF EXISTS kb.idx_kb_reconcile_runs_started;
DROP INDEX IF EXISTS kb.idx_kb_reconcile_runs_status;
DROP TABLE IF EXISTS kb.reconcile_runs;

DROP INDEX IF EXISTS kb.idx_kb_entity_merges_run;
DROP INDEX IF EXISTS kb.idx_kb_entity_merges_into;
DROP INDEX IF EXISTS kb.idx_kb_entity_merges_from;
DROP TABLE IF EXISTS kb.entity_merges;

DROP INDEX IF EXISTS kb.idx_kb_emc_run_id;
DROP INDEX IF EXISTS kb.idx_kb_emc_score;
DROP INDEX IF EXISTS kb.idx_kb_emc_decision;
DROP TABLE IF EXISTS kb.entity_merge_candidates;

DROP TRIGGER IF EXISTS trg_kb_entities_touch_modify_time ON kb.entities;
DROP FUNCTION IF EXISTS kb.touch_modify_time();
DROP INDEX IF EXISTS kb.idx_kb_entities_modify_time;
DROP INDEX IF EXISTS kb.idx_kb_entities_reconcile_status;
DROP INDEX IF EXISTS kb.idx_kb_entities_canonical_entity_id;
ALTER TABLE kb.entities DROP COLUMN IF EXISTS content_fingerprint;
ALTER TABLE kb.entities DROP COLUMN IF EXISTS modify_time;
ALTER TABLE kb.entities DROP COLUMN IF EXISTS reconciled_at;
ALTER TABLE kb.entities DROP COLUMN IF EXISTS reconcile_status;
ALTER TABLE kb.entities DROP COLUMN IF EXISTS canonical_entity_id;
