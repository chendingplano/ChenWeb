// Package names implements the cross-family name resolution facade (§9.5):
// one call resolves a producer-asserted name through the governed term
// layer first, then the keyword family's lexical layer, and reports one of
// five statuses. Read and write are separate calls: ResolveName writes
// nothing — not even a decision-log row; the observe calls write exactly
// one occurrence linked to exactly one decision.
package names

import (
	"context"

	"github.com/chendingplano/deepdoc/server/api/ontology/keywords"
	"github.com/chendingplano/deepdoc/server/api/ontology/semid"
	"github.com/lib/pq"
)

// ResolutionStatus is the §9.5 outcome vocabulary. All five are normal
// results, not errors.
type ResolutionStatus string

const (
	StatusTermResolved    ResolutionStatus = "term_resolved"
	StatusLexicalResolved ResolutionStatus = "lexical_resolved"
	StatusAmbiguous       ResolutionStatus = "ambiguous"
	StatusUnresolved      ResolutionStatus = "unresolved"
	StatusDisabled        ResolutionStatus = "disabled"
)

// ResolveNameRequest is one name to resolve. Scope is the caller's input
// (K2) — the lexical search runs at exactly this scope. The expected-*
// fields filter the governed layer only.
type ResolveNameRequest struct {
	Name  string
	Scope string
	// ExpectedTermKinds filters released terms by term_kind
	// ("metric_definition", "unit", ...); empty accepts every kind.
	ExpectedTermKinds []string
	// ExpectedModules filters released terms by module_id; empty accepts
	// every module.
	ExpectedModules []string
	// Language filters released labels by lang; empty accepts every
	// language.
	Language string
}

// NameCandidate is one lexical candidate from the keyword family's tier
// queries — the tied set on ambiguous, the flagged top-1 on human_review.
type NameCandidate struct {
	ConceptID string
	PrefName  string
	Method    string
	Score     float64
}

// NameResolution is the two-layer answer. The ungoverned lexical layer
// (Concept*) and the governed layer (Term*) are reported separately; the
// layer rule is that TermID is only ever set on an unambiguous exact match
// against a released term's label — a lexical hit alone never produces a
// TermID.
type NameResolution struct {
	RawName       string
	NormalizedKey string
	Status        ResolutionStatus

	ConceptID       string
	ConceptPrefName string

	TermID       string
	TermPrefName string
	TermKind     string
	ModuleID     string

	Candidates []NameCandidate
	Method     string
	Confidence float64
}

// NameOccurrence is the consumer-supplied provenance of one observed name —
// the corrected K4 shape with consumer identity (artifact_type/artifact_id/
// field_path, e.g. "metric_name") left opaque to the resolver (§9.5).
type NameOccurrence struct {
	ArtifactType string
	ArtifactID   string
	FieldPath    string
	RawName      string
	Scope        string
	Context      string
	ChunkRef     string
}

// NameResolver is the read side of §9.5. The write side (ObserveName,
// ResolveAndObserve) is exposed on the same concrete Resolver.
type NameResolver interface {
	ResolveName(ctx context.Context, req ResolveNameRequest) (NameResolution, error)
	ResolveNames(ctx context.Context, reqs []ResolveNameRequest) ([]NameResolution, error)
}

// Resolver implements NameResolver plus the observe calls, composing the
// keyword family (lexical layer) with a direct lookup over the released
// ontology terms (governed layer).
type Resolver struct {
	Family *keywords.KeywordFamily
}

func NewResolver(family *keywords.KeywordFamily) *Resolver {
	return &Resolver{Family: family}
}

// ResolveName resolves one name and writes nothing — no occurrence, no
// decision-log row, no backlog (§9.5 read/write split). A disabled
// resolver is reported as StatusDisabled, not an error.
func (r *Resolver) ResolveName(ctx context.Context, req ResolveNameRequest) (NameResolution, error) {
	out := NameResolution{RawName: req.Name}
	if r.Family == nil || r.Family.Mode() == "off" {
		out.Status = StatusDisabled
		return out, nil
	}

	n := semid.Normalizer{Version: r.Family.NormVersion()}
	out.NormalizedKey = n.Normalize(req.Name).Norm

	// Governed layer first: an unambiguous exact match against a released
	// term's label wins the TermID.
	term, err := r.matchReleasedTerm(ctx, req, n)
	if err != nil {
		return out, err
	}

	// Lexical layer: the family's pure kernel run (no writes).
	res, err := r.Family.ResolveSurface(ctx, req.Name, req.Scope)
	if err != nil {
		return out, err
	}

	out.Candidates = r.candidates(ctx, res.Matches)
	out.Method = res.Method
	if len(res.Matches) > 0 {
		out.Confidence = res.Matches[0].Score
	}
	topPrefName := func() string {
		if len(out.Candidates) > 0 && out.Candidates[0].ConceptID == res.ResolvedNodeID {
			return out.Candidates[0].PrefName
		}
		return r.prefName(ctx, res.ResolvedNodeID)
	}

	switch res.Verdict {
	case semid.VerdictAutoAccept:
		out.Status = StatusLexicalResolved
		out.ConceptID = res.ResolvedNodeID
		out.ConceptPrefName = topPrefName()
	case semid.VerdictAmbiguous:
		out.Status = StatusAmbiguous
		out.ConceptID = res.ResolvedNodeID // top-1 (§8.3/D11)
		out.ConceptPrefName = topPrefName()
	default:
		// deferred and human_review: no committed concept on the read
		// path — auto-creation is a write and belongs to the observe
		// calls. The candidates keep a flagged top-1 visible.
		out.Status = StatusUnresolved
	}

	if term != nil {
		out.Status = StatusTermResolved
		out.TermID = term.termID
		out.TermPrefName = term.prefName
		out.TermKind = term.termKind
		out.ModuleID = term.moduleID
		out.Method = "term_exact"
		out.Confidence = 1.0
	}
	return out, nil
}

// ResolveNames resolves a batch of names; one failed request aborts the
// batch.
func (r *Resolver) ResolveNames(ctx context.Context, reqs []ResolveNameRequest) ([]NameResolution, error) {
	out := make([]NameResolution, 0, len(reqs))
	for _, req := range reqs {
		res, err := r.ResolveName(ctx, req)
		if err != nil {
			return nil, err
		}
		out = append(out, res)
	}
	return out, nil
}

// ObserveName records one producer-asserted name occurrence: it resolves,
// appends the decision log, writes the occurrence linked to that decision,
// and applies the verdict side effects — including D11 auto-creation, since
// an occurrence with field provenance is a targeted path. A disabled
// resolver writes nothing.
func (r *Resolver) ObserveName(ctx context.Context, occurrence NameOccurrence) error {
	if r.Family == nil || r.Family.Mode() == "off" {
		return nil
	}
	_, err := r.Family.ObserveOccurrence(ctx, familyOccurrence(occurrence), true)
	return err
}

// ResolveAndObserve resolves the request and records the occurrence in one
// call — exactly one decision-log row and one linked occurrence (§9.5).
// ResolveName contributes no writes of its own; the observe step re-runs
// the deterministic kernel to populate the row. The governed TermID from
// the resolution is carried into the occurrence, which marks it
// term_resolved.
func (r *Resolver) ResolveAndObserve(ctx context.Context, req ResolveNameRequest, occ NameOccurrence) (NameResolution, error) {
	res, err := r.ResolveName(ctx, req)
	if err != nil {
		return res, err
	}
	if res.Status == StatusDisabled {
		return res, nil
	}
	if occ.RawName == "" {
		occ.RawName = req.Name
	}
	if occ.Scope == "" {
		occ.Scope = req.Scope
	}
	kocc := familyOccurrence(occ)
	if res.TermID != "" {
		termID := res.TermID
		kocc.TermID = &termID
	}
	if _, err := r.Family.ObserveOccurrence(ctx, kocc, true); err != nil {
		return res, err
	}
	return res, nil
}

func familyOccurrence(occ NameOccurrence) *keywords.Occurrence {
	kocc := &keywords.Occurrence{
		ArtifactType: occ.ArtifactType,
		ArtifactID:   occ.ArtifactID,
		FieldPath:    occ.FieldPath,
		RawName:      occ.RawName,
		Scope:        occ.Scope,
		ContextText:  occ.Context,
	}
	if occ.ChunkRef != "" {
		chunkRef := occ.ChunkRef
		kocc.ChunkRef = &chunkRef
	}
	return kocc
}

// candidates materializes the kernel's scored matches into NameCandidates,
// looking up each concept's preferred label (read-only).
func (r *Resolver) candidates(ctx context.Context, matches []semid.ScoredMatch) []NameCandidate {
	if len(matches) == 0 {
		return nil
	}
	out := make([]NameCandidate, 0, len(matches))
	for _, m := range matches {
		out = append(out, NameCandidate{
			ConceptID: m.NodeID,
			PrefName:  r.prefName(ctx, m.NodeID),
			Method:    m.Method,
			Score:     m.Score,
		})
	}
	return out
}

func (r *Resolver) prefName(ctx context.Context, conceptID string) string {
	if conceptID == "" || r.Family == nil {
		return ""
	}
	c, err := r.Family.ConceptStore.GetConcept(ctx, conceptID)
	if err != nil {
		return ""
	}
	return c.PrefLabel
}

// releasedTermSQL joins governed terms to their labels; the release
// lifecycle sets status='included_in_release' on both tables
// (ontology/modules/releases_store.go), so the double filter selects
// exactly the released content. Term rows are versioned — dedup by term_id
// happens in Go.
//
// F2: term_kind/module_id/lang are pushed down here to cut the scanned and
// normalized row set — the pilot's QUDT import alone seeds 4151 quantity
// terms (§17.2), and this runs on every ResolveName call. This is a
// row-reduction, not the full fix: label comparison still needs Go-side
// normalization (NFKC/case-fold has no SQL equivalent here), so an
// unfiltered request still scans every released label. A persisted
// normalized-label column + index would close that gap; deferred.
const releasedTermSQL = `
SELECT t.term_id, t.term_kind, t.module_id, l.label, l.lang, l.label_role
FROM kb.ontology_terms t
JOIN kb.ontology_term_labels l ON l.term_id = t.term_id
WHERE t.status = 'included_in_release'
  AND l.status = 'included_in_release'
  AND ($1::text[] IS NULL OR t.term_kind = ANY($1))
  AND ($2::text[] IS NULL OR t.module_id = ANY($2))
  AND ($3 = '' OR l.lang = $3)`

type termHit struct {
	termID   string
	termKind string
	moduleID string
	prefName string
}

type labelRow struct {
	label string
	lang  string
	role  string
}

// matchReleasedTerm returns the single released term whose label matches
// the request's name after normalization, or nil. Comparison runs on Norm —
// the same NFKC/full-case-fold/whitespace key tier 1 matches on — never
// SQL LOWER (not full Unicode case folding) and never Exact (trim-space
// only: "luminance" would not match a released "Luminance", F1). Two or
// more distinct terms matching is ambiguous by definition, so no TermID is
// assigned and the call falls through to the lexical layer. Accepted
// aligns_to_term alignments (REQ-2/3) will extend this lookup once the
// predicate terms are seeded (§16.1).
func (r *Resolver) matchReleasedTerm(ctx context.Context, req ResolveNameRequest, n semid.Normalizer) (*termHit, error) {
	if r.Family == nil || r.Family.DB == nil {
		return nil, nil
	}
	queryKey := n.Normalize(req.Name).Norm
	if queryKey == "" {
		return nil, nil
	}

	rows, err := r.Family.DB.QueryContext(ctx, releasedTermSQL,
		arrayOrNil(req.ExpectedTermKinds), arrayOrNil(req.ExpectedModules), req.Language)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	terms := map[string]*termHit{}    // term_id -> hit skeleton
	labels := map[string][]labelRow{} // term_id -> released labels (filtered)
	for rows.Next() {
		var termID, termKind, moduleID, label, lang, role string
		if err := rows.Scan(&termID, &termKind, &moduleID, &label, &lang, &role); err != nil {
			return nil, err
		}
		// Authoritative in Go even though the SQL already filters (F2): the
		// mock-backed test suite can't evaluate a real WHERE clause, and
		// this stays correct regardless of what the driver returns.
		if len(req.ExpectedTermKinds) > 0 && !containsString(req.ExpectedTermKinds, termKind) {
			continue
		}
		if len(req.ExpectedModules) > 0 && !containsString(req.ExpectedModules, moduleID) {
			continue
		}
		if req.Language != "" && lang != req.Language {
			continue
		}
		if _, ok := terms[termID]; !ok {
			terms[termID] = &termHit{termID: termID, termKind: termKind, moduleID: moduleID}
		}
		labels[termID] = append(labels[termID], labelRow{label: label, lang: lang, role: role})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var matched []string
	for termID, ls := range labels {
		for _, l := range ls {
			if n.Normalize(l.label).Norm == queryKey {
				matched = append(matched, termID)
				break
			}
		}
	}
	if len(matched) != 1 {
		// Miss or ambiguous: the layer rule withholds the TermID (§9.5).
		return nil, nil
	}
	hit := terms[matched[0]]
	hit.prefName = pickPrefName(labels[hit.termID], req.Language)
	return hit, nil
}

// pickPrefName prefers the released prefLabel in the requested language,
// then any prefLabel, then any label — the governed layer's own ordering.
func pickPrefName(labels []labelRow, lang string) string {
	fallbackPref := ""
	fallbackAny := ""
	for _, l := range labels {
		if fallbackAny == "" {
			fallbackAny = l.label
		}
		if l.role != "prefLabel" {
			continue
		}
		if fallbackPref == "" {
			fallbackPref = l.label
		}
		if lang == "" || l.lang == lang {
			return l.label
		}
	}
	if fallbackPref != "" {
		return fallbackPref
	}
	return fallbackAny
}

// arrayOrNil produces a driver value for the $1::text[] IS NULL OR ... form:
// an empty/nil slice ("accepts every kind") must bind to SQL NULL, not an
// empty array literal, which would match nothing.
func arrayOrNil(list []string) any {
	if len(list) == 0 {
		return nil
	}
	return pq.Array(list)
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
