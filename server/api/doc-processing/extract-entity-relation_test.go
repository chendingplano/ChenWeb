package docprocessing

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

func TestNormalizeEntityRows(t *testing.T) {
	raw := []any{
		map[string]any{
			"entity":         "PostgreSQL",
			"entity_en":      "PostgreSQL",
			"entity_type":    "software_system",
			"entity_type_en": "software system",
			"aliases":        []any{"postgres", " pg "},
			"aliases_en":     []any{},
			"desc":           "relational database",
			"desc_en":        "",
			"keywords":       []any{"db", "rdbms"},
			"keywords_en":    []any{},
			"lines":          []any{"12", "14-16"},
			"confidence":     0.92,
		},
		map[string]any{
			// dropped: missing entity name
			"entity":     "  ",
			"desc":       "should be dropped",
			"confidence": 0.99,
		},
		"not-a-map",
	}
	out := normalizeEntityRows(raw, 3)
	if len(out) != 1 {
		t.Fatalf("expected 1 normalized entity, got %d: %#v", len(out), out)
	}
	got := out[0]
	if got["entity"] != "PostgreSQL" {
		t.Errorf("entity = %q, want PostgreSQL", got["entity"])
	}
	if got["chunk_seq_no"] != 3 {
		t.Errorf("chunk_seq_no = %v, want 3", got["chunk_seq_no"])
	}
	wantAliases := []string{"postgres", "pg"}
	if !reflect.DeepEqual(got["aliases"], wantAliases) {
		t.Errorf("aliases = %#v, want %#v", got["aliases"], wantAliases)
	}
	wantSpans := []string{"12", "14-16"}
	if !reflect.DeepEqual(got["line_spans"], wantSpans) {
		t.Errorf("line_spans = %#v, want %#v", got["line_spans"], wantSpans)
	}
}

func TestNormalizeRelationRows(t *testing.T) {
	raw := []any{
		map[string]any{
			"subject":      "temperature_monitoring_device",
			"subject_en":   "temperature monitoring device",
			"predicate":    "Triggers",
			"predicate_en": "triggers",
			"object":       "excursion_alarm",
			"object_en":    "excursion alarm",
			"desc":         "device triggers alarm",
			"keywords":     []any{"trigger", "alarm"},
			"lines":        []any{"40"},
			"confidence":   0.88,
		},
		map[string]any{
			// dropped: empty object
			"subject":   "x",
			"predicate": "y",
			"object":    "",
		},
	}
	out := normalizeRelationRows(raw, 7)
	if len(out) != 1 {
		t.Fatalf("expected 1 normalized relation, got %d: %#v", len(out), out)
	}
	got := out[0]
	if got["predicate"] != "triggers" {
		t.Errorf("predicate = %q, want triggers (lowercased+snake)", got["predicate"])
	}
	if got["chunk_seq_no"] != 7 {
		t.Errorf("chunk_seq_no = %v, want 7", got["chunk_seq_no"])
	}
}

func TestNormalizePredicate(t *testing.T) {
	cases := map[string]string{
		"":            "",
		"   ":         "",
		"triggers":    "triggers",
		"DEPENDS ON":  "depends_on",
		"  Has  Many": "has_many",
	}
	for in, want := range cases {
		if got := normalizePredicate(in); got != want {
			t.Errorf("normalizePredicate(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsLanguageEnglish(t *testing.T) {
	if !isLanguageEnglish("en") {
		t.Error("en should be English")
	}
	if !isLanguageEnglish(" English ") {
		t.Error("English should be English")
	}
	if isLanguageEnglish("zh") {
		t.Error("zh should not be English")
	}
	if isLanguageEnglish("") {
		t.Error("empty should not be English")
	}
}

func TestAppendEntityRelationStatusAppendsNew(t *testing.T) {
	existing := `[{"operation":"chunked","proc_status":"success"}]`
	out, err := appendEntityRelationStatus(existing, entityRelationStatusParams{
		RecordID:      42,
		FileType:      "pdf",
		InputFilename: "f.txt",
		Start:         time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		DurationMs:    100,
		ProcErr:       nil,
	})
	if err != nil {
		t.Fatalf("append err: %v", err)
	}
	var arr []map[string]any
	if err := json.Unmarshal([]byte(out), &arr); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(arr) != 2 {
		t.Fatalf("len=%d, want 2: %s", len(arr), out)
	}
	if arr[1]["operation"] != "extract_entity_relation" {
		t.Errorf("appended op = %v, want extract_entity_relation", arr[1]["operation"])
	}
	if arr[1]["proc_status"] != "success" {
		t.Errorf("proc_status = %v, want success", arr[1]["proc_status"])
	}
}

func TestAppendEntityRelationStatusReplacesExisting(t *testing.T) {
	existing := `[{"operation":"extract_entity_relation","proc_status":"failed","error":"old"}]`
	out, err := appendEntityRelationStatus(existing, entityRelationStatusParams{
		RecordID:   42,
		FileType:   "pdf",
		Start:      time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		DurationMs: 200,
		ProcErr:    nil,
	})
	if err != nil {
		t.Fatalf("append err: %v", err)
	}
	var arr []map[string]any
	if err := json.Unmarshal([]byte(out), &arr); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(arr) != 1 {
		t.Fatalf("len=%d, want 1: %s", len(arr), out)
	}
	if arr[0]["proc_status"] != "success" {
		t.Errorf("proc_status = %v, want success (replaced)", arr[0]["proc_status"])
	}
	if _, ok := arr[0]["error"]; ok {
		t.Errorf("stale error field should be gone after replacement: %v", arr[0])
	}
}

func TestWriteEntityRelationArtifactFile(t *testing.T) {
	tmp := t.TempDir()
	rec := DocMetadataInputRecord{
		StagingFilename: "std_20039_opendata.pdf",
		ParserName:      "marker",
	}
	rows := []map[string]any{
		{"entity": "PostgreSQL", "desc": "a database"},
		{"entity": "FDA", "desc": "an agency"},
	}
	if err := writeEntityRelationArtifactFile(tmp, 100, rec, rows, ".entities", "MID_TEST"); err != nil {
		t.Fatalf("write: %v", err)
	}
	path := filepath.Join(tmp, "0", "100", "std_20039_opendata_marker.entities")
	bs, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var got []map[string]any
	if err := json.Unmarshal(bs, &got); err != nil {
		t.Fatalf("unmarshal: %v\nfile:\n%s", err, string(bs))
	}
	if len(got) != 2 {
		t.Fatalf("len=%d, want 2", len(got))
	}
}

func TestWriteEntityRelationArtifactFileEmptyIsNoop(t *testing.T) {
	tmp := t.TempDir()
	rec := DocMetadataInputRecord{StagingFilename: "x.pdf", ParserName: "marker"}
	if err := writeEntityRelationArtifactFile(tmp, 100, rec, nil, ".entities", "MID_TEST"); err != nil {
		t.Fatalf("write nil: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, "0")); !os.IsNotExist(err) {
		t.Errorf("expected no directory for empty rows, got err=%v", err)
	}
}

func TestEntityExtractionContractShape(t *testing.T) {
	contract := entityExtractionContract()
	if contract.Name == "" {
		t.Fatal("contract name empty")
	}
	if len(contract.Schema) == 0 {
		t.Fatal("contract schema empty")
	}
	var schema map[string]any
	if err := json.Unmarshal(contract.Schema, &schema); err != nil {
		t.Fatalf("schema unmarshal: %v", err)
	}
	props, _ := schema["properties"].(map[string]any)
	for _, key := range []string{"language", "entities"} {
		if _, ok := props[key]; !ok {
			t.Errorf("schema properties missing %q: %v", key, props)
		}
	}
	// Phase 1 is entity-only: relations must NOT be in the schema (ADR 2026061302 D2).
	if _, ok := props["relations"]; ok {
		t.Errorf("entity contract should not contain 'relations': %v", props)
	}
}

func TestRelationExtractionContractShape(t *testing.T) {
	contract := relationExtractionContract()
	var schema map[string]any
	if err := json.Unmarshal(contract.Schema, &schema); err != nil {
		t.Fatalf("schema unmarshal: %v", err)
	}
	props, _ := schema["properties"].(map[string]any)
	for _, key := range []string{"language", "relations"} {
		if _, ok := props[key]; !ok {
			t.Errorf("schema properties missing %q: %v", key, props)
		}
	}
}

// erSimpleExtractor is a non-thread-safe extractor that returns a fixed result for every call.
// It does NOT implement LLMStructuredJSONExtractor, so extractEntityRelationPayload uses ExtractJSON.
type erSimpleExtractor struct {
	out map[string]any
	err error
}

func (e *erSimpleExtractor) ExtractJSON(_ context.Context, _ llmclients.JSONExtractionInput) (map[string]any, error) {
	return e.out, e.err
}

// erSeqExtractor pops one out/err pair per call (thread-safe for concurrent use).
type erSeqExtractor struct {
	mu   sync.Mutex
	outs []map[string]any
	errs []error
}

func (e *erSeqExtractor) ExtractJSON(_ context.Context, _ llmclients.JSONExtractionInput) (map[string]any, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out map[string]any
	var err error
	if len(e.outs) > 0 {
		out = e.outs[0]
		e.outs = e.outs[1:]
	}
	if len(e.errs) > 0 {
		err = e.errs[0]
		e.errs = e.errs[1:]
	}
	return out, err
}

type erHandleEventInputStore struct {
	rec DocMetadataInputRecord
}

func (s *erHandleEventInputStore) GetInputRecord(_ context.Context, id int64) (DocMetadataInputRecord, error) {
	if id != s.rec.ID {
		return DocMetadataInputRecord{}, errors.New("not found")
	}
	return s.rec, nil
}

func (s *erHandleEventInputStore) UpdateInputMetadata(_ context.Context, id int64, req DocMetadataUpdate) error {
	if id != s.rec.ID {
		return errors.New("not found")
	}
	s.rec.StatusRaw = req.StatusRaw
	return nil
}

type erHandleEventEntityStore struct {
	savedEntities  SaveEntitiesRequest
	savedRelations SaveRelationsRequest
}

func (s *erHandleEventEntityStore) EntitiesExist(context.Context, int64) (bool, error) {
	return false, nil
}

func (s *erHandleEventEntityStore) DeleteEntitiesByInputRecordID(context.Context, int64) (int64, error) {
	return 0, nil
}

func (s *erHandleEventEntityStore) SaveEntities(_ context.Context, req SaveEntitiesRequest) (int64, error) {
	s.savedEntities = req
	return int64(len(req.Entities)), nil
}

func (s *erHandleEventEntityStore) RelationsExist(context.Context, int64) (bool, error) {
	return false, nil
}

func (s *erHandleEventEntityStore) DeleteRelationsByInputRecordID(context.Context, int64) (int64, error) {
	return 0, nil
}

func (s *erHandleEventEntityStore) SaveRelations(_ context.Context, req SaveRelationsRequest) (int64, error) {
	s.savedRelations = req
	return int64(len(req.Relations)), nil
}

// erBlockingExtractor blocks each call until a gate signal is received.
// It is thread-safe and tracks the maximum number of concurrent callers.
type erBlockingExtractor struct {
	mu          sync.Mutex
	inFlight    int
	MaxInFlight int
	ready       chan struct{}
	readyOnce   sync.Once
	readyAt     int // close ready when inFlight reaches this value
	gate        chan struct{}
	out         map[string]any
}

func (e *erBlockingExtractor) ExtractJSON(ctx context.Context, _ llmclients.JSONExtractionInput) (map[string]any, error) {
	e.mu.Lock()
	e.inFlight++
	if e.inFlight > e.MaxInFlight {
		e.MaxInFlight = e.inFlight
	}
	if e.inFlight >= e.readyAt {
		e.readyOnce.Do(func() { close(e.ready) })
	}
	e.mu.Unlock()

	defer func() {
		e.mu.Lock()
		e.inFlight--
		e.mu.Unlock()
	}()

	select {
	case <-e.gate:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return e.out, nil
}

func makeTestChunk(seqNo int) Chunk {
	return Chunk{
		SeqNo: seqNo,
		Lines: []MarkedLine{{Line: Line{Content: "text", LineNo: seqNo}, Mark: "r"}},
	}
}

func makeTestProcessor(ext LLMJSONExtractor, maxTasks int) *EntityRelationProcessor {
	return &EntityRelationProcessor{
		Logger:                        loggerutil.CreateDefaultLogger("TEST_ER"),
		Extractor:                     ext,
		Now:                           time.Now,
		ModelName:                     "test-model",
		LLMController:                 newLLMCallController(llmCallControllerConfig{DefaultLimit: maxTasks, LeaseTTL: 320 * time.Second}),
		ExtractEntityRelationMaxTasks: maxTasks,
	}
}

func TestHandleEventDoesNotGenerateEntityContext(t *testing.T) {
	const recordID int64 = 101
	tmp := t.TempDir()
	lineFile := filepath.Join(tmp, "sample_marker.txt")
	lineContent := "1\t1\tparagraph\tArial\t12\t[0,0,1,1]\tEntity Alpha appears here\n"
	if err := os.WriteFile(lineFile, []byte(lineContent), 0o644); err != nil {
		t.Fatalf("write line file: %v", err)
	}
	targetDir := filepath.Join(tmp, "0", "101")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("mkdir artifact dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "sample_marker.chunks"), []byte("overlap: []\nlines: [1]\n"), 0o644); err != nil {
		t.Fatalf("write chunks file: %v", err)
	}

	inputStore := &erHandleEventInputStore{rec: DocMetadataInputRecord{
		ID:              recordID,
		ParserName:      "marker",
		ResultFilename:  lineFile,
		StagingFilename: "sample.pdf",
		StatusRaw:       "[]",
		Title:           "Sample Document",
	}}
	entityStore := &erHandleEventEntityStore{}
	p := &EntityRelationProcessor{
		InputStore: inputStore,
		Store:      entityStore,
		Logger:     &fakeLogger{},
		Extractor: &erSimpleExtractor{out: map[string]any{
			"language": "en",
			"entities": []any{
				map[string]any{"entity": "Entity Alpha", "lines": []any{"1"}, "confidence": 0.9},
			},
			"relations": []any{},
		}},
		Now:                           time.Now,
		PromptText:                    "extract entities",
		PromptRef:                     "test-prompt",
		ModelName:                     "test-model",
		RelationPromptText:            "",
		RelationPromptErr:             errors.New("relation prompt intentionally disabled"),
		ArtifactDir:                   tmp,
		ExtractEntityRelationMaxTasks: 1,
	}

	payload := []byte(`{"record_id":101,"force":true,"type":"pdf","status":"success"}`)
	if err := p.HandleEvent(context.Background(), payload); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if len(entityStore.savedEntities.Entities) != 1 {
		t.Fatalf("saved entities=%d, want 1", len(entityStore.savedEntities.Entities))
	}
	if got := strings.TrimSpace(asString(entityStore.savedEntities.Entities[0]["entity_context"])); got != "" {
		t.Fatalf("entity_context should not be generated before SaveEntities, got %q", got)
	}
}

func TestEntitySearchDocumentDedupeMigrationExists(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "../../.."))
	path := filepath.Join(repoRoot, "project_migrations", "20260617000003_dedupe_kb_entity_search_document.sql")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	text := string(body)
	for _, want := range []string{
		"CREATE OR REPLACE FUNCTION kb.entity_search_document",
		"jsonb_array_elements_text",
		"DISTINCT ON",
		"lower(part)",
		"UPDATE kb.entities",
		"entity_context = NULL",
		"search_vector = to_tsvector",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("migration %s missing %q", path, want)
		}
	}
}

func entityPayload(entityName string) map[string]any {
	return map[string]any{
		"language": "en",
		"entities": []any{
			map[string]any{"entity": entityName, "confidence": 0.9},
		},
		"relations": []any{},
	}
}

// TestExtractEntityRelationConcurrencyBound verifies that at most MaxTasks
// goroutines call the extractor concurrently.
func TestExtractEntityRelationConcurrencyBound(t *testing.T) {
	const numChunks = 4
	const maxTasks = 2

	ext := &erBlockingExtractor{
		ready:   make(chan struct{}),
		readyAt: maxTasks,
		gate:    make(chan struct{}, numChunks),
		out:     entityPayload("ent"),
	}

	chunks := make([]Chunk, numChunks)
	for i := range chunks {
		chunks[i] = makeTestChunk(i + 1)
	}

	p := makeTestProcessor(ext, maxTasks)

	done := make(chan error, 1)
	go func() {
		_, err := p.extractEntitiesFromChunks(context.Background(), 1, chunks, "")
		done <- err
	}()

	// Wait until maxTasks workers are simultaneously inside ExtractJSON.
	select {
	case <-ext.ready:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for workers to reach maxTasks in-flight")
	}

	// Release all workers.
	for range numChunks {
		ext.gate <- struct{}{}
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for extraction to complete")
	}

	ext.mu.Lock()
	got := ext.MaxInFlight
	ext.mu.Unlock()
	if got != maxTasks {
		t.Errorf("MaxInFlight = %d, want %d", got, maxTasks)
	}
}

func TestExtractEntityRelationModelPermitBound(t *testing.T) {
	const numChunks = 4
	const maxTasks = 4

	ext := &erBlockingExtractor{
		ready:   make(chan struct{}),
		readyAt: 1,
		gate:    make(chan struct{}, numChunks),
		out:     entityPayload("ent"),
	}

	chunks := make([]Chunk, numChunks)
	for i := range chunks {
		chunks[i] = makeTestChunk(i + 1)
	}

	p := makeTestProcessor(ext, maxTasks)
	p.LLMController = newLLMCallController(llmCallControllerConfig{
		DefaultLimit: 1,
		LeaseTTL:     320 * time.Second,
	})

	done := make(chan error, 1)
	go func() {
		_, err := p.extractEntitiesFromChunks(context.Background(), 1, chunks, "")
		done <- err
	}()

	select {
	case <-ext.ready:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for first worker to enter ExtractJSON")
	}

	select {
	case err := <-done:
		t.Fatalf("extraction should still be running behind the model permit, got early err=%v", err)
	case <-time.After(150 * time.Millisecond):
	}

	for range numChunks {
		ext.gate <- struct{}{}
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for extraction to complete")
	}

	ext.mu.Lock()
	got := ext.MaxInFlight
	ext.mu.Unlock()
	if got != 1 {
		t.Errorf("MaxInFlight = %d, want 1 with model permit limit", got)
	}
}

// TestExtractEntityRelationChunkOrder verifies that entities from multiple
// concurrent workers are aggregated in chunk-index order, not completion order.
func TestExtractEntityRelationChunkOrder(t *testing.T) {
	seqNos := []int{10, 20, 30}
	chunks := make([]Chunk, len(seqNos))
	for i, sn := range seqNos {
		chunks[i] = makeTestChunk(sn)
	}

	// Each call returns a single entity; chunk_seq_no is set by processChunk from the actual chunk.
	ext := &erSeqExtractor{
		outs: []map[string]any{
			entityPayload("alpha"),
			entityPayload("beta"),
			entityPayload("gamma"),
		},
		errs: []error{nil, nil, nil},
	}

	p := makeTestProcessor(ext, len(chunks)) // fully concurrent
	result, err := p.extractEntitiesFromChunks(context.Background(), 1, chunks, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Entities) != 3 {
		t.Fatalf("expected 3 entities, got %d", len(result.Entities))
	}
	for i, sn := range seqNos {
		got, _ := result.Entities[i]["chunk_seq_no"].(int)
		if got != sn {
			t.Errorf("entities[%d].chunk_seq_no = %d, want %d (chunk order not preserved)", i, got, sn)
		}
	}
}

// TestExtractEntityRelationSkipsFailedChunks verifies that an LLM error on one
// chunk increments failedChunks but does not abort sibling chunks.
func TestExtractEntityRelationSkipsFailedChunks(t *testing.T) {
	ext := &erSeqExtractor{
		outs: []map[string]any{
			entityPayload("alpha"),
			nil,
			entityPayload("gamma"),
		},
		errs: []error{
			nil,
			errors.New("llm transport error"),
			nil,
		},
	}

	chunks := []Chunk{makeTestChunk(1), makeTestChunk(2), makeTestChunk(3)}
	p := makeTestProcessor(ext, 1) // sequential for determinism
	result, err := p.extractEntitiesFromChunks(context.Background(), 1, chunks, "")
	if err != nil {
		t.Fatalf("LLM error on one chunk must not abort extraction, got err=%v", err)
	}
	if result.FailedChunks != 1 {
		t.Errorf("FailedChunks = %d, want 1", result.FailedChunks)
	}
	if len(result.Entities) != 2 {
		t.Errorf("expected 2 entities from successful chunks, got %d", len(result.Entities))
	}
}

// TestExtractEntityRelationStopPropagation verifies that a pipeline-stop context
// causes extractEntitiesFromChunks to return ErrPipelineStopped.
func TestExtractEntityRelationStopPropagation(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(ErrPipelineStopped)

	ext := &erSimpleExtractor{out: entityPayload("any")}
	chunks := []Chunk{makeTestChunk(1), makeTestChunk(2)}
	p := makeTestProcessor(ext, 1)

	_, err := p.extractEntitiesFromChunks(ctx, 1, chunks, "")
	if !errors.Is(err, ErrPipelineStopped) {
		t.Errorf("expected ErrPipelineStopped, got %v", err)
	}
}

func TestEntityRelationExtractionUsesFallbackOnPrimaryError(t *testing.T) {
	// fake pops one item per call from each slice. First call: (nil, error).
	// Second call (fallback model): (FDA payload, nil).
	fake := &fakeJSONExtractor{
		outs: []map[string]any{
			nil,
			{
				"language": "en",
				"entities": []any{
					map[string]any{"entity": "FDA", "desc": "agency"},
				},
				"relations": []any{},
			},
		},
		errs: []error{
			errors.New("primary llm transport error"),
			nil,
		},
	}
	p := &EntityRelationProcessor{
		Logger:            loggerutil.CreateDefaultLogger("TEST_ENT_REL"),
		Extractor:         fake,
		ModelName:         "primary",
		FallbackModelName: "fallback",
	}
	payload, modelName, err := p.extractEntityRelationWithFallback(context.Background(), "irrelevant")
	if err != nil {
		t.Fatalf("expected fallback to succeed, got err=%v", err)
	}
	if modelName != "fallback" {
		t.Errorf("modelName=%q, want fallback", modelName)
	}
	ents, _ := payload["entities"].([]any)
	if len(ents) != 1 {
		t.Errorf("entities len=%d, want 1: %#v", len(ents), payload)
	}
}

func TestParseLineSpanRange(t *testing.T) {
	cases := []struct {
		in           string
		wantS, wantE int
	}{
		{"14", 14, 14},
		{"14-16", 14, 16},
		{"14:16", 14, 16},
		{"  5  ", 5, 5},
		{"0", 0, 0}, // zero is invalid
		{"-1", 0, 0},
		{"abc", 0, 0},
		{"14-12", 0, 0}, // end < start
	}
	for _, c := range cases {
		s, e := parseLineSpanRange(c.in)
		if s != c.wantS || e != c.wantE {
			t.Errorf("parseLineSpanRange(%q) = (%d,%d), want (%d,%d)", c.in, s, e, c.wantS, c.wantE)
		}
	}
}

func TestBuildEntityContextForEntities(t *testing.T) {
	// Three document lines; chunk covers lines 2-3; level-0 summary for seqNo=1.
	lines := []Line{
		{LineNo: 1, Content: "intro line"},
		{LineNo: 2, Content: "entity line A"},
		{LineNo: 3, Content: "entity line B"},
	}
	chunks := []Chunk{
		{
			SeqNo: 1,
			Lines: []MarkedLine{
				{Line: lines[1], Mark: "r"},
				{Line: lines[2], Mark: "r"},
			},
		},
	}
	summaries := map[int]string{1: "chunk one summary"}
	entities := []map[string]any{
		{
			"entity":       "TestEntity",
			"line_spans":   []string{"2-3"},
			"chunk_seq_no": 1,
		},
	}

	buildEntityContextForEntities(entities, chunks, lines, summaries, 1)

	ctx, ok := entities[0]["entity_context"].(string)
	if !ok || ctx == "" {
		t.Fatalf("entity_context not set or empty: %#v", entities[0]["entity_context"])
	}
	if !strings.Contains(ctx, "Summary: chunk one summary") {
		t.Errorf("entity_context missing prefixed chunk summary; got:\n%s", ctx)
	}
	if !strings.Contains(ctx, "entity line A") {
		t.Errorf("entity_context missing span line; got:\n%s", ctx)
	}
	if !strings.Contains(ctx, "intro line") {
		t.Errorf("entity_context missing leading line; got:\n%s", ctx)
	}
}

func TestNormalizeEntityRowsCategories(t *testing.T) {
	raw := []any{
		map[string]any{
			"entity":            "FDA",
			"entity_categories": []any{"regulation", "agency"},
		},
		map[string]any{
			"entity": "TCP",
			// no entity_categories field — should result in nil/empty slice
		},
	}
	out := normalizeEntityRows(raw, 1)
	if len(out) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(out))
	}
	cats := out[0]["entity_categories"].([]string)
	if len(cats) != 2 || cats[0] != "regulation" || cats[1] != "agency" {
		t.Errorf("entity_categories = %v, want [regulation agency]", cats)
	}
	if cats2 := toStringSlice(out[1]["entity_categories"]); len(cats2) != 0 {
		t.Errorf("expected empty categories for entity without field, got %v", cats2)
	}
}

func TestBuildEntityContextForEntities_NoSpans(t *testing.T) {
	entities := []map[string]any{{"entity": "X", "line_spans": []string{}}}
	buildEntityContextForEntities(entities, nil, nil, nil, 3)
	if _, ok := entities[0]["entity_context"]; ok {
		t.Errorf("expected no entity_context key for empty spans, got %v", entities[0]["entity_context"])
	}
}
