package keywords

import (
	"context"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/ontology/assertions"
	"github.com/chendingplano/deepdoc/server/api/ontology/semid"
	"github.com/chendingplano/deepdoc/server/api/ontology/terms"
)

// synthesisMethod/synthesisEvidence/synthesisScore describe how
// associate_semantics-triggered class synthesis characterizes its own
// keyword-alignment step, mirroring metric_lossless_writer.go's own
// "deterministic_definition_term" class-resolution-decision method string.
// Deterministic (no fuzzy matching involved -- design.md D1), so a fixed
// score of 1.0 rather than a computed confidence.
const (
	synthesisMethod   = "deterministic_class_synthesis"
	synthesisEvidence = "auto-promoted via associate_semantics (bug 2026082101 finding 5)"
	synthesisScore    = 1.0
)

// init registers keywords' class synthesis as the assertions package's
// ClassSynthesizer (metric-class-synthesis-seam). keywords already imports
// assertions to persist keyword-concept alignments as kb.semantic_assertions
// rows, so this is the valid registration direction -- assertions cannot
// import keywords back without an import cycle.
func init() {
	if err := assertions.RegisterClassSynthesizer(synthesizeClass); err != nil {
		panic(err)
	}
}

// synthesizeClass implements assertions.ClassSynthesizer. When input.ConceptID
// is set, it reuses EnsureAcceptedOrCreate's transaction-scoped core so the
// synthesized or selected term keeps the same keyword-concept alignment
// (core:aligns_to_term) auto-promotion gave it before this change. With no
// ConceptID, it creates the term directly with no alignment step -- the
// caller (resolveOrCreateMetricClass) only calls this once it has confirmed
// no term already exists for candidateTermID.
func synthesizeClass(ctx context.Context, db assertions.DBX, candidateTermID string, input assertions.ClassSynthesisInput) (string, bool, error) {
	synth := TermSynthesisInput{
		CanonicalName:        input.CanonicalName,
		Definition:           input.Definition,
		ValueType:            input.ValueType,
		RangeType:            input.RangeType,
		PermittedUnitTermIDs: input.PermittedUnitTermIDs,
		RawUnit:              input.RawUnit,
		ExtraProperties:      input.ExtraProperties,
	}

	if conceptID := strings.TrimSpace(input.ConceptID); conceptID != "" {
		store := AlignmentsStore{
			Assertions:  assertions.AssertionStore{DB: db},
			DecisionLog: semid.DecisionLogStore{DB: db},
			Scope:       "_",
		}
		aligned, created, err := store.ensureAcceptedOrCreate(ctx, db, conceptID, synth, synthesisMethod, synthesisScore, synthesisEvidence)
		if err != nil {
			return "", false, err
		}
		return aligned.ObjectTermID, created, nil
	}

	created, err := terms.TermStore{DB: db}.CreateTerm(ctx, terms.Term{
		TermID:     candidateTermID,
		TermKind:   "metric_definition",
		ModuleID:   "measurement",
		Status:     "auto-promoted",
		Definition: strings.TrimSpace(synth.Definition),
		Scope:      "document-derived, auto-promoted (ADR 2026081201)",
		Properties: synth.termProperties(),
	})
	if err != nil {
		return "", false, err
	}
	return created.TermID, true, nil
}
