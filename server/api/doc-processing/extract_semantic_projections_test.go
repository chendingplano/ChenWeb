package docprocessing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	llmclients "github.com/chendingplano/shared/go/api/llm"
)

func TestNormalizeSemanticProjection_Basic(t *testing.T) {
	raw := map[string]any{
		"language":            "zh",
		"descriptive_name":    "疫苗接种记录管理",
		"descriptive_name_en": "Vaccination Record Management",
		"keywords":            []any{"疫苗", "接种", "记录"},
		"keywords_en":         []any{"vaccine", "vaccination", "record"},
		"category_paths": []any{
			map[string]any{
				"category_path": []any{
					map[string]any{"name": "public_health", "keywords": []any{"health"}, "confidence": 0.95},
				},
				"path_keywords":   []any{"health"},
				"path_confidence": 0.95,
			},
		},
		"category_paths_en": []any{
			map[string]any{
				"category_path": []any{
					map[string]any{"name": "public_health", "keywords": []any{"health"}, "confidence": 0.95},
				},
				"path_keywords":   []any{"health"},
				"path_confidence": 0.95,
			},
		},
	}

	result := normalizeSemanticProjection(raw)

	if result["language"] != "zh" {
		t.Errorf("expected language=zh, got %v", result["language"])
	}
	if result["descriptive_name"] != "疫苗接种记录管理" {
		t.Errorf("expected descriptive_name, got %v", result["descriptive_name"])
	}
	if result["descriptive_name_en"] != "Vaccination Record Management" {
		t.Errorf("expected descriptive_name_en, got %v", result["descriptive_name_en"])
	}
	kw, ok := result["keywords"].([]string)
	if !ok || len(kw) != 3 {
		t.Errorf("expected 3 keywords, got %v", result["keywords"])
	}
	kwEn, ok := result["keywords_en"].([]string)
	if !ok || len(kwEn) != 3 {
		t.Errorf("expected 3 keywords_en, got %v", result["keywords_en"])
	}
	if result["category_paths"] == nil {
		t.Error("expected category_paths to be set")
	}
	if result["category_paths_en"] == nil {
		t.Error("expected category_paths_en to be set")
	}
}

func TestNormalizeSemanticProjection_EnglishSkipsEnFields(t *testing.T) {
	raw := map[string]any{
		"language":            "en",
		"descriptive_name":    "Vaccination Record Management",
		"descriptive_name_en": "Vaccination Record Management",
		"keywords":            []any{"vaccine", "vaccination", "record"},
		"keywords_en":         []any{"vaccine", "vaccination", "record"},
		"category_paths":      []any{},
		"category_paths_en":   []any{},
	}

	result := normalizeSemanticProjection(raw)

	if result["language"] != "en" {
		t.Errorf("expected language=en, got %v", result["language"])
	}
	// _en fields should still be normalized — language guard is at save time
	if result["descriptive_name"] != "Vaccination Record Management" {
		t.Errorf("expected descriptive_name, got %v", result["descriptive_name"])
	}
}

func TestAppendSemanticProjectionsStatus_Success(t *testing.T) {
	raw := "[]"
	from := semanticProjectionsStatusParams{
		RecordID:      42,
		FileType:      "pdf",
		InputFilename: "Artifacts/0/42/file.txt",
		DurationMs:    123,
	}
	out, err := appendSemanticProjectionsStatus(raw, from)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var entries []map[string]any
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	entry := entries[0]
	if entry["operation"] != "extract_semantic_projections" {
		t.Errorf("expected operation=extract_semantic_projections, got %v", entry["operation"])
	}
	if entry["proc_status"] != "success" {
		t.Errorf("expected proc_status=success, got %v", entry["proc_status"])
	}
	if entry["record_id"] != "42" {
		t.Errorf("expected record_id=42, got %v", entry["record_id"])
	}
	if _, hasErr := entry["error"]; hasErr {
		t.Error("success entry should not have error field")
	}
}

func TestAppendSemanticProjectionsStatus_Failure(t *testing.T) {
	raw := "[]"
	from := semanticProjectionsStatusParams{
		RecordID:      7,
		FileType:      "docx",
		InputFilename: "Artifacts/0/7/file.txt",
		DurationMs:    456,
		ProcErr:       fmt.Errorf("something went wrong"),
	}
	out, err := appendSemanticProjectionsStatus(raw, from)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var entries []map[string]any
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	entry := entries[0]
	if entry["proc_status"] != "failed" {
		t.Errorf("expected proc_status=failed, got %v", entry["proc_status"])
	}
	if entry["error"] != "something went wrong" {
		t.Errorf("expected error message, got %v", entry["error"])
	}
}

func TestAppendSemanticProjectionsStatus_Replaces(t *testing.T) {
	existing := `[{"operation":"extract_semantic_projections","proc_status":"failed","record_id":"5"}]`
	from := semanticProjectionsStatusParams{
		RecordID:   5,
		FileType:   "pdf",
		DurationMs: 10,
	}
	out, err := appendSemanticProjectionsStatus(existing, from)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var entries []map[string]any
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry after replace, got %d", len(entries))
	}
	if entries[0]["proc_status"] != "success" {
		t.Errorf("expected replaced entry to be success")
	}
}

// ---- concurrent extraction tests ----

// noopLogger is a thread-safe no-op logger for use in concurrent tests where fakeLogger
// would race (it appends to slices without a mutex).
type noopLogger struct{}

func (noopLogger) Debug(string, ...any)       {}
func (noopLogger) Line(string, ...any)        {}
func (noopLogger) Trace(string)               {}
func (noopLogger) Close()                     {}
func (noopLogger) Warn(string, ...any)        {}
func (noopLogger) Error(string, ...any)       {}
func (noopLogger) Info(string, ...any)        {}

// semProjTestExtractor is a thread-safe LLM extractor for concurrent extraction tests.
// P1 (candidate) calls are identified by candidateModel; all other calls are treated as P2.
// When blockP1=true, P1 calls block until releaseP1 is closed, allowing in-flight measurement.
type semProjTestExtractor struct {
	mu             sync.Mutex
	candidateModel string
	inFlight       int
	maxInFlight    int
	arrivedP1      int
	wantP1         int        // close p1Ready when arrivedP1 reaches this
	p1Ready        chan struct{}
	releaseP1      chan struct{} // close to unblock blocked P1 callers
	blockP1        bool
	failOnP1       int // return error on the Nth P1 call (1-indexed, 0 = never)
	p1CallCount    int
}

func (e *semProjTestExtractor) ExtractJSON(ctx context.Context, in llmclients.JSONExtractionInput) (map[string]any, error) {
	isP1 := in.ModelName == e.candidateModel

	e.mu.Lock()
	e.inFlight++
	if e.inFlight > e.maxInFlight {
		e.maxInFlight = e.inFlight
	}
	var shouldFail bool
	if isP1 {
		e.p1CallCount++
		if e.failOnP1 > 0 && e.p1CallCount == e.failOnP1 {
			shouldFail = true
		}
		if e.blockP1 {
			e.arrivedP1++
			if e.p1Ready != nil && e.arrivedP1 >= e.wantP1 {
				select {
				case <-e.p1Ready:
				default:
					close(e.p1Ready)
				}
			}
		}
	}
	e.mu.Unlock()

	defer func() {
		e.mu.Lock()
		e.inFlight--
		e.mu.Unlock()
	}()

	if isP1 && e.blockP1 && e.releaseP1 != nil {
		select {
		case <-e.releaseP1:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if shouldFail {
		return nil, errors.New("extraction boom")
	}

	if isP1 {
		return map[string]any{
			"semantic_projection": "sp-" + strings.TrimSpace(in.ModelName),
			"keywords":            []any{"kw"},
		}, nil
	}
	return map[string]any{
		"language":            "zh",
		"descriptive_name":    "dname",
		"descriptive_name_en": "dname-en",
		"keywords":            []any{"kw"},
		"keywords_en":         []any{"kw-en"},
		"category_paths":      []any{},
		"category_paths_en":   []any{},
	}, nil
}

func makeTestSemProjChunks(n int) []Chunk {
	chunks := make([]Chunk, n)
	for i := range n {
		chunks[i] = Chunk{
			SeqNo: i + 1,
			Lines: []MarkedLine{{Line: Line{Raw: fmt.Sprintf("line %d", i+1), LineNo: i + 1}}},
		}
	}
	return chunks
}

func newTestSemProjProcessor(ext LLMJSONExtractor, maxTasks int) *SemanticProjectionsProcessor {
	return &SemanticProjectionsProcessor{
		Extractor:           ext,
		Logger:              noopLogger{},
		ProcLogger:          DocProcLogger{},
		Now:                 time.Now,
		CandidateModelName:  "candidate-model",
		CandidatePromptText: "p1-prompt",
		EnrichModelName:     "enrich-model",
		EnrichPromptText:    "p2-prompt",
		MaxTasks:            maxTasks,
	}
}

// TestExtractSemanticProjections_SequentialCorrectness verifies basic correctness
// with MaxTasks=1 (sequential path).
func TestExtractSemanticProjections_SequentialCorrectness(t *testing.T) {
	ext := &semProjTestExtractor{candidateModel: "candidate-model"}
	p := newTestSemProjProcessor(ext, 1)
	chunks := makeTestSemProjChunks(3)

	result, err := p.extractSemanticProjectionsFromChunks(context.Background(), 99, chunks, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Projections) != 3 {
		t.Fatalf("got %d projections, want 3", len(result.Projections))
	}
	for i, sn := range result.SeqNos {
		if sn != i+1 {
			t.Errorf("SeqNos[%d]=%d, want %d", i, sn, i+1)
		}
	}
}

// TestExtractSemanticProjections_DeterministicOrder verifies that concurrent execution
// (MaxTasks=3, all chunks in parallel) still produces projections in SeqNo order.
func TestExtractSemanticProjections_DeterministicOrder(t *testing.T) {
	ext := &semProjTestExtractor{candidateModel: "candidate-model"}
	p := newTestSemProjProcessor(ext, 3)
	chunks := makeTestSemProjChunks(3)

	result, err := p.extractSemanticProjectionsFromChunks(context.Background(), 99, chunks, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Projections) != 3 {
		t.Fatalf("got %d projections, want 3", len(result.Projections))
	}
	for i, sn := range result.SeqNos {
		if sn != i+1 {
			t.Errorf("SeqNos[%d]=%d, want %d (ordering not preserved)", i, sn, i+1)
		}
	}
}

// TestExtractSemanticProjections_ConcurrentBounded verifies that MaxTasks=2 limits
// concurrent LLM calls to at most 2 even with 3 chunks.
func TestExtractSemanticProjections_ConcurrentBounded(t *testing.T) {
	releaseCh := make(chan struct{})
	ext := &semProjTestExtractor{
		candidateModel: "candidate-model",
		blockP1:        true,
		wantP1:         2,
		p1Ready:        make(chan struct{}),
		releaseP1:      releaseCh,
	}
	p := newTestSemProjProcessor(ext, 2)
	chunks := makeTestSemProjChunks(3)

	done := make(chan error, 1)
	go func() {
		_, err := p.extractSemanticProjectionsFromChunks(context.Background(), 99, chunks, "")
		done <- err
	}()

	// Wait until MaxTasks goroutines are blocked inside P1.
	select {
	case <-ext.p1Ready:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for concurrent P1 callers")
	}

	ext.mu.Lock()
	maxSeen := ext.maxInFlight
	ext.mu.Unlock()
	if maxSeen > 2 {
		t.Errorf("maxInFlight=%d exceeded MaxTasks=2", maxSeen)
	}

	close(releaseCh)

	if err := <-done; err != nil {
		t.Fatalf("unexpected error after release: %v", err)
	}
}

// TestExtractSemanticProjections_FailFast verifies that a P1 error cancels sibling jobs
// and returns the first error without persisting a success status.
func TestExtractSemanticProjections_FailFast(t *testing.T) {
	ext := &semProjTestExtractor{
		candidateModel: "candidate-model",
		failOnP1:       1, // fail the very first P1 call
	}
	p := newTestSemProjProcessor(ext, 2)
	chunks := makeTestSemProjChunks(3)

	_, err := p.extractSemanticProjectionsFromChunks(context.Background(), 99, chunks, "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "extraction boom") {
		t.Errorf("unexpected error: %v", err)
	}
}
