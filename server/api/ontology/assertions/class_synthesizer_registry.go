package assertions

import (
	"context"
	"errors"
	"sync"
)

// ClassSynthesisInput is the structural, already-extracted data a caller
// supplies to create or select a governed class term for an occurrence
// (bug 2026082101 finding 5). No field is authored by an LLM here.
type ClassSynthesisInput struct {
	// ConceptID is the keyword_concept_id linking this occurrence to a
	// governed keyword concept, if one was resolved. May be empty.
	ConceptID            string
	CanonicalName        string
	Definition           string
	ValueType            string
	RangeType            string
	PermittedUnitTermIDs []string
	RawUnit              string
	// ExtraProperties holds additional class-level artifact fields the
	// operator has exposed via [ontology_term_property_map]. Instance-level
	// fields do not belong here (bug 2026082101 finding 6) -- callers are
	// responsible for only passing class-level facts.
	ExtraProperties map[string]any
}

// ClassSynthesizer creates or selects a governed class term for candidateTermID,
// running against the caller-supplied db so it participates in the caller's
// own transaction (metric-class-synthesis-seam: "Class synthesis runs inside
// the caller's transaction"). created is true when a new term was minted,
// false when an existing one was selected.
type ClassSynthesizer func(ctx context.Context, db DBX, candidateTermID string, input ClassSynthesisInput) (termID string, created bool, err error)

var (
	classSynthesizerMu sync.RWMutex
	defaultClassSynth  ClassSynthesizer
)

// RegisterClassSynthesizer sets the default registry's synthesizer.
// Registering a second one is a programming error, mirroring
// RegisterNormalizer's existing-family check.
func RegisterClassSynthesizer(fn ClassSynthesizer) error {
	classSynthesizerMu.Lock()
	defer classSynthesizerMu.Unlock()
	if fn == nil {
		return errors.New("class synthesizer must not be nil")
	}
	if defaultClassSynth != nil {
		return errors.New("class synthesizer already registered")
	}
	defaultClassSynth = fn
	return nil
}

// SynthesizeClass invokes the registered ClassSynthesizer. It returns an
// error rather than panicking when nothing is registered, since a missing
// registration is a wiring bug the caller should surface, not crash on.
func SynthesizeClass(ctx context.Context, db DBX, candidateTermID string, input ClassSynthesisInput) (termID string, created bool, err error) {
	classSynthesizerMu.RLock()
	fn := defaultClassSynth
	classSynthesizerMu.RUnlock()
	if fn == nil {
		return "", false, errors.New("no class synthesizer registered")
	}
	return fn(ctx, db, candidateTermID, input)
}
