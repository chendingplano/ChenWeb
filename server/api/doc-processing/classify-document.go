package docprocessing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/chendingplano/deepdoc/server/api/ontology/policyaudit"
	"github.com/chendingplano/deepdoc/server/api/ontology/semrules"
)

// ClassifyDocumentName is the canonical processor name for the mandatory-gated
// tier-3 classifier. It is registered in productionProcessorSpecs with class
// "mandatory_gated" so isMandatoryProcessor returns true and ordinary
// processor-gate rules cannot skip or require it (spec 2026080102 section 7).
const ClassifyDocumentName = "classify_document"

// DefaultClassifyDocumentMaxSample is the maximum number of characters from
// the document text sent to the classifier. The spec calls this a "bounded
// document sample" (section 7).
const DefaultClassifyDocumentMaxSample = 4000

// GovernedVocabulary is the pinned, release-scoped allowed-value set for
// tier-3 fact paths. The classifier validates every LLM-produced value
// against this vocabulary before persisting; an unknown value is rejected
// rather than stored (spec section 7: "returns values from the decision
// attempt's pinned document-authority vocabulary").
type GovernedVocabulary struct {
	ReleaseID int64
	Paths     map[string][]string // path -> sorted allowed values
}

// Allows reports whether value is in the allowed set for path.
func (v GovernedVocabulary) Allows(path, value string) bool {
	allowed, ok := v.Paths[path]
	if !ok {
		return false
	}
	norm := strings.ToLower(strings.TrimSpace(value))
	for _, a := range allowed {
		if strings.ToLower(a) == norm {
			return true
		}
	}
	return false
}

// Tier3Paths returns the governed paths that are tier-3 producible from the
// current semrules fact registry. The classifier only accepts requests for
// these paths.
func Tier3Paths() []string {
	snapshot := semrules.RegisteredFactPaths()
	var out []string
	for path, spec := range snapshot {
		if spec.Tier3Producible {
			out = append(out, path)
		}
	}
	sort.Strings(out)
	return out
}

// ClassifyRequest is the input for one classifier invocation.
type ClassifyRequest struct {
	RecordID          int64
	DecisionAttemptID string
	InvocationID      string
	// MissingPaths are the decision-relevant tier-3 paths the resolver
	// identified as needing classification. Only paths that are
	// Tier3Producible in the fact registry are accepted.
	MissingPaths []string
	// DocumentSample is the bounded text excerpt sent to the LLM.
	DocumentSample string
	// VocabularyReleaseID pins the governed vocabulary release.
	VocabularyReleaseID int64
}

// ClassifyObservation is one validated classifier output, ready for
// persistence as a FacetObservation.
type ClassifyObservation struct {
	Path       string
	Value      string
	Confidence float64
	Evidence   string
}

// ClassifyResult is the classifier's output.
type ClassifyResult struct {
	Observations []ClassifyObservation
	// UnresolvedPaths are the requested paths the classifier could not
	// resolve (omitted from LLM output, rejected by vocabulary, or below
	// minimum confidence).
	UnresolvedPaths []string
	// AlreadyExists is true when the invocation_id already had persisted
	// observations (stable retry).
	AlreadyExists bool
	// Failed is true only when the underlying LLM call itself errored or
	// its response could not be parsed -- distinct from a well-formed LLM
	// response that legitimately produced no valid classification for any
	// requested path (e.g. every candidate value was outside the governed
	// vocabulary). Both cases leave Observations empty and UnresolvedPaths
	// full, which previously made them indistinguishable to callers (P5
	// review 2026080302 finding P5-2).
	Failed bool
}

// DocumentClassifier is the injected-client tier-3 classifier (spec section
// 7). It is not a regular pipeline processor: the G2 resolver invokes it
// between the initial applicability pass and the final freeze.
type DocumentClassifier struct {
	Extractor     LLMJSONExtractor
	PromptText    string
	PromptRef     string
	PromptPath    string
	ModelName     string
	MaxSample     int
	Vocabulary    GovernedVocabulary
	Facets        FacetObservationStore
	Audit         policyaudit.Writer
	MinConfidence float64
}

// classifyDocumentLLMResponse is the JSON shape the LLM is instructed to
// return (see prompt-classify-document-v1.md).
type classifyDocumentLLMResponse struct {
	Classifications []classifyDocumentLLMClassification `json:"classifications"`
}

type classifyDocumentLLMClassification struct {
	Path       string  `json:"path"`
	Value      string  `json:"value"`
	Confidence float64 `json:"confidence"`
	Evidence   string  `json:"evidence"`
}

// Classify executes the tier-3 classification for the given request. It:
//  1. Validates that requested paths are tier-3 producible.
//  2. Checks for an existing invocation (stable retry).
//  3. Calls the LLM with the bounded sample and governed keys.
//  4. Validates output against the governed vocabulary.
//  5. Persists facet observations.
//  6. Emits content-safe policyaudit events.
func (c *DocumentClassifier) Classify(ctx context.Context, req ClassifyRequest) (ClassifyResult, error) {
	if c.Extractor == nil {
		return ClassifyResult{}, errors.New("classify_document: extractor is nil")
	}
	if req.RecordID <= 0 {
		return ClassifyResult{}, errors.New("classify_document: record_id is required")
	}
	if req.DecisionAttemptID == "" {
		return ClassifyResult{}, errors.New("classify_document: decision_attempt_id is required")
	}
	if req.InvocationID == "" {
		return ClassifyResult{}, errors.New("classify_document: invocation_id is required")
	}

	// --- validate requested paths are tier-3 producible ---
	tier3 := tier3PathSet()
	var validPaths []string
	for _, path := range req.MissingPaths {
		if !tier3[path] {
			return ClassifyResult{}, fmt.Errorf("classify_document: path %q is not tier-3 producible", path)
		}
		validPaths = append(validPaths, path)
	}
	if len(validPaths) == 0 {
		return ClassifyResult{}, nil
	}

	// --- stable retry: check for existing invocation ---
	if c.Facets != nil {
		existing, err := c.Facets.ListFacetObservations(ctx, req.RecordID, req.VocabularyReleaseID)
		if err == nil {
			for _, obs := range existing {
				if obs.InvocationID == req.InvocationID && obs.Method == FacetMethodClassifier {
					return c.resultFromExisting(existing, validPaths), nil
				}
			}
		}
	}

	// --- emit invocation audit event ---
	c.emitAudit(ctx, policyaudit.Event{
		Kind:        "classifier_invoked",
		RecordID:    req.RecordID,
		SubjectKind: "classify_document",
		Detail: map[string]any{
			"decision_attempt_id": req.DecisionAttemptID,
			"invocation_id":       req.InvocationID,
			"requested_paths":     validPaths,
			"vocabulary_release":  req.VocabularyReleaseID,
			"model":               c.ModelName,
		},
	})

	// --- build LLM input ---
	sample := c.truncateSample(req.DocumentSample)
	keys := make([]map[string]any, 0, len(validPaths))
	for _, path := range validPaths {
		allowed := c.Vocabulary.Paths[path]
		if len(allowed) == 0 {
			continue
		}
		keys = append(keys, map[string]any{
			"path":    path,
			"allowed": allowed,
		})
	}
	if len(keys) == 0 {
		return ClassifyResult{UnresolvedPaths: validPaths}, nil
	}

	llmInput, err := json.Marshal(map[string]any{
		"sample": sample,
		"keys":   keys,
	})
	if err != nil {
		return ClassifyResult{}, fmt.Errorf("classify_document: marshal input: %w", err)
	}

	// --- call LLM ---
	raw, err := c.Extractor.ExtractJSON(ctx, newLLMJSONInput(
		ctx, ClassifyDocumentName, c.PromptText, c.ModelName,
		string(llmInput), "P5-CLASSIFY-DOCUMENT", "classify-document",
	))
	if err != nil {
		// Tier-3 classifier failure: preserve indeterminate; do not
		// overwrite prior known facts (spec section 11).
		c.emitAudit(ctx, policyaudit.Event{
			Kind:        "classifier_failed",
			RecordID:    req.RecordID,
			SubjectKind: "classify_document",
			Detail: map[string]any{
				"decision_attempt_id": req.DecisionAttemptID,
				"invocation_id":       req.InvocationID,
				"error":               err.Error(),
			},
		})
		return ClassifyResult{UnresolvedPaths: validPaths, Failed: true}, nil
	}

	// --- parse response ---
	response, err := parseClassifyResponse(raw)
	if err != nil {
		c.emitAudit(ctx, policyaudit.Event{
			Kind:        "classifier_failed",
			RecordID:    req.RecordID,
			SubjectKind: "classify_document",
			Detail: map[string]any{
				"decision_attempt_id": req.DecisionAttemptID,
				"invocation_id":       req.InvocationID,
				"error":               err.Error(),
			},
		})
		return ClassifyResult{UnresolvedPaths: validPaths, Failed: true}, nil
	}

	// --- validate against governed vocabulary and persist ---
	resolved := make(map[string]bool)
	var observations []ClassifyObservation
	for _, cls := range response.Classifications {
		path := strings.TrimSpace(cls.Path)
		value := strings.TrimSpace(cls.Value)
		if path == "" || value == "" {
			continue
		}
		if !containsString(validPaths, path) {
			continue // LLM returned an unrequested path
		}
		if !c.Vocabulary.Allows(path, value) {
			continue // unknown value rejected
		}
		if c.MinConfidence > 0 && cls.Confidence < c.MinConfidence {
			continue
		}
		observations = append(observations, ClassifyObservation{
			Path:       path,
			Value:      value,
			Confidence: cls.Confidence,
			Evidence:   cls.Evidence,
		})
		resolved[path] = true
	}

	// --- persist facet observations ---
	sourceFP := classifySourceFingerprint(c.PromptRef, c.ModelName, sample)
	for _, obs := range observations {
		confidence := obs.Confidence
		if c.Facets != nil {
			_, _ = c.Facets.InsertFacetObservation(ctx, FacetObservation{
				RecordID:            req.RecordID,
				Path:                obs.Path,
				Value:               obs.Value,
				State:               semrules.FactKnown,
				Method:              FacetMethodClassifier,
				Confidence:          &confidence,
				Evidence:            map[string]any{"span": obs.Evidence},
				SourceFingerprint:   sourceFP,
				DecisionAttemptID:   req.DecisionAttemptID,
				InvocationID:        req.InvocationID,
				VocabularyReleaseID: req.VocabularyReleaseID,
			})
		}
	}

	// --- emit result audit event (content-safe: no document text) ---
	resultPaths := make([]string, 0, len(observations))
	for _, obs := range observations {
		resultPaths = append(resultPaths, obs.Path)
	}
	sort.Strings(resultPaths)
	var unresolved []string
	for _, path := range validPaths {
		if !resolved[path] {
			unresolved = append(unresolved, path)
		}
	}
	c.emitAudit(ctx, policyaudit.Event{
		Kind:        "classifier_completed",
		RecordID:    req.RecordID,
		SubjectKind: "classify_document",
		Detail: map[string]any{
			"decision_attempt_id": req.DecisionAttemptID,
			"invocation_id":       req.InvocationID,
			"resolved_paths":      resultPaths,
			"unresolved_paths":    unresolved,
			"vocabulary_release":  req.VocabularyReleaseID,
		},
	})

	return ClassifyResult{
		Observations:    observations,
		UnresolvedPaths: unresolved,
	}, nil
}

func (c *DocumentClassifier) truncateSample(text string) string {
	maxSample := c.MaxSample
	if maxSample <= 0 {
		maxSample = DefaultClassifyDocumentMaxSample
	}
	if len(text) <= maxSample {
		return text
	}
	// Cut on a rune boundary: a plain byte-index slice can split a
	// multi-byte UTF-8 rune, corrupting the sample sent to the LLM -- the
	// pilot corpus is predominantly Chinese (P5 review 2026080302 finding
	// P5-25). Walk back to the start of the rune straddling the cut point.
	cut := maxSample
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut]
}

func (c *DocumentClassifier) emitAudit(ctx context.Context, event policyaudit.Event) {
	if c.Audit == nil {
		return
	}
	_ = c.Audit.WriteEvent(ctx, event)
}

func (c *DocumentClassifier) resultFromExisting(existing []FacetObservation, requestedPaths []string) ClassifyResult {
	resolved := make(map[string]bool)
	var observations []ClassifyObservation
	for _, obs := range existing {
		if obs.Method != FacetMethodClassifier {
			continue
		}
		if !containsString(requestedPaths, obs.Path) {
			continue
		}
		if obs.State != semrules.FactKnown {
			continue
		}
		value, _ := obs.Value.(string)
		if value == "" {
			continue
		}
		confidence := 0.0
		if obs.Confidence != nil {
			confidence = *obs.Confidence
		}
		evidence := ""
		if obs.Evidence != nil {
			if span, ok := obs.Evidence["span"].(string); ok {
				evidence = span
			}
		}
		observations = append(observations, ClassifyObservation{
			Path:       obs.Path,
			Value:      value,
			Confidence: confidence,
			Evidence:   evidence,
		})
		resolved[obs.Path] = true
	}
	var unresolved []string
	for _, path := range requestedPaths {
		if !resolved[path] {
			unresolved = append(unresolved, path)
		}
	}
	return ClassifyResult{
		Observations:    observations,
		UnresolvedPaths: unresolved,
		AlreadyExists:   true,
	}
}

func parseClassifyResponse(raw map[string]any) (classifyDocumentLLMResponse, error) {
	var response classifyDocumentLLMResponse
	// The LLM extractor returns map[string]any; re-marshal and unmarshal
	// into the typed response.
	bs, err := json.Marshal(raw)
	if err != nil {
		return response, fmt.Errorf("classify_document: marshal response: %w", err)
	}
	if err := json.Unmarshal(bs, &response); err != nil {
		return response, fmt.Errorf("classify_document: unmarshal response: %w", err)
	}
	return response, nil
}

func classifySourceFingerprint(promptRef, modelName, sample string) string {
	h := sha256.New()
	h.Write([]byte(promptRef))
	h.Write([]byte{0})
	h.Write([]byte(modelName))
	h.Write([]byte{0})
	h.Write([]byte(sample))
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

func tier3PathSet() map[string]bool {
	out := make(map[string]bool)
	for _, path := range Tier3Paths() {
		out[path] = true
	}
	return out
}

// LoadClassifyDocumentPrompt loads the versioned classify_document prompt
// from the standard prompt search paths. The prompt is never embedded in
// Go (spec section 7: "The prompt is a versioned file under ChenWeb/prompts;
// it is never embedded in Go").
func LoadClassifyDocumentPrompt() (promptText, promptRef, promptPath string, err error) {
	return loadPromptByRef("prompt-classify-document-v1.md")
}

// NewDocumentClassifier builds a classifier with the versioned prompt and
// the supplied dependencies. modelName defaults to the
// CLASSIFY_DOCUMENT_MODEL_NAME environment variable when empty.
func NewDocumentClassifier(extractor LLMJSONExtractor, facets FacetObservationStore, audit policyaudit.Writer, vocabulary GovernedVocabulary) (*DocumentClassifier, error) {
	promptText, promptRef, promptPath, err := LoadClassifyDocumentPrompt()
	if err != nil {
		return nil, fmt.Errorf("classify_document: load prompt: %w", err)
	}
	modelName := envString("CLASSIFY_DOCUMENT_MODEL_NAME", "deepseek-chat")
	return &DocumentClassifier{
		Extractor:  extractor,
		PromptText: promptText,
		PromptRef:  promptRef,
		PromptPath: promptPath,
		ModelName:  modelName,
		Vocabulary: vocabulary,
		Facets:     facets,
		Audit:      audit,
	}, nil
}
