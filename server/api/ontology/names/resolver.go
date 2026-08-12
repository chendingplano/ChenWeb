// Package names implements the cross-family name resolution facade (§9.5):
// one call resolves a producer-asserted name through the governed term
// layer first, then the keyword family's lexical layer, and reports one of
// five statuses. Read and write are separate calls: ResolveName writes
// nothing — not even a decision-log row; the observe calls write exactly
// one occurrence linked to exactly one decision.
package names

import (
	"context"
	"errors"

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
	// alignments, when wired, lets an accepted aligns_to_term carry a concept
	// to StatusTermResolved even when the raw name is not a released label
	// (REQ-2, §16.2), and mints alignments on the observe path for concepts
	// whose pref_label exactly matches a released metric_definition label
	// (§16.1). Nil keeps today's behavior — every existing caller is unchanged.
	alignments *keywords.AlignmentsStore
}

func NewResolver(family *keywords.KeywordFamily) *Resolver {
	return &Resolver{Family: family}
}

// NewResolverWithAlignments returns a resolver that reads (and, on the
// observe path, writes) aligns_to_term assertions (spec step 12, REQ-2).
func NewResolverWithAlignments(family *keywords.KeywordFamily, alignments *keywords.AlignmentsStore) *Resolver {
	return &Resolver{Family: family, alignments: alignments}
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
	// REQ-2 (spec §16.2): an accepted aligns_to_term on the resolved concept is
	// the second way a name reaches a governed term — the cross-lingual/merged
	// case, where the raw string is not a released label but the concept it
	// resolved to is aligned to one. The governed layer's own exact label match
	// (term != nil above) stays authoritative, so this runs only on the
	// no-governed-hit path; a read error degrades to the lexical result rather
	// than failing the resolve.
	if term == nil && r.alignments != nil && res.Verdict == semid.VerdictAutoAccept && res.ResolvedNodeID != "" {
		if hit, err := r.alignments.AcceptedForConcept(ctx, r.alignments.Assertions.DB, res.ResolvedNodeID); err == nil && hit != nil {
			out.Status = StatusTermResolved
			out.TermID = hit.ObjectTermID
			out.TermKind = "metric_definition"
			out.Method = "aligns_to_term"
			out.Confidence = 1.0
			out.TermPrefName = r.termPrefName(ctx, hit.ObjectTermID)
		}
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
	// §16.1 D11 auto-assign producer: a concept whose pref_label exactly matches
	// a released metric_definition label is aligned to it (exact normalized match
	// only — conservative, per D10's under-merge bias). The freshly minted
	// alignment is applied to this resolution so the returned result and the
	// occurrence below both see term_resolved. A conflict (already aligned to a
	// different term) is a state, not a failure: it is not overridden and the
	// observe proceeds; any other error propagates (the audit row for the
	// observe must not be lost behind a swallowed alignment error, DR15).
	if r.alignments != nil && res.Status == StatusLexicalResolved && res.ConceptID != "" && res.ConceptPrefName != "" {
		term, err := r.matchLabelToReleasedTerm(ctx, res.ConceptPrefName, []string{"metric_definition"})
		if err != nil {
			return res, err
		}
		if term != nil {
			if _, err := r.alignments.EnsureAccepted(ctx, res.ConceptID, term.termID, "term_exact", 1.0,
				"pref_label exact match to a released metric_definition label (auto-assign, §16.1)"); err != nil {
				if !errors.Is(err, keywords.ErrAlignmentConflict) {
					return res, err
				}
			} else {
				res.Status = StatusTermResolved
				res.TermID = term.termID
				res.TermKind = "metric_definition"
				res.Method = "aligns_to_term"
				res.TermPrefName = term.prefName
				res.Confidence = 1.0
			}
		}
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
	observed, err := r.Family.ObserveOccurrence(ctx, kocc, true)
	if err != nil {
		return res, err
	}
	// D11 auto-creation is a write-path effect: on a targeted deferred/
	// human_review miss, ObserveOccurrence mints a provisional concept and
	// assigns it (ResolvedNodeID), but the read-only ResolveName pass above
	// never sees it (auto-creation deliberately does not happen on the read
	// path). Without this, the newly created concept exists in
	// kb.keyword_concepts but is invisible to this call's own caller —
	// res.Status stays "unresolved" and res.ConceptID stays empty even
	// though a real assignment was just made. Only fills in what ResolveName
	// left empty; an existing term/concept resolution from the read pass is
	// never overwritten.
	if res.ConceptID == "" && observed != nil && observed.ResolvedNodeID != "" {
		res.ConceptID = observed.ResolvedNodeID
		res.ConceptPrefName = r.prefName(ctx, observed.ResolvedNodeID)
		res.Method = observed.Method
		if len(observed.Matches) > 0 {
			res.Confidence = observed.Matches[0].Score
		}
		switch observed.Verdict {
		case semid.VerdictAmbiguous:
			res.Status = StatusAmbiguous
		default:
			// VerdictAutoAccept, or a targeted VerdictDeferred/VerdictHumanReview
			// resolved via D11 auto-creation — both are a real, usable concept
			// assignment from the caller's point of view.
			res.Status = StatusLexicalResolved
		}
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

// termPrefName returns a released term's preferred label (best-effort; "" on
// error or absence) — used only for the alignment-follow resolution's
// TermPrefName, which is display, not identity.
func (r *Resolver) termPrefName(ctx context.Context, termID string) string {
	if termID == "" || r.Family == nil || r.Family.DB == nil {
		return ""
	}
	var label string
	err := r.Family.DB.QueryRowContext(ctx, `
SELECT l.label FROM kb.ontology_term_labels l
WHERE l.term_id = $1 AND l.status = 'included_in_release'
ORDER BY (l.label_role = 'prefLabel') DESC, (l.lang = 'en') DESC
LIMIT 1`, termID).Scan(&label)
	if err != nil {
		return ""
	}
	return label
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
	return r.lookupReleasedTerm(ctx, queryKey, req.ExpectedTermKinds, req.ExpectedModules, req.Language)
}

// lookupReleasedTerm returns the single released term with a released label
// whose normalization equals queryKey (kinds/modules/lang-filtered), or nil.
// Exactly-one-distinct-term wins; a miss or an ambiguous multi-term match
// withholds the TermID (§9.5 layer rule).
func (r *Resolver) lookupReleasedTerm(ctx context.Context, queryKey string, kinds, modules []string, lang string) (*termHit, error) {
	rows, err := r.Family.DB.QueryContext(ctx, releasedTermSQL,
		arrayOrNil(kinds), arrayOrNil(modules), lang)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	n := semid.Normalizer{Version: r.Family.NormVersion()}

	terms := map[string]*termHit{}    // term_id -> hit skeleton
	labels := map[string][]labelRow{} // term_id -> released labels (filtered)
	for rows.Next() {
		var termID, termKind, moduleID, label, rowLang, role string
		if err := rows.Scan(&termID, &termKind, &moduleID, &label, &rowLang, &role); err != nil {
			return nil, err
		}
		// Authoritative in Go even though the SQL already filters (F2): the
		// mock-backed test suite can't evaluate a real WHERE clause, and
		// this stays correct regardless of what the driver returns.
		if len(kinds) > 0 && !containsString(kinds, termKind) {
			continue
		}
		if len(modules) > 0 && !containsString(modules, moduleID) {
			continue
		}
		if lang != "" && rowLang != lang {
			continue
		}
		if _, ok := terms[termID]; !ok {
			terms[termID] = &termHit{termID: termID, termKind: termKind, moduleID: moduleID}
		}
		labels[termID] = append(labels[termID], labelRow{label: label, lang: rowLang, role: role})
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
	hit.prefName = pickPrefName(labels[hit.termID], lang)
	return hit, nil
}

// MatchUnitLabel resolves a raw unit string (e.g. kb.metrics.metric_unit)
// to a released `unit`-kind term id, or "" if no exact governed label
// matches. Exported wrapper around matchLabelToReleasedTerm (ADR 2026081201
// DR3): reuses the same governed-label-exact-match logic the rest of this
// package already relies on, rather than a separate hardcoded unit map.
func (r *Resolver) MatchUnitLabel(ctx context.Context, unit string) (string, error) {
	hit, err := r.matchLabelToReleasedTerm(ctx, unit, []string{"unit"})
	if err != nil || hit == nil {
		return "", err
	}
	return hit.termID, nil
}

// matchLabelToReleasedTerm is the same lookup against an arbitrary label
// (not a ResolveNameRequest) — used by the observe-path auto-align to test
// a concept's pref_label against released metric_definition labels (§16.1).
func (r *Resolver) matchLabelToReleasedTerm(ctx context.Context, label string, kinds []string) (*termHit, error) {
	if r.Family == nil || r.Family.DB == nil {
		return nil, nil
	}
	n := semid.Normalizer{Version: r.Family.NormVersion()}
	queryKey := n.Normalize(label).Norm
	if queryKey == "" {
		return nil, nil
	}
	return r.lookupReleasedTerm(ctx, queryKey, kinds, nil, "")
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
