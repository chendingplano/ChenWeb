package docprocessing

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
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

// ProductionRuntimeOptions lets embedders select the processors they intend
// to run without depending on command-global Viper state.
type ProductionRuntimeOptions struct {
	Logger             ApiTypes.JimoLogger
	Overrides          map[string]string
	RequiredProcessors []string
}

type productionRuntimeComponents struct {
	inputStore DocMetadataStore
	fixed      *FixedSizeChunkingService
	processors []Processor
	blocking   Processor
}

var buildProductionRuntimeComponents = defaultProductionRuntimeComponents

func defaultProductionRuntimeComponents(logger ApiTypes.JimoLogger) productionRuntimeComponents {
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
	return productionRuntimeComponents{inputStore: inputStore, fixed: fixed, processors: all, blocking: NewBlockingProcessor(inputStore, logger)}
}

// NewProductionRuntime builds the same dependency-ordered graph as the
// doc-processor command. The optional logger argument is accepted as an any
// value to keep this seam convenient for command and worker callers.
func NewProductionRuntime(args ...any) (*ProductionRuntime, error) {
	var logger ApiTypes.JimoLogger
	var overrides map[string]string
	var required []string
	explicitSelection := false
	for _, arg := range args {
		if l, ok := arg.(ApiTypes.JimoLogger); ok {
			logger = l
		}
		if m, ok := arg.(map[string]string); ok {
			overrides = m
		}
		if names, ok := arg.([]string); ok {
			required, explicitSelection = names, true
		}
		if opts, ok := arg.(ProductionRuntimeOptions); ok {
			logger, overrides, required, explicitSelection = opts.Logger, opts.Overrides, opts.RequiredProcessors, true
		}
	}
	if !explicitSelection {
		required = configuredNames()
		required = append(required, "extract_doc_metadata")
	} else if err := validateRequiredProcessors(required); err != nil {
		return nil, err
	}
	components := buildProductionRuntimeComponents(logger)
	fixed := components.fixed
	if fixed == nil {
		return nil, fmt.Errorf("production runtime: missing chunking service")
	}
	if err := applyRuntimeOverrides(fixed, overrides); err != nil {
		return nil, err
	}
	if err := validateFixedRuntimeConfig(fixed, required, explicitSelection); err != nil {
		return nil, err
	}
	control := &ControlService{Logger: logger, InputStore: components.inputStore, EventStore: SQLStore{DB: ApiTypes.ProjectDBHandle}, RunStore: SQLStore{DB: ApiTypes.ProjectDBHandle}, StopStore: StopRequestSQLStore{DB: ApiTypes.ProjectDBHandle}, Now: time.Now, MaxDocProcessPipelines: MaxDocProcessPipelinesFromEnv(), BlockingProcessor: components.blocking, Processors: filterProcessors(components.processors, required)}
	for _, p := range control.Processors {
		if err := processorInitError(p); err != nil {
			return nil, err
		}
	}
	for _, p := range control.Processors {
		if m, ok := p.(*MetricsProcessor); ok {
			if err := applyMetricsRuntimeOverrides(m, overrides); err != nil {
				return nil, err
			}
			if err := processorInitError(m); err != nil {
				return nil, err
			}
		}
	}
	r := &ProductionRuntime{Control: control, Processors: control.Processors, services: map[string]any{"chunking": fixed.RuntimeConfig()}}
	for _, p := range control.Processors {
		if m, ok := p.(*MetricsProcessor); ok {
			r.services["extract_metrics"] = m.RuntimeConfig()
		}
	}
	r.config = makeResolvedConfig(control, r.services)
	return r, nil
}

func validateFixedRuntimeConfig(fixed *FixedSizeChunkingService, required []string, explicit bool) error {
	if fixed == nil {
		return fmt.Errorf("production runtime: missing chunking service")
	}
	var checks []error
	if !explicit {
		checks = []error{fixed.PromptErr, fixed.ModelErr, fixed.SummaryPromptErr, fixed.SummaryModelErr, fixed.TranslationModelErr, fixed.FallbackModelErr}
	} else {
		enabled := map[string]bool{}
		for _, name := range resolveRequiredProcessors(required) {
			enabled[name] = true
		}
		if enabled["generate_topics"] {
			checks = append(checks, fixed.PromptErr, fixed.ModelErr, fixed.TranslationModelErr, fixed.FallbackModelErr)
		}
		if enabled["generate_summaries"] {
			checks = append(checks, fixed.SummaryPromptErr, fixed.SummaryModelErr, fixed.TranslationModelErr)
		}
	}
	for _, err := range checks {
		if err != nil {
			return err
		}
	}
	return nil
}

func processorInitError(p Processor) error {
	switch x := p.(type) {
	case *MetricsProcessor:
		for _, e := range []error{x.MentionPromptErr, x.RelationPromptErr, x.MergeResolvePromptErr, x.MentionModelErr, x.RelationModelErr, x.MergeResolveModelErr, x.FallbackMentionModelErr, x.FallbackMergeResolveModelErr} {
			if e != nil {
				return e
			}
		}
	}
	return nil
}

func applyMetricsRuntimeOverrides(p *MetricsProcessor, o map[string]string) error {
	for k, v := range o {
		switch strings.ToUpper(k) {
		case "EXTRACT_METRIC_CANDIDATES_PROMPT":
			p.MentionPromptText, p.MentionPromptRef, p.MentionPromptPath, p.MentionPromptErr = loadPromptByRef(v)
		case "ENRICH_METRICS_PROMPT", "EXTRACT_METRICS_PROMPT":
			p.RelationPromptText, p.RelationPromptRef, p.RelationPromptPath, p.RelationPromptErr = loadPromptByRef(v)
			p.PromptText, p.PromptRef, p.PromptPath, p.PromptErr = p.RelationPromptText, p.RelationPromptRef, p.RelationPromptPath, p.RelationPromptErr
		case "METRIC_MERGE_RESOLVE_PROMPT":
			p.MergeResolvePromptText, p.MergeResolvePromptRef, p.MergeResolvePromptPath, p.MergeResolvePromptErr = loadPromptByRef(v)
		case "EXTRACT_METRIC_CANDIDATES_MODEL_NAME":
			p.MentionModelRef, p.MentionModelCfgPath, p.MentionModelCfg, p.MentionModelErr = loadModelConfigByRef(v, "MODEL_DEF_FILE")
			p.MentionModelName = p.MentionModelCfg.ModelName
		case "ENRICH_METRICS_MODEL_NAME", "EXTRACT_METRICS_MODEL_NAME":
			p.RelationModelRef, p.RelationModelCfgPath, p.RelationModelCfg, p.RelationModelErr = loadModelConfigByRef(v, "MODEL_DEF_FILE")
			p.RelationModelName = p.RelationModelCfg.ModelName
			p.ModelRef, p.ModelCfgPath, p.ModelName, p.ModelErr = p.RelationModelRef, p.RelationModelCfgPath, p.RelationModelName, p.RelationModelErr
		case "METRIC_MERGE_RESOLVE_MODEL_NAME":
			p.MergeResolveModelRef, p.MergeResolveModelCfgPath, p.MergeResolveModelCfg, p.MergeResolveModelErr = loadModelConfigByRef(v, "MODEL_DEF_FILE")
			p.MergeResolveModelName = p.MergeResolveModelCfg.ModelName
		case "EXTRACT_METRIC_CANDIDATES_MODEL_FALLBACK":
			p.FallbackMentionModelRef, p.FallbackMentionModelCfgPath, p.FallbackMentionModelCfg, p.FallbackMentionModelErr = loadModelConfigByRef(v, "MODEL_DEF_FILE")
			p.FallbackMentionModelName = p.FallbackMentionModelCfg.ModelName
		case "METRIC_MERGE_RESOLVE_MODEL_FALLBACK":
			p.FallbackMergeResolveModelRef, p.FallbackMergeResolveModelCfgPath, p.FallbackMergeResolveModelCfg, p.FallbackMergeResolveModelErr = loadModelConfigByRef(v, "MODEL_DEF_FILE")
			p.FallbackMergeResolveModelName = p.FallbackMergeResolveModelCfg.ModelName
		case "METRIC_ENRICH_GROUP_SIZE":
			n, e := strconv.Atoi(v)
			if e != nil || n < 1 {
				return fmt.Errorf("invalid METRIC_ENRICH_GROUP_SIZE")
			}
			p.MetricEnrichGroupSize = n
		case "EXTRACT_METRICS_MAX_TASKS":
			n, e := strconv.Atoi(v)
			if e != nil || n < 1 {
				return fmt.Errorf("invalid EXTRACT_METRICS_MAX_TASKS")
			}
			p.MaxTasks = n
		case "CHUNK_SIZE", "CHUNK_OVERLAP_PERCENT":
		default:
			continue
		}
	}
	return nil
}

func applyRuntimeOverrides(f *FixedSizeChunkingService, o map[string]string) error {
	for k, v := range o {
		switch strings.ToUpper(k) {
		case "CHUNK_SIZE":
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 {
				return fmt.Errorf("invalid CHUNK_SIZE")
			}
			f.ChunkSize = n
		case "CHUNK_OVERLAP_PERCENT":
			n, err := strconv.Atoi(v)
			if err != nil || n < 0 {
				return fmt.Errorf("invalid CHUNK_OVERLAP_PERCENT")
			}
			f.OverlapPercent = n
		case "EXTRACT_METRIC_CANDIDATES_PROMPT", "ENRICH_METRICS_PROMPT", "EXTRACT_METRICS_PROMPT", "METRIC_MERGE_RESOLVE_PROMPT", "EXTRACT_METRIC_CANDIDATES_MODEL_NAME", "ENRICH_METRICS_MODEL_NAME", "EXTRACT_METRICS_MODEL_NAME", "METRIC_MERGE_RESOLVE_MODEL_NAME", "EXTRACT_METRIC_CANDIDATES_MODEL_FALLBACK", "METRIC_MERGE_RESOLVE_MODEL_FALLBACK", "METRIC_ENRICH_GROUP_SIZE", "EXTRACT_METRICS_MAX_TASKS":
			// Applied after the metrics processor is initialized.
		default:
			return fmt.Errorf("unsupported runtime override %q", k)
		}
	}
	return nil
}

func configuredNames() []string { return viper.GetStringSlice("doc-processing.required_processors") }

func resolveRequiredProcessors(requested []string) []string {
	wanted := map[string]bool{"static_analyzer": true, "chunking": true}
	for _, name := range requested {
		wanted[normalizeRuntimeName(name)] = true
	}
	productionOrder := []string{"static_analyzer", "chunking", "generate_summaries", "generate_topics", "extract_doc_metadata", "extract_semantic_projections", "extract_structured_knowledge", "extract_entity", "extract_relation", "extract_inventory_items", "extract_metrics", "extract_provisions", "generate_scene_blocks"}
	out := make([]string, 0, len(wanted))
	for _, name := range productionOrder {
		if wanted[name] {
			out = append(out, name)
		}
	}
	return out
}

func validateRequiredProcessors(requested []string) error {
	known := map[string]bool{}
	for _, name := range resolveRequiredProcessors(nil) {
		known[name] = true
	}
	for _, name := range []string{"generate_summaries", "generate_topics", "extract_doc_metadata", "extract_semantic_projections", "extract_structured_knowledge", "extract_entity", "extract_relation", "extract_inventory_items", "extract_metrics", "extract_provisions", "generate_scene_blocks"} {
		known[name] = true
	}
	for _, raw := range requested {
		name := normalizeRuntimeName(raw)
		if !known[name] {
			return fmt.Errorf("unknown required processor %q", raw)
		}
	}
	return nil
}

func filterProcessors(processors []Processor, required []string) []Processor {
	enabled := map[string]bool{}
	for _, n := range resolveRequiredProcessors(required) {
		enabled[normalizeRuntimeName(n)] = true
	}
	out := make([]Processor, 0, len(processors))
	for _, p := range processors {
		if p != nil && enabled[normalizeRuntimeName(p.Name())] {
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
	if r == nil || r.Control == nil {
		return ResolvedConfigSnapshot{}
	}
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
		for name, raw := range services {
			if cfg, ok := raw.(map[string]any); ok {
				if name == "chunking" {
					if n, ok := cfg["chunk_size"].(int); ok {
						v["chunk_size"] = n
					}
					if n, ok := cfg["overlap_percent"].(int); ok {
						v["chunk_overlap_percent"] = n
					}
				}
				for k, x := range cfg {
					if strings.Contains(k, "model") {
						v["model_references"].(map[string]any)[name+"."+k] = x
					}
					if strings.Contains(k, "prompt") {
						if m, ok := x.(map[string]any); ok {
							if h, ok := m["content_sha256"].(string); ok && h != "" {
								v["prompt_hashes"].(map[string]string)[name+"."+k] = h
							}
						}
					}
				}
			}
		}
	}
	if c != nil {
		v["max_doc_process_pipelines"] = c.MaxDocProcessPipelines
	}
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return ResolvedConfigSnapshot{Values: v, CanonicalJSON: b, Hash: hex.EncodeToString(h[:])}
}
