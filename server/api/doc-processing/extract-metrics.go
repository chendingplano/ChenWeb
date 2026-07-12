package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/ApiUtils"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

type MetricsProcessor struct {
	InputStore                       DocMetadataStore
	Store                            MetricsStore
	ObjectStore                      ArtifactObjectsStore
	ObjectReconciler                 ObjectReconciler
	AmbiguousObjectLLMResolver       AmbiguousObjectLLMResolver
	ResolveAmbiguousMinConfidence    float64
	Extractor                        LLMJSONExtractor
	Logger                           ApiTypes.JimoLogger
	ProcLogger                       DocProcLogger
	Now                              func() time.Time
	MentionPromptText                string
	MentionPromptRef                 string
	MentionPromptPath                string
	MentionPromptErr                 error
	MentionModelRef                  string
	MentionModelCfgPath              string
	MentionModelErr                  error
	MentionModelName                 string
	MentionModelCfg                  structureModelConfig
	FallbackMentionModelRef          string
	FallbackMentionModelCfgPath      string
	FallbackMentionModelErr          error
	FallbackMentionModelName         string
	FallbackMentionModelCfg          structureModelConfig
	MergeResolvePromptText           string
	MergeResolvePromptRef            string
	MergeResolvePromptPath           string
	MergeResolvePromptErr            error
	MergeResolveModelRef             string
	MergeResolveModelCfgPath         string
	MergeResolveModelErr             error
	MergeResolveModelName            string
	MergeResolveModelCfg             structureModelConfig
	FallbackMergeResolveModelRef     string
	FallbackMergeResolveModelCfgPath string
	FallbackMergeResolveModelErr     error
	FallbackMergeResolveModelName    string
	FallbackMergeResolveModelCfg     structureModelConfig
	RelationPromptText               string
	RelationPromptRef                string
	RelationPromptPath               string
	RelationPromptErr                error
	RelationModelRef                 string
	RelationModelCfgPath             string
	RelationModelErr                 error
	RelationModelName                string
	RelationModelCfg                 structureModelConfig
	PromptText                       string
	PromptRef                        string
	PromptPath                       string
	PromptErr                        error
	ModelRef                         string
	ModelCfgPath                     string
	ModelErr                         error
	ModelName                        string
	ChunkDir                         string
	MetricEnrichGroupSize            int
	MaxTasks                         int

	// batch state (set by ChunkBatchProcessor.InitChunkBatch)
	batchRecordID  int64
	batchChunks    []Chunk
	batchDocCtx    string
	batchStart     time.Time
	batchMentions  []metricCandidateMention // Pass 1 accumulator (converted to candidates in Finalize)
	batchMu        sync.Mutex               // protects batchMentions/batchLang/batchModelName under concurrent Phase 3
	batchLang      string
	batchModelName string

	batchSkip            bool
	batchForceClear      bool
	batchExistingMetrics []map[string]any
}

type metricsProgressTracker struct {
	mu        sync.Mutex
	Total     int
	Completed int
}

func (t *metricsProgressTracker) advance() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.Completed < t.Total {
		t.Completed++
	}
	pct := 0
	if t.Total > 0 {
		pct = t.Completed * 100 / t.Total
	}
	return fmt.Sprintf("%d%% (%d/%d)", pct, t.Completed, t.Total)
}

type MetricsStore interface {
	MetricsExist(ctx context.Context, inputRecordID int64) (bool, error)
	DeleteMetricsByInputRecordID(ctx context.Context, inputRecordID int64) (int64, error)
	SaveMetrics(ctx context.Context, req SaveMetricsRequest) (int64, error)
	GetMetricsByInputRecordID(ctx context.Context, inputRecordID int64) ([]map[string]any, error)
	UpsertMetrics(ctx context.Context, req SaveMetricsRequest) (int64, error)
}

type ArtifactObjectsStore interface {
	ReplaceObjectsForRecord(ctx context.Context, recordID int64, artifactType string, objects []ArtifactObject) error
}

type MetricsSQLStore struct {
	DB *sql.DB
}

type SaveMetricsRequest struct {
	InputRecordID int64
	EventID       string
	Language      string
	ModelName     string
	PromptName    string
	Metrics       []map[string]any
}

type metricExtractionResult struct {
	Language         string
	Candidates       int
	Metrics          []map[string]any
	UncertainMetrics []map[string]any
	ModelName        string
	FallbackCount    int
	LLMCallCount     int
}

type metricCandidateMention struct {
	CandidateID       string
	MetricNameHint    string
	SubjectHint       string
	ObjectHint        string
	ObjectTypeHint    string
	ObjectRoleHint    string
	EvidenceQuote     string
	SourceLineSpans   []string
	UnitHint          string
	ValueHint         string
	Confidence        float64
	ConfidenceReason  string
	ChunkIndex        int
	ChunkLines        []BlockLine
	HasNormalEvidence bool
}

type metricCandidate struct {
	CandidateID        string
	ChunkIndex         int
	MetricNameHint     string
	SubjectHint        string
	ObjectHint         string
	ObjectTypeHint     string
	ObjectRoleHint     string
	UnitHint           string
	ValueHint          string
	SupportingMentions []map[string]any
	SupportLines       []BlockLine
}

type candidateBatch struct {
	chunkIdx   int
	candidates []metricCandidate
}

// metricsPass1Result is the per-chunk candidate-extraction output, promoted from
// the local type in extractMetricsFromChunksWithLLM so batch methods can share it.
type metricsPass1Result struct {
	mentions    []metricCandidateMention
	language    string
	modelName   string
	didFallback bool
}

func groupCandidatesByChunk(candidates []metricCandidate, maxGroupSize int) []candidateBatch {
	if maxGroupSize <= 0 {
		maxGroupSize = 5
	}
	var batches []candidateBatch
	for i := 0; i < len(candidates); {
		chunkIdx := candidates[i].ChunkIndex
		j := i
		for j < len(candidates) && candidates[j].ChunkIndex == chunkIdx && j-i < maxGroupSize {
			j++
		}
		batches = append(batches, candidateBatch{chunkIdx: chunkIdx, candidates: candidates[i:j]})
		i = j
	}
	return batches
}

func NewMetricsProcessor(inputStore DocMetadataStore, store MetricsStore, extractor LLMJSONExtractor, _ ApiTypes.JimoLogger) *MetricsProcessor {
	// if logger == nil {
	logger := loggerutil.CreateDefaultLogger("MID_26042470")
	// }
	mentionPromptText, mentionPromptRef, mentionPromptPath, mentionPromptErr := loadProductPromptFromEnvKeys(
		[]string{"EXTRACT_METRIC_CANDIDATES_PROMPT"},
		"prompt-extract-metric-candidates-v5.md",
	)
	relationPromptText, relationPromptRef, relationPromptPath, relationPromptErr := loadProductPromptFromEnvKeys(
		[]string{"ENRICH_METRICS_PROMPT", "EXTRACT_METRICS_PROMPT", "PROMPT_FILE_NAME"},
		"prompt-enrich-metrics-v3.md",
	)
	mentionModelRef, mentionModelCfgPath, mentionModelCfg, mentionModelErr := loadModelConfigFromEnvKeys(
		[]string{"EXTRACT_METRIC_CANDIDATES_MODEL_NAME", "EXTRACT_METRICS_MODEL_NAME"},
		"MODEL_DEF_FILE",
	)
	fallbackMentionModelRef, fallbackMentionModelCfgPath, fallbackMentionModelCfg, fallbackMentionModelErr := loadOptionalModelConfigFromEnv(
		"EXTRACT_METRIC_CANDIDATES_MODEL_FALLBACK",
		"MODEL_DEF_FILE",
	)
	relationModelRef, relationModelCfgPath, relationModelCfg, relationModelErr := loadModelConfigFromEnvKeys(
		[]string{"ENRICH_METRICS_MODEL_NAME", "EXTRACT_METRICS_MODEL_NAME"},
		"MODEL_DEF_FILE",
	)
	mergeResolvePromptText, mergeResolvePromptRef, mergeResolvePromptPath, mergeResolvePromptErr := loadProductPromptFromEnvKeys(
		[]string{"METRIC_MERGE_RESOLVE_PROMPT"},
		"prompt-merge-resolve-metrics-v1.md",
	)
	mergeResolveModelRef, mergeResolveModelCfgPath, mergeResolveModelCfg, mergeResolveModelErr := loadModelConfigFromEnvKeys(
		[]string{"METRIC_MERGE_RESOLVE_MODEL_NAME"},
		"MODEL_DEF_FILE",
	)
	fallbackMergeResolveModelRef, fallbackMergeResolveModelCfgPath, fallbackMergeResolveModelCfg, fallbackMergeResolveModelErr := loadOptionalModelConfigFromEnv(
		"METRIC_MERGE_RESOLVE_MODEL_FALLBACK",
		"MODEL_DEF_FILE",
	)
	enrichGroupSize := 5
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv("METRIC_ENRICH_GROUP_SIZE"))); err == nil && v > 0 {
		enrichGroupSize = v
	}
	maxTasks := envInt("EXTRACT_METRICS_MAX_TASKS", 1, 1)
	applyStructureModelConfigToExtractor(extractor, relationModelCfg)
	p := &MetricsProcessor{
		InputStore:                  inputStore,
		Store:                       store,
		Extractor:                   extractor,
		Logger:                      logger,
		ProcLogger:                  DocProcLogger{DB: ApiTypes.ProjectDBHandle},
		Now:                         time.Now,
		MentionPromptText:           mentionPromptText,
		MentionPromptRef:            mentionPromptRef,
		MentionPromptPath:           mentionPromptPath,
		MentionPromptErr:            mentionPromptErr,
		MentionModelRef:             mentionModelRef,
		MentionModelCfgPath:         mentionModelCfgPath,
		MentionModelErr:             mentionModelErr,
		MentionModelName:            mentionModelCfg.ModelName,
		MentionModelCfg:             mentionModelCfg,
		FallbackMentionModelRef:     fallbackMentionModelRef,
		FallbackMentionModelCfgPath: fallbackMentionModelCfgPath,
		FallbackMentionModelErr:     fallbackMentionModelErr,
		FallbackMentionModelName:    fallbackMentionModelCfg.ModelName,
		FallbackMentionModelCfg:     fallbackMentionModelCfg,

		MergeResolvePromptText:           mergeResolvePromptText,
		MergeResolvePromptRef:            mergeResolvePromptRef,
		MergeResolvePromptPath:           mergeResolvePromptPath,
		MergeResolvePromptErr:            mergeResolvePromptErr,
		MergeResolveModelRef:             mergeResolveModelRef,
		MergeResolveModelCfgPath:         mergeResolveModelCfgPath,
		MergeResolveModelErr:             mergeResolveModelErr,
		MergeResolveModelName:            mergeResolveModelCfg.ModelName,
		MergeResolveModelCfg:             mergeResolveModelCfg,
		FallbackMergeResolveModelRef:     fallbackMergeResolveModelRef,
		FallbackMergeResolveModelCfgPath: fallbackMergeResolveModelCfgPath,
		FallbackMergeResolveModelErr:     fallbackMergeResolveModelErr,
		FallbackMergeResolveModelName:    fallbackMergeResolveModelCfg.ModelName,
		FallbackMergeResolveModelCfg:     fallbackMergeResolveModelCfg,

		RelationPromptText:    relationPromptText,
		RelationPromptRef:     relationPromptRef,
		RelationPromptPath:    relationPromptPath,
		RelationPromptErr:     relationPromptErr,
		RelationModelRef:      relationModelRef,
		RelationModelCfgPath:  relationModelCfgPath,
		RelationModelErr:      relationModelErr,
		RelationModelName:     relationModelCfg.ModelName,
		RelationModelCfg:      relationModelCfg,
		PromptText:            relationPromptText,
		PromptRef:             relationPromptRef,
		PromptPath:            relationPromptPath,
		PromptErr:             relationPromptErr,
		ModelRef:              relationModelRef,
		ModelCfgPath:          relationModelCfgPath,
		ModelErr:              relationModelErr,
		ModelName:             relationModelCfg.ModelName,
		ChunkDir:              strings.TrimSpace(os.Getenv("ARTIFACT_DIR")),
		MetricEnrichGroupSize: enrichGroupSize,
		MaxTasks:              maxTasks,
	}
	if resolver, minConfidence, err := NewAmbiguousObjectLLMResolverFromEnv(); err == nil {
		p.AmbiguousObjectLLMResolver = resolver
		p.ResolveAmbiguousMinConfidence = minConfidence
	} else if logger != nil {
		logger.Warn("configure ambiguous object LLM resolver failed", "err", err)
	}
	if ApiTypes.ProjectDBHandle != nil {
		p.ObjectStore = ArtifactObjectSQLStore{DB: ApiTypes.ProjectDBHandle}
		p.ObjectReconciler = ObjectReconciler{
			Store:   ObjectNodeSQLStore{DB: ApiTypes.ProjectDBHandle},
			Options: ObjectReconcileOptionsFromEnv(),
		}
	}
	p.forceDisableThinking()
	applyStructureModelConfigToExtractor(extractor, p.RelationModelCfg)
	return p
}

func (p *MetricsProcessor) Name() string { return "extract_metrics" }

func (p *MetricsProcessor) HandleEvent(ctx context.Context, payload []byte) error {
	p.forceDisableThinking()
	start := p.Now()
	evt, err := ParseLineFileGeneratedEvent(payload)
	if err != nil {
		p.Logger.Error("parse input file failed", "error", err)
		return fmt.Errorf("(MID_26042457) parse event payload: %w", err)
	}
	ctx = withLLMRecordID(ctx, evt.RecordID)
	if ShouldSkipLineFileGeneratedEvent(evt) {
		p.Logger.Warn("processor skipped")
		return nil
	}
	if p.MentionPromptErr != nil {
		p.Logger.Error("prompt error", "error", p.MentionPromptErr)
		return fmt.Errorf("(MID_26042458) load metric candidates prompt %q: %w", p.MentionPromptRef, p.MentionPromptErr)
	}
	if p.RelationPromptErr != nil {
		p.Logger.Error("prompt error", "error", p.RelationPromptErr)
		return fmt.Errorf("(MID_26042458) load metrics prompt %q: %w", p.RelationPromptRef, p.RelationPromptErr)
	}

	rec, err := p.InputStore.GetInputRecord(ctx, evt.RecordID)
	if err != nil {
		if err == sql.ErrNoRows {
			p.Logger.Error("kb.inputs record not found", "record_id", evt.RecordID)
			return nil
		}
		p.Logger.Error("retrieve record failed", "record_id", evt.RecordID, "error", err)
		return fmt.Errorf("(MID_26042459) load kb.inputs record %d: %w", evt.RecordID, err)
	}
	if p.MentionModelErr != nil {
		p.Logger.Warn("metrics extraction skipped: mention model config error",
			"record_id", evt.RecordID, "model_ref", p.MentionModelRef, "error", p.MentionModelErr)
		p.persistMetricsStatus(ctx, rec, start, p.MentionModelErr)
		return nil
	}
	if p.RelationModelErr != nil {
		p.Logger.Warn("metrics extraction skipped: relation model config error",
			"record_id", evt.RecordID, "model_ref", p.RelationModelRef, "error", p.RelationModelErr)
		p.persistMetricsStatus(ctx, rec, start, p.RelationModelErr)
		return nil
	}

	lineFilePath, lineFileErr := ResolveInputFilePath(evt, rec.ResultFilename, rec.ParserName, rec.StagingFilename)
	if lineFileErr != nil {
		p.Logger.Error("resolve input file path failed", "record_id", evt.RecordID, "error", lineFileErr)
		p.persistMetricsStatus(ctx, rec, start, fmt.Errorf("(MID_26042466) resolve line file for record_id=%d: %w", evt.RecordID, lineFileErr))
		return nil
	}

	if evt.Force {
		_, _ = p.Store.DeleteMetricsByInputRecordID(ctx, evt.RecordID)
	} else {
		exists, err := p.Store.MetricsExist(ctx, evt.RecordID)
		if err != nil {
			p.Logger.Error("check metrics exist failed", "record_id", evt.RecordID, "error", err)
			p.persistMetricsStatus(ctx, rec, start, err)
			return fmt.Errorf("(MID_26050707) failed checking metrics, error:%w, record_id:%d", err, evt.RecordID)
		}
		if exists {
			p.Logger.Info("metrics extraction skipped", "record_id", evt.RecordID, "reason", "metrics already exist and force=false")
			// Indexing (search registry + connections) is deferred to Phase C
			// (PostProcessIndex), which reindexes existing metrics too.
			p.persistMetricsStatus(ctx, rec, start, nil)
			return nil
		}
	}

	// Load the source line file so chunk line numbers can be resolved.
	lineBody, readErr := os.ReadFile(lineFilePath)
	if readErr != nil {
		p.Logger.Error("read line file failed", "record_id", evt.RecordID, "path", lineFilePath, "error", readErr)
		procErr := fmt.Errorf("(MID_26042466) read line file for record_id=%d: %w", evt.RecordID, readErr)
		p.persistMetricsStatus(ctx, rec, start, procErr)
		return procErr
	}
	lines, parseErr := ParseInputLines(lineBody)
	if parseErr != nil {
		p.Logger.Error("parse line file failed", "record_id", evt.RecordID, "error", parseErr)
		procErr := fmt.Errorf("(MID_26042467) parse line file for record_id=%d: %w", evt.RecordID, parseErr)
		p.persistMetricsStatus(ctx, rec, start, procErr)
		return procErr
	}

	// Load chunks from the persisted .chunks artifact; chunking must have run first.
	artifactBase := buildChunkArtifactBaseName(rec.StagingFilename, rec.ParserName)
	chunks, chunksErr := loadChunksFromArtifactFile(p.ChunkDir, evt.RecordID, artifactBase+".chunks", lines)
	if chunksErr != nil {
		p.Logger.Error("load chunks file failed", "record_id", evt.RecordID, "error", chunksErr)
		procErr := fmt.Errorf("(MID_26042468) load chunks for record_id=%d: %w", evt.RecordID, chunksErr)
		p.persistMetricsStatus(ctx, rec, start, procErr)
		return procErr
	}
	inputChunks := chunksToBlocks(chunks)
	if len(inputChunks) == 0 {
		procErr := fmt.Errorf("(MID_26042460) no chunks found for record_id=%d", evt.RecordID)
		p.Logger.Error("no chunks", "record_id", evt.RecordID, "chunks_file", artifactBase+".chunks")
		p.persistMetricsStatus(ctx, rec, start, procErr)
		return procErr
	}

	p.persistMetricsInProgressStatus(ctx, rec, start, fmt.Sprintf("0%% (0/%d)", len(inputChunks)))
	result, err := p.extractMetricsFromChunksWithLLM(ctx, evt.RecordID, chunks, buildDocContextLine(rec))
	if err != nil {
		if errors.Is(err, ErrPipelineStopped) || isCtxStopped(ctx) {
			p.stopAndPersistMetrics(context.Background(), rec, start)
			return ErrPipelineStopped
		}
		p.persistMetricsStatus(ctx, rec, start, err)
		return fmt.Errorf("(MID_26050701) extractMetrics failed, error:%w", err)
	}
	allMetrics := result.Metrics
	detectedLanguage := result.Language
	if detectedLanguage == "" {
		detectedLanguage = "unknown"
	}
	for i, m := range allMetrics {
		m["metric_id"] = fmt.Sprintf("%d_mtc_%d", evt.RecordID, i+1)
		allMetrics[i] = m
	}

	inserted, err := p.Store.SaveMetrics(ctx, SaveMetricsRequest{
		InputRecordID: evt.RecordID,
		EventID:       eventIDFromContext(ctx),
		Language:      detectedLanguage,
		ModelName:     firstNonEmptyTrimmed(result.ModelName, p.ModelName),
		PromptName:    p.RelationPromptRef,
		Metrics:       allMetrics,
	})
	if err != nil {
		p.persistMetricsStatus(ctx, rec, start, err)
		return fmt.Errorf("(MID_26050703) insert records failed, error:%w", err)
	}
	p.logFinalMetricsSaved(ctx, evt.RecordID, detectedLanguage,
		[]string{firstNonEmptyTrimmed(result.ModelName, p.ModelName)}, p.RelationPromptRef,
		allMetrics, start, p.Now())

	if err := p.persistMetricObjects(ctx, evt.RecordID, allMetrics); err != nil {
		p.persistMetricsStatus(ctx, rec, start, err)
		return fmt.Errorf("(MID_26050704) persist metric objects failed, error:%w", err)
	}

	if fileErr := p.saveMetricsToFile(evt.RecordID, rec, allMetrics); fileErr != nil {
		p.Logger.Warn("save metrics to file failed", "record_id", evt.RecordID, "error", fileErr)
	}

	// Artifact indexing (search registry, line-overlap connections, connected_artifacts,
	// category_name links, category-path metrics.txt, and hybrid_search links) is
	// intentionally NOT done here. It runs in Phase C (post-process) via
	// PostProcessIndex, after every doc processor finishes, because metric indexing reads
	// other processors' artifacts (semantic_projections, topics, scenes, provisions,
	// entities, inventory_items) that may not exist yet during Phase B.

	p.Logger.Info("metrics extracted",
		"record_id", evt.RecordID,
		"inserted_rows", inserted,
		"metrics_count", len(allMetrics),
		"uncertain_metrics_count", len(result.UncertainMetrics),
		"num_chunks", len(inputChunks),
	)
	p.persistMetricsStatus(ctx, rec, start, nil)
	p.logMetricsSummary(ctx, evt.RecordID, start, p.Now(), result,
		int64(result.Candidates), inserted, len(inputChunks))
	return nil
}

// PostProcessIndex runs the metrics indexing workflow in Phase C, after all doc
// processors have finished. It is invoked by the pipeline controller for every record
// whose pipeline included extract_metrics. Running here (rather than inside HandleEvent)
// guarantees that the artifacts metric indexing depends on — semantic_projections,
// topics, scenes, provisions, entities, inventory_items, and their kb.search_artifacts
// rows — are already present, regardless of Phase B ordering. It is idempotent and safe
// to run for both freshly-extracted and pre-existing (skipped) metrics.
func (p *MetricsProcessor) PostProcessIndex(ctx context.Context, recordID int64) error {
	rec, err := p.InputStore.GetInputRecord(ctx, recordID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return fmt.Errorf("(MID_26060501) load kb.inputs record %d: %w", recordID, err)
	}

	exists, err := p.Store.MetricsExist(ctx, recordID)
	if err != nil {
		return fmt.Errorf("(MID_26060502) check metrics exist for record %d: %w", recordID, err)
	}
	if !exists {
		return nil
	}

	// Chunks back connected_artifacts.chunks and the has-metrics line-overlap edges. If
	// they cannot be loaded, indexing still proceeds for the remaining outputs.
	chunks, chunkErr := p.loadRecordChunks(rec, recordID)
	if chunkErr != nil {
		p.Logger.Warn("post-process metrics: load chunks failed; indexing without chunk overlap",
			"record_id", recordID, "error", chunkErr)
	}

	if reindexErr := ReindexMetricSearchForRecord(ctx, recordID, p.Logger); reindexErr != nil {
		p.Logger.Warn("reindex metric search registry failed", "record_id", recordID, "error", reindexErr)
	}
	if connErr := WriteLineOverlapConnectionsFromRegistry(ctx, recordID, searchArtifactMetric, RelationHasMetrics, chunks); connErr != nil {
		p.Logger.Warn("write has-metrics connections failed", "record_id", recordID, "error", connErr)
	}
	IndexMetricsForRecord(ctx, recordID, p.RelationModelName, p.RelationPromptRef, chunks, p.Logger)
	if refreshErr := refreshMetricArtifactFile(ctx, ApiTypes.ProjectDBHandle, p.ChunkDir, recordID, rec); refreshErr != nil {
		p.Logger.Warn("refresh metric artifact failed", "record_id", recordID, "error", refreshErr)
	}
	return nil
}

func (p *MetricsProcessor) persistMetricObjects(ctx context.Context, recordID int64, metrics []map[string]any) error {
	if p.ObjectStore == nil {
		return nil
	}
	objects := make([]ArtifactObject, 0)
	for _, metric := range metrics {
		metricID := strings.TrimSpace(asString(metric["metric_id"]))
		if metricID == "" {
			continue
		}
		objects = append(objects, normalizeArtifactObjectsForArtifact(recordID, searchArtifactMetric, metricID, metric)...)
	}
	if p.ObjectReconciler.Store != nil && len(objects) > 0 {
		reconciled, err := reconcileArtifactObjectsWithLLM(ctx, objects, p.ObjectReconciler, p.Logger, p.AmbiguousObjectLLMResolver, p.ResolveAmbiguousMinConfidence, objectReconcileLogSink{ProcLogger: p.ProcLogger, DocProcName: p.Name(), CallReason: p.Name()})
		if err != nil {
			return err
		}
		objects = reconciled
	}
	return p.ObjectStore.ReplaceObjectsForRecord(ctx, recordID, searchArtifactMetric, objects)
}

// loadRecordChunks resolves and loads the record's persisted chunk blocks from the
// .chunks artifact, mirroring HandleEvent's loader but without status side effects so it
// is safe to call from the Phase C post-process step.
func (p *MetricsProcessor) loadRecordChunks(rec DocMetadataInputRecord, recordID int64) ([]Block, error) {
	lineFilePath, err := ResolveInputFilePath(LineFileGeneratedEvent{RecordID: recordID}, rec.ResultFilename, rec.ParserName, rec.StagingFilename)
	if err != nil {
		return nil, fmt.Errorf("resolve line file: %w", err)
	}
	lineBody, err := os.ReadFile(lineFilePath)
	if err != nil {
		return nil, fmt.Errorf("read line file: %w", err)
	}
	lines, err := ParseInputLines(lineBody)
	if err != nil {
		return nil, fmt.Errorf("parse line file: %w", err)
	}
	artifactBase := buildChunkArtifactBaseName(rec.StagingFilename, rec.ParserName)
	chunks, err := loadChunksFromArtifactFile(p.ChunkDir, recordID, artifactBase+".chunks", lines)
	if err != nil {
		return nil, fmt.Errorf("load chunks: %w", err)
	}
	return chunksToBlocks(chunks), nil
}

func forceDisableThinking(cfg structureModelConfig) structureModelConfig {
	cfg.ThinkingType = "disabled"
	return cfg
}

func (p *MetricsProcessor) forceDisableThinking() {
	p.MentionModelCfg = forceDisableThinking(p.MentionModelCfg)
	p.FallbackMentionModelCfg = forceDisableThinking(p.FallbackMentionModelCfg)
	p.RelationModelCfg = forceDisableThinking(p.RelationModelCfg)
	p.MergeResolveModelCfg = forceDisableThinking(p.MergeResolveModelCfg)
	p.FallbackMergeResolveModelCfg = forceDisableThinking(p.FallbackMergeResolveModelCfg)
}

func (p *MetricsProcessor) saveMetricsToFile(recordID int64, rec DocMetadataInputRecord, metrics []map[string]any) error {
	if len(metrics) == 0 {
		return nil
	}
	if strings.TrimSpace(p.ChunkDir) == "" {
		return fmt.Errorf("(MID_26042480) missing ARTIFACT_DIR")
	}

	stagingBase := filepath.Base(strings.TrimSpace(rec.StagingFilename))
	filenameRoot := strings.TrimSuffix(stagingBase, filepath.Ext(stagingBase))
	if filenameRoot == "" {
		return fmt.Errorf("(MID_26042481) cannot determine filename root from staging_filename %q", rec.StagingFilename)
	}

	parserName := strings.TrimSpace(rec.ParserName)
	if parserName == "" {
		return fmt.Errorf("(MID_26042482) parser_name is empty for record_id=%d", recordID)
	}

	groupID := recordID / 1000
	dir := filepath.Join(p.ChunkDir, strconv.FormatInt(groupID, 10), strconv.FormatInt(recordID, 10))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("(MID_26042483) create directory %s: %w", dir, err)
	}

	path := filepath.Join(dir, filenameRoot+"_"+parserName+".metrics")
	bs, err := json.MarshalIndent(metrics, "", "  ")
	if err != nil {
		return fmt.Errorf("(MID_26042484) marshal metrics: %w", err)
	}
	if err := os.WriteFile(path, bs, 0644); err != nil {
		return fmt.Errorf("(MID_26042485) write metrics file %s: %w", path, err)
	}
	return nil
}

type metricsStatusParams struct {
	RecordID      int64
	FileType      string
	InputFilename string
	Start         time.Time
	DurationMs    int64
	ProcStatus    string
	Progress      string
	ProcErr       error
}

func detectMetricsFileType(rec DocMetadataInputRecord) string {
	for _, candidate := range []string{rec.FileName, rec.StagingFilename, rec.ResultFilename} {
		ext := strings.ToLower(strings.TrimSpace(filepath.Ext(strings.TrimSpace(candidate))))
		if ext != "" {
			return strings.TrimPrefix(ext, ".")
		}
	}
	return ""
}

func (p *MetricsProcessor) persistMetricsInProgressStatus(ctx context.Context, rec DocMetadataInputRecord, start time.Time, progress string) {
	if err := updateInputStatusAtomic(ctx, p.InputStore, rec.ID, func(current string) (DocMetadataUpdate, error) {
		statusRaw, err := appendMetricsStatus(current, metricsStatusParams{
			RecordID:      rec.ID,
			FileType:      detectMetricsFileType(rec),
			InputFilename: strings.TrimSpace(rec.ResultFilename),
			Start:         start,
			DurationMs:    time.Since(start).Milliseconds(),
			ProcStatus:    "in_progress",
			Progress:      progress,
		})
		if err != nil {
			return DocMetadataUpdate{}, err
		}
		return DocMetadataUpdate{StatusRaw: statusRaw}, nil
	}); err != nil {
		p.Logger.Warn("failed persisting metrics in-progress status", "record_id", rec.ID, "error", err)
	}
}

func (p *MetricsProcessor) persistMetricsStatus(ctx context.Context, rec DocMetadataInputRecord, start time.Time, procErr error) {
	errMsg := (*string)(nil)
	if procErr != nil {
		msg := strings.TrimSpace(procErr.Error())
		errMsg = &msg
	}
	if err := updateInputStatusAtomic(ctx, p.InputStore, rec.ID, func(current string) (DocMetadataUpdate, error) {
		statusRaw, err := appendMetricsStatus(current, metricsStatusParams{
			RecordID:      rec.ID,
			FileType:      detectMetricsFileType(rec),
			InputFilename: strings.TrimSpace(rec.ResultFilename),
			Start:         start,
			DurationMs:    time.Since(start).Milliseconds(),
			ProcErr:       procErr,
		})
		if err != nil {
			return DocMetadataUpdate{}, err
		}
		return DocMetadataUpdate{StatusRaw: statusRaw, ErrorMsg: errMsg}, nil
	}); err != nil {
		p.Logger.Error("failed persisting metrics status", "record_id", rec.ID, "error", err)
	}
}

func (p *MetricsProcessor) stopAndPersistMetrics(ctx context.Context, rec DocMetadataInputRecord, start time.Time) {
	if updateErr := updateInputStatusAtomic(ctx, p.InputStore, rec.ID, func(current string) (DocMetadataUpdate, error) {
		statusRaw, err := appendMetricsStatus(current, metricsStatusParams{
			RecordID:      rec.ID,
			FileType:      detectMetricsFileType(rec),
			InputFilename: strings.TrimSpace(rec.ResultFilename),
			Start:         start,
			DurationMs:    time.Since(start).Milliseconds(),
			ProcStatus:    "stopped",
		})
		if err != nil {
			return DocMetadataUpdate{}, err
		}
		return DocMetadataUpdate{StatusRaw: statusRaw}, nil
	}); updateErr != nil {
		p.Logger.Error("(MID_26052842) failed persisting metrics stopped status", "record_id", rec.ID, "error", updateErr)
	}
	p.Logger.Info("(MID_26052843) extract_metrics stopped by user request", "record_id", rec.ID)
}

func appendMetricsStatus(raw string, p metricsStatusParams) (string, error) {
	entries := decodeDocMetaStatus(raw)
	entry := map[string]any{
		"record_id":      strconv.FormatInt(p.RecordID, 10),
		"file_type":      strings.ToLower(strings.TrimSpace(p.FileType)),
		"operation":      "extract_metrics",
		"input_filename": strings.TrimSpace(p.InputFilename),
		"start_time":     p.Start.Format(defaultDocMetaStatusTime),
		"ms_used":        p.DurationMs,
	}
	if progress := strings.TrimSpace(p.Progress); progress != "" {
		entry["progress"] = progress
	}
	if override := strings.TrimSpace(p.ProcStatus); override != "" {
		entry["proc_status"] = override
		if p.ProcErr != nil {
			entry["error"] = strings.TrimSpace(p.ProcErr.Error())
		}
	} else if p.ProcErr == nil {
		entry["proc_status"] = "success"
	} else {
		entry["proc_status"] = "failed"
		entry["error"] = strings.TrimSpace(p.ProcErr.Error())
	}

	replaced := false
	out := make([]map[string]any, 0, len(entries)+1)
	for _, e := range entries {
		op := strings.ToLower(strings.TrimSpace(asString(e["operation"])))
		if op != "extract_metrics" && op != "extract-metrics" {
			out = append(out, e)
			continue
		}
		if !replaced {
			out = append(out, entry)
			replaced = true
		}
	}
	if !replaced {
		out = append(out, entry)
	}
	bs, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(bs), nil
}

/*
func (p *MetricsProcessor) extractMetricsFromLinesWithLLM(ctx context.Context, lines []string) (metricExtractionResult, error) {
	parsedLines := parseMetricInputLines(lines)
	blockLines := make([]BlockLine, 0, len(parsedLines))
	for _, line := range parsedLines {
		blockLines = append(blockLines, BlockLine{
			Flag:       line.Flag,
			LineNumber: line.LineNumber,
			PageNumber: line.PageNumber,
			LineType:   line.LineType,
			Content:    line.Content,
		})
	}
	return p.extractMetricsFromBlocksWithLLM(ctx, []Block{{Index: 1, Lines: blockLines}})
}
*/

func (p *MetricsProcessor) extractMetricsFromChunksWithLLM(
	ctx context.Context,
	record_id int64,
	chunks []Chunk,
	docCtx string) (metricExtractionResult, error) {

	// Convert to blocks for internal output mapping; chunksToBlocks is 1:1 so
	// chunks[i] and blocks[i] correspond to the same logical unit.
	blocks := chunksToBlocks(chunks)

	maxTasks := p.MaxTasks
	if maxTasks <= 0 {
		maxTasks = 1
	}

	// ── Pass 1: concurrent candidate extraction ──────────────────────────────
	tracker := &metricsProgressTracker{Total: len(chunks)}

	p.Logger.Info("extract metric",
		"record_id", record_id,
		"chunks", len(chunks),
		"model name", p.MentionModelName,
		"prompt", p.MentionPromptRef,
	)
	pass1Results, pass1Err := runConcurrent(ctx, maxTasks, len(chunks), func(concCtx context.Context, i int) (metricsPass1Result, error) {
		origChunk := chunks[i]
		block := blocks[i]
		if isCtxStopped(concCtx) {
			return metricsPass1Result{}, ErrPipelineStopped
		}
		// Canonical chunk InputText (shared, cacheable prefix); schema in the prompt.
		// See ADR 2026062701 §Phase 2.3.
		inputText := canonicalChunkInputText(origChunk.Lines, docCtx)
		taskPrompt := metricCandidateTask(p.MentionPromptText)
		callStart := p.Now()
		p.Logger.Info("extract metrics start",
			"record_id", record_id,
			"chunk", i,
			"total", len(chunks),
			"model name", p.MentionModelName,
			"prompt name", p.MentionPromptRef,
		)
		callID := fmt.Sprintf("%s_p1_b%d", eventIDFromContext(ctx), block.Index)
		payload, modelName, err := p.extractMetricCandidatePayloadWithFallback(concCtx, inputText, taskPrompt)
		annotateMetricCandidatePayload(payload, block.Index)
		progress := tracker.advance()
		p.logExtractMetricsChunk(ctx, record_id, callID, block.Index, len(chunks),
			[]string{strings.TrimSpace(modelName)}, p.MentionPromptRef,
			payload, err, callStart, p.Now(), progress)
		if err != nil {
			if isCtxStopped(concCtx) {
				return metricsPass1Result{}, ErrPipelineStopped
			}
			return metricsPass1Result{}, fmt.Errorf("(MID_26042451) extract metric candidates via llm: %w", err)
		}
		lang := ApiUtils.NormalizeLang(asString(payload["language"]))
		if lang == "chinese" || lang == "中文" || lang == "zh-cn" {
			lang = "zh"
		}
		usedModel := strings.TrimSpace(firstNonEmptyTrimmed(modelName, p.MentionModelName))
		didFallback := strings.TrimSpace(modelName) != strings.TrimSpace(p.MentionModelName)
		raw, _ := payload["candidates"].([]any)
		result := metricsPass1Result{
			mentions:    normalizeMetricCandidateMentions(raw, block),
			language:    lang,
			modelName:   usedModel,
			didFallback: didFallback,
		}
		for _, mention := range result.mentions {
			if len(mention.SourceLineSpans) == 0 {
				p.Logger.Error("pass1: candidate has empty source_line_spans",
					"record_id", record_id,
					"chunk_index", block.Index,
					"metric_name_hint", mention.MetricNameHint,
					"evidence_quote", mention.EvidenceQuote,
				)
			}
		}
		cacheHit, cacheMiss := cacheTokenCounts(p.Extractor)
		p.Logger.Info("extract metrics end  ",
			"record_id", record_id,
			"extracted metrics", len(result.mentions),
			"chunk", i,
			"cache_hit", cacheHit,
			"cache_miss", cacheMiss,
			"ms_used", time.Since(callStart).Milliseconds(),
		)

		return result, nil
	})

	if pass1Err != nil {
		if isCtxStopped(ctx) {
			return metricExtractionResult{}, ErrPipelineStopped
		}
		return metricExtractionResult{}, pass1Err
	}

	// Merge pass1 results in chunk-index order (deterministic)
	mentions := make([]metricCandidateMention, 0, len(chunks))
	detectedLanguage := "unknown"
	usedMentionModel := strings.TrimSpace(p.MentionModelName)
	var fallbackCount int
	llmCallCount := len(chunks)

	for _, r := range pass1Results {
		mentions = append(mentions, r.mentions...)
		if r.language != "" && detectedLanguage == "unknown" {
			detectedLanguage = r.language
		}
		usedMentionModel = strings.TrimSpace(firstNonEmptyTrimmed(r.modelName, usedMentionModel, p.MentionModelName))
		if r.didFallback {
			fallbackCount++
		}
	}

	// ── Step 2 (deterministic): convert mentions to candidates ───────────────
	candidates := mentionsAsCandidates(mentions)
	p.Logger.Info("Metric candidates (merge disabled)",
		"record_id", record_id,
		"mentions_count", len(mentions),
		"candidate_count", len(candidates),
		"record_stage", "post_merge",
	)

	// ── Pass 2: concurrent enrichment batches (delegated to enrichMetricCandidates) ──
	metrics, uncertainMetrics, enrichErr := p.enrichMetricCandidates(ctx, record_id, candidates, docCtx)
	if enrichErr != nil {
		if isCtxStopped(ctx) {
			return metricExtractionResult{}, ErrPipelineStopped
		}
		return metricExtractionResult{}, enrichErr
	}

	usedRelationModel := strings.TrimSpace(p.RelationModelName)
	p.Logger.Info("Final metric rows",
		"record_id", record_id,
		"metrics_after_dedup", len(metrics),
		"record_stage", "post_metric_dedup",
	)

	return metricExtractionResult{
		Language:         detectedLanguage,
		Candidates:       len(candidates),
		Metrics:          metrics,
		UncertainMetrics: uncertainMetrics,
		ModelName:        firstNonEmptyTrimmed(usedRelationModel, usedMentionModel, p.RelationModelName, p.ModelName),
		FallbackCount:    fallbackCount,
		LLMCallCount:     llmCallCount,
	}, nil
}

/*
// metricParsedLine mirrors the block format: <flag>\t<line_number>\t<page_number>\t<line_type>\t<content>
type metricParsedLine struct {
	Flag       string `json:"flag"`
	LineNumber int    `json:"line_number"`
	PageNumber int    `json:"page_number"`
	LineType   string `json:"line_type"`
	Content    string `json:"content"`
}

func parseMetricInputLine(line string) (metricParsedLine, bool, error) {
	raw := strings.TrimSpace(line)
	if raw == "" {
		return metricParsedLine{}, false, nil
	}
	// Block format: <flag>\t<line_number>\t<page_number>\t<line_type>\t<content>
	fields := strings.Split(raw, "\t")
	if len(fields) != 5 {
		return metricParsedLine{}, false, fmt.Errorf("expected 5 tab-separated fields, got %d: %q", len(fields), raw)
	}
	flag := strings.TrimSpace(fields[0])
	if flag != "o" && flag != "n" {
		return metricParsedLine{}, false, fmt.Errorf("invalid flag %q (expected 'o' or 'n'): %q", flag, raw)
	}
	lineNo, err := strconv.Atoi(strings.TrimSpace(fields[1]))
	if err != nil || lineNo < 1 {
		return metricParsedLine{}, false, fmt.Errorf("invalid line_number %q: %q", fields[1], raw)
	}
	pageNo, err := strconv.Atoi(strings.TrimSpace(fields[2]))
	if err != nil || pageNo < 1 {
		return metricParsedLine{}, false, fmt.Errorf("invalid page_number %q: %q", fields[2], raw)
	}
	lineType := strings.TrimSpace(fields[3])
	if lineType == "" {
		return metricParsedLine{}, false, fmt.Errorf("empty line_type: %q", raw)
	}
	return metricParsedLine{
		Flag:       flag,
		LineNumber: lineNo,
		PageNumber: pageNo,
		LineType:   lineType,
		Content:    strings.TrimSpace(fields[4]),
	}, true, nil
}

func parseMetricInputLines(lines []string) ([]metricParsedLine, error) {
	parsedLines := make([]metricParsedLine, 0, len(lines))
	for _, raw := range lines {
		line, ok, err := parseMetricInputLine(raw)
		if err != nil {
			return nil, err
		}
		if ok {
			parsedLines = append(parsedLines, line)
		}
	}
	return parsedLines, nil
}
*/

// metricCandidateTask is the pass-1 task prompt (base prompt + output schema).
// The chunk lines are NOT included — they are the canonical InputText (document section)
// under document-first layout, so this chunk's prefix matches the other chunk processors.
// See ADR 2026062701 §Phase 2.3.
func metricCandidateTask(base string) string {
	schema := map[string]any{
		"language": "string",
		"candidates": []map[string]any{{
			"metric_name_hint":  "string",
			"subject_hint":      "string",
			"object_hint":       "string",
			"object_type_hint":  "string",
			"object_role_hint":  "string",
			"evidence_quote":    "string",
			"source_line_spans": []string{"5", "12:14"},
			"unit_hint":         "string",
			"value_hint":        "string",
			"confidence":        0.0,
			"confidence_reason": "string",
		}},
	}
	schemaJSON, _ := json.Marshal(schema)
	task := "Return JSON only. Use exactly this top-level schema:\n" + string(schemaJSON)
	if strings.TrimSpace(base) == "" {
		return task
	}
	return strings.TrimSpace(base) + "\n\n" + task
}

/*
func buildMetricUserPrompt(lines []string, parsedLines []metricParsedLine) string {
	schema := map[string]any{
		"language": "string",
		"metrics": []map[string]any{{
			"candidate_id":          "string",
			"metric_name":           "string",
			"metric_name_en":        "string",
			"source_line_spans":     []string{"5", "12:14"},
			"subject":               "string",
			"subject_en":            "string",
			"desc":                  "string",
			"desc_en":               "string",
			"context":               "string",
			"context_en":            "string",
			"keywords":              []string{"string"},
			"keywords_en":           []string{"string"},
			"location_type":         "string",
			"unit":                  "string",
			"unit_en":               "string",
			"metric_value":          "string",
			"value_data_type":       "string",
			"value_range_type":      "string",
			"value_class":           "string",
			"value_class_en":        "string",
			"formula_or_definition": "string",
			"threshold_or_target":   "string",
			"measurement_frequency": "string",
			"confidence":            0.0,
			"is_explicit_metric":    true,
			"table_name_or_section": "string",
			"reasoning_tags":        []string{"string"},
			"category_paths": []map[string]any{{
				"category_path": []map[string]any{{
					"name":       "string",
					"keywords":   []string{"string"},
					"confidence": 0.0,
				}},
				"path_keywords":   []string{"string"},
				"path_confidence": 0.0,
			}},
			"category_paths_en": []map[string]any{{
				"category_path": []map[string]any{{
					"name":       "string",
					"keywords":   []string{"string"},
					"confidence": 0.0,
				}},
				"path_keywords":   []string{"string"},
				"path_confidence": 0.0,
			}},
		}},
		"uncertain_metrics": []any{},
	}

	linesJSON, _ := json.Marshal(lines)
	parsedLinesJSON, _ := json.Marshal(parsedLines)
	schemaJSON, _ := json.Marshal(schema)
	return "Return JSON only. Use exactly this top-level schema:\n" + string(schemaJSON) +
		"\n\nInput lines (raw, JSON array):\n" + string(linesJSON) +
		"\n\nParsed line hints (best effort; do not treat as complete):\n" + string(parsedLinesJSON)
}

func buildMetricRelationUserPrompt(candidate metricCandidate) string {
	candidateJSON, _ := json.Marshal(map[string]any{
		"candidate_id":        candidate.CandidateID,
		"metric_name_hint":    candidate.MetricNameHint,
		"subject_hint":        candidate.SubjectHint,
		"unit_hint":           candidate.UnitHint,
		"value_hint":          candidate.ValueHint,
		"supporting_mentions": candidate.SupportingMentions,
	})
	schema := map[string]any{
		"language": "string",
		"metrics": []map[string]any{{
			"metric_name":           "string",
			"metric_name_en":        "string",
			"source_line_spans":     []string{"5", "12:14"},
			"subject":               "string",
			"subject_en":            "string",
			"desc":                  "string",
			"desc_en":               "string",
			"context":               "string",
			"context_en":            "string",
			"keywords":              []string{"string"},
			"keywords_en":           []string{"string"},
			"location_type":         "string",
			"unit":                  "string",
			"unit_en":               "string",
			"metric_value":          "string",
			"value_data_type":       "string",
			"value_range_type":      "string",
			"value_class":           "string",
			"value_class_en":        "string",
			"formula_or_definition": "string",
			"threshold_or_target":   "string",
			"measurement_frequency": "string",
			"confidence":            0.0,
			"is_explicit_metric":    true,
			"table_name_or_section": "string",
			"reasoning_tags":        []string{"string"},
			"metric_categories":     []string{"category_key"},
			"metric_categories_en":  []string{"category_key_en"},
		}},
		"uncertain_metrics": []any{},
	}
	schemaJSON, _ := json.Marshal(schema)
	return "Return JSON only. Use exactly this top-level schema:\n" + string(schemaJSON) +
		"\n\nCandidate:\n" + string(candidateJSON) +
		"\n\nSource lines (JSON array):\n" + blockLinesToJSON(candidate.SupportLines)
}
*/

func buildMetricRelationBatchPrompt(candidates []metricCandidate) string {
	candidatesData := make([]map[string]any, 0, len(candidates))
	for _, c := range candidates {
		candidatesData = append(candidatesData, map[string]any{
			"candidate_id":        c.CandidateID,
			"metric_name_hint":    c.MetricNameHint,
			"subject_hint":        c.SubjectHint,
			"object_hint":         c.ObjectHint,
			"object_type_hint":    c.ObjectTypeHint,
			"object_role_hint":    c.ObjectRoleHint,
			"unit_hint":           c.UnitHint,
			"value_hint":          c.ValueHint,
			"supporting_mentions": c.SupportingMentions,
		})
	}
	candidatesJSON, _ := json.Marshal(candidatesData)
	schema := map[string]any{
		"language": "string",
		"metrics": []map[string]any{{
			"metric_name":           "string",
			"metric_name_en":        "string",
			"source_line_spans":     []string{"5", "12:14"},
			"subject":               "string",
			"subject_en":            "string",
			"desc":                  "string",
			"desc_en":               "string",
			"context":               "string",
			"context_en":            "string",
			"keywords":              []string{"string"},
			"keywords_en":           []string{"string"},
			"location_type":         "string",
			"unit":                  "string",
			"unit_en":               "string",
			"metric_value":          "string",
			"value_data_type":       "string",
			"value_range_type":      "string",
			"value_class":           "string",
			"value_class_en":        "string",
			"formula_or_definition": "string",
			"threshold_or_target":   "string",
			"measurement_frequency": "string",
			"confidence":            0.0,
			"is_explicit_metric":    true,
			"table_name_or_section": "string",
			"reasoning_tags":        []string{"string"},
			"metric_categories":     []string{"category_key"},
			"metric_categories_en":  []string{"category_key_en"},
			"objects": []map[string]any{{
				"object_name":       "string",
				"object_name_en":    "string",
				"object_name_zh":    "string",
				"language":          "string",
				"object_type":       "equipment|material|system|process|organization|person|place|document|concept|other",
				"object_role":       "measured_object|regulated_object|requirement_target|inventory_item|component|parent_system|self|other",
				"aliases":           []string{"string"},
				"acronyms":          []string{"string"},
				"description":       "string",
				"evidence_quote":    "string",
				"source_line_spans": []string{"5", "12:14"},
				"confidence":        0.0,
			}},
		}},
		"uncertain_metrics": []any{},
	}
	schemaJSON, _ := json.Marshal(schema)
	// Merge support lines from all candidates (same chunk); deduplicate by page+line number.
	seen := make(map[[2]int]struct{})
	merged := make([]BlockLine, 0)
	for _, c := range candidates {
		for _, l := range c.SupportLines {
			key := [2]int{l.PageNumber, l.LineNumber}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			merged = append(merged, l)
		}
	}
	sort.Slice(merged, func(i, j int) bool {
		if merged[i].PageNumber != merged[j].PageNumber {
			return merged[i].PageNumber < merged[j].PageNumber
		}
		return merged[i].LineNumber < merged[j].LineNumber
	})
	sourceLines := blockLinesToJSON(merged)
	return "Return JSON only. Use exactly this top-level schema:\n" + string(schemaJSON) +
		"\n\nCandidates:\n" + string(candidatesJSON) +
		"\n\nSource lines (JSON array):\n" + sourceLines
}

func normalizeMetricList(items []any) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		raw, ok := item.(map[string]any)
		if !ok {
			continue
		}
		normalized := map[string]any{
			"candidate_id":          strings.TrimSpace(asString(raw["candidate_id"])),
			"metric_name":           strings.TrimSpace(asString(raw["metric_name"])),
			"metric_name_en":        strings.TrimSpace(asString(raw["metric_name_en"])),
			"subject":               strings.TrimSpace(asString(raw["subject"])),
			"subject_en":            strings.TrimSpace(asString(raw["subject_en"])),
			"desc":                  strings.TrimSpace(asString(raw["desc"])),
			"desc_en":               strings.TrimSpace(asString(raw["desc_en"])),
			"context":               strings.TrimSpace(asString(raw["context"])),
			"context_en":            strings.TrimSpace(asString(raw["context_en"])),
			"keywords":              toStringSlice(raw["keywords"]),
			"keywords_en":           toStringSlice(raw["keywords_en"]),
			"location_type":         strings.TrimSpace(asString(raw["location_type"])),
			"unit":                  strings.TrimSpace(asString(raw["unit"])),
			"unit_en":               strings.TrimSpace(asString(raw["unit_en"])),
			"metric_value":          strings.TrimSpace(asString(raw["metric_value"])),
			"value_data_type":       strings.TrimSpace(asString(raw["value_data_type"])),
			"value_range_type":      strings.TrimSpace(asString(raw["value_range_type"])),
			"value_class":           strings.TrimSpace(asString(raw["value_class"])),
			"value_class_en":        strings.TrimSpace(asString(raw["value_class_en"])),
			"formula_or_definition": strings.TrimSpace(asString(raw["formula_or_definition"])),
			"threshold_or_target":   strings.TrimSpace(asString(raw["threshold_or_target"])),
			"measurement_frequency": strings.TrimSpace(asString(raw["measurement_frequency"])),
			"confidence":            toFloat(raw["confidence"]),
			"is_explicit_metric":    toBool(raw["is_explicit_metric"]),
			"table_name_or_section": strings.TrimSpace(asString(raw["table_name_or_section"])),
			"reasoning_tags":        toStringSlice(raw["reasoning_tags"]),
			"metric_categories":     metricCategoryKeysFromValue(raw["metric_categories"]),
			"metric_categories_en":  metricCategoryKeysFromValue(raw["metric_categories_en"]),
			"category_paths":        raw["category_paths"],
			"category_paths_en":     raw["category_paths_en"],
		}
		if objects := objectItemsFromValue(raw["objects"]); len(objects) > 0 {
			normalized["objects"] = objects
		}
		normalized["source_line_spans"] = normalizeSourceLineSpans(raw["source_line_spans"])
		out = append(out, normalized)
	}
	return out
}

func normalizeMetricCandidateMentions(items []any, block Block) []metricCandidateMention {
	out := make([]metricCandidateMention, 0, len(items))
	for _, item := range items {
		raw, ok := item.(map[string]any)
		if !ok {
			continue
		}
		spans := normalizeSourceLineSpans(raw["source_line_spans"])
		quote := strings.TrimSpace(asString(raw["evidence_quote"]))
		out = append(out, metricCandidateMention{
			CandidateID:       firstNonEmptyTrimmed(strings.TrimSpace(asString(raw["candidate_id"])), fmt.Sprintf("%d_%d", block.Index, len(out)+1)),
			MetricNameHint:    strings.TrimSpace(asString(raw["metric_name_hint"])),
			SubjectHint:       strings.TrimSpace(asString(raw["subject_hint"])),
			ObjectHint:        strings.TrimSpace(asString(raw["object_hint"])),
			ObjectTypeHint:    strings.TrimSpace(asString(raw["object_type_hint"])),
			ObjectRoleHint:    strings.TrimSpace(asString(raw["object_role_hint"])),
			EvidenceQuote:     quote,
			SourceLineSpans:   spans,
			UnitHint:          strings.TrimSpace(asString(raw["unit_hint"])),
			ValueHint:         strings.TrimSpace(asString(raw["value_hint"])),
			Confidence:        toFloat(raw["confidence"]),
			ConfidenceReason:  strings.TrimSpace(asString(raw["confidence_reason"])),
			ChunkIndex:        block.Index,
			ChunkLines:        append([]BlockLine(nil), block.Lines...),
			HasNormalEvidence: metricCandidateHasNormalEvidence(block, spans, quote),
		})
	}
	return out
}

func annotateMetricCandidatePayload(payload map[string]any, chunkID int) {
	if payload == nil {
		return
	}
	rawItems, ok := payload["candidates"].([]any)
	if !ok {
		return
	}
	annotated := make([]any, 0, len(rawItems))
	for i, item := range rawItems {
		raw, ok := item.(map[string]any)
		if !ok {
			annotated = append(annotated, item)
			continue
		}
		cloned := cloneMetricMap(raw)
		if strings.TrimSpace(asString(cloned["candidate_id"])) == "" {
			cloned["candidate_id"] = fmt.Sprintf("%d_%d", chunkID, i+1)
		}
		annotated = append(annotated, cloned)
	}
	payload["candidates"] = annotated
}

func metricCandidateHasNormalEvidence(block Block, spans []string, quote string) bool {
	lineNums := make(map[int]struct{})
	for _, span := range spans {
		start, end, ok := parseMetricLineSpan(span)
		if !ok {
			continue
		}
		for i := start; i <= end; i++ {
			lineNums[i] = struct{}{}
		}
	}
	for _, line := range block.Lines {
		if line.Flag != "n" {
			continue
		}
		if len(lineNums) == 0 {
			if quote == "" || strings.Contains(strings.ToLower(line.Content), strings.ToLower(quote)) {
				return true
			}
			continue
		}
		if _, ok := lineNums[line.LineNumber]; ok {
			return true
		}
	}
	return false
}

func parseMetricLineSpan(span string) (int, int, bool) {
	span = strings.TrimSpace(span)
	if span == "" {
		return 0, 0, false
	}
	sepIdx := strings.Index(span, ":")
	if sepIdx < 0 {
		sepIdx = strings.Index(span, "-")
	}
	if sepIdx > 0 {
		parts := []string{span[:sepIdx], span[sepIdx+1:]}
		start, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
		end, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err1 != nil || err2 != nil || start <= 0 || end < start {
			return 0, 0, false
		}
		return start, end, true
	}
	n, err := strconv.Atoi(span)
	if err != nil || n <= 0 {
		return 0, 0, false
	}
	return n, n, true
}

func mentionsAsCandidates(mentions []metricCandidateMention) []metricCandidate {
	out := make([]metricCandidate, 0, len(mentions))
	for _, mention := range mentions {
		if !mention.HasNormalEvidence {
			continue
		}
		out = append(out, metricCandidate{
			CandidateID:    firstNonEmptyTrimmed(mention.CandidateID, fmt.Sprintf("%d_%d", mention.ChunkIndex, len(out)+1)),
			ChunkIndex:     mention.ChunkIndex,
			MetricNameHint: mention.MetricNameHint,
			SubjectHint:    mention.SubjectHint,
			ObjectHint:     mention.ObjectHint,
			ObjectTypeHint: mention.ObjectTypeHint,
			ObjectRoleHint: mention.ObjectRoleHint,
			UnitHint:       mention.UnitHint,
			ValueHint:      mention.ValueHint,
			SupportingMentions: []map[string]any{{
				"metric_name_hint":  mention.MetricNameHint,
				"subject_hint":      mention.SubjectHint,
				"evidence_quote":    mention.EvidenceQuote,
				"source_line_spans": mention.SourceLineSpans,
				"unit_hint":         mention.UnitHint,
				"value_hint":        mention.ValueHint,
				"confidence":        mention.Confidence,
				"confidence_reason": mention.ConfidenceReason,
			}},
			SupportLines: append([]BlockLine(nil), mention.ChunkLines...),
		})
	}
	return out
}

/*
func mergeMetricMentionCandidates(mentions []metricCandidateMention) []metricCandidate {
	type bucket struct {
		mentions []metricCandidateMention
	}
	grouped := map[string]*bucket{}
	order := make([]string, 0, len(mentions))
	for _, mention := range mentions {
		key := normalizedMetricCandidateKey(mention.MetricNameHint, mention.SubjectHint, mention.UnitHint, mention.ValueHint)
		if key == "" {
			continue
		}
		if grouped[key] == nil {
			grouped[key] = &bucket{}
			order = append(order, key)
		}
		grouped[key].mentions = append(grouped[key].mentions, mention)
	}

	out := make([]metricCandidate, 0, len(order))
	for _, key := range order {
		b := grouped[key]
		if b == nil || len(b.mentions) == 0 {
			continue
		}
		hasNormal := false
		for _, mention := range b.mentions {
			if mention.HasNormalEvidence {
				hasNormal = true
				break
			}
		}
		if !hasNormal {
			continue
		}
		supportMentions := make([]map[string]any, 0, len(b.mentions))
		lineMap := map[string]BlockLine{}
		metricName := ""
		subject := ""
		unit := ""
		value := ""
		for _, mention := range b.mentions {
			metricName = firstNonEmptyTrimmed(metricName, mention.MetricNameHint)
			subject = firstNonEmptyTrimmed(subject, mention.SubjectHint)
			unit = firstNonEmptyTrimmed(unit, mention.UnitHint)
			value = firstNonEmptyTrimmed(value, mention.ValueHint)
			supportMentions = append(supportMentions, map[string]any{
				"metric_name_hint":  mention.MetricNameHint,
				"subject_hint":      mention.SubjectHint,
				"evidence_quote":    mention.EvidenceQuote,
				"source_line_spans": mention.SourceLineSpans,
				"unit_hint":         mention.UnitHint,
				"value_hint":        mention.ValueHint,
				"confidence":        mention.Confidence,
				"confidence_reason": mention.ConfidenceReason,
			})
			for _, line := range mention.BlockLines {
				lineKey := fmt.Sprintf("%d:%d:%s", line.PageNumber, line.LineNumber, line.Content)
				existing, exists := lineMap[lineKey]
				if !exists || (existing.Flag != "n" && line.Flag == "n") {
					lineMap[lineKey] = line
				}
			}
		}
		supportLines := make([]BlockLine, 0, len(lineMap))
		for _, line := range lineMap {
			supportLines = append(supportLines, line)
		}
		sort.Slice(supportLines, func(i, j int) bool {
			if supportLines[i].PageNumber != supportLines[j].PageNumber {
				return supportLines[i].PageNumber < supportLines[j].PageNumber
			}
			if supportLines[i].LineNumber != supportLines[j].LineNumber {
				return supportLines[i].LineNumber < supportLines[j].LineNumber
			}
			return supportLines[i].Flag < supportLines[j].Flag
		})
		out = append(out, metricCandidate{
			CandidateID:        fmt.Sprintf("metric_cand_%d", len(out)+1),
			MetricNameHint:     metricName,
			SubjectHint:        subject,
			UnitHint:           unit,
			ValueHint:          value,
			SupportingMentions: supportMentions,
			SupportLines:       supportLines,
		})
	}
	return out
}
*/

func normalizedMetricCandidateKey(parts ...string) string {
	normalized := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.ToLower(strings.TrimSpace(part))
		part = strings.Join(strings.Fields(part), " ")
		if part != "" {
			normalized = append(normalized, part)
		}
	}
	return strings.Join(normalized, "|")
}

func dedupeFinalMetricRows(metrics []map[string]any) []map[string]any {
	grouped := map[string]map[string]any{}
	order := make([]string, 0, len(metrics))
	for _, metric := range metrics {
		key := normalizedMetricCandidateKey(
			asString(metric["metric_name"]),
			asString(metric["subject"]),
			asString(metric["unit"]),
			asString(metric["metric_value"]),
			strings.Join(normalizeSourceLineSpans(metric["source_line_spans"]), ","),
		)
		if existing, ok := grouped[key]; ok {
			mergedSpans := normalizeSourceLineSpans(existing["source_line_spans"])
			for _, span := range normalizeSourceLineSpans(metric["source_line_spans"]) {
				if !slices.Contains(mergedSpans, span) {
					mergedSpans = append(mergedSpans, span)
				}
			}
			existing["source_line_spans"] = mergedSpans
			mergedCategories := metricCategoryKeysFromValue(existing["metric_categories"])
			for _, category := range metricCategoryKeysFromValue(metric["metric_categories"]) {
				mergedCategories = appendUniqueString(mergedCategories, category)
			}
			if len(mergedCategories) > 0 {
				existing["metric_categories"] = mergedCategories
			}
			mergedCategoriesEn := metricCategoryKeysFromValue(existing["metric_categories_en"])
			for _, category := range metricCategoryKeysFromValue(metric["metric_categories_en"]) {
				mergedCategoriesEn = appendUniqueString(mergedCategoriesEn, category)
			}
			if len(mergedCategoriesEn) > 0 {
				existing["metric_categories_en"] = mergedCategoriesEn
			}
			if toFloat(metric["confidence"]) > toFloat(existing["confidence"]) {
				existing["confidence"] = metric["confidence"]
			}
			continue
		}
		cloned := map[string]any{}
		for k, v := range metric {
			cloned[k] = v
		}
		grouped[key] = cloned
		order = append(order, key)
	}
	out := make([]map[string]any, 0, len(order))
	for _, key := range order {
		out = append(out, grouped[key])
	}
	return out
}

func backfillMetricResultCandidateIDs(metrics []map[string]any, candidates []metricCandidate) {
	candidateBySpanKey := make(map[string]string, len(candidates))
	ambiguousSpanKeys := map[string]struct{}{}
	for _, candidate := range candidates {
		spanKey := strings.Join(candidateSourceLineSpans(candidate), ",")
		if spanKey == "" {
			continue
		}
		if existing, ok := candidateBySpanKey[spanKey]; ok && existing != candidate.CandidateID {
			delete(candidateBySpanKey, spanKey)
			ambiguousSpanKeys[spanKey] = struct{}{}
			continue
		}
		if _, ambiguous := ambiguousSpanKeys[spanKey]; !ambiguous {
			candidateBySpanKey[spanKey] = candidate.CandidateID
		}
	}
	for i := range metrics {
		if strings.TrimSpace(asString(metrics[i]["candidate_id"])) != "" {
			continue
		}
		spanKey := strings.Join(normalizeSourceLineSpans(metrics[i]["source_line_spans"]), ",")
		if candidateID := strings.TrimSpace(candidateBySpanKey[spanKey]); candidateID != "" {
			metrics[i]["candidate_id"] = candidateID
		}
	}
	if len(metrics) != len(candidates) {
		return
	}
	for i := range metrics {
		if strings.TrimSpace(asString(metrics[i]["candidate_id"])) == "" {
			metrics[i]["candidate_id"] = candidates[i].CandidateID
		}
	}
}

func candidateSourceLineSpans(candidate metricCandidate) []string {
	var combined []string
	for _, mention := range candidate.SupportingMentions {
		combined = append(combined, normalizeSourceLineSpans(mention["source_line_spans"])...)
	}
	return normalizeSourceLineSpans(combined)
}

func cloneMetricRows(metrics []map[string]any) []map[string]any {
	if len(metrics) == 0 {
		return []map[string]any{}
	}
	out := make([]map[string]any, 0, len(metrics))
	for _, metric := range metrics {
		out = append(out, cloneMetricMap(metric))
	}
	return out
}

func compactNonEmptyStrings(items []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func assignMergedMetricCandidateID(metric map[string]any, groupByID map[string]map[string]any, absorbed []any) {
	if strings.TrimSpace(asString(metric["candidate_id"])) != "" {
		return
	}
	if base := groupByID[asString(metric["metric_id"])]; base != nil {
		if candidateID := strings.TrimSpace(asString(base["candidate_id"])); candidateID != "" {
			metric["candidate_id"] = candidateID
			return
		}
	}
	for _, absorbedID := range absorbed {
		if base := groupByID[asString(absorbedID)]; base != nil {
			if candidateID := strings.TrimSpace(asString(base["candidate_id"])); candidateID != "" {
				metric["candidate_id"] = candidateID
				return
			}
		}
	}
}

func normalizeSourceLineSpans(value any) []string {
	var items []any
	switch v := value.(type) {
	case []any:
		items = v
	case []string:
		// Already normalized by a prior normalizeSourceLineSpans call; convert back to
		// []any so the parsing/merging logic below can run uniformly.
		items = make([]any, len(v))
		for i, s := range v {
			items[i] = s
		}
	default:
		return nil
	}

	type lineSpan struct{ start, end int }
	spans := make([]lineSpan, 0, len(items))

	for _, item := range items {
		switch v := item.(type) {
		case map[string]any:
			lineNo := int(toFloat(v["line_number"]))
			if lineNo > 0 {
				spans = append(spans, lineSpan{lineNo, lineNo})
			}
		case float64:
			lineNo := int(v)
			if lineNo > 0 {
				spans = append(spans, lineSpan{lineNo, lineNo})
			}
		case string:
			s := strings.TrimSpace(v)
			start, end, ok := parseMetricLineSpan(s)
			if ok {
				spans = append(spans, lineSpan{start, end})
			}
		}
	}

	if len(spans) == 0 {
		return nil
	}

	sort.Slice(spans, func(i, j int) bool {
		if spans[i].start != spans[j].start {
			return spans[i].start < spans[j].start
		}
		return spans[i].end < spans[j].end
	})

	merged := []lineSpan{spans[0]}
	for _, s := range spans[1:] {
		last := &merged[len(merged)-1]
		if s.start <= last.end+1 {
			if s.end > last.end {
				last.end = s.end
			}
		} else {
			merged = append(merged, s)
		}
	}

	out := make([]string, 0, len(merged))
	for _, s := range merged {
		if s.start == s.end {
			out = append(out, strconv.Itoa(s.start))
		} else {
			out = append(out, fmt.Sprintf("%d:%d", s.start, s.end))
		}
	}
	return out
}

func toStringSlice(v any) []string {
	switch x := v.(type) {
	case []any:
		out := make([]string, 0, len(x))
		for _, item := range x {
			s := strings.TrimSpace(asString(item))
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return trimStringSlice(x)
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return nil
		}
		return []string{s}
	default:
		return nil
	}
}

func metricCategoryKeysFromValue(v any) []string {
	raw := toStringSlice(v)
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		for _, part := range strings.Split(item, ",") {
			key := normalizeCategorySegment(part)
			if key != "" {
				out = appendUniqueString(out, key)
			}
		}
	}
	return out
}

func metricCategoryKeysFromMetric(metric map[string]any) []string {
	keys := metricCategoryKeysFromValue(metric["metric_categories"])
	if len(keys) > 0 {
		return keys
	}
	return metricCategoryKeysFromCategoryPaths(metric["category_paths_en"], metric["category_paths"])
}

func metricCategoryKeysFromCategoryPaths(rawPaths ...any) []string {
	out := []string{}
	for _, raw := range rawPaths {
		for _, entry := range parseCategoryPathsAny(raw) {
			for _, node := range entry.Nodes {
				key := normalizeCategorySegment(node.Name)
				if key != "" {
					out = appendUniqueString(out, key)
				}
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return out
}

func toFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case json.Number:
		f, _ := x.Float64()
		return f
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(x), 64)
		return f
	default:
		return 0
	}
}

func toBool(v any) bool {
	b, err := asBool(v, false)
	if err != nil {
		return false
	}
	return b
}

// logLLMCall writes one llm_call log entry. Errors are logged as warnings and never abort processing.
/*
func (p *MetricsProcessor) logLLMCall(
	ctx context.Context,
	callID, activity string,
	pass int,
	modelNames []string,
	promptName string,
	payload map[string]any,
	callErr error,
	start, end time.Time,
) {
	var artifactStr *string
	if payload != nil {
		if bs, err := json.Marshal(payload); err == nil {
			s := string(bs)
			artifactStr = &s
		}
	}
	var errStr *string
	if callErr != nil {
		s := callErr.Error()
		errStr = &s
	}
	rec := DocProcLogRecord{
		DocProcName:  p.Name(),
		ModelNames:   modelNames,
		PromptName:   promptName,
		Pass:         &pass,
		LLMCallID:    &callID,
		ActivityName: &activity,
		ArtifactJSON: artifactStr,
		Errors:       errStr,
		MSUsed:       int64Ptr(end.Sub(start).Milliseconds()),
	}
	if err := p.ProcLogger.LogLLMCall(ctx, rec, "MID-26052810"); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			p.Logger.Info("llm_call log skipped: doc processor stopped by user request", "call_id", callID)
		} else {
			p.Logger.Warn("failed to write llm_call log", "call_id", callID, "error", err)
		}
	}
}
*/

// logExtractMetricsChunk writes one extract_metrics log entry for a single Pass 1 chunk.
func (p *MetricsProcessor) logExtractMetricsChunk(
	ctx context.Context,
	recordID int64,
	callID string,
	chunkIdx, totalChunks int,
	modelNames []string, promptName string,
	payload map[string]any, callErr error,
	start, end time.Time,
	progress string,
) {
	candidates, _ := payload["candidates"].([]any)
	numMetrics := len(candidates)
	extraInfo := map[string]any{
		"chunk":        chunkIdx,
		"total_chunks": totalChunks,
		"num_metrics":  numMetrics,
		"percent":      progress,
	}
	extraBytes, _ := json.Marshal(extraInfo)
	extraStr := string(extraBytes)
	procProgress := progress

	var artifactStr *string
	if payload != nil {
		if bs, err := json.Marshal(payload); err == nil {
			s := string(bs)
			artifactStr = &s
		}
	}
	var errStr *string
	if callErr != nil {
		s := callErr.Error()
		errStr = &s
	}
	activity := "extract_metric_candidates"
	cacheHit, cacheMiss := extractorCacheTokens(p.Extractor)
	rec := DocProcLogRecord{
		CallReason:            p.Name(),
		DocProcName:           p.Name(),
		ModelNames:            modelNames,
		PromptName:            promptName,
		RecordID:              int64Ptr(recordID),
		ProcProgress:          &procProgress,
		LLMCallID:             &callID,
		ActivityName:          &activity,
		ArtifactJSON:          artifactStr,
		Errors:                errStr,
		ExtraInfoJSON:         &extraStr,
		MSUsed:                int64Ptr(end.Sub(start).Milliseconds()),
		PromptCacheHitTokens:  cacheHit,
		PromptCacheMissTokens: cacheMiss,
	}
	if err := p.ProcLogger.LogExtractMetrics(ctx, rec, "MID-26052811"); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			p.Logger.Info("extract_metrics log skipped: doc processor stopped by user request", "call_id", callID)
		} else {
			p.Logger.Warn("failed to write extract_metrics log", "call_id", callID, "error", err)
		}
	}
}

// logEnrichMetricsChunk writes one enrich_metrics log entry for a single Pass 2 chunk batch.
func (p *MetricsProcessor) logEnrichMetricsChunk(
	ctx context.Context,
	recordID int64,
	callID string,
	chunkIdx, totalChunks int,
	modelNames []string, promptName string,
	payload map[string]any, callErr error,
	start, end time.Time,
	progress string,
) {
	metricsRaw, _ := payload["metrics"].([]any)
	numMetrics := len(metricsRaw)
	extraInfo := map[string]any{
		"chunk":        chunkIdx,
		"total_chunks": totalChunks,
		"num_metrics":  numMetrics,
		"percent":      progress,
	}
	extraBytes, _ := json.Marshal(extraInfo)
	extraStr := string(extraBytes)
	procProgress := progress

	var artifactStr *string
	if payload != nil {
		if bs, err := json.Marshal(payload); err == nil {
			s := string(bs)
			artifactStr = &s
		}
	}
	var errStr *string
	if callErr != nil {
		s := callErr.Error()
		errStr = &s
	}
	activity := "enrich_metrics"
	cacheHit, cacheMiss := extractorCacheTokens(p.Extractor)
	rec := DocProcLogRecord{
		CallReason:            p.Name(),
		DocProcName:           p.Name(),
		ModelNames:            modelNames,
		PromptName:            promptName,
		RecordID:              int64Ptr(recordID),
		ProcProgress:          &procProgress,
		LLMCallID:             &callID,
		ActivityName:          &activity,
		ArtifactJSON:          artifactStr,
		Errors:                errStr,
		ExtraInfoJSON:         &extraStr,
		MSUsed:                int64Ptr(end.Sub(start).Milliseconds()),
		PromptCacheHitTokens:  cacheHit,
		PromptCacheMissTokens: cacheMiss,
	}
	if err := p.ProcLogger.LogEnrichMetrics(ctx, rec, "MID-26052812"); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			p.Logger.Info("enrich_metrics log skipped: doc processor stopped by user request", "call_id", callID)
		} else {
			p.Logger.Warn("failed to write enrich_metrics log", "call_id", callID, "error", err)
		}
	}
}

func (p *MetricsProcessor) logFinalMetricsSaved(
	ctx context.Context,
	recordID int64,
	language string,
	modelNames []string,
	promptName string,
	metrics []map[string]any,
	start, end time.Time,
) {
	payload := map[string]any{
		"language": firstNonEmptyTrimmed(language, "unknown"),
		"metrics":  cloneMetricRows(metrics),
	}
	artifactBytes, _ := json.Marshal(payload)
	artifactStr := string(artifactBytes)
	extraInfo := map[string]any{
		"num_metrics": len(metrics),
	}
	extraBytes, _ := json.Marshal(extraInfo)
	extraStr := string(extraBytes)
	progress := "100%"
	activityName := "extract_metrics_final"
	rec := DocProcLogRecord{
		CallReason:    p.Name(),
		DocProcName:   p.Name(),
		ModelNames:    compactNonEmptyStrings(modelNames),
		PromptName:    promptName,
		RecordID:      int64Ptr(recordID),
		ProcProgress:  &progress,
		ActivityName:  &activityName,
		ArtifactJSON:  &artifactStr,
		ExtraInfoJSON: &extraStr,
		MSUsed:        int64Ptr(end.Sub(start).Milliseconds()),
	}
	if err := p.ProcLogger.LogExtractMetrics(ctx, rec, "MID-26071201"); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			p.Logger.Info("extract_metrics_final log skipped: doc processor stopped by user request", "record_id", recordID)
		} else {
			p.Logger.Warn("failed to write extract_metrics_final log", "record_id", recordID, "error", err)
		}
	}
}

// logMetricsSummary writes one extract_metrics log entry after the processor finishes.
func (p *MetricsProcessor) logMetricsSummary(
	ctx context.Context,
	recordID int64,
	start, end time.Time,
	result metricExtractionResult,
	total_candidates int64,
	inserted int64,
	numChunks int,
) {
	extraInfo := map[string]any{
		"total_candidates":  total_candidates,
		"total_metrics":     inserted,
		"uncertain_metrics": len(result.UncertainMetrics),
		"fallback_count":    result.FallbackCount,
		"llm_call_count":    result.LLMCallCount,
		"num_chunks":        numChunks,
	}
	extraJSON, _ := json.Marshal(extraInfo)
	extraStr := string(extraJSON)

	seen := map[string]struct{}{}
	modelNames := make([]string, 0, 3)
	for _, n := range []string{p.MentionModelName, p.FallbackMentionModelName, p.RelationModelName} {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		modelNames = append(modelNames, n)
	}
	promptName := firstNonEmptyTrimmed(p.MentionPromptRef, p.RelationPromptRef)
	progress := "100%"
	activityName := "extract_metrics_finish"

	rec := DocProcLogRecord{
		CallReason:    p.Name(),
		DocProcName:   p.Name(),
		ModelNames:    modelNames,
		PromptName:    promptName,
		RecordID:      int64Ptr(recordID),
		ProcProgress:  &progress,
		ActivityName:  &activityName,
		ExtraInfoJSON: &extraStr,
		MSUsed:        int64Ptr(end.Sub(start).Milliseconds()),
	}
	if err := p.ProcLogger.LogExtractMetricsSummary(ctx, rec, "MID-26060802"); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			p.Logger.Info("extract_metrics log skipped: doc processor stopped by user request")
		} else {
			p.Logger.Warn("failed to write extract_metrics log", "error", err)
		}
	}
}

func (p *MetricsProcessor) extractMetricPayload(
	ctx context.Context,
	inputText string,
	promptText string,
	modelName string,
	cfg structureModelConfig,
	callReason string,
	loc string) (map[string]any, error) {
	applyStructureModelConfigToExtractor(p.Extractor, cfg)
	callLoc := strings.TrimSpace(loc)
	if callLoc == "" {
		callLoc = "MID-CWB-EXTRACT-METRICS"
	}
	in := newLLMJSONInput(ctx, firstNonEmptyTrimmed(p.MentionPromptRef, p.RelationPromptRef), promptText, modelName, inputText, callReason, callLoc)
	var (
		payload map[string]any
		err     error
	)

	// p.Logger.Info("llm_input_preview",
	// 	"prompt_name", in.PromptName,
	// 	"model_name", in.ModelName,
	// 	"document_first", in.DocumentFirst,
	// 	"messages", llmclients.PreviewMessages(in),
	// )
	if structuredExtractor, ok := p.Extractor.(LLMStructuredJSONExtractor); ok {
		var result *llmclients.StructuredOutputResult
		result, err = structuredExtractor.ExtractStructuredJSON(ctx, in, metricsExtractionContract())
		if err != nil {
			return nil, fmt.Errorf("(MID-26061701) failed extracting metrics, error:%v", err)
		}

		if result == nil {
			return nil, fmt.Errorf("(MID-26061702) no metrics extracted")
		}
		payload = result.Parsed
	} else {
		payload, err = p.Extractor.ExtractJSON(ctx, in)
		if err != nil {
			return nil, fmt.Errorf("(MID-26061703) failed extracting metrics, error:%v", err)
		}

		if payload == nil {
			return nil, fmt.Errorf("(MID-26061704) no metrics extracted")
		}
	}

	if _, ok := payload["metrics"]; ok {
		return payload, nil
	}

	if _, ok := payload["candidates"]; ok {
		return payload, nil
	}
	return nil, fmt.Errorf("(MID_26042491) llm output must contain 'metrics' or 'candidates'")
}

/*
func firstPromptLine(promptText string) string {
	promptText = strings.TrimSpace(promptText)
	if promptText == "" {
		return ""
	}
	if idx := strings.IndexByte(promptText, '\n'); idx >= 0 {
		return strings.TrimSpace(promptText[:idx])
	}
	return promptText
}
*/

func (p *MetricsProcessor) extractMetricCandidatePayloadWithFallback(ctx context.Context, inputText, taskPrompt string) (map[string]any, string, error) {
	payload, err := p.extractMetricPayload(ctx, inputText, taskPrompt, p.MentionModelName, p.MentionModelCfg, "extract_metric_candidates", "MID-26052901")
	if err == nil {
		return payload, strings.TrimSpace(p.MentionModelName), nil
	}

	primaryModelName := strings.TrimSpace(p.MentionModelName)
	fallbackModelName := strings.TrimSpace(p.FallbackMentionModelName)
	if fallbackModelName == "" {
		return nil, primaryModelName, err
	}
	if p.FallbackMentionModelErr != nil {
		return nil, fallbackModelName, fmt.Errorf("(MID_26042492) primary metric candidate extraction failed and fallback model %q is unavailable: %w", p.FallbackMentionModelRef, err)
	}

	if isEmptyMetricExtractionError(err) {
		p.Logger.Info("primary metric candidate extraction returned empty JSON; retrying fallback model",
			"primary_model", primaryModelName,
			"fallback_model", fallbackModelName,
			"prompt_name", p.MentionPromptRef,
		)
	} else {
		p.Logger.Warn("primary metric candidate extraction failed; retrying fallback model",
			"primary_model", primaryModelName,
			"fallback_model", fallbackModelName,
			"error", err,
			"prompt_name", p.MentionPromptRef,
		)
	}

	payload, fallbackErr := p.extractMetricPayload(ctx, inputText, taskPrompt, fallbackModelName, p.FallbackMentionModelCfg, "extract_metric_candidates", "MID-26052902")
	if fallbackErr != nil {
		if isEmptyMetricExtractionError(fallbackErr) {
			p.Logger.Info("fallback metric candidate extraction returned empty JSON; treating as empty result",
				"fallback_model", fallbackModelName,
				"prompt_name", p.MentionPromptRef,
			)
			return map[string]any{"language": "unknown", "candidates": []any{}}, fallbackModelName, nil
		}
		return nil, fallbackModelName, fmt.Errorf("(MID_26042493) primary extraction failed: %w; fallback extraction failed: %v", err, fallbackErr)
	}
	return payload, fallbackModelName, nil
}

func isEmptyMetricExtractionError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.TrimSpace(err.Error())
	return (strings.Contains(msg, "unexpected end of JSON input") && strings.Contains(msg, "json:{[]}")) ||
		strings.Contains(msg, "(MID_26042490)")
}

// chunksToBlocks converts Chunk values (from the chunking processor) into Block
// values so that the metrics extractor can process them uniformly. The Mark field
// "o" (overlap) maps to Flag "o"; everything else maps to Flag "n" (normal).
func chunksToBlocks(chunks []Chunk) []Block {
	blocks := make([]Block, 0, len(chunks))
	for _, c := range chunks {
		lines := make([]BlockLine, 0, len(c.Lines))
		for _, ml := range c.Lines {
			flag := "n"
			if strings.EqualFold(strings.TrimSpace(ml.Mark), "o") {
				flag = "o"
			}
			lines = append(lines, BlockLine{
				Flag:       flag,
				LineNumber: ml.Line.LineNo,
				PageNumber: ml.Line.PageNo,
				LineType:   ml.Line.LineType,
				Content:    ml.Line.Content,
			})
		}
		blocks = append(blocks, Block{Index: c.SeqNo, Lines: lines})
	}
	return blocks
}

func loadMetricsPromptFromEnv() (promptText string, promptRef string, promptPath string, promptErr error) {
	for _, key := range []string{"EXTRACT_METRICS_PROMPT", "PROMPT_FILE_NAME"} {
		promptRef = strings.TrimSpace(os.Getenv(key))
		if promptRef != "" {
			break
		}
	}
	if promptRef == "" {
		promptRef = "prompt-enrich-metrics-v3.md"
	}

	paths := make([]string, 0, 8)
	addCandidate := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" {
			return
		}
		for _, existing := range paths {
			if existing == p {
				return
			}
		}
		paths = append(paths, p)
	}

	if filepath.IsAbs(promptRef) {
		addCandidate(promptRef)
	} else {
		addCandidate(promptRef)
		if promptDir := strings.TrimSpace(os.Getenv("PROMPT_DIR")); promptDir != "" {
			addCandidate(filepath.Join(promptDir, promptRef))
		}
		addCandidate(filepath.Join("server", "cmd", "doc-processor", promptRef))
		addCandidate(filepath.Join("server", "cmd", "doc-processor", "prompts", promptRef))
		addCandidate(filepath.Join("python", "extract_metrics", "prompts", promptRef))
		addCandidate(filepath.Join("prompts", promptRef))
	}

	var lastErr error
	for _, candidate := range paths {
		bs, err := os.ReadFile(candidate)
		if err != nil {
			lastErr = fmt.Errorf("(MID_26042465) failed reading file. Path:%s, error:%w", candidate, err)
			continue
		}
		text := strings.TrimSpace(string(bs))
		if text == "" {
			return "", promptRef, candidate, fmt.Errorf("(MID_26042453) prompt file is empty")
		}
		return text, promptRef, candidate, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("(MID_26042454) no candidate path available")
	}
	return "", promptRef, "", fmt.Errorf("(MID_26042455) prompt file not found: %w", lastErr)
}

func (s MetricsSQLStore) ensureMetricsTable(ctx context.Context) error {
	if s.DB == nil {
		return fmt.Errorf("(MID_26042456) db is nil")
	}
	const ddl = `
CREATE SCHEMA IF NOT EXISTS kb;

CREATE TABLE IF NOT EXISTS kb.metrics (
    id BIGSERIAL PRIMARY KEY,
    event_id TEXT,
    input_record_id BIGINT NOT NULL,
    metric_id TEXT,
    metric_name TEXT,
    metric_name_en TEXT,
    source_line_spans JSONB,
    metric_subject TEXT,
    metric_subject_en TEXT,
    metric_desc TEXT,
    metric_desc_en TEXT,
    metric_context TEXT,
    metric_context_en TEXT,
    metric_keywords JSONB,
    metric_keywords_en JSONB,
    model_name TEXT,
    prompt_name TEXT,
    location_type TEXT,
    metric_unit TEXT,
    metric_unit_en TEXT,
    metric_value TEXT,
    value_data_type TEXT,
    value_range_type TEXT,
    value_class TEXT,
    value_class_en TEXT,
    formula_or_definition TEXT,
    threshold_or_target TEXT,
    measurement_frequency TEXT,
    confidence DOUBLE PRECISION,
    is_explicit_metric BOOLEAN,
    table_name_or_section TEXT,
    reasoning_tags JSONB,
    metric_categories TEXT NOT NULL DEFAULT '',
    metric_categories_en TEXT,
    category_paths JSONB,
    category_paths_en JSONB,
    search_document TEXT,
    search_vector TSVECTOR,
    ext_info JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE kb.metrics ADD COLUMN IF NOT EXISTS metric_categories TEXT NOT NULL DEFAULT '';
ALTER TABLE kb.metrics ADD COLUMN IF NOT EXISTS metric_categories_en TEXT;
`
	_, err := s.DB.ExecContext(ctx, ddl)
	return err
}

func (s MetricsSQLStore) MetricsExist(ctx context.Context, inputRecordID int64) (bool, error) {
	if err := s.ensureMetricsTable(ctx); err != nil {
		return false, err
	}
	const q = `SELECT 1 FROM kb.metrics WHERE input_record_id = $1 LIMIT 1`
	var one int
	err := s.DB.QueryRowContext(ctx, q, inputRecordID).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s MetricsSQLStore) DeleteMetricsByInputRecordID(ctx context.Context, inputRecordID int64) (int64, error) {
	if err := s.ensureMetricsTable(ctx); err != nil {
		return 0, err
	}
	res, err := s.DB.ExecContext(ctx, `DELETE FROM kb.metrics WHERE input_record_id = $1`, inputRecordID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// GetMetricsByInputRecordID reads every persisted metric row for a record into
// memory so incremental-merge logic (Task 8+) can diff against freshly
// extracted metrics before deciding what to upsert.
func (s MetricsSQLStore) GetMetricsByInputRecordID(ctx context.Context, inputRecordID int64) ([]map[string]any, error) {
	if err := s.ensureMetricsTable(ctx); err != nil {
		return nil, err
	}
	const q = `
SELECT id, metric_id, metric_name, metric_name_en, source_line_spans, metric_subject,
       metric_subject_en, metric_desc, metric_desc_en, metric_context, metric_context_en,
       metric_keywords, metric_keywords_en, metric_unit, metric_unit_en, metric_value,
       value_data_type, value_range_type, value_class, value_class_en,
       formula_or_definition, threshold_or_target, measurement_frequency,
       metric_categories, metric_categories_en, category_paths, category_paths_en,
       ext_info
FROM kb.metrics
WHERE input_record_id = $1`
	rows, err := s.DB.QueryContext(ctx, q, inputRecordID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []map[string]any
	for rows.Next() {
		var (
			id                                                                           int64
			metricID, name, nameEn, subject, subjectEn, desc, descEn, ctx1, ctxEn        sql.NullString
			unit, unitEn, value, valueDataType, valueRangeType, valueClass, valueClassEn sql.NullString
			formula, threshold, freq, categories, categoriesEn, catPaths, catPathsEn     sql.NullString
			spansJSON, keywordsJSON, keywordsEnJSON, extInfoJSON                         sql.NullString
		)
		if err := rows.Scan(&id, &metricID, &name, &nameEn, &spansJSON, &subject, &subjectEn,
			&desc, &descEn, &ctx1, &ctxEn, &keywordsJSON, &keywordsEnJSON, &unit, &unitEn, &value,
			&valueDataType, &valueRangeType, &valueClass, &valueClassEn, &formula, &threshold, &freq,
			&categories, &categoriesEn, &catPaths, &catPathsEn, &extInfoJSON); err != nil {
			return nil, err
		}
		m := map[string]any{
			"id": id, "metric_id": metricID.String, "metric_name": name.String,
			"metric_name_en": nameEn.String, "metric_subject": subject.String,
			"metric_subject_en": subjectEn.String, "metric_desc": desc.String,
			"metric_desc_en": descEn.String, "metric_context": ctx1.String,
			"metric_context_en": ctxEn.String, "metric_unit": unit.String,
			"metric_unit_en": unitEn.String, "metric_value": value.String,
			"value_data_type": valueDataType.String, "value_range_type": valueRangeType.String,
			"value_class": valueClass.String, "value_class_en": valueClassEn.String,
			"formula_or_definition": formula.String, "threshold_or_target": threshold.String,
			"measurement_frequency": freq.String,
		}
		if spansJSON.Valid {
			var spans any
			_ = json.Unmarshal([]byte(spansJSON.String), &spans)
			m["source_line_spans"] = spans
		}
		if keywordsJSON.Valid {
			var kw any
			_ = json.Unmarshal([]byte(keywordsJSON.String), &kw)
			m["metric_keywords"] = kw
		}
		if keywordsEnJSON.Valid {
			var kwEn any
			_ = json.Unmarshal([]byte(keywordsEnJSON.String), &kwEn)
			m["metric_keywords_en"] = kwEn
		}
		if categories.Valid {
			var cats any
			_ = json.Unmarshal([]byte(categories.String), &cats)
			m["metric_categories"] = cats
		}
		if categoriesEn.Valid {
			var catsEn any
			_ = json.Unmarshal([]byte(categoriesEn.String), &catsEn)
			m["metric_categories_en"] = catsEn
		}
		if catPaths.Valid {
			var cp any
			_ = json.Unmarshal([]byte(catPaths.String), &cp)
			m["category_paths"] = cp
		}
		if catPathsEn.Valid {
			var cpEn any
			_ = json.Unmarshal([]byte(catPathsEn.String), &cpEn)
			m["category_paths_en"] = cpEn
		}
		if extInfoJSON.Valid {
			var ext map[string]any
			_ = json.Unmarshal([]byte(extInfoJSON.String), &ext)
			m["ext_info"] = ext
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s MetricsSQLStore) SaveMetrics(ctx context.Context, req SaveMetricsRequest) (int64, error) {
	if err := s.ensureMetricsTable(ctx); err != nil {
		return 0, err
	}
	if len(req.Metrics) == 0 {
		return 0, nil
	}

	const stmt = `
INSERT INTO kb.metrics (
	event_id,
	input_record_id,
	metric_id,
	metric_name,
	metric_name_en,
	source_line_spans,
	metric_subject,
	metric_subject_en,
	metric_desc,
	metric_desc_en,
	metric_context,
	metric_context_en,
	metric_keywords,
	metric_keywords_en,
	model_name,
	prompt_name,
	location_type,
	metric_unit,
	metric_unit_en,
	metric_value,
	value_data_type,
	value_range_type,
	value_class,
	value_class_en,
	formula_or_definition,
	threshold_or_target,
	measurement_frequency,
	confidence,
	is_explicit_metric,
	table_name_or_section,
	reasoning_tags,
	metric_categories,
	metric_categories_en,
	category_paths,
	category_paths_en,
	search_document,
	ext_info
)
VALUES (
	$1,$2,$3,$4,$5,$6::jsonb,$7,$8,$9,$10,$11,$12,$13::jsonb,$14::jsonb,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31::jsonb,$32,$33,$34::jsonb,$35::jsonb,$36,$37::jsonb
)`

	isEnglish := strings.EqualFold(strings.TrimSpace(req.Language), "en") ||
		strings.EqualFold(strings.TrimSpace(req.Language), "english")

	var eventIDVal interface{}
	if id := strings.TrimSpace(req.EventID); id != "" {
		eventIDVal = id
	}

	var inserted int64
	for _, metric := range req.Metrics {
		sourceSpansJSON, _ := json.Marshal(metric["source_line_spans"])
		keywordsJSON, _ := json.Marshal(metric["keywords"])
		reasoningTagsJSON, _ := json.Marshal(metric["reasoning_tags"])
		extInfo, _ := json.Marshal(map[string]any{
			"language":       req.Language,
			"schema_version": "2",
		})

		var (
			metricNameEn  any
			subjectEn     any
			descEn        any
			contextEn     any
			keywordsEnVal any
			unitEn        any
			valueClassEn  any
		)
		if !isEnglish {
			metricNameEn = strings.TrimSpace(asString(metric["metric_name_en"]))
			subjectEn = strings.TrimSpace(asString(metric["subject_en"]))
			descEn = strings.TrimSpace(asString(metric["desc_en"]))
			contextEn = strings.TrimSpace(asString(metric["context_en"]))
			kw, _ := json.Marshal(metric["keywords_en"])
			keywordsEnVal = string(kw)
			unitEn = strings.TrimSpace(asString(metric["unit_en"]))
			valueClassEn = strings.TrimSpace(asString(metric["value_class_en"]))
		}

		searchDocument := buildMetricSearchDocument(metric, !isEnglish)

		// metric_categories is a TEXT column; store the category keys as a JSON array
		// string so the post-save indexing step can parse them back for category links.
		metricCategoriesJSON, _ := json.Marshal(metricCategoryKeysFromMetric(metric))
		metricCategoriesText := string(metricCategoriesJSON)
		metricCategoriesEnJSON, _ := json.Marshal(metricCategoryKeysFromValue(metric["metric_categories_en"]))
		metricCategoriesEnText := string(metricCategoriesEnJSON)
		categoryPathsJSON, _ := json.Marshal(metric["category_paths"])
		categoryPathsEnJSON, _ := json.Marshal(metric["category_paths_en"])

		_, err := s.DB.ExecContext(ctx, stmt,
			eventIDVal,
			req.InputRecordID,
			strings.TrimSpace(asString(metric["metric_id"])),
			strings.TrimSpace(asString(metric["metric_name"])),
			metricNameEn,
			string(sourceSpansJSON),
			strings.TrimSpace(asString(metric["subject"])),
			subjectEn,
			strings.TrimSpace(asString(metric["desc"])),
			descEn,
			strings.TrimSpace(asString(metric["context"])),
			contextEn,
			string(keywordsJSON),
			keywordsEnVal,
			strings.TrimSpace(req.ModelName),
			strings.TrimSpace(req.PromptName),
			strings.TrimSpace(asString(metric["location_type"])),
			strings.TrimSpace(asString(metric["unit"])),
			unitEn,
			strings.TrimSpace(asString(metric["metric_value"])),
			strings.TrimSpace(asString(metric["value_data_type"])),
			strings.TrimSpace(asString(metric["value_range_type"])),
			strings.TrimSpace(asString(metric["value_class"])),
			valueClassEn,
			strings.TrimSpace(asString(metric["formula_or_definition"])),
			strings.TrimSpace(asString(metric["threshold_or_target"])),
			strings.TrimSpace(asString(metric["measurement_frequency"])),
			toFloat(metric["confidence"]),
			toBool(metric["is_explicit_metric"]),
			strings.TrimSpace(asString(metric["table_name_or_section"])),
			string(reasoningTagsJSON),
			metricCategoriesText,
			metricCategoriesEnText,
			string(categoryPathsJSON),
			string(categoryPathsEnJSON),
			searchDocument,
			string(extInfo),
		)
		if err != nil {
			return inserted, err
		}
		inserted++
	}
	return inserted, nil
}

// UpsertMetrics inserts-or-updates the given metrics for a record using the
// partial unique index on (input_record_id, metric_id) added in the Task 3
// migration. Unlike SaveMetrics, the caller (Task 8's merge logic) is
// responsible for deciding which metrics are new-or-changed and must supply
// "ext_info" per metric — this method does not skip or dedupe anything itself.
func (s MetricsSQLStore) UpsertMetrics(ctx context.Context, req SaveMetricsRequest) (int64, error) {
	if err := s.ensureMetricsTable(ctx); err != nil {
		return 0, err
	}
	if len(req.Metrics) == 0 {
		return 0, nil
	}

	const stmt = `
INSERT INTO kb.metrics (
	event_id, input_record_id, metric_id, metric_name, metric_name_en, source_line_spans,
	metric_subject, metric_subject_en, metric_desc, metric_desc_en, metric_context, metric_context_en,
	metric_keywords, metric_keywords_en, model_name, prompt_name, location_type, metric_unit, metric_unit_en,
	metric_value, value_data_type, value_range_type, value_class, value_class_en, formula_or_definition,
	threshold_or_target, measurement_frequency, confidence, is_explicit_metric, table_name_or_section,
	reasoning_tags, metric_categories, metric_categories_en, category_paths, category_paths_en,
	search_document, ext_info
) VALUES (
	$1,$2,$3,$4,$5,$6::jsonb,$7,$8,$9,$10,$11,$12,$13::jsonb,$14::jsonb,$15,$16,$17,$18,$19,$20,$21,$22,$23,
	$24,$25,$26,$27,$28,$29,$30,$31::jsonb,$32,$33,$34::jsonb,$35::jsonb,$36,$37::jsonb
)
ON CONFLICT (input_record_id, metric_id) WHERE metric_id IS NOT NULL DO UPDATE SET
	metric_name = EXCLUDED.metric_name, metric_name_en = EXCLUDED.metric_name_en,
	source_line_spans = EXCLUDED.source_line_spans, metric_subject = EXCLUDED.metric_subject,
	metric_subject_en = EXCLUDED.metric_subject_en, metric_desc = EXCLUDED.metric_desc,
	metric_desc_en = EXCLUDED.metric_desc_en, metric_context = EXCLUDED.metric_context,
	metric_context_en = EXCLUDED.metric_context_en, metric_keywords = EXCLUDED.metric_keywords,
	metric_keywords_en = EXCLUDED.metric_keywords_en, metric_unit = EXCLUDED.metric_unit,
	metric_unit_en = EXCLUDED.metric_unit_en, metric_value = EXCLUDED.metric_value,
	value_data_type = EXCLUDED.value_data_type, value_range_type = EXCLUDED.value_range_type,
	value_class = EXCLUDED.value_class, value_class_en = EXCLUDED.value_class_en,
	formula_or_definition = EXCLUDED.formula_or_definition, threshold_or_target = EXCLUDED.threshold_or_target,
	measurement_frequency = EXCLUDED.measurement_frequency, metric_categories = EXCLUDED.metric_categories,
	metric_categories_en = EXCLUDED.metric_categories_en, category_paths = EXCLUDED.category_paths,
	category_paths_en = EXCLUDED.category_paths_en, search_document = EXCLUDED.search_document,
	ext_info = EXCLUDED.ext_info`

	isEnglish := strings.EqualFold(strings.TrimSpace(req.Language), "en") ||
		strings.EqualFold(strings.TrimSpace(req.Language), "english")
	var eventIDVal any
	if id := strings.TrimSpace(req.EventID); id != "" {
		eventIDVal = id
	}

	var affected int64
	for _, metric := range req.Metrics {
		sourceSpansJSON, _ := json.Marshal(metric["source_line_spans"])
		keywordsJSON, _ := json.Marshal(metric["keywords"])
		reasoningTagsJSON, _ := json.Marshal(metric["reasoning_tags"])
		extInfo, _ := json.Marshal(metric["ext_info"])

		var metricNameEn, subjectEn, descEn, contextEn, keywordsEnVal, unitEn, valueClassEn any
		if !isEnglish {
			metricNameEn = strings.TrimSpace(asString(metric["metric_name_en"]))
			subjectEn = strings.TrimSpace(asString(metric["subject_en"]))
			descEn = strings.TrimSpace(asString(metric["desc_en"]))
			contextEn = strings.TrimSpace(asString(metric["context_en"]))
			kw, _ := json.Marshal(metric["keywords_en"])
			keywordsEnVal = string(kw)
			unitEn = strings.TrimSpace(asString(metric["unit_en"]))
			valueClassEn = strings.TrimSpace(asString(metric["value_class_en"]))
		}

		searchDocument := buildMetricSearchDocument(metric, !isEnglish)
		metricCategoriesJSON, _ := json.Marshal(metricCategoryKeysFromMetric(metric))
		metricCategoriesEnJSON, _ := json.Marshal(metricCategoryKeysFromValue(metric["metric_categories_en"]))
		categoryPathsJSON, _ := json.Marshal(metric["category_paths"])
		categoryPathsEnJSON, _ := json.Marshal(metric["category_paths_en"])

		res, err := s.DB.ExecContext(ctx, stmt,
			eventIDVal, req.InputRecordID, strings.TrimSpace(asString(metric["metric_id"])),
			strings.TrimSpace(asString(metric["metric_name"])), metricNameEn, string(sourceSpansJSON),
			strings.TrimSpace(asString(metric["subject"])), subjectEn,
			strings.TrimSpace(asString(metric["desc"])), descEn,
			strings.TrimSpace(asString(metric["context"])), contextEn,
			string(keywordsJSON), keywordsEnVal, strings.TrimSpace(req.ModelName), strings.TrimSpace(req.PromptName),
			strings.TrimSpace(asString(metric["location_type"])),
			strings.TrimSpace(asString(metric["unit"])), unitEn,
			strings.TrimSpace(asString(metric["metric_value"])),
			strings.TrimSpace(asString(metric["value_data_type"])),
			strings.TrimSpace(asString(metric["value_range_type"])),
			strings.TrimSpace(asString(metric["value_class"])), valueClassEn,
			strings.TrimSpace(asString(metric["formula_or_definition"])),
			strings.TrimSpace(asString(metric["threshold_or_target"])),
			strings.TrimSpace(asString(metric["measurement_frequency"])),
			toFloat(metric["confidence"]), toBool(metric["is_explicit_metric"]),
			strings.TrimSpace(asString(metric["table_name_or_section"])), string(reasoningTagsJSON),
			string(metricCategoriesJSON), string(metricCategoriesEnJSON), string(categoryPathsJSON),
			string(categoryPathsEnJSON), searchDocument, string(extInfo),
		)
		if err != nil {
			return affected, err
		}
		n, _ := res.RowsAffected()
		affected += n
	}
	return affected, nil
}

// ---- Phase C artifact file refresh ----

// loadPersistedMetricArtifactRows loads all metric rows for a record from the DB,
// including connected_artifacts populated by Phase C.
func loadPersistedMetricArtifactRows(ctx context.Context, db *sql.DB, recordID int64) ([]map[string]any, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT metric_id,
		       COALESCE(metric_name, ''),
		       COALESCE(metric_name_en, ''),
		       COALESCE(source_line_spans, '[]'::jsonb),
		       COALESCE(metric_subject, ''),
		       COALESCE(metric_subject_en, ''),
		       COALESCE(metric_desc, ''),
		       COALESCE(metric_desc_en, ''),
		       COALESCE(metric_context, ''),
		       COALESCE(metric_context_en, ''),
		       COALESCE(metric_keywords, '[]'::jsonb),
		       COALESCE(metric_keywords_en, '[]'::jsonb),
		       COALESCE(location_type, ''),
		       COALESCE(metric_unit, ''),
		       COALESCE(metric_unit_en, ''),
		       COALESCE(metric_value, ''),
		       COALESCE(value_data_type, ''),
		       COALESCE(value_range_type, ''),
		       COALESCE(value_class, ''),
		       COALESCE(value_class_en, ''),
		       COALESCE(formula_or_definition, ''),
		       COALESCE(threshold_or_target, ''),
		       COALESCE(measurement_frequency, ''),
		       COALESCE(confidence, 0),
		       COALESCE(is_explicit_metric, false),
		       COALESCE(table_name_or_section, ''),
		       COALESCE(reasoning_tags, '[]'::jsonb),
		       COALESCE(metric_categories, ''),
		       COALESCE(metric_categories_en, ''),
		       COALESCE(category_paths, '[]'::jsonb),
		       COALESCE(category_paths_en, '[]'::jsonb),
		       COALESCE(search_document, ''),
		       kb.connected_artifacts(input_record_id, 'metric', id)
		FROM kb.metrics
		WHERE input_record_id = $1
		ORDER BY id`, recordID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []map[string]any
	for rows.Next() {
		var (
			metricID, metricName, metricNameEn        string
			sourceLineSpansRaw                        []byte
			metricSubject, metricSubjectEn            string
			metricDesc, metricDescEn                  string
			metricContext, metricContextEn            string
			metricKeywordsRaw, metricKeywordsEnRaw    []byte
			locationType                              string
			metricUnit, metricUnitEn                  string
			metricValue                               string
			valueDataType, valueRangeType             string
			valueClass, valueClassEn                  string
			formulaOrDef, thresholdOrTarget, measFreq string
			confidence                                float64
			isExplicit                                bool
			tableNameOrSection                        string
			reasoningTagsRaw                          []byte
			metricCategories, metricCategoriesEn      string
			categoryPathsRaw, categoryPathsEnRaw      []byte
			searchDocument                            string
			connectedArtifactsRaw                     []byte
		)
		if err := rows.Scan(
			&metricID, &metricName, &metricNameEn,
			&sourceLineSpansRaw,
			&metricSubject, &metricSubjectEn,
			&metricDesc, &metricDescEn,
			&metricContext, &metricContextEn,
			&metricKeywordsRaw, &metricKeywordsEnRaw,
			&locationType,
			&metricUnit, &metricUnitEn,
			&metricValue,
			&valueDataType, &valueRangeType,
			&valueClass, &valueClassEn,
			&formulaOrDef, &thresholdOrTarget, &measFreq,
			&confidence, &isExplicit, &tableNameOrSection,
			&reasoningTagsRaw,
			&metricCategories, &metricCategoriesEn,
			&categoryPathsRaw, &categoryPathsEnRaw,
			&searchDocument,
			&connectedArtifactsRaw,
		); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"metric_id":             strings.TrimSpace(metricID),
			"metric_name":           strings.TrimSpace(metricName),
			"metric_name_en":        strings.TrimSpace(metricNameEn),
			"source_line_spans":     jsonColumnToValue(sourceLineSpansRaw, []any{}),
			"metric_subject":        strings.TrimSpace(metricSubject),
			"metric_subject_en":     strings.TrimSpace(metricSubjectEn),
			"metric_desc":           strings.TrimSpace(metricDesc),
			"metric_desc_en":        strings.TrimSpace(metricDescEn),
			"metric_context":        strings.TrimSpace(metricContext),
			"metric_context_en":     strings.TrimSpace(metricContextEn),
			"keywords":              jsonColumnToValue(metricKeywordsRaw, []any{}),
			"keywords_en":           jsonColumnToValue(metricKeywordsEnRaw, []any{}),
			"location_type":         strings.TrimSpace(locationType),
			"metric_unit":           strings.TrimSpace(metricUnit),
			"metric_unit_en":        strings.TrimSpace(metricUnitEn),
			"metric_value":          strings.TrimSpace(metricValue),
			"value_data_type":       strings.TrimSpace(valueDataType),
			"value_range_type":      strings.TrimSpace(valueRangeType),
			"value_class":           strings.TrimSpace(valueClass),
			"value_class_en":        strings.TrimSpace(valueClassEn),
			"formula_or_definition": strings.TrimSpace(formulaOrDef),
			"threshold_or_target":   strings.TrimSpace(thresholdOrTarget),
			"measurement_frequency": strings.TrimSpace(measFreq),
			"confidence":            confidence,
			"is_explicit_metric":    isExplicit,
			"table_name_or_section": strings.TrimSpace(tableNameOrSection),
			"reasoning_tags":        jsonColumnToValue(reasoningTagsRaw, []any{}),
			"metric_categories":     strings.TrimSpace(metricCategories),
			"metric_categories_en":  strings.TrimSpace(metricCategoriesEn),
			"category_paths":        jsonColumnToValue(categoryPathsRaw, []any{}),
			"category_paths_en":     jsonColumnToValue(categoryPathsEnRaw, []any{}),
			"search_document":       strings.TrimSpace(searchDocument),
			"connected_artifacts":   jsonColumnToValue(connectedArtifactsRaw, map[string]any{}),
		})
	}
	return out, rows.Err()
}

func refreshMetricArtifactFile(ctx context.Context, db *sql.DB, artifactDir string, recordID int64, rec DocMetadataInputRecord) error {
	rows, err := loadPersistedMetricArtifactRows(ctx, db, recordID)
	if err != nil {
		return err
	}
	return writeEntityRelationArtifactFile(artifactDir, recordID, rec, rows, ".metrics", "MID_26060703")
}

// ---- ChunkBatchProcessor implementation ----

// enrichMetricCandidates runs Pass 2 (concurrent enrichment) on the given
// candidates and returns the deduplicated enriched metric rows plus any
// uncertain metrics identified by the LLM. It is called from both
// extractMetricsFromChunksWithLLM (DRY) and FinalizeChunkBatch.
func (p *MetricsProcessor) enrichMetricCandidates(ctx context.Context, recordID int64, candidates []metricCandidate, _ string) ([]map[string]any, []map[string]any, error) {
	type pass2Result struct {
		metrics   []map[string]any
		uncertain []map[string]any
		language  string
	}

	maxTasks := p.MaxTasks
	if maxTasks <= 0 {
		maxTasks = 1
	}

	batches := groupCandidatesByChunk(candidates, p.MetricEnrichGroupSize)
	p.Logger.Info("enrich metric candidates (grouped)",
		"record_id", recordID,
		"model_name", p.RelationModelName,
		"prompt_name", p.RelationPromptRef,
		"total_batches", len(batches),
	)

	pass2Results, pass2Err := runConcurrent(ctx, maxTasks, len(batches), func(concCtx context.Context, i int) (pass2Result, error) {
		batch := batches[i]
		if isCtxStopped(concCtx) {
			return pass2Result{}, ErrPipelineStopped
		}
		batchStart := time.Now()
		p.Logger.Info("enrich metric start",
			"record_id", recordID,
			"batch", fmt.Sprintf("batch:%d/%d", i+1, len(batches)),
			"total_batches", len(batches),
		)
		enrichStart := p.Now()
		enrichCallID := fmt.Sprintf("%s_p2_b%d", eventIDFromContext(ctx), i+1)
		payload, err := p.extractMetricPayload(concCtx, buildMetricRelationBatchPrompt(batch.candidates),
			p.RelationPromptText, p.RelationModelName, p.RelationModelCfg, "enrich_metrics", "MID-26052903")
		p.logEnrichMetricsChunk(ctx, recordID, enrichCallID, batch.chunkIdx, len(batches),
			[]string{strings.TrimSpace(p.RelationModelName)}, p.RelationPromptRef,
			payload, err, enrichStart, p.Now(), "")
		if err != nil {
			if isCtxStopped(concCtx) {
				return pass2Result{}, ErrPipelineStopped
			}
			return pass2Result{}, fmt.Errorf("(MID_26042452) enrich metrics via llm: %w", err)
		}
		lang := strings.TrimSpace(asString(payload["language"]))
		metricsRaw, _ := payload["metrics"].([]any)
		uncertainRaw, _ := payload["uncertain_metrics"].([]any)
		result := pass2Result{
			metrics:   normalizeMetricList(metricsRaw),
			uncertain: normalizeMetricList(uncertainRaw),
			language:  lang,
		}
		backfillMetricResultCandidateIDs(result.metrics, batch.candidates)
		for j, m := range result.metrics {
			spans, _ := m["source_line_spans"].([]string)
			if len(spans) == 0 {
				p.Logger.Error("pass2: enriched metric has empty source_line_spans",
					"record_id", recordID,
					"batch", fmt.Sprintf("batch:%d/%d", i+1, len(batches)),
					"metric_index", j,
					"metric_name", m["metric_name"],
				)
			}
		}
		cacheHit, cacheMiss := cacheTokenCounts(p.Extractor)
		p.Logger.Info("enrich metric end  ",
			"record_id", recordID,
			"batch", fmt.Sprintf("batch:%d/%d", i+1, len(batches)),
			"metrics_in_batch", len(result.metrics),
			"ms_used", time.Since(batchStart).Milliseconds(),
			"cache_hit", cacheHit,
			"cache_miss", cacheMiss,
		)
		return result, nil
	})

	if pass2Err != nil {
		if isCtxStopped(ctx) {
			return nil, nil, ErrPipelineStopped
		}
		return nil, nil, pass2Err
	}

	metrics := make([]map[string]any, 0, len(candidates))
	uncertain := make([]map[string]any, 0)
	detectedLanguage := "unknown"
	for _, r := range pass2Results {
		metrics = append(metrics, r.metrics...)
		uncertain = append(uncertain, r.uncertain...)
		if r.language != "" && detectedLanguage == "unknown" {
			detectedLanguage = r.language
		}
	}
	metrics = dedupeFinalMetricRows(metrics)
	return metrics, uncertain, nil
}

func (p *MetricsProcessor) InitChunkBatch(ctx context.Context, recordID int64, chunks []Chunk, docCtx string) error {
	if p.MentionPromptErr != nil {
		return fmt.Errorf("(MID_26062750) %s candidate prompt error: %w", p.Name(), p.MentionPromptErr)
	}
	if p.ModelErr != nil {
		p.Logger.Warn("%s skipped: model config error", p.Name(), "record_id", recordID, "error", p.ModelErr)
		return nil
	}

	force, forceClear := docProcessorFlagsFromContext(ctx)
	p.batchForceClear = forceClear
	p.batchSkip = false
	p.batchExistingMetrics = nil

	p.Logger.Info("metrics InitChunkBatch", "record_id", recordID,
		"force", force, "force_clear", forceClear, "record_stage", "init_batch")

	if !force {
		exists, err := p.Store.MetricsExist(ctx, recordID)
		if err != nil {
			return fmt.Errorf("(MID_26071110) %s check metrics exist: %w", p.Name(), err)
		}
		if exists {
			p.Logger.Info("metrics extraction skipped", "record_id", recordID, "reason", "metrics already exist and force=false")
			p.batchSkip = true
			return nil
		}
	}

	if !forceClear {
		existing, err := p.Store.GetMetricsByInputRecordID(ctx, recordID)
		if err != nil {
			return fmt.Errorf("(MID_26071111) %s load existing metrics: %w", p.Name(), err)
		}
		p.Logger.Info("metrics merge: loaded existing metrics", "record_id", recordID,
			"existing_count", len(existing), "record_stage", "init_batch")
		p.batchExistingMetrics = existing
	}

	p.batchStart = p.Now()
	p.batchRecordID = recordID
	p.batchChunks = chunks
	p.batchDocCtx = docCtx
	p.batchMentions = nil
	p.batchLang = "unknown"
	p.batchModelName = strings.TrimSpace(p.MentionModelName)
	return nil
}

func (p *MetricsProcessor) ProcessChunk(ctx context.Context, chunkIdx int) error {
	if p.batchSkip {
		return nil
	}
	if chunkIdx < 0 || chunkIdx >= len(p.batchChunks) {
		return fmt.Errorf("(MID_26062751) %s chunk index %d out of range (len=%d)", p.Name(), chunkIdx, len(p.batchChunks))
	}
	if isCtxStopped(ctx) {
		return ErrPipelineStopped
	}
	chunk := p.batchChunks[chunkIdx]
	block := chunksToBlocks([]Chunk{chunk})[0]
	inputText := canonicalChunkInputText(chunk.Lines, p.batchDocCtx)
	taskPrompt := metricCandidateTask(p.MentionPromptText)
	callStart := p.Now()
	callID := fmt.Sprintf("%d_p1_c%d", p.batchRecordID, chunkIdx)
	p.Logger.Info("extract metrics start (batch)",
		"record_id", p.batchRecordID,
		"chunk", chunkIdx,
		"total", len(p.batchChunks),
		"model name", p.MentionModelName,
		"prompt name", p.MentionPromptRef,
	)
	payload, modelName, err := p.extractMetricCandidatePayloadWithFallback(ctx, inputText, taskPrompt)
	annotateMetricCandidatePayload(payload, block.Index)
	p.logExtractMetricsChunk(ctx, p.batchRecordID, callID, block.Index, len(p.batchChunks),
		[]string{strings.TrimSpace(modelName)}, p.MentionPromptRef, payload, err, callStart, p.Now(), "")
	if err != nil {
		if isCtxStopped(ctx) {
			return ErrPipelineStopped
		}
		p.Logger.Warn("%s chunk failed", p.Name(), "record_id", p.batchRecordID, "chunk", chunkIdx, "error", err)
		return nil
	}
	lang := ApiUtils.NormalizeLang(asString(payload["language"]))
	if lang == "chinese" || lang == "中文" || lang == "zh-cn" {
		lang = "zh"
	}
	raw, _ := payload["candidates"].([]any)
	cacheHit, cacheMiss := cacheTokenCounts(p.Extractor)
	p.Logger.Info("extract metrics end  (batch)",
		"record_id", p.batchRecordID,
		"extracted metrics", len(raw),
		"chunk", chunkIdx,
		"cache_hit", cacheHit,
		"cache_miss", cacheMiss,
		"ms_used", time.Since(callStart).Milliseconds(),
	)
	p.batchMu.Lock()
	if lang != "" && p.batchLang == "unknown" {
		p.batchLang = lang
	}
	if m := strings.TrimSpace(modelName); m != "" {
		p.batchModelName = m
	}
	p.batchMentions = append(p.batchMentions, normalizeMetricCandidateMentions(raw, block)...)
	p.batchMu.Unlock()
	return nil
}

func (p *MetricsProcessor) FinalizeChunkBatch(ctx context.Context) error {
	if p.batchSkip {
		return nil
	}
	candidates := mentionsAsCandidates(p.batchMentions)
	if len(candidates) == 0 {
		p.Logger.Info("%s batch: no candidates", p.Name(), "record_id", p.batchRecordID)
		return nil
	}
	if isCtxStopped(ctx) {
		return ErrPipelineStopped
	}
	metrics, _, err := p.enrichMetricCandidates(ctx, p.batchRecordID, candidates, p.batchDocCtx)
	if err != nil {
		if errors.Is(err, ErrPipelineStopped) {
			return ErrPipelineStopped
		}
		return fmt.Errorf("(MID_26062752) %s enrich metrics: %w", p.Name(), err)
	}
	rec, err := p.InputStore.GetInputRecord(ctx, p.batchRecordID)
	if err != nil {
		return fmt.Errorf("(MID_26062753) %s load record: %w", p.Name(), err)
	}

	if p.batchForceClear {
		for i, m := range metrics {
			m["metric_id"] = fmt.Sprintf("%d_mtc_%d", p.batchRecordID, i+1)
			metrics[i] = m
		}
		deleted, _ := p.Store.DeleteMetricsByInputRecordID(ctx, p.batchRecordID)
		p.Logger.Info("metrics wipe (force_clear=true)", "record_id", p.batchRecordID,
			"deleted_rows", deleted, "new_count", len(metrics))
		if _, err := p.Store.SaveMetrics(ctx, SaveMetricsRequest{
			InputRecordID: p.batchRecordID,
			EventID:       eventIDFromContext(ctx),
			Language:      firstNonEmptyTrimmed(p.batchLang, "unknown"),
			ModelName:     firstNonEmptyTrimmed(p.batchModelName, p.MentionModelName),
			PromptName:    p.RelationPromptRef,
			Metrics:       metrics,
		}); err != nil {
			return fmt.Errorf("(MID_26062754) %s save metrics: %w", p.Name(), err)
		}
		p.logFinalMetricsSaved(ctx, p.batchRecordID, firstNonEmptyTrimmed(p.batchLang, "unknown"),
			[]string{firstNonEmptyTrimmed(p.batchModelName, p.MentionModelName)}, p.RelationPromptRef,
			metrics, p.batchStart, p.Now())
		if fileErr := p.saveMetricsToFile(p.batchRecordID, rec, metrics); fileErr != nil {
			p.Logger.Warn("save metrics to file failed", "record_id", p.batchRecordID, "error", fileErr)
		}
		return nil
	}

	p.Logger.Info("metrics merge (force_clear=false)", "record_id", p.batchRecordID,
		"existing_count", len(p.batchExistingMetrics), "new_enriched_count", len(metrics),
		"record_stage", "merge_start")

	dirty, err := p.mergeAndCollectDirtyMetrics(ctx, metrics)
	if err != nil {
		return fmt.Errorf("(MID_26071112) %s merge metrics: %w", p.Name(), err)
	}
	if len(dirty) > 0 {
		inserted, err := p.Store.UpsertMetrics(ctx, SaveMetricsRequest{
			InputRecordID: p.batchRecordID,
			EventID:       eventIDFromContext(ctx),
			Language:      firstNonEmptyTrimmed(p.batchLang, "unknown"),
			ModelName:     firstNonEmptyTrimmed(p.batchModelName, p.MentionModelName),
			PromptName:    p.RelationPromptRef,
			Metrics:       dirty,
		})
		if err != nil {
			return fmt.Errorf("(MID_26071113) %s upsert metrics: %w", p.Name(), err)
		}
		p.Logger.Info("metrics upserted (merge mode)", "record_id", p.batchRecordID,
			"dirty_count", len(dirty), "rows_affected", inserted)
	} else {
		p.Logger.Info("metrics merge: nothing dirty", "record_id", p.batchRecordID,
			"existing_count", len(p.batchExistingMetrics))
	}
	p.logFinalMetricsSaved(ctx, p.batchRecordID, firstNonEmptyTrimmed(p.batchLang, "unknown"),
		[]string{firstNonEmptyTrimmed(p.batchModelName, p.MentionModelName)}, p.RelationPromptRef,
		dirty, p.batchStart, p.Now())
	if fileErr := p.saveMetricsToFile(p.batchRecordID, rec, dirty); fileErr != nil {
		p.Logger.Warn("save metrics to file failed", "record_id", p.batchRecordID, "error", fileErr)
	}
	return nil
}

// mergeResolveDecidedFields lists the fields the Merge Resolution LLM call's
// winning_metrics response actually returns (prompt-merge-resolve-metrics-v1.md).
// Everything else on a Rule-4 winner must come from the full source row, not
// this sparse LLM output, or those columns get silently blanked on write.
var mergeResolveDecidedFields = []string{
	"metric_name", "metric_subject", "metric_unit", "metric_value",
	"value_data_type", "value_range_type", "value_class",
	"threshold_or_target", "metric_categories", "source_line_spans",
}

// metricFieldAliasPairs lists the (raw, canonical) field-name pairs that
// differ between the two shapes a metric map can arrive in:
//   - "raw" names (subject/unit/desc/context, ...) as produced by
//     enrichMetricCandidates (pass-2 LLM output, normalizeMetricList) and
//     required by SaveMetrics/UpsertMetrics when writing a row.
//   - "canonical" DB-column-style names (metric_subject/metric_unit/
//     metric_desc/metric_context, ...) as produced by
//     MetricsStore.GetMetricsByInputRecordID and required by mergeMetrics/
//     metricsIdentical/metricContentEqual/resolveMergeAmbiguities for
//     comparison and for the Merge Resolution LLM call's candidate payload.
//
// Every other field mergeMetrics/metricContentEqual compares (metric_name,
// metric_value, value_data_type, value_range_type, value_class,
// threshold_or_target, source_line_spans) already uses the same key name in
// both shapes.
var metricFieldAliasPairs = [][2]string{
	{"subject", "metric_subject"},
	{"subject_en", "metric_subject_en"},
	{"unit", "metric_unit"},
	{"unit_en", "metric_unit_en"},
	{"desc", "metric_desc"},
	{"desc_en", "metric_desc_en"},
	{"context", "metric_context"},
	{"context_en", "metric_context_en"},
	{"keywords", "metric_keywords"},
	{"keywords_en", "metric_keywords_en"},
}

// canonicalizeMetricFieldAliases returns a copy of m with both the raw and
// canonical spellings of subject/unit/desc/context/keywords populated from whichever
// one is present. Without this bridge, Rule-2's exact-duplicate check would
// never fire for freshly-enriched candidates (their "metric_subject" key is
// always empty, so they'd never match an existing row's real subject), and
// any previously-existing row that must be re-written by UpsertMetrics after
// a merge would have its subject/desc/context/unit/keywords silently blanked out
// (UpsertMetrics reads the raw names only).
func canonicalizeMetricFieldAliases(m map[string]any) map[string]any {
	out := cloneMetricMap(m)
	for _, pair := range metricFieldAliasPairs {
		rawKey, canonKey := pair[0], pair[1]
		rawVal := strings.TrimSpace(asString(out[rawKey]))
		canonVal := strings.TrimSpace(asString(out[canonKey]))
		if rawVal == "" && canonVal != "" {
			out[rawKey] = out[canonKey]
		} else if canonVal == "" && rawVal != "" {
			out[canonKey] = out[rawKey]
		}
	}
	return out
}

// mergeAndCollectDirtyMetrics runs DR2 (Rule-2/3/4) against this run's newly
// enriched metrics and the existing metrics loaded in InitChunkBatch, resolves
// any ambiguous groups via the Merge Resolution LLM call (DR4), and returns
// only the metrics that are new or actually changed content — each already
// carrying an ext_info.merge_log entry.
func (p *MetricsProcessor) mergeAndCollectDirtyMetrics(ctx context.Context, newMetrics []map[string]any) ([]map[string]any, error) {
	existing := make([]map[string]any, 0, len(p.batchExistingMetrics))
	for _, ex := range p.batchExistingMetrics {
		existing = append(existing, canonicalizeMetricFieldAliases(ex))
	}
	candidates := make([]map[string]any, 0, len(newMetrics))
	for _, m := range newMetrics {
		candidates = append(candidates, canonicalizeMetricFieldAliases(m))
	}

	seqno := newMetricSeqnoCounter(existing)
	merged := mergeMetrics(existing, candidates, seqno, p.batchRecordID)

	var dirty []map[string]any
	now := p.Now()

	for _, added := range merged.Added {
		added["ext_info"] = map[string]any{"merge_log": []map[string]any{
			{"run_time": now.Format(time.RFC3339), "action": "added"},
		}}
		dirty = append(dirty, added)
	}

	existingByID := map[string]map[string]any{}
	for _, ex := range existing {
		existingByID[asString(ex["metric_id"])] = ex
	}

	for _, group := range merged.PendingGroups {
		p.Logger.Info("merge resolve: sending pending group to LLM", "record_id", p.batchRecordID,
			"candidates_in_group", len(group))
		winners, err := p.resolveMergeAmbiguities(ctx, p.batchRecordID, group)
		if err != nil {
			return nil, err
		}
		p.Logger.Info("merge resolve: LLM returned winners", "record_id", p.batchRecordID,
			"winners_count", len(winners))
		groupByID := map[string]map[string]any{}
		for _, g := range group {
			groupByID[asString(g["metric_id"])] = g
		}
		for _, w := range winners {
			w = canonicalizeMetricFieldAliases(w)
			absorbed, _ := w["absorbed_metric_ids"].([]any)
			id := asString(w["metric_id"])
			existingRow, wasExisting := existingByID[id]
			if len(absorbed) == 0 && wasExisting && metricContentEqual(existingRow, w) {
				continue // DR4 dirty-check: unchanged existing row, skip entirely.
			}
			if full, ok := groupByID[id]; ok {
				reconstructed := cloneMetricMap(full)
				for _, f := range mergeResolveDecidedFields {
					if v, ok := w[f]; ok {
						reconstructed[f] = v
					}
				}
				reconstructed["metric_id"] = id
				w = reconstructed
			}
			assignMergedMetricCandidateID(w, groupByID, absorbed)
			action := "added"
			if wasExisting {
				action = "merged"
			}
			absorbedIDs := make([]string, 0, len(absorbed))
			for _, a := range absorbed {
				absorbedIDs = append(absorbedIDs, asString(a))
			}
			logEntry := map[string]any{"run_time": now.Format(time.RFC3339), "action": action}
			if len(absorbedIDs) > 0 {
				logEntry["absorbed_metric_ids"] = absorbedIDs
			}
			w["ext_info"] = map[string]any{"merge_log": []map[string]any{logEntry}}
			dirty = append(dirty, w)
		}
	}
	p.Logger.Info("metrics merge complete", "record_id", p.batchRecordID,
		"existing_loaded", len(existing), "new_enriched", len(newMetrics),
		"rule3_added", len(merged.Added), "rule4_pending_groups", len(merged.PendingGroups),
		"dirty_result", len(dirty), "record_stage", "merge_complete")
	return dirty, nil
}

// metricContentEqual compares the fields the merge pipeline actually produces
// (DR4 dirty-check) — deliberately excludes metric_id, ext_info, and any
// internal "_merge_source" tag.
func metricContentEqual(existing, candidate map[string]any) bool {
	fields := []string{"metric_name", "metric_subject", "metric_unit", "metric_value",
		"value_data_type", "value_range_type", "value_class", "threshold_or_target"}
	for _, f := range fields {
		if strings.TrimSpace(asString(existing[f])) != strings.TrimSpace(asString(candidate[f])) {
			return false
		}
	}
	existingSpans := strings.Join(normalizeSourceLineSpans(existing["source_line_spans"]), ",")
	candidateSpans := strings.Join(normalizeSourceLineSpans(candidate["source_line_spans"]), ",")
	if existingSpans != candidateSpans {
		return false
	}
	existingCats := strings.Join(metricCategoryKeysFromValue(existing["metric_categories"]), ",")
	candidateCats := strings.Join(metricCategoryKeysFromValue(candidate["metric_categories"]), ",")
	return existingCats == candidateCats
}
