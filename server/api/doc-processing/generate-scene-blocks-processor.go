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
	"time"
	"unicode"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/ApiUtils"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

type SceneBlocksProcessor struct {
	InputStore                  DocMetadataStore
	Store                       SceneObjectsStore
	Extractor                   LLMJSONExtractor
	Logger                      ApiTypes.JimoLogger
	ProcLogger                  DocProcLogger
	Now                         func() time.Time
	MentionPromptText           string
	MentionPromptRef            string
	MentionPromptErr            error
	MentionModelRef             string
	MentionModelErr             error
	MentionModelName            string
	MentionModelCfg             structureModelConfig
	RelationPromptText          string
	RelationPromptRef           string
	RelationPromptErr           error
	RelationModelRef            string
	RelationModelErr            error
	RelationModelName           string
	RelationModelCfg            structureModelConfig
	PromptText                  string
	PromptRef                   string
	PromptErr                   error
	ModelRef                    string
	ModelErr                    error
	ModelName                   string
	ModelCfg                    structureModelConfig
	FallbackModelRef            string
	FallbackModelErr            error
	FallbackModelName           string
	FallbackModelCfg            structureModelConfig
	ChunkSize                   int
	OverlapPercent              int
	PrevOverlap                 int
	NextOverlap                 int
	RemoveTOC                   bool
	ArtifactDir                 string
	ArtifactWebDir              string
	GenerateSceneBlocksMaxTasks int
}

type sceneExtractionResult struct {
	SceneBlocks   []map[string]any
	ModelName     string
	LLMCallCount  int
	FallbackCount int
	MentionsCount int
}

type sceneCandidateMention struct {
	SceneKey          string
	SceneTypeHint     string
	Title             string
	SummaryHint       string
	EvidenceQuote     string
	LineSpans         []string
	Confidence        float64
	ConfidenceReason  string
	ChunkIndex        int
	ChunkLines        []MarkedLine
	HasNormalEvidence bool
}

type sceneCandidate struct {
	CandidateID        string
	SceneKey           string
	SceneTypeHint      string
	Title              string
	SummaryHint        string
	SupportingMentions []map[string]any
	SupportLines       []MarkedLine
	PrimaryChunkIndex  int
}

type candidateChunkGroup struct {
	ChunkIndex int
	Candidates []sceneCandidate
}

type SceneObjectsStore interface {
	SceneObjectsExist(ctx context.Context, inputRecordID int64) (bool, error)
	DeleteSceneObjectsByInputRecordID(ctx context.Context, inputRecordID int64) (int64, error)
	UpsertSceneObject(ctx context.Context, req UpsertSceneObjectRequest) error
}

type UpsertSceneObjectRequest struct {
	InputRecordID int64
	ObjectID      string
	EventID       string
	SceneBlock    map[string]any
	ModelName     string
	PromptName    string
	ExtInfo       map[string]any
}

func buildSceneBlockExtInfo(block map[string]any) map[string]any {
	if len(block) == 0 {
		return map[string]any{}
	}
	out := map[string]any{}
	if titleEn := strings.TrimSpace(asString(block["title_en"])); titleEn != "" {
		out["title_en"] = titleEn
	}
	if summaryEn := strings.TrimSpace(asString(block["summary_en"])); summaryEn != "" {
		out["summary_en"] = summaryEn
	}
	lineSpans := normalizeProductEvidenceLines(block["line_spans"])
	if len(lineSpans) == 0 {
		lineSpans = normalizeProductEvidenceLines(block["evidence_lines"])
	}
	if len(lineSpans) > 0 {
		out["line_spans"] = lineSpans
	}
	if keywordsEn := normalizeProductEvidenceLines(block["keywords_en"]); len(keywordsEn) > 0 {
		out["keywords_en"] = keywordsEn
	}
	if statesEn := normalizeProductEvidenceLines(block["states_en"]); len(statesEn) > 0 {
		out["states_en"] = statesEn
	}
	return out
}

type SceneObjectsSQLStore struct {
	DB *sql.DB
}

func NewSceneBlocksProcessor(inputStore DocMetadataStore, store SceneObjectsStore, extractor LLMJSONExtractor, _ ApiTypes.JimoLogger) *SceneBlocksProcessor {
	// if logger == nil {
	logger := loggerutil.CreateDefaultLogger("MID_26051801")
	// }
	mentionPromptText, mentionPromptRef, mentionPromptErr := loadScenePromptFromEnvKeys(
		[]string{"EXTRACT_SCENE_CANDIDATES_PROMPT"},
		"prompt-extract-scene-candidates-v1.md",
	)
	relationPromptText, relationPromptRef, relationPromptErr := loadScenePromptFromEnvKeys(
		[]string{"ENRICH_SCENE_BLOCKS_PROMPT", "EXTRACT_SCENE_BLOCKS_PROMPT"},
		"prompt-enrich-scene-blocks-v1.md",
	)
	mentionModelRef, _, mentionModelCfg, mentionModelErr := loadModelConfigFromEnvKeys(
		[]string{"EXTRACT_SCENE_CANDIDATES_MODEL_NAME", "EXTRACT_SCENE_BLOCKS_MODEL_NAME"},
		"MODEL_DEF_FILE",
	)
	relationModelRef, _, relationModelCfg, relationModelErr := loadModelConfigFromEnvKeys(
		[]string{"ENRICH_SCENE_BLOCKS_MODEL_NAME", "EXTRACT_SCENE_BLOCKS_MODEL_NAME"},
		"MODEL_DEF_FILE",
	)
	fallbackModelRef, _, fallbackModelCfg, fallbackModelErr := loadOptionalModelConfigFromEnv("EXTRACT_SCENE_BLOCKS_MODEL_FALLBACK", "MODEL_DEF_FILE")
	applyStructureModelConfigToExtractor(extractor, relationModelCfg)
	prevOverlap, nextOverlap, removeTOC := blockingConfigFromViper()
	return &SceneBlocksProcessor{
		InputStore:                  inputStore,
		Store:                       store,
		Extractor:                   extractor,
		Logger:                      logger,
		ProcLogger:                  DocProcLogger{DB: ApiTypes.ProjectDBHandle},
		Now:                         time.Now,
		MentionPromptText:           mentionPromptText,
		MentionPromptRef:            mentionPromptRef,
		MentionPromptErr:            mentionPromptErr,
		MentionModelRef:             mentionModelRef,
		MentionModelErr:             mentionModelErr,
		MentionModelName:            mentionModelCfg.ModelName,
		MentionModelCfg:             mentionModelCfg,
		RelationPromptText:          relationPromptText,
		RelationPromptRef:           relationPromptRef,
		RelationPromptErr:           relationPromptErr,
		RelationModelRef:            relationModelRef,
		RelationModelErr:            relationModelErr,
		RelationModelName:           relationModelCfg.ModelName,
		RelationModelCfg:            relationModelCfg,
		PromptText:                  relationPromptText,
		PromptRef:                   relationPromptRef,
		PromptErr:                   relationPromptErr,
		ModelRef:                    relationModelRef,
		ModelErr:                    relationModelErr,
		ModelName:                   relationModelCfg.ModelName,
		ModelCfg:                    relationModelCfg,
		FallbackModelRef:            fallbackModelRef,
		FallbackModelErr:            fallbackModelErr,
		FallbackModelName:           fallbackModelCfg.ModelName,
		FallbackModelCfg:            fallbackModelCfg,
		ChunkSize:                   envInt("CHUNK_SIZE", DefaultChunkSize, 1),
		OverlapPercent:              envInt("CHUNK_OVERLAP_PERCENT", DefaultOverlapPercent, 0),
		PrevOverlap:                 prevOverlap,
		NextOverlap:                 nextOverlap,
		RemoveTOC:                   removeTOC,
		ArtifactDir:                 strings.TrimSpace(os.Getenv("ARTIFACT_DIR")),
		ArtifactWebDir:              strings.TrimSpace(os.Getenv("ARTIFACT_WEB_DIR")),
		GenerateSceneBlocksMaxTasks: envInt("GENERATE_SCENE_BLOCKS_MAX_TASKS", 1, 1),
	}
}

func (p *SceneBlocksProcessor) Name() string { return "generate_scene_blocks" }

func (p *SceneBlocksProcessor) HandleEvent(ctx context.Context, payload []byte) error {
	p.Logger.Info("Generate Scene Blocks handle event")
	start := p.Now()
	evt, err := ParseLineFileGeneratedEvent(payload)
	if err != nil {
		return fmt.Errorf("(MID_26051802) parse event payload: %w", err)
	}

	if p.MentionPromptErr != nil {
		p.Logger.Error("prompt error", "prompt_name", p.MentionPromptRef, "error", p.MentionPromptErr)
		return fmt.Errorf("(MID_26051803) load scene candidates prompt %q: %w", p.MentionPromptRef, p.MentionPromptErr)
	}
	if p.RelationPromptErr != nil {
		p.Logger.Error("prompt error", "prompt_name", p.RelationPromptRef, "error", p.RelationPromptErr)
		return fmt.Errorf("(MID_26051803) load scene blocks prompt %q: %w", p.RelationPromptRef, p.RelationPromptErr)
	}
	if p.InputStore == nil {
		p.Logger.Error("input store is nil")
		return errors.New("(MID_26051804) input store is nil")
	}
	if p.Store == nil {
		p.Logger.Error("scene objects store is nil")
		return errors.New("(MID_26051805) scene objects store is nil")
	}

	rec, err := p.InputStore.GetInputRecord(ctx, evt.RecordID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			p.Logger.Error("kb.inputs record not found", "record_id", evt.RecordID)
			return nil
		}
		p.Logger.Error("failed get record", "error", err)
		return fmt.Errorf("(MID_26051806) load kb.inputs record %d: %w", evt.RecordID, err)
	}
	if p.MentionModelErr != nil {
		p.Logger.Warn("scene blocks extraction skipped: mention model config error",
			"record_id", evt.RecordID, "model_ref", p.MentionModelRef, "error", p.MentionModelErr)
		p.persistSceneBlocksStatus(ctx, rec, start, p.MentionModelErr)
		return nil
	}
	if p.RelationModelErr != nil {
		p.Logger.Warn("scene blocks extraction skipped: relation model config error",
			"record_id", evt.RecordID, "model_ref", p.RelationModelRef, "error", p.RelationModelErr)
		p.persistSceneBlocksStatus(ctx, rec, start, p.RelationModelErr)
		return nil
	}

	if evt.Force {
		_, _ = p.Store.DeleteSceneObjectsByInputRecordID(ctx, evt.RecordID)
	} else {
		exists, err := p.Store.SceneObjectsExist(ctx, evt.RecordID)
		if err != nil {
			p.persistSceneBlocksStatus(ctx, rec, start, err)
			p.Logger.Error("scene objects exist check failed", "error", err, "record_id", evt.RecordID)
			return nil
		}
		if exists {
			p.Logger.Info("scene blocks extraction skipped", "record_id", evt.RecordID, "reason", "scene objects already exist and force=false")
			p.persistSceneBlocksStatus(ctx, rec, start, nil)
			return nil
		}
	}

	lines, err := p.resolveInputLines(ctx, evt, rec)
	if err != nil {
		p.persistSceneBlocksStatus(ctx, rec, start, err)
		p.Logger.Error("resolveInputLines error", "error", err, "record_id", evt.RecordID)
		return nil
	}

	chunks, err := BuildChunks(lines, ChunkOptions{ChunkSize: p.ChunkSize, OverlapPercent: p.OverlapPercent})
	if err != nil {
		procErr := fmt.Errorf("(MID_26051807) build chunks for record_id=%d: %w", evt.RecordID, err)
		p.persistSceneBlocksStatus(ctx, rec, start, procErr)
		return nil
	}
	if len(chunks) == 0 {
		procErr := fmt.Errorf("(MID_26051808) no chunks found for record_id=%d", evt.RecordID)
		p.persistSceneBlocksStatus(ctx, rec, start, procErr)
		return nil
	}

	p.Logger.Info("Before calling LLM",
		"num_chunks", len(chunks),
		"record_id", evt.RecordID,
	)

	result, err := p.extractSceneBlocksFromChunksWithLLM(ctx, evt.RecordID, chunks, rec, start)
	if err != nil {
		if errors.Is(err, ErrPipelineStopped) {
			p.stopAndPersistSceneBlocks(context.Background(), rec, start)
			return ErrPipelineStopped
		}
		p.persistSceneBlocksStatus(ctx, rec, start, err)
		p.Logger.Error("scene blocks extraction failed", "record_id", evt.RecordID, "error", err)
		return nil
	}

	eventID := eventIDFromContext(ctx)
	for idx := range result.SceneBlocks {
		block := result.SceneBlocks[idx]
		objectID := fmt.Sprintf("%d_%d", evt.RecordID, idx+1)
		if strings.TrimSpace(asString(block["scene_id"])) == "" {
			block["scene_id"] = objectID
		}
		req := UpsertSceneObjectRequest{
			InputRecordID: evt.RecordID,
			ObjectID:      objectID,
			EventID:       eventID,
			SceneBlock:    block,
			ModelName:     strings.TrimSpace(result.ModelName),
			PromptName:    strings.TrimSpace(p.RelationPromptRef),
			ExtInfo:       buildSceneBlockExtInfo(block),
		}
		if err := p.Store.UpsertSceneObject(ctx, req); err != nil {
			procErr := fmt.Errorf("(MID_26051810) upsert scene object %s: %w", objectID, err)
			p.persistSceneBlocksStatus(ctx, rec, start, procErr)
			p.Logger.Error("failed updating db",
				"error", err,
				"object_id", objectID,
				"model name", result.ModelName,
				"prompt name", p.RelationPromptRef)
			return nil
		}
		block["object_id"] = objectID
	}

	if err := p.writeSceneBlocksArtifact(evt.RecordID, rec, result.SceneBlocks); err != nil {
		p.Logger.Error("write scene blocks artifact error", "error", err, "record_id", evt.RecordID)
		p.persistSceneBlocksStatus(ctx, rec, start, err)
		return nil
	}
	if indexErr := p.indexSceneBlocks(evt.RecordID, result.SceneBlocks); indexErr != nil {
		p.Logger.Warn("index scene blocks failed", "record_id", evt.RecordID, "error", indexErr)
	}
	if reindexErr := ReindexSceneBlockSearchForRecord(ctx, evt.RecordID, p.Logger); reindexErr != nil {
		p.Logger.Warn("reindex scene block search registry failed", "record_id", evt.RecordID, "error", reindexErr)
	}

	p.Logger.Info("scene blocks extracted",
		"record_id", evt.RecordID,
		"scene_blocks_count", len(result.SceneBlocks),
		"num_chunks", len(chunks),
		"model_name", result.ModelName,
		"prompt_name", p.RelationPromptRef)

	p.persistSceneBlocksStatus(ctx, rec, start, nil)
	p.logSceneBlocksSummary(ctx, start, p.Now(), result, len(chunks), evt.RecordID)
	return nil
}

func (p *SceneBlocksProcessor) resolveInputLines(ctx context.Context, evt LineFileGeneratedEvent, rec DocMetadataInputRecord) ([]Line, error) {
	if buf := BlockBufferFromContext(ctx); buf != nil {
		return ParseBlockBufferLines(buf), nil
	}
	inputPath, err := ResolveInputFilePath(evt, rec.ResultFilename, rec.ParserName, rec.StagingFilename)
	if err != nil {
		return nil, fmt.Errorf("(MID_26051811) resolve input file for record_id=%d: %w", evt.RecordID, err)
	}
	body, err := os.ReadFile(inputPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("(MID_26051812) input file not exist: %s", inputPath)
		}
		return nil, fmt.Errorf("(MID_26051813) read input file: %w", err)
	}
	lines, err := ParseInputLines(body)
	if err != nil {
		return nil, fmt.Errorf("(MID_26051814) parse input lines: %w", err)
	}
	return lines, nil
}

/*
func buildSceneBlocksChunkText(chunk Chunk) string {
	return buildMarkedChunkInputText(chunk.Lines)
}
*/

func (p *SceneBlocksProcessor) extractSceneBlocksFromChunksWithLLM(
	ctx context.Context,
	record_id int64,
	chunks []Chunk,
	rec DocMetadataInputRecord,
	start time.Time,
) (sceneExtractionResult, error) {
	eventID := eventIDFromContext(ctx)
	usedMentionModel := strings.TrimSpace(p.MentionModelName)

	type pass1ChunkResult struct {
		mentions   []sceneCandidateMention
		modelName  string
		isFallback bool
	}

	p.Logger.Info("extract scene blocks",
			"record_id", record_id,
			"model_names", fmt.Sprintf("%s/%s", p.MentionModelName, p.RelationModelName),
			"prompt_names", fmt.Sprintf("%s/%s", p.RelationPromptRef, p.MentionPromptRef),
		)

	pass1Results, pass1Err := runConcurrent(ctx, p.GenerateSceneBlocksMaxTasks, len(chunks), func(jobCtx context.Context, idx int) (pass1ChunkResult, error) {
		if isCtxStopped(jobCtx) {
			return pass1ChunkResult{}, ErrPipelineStopped
		}
		chunk := chunks[idx]
		callStart := p.Now()
		p.Logger.Info("extract scene start",
			"idx", idx,
			"record_id", record_id,
			"total chunks", len(chunks),
		)
		payload, modelName, err := p.extractScenePayloadWithFallback(jobCtx, buildSceneCandidateUserPrompt(chunk), p.MentionPromptText, p.MentionPromptRef, p.MentionModelName, p.MentionModelCfg)
		isFallback := strings.TrimSpace(modelName) != strings.TrimSpace(p.MentionModelName) && strings.TrimSpace(modelName) != ""
		p.logLLMCall(ctx, fmt.Sprintf("%s_p1_c%d", eventID, idx), "extract_scene_block_candidates", 1, []string{strings.TrimSpace(modelName)}, strings.TrimSpace(p.MentionPromptRef), payload, err, callStart, p.Now(), record_id, idx, len(chunks), 0)
		p1Percent := (idx + 1) * 100 / len(chunks) / 2
		p.persistSceneBlocksInProgressStatus(ctx, rec, start, fmt.Sprintf("%d%% (pass 1: %d/%d)", p1Percent, idx+1, len(chunks)))
		if err != nil {
			return pass1ChunkResult{}, fmt.Errorf("(MID_26051809) extract scene candidates via llm for chunk %d: %w", chunk.SeqNo, err)
		}
		raw, _ := payload["candidates"].([]any)
		mentions := normalizeSceneCandidateMentions(raw, chunk)
		p.Logger.Info("extract scene end  ",
			"extracted", len(raw),
			"ms_used", time.Since(callStart).Milliseconds(),
		)
		return pass1ChunkResult{
			mentions:   mentions,
			modelName:  modelName,
			isFallback: isFallback,
		}, nil
	})
	if pass1Err != nil {
		if isCtxStopped(ctx) {
			return sceneExtractionResult{}, ErrPipelineStopped
		}
		return sceneExtractionResult{}, pass1Err
	}

	llmCallCount := len(chunks)
	fallbackCount := 0
	mentions := make([]sceneCandidateMention, 0, len(chunks))
	for _, r := range pass1Results {
		if r.isFallback {
			fallbackCount++
		}
		if strings.TrimSpace(r.modelName) != "" {
			usedMentionModel = strings.TrimSpace(r.modelName)
		}
		mentions = append(mentions, r.mentions...)
	}

	candidates := mergeSceneCandidateMentions(mentions)
	p.Logger.Info("Merged scene candidates",
		"record_id", record_id,
		"mentions_count", len(mentions),
		"candidate_count", len(candidates),
		"record_stage", "post_merge",
	)

	usedRelationModel := strings.TrimSpace(p.RelationModelName)
	chunkGroups := groupSceneCandidatesByChunk(candidates)

	type pass2GroupResult struct {
		blocks     []map[string]any
		modelName  string
		isFallback bool
	}

	pass2Results, pass2Err := runConcurrent(ctx, p.GenerateSceneBlocksMaxTasks, len(chunkGroups), func(jobCtx context.Context, groupIdx int) (pass2GroupResult, error) {
		if isCtxStopped(jobCtx) {
			return pass2GroupResult{}, ErrPipelineStopped
		}
		group := chunkGroups[groupIdx]
		callStart := p.Now()
		candidateIDs := make([]string, 0, len(group.Candidates))
		for _, c := range group.Candidates {
			candidateIDs = append(candidateIDs, c.CandidateID)
		}
		p.Logger.Info("enrich scene start ",
			"record_id", record_id,
			"group_idx", groupIdx,
			"chunk_index", group.ChunkIndex,
		)
		payload, modelName, err := p.extractScenePayloadWithFallback(jobCtx, buildSceneRelationUserPromptForGroup(group.Candidates), p.RelationPromptText, p.RelationPromptRef, p.RelationModelName, p.RelationModelCfg)
		isFallback := strings.TrimSpace(modelName) != strings.TrimSpace(p.RelationModelName) && strings.TrimSpace(modelName) != ""
		p.logLLMCall(ctx, fmt.Sprintf("%s_p2_g%d", eventID, groupIdx), "enrich_scene_blocks", 2, []string{strings.TrimSpace(modelName)}, strings.TrimSpace(p.RelationPromptRef), payload, err, callStart, p.Now(), record_id, groupIdx, len(chunkGroups), 0)
		p2Percent := (groupIdx+1)*100/len(chunkGroups)/2 + 50
		p.persistSceneBlocksInProgressStatus(ctx, rec, start, fmt.Sprintf("%d%% (pass 2: %d/%d)", p2Percent, groupIdx+1, len(chunkGroups)))
		if err != nil {
			return pass2GroupResult{}, fmt.Errorf("(MID_26051827) enrich scene blocks via llm for chunk group %d (candidates: %v): %w", group.ChunkIndex, candidateIDs, err)
		}
		raw, _ := payload["scene_blocks"].([]any)
		normalized := normalizeSceneBlockListForGroup(raw, group.Candidates)
		p.Logger.Info("enrich scene end   ",
			"record_id", record_id,
			"group_idx", groupIdx,
			"chunk_index", group.ChunkIndex,
			"blocks_for_group", len(normalized),
			"ms_used", time.Since(callStart).Milliseconds(),
		)
		return pass2GroupResult{
			blocks:     normalized,
			modelName:  modelName,
			isFallback: isFallback,
		}, nil
	})
	if pass2Err != nil {
		if isCtxStopped(ctx) {
			return sceneExtractionResult{}, ErrPipelineStopped
		}
		return sceneExtractionResult{}, pass2Err
	}

	llmCallCount += len(chunkGroups)
	sceneBlocks := make([]map[string]any, 0, len(chunkGroups))
	for _, r := range pass2Results {
		if r.isFallback {
			fallbackCount++
		}
		if strings.TrimSpace(r.modelName) != "" {
			usedRelationModel = strings.TrimSpace(r.modelName)
		}
		sceneBlocks = append(sceneBlocks, r.blocks...)
	}

	preDedupeCount := len(sceneBlocks)
	sceneBlocks = dedupeFinalSceneBlocks(sceneBlocks)
	p.Logger.Info("Deduped final scene blocks",
		"rows_before_dedup", preDedupeCount,
		"rows_after_dedup", len(sceneBlocks),
		"record_stage", "post_scene_dedup",
	)

	return sceneExtractionResult{
		SceneBlocks:   sceneBlocks,
		ModelName:     firstNonEmptyTrimmed(usedRelationModel, usedMentionModel, p.RelationModelName, p.ModelName),
		LLMCallCount:  llmCallCount,
		FallbackCount: fallbackCount,
		MentionsCount: len(mentions),
	}, nil
}

func (p *SceneBlocksProcessor) logLLMCall(
	ctx context.Context,
	callID, activity string,
	pass int,
	modelNames []string,
	promptName string,
	payload map[string]any,
	callErr error,
	start, end time.Time,
	recordID int64,
	itemIdx int,
	totalItems int,
	runningTotal int,
) {
	if p.ProcLogger.DB == nil {
		return
	}
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
	percent := 0
	if totalItems > 0 {
		if pass == 1 {
			percent = (itemIdx + 1) * 100 / totalItems / 2
		} else {
			percent = (itemIdx+1)*100/totalItems/2 + 50
		}
	}
	passLabel := fmt.Sprintf("pass %d", pass)
	progress := fmt.Sprintf("%d%% (%s: %d/%d)", percent, passLabel, itemIdx+1, totalItems)
	var extraInfo map[string]any
	if pass == 1 {
		numScens := 0
		if payload != nil {
			if raw, ok := payload["candidates"].([]any); ok {
				numScens = len(raw)
			}
		}
		extraInfo = map[string]any{
			"chunk":        itemIdx + 1,
			"num_scens":    numScens,
			"total_so_far": runningTotal,
			"percent":      progress,
			"total_chunks": totalItems,
		}
	} else {
		numScenes := 0
		if payload != nil {
			if raw, ok := payload["scene_blocks"].([]any); ok {
				numScenes = len(raw)
			}
		}
		extraInfo = map[string]any{
			"chunk":        itemIdx + 1,
			"num_scenes":   numScenes,
			"total_scenes": runningTotal,
			"percent":      progress,
			"total_chunks": totalItems,
		}
	}
	extraJSON, _ := json.Marshal(extraInfo)
	extraStr := string(extraJSON)
	callReason := "extract scene blocks"
	if pass == 2 {
		callReason = "enrich scene blocks"
	}
	rec := DocProcLogRecord{
		CallReason:    callReason,
		DocProcName:   p.Name(),
		ModelNames:    modelNames,
		PromptName:    promptName,
		RecordID:      &recordID,
		ProcProgress:  &progress,
		Pass:          &pass,
		LLMCallID:     &callID,
		ActivityName:  &activity,
		ArtifactJSON:  artifactStr,
		Errors:        errStr,
		ExtraInfoJSON: &extraStr,
		MSUsed:        int64Ptr(end.Sub(start).Milliseconds()),
	}
	logFn := p.ProcLogger.LogExtractSceneBlocks
	if pass == 2 {
		logFn = p.ProcLogger.LogEnrichSceneBlocks
	}
	if err := logFn(ctx, rec, "MID-26052815"); err != nil {
		p.Logger.Warn("failed to write llm_call log", "call_id", callID, "error", err)
	}
}

func (p *SceneBlocksProcessor) logSceneBlocksSummary(ctx context.Context, start, end time.Time, result sceneExtractionResult, numChunks int, recordID int64) {
	if p.ProcLogger.DB == nil {
		return
	}
	modelNames := make([]string, 0, 3)
	for _, n := range []string{
		strings.TrimSpace(p.MentionModelName),
		strings.TrimSpace(p.FallbackModelName),
		strings.TrimSpace(p.RelationModelName),
	} {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if !slices.Contains(modelNames, n) {
			modelNames = append(modelNames, n)
		}
	}
	promptName := firstNonEmptyTrimmed(p.MentionPromptRef, p.RelationPromptRef)
	extraInfo, _ := json.Marshal(map[string]interface{}{
		"total_scene_blocks": len(result.SceneBlocks),
		"mentions_count":     result.MentionsCount,
		"llm_call_count":     result.LLMCallCount,
		"fallback_count":     result.FallbackCount,
		"num_chunks":         numChunks,
	})
	extraStr := string(extraInfo)
	progress := "100%"
	activityName := "extract_scene_blocks"
	if err := p.ProcLogger.LogSummary(ctx, DocProcLogRecord{
		CallReason:    "extract scene blocks",
		DocProcName:   p.Name(),
		ModelNames:    modelNames,
		PromptName:    promptName,
		RecordID:      &recordID,
		ProcProgress:  &progress,
		ActivityName:  &activityName,
		ExtraInfoJSON: &extraStr,
		MSUsed:        int64Ptr(end.Sub(start).Milliseconds()),
	}, "MID-26052815"); err != nil {
		p.Logger.Warn("failed to write doc_proc_summary log", "error", err)
	}
}

func (p *SceneBlocksProcessor) extractScenePayloadWithFallback(ctx context.Context, inputText string, promptText string, promptRef string, modelName string, cfg structureModelConfig) (map[string]any, string, error) {
	// p.Logger.Info("call llm", "inputText", inputText)

	payload, err := p.extractScenePayload(ctx, inputText, promptText, modelName, cfg)
	if err == nil {
		return payload, strings.TrimSpace(modelName), nil
	}

	primaryModelName := strings.TrimSpace(modelName)
	fallbackModelName := strings.TrimSpace(p.FallbackModelName)
	if fallbackModelName == "" {
		return nil, primaryModelName, err
	}
	if p.FallbackModelErr != nil {
		return nil, fallbackModelName, fmt.Errorf("(MID_26051825) primary extraction failed and fallback model %q is unavailable: %w", p.FallbackModelRef, err)
	}

	if ApiUtils.IsEmptyJSONResponse(err) {
		p.Logger.Warn("primary scene extraction returned empty JSON; retrying fallback model",
			"primary_model", primaryModelName,
			"fallback_model", fallbackModelName,
			"error", err,
			"prompt_name", promptRef,
		)
	} else {
		p.Logger.Warn("primary scene extraction failed; retrying fallback model",
			"primary_model", primaryModelName,
			"fallback_model", fallbackModelName,
			"error", err,
			"prompt_name", promptRef,
		)
	}

	payload, fallbackErr := p.extractScenePayload(ctx, inputText, promptText, fallbackModelName, p.FallbackModelCfg)
	if fallbackErr != nil {
		if ApiUtils.IsEmptyJSONResponse(fallbackErr) {
			p.Logger.Warn("fallback scene extraction returned empty JSON; treating as empty result",
				"fallback_model", fallbackModelName,
				"error", fallbackErr,
				"prompt_name", promptRef,
			)
			return map[string]any{"scene_blocks": []any{}, "candidates": []any{}}, fallbackModelName, nil
		}
		return nil, fallbackModelName, fmt.Errorf("(MID_26051826) primary extraction failed: %w; fallback extraction failed: %v", err, fallbackErr)
	}
	return payload, fallbackModelName, nil
}

func (p *SceneBlocksProcessor) extractSceneBlocksWithModel(ctx context.Context, inputText string, modelName string, cfg structureModelConfig) (map[string]any, error) {
	return p.extractScenePayload(ctx, inputText, p.PromptText, modelName, cfg)
}

func (p *SceneBlocksProcessor) extractScenePayload(ctx context.Context, inputText string, promptText string, modelName string, cfg structureModelConfig) (map[string]any, error) {
	applyStructureModelConfigToExtractor(p.Extractor, cfg)
	payload, err := extractScenePayloadWithContract(ctx, p.Extractor, llmclients.JSONExtractionInput{
		PromptText: promptText,
		ModelName:  modelName,
		InputText:  inputText,
	})
	if err != nil {
		return payload, err
	}
	payload = normalizeScenePayload(payload)
	if len(payload) == 0 {
		return nil, errors.New("(MID_26051828) empty llm json object")
	}
	if raw, ok := payload["candidates"]; ok {
		items, ok := normalizeSceneItems(raw)
		if !ok {
			return nil, fmt.Errorf("(MID_26051829) llm output field 'candidates' must be an array, JSON:%v", payload)
		}
		payload["candidates"] = items
		return payload, nil
	}
	if raw, ok := payload["scene_blocks"]; ok {
		items, ok := normalizeSceneItems(raw)
		if !ok {
			return nil, fmt.Errorf("(MID_26051830) llm output field 'scene_blocks' must be an array, JSON:%v", payload)
		}
		payload["scene_blocks"] = items
		return payload, nil
	}
	return nil, fmt.Errorf("(MID_26051831) llm output must contain 'scene_blocks' or 'candidates', JSON:%v", payload)
}

func extractScenePayloadWithContract(ctx context.Context, extractor LLMJSONExtractor, in llmclients.JSONExtractionInput) (map[string]any, error) {
	if structuredExtractor, ok := extractor.(LLMStructuredJSONExtractor); ok {
		result, err := structuredExtractor.ExtractStructuredJSON(ctx, in, sceneExtractionContract())
		if err != nil {
			return nil, err
		}
		if result == nil {
			return map[string]any{}, nil
		}
		return result.Parsed, nil
	}
	return extractor.ExtractJSON(ctx, in)
}

func normalizeScenePayload(payload map[string]any) map[string]any {
	if len(payload) == 0 {
		return payload
	}
	if _, ok := payload["scene_blocks"]; ok {
		return payload
	}
	if _, ok := payload["candidates"]; ok {
		return payload
	}
	if looksLikeSceneBlockRecord(payload) {
		return map[string]any{"scene_blocks": []any{payload}}
	}
	if looksLikeSceneCandidateRecord(payload) {
		return map[string]any{"candidates": []any{payload}}
	}
	return payload
}

func normalizeSceneItems(value any) ([]any, bool) {
	switch v := value.(type) {
	case []any:
		return v, true
	case map[string]any:
		return []any{v}, true
	default:
		return nil, false
	}
}

func looksLikeSceneBlockRecord(payload map[string]any) bool {
	for _, key := range []string{"scene_id", "scene_type", "title", "summary"} {
		if strings.TrimSpace(asString(payload[key])) != "" {
			return true
		}
	}
	return false
}

func looksLikeSceneCandidateRecord(payload map[string]any) bool {
	for _, key := range []string{"scene_key", "scene_type_hint", "title", "summary_hint"} {
		if strings.TrimSpace(asString(payload[key])) != "" {
			return true
		}
	}
	return false
}

func buildSceneCandidateUserPrompt(chunk Chunk) string {
	schema := map[string]any{
		"candidates": []map[string]any{{
			"scene_key":         "stable_snake_case_identifier",
			"scene_type_hint":   "workflow|operation|failure|decision|monitoring|compliance|state_transition|interaction|other",
			"title":             "human readable title",
			"summary_hint":      "one sentence description of the scene",
			"evidence_quote":    "short supporting quote",
			"line_spans":        []string{"12", "13-15"},
			"confidence":        0.0,
			"confidence_reason": "brief reason",
		}},
	}
	schemaJSON, _ := json.Marshal(schema)
	return "Return JSON only. Use exactly this top-level schema:\n" + string(schemaJSON) +
		"\n\nChunk index: " + strconv.Itoa(chunk.SeqNo) +
		"\n\nInput chunk lines (JSON array):\n" + markedLinesToJSON(chunk.Lines)
}

func buildSceneRelationUserPromptForGroup(candidates []sceneCandidate) string {
	candidateList := make([]map[string]any, 0, len(candidates))
	lineMap := map[string]MarkedLine{}
	for _, candidate := range candidates {
		candidateList = append(candidateList, map[string]any{
			"candidate_id":        candidate.CandidateID,
			"scene_key":           candidate.SceneKey,
			"scene_type_hint":     candidate.SceneTypeHint,
			"title":               candidate.Title,
			"summary_hint":        candidate.SummaryHint,
			"supporting_mentions": candidate.SupportingMentions,
		})
		for _, line := range candidate.SupportLines {
			lineKey := fmt.Sprintf("%d:%d:%s", line.Line.PageNo, line.Line.LineNo, line.Line.Content)
			existing, exists := lineMap[lineKey]
			if !exists || (existing.Mark == "o" && line.Mark != "o") {
				lineMap[lineKey] = line
			}
		}
	}
	candidatesJSON, _ := json.Marshal(candidateList)
	supportLines := make([]MarkedLine, 0, len(lineMap))
	for _, line := range lineMap {
		supportLines = append(supportLines, line)
	}
	sort.Slice(supportLines, func(i, j int) bool {
		if supportLines[i].Line.PageNo != supportLines[j].Line.PageNo {
			return supportLines[i].Line.PageNo < supportLines[j].Line.PageNo
		}
		if supportLines[i].Line.LineNo != supportLines[j].Line.LineNo {
			return supportLines[i].Line.LineNo < supportLines[j].Line.LineNo
		}
		return supportLines[i].Mark < supportLines[j].Mark
	})
	schema := map[string]any{
		"scene_blocks": []map[string]any{{
			"scene_id":       "stable_snake_case_identifier",
			"scene_type":     "string",
			"title":          "human readable title",
			"summary":        "string",
			"actors":         []any{},
			"resources":      []any{},
			"preconditions":  []string{},
			"triggers":       []string{},
			"states":         []string{},
			"actions":        []any{},
			"constraints":    []string{},
			"decisions":      []string{},
			"outcomes":       []string{},
			"failure_modes":  []string{},
			"root_causes":    []string{},
			"resolutions":    []string{},
			"relationships":  []any{},
			"discriminators": []any{},
			"keywords":       []string{},
			"confidence":     0.0,
			"source_refs":    []any{},
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
	}
	schemaJSON, _ := json.Marshal(schema)
	return "Return JSON only. Use exactly this top-level schema:\n" + string(schemaJSON) +
		"\n\nCandidates:\n" + string(candidatesJSON) +
		"\n\nSource lines (JSON array):\n" + markedLinesToJSON(supportLines)
}

func normalizeSceneCandidateMentions(items []any, chunk Chunk) []sceneCandidateMention {
	out := make([]sceneCandidateMention, 0, len(items))
	for _, item := range items {
		raw, ok := item.(map[string]any)
		if !ok {
			continue
		}
		lineSpans := normalizeProductEvidenceLines(raw["line_spans"])
		evidenceQuote := strings.TrimSpace(asString(raw["evidence_quote"]))
		out = append(out, sceneCandidateMention{
			SceneKey:          strings.TrimSpace(asString(raw["scene_key"])),
			SceneTypeHint:     strings.TrimSpace(asString(raw["scene_type_hint"])),
			Title:             strings.TrimSpace(asString(raw["title"])),
			SummaryHint:       strings.TrimSpace(asString(raw["summary_hint"])),
			EvidenceQuote:     evidenceQuote,
			LineSpans:         lineSpans,
			Confidence:        toFloat(raw["confidence"]),
			ConfidenceReason:  strings.TrimSpace(asString(raw["confidence_reason"])),
			ChunkIndex:        chunk.SeqNo,
			ChunkLines:        append([]MarkedLine(nil), chunk.Lines...),
			HasNormalEvidence: sceneCandidateHasNormalEvidence(chunk, lineSpans, evidenceQuote),
		})
	}
	return out
}

func sceneCandidateHasNormalEvidence(chunk Chunk, spans []string, quote string) bool {
	lineNums := make(map[int]struct{})
	for _, span := range spans {
		start, end, ok := parseCompactLineSpan(span)
		if !ok {
			continue
		}
		for i := start; i <= end; i++ {
			lineNums[i] = struct{}{}
		}
	}
	for _, line := range chunk.Lines {
		if line.Mark == "o" {
			continue
		}
		if len(lineNums) == 0 {
			if quote == "" || strings.Contains(strings.ToLower(line.Line.Content), strings.ToLower(quote)) {
				return true
			}
			continue
		}
		if _, ok := lineNums[line.Line.LineNo]; ok {
			return true
		}
	}
	return false
}

func mergeSceneCandidateMentions(mentions []sceneCandidateMention) []sceneCandidate {
	type bucket struct {
		mentions []sceneCandidateMention
	}
	grouped := map[string]*bucket{}
	order := make([]string, 0, len(mentions))
	for _, mention := range mentions {
		key := normalizedSceneCandidateKey(mention.SceneKey, mention.Title, mention.SummaryHint)
		if key == "" {
			continue
		}
		if grouped[key] == nil {
			grouped[key] = &bucket{}
			order = append(order, key)
		}
		grouped[key].mentions = append(grouped[key].mentions, mention)
	}

	out := make([]sceneCandidate, 0, len(order))
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
		lineMap := map[string]MarkedLine{}
		sceneKey := ""
		title := ""
		summaryHint := ""
		sceneType := "other"
		typeCounts := map[string]int{}
		for _, mention := range b.mentions {
			sceneKey = firstNonEmptyTrimmed(sceneKey, mention.SceneKey)
			title = firstNonEmptyTrimmed(title, mention.Title)
			summaryHint = firstNonEmptyTrimmed(summaryHint, mention.SummaryHint)
			if t := strings.TrimSpace(mention.SceneTypeHint); t != "" {
				typeCounts[t]++
			}
			supportMentions = append(supportMentions, map[string]any{
				"scene_key":         mention.SceneKey,
				"title":             mention.Title,
				"summary_hint":      mention.SummaryHint,
				"evidence_quote":    mention.EvidenceQuote,
				"line_spans":        mention.LineSpans,
				"confidence":        mention.Confidence,
				"confidence_reason": mention.ConfidenceReason,
			})
			for _, line := range mention.ChunkLines {
				lineKey := fmt.Sprintf("%d:%d:%s", line.Line.PageNo, line.Line.LineNo, line.Line.Content)
				existing, exists := lineMap[lineKey]
				if !exists || (existing.Mark == "o" && line.Mark != "o") {
					lineMap[lineKey] = line
				}
			}
		}
		for candidateType, count := range typeCounts {
			if count > typeCounts[sceneType] {
				sceneType = candidateType
			}
		}
		if sceneKey == "" {
			sceneKey = normalizedSceneCandidateKey("", title, summaryHint)
		}
		if sceneType == "" {
			sceneType = "other"
		}
		supportLines := make([]MarkedLine, 0, len(lineMap))
		for _, line := range lineMap {
			supportLines = append(supportLines, line)
		}
		sort.Slice(supportLines, func(i, j int) bool {
			if supportLines[i].Line.PageNo != supportLines[j].Line.PageNo {
				return supportLines[i].Line.PageNo < supportLines[j].Line.PageNo
			}
			if supportLines[i].Line.LineNo != supportLines[j].Line.LineNo {
				return supportLines[i].Line.LineNo < supportLines[j].Line.LineNo
			}
			return supportLines[i].Mark < supportLines[j].Mark
		})
		out = append(out, sceneCandidate{
			CandidateID:        fmt.Sprintf("scene_cand_%d", len(out)+1),
			SceneKey:           sceneKey,
			SceneTypeHint:      sceneType,
			Title:              title,
			SummaryHint:        summaryHint,
			SupportingMentions: supportMentions,
			SupportLines:       supportLines,
			PrimaryChunkIndex:  b.mentions[0].ChunkIndex,
		})
	}
	return out
}

func normalizedSceneCandidateKey(sceneKey string, title string, summaryHint string) string {
	base := firstNonEmptyTrimmed(sceneKey, title, summaryHint)
	base = strings.ToLower(strings.TrimSpace(base))
	base = strings.Map(func(r rune) rune {
		switch {
		case unicode.IsLetter(r):
			return unicode.ToLower(r)
		case unicode.IsNumber(r):
			return r
		case r == '_':
			return r
		case unicode.IsSpace(r):
			return r
		default:
			return ' '
		}
	}, base)
	return strings.Join(strings.Fields(base), " ")
}

func normalizeSceneBlockList(items []any, candidate sceneCandidate) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		raw, ok := item.(map[string]any)
		if !ok {
			continue
		}
		block := map[string]any{}
		for k, v := range raw {
			block[k] = v
		}
		if strings.TrimSpace(asString(block["scene_id"])) == "" {
			block["scene_id"] = candidate.SceneKey
		}
		if strings.TrimSpace(asString(block["scene_type"])) == "" {
			block["scene_type"] = candidate.SceneTypeHint
		}
		if strings.TrimSpace(asString(block["title"])) == "" {
			block["title"] = candidate.Title
		}
		if strings.TrimSpace(asString(block["summary"])) == "" {
			block["summary"] = candidate.SummaryHint
		}
		if len(normalizeProductEvidenceLines(block["line_spans"])) == 0 {
			block["line_spans"] = flattenCandidateLineSpans(candidate.SupportingMentions)
		}
		out = append(out, block)
	}
	return out
}

func groupSceneCandidatesByChunk(candidates []sceneCandidate) []candidateChunkGroup {
	order := make([]int, 0, len(candidates))
	seen := map[int]bool{}
	groups := make(map[int][]sceneCandidate)
	for _, c := range candidates {
		ci := c.PrimaryChunkIndex
		if !seen[ci] {
			seen[ci] = true
			order = append(order, ci)
		}
		groups[ci] = append(groups[ci], c)
	}
	out := make([]candidateChunkGroup, 0, len(order))
	for _, ci := range order {
		out = append(out, candidateChunkGroup{ChunkIndex: ci, Candidates: groups[ci]})
	}
	return out
}

func normalizeSceneBlockListForGroup(items []any, candidates []sceneCandidate) []map[string]any {
	if len(candidates) == 1 {
		return normalizeSceneBlockList(items, candidates[0])
	}
	candByKey := make(map[string]sceneCandidate, len(candidates))
	for _, c := range candidates {
		if key := strings.ToLower(strings.TrimSpace(c.SceneKey)); key != "" {
			candByKey[key] = c
		}
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		raw, ok := item.(map[string]any)
		if !ok {
			continue
		}
		block := map[string]any{}
		for k, v := range raw {
			block[k] = v
		}
		sid := strings.ToLower(strings.TrimSpace(asString(block["scene_id"])))
		if cand, ok := candByKey[sid]; ok {
			if strings.TrimSpace(asString(block["scene_id"])) == "" {
				block["scene_id"] = cand.SceneKey
			}
			if strings.TrimSpace(asString(block["scene_type"])) == "" {
				block["scene_type"] = cand.SceneTypeHint
			}
			if strings.TrimSpace(asString(block["title"])) == "" {
				block["title"] = cand.Title
			}
			if strings.TrimSpace(asString(block["summary"])) == "" {
				block["summary"] = cand.SummaryHint
			}
			if len(normalizeProductEvidenceLines(block["line_spans"])) == 0 {
				block["line_spans"] = flattenCandidateLineSpans(cand.SupportingMentions)
			}
		}
		out = append(out, block)
	}
	return out
}

func flattenCandidateLineSpans(mentions []map[string]any) []string {
	out := make([]string, 0, len(mentions))
	for _, mention := range mentions {
		for _, span := range normalizeProductEvidenceLines(mention["line_spans"]) {
			out = appendUniqueString(out, span)
		}
	}
	return out
}

func dedupeFinalSceneBlocks(sceneBlocks []map[string]any) []map[string]any {
	grouped := map[string]map[string]any{}
	order := make([]string, 0, len(sceneBlocks))
	for _, block := range sceneBlocks {
		key := strings.Join([]string{
			normalizedSceneCandidateKey(asString(block["scene_id"]), asString(block["title"]), asString(block["summary"])),
			strings.ToLower(strings.TrimSpace(asString(block["scene_type"]))),
		}, "|")
		if existing, ok := grouped[key]; ok {
			mergedSpans := normalizeProductEvidenceLines(existing["line_spans"])
			for _, span := range normalizeProductEvidenceLines(block["line_spans"]) {
				mergedSpans = appendUniqueString(mergedSpans, span)
			}
			existing["line_spans"] = mergedSpans
			if toFloat(block["confidence"]) > toFloat(existing["confidence"]) {
				existing["confidence"] = block["confidence"]
			}
			if strings.TrimSpace(asString(existing["summary"])) == "" {
				existing["summary"] = block["summary"]
			}
			continue
		}
		cloned := map[string]any{}
		for k, v := range block {
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

func (p *SceneBlocksProcessor) writeSceneBlocksArtifact(recordID int64, rec DocMetadataInputRecord, sceneBlocks []map[string]any) error {
	artifactDir := strings.TrimSpace(p.ArtifactDir)
	if artifactDir == "" {
		artifactDir = strings.TrimSpace(os.Getenv("ARTIFACT_DIR"))
	}
	if artifactDir == "" {
		return errors.New("(MID_26051815) missing ARTIFACT_DIR")
	}
	if recordID <= 0 {
		return fmt.Errorf("(MID_26051816) invalid record_id: %d", recordID)
	}
	outDir, err := buildRecordArtifactDir(artifactDir, recordID)
	if err != nil {
		return fmt.Errorf("(MID_26051817) create artifact dir: %w", err)
	}
	artifactBase := buildChunkArtifactBaseName(rec.StagingFilename, rec.ParserName)
	outPath := filepath.Join(outDir, artifactBase+".scene_blocks")
	nowFn := p.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	stampedSceneBlocks := addSceneBlockCreateTimes(sceneBlocks, nowFn())
	bs, err := json.MarshalIndent(stampedSceneBlocks, "", "  ")
	if err != nil {
		return fmt.Errorf("(MID_26051818) marshal scene blocks: %w", err)
	}
	if err := os.WriteFile(outPath, bs, 0o644); err != nil {
		return fmt.Errorf("(MID_26051819) write scene blocks artifact: %w", err)
	}
	return nil
}

func addSceneBlockCreateTimes(sceneBlocks []map[string]any, now time.Time) []map[string]any {
	if len(sceneBlocks) == 0 {
		return sceneBlocks
	}
	createTime := now.Format("20060102-150405")
	out := make([]map[string]any, 0, len(sceneBlocks))
	for _, block := range sceneBlocks {
		if block == nil {
			out = append(out, nil)
			continue
		}
		cloned := make(map[string]any, len(block)+1)
		for k, v := range block {
			cloned[k] = v
		}
		if strings.TrimSpace(asString(cloned["create_time"])) == "" {
			cloned["create_time"] = createTime
		}
		out = append(out, cloned)
	}
	return out
}

type sceneBlocksStatusParams struct {
	RecordID      int64
	FileType      string
	InputFilename string
	Start         time.Time
	DurationMs    int64
	ProcStatus    string
	Progress      string
	ProcErr       error
	ModelName     string
	PromptName    string
}

func (p *SceneBlocksProcessor) persistSceneBlocksStatus(ctx context.Context, rec DocMetadataInputRecord, start time.Time, procErr error) {
	errMsg := (*string)(nil)
	if procErr != nil {
		msg := strings.TrimSpace(procErr.Error())
		errMsg = &msg
	}
	if err := updateInputStatusAtomic(ctx, p.InputStore, rec.ID, func(current string) (DocMetadataUpdate, error) {
		statusRaw, err := appendSceneBlocksStatus(current, sceneBlocksStatusParams{
			RecordID:      rec.ID,
			FileType:      detectSceneBlocksFileType(rec),
			InputFilename: strings.TrimSpace(rec.ResultFilename),
			Start:         start,
			DurationMs:    time.Since(start).Milliseconds(),
			ProcErr:       procErr,
			ModelName:     strings.TrimSpace(p.ModelName),
			PromptName:    strings.TrimSpace(p.PromptRef),
		})
		if err != nil {
			return DocMetadataUpdate{}, err
		}
		return DocMetadataUpdate{StatusRaw: statusRaw, ErrorMsg: errMsg}, nil
	}); err != nil {
		p.Logger.Error("failed persisting scene blocks status", "record_id", rec.ID, "error", err)
	}
}

func (p *SceneBlocksProcessor) persistSceneBlocksInProgressStatus(ctx context.Context, rec DocMetadataInputRecord, start time.Time, progress string) {
	if err := updateInputStatusAtomic(ctx, p.InputStore, rec.ID, func(current string) (DocMetadataUpdate, error) {
		statusRaw, err := appendSceneBlocksStatus(current, sceneBlocksStatusParams{
			RecordID:      rec.ID,
			FileType:      detectSceneBlocksFileType(rec),
			InputFilename: strings.TrimSpace(rec.ResultFilename),
			Start:         start,
			DurationMs:    time.Since(start).Milliseconds(),
			ProcStatus:    "in_progress",
			Progress:      progress,
			ModelName:     strings.TrimSpace(p.ModelName),
			PromptName:    strings.TrimSpace(p.PromptRef),
		})
		if err != nil {
			return DocMetadataUpdate{}, err
		}
		return DocMetadataUpdate{StatusRaw: statusRaw}, nil
	}); err != nil {
		p.Logger.Warn("failed persisting scene blocks in-progress status", "record_id", rec.ID, "error", err)
	}
}

func (p *SceneBlocksProcessor) stopAndPersistSceneBlocks(ctx context.Context, rec DocMetadataInputRecord, start time.Time) {
	if updateErr := updateInputStatusAtomic(ctx, p.InputStore, rec.ID, func(current string) (DocMetadataUpdate, error) {
		statusRaw, err := appendSceneBlocksStatus(current, sceneBlocksStatusParams{
			RecordID:      rec.ID,
			FileType:      detectSceneBlocksFileType(rec),
			InputFilename: strings.TrimSpace(rec.ResultFilename),
			Start:         start,
			DurationMs:    time.Since(start).Milliseconds(),
			ProcStatus:    "stopped",
			ModelName:     strings.TrimSpace(p.ModelName),
			PromptName:    strings.TrimSpace(p.PromptRef),
		})
		if err != nil {
			return DocMetadataUpdate{}, err
		}
		return DocMetadataUpdate{StatusRaw: statusRaw}, nil
	}); updateErr != nil {
		p.Logger.Error("(MID_26052842) failed persisting scene blocks stopped status", "record_id", rec.ID, "error", updateErr)
	}
	p.Logger.Info("(MID_26052843) generate_scene_blocks stopped by user request", "record_id", rec.ID)
}

func detectSceneBlocksFileType(rec DocMetadataInputRecord) string {
	for _, candidate := range []string{rec.FileName, rec.StagingFilename, rec.ResultFilename} {
		ext := strings.ToLower(strings.TrimSpace(filepath.Ext(strings.TrimSpace(candidate))))
		if ext != "" {
			return strings.TrimPrefix(ext, ".")
		}
	}
	return ""
}

func appendSceneBlocksStatus(raw string, p sceneBlocksStatusParams) (string, error) {
	entries := decodeDocMetaStatus(raw)
	operation := "extract_scene_blocks"
	if p.ProcErr != nil {
		operation = "generate_scene_blocks"
	}
	entry := map[string]any{
		"record_id":      strconv.FormatInt(p.RecordID, 10),
		"file_type":      strings.ToLower(strings.TrimSpace(p.FileType)),
		"operation":      operation,
		"input_filename": strings.TrimSpace(p.InputFilename),
		"start_time":     p.Start.Format(defaultDocMetaStatusTime),
		"ms_used":        p.DurationMs,
		"model_name":     p.ModelName,
		"prompt_name":    p.PromptName,
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
		if op != "extract_scene_blocks" && op != "generate_scene_blocks" {
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

func (p *SceneBlocksProcessor) indexSceneBlocks(recordID int64, sceneBlocks []map[string]any) error {
	if strings.TrimSpace(p.ArtifactWebDir) == "" {
		return fmt.Errorf("(MID_26051832) missing ARTIFACT_WEB_DIR")
	}
	return indexSceneBlocksInTreeDir(p.Logger, p.ArtifactWebDir, recordID, sceneBlocks)
}

func indexSceneBlocksInTreeDir(logger ApiTypes.JimoLogger, treeRootDir string, recordID int64, sceneBlocks []map[string]any) error {
	if strings.TrimSpace(treeRootDir) == "" {
		return fmt.Errorf("(MID_26051833) scene block tree root dir is empty")
	}
	if recordID <= 0 {
		return fmt.Errorf("(MID_26051834) invalid record id: %d", recordID)
	}
	if err := os.MkdirAll(treeRootDir, 0o755); err != nil {
		return err
	}
	if err := removeSceneBlockTreeRecord(treeRootDir, recordID); err != nil {
		return fmt.Errorf("(MID_26051835) remove old scene block tree entries for record %d: %w", recordID, err)
	}
	now := time.Now()
	for _, block := range sceneBlocks {
		objectID := strings.TrimSpace(asString(block["object_id"]))
		if objectID == "" {
			continue
		}
		for _, pair := range pairCategoryPathEntries(block["category_paths"], block["category_paths_en"]) {
			if err := writeSceneBlockTreeEntry(logger, treeRootDir, objectID, pair.Index, pair.Original, now); err != nil {
				return fmt.Errorf("(MID_26051836) index scene block %s path: %w", objectID, err)
			}
		}
	}
	return nil
}

func writeSceneBlockTreeEntry(logger ApiTypes.JimoLogger, treeRootDir string, objectID string, indexEntry CategoryPathEntry, originalEntry CategoryPathEntry, now time.Time) error {
	leafDir, err := categoryTreeLeafDirForEntry(logger, treeRootDir, indexEntry, originalEntry, now)
	if err != nil {
		return err
	}
	if leafDir == "" {
		return nil
	}
	return upsertSceneBlockToLeafDir(leafDir, objectID)
}

func upsertSceneBlockToLeafDir(leafDir string, objectID string) error {
	filePath := filepath.Join(leafDir, "scenes.txt")
	existing := make([]string, 0)
	if bs, err := os.ReadFile(filePath); err == nil {
		for _, row := range strings.Split(string(bs), "\n") {
			row = strings.TrimSpace(row)
			if row != "" {
				existing = append(existing, row)
			}
		}
	}
	existing = appendUniqueString(existing, objectID)
	sort.Strings(existing)
	return os.WriteFile(filePath, []byte(strings.Join(existing, "\n")), 0o644)
}

func removeSceneBlockTreeRecord(treeRootDir string, recordID int64) error {
	prefix := strconv.FormatInt(recordID, 10) + "_"
	return filepath.WalkDir(treeRootDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != "scenes.txt" {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rows := make([]string, 0)
		for _, row := range strings.Split(string(body), "\n") {
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

func loadSceneBlocksPromptFromEnv() (promptText string, promptRef string, promptErr error) {
	return loadScenePromptFromEnvKeys([]string{"ENRICH_SCENE_BLOCKS_PROMPT", "EXTRACT_SCENE_BLOCKS_PROMPT"}, "prompt-enrich-scene-blocks-v1.md")
}

func loadScenePromptFromEnvKeys(envKeys []string, defaultRef string) (promptText string, promptRef string, promptErr error) {
	for _, key := range envKeys {
		promptRef = strings.TrimSpace(os.Getenv(key))
		if promptRef != "" {
			break
		}
	}
	if promptRef == "" {
		promptRef = defaultRef
	}

	paths := make([]string, 0, 8)
	addCandidate := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" || slices.Contains(paths, p) {
			return
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
		addCandidate(filepath.Join("prompts", promptRef))
	}

	var lastErr error
	for _, candidate := range paths {
		bs, err := os.ReadFile(candidate)
		if err != nil {
			lastErr = fmt.Errorf("(MID_26051820) failed reading file. Path:%s, error:%w", candidate, err)
			continue
		}
		text := strings.TrimSpace(string(bs))
		if text == "" {
			return "", promptRef, fmt.Errorf("(MID_26051821) prompt file is empty")
		}
		return text, promptRef, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("(MID_26051822) no candidate path available")
	}
	return "", promptRef, fmt.Errorf("(MID_26051823) prompt file not found: %w", lastErr)
}

func (s SceneObjectsSQLStore) ensureReady() error {
	if s.DB == nil {
		return fmt.Errorf("(MID_26051824) db is nil")
	}
	return nil
}

func (s SceneObjectsSQLStore) SceneObjectsExist(ctx context.Context, inputRecordID int64) (bool, error) {
	if err := s.ensureReady(); err != nil {
		return false, err
	}
	const q = `
	SELECT 1
	FROM kb.scene_objects
	WHERE input_record_id = $1
	LIMIT 1`
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

func (s SceneObjectsSQLStore) DeleteSceneObjectsByInputRecordID(ctx context.Context, inputRecordID int64) (int64, error) {
	if err := s.ensureReady(); err != nil {
		return 0, err
	}
	res, err := s.DB.ExecContext(ctx, `DELETE FROM kb.scene_objects WHERE input_record_id = $1`, inputRecordID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s SceneObjectsSQLStore) UpsertSceneObject(ctx context.Context, req UpsertSceneObjectRequest) error {
	if err := s.ensureReady(); err != nil {
		return err
	}
	block := req.SceneBlock
	if block == nil {
		block = map[string]any{}
	}

	toJSON := func(v any) string {
		if v == nil {
			return "null"
		}
		bs, _ := json.Marshal(v)
		return string(bs)
	}
	extInfo := req.ExtInfo
	if extInfo == nil {
		extInfo = map[string]any{}
	}

	const stmt = `
	INSERT INTO kb.scene_objects (
		object_id, input_record_id, event_id, scene_id, scene_type, title, summary,
		actors, resources, preconditions, triggers, states, actions, constraints,
		decisions, outcomes, failure_modes, root_causes, resolutions, relationships,
		discriminators, keywords, confidence, source_refs, model_name, prompt_name, ext_info,
		create_time, modify_time
	) VALUES (
		$1, $2, $3, $4, $5, $6, $7,
		$8::jsonb, $9::jsonb, $10::jsonb, $11::jsonb, $12::jsonb, $13::jsonb, $14::jsonb,
		$15::jsonb, $16::jsonb, $17::jsonb, $18::jsonb, $19::jsonb, $20::jsonb,
		$21::jsonb, $22::jsonb, $23, $24::jsonb, $25, $26, $27::jsonb,
		NOW(), NOW()
	)
	ON CONFLICT (input_record_id, object_id) DO UPDATE SET
		event_id = EXCLUDED.event_id,
		scene_id = EXCLUDED.scene_id,
		scene_type = EXCLUDED.scene_type,
		title = EXCLUDED.title,
		summary = EXCLUDED.summary,
		actors = EXCLUDED.actors,
		resources = EXCLUDED.resources,
		preconditions = EXCLUDED.preconditions,
		triggers = EXCLUDED.triggers,
		states = EXCLUDED.states,
		actions = EXCLUDED.actions,
		constraints = EXCLUDED.constraints,
		decisions = EXCLUDED.decisions,
		outcomes = EXCLUDED.outcomes,
		failure_modes = EXCLUDED.failure_modes,
		root_causes = EXCLUDED.root_causes,
		resolutions = EXCLUDED.resolutions,
		relationships = EXCLUDED.relationships,
		discriminators = EXCLUDED.discriminators,
		keywords = EXCLUDED.keywords,
		confidence = EXCLUDED.confidence,
		source_refs = EXCLUDED.source_refs,
		model_name = EXCLUDED.model_name,
		prompt_name = EXCLUDED.prompt_name,
		ext_info = EXCLUDED.ext_info,
		modify_time = NOW()`

	_, err := s.DB.ExecContext(ctx, stmt,
		strings.TrimSpace(req.ObjectID),                  // $1
		req.InputRecordID,                                // $2
		strings.TrimSpace(req.EventID),                   // $3
		strings.TrimSpace(asString(block["scene_id"])),   // $4
		strings.TrimSpace(asString(block["scene_type"])), // $5
		strings.TrimSpace(asString(block["title"])),      // $6
		strings.TrimSpace(asString(block["summary"])),    // $7
		toJSON(block["actors"]),                          // $8
		toJSON(block["resources"]),                       // $9
		toJSON(block["preconditions"]),                   // $10
		toJSON(block["triggers"]),                        // $11
		toJSON(block["states"]),                          // $12
		toJSON(block["actions"]),                         // $13
		toJSON(block["constraints"]),                     // $14
		toJSON(block["decisions"]),                       // $15
		toJSON(block["outcomes"]),                        // $16
		toJSON(block["failure_modes"]),                   // $17
		toJSON(block["root_causes"]),                     // $18
		toJSON(block["resolutions"]),                     // $19
		toJSON(block["relationships"]),                   // $20
		toJSON(block["discriminators"]),                  // $21
		toJSON(block["keywords"]),                        // $22
		toFloat(block["confidence"]),                     // $23
		toJSON(block["source_refs"]),                     // $24
		strings.TrimSpace(req.ModelName),                 // $25
		strings.TrimSpace(req.PromptName),                // $26
		toJSON(extInfo),                                  // $27
	)
	return err
}
