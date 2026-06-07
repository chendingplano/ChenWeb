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
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

// knowledgeTypeOrder is the canonical order used when flattening enriched chunk output.
var knowledgeTypeOrder = []string{
	"entity",
	"concept",
	"relationship",
	"normative_statement",
	"quantitative_constraint",
	"temporal_constraint",
	"conditional_logic",
	"causal_relationship",
	"assumption",
	"reference",
	"procedure",
	"ambiguity",
}

// knowledgeTypeArrayKeys maps knowledge type → JSON array key in LLM output.
var knowledgeTypeArrayKeys = map[string]string{
	"entity":                  "entities",
	"concept":                 "concepts",
	"relationship":            "relationships",
	"normative_statement":     "normative_statements",
	"quantitative_constraint": "quantitative_constraints",
	"temporal_constraint":     "temporal_constraints",
	"conditional_logic":       "conditional_logic",
	"causal_relationship":     "causal_relationships",
	"assumption":              "assumptions",
	"reference":               "references",
	"procedure":               "procedures",
	"ambiguity":               "ambiguities",
}

// knowledgeTypePrimaryFields maps knowledge type → primary field name in each item.
var knowledgeTypePrimaryFields = map[string]string{
	"entity":                  "entity",
	"concept":                 "concept",
	"relationship":            "relation",
	"normative_statement":     "normative_stmt",
	"quantitative_constraint": "quantitative_constraint",
	"temporal_constraint":     "temporal_constraint",
	"conditional_logic":       "conditiona_logic",
	"causal_relationship":     "causal_relation",
	"assumption":              "assumption",
	"reference":               "reference",
	"procedure":               "procedure",
	"ambiguity":               "ambiguity",
}

type StructuredKnowledgeProcessor struct {
	InputStore            DocMetadataStore
	Store                 StructuredKnowledgeStore
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
}

type StructuredKnowledgeStore interface {
	StructuredKnowledgesExist(ctx context.Context, inputRecordID int64) (bool, error)
	DeleteStructuredKnowledgesByInputRecordID(ctx context.Context, inputRecordID int64) (int64, error)
	SaveStructuredKnowledges(ctx context.Context, req SaveStructuredKnowledgeRequest) (int64, error)
}

type StructuredKnowledgeSQLStore struct {
	DB *sql.DB
}

type SaveStructuredKnowledgeRequest struct {
	InputRecordID int64
	EventID       string
	Language      string
	ModelName     string
	PromptName    string
	Knowledges    []map[string]any
}

type structuredKnowledgeExtractionResult struct {
	Knowledges    []map[string]any
	Language      string
	ModelName     string
	LLMCallCount  int
	FallbackCount int
	FailedChunks  int
}

type structuredKnowledgeCandidate struct {
	ChunkSeqNo int
	Payload    map[string]any
	ChunkLines []MarkedLine
}

func NewStructuredKnowledgeProcessor(
	inputStore DocMetadataStore,
	store StructuredKnowledgeStore,
	extractor LLMJSONExtractor,
	_ ApiTypes.JimoLogger,
) *StructuredKnowledgeProcessor {
	// if logger == nil {
	logger := loggerutil.CreateDefaultLogger("MID_26052601")
	// }
	candidatePromptText, candidatePromptRef, candidatePromptPath, candidatePromptErr := loadProductPromptFromEnvKeys(
		[]string{"EXTRACT_STRUCTURED_KNOWLEDGE_PROMPT"},
		"prompt-extract-structured-knowledge-v1.md",
	)
	enrichPromptText, enrichPromptRef, enrichPromptPath, enrichPromptErr := loadProductPromptFromEnvKeys(
		[]string{"ENRICH_STRUCTURED_KNOWLEDGE_PROMPT"},
		"prompt-enrich-structured-knowledge-v1.md",
	)
	candidateModelRef, candidateModelCfgPath, candidateModelCfg, candidateModelErr := loadModelConfigFromEnvKeys(
		[]string{"EXTRACT_STRUCTURED_KNOWLEDGE_MODEL_NAME"},
		"MODEL_DEF_FILE",
	)
	fallbackModelRef, fallbackModelCfgPath, fallbackModelCfg, fallbackModelErr := loadOptionalModelConfigFromEnv(
		"EXTRACT_STRUCTURED_KNOWLEDGE_MODEL_FALLBACK",
		"MODEL_DEF_FILE",
	)
	enrichModelRef, enrichModelCfgPath, enrichModelCfg, enrichModelErr := loadModelConfigFromEnvKeys(
		[]string{"ENRICH_STRUCTURED_KNOWLEDGE_MODEL_NAME"},
		"MODEL_DEF_FILE",
	)
	applyStructureModelConfigToExtractor(extractor, enrichModelCfg)
	return &StructuredKnowledgeProcessor{
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
	}
}

func (p *StructuredKnowledgeProcessor) Name() string { return "extract_structured_knowledge" }

func (p *StructuredKnowledgeProcessor) HandleEvent(ctx context.Context, payload []byte) error {
	start := p.Now()
	evt, err := ParseLineFileGeneratedEvent(payload)
	if err != nil {
		p.Logger.Error("parse input file failed", "error", err)
		return fmt.Errorf("(MID_26052602) parse event payload: %w", err)
	}
	if ShouldSkipLineFileGeneratedEvent(evt) {
		p.Logger.Warn("structured knowledge processor skipped")
		return nil
	}
	if p.CandidatePromptErr != nil {
		p.Logger.Error("candidate prompt error", "error", p.CandidatePromptErr)
		return fmt.Errorf("(MID_26052603) load structured knowledge candidate prompt %q: %w", p.CandidatePromptRef, p.CandidatePromptErr)
	}
	if p.EnrichPromptErr != nil {
		p.Logger.Error("enrich prompt error", "error", p.EnrichPromptErr)
		return fmt.Errorf("(MID_26052604) load structured knowledge enrich prompt %q: %w", p.EnrichPromptRef, p.EnrichPromptErr)
	}

	rec, err := p.InputStore.GetInputRecord(ctx, evt.RecordID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			p.Logger.Error("kb.inputs record not found", "record_id", evt.RecordID)
			return nil
		}
		p.Logger.Error("retrieve record failed", "record_id", evt.RecordID, "error", err)
		return fmt.Errorf("(MID_26052605) load kb.inputs record %d: %w", evt.RecordID, err)
	}
	if p.CandidateModelErr != nil {
		p.Logger.Warn("structured knowledge extraction skipped: candidate model config error",
			"record_id", evt.RecordID, "model_ref", p.CandidateModelRef, "error", p.CandidateModelErr)
		p.persistStructuredKnowledgeStatus(ctx, rec, start, p.CandidateModelErr)
		return nil
	}
	if p.EnrichModelErr != nil {
		p.Logger.Warn("structured knowledge extraction skipped: enrich model config error",
			"record_id", evt.RecordID, "model_ref", p.EnrichModelRef, "error", p.EnrichModelErr)
		p.persistStructuredKnowledgeStatus(ctx, rec, start, p.EnrichModelErr)
		return nil
	}

	lineFilePath, lineFileErr := ResolveInputFilePath(evt, rec.ResultFilename, rec.ParserName, rec.StagingFilename)
	if lineFileErr != nil {
		p.Logger.Error("resolve input file path failed", "record_id", evt.RecordID, "error", lineFileErr)
		p.persistStructuredKnowledgeStatus(ctx, rec, start, fmt.Errorf("(MID_26052606) resolve line file for record_id=%d: %w", evt.RecordID, lineFileErr))
		return nil
	}

	if evt.Force {
		_, _ = p.Store.DeleteStructuredKnowledgesByInputRecordID(ctx, evt.RecordID)
	} else {
		exists, checkErr := p.Store.StructuredKnowledgesExist(ctx, evt.RecordID)
		if checkErr != nil {
			p.Logger.Error("check structured knowledges exist failed", "record_id", evt.RecordID, "error", checkErr)
			p.persistStructuredKnowledgeStatus(ctx, rec, start, checkErr)
			return fmt.Errorf("(MID_26052607) check structured knowledges for record_id=%d: %w", evt.RecordID, checkErr)
		}
		if exists {
			p.Logger.Info("structured knowledge extraction skipped",
				"record_id", evt.RecordID, "reason", "structured knowledges already exist and force=false")
			reindexExistingSearchOnSkip(ctx, searchArtifactKnowledge, evt.RecordID, p.Logger, ReindexKnowledgeSearchForRecord)
			p.persistStructuredKnowledgeStatus(ctx, rec, start, nil)
			return nil
		}
	}

	body, readErr := os.ReadFile(lineFilePath)
	if readErr != nil {
		p.Logger.Error("read line file failed", "record_id", evt.RecordID, "path", lineFilePath, "error", readErr)
		p.persistStructuredKnowledgeStatus(ctx, rec, start, fmt.Errorf("(MID_26052608) read line file for record_id=%d: %w", evt.RecordID, readErr))
		return fmt.Errorf("(MID_26052609) failed reading line file, error:%w, path:%s", readErr, lineFilePath)
	}
	lines, parseErr := ParseInputLinesIncludingTOC(body)
	if parseErr != nil {
		p.Logger.Error("parse input lines failed", "record_id", evt.RecordID, "error", parseErr)
		p.persistStructuredKnowledgeStatus(ctx, rec, start, fmt.Errorf("(MID_26052610) parse input lines for record_id=%d: %w", evt.RecordID, parseErr))
		return fmt.Errorf("(MID_26052611) failed parsing input lines, error:%w", parseErr)
	}
	artifactBase := buildChunkArtifactBaseName(rec.StagingFilename, rec.ParserName)
	chunks, loadErr := loadChunksFromArtifactFile(p.ArtifactDir, evt.RecordID, artifactBase+".chunks", lines)
	if loadErr != nil {
		p.Logger.Error("load chunk artifact failed", "record_id", evt.RecordID, "artifact_base", artifactBase, "error", loadErr)
		p.persistStructuredKnowledgeStatus(ctx, rec, start, fmt.Errorf("(MID_26052612) load chunk artifact for record_id=%d: %w", evt.RecordID, loadErr))
		return fmt.Errorf("(MID_26052613) failed loading chunk artifact, error:%w", loadErr)
	}
	if len(chunks) == 0 {
		procErr := fmt.Errorf("(MID_26052614) no chunks found for record_id=%d", evt.RecordID)
		p.Logger.Error("no chunks", "record_id", evt.RecordID, "artifact_base", artifactBase)
		p.persistStructuredKnowledgeStatus(ctx, rec, start, procErr)
		return nil
	}

	result, err := p.extractStructuredKnowledgeFromChunks(ctx, evt.RecordID, chunks)
	if err != nil {
		if errors.Is(err, ErrPipelineStopped) {
			p.stopAndPersistStructuredKnowledge(context.Background(), rec, start)
			return ErrPipelineStopped
		}
		p.persistStructuredKnowledgeStatus(ctx, rec, start, err)
		return fmt.Errorf("(MID_26052615) extractStructuredKnowledge failed, error:%w", err)
	}

	detectedLanguage := result.Language
	if detectedLanguage == "" {
		detectedLanguage = "unknown"
	}
	createTime := p.Now().Format(defaultDocMetaStatusTime)
	for i, k := range result.Knowledges {
		k["create_time"] = createTime
		result.Knowledges[i] = k
	}

	inserted, err := p.Store.SaveStructuredKnowledges(ctx, SaveStructuredKnowledgeRequest{
		InputRecordID: evt.RecordID,
		EventID:       eventIDFromContext(ctx),
		Language:      detectedLanguage,
		ModelName:     firstNonEmptyTrimmed(result.ModelName, p.EnrichModelName),
		PromptName:    p.EnrichPromptRef,
		Knowledges:    result.Knowledges,
	})
	if err != nil {
		p.persistStructuredKnowledgeStatus(ctx, rec, start, err)
		return fmt.Errorf("(MID_26052616) insert structured knowledges failed, error:%w", err)
	}

	if fileErr := p.saveKnowledgesToFile(evt.RecordID, rec, result.Knowledges); fileErr != nil {
		p.Logger.Warn("save structured knowledges to file failed", "record_id", evt.RecordID, "error", fileErr)
	}

	if indexErr := p.indexKnowledges(evt.RecordID, result.Knowledges); indexErr != nil {
		p.Logger.Warn("index structured knowledges failed", "record_id", evt.RecordID, "error", indexErr)
	}

	if reindexErr := ReindexKnowledgeSearchForRecord(ctx, evt.RecordID, p.Logger); reindexErr != nil {
		p.Logger.Warn("reindex structured knowledge search registry failed", "record_id", evt.RecordID, "error", reindexErr)
	}

	p.Logger.Info("structured knowledges extracted",
		"record_id", evt.RecordID,
		"inserted_rows", inserted,
		"knowledges_count", len(result.Knowledges),
		"num_chunks", len(chunks),
	)
	p.persistStructuredKnowledgeStatus(ctx, rec, start, nil)
	p.logStructuredKnowledgeSummary(ctx, start, p.Now(), result, inserted, len(chunks), evt.RecordID)
	return nil
}

func (p *StructuredKnowledgeProcessor) extractStructuredKnowledgeFromChunks(
	ctx context.Context,
	recordID int64,
	chunks []Chunk,
) (structuredKnowledgeExtractionResult, error) {
	candidates := make([]structuredKnowledgeCandidate, 0, len(chunks))
	usedCandidateModel := strings.TrimSpace(p.CandidateModelName)
	var llmCallCount, fallbackCount, failedChunks int

	// Pass 1: extract structured knowledge candidates from each chunk
	for idx, chunk := range chunks {
		if isCtxStopped(ctx) {
			return structuredKnowledgeExtractionResult{
				LLMCallCount: llmCallCount, FallbackCount: fallbackCount, FailedChunks: failedChunks,
			}, ErrPipelineStopped
		}
		chunkText := buildMarkedChunkInputText(chunk.Lines)
		callStart := p.Now()
		p.Logger.Info("structknow start",
			"record_id", recordID,
			"chunk_idx", idx,
			"seq_no", chunk.SeqNo,
			"model_name", p.CandidateModelName,
			"prompt", p.CandidatePromptRef,
		)
		callID := fmt.Sprintf("%s_p1_c%d", eventIDFromContext(ctx), idx)
		userPrompt := buildKnowledgeCandidateUserPrompt(chunkText)
		payload, modelName, err := p.extractKnowledgeCandidateWithFallback(ctx, userPrompt)
		llmCallCount++
		if strings.TrimSpace(modelName) != strings.TrimSpace(p.CandidateModelName) && strings.TrimSpace(modelName) != "" {
			fallbackCount++
		}
		p.logLLMCall(ctx, callID, "extract_knowledge_candidates", 1,
			[]string{strings.TrimSpace(modelName)}, p.CandidatePromptRef,
			payload, err, callStart, p.Now(),
			recordID, idx, len(chunks))
		if err != nil {
			failedChunks++
			p.Logger.Error("raw parsed LLM payload/error",
				"record_id", recordID,
				"chunk_idx", idx,
				"seq_no", chunk.SeqNo,
				"error", err,
			)
			// Both primary and fallback failed — log and continue per spec
			continue
		}
		usedCandidateModel = strings.TrimSpace(firstNonEmptyTrimmed(modelName, usedCandidateModel))
		if isEmptyKnowledgePayload(payload) {
			p.Logger.Info("empty knowledge candidate; skipping chunk",
				"record_id", recordID, "seq_no", chunk.SeqNo)
			continue
		}
		p.Logger.Info("structknow end  ",
			"record_id", recordID,
			"chunk_idx", idx,
			"seq_no", chunk.SeqNo,
			"num_lines", len(chunk.Lines),
			"ms_used", time.Since(callStart).Milliseconds(),
		)
		candidates = append(candidates, structuredKnowledgeCandidate{
			ChunkSeqNo: chunk.SeqNo,
			Payload:    payload,
			ChunkLines: append([]MarkedLine(nil), chunk.Lines...),
		})
	}

	// Pass 2: enrich each candidate into final structured knowledge
	knowledges := make([]map[string]any, 0)
	detectedLanguage := "unknown"
	usedEnrichModel := strings.TrimSpace(p.EnrichModelName)
	globalSeqNo := 1
	for enrichIdx, cand := range candidates {
		if isCtxStopped(ctx) {
			return structuredKnowledgeExtractionResult{
				LLMCallCount: llmCallCount, FallbackCount: fallbackCount, FailedChunks: failedChunks,
			}, ErrPipelineStopped
		}
		callStart := p.Now()
		p.Logger.Info("enrich start",
			"record_id", recordID,
			"chunk_seq_no", cand.ChunkSeqNo,
			"model_name", p.EnrichModelName,
			"prompt", p.EnrichPromptRef,
		)
		callID := fmt.Sprintf("%s_p2_c%d", eventIDFromContext(ctx), enrichIdx)
		userPrompt := buildKnowledgeEnrichUserPrompt(cand)
		enriched, err := p.extractKnowledgeEnrichPayload(ctx, userPrompt)
		llmCallCount++
		p.logLLMCall(ctx, callID, "enrich_structured_knowledge", 2,
			[]string{strings.TrimSpace(p.EnrichModelName)}, p.EnrichPromptRef,
			enriched, err, callStart, p.Now(),
			recordID, enrichIdx, len(candidates))
		if err != nil {
			p.Logger.Error("enrichment-pass error",
				"record_id", recordID,
				"chunk_seq_no", cand.ChunkSeqNo,
				"error", err,
			)
			return structuredKnowledgeExtractionResult{
				LLMCallCount: llmCallCount, FallbackCount: fallbackCount, FailedChunks: failedChunks,
			}, fmt.Errorf("(MID_26052620) enrich structured knowledge chunk_seq=%d: %w", cand.ChunkSeqNo, err)
		}
		if lang := strings.TrimSpace(asString(enriched["language"])); lang != "" && detectedLanguage == "unknown" {
			detectedLanguage = lang
		}
		p.Logger.Info("enrich done ",
			"record_id", recordID,
			"chunk_seq_no", cand.ChunkSeqNo,
			"language", asString(enriched["language"]),
			"ms_used", time.Since(callStart).Milliseconds(),
		)

		items := flattenKnowledgeItems(recordID, enriched, cand.ChunkSeqNo, &globalSeqNo)
		knowledges = append(knowledges, items...)
	}

	p.Logger.Info("structured knowledges final results",
		"record_id", recordID,
		"total_chunks", len(chunks),
		"candidates", len(candidates),
		"knowledges", len(knowledges),
		"language", detectedLanguage,
	)
	return structuredKnowledgeExtractionResult{
		Knowledges:    knowledges,
		Language:      detectedLanguage,
		ModelName:     firstNonEmptyTrimmed(usedEnrichModel, usedCandidateModel, p.EnrichModelName),
		LLMCallCount:  llmCallCount,
		FallbackCount: fallbackCount,
		FailedChunks:  failedChunks,
	}, nil
}

func (p *StructuredKnowledgeProcessor) logLLMCall(
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
	if totalItems > 0 {
		if pass == 1 {
			percent = (itemIdx + 1) * 100 / totalItems / 2
		} else {
			percent = (itemIdx+1)*100/totalItems/2 + 50
		}
	}
	passLabel := fmt.Sprintf("pass %d", pass)
	progress := fmt.Sprintf("%d%% (%s: %d/%d)", percent, passLabel, itemIdx+1, totalItems)
	extraInfo := map[string]any{
		"chunk":        itemIdx + 1,
		"total_chunks": totalItems,
		"percent":      progress,
	}
	extraJSON, _ := json.Marshal(extraInfo)
	extraStr := string(extraJSON)
	callReason := "extract structured knowledge"
	if pass == 2 {
		callReason = "enrich structured knowledge"
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
	logFn := p.ProcLogger.LogExtractStructuredKnowledge
	if pass == 2 {
		logFn = p.ProcLogger.LogEnrichStructuredKnowledge
	}
	if err := logFn(ctx, rec, "MID-26060803"); err != nil {
		p.Logger.Warn("failed to write llm_call log", "call_id", callID, "error", err)
	}
}

func (p *StructuredKnowledgeProcessor) logStructuredKnowledgeSummary(
	ctx context.Context,
	start, end time.Time,
	result structuredKnowledgeExtractionResult,
	inserted int64,
	numChunks int,
	recordID int64,
) {
	extraInfo := map[string]any{
		"total_items":      inserted,
		"candidates_count": len(result.Knowledges),
		"llm_call_count":   result.LLMCallCount,
		"failed_chunks":    result.FailedChunks,
		"num_chunks":       numChunks,
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

	progress := "100%"
	activityName := "extract_structured_knowledge"
	promptName := firstNonEmptyTrimmed(p.CandidatePromptRef, p.EnrichPromptRef)
	rec := DocProcLogRecord{
		CallReason:    "extract structured knowledge",
		DocProcName:   p.Name(),
		ModelNames:    modelNames,
		PromptName:    promptName,
		RecordID:      &recordID,
		ProcProgress:  &progress,
		ActivityName:  &activityName,
		ExtraInfoJSON: &extraStr,
		MSUsed:        int64Ptr(end.Sub(start).Milliseconds()),
	}
	if err := p.ProcLogger.LogSummary(ctx, "extract_structured_knowledge", rec, "MID-26060804"); err != nil {
		p.Logger.Warn("failed to write doc_proc_summary log", "error", err)
	}
}

// flattenKnowledgeItems converts an enriched chunk payload into individual knowledge item maps.
// globalSeqNo is incremented for each item produced.
func flattenKnowledgeItems(recordID int64, enriched map[string]any, chunkSeqNo int, globalSeqNo *int) []map[string]any {
	items := make([]map[string]any, 0)
	language := strings.TrimSpace(asString(enriched["language"]))
	isEnglish := strings.EqualFold(language, "en") || strings.EqualFold(language, "english")
	categoryPaths := enriched["category_paths"]
	categoryPathsEn := enriched["category_paths_en"]

	for _, ktype := range knowledgeTypeOrder {
		arrayKey := knowledgeTypeArrayKeys[ktype]
		primaryField := knowledgeTypePrimaryFields[ktype]
		enArrayKey := arrayKey + "_en"

		rawArr, ok := enriched[arrayKey]
		if !ok {
			continue
		}
		arr, ok := rawArr.([]any)
		if !ok || len(arr) == 0 {
			continue
		}

		var enArr []any
		if !isEnglish {
			if rawEnArr, ok2 := enriched[enArrayKey]; ok2 {
				enArr, _ = rawEnArr.([]any)
			}
		}

		for i, rawItem := range arr {
			item, ok := rawItem.(map[string]any)
			if !ok {
				continue
			}
			primaryValue := strings.TrimSpace(asString(item[primaryField]))
			if primaryValue == "" {
				continue
			}

			knowledge := map[string]any{
				"knowledge_id":    fmt.Sprintf("%d_knw_%d", recordID, *globalSeqNo),
				"knowledge_type":  ktype,
				"knowledge_value": primaryValue,
				"desc_text":       strings.TrimSpace(asString(item["desc"])),
				"keywords":        toStringSlice(item["keywords"]),
				"lines":           item["lines"],
				"language":        language,
				"category_paths":  categoryPaths,
				"chunk_seq_no":    chunkSeqNo,
			}

			if !isEnglish && i < len(enArr) {
				if enItem, ok := enArr[i].(map[string]any); ok {
					knowledge["knowledge_value_en"] = strings.TrimSpace(asString(enItem[primaryField]))
					knowledge["desc_text_en"] = strings.TrimSpace(asString(enItem["desc"]))
					knowledge["keywords_en"] = toStringSlice(enItem["keywords"])
				}
				knowledge["category_paths_en"] = categoryPathsEn
			}

			items = append(items, knowledge)
			*globalSeqNo++
		}
	}
	return items
}

// ---- Pass 1 extraction ----

func (p *StructuredKnowledgeProcessor) extractKnowledgeCandidateWithFallback(ctx context.Context, inputText string) (map[string]any, string, error) {
	payload, err := p.extractKnowledgePayloadWithContract(ctx, inputText, p.CandidatePromptText, p.CandidateModelName, p.CandidateModelCfg, structuredKnowledgeCandidateContract())
	if err == nil {
		return payload, strings.TrimSpace(p.CandidateModelName), nil
	}

	primaryModelName := strings.TrimSpace(p.CandidateModelName)
	fallbackModelName := strings.TrimSpace(p.FallbackModelName)
	if fallbackModelName == "" {
		return nil, primaryModelName, err
	}
	if p.FallbackModelErr != nil {
		return nil, fallbackModelName, fmt.Errorf("(MID_26052630) primary candidate extraction failed and fallback model %q is unavailable: %w", p.FallbackModelRef, err)
	}

	p.Logger.Warn("primary structured knowledge candidate extraction failed; retrying fallback model",
		"primary_model", primaryModelName,
		"fallback_model", fallbackModelName,
		"error", err,
		"prompt_name", p.CandidatePromptRef,
	)

	fallbackPayload, fallbackErr := p.extractKnowledgePayloadWithContract(ctx, inputText, p.CandidatePromptText, fallbackModelName, p.FallbackModelCfg, structuredKnowledgeCandidateContract())
	if fallbackErr != nil {
		if isEmptyKnowledgeError(fallbackErr) {
			p.Logger.Info("fallback candidate extraction returned empty JSON; treating as empty result",
				"fallback_model", fallbackModelName)
			return nil, fallbackModelName, fmt.Errorf("(MID_26052631) primary extraction failed: %w; fallback returned empty JSON: %v", err, fallbackErr)
		}
		return nil, fallbackModelName, fmt.Errorf("(MID_26052632) primary extraction failed: %w; fallback extraction failed: %v", err, fallbackErr)
	}
	return fallbackPayload, fallbackModelName, nil
}

// ---- Pass 2 extraction ----

func (p *StructuredKnowledgeProcessor) extractKnowledgeEnrichPayload(ctx context.Context, inputText string) (map[string]any, error) {
	payload, err := p.extractKnowledgePayloadWithContract(ctx, inputText, p.EnrichPromptText, p.EnrichModelName, p.EnrichModelCfg, structuredKnowledgeEnrichContract())
	if err != nil {
		return nil, fmt.Errorf("(MID_26052633) enrich structured knowledge: %w", err)
	}
	return payload, nil
}

func (p *StructuredKnowledgeProcessor) extractKnowledgePayloadWithContract(
	ctx context.Context,
	inputText string,
	promptText string,
	modelName string,
	cfg structureModelConfig,
	contract llmclients.StructuredOutputContract,
) (map[string]any, error) {
	applyStructureModelConfigToExtractor(p.Extractor, cfg)
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
		result, err = structuredExtractor.ExtractStructuredJSON(ctx, in, contract)
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
		return nil, errors.New("(MID_26052634) empty llm payload")
	}
	return payload, nil
}

func isEmptyKnowledgePayload(payload map[string]any) bool {
	if payload == nil {
		return true
	}
	for _, ktype := range knowledgeTypeOrder {
		arrayKey := knowledgeTypeArrayKeys[ktype]
		if raw, ok := payload[arrayKey]; ok {
			if arr, ok := raw.([]any); ok && len(arr) > 0 {
				return false
			}
		}
	}
	return true
}

func isEmptyKnowledgeError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.TrimSpace(err.Error())
	return (strings.Contains(msg, "unexpected end of JSON input") && strings.Contains(msg, "json:{[]}")) ||
		strings.Contains(msg, "(MID_26052634)")
}

// ---- Prompt builders ----

func buildKnowledgeCandidateUserPrompt(chunkText string) string {
	schema := map[string]any{
		"entities":                 knowledgeItemsArraySchema("entity"),
		"concepts":                 knowledgeItemsArraySchema("concept"),
		"relationships":            knowledgeItemsArraySchema("relation"),
		"normative_statements":     knowledgeItemsArraySchema("normative_stmt"),
		"quantitative_constraints": knowledgeItemsArraySchema("quantitative_constraint"),
		"temporal_constraints":     knowledgeItemsArraySchema("temporal_constraint"),
		"conditional_logic":        knowledgeItemsArraySchema("conditiona_logic"),
		"causal_relationships":     knowledgeItemsArraySchema("causal_relation"),
		"assumptions":              knowledgeItemsArraySchema("assumption"),
		"references":               knowledgeItemsArraySchema("reference"),
		"procedures":               knowledgeItemsArraySchema("procedure"),
		"ambiguities":              knowledgeItemsArraySchema("ambiguity"),
	}
	schemaJSON, _ := json.Marshal(schema)
	return "Return JSON only. Use exactly this top-level schema:\n" + string(schemaJSON) +
		"\n\nInput chunk lines:\n" + chunkText
}

func knowledgeItemsArraySchema(primaryField string) []map[string]any {
	return []map[string]any{{
		primaryField: "string",
		"desc":       "string",
		"keywords":   []string{"string"},
		"lines":      []any{"ddd | ddd-ddd"},
	}}
}

func buildKnowledgeEnrichUserPrompt(cand structuredKnowledgeCandidate) string {
	chunkText := buildMarkedChunkInputText(cand.ChunkLines)
	candidateJSON, _ := json.Marshal(cand.Payload)

	enrichSchema := map[string]any{
		"language":                    "string — detected language of the content",
		"entities":                    knowledgeItemsArraySchema("entity"),
		"entities_en":                 knowledgeItemsArraySchema("entity"),
		"concepts":                    knowledgeItemsArraySchema("concept"),
		"concepts_en":                 knowledgeItemsArraySchema("concept"),
		"relationships":               knowledgeItemsArraySchema("relation"),
		"relationships_en":            knowledgeItemsArraySchema("relation"),
		"normative_statements":        knowledgeItemsArraySchema("normative_stmt"),
		"normative_statements_en":     knowledgeItemsArraySchema("normative_stmt"),
		"quantitative_constraints":    knowledgeItemsArraySchema("quantitative_constraint"),
		"quantitative_constraints_en": knowledgeItemsArraySchema("quantitative_constraint"),
		"temporal_constraints":        knowledgeItemsArraySchema("temporal_constraint"),
		"temporal_constraints_en":     knowledgeItemsArraySchema("temporal_constraint"),
		"conditional_logic":           knowledgeItemsArraySchema("conditiona_logic"),
		"conditional_logic_en":        knowledgeItemsArraySchema("conditiona_logic"),
		"causal_relationships":        knowledgeItemsArraySchema("causal_relation"),
		"causal_relationships_en":     knowledgeItemsArraySchema("causal_relation"),
		"assumptions":                 knowledgeItemsArraySchema("assumption"),
		"assumptions_en":              knowledgeItemsArraySchema("assumption"),
		"references":                  knowledgeItemsArraySchema("reference"),
		"references_en":               knowledgeItemsArraySchema("reference"),
		"procedures":                  knowledgeItemsArraySchema("procedure"),
		"procedures_en":               knowledgeItemsArraySchema("procedure"),
		"ambiguities":                 knowledgeItemsArraySchema("ambiguity"),
		"ambiguities_en":              knowledgeItemsArraySchema("ambiguity"),
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
	schemaJSON, _ := json.Marshal(enrichSchema)
	return "Return JSON only. Use exactly this top-level schema:\n" + string(schemaJSON) +
		"\n\nStructured knowledge candidates:\n" + string(candidateJSON) +
		"\n\nSource chunk lines:\n" + chunkText
}

// ---- Save to file ----

func (p *StructuredKnowledgeProcessor) saveKnowledgesToFile(
	recordID int64,
	rec DocMetadataInputRecord,
	knowledges []map[string]any,
) error {
	if len(knowledges) == 0 {
		return nil
	}
	artifactDir := strings.TrimSpace(p.ArtifactDir)
	if artifactDir == "" {
		return fmt.Errorf("(MID_26052640) missing ARTIFACT_DIR")
	}
	stagingBase := filepath.Base(strings.TrimSpace(rec.StagingFilename))
	filenameRoot := strings.TrimSuffix(stagingBase, filepath.Ext(stagingBase))
	if filenameRoot == "" {
		return fmt.Errorf("(MID_26052641) cannot determine filename root from staging_filename %q", rec.StagingFilename)
	}
	parserName := strings.TrimSpace(rec.ParserName)
	if parserName == "" {
		return fmt.Errorf("(MID_26052642) parser_name is empty for record_id=%d", recordID)
	}
	groupID := recordID / 1000
	dir := filepath.Join(artifactDir, strconv.FormatInt(groupID, 10), strconv.FormatInt(recordID, 10))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("(MID_26052643) create directory %s: %w", dir, err)
	}
	path := filepath.Join(dir, filenameRoot+"_"+parserName+".knowledges")
	bs, err := json.MarshalIndent(knowledges, "", "  ")
	if err != nil {
		return fmt.Errorf("(MID_26052644) marshal structured knowledges: %w", err)
	}
	if err := os.WriteFile(path, bs, 0644); err != nil {
		return fmt.Errorf("(MID_26052645) write structured knowledges file %s: %w", path, err)
	}
	return nil
}

// ---- Index by category paths ----

func (p *StructuredKnowledgeProcessor) indexKnowledges(recordID int64, knowledges []map[string]any) error {
	dir := strings.TrimSpace(p.ArtifactWebDir)
	if dir == "" {
		return fmt.Errorf("(MID_26052650) missing ARTIFACT_WEB_DIR")
	}
	if recordID <= 0 {
		return fmt.Errorf("(MID_26052651) invalid record_id: %d", recordID)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("(MID_26052652) create artifact web dir: %w", err)
	}
	if err := removeKnowledgeTreeRecord(dir, recordID); err != nil {
		return fmt.Errorf("(MID_26052653) remove old knowledge tree entries for record %d: %w", recordID, err)
	}
	now := p.Now()
	for _, k := range knowledges {
		knowledgeID := strings.TrimSpace(asString(k["knowledge_id"]))
		if knowledgeID == "" {
			continue
		}
		for _, pair := range pairCategoryPathEntries(k["category_paths"], k["category_paths_en"]) {
			if err := writeKnowledgeTreeEntry(p.Logger, dir, knowledgeID, k, pair.Index, pair.Original, now); err != nil {
				return fmt.Errorf("(MID_26052654) index knowledge %s path: %w", knowledgeID, err)
			}
		}
	}
	return nil
}

func writeKnowledgeTreeEntry(
	logger ApiTypes.JimoLogger,
	treeRootDir string,
	knowledgeID string,
	knowledge map[string]any,
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
	return upsertKnowledgeToLeafDir(leafDir, knowledgeID, knowledge)
}

// upsertKnowledgeToLeafDir writes full knowledge JSON objects to knowledges.txt,
// keyed and de-duplicated by knowledge_id.
func upsertKnowledgeToLeafDir(leafDir string, knowledgeID string, knowledge map[string]any) error {
	filePath := filepath.Join(leafDir, "knowledges.txt")

	type entry struct {
		knowledgeID string
		line        string
	}
	existing := make([]entry, 0)
	if bs, err := os.ReadFile(filePath); err == nil {
		for line := range strings.SplitSeq(string(bs), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var obj map[string]any
			if jsonErr := json.Unmarshal([]byte(line), &obj); jsonErr == nil {
				kid := strings.TrimSpace(asString(obj["knowledge_id"]))
				existing = append(existing, entry{knowledgeID: kid, line: line})
			}
		}
	}

	newLine, err := json.Marshal(knowledge)
	if err != nil {
		return err
	}
	newEntry := entry{knowledgeID: knowledgeID, line: string(newLine)}

	replaced := false
	out := make([]entry, 0, len(existing)+1)
	for _, e := range existing {
		if e.knowledgeID == knowledgeID {
			if !replaced {
				out = append(out, newEntry)
				replaced = true
			}
		} else {
			out = append(out, e)
		}
	}
	if !replaced {
		out = append(out, newEntry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].knowledgeID < out[j].knowledgeID })

	lines := make([]string, 0, len(out))
	for _, e := range out {
		lines = append(lines, e.line)
	}
	return os.WriteFile(filePath, []byte(strings.Join(lines, "\n")), 0o644)
}

func removeKnowledgeTreeRecord(treeRootDir string, recordID int64) error {
	prefix := strconv.FormatInt(recordID, 10) + "_"
	return filepath.WalkDir(treeRootDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != "knowledges.txt" {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rows := make([]string, 0)
		for line := range strings.SplitSeq(string(body), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var obj map[string]any
			if jsonErr := json.Unmarshal([]byte(line), &obj); jsonErr == nil {
				kid := strings.TrimSpace(asString(obj["knowledge_id"]))
				if strings.HasPrefix(kid, prefix) {
					continue
				}
			}
			rows = append(rows, line)
		}
		if len(rows) == 0 {
			if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				return removeErr
			}
			return nil
		}
		return os.WriteFile(path, []byte(strings.Join(rows, "\n")), 0o644)
	})
}

// ---- Status ----

type structuredKnowledgeStatusParams struct {
	RecordID      int64
	FileType      string
	InputFilename string
	Start         time.Time
	DurationMs    int64
	ProcStatus    string
	ProcErr       error
}

func detectStructuredKnowledgeFileType(rec DocMetadataInputRecord) string {
	for _, candidate := range []string{rec.FileName, rec.StagingFilename, rec.ResultFilename} {
		ext := strings.ToLower(strings.TrimSpace(filepath.Ext(strings.TrimSpace(candidate))))
		if ext != "" {
			return strings.TrimPrefix(ext, ".")
		}
	}
	return ""
}

func (p *StructuredKnowledgeProcessor) persistStructuredKnowledgeStatus(
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
		statusRaw, err := appendStructuredKnowledgeStatus(current, structuredKnowledgeStatusParams{
			RecordID:      rec.ID,
			FileType:      detectStructuredKnowledgeFileType(rec),
			InputFilename: strings.TrimSpace(rec.ResultFilename),
			Start:         start,
			DurationMs:    time.Since(start).Milliseconds(),
			ProcErr:       procErr,
		})
		if err != nil {
			return DocMetadataUpdate{}, err
		}
		return DocMetadataUpdate{StatusRaw: statusRaw, ErrorMsg: errMsg}, nil
	}); updateErr != nil {
		p.Logger.Error("failed persisting structured knowledge status", "record_id", rec.ID, "error", updateErr)
	}
}

func (p *StructuredKnowledgeProcessor) stopAndPersistStructuredKnowledge(ctx context.Context, rec DocMetadataInputRecord, start time.Time) {
	if updateErr := updateInputStatusAtomic(ctx, p.InputStore, rec.ID, func(current string) (DocMetadataUpdate, error) {
		statusRaw, err := appendStructuredKnowledgeStatus(current, structuredKnowledgeStatusParams{
			RecordID:      rec.ID,
			FileType:      detectStructuredKnowledgeFileType(rec),
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
		p.Logger.Error("(MID_26052842) failed persisting structured knowledge stopped status", "record_id", rec.ID, "error", updateErr)
	}
	p.Logger.Info("(MID_26052843) extract_structured_knowledge stopped by user request", "record_id", rec.ID)
}

func appendStructuredKnowledgeStatus(raw string, p structuredKnowledgeStatusParams) (string, error) {
	entries := decodeDocMetaStatus(raw)
	entry := map[string]any{
		"record_id":      strconv.FormatInt(p.RecordID, 10),
		"file_type":      strings.ToLower(strings.TrimSpace(p.FileType)),
		"operation":      "extract_structured_knowledge",
		"input_filename": strings.TrimSpace(p.InputFilename),
		"start_time":     p.Start.Format(defaultDocMetaStatusTime),
		"ms_used":        p.DurationMs,
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
		if op != "extract_structured_knowledge" {
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

func (s StructuredKnowledgeSQLStore) ensureKnowledgesTable(ctx context.Context) error {
	if s.DB == nil {
		return fmt.Errorf("(MID_26052660) db is nil")
	}
	const ddl = `
CREATE SCHEMA IF NOT EXISTS kb;

CREATE TABLE IF NOT EXISTS kb.knowledges (
    id BIGSERIAL PRIMARY KEY,
    event_id TEXT,
    input_record_id BIGINT NOT NULL,
    knowledge_id TEXT,
    language TEXT,
    knowledge_type TEXT,
    knowledge_value TEXT,
    knowledge_value_en TEXT,
    desc_text TEXT,
    desc_text_en TEXT,
    keywords JSONB,
    keywords_en JSONB,
    lines JSONB,
    category_paths JSONB,
    category_paths_en JSONB,
    model_name TEXT,
    prompt_name TEXT,
    search_document TEXT,
    search_vector TSVECTOR,
    ext_info JSONB,
    create_time TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_kb_knowledges_input_record_id ON kb.knowledges (input_record_id);
CREATE INDEX IF NOT EXISTS idx_kb_knowledges_knowledge_id ON kb.knowledges (knowledge_id);
`
	_, err := s.DB.ExecContext(ctx, ddl)
	return err
}

func (s StructuredKnowledgeSQLStore) StructuredKnowledgesExist(ctx context.Context, inputRecordID int64) (bool, error) {
	if err := s.ensureKnowledgesTable(ctx); err != nil {
		return false, err
	}
	const q = `SELECT 1 FROM kb.knowledges WHERE input_record_id = $1 LIMIT 1`
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

func (s StructuredKnowledgeSQLStore) DeleteStructuredKnowledgesByInputRecordID(ctx context.Context, inputRecordID int64) (int64, error) {
	if err := s.ensureKnowledgesTable(ctx); err != nil {
		return 0, err
	}
	res, err := s.DB.ExecContext(ctx, `DELETE FROM kb.knowledges WHERE input_record_id = $1`, inputRecordID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s StructuredKnowledgeSQLStore) SaveStructuredKnowledges(ctx context.Context, req SaveStructuredKnowledgeRequest) (int64, error) {
	if err := s.ensureKnowledgesTable(ctx); err != nil {
		return 0, err
	}
	if len(req.Knowledges) == 0 {
		return 0, nil
	}

	const stmt = `
INSERT INTO kb.knowledges (
    event_id,
    input_record_id,
    knowledge_id,
    language,
    knowledge_type,
    knowledge_value,
    knowledge_value_en,
    desc_text,
    desc_text_en,
    keywords,
    keywords_en,
    lines,
    category_paths,
    category_paths_en,
    model_name,
    prompt_name,
    ext_info
) VALUES (
    $1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb,$11::jsonb,$12::jsonb,$13::jsonb,$14::jsonb,$15,$16,$17::jsonb
)`

	isEnglish := strings.EqualFold(strings.TrimSpace(req.Language), "en") ||
		strings.EqualFold(strings.TrimSpace(req.Language), "english")

	var eventIDVal any
	if id := strings.TrimSpace(req.EventID); id != "" {
		eventIDVal = id
	}

	var inserted int64
	for _, k := range req.Knowledges {
		extInfo, _ := json.Marshal(map[string]any{
			"language":       req.Language,
			"schema_version": "1",
			"chunk_seq_no":   k["chunk_seq_no"],
		})

		keywordsJSON, _ := json.Marshal(k["keywords"])
		linesJSON, _ := json.Marshal(k["lines"])
		categoryPathsJSON, _ := json.Marshal(k["category_paths"])

		var (
			knowledgeValueEnVal any
			descTextEnVal       any
			keywordsEnVal       any
			categoryPathsEnVal  any
		)
		if !isEnglish {
			knowledgeValueEnVal = strings.TrimSpace(asString(k["knowledge_value_en"]))
			descTextEnVal = strings.TrimSpace(asString(k["desc_text_en"]))
			kw, _ := json.Marshal(k["keywords_en"])
			keywordsEnVal = string(kw)
			cp, _ := json.Marshal(k["category_paths_en"])
			categoryPathsEnVal = string(cp)
		}

		_, err := s.DB.ExecContext(ctx, stmt,
			eventIDVal,
			req.InputRecordID,
			strings.TrimSpace(asString(k["knowledge_id"])),
			strings.TrimSpace(asString(k["language"])),
			strings.TrimSpace(asString(k["knowledge_type"])),
			strings.TrimSpace(asString(k["knowledge_value"])),
			knowledgeValueEnVal,
			strings.TrimSpace(asString(k["desc_text"])),
			descTextEnVal,
			string(keywordsJSON),
			keywordsEnVal,
			string(linesJSON),
			string(categoryPathsJSON),
			categoryPathsEnVal,
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
