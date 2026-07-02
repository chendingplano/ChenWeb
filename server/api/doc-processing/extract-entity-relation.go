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
	"sync"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/ApiUtils"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

// EntityRelationProcessor extracts entities and relations from chunks using
// a single LLM call per chunk. It writes to kb.entities and kb.relations,
// emits .entities and .relations artifact files, and refreshes the hybrid
// search registry. It does NOT generate category paths and does NOT touch
// ARTIFACT_WEB_DIR.
type EntityRelationProcessor struct {
	InputStore                    DocMetadataStore
	Store                         EntityRelationStore
	Extractor                     LLMJSONExtractor
	Logger                        ApiTypes.JimoLogger
	ProcLogger                    DocProcLogger
	Now                           func() time.Time
	PromptText                    string
	PromptRef                     string
	PromptPath                    string
	PromptErr                     error
	RelationPromptText            string
	RelationPromptRef             string
	RelationPromptPath            string
	RelationPromptErr             error
	RelationWindowSize            int
	RelationWindowOverlap         int
	ModelRef                      string
	ModelCfgPath                  string
	ModelErr                      error
	ModelName                     string
	ModelCfg                      structureModelConfig
	FallbackModelRef              string
	FallbackModelPath             string
	FallbackModelErr              error
	FallbackModelName             string
	FallbackModelCfg              structureModelConfig
	LLMController                 llmPermitController
	ArtifactDir                   string
	ExtractEntityRelationMaxTasks int
	// StatusOperation is the doc_proc status operation this processor logs under.
	// Empty defaults to "extract_entity_relation" (the legacy combined processor).
	// The split processors (ADR 2026061702) set "extract_entity" / "extract_relation".
	StatusOperation string

	// batch state (set by ChunkBatchProcessor.InitChunkBatch)
	batchRecordID  int64
	batchChunks    []Chunk
	batchDocCtx    string
	batchLines     []Line // input lines for Phase 2 relation windows
	batchResults   []entityRelationChunkResult
	batchMu        sync.Mutex // protects batchResults/batchRelations append under concurrent Phase 3
	batchStart     time.Time
	batchRec       DocMetadataInputRecord
	batchRelations []map[string]any // accumulated by RelationProcessor.ProcessChunk
	batchRelLang   string           // detected language for relation batch
}

// statusOp returns the doc_proc operation name for status entries.
func (p *EntityRelationProcessor) statusOp() string {
	if op := strings.TrimSpace(p.StatusOperation); op != "" {
		return op
	}
	return "extract_entity_relation"
}

// Entity provenance markers stored in kb.entities.entity_status (ADR 2026061302 D4).
const (
	// entityStatusExtracted marks entities produced by Phase 1 extraction.
	entityStatusExtracted = "extracted"
	// entityStatusProvisional marks entities auto-created during relation
	// endpoint resolution because the endpoint matched no extracted entity.
	entityStatusProvisional = "provisional"
)

type EntityRelationStore interface {
	EntitiesExist(ctx context.Context, inputRecordID int64) (bool, error)
	DeleteEntitiesByInputRecordID(ctx context.Context, inputRecordID int64) (int64, error)
	SaveEntities(ctx context.Context, req SaveEntitiesRequest) (int64, error)
	RelationsExist(ctx context.Context, inputRecordID int64) (bool, error)
	DeleteRelationsByInputRecordID(ctx context.Context, inputRecordID int64) (int64, error)
	SaveRelations(ctx context.Context, req SaveRelationsRequest) (int64, error)
}

type EntityRelationSQLStore struct {
	DB *sql.DB
}

type SaveEntitiesRequest struct {
	InputRecordID int64
	EventID       string
	Language      string
	ModelName     string
	PromptName    string
	DocName       string
	Entities      []map[string]any
}

type SaveRelationsRequest struct {
	InputRecordID int64
	EventID       string
	Language      string
	ModelName     string
	PromptName    string
	Relations     []map[string]any
}

type entityRelationExtractionResult struct {
	Language      string
	Entities      []map[string]any
	Relations     []map[string]any
	ModelName     string
	LLMCallCount  int
	FallbackCount int
	FailedChunks  int
}

// entityRelationChunkResult holds the output of one concurrent chunk worker.
// LLM errors are captured as Failed=true so siblings are not cancelled.
type entityRelationChunkResult struct {
	Entities      []map[string]any
	Relations     []map[string]any
	Language      string
	ModelName     string
	LLMCallCount  int
	FallbackCount int
	Failed        bool
}

func NewEntityRelationProcessor(
	inputStore DocMetadataStore,
	store EntityRelationStore,
	extractor LLMJSONExtractor,
	_ ApiTypes.JimoLogger,
) *EntityRelationProcessor {
	logger := loggerutil.CreateDefaultLogger("MID_26052701")
	promptText, promptRef, promptPath, promptErr := loadProductPromptFromEnvKeys(
		[]string{"EXTRACT_ENTITY_RELATION_PROMPT"},
		"prompt-extract-entity-relation-v3.md",
	)
	// No default: the relation prompt is supplied via EXTRACT_RELATION_PROMPT. If
	// unset, RelationPromptErr is non-nil and Phase 2 (D5) is skipped with a warning.
	relPromptText, relPromptRef, relPromptPath, relPromptErr := loadProductPromptFromEnvKeys(
		[]string{"EXTRACT_RELATION_PROMPT"},
		"",
	)
	modelRef, modelCfgPath, modelCfg, modelErr := loadModelConfigFromEnvKeys(
		[]string{"EXTRACT_ENTITY_RELATION_MODEL_NAME"},
		"MODEL_DEF_FILE",
	)
	fallbackModelRef, fallbackModelPath, fallbackModelCfg, fallbackModelErr := loadOptionalModelConfigFromEnv(
		"EXTRACT_ENTITY_RELATION_FALLBACK",
		"MODEL_DEF_FILE",
	)
	applyStructureModelConfigToExtractor(extractor, modelCfg)
	return &EntityRelationProcessor{
		InputStore:                    inputStore,
		Store:                         store,
		Extractor:                     extractor,
		Logger:                        logger,
		ProcLogger:                    DocProcLogger{DB: ApiTypes.ProjectDBHandle},
		Now:                           time.Now,
		PromptText:                    promptText,
		PromptRef:                     promptRef,
		PromptPath:                    promptPath,
		PromptErr:                     promptErr,
		RelationPromptText:            relPromptText,
		RelationPromptRef:             relPromptRef,
		RelationPromptPath:            relPromptPath,
		RelationPromptErr:             relPromptErr,
		RelationWindowSize:            envInt("RELATION_WINDOW_SIZE", 20, 1),    // pages (Change 01)
		RelationWindowOverlap:         envInt("RELATION_WINDOW_OVERLAP", 20, 0), // lines
		ModelRef:                      modelRef,
		ModelCfgPath:                  modelCfgPath,
		ModelErr:                      modelErr,
		ModelName:                     modelCfg.ModelName,
		ModelCfg:                      modelCfg,
		FallbackModelRef:              fallbackModelRef,
		FallbackModelPath:             fallbackModelPath,
		FallbackModelErr:              fallbackModelErr,
		FallbackModelName:             fallbackModelCfg.ModelName,
		FallbackModelCfg:              fallbackModelCfg,
		LLMController:                 defaultDocProcessorLLMController,
		ArtifactDir:                   strings.TrimSpace(os.Getenv("ARTIFACT_DIR")),
		ExtractEntityRelationMaxTasks: envInt("EXTRACT_ENTITY_RELATION_MAX_TASKS", 1, 1),
	}
}

func (p *EntityRelationProcessor) Name() string { return "extract_entity_relation" }

// buildDocContextLine returns a compact one-line document description from the
// record's title and doc_metadata. Returns "" when no useful info is available.
func buildDocContextLine(rec DocMetadataInputRecord) string {
	var parts []string
	if t := strings.TrimSpace(rec.Title); t != "" {
		parts = append(parts, t)
	}
	if rec.DocMetadataJSON != "" {
		var m map[string]any
		if err := json.Unmarshal([]byte(rec.DocMetadataJSON), &m); err == nil {
			if dn := strings.TrimSpace(asString(m["doc_no"])); dn != "" {
				// Include doc_no only if it's not already embedded in the title.
				if !strings.Contains(strings.ToLower(strings.Join(parts, " ")), strings.ToLower(dn)) {
					parts = append(parts, dn)
				}
			}
		}
	}
	return strings.Join(parts, " | ")
}

// wrapLinesWithDocContext wraps a lines JSON array in a context envelope:
//
//	{"doc_context": "<title and doc_no>", "lines": [...]}
//
// Both extraction prompts (entity v3, relation v2) describe this input shape.
// Falls back to the bare linesJSON when docCtx is empty.
func wrapLinesWithDocContext(linesJSON, docCtx string) string {
	docCtx = strings.TrimSpace(docCtx)
	if docCtx == "" {
		return linesJSON
	}
	type envelope struct {
		DocContext string          `json:"doc_context"`
		Lines      json.RawMessage `json:"lines"`
	}
	bs, err := json.Marshal(envelope{DocContext: docCtx, Lines: json.RawMessage(linesJSON)})
	if err != nil {
		return linesJSON
	}
	return string(bs)
}

func (p *EntityRelationProcessor) HandleEvent(ctx context.Context, payload []byte) error {
	start := p.Now()
	evt, err := ParseLineFileGeneratedEvent(payload)
	if err != nil {
		p.Logger.Error("parse input file failed", "error", err)
		return fmt.Errorf("(MID_26052702) parse event payload: %w", err)
	}
	ctx = withLLMRecordID(ctx, evt.RecordID)
	if ShouldSkipLineFileGeneratedEvent(evt) {
		p.Logger.Warn("entity-relation processor skipped")
		return nil
	}
	if p.PromptErr != nil {
		p.Logger.Error("prompt error", "error", p.PromptErr)
		return fmt.Errorf("(MID_26052703) load entity-relation prompt %q: %w", p.PromptRef, p.PromptErr)
	}

	rec, err := p.InputStore.GetInputRecord(ctx, evt.RecordID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			p.Logger.Error("kb.inputs record not found", "record_id", evt.RecordID)
			return nil
		}
		p.Logger.Error("retrieve record failed", "record_id", evt.RecordID, "error", err)
		return fmt.Errorf("(MID_26052704) load kb.inputs record %d: %w", evt.RecordID, err)
	}
	if p.ModelErr != nil {
		p.Logger.Warn("entity-relation extraction skipped: model config error",
			"record_id", evt.RecordID, "model_ref", p.ModelRef, "error", p.ModelErr)
		p.persistEntityRelationStatus(ctx, rec, start, p.ModelErr)
		return nil
	}

	lineFilePath, lineFileErr := ResolveInputFilePath(evt, rec.ResultFilename, rec.ParserName, rec.StagingFilename)
	if lineFileErr != nil {
		p.Logger.Error("resolve input file path failed", "record_id", evt.RecordID, "error", lineFileErr)
		p.persistEntityRelationStatus(ctx, rec, start, fmt.Errorf("(MID_26052705) resolve line file for record_id=%d: %w", evt.RecordID, lineFileErr))
		return nil
	}

	// If evt.Force is not true and entities and relations are already there,
	// the request will be skipped.
	if evt.Force {
		_, _ = p.Store.DeleteEntitiesByInputRecordID(ctx, evt.RecordID)
		_, _ = p.Store.DeleteRelationsByInputRecordID(ctx, evt.RecordID)
	} else {
		entitiesExist, eeErr := p.Store.EntitiesExist(ctx, evt.RecordID)
		if eeErr != nil {
			p.Logger.Error("check entities exist failed", "record_id", evt.RecordID, "error", eeErr)
			p.persistEntityRelationStatus(ctx, rec, start, eeErr)
			return fmt.Errorf("(MID_26052706) check entities for record_id=%d: %w", evt.RecordID, eeErr)
		}
		relationsExist, reErr := p.Store.RelationsExist(ctx, evt.RecordID)
		if reErr != nil {
			p.Logger.Error("check relations exist failed", "record_id", evt.RecordID, "error", reErr)
			p.persistEntityRelationStatus(ctx, rec, start, reErr)
			return fmt.Errorf("(MID_26052707) check relations for record_id=%d: %w", evt.RecordID, reErr)
		}
		if entitiesExist || relationsExist {
			p.Logger.Info("entity-relation extraction skipped",
				"record_id", evt.RecordID, "reason", "rows already exist and force=false")
			reindexExistingSearchOnSkip(ctx, searchArtifactEntity, evt.RecordID, p.Logger, ReindexEntitySearchForRecord)
			reindexExistingSearchOnSkip(ctx, searchArtifactRelation, evt.RecordID, p.Logger, ReindexRelationSearchForRecord)
			p.persistEntityRelationStatus(ctx, rec, start, nil)
			return nil
		}
	}

	// Ready to process.
	// Step 1: retrieve the chunks.
	body, readErr := os.ReadFile(lineFilePath)
	if readErr != nil {
		p.Logger.Error("read line file failed", "record_id", evt.RecordID, "path", lineFilePath, "error", readErr)
		p.persistEntityRelationStatus(ctx, rec, start, fmt.Errorf("(MID_26052708) read line file for record_id=%d: %w", evt.RecordID, readErr))
		return fmt.Errorf("(MID_26052709) failed reading line file, error:%w, path:%s", readErr, lineFilePath)
	}
	lines, parseErr := ParseInputLinesIncludingTOC(body)
	if parseErr != nil {
		p.Logger.Error("parse input lines failed", "record_id", evt.RecordID, "error", parseErr)
		p.persistEntityRelationStatus(ctx, rec, start, fmt.Errorf("(MID_26052710) parse input lines for record_id=%d: %w", evt.RecordID, parseErr))
		return fmt.Errorf("(MID_26052711) failed parsing input lines, error:%w", parseErr)
	}
	artifactBase := buildChunkArtifactBaseName(rec.StagingFilename, rec.ParserName)
	chunks, loadErr := loadChunksFromArtifactFile(p.ArtifactDir, evt.RecordID, artifactBase+".chunks", lines)
	if loadErr != nil {
		p.Logger.Error("load chunk artifact failed", "record_id", evt.RecordID, "artifact_base", artifactBase, "error", loadErr)
		p.persistEntityRelationStatus(ctx, rec, start, fmt.Errorf("(MID_26052712) load chunk artifact for record_id=%d: %w", evt.RecordID, loadErr))
		return fmt.Errorf("(MID_26052713) failed loading chunk artifact, error:%w", loadErr)
	}
	if len(chunks) == 0 {
		procErr := fmt.Errorf("(MID_26052714) no chunks found for record_id=%d", evt.RecordID)
		p.Logger.Error("no chunks", "record_id", evt.RecordID, "artifact_base", artifactBase)
		p.persistEntityRelationStatus(ctx, rec, start, procErr)
		return nil
	}

	// Step 2: extract entities
	p.persistEntityRelationInProgressStatus(ctx, rec, start, fmt.Sprintf("0%% (0/%d)", len(chunks)))
	result, err := p.extractEntitiesFromChunks(ctx, evt.RecordID, chunks, buildDocContextLine(rec))
	if err != nil {
		if errors.Is(err, ErrPipelineStopped) {
			p.stopAndPersistEntityRelation(context.Background(), rec, start)
			return ErrPipelineStopped
		}
		p.persistEntityRelationStatus(ctx, rec, start, err)
		return fmt.Errorf("(MID_26052717) extractEntityRelation failed, error:%w", err)
	}
	detectedLanguage := result.Language
	if detectedLanguage == "" {
		detectedLanguage = "unknown"
	}

	createTime := p.Now().UTC().Format(time.RFC3339)

	// Step 3: Consolidate entities
	// Phase 1.5 — consolidate duplicate entities extracted across chunks into a
	// single canonical entity before assigning ids (ADR 2026061302 D6), so a
	// relation endpoint resolves to exactly one entity_id.
	rawEntityCount := len(result.Entities)
	result.Entities = consolidateEntities(result.Entities)
	for i := range result.Entities {
		result.Entities[i]["entity_id"] = fmt.Sprintf("%d_ent_%d", evt.RecordID, i+1)
		result.Entities[i]["create_time"] = createTime
	}

	p.Logger.Info("consolidate entities",
		"record_id", evt.RecordID,
		"entities", len(result.Entities),
	)

	// Step 4: extract relations
	// Phase 2 (D5) — extract relations per overlapping positional window using the
	// entity-aware relation prompt. With the entity-only Phase 1 prompt this is the
	// sole source of relations (result.Relations is empty before this point). If
	// the relation prompt is unavailable or the pass fails, any relations the
	// Phase 1 prompt happened to return are kept as a fallback.
	if strings.TrimSpace(p.RelationPromptText) != "" && p.RelationPromptErr == nil {
		windows := buildRelationWindows(result.Entities, lines, p.RelationWindowSize, p.RelationWindowOverlap)
		windowRelations, relErr := p.extractRelationsFromWindows(ctx, evt.RecordID, windows)
		if relErr != nil {
			if errors.Is(relErr, ErrPipelineStopped) {
				p.stopAndPersistEntityRelation(context.Background(), rec, start)
				return ErrPipelineStopped
			}
			p.Logger.Warn("phase-2 relation extraction failed; keeping any phase-1 relations",
				"record_id", evt.RecordID, "error", relErr)
		} else {
			p.Logger.Info("phase-2 relation extraction",
				"record_id", evt.RecordID,
				"windows", len(windows),
				"window_relations", len(windowRelations),
				"replaced_phase1_relations", len(result.Relations),
			)
			result.Relations = windowRelations
		}
	} else {
		p.Logger.Warn("phase-2 relation prompt unavailable; relations rely on the phase-1 prompt only",
			"record_id", evt.RecordID, "relation_prompt_err", p.RelationPromptErr)
	}

	// D1/D4 — resolve every relation endpoint to a canonical entity_id; unresolved
	// endpoints are promoted to provisional entities (appended to result.Entities)
	// rather than dropped, and duplicate edges are removed.
	resolutionIndex := buildEntityResolutionIndex(result.Entities)
	consolidatedEntityCount := len(result.Entities)
	result.Relations, result.Entities = resolveAndLinkRelations(
		result.Relations, result.Entities, resolutionIndex,
		evt.RecordID, len(result.Entities)+1, createTime,
	)
	provisionalCount := len(result.Entities) - consolidatedEntityCount
	var unlinkedRelations int
	for i := range result.Relations {
		result.Relations[i]["relation_id"] = fmt.Sprintf("%d_rel_%d", evt.RecordID, i+1)
		result.Relations[i]["create_time"] = createTime
		if strings.TrimSpace(asString(result.Relations[i]["subject_entity_id"])) == "" ||
			strings.TrimSpace(asString(result.Relations[i]["object_entity_id"])) == "" {
			unlinkedRelations++
		}
	}
	if unlinkedRelations > 0 {
		p.Logger.Warn("relations with missing entity IDs after linking; reprocess with force=true to backfill old rows",
			"record_id", evt.RecordID,
			"unlinked_relations", unlinkedRelations,
		)
	}
	p.Logger.Info("entity-relation linking",
		"record_id", evt.RecordID,
		"raw_entities", rawEntityCount,
		"consolidated_entities", consolidatedEntityCount,
		"provisional_entities", provisionalCount,
		"linked_relations", len(result.Relations),
	)

	eventID := eventIDFromContext(ctx)
	insertedEntities, err := p.Store.SaveEntities(ctx, SaveEntitiesRequest{
		InputRecordID: evt.RecordID,
		EventID:       eventID,
		Language:      detectedLanguage,
		ModelName:     firstNonEmptyTrimmed(result.ModelName, p.ModelName),
		PromptName:    p.PromptRef,
		DocName:       strings.TrimSpace(rec.Title),
		Entities:      result.Entities,
	})
	if err != nil {
		p.persistEntityRelationStatus(ctx, rec, start, err)
		return fmt.Errorf("(MID_26052715) insert entities failed, error:%w", err)
	}
	insertedRelations, err := p.Store.SaveRelations(ctx, SaveRelationsRequest{
		InputRecordID: evt.RecordID,
		EventID:       eventID,
		Language:      detectedLanguage,
		ModelName:     firstNonEmptyTrimmed(result.ModelName, p.ModelName),
		PromptName:    p.PromptRef,
		Relations:     result.Relations,
	})
	if err != nil {
		p.persistEntityRelationStatus(ctx, rec, start, err)
		return fmt.Errorf("(MID_26052716) insert relations failed, error:%w", err)
	}

	if fileErr := p.saveEntitiesToFile(evt.RecordID, rec, result.Entities); fileErr != nil {
		p.Logger.Warn("save entities to file failed", "record_id", evt.RecordID, "error", fileErr)
	}
	if fileErr := p.saveRelationsToFile(evt.RecordID, rec, result.Relations); fileErr != nil {
		p.Logger.Warn("save relations to file failed", "record_id", evt.RecordID, "error", fileErr)
	}

	if reindexErr := ReindexEntitySearchForRecord(ctx, evt.RecordID, p.Logger); reindexErr != nil {
		p.Logger.Warn("reindex entity search registry failed", "record_id", evt.RecordID, "error", reindexErr)
	}
	if reindexErr := ReindexRelationSearchForRecord(ctx, evt.RecordID, p.Logger); reindexErr != nil {
		p.Logger.Warn("reindex relation search registry failed", "record_id", evt.RecordID, "error", reindexErr)
	}

	p.Logger.Info("entity-relation extracted",
		"record_id", evt.RecordID,
		"inserted_entities", insertedEntities,
		"inserted_relations", insertedRelations,
		"num_chunks", len(chunks),
		"language", detectedLanguage,
	)
	p.persistEntityRelationStatus(ctx, rec, start, nil)
	p.logEntityRelationSummary(ctx, start, p.Now(), result, insertedEntities, insertedRelations, len(chunks), evt.RecordID)
	return nil
}

func (p *EntityRelationProcessor) extractEntitiesFromChunks(
	ctx context.Context,
	recordID int64,
	chunks []Chunk,
	docCtx string,
) (entityRelationExtractionResult, error) {
	p.Logger.Info("extract entities", "total_chunks", len(chunks),
		"model_name", p.ModelName,
		"prompt", p.PromptRef,
	)

	chunkResults, runErr := runConcurrent(ctx, p.ExtractEntityRelationMaxTasks, len(chunks),
		func(workerCtx context.Context, i int) (entityRelationChunkResult, error) {
			if isCtxStopped(workerCtx) {
				return entityRelationChunkResult{}, ErrPipelineStopped
			}
			return p.processChunk(workerCtx, recordID, i, len(chunks), chunks[i], docCtx), nil
		},
	)
	if runErr != nil {
		if isCtxStopped(ctx) {
			return entityRelationExtractionResult{}, ErrPipelineStopped
		}
		return entityRelationExtractionResult{}, runErr
	}

	entities := make([]map[string]any, 0)
	relations := make([]map[string]any, 0)
	detectedLanguage := "unknown"
	usedModel := strings.TrimSpace(p.ModelName)
	var llmCallCount, fallbackCount, failedChunks int

	for _, r := range chunkResults {
		llmCallCount += r.LLMCallCount
		fallbackCount += r.FallbackCount
		if r.Failed {
			failedChunks++
			continue
		}
		if r.Language != "" && detectedLanguage == "unknown" {
			detectedLanguage = r.Language
		}
		if strings.TrimSpace(r.ModelName) != "" {
			usedModel = strings.TrimSpace(r.ModelName)
		}
		entities = append(entities, r.Entities...)
		relations = append(relations, r.Relations...)
	}

	p.Logger.Info("extract entities final results",
		"record_id", recordID,
		"total_chunks", len(chunks),
		"entities", len(entities),
		"relations", len(relations),
		"language", detectedLanguage,
	)
	return entityRelationExtractionResult{
		Language:      detectedLanguage,
		Entities:      entities,
		Relations:     relations,
		ModelName:     usedModel,
		LLMCallCount:  llmCallCount,
		FallbackCount: fallbackCount,
		FailedChunks:  failedChunks,
	}, nil
}

func (p *EntityRelationProcessor) processChunk(
	ctx context.Context,
	recordID int64,
	idx int,
	totalChunks int,
	chunk Chunk,
	docCtx string,
) entityRelationChunkResult {
	chunkText := canonicalChunkInputText(chunk.Lines, docCtx)
	callStart := p.Now()
	p.Logger.Info("extract entities start",
		"record_id", recordID,
		"chunk_idx", idx,
		"seq_no", chunk.SeqNo,
	)
	callID := fmt.Sprintf("%s_p1_c%d", eventIDFromContext(ctx), idx)
	payload, modelName, err := p.extractEntityRelationWithFallback(ctx, chunkText)

	llmCallCount := 1
	fallbackCount := 0
	if strings.TrimSpace(modelName) != strings.TrimSpace(p.ModelName) && strings.TrimSpace(modelName) != "" {
		fallbackCount = 1
	}

	var chunkEntityCount, chunkRelationCount int
	var chunkEntities []map[string]any
	var chunkRelations []map[string]any
	var detectedLanguage string

	if err == nil && payload != nil {
		if lang := strings.TrimSpace(asString(payload["language"])); lang != "" {
			detectedLanguage = lang
		}
		chunkEntities = normalizeEntityRows(payload["entities"], chunk.SeqNo)
		chunkRelations = normalizeRelationRows(payload["relations"], chunk.SeqNo)
		chunkEntityCount = len(chunkEntities)
		chunkRelationCount = len(chunkRelations)
		cacheHit, cacheMiss := cacheTokenCounts(p.Extractor)
		p.Logger.Info("extract entities end  ",
			"record_id", recordID,
			"chunk_idx", idx,
			"seq_no", chunk.SeqNo,
			"entities", fmt.Sprintf("entities:%d", chunkEntityCount),
			"ms_used", time.Since(callStart).Milliseconds(),
			"cache_hit", cacheHit,
			"cache_miss", cacheMiss,
		)
	}

	// Global totals are unknown during concurrent execution; pass 0.
	p.logLLMCall(ctx, callID, "extract_entities", 1,
		[]string{strings.TrimSpace(modelName)}, p.PromptRef,
		payload, err, callStart, p.Now(),
		recordID, idx, totalChunks,
		chunkEntityCount, 0, chunkRelationCount, 0)

	if err != nil {
		p.Logger.Warn("entity-relation extraction failed for chunk; skipping",
			"record_id", recordID,
			"chunk_idx", idx,
			"seq_no", chunk.SeqNo,
			"error", err,
		)
		return entityRelationChunkResult{
			LLMCallCount:  llmCallCount,
			FallbackCount: fallbackCount,
			Failed:        true,
		}
	}

	return entityRelationChunkResult{
		Entities:      chunkEntities,
		Relations:     chunkRelations,
		Language:      detectedLanguage,
		ModelName:     strings.TrimSpace(modelName),
		LLMCallCount:  llmCallCount,
		FallbackCount: fallbackCount,
	}
}

// relationWindowResult holds the output of one concurrent Phase 2 window worker.
type relationWindowResult struct {
	Relations []map[string]any
	Language  string
	ModelName string
	Failed    bool
}

// extractRelationsFromWindows runs Phase 2 (D5): one entity-aware relation call
// per positional window, concurrently. Failed windows are skipped (not fatal).
func (p *EntityRelationProcessor) extractRelationsFromWindows(
	ctx context.Context,
	recordID int64,
	windows []relationWindow,
) ([]map[string]any, error) {
	results, runErr := runConcurrent(ctx, p.ExtractEntityRelationMaxTasks, len(windows),
		func(workerCtx context.Context, i int) (relationWindowResult, error) {
			if isCtxStopped(workerCtx) {
				return relationWindowResult{}, ErrPipelineStopped
			}
			return p.processRelationWindow(workerCtx, recordID, i, len(windows), windows[i]), nil
		},
	)
	if runErr != nil {
		if isCtxStopped(ctx) {
			return nil, ErrPipelineStopped
		}
		return nil, runErr
	}

	relations := make([]map[string]any, 0)
	for _, r := range results {
		if r.Failed {
			continue
		}
		relations = append(relations, r.Relations...)
	}
	return relations, nil
}

func (p *EntityRelationProcessor) processRelationWindow(
	ctx context.Context,
	recordID int64,
	idx int,
	totalWindows int,
	window relationWindow,
) relationWindowResult {
	inputText := buildRelationWindowInputText(window)
	callStart := p.Now()
	p.Logger.Info("extract relations start",
		"record_id", recordID,
		"window_idx", idx,
		"entities", len(window.Entities),
		"pages", fmt.Sprintf("%d-%d", window.PageLo, window.PageHi),
		"input len", len(inputText),
	)
	callID := fmt.Sprintf("%s_p2_w%d", eventIDFromContext(ctx), idx)
	payload, modelName, err := p.extractRelationsWithFallback(ctx, inputText)

	var relations []map[string]any
	var detectedLanguage string
	if err == nil && payload != nil {
		detectedLanguage = strings.TrimSpace(asString(payload["language"]))
		relations = normalizeRelationRows(payload["relations"], 0)
		p.Logger.Info("extract relations end  ",
			"record_id", recordID,
			"window_idx", idx,
			"relations", len(relations),
			"ms_used", time.Since(callStart).Milliseconds(),
		)
	}

	p.logLLMCall(ctx, callID, "extract_relations", 2,
		[]string{strings.TrimSpace(modelName)}, p.RelationPromptRef,
		payload, err, callStart, p.Now(),
		recordID, idx, totalWindows,
		0, 0, len(relations), 0)

	if err != nil {
		p.Logger.Warn("relation extraction failed for window; skipping",
			"record_id", recordID,
			"window_idx", idx,
			"error", err,
		)
		return relationWindowResult{Failed: true}
	}
	return relationWindowResult{
		Relations: relations,
		Language:  detectedLanguage,
		ModelName: strings.TrimSpace(modelName),
	}
}

func (p *EntityRelationProcessor) logLLMCall(
	ctx context.Context,
	callID, activity string,
	pass int,
	modelNames []string,
	promptName string,
	payload map[string]any,
	callErr error,
	start, end time.Time,
	recordID int64,
	chunkIdx int,
	totalChunks int,
	chunkEntityCount, totalEntityCount int,
	chunkRelationCount, totalRelationCount int,
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
	percent := 0
	if totalChunks > 0 {
		percent = (chunkIdx + 1) * 100 / totalChunks
	}
	progress := fmt.Sprintf("%d%% (%d/%d)", percent, chunkIdx+1, totalChunks)
	extraInfo := map[string]any{
		"chunk":                chunkIdx + 1,
		"total_chunks":         totalChunks,
		"percent":              progress,
		"chunk_entity_count":   chunkEntityCount,
		"total_entity_count":   totalEntityCount,
		"chunk_relation_count": chunkRelationCount,
		"total_relation_count": totalRelationCount,
	}
	extraJSON, _ := json.Marshal(extraInfo)
	extraStr := string(extraJSON)
	cacheHit, cacheMiss := extractorCacheTokens(p.Extractor)
	rec := DocProcLogRecord{
		CallReason:            "extract entities",
		DocProcName:           p.Name(),
		ModelNames:            modelNames,
		PromptName:            promptName,
		RecordID:              &recordID,
		ProcProgress:          &progress,
		Pass:                  &pass,
		LLMCallID:             &callID,
		ActivityName:          &activity,
		ArtifactJSON:          artifactStr,
		Errors:                errStr,
		ExtraInfoJSON:         &extraStr,
		MSUsed:                int64Ptr(end.Sub(start).Milliseconds()),
		PromptCacheHitTokens:  cacheHit,
		PromptCacheMissTokens: cacheMiss,
	}
	if activity == "extract_relations" {
		if err := p.ProcLogger.LogExtractRelations(ctx, rec, "MID-26061301"); err != nil {
			p.Logger.Warn("failed to write llm_call log", "call_id", callID, "error", err)
		}

	} else {
		if err := p.ProcLogger.LogExtractEntities(ctx, rec, "MID-26061302"); err != nil {
			p.Logger.Warn("failed to write llm_call log", "call_id", callID, "error", err)
		}
	}
}

func (p *EntityRelationProcessor) logEntityRelationSummary(
	ctx context.Context,
	start, end time.Time,
	result entityRelationExtractionResult,
	insertedEntities, insertedRelations int64,
	numChunks int,
	recordID int64,
) {
	extraInfo := map[string]any{
		"total_entities":  insertedEntities,
		"total_relations": insertedRelations,
		"llm_call_count":  result.LLMCallCount,
		"fallback_count":  result.FallbackCount,
		"failed_chunks":   result.FailedChunks,
		"num_chunks":      numChunks,
	}
	extraJSON, _ := json.Marshal(extraInfo)
	extraStr := string(extraJSON)

	seen := map[string]struct{}{}
	modelNames := make([]string, 0, 2)
	for _, n := range []string{p.ModelName, p.FallbackModelName} {
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

	progress := "100%"
	activityName := "extract_entity_relation_finish"
	rec := DocProcLogRecord{
		CallReason:    "extract entities and relations",
		DocProcName:   p.Name(),
		ModelNames:    modelNames,
		PromptName:    p.PromptRef,
		RecordID:      &recordID,
		ProcProgress:  &progress,
		ActivityName:  &activityName,
		ExtraInfoJSON: &extraStr,
		MSUsed:        int64Ptr(end.Sub(start).Milliseconds()),
	}
	if err := p.ProcLogger.LogSummary(ctx, "extract_entity_relation_finish", rec, "MID-26052808"); err != nil {
		p.Logger.Warn("failed to write doc_proc_summary log", "error", err)
	}
}

func (p *EntityRelationProcessor) extractEntityRelationWithFallback(ctx context.Context, inputText string) (map[string]any, string, error) {
	return p.extractStructuredWithFallback(ctx, p.PromptText, p.PromptRef, inputText,
		entityExtractionContract(),
		map[string]any{"language": "unknown", "entities": []any{}})
}

// extractRelationsWithFallback runs the Phase 2 (D5) relation-only prompt over a
// window's entity-list input text, with the same primary/fallback model policy.
func (p *EntityRelationProcessor) extractRelationsWithFallback(ctx context.Context, inputText string) (map[string]any, string, error) {
	promptText, promptRef := p.RelationPromptText, p.RelationPromptRef
	// usedFallback := false
	if strings.TrimSpace(promptText) == "" {
		// No dedicated relation prompt configured; fall back to the combined prompt.
		promptText, promptRef = p.PromptText, p.PromptRef
		// usedFallback = true
	}
	// p.Logger.Info("relation extraction prompt selected",
	// 	"prompt_ref", promptRef, "fell_back_to_entity_prompt", usedFallback)
	return p.extractStructuredWithFallback(ctx, promptText, promptRef, inputText,
		relationExtractionContract(),
		map[string]any{"language": "unknown", "relations": []any{}})
}

// extractStructuredWithFallback runs a structured extraction with the primary
// model and, on failure, the configured fallback model. emptyResult is returned
// when the fallback yields an empty-JSON response so a single bad chunk/window
// does not abort the batch.
func (p *EntityRelationProcessor) extractStructuredWithFallback(
	ctx context.Context,
	promptText, promptRef, inputText string,
	contract llmclients.StructuredOutputContract,
	emptyResult map[string]any,
) (map[string]any, string, error) {
	payload, err := p.extractStructuredPayload(ctx, promptText, inputText, p.ModelName, p.ModelCfg, contract)
	if err == nil {
		return payload, strings.TrimSpace(p.ModelName), nil
	}

	primaryModelName := strings.TrimSpace(p.ModelName)
	fallbackModelName := strings.TrimSpace(p.FallbackModelName)
	if fallbackModelName == "" {
		return nil, primaryModelName, err
	}
	if p.FallbackModelErr != nil {
		return nil, fallbackModelName, fmt.Errorf("(MID_26052720) primary extraction failed and fallback model %q is unavailable: %w", p.FallbackModelRef, err)
	}

	if ApiUtils.IsEmptyJSONResponse(err) {
		p.Logger.Info("primary timeout. Try the fallback")
	} else {
		p.Logger.Warn("primary extraction failed; retrying fallback model. Try the fallback!",
			"primary_model", primaryModelName,
			"fallback_model", fallbackModelName,
			"timeout", p.ModelCfg.TimeoutSec,
			"error", err,
			"prompt_name", promptRef,
		)
	}

	fallbackPayload, fallbackErr := p.extractStructuredPayload(ctx, promptText, inputText, fallbackModelName, p.FallbackModelCfg, contract)
	if fallbackErr != nil {
		if isEmptyEntityRelationError(fallbackErr) {
			p.Logger.Info("fallback extraction returned empty JSON; treating as empty result",
				"fallback_model", fallbackModelName)
			return emptyResult, fallbackModelName, nil
		}
		return nil, fallbackModelName, fmt.Errorf("(MID_26052721) primary extraction failed: %w; fallback extraction failed: %v", err, fallbackErr)
	}
	return fallbackPayload, fallbackModelName, nil
}

func (p *EntityRelationProcessor) extractStructuredPayload(
	ctx context.Context,
	promptText string,
	inputText string,
	modelName string,
	cfg structureModelConfig,
	contract llmclients.StructuredOutputContract,
) (map[string]any, error) {
	permit, err := p.acquireLLMPermit(ctx, modelName)
	if err != nil {
		return nil, err
	}
	defer permit.Release()

	applyStructureModelConfigToExtractor(p.Extractor, cfg)
	callReason := "extract_entities"
	if strings.TrimSpace(promptText) == strings.TrimSpace(p.RelationPromptText) {
		callReason = "extract_relations"
	}
	in := newLLMJSONInput(ctx, firstNonEmptyTrimmed(p.PromptRef, p.RelationPromptRef), promptText, modelName, inputText, callReason, "MID-CWB-ENTITY-RELATION")
	var (
		payload    map[string]any
		extractErr error
	)
	if structuredExtractor, ok := p.Extractor.(LLMStructuredJSONExtractor); ok {
		var result *llmclients.StructuredOutputResult
		result, extractErr = structuredExtractor.ExtractStructuredJSON(ctx, in, contract)
		if result != nil {
			payload = result.Parsed
		}
	} else {
		payload, extractErr = p.Extractor.ExtractJSON(ctx, in)
	}
	if extractErr != nil {
		return nil, extractErr
	}
	if payload == nil {
		return nil, errors.New("(MID_26052730) empty llm payload")
	}
	return payload, nil
}

func (p *EntityRelationProcessor) acquireLLMPermit(ctx context.Context, modelName string) (llmCallPermit, error) {
	if _, ok := p.Extractor.(*llmclients.OpenAIJSONClient); ok {
		return &acquiredLLMPermit{}, nil
	}
	controller := p.LLMController
	if controller == nil {
		controller = defaultDocProcessorLLMController
	}
	permit, err := controller.Acquire(ctx, modelName)
	if err != nil {
		return nil, fmt.Errorf("(MID_26061801) acquire llm permit for model %q: %w", strings.TrimSpace(modelName), err)
	}
	if permit == nil {
		return &acquiredLLMPermit{}, nil
	}
	return permit, nil
}

func isEmptyEntityRelationError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.TrimSpace(err.Error())
	return (strings.Contains(msg, "unexpected end of JSON input") && strings.Contains(msg, "json:{[]}")) ||
		strings.Contains(msg, "(MID_26052730)")
}

// ---- Normalization ----

// normalizeEntityLineSpans parses a `lines` array from the LLM output (which
// may use `-` or `:` as range separators) and returns sorted, merged spans in
// `n-m` format (e.g. "14-16"). Single-line spans are returned as plain numbers.
func normalizeEntityLineSpans(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	type span struct{ start, end int }
	spans := make([]span, 0, len(items))
	for _, item := range items {
		switch v := item.(type) {
		case float64:
			n := int(v)
			if n > 0 {
				spans = append(spans, span{n, n})
			}
		case string:
			s := strings.TrimSpace(v)
			sep := strings.IndexAny(s, "-:")
			if sep > 0 {
				start, err1 := strconv.Atoi(strings.TrimSpace(s[:sep]))
				end, err2 := strconv.Atoi(strings.TrimSpace(s[sep+1:]))
				if err1 == nil && err2 == nil && start > 0 && end >= start {
					spans = append(spans, span{start, end})
				}
			} else {
				n, err := strconv.Atoi(s)
				if err == nil && n > 0 {
					spans = append(spans, span{n, n})
				}
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
	merged := []span{spans[0]}
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
			out = append(out, fmt.Sprintf("%d-%d", s.start, s.end))
		}
	}
	return out
}

func normalizeEntityRows(raw any, chunkSeqNo int) []map[string]any {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		entityName := strings.TrimSpace(asString(m["entity"]))
		if entityName == "" {
			continue
		}
		row := map[string]any{
			"entity":            entityName,
			"entity_en":         strings.TrimSpace(asString(m["entity_en"])),
			"entity_type":       strings.TrimSpace(asString(m["entity_type"])),
			"entity_type_en":    strings.TrimSpace(asString(m["entity_type_en"])),
			"aliases":           toStringSlice(m["aliases"]),
			"aliases_en":        toStringSlice(m["aliases_en"]),
			"desc":              strings.TrimSpace(asString(m["desc"])),
			"desc_en":           strings.TrimSpace(asString(m["desc_en"])),
			"keywords":          toStringSlice(m["keywords"]),
			"keywords_en":       toStringSlice(m["keywords_en"]),
			"line_spans":        normalizeEntityLineSpans(m["lines"]),
			"confidence":        toFloat(m["confidence"]),
			"chunk_seq_no":      chunkSeqNo,
			"entity_categories": toStringSlice(m["entity_categories"]),
		}
		out = append(out, row)
	}
	return out
}

func normalizeRelationRows(raw any, chunkSeqNo int) []map[string]any {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		subject := strings.TrimSpace(asString(m["subject"]))
		predicate := normalizePredicate(asString(m["predicate"]))
		object := strings.TrimSpace(asString(m["object"]))
		if subject == "" || predicate == "" || object == "" {
			continue
		}
		// v2 free-form prompt (ADR 2026061702) cites each endpoint's lines; v1 had a
		// single `lines`. Capture both: endpoint lines feed Phase C line-overlap
		// linking; line_spans falls back to their union when no top-level `lines`.
		subjectLines := normalizeEntityLineSpans(m["subject_lines"])
		predicateLines := normalizeEntityLineSpans(m["predicate_lines"])
		objectLines := normalizeEntityLineSpans(m["object_lines"])
		lineSpans := normalizeEntityLineSpans(m["lines"])
		if len(lineSpans) == 0 {
			lineSpans = sortedUniqueSpans(append(append(append([]string{}, subjectLines...), predicateLines...), objectLines...))
		}
		row := map[string]any{
			"subject":             subject,
			"subject_en":          strings.TrimSpace(asString(m["subject_en"])),
			"subject_desc":        strings.TrimSpace(asString(m["subject_desc"])),
			"predicate":           predicate,
			"predicate_en":        normalizePredicate(asString(m["predicate_en"])),
			"object":              object,
			"object_en":           strings.TrimSpace(asString(m["object_en"])),
			"object_desc":         strings.TrimSpace(asString(m["object_desc"])),
			"desc":                strings.TrimSpace(asString(m["desc"])),
			"desc_en":             strings.TrimSpace(asString(m["desc_en"])),
			"keywords":            toStringSlice(m["keywords"]),
			"keywords_en":         toStringSlice(m["keywords_en"]),
			"line_spans":          lineSpans,
			"subject_lines":       subjectLines,
			"predicate_lines":     predicateLines,
			"object_lines":        objectLines,
			"confidence":          toFloat(m["confidence"]),
			"chunk_seq_no":        chunkSeqNo,
			"relation_categories": toStringSlice(m["relation_categories"]),
		}
		out = append(out, row)
	}
	return out
}

// ---- Entity context ----

// parseLineSpanRange parses a single span string ("14" or "14-16") into (start, end).
// Returns (0, 0) on parse failure.
func parseLineSpanRange(s string) (start, end int) {
	s = strings.TrimSpace(s)
	sep := strings.IndexAny(s, "-:")
	if sep > 0 {
		a, err1 := strconv.Atoi(strings.TrimSpace(s[:sep]))
		b, err2 := strconv.Atoi(strings.TrimSpace(s[sep+1:]))
		if err1 == nil && err2 == nil && a > 0 && b >= a {
			return a, b
		}
		return 0, 0
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0, 0
	}
	return n, n
}

// buildEntityContextForEntities sets the "entity_context" key on each entity map.
// For every entity it groups its line_spans by chunk, then for each chunk builds:
//
//	chunk_summary + (leading_lines + line_range) per span in that chunk
//
// Multiple per-chunk contexts are separated by "\n\n".
func buildEntityContextForEntities(
	entities []map[string]any,
	chunks []Chunk,
	allLines []Line,
	chunkSummaries map[int]string,
	contextSize int,
) {
	if len(entities) == 0 {
		return
	}

	// line number → document line
	lineByNo := make(map[int]Line, len(allLines))
	sortedNos := make([]int, 0, len(allLines))
	for _, l := range allLines {
		lineByNo[l.LineNo] = l
		sortedNos = append(sortedNos, l.LineNo)
	}
	sort.Ints(sortedNos)

	// document line number → chunk SeqNo
	lineToChunk := make(map[int]int, len(allLines))
	for _, c := range chunks {
		for _, ml := range c.Lines {
			if ml.Line.LineNo > 0 {
				lineToChunk[ml.Line.LineNo] = c.SeqNo
			}
		}
	}

	type spanRange struct{ start, end int }

	for _, entity := range entities {
		spans := toStringSlice(entity["line_spans"])
		if len(spans) == 0 {
			continue
		}

		// Group spans by chunk SeqNo, preserving first-seen order.
		chunkSpans := make(map[int][]spanRange)
		var chunkOrder []int
		chunkSeen := make(map[int]bool)

		for _, span := range spans {
			st, en := parseLineSpanRange(span)
			if st <= 0 {
				continue
			}
			seqNo := 0
			found := false
			for lineNo := st; lineNo <= en; lineNo++ {
				if sn, ok := lineToChunk[lineNo]; ok {
					seqNo, found = sn, true
					break
				}
			}
			if !found {
				if csn, ok := entity["chunk_seq_no"].(int); ok {
					seqNo = csn
				}
			}
			chunkSpans[seqNo] = append(chunkSpans[seqNo], spanRange{st, en})
			if !chunkSeen[seqNo] {
				chunkSeen[seqNo] = true
				chunkOrder = append(chunkOrder, seqNo)
			}
		}
		sort.Ints(chunkOrder)

		var chunkContexts []string
		for _, seqNo := range chunkOrder {
			summary := strings.TrimSpace(chunkSummaries[seqNo])
			var spanTexts []string

			for _, sr := range chunkSpans[seqNo] {
				// Collect contextSize leading lines (lines before sr.start).
				leadingStart := sort.SearchInts(sortedNos, sr.start)
				leadStart := leadingStart - contextSize
				if leadStart < 0 {
					leadStart = 0
				}
				var parts []string
				for _, lineNo := range sortedNos[leadStart:leadingStart] {
					if c := strings.TrimSpace(lineByNo[lineNo].Content); c != "" {
						parts = append(parts, c)
					}
				}
				// Span lines.
				for lineNo := sr.start; lineNo <= sr.end; lineNo++ {
					if l, ok := lineByNo[lineNo]; ok {
						if c := strings.TrimSpace(l.Content); c != "" {
							parts = append(parts, c)
						}
					}
				}
				if len(parts) > 0 {
					spanTexts = append(spanTexts, strings.Join(parts, "\n"))
				}
			}

			if len(spanTexts) == 0 {
				continue
			}
			var contextParts []string
			if summary != "" {
				contextParts = append(contextParts, "Summary: "+summary)
			}
			contextParts = append(contextParts, strings.Join(spanTexts, "\n\n"))
			chunkContexts = append(chunkContexts, strings.Join(contextParts, "\n"))
		}

		entity["entity_context"] = strings.Join(chunkContexts, "\n\n")
	}
}

// normalizePredicate lowercases and replaces internal whitespace with single
// underscores so callers see e.g. "Depends On" -> "depends_on".
func normalizePredicate(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	s = strings.ToLower(s)
	fields := strings.Fields(s)
	return strings.Join(fields, "_")
}

// ---- Save to file ----

func (p *EntityRelationProcessor) saveEntitiesToFile(
	recordID int64,
	rec DocMetadataInputRecord,
	entities []map[string]any,
) error {
	return writeEntityRelationArtifactFile(p.ArtifactDir, recordID, rec, entities, ".entities", "MID_26052740")
}

func (p *EntityRelationProcessor) saveRelationsToFile(
	recordID int64,
	rec DocMetadataInputRecord,
	relations []map[string]any,
) error {
	return writeEntityRelationArtifactFile(p.ArtifactDir, recordID, rec, relations, ".relations", "MID_26052741")
}

func writeEntityRelationArtifactFile(
	artifactDir string,
	recordID int64,
	rec DocMetadataInputRecord,
	rows []map[string]any,
	ext string,
	errCode string,
) error {
	if len(rows) == 0 {
		return nil
	}
	if strings.TrimSpace(artifactDir) == "" {
		return fmt.Errorf("(%s) missing ARTIFACT_DIR", errCode)
	}
	stagingBase := filepath.Base(strings.TrimSpace(rec.StagingFilename))
	filenameRoot := strings.TrimSuffix(stagingBase, filepath.Ext(stagingBase))
	if filenameRoot == "" {
		return fmt.Errorf("(%s) cannot determine filename root from staging_filename %q", errCode, rec.StagingFilename)
	}
	parserName := strings.TrimSpace(rec.ParserName)
	if parserName == "" {
		return fmt.Errorf("(%s) parser_name is empty for record_id=%d", errCode, recordID)
	}
	groupID := recordID / 1000
	dir := filepath.Join(artifactDir, strconv.FormatInt(groupID, 10), strconv.FormatInt(recordID, 10))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("(%s) create directory %s: %w", errCode, dir, err)
	}
	path := filepath.Join(dir, filenameRoot+"_"+parserName+ext)
	bs, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return fmt.Errorf("(%s) marshal rows: %w", errCode, err)
	}
	if err := os.WriteFile(path, bs, 0o644); err != nil {
		return fmt.Errorf("(%s) write file %s: %w", errCode, path, err)
	}
	return nil
}

// ---- Status ----

type entityRelationStatusParams struct {
	RecordID      int64
	FileType      string
	InputFilename string
	Operation     string // doc_proc status operation; defaults to extract_entity_relation
	Start         time.Time
	DurationMs    int64
	ProcStatus    string
	Progress      string
	ProcErr       error
}

func detectEntityRelationFileType(rec DocMetadataInputRecord) string {
	for _, candidate := range []string{rec.FileName, rec.StagingFilename, rec.ResultFilename} {
		ext := strings.ToLower(strings.TrimSpace(filepath.Ext(strings.TrimSpace(candidate))))
		if ext != "" {
			return strings.TrimPrefix(ext, ".")
		}
	}
	return ""
}

func (p *EntityRelationProcessor) persistEntityRelationInProgressStatus(ctx context.Context, rec DocMetadataInputRecord, start time.Time, progress string) {
	if err := updateInputStatusAtomic(ctx, p.InputStore, rec.ID, func(current string) (DocMetadataUpdate, error) {
		statusRaw, err := appendEntityRelationStatus(current, entityRelationStatusParams{
			RecordID:      rec.ID,
			FileType:      detectEntityRelationFileType(rec),
			InputFilename: strings.TrimSpace(rec.ResultFilename),
			Operation:     p.statusOp(),
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
		p.Logger.Warn("failed persisting entity-relation in-progress status", "record_id", rec.ID, "error", err)
	}
}

func (p *EntityRelationProcessor) persistEntityRelationStatus(
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
	if updateErr := updateInputStatusAtomic(ctx, p.InputStore, rec.ID, func(current string) (DocMetadataUpdate, error) {
		statusRaw, err := appendEntityRelationStatus(current, entityRelationStatusParams{
			RecordID:      rec.ID,
			FileType:      detectEntityRelationFileType(rec),
			InputFilename: strings.TrimSpace(rec.ResultFilename),
			Operation:     p.statusOp(),
			Start:         start,
			DurationMs:    time.Since(start).Milliseconds(),
			ProcErr:       procErr,
		})
		if err != nil {
			return DocMetadataUpdate{}, err
		}
		return DocMetadataUpdate{StatusRaw: statusRaw, ErrorMsg: errMsg}, nil
	}); updateErr != nil {
		p.Logger.Error("failed persisting entity-relation status", "record_id", rec.ID, "error", updateErr)
	}
}

func (p *EntityRelationProcessor) stopAndPersistEntityRelation(ctx context.Context, rec DocMetadataInputRecord, start time.Time) {
	if updateErr := updateInputStatusAtomic(ctx, p.InputStore, rec.ID, func(current string) (DocMetadataUpdate, error) {
		statusRaw, err := appendEntityRelationStatus(current, entityRelationStatusParams{
			RecordID:      rec.ID,
			FileType:      detectEntityRelationFileType(rec),
			InputFilename: strings.TrimSpace(rec.ResultFilename),
			Operation:     p.statusOp(),
			Start:         start,
			DurationMs:    time.Since(start).Milliseconds(),
			ProcStatus:    "stopped",
		})
		if err != nil {
			return DocMetadataUpdate{}, err
		}
		return DocMetadataUpdate{StatusRaw: statusRaw}, nil
	}); updateErr != nil {
		p.Logger.Error("(MID_26052842) failed persisting entity-relation stopped status", "record_id", rec.ID, "error", updateErr)
	}
	p.Logger.Info("(MID_26052843) extract_entity_relation stopped by user request", "record_id", rec.ID)
}

func appendEntityRelationStatus(raw string, p entityRelationStatusParams) (string, error) {
	entries := decodeDocMetaStatus(raw)
	operation := strings.TrimSpace(p.Operation)
	if operation == "" {
		operation = "extract_entity_relation"
	}
	entry := map[string]any{
		"record_id":      strconv.FormatInt(p.RecordID, 10),
		"file_type":      strings.ToLower(strings.TrimSpace(p.FileType)),
		"operation":      operation,
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
		if op != operation {
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

func (s EntityRelationSQLStore) ensureTables(ctx context.Context) error {
	if s.DB == nil {
		return fmt.Errorf("(MID_26052750) db is nil")
	}
	const ddl = `
CREATE SCHEMA IF NOT EXISTS kb;

CREATE TABLE IF NOT EXISTS kb.entities (
    id BIGSERIAL PRIMARY KEY,
    event_id TEXT,
    input_record_id BIGINT NOT NULL,
    entity_id TEXT,
    language TEXT,
    entity TEXT,
    entity_en TEXT,
    entity_type TEXT,
    entity_type_en TEXT,
    aliases JSONB,
    aliases_en JSONB,
    desc_text TEXT,
    desc_text_en TEXT,
    keywords JSONB,
    keywords_en JSONB,
    line_spans JSONB,
    confidence DOUBLE PRECISION,
    model_name TEXT,
    prompt_name TEXT,
    entity_context TEXT,
    doc_name TEXT,
    categories JSONB,
    entity_status TEXT NOT NULL DEFAULT 'extracted',
    search_document TEXT,
    search_vector TSVECTOR,
    ext_info JSONB,
    create_time TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE kb.entities ADD COLUMN IF NOT EXISTS entity_context TEXT;
ALTER TABLE kb.entities ADD COLUMN IF NOT EXISTS doc_name TEXT;
ALTER TABLE kb.entities ADD COLUMN IF NOT EXISTS categories JSONB;
ALTER TABLE kb.entities ADD COLUMN IF NOT EXISTS entity_status TEXT NOT NULL DEFAULT 'extracted';

CREATE INDEX IF NOT EXISTS idx_kb_entities_input_record_id ON kb.entities (input_record_id);
CREATE INDEX IF NOT EXISTS idx_kb_entities_entity_id ON kb.entities (entity_id);
CREATE INDEX IF NOT EXISTS idx_kb_entities_entity_status ON kb.entities (entity_status);

CREATE TABLE IF NOT EXISTS kb.entity_names (
    name_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name_key TEXT NOT NULL,
    name_raw TEXT NOT NULL DEFAULT '',
    display_names JSONB NOT NULL DEFAULT '[]'::jsonb,
    aliases JSONB NOT NULL DEFAULT '[]'::jsonb,
    name_desc TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending_review',
    seen_count BIGINT NOT NULL DEFAULT 0,
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    create_time TIMESTAMPTZ NOT NULL DEFAULT now(),
    modify_time TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT entity_names_key_uniq UNIQUE (name_key)
);
CREATE INDEX IF NOT EXISTS idx_kb_entity_names_seen_count ON kb.entity_names USING btree (seen_count DESC);

CREATE TABLE IF NOT EXISTS kb.relations (
    id BIGSERIAL PRIMARY KEY,
    event_id TEXT,
    input_record_id BIGINT NOT NULL,
    relation_id TEXT,
    language TEXT,
    subject TEXT,
    subject_en TEXT,
    subject_desc TEXT,
    predicate TEXT,
    predicate_en TEXT,
    object TEXT,
    object_en TEXT,
    object_desc TEXT,
    desc_text TEXT,
    desc_text_en TEXT,
    keywords JSONB,
    keywords_en JSONB,
    line_spans JSONB,
    confidence DOUBLE PRECISION,
    model_name TEXT,
    prompt_name TEXT,
    subject_entity_id TEXT,
    object_entity_id TEXT,
    categories JSONB,
    search_document TEXT,
    search_vector TSVECTOR,
    ext_info JSONB,
    create_time TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE kb.relations ADD COLUMN IF NOT EXISTS subject_entity_id TEXT;
ALTER TABLE kb.relations ADD COLUMN IF NOT EXISTS object_entity_id TEXT;
ALTER TABLE kb.relations DROP COLUMN IF EXISTS source_line_spans;
ALTER TABLE kb.relations ADD COLUMN IF NOT EXISTS categories JSONB;
ALTER TABLE kb.relations ADD COLUMN IF NOT EXISTS subject_lines JSONB;
ALTER TABLE kb.relations ADD COLUMN IF NOT EXISTS predicate_lines JSONB;
ALTER TABLE kb.relations ADD COLUMN IF NOT EXISTS object_lines JSONB;
ALTER TABLE kb.relations ADD COLUMN IF NOT EXISTS subject_desc TEXT;
ALTER TABLE kb.relations ADD COLUMN IF NOT EXISTS object_desc TEXT;

CREATE INDEX IF NOT EXISTS idx_kb_relations_input_record_id ON kb.relations (input_record_id);
CREATE INDEX IF NOT EXISTS idx_kb_relations_relation_id ON kb.relations (relation_id);
CREATE INDEX IF NOT EXISTS idx_kb_relations_subject_entity_id ON kb.relations (subject_entity_id);
CREATE INDEX IF NOT EXISTS idx_kb_relations_object_entity_id ON kb.relations (object_entity_id);
`
	_, err := s.DB.ExecContext(ctx, ddl)
	return err
}

func (s EntityRelationSQLStore) EntitiesExist(ctx context.Context, inputRecordID int64) (bool, error) {
	if err := s.ensureTables(ctx); err != nil {
		return false, err
	}
	const q = `SELECT 1 FROM kb.entities WHERE input_record_id = $1 LIMIT 1`
	var one int
	err := s.DB.QueryRowContext(ctx, q, inputRecordID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s EntityRelationSQLStore) DeleteEntitiesByInputRecordID(ctx context.Context, inputRecordID int64) (int64, error) {
	if err := s.ensureTables(ctx); err != nil {
		return 0, err
	}
	res, err := s.DB.ExecContext(ctx, `DELETE FROM kb.entities WHERE input_record_id = $1`, inputRecordID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s EntityRelationSQLStore) RelationsExist(ctx context.Context, inputRecordID int64) (bool, error) {
	if err := s.ensureTables(ctx); err != nil {
		return false, err
	}
	const q = `SELECT 1 FROM kb.relations WHERE input_record_id = $1 LIMIT 1`
	var one int
	err := s.DB.QueryRowContext(ctx, q, inputRecordID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s EntityRelationSQLStore) DeleteRelationsByInputRecordID(ctx context.Context, inputRecordID int64) (int64, error) {
	if err := s.ensureTables(ctx); err != nil {
		return 0, err
	}
	res, err := s.DB.ExecContext(ctx, `DELETE FROM kb.relations WHERE input_record_id = $1`, inputRecordID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s EntityRelationSQLStore) SaveEntities(ctx context.Context, req SaveEntitiesRequest) (int64, error) {
	if err := s.ensureTables(ctx); err != nil {
		return 0, err
	}
	if len(req.Entities) == 0 {
		return 0, nil
	}
	const stmt = `
INSERT INTO kb.entities (
    event_id,
    input_record_id,
    entity_id,
    language,
    entity,
    entity_en,
    entity_type,
    entity_type_en,
    aliases,
    aliases_en,
    desc_text,
    desc_text_en,
    keywords,
    keywords_en,
    line_spans,
    confidence,
    model_name,
    prompt_name,
    entity_context,
    doc_name,
    categories,
    entity_status,
    ext_info
) VALUES (
    $1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10::jsonb,$11,$12,$13::jsonb,$14::jsonb,$15::jsonb,$16,$17,$18,$19,$20,$21::jsonb,$22,$23::jsonb
)`

	isEnglish := isLanguageEnglish(req.Language)
	var eventIDVal any
	if id := strings.TrimSpace(req.EventID); id != "" {
		eventIDVal = id
	}

	var inserted int64
	for _, e := range req.Entities {
		aliasesJSON, _ := json.Marshal(e["aliases"])
		keywordsJSON, _ := json.Marshal(e["keywords"])
		spansJSON, _ := json.Marshal(e["line_spans"])
		extInfo, _ := json.Marshal(map[string]any{
			"language":       req.Language,
			"schema_version": "1",
			"chunk_seq_no":   e["chunk_seq_no"],
		})

		var (
			entityEnVal     any
			entityTypeEnVal any
			aliasesEnVal    any
			descEnVal       any
			keywordsEnVal   any
		)
		if !isEnglish {
			entityEnVal = strings.TrimSpace(asString(e["entity_en"]))
			entityTypeEnVal = strings.TrimSpace(asString(e["entity_type_en"]))
			ae, _ := json.Marshal(e["aliases_en"])
			aliasesEnVal = string(ae)
			descEnVal = strings.TrimSpace(asString(e["desc_en"]))
			ke, _ := json.Marshal(e["keywords_en"])
			keywordsEnVal = string(ke)
		}

		var entityContextArg any
		docNameVal := strings.TrimSpace(req.DocName)
		var docNameArg any
		if docNameVal != "" {
			docNameArg = docNameVal
		}
		categoriesJSON, _ := json.Marshal(e["entity_categories"])
		var categoriesArg any
		if cats := toStringSlice(e["entity_categories"]); len(cats) > 0 {
			categoriesArg = string(categoriesJSON)
		}

		entityStatus := strings.TrimSpace(asString(e["entity_status"]))
		if entityStatus == "" {
			entityStatus = entityStatusExtracted
		}

		if _, err := s.DB.ExecContext(ctx, stmt,
			eventIDVal,
			req.InputRecordID,
			strings.TrimSpace(asString(e["entity_id"])),
			strings.TrimSpace(req.Language),
			strings.TrimSpace(asString(e["entity"])),
			entityEnVal,
			strings.TrimSpace(asString(e["entity_type"])),
			entityTypeEnVal,
			string(aliasesJSON),
			aliasesEnVal,
			strings.TrimSpace(asString(e["desc"])),
			descEnVal,
			string(keywordsJSON),
			keywordsEnVal,
			string(spansJSON),
			toFloat(e["confidence"]),
			strings.TrimSpace(req.ModelName),
			strings.TrimSpace(req.PromptName),
			entityContextArg,
			docNameArg,
			categoriesArg,
			entityStatus,
			string(extInfo),
		); err != nil {
			return inserted, err
		}
		inserted++
	}
	return inserted, nil
}

func (s EntityRelationSQLStore) SaveRelations(ctx context.Context, req SaveRelationsRequest) (int64, error) {
	if err := s.ensureTables(ctx); err != nil {
		return 0, err
	}
	if len(req.Relations) == 0 {
		return 0, nil
	}
	const stmt = `
INSERT INTO kb.relations (
    event_id,
    input_record_id,
    relation_id,
    language,
    subject,
    subject_en,
    subject_desc,
    predicate,
    predicate_en,
    object,
    object_en,
    object_desc,
    desc_text,
    desc_text_en,
    keywords,
    keywords_en,
    line_spans,
    confidence,
    model_name,
    prompt_name,
    subject_entity_id,
    object_entity_id,
    categories,
    subject_lines,
    predicate_lines,
    object_lines,
    ext_info
) VALUES (
    $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15::jsonb,$16::jsonb,$17::jsonb,$18,$19,$20,$21,$22,$23::jsonb,$25::jsonb,$26::jsonb,$27::jsonb,$24::jsonb
)`

	isEnglish := isLanguageEnglish(req.Language)
	var eventIDVal any
	if id := strings.TrimSpace(req.EventID); id != "" {
		eventIDVal = id
	}

	var inserted int64
	for _, r := range req.Relations {
		keywordsJSON, _ := json.Marshal(r["keywords"])
		spansJSON, _ := json.Marshal(r["line_spans"])
		subjectLinesJSON, _ := json.Marshal(toStringSlice(r["subject_lines"]))
		predicateLinesJSON, _ := json.Marshal(toStringSlice(r["predicate_lines"]))
		objectLinesJSON, _ := json.Marshal(toStringSlice(r["object_lines"]))
		extInfo, _ := json.Marshal(map[string]any{
			"language":       req.Language,
			"schema_version": "1",
			"chunk_seq_no":   r["chunk_seq_no"],
		})

		var (
			subjectEnVal   any
			predicateEnVal any
			objectEnVal    any
			descEnVal      any
			keywordsEnVal  any
		)
		if !isEnglish {
			subjectEnVal = strings.TrimSpace(asString(r["subject_en"]))
			predicateEnVal = strings.TrimSpace(asString(r["predicate_en"]))
			objectEnVal = strings.TrimSpace(asString(r["object_en"]))
			descEnVal = strings.TrimSpace(asString(r["desc_en"]))
			ke, _ := json.Marshal(r["keywords_en"])
			keywordsEnVal = string(ke)
		}

		var subjectEntityIDArg any
		if v := strings.TrimSpace(asString(r["subject_entity_id"])); v != "" {
			subjectEntityIDArg = v
		}
		var objectEntityIDArg any
		if v := strings.TrimSpace(asString(r["object_entity_id"])); v != "" {
			objectEntityIDArg = v
		}

		var categoriesArg any
		if cats := toStringSlice(r["relation_categories"]); len(cats) > 0 {
			catsJSON, _ := json.Marshal(cats)
			categoriesArg = string(catsJSON)
		}

		var subjectDescVal any
		if v := strings.TrimSpace(asString(r["subject_desc"])); v != "" {
			subjectDescVal = v
		}
		var objectDescVal any
		if v := strings.TrimSpace(asString(r["object_desc"])); v != "" {
			objectDescVal = v
		}

		if _, err := s.DB.ExecContext(ctx, stmt,
			eventIDVal,        // $1
			req.InputRecordID, // $2
			strings.TrimSpace(asString(r["relation_id"])), // $3
			strings.TrimSpace(req.Language),               // $4
			strings.TrimSpace(asString(r["subject"])),     // $5
			subjectEnVal,   // $6
			subjectDescVal, // $7
			strings.TrimSpace(asString(r["predicate"])), // $8
			predicateEnVal,                           // $9
			strings.TrimSpace(asString(r["object"])), // $10
			objectEnVal,                              // $11
			objectDescVal,                            // $12
			strings.TrimSpace(asString(r["desc"])),   // $13
			descEnVal,                                // $14
			string(keywordsJSON),                     // $15
			keywordsEnVal,                            // $16
			string(spansJSON),                        // $17
			toFloat(r["confidence"]),                 // $18
			strings.TrimSpace(req.ModelName),         // $19
			strings.TrimSpace(req.PromptName),        // $20
			subjectEntityIDArg,                       // $21
			objectEntityIDArg,                        // $22
			categoriesArg,                            // $23
			string(extInfo),                          // $24
			string(subjectLinesJSON),                 // $25
			string(predicateLinesJSON),               // $26
			string(objectLinesJSON),                  // $27
		); err != nil {
			return inserted, err
		}
		inserted++
	}
	return inserted, nil
}

/*
func debugLogEntitySearchDocument(ctx context.Context, store EntityRelationStore, recordID int64, label string, logger ApiTypes.JimoLogger) {
	sqlStore, ok := store.(EntityRelationSQLStore)
	if !ok {
		return
	}
	entityID := fmt.Sprintf("%d_ent_55", recordID)
	var doc string
	err := sqlStore.DB.QueryRowContext(ctx,
		`SELECT COALESCE(search_document, '') FROM kb.entities WHERE input_record_id = $1 AND entity_id = $2`,
		recordID, entityID,
	).Scan(&doc)
	if err != nil {
		logger.Info("[DEBUG] search_document query failed", "label", label, "entity_id", entityID, "error", err)
		return
	}
	logger.Info("[DEBUG] search_document", "label", label, "entity_id", entityID, "len", len(doc), "content", doc)
}
*/

func isLanguageEnglish(language string) bool {
	s := strings.TrimSpace(language)
	return strings.EqualFold(s, "en") || strings.EqualFold(s, "english")
}

func (s EntityRelationSQLStore) GetChunkSummaries(ctx context.Context, inputRecordID int64) (map[int]string, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT summary_seq_no, COALESCE(summary_text, '')
		FROM kb.summaries
		WHERE input_record_id = $1 AND summary_level = 0
		ORDER BY summary_seq_no`, inputRecordID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := make(map[int]string)
	for rows.Next() {
		var seqNo int
		var text string
		if err := rows.Scan(&seqNo, &text); err != nil {
			return nil, err
		}
		result[seqNo] = text
	}
	return result, rows.Err()
}

// ---- Phase C artifact file refresh ----

// loadPersistedEntityArtifactRows loads all entity rows for a record from the DB,
// including connected_artifacts populated by Phase C, and returns them as artifact maps
// ready to be written to the .entities file.
func loadPersistedEntityArtifactRows(ctx context.Context, db *sql.DB, recordID int64) ([]map[string]any, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT entity_id,
		       COALESCE(entity, ''),
		       COALESCE(entity_en, ''),
		       COALESCE(entity_type, ''),
		       COALESCE(entity_type_en, ''),
		       COALESCE(aliases, '[]'::jsonb),
		       COALESCE(aliases_en, '[]'::jsonb),
		       COALESCE(desc_text, ''),
		       COALESCE(desc_text_en, ''),
		       COALESCE(keywords, '[]'::jsonb),
		       COALESCE(keywords_en, '[]'::jsonb),
		       COALESCE(line_spans, '[]'::jsonb),
		       COALESCE(confidence, 0),
		       COALESCE(model_name, ''),
		       COALESCE(prompt_name, ''),
		       COALESCE(entity_context, ''),
		       COALESCE(doc_name, ''),
		       COALESCE(categories, '[]'::jsonb),
		       COALESCE(entity_status, 'extracted'),
		       COALESCE(search_document, ''),
		       kb.connected_artifacts(input_record_id, 'entity', id),
		       COALESCE(ext_info, '{}'::jsonb)
		FROM kb.entities
		WHERE input_record_id = $1
		ORDER BY id`, recordID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []map[string]any
	for rows.Next() {
		var (
			entityID, entity, entityEn, entityType, entityTypeEn string
			aliasesRaw, aliasesEnRaw                             []byte
			descText, descTextEn                                 string
			keywordsRaw, keywordsEnRaw                           []byte
			lineSpansRaw                                         []byte
			confidence                                           float64
			modelName, promptName                                string
			entityContext, docName                               string
			categoriesRaw                                        []byte
			entityStatus                                         string
			searchDocument                                       string
			connectedArtifactsRaw, extInfoRaw                    []byte
		)
		if err := rows.Scan(
			&entityID, &entity, &entityEn, &entityType, &entityTypeEn,
			&aliasesRaw, &aliasesEnRaw,
			&descText, &descTextEn,
			&keywordsRaw, &keywordsEnRaw,
			&lineSpansRaw,
			&confidence, &modelName, &promptName,
			&entityContext, &docName, &categoriesRaw, &entityStatus, &searchDocument,
			&connectedArtifactsRaw, &extInfoRaw,
		); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"entity_id":           strings.TrimSpace(entityID),
			"entity":              strings.TrimSpace(entity),
			"entity_en":           strings.TrimSpace(entityEn),
			"entity_type":         strings.TrimSpace(entityType),
			"entity_type_en":      strings.TrimSpace(entityTypeEn),
			"aliases":             jsonColumnToValue(aliasesRaw, []any{}),
			"aliases_en":          jsonColumnToValue(aliasesEnRaw, []any{}),
			"desc_text":           strings.TrimSpace(descText),
			"desc_text_en":        strings.TrimSpace(descTextEn),
			"keywords":            jsonColumnToValue(keywordsRaw, []any{}),
			"keywords_en":         jsonColumnToValue(keywordsEnRaw, []any{}),
			"line_spans":          jsonColumnToValue(lineSpansRaw, []any{}),
			"confidence":          confidence,
			"model_name":          strings.TrimSpace(modelName),
			"prompt_name":         strings.TrimSpace(promptName),
			"entity_context":      strings.TrimSpace(entityContext),
			"doc_name":            strings.TrimSpace(docName),
			"categories":          jsonColumnToValue(categoriesRaw, []any{}),
			"entity_status":       strings.TrimSpace(entityStatus),
			"search_document":     strings.TrimSpace(searchDocument),
			"connected_artifacts": jsonColumnToValue(connectedArtifactsRaw, map[string]any{}),
			"ext_info":            jsonColumnToValue(extInfoRaw, map[string]any{}),
		})
	}
	return out, rows.Err()
}

func refreshEntityArtifactFile(ctx context.Context, db *sql.DB, artifactDir string, recordID int64, rec DocMetadataInputRecord) error {
	rows, err := loadPersistedEntityArtifactRows(ctx, db, recordID)
	if err != nil {
		return err
	}
	return writeEntityRelationArtifactFile(artifactDir, recordID, rec, rows, ".entities", "MID_26060701")
}

// loadPersistedRelationArtifactRows loads all relation rows for a record from the DB,
// including connected_artifacts populated by Phase C.
func loadPersistedRelationArtifactRows(ctx context.Context, db *sql.DB, recordID int64) ([]map[string]any, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT relation_id,
		       COALESCE(subject, ''),
		       COALESCE(subject_en, ''),
		       COALESCE(predicate, ''),
		       COALESCE(predicate_en, ''),
		       COALESCE(object, ''),
		       COALESCE(object_en, ''),
		       COALESCE(desc_text, ''),
		       COALESCE(desc_text_en, ''),
		       COALESCE(keywords, '[]'::jsonb),
		       COALESCE(keywords_en, '[]'::jsonb),
		       COALESCE(line_spans, '[]'::jsonb),
		       COALESCE(confidence, 0),
		       COALESCE(model_name, ''),
		       COALESCE(prompt_name, ''),
		       COALESCE(subject_entity_id, ''),
		       COALESCE(object_entity_id, ''),
		       COALESCE(categories, '[]'::jsonb),
		       COALESCE(search_document, ''),
		       kb.connected_artifacts(input_record_id, 'relation', id),
		       COALESCE(ext_info, '{}'::jsonb)
		FROM kb.relations
		WHERE input_record_id = $1
		ORDER BY id`, recordID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []map[string]any
	for rows.Next() {
		var (
			relationID, subject, subjectEn, predicate, predicateEn string
			object, objectEn                                       string
			descText, descTextEn                                   string
			keywordsRaw, keywordsEnRaw                             []byte
			lineSpansRaw                                           []byte
			confidence                                             float64
			modelName, promptName                                  string
			subjectEntityID, objectEntityID                        string
			categoriesRaw                                          []byte
			searchDocument                                         string
			connectedArtifactsRaw, extInfoRaw                      []byte
		)
		if err := rows.Scan(
			&relationID, &subject, &subjectEn, &predicate, &predicateEn,
			&object, &objectEn,
			&descText, &descTextEn,
			&keywordsRaw, &keywordsEnRaw,
			&lineSpansRaw,
			&confidence, &modelName, &promptName,
			&subjectEntityID, &objectEntityID,
			&categoriesRaw, &searchDocument,
			&connectedArtifactsRaw, &extInfoRaw,
		); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"relation_id":         strings.TrimSpace(relationID),
			"subject":             strings.TrimSpace(subject),
			"subject_en":          strings.TrimSpace(subjectEn),
			"predicate":           strings.TrimSpace(predicate),
			"predicate_en":        strings.TrimSpace(predicateEn),
			"object":              strings.TrimSpace(object),
			"object_en":           strings.TrimSpace(objectEn),
			"desc_text":           strings.TrimSpace(descText),
			"desc_text_en":        strings.TrimSpace(descTextEn),
			"keywords":            jsonColumnToValue(keywordsRaw, []any{}),
			"keywords_en":         jsonColumnToValue(keywordsEnRaw, []any{}),
			"line_spans":          jsonColumnToValue(lineSpansRaw, []any{}),
			"confidence":          confidence,
			"model_name":          strings.TrimSpace(modelName),
			"prompt_name":         strings.TrimSpace(promptName),
			"subject_entity_id":   strings.TrimSpace(subjectEntityID),
			"object_entity_id":    strings.TrimSpace(objectEntityID),
			"categories":          jsonColumnToValue(categoriesRaw, []any{}),
			"search_document":     strings.TrimSpace(searchDocument),
			"connected_artifacts": jsonColumnToValue(connectedArtifactsRaw, map[string]any{}),
			"ext_info":            jsonColumnToValue(extInfoRaw, map[string]any{}),
		})
	}
	return out, rows.Err()
}

func refreshRelationArtifactFile(ctx context.Context, db *sql.DB, artifactDir string, recordID int64, rec DocMetadataInputRecord) error {
	rows, err := loadPersistedRelationArtifactRows(ctx, db, recordID)
	if err != nil {
		return err
	}
	return writeEntityRelationArtifactFile(artifactDir, recordID, rec, rows, ".relations", "MID_26060702")
}

// ---- per-type batch helpers (used by EntityProcessor / RelationProcessor) ----

// initEntityBatch initialises the entity-relation processor for per-chunk
// batching of the Phase 1 (entity extraction) pass.
func (p *EntityRelationProcessor) initEntityBatch(ctx context.Context, recordID int64, chunks []Chunk, docCtx string) error {
	if p.PromptErr != nil {
		return fmt.Errorf("(MID_26062720) %s prompt error: %w", p.Name(), p.PromptErr)
	}
	if p.ModelErr != nil {
		p.Logger.Warn("%s skipped: model config error", p.Name(),
			"record_id", recordID, "model_ref", p.ModelRef, "error", p.ModelErr)
		return nil
	}

	p.batchStart = p.Now()
	p.batchRecordID = recordID
	p.batchChunks = chunks
	p.batchDocCtx = docCtx
	p.batchResults = make([]entityRelationChunkResult, 0, len(chunks))
	return nil
}

// processEntityChunk processes one chunk for Phase 1 entity extraction: calls the
// LLM and accumulates entity/relation results. Called sequentially by the
// per-chunk batching coordinator via EntityProcessor.ProcessChunk.
func (p *EntityRelationProcessor) processEntityChunk(ctx context.Context, chunkIdx int) error {
	if chunkIdx < 0 || chunkIdx >= len(p.batchChunks) {
		return fmt.Errorf("(MID_26062721) %s chunk index %d out of range (len=%d)",
			p.Name(), chunkIdx, len(p.batchChunks))
	}
	if isCtxStopped(ctx) {
		return ErrPipelineStopped
	}

	// Reuse processChunk which handles the LLM call and produces entityRelationChunkResult.
	// It already reads cacheTokenCounts and logs.
	result := p.processChunk(ctx, p.batchRecordID, chunkIdx, len(p.batchChunks),
		p.batchChunks[chunkIdx], p.batchDocCtx)
	if result.Failed {
		p.Logger.Warn("%s chunk %d failed (result.Failed=true), skipping",
			p.Name(), chunkIdx, "record_id", p.batchRecordID)
	}
	p.batchMu.Lock()
	p.batchResults = append(p.batchResults, result)
	p.batchMu.Unlock()
	return nil
}

// finalizeEntityBatch runs after all chunks have been processed. It:
//  1. Accumulates and consolidates entities (Phase 1.5)
//  2. Saves entities to the database
//  3. Writes entity artifact file and reindexes entity search
//
// Phase 2 (window-based relation extraction) is intentionally omitted here;
// relations are handled exclusively by RelationProcessor.FinalizeChunkBatch.
func (p *EntityRelationProcessor) finalizeEntityBatch(ctx context.Context) error {
	if len(p.batchResults) == 0 {
		p.Logger.Info("%s batch: no results", p.Name(), "record_id", p.batchRecordID)
		return nil
	}
	if isCtxStopped(ctx) {
		return ErrPipelineStopped
	}

	// --- Accumulate chunk results ---
	entities := make([]map[string]any, 0)
	detectedLanguage := "unknown"
	usedModel := strings.TrimSpace(p.ModelName)
	var failedChunks int

	for _, r := range p.batchResults {
		if r.Failed {
			failedChunks++
			continue
		}
		if r.Language != "" && detectedLanguage == "unknown" {
			detectedLanguage = r.Language
		}
		if strings.TrimSpace(r.ModelName) != "" {
			usedModel = strings.TrimSpace(r.ModelName)
		}
		entities = append(entities, r.Entities...)
	}

	if detectedLanguage == "" {
		detectedLanguage = "unknown"
	}

	// Load the record for finalization.
	rec, err := p.InputStore.GetInputRecord(ctx, p.batchRecordID)
	if err != nil {
		return fmt.Errorf("(MID_26062722) %s load record: %w", p.Name(), err)
	}
	p.batchRec = rec

	createTime := p.Now().UTC().Format(time.RFC3339)

	// Phase 1.5: consolidate duplicate entities.
	entities = consolidateEntities(entities)
	for i := range entities {
		entities[i]["entity_id"] = fmt.Sprintf("%d_ent_%d", p.batchRecordID, i+1)
		entities[i]["create_time"] = createTime
	}

	p.Logger.Info("consolidate entities (batch)",
		"record_id", p.batchRecordID,
		"entities", len(entities),
		"failed_chunks", failedChunks,
	)

	// ---- Save entities to DB ----
	eventID := fmt.Sprintf("batch_%d", p.batchRecordID)
	insertedEntities, saveErr := p.Store.SaveEntities(ctx, SaveEntitiesRequest{
		InputRecordID: p.batchRecordID,
		EventID:       eventID,
		Language:      detectedLanguage,
		ModelName:     firstNonEmptyTrimmed(usedModel, p.ModelName),
		PromptName:    p.PromptRef,
		DocName:       strings.TrimSpace(rec.Title),
		Entities:      entities,
	})
	if saveErr != nil {
		return fmt.Errorf("(MID_26062723) %s save entities: %w", p.Name(), saveErr)
	}

	// ---- Write entity artifact file ----
	if fileErr := p.saveEntitiesToFile(p.batchRecordID, rec, entities); fileErr != nil {
		p.Logger.Warn("save entities to file failed", "record_id", p.batchRecordID, "error", fileErr)
	}

	// ---- Reindex entity search ----
	if reindexErr := ReindexEntitySearchForRecord(ctx, p.batchRecordID, p.Logger); reindexErr != nil {
		p.Logger.Warn("reindex entity search registry failed", "record_id", p.batchRecordID, "error", reindexErr)
	}

	p.Logger.Info("entity batch complete",
		"record_id", p.batchRecordID,
		"inserted_entities", insertedEntities,
		"num_chunks", len(p.batchChunks),
		"language", detectedLanguage,
	)
	return nil
}
