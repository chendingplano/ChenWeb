package docprocessing

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"

	"github.com/chendingplano/deepdoc/server/api/ontology/semrules"
	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// VocabularyReleaseStore resolves the active document-authority module
// release id. Defined here (not imported from ontology/modules) to avoid an
// import cycle: modules -> profiles -> docprocessing.
type VocabularyReleaseStore interface {
	ActiveDocumentAuthorityReleaseID(ctx context.Context) (int64, error)
}

// VocabularyReleaseSQLStore is the production VocabularyReleaseStore. It
// reads the same kb.ontology_active_releases table
// ontology/profiles.ProfileStore.LoadReleasedProfiles already pins releases
// from, so classify_document's governed vocabulary tracks the same
// activation pointer review-profile selection does.
type VocabularyReleaseSQLStore struct{ DB *sql.DB }

func (s VocabularyReleaseSQLStore) ActiveDocumentAuthorityReleaseID(ctx context.Context) (int64, error) {
	if s.DB == nil {
		return 0, errors.New("db is nil")
	}
	var releaseID int64
	err := s.DB.QueryRowContext(ctx,
		`SELECT release_id FROM kb.ontology_active_releases WHERE module_id = 'document-authority' AND deactivated_at IS NULL`,
	).Scan(&releaseID)
	if errors.Is(err, sql.ErrNoRows) {
		// No active document-authority release yet: 0 means "unpinned",
		// matching the pre-fix hardcoded behavior for this case.
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return releaseID, nil
}

// ApplicabilityResolver orchestrates the two-pass semrules evaluation with
// optional tier-3 classification between passes (spec 2026080102 section 7).
// Both extraction routing and deterministic review-scope selection share this
// resolver; the consumer-specific decision-relevance and fact sources differ.
type ApplicabilityResolver struct {
	Classifier *DocumentClassifier
	Facets     FacetObservationStore
	// VocabularyReleases resolves the active document-authority module
	// release id, pinning classify_document's governed-vocabulary
	// provenance. May be nil, in which case VocabularyReleaseID stays 0 --
	// "no resolver exists" (P5 review 2026080302 finding P5-2), same as
	// before this field existed.
	VocabularyReleases VocabularyReleaseStore
}

// NewProductionApplicabilityResolver builds the production tier-3 resolver
// (real LLM classifier, SQL facet/audit stores, active release pinning) from
// environment configuration. It degrades to a nil resolver on configuration
// failure, logging a warning -- see buildProductionResolver. The review-scope
// handler calls this when CLASSIFY_DOCUMENT_ENABLED is set so deterministic
// review-profile selection can classify decision-relevant missing facets too.
func NewProductionApplicabilityResolver(db *sql.DB, logger ApiTypes.JimoLogger) *ApplicabilityResolver {
	return buildProductionResolver(db, logger)
}

// BoundedDocumentSampleForRecord sources the bounded classifier sample for a
// reviewed document from its already-parsed line file (never the raw upload),
// the same source the extraction-routing path uses. The line-file location is
// read from kb.inputs (result/staging filename + parser name), so the review
// path needs no event payload. A missing/unreadable line file degrades to an
// empty sample.
func BoundedDocumentSampleForRecord(ctx context.Context, db *sql.DB, recordID int64) string {
	rec, err := (DocMetadataSQLStore{DB: db}).GetInputRecord(ctx, recordID)
	if err != nil {
		return ""
	}
	path, err := ResolveInputFilePath(LineFileGeneratedEvent{RecordID: recordID}, rec.ResultFilename, rec.ParserName, rec.StagingFilename)
	if err != nil {
		return ""
	}
	return readBoundedDocumentSample(path)
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
	if err != nil || classifyResult.Failed {
		// Classifier failure: preserve indeterminate, continue with base facts
		// (spec section 11: "Tier-3 classifier failure: preserve indeterminate;
		// do not overwrite prior known facts"). The Classify method returns a
		// nil error with Failed=true on LLM/parse failure, so both cases are
		// handled here. A well-formed response that legitimately validated no
		// classifications (Failed=false, Observations empty) falls through to
		// pass 2 instead, since it is not a failure to preserve indeterminate
		// for (P5 review 2026080302 finding P5-2).
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

// activeRoutingPredicates collects every predicate document that could
// influence this record's routing decision: each active conditional
// binding's predicate (store_default bindings carry none) and every active
// processor gate's predicate. ResolveExtractionFacts uses this so pass 1 can
// determine whether ANY of them has an unresolved decision-relevant tier-3
// path before binding/gate resolution runs -- previously ResolveExtractionFacts
// never populated ResolverRequest.Predicates at all, so pass 1 always
// evaluated zero predicates and the classifier could never be reached (P5
// review 2026080302 finding P5-2).
func activeRoutingPredicates() []semrules.Document {
	bindings := currentProductionPipelineBindings()
	gates := currentProductionPipelineGates()
	docs := make([]semrules.Document, 0, len(bindings)+len(gates))
	for _, binding := range bindings {
		if binding.BindingKind != PipelineBindingKindConditional || binding.Predicate.Expression.Kind == "" {
			continue
		}
		docs = append(docs, binding.Predicate)
	}
	for _, gate := range gates {
		if gate.Predicate.Expression.Kind == "" {
			continue
		}
		docs = append(docs, gate.Predicate)
	}
	return docs
}

// ResolveExtractionFacts is the extraction-routing convenience entry point.
// It builds tier-1/tier-2 facts from ProductionPlanFacts, resolves with the
// two-pass classifier, and returns the enriched fact set suitable for
// pipeline binding and processor gate evaluation.
//
// attemptKey identifies this specific routing-decision attempt independent
// of any kb.doc_process_runs row -- routing must be decided before that row
// exists (before dispatch), so a run id cannot serve this purpose. It must
// be stable across a retry/redelivery of the same event (so the classifier's
// stable-retry-via-invocation-id correctly avoids a duplicate LLM call) and
// distinct across a genuinely new processing attempt (so a stale observation
// is never silently reused forever). Callers should pass the just-generated
// line file's own name, which is attempt-unique by construction (a fresh
// parse produces a fresh file) -- see resolverAttemptKey in control.go.
// Previously this was runID, hardcoded to 0 at the only call site because no
// run row exists yet, which collapsed every invocation for a record to the
// same invocation_id (P5 review 2026080302 finding P5-2).
func (r *ApplicabilityResolver) ResolveExtractionFacts(ctx context.Context, planFacts ProductionPlanFacts, recordID int64, attemptKey string, documentSample string) (semrules.FactSet, *ResolverResult, error) {
	baseFacts := BuildPipelineBindingFactSet(planFacts)
	var vocabularyReleaseID int64
	if r.VocabularyReleases != nil {
		if id, err := r.VocabularyReleases.ActiveDocumentAuthorityReleaseID(ctx); err == nil {
			vocabularyReleaseID = id
		}
	}
	req := ResolverRequest{
		RecordID:            recordID,
		DecisionAttemptID:   fmt.Sprintf("extract-%s", attemptKey),
		InvocationID:        fmt.Sprintf("extraction-%d-%s", recordID, attemptKey),
		BaseFacts:           baseFacts,
		Predicates:          activeRoutingPredicates(),
		VocabularyReleaseID: vocabularyReleaseID,
		DocumentSample:      documentSample,
	}
	result, err := r.Resolve(ctx, req)
	if err != nil {
		return baseFacts, nil, err
	}
	return result.Facts, &result, nil
}

// ResolveReviewFacts is the review-side deterministic-selection entry point
// (spec 2026080102 section 7: "optional classify_document for newly
// decision-relevant missing facets"; section 6 flow "initial semrules
// profile-applicability pass -> optional classify_document -> final semrules
// pass and immutable review-scope freeze"). It mirrors ResolveExtractionFacts
// but for profile applicability: base is the merged review-context + subject
// fact set and predicates are the released profile applicability documents
// under consideration, so decision relevance spans the whole profile set.
//
// scopeID is the stable review-scope identity: it becomes the decision/
// invocation identity, so retries of the same scope creation reuse the same
// invocation_id and the classifier's stable-retry (at most one LLM call per
// record/review-selection-attempt) deduplicates across them. Trivial
// predicates (empty expression, i.e. unconditional applicability) carry no
// tier-3 references and are dropped. Review-time calls write only facet
// observations -- the classifier never reruns extraction or Phase D.
func (r *ApplicabilityResolver) ResolveReviewFacts(ctx context.Context, recordID int64, scopeID string, base semrules.FactSet, predicates []semrules.Document, documentSample string) (semrules.FactSet, error) {
	// Trivial/unconditional applicability documents have no expression and so
	// no tier-3 references; keep only the ones that can be decision-relevant.
	relevant := make([]semrules.Document, 0, len(predicates))
	for _, doc := range predicates {
		if doc.Expression.Kind != "" {
			relevant = append(relevant, doc)
		}
	}
	var vocabularyReleaseID int64
	if r.VocabularyReleases != nil {
		if id, err := r.VocabularyReleases.ActiveDocumentAuthorityReleaseID(ctx); err == nil {
			vocabularyReleaseID = id
		}
	}
	req := ResolverRequest{
		RecordID:            recordID,
		DecisionAttemptID:   fmt.Sprintf("review-select-%s", scopeID),
		InvocationID:        fmt.Sprintf("review-select-%d-%s", recordID, scopeID),
		BaseFacts:           base,
		Predicates:          relevant,
		VocabularyReleaseID: vocabularyReleaseID,
		DocumentSample:      documentSample,
	}
	result, err := r.Resolve(ctx, req)
	if err != nil {
		return base, err
	}
	return result.Facts, nil
}
