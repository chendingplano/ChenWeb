package docprocessing

import (
	"context"
	"fmt"
	"sort"

	"github.com/chendingplano/deepdoc/server/api/ontology/semrules"
)

// ApplicabilityResolver orchestrates the two-pass semrules evaluation with
// optional tier-3 classification between passes (spec 2026080102 section 7).
// Both extraction routing and deterministic review-scope selection share this
// resolver; the consumer-specific decision-relevance and fact sources differ.
type ApplicabilityResolver struct {
	Classifier *DocumentClassifier
	Facets     FacetObservationStore
}

// ResolverRequest is the immutable input for one two-pass resolution.
type ResolverRequest struct {
	RecordID            int64
	DecisionAttemptID   string
	InvocationID        string
	BaseFacts           semrules.FactSet
	Predicates          []semrules.Document
	VocabularyReleaseID int64
	DocumentSample      string
}

// ResolverResult is the frozen two-pass output.
type ResolverResult struct {
	Facts            semrules.FactSet
	Traces           []semrules.Result
	Classified       bool
	ClassifyResult   *ClassifyResult
	DecisionRelevant []string // tier-3 paths that were decision-relevant
}

// Resolve performs the two-pass evaluation:
//  1. Evaluate predicates with base facts.
//  2. Identify decision-relevant missing tier-3 paths.
//  3. If any exist, invoke the classifier, persist observations, rebuild facts.
//  4. Re-evaluate with enriched facts.
//
// The classifier is invoked at most once per (record, decision_attempt_id,
// invocation_id). Review-time calls write only facet observations; they do
// not rerun extraction, semantic association, or projections.
func (r *ApplicabilityResolver) Resolve(ctx context.Context, req ResolverRequest) (ResolverResult, error) {
	if req.RecordID <= 0 {
		return ResolverResult{}, fmt.Errorf("applicability resolver: record_id is required")
	}
	if req.DecisionAttemptID == "" {
		return ResolverResult{}, fmt.Errorf("applicability resolver: decision_attempt_id is required")
	}
	if req.InvocationID == "" {
		return ResolverResult{}, fmt.Errorf("applicability resolver: invocation_id is required")
	}

	// --- Pass 1: evaluate with base facts ---
	pass1, err := evaluateAll(req.Predicates, req.BaseFacts)
	if err != nil {
		return ResolverResult{}, fmt.Errorf("applicability resolver: pass 1: %w", err)
	}

	// --- identify decision-relevant missing tier-3 paths ---
	missing := decisionRelevantTier3Paths(pass1)
	if len(missing) == 0 {
		return ResolverResult{
			Facts:  req.BaseFacts,
			Traces: pass1,
		}, nil
	}

	// --- invoke classifier (if available) ---
	if r.Classifier == nil {
		return ResolverResult{
			Facts:            req.BaseFacts,
			Traces:           pass1,
			DecisionRelevant: missing,
		}, nil
	}

	classifyReq := ClassifyRequest{
		RecordID:            req.RecordID,
		DecisionAttemptID:   req.DecisionAttemptID,
		InvocationID:        req.InvocationID,
		MissingPaths:        missing,
		DocumentSample:      req.DocumentSample,
		VocabularyReleaseID: req.VocabularyReleaseID,
	}
	classifyResult, err := r.Classifier.Classify(ctx, classifyReq)
	if err != nil || (len(classifyResult.Observations) == 0 && len(classifyResult.UnresolvedPaths) > 0) {
		// Classifier failure: preserve indeterminate, continue with base facts
		// (spec section 11: "Tier-3 classifier failure: preserve indeterminate;
		// do not overwrite prior known facts"). The Classify method returns
		// nil error with all paths unresolved on LLM failure, so both cases
		// are handled here.
		return ResolverResult{
			Facts:            req.BaseFacts,
			Traces:           pass1,
			DecisionRelevant: missing,
		}, nil
	}

	// --- rebuild fact set with classifier observations ---
	enrichedFacts := enrichFactSet(req.BaseFacts, classifyResult.Observations)

	// --- Pass 2: re-evaluate with enriched facts ---
	pass2, err := evaluateAll(req.Predicates, enrichedFacts)
	if err != nil {
		return ResolverResult{}, fmt.Errorf("applicability resolver: pass 2: %w", err)
	}

	return ResolverResult{
		Facts:            enrichedFacts,
		Traces:           pass2,
		Classified:       true,
		ClassifyResult:   &classifyResult,
		DecisionRelevant: missing,
	}, nil
}

// evaluateAll evaluates each predicate document against the fact set.
func evaluateAll(predicates []semrules.Document, facts semrules.FactSet) ([]semrules.Result, error) {
	results := make([]semrules.Result, len(predicates))
	for i, doc := range predicates {
		result, err := semrules.EvaluateDocumentValidated(doc, facts)
		if err != nil {
			return nil, fmt.Errorf("predicate %d: %w", i, err)
		}
		results[i] = result
	}
	return results, nil
}

// decisionRelevantTier3Paths collects the unique tier-3 producible paths
// from the DecisionRelevantMissingPaths of all evaluation results. Only
// paths that are registered as Tier3Producible in the fact registry are
// returned; masked paths (logically non-contributing) are already excluded
// by the evaluator's truth-table reduction.
func decisionRelevantTier3Paths(results []semrules.Result) []string {
	tier3 := tier3PathSet()
	seen := make(map[string]bool)
	var out []string
	for _, result := range results {
		for _, path := range result.DecisionRelevantMissingPaths {
			if !tier3[path] || seen[path] {
				continue
			}
			seen[path] = true
			out = append(out, path)
		}
	}
	sort.Strings(out)
	return out
}

// enrichFactSet merges classifier observations into the base fact set.
// Classifier values only fill missing paths; they never overwrite an
// existing known fact (deterministic > metadata > classifier reduction
// order is enforced by ReduceFacetObservations, not here).
func enrichFactSet(base semrules.FactSet, observations []ClassifyObservation) semrules.FactSet {
	enriched := make(semrules.FactSet, len(base)+len(observations))
	for path, fact := range base {
		enriched[path] = fact
	}
	for _, obs := range observations {
		if existing, ok := enriched[obs.Path]; ok && existing.State == semrules.FactKnown {
			continue // deterministic/metadata fact takes precedence
		}
		confidence := obs.Confidence
		enriched[obs.Path] = semrules.Fact{
			Path:       obs.Path,
			Value:      obs.Value,
			State:      semrules.FactKnown,
			Confidence: &confidence,
			Method:     FacetMethodClassifier,
		}
	}
	return enriched
}

// ResolveExtractionFacts is the extraction-routing convenience entry point.
// It builds tier-1/tier-2 facts from ProductionPlanFacts, resolves with the
// two-pass classifier, and returns the enriched fact set suitable for
// pipeline binding and processor gate evaluation.
func (r *ApplicabilityResolver) ResolveExtractionFacts(ctx context.Context, planFacts ProductionPlanFacts, recordID int64, runID int64, documentSample string) (semrules.FactSet, *ResolverResult, error) {
	baseFacts := BuildPipelineBindingFactSet(planFacts)
	req := ResolverRequest{
		RecordID:            recordID,
		DecisionAttemptID:   fmt.Sprintf("run-%d", runID),
		InvocationID:        fmt.Sprintf("extraction-%d-%d", recordID, runID),
		BaseFacts:           baseFacts,
		VocabularyReleaseID: 0, // pinned by the caller when governed vocabulary is available
		DocumentSample:      documentSample,
	}
	result, err := r.Resolve(ctx, req)
	if err != nil {
		return baseFacts, nil, err
	}
	return result.Facts, &result, nil
}
