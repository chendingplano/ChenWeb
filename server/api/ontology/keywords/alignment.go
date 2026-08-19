package keywords

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/ontology/assertions"
	"github.com/chendingplano/deepdoc/server/api/ontology/semid"
	"github.com/chendingplano/deepdoc/server/api/ontology/terms"
)

// ErrAlignmentConflict is returned when an accepted aligns_to_term assertion
// already exists for a concept to a *different* governed term (spec §14.2's
// conflict — two concepts aligned to two distinct terms are evidence they are
// not the same thing; a single concept aligned to two terms is ambiguous
// identity, which the module must never auto-decide).
var ErrAlignmentConflict = errors.New("alignment conflict: concept already aligned to a different term")

// errNotReleased is the wrapped cause of a released-guard failure: the
// alignment's object term (or the predicate itself) is not released governed
// content (spec §16.2 — only released terms are eligible to be the bridge's
// governed side), which the module refuses to bridge to.
var errNotReleased = errors.New("not a released term")

// alignPredicateTermID is the seeded predicate id (Task 2). A constant rather
// than a lookup because ontology-seed authors exactly this id.
const alignPredicateTermID = "core:aligns_to_term"

// acceptedForConceptSQL reads the current (latest-revision) accepted
// aligns_to_term row for one keyword concept. The latest-revision subselect
// has the same shape as AssertionStore.ListBySubjectObject so a superseded
// revision never wins.
const acceptedForConceptSQL = `
SELECT id, subject_ref_id, object_ref_kind, object_ref_id, status, qualifiers, confidence, decision_reason
FROM kb.semantic_assertions
WHERE subject_ref_kind = 'keyword_concept'
  AND subject_ref_id = $1
  AND predicate_term_id = 'core:aligns_to_term'
  AND status = 'accepted'
  AND revision = (SELECT MAX(revision) FROM kb.semantic_assertions a2
                  WHERE a2.logical_identity_key = kb.semantic_assertions.logical_identity_key)
ORDER BY id DESC LIMIT 1`

// releasedTermExistsSQL is the released-guard probe: a governed term must be
// usable content -- curator-released (status='included_in_release' on
// kb.ontology_terms) or auto-promoted (ADR 2026081201 DR2/DR4: usable
// immediately, distinguished only for attribution, not gated) -- and the
// expected term_kind for the slot it fills. names/resolver.go's
// releasedTermSQL (a different call site: matching a raw name string to a
// governed label) intentionally stays included_in_release-only -- it is
// not this guard.
const releasedTermExistsSQL = `
SELECT EXISTS (
  SELECT 1 FROM kb.ontology_terms t
  WHERE t.term_id = $1 AND t.status IN ('included_in_release', 'auto-promoted') AND t.term_kind = $2)`

// followMergeSQL re-points accepted keyword_concept aligns_to_term rows from
// the absorbed concept to the survivor.
const followMergeSQL = `
UPDATE kb.semantic_assertions
SET subject_ref_id = $2, modify_time = NOW()
WHERE subject_ref_kind = 'keyword_concept'
  AND subject_ref_id = $1
  AND predicate_term_id = 'core:aligns_to_term'
  AND status = 'accepted'`

// Alignment is the keyword-facing projection of one accepted aligns_to_term
// row.
type Alignment struct {
	ID           int64
	ConceptID    string   // subject_ref_id
	ObjectTermID string   // object_ref_id
	Method       string   // qualifiers->method
	Score        *float64 // confidence
	Evidence     string   // decision_reason
}

// AlignmentsStore persists and queries accepted aligns_to_term assertions
// (spec §16.1/§16.2, REQ-2): the keyword-facing bridge from a concept to the
// governed term it is an accepted alias of. Writes go through the assertions
// store — its logical_identity_key revision semantics (spec §10.7) come for
// free — and every real write is audited in the semid decision log (DR15).
type AlignmentsStore struct {
	Assertions  assertions.AssertionStore // writes: revision semantics for free
	DecisionLog semid.DecisionLogStore    // DR15 audit on a real write
	Scope       string                    // semid scope for the decision log
}

// AcceptedForConcept returns the current accepted alignment for a concept, or
// (nil, nil) when it has none. It takes an explicit db so a caller inside its
// own transaction can run the read under the transaction handle.
func (s AlignmentsStore) AcceptedForConcept(ctx context.Context, db DBX, conceptID string) (*Alignment, error) {
	if db == nil {
		return nil, errors.New("db is nil")
	}
	var (
		id             int64
		subjectRefID   string
		objectRefKind  sql.NullString
		objectRefID    sql.NullString
		status         string
		qualifiers     json.RawMessage
		confidence     sql.NullFloat64
		decisionReason sql.NullString
	)
	err := db.QueryRowContext(ctx, acceptedForConceptSQL, conceptID).Scan(
		&id, &subjectRefID, &objectRefKind, &objectRefID, &status,
		&qualifiers, &confidence, &decisionReason,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	al := &Alignment{
		ID:           id,
		ConceptID:    subjectRefID,
		ObjectTermID: objectRefID.String,
		Method:       methodOf(qualifiers),
		Evidence:     decisionReason.String,
	}
	if confidence.Valid {
		v := confidence.Float64
		al.Score = &v
	}
	return al, nil
}

// MergeConflict reports whether two concepts cannot be merged on alignment
// grounds (spec §14.2): true only when both concepts have accepted alignments
// to *different* terms — two concepts aligned to two distinct terms are
// evidence they are not the same thing, so the merge must be refused. Either
// concept unaligned, or both aligned to the same term, is not a conflict.
func (s AlignmentsStore) MergeConflict(ctx context.Context, db DBX, a, b string) (bool, error) {
	alA, err := s.AcceptedForConcept(ctx, db, a)
	if err != nil {
		return false, err
	}
	alB, err := s.AcceptedForConcept(ctx, db, b)
	if err != nil {
		return false, err
	}
	if alA == nil || alB == nil {
		return false, nil
	}
	return alA.ObjectTermID != alB.ObjectTermID, nil
}

// FollowMerge re-points accepted keyword_concept aligns_to_term rows from the
// absorbed concept to the survivor (spec §14.1, REQ-2/3). It runs on the
// passed db so a caller merging inside a transaction carries the re-point
// atomically with the tombstone. No decision-log row is written here: the
// caller owns the merge transaction's audit boundary.
func (s AlignmentsStore) FollowMerge(ctx context.Context, db DBX, absorbedID, survivorID string) error {
	if db == nil {
		return errors.New("db is nil")
	}
	tx, ok := db.(*sql.Tx)
	if !ok {
		return errors.New("alignment follow-merge requires caller-owned transaction")
	}
	if err := semid.AcquireKeywordIdentityMutationLock(ctx, tx); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, followMergeSQL, absorbedID, survivorID)
	return err
}

// EnsureAccepted proposes-and-accepts a concept's aligns_to_term alignment
// (spec §16.1/§16.2, REQ-2) on the observe path. It is idempotent: aligning a
// concept to the same term twice is a no-op. Conflicts and missing released
// content are refused before any write. On a real write it appends a
// keyword_align decision-log row (DR15); the no-op path appends nothing.
func (s AlignmentsStore) EnsureAccepted(ctx context.Context, conceptID, termID, method string, score float64, evidence string) (Alignment, error) {
	if s.Assertions.DB == nil {
		return Alignment{}, errors.New("db is nil")
	}
	var result Alignment
	err := withKeywordIdentityMutation(ctx, s.Assertions.DB, func(db DBX) error {
		txStore := s
		txStore.Assertions.DB = db
		txStore.DecisionLog.DB = db
		var err error
		result, err = txStore.ensureAccepted(ctx, db, conceptID, termID, method, score, evidence)
		return err
	})
	return result, err
}

func (s AlignmentsStore) ensureAccepted(ctx context.Context, db DBX, conceptID, termID, method string, score float64, evidence string) (Alignment, error) {

	// Conflict gate (spec §14.2): never auto-decide between two distinct terms
	// for one concept.
	existing, err := s.AcceptedForConcept(ctx, db, conceptID)
	if err != nil {
		return Alignment{}, err
	}
	if existing != nil && existing.ObjectTermID != termID {
		return Alignment{}, fmt.Errorf("%w: concept %s already aligned to %s (wanted %s)",
			ErrAlignmentConflict, conceptID, existing.ObjectTermID, termID)
	}

	// Released guard (spec §16.2): the object term must be a released
	// metric_definition and the predicate a released property. A guard, not a
	// blocker — Task 2 seeds the predicate and production seeds the object
	// term before alignment runs.
	for _, g := range []struct{ termID, kind string }{
		{termID, "metric_definition"},
		{alignPredicateTermID, "property"},
	} {
		ok, err := releasedTermExists(ctx, db, g.termID, g.kind)
		if err != nil {
			return Alignment{}, err
		}
		if !ok {
			return Alignment{}, fmt.Errorf("cannot align: %q not a released %s term: %w", g.termID, g.kind, errNotReleased)
		}
	}

	// Idempotency via the opaque identity key (kind-prefixed,
	// colon-separated, exact-match only — never parsed back).
	lk := fmt.Sprintf("kwc:%s:%s:%s", conceptID, alignPredicateTermID, termID)
	qualifiers, err := json.Marshal(map[string]string{"method": method, "evidence": evidence})
	if err != nil {
		return Alignment{}, err
	}
	next := assertions.Assertion{
		LogicalIdentityKey: lk,
		SubjectRefKind:     "keyword_concept",
		SubjectRefID:       conceptID,
		PredicateTermID:    alignPredicateTermID,
		ObjectRefKind:      "ontology_term",
		ObjectRefID:        termID,
		Status:             assertions.StatusAccepted,
		Polarity:           "positive",
		Qualifiers:         qualifiers,
		Confidence:         &score,
		DecisionReason:     evidence,
	}

	prior, err := s.Assertions.GetLatest(ctx, lk)
	if err == nil && prior.Status == assertions.StatusAccepted {
		// No-op: this exact identity is already aligned and accepted.
		return projectAlignment(prior), nil
	}

	var created assertions.Assertion
	if err != nil {
		created, err = s.Assertions.CreateAssertion(ctx, next)
	} else {
		// A stale (non-accepted) prior revision exists for this identity:
		// supersede it with a new revision rather than writing a duplicate.
		created, err = s.Assertions.CreateRevision(ctx, next)
	}
	if err != nil {
		return Alignment{}, err
	}

	// DR15 audit row on a real write. The assertion and audit stores are bound
	// to the same transaction by EnsureAccepted. The returned id is ignored:
	// nothing links to it yet, but a failed append must still roll back both.
	input, _ := json.Marshal(map[string]string{"concept_id": conceptID, "term_id": termID})
	output, _ := json.Marshal(created)
	if _, err := s.DecisionLog.Append(ctx, semid.DecisionLogEntry{
		Family:  "keyword_align",
		Scope:   s.Scope,
		Input:   input,
		Output:  output,
		Verdict: "accepted",
		Actor:   "auto-align",
	}); err != nil {
		return Alignment{}, err
	}
	return projectAlignment(created), nil
}

// TermSynthesisInput is the structural, already-extracted data a caller
// supplies to auto-create a metric_definition term (ADR 2026081201 DR3). No
// field is authored by an LLM here -- CanonicalName/Definition/ValueType/
// RangeType/PermittedUnitTermIDs are transcribed from the triggering
// metric's own extracted fields (or left empty, never guessed). Unit
// resolution (a raw unit string -> a released `unit` term id) is the
// caller's job -- keywords must not depend on the names package (names
// already depends on keywords; a reverse import would cycle).
type TermSynthesisInput struct {
	CanonicalName        string
	Aliases              []string
	Definition           string
	ValueType            string
	RangeType            string
	PermittedUnitTermIDs []string
}

// EnsureAcceptedOrCreate is EnsureAccepted extended with ADR 2026081201 DR1:
// when a concept has no existing accepted aligns_to_term, it auto-creates a
// new metric_definition term (status='auto-promoted') from synth and
// aligns the concept to it, instead of leaving the concept unaligned until
// a curator promotes a candidate. The existing-alignment path is byte-for-
// byte EnsureAccepted's (same conflict gate, same idempotency, same
// decision-log write) -- this only adds the create-if-absent branch.
//
// No tier 0-6 fuzzy matching runs here (design.md D1): a concept_id is
// already a deduplicated identity, so the only question left is existence,
// not similarity. The term-insert and the alignment-insert happen in one
// transaction (D1 step 3): a term with no alignment, or an alignment to a
// nonexistent term, must never be observable.
func (s AlignmentsStore) EnsureAcceptedOrCreate(ctx context.Context, conceptID string, synth TermSynthesisInput, method string, score float64, evidence string) (Alignment, error) {
	if s.Assertions.DB == nil {
		return Alignment{}, errors.New("db is nil")
	}
	if strings.TrimSpace(conceptID) == "" {
		return Alignment{}, errors.New("concept id is required")
	}
	if strings.TrimSpace(synth.CanonicalName) == "" {
		return Alignment{}, errors.New("canonical name is required to auto-create a term")
	}
	var result Alignment
	err := withKeywordIdentityMutation(ctx, s.Assertions.DB, func(db DBX) error {
		txStore := s
		txStore.Assertions.DB = db
		txStore.DecisionLog.DB = db

		// Existing-alignment path: identical to EnsureAccepted, run inside
		// this same transaction so a concurrent creator's write is visible.
		existing, err := txStore.AcceptedForConcept(ctx, db, conceptID)
		if err != nil {
			return err
		}
		if existing != nil {
			result = *existing
			return nil
		}

		termID := autoPromotedTermID(conceptID)
		created, err := terms.TermStore{DB: db}.CreateTerm(ctx, terms.Term{
			TermID:               termID,
			TermKind:             "metric_definition",
			ModuleID:             "measurement",
			Status:               "auto-promoted",
			Definition:           strings.TrimSpace(synth.Definition),
			Scope:                "document-derived, auto-promoted (ADR 2026081201)",
			ValueType:            strings.TrimSpace(synth.ValueType),
			RangeType:            strings.TrimSpace(synth.RangeType),
			PermittedUnitTermIDs: synth.PermittedUnitTermIDs,
		})
		if err != nil {
			return fmt.Errorf("auto-create metric_definition term: %w", err)
		}
		labelStore := terms.LabelStore{DB: db}
		if _, err := labelStore.CreateLabel(ctx, terms.TermLabel{
			TermID:    termID,
			Label:     synth.CanonicalName,
			Lang:      autoPromotedLabelLanguage(synth.CanonicalName),
			LabelRole: "prefLabel",
			Status:    "auto-promoted",
		}); err != nil {
			return fmt.Errorf("auto-create term prefLabel: %w", err)
		}
		for _, alias := range synth.Aliases {
			alias = strings.TrimSpace(alias)
			if alias == "" || alias == synth.CanonicalName {
				continue
			}
			if _, err := labelStore.CreateLabel(ctx, terms.TermLabel{
				TermID:    termID,
				Label:     alias,
				Lang:      autoPromotedLabelLanguage(alias),
				LabelRole: "altLabel",
				Status:    "auto-promoted",
			}); err != nil {
				return fmt.Errorf("auto-create term altLabel %q: %w", alias, err)
			}
		}

		aligned, err := txStore.ensureAccepted(ctx, db, conceptID, created.TermID, method, score, evidence)
		if err != nil {
			return err
		}
		result = aligned
		return nil
	})
	return result, err
}

// autoPromotedTermID derives a deterministic, collision-free term id from
// the concept id it is being created for -- concept_id is already the
// deduplicated identity (D1), so folding it directly into the term id (as
// opposed to slugifying the human-readable label, which two distinct
// concepts could coincidentally collide on) guarantees exactly one term per
// concept without a uniqueness race. The lifecycle state belongs in the
// ontology_terms.status column, not in the stable identifier, so promotion
// from auto-promoted to approved does not leave an "auto" marker in the ID.
func autoPromotedTermID(conceptID string) string {
	return "measurement:" + conceptID
}

// releasedTermExists probes whether a term is released governed content of
// the expected kind.
func releasedTermExists(ctx context.Context, db DBX, termID, termKind string) (bool, error) {
	var exists bool
	err := db.QueryRowContext(ctx, releasedTermExistsSQL, termID, termKind).Scan(&exists)
	return exists, err
}

// projectAlignment maps an assertion row onto the keyword-facing Alignment.
func projectAlignment(a assertions.Assertion) Alignment {
	al := Alignment{
		ID:           a.ID,
		ConceptID:    a.SubjectRefID,
		ObjectTermID: a.ObjectRefID,
		Method:       methodOf(a.Qualifiers),
		Evidence:     a.DecisionReason,
	}
	if a.Confidence != nil {
		v := *a.Confidence
		al.Score = &v
	}
	return al
}

// methodOf reads the alignment method out of an assertion's qualifiers JSON,
// returning "" when absent or malformed.
func methodOf(qualifiers json.RawMessage) string {
	if len(qualifiers) == 0 {
		return ""
	}
	var q map[string]any
	if err := json.Unmarshal(qualifiers, &q); err != nil {
		return ""
	}
	m, _ := q["method"].(string)
	return m
}
