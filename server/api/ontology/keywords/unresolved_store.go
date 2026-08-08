package keywords

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// Unresolved is one backlog entry for a surface form that could not be
// resolved to a keyword concept. Keyed on (norm_key, scope).
type Unresolved struct {
	NormKey     string          `json:"norm_key"`
	Scope       string          `json:"scope"`
	Surfaces    []string        `json:"surfaces"`
	Contexts    []string        `json:"contexts"`
	Candidates  []TiedCandidate `json:"candidates"`
	Hits        int             `json:"hits"`
	Status      string          `json:"status"`
	Attempts    int             `json:"attempts"`
	LastAttempt *string         `json:"last_attempt"`
	Priority    float64         `json:"priority"`
	FirstSeen   time.Time       `json:"first_seen"`
	LastSeen    time.Time       `json:"last_seen"`
}

// TiedCandidate is one member of an `ambiguous` verdict's tied top-score set
// (spec 2026080403 §9.1/§13), captured at the moment the verdict is logged
// so an ambiguity reconciliation pass (Reconciler.ReconcileAmbiguous) has a
// concrete set to re-rank later without re-deriving it from decision-log
// JSON. Nil/empty for a plain no-candidate backlog row (Deferred/
// HumanReview) — only VerdictAmbiguous populates it.
type TiedCandidate struct {
	ConceptID string  `json:"concept_id"`
	Score     float64 `json:"score"`
	Method    string  `json:"method"`
}

// maxUnresolvedCandidates caps the persisted tied set the same way surfaces
// are capped at 10 — an unbounded tie set would mean unbounded row growth
// for a pathologically overloaded norm_key.
const maxUnresolvedCandidates = 10

// AllowedUnresolvedStatuses mirrors the schema CHECK constraint.
var AllowedUnresolvedStatuses = map[string]bool{
	"pending":              true,
	"batched":              true,
	"needs_human":          true,
	"resolved":             true,
	"junk":                 true,
	"insufficient_context": true,
}

// UnresolvedStore persists unresolved keyword backlog rows.
type UnresolvedStore struct {
	DB DBX
}

// UpsertUnresolved inserts or updates a backlog row. Surfaces are deduplicated
// and capped at 10; contexts keep a reservoir sample of ≤5 snippets at ≤200
// chars each. candidates is the tied concept set for an `ambiguous` verdict
// (nil for a plain no-candidate miss) — merged by ConceptID (latest score/
// method wins), capped the same way surfaces are.
func (s UnresolvedStore) UpsertUnresolved(ctx context.Context, normKey, scope, surface, contextText string, candidates []TiedCandidate) error {
	// Fetch current row if it exists.
	var (
		existingSurfaces   []byte
		existingContexts   []byte
		existingCandidates []byte
		hits               int
	)
	row := s.DB.QueryRowContext(ctx, `
		SELECT surfaces, contexts, candidates, hits FROM kb.keyword_unresolved
		WHERE norm_key = $1 AND scope = $2`, normKey, scope)
	err := row.Scan(&existingSurfaces, &existingContexts, &existingCandidates, &hits)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("upsert unresolved: %w", err)
	}

	var surfaces []string
	var contexts []string
	var tied []TiedCandidate
	if err == nil {
		// Existing row: merge.
		_ = json.Unmarshal(existingSurfaces, &surfaces)
		_ = json.Unmarshal(existingContexts, &contexts)
		_ = json.Unmarshal(existingCandidates, &tied)
	}
	tied = mergeTiedCandidates(tied, candidates)

	// Deduplicate and cap surfaces.
	if surface != "" {
		found := false
		for _, s := range surfaces {
			if s == surface {
				found = true
				break
			}
		}
		if !found {
			surfaces = append(surfaces, surface)
			if len(surfaces) > 10 {
				surfaces = surfaces[len(surfaces)-10:]
			}
		}
	}

	// Reservoir sample contexts (cap at 5, ≤200 runes each — byte-slicing
	// split multi-byte CJK into invalid UTF-8, §10.5).
	if contextText != "" {
		if runes := []rune(contextText); len(runes) > 200 {
			contextText = string(runes[:200])
		}
		contexts = append(contexts, contextText)
		if len(contexts) > 5 {
			contexts = contexts[len(contexts)-5:]
		}
	}

	surfacesJSON, _ := json.Marshal(surfaces)
	contextsJSON, _ := json.Marshal(contexts)
	candidatesJSON := nullableTiedCandidatesJSON(tied)

	if err == sql.ErrNoRows {
		_, err = s.DB.ExecContext(ctx, `
			INSERT INTO kb.keyword_unresolved (norm_key, scope, surfaces, contexts, candidates, hits)
			VALUES ($1, $2, $3, $4, $5, 1)`,
			normKey, scope, surfacesJSON, contextsJSON, candidatesJSON)
	} else {
		_, err = s.DB.ExecContext(ctx, `
			UPDATE kb.keyword_unresolved
			SET surfaces = $3, contexts = $4, candidates = $5, hits = hits + 1, last_seen = NOW()
			WHERE norm_key = $1 AND scope = $2`,
			normKey, scope, surfacesJSON, contextsJSON, candidatesJSON)
	}
	return err
}

// mergeTiedCandidates merges a new tied-candidate observation into the
// stored set by ConceptID (latest score/method for a repeated concept_id
// wins — ties can shift slightly across normalizer/tier-5 runs), capped at
// maxUnresolvedCandidates the same way surfaces are capped.
func mergeTiedCandidates(existing, incoming []TiedCandidate) []TiedCandidate {
	if len(incoming) == 0 {
		return existing
	}
	byID := make(map[string]TiedCandidate, len(existing)+len(incoming))
	order := make([]string, 0, len(existing)+len(incoming))
	for _, c := range existing {
		if _, ok := byID[c.ConceptID]; !ok {
			order = append(order, c.ConceptID)
		}
		byID[c.ConceptID] = c
	}
	for _, c := range incoming {
		if _, ok := byID[c.ConceptID]; !ok {
			order = append(order, c.ConceptID)
		}
		byID[c.ConceptID] = c
	}
	if len(order) > maxUnresolvedCandidates {
		order = order[len(order)-maxUnresolvedCandidates:]
	}
	merged := make([]TiedCandidate, 0, len(order))
	for _, id := range order {
		merged = append(merged, byID[id])
	}
	return merged
}

// nullableTiedCandidatesJSON marshals a tied-candidate set for storage, or
// returns SQL NULL for an empty set — a plain no-candidate backlog row must
// stay distinguishable from an ambiguous one (§13's ambiguity reconciliation
// scans WHERE candidates IS NOT NULL).
func nullableTiedCandidatesJSON(candidates []TiedCandidate) any {
	if len(candidates) == 0 {
		return nil
	}
	data, err := json.Marshal(candidates)
	if err != nil {
		return nil
	}
	return data
}

const unresolvedColumns = `norm_key, scope, surfaces, contexts, candidates, hits, status, attempts, last_attempt, priority, first_seen, last_seen`

// ListUnresolved returns unresolved entries, filtered by scope and status.
func (s UnresolvedStore) ListUnresolved(ctx context.Context, scope, status string, limit int) ([]Unresolved, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT `+unresolvedColumns+`
		FROM kb.keyword_unresolved
		WHERE scope = $1 AND status = $2
		ORDER BY priority DESC, hits DESC
		LIMIT $3`, scope, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Unresolved
	for rows.Next() {
		u, err := scanUnresolved(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// ListAmbiguous returns pending backlog rows carrying a tied-candidate set —
// i.e. rows an `ambiguous` verdict wrote, not a plain no-candidate miss
// (§13's ambiguity reconciliation, Reconciler.ReconcileAmbiguous). Ordered
// oldest-first, same convention as ListAutoCreatedProvisional.
func (s UnresolvedStore) ListAmbiguous(ctx context.Context, scope string, limit int) ([]Unresolved, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT `+unresolvedColumns+`
		FROM kb.keyword_unresolved
		WHERE scope = $1 AND status = 'pending' AND candidates IS NOT NULL
		ORDER BY first_seen
		LIMIT $2`, scope, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Unresolved
	for rows.Next() {
		u, err := scanUnresolved(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// UpdateUnresolvedStatus transitions a backlog entry's status and increments attempts.
func (s UnresolvedStore) UpdateUnresolvedStatus(ctx context.Context, normKey, scope, status, lastAttempt string) error {
	if !AllowedUnresolvedStatuses[status] {
		return fmt.Errorf("invalid unresolved status: %s", status)
	}
	_, err := s.DB.ExecContext(ctx, `
		UPDATE kb.keyword_unresolved
		SET status = $3, attempts = attempts + 1, last_attempt = $4, last_seen = NOW()
		WHERE norm_key = $1 AND scope = $2`,
		normKey, scope, status, nullableString(lastAttempt))
	return err
}

func scanUnresolved(scan func(dest ...any) error) (Unresolved, error) {
	var (
		u              Unresolved
		surfacesJSON   []byte
		contextsJSON   []byte
		candidatesJSON []byte
		lastAttempt    sql.NullString
	)
	if err := scan(&u.NormKey, &u.Scope, &surfacesJSON, &contextsJSON, &candidatesJSON, &u.Hits,
		&u.Status, &u.Attempts, &lastAttempt, &u.Priority, &u.FirstSeen, &u.LastSeen); err != nil {
		return Unresolved{}, err
	}
	_ = json.Unmarshal(surfacesJSON, &u.Surfaces)
	_ = json.Unmarshal(contextsJSON, &u.Contexts)
	if candidatesJSON != nil {
		_ = json.Unmarshal(candidatesJSON, &u.Candidates)
	}
	if u.Surfaces == nil {
		u.Surfaces = []string{}
	}
	if u.Contexts == nil {
		u.Contexts = []string{}
	}
	if u.Candidates == nil {
		u.Candidates = []TiedCandidate{}
	}
	if lastAttempt.Valid {
		u.LastAttempt = &lastAttempt.String
	}
	return u, nil
}
