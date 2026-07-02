package docprocessing

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// Split entity / relation extraction (ADR 2026061702). The legacy combined
// EntityRelationProcessor.HandleEvent is superseded by two independent
// processors that subscribe to the same line-file-generated event and run in
// parallel (in concurrent mode):
//
//   - EntityProcessor   ("extract_entity")   — Phase 1 entity extraction + save.
//   - RelationProcessor ("extract_relation") — free-form relation extraction +
//                                               save; endpoint linking deferred
//                                               to Phase C (linkRecordEndpoints).
//
// Both wrap a shared *EntityRelationProcessor core (reusing its extraction,
// status, and indexing helpers); they differ only in Name(), which phase they
// run, and their StatusOperation. Endpoint linking + indexing run once in
// Phase C, owned by RelationProcessor (EntityProcessor no-ops its Phase C when
// relations exist, avoiding a concurrent double-index).
//
// NOTE: staging-unverified — compiles and the pure helpers are unit-tested, but
// the end-to-end flow (LLM calls, status logging, indexing) has not been run.

type extractionPrep struct {
	evt    LineFileGeneratedEvent
	rec    DocMetadataInputRecord
	docCtx string
	lines  []Line
	chunks []Chunk
	start  time.Time
}

// prepareForExtraction does the work shared by both phases: parse the event,
// load the record, resolve + read + parse the line file, and load chunks.
// proceed=false means the event was already handled (skipped, or an error was
// persisted) and the caller should return err as-is.
func (p *EntityRelationProcessor) prepareForExtraction(ctx context.Context, payload []byte) (extractionPrep, bool, error) {
	start := p.Now()
	evt, err := ParseLineFileGeneratedEvent(payload)
	if err != nil {
		p.Logger.Error("parse input file failed", "error", err)
		return extractionPrep{}, false, fmt.Errorf("(MID_26052702) parse event payload: %w", err)
	}
	if ShouldSkipLineFileGeneratedEvent(evt) {
		p.Logger.Warn("entity/relation processor skipped", "operation", p.statusOp())
		return extractionPrep{}, false, nil
	}
	rec, err := p.InputStore.GetInputRecord(ctx, evt.RecordID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			p.Logger.Error("kb.inputs record not found", "record_id", evt.RecordID)
			return extractionPrep{}, false, nil
		}
		return extractionPrep{}, false, fmt.Errorf("(MID_26052704) load kb.inputs record %d: %w", evt.RecordID, err)
	}
	docCtx := buildDocContextLine(rec)
	if docCtx != "" {
		p.Logger.Info("document context for extraction",
			"record_id", evt.RecordID,
			"doc_context", docCtx)
	}
	if p.ModelErr != nil {
		p.Logger.Warn("extraction skipped: model config error", "record_id", evt.RecordID, "error", p.ModelErr)
		p.persistEntityRelationStatus(ctx, rec, start, p.ModelErr)
		return extractionPrep{}, false, nil
	}
	lineFilePath, lineFileErr := ResolveInputFilePath(evt, rec.ResultFilename, rec.ParserName, rec.StagingFilename)
	if lineFileErr != nil {
		p.persistEntityRelationStatus(ctx, rec, start, fmt.Errorf("(MID_26052705) resolve line file for record_id=%d: %w", evt.RecordID, lineFileErr))
		return extractionPrep{}, false, nil
	}
	body, readErr := os.ReadFile(lineFilePath)
	if readErr != nil {
		p.persistEntityRelationStatus(ctx, rec, start, fmt.Errorf("(MID_26052708) read line file for record_id=%d: %w", evt.RecordID, readErr))
		return extractionPrep{}, false, fmt.Errorf("(MID_26052709) failed reading line file, error:%w, path:%s", readErr, lineFilePath)
	}
	lines, parseErr := ParseInputLinesIncludingTOC(body)
	if parseErr != nil {
		p.persistEntityRelationStatus(ctx, rec, start, fmt.Errorf("(MID_26052710) parse input lines for record_id=%d: %w", evt.RecordID, parseErr))
		return extractionPrep{}, false, fmt.Errorf("(MID_26052711) failed parsing input lines, error:%w", parseErr)
	}
	artifactBase := buildChunkArtifactBaseName(rec.StagingFilename, rec.ParserName)
	chunks, loadErr := loadChunksFromArtifactFile(p.ArtifactDir, evt.RecordID, artifactBase+".chunks", lines)
	if loadErr != nil {
		p.persistEntityRelationStatus(ctx, rec, start, fmt.Errorf("(MID_26052712) load chunk artifact for record_id=%d: %w", evt.RecordID, loadErr))
		return extractionPrep{}, false, fmt.Errorf("(MID_26052713) failed loading chunk artifact, error:%w", loadErr)
	}
	if len(chunks) == 0 {
		p.persistEntityRelationStatus(ctx, rec, start, fmt.Errorf("(MID_26052714) no chunks found for record_id=%d", evt.RecordID))
		return extractionPrep{}, false, nil
	}
	return extractionPrep{evt: evt, rec: rec, docCtx: docCtx, lines: lines, chunks: chunks, start: start}, true, nil
}

// handleEntityExtraction is Phase 1 only: extract + consolidate + save entities.
func (p *EntityRelationProcessor) handleEntityExtraction(ctx context.Context, payload []byte) error {
	if p.PromptErr != nil {
		p.Logger.Error("entity prompt error", "error", p.PromptErr)
		return fmt.Errorf("(MID_26052703) load entity prompt %q: %w", p.PromptRef, p.PromptErr)
	}
	prep, proceed, err := p.prepareForExtraction(ctx, payload)
	if err != nil || !proceed {
		return err
	}
	evt, rec, start := prep.evt, prep.rec, prep.start

	if evt.Force {
		_, _ = p.Store.DeleteEntitiesByInputRecordID(ctx, evt.RecordID)
	} else {
		exist, eErr := p.Store.EntitiesExist(ctx, evt.RecordID)
		if eErr != nil {
			p.persistEntityRelationStatus(ctx, rec, start, eErr)
			return fmt.Errorf("(MID_26052706) check entities for record_id=%d: %w", evt.RecordID, eErr)
		}
		if exist {
			p.Logger.Info("entity extraction skipped: rows exist and force=false", "record_id", evt.RecordID)
			reindexExistingSearchOnSkip(ctx, searchArtifactEntity, evt.RecordID, p.Logger, ReindexEntitySearchForRecord)
			p.persistEntityRelationStatus(ctx, rec, start, nil)
			return nil
		}
	}

	p.persistEntityRelationInProgressStatus(ctx, rec, start, fmt.Sprintf("0%% (0/%d)", len(prep.chunks)))
	result, err := p.extractEntitiesFromChunks(ctx, evt.RecordID, prep.chunks, prep.docCtx)
	if err != nil {
		if errors.Is(err, ErrPipelineStopped) {
			p.stopAndPersistEntityRelation(context.Background(), rec, start)
			return ErrPipelineStopped
		}
		p.persistEntityRelationStatus(ctx, rec, start, err)
		return fmt.Errorf("(MID_26052717) entity extraction failed, error:%w", err)
	}

	language := result.Language
	if language == "" {
		language = "unknown"
	}
	createTime := p.Now().UTC().Format(time.RFC3339)
	result.Entities = consolidateEntities(result.Entities)
	for i := range result.Entities {
		result.Entities[i]["entity_id"] = fmt.Sprintf("%d_ent_%d", evt.RecordID, i+1)
		result.Entities[i]["create_time"] = createTime
	}

	eventID := eventIDFromContext(ctx)
	inserted, err := p.Store.SaveEntities(ctx, SaveEntitiesRequest{
		InputRecordID: evt.RecordID,
		EventID:       eventID,
		Language:      language,
		ModelName:     firstNonEmptyTrimmed(result.ModelName, p.ModelName),
		PromptName:    p.PromptRef,
		DocName:       strings.TrimSpace(rec.Title),
		Entities:      result.Entities,
	})
	if err != nil {
		p.persistEntityRelationStatus(ctx, rec, start, err)
		return fmt.Errorf("(MID_26052715) insert entities failed, error:%w", err)
	}
	if fileErr := p.saveEntitiesToFile(evt.RecordID, rec, result.Entities); fileErr != nil {
		p.Logger.Warn("save entities to file failed", "record_id", evt.RecordID, "error", fileErr)
	}
	if reErr := ReindexEntitySearchForRecord(ctx, evt.RecordID, p.Logger); reErr != nil {
		p.Logger.Warn("reindex entity search registry failed", "record_id", evt.RecordID, "error", reErr)
	}
	p.Logger.Info("entities extracted", "record_id", evt.RecordID, "inserted_entities", inserted, "language", language)
	p.persistEntityRelationStatus(ctx, rec, start, nil)
	return nil
}

// handleRelationExtraction is the free-form relation pass: extract + save
// relations with empty entity ids; Phase C links the endpoints.
func (p *EntityRelationProcessor) handleRelationExtraction(ctx context.Context, payload []byte) error {
	if strings.TrimSpace(p.RelationPromptText) == "" || p.RelationPromptErr != nil {
		p.Logger.Warn("relation prompt unavailable; relation extraction skipped",
			"relation_prompt_err", p.RelationPromptErr)
		return nil
	}
	prep, proceed, err := p.prepareForExtraction(ctx, payload)
	if err != nil || !proceed {
		return err
	}
	evt, rec, start := prep.evt, prep.rec, prep.start

	if evt.Force {
		_, _ = p.Store.DeleteRelationsByInputRecordID(ctx, evt.RecordID)
	} else {
		exist, rErr := p.Store.RelationsExist(ctx, evt.RecordID)
		if rErr != nil {
			p.persistEntityRelationStatus(ctx, rec, start, rErr)
			return fmt.Errorf("(MID_26052707) check relations for record_id=%d: %w", evt.RecordID, rErr)
		}
		if exist {
			p.Logger.Info("relation extraction skipped: rows exist and force=false", "record_id", evt.RecordID)
			reindexExistingSearchOnSkip(ctx, searchArtifactRelation, evt.RecordID, p.Logger, ReindexRelationSearchForRecord)
			p.persistEntityRelationStatus(ctx, rec, start, nil)
			return nil
		}
	}

	p.persistEntityRelationInProgressStatus(ctx, rec, start, "0%")
	relations, language, err := p.extractFreeformRelations(ctx, evt.RecordID, prep.chunks, prep.docCtx)
	if err != nil {
		if errors.Is(err, ErrPipelineStopped) {
			p.stopAndPersistEntityRelation(context.Background(), rec, start)
			return ErrPipelineStopped
		}
		p.persistEntityRelationStatus(ctx, rec, start, err)
		return fmt.Errorf("(MID_26052717) relation extraction failed, error:%w", err)
	}
	if language == "" {
		language = "unknown"
	}
	createTime := p.Now().UTC().Format(time.RFC3339)
	for i := range relations {
		relations[i]["relation_id"] = fmt.Sprintf("%d_rel_%d", evt.RecordID, i+1)
		relations[i]["create_time"] = createTime
		// subject_entity_id / object_entity_id are assigned in Phase C (DR3/DR4).
	}

	eventID := eventIDFromContext(ctx)
	inserted, err := p.Store.SaveRelations(ctx, SaveRelationsRequest{
		InputRecordID: evt.RecordID,
		EventID:       eventID,
		Language:      language,
		ModelName:     p.ModelName,
		PromptName:    p.RelationPromptRef,
		Relations:     relations,
	})
	if err != nil {
		p.persistEntityRelationStatus(ctx, rec, start, err)
		return fmt.Errorf("(MID_26052716) insert relations failed, error:%w", err)
	}
	if fileErr := p.saveRelationsToFile(evt.RecordID, rec, relations); fileErr != nil {
		p.Logger.Warn("save relations to file failed", "record_id", evt.RecordID, "error", fileErr)
	}
	if reErr := ReindexRelationSearchForRecord(ctx, evt.RecordID, p.Logger); reErr != nil {
		p.Logger.Warn("reindex relation search registry failed", "record_id", evt.RecordID, "error", reErr)
	}
	p.Logger.Info("relations extracted (free-form)", "record_id", evt.RecordID, "inserted_relations", inserted, "language", language)
	p.persistEntityRelationStatus(ctx, rec, start, nil)
	return nil
}

// ---- processor types ----

// EntityProcessor runs entity extraction only (Name "extract_entity").
type EntityProcessor struct {
	*EntityRelationProcessor
}

func NewEntityProcessor(inputStore DocMetadataStore, store EntityRelationStore, extractor LLMJSONExtractor, logger ApiTypes.JimoLogger) *EntityProcessor {
	core := NewEntityRelationProcessor(inputStore, store, extractor, logger)
	core.StatusOperation = "extract_entity"
	return &EntityProcessor{core}
}

func (p *EntityProcessor) Name() string { return "extract_entity" }

func (p *EntityProcessor) HandleEvent(ctx context.Context, payload []byte) error {
	return p.handleEntityExtraction(ctx, payload)
}

// PostProcessIndex indexes entities for the entity-only case. When relations also
// exist for the record, RelationProcessor owns the single Phase C indexing pass,
// so this no-ops to avoid a concurrent double-index.
func (p *EntityProcessor) PostProcessIndex(ctx context.Context, recordID int64) error {
	// Semantic clustering runs regardless of whether relations exist for this record
	// — it depends only on entity search registry reindex (which happened before
	// PostProcessIndex is called).
	if semClusterErr := semClusterEntities(ctx, ApiTypes.ProjectDBHandle, recordID, p.Logger); semClusterErr != nil {
		p.Logger.Warn("semantic clustering failed (non-fatal)", "record_id", recordID, "error", semClusterErr)
	}
	if rel, err := p.Store.RelationsExist(ctx, recordID); err == nil && rel {
		return nil
	}
	return p.EntityRelationProcessor.PostProcessIndex(ctx, recordID)
}

// RelationProcessor runs free-form relation extraction (Name "extract_relation")
// and owns Phase C endpoint linking + indexing.
type RelationProcessor struct {
	*EntityRelationProcessor
}

func NewRelationProcessor(inputStore DocMetadataStore, store EntityRelationStore, extractor LLMJSONExtractor, logger ApiTypes.JimoLogger) *RelationProcessor {
	core := NewEntityRelationProcessor(inputStore, store, extractor, logger)
	core.StatusOperation = "extract_relation"
	return &RelationProcessor{core}
}

func (p *RelationProcessor) Name() string { return "extract_relation" }

func (p *RelationProcessor) HandleEvent(ctx context.Context, payload []byte) error {
	return p.handleRelationExtraction(ctx, payload)
}

// PostProcessIndex runs endpoint linking (DR3/DR4) before delegating to the core
// indexing pass, which reads the now-populated subject/object entity ids.
func (p *RelationProcessor) PostProcessIndex(ctx context.Context, recordID int64) error {
	if ls, ok := p.Store.(RelationLinkStore); ok {
		createTime := p.Now().UTC().Format(time.RFC3339)
		stats, err := linkRecordEndpoints(ctx, ls, recordID, eventIDFromContext(ctx), "", p.ModelName, p.RelationPromptRef, createTime)
		if err != nil {
			p.Logger.Error("phase C endpoint linking failed", "record_id", recordID, "error", err)
			return err
		}
		p.Logger.Info("phase C endpoint linking",
			"record_id", recordID,
			"relations", stats.Relations, "linked", stats.Linked, "provisional", stats.Provisional)
	} else {
		p.Logger.Warn("store does not support endpoint linking; skipping link step", "record_id", recordID)
	}
	return p.EntityRelationProcessor.PostProcessIndex(ctx, recordID)
}

// ---- ChunkBatchProcessor implementation for EntityProcessor (entity-only) ----

func (p *EntityProcessor) InitChunkBatch(ctx context.Context, recordID int64, chunks []Chunk, docCtx string) error {
	return p.EntityRelationProcessor.initEntityBatch(ctx, recordID, chunks, docCtx)
}

func (p *EntityProcessor) ProcessChunk(ctx context.Context, chunkIdx int) error {
	return p.EntityRelationProcessor.processEntityChunk(ctx, chunkIdx)
}

func (p *EntityProcessor) FinalizeChunkBatch(ctx context.Context) error {
	return p.EntityRelationProcessor.finalizeEntityBatch(ctx)
}

// ---- ChunkBatchProcessor implementation for RelationProcessor (relation-only) ----

func (p *RelationProcessor) InitChunkBatch(ctx context.Context, recordID int64, chunks []Chunk, docCtx string) error {
	if strings.TrimSpace(p.RelationPromptText) == "" || p.RelationPromptErr != nil {
		p.Logger.Warn("relation prompt unavailable; relation batch skipped", "record_id", recordID)
	}
	p.batchStart = p.Now()
	p.batchRecordID = recordID
	p.batchChunks = chunks
	p.batchDocCtx = docCtx
	p.batchRelations = nil
	p.batchRelLang = "unknown"
	return nil
}

func (p *RelationProcessor) ProcessChunk(ctx context.Context, chunkIdx int) error {
	if chunkIdx < 0 || chunkIdx >= len(p.batchChunks) {
		return fmt.Errorf("(MID_26062741) %s chunk index %d out of range (len=%d)",
			p.Name(), chunkIdx, len(p.batchChunks))
	}
	if isCtxStopped(ctx) {
		return ErrPipelineStopped
	}
	if strings.TrimSpace(p.RelationPromptText) == "" || p.RelationPromptErr != nil {
		return nil // relation extraction disabled; nothing to do
	}
	// Free-form relations for a single chunk.
	rels, lang, err := p.extractFreeformRelations(ctx, p.batchRecordID,
		p.batchChunks[chunkIdx:chunkIdx+1], p.batchDocCtx)
	if err != nil {
		if errors.Is(err, ErrPipelineStopped) {
			return ErrPipelineStopped
		}
		p.Logger.Warn("relation chunk failed", "processor", p.Name(), "record_id", p.batchRecordID, "chunk", chunkIdx, "error", err)
		return nil
	}
	if lang != "" && p.batchRelLang == "unknown" {
		p.batchRelLang = lang
	}
	p.batchRelations = append(p.batchRelations, rels...)
	return nil
}

func (p *RelationProcessor) FinalizeChunkBatch(ctx context.Context) error {
	if len(p.batchRelations) == 0 {
		return nil
	}
	if isCtxStopped(ctx) {
		return ErrPipelineStopped
	}
	rec, err := p.InputStore.GetInputRecord(ctx, p.batchRecordID)
	if err != nil {
		return fmt.Errorf("(MID_26062742) %s load record: %w", p.Name(), err)
	}
	createTime := p.Now().UTC().Format(time.RFC3339)
	for i := range p.batchRelations {
		p.batchRelations[i]["relation_id"] = fmt.Sprintf("%d_rel_%d", p.batchRecordID, i+1)
		p.batchRelations[i]["create_time"] = createTime
	}
	if _, err := p.Store.SaveRelations(ctx, SaveRelationsRequest{
		InputRecordID: p.batchRecordID,
		EventID:       eventIDFromContext(ctx),
		Language:      firstNonEmptyTrimmed(p.batchRelLang, "unknown"),
		ModelName:     p.ModelName,
		PromptName:    p.RelationPromptRef,
		Relations:     p.batchRelations,
	}); err != nil {
		return fmt.Errorf("(MID_26062743) %s save relations: %w", p.Name(), err)
	}
	if fileErr := p.saveRelationsToFile(p.batchRecordID, rec, p.batchRelations); fileErr != nil {
		p.Logger.Warn("save relations to file failed", "record_id", p.batchRecordID, "error", fileErr)
	}
	if reErr := ReindexRelationSearchForRecord(ctx, p.batchRecordID, p.Logger); reErr != nil {
		p.Logger.Warn("reindex relation search failed", "record_id", p.batchRecordID, "error", reErr)
	}
	return nil
}
