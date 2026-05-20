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
	"strconv"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

type SceneBlocksProcessor struct {
	InputStore        DocMetadataStore
	Store             SceneObjectsStore
	Extractor         LLMJSONExtractor
	Logger            ApiTypes.JimoLogger
	Now               func() time.Time
	PromptText        string
	PromptRef         string
	PromptErr         error
	ModelRef          string
	ModelErr          error
	ModelName         string
	ModelCfg          structureModelConfig
	FallbackModelRef  string
	FallbackModelErr  error
	FallbackModelName string
	FallbackModelCfg  structureModelConfig
	ChunkSize         int
	OverlapPercent    int
	PrevOverlap       int
	NextOverlap       int
	RemoveTOC         bool
	ArtifactDir       string
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
	jsonStringSlice := func(v any) []string {
		items, ok := v.([]any)
		if !ok {
			return nil
		}
		out := make([]string, 0, len(items))
		for _, item := range items {
			s, ok := item.(string)
			if !ok {
				continue
			}
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			out = append(out, s)
		}
		return out
	}
	out := map[string]any{}
	if titleEn := strings.TrimSpace(asString(block["title_en"])); titleEn != "" {
		out["title_en"] = titleEn
	}
	if summaryEn := strings.TrimSpace(asString(block["summary_en"])); summaryEn != "" {
		out["summary_en"] = summaryEn
	}
	if evidenceLines := jsonStringSlice(block["evidence_lines"]); len(evidenceLines) > 0 {
		out["evidence_lines"] = evidenceLines
	}
	if keywordsEn := jsonStringSlice(block["keywords_en"]); len(keywordsEn) > 0 {
		out["keywords_en"] = keywordsEn
	}
	if statesEn := jsonStringSlice(block["states_en"]); len(statesEn) > 0 {
		out["states_en"] = statesEn
	}
	return out
}

type SceneObjectsSQLStore struct {
	DB *sql.DB
}

func NewSceneBlocksProcessor(inputStore DocMetadataStore, store SceneObjectsStore, extractor LLMJSONExtractor, logger ApiTypes.JimoLogger) *SceneBlocksProcessor {
	if logger == nil {
		logger = loggerutil.CreateDefaultLogger("MID_26051801")
	}
	promptText, promptRef, promptErr := loadSceneBlocksPromptFromEnv()
	modelRef, _, modelCfg, modelErr := loadModelConfigFromEnv("EXTRACT_SCENE_BLOCKS_MODEL_NAME", "EXTRACT_SCENE_BLOCKS_MODELS_FILE")
	fallbackModelRef, _, fallbackModelCfg, fallbackModelErr := loadOptionalModelConfigFromEnv("EXTRACT_SCENE_BLOCKS_MODEL_FALLBACK", "EXTRACT_SCENE_BLOCKS_MODELS_FILE")
	applyStructureModelConfigToExtractor(extractor, modelCfg)
	prevOverlap, nextOverlap, removeTOC := blockingConfigFromViper()
	return &SceneBlocksProcessor{
		InputStore:        inputStore,
		Store:             store,
		Extractor:         extractor,
		Logger:            logger,
		Now:               time.Now,
		PromptText:        promptText,
		PromptRef:         promptRef,
		PromptErr:         promptErr,
		ModelRef:          modelRef,
		ModelErr:          modelErr,
		ModelName:         modelCfg.ModelName,
		ModelCfg:          modelCfg,
		FallbackModelRef:  fallbackModelRef,
		FallbackModelErr:  fallbackModelErr,
		FallbackModelName: fallbackModelCfg.ModelName,
		FallbackModelCfg:  fallbackModelCfg,
		ChunkSize:         envInt("CHUNK_SIZE", DefaultChunkSize, 1),
		OverlapPercent:    envInt("CHUNK_OVERLAP_PERCENT", DefaultOverlapPercent, 0),
		PrevOverlap:       prevOverlap,
		NextOverlap:       nextOverlap,
		RemoveTOC:         removeTOC,
		ArtifactDir:       strings.TrimSpace(os.Getenv("ARTIFACT_DIR")),
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

	// if ShouldSkipLineFileGeneratedEvent(evt) {
	// 	p.Logger.Warn("generate scene blocks - skipped")
	// 	return nil
	// }

	if p.PromptErr != nil {
		p.Logger.Error("prompt error", "prompt_name", p.PromptRef, "error", p.PromptErr)
		return fmt.Errorf("(MID_26051803) load scene blocks prompt %q: %w", p.PromptRef, p.PromptErr)
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
	if p.ModelErr != nil {
		p.Logger.Warn("scene blocks extraction skipped: model config error",
			"record_id", evt.RecordID, "model_ref", p.ModelRef, "error", p.ModelErr)
		p.persistSceneBlocksStatus(ctx, rec, start, p.ModelErr)
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

	eventID := eventIDFromContext(ctx)
	var allSceneBlocks []map[string]any
	seqno := 1
	for idx, chunk := range chunks {
		chunkText := buildSceneBlocksChunkText(chunk)
		p.Logger.Info("Begin generating scene",
			"idx", idx,
			"model name", p.ModelName,
			"prompt name", p.PromptRef,
			"chunk", chunkText)
		payload, usedModelName, err := p.extractSceneBlocksWithFallback(ctx, chunkText)
		if err != nil {
			procErr := fmt.Errorf("(MID_26051809) extract scene blocks via llm for chunk %d: %w", chunk.SeqNo, err)
			p.persistSceneBlocksStatus(ctx, rec, start, procErr)
			p.Logger.Error("LLM failed",
				"chunk seqno", chunk.SeqNo,
				"error", err,
				"model name", usedModelName,
				"prompt name", p.PromptRef)
			return nil
		}
		raw, _ := payload["scene_blocks"].([]any)
		for block_idx, item := range raw {
			block, ok := item.(map[string]any)
			if !ok {
				continue
			}
			objectID := fmt.Sprintf("%d_%d", evt.RecordID, seqno)
			req := UpsertSceneObjectRequest{
				InputRecordID: evt.RecordID,
				ObjectID:      objectID,
				EventID:       eventID,
				SceneBlock:    block,
				ModelName:     strings.TrimSpace(usedModelName),
				PromptName:    strings.TrimSpace(p.PromptRef),
				ExtInfo:       buildSceneBlockExtInfo(block),
			}
			if err := p.Store.UpsertSceneObject(ctx, req); err != nil {
				procErr := fmt.Errorf("(MID_26051810) upsert scene object %s: %w", objectID, err)
				p.persistSceneBlocksStatus(ctx, rec, start, procErr)
				p.Logger.Error("failed updating db",
					"error", err,
					"seqno", chunk.SeqNo,
					"block", block_idx,
					"model name", usedModelName,
					"prompt name", p.PromptRef)
				return nil
			}
			block["object_id"] = objectID
			allSceneBlocks = append(allSceneBlocks, block)
			seqno++
		}
		p.Logger.Info("LLM responded",
			"dx", idx,
			"num blocks", len(raw))
	}

	if err := p.writeSceneBlocksArtifact(evt.RecordID, rec, allSceneBlocks); err != nil {
		p.Logger.Error("write scene blocks artifact error", "error", err, "record_id", evt.RecordID)
		p.persistSceneBlocksStatus(ctx, rec, start, err)
		return nil
	}

	p.Logger.Info("scene blocks extracted",
		"record_id", evt.RecordID,
		"scene_blocks_count", len(allSceneBlocks),
		"num_chunks", len(chunks),
		"model_name", p.ModelName,
		"prompt_name", p.PromptRef)

	p.persistSceneBlocksStatus(ctx, rec, start, nil)
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

func buildSceneBlocksChunkText(chunk Chunk) string {
	return buildMarkedChunkInputText(chunk.Lines)
}

func (p *SceneBlocksProcessor) extractSceneBlocksWithFallback(ctx context.Context, inputText string) (map[string]any, string, error) {
	payload, err := p.extractSceneBlocksWithModel(ctx, inputText, p.ModelName, p.ModelCfg)
	if err == nil {
		return payload, strings.TrimSpace(p.ModelName), nil
	}

	primaryModelName := strings.TrimSpace(p.ModelName)
	fallbackModelName := strings.TrimSpace(p.FallbackModelName)
	if fallbackModelName == "" {
		return nil, primaryModelName, err
	}
	if p.FallbackModelErr != nil {
		return nil, fallbackModelName, fmt.Errorf("(MID_26051825) primary extraction failed and fallback model %q is unavailable: %w", p.FallbackModelRef, err)
	}

	if isEmptySceneBlocksExtractionError(err) {
		p.Logger.Warn("primary scene blocks extraction returned empty JSON; retrying fallback model",
			"primary_model", primaryModelName,
			"fallback_model", fallbackModelName,
			"error", err,
			"prompt_name", p.PromptRef,
		)
	} else {
		p.Logger.Warn("primary scene blocks extraction failed; retrying fallback model",
			"primary_model", primaryModelName,
			"fallback_model", fallbackModelName,
			"error", err,
			"prompt_name", p.PromptRef,
		)
	}

	payload, fallbackErr := p.extractSceneBlocksWithModel(ctx, inputText, fallbackModelName, p.FallbackModelCfg)
	if fallbackErr != nil {
		if isEmptySceneBlocksExtractionError(fallbackErr) {
			p.Logger.Warn("fallback scene blocks extraction returned empty JSON; treating as empty result",
				"fallback_model", fallbackModelName,
				"error", fallbackErr,
				"prompt_name", p.PromptRef,
			)
			return map[string]any{"scene_blocks": []any{}}, fallbackModelName, nil
		}
		return nil, fallbackModelName, fmt.Errorf("(MID_26051826) primary extraction failed: %w; fallback extraction failed: %v", err, fallbackErr)
	}
	return payload, fallbackModelName, nil
}

func (p *SceneBlocksProcessor) extractSceneBlocksWithModel(ctx context.Context, inputText string, modelName string, cfg structureModelConfig) (map[string]any, error) {
	applyStructureModelConfigToExtractor(p.Extractor, cfg)
	return p.Extractor.ExtractJSON(ctx, llmclients.JSONExtractionInput{
		PromptText: p.PromptText,
		ModelName:  modelName,
		InputText:  inputText,
	})
}

func isEmptySceneBlocksExtractionError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.TrimSpace(err.Error())
	if msg == "" {
		return false
	}
	return strings.Contains(msg, "unexpected end of JSON input") &&
		strings.Contains(msg, "json:{[]}")
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
	statusRaw, err := appendSceneBlocksStatus(rec.StatusRaw, sceneBlocksStatusParams{
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
		p.Logger.Error("failed building scene blocks status", "record_id", rec.ID, "error", err)
		return
	}
	if err := p.InputStore.UpdateInputMetadata(ctx, rec.ID, DocMetadataUpdate{
		StatusRaw: statusRaw,
		ErrorMsg:  errMsg,
	}); err != nil {
		p.Logger.Error("failed persisting scene blocks status", "record_id", rec.ID, "error", err)
	}
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

func loadSceneBlocksPromptFromEnv() (promptText string, promptRef string, promptErr error) {
	promptRef = strings.TrimSpace(os.Getenv("EXTRACT_SCENE_BLOCKS_PROMPT"))
	if promptRef == "" {
		promptRef = "prompt-generate-scene-blocks-v2.md"
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
