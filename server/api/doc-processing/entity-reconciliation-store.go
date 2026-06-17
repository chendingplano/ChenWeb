package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// ReconcileSQLStore implements ReconcileStore (ADR 2026061701) against the kb
// schema. Blocking is exact normalized-name matching (lower + whitespace-folded
// entity_en, falling back to entity); the codebase has no pg_trgm, so fuzzy
// blocking is a later addition. The pure scorers (computeSignals/scoreCandidate)
// run in Go so the SQL layer only finds and persists candidates.
type ReconcileSQLStore struct {
	DB *sql.DB
}

// Compile-time guarantee the SQL store satisfies the reconciler's seam.
var _ ReconcileStore = ReconcileSQLStore{}

// BeginRun opens a kb.reconcile_runs row and returns the watermark to scan
// since (the previous successful run's watermark_to, or epoch).
func (s ReconcileSQLStore) BeginRun(ctx context.Context, phase string) (int64, time.Time, error) {
	var from time.Time
	const wq = `SELECT COALESCE(MAX(watermark_to), 'epoch'::timestamptz)
	            FROM kb.reconcile_runs WHERE status = 'ok'`
	if err := s.DB.QueryRowContext(ctx, wq).Scan(&from); err != nil {
		return 0, time.Time{}, fmt.Errorf("(MID_26061710) read watermark: %w", err)
	}
	var runID int64
	const iq = `INSERT INTO kb.reconcile_runs (phase, status, started_at)
	            VALUES ($1, 'running', now()) RETURNING run_id`
	if err := s.DB.QueryRowContext(ctx, iq, phase).Scan(&runID); err != nil {
		return 0, time.Time{}, fmt.Errorf("(MID_26061711) begin run: %w", err)
	}
	return runID, from, nil
}

// FinishRun records the outcome; watermark_to advances only when ok (R6).
func (s ReconcileSQLStore) FinishRun(ctx context.Context, runID int64, ok bool, watermarkTo time.Time, stats ReconcileStats, runErr error) error {
	status := "failed"
	if ok {
		status = "ok"
	}
	var errText sql.NullString
	if runErr != nil {
		errText = sql.NullString{String: runErr.Error(), Valid: true}
	}
	statsJSON, _ := json.Marshal(stats)
	const q = `
UPDATE kb.reconcile_runs
SET status = $2, stats = $3::jsonb, error = $4, finished_at = now(),
    watermark_to = CASE WHEN $5 THEN $6 ELSE watermark_to END
WHERE run_id = $1`
	_, err := s.DB.ExecContext(ctx, q, runID, status, statsJSON, errText, ok, watermarkTo)
	return err
}

// BlockCandidates upserts name-blocked pairs, (re)scores the pending ones that
// reference an entity changed since watermarkFrom, and returns them.
func (s ReconcileSQLStore) BlockCandidates(ctx context.Context, runID int64, watermarkFrom time.Time, cfg ReconcileConfig) ([]CandidatePair, error) {
	// 1. Blocking: distinct cross-id pairs sharing a normalized surface name,
	//    where at least one side changed, neither is merged, and they are not
	//    already in the same canonical cluster. ON CONFLICT keeps it idempotent.
	const block = `
INSERT INTO kb.entity_merge_candidates (lo_entity_id, hi_entity_id)
SELECT DISTINCT a.entity_id, b.entity_id
FROM kb.entities a
JOIN kb.entities b
  ON a.entity_id < b.entity_id
 AND lower(regexp_replace(btrim(COALESCE(NULLIF(a.entity_en, ''), a.entity)), '\s+', ' ', 'g'))
   = lower(regexp_replace(btrim(COALESCE(NULLIF(b.entity_en, ''), b.entity)), '\s+', ' ', 'g'))
WHERE (a.modify_time > $1 OR b.modify_time > $1)
  AND a.reconcile_status <> 'merged'
  AND b.reconcile_status <> 'merged'
  AND COALESCE(NULLIF(a.canonical_entity_id, ''), a.entity_id)
   <> COALESCE(NULLIF(b.canonical_entity_id, ''), b.entity_id)
ON CONFLICT (lo_entity_id, hi_entity_id) DO NOTHING`
	if _, err := s.DB.ExecContext(ctx, block, watermarkFrom); err != nil {
		return nil, fmt.Errorf("(MID_26061712) block candidates: %w", err)
	}

	// 2. Pull pending candidates touching a changed entity, with a representative
	//    row per endpoint (prefer the extracted, most-recent row).
	batch := cfg.BatchSize
	if batch <= 0 {
		batch = 1000
	}
	const sel = `
SELECT c.candidate_id, c.lo_entity_id, c.hi_entity_id,
       lo.entity, lo.entity_en, lo.entity_type_en, lo.aliases, lo.categories, lo.entity_status,
       hi.entity, hi.entity_en, hi.entity_type_en, hi.aliases, hi.categories, hi.entity_status
FROM kb.entity_merge_candidates c
JOIN LATERAL (
    SELECT entity, entity_en, entity_type_en,
           COALESCE(aliases, '[]'::jsonb) AS aliases,
           COALESCE(categories, '[]'::jsonb) AS categories,
           entity_status, modify_time
    FROM kb.entities e WHERE e.entity_id = c.lo_entity_id
    ORDER BY (e.entity_status = 'extracted') DESC, e.modify_time DESC LIMIT 1
) lo ON true
JOIN LATERAL (
    SELECT entity, entity_en, entity_type_en,
           COALESCE(aliases, '[]'::jsonb) AS aliases,
           COALESCE(categories, '[]'::jsonb) AS categories,
           entity_status, modify_time
    FROM kb.entities e WHERE e.entity_id = c.hi_entity_id
    ORDER BY (e.entity_status = 'extracted') DESC, e.modify_time DESC LIMIT 1
) hi ON true
WHERE c.decision = 'pending'
  AND (c.run_id IS NULL OR lo.modify_time > $1 OR hi.modify_time > $1)
LIMIT $2`
	rows, err := s.DB.QueryContext(ctx, sel, watermarkFrom, batch)
	if err != nil {
		return nil, fmt.Errorf("(MID_26061713) select candidates: %w", err)
	}
	defer rows.Close()

	type scored struct {
		pair CandidatePair
	}
	var pending []scored
	for rows.Next() {
		var (
			cid                    int64
			loID, hiID             string
			la, le, lt, ls         string
			lAliases, lCategories  []byte
			ha, he, ht, hs         string
			hAliases, hCategories  []byte
		)
		if err := rows.Scan(
			&cid, &loID, &hiID,
			&la, &le, &lt, &lAliases, &lCategories, &ls,
			&ha, &he, &ht, &hAliases, &hCategories, &hs,
		); err != nil {
			return nil, fmt.Errorf("(MID_26061714) scan candidate: %w", err)
		}
		left := map[string]any{
			"entity": la, "entity_en": le, "entity_type_en": lt, "entity_status": ls,
			"aliases": scanJSONStrings(lAliases), "entity_categories": scanJSONStrings(lCategories),
		}
		right := map[string]any{
			"entity": ha, "entity_en": he, "entity_type_en": ht, "entity_status": hs,
			"aliases": scanJSONStrings(hAliases), "entity_categories": scanJSONStrings(hCategories),
		}
		sig := computeSignals(left, right)
		score := scoreCandidate(sig)
		pending = append(pending, scored{CandidatePair{
			CandidateID: cid, LoEntityID: loID, HiEntityID: hiID, Signals: sig, Score: score,
		}})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// 3. Persist the scores; return the scored pairs to the adjudicator.
	out := make([]CandidatePair, 0, len(pending))
	const upd = `UPDATE kb.entity_merge_candidates
	             SET signals = $1::jsonb, score = $2, run_id = $3
	             WHERE candidate_id = $4 AND decision = 'pending'`
	for _, p := range pending {
		sigJSON, _ := json.Marshal(p.pair.Signals)
		if _, err := s.DB.ExecContext(ctx, upd, sigJSON, p.pair.Score, runID, p.pair.CandidateID); err != nil {
			return nil, fmt.Errorf("(MID_26061715) score candidate %d: %w", p.pair.CandidateID, err)
		}
		out = append(out, p.pair)
	}
	return out, nil
}

// LoadEntity returns a representative row's identity fields for scoring/election.
func (s ReconcileSQLStore) LoadEntity(ctx context.Context, entityID string) (map[string]any, error) {
	const q = `
SELECT entity, entity_en, entity_type_en,
       COALESCE(aliases, '[]'::jsonb), COALESCE(categories, '[]'::jsonb), entity_status
FROM kb.entities WHERE entity_id = $1
ORDER BY (entity_status = 'extracted') DESC, modify_time DESC LIMIT 1`
	var (
		entity, en, typeEn, status string
		aliases, categories        []byte
	)
	err := s.DB.QueryRowContext(ctx, q, entityID).
		Scan(&entity, &en, &typeEn, &aliases, &categories, &status)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"entity_id": entityID, "entity": entity, "entity_en": en,
		"entity_type_en": typeEn, "entity_status": status,
		"aliases": scanJSONStrings(aliases), "entity_categories": scanJSONStrings(categories),
	}, nil
}

func (s ReconcileSQLStore) RecordDecision(ctx context.Context, candidateID int64, decision, method, by string) error {
	const q = `UPDATE kb.entity_merge_candidates
	           SET decision = $2, decided_method = $3, decided_by = $4, decided_at = now()
	           WHERE candidate_id = $1`
	_, err := s.DB.ExecContext(ctx, q, candidateID, decision, method, by)
	return err
}

// ApplyMerge folds from->into atomically: re-points relations, writes the
// provenance row, folds the loser, elects the head, and bumps the name dict.
func (s ReconcileSQLStore) ApplyMerge(ctx context.Context, m MergeDecision) (int64, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	repoint := func(col string) (int64, error) {
		q := fmt.Sprintf(`UPDATE kb.relations SET %s = $1 WHERE %s = $2`, col, col)
		res, err := tx.ExecContext(ctx, q, m.IntoEntityID, m.FromEntityID)
		if err != nil {
			return 0, err
		}
		return res.RowsAffected()
	}
	subN, err := repoint("subject_entity_id")
	if err != nil {
		return 0, fmt.Errorf("(MID_26061716) repoint subjects: %w", err)
	}
	objN, err := repoint("object_entity_id")
	if err != nil {
		return 0, fmt.Errorf("(MID_26061717) repoint objects: %w", err)
	}
	repointed := subN + objN

	// Fold the loser; promote it if it was provisional (now linked to a head).
	const fold = `
UPDATE kb.entities
SET canonical_entity_id = $1, reconcile_status = 'merged', reconciled_at = now(),
    entity_status = CASE WHEN entity_status = 'provisional' THEN 'extracted' ELSE entity_status END
WHERE entity_id = $2`
	if _, err := tx.ExecContext(ctx, fold, m.IntoEntityID, m.FromEntityID); err != nil {
		return 0, fmt.Errorf("(MID_26061718) fold loser: %w", err)
	}

	// Elect the head: point it at itself so kb.relations_resolved resolves.
	const head = `
UPDATE kb.entities
SET canonical_entity_id = $1, reconcile_status = 'clustered', reconciled_at = now()
WHERE entity_id = $1
  AND (canonical_entity_id IS NULL OR canonical_entity_id = '' OR canonical_entity_id = $1)`
	if _, err := tx.ExecContext(ctx, head, m.IntoEntityID); err != nil {
		return 0, fmt.Errorf("(MID_26061719) elect head: %w", err)
	}

	evidence, _ := json.Marshal(m.Evidence)
	var candID sql.NullInt64
	if m.CandidateID != 0 {
		candID = sql.NullInt64{Int64: m.CandidateID, Valid: true}
	}
	const ins = `
INSERT INTO kb.entity_merges (
    from_entity_id, into_entity_id, method, confidence, reason, evidence,
    candidate_id, relations_repointed, run_id, decided_by
) VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7,$8,$9,$10)`
	if _, err := tx.ExecContext(ctx, ins,
		m.FromEntityID, m.IntoEntityID, m.Method, m.Confidence, m.Reason, evidence,
		candID, repointed, nullableInt64(m.RunID), m.DecidedBy,
	); err != nil {
		return 0, fmt.Errorf("(MID_26061720) write merge provenance: %w", err)
	}

	// Best-effort: sync the survivor's English name into the dictionary (R8).
	if err := s.bumpEntityName(ctx, tx, m.IntoEntityID); err != nil {
		return 0, fmt.Errorf("(MID_26061721) sync entity_names: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return repointed, nil
}

func (s ReconcileSQLStore) bumpEntityName(ctx context.Context, tx *sql.Tx, entityID string) error {
	var nameRaw, nameEn string
	const q = `SELECT COALESCE(NULLIF(entity_en, ''), entity), COALESCE(entity_en, '')
	           FROM kb.entities WHERE entity_id = $1
	           ORDER BY (entity_status = 'extracted') DESC, modify_time DESC LIMIT 1`
	if err := tx.QueryRowContext(ctx, q, entityID).Scan(&nameRaw, &nameEn); err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	}
	key := normalizeSurfaceForm(nameEn)
	if key == "" {
		key = normalizeSurfaceForm(nameRaw)
	}
	if key == "" {
		return nil
	}
	const up = `
INSERT INTO kb.entity_names (name_key, name_raw, seen_count, last_seen_at)
VALUES ($1, $2, 1, now())
ON CONFLICT (name_key) DO UPDATE
SET seen_count = kb.entity_names.seen_count + 1, last_seen_at = now(), modify_time = now()`
	_, err := tx.ExecContext(ctx, up, key, nameRaw)
	return err
}

func (s ReconcileSQLStore) MarkClustered(ctx context.Context, entityIDs []string, status string) error {
	if len(entityIDs) == 0 {
		return nil
	}
	ids, _ := json.Marshal(entityIDs)
	const q = `
UPDATE kb.entities SET reconcile_status = $2, reconciled_at = now()
WHERE entity_id IN (SELECT value FROM jsonb_array_elements_text($1::jsonb))`
	_, err := s.DB.ExecContext(ctx, q, ids, status)
	return err
}

// ---- helpers ----

func scanJSONStrings(b []byte) []string {
	if len(b) == 0 {
		return nil
	}
	var out []string
	_ = json.Unmarshal(b, &out)
	return out
}

func nullableInt64(v int64) sql.NullInt64 {
	if v == 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: v, Valid: true}
}
