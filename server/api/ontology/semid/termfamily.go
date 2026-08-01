package semid

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
)

// TermFamily adapts the kernel to the ontology-term identity family (DR15):
// surfaces are term proposals (kb.ontology_candidates), canonical nodes are
// governed terms (kb.ontology_terms), scope is the module. Terms are governed,
// so the auto-accept policy is disabled and adjudication always ends at a
// reviewable change set -- the kernel never auto-accepts a term match.
type TermFamily struct {
	DB                *sql.DB
	NormalizerVersion int
}

// FamilyName implements FamilyAdapter.
func (f TermFamily) FamilyName() string { return "ontology_term" }

// Normalizer implements FamilyAdapter.
func (f TermFamily) Normalizer() Normalizer {
	if f.NormalizerVersion <= 0 {
		f.NormalizerVersion = 1
	}
	return Normalizer{Name: "basic", Version: f.NormalizerVersion}
}

// AutoAcceptPolicy implements FamilyAdapter. Governed: always off.
func (f TermFamily) AutoAcceptPolicy() AutoAcceptPolicy {
	return AutoAcceptPolicy{Enabled: false, MinScore: 0}
}

// Scope implements FamilyAdapter: the term family scopes by module.
func (f TermFamily) Scope(surface string) string { return "" }

// CandidateNodes implements FamilyAdapter. It blocks on the module scope and
// returns existing terms whose term_id or a label matches the surface
// (lexically, after the normalizer). Blocking is intentionally cheap; richer
// trigram/vector blocking joins the keyword family in P3.
func (f TermFamily) CandidateNodes(ctx context.Context, surface, scope string) ([]NodeCandidate, error) {
	if f.DB == nil {
		return nil, nil
	}
	bundle := f.Normalizer().Normalize(surface)
	key := bundle.CanonicalKey
	if key == "" {
		return nil, nil
	}
	const stmt = `
SELECT DISTINCT t.term_id, COALESCE(l.label, '')
FROM kb.ontology_terms t
LEFT JOIN LATERAL (
	SELECT l.label
	FROM kb.ontology_term_labels l
	WHERE l.term_id = t.term_id
	  AND LOWER(l.label) = $2
	  AND l.status NOT IN ('rejected', 'superseded')
	LIMIT 1
) l ON true
WHERE ($1 = '' OR t.module_id = $1)
  AND t.status NOT IN ('rejected', 'superseded')
  AND (t.term_id = $2 OR LOWER(l.label) = $2)`
	rows, err := f.DB.QueryContext(ctx, stmt, scope, key)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]NodeCandidate, 0)
	for rows.Next() {
		var id, label string
		if err := rows.Scan(&id, &label); err != nil {
			return nil, err
		}
		kb := f.Normalizer().Normalize(label)
		if kb.CanonicalKey == "" {
			kb = KeyBundle{CanonicalKey: key} // term_id match, no label
		}
		out = append(out, NodeCandidate{NodeID: id, KeyBundle: kb})
	}
	return out, rows.Err()
}

// TermCandidatePayload is the subset of a term candidate's proposed payload
// the kernel needs to resolve it: a proposed term_id and, when present, a
// surface label.
type TermCandidatePayload struct {
	TermID string `json:"term_id"`
	Label  string `json:"label"`
}

// ResolveCandidate runs the kernel over a term candidate in kb.ontology_candidates
// and records the result into the candidate's candidate_matches plus an
// append-only decision-log row. The candidate must exist; its status is not
// changed here (the governed lifecycle and change-set application are separate
// acts).
func (f TermFamily) ResolveCandidate(ctx context.Context, candidateID int64, surface string) (Resolution, error) {
	if f.DB == nil {
		return Resolution{}, errNoDB
	}
	var (
		payload []byte
		module  sql.NullString
	)
	err := f.DB.QueryRowContext(ctx, `
SELECT proposed_payload, proposed_module_id
FROM kb.ontology_candidates
WHERE id = $1`, candidateID).Scan(&payload, &module)
	if err != nil {
		return Resolution{}, err
	}

	var cand TermCandidatePayload
	_ = json.Unmarshal(payload, &cand)
	if strings.TrimSpace(surface) == "" {
		surface = cand.Label
	}
	if strings.TrimSpace(surface) == "" {
		surface = localName(cand.TermID)
	}
	scope := ""
	if module.Valid {
		scope = module.String
	}

	res, err := (Kernel{Family: f}).Resolve(ctx, surface)
	if err != nil {
		return Resolution{}, err
	}
	res.Scope = scope

	matchesJSON, _ := json.Marshal(res.Matches)
	if _, err := f.DB.ExecContext(ctx, `
UPDATE kb.ontology_candidates
SET candidate_matches = $2::jsonb, modify_time = NOW()
WHERE id = $1`, candidateID, string(matchesJSON)); err != nil {
		return Resolution{}, err
	}

	input, _ := json.Marshal(map[string]any{"candidate_id": candidateID, "surface": surface, "scope": scope})
	output, _ := json.Marshal(res)
	_ = (DecisionLogStore{DB: f.DB}).Append(ctx, DecisionLogEntry{
		Family:  f.FamilyName(),
		Scope:   scope,
		Input:   input,
		Output:  output,
		Verdict: string(res.Verdict),
		Actor:   "semid_term_family",
	})
	return res, nil
}

func localName(id string) string {
	if i := strings.LastIndex(id, ":"); i >= 0 {
		return id[i+1:]
	}
	return id
}

var errNoDB = errorString("semid: database handle is nil")
