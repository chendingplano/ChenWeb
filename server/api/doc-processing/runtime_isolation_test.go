package docprocessing

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
)

type isolationInputStore struct {
	DocMetadataStore
	records map[int64]DocMetadataInputRecord
}

func (s isolationInputStore) GetInputRecord(_ context.Context, id int64) (DocMetadataInputRecord, error) {
	return s.records[id], nil
}

func (s isolationInputStore) UpdateInputMetadata(context.Context, int64, DocMetadataUpdate) error {
	return nil
}

type isolationMetricsStore struct {
	MetricsStore
	mu    sync.Mutex
	saved map[int64][]string
}

func (s *isolationMetricsStore) MetricsExist(context.Context, int64) (bool, error) {
	return false, nil
}

func (s *isolationMetricsStore) GetMetricsByInputRecordID(context.Context, int64) ([]map[string]any, error) {
	return nil, nil
}

func (s *isolationMetricsStore) DeleteMetricsByInputRecordID(context.Context, int64) (int64, error) {
	return 0, nil
}

func (s *isolationMetricsStore) SaveMetrics(_ context.Context, req SaveMetricsRequest) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, row := range req.Metrics {
		s.saved[req.InputRecordID] = append(s.saved[req.InputRecordID], asString(row["metric_name"]))
	}
	return int64(len(req.Metrics)), nil
}

func (s *isolationMetricsStore) UpsertMetrics(ctx context.Context, req SaveMetricsRequest) (int64, error) {
	return s.SaveMetrics(ctx, req)
}

// Both calls must enter extraction before either can finish. Reusing a processor
// therefore deterministically overwrites the first document's batch record ID.
func TestProductionRuntimeOverlappingMetricsStayWithTheirDocument(t *testing.T) {
	for _, forceClear := range []bool{false, true} {
		for _, explicit := range []bool{false, true} {
			t.Run(fmt.Sprintf("force_clear=%t/explicit=%t", forceClear, explicit), func(t *testing.T) {
				t.Setenv("LLM_CALL_STAGGER", "0")
				t.Setenv("RUN_DOC_PROCESSOR_CONCURRENT", "true")
				t.Setenv("CLASSIFY_DOCUMENT_MODEL_NAME", "")
				dir := t.TempDir()
				lineFile := filepath.Join(dir, "source_opendata.txt")
				writeLineFile(t, lineFile)
				input := isolationInputStore{records: map[int64]DocMetadataInputRecord{}}
				for _, id := range []int64{218, 219} {
					input.records[id] = DocMetadataInputRecord{
						ID: id, ParserName: "opendata", ResultFilename: filepath.Join(dir, "result.json"),
						StagingFilename: filepath.Join(dir, "source.pdf"), StatusRaw: "[]",
					}
				}
				store := &isolationMetricsStore{saved: map[int64][]string{}}
				entered := make(chan struct{}, 2)
				release := make(chan struct{})
				original := buildProductionRuntimeComponents
				buildProductionRuntimeComponents = func(ApiTypes.JimoLogger) productionRuntimeComponents {
					ext := &simpleMetricsExtractor{respond: func(_ int, in llmclients.JSONExtractionInput) (map[string]any, error) {
						entered <- struct{}{}
						<-release
						name := "document 218 pressure"
						if strings.Contains(in.InputText, "document 219") {
							name = "document 219 pressure"
						}
						return map[string]any{"lang": "en", "m": []any{map[string]any{
							"n": name, "s": "compressor", "v": "10", "u": "MPa", "r": "upper_bound",
							"k": "requirement", "l": []any{"10"},
						}}}, nil
					}}
					p := makeTestSimpleMetricsProcessor(ext, store, input)
					p.ChunkDir = dir
					return productionRuntimeComponents{
						inputStore: input, fixed: &FixedSizeChunkingService{},
						processors: []Processor{runtimeSelectionProcessor("static_analyzer"),
							&isolationChunkingProcessor{}, p},
					}
				}
				t.Cleanup(func() { buildProductionRuntimeComponents = original })
				r, err := NewProductionRuntime(ProductionRuntimeOptions{
					RequiredProcessors: []string{"extract_metrics_simple"},
					DefaultProcessors:  []string{"extract_metrics_simple"},
				})
				if err != nil {
					t.Fatal(err)
				}
				control := r.Control
				control.EventStore, control.RunStore, control.PlanStore = nil, nil, nil
				control.StopStore, control.FacetStore, control.Facets = nil, nil, nil
				control.RoutingClearances, control.RoutingAlarms, control.PolicyAudit = nil, nil, nil
				control.TaskPolicy = taskEnginePolicy{}
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				errs := make(chan error, 2)
				for i, id := range []int64{218, 219} {
					go func() {
						operations := ""
						if explicit {
							operations = `,"operations":["extract_metrics_simple"]`
						}
						payload := fmt.Sprintf(`{"user_id":"test-user","record_id":%d,"force":true,"force_clear":%t%s}`, id, forceClear, operations)
						errs <- control.RunEvent(ctx, []byte(payload))
					}()
					select {
					case <-entered:
					case <-ctx.Done():
						close(release)
						for range i + 1 {
							<-errs
						}
						t.Fatalf("record %d never entered extraction: %v", id, ctx.Err())
					}
				}
				close(release)
				for range 2 {
					if err := <-errs; err != nil {
						t.Error(err)
					}
				}
				for _, id := range []int64{218, 219} {
					want := fmt.Sprintf("document %d pressure", id)
					if got := store.saved[id]; len(got) != 1 || got[0] != want {
						t.Errorf("record %d saved metrics %v, want [%s]", id, got, want)
					}
				}
			})
		}
	}
}

type isolationChunkingProcessor struct{}

func (*isolationChunkingProcessor) Name() string { return "chunking" }
func (*isolationChunkingProcessor) HandleEvent(ctx context.Context, payload []byte) error {
	evt, err := ParseLineFileGeneratedEvent(payload)
	if err != nil {
		return err
	}
	storeChunksInContext(ctx, []Chunk{{SeqNo: 0, Lines: []MarkedLine{
		simpleLine("n", 10, "paragraph", fmt.Sprintf("document %d: pressure <= 10 MPa", evt.RecordID)),
	}}})
	return nil
}

func TestProductionRuntimeRunGraphsKeepOverridesAndOwnTheirDependencies(t *testing.T) {
	original := buildProductionRuntimeComponents
	buildProductionRuntimeComponents = func(ApiTypes.JimoLogger) productionRuntimeComponents {
		fixed := &FixedSizeChunkingService{}
		return productionRuntimeComponents{
			fixed: fixed, blocking: &BlockingProcessor{},
			processors: []Processor{
				runtimeSelectionProcessor("static_analyzer"),
				&ChunkingProcessor{Service: fixed},
				makeTestSimpleMetricsProcessor(&simpleMetricsExtractor{}, &fakeMetricsStore{}, nil),
			},
		}
	}
	t.Cleanup(func() { buildProductionRuntimeComponents = original })
	required := []string{"extract_metrics_simple"}
	overrides := map[string]string{"CHUNK_SIZE": "123", "EXTRACT_METRICS_MAX_TASKS": "3"}
	r, err := NewProductionRuntime(ProductionRuntimeOptions{RequiredProcessors: required, Overrides: overrides})
	if err != nil {
		t.Fatal(err)
	}
	// Mutating caller-owned options must not change later graphs.
	required[0], overrides["CHUNK_SIZE"], overrides["EXTRACT_METRICS_MAX_TASKS"] = "extract_provisions", "999", "9"
	previous, previousBlocking := r.Processors, r.Control.BlockingProcessor
	for range 2 {
		processors, blocking, err := r.Control.newRunProcessors()
		if err != nil {
			t.Fatal(err)
		}
		if blocking == previousBlocking {
			t.Fatal("runs share a blocking processor")
		}
		for i, p := range processors {
			switch p := p.(type) {
			case *ChunkingProcessor:
				fixed := p.Service.(*FixedSizeChunkingService)
				if fixed == previous[i].(*ChunkingProcessor).Service || fixed.ChunkSize != 123 {
					t.Fatal("chunking service is shared or lost its override")
				}
			case *MetricsProcessor:
				old := previous[i].(*MetricsProcessor)
				if p == old || p.Extractor == old.Extractor || p.MaxTasks != 3 {
					t.Fatal("metrics processor/client is shared or lost its override")
				}
			}
		}
		previous, previousBlocking = processors, blocking
	}
}

func TestControlServiceRunGraphFailureDoesNotUseCatalogProcessors(t *testing.T) {
	wantErr := fmt.Errorf("processor configuration unavailable")
	processor := &nonBatchProc{name: "extract_metrics_simple"}
	control := &ControlService{
		Processors: []Processor{processor}, BlockingProcessor: processor,
		newRunProcessors: func() ([]Processor, Processor, error) { return nil, nil, wantErr },
	}
	err := control.RunEvent(context.Background(), []byte(`{"user_id":"test-user","record_id":218}`))
	if err == nil || !strings.Contains(err.Error(), wantErr.Error()) {
		t.Fatalf("RunEvent error = %v, want graph construction failure", err)
	}
	if processor.ran != 0 {
		t.Fatal("ran shared catalog processor after graph construction failed")
	}
}
