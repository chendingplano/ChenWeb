package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/ApiUtils"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

type SemanticProjectionsProcessor struct {
	InputStore            DocMetadataStore
	Store                 SemanticProjectionsStore
	Extractor             LLMJSONExtractor
	Logger                ApiTypes.JimoLogger
	ProcLogger            DocProcLogger
	Now                   func() time.Time
	CandidatePromptText   string
	CandidatePromptRef    string
	CandidatePromptPath   string
	CandidatePromptErr    error
	CandidateModelRef     string
	CandidateModelCfgPath string
	CandidateModelErr     error
	CandidateModelName    string
	CandidateModelCfg     structureModelConfig
	FallbackModelRef      string
	FallbackModelCfgPath  string
	FallbackModelErr      error
	FallbackModelName     string
	FallbackModelCfg      structureModelConfig
	EnrichPromptText      string
	EnrichPromptRef       string
	EnrichPromptPath      string
	EnrichPromptErr       error
	EnrichModelRef        string
	EnrichModelCfgPath    string
	EnrichModelErr        error
	EnrichModelName       string
	EnrichModelCfg        structureModelConfig
	ArtifactDir           string
	ArtifactWebDir        string
	MaxTasks              int
}

type SemanticProjectionsStore interface {
	SemanticProjectionsExist(ctx context.Context, inputRecordID int64) (bool, error)
	DeleteSemanticProjectionsByInputRecordID(ctx context.Context, inputRecordID int64) (int64, error)
	SaveSemanticProjections(ctx context.Context, req SaveSemanticProjectionsRequest) (int64, error)
}

type SemanticProjectionsSQLStore struct {
	DB *sql.DB
}

type SaveSemanticProjectionsRequest struct {
	InputRecordID int64
	EventID       string
	Language      string
	ModelName     string
	PromptName    string
	Projections   []map[string]any
}

type semanticProjectionExtractionResult struct {
	Projections   []map[string]any
	SeqNos        []int
	LineSpans     [][]string
	Language      string
	ModelName     string
	LLMCallCount  int
	FallbackCount int
}

type semanticProjectionCandidate struct {
	SeqNo              int
	SemanticProjection string
	Keywords           []string
	ChunkLines         []MarkedLine
}

func NewSemanticProjectionsProcessor(
	inputStore DocMetadataStore,
	store SemanticProjectionsStore,
	extractor LLMJSONExtractor,
	_ ApiTypes.JimoLogger,
) *SemanticProjectionsProcessor {
	logger := loggerutil.CreateDefaultLogger("MID_26052101")
	candidatePromptText, candidatePromptRef, candidatePromptPath, candidatePromptErr := loadProductPromptFromEnvKeys(
		[]string{"EXTRACT_SEMANTIC_PROJECTION_PROMPT"},
		"prompt-extract-semantic-projection-v1.md",
	)
	enrichPromptText, enrichPromptRef, enrichPromptPath, enrichPromptErr := loadProductPromptFromEnvKeys(
		[]string{"ENRICH_SEMANTIC_PROJECTION_PROMPT"},
		"prompt-enrich-semantic-projection-v1.md",
	)
	candidateModelRef, candidateModelCfgPath, candidateModelCfg, candidateModelErr := loadModelConfigFromEnvKeys(
		[]string{"EXTRACT_SEMANTIC_PROJECTION_MODEL_NAME"},
		"MODEL_DEF_FILE",
	)
	fallbackModelRef, fallbackModelCfgPath, fallbackModelCfg, fallbackModelErr := loadOptionalModelConfigFromEnv(
		"EXTRACT_SEMANTIC_PROJECTION_MODEL_FALLBACK",
		"MODEL_DEF_FILE",
	)
	enrichModelRef, enrichModelCfgPath, enrichModelCfg, enrichModelErr := loadModelConfigFromEnvKeys(
		[]string{"ENRICH_SEMANTIC_PROJECTION_MODEL_NAME"},
		"MODEL_DEF_FILE",
	)
	applyStructureModelConfigToExtractor(extractor, enrichModelCfg)
	return &SemanticProjectionsProcessor{
		InputStore:            inputStore,
		Store:                 store,
		Extractor:             extractor,
		Logger:                logger,
		ProcLogger:            DocProcLogger{DB: ApiTypes.ProjectDBHandle},
		Now:                   time.Now,
		CandidatePromptText:   candidatePromptText,
		CandidatePromptRef:    candidatePromptRef,
		CandidatePromptPath:   candidatePromptPath,
		CandidatePromptErr:    candidatePromptErr,
		CandidateModelRef:     candidateModelRef,
		CandidateModelCfgPath: candidateModelCfgPath,
		CandidateModelErr:     candidateModelErr,
		CandidateModelName:    candidateModelCfg.ModelName,
		CandidateModelCfg:     candidateModelCfg,
		FallbackModelRef:      fallbackModelRef,
		FallbackModelCfgPath:  fallbackModelCfgPath,
		FallbackModelErr:      fallbackModelErr,
		FallbackModelName:     fallbackModelCfg.ModelName,
		FallbackModelCfg:      fallbackModelCfg,
		EnrichPromptText:      enrichPromptText,
		EnrichPromptRef:       enrichPromptRef,
		EnrichPromptPath:      enrichPromptPath,
		EnrichPromptErr:       enrichPromptErr,
		EnrichModelRef:        enrichModelRef,
		EnrichModelCfgPath:    enrichModelCfgPath,
		EnrichModelErr:        enrichModelErr,
		EnrichModelName:       enrichModelCfg.ModelName,
		EnrichModelCfg:        enrichModelCfg,
		ArtifactDir:           strings.TrimSpace(os.Getenv("ARTIFACT_DIR")),
		ArtifactWebDir:        strings.TrimSpace(os.Getenv("ARTIFACT_WEB_DIR")),
		MaxTasks:              envInt("EXTRACT_SEMANTIC_PROJECTIONS_MAX_TASKS", 1, 1),
	}
}

func (p *SemanticProjectionsProcessor) Name() string { return "extract_semantic_projections" }

func (p *SemanticProjectionsProcessor) HandleEvent(ctx context.Context, payload []byte) error {
	start := p.Now()
	evt, err := ParseLineFileGeneratedEvent(payload)
	if err != nil {
		p.Logger.Error("parse input file failed", "error", err)
		return fmt.Errorf("(MID_26052102) parse event payload: %w", err)
	}
	if ShouldSkipLineFileGeneratedEvent(evt) {
		p.Logger.Warn("semantic projections processor skipped")
		return nil
	}
	if p.CandidatePromptErr != nil {
		p.Logger.Error("candidate prompt error", "error", p.CandidatePromptErr)
		return fmt.Errorf("(MID_26052103) load semantic projection candidate prompt %q: %w", p.CandidatePromptRef, p.CandidatePromptErr)
	}
	if p.EnrichPromptErr != nil {
		p.Logger.Error("enrich prompt error", "error", p.EnrichPromptErr)
		return fmt.Errorf("(MID_26052104) load semantic projection enrich prompt %q: %w", p.EnrichPromptRef, p.EnrichPromptErr)
	}

	rec, err := p.InputStore.GetInputRecord(ctx, evt.RecordID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			p.Logger.Error("kb.inputs record not found", "record_id", evt.RecordID)
			return nil
		}
		p.Logger.Error("retrieve record failed", "record_id", evt.RecordID, "error", err)
		return fmt.Errorf("(MID_26052105) load kb.inputs record %d: %w", evt.RecordID, err)
	}
	if p.CandidateModelErr != nil {
		p.Logger.Warn("semantic projections extraction skipped: candidate model config error",
			"record_id", evt.RecordID, "model_ref", p.CandidateModelRef, "error", p.CandidateModelErr)
		p.persistSemanticProjectionsStatus(ctx, rec, start, p.CandidateModelErr)
		return nil
	}
	if p.EnrichModelErr != nil {
		p.Logger.Warn("semantic projections extraction skipped: enrich model config error",
			"record_id", evt.RecordID, "model_ref", p.EnrichModelRef, "error", p.EnrichModelErr)
		p.persistSemanticProjectionsStatus(ctx, rec, start, p.EnrichModelErr)
		return nil
	}

	lineFilePath, lineFileErr := ResolveInputFilePath(evt, rec.ResultFilename, rec.ParserName, rec.StagingFilename)
	if lineFileErr != nil {
		p.Logger.Error("resolve input file path failed", "record_id", evt.RecordID, "error", lineFileErr)
		p.persistSemanticProjectionsStatus(ctx, rec, start, fmt.Errorf("(MID_26052106) resolve line file for record_id=%d: %w", evt.RecordID, lineFileErr))
		return nil
	}

	if evt.Force {
		_, _ = p.Store.DeleteSemanticProjectionsByInputRecordID(ctx, evt.RecordID)
	} else {
		exists, err := p.Store.SemanticProjectionsExist(ctx, evt.RecordID)
		if err != nil {
			p.Logger.Error("check semantic projections exist failed", "record_id", evt.RecordID, "error", err)
			p.persistSemanticProjectionsStatus(ctx, rec, start, err)
			return fmt.Errorf("(MID_26052107) check semantic projections for record_id=%d: %w", evt.RecordID, err)
		}
		if exists {
			p.Logger.Info("semantic projections extraction skipped",
				"record_id", evt.RecordID, "reason", "semantic projections already exist and force=false")
			reindexExistingSearchOnSkip(ctx, searchArtifactSemanticProjection, evt.RecordID, p.Logger, ReindexSemanticProjectionSearchForRecord)
			p.persistSemanticProjectionsStatus(ctx, rec, start, nil)
			return nil
		}
	}

	body, readErr := os.ReadFile(lineFilePath)
	if readErr != nil {
		p.Logger.Error("read line file failed", "record_id", evt.RecordID, "path", lineFilePath, "error", readErr)
		p.persistSemanticProjectionsStatus(ctx, rec, start, fmt.Errorf("(MID_26052108) read line file for record_id=%d: %w", evt.RecordID, readErr))
		return fmt.Errorf("(MID_26052109) failed reading line file, error:%w, path:%s", readErr, lineFilePath)
	}
	lines, parseErr := ParseInputLinesIncludingTOC(body)
	if parseErr != nil {
		p.Logger.Error("parse input lines failed", "record_id", evt.RecordID, "error", parseErr)
		p.persistSemanticProjectionsStatus(ctx, rec, start, fmt.Errorf("(MID_26052110) parse input lines for record_id=%d: %w", evt.RecordID, parseErr))
		return fmt.Errorf("(MID_26052111) failed parsing input lines, error:%w", parseErr)
	}
	artifactBase := buildChunkArtifactBaseName(rec.StagingFilename, rec.ParserName)
	chunks, loadErr := loadChunksFromArtifactFile(p.ArtifactDir, evt.RecordID, artifactBase+".chunks", lines)
	if loadErr != nil {
		p.Logger.Error("load chunk artifact failed", "record_id", evt.RecordID, "artifact_base", artifactBase, "error", loadErr)
		p.persistSemanticProjectionsStatus(ctx, rec, start, fmt.Errorf("(MID_26052112) load chunk artifact for record_id=%d: %w", evt.RecordID, loadErr))
		return fmt.Errorf("(MID_26052113) failed loading chunk artifact, error:%w", loadErr)
	}
	if len(chunks) == 0 {
		procErr := fmt.Errorf("(MID_26052114) no chunks found for record_id=%d", evt.RecordID)
		p.Logger.Error("no chunks", "record_id", evt.RecordID, "artifact_base", artifactBase)
		p.persistSemanticProjectionsStatus(ctx, rec, start, procErr)
		return nil
	}

	p.persistSemanticProjectionsInProgressStatus(ctx, rec, start, fmt.Sprintf("0%% (0/%d)", len(chunks)))
	result, err := p.extractSemanticProjectionsFromChunks(ctx, evt.RecordID, chunks)
	if err != nil {
		if errors.Is(err, ErrPipelineStopped) {
			p.stopAndPersistSemanticProjections(context.Background(), rec, start)
			return ErrPipelineStopped
		}
		p.persistSemanticProjectionsStatus(ctx, rec, start, err)
		return fmt.Errorf("(MID_26052115) extractSemanticProjections failed, error:%w", err)
	}

	detectedLanguage := result.Language
	if detectedLanguage == "" {
		detectedLanguage = "unknown"
	}
	createTime := p.Now().Format(defaultDocMetaStatusTime)
	for i, proj := range result.Projections {
		proj["semantic_proj_id"] = fmt.Sprintf("%d_0_%d", evt.RecordID, result.SeqNos[i])
		proj["line_spans"] = result.LineSpans[i]
		proj["create_time"] = createTime
		result.Projections[i] = proj
	}

	inserted, err := p.Store.SaveSemanticProjections(ctx, SaveSemanticProjectionsRequest{
		InputRecordID: evt.RecordID,
		EventID:       eventIDFromContext(ctx),
		Language:      detectedLanguage,
		ModelName:     firstNonEmptyTrimmed(result.ModelName, p.EnrichModelName),
		PromptName:    p.EnrichPromptRef,
		Projections:   result.Projections,
	})
	if err != nil {
		p.persistSemanticProjectionsStatus(ctx, rec, start, err)
		return fmt.Errorf("(MID_26052114) insert semantic projections failed, error:%w", err)
	}

	if fileErr := p.saveSemanticProjectionsToFile(evt.RecordID, rec, result.Projections); fileErr != nil {
		p.Logger.Warn("save semantic projections to file failed", "record_id", evt.RecordID, "error", fileErr)
	}

	if indexErr := p.indexSemanticProjections(evt.RecordID, result.Projections); indexErr != nil {
		p.Logger.Warn("index semantic projections failed", "record_id", evt.RecordID, "error", indexErr)
	}

	if reindexErr := ReindexSemanticProjectionSearchForRecord(ctx, evt.RecordID, p.Logger); reindexErr != nil {
		p.Logger.Warn("reindex semantic projection search registry failed", "record_id", evt.RecordID, "error", reindexErr)
	}

	p.Logger.Info("semantic projections extracted",
		"record_id", evt.RecordID,
		"inserted_rows", inserted,
		"projections_count", len(result.Projections),
		"num_chunks", len(chunks),
	)
	p.persistSemanticProjectionsStatus(ctx, rec, start, nil)
	p.logSemanticProjectionsSummary(ctx, evt.RecordID, start, p.Now(), len(chunks))
	return nil
}

type semanticProjectionWorkerResult struct {
	seqNo          int
	projection     map[string]any // nil when chunk was skipped (empty semantic_projection)
	lineSpans      []string
	language       string
	candidateModel string
	llmCallCount   int
	fallbackCount  int
}

func (p *SemanticProjectionsProcessor) extractSemanticProjectionsFromChunks(
	ctx context.Context,
	recordID int64,
	chunks []Chunk,
) (semanticProjectionExtractionResult, error) {
	var completedP1, completedP2 atomic.Int32

	p.Logger.Info("semantic projection",
		"record_id", recordID,
		"model_names", fmt.Sprintf("%s/%s", p.CandidateModelName, p.EnrichModelName),
		"prompts", fmt.Sprintf("%s/%s", p.CandidatePromptRef, p.EnrichPromptRef),
	)

	workerResults, runErr := runConcurrent(ctx, p.MaxTasks, len(chunks), func(jobCtx context.Context, i int) (semanticProjectionWorkerResult, error) {
		chunk := chunks[i]
		if isCtxStopped(jobCtx) {
			return semanticProjectionWorkerResult{}, ErrPipelineStopped
		}

		lineSpans := chunkLineSpans(chunk.Lines)

		// Pass 1: extract semantic projection candidate
		chunkText := buildMarkedChunkInputText(chunk.Lines)
		callStart := p.Now()
		p.Logger.Info("semantic proj start       ",
			"record_id", recordID,
			"chunk_idx", i,
			"seq_no", chunk.SeqNo,
			"num_lines", len(chunk.Lines),
		)
		callID := fmt.Sprintf("%s_p1_c%d", eventIDFromContext(ctx), i)
		userPrompt := buildSemanticProjectionCandidateUserPrompt(chunkText)
		payload, modelName, err := p.extractCandidatePayloadWithFallback(jobCtx, userPrompt)
		llmCallCount := 1
		fallbackCount := 0
		if strings.TrimSpace(modelName) != strings.TrimSpace(p.CandidateModelName) && strings.TrimSpace(modelName) != "" {
			fallbackCount++
		}
		var completedChunksForLog int
		if err == nil {
			completedChunksForLog = int(completedP1.Add(1))
		} else {
			completedChunksForLog = int(completedP1.Load())
		}
		p.logExtractProjectionsChunk(jobCtx, recordID, callID, i+1, len(chunks), completedChunksForLog,
			[]string{strings.TrimSpace(modelName)}, p.CandidatePromptRef,
			payload, err, callStart, p.Now())
		if err != nil {
			p.Logger.Error("raw candidate LLM payload/error",
				"record_id", recordID,
				"chunk_idx", i,
				"seq_no", chunk.SeqNo,
				"error", err,
			)
			return semanticProjectionWorkerResult{},
				fmt.Errorf("(MID_26052120) extract semantic projection candidate for chunk seq=%d: %w", chunk.SeqNo, err)
		}
		p.Logger.Info("semantic proj end         ",
			"record_id", recordID,
			"chunk_idx", i,
			"seq_no", chunk.SeqNo,
			"semantic_projection_len", len(strings.TrimSpace(asString(payload["semantic_projection"]))),
			"keywords_count", len(toStringSlice(payload["keywords"])),
			"ms_used", time.Since(callStart).Milliseconds(),
		)

		usedCandidateModel := strings.TrimSpace(firstNonEmptyTrimmed(modelName, p.CandidateModelName))
		spText := strings.TrimSpace(asString(payload["semantic_projection"]))
		if spText == "" {
			p.Logger.Warn("empty semantic projection in candidate pass; skipping chunk",
				"record_id", recordID, "seq_no", chunk.SeqNo)
			return semanticProjectionWorkerResult{
				seqNo:          chunk.SeqNo,
				lineSpans:      lineSpans,
				candidateModel: usedCandidateModel,
				llmCallCount:   llmCallCount,
				fallbackCount:  fallbackCount,
			}, nil
		}

		cand := semanticProjectionCandidate{
			SeqNo:              chunk.SeqNo,
			SemanticProjection: spText,
			Keywords:           toStringSlice(payload["keywords"]),
			ChunkLines:         append([]MarkedLine(nil), chunk.Lines...),
		}

		// Pass 2: enrich the candidate
		if isCtxStopped(jobCtx) {
			return semanticProjectionWorkerResult{}, ErrPipelineStopped
		}
		callStart2 := p.Now()
		p.Logger.Info("semantic proj enrich start",
			"record_id", recordID,
			"seq_no", cand.SeqNo,
		)
		callID2 := fmt.Sprintf("%s_p2_c%d", eventIDFromContext(ctx), i)
		userPrompt2 := buildSemanticProjectionEnrichUserPrompt(cand)
		payload2, err := p.extractEnrichPayload(jobCtx, userPrompt2)
		llmCallCount++
		var completedP2ForLog int
		if err == nil {
			completedP2ForLog = int(completedP2.Add(1))
		} else {
			completedP2ForLog = int(completedP2.Load())
		}
		p.logEnrichProjectionsChunk(jobCtx, recordID, callID2, i+1, len(chunks), completedP2ForLog,
			[]string{strings.TrimSpace(p.EnrichModelName)}, p.EnrichPromptRef,
			payload2, err, callStart2, p.Now())
		if err != nil {
			p.Logger.Error("enrichment payload error",
				"record_id", recordID,
				"seq_no", cand.SeqNo,
				"error", err,
			)
			return semanticProjectionWorkerResult{},
				fmt.Errorf("(MID_26052121) enrich semantic projection seq=%d: %w", cand.SeqNo, err)
		}

		lang := ApiUtils.NormalizeLang(strings.TrimSpace(asString(payload2["language"])))
		p.Logger.Info("semantic proj end         ",
			"record_id", recordID,
			"seq_no", cand.SeqNo,
			"language", lang,
			"descriptive_name", asString(payload2["descriptive_name"]),
			"ms_used", time.Since(callStart2).Milliseconds(),
		)
		normalized := normalizeSemanticProjection(payload2)
		return semanticProjectionWorkerResult{
			seqNo:          chunk.SeqNo,
			projection:     normalized,
			lineSpans:      lineSpans,
			language:       lang,
			candidateModel: usedCandidateModel,
			llmCallCount:   llmCallCount,
			fallbackCount:  fallbackCount,
		}, nil
	})

	if runErr != nil {
		if errors.Is(runErr, ErrPipelineStopped) {
			return semanticProjectionExtractionResult{}, ErrPipelineStopped
		}
		return semanticProjectionExtractionResult{}, runErr
	}

	// Collect results in chunk-index (SeqNo) order — runConcurrent preserves original index order.
	projections := make([]map[string]any, 0, len(workerResults))
	seqNos := make([]int, 0, len(workerResults))
	lineSpansList := make([][]string, 0, len(workerResults))
	detectedLanguage := "unknown"
	var llmCallCount, fallbackCount int
	usedCandidateModel := strings.TrimSpace(p.CandidateModelName)

	for _, r := range workerResults {
		llmCallCount += r.llmCallCount
		fallbackCount += r.fallbackCount
		if r.candidateModel != "" {
			usedCandidateModel = r.candidateModel
		}
		if r.projection == nil {
			continue
		}
		if r.language != "" && detectedLanguage == "unknown" {
			detectedLanguage = r.language
		}
		projections = append(projections, r.projection)
		seqNos = append(seqNos, r.seqNo)
		lineSpansList = append(lineSpansList, r.lineSpans)
	}

	p.Logger.Info("semantic projections final results",
		"record_id", recordID,
		"total_chunks", len(chunks),
		"candidates", len(projections),
		"projections", len(projections),
		"language", detectedLanguage,
	)
	return semanticProjectionExtractionResult{
		Projections:   projections,
		SeqNos:        seqNos,
		LineSpans:     lineSpansList,
		Language:      detectedLanguage,
		ModelName:     firstNonEmptyTrimmed(p.EnrichModelName, usedCandidateModel),
		LLMCallCount:  llmCallCount,
		FallbackCount: fallbackCount,
	}, nil
}

func (p *SemanticProjectionsProcessor) logExtractProjectionsChunk(
	ctx context.Context,
	recordID int64,
	callID string,
	chunkNo, totalChunks, completedChunks int,
	modelNames []string,
	promptName string,
	payload map[string]any,
	callErr error,
	start, end time.Time,
) {
	overallPercent := 0
	perPassPercent := 0
	if totalChunks > 0 {
		overallPercent = completedChunks * 100 / totalChunks / 2
		perPassPercent = chunkNo * 100 / totalChunks
	}
	progressStr := fmt.Sprintf("%d%% (pass 1: %d/%d)", overallPercent, completedChunks, totalChunks)
	extraInfo := map[string]any{
		"chunk":        chunkNo,
		"total_chunks": totalChunks,
		"percent":      fmt.Sprintf("%d%% (%d/%d)", perPassPercent, chunkNo, totalChunks),
	}
	extraBytes, _ := json.Marshal(extraInfo)
	extraStr := string(extraBytes)

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
	pass := 1
	activity := "extract_projection"
	rec := DocProcLogRecord{
		CallReason:    "extract semantic projections",
		DocProcName:   p.Name(),
		ModelNames:    modelNames,
		PromptName:    promptName,
		RecordID:      &recordID,
		ProcProgress:  &progressStr,
		Pass:          &pass,
		LLMCallID:     &callID,
		ActivityName:  &activity,
		ArtifactJSON:  artifactStr,
		Errors:        errStr,
		ExtraInfoJSON: &extraStr,
		MSUsed:        int64Ptr(end.Sub(start).Milliseconds()),
	}
	if err := p.ProcLogger.LogExtractProjections(ctx, rec, "MID-26052813"); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			p.Logger.Info("extract_projections log skipped: doc processor stopped", "call_id", callID)
		} else {
			p.Logger.Warn("failed to write extract_projections log", "call_id", callID, "error", err)
		}
	}
}

func (p *SemanticProjectionsProcessor) logEnrichProjectionsChunk(
	ctx context.Context,
	recordID int64,
	callID string,
	enrichNo, totalProjections, completedProjections int,
	modelNames []string,
	promptName string,
	payload map[string]any,
	callErr error,
	start, end time.Time,
) {
	overallPercent := 50
	perPassPercent := 0
	if totalProjections > 0 {
		overallPercent = completedProjections*100/totalProjections/2 + 50
		perPassPercent = enrichNo * 100 / totalProjections
	}
	progressStr := fmt.Sprintf("%d%% (pass 2: %d/%d)", overallPercent, completedProjections, totalProjections)
	extraInfo := map[string]any{
		"chunk":        enrichNo,
		"total_chunks": totalProjections,
		"percent":      fmt.Sprintf("%d%%", perPassPercent),
	}
	extraBytes, _ := json.Marshal(extraInfo)
	extraStr := string(extraBytes)

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
	pass := 2
	activity := "enrich_projection"
	rec := DocProcLogRecord{
		CallReason:    "enrich semantic projections",
		DocProcName:   p.Name(),
		ModelNames:    modelNames,
		PromptName:    promptName,
		RecordID:      &recordID,
		ProcProgress:  &progressStr,
		Pass:          &pass,
		LLMCallID:     &callID,
		ActivityName:  &activity,
		ArtifactJSON:  artifactStr,
		Errors:        errStr,
		ExtraInfoJSON: &extraStr,
		MSUsed:        int64Ptr(end.Sub(start).Milliseconds()),
	}
	if err := p.ProcLogger.LogEnrichProjections(ctx, rec, "MID-26052852"); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			p.Logger.Info("enrich_projections log skipped: doc processor stopped", "call_id", callID)
		} else {
			p.Logger.Warn("failed to write enrich_projections log", "call_id", callID, "error", err)
		}
	}
}

func (p *SemanticProjectionsProcessor) logSemanticProjectionsSummary(
	ctx context.Context,
	recordID int64,
	start, end time.Time,
	numChunks int,
) {
	extraInfo := map[string]any{
		"total_chunks":  numChunks,
		"total_time_ms": end.Sub(start).Milliseconds(),
	}
	extraJSON, _ := json.Marshal(extraInfo)
	extraStr := string(extraJSON)

	seen := map[string]struct{}{}
	modelNames := make([]string, 0, 3)
	for _, n := range []string{p.CandidateModelName, p.FallbackModelName, p.EnrichModelName} {
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

	promptName := firstNonEmptyTrimmed(p.CandidatePromptRef, p.EnrichPromptRef)
	progress := "100%"
	activity := "extract_projections"
	rec := DocProcLogRecord{
		CallReason:    "extract projections",
		DocProcName:   p.Name(),
		ModelNames:    modelNames,
		PromptName:    promptName,
		RecordID:      &recordID,
		ProcProgress:  &progress,
		ActivityName:  &activity,
		ExtraInfoJSON: &extraStr,
		MSUsed:        int64Ptr(end.Sub(start).Milliseconds()),
	}
	if err := p.ProcLogger.LogExtractProjections(ctx, rec, "MID-26052815"); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			p.Logger.Info("extract_projections summary log skipped: doc processor stopped")
		} else {
			p.Logger.Warn("failed to write extract_projections summary log", "error", err)
		}
	}
}

func (p *SemanticProjectionsProcessor) extractCandidatePayloadWithFallback(ctx context.Context, inputText string) (map[string]any, string, error) {
	payload, err := p.extractSemanticProjectionPayload(ctx, inputText, p.CandidatePromptText, p.CandidateModelName, p.CandidateModelCfg)
	if err == nil {
		return payload, strings.TrimSpace(p.CandidateModelName), nil
	}

	primaryModelName := strings.TrimSpace(p.CandidateModelName)
	fallbackModelName := strings.TrimSpace(p.FallbackModelName)
	if fallbackModelName == "" {
		return nil, primaryModelName, err
	}
	if p.FallbackModelErr != nil {
		return nil, fallbackModelName, fmt.Errorf("(MID_26052130) primary candidate extraction failed and fallback model %q is unavailable: %w", p.FallbackModelRef, err)
	}

	p.Logger.Warn("primary semantic projection candidate extraction failed; retrying fallback model",
		"primary_model", primaryModelName,
		"fallback_model", fallbackModelName,
		"error", err,
		"prompt_name", p.CandidatePromptRef,
	)

	fallbackPayload, fallbackErr := p.extractSemanticProjectionPayload(ctx, inputText, p.CandidatePromptText, fallbackModelName, p.FallbackModelCfg)
	if fallbackErr != nil {
		if isEmptySemanticProjectionError(fallbackErr) {
			p.Logger.Info("fallback candidate extraction returned empty JSON; treating as empty result",
				"fallback_model", fallbackModelName)
			return nil, fallbackModelName, fmt.Errorf("(MID_26052131) primary extraction failed: %w; fallback returned empty JSON: %v", err, fallbackErr)
		}
		return nil, fallbackModelName, fmt.Errorf("(MID_26052132) primary extraction failed: %w; fallback extraction failed: %v", err, fallbackErr)
	}
	return fallbackPayload, fallbackModelName, nil
}

func (p *SemanticProjectionsProcessor) extractEnrichPayload(ctx context.Context, inputText string) (map[string]any, error) {
	payload, err := p.extractSemanticProjectionPayload(ctx, inputText, p.EnrichPromptText, p.EnrichModelName, p.EnrichModelCfg)
	if err != nil {
		return nil, fmt.Errorf("(MID_26052133) enrich semantic projection: %w", err)
	}
	return payload, nil
}

func (p *SemanticProjectionsProcessor) extractSemanticProjectionPayload(
	ctx context.Context,
	inputText string,
	promptText string,
	modelName string,
	_ structureModelConfig,
) (map[string]any, error) {
	// cfg is intentionally not applied to p.Extractor here: mutating the shared
	// extractor inside concurrent goroutines causes a data race. The constructor
	// already applies the enrich config once, and modelName is carried in the
	// JSONExtractionInput so the correct model is selected per-call.
	in := llmclients.JSONExtractionInput{
		PromptText: promptText,
		ModelName:  modelName,
		InputText:  inputText,
	}
	var (
		payload map[string]any
		err     error
	)
	if structuredExtractor, ok := p.Extractor.(LLMStructuredJSONExtractor); ok {
		var result *llmclients.StructuredOutputResult
		result, err = structuredExtractor.ExtractStructuredJSON(ctx, in, semanticProjectionExtractionContract())
		if result != nil {
			payload = result.Parsed
		}
	} else {
		payload, err = p.Extractor.ExtractJSON(ctx, in)
	}
	if err != nil {
		return nil, err
	}
	if payload == nil {
		return nil, errors.New("(MID_26052134) empty llm payload")
	}
	return payload, nil
}

func isEmptySemanticProjectionError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.TrimSpace(err.Error())
	return (strings.Contains(msg, "unexpected end of JSON input") && strings.Contains(msg, "json:{[]}")) ||
		strings.Contains(msg, "(MID_26052134)")
}

func buildSemanticProjectionCandidateUserPrompt(chunkText string) string {
	schema := map[string]any{
		"semantic_projection": "string — compact semantically rich summary of this chunk",
		"keywords":            []string{"string"},
	}
	schemaJSON, _ := json.Marshal(schema)
	return "Return JSON only. Use exactly this top-level schema:\n" + string(schemaJSON) +
		"\n\nInput chunk lines:\n" + chunkText
}

func buildSemanticProjectionEnrichUserPrompt(cand semanticProjectionCandidate) string {
	chunkText := buildMarkedChunkInputText(cand.ChunkLines)
	candidateJSON, _ := json.Marshal(map[string]any{
		"seq_no":              cand.SeqNo,
		"semantic_projection": cand.SemanticProjection,
		"keywords":            cand.Keywords,
	})
	schema := map[string]any{
		"language":               "string",
		"descriptive_name":       "string",
		"descriptive_name_en":    "string",
		"keywords":               []string{"string"},
		"keywords_en":            []string{"string"},
		"semantic_projection":    "string",
		"semantic_projection_en": "string",
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
	}
	schemaJSON, _ := json.Marshal(schema)
	return "Return JSON only. Use exactly this top-level schema:\n" + string(schemaJSON) +
		"\n\nSemantic projection candidate:\n" + string(candidateJSON) +
		"\n\nSource chunk lines:\n" + chunkText
}

func chunkLineSpans(lines []MarkedLine) []string {
	nums := make([]int, 0, len(lines))
	for _, ml := range lines {
		if ml.Mark != "o" {
			nums = append(nums, ml.Line.LineNo)
		}
	}
	return lineRangesFromNumbers(nums)
}

func normalizeSemanticProjection(raw map[string]any) map[string]any {
	out := map[string]any{
		"language":               strings.TrimSpace(asString(raw["language"])),
		"descriptive_name":       strings.TrimSpace(asString(raw["descriptive_name"])),
		"descriptive_name_en":    strings.TrimSpace(asString(raw["descriptive_name_en"])),
		"keywords":               toStringSlice(raw["keywords"]),
		"keywords_en":            toStringSlice(raw["keywords_en"]),
		"semantic_projection":    strings.TrimSpace(asString(raw["semantic_projection"])),
		"semantic_projection_en": strings.TrimSpace(asString(raw["semantic_projection_en"])),
		"category_paths":         raw["category_paths"],
		"category_paths_en":      raw["category_paths_en"],
	}
	return out
}

// ---- Save to file ----

func (p *SemanticProjectionsProcessor) saveSemanticProjectionsToFile(
	recordID int64,
	rec DocMetadataInputRecord,
	projections []map[string]any,
) error {
	if len(projections) == 0 {
		return nil
	}
	artifactDir := strings.TrimSpace(p.ArtifactDir)
	if artifactDir == "" {
		return fmt.Errorf("(MID_26052140) missing ARTIFACT_DIR")
	}
	stagingBase := filepath.Base(strings.TrimSpace(rec.StagingFilename))
	filenameRoot := strings.TrimSuffix(stagingBase, filepath.Ext(stagingBase))
	if filenameRoot == "" {
		return fmt.Errorf("(MID_26052141) cannot determine filename root from staging_filename %q", rec.StagingFilename)
	}
	parserName := strings.TrimSpace(rec.ParserName)
	if parserName == "" {
		return fmt.Errorf("(MID_26052142) parser_name is empty for record_id=%d", recordID)
	}
	groupID := recordID / 1000
	dir := filepath.Join(artifactDir, strconv.FormatInt(groupID, 10), strconv.FormatInt(recordID, 10))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("(MID_26052143) create directory %s: %w", dir, err)
	}
	path := filepath.Join(dir, filenameRoot+"_"+parserName+".semantic_projections")
	bs, err := json.MarshalIndent(projections, "", "  ")
	if err != nil {
		return fmt.Errorf("(MID_26052144) marshal semantic projections: %w", err)
	}
	if err := os.WriteFile(path, bs, 0644); err != nil {
		return fmt.Errorf("(MID_26052145) write semantic projections file %s: %w", path, err)
	}
	return nil
}

// ---- Index by category paths ----

func (p *SemanticProjectionsProcessor) indexSemanticProjections(recordID int64, projections []map[string]any) error {
	dir := strings.TrimSpace(p.ArtifactWebDir)
	if dir == "" {
		return fmt.Errorf("(MID_26052150) missing ARTIFACT_WEB_DIR")
	}
	if recordID <= 0 {
		return fmt.Errorf("(MID_26052151) invalid record_id: %d", recordID)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("(MID_26052152) create artifact web dir: %w", err)
	}
	if err := removeSemanticProjectionTreeRecord(dir, recordID); err != nil {
		return fmt.Errorf("(MID_26052153) remove old semantic projection tree entries for record %d: %w", recordID, err)
	}
	now := p.Now()
	for _, proj := range projections {
		projID := strings.TrimSpace(asString(proj["semantic_proj_id"]))
		if projID == "" {
			continue
		}
		for _, pair := range pairCategoryPathEntries(proj["category_paths"], proj["category_paths_en"]) {
			if err := writeSemanticProjectionTreeEntry(p.Logger, dir, projID, pair.Index, pair.Original, now); err != nil {
				return fmt.Errorf("(MID_26052154) index semantic projection %s path: %w", projID, err)
			}
		}
	}
	return nil
}

func writeSemanticProjectionTreeEntry(
	logger ApiTypes.JimoLogger,
	treeRootDir string,
	projID string,
	indexEntry CategoryPathEntry,
	originalEntry CategoryPathEntry,
	now time.Time,
) error {
	leafDir, err := categoryTreeLeafDirForEntry(logger, treeRootDir, indexEntry, originalEntry, now)
	if err != nil {
		return err
	}
	if leafDir == "" {
		return nil
	}
	return upsertSemanticProjectionToLeafDir(leafDir, projID)
}

func upsertSemanticProjectionToLeafDir(leafDir string, projID string) error {
	filePath := filepath.Join(leafDir, "semantic_projections.txt")
	existing := make([]string, 0)
	if bs, err := os.ReadFile(filePath); err == nil {
		for row := range strings.SplitSeq(string(bs), "\n") {
			row = strings.TrimSpace(row)
			if row != "" {
				existing = append(existing, row)
			}
		}
	}
	existing = appendUniqueString(existing, projID)
	// store JSON object per line as specified (save semantic projections JSON)
	sort.Strings(existing)
	return os.WriteFile(filePath, []byte(strings.Join(existing, "\n")), 0o644)
}

func removeSemanticProjectionTreeRecord(treeRootDir string, recordID int64) error {
	prefix := strconv.FormatInt(recordID, 10) + "_"
	return filepath.WalkDir(treeRootDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != "semantic_projections.txt" {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rows := make([]string, 0)
		for row := range strings.SplitSeq(string(body), "\n") {
			row = strings.TrimSpace(row)
			if row == "" || strings.HasPrefix(row, prefix) {
				continue
			}
			rows = append(rows, row)
		}
		if len(rows) == 0 {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			return nil
		}
		sort.Strings(rows)
		return os.WriteFile(path, []byte(strings.Join(rows, "\n")), 0o644)
	})
}

// ---- Status ----

type semanticProjectionsStatusParams struct {
	RecordID      int64
	FileType      string
	InputFilename string
	Start         time.Time
	DurationMs    int64
	ProcStatus    string
	Progress      string
	ProcErr       error
}

func detectSemanticProjectionsFileType(rec DocMetadataInputRecord) string {
	for _, candidate := range []string{rec.FileName, rec.StagingFilename, rec.ResultFilename} {
		ext := strings.ToLower(strings.TrimSpace(filepath.Ext(strings.TrimSpace(candidate))))
		if ext != "" {
			return strings.TrimPrefix(ext, ".")
		}
	}
	return ""
}

func (p *SemanticProjectionsProcessor) stopAndPersistSemanticProjections(ctx context.Context, rec DocMetadataInputRecord, start time.Time) {
	if updateErr := updateInputStatusAtomic(ctx, p.InputStore, rec.ID, func(current string) (DocMetadataUpdate, error) {
		statusRaw, err := appendSemanticProjectionsStatus(current, semanticProjectionsStatusParams{
			RecordID:      rec.ID,
			FileType:      detectSemanticProjectionsFileType(rec),
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
		p.Logger.Error("(MID_26052842) failed persisting semantic projections stopped status", "record_id", rec.ID, "error", updateErr)
	}
	p.Logger.Info("(MID_26052843) extract_semantic_projections stopped by user request", "record_id", rec.ID)
}

func (p *SemanticProjectionsProcessor) persistSemanticProjectionsInProgressStatus(ctx context.Context, rec DocMetadataInputRecord, start time.Time, progress string) {
	if err := updateInputStatusAtomic(ctx, p.InputStore, rec.ID, func(current string) (DocMetadataUpdate, error) {
		statusRaw, err := appendSemanticProjectionsStatus(current, semanticProjectionsStatusParams{
			RecordID:      rec.ID,
			FileType:      detectSemanticProjectionsFileType(rec),
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
		p.Logger.Warn("failed persisting semantic projections in-progress status", "record_id", rec.ID, "error", err)
	}
}

func (p *SemanticProjectionsProcessor) persistSemanticProjectionsStatus(
	ctx context.Context,
	rec DocMetadataInputRecord,
	start time.Time,
	procErr error,
) {
	errMsg := (*string)(nil)
	if procErr != nil {
		msg := strings.TrimSpace(procErr.Error())
		errMsg = &msg
	}
	if err := updateInputStatusAtomic(ctx, p.InputStore, rec.ID, func(current string) (DocMetadataUpdate, error) {
		statusRaw, err := appendSemanticProjectionsStatus(current, semanticProjectionsStatusParams{
			RecordID:      rec.ID,
			FileType:      detectSemanticProjectionsFileType(rec),
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
		p.Logger.Error("failed persisting semantic projections status", "record_id", rec.ID, "error", err)
	}
}

func appendSemanticProjectionsStatus(raw string, p semanticProjectionsStatusParams) (string, error) {
	entries := decodeDocMetaStatus(raw)
	entry := map[string]any{
		"record_id":      strconv.FormatInt(p.RecordID, 10),
		"file_type":      strings.ToLower(strings.TrimSpace(p.FileType)),
		"operation":      "extract_semantic_projections",
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
		if op != "extract_semantic_projections" && op != "extract-semantic-projections" {
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

// ---- SQL Store ----

func (s SemanticProjectionsSQLStore) ensureSemanticProjectionsTable(ctx context.Context) error {
	if s.DB == nil {
		return fmt.Errorf("(MID_26052160) db is nil")
	}
	const ddl = `
CREATE SCHEMA IF NOT EXISTS kb;

CREATE TABLE IF NOT EXISTS kb.semantic_projections (
    id BIGSERIAL PRIMARY KEY,
    event_id TEXT,
    input_record_id BIGINT NOT NULL,
    semantic_proj_id TEXT,
    language TEXT,
    descriptive_name TEXT,
    descriptive_name_en TEXT,
    keywords JSONB,
    keywords_en JSONB,
    semantic_projection TEXT,
    semantic_projection_en TEXT,
    category_paths JSONB,
    category_paths_en JSONB,
    line_spans JSONB,
    model_name TEXT,
    prompt_name TEXT,
    search_document TEXT,
    search_vector TSVECTOR,
    connected_artifacts JSONB NOT NULL DEFAULT '{}'::jsonb,
    ext_info JSONB,
    create_time TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
`
	_, err := s.DB.ExecContext(ctx, ddl)
	return err
}

func (s SemanticProjectionsSQLStore) SemanticProjectionsExist(ctx context.Context, inputRecordID int64) (bool, error) {
	if err := s.ensureSemanticProjectionsTable(ctx); err != nil {
		return false, err
	}
	const q = `SELECT 1 FROM kb.semantic_projections WHERE input_record_id = $1 LIMIT 1`
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

func (s SemanticProjectionsSQLStore) DeleteSemanticProjectionsByInputRecordID(ctx context.Context, inputRecordID int64) (int64, error) {
	if err := s.ensureSemanticProjectionsTable(ctx); err != nil {
		return 0, err
	}
	res, err := s.DB.ExecContext(ctx, `DELETE FROM kb.semantic_projections WHERE input_record_id = $1`, inputRecordID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s SemanticProjectionsSQLStore) SaveSemanticProjections(ctx context.Context, req SaveSemanticProjectionsRequest) (int64, error) {
	if err := s.ensureSemanticProjectionsTable(ctx); err != nil {
		return 0, err
	}
	if len(req.Projections) == 0 {
		return 0, nil
	}

	const stmt = `
INSERT INTO kb.semantic_projections (
    event_id,
    input_record_id,
    semantic_proj_id,
    language,
    descriptive_name,
    descriptive_name_en,
    keywords,
    keywords_en,
    semantic_projection,
    semantic_projection_en,
    category_paths,
    category_paths_en,
    line_spans,
    model_name,
    prompt_name,
    ext_info
) VALUES (
    $1,$2,$3,$4,$5,$6,$7::jsonb,$8::jsonb,$9,$10,$11::jsonb,$12::jsonb,$13::jsonb,$14,$15,$16::jsonb
)`

	isEnglish := strings.EqualFold(strings.TrimSpace(req.Language), "en") ||
		strings.EqualFold(strings.TrimSpace(req.Language), "english")

	var eventIDVal any
	if id := strings.TrimSpace(req.EventID); id != "" {
		eventIDVal = id
	}

	extInfo, _ := json.Marshal(map[string]any{
		"language":       req.Language,
		"schema_version": "1",
	})

	var inserted int64
	for _, proj := range req.Projections {
		keywordsJSON, _ := json.Marshal(proj["keywords"])
		categoryPathsJSON, _ := json.Marshal(proj["category_paths"])

		var (
			descriptiveNameEn  any
			keywordsEnVal      any
			semanticProjEnVal  any
			categoryPathsEnVal any
		)
		if !isEnglish {
			descriptiveNameEn = strings.TrimSpace(asString(proj["descriptive_name_en"]))
			kw, _ := json.Marshal(proj["keywords_en"])
			keywordsEnVal = string(kw)
			semanticProjEnVal = strings.TrimSpace(asString(proj["semantic_projection_en"]))
			cp, _ := json.Marshal(proj["category_paths_en"])
			categoryPathsEnVal = string(cp)
		}

		lineSpansJSON, _ := json.Marshal(proj["line_spans"])

		_, err := s.DB.ExecContext(ctx, stmt,
			eventIDVal,
			req.InputRecordID,
			strings.TrimSpace(asString(proj["semantic_proj_id"])),
			strings.TrimSpace(asString(proj["language"])),
			strings.TrimSpace(asString(proj["descriptive_name"])),
			descriptiveNameEn,
			string(keywordsJSON),
			keywordsEnVal,
			strings.TrimSpace(asString(proj["semantic_projection"])),
			semanticProjEnVal,
			string(categoryPathsJSON),
			categoryPathsEnVal,
			string(lineSpansJSON),
			strings.TrimSpace(req.ModelName),
			strings.TrimSpace(req.PromptName),
			string(extInfo),
		)
		if err != nil {
			return inserted, err
		}
		inserted++
	}
	return inserted, nil
}
