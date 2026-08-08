package docprocessing

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/chendingplano/deepdoc/server/api/ontology/policyaudit"
	"github.com/chendingplano/deepdoc/server/api/ontology/semrules"
	llmclients "github.com/chendingplano/shared/go/api/llm"
)

// stubExtractor is a test double for LLMJSONExtractor.
type stubExtractor struct {
	result map[string]any
	err    error
	input  llmclients.JSONExtractionInput
}

func (s *stubExtractor) ExtractJSON(_ context.Context, in llmclients.JSONExtractionInput) (map[string]any, error) {
	s.input = in
	return s.result, s.err
}

// stubFacetStore is a test double for FacetObservationStore.
type stubFacetStore struct {
	observations []FacetObservation
	inserted     []FacetObservation
	insertErr    error
}

func (s *stubFacetStore) InsertFacetObservation(_ context.Context, obs FacetObservation) (FacetObservation, error) {
	if s.insertErr != nil {
		return FacetObservation{}, s.insertErr
	}
	s.inserted = append(s.inserted, obs)
	obs.ID = int64(len(s.inserted))
	s.observations = append(s.observations, obs)
	return obs, nil
}

func (s *stubFacetStore) ListFacetObservations(_ context.Context, recordID, vocabularyReleaseID int64) ([]FacetObservation, error) {
	var out []FacetObservation
	for _, obs := range s.observations {
		if obs.RecordID == recordID && obs.VocabularyReleaseID == vocabularyReleaseID {
			out = append(out, obs)
		}
	}
	return out, nil
}

func (s *stubFacetStore) ListFacetObservationsAnyRelease(_ context.Context, recordID int64) ([]FacetObservation, error) {
	var out []FacetObservation
	for _, obs := range s.observations {
		if obs.RecordID == recordID {
			out = append(out, obs)
		}
	}
	return out, nil
}

// stubAudit captures policyaudit events.
type stubAudit struct {
	events []policyaudit.Event
}

func (s *stubAudit) WriteEvent(_ context.Context, event policyaudit.Event) error {
	s.events = append(s.events, event)
	return nil
}

func testVocabulary() GovernedVocabulary {
	return GovernedVocabulary{
		ReleaseID: 1,
		Paths: map[string][]string{
			"document.doc_kind":         {"product_specification", "regulated_reference", "narrative_research", "test_report"},
			"document.domain":           {"medical_devices", "pharmaceuticals", "industrial_equipment"},
			"document.normative_status": {"mandatory", "recommended", "informative"},
			"document.jurisdiction":     {"CN", "US", "EU", "ISO"},
		},
	}
}

func testClassifyRequest() ClassifyRequest {
	return ClassifyRequest{
		RecordID:            42,
		DecisionAttemptID:   "attempt-1",
		InvocationID:        "inv-1",
		MissingPaths:        []string{"document.doc_kind", "document.domain"},
		DocumentSample:      "This document specifies display module requirements for medical ventilators.",
		VocabularyReleaseID: 1,
	}
}

func TestClassifyDocumentRequestedGovernedKeysOnly(t *testing.T) {
	// The classifier must reject paths that are not tier-3 producible.
	ext := &stubExtractor{}
	c := &DocumentClassifier{
		Extractor:  ext,
		PromptText: "test prompt",
		ModelName:  "test-model",
		Vocabulary: testVocabulary(),
	}
	req := testClassifyRequest()
	req.MissingPaths = []string{"document.input_doc_type"} // not tier-3
	_, err := c.Classify(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for non-tier-3 path, got nil")
	}
	if !strings.Contains(err.Error(), "not tier-3 producible") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClassifyDocumentBoundedSample(t *testing.T) {
	// The classifier must truncate the document sample to MaxSample.
	ext := &stubExtractor{
		result: map[string]any{
			"classifications": []any{
				map[string]any{
					"path": "document.doc_kind", "value": "product_specification",
					"confidence": 0.9, "evidence": "display module",
				},
			},
		},
	}
	store := &stubFacetStore{}
	c := &DocumentClassifier{
		Extractor:  ext,
		PromptText: "test prompt",
		ModelName:  "test-model",
		MaxSample:  20,
		Vocabulary: testVocabulary(),
		Facets:     store,
	}
	req := testClassifyRequest()
	req.DocumentSample = "This is a very long document sample that should be truncated by the classifier before sending to the LLM."
	_, err := c.Classify(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Verify the LLM received truncated input.
	if !strings.Contains(ext.input.InputText, `"sample":"This is a very long `) {
		t.Fatalf("expected truncated sample in LLM input, got: %s", ext.input.InputText)
	}
	if strings.Contains(ext.input.InputText, "truncated by the classifier") {
		t.Fatal("sample was not truncated")
	}
}

// TestTruncateSampleCutsOnRuneBoundary proves the sample is truncated
// without splitting a multi-byte UTF-8 rune, which would corrupt the text
// sent to the LLM -- the pilot corpus is predominantly Chinese (P5 review
// 2026080302 finding P5-25).
func TestTruncateSampleCutsOnRuneBoundary(t *testing.T) {
	c := &DocumentClassifier{MaxSample: 10}
	// Each Chinese character is 3 bytes in UTF-8; 10 is not a multiple of 3,
	// so a byte-boundary cut at exactly 10 splits the 4th character.
	text := "呼吸机显示模块规格标准文档"
	got := c.truncateSample(text)
	if !utf8.ValidString(got) {
		t.Fatalf("truncateSample produced invalid UTF-8: %q (bytes %x)", got, []byte(got))
	}
	if len(got) > 10 {
		t.Fatalf("truncated length = %d, want <= 10", len(got))
	}
	if len(got) == 0 {
		t.Fatal("expected a non-empty truncated sample")
	}
}

func TestClassifyDocumentUnknownValueRejection(t *testing.T) {
	// The classifier must reject LLM values not in the governed vocabulary.
	ext := &stubExtractor{
		result: map[string]any{
			"classifications": []any{
				map[string]any{
					"path": "document.doc_kind", "value": "unknown_kind",
					"confidence": 0.95, "evidence": "some text",
				},
				map[string]any{
					"path": "document.domain", "value": "medical_devices",
					"confidence": 0.8, "evidence": "ventilators",
				},
			},
		},
	}
	store := &stubFacetStore{}
	c := &DocumentClassifier{
		Extractor:  ext,
		PromptText: "test prompt",
		ModelName:  "test-model",
		Vocabulary: testVocabulary(),
		Facets:     store,
	}
	result, err := c.Classify(context.Background(), testClassifyRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// "unknown_kind" must be rejected; only "medical_devices" survives.
	if len(result.Observations) != 1 {
		t.Fatalf("expected 1 observation, got %d", len(result.Observations))
	}
	if result.Observations[0].Value != "medical_devices" {
		t.Fatalf("expected medical_devices, got %s", result.Observations[0].Value)
	}
	if len(result.UnresolvedPaths) != 1 || result.UnresolvedPaths[0] != "document.doc_kind" {
		t.Fatalf("expected document.doc_kind unresolved, got %v", result.UnresolvedPaths)
	}
}

func TestClassifyDocumentConfidenceAndEvidence(t *testing.T) {
	// The classifier must persist confidence and evidence on observations.
	confidence := 0.87
	ext := &stubExtractor{
		result: map[string]any{
			"classifications": []any{
				map[string]any{
					"path": "document.doc_kind", "value": "product_specification",
					"confidence": confidence, "evidence": "Section 3 display specs",
				},
			},
		},
	}
	store := &stubFacetStore{}
	c := &DocumentClassifier{
		Extractor:  ext,
		PromptText: "test prompt",
		ModelName:  "test-model",
		Vocabulary: testVocabulary(),
		Facets:     store,
	}
	result, err := c.Classify(context.Background(), testClassifyRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Observations) != 1 {
		t.Fatalf("expected 1 observation, got %d", len(result.Observations))
	}
	if result.Observations[0].Confidence != confidence {
		t.Fatalf("confidence = %f, want %f", result.Observations[0].Confidence, confidence)
	}
	if result.Observations[0].Evidence != "Section 3 display specs" {
		t.Fatalf("evidence = %q, want %q", result.Observations[0].Evidence, "Section 3 display specs")
	}
	// Verify persisted observation.
	if len(store.inserted) != 1 {
		t.Fatalf("expected 1 persisted observation, got %d", len(store.inserted))
	}
	if store.inserted[0].Method != FacetMethodClassifier {
		t.Fatalf("method = %q, want %q", store.inserted[0].Method, FacetMethodClassifier)
	}
	if store.inserted[0].Confidence == nil || *store.inserted[0].Confidence != confidence {
		t.Fatal("persisted confidence mismatch")
	}
}

func TestClassifyDocumentStableRetry(t *testing.T) {
	// A second call with the same invocation_id must return existing
	// observations without calling the LLM again.
	ext := &stubExtractor{
		result: map[string]any{
			"classifications": []any{
				map[string]any{
					"path": "document.doc_kind", "value": "test_report",
					"confidence": 0.9, "evidence": "test data",
				},
			},
		},
	}
	store := &stubFacetStore{}
	c := &DocumentClassifier{
		Extractor:  ext,
		PromptText: "test prompt",
		ModelName:  "test-model",
		Vocabulary: testVocabulary(),
		Facets:     store,
	}
	req := testClassifyRequest()
	// First call.
	result1, err := c.Classify(context.Background(), req)
	if err != nil {
		t.Fatalf("first call error: %v", err)
	}
	if result1.AlreadyExists {
		t.Fatal("first call should not report AlreadyExists")
	}
	if len(store.inserted) != 1 {
		t.Fatalf("expected 1 insert, got %d", len(store.inserted))
	}

	// Second call with same invocation_id.
	result2, err := c.Classify(context.Background(), req)
	if err != nil {
		t.Fatalf("second call error: %v", err)
	}
	if !result2.AlreadyExists {
		t.Fatal("second call should report AlreadyExists")
	}
	if len(result2.Observations) != 1 || result2.Observations[0].Value != "test_report" {
		t.Fatalf("retry result mismatch: %+v", result2)
	}
	// No additional inserts.
	if len(store.inserted) != 1 {
		t.Fatalf("expected still 1 insert after retry, got %d", len(store.inserted))
	}
}

func TestClassifyDocumentContentSafeAuditEvent(t *testing.T) {
	// Audit events must not contain document text.
	ext := &stubExtractor{
		result: map[string]any{
			"classifications": []any{
				map[string]any{
					"path": "document.doc_kind", "value": "product_specification",
					"confidence": 0.9, "evidence": "display specs",
				},
			},
		},
	}
	audit := &stubAudit{}
	c := &DocumentClassifier{
		Extractor:  ext,
		PromptText: "test prompt",
		ModelName:  "test-model",
		Vocabulary: testVocabulary(),
		Facets:     &stubFacetStore{},
		Audit:      audit,
	}
	_, err := c.Classify(context.Background(), testClassifyRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(audit.events) < 2 {
		t.Fatalf("expected at least 2 audit events (invocation + completion), got %d", len(audit.events))
	}
	for _, event := range audit.events {
		raw, _ := json.Marshal(event.Detail)
		text := string(raw)
		if strings.Contains(text, "medical ventilators") {
			t.Fatalf("audit event contains document content: %s", text)
		}
	}
	// Verify invocation event.
	if audit.events[0].Kind != "classifier_invoked" {
		t.Fatalf("first event kind = %q, want classifier_invoked", audit.events[0].Kind)
	}
	// Verify completion event.
	last := audit.events[len(audit.events)-1]
	if last.Kind != "classifier_completed" {
		t.Fatalf("last event kind = %q, want classifier_completed", last.Kind)
	}
}

func TestClassifyDocumentIsRoutedNotMandatory(t *testing.T) {
	// classify_document must be Class "routed" (not mandatory), so an
	// authored kb.pipeline_rules row targeting it by name can
	// actually skip it -- the same lever every other routed processor gets.
	// isMandatoryProcessor's require-immunity must not apply to it.
	found := false
	for _, spec := range productionProcessorSpecs {
		if spec.Name == ClassifyDocumentName {
			found = true
			if spec.Class != "routed" {
				t.Fatalf("classify_document class = %q, want routed", spec.Class)
			}
			if isMandatoryProcessor(spec) {
				t.Fatal("isMandatoryProcessor returned true for classify_document, want false")
			}
			break
		}
	}
	if !found {
		t.Fatal("classify_document not found in productionProcessorSpecs")
	}
}

func TestClassifyDocumentLLMFailurePreservesIndeterminate(t *testing.T) {
	// When the LLM fails, the classifier must return all paths as
	// unresolved (preserving indeterminate) rather than erroring.
	ext := &stubExtractor{err: errors.New("LLM timeout")}
	c := &DocumentClassifier{
		Extractor:  ext,
		PromptText: "test prompt",
		ModelName:  "test-model",
		Vocabulary: testVocabulary(),
		Facets:     &stubFacetStore{},
		Audit:      &stubAudit{},
	}
	result, err := c.Classify(context.Background(), testClassifyRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Observations) != 0 {
		t.Fatalf("expected 0 observations on LLM failure, got %d", len(result.Observations))
	}
	if len(result.UnresolvedPaths) != 2 {
		t.Fatalf("expected 2 unresolved paths, got %d", len(result.UnresolvedPaths))
	}
	// P5 review 2026080302 finding: "LLM failed" and "LLM legitimately
	// returned no classification" both surfaced as empty observations with
	// no way to tell them apart. A genuine LLM call error must set Failed.
	if !result.Failed {
		t.Fatal("expected Failed=true for a genuine LLM call error")
	}
}

// TestClassifyDocumentWellFormedEmptyResponseIsNotFailed proves a
// successful LLM call that produces no valid classification for any
// requested path (every candidate value rejected by the governed
// vocabulary) is distinguishable from a genuine classifier failure: the
// observable shape (empty Observations, full UnresolvedPaths) is the same,
// but Failed must stay false.
func TestClassifyDocumentWellFormedEmptyResponseIsNotFailed(t *testing.T) {
	ext := &stubExtractor{
		result: map[string]any{
			"classifications": []any{
				map[string]any{"path": "document.doc_kind", "value": "not_a_governed_value", "confidence": 0.9, "evidence": "x"},
			},
		},
	}
	c := &DocumentClassifier{
		Extractor:  ext,
		PromptText: "test prompt",
		ModelName:  "test-model",
		Vocabulary: testVocabulary(),
		Facets:     &stubFacetStore{},
		Audit:      &stubAudit{},
	}
	result, err := c.Classify(context.Background(), testClassifyRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Observations) != 0 {
		t.Fatalf("expected 0 observations (value rejected by vocabulary), got %d", len(result.Observations))
	}
	if result.Failed {
		t.Fatal("a well-formed LLM response with no valid classification must not be treated as a classifier failure")
	}
}

func TestClassifyDocumentEmptyMissingPaths(t *testing.T) {
	// No missing paths means no LLM call.
	ext := &stubExtractor{}
	c := &DocumentClassifier{
		Extractor:  ext,
		PromptText: "test prompt",
		ModelName:  "test-model",
		Vocabulary: testVocabulary(),
	}
	req := testClassifyRequest()
	req.MissingPaths = nil
	result, err := c.Classify(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Observations) != 0 {
		t.Fatal("expected no observations for empty missing paths")
	}
	if ext.input.InputText != "" {
		t.Fatal("LLM should not have been called")
	}
}

func TestClassifyDocumentMinConfidence(t *testing.T) {
	// Observations below MinConfidence are treated as unresolved.
	ext := &stubExtractor{
		result: map[string]any{
			"classifications": []any{
				map[string]any{
					"path": "document.doc_kind", "value": "product_specification",
					"confidence": 0.3, "evidence": "weak signal",
				},
			},
		},
	}
	c := &DocumentClassifier{
		Extractor:     ext,
		PromptText:    "test prompt",
		ModelName:     "test-model",
		Vocabulary:    testVocabulary(),
		Facets:        &stubFacetStore{},
		MinConfidence: 0.5,
	}
	result, err := c.Classify(context.Background(), testClassifyRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Observations) != 0 {
		t.Fatalf("expected 0 observations below min confidence, got %d", len(result.Observations))
	}
	if len(result.UnresolvedPaths) != 2 {
		t.Fatalf("expected 2 unresolved paths, got %v", result.UnresolvedPaths)
	}
}

func TestGovernedVocabularyAllows(t *testing.T) {
	vocab := testVocabulary()
	if !vocab.Allows("document.doc_kind", "product_specification") {
		t.Fatal("expected product_specification to be allowed")
	}
	if !vocab.Allows("document.doc_kind", "Product_Specification") {
		t.Fatal("expected case-insensitive match")
	}
	if vocab.Allows("document.doc_kind", "unknown_kind") {
		t.Fatal("expected unknown_kind to be rejected")
	}
	if vocab.Allows("unknown.path", "any_value") {
		t.Fatal("expected unknown path to be rejected")
	}
}

func TestTier3Paths(t *testing.T) {
	paths := Tier3Paths()
	if len(paths) == 0 {
		t.Fatal("expected at least one tier-3 path")
	}
	// Verify known tier-3 paths are present.
	expected := map[string]bool{
		"document.doc_kind":         false,
		"document.domain":           false,
		"document.normative_status": false,
		"document.jurisdiction":     false,
	}
	for _, path := range paths {
		if _, ok := expected[path]; ok {
			expected[path] = true
		}
	}
	for path, found := range expected {
		if !found {
			t.Fatalf("expected tier-3 path %q not found", path)
		}
	}
}

func TestClassifyDocumentPersistedObservationShape(t *testing.T) {
	// Verify the persisted FacetObservation has the correct shape per spec
	// section 7: method=classifier, state=known, vocabulary_release_id pinned.
	ext := &stubExtractor{
		result: map[string]any{
			"classifications": []any{
				map[string]any{
					"path": "document.domain", "value": "medical_devices",
					"confidence": 0.92, "evidence": "ventilator display",
				},
			},
		},
	}
	store := &stubFacetStore{}
	c := &DocumentClassifier{
		Extractor:  ext,
		PromptText: "test prompt",
		PromptRef:  "prompt-classify-document-v1.md",
		ModelName:  "test-model",
		Vocabulary: testVocabulary(),
		Facets:     store,
	}
	req := testClassifyRequest()
	req.MissingPaths = []string{"document.domain"}
	_, err := c.Classify(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.inserted) != 1 {
		t.Fatalf("expected 1 insert, got %d", len(store.inserted))
	}
	obs := store.inserted[0]
	if obs.Path != "document.domain" {
		t.Fatalf("path = %q, want document.domain", obs.Path)
	}
	if obs.Value != "medical_devices" {
		t.Fatalf("value = %v, want medical_devices", obs.Value)
	}
	if obs.State != semrules.FactKnown {
		t.Fatalf("state = %q, want known", obs.State)
	}
	if obs.Method != FacetMethodClassifier {
		t.Fatalf("method = %q, want classifier", obs.Method)
	}
	if obs.DecisionAttemptID != "attempt-1" {
		t.Fatalf("decision_attempt_id = %q, want attempt-1", obs.DecisionAttemptID)
	}
	if obs.InvocationID != "inv-1" {
		t.Fatalf("invocation_id = %q, want inv-1", obs.InvocationID)
	}
	if obs.VocabularyReleaseID != 1 {
		t.Fatalf("vocabulary_release_id = %d, want 1", obs.VocabularyReleaseID)
	}
	if obs.SourceFingerprint == "" {
		t.Fatal("source_fingerprint is empty")
	}
	if !strings.HasPrefix(obs.SourceFingerprint, "sha256:") {
		t.Fatalf("source_fingerprint = %q, want sha256: prefix", obs.SourceFingerprint)
	}
}

// TestNewProductionDocumentClassifierBuildsFromModelDefFile proves the
// production constructor resolves its LLM client credentials the same way
// every other real extractor in this package does (loadModelConfigFromEnvKeys
// against MODEL_DEF_FILE), rather than the placeholder bare-model-name
// default the earlier draft used with no corresponding APIKey/BaseURL.
func TestNewProductionDocumentClassifierBuildsFromModelDefFile(t *testing.T) {
	tmp := t.TempDir()
	modelsPath := filepath.Join(tmp, ".models.toml")
	modelsBody := `
[classify-document-model]
host = "cloud"
model_name = "deepseek-chat"
api_key = "sk-test-classify"
base_url = "https://api.deepseek.com"
timeout_sec = 45
`
	if err := os.WriteFile(modelsPath, []byte(modelsBody), 0o644); err != nil {
		t.Fatalf("write models file: %v", err)
	}
	promptPath := filepath.Join(tmp, "prompt-classify-document-v1.md")
	if err := os.WriteFile(promptPath, []byte("classify the document's governed attributes"), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}
	t.Setenv("PROMPT_DIR", tmp)
	t.Setenv("MODEL_DEF_FILE", modelsPath)
	t.Setenv("CLASSIFY_DOCUMENT_MODEL_NAME", "classify-document-model")

	classifier, err := newProductionDocumentClassifier(nil, nil)
	if err != nil {
		t.Fatalf("newProductionDocumentClassifier: %v", err)
	}
	if classifier.ModelName != "deepseek-chat" {
		t.Fatalf("model name = %q, want deepseek-chat", classifier.ModelName)
	}
	client, ok := classifier.Extractor.(*llmclients.OpenAIJSONClient)
	if !ok {
		t.Fatalf("extractor type = %T, want *llmclients.OpenAIJSONClient", classifier.Extractor)
	}
	if client.APIKey != "sk-test-classify" {
		t.Fatalf("api key = %q, want sk-test-classify", client.APIKey)
	}
	if client.BaseURL != "https://api.deepseek.com" {
		t.Fatalf("base url = %q, want https://api.deepseek.com", client.BaseURL)
	}
	if classifier.PromptText == "" {
		t.Fatal("expected the versioned prompt to be loaded")
	}
	if len(classifier.Vocabulary.Paths) == 0 {
		t.Fatal("expected the default governed vocabulary to be set")
	}
}

// TestNewProductionDocumentClassifierMissingModelConfigReturnsError proves
// a missing/unconfigured CLASSIFY_DOCUMENT_MODEL_NAME fails loudly rather
// than silently building a client with empty credentials.
func TestNewProductionDocumentClassifierMissingModelConfigReturnsError(t *testing.T) {
	t.Setenv("CLASSIFY_DOCUMENT_MODEL_NAME", "")
	if _, err := newProductionDocumentClassifier(nil, nil); err == nil {
		t.Fatal("expected an error when CLASSIFY_DOCUMENT_MODEL_NAME is unset")
	}
}
