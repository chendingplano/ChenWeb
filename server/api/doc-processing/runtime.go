package docprocessing

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/spf13/viper"
)

// ProductionRuntime is the fully initialized processor graph used by the
// command and by embedders such as benchmark workers.
type ProductionRuntime struct {
	Control    *ControlService
	Processors []Processor
	services   map[string]any
	config     ResolvedConfigSnapshot
}

// ResolvedConfigSnapshot is a deterministic, secret-free description of the
// effective production runtime configuration.
type ResolvedConfigSnapshot struct {
	Values        map[string]any `json:"values"`
	CanonicalJSON []byte         `json:"-"`
	Hash          string         `json:"hash"`
}

// NewProductionRuntime builds the same dependency-ordered graph as the
// doc-processor command. The optional logger argument is accepted as an any
// value to keep this seam convenient for command and worker callers.
func NewProductionRuntime(args ...any) (*ProductionRuntime, error) {
	var logger ApiTypes.JimoLogger
	for _, arg := range args {
		if l, ok := arg.(ApiTypes.JimoLogger); ok {
			logger = l
		}
	}
	newClient := func() *llmclients.OpenAIJSONClient {
		return &llmclients.OpenAIJSONClient{HTTPClient: &http.Client{Timeout: 100 * time.Second}}
	}
	inputStore := DocMetadataSQLStore{DB: ApiTypes.ProjectDBHandle}
	fixed := NewFixedSizeChunkingService(SQLStore{DB: ApiTypes.ProjectDBHandle}, newClient(), logger)
	phase := []Processor{NewChunkingProcessor(inputStore, fixed, logger), NewGenerateSummariesProcessor(inputStore, fixed, logger), NewGenerateTopicsProcessor(inputStore, fixed, logger)}
	all := []Processor{
		NewStaticAnalyzerProcessor(inputStore, newClient(), logger), phase[0], phase[1], phase[2],
		NewExtractDocMetadataProcessor(inputStore, newClient(), logger),
		NewSemanticProjectionsProcessor(inputStore, SemanticProjectionsSQLStore{DB: ApiTypes.ProjectDBHandle}, newClient(), logger),
		NewStructuredKnowledgeProcessor(inputStore, StructuredKnowledgeSQLStore{DB: ApiTypes.ProjectDBHandle}, newClient(), logger),
		NewEntityProcessor(inputStore, EntityRelationSQLStore{DB: ApiTypes.ProjectDBHandle}, newClient(), logger),
		NewRelationProcessor(inputStore, EntityRelationSQLStore{DB: ApiTypes.ProjectDBHandle}, newClient(), logger),
		NewInventoryItemsProcessor(inputStore, InventoryItemsSQLStore{DB: ApiTypes.ProjectDBHandle}, newClient(), logger),
		NewMetricsProcessor(inputStore, MetricsSQLStore{DB: ApiTypes.ProjectDBHandle}, newClient(), logger),
		NewProvisionsProcessor(inputStore, ProvisionsSQLStore{DB: ApiTypes.ProjectDBHandle}, newClient(), logger),
		NewSceneBlocksProcessor(inputStore, SceneObjectsSQLStore{DB: ApiTypes.ProjectDBHandle}, newClient(), logger),
	}
	control := &ControlService{Logger: logger, InputStore: inputStore, EventStore: SQLStore{DB: ApiTypes.ProjectDBHandle}, RunStore: SQLStore{DB: ApiTypes.ProjectDBHandle}, StopStore: StopRequestSQLStore{DB: ApiTypes.ProjectDBHandle}, Now: time.Now, MaxDocProcessPipelines: MaxDocProcessPipelinesFromEnv(), BlockingProcessor: NewBlockingProcessor(inputStore, logger), Processors: filterProcessors(all, configuredNames())}
	r := &ProductionRuntime{Control: control, Processors: control.Processors, services: map[string]any{"chunking": fixed.RuntimeConfig()}}
	for _, p := range control.Processors {
		if m, ok := p.(*MetricsProcessor); ok {
			r.services["extract_metrics"] = m.RuntimeConfig()
		}
	}
	r.config = makeResolvedConfig(control, r.services)
	return r, nil
}

func configuredNames() []string { return viper.GetStringSlice("doc-processing.required_processors") }

func filterProcessors(processors []Processor, required []string) []Processor {
	mandatory := map[string]bool{"static_analyzer": true, "chunking": true, "extract_doc_metadata": true}
	enabled := map[string]bool{}
	for _, n := range required {
		enabled[normalizeRuntimeName(n)] = true
	}
	out := make([]Processor, 0, len(processors))
	for _, p := range processors {
		if p != nil && (mandatory[normalizeRuntimeName(p.Name())] || enabled[normalizeRuntimeName(p.Name())]) {
			out = append(out, p)
		}
	}
	return out
}

func normalizeRuntimeName(s string) string {
	s = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(s, "-", "_")))
	if s == "extract_metadata" {
		return "extract_doc_metadata"
	}
	return s
}

func (r *ProductionRuntime) AllowedOverrides() map[string][]string {
	return map[string][]string{
		"chunking":        {"CHUNK_SIZE", "CHUNK_OVERLAP_PERCENT"},
		"extract_metrics": {"EXTRACT_METRIC_CANDIDATES_PROMPT", "EXTRACT_METRIC_CANDIDATES_MODEL_NAME", "EXTRACT_METRIC_CANDIDATES_MODEL_FALLBACK", "ENRICH_METRICS_PROMPT", "ENRICH_METRICS_MODEL_NAME", "EXTRACT_METRICS_MODEL_NAME", "METRIC_MERGE_RESOLVE_PROMPT", "METRIC_MERGE_RESOLVE_MODEL_NAME", "METRIC_MERGE_RESOLVE_MODEL_FALLBACK", "METRIC_ENRICH_GROUP_SIZE", "EXTRACT_METRICS_MAX_TASKS"},
	}
}

func (r *ProductionRuntime) ResolvedConfig() ResolvedConfigSnapshot {
	if r.config.Hash == "" {
		r.config = makeResolvedConfig(r.Control, r.services)
	}
	return r.config
}
func makeResolvedConfig(c *ControlService, services map[string]any) ResolvedConfigSnapshot {
	names := []string{}
	if c != nil {
		for _, p := range c.Processors {
			names = append(names, p.Name())
		}
	}
	sort.Strings(names)
	v := map[string]any{"processors": names, "max_doc_process_pipelines": 0, "run_doc_processor_concurrent": RunDocProcessorConcurrentFromEnv(), "chunk_size": envInt("CHUNK_SIZE", DefaultChunkSize, 1), "chunk_overlap_percent": envInt("CHUNK_OVERLAP_PERCENT", DefaultOverlapPercent, 0)}
	v["prompt_hashes"] = map[string]string{}
	v["model_references"] = map[string]any{}
	v["concurrency"] = map[string]any{"run_doc_processor_concurrent": RunDocProcessorConcurrentFromEnv()}
	v["seed_support"] = false
	if services != nil {
		v["services"] = services
	}
	if c != nil {
		v["max_doc_process_pipelines"] = c.MaxDocProcessPipelines
	}
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return ResolvedConfigSnapshot{Values: v, CanonicalJSON: b, Hash: hex.EncodeToString(h[:])}
}
