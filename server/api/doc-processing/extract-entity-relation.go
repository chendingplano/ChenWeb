package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

// EntityRelationProcessor extracts entities and relations from chunks using
// a single LLM call per chunk. It writes to kb.entities and kb.relations,
// emits .entities and .relations artifact files, and refreshes the hybrid
// search registry. It does NOT generate category paths and does NOT touch
// ARTIFACT_WEB_DIR.
type EntityRelationProcessor struct {
	InputStore        DocMetadataStore
	Store             EntityRelationStore
	Extractor         LLMJSONExtractor
	Logger            ApiTypes.JimoLogger
	Now               func() time.Time
	PromptText        string
	PromptRef         string
	PromptPath        string
	PromptErr         error
	ModelRef          string
	ModelCfgPath      string
	ModelErr          error
	ModelName         string
	ModelCfg          structureModelConfig
	FallbackModelRef  string
	FallbackModelPath string
	FallbackModelErr  error
	FallbackModelName string
	FallbackModelCfg  structureModelConfig
	ArtifactDir       string
}

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
	Language  string
	Entities  []map[string]any
	Relations []map[string]any
	ModelName string
}

func NewEntityRelationProcessor(
	inputStore DocMetadataStore,
	store EntityRelationStore,
	extractor LLMJSONExtractor,
	logger ApiTypes.JimoLogger,
) *EntityRelationProcessor {
	if logger == nil {
		logger = loggerutil.CreateDefaultLogger("MID_26052701")
	}
	promptText, promptRef, promptPath, promptErr := loadProductPromptFromEnvKeys(
		[]string{"EXTRACT_ENTITY_RELATION_PROMPT"},
		"prompt-extract-entity-relation-v1.md",
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
		InputStore:        inputStore,
		Store:             store,
		Extractor:         extractor,
		Logger:            logger,
		Now:               time.Now,
		PromptText:        promptText,
		PromptRef:         promptRef,
		PromptPath:        promptPath,
		PromptErr:         promptErr,
		ModelRef:          modelRef,
		ModelCfgPath:      modelCfgPath,
		ModelErr:          modelErr,
		ModelName:         modelCfg.ModelName,
		ModelCfg:          modelCfg,
		FallbackModelRef:  fallbackModelRef,
		FallbackModelPath: fallbackModelPath,
		FallbackModelErr:  fallbackModelErr,
		FallbackModelName: fallbackModelCfg.ModelName,
		FallbackModelCfg:  fallbackModelCfg,
		ArtifactDir:       strings.TrimSpace(os.Getenv("ARTIFACT_DIR")),
	}
}

func (p *EntityRelationProcessor) Name() string { return "extract_entity_relation" }

func (p *EntityRelationProcessor) HandleEvent(ctx context.Context, payload []byte) error {
	start := p.Now()
	evt, err := ParseLineFileGeneratedEvent(payload)
	if err != nil {
		p.Logger.Error("parse input file failed", "error", err)
		return fmt.Errorf("(MID_26052702) parse event payload: %w", err)
	}
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
			p.persistEntityRelationStatus(ctx, rec, start, nil)
			return nil
		}
	}

	body, readErr := os.ReadFile(lineFilePath)
	if readErr != nil {
		p.Logger.Error("read line file failed", "record_id", evt.RecordID, "path", lineFilePath, "error", readErr)
		p.persistEntityRelationStatus(ctx, rec, start, fmt.Errorf("(MID_26052708) read line file for record_id=%d: %w", evt.RecordID, readErr))
		return fmt.Errorf("(MID_26052709) failed reading line file, error:%w, path:%s", readErr, lineFilePath)
	}
	lines, parseErr := ParseInputLines(body)
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

	result := p.extractEntityRelationFromChunks(ctx, evt.RecordID, chunks)
	detectedLanguage := result.Language
	if detectedLanguage == "" {
		detectedLanguage = "unknown"
	}

	for i := range result.Entities {
		result.Entities[i]["entity_id"] = fmt.Sprintf("%d_e_%d", evt.RecordID, i+1)
	}
	for i := range result.Relations {
		result.Relations[i]["relation_id"] = fmt.Sprintf("%d_r_%d", evt.RecordID, i+1)
	}

	eventID := eventIDFromContext(ctx)
	insertedEntities, err := p.Store.SaveEntities(ctx, SaveEntitiesRequest{
		InputRecordID: evt.RecordID,
		EventID:       eventID,
		Language:      detectedLanguage,
		ModelName:     firstNonEmptyTrimmed(result.ModelName, p.ModelName),
		PromptName:    p.PromptRef,
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
	return nil
}

func (p *EntityRelationProcessor) extractEntityRelationFromChunks(
	ctx context.Context,
	recordID int64,
	chunks []Chunk,
) entityRelationExtractionResult {
	entities := make([]map[string]any, 0)
	relations := make([]map[string]any, 0)
	detectedLanguage := "unknown"
	usedModel := strings.TrimSpace(p.ModelName)

	for idx, chunk := range chunks {
		chunkText := buildMarkedChunkInputText(chunk.Lines)
		startTime := time.Now()
		p.Logger.Info("entity-relation start",
			"record_id", recordID,
			"chunk_idx", idx,
			"seq_no", chunk.SeqNo,
			"model_name", p.ModelName,
			"prompt", p.PromptRef,
		)
		payload, modelName, err := p.extractEntityRelationWithFallback(ctx, chunkText)
		if err != nil {
			p.Logger.Warn("entity-relation extraction failed for chunk; skipping",
				"record_id", recordID,
				"chunk_idx", idx,
				"seq_no", chunk.SeqNo,
				"error", err,
			)
			continue
		}
		usedModel = strings.TrimSpace(firstNonEmptyTrimmed(modelName, usedModel))
		if payload == nil {
			continue
		}
		if lang := strings.TrimSpace(asString(payload["language"])); lang != "" && detectedLanguage == "unknown" {
			detectedLanguage = lang
		}

		chunkEntities := normalizeEntityRows(payload["entities"], chunk.SeqNo)
		chunkRelations := normalizeRelationRows(payload["relations"], chunk.SeqNo)
		entities = append(entities, chunkEntities...)
		relations = append(relations, chunkRelations...)
		p.Logger.Info("entity-relation done",
			"record_id", recordID,
			"chunk_idx", idx,
			"seq_no", chunk.SeqNo,
			"entities", len(chunkEntities),
			"relations", len(chunkRelations),
			"ms_used", time.Since(startTime).Milliseconds(),
		)
	}

	p.Logger.Info("entity-relation final results",
		"record_id", recordID,
		"total_chunks", len(chunks),
		"entities", len(entities),
		"relations", len(relations),
		"language", detectedLanguage,
	)
	return entityRelationExtractionResult{
		Language:  detectedLanguage,
		Entities:  entities,
		Relations: relations,
		ModelName: usedModel,
	}
}

func (p *EntityRelationProcessor) extractEntityRelationWithFallback(ctx context.Context, inputText string) (map[string]any, string, error) {
	payload, err := p.extractEntityRelationPayload(ctx, inputText, p.ModelName, p.ModelCfg)
	if err == nil {
		return payload, strings.TrimSpace(p.ModelName), nil
	}

	primaryModelName := strings.TrimSpace(p.ModelName)
	fallbackModelName := strings.TrimSpace(p.FallbackModelName)
	if fallbackModelName == "" {
		return nil, primaryModelName, err
	}
	if p.FallbackModelErr != nil {
		return nil, fallbackModelName, fmt.Errorf("(MID_26052720) primary entity-relation extraction failed and fallback model %q is unavailable: %w", p.FallbackModelRef, err)
	}

	p.Logger.Warn("primary entity-relation extraction failed; retrying fallback model",
		"primary_model", primaryModelName,
		"fallback_model", fallbackModelName,
		"error", err,
		"prompt_name", p.PromptRef,
	)

	fallbackPayload, fallbackErr := p.extractEntityRelationPayload(ctx, inputText, fallbackModelName, p.FallbackModelCfg)
	if fallbackErr != nil {
		if isEmptyEntityRelationError(fallbackErr) {
			p.Logger.Info("fallback entity-relation extraction returned empty JSON; treating as empty result",
				"fallback_model", fallbackModelName)
			return map[string]any{"language": "unknown", "entities": []any{}, "relations": []any{}}, fallbackModelName, nil
		}
		return nil, fallbackModelName, fmt.Errorf("(MID_26052721) primary extraction failed: %w; fallback extraction failed: %v", err, fallbackErr)
	}
	return fallbackPayload, fallbackModelName, nil
}

func (p *EntityRelationProcessor) extractEntityRelationPayload(
	ctx context.Context,
	inputText string,
	modelName string,
	cfg structureModelConfig,
) (map[string]any, error) {
	applyStructureModelConfigToExtractor(p.Extractor, cfg)
	in := llmclients.JSONExtractionInput{
		PromptText: p.PromptText,
		ModelName:  modelName,
		InputText:  inputText,
	}
	var (
		payload map[string]any
		err     error
	)
	if structuredExtractor, ok := p.Extractor.(LLMStructuredJSONExtractor); ok {
		var result *llmclients.StructuredOutputResult
		result, err = structuredExtractor.ExtractStructuredJSON(ctx, in, entityRelationExtractionContract())
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
		return nil, errors.New("(MID_26052730) empty llm payload")
	}
	return payload, nil
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

// normalizeLineSpansInput accepts the entity/relation prompt's `lines` array
// (which uses `-` separators per the prompt spec) and rewrites each item so
// that the shared normalizeSourceLineSpans (which expects `:`) can consume it.
// Pass-through for items that are already numbers or use `:`.
func normalizeLineSpansInput(raw any) any {
	items, ok := raw.([]any)
	if !ok {
		return raw
	}
	out := make([]any, 0, len(items))
	for _, item := range items {
		switch v := item.(type) {
		case string:
			s := strings.TrimSpace(v)
			if strings.Contains(s, ":") {
				out = append(out, s)
				continue
			}
			if idx := strings.Index(s, "-"); idx > 0 {
				out = append(out, s[:idx]+":"+s[idx+1:])
				continue
			}
			out = append(out, s)
		default:
			out = append(out, item)
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
			"source_line_spans": normalizeSourceLineSpans(normalizeLineSpansInput(m["lines"])),
			"confidence":        toFloat(m["confidence"]),
			"chunk_seq_no":      chunkSeqNo,
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
		row := map[string]any{
			"subject":           subject,
			"subject_en":        strings.TrimSpace(asString(m["subject_en"])),
			"predicate":         predicate,
			"predicate_en":      normalizePredicate(asString(m["predicate_en"])),
			"object":            object,
			"object_en":         strings.TrimSpace(asString(m["object_en"])),
			"desc":              strings.TrimSpace(asString(m["desc"])),
			"desc_en":           strings.TrimSpace(asString(m["desc_en"])),
			"keywords":          toStringSlice(m["keywords"]),
			"keywords_en":       toStringSlice(m["keywords_en"]),
			"source_line_spans": normalizeSourceLineSpans(normalizeLineSpansInput(m["lines"])),
			"confidence":        toFloat(m["confidence"]),
			"chunk_seq_no":      chunkSeqNo,
		}
		out = append(out, row)
	}
	return out
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
	Start         time.Time
	DurationMs    int64
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
	statusRaw, err := appendEntityRelationStatus(rec.StatusRaw, entityRelationStatusParams{
		RecordID:      rec.ID,
		FileType:      detectEntityRelationFileType(rec),
		InputFilename: strings.TrimSpace(rec.ResultFilename),
		Start:         start,
		DurationMs:    time.Since(start).Milliseconds(),
		ProcErr:       procErr,
	})
	if err != nil {
		p.Logger.Error("failed building entity-relation status", "record_id", rec.ID, "error", err)
		return
	}
	if updateErr := p.InputStore.UpdateInputMetadata(ctx, rec.ID, DocMetadataUpdate{
		StatusRaw: statusRaw,
		ErrorMsg:  errMsg,
	}); updateErr != nil {
		p.Logger.Error("failed persisting entity-relation status", "record_id", rec.ID, "error", updateErr)
	}
}

func appendEntityRelationStatus(raw string, p entityRelationStatusParams) (string, error) {
	entries := decodeDocMetaStatus(raw)
	entry := map[string]any{
		"record_id":      strconv.FormatInt(p.RecordID, 10),
		"file_type":      strings.ToLower(strings.TrimSpace(p.FileType)),
		"operation":      "extract_entity_relation",
		"input_filename": strings.TrimSpace(p.InputFilename),
		"start_time":     p.Start.Format(defaultDocMetaStatusTime),
		"ms_used":        p.DurationMs,
	}
	if p.ProcErr == nil {
		entry["proc_status"] = "success"
	} else {
		entry["proc_status"] = "failed"
		entry["error"] = strings.TrimSpace(p.ProcErr.Error())
	}

	replaced := false
	out := make([]map[string]any, 0, len(entries)+1)
	for _, e := range entries {
		op := strings.ToLower(strings.TrimSpace(asString(e["operation"])))
		if op != "extract_entity_relation" {
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
    source_line_spans JSONB,
    confidence DOUBLE PRECISION,
    model_name TEXT,
    prompt_name TEXT,
    search_document TEXT,
    search_vector TSVECTOR,
    ext_info JSONB,
    create_time TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_kb_entities_input_record_id ON kb.entities (input_record_id);
CREATE INDEX IF NOT EXISTS idx_kb_entities_entity_id ON kb.entities (entity_id);

CREATE TABLE IF NOT EXISTS kb.relations (
    id BIGSERIAL PRIMARY KEY,
    event_id TEXT,
    input_record_id BIGINT NOT NULL,
    relation_id TEXT,
    language TEXT,
    subject TEXT,
    subject_en TEXT,
    predicate TEXT,
    predicate_en TEXT,
    object TEXT,
    object_en TEXT,
    desc_text TEXT,
    desc_text_en TEXT,
    keywords JSONB,
    keywords_en JSONB,
    source_line_spans JSONB,
    confidence DOUBLE PRECISION,
    model_name TEXT,
    prompt_name TEXT,
    search_document TEXT,
    search_vector TSVECTOR,
    ext_info JSONB,
    create_time TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_kb_relations_input_record_id ON kb.relations (input_record_id);
CREATE INDEX IF NOT EXISTS idx_kb_relations_relation_id ON kb.relations (relation_id);
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
    source_line_spans,
    confidence,
    model_name,
    prompt_name,
    ext_info
) VALUES (
    $1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10::jsonb,$11,$12,$13::jsonb,$14::jsonb,$15::jsonb,$16,$17,$18,$19::jsonb
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
		spansJSON, _ := json.Marshal(e["source_line_spans"])
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
    predicate,
    predicate_en,
    object,
    object_en,
    desc_text,
    desc_text_en,
    keywords,
    keywords_en,
    source_line_spans,
    confidence,
    model_name,
    prompt_name,
    ext_info
) VALUES (
    $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13::jsonb,$14::jsonb,$15::jsonb,$16,$17,$18,$19::jsonb
)`

	isEnglish := isLanguageEnglish(req.Language)
	var eventIDVal any
	if id := strings.TrimSpace(req.EventID); id != "" {
		eventIDVal = id
	}

	var inserted int64
	for _, r := range req.Relations {
		keywordsJSON, _ := json.Marshal(r["keywords"])
		spansJSON, _ := json.Marshal(r["source_line_spans"])
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

		if _, err := s.DB.ExecContext(ctx, stmt,
			eventIDVal,
			req.InputRecordID,
			strings.TrimSpace(asString(r["relation_id"])),
			strings.TrimSpace(req.Language),
			strings.TrimSpace(asString(r["subject"])),
			subjectEnVal,
			strings.TrimSpace(asString(r["predicate"])),
			predicateEnVal,
			strings.TrimSpace(asString(r["object"])),
			objectEnVal,
			strings.TrimSpace(asString(r["desc"])),
			descEnVal,
			string(keywordsJSON),
			keywordsEnVal,
			string(spansJSON),
			toFloat(r["confidence"]),
			strings.TrimSpace(req.ModelName),
			strings.TrimSpace(req.PromptName),
			string(extInfo),
		); err != nil {
			return inserted, err
		}
		inserted++
	}
	return inserted, nil
}

func isLanguageEnglish(language string) bool {
	s := strings.TrimSpace(language)
	return strings.EqualFold(s, "en") || strings.EqualFold(s, "english")
}
