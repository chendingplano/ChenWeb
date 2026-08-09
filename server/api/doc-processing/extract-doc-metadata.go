package docprocessing

import (
	"bufio"
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

	"github.com/chendingplano/deepdoc/server/api/ontology/semrules"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/ApiUtils"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

const defaultDocMetaStatusTime = "20060102 15:04:05"

type ExtractDocMetadataProcessor struct {
	Store DocMetadataStore
	// Facets is optional (nil-safe): when set, a successful extraction also
	// computes and persists tier-2 facets (spec 2026072901 S16.1 "Facet
	// tiers 1-2") from the just-extracted doc_no/publish_date, right where
	// they become known -- doc_metadata isn't available any earlier than
	// this processor's own successful update, so this is the natural
	// write point regardless of which resolver call later reads them back
	// (ApplicabilityResolver.enrichWithPersistedFacets).
	Facets       FacetObservationStore
	Client       LLMJSONExtractor
	Logger       ApiTypes.JimoLogger
	ProcLogger   DocProcLogger
	Now          func() time.Time
	InitialPages int
	PromptText   string
	PromptRef    string
	PromptPath   string
	PromptErr    error
	ModelRef     string
	ModelCfgPath string
	ModelErr     error
	ModelName    string
	ModelCfg     structureModelConfig

	FallbackModelRef     string
	FallbackModelCfgPath string
	FallbackModelErr     error
	FallbackModelName    string
	FallbackModelCfg     structureModelConfig
}

type LLMJSONExtractor interface {
	ExtractJSON(ctx context.Context, in llmclients.JSONExtractionInput) (map[string]any, error)
}

// LLMStructuredJSONExtractor is the preferred capability for JSON-producing
// doc-processing call sites. Implementations may still satisfy only
// LLMJSONExtractor during migration, but new code should prefer this path.
type LLMStructuredJSONExtractor interface {
	ExtractStructuredJSON(ctx context.Context, in llmclients.JSONExtractionInput, contract llmclients.StructuredOutputContract) (*llmclients.StructuredOutputResult, error)
}

func NewExtractDocMetadataProcessor(store DocMetadataStore, client LLMJSONExtractor, _ ApiTypes.JimoLogger) *ExtractDocMetadataProcessor {
	logger := loggerutil.CreateDefaultLogger("MID_26041830")
	promptText, promptRef, promptPath, promptErr := loadDocMetaPromptFromEnv()
	modelRef, modelCfgPath, modelCfg, modelErr := loadModelConfigFromEnv("EXTRACT_DOCMETA_MODEL_NAME", "EXTRACT_DOCMETA_MODELS_FILE")
	fallbackModelRef, fallbackModelCfgPath, fallbackModelCfg, fallbackModelErr := loadOptionalModelConfigFromEnv("EXTRACT_DOCMETA_MODEL_FALLBACK", "EXTRACT_DOCMETA_MODELS_FILE")
	applyStructureModelConfigToExtractor(client, modelCfg)
	return &ExtractDocMetadataProcessor{
		Store:        store,
		Facets:       SQLStore{DB: ApiTypes.ProjectDBHandle},
		Client:       client,
		Logger:       logger,
		ProcLogger:   DocProcLogger{DB: ApiTypes.ProjectDBHandle},
		Now:          time.Now,
		InitialPages: envInt("EXTRACT_DOCMETA_NUM_PAGES", 2, 1),
		PromptText:   promptText,
		PromptRef:    promptRef,
		PromptPath:   promptPath,
		PromptErr:    promptErr,
		ModelRef:     modelRef,
		ModelCfgPath: modelCfgPath,
		ModelErr:     modelErr,
		ModelName:    modelCfg.ModelName,
		ModelCfg:     modelCfg,

		FallbackModelRef:     fallbackModelRef,
		FallbackModelCfgPath: fallbackModelCfgPath,
		FallbackModelErr:     fallbackModelErr,
		FallbackModelName:    fallbackModelCfg.ModelName,
		FallbackModelCfg:     fallbackModelCfg,
	}
}

func (p *ExtractDocMetadataProcessor) Name() string { return "extract_doc_metadata" }

func (p *ExtractDocMetadataProcessor) HandleEvent(ctx context.Context, payload []byte) error {
	evt, err := ParseLineFileGeneratedEvent(payload)
	if err != nil {
		return fmt.Errorf("(MID_26042410) failed parsing event payload: %w", err)
	}
	ctx = withLLMRecordID(ctx, evt.RecordID)
	if ShouldSkipLineFileGeneratedEvent(evt) {
		p.Logger.Info("doc metadata event skipped", "record_id", evt.RecordID, "type", evt.Type, "status", evt.Status)
		return nil
	}

	rec, err := p.Store.GetInputRecord(ctx, evt.RecordID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			p.Logger.Error("kb.inputs record not found", "record_id", evt.RecordID)
			return nil
		}
		p.Logger.Error("failed loading kb.inputs record", "record_id", evt.RecordID, "error", err)
		return nil
	}

	if strings.TrimSpace(rec.ParserName) == "" {
		return p.failAndPersist(ctx, rec, errors.New("(MID-26060811) missing parser name"))
	}
	if strings.TrimSpace(rec.ResultFilename) == "" {
		return p.failAndPersist(ctx, rec, errors.New("(MID-26060812) missing result filename"))
	}
	if p.ModelErr != nil {
		return p.failAndPersist(ctx, rec, p.ModelErr)
	}

	inputPath, err := ResolveInputFilePath(evt, rec.ResultFilename, rec.ParserName, rec.StagingFilename)
	if err != nil {
		return p.failAndPersist(ctx, rec, err)
	}

	fi, err := os.Stat(inputPath)
	if err != nil {
		if os.IsNotExist(err) {
			return p.failAndPersist(ctx, rec, fmt.Errorf("(MID_26042411) input file not exist: %s", inputPath))
		}
		return p.failAndPersist(ctx, rec, fmt.Errorf("(MID_26042412) stat input file: %w", err))
	}
	if fi.Size() == 0 {
		return p.failAndPersist(ctx, rec, fmt.Errorf("(MID-26060810) input file empty, inputPath:%s", inputPath))
	}

	readStart := p.Now()
	linesByPage, maxPage, err := readLineFile(inputPath)
	if err != nil {
		return p.failAndPersist(ctx, rec, err)
	}
	p.Logger.Debug("file read complete",
		"record_id", rec.ID,
		"max_page", maxPage,
		"ms_used", p.Now().Sub(readStart).Milliseconds(),
	)

	start := p.Now()
	lastPage := p.InitialPages
	if lastPage < 1 {
		lastPage = 1
	}
	if maxPage > 0 && lastPage > maxPage {
		lastPage = maxPage
	}

	var llmCallCount int
	var fallbackUsed bool
	var numPagesUsed int
	var out docMetadataExtractionOutput
	for i := 0; i < 10; i++ {
		if p.PromptErr != nil {
			procErr := fmt.Errorf("(MID_26042413) load prompt file %q failed: %w", p.PromptRef, p.PromptErr)
			p.logDocMetaSummary(ctx, start, p.Now(), llmCallCount, fallbackUsed, numPagesUsed, maxPage, procErr)
			return p.failAndPersist(ctx, rec, procErr)
		}

		inputText := buildInputText(linesByPage, lastPage)
		p.Logger.Debug("llm call starting",
			"record_id", rec.ID,
			"pages_used", lastPage,
			"input_chars", len(inputText),
			"model", p.ModelName,
			"attempt", i,
		)
		callStart := p.Now()
		callID := fmt.Sprintf("%s_p1_i%d", eventIDFromContext(ctx), i)
		parsed, usedModelName, extractErr := p.extractMetadataWithFallback(ctx, inputText)
		p.Logger.Debug("llm call finished",
			"record_id", rec.ID,
			"model_used", usedModelName,
			"ms_used", p.Now().Sub(callStart).Milliseconds(),
			"error", extractErr,
		)
		llmCallCount++
		numPagesUsed = lastPage
		if strings.TrimSpace(usedModelName) != strings.TrimSpace(p.ModelName) && strings.TrimSpace(usedModelName) != "" {
			fallbackUsed = true
		}
		p.logLLMCall(ctx, callID, "extract_doc_metadata", 1,
			[]string{strings.TrimSpace(usedModelName)}, p.PromptRef,
			parsed, extractErr, callStart, p.Now())
		if extractErr != nil {
			procErr := fmt.Errorf("(MID_26042414) extract metadata via llm: %w, model name:%s, prompt file:%s", extractErr, usedModelName, p.PromptRef)
			p.logDocMetaSummary(ctx, start, p.Now(), llmCallCount, fallbackUsed, numPagesUsed, maxPage, procErr)
			return p.failAndPersist(ctx, rec, procErr)
		}
		out = parseDocMetadataOutput(parsed)
		if !out.NeedMorePages || maxPage == 0 || lastPage >= maxPage {
			break
		}
		lastPage += p.InitialPages
		if lastPage > maxPage {
			lastPage = maxPage
		}
	}

	statusRaw, statusErr := appendDocMetaStatus(rec.StatusRaw, start, time.Since(start).Milliseconds(), nil)
	if statusErr != nil {
		return fmt.Errorf("(MID_26042415) append status: %w", statusErr)
	}

	upd := DocMetadataUpdate{
		Title:       strings.TrimSpace(out.Title),
		DocNo:       strings.TrimSpace(out.DocNo),
		PublishDate: normalizePublishDateForColumn(out.PublishDate),
		Authors:     pickPreferredAuthors(out),
		DocMetadata: copyMetadata(out.Metadata),
		StatusRaw:   statusRaw,
		ErrorMsg:    nil,
	}
	if err := p.Store.UpdateInputMetadata(ctx, rec.ID, upd); err != nil {
		return fmt.Errorf("(MID_26042416) update kb.inputs metadata: %w", err)
	}
	if p.Facets != nil {
		// facetTier2GatedOff consults an authored kb.pipeline_rules row
		// (target_processor="facet_tier2") so the processor can be
		// individually disabled for debugging, testing, or bug fixing; a
		// gate-resolution error fails open (runs) rather than silently
		// dropping load-bearing facets over a misconfigured rule. See
		// facetTier1GatedOff (facet_tier1.go) for the full contract.
		skipTier2, gateErr := facetTier2GatedOff(semrules.FactSet{})
		if gateErr != nil {
			p.Logger.Warn("facet_tier2 gate resolution failed, running unconditionally", "record_id", rec.ID, "error", gateErr)
		}
		if skipTier2 {
			p.Logger.Info("facet_tier2 skipped by processor gate", "record_id", rec.ID)
		} else {
			// InsertFacetObservation requires a non-empty DecisionAttemptID/
			// InvocationID (validated before the SQL, doc_facet_store.go).
			// eventIDFromContext can be "" outside a real event (e.g. tests, or
			// a call with no event context), same reason resolverAttemptKey
			// (control.go) falls back to the record id.
			attemptKey := eventIDFromContext(ctx)
			if attemptKey == "" {
				attemptKey = fmt.Sprintf("record-%d", rec.ID)
			}
			for _, obs := range tier2FacetsFromSource(rec.ID, upd.DocNo, upd.PublishDate) {
				obs.DecisionAttemptID = "tier2-" + attemptKey
				obs.InvocationID = fmt.Sprintf("tier2-%d-%s", rec.ID, attemptKey)
				if _, err := p.Facets.InsertFacetObservation(ctx, obs); err != nil {
					p.Logger.Warn("tier-2 facet persist failed", "record_id", rec.ID, "path", obs.Path, "error", err)
				}
			}
		}
	}

	p.Logger.Info("metadata extracted",
		"record_id", rec.ID,
		"input_file", inputPath,
		"title", upd.Title,
		"doc_no", upd.DocNo,
		"authors", len(upd.Authors),
		"model", p.ModelName,
		"prompt", p.PromptRef,
		"ms_used", time.Since(start).Milliseconds(),
	)
	p.logDocMetaSummary(ctx, start, p.Now(), llmCallCount, fallbackUsed, numPagesUsed, maxPage, nil)
	return nil
}

func (p *ExtractDocMetadataProcessor) logLLMCall(
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
	cacheHit, cacheMiss := extractorCacheTokens(p.Client)
	rec := DocProcLogRecord{
		DocProcName:           p.Name(),
		ModelNames:            modelNames,
		PromptName:            promptName,
		Pass:                  &pass,
		LLMCallID:             &callID,
		ActivityName:          &activity,
		ArtifactJSON:          artifactStr,
		Errors:                errStr,
		MSUsed:                int64Ptr(end.Sub(start).Milliseconds()),
		PromptCacheHitTokens:  cacheHit,
		PromptCacheMissTokens: cacheMiss,
	}
	if err := p.ProcLogger.LogExtractDocMetadata(ctx, rec, "MID-26052805"); err != nil {
		p.Logger.Warn("failed to write extract_doc_metadata log", "call_id", callID, "error", err)
	}
}

func (p *ExtractDocMetadataProcessor) logDocMetaSummary(
	ctx context.Context,
	start, end time.Time,
	llmCallCount int,
	fallbackUsed bool,
	numPagesUsed int,
	numPagesAvailable int,
	procErr error,
) {
	extraInfo := map[string]any{
		"llm_call_count":      llmCallCount,
		"fallback_used":       fallbackUsed,
		"num_pages_used":      numPagesUsed,
		"num_pages_available": numPagesAvailable,
	}
	extraJSON, _ := json.Marshal(extraInfo)
	extraStr := string(extraJSON)

	modelNames := make([]string, 0, 2)
	seen := map[string]struct{}{}
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

	var errStr *string
	if procErr != nil {
		s := procErr.Error()
		errStr = &s
	}

	rec := DocProcLogRecord{
		DocProcName:   p.Name(),
		ModelNames:    modelNames,
		PromptName:    p.PromptRef,
		ExtraInfoJSON: &extraStr,
		Errors:        errStr,
		MSUsed:        int64Ptr(end.Sub(start).Milliseconds()),
	}
	if err := p.ProcLogger.LogSummary(ctx, "extract_doc_metadata", rec, "MID-26052806"); err != nil {
		p.Logger.Warn("failed to write doc_proc_summary log", "error", err)
	}
}

func (p *ExtractDocMetadataProcessor) extractMetadataWithFallback(ctx context.Context, inputText string) (map[string]any, string, error) {
	parsed, err := p.extractMetadataWithModel(ctx, inputText, p.ModelName, p.ModelCfg)
	if err == nil {
		return parsed, strings.TrimSpace(p.ModelName), nil
	}

	primaryModelName := strings.TrimSpace(p.ModelName)
	fallbackModelName := strings.TrimSpace(p.FallbackModelName)
	if fallbackModelName == "" {
		return nil, primaryModelName, fmt.Errorf("(MID_26051001) primary extraction failed and fallback model not configured: %w", err)
	}
	if p.FallbackModelErr != nil {
		return nil, fallbackModelName, fmt.Errorf("(MID_26051002) primary extraction failed and fallback model %q is unavailable: %w", p.FallbackModelRef, err)
	}

	p.Logger.Warn("primary doc metadata extraction failed; retrying fallback model",
		"primary_model", primaryModelName,
		"fallback_model", fallbackModelName,
		"error", err,
		"prompt_name", p.PromptRef,
		"inputText", inputText,
	)

	parsed, fallbackErr := p.extractMetadataWithModel(ctx, inputText, fallbackModelName, p.FallbackModelCfg)
	if fallbackErr != nil {
		if ApiUtils.IsEmptyJSONResponse(fallbackErr) {
			p.Logger.Warn("fallback doc metadata extraction returned empty JSON; treating as empty result",
				"fallback_model", fallbackModelName,
				"error", fallbackErr,
				"prompt_name", p.PromptRef,
			)
			return map[string]any{}, fallbackModelName, nil
		}
		return nil, fallbackModelName, fmt.Errorf("(MID_26051003) primary extraction failed: %w; fallback extraction failed: %v", err, fallbackErr)
	}
	return parsed, fallbackModelName, nil
}

func (p *ExtractDocMetadataProcessor) extractMetadataWithModel(ctx context.Context, inputText string, modelName string, cfg structureModelConfig) (map[string]any, error) {
	applyStructureModelConfigToExtractor(p.Client, cfg)
	in := newLLMJSONInput(ctx, p.PromptRef, p.PromptText, modelName, inputText, "extract_doc_metadata", "MID-CWB-EXTRACT-DOC-METADATA")
	apiStart := time.Now()
	if structuredExtractor, ok := p.Client.(LLMStructuredJSONExtractor); ok {
		result, err := structuredExtractor.ExtractStructuredJSON(ctx, in, docMetadataExtractionContract())
		p.Logger.Debug("structured llm api returned",
			"model", modelName,
			"ms_used", time.Since(apiStart).Milliseconds(),
			"error", err,
		)
		if err != nil {
			return nil, err
		}
		if result == nil {
			return map[string]any{}, nil
		}
		return result.Parsed, nil
	}
	result, err := p.Client.ExtractJSON(ctx, in)
	p.Logger.Debug("unstructured llm api returned",
		"model", modelName,
		"ms_used", time.Since(apiStart).Milliseconds(),
		"error", err,
	)
	return result, err
}

type docMetadataExtractionOutput struct {
	Title               string
	DocNo               string
	PublishDate         string
	Authors             []string
	MainDraftingPersons []string
	DraftingPersons     []string
	Metadata            map[string]any
	NeedMorePages       bool
}

func parseDocMetadataOutput(parsed map[string]any) docMetadataExtractionOutput {
	out := docMetadataExtractionOutput{
		Title:               strings.TrimSpace(asString(parsed["title"])),
		DocNo:               strings.TrimSpace(asString(parsed["doc_no"])),
		PublishDate:         strings.TrimSpace(asString(parsed["publish_date"])),
		Authors:             asStringSlice(parsed["authors"]),
		MainDraftingPersons: asStringSlice(parsed["main_drafting_persons"]),
		DraftingPersons:     asStringSlice(parsed["drafting_persons"]),
		Metadata:            copyExtractedDocMetadata(parsed),
	}
	if out.Metadata == nil {
		out.Metadata = map[string]any{}
	}
	if v, ok := parsed["need_more_pages"]; ok {
		b, err := asBool(v, false)
		if err == nil {
			out.NeedMorePages = b
		}
	}
	return out
}

func (p *ExtractDocMetadataProcessor) failAndPersist(ctx context.Context, rec DocMetadataInputRecord, procErr error) error {
	errMsg := strings.TrimSpace(procErr.Error())
	statusRaw, err := appendDocMetaStatus(rec.StatusRaw, p.Now(), 0, procErr)
	if err != nil {
		p.Logger.Error("failed building status json", "record_id", rec.ID, "error", err)
		return nil
	}

	updateErr := p.Store.UpdateInputMetadata(ctx, rec.ID, DocMetadataUpdate{
		StatusRaw: statusRaw,
		ErrorMsg:  &errMsg,
	})
	if updateErr != nil {
		p.Logger.Error("failed persisting error status", "record_id", rec.ID, "error", updateErr)
		return nil
	}

	p.Logger.Error("doc metadata extraction failed",
		"record_id", rec.ID,
		"error", procErr,
		"prompt_ref", p.PromptRef,
		"prompt_path", p.PromptPath,
	)
	return nil
}

func loadDocMetaPromptFromEnv() (promptText string, promptRef string, promptPath string, promptErr error) {
	promptRef = strings.TrimSpace(os.Getenv("EXTRACT_DOCMETA_PROMPT"))
	if promptRef == "" {
		return defaultDocMetaPrompt, "", "", nil
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
		addCandidate(filepath.Join("prompts", promptRef))
	}

	var lastErr error
	for _, candidate := range paths {
		bs, err := os.ReadFile(candidate)
		if err != nil {
			lastErr = err
			continue
		}
		text := strings.TrimSpace(string(bs))
		if text == "" {
			return "", promptRef, candidate, fmt.Errorf("(MID_26042417) prompt file is empty")
		}
		return text, promptRef, candidate, nil
	}
	if lastErr == nil {
		lastErr = errors.New("(MID-26060813) no candidate path available")
	}
	return "", promptRef, "", fmt.Errorf("(MID_26042418) prompt file not found: %w", lastErr)
}

func readLineFile(path string) (map[int][]string, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()

	linesByPage := map[int][]string{}
	maxPage := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		page := parsePageNumber(line)
		if page <= 0 {
			page = 1
		}
		linesByPage[page] = append(linesByPage[page], line)
		if page > maxPage {
			maxPage = page
		}
	}
	if err := sc.Err(); err != nil {
		return nil, 0, err
	}
	if maxPage == 0 {
		maxPage = 1
	}
	return linesByPage, maxPage, nil
}

func parsePageNumber(line string) int {
	parts := strings.Split(line, "\t")
	if len(parts) != 7 {
		return 0
	}
	page, err := parsePositiveIntField(parts[1])
	if err != nil {
		return 0
	}
	return page
}

func parsePositiveIntField(raw string) (int, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return 0, fmt.Errorf("(MID_26042408) empty integer field")
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, err
	}
	if n < 1 {
		return 0, fmt.Errorf("(MID_26042409) value must be >= 1")
	}
	return n, nil
}

func buildInputText(linesByPage map[int][]string, lastPage int) string {
	if lastPage < 1 {
		lastPage = 1
	}
	var b strings.Builder
	for p := 1; p <= lastPage; p++ {
		for _, line := range linesByPage[p] {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	return strings.TrimSpace(b.String())
}

func normalizePublishDateForColumn(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t.Format("2006-01-02")
	}

	for _, layout := range []string{
		"2006-01-02",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04",
	} {
		if t, err := time.ParseInLocation(layout, raw, time.Local); err == nil {
			return t.Format("2006-01-02")
		}
	}

	return ""
}

func appendDocMetaStatus(raw string, start time.Time, durationMs int64, procErr error) (string, error) {
	entries := decodeDocMetaStatus(raw)
	entry := map[string]any{
		"operation":  "extract_metadata",
		"start_time": start.Format(defaultDocMetaStatusTime),
		"ms-used":    durationMs,
	}
	if procErr == nil {
		entry["proc_status"] = "success"
	} else {
		entry["proc_status"] = "failed"
		entry["error"] = procErr.Error()
	}

	replaced := false
	out := make([]map[string]any, 0, len(entries)+1)
	for _, e := range entries {
		op := strings.ToLower(strings.TrimSpace(asString(e["operation"])))
		if op != "extract_metadata" && op != "extract_doc_metadata" {
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

func decodeDocMetaStatus(raw string) []map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return []map[string]any{}
	}
	var arr []map[string]any
	if err := json.Unmarshal([]byte(raw), &arr); err == nil {
		return arr
	}
	var one map[string]any
	if err := json.Unmarshal([]byte(raw), &one); err == nil {
		return []map[string]any{one}
	}
	return []map[string]any{}
}

func dedupeNonEmpty(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, item := range in {
		v := strings.TrimSpace(item)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func pickPreferredAuthors(out docMetadataExtractionOutput) []string {
	if authors := dedupeNonEmpty(out.Authors); len(authors) > 0 {
		return authors
	}
	if authors := dedupeNonEmpty(out.MainDraftingPersons); len(authors) > 0 {
		return authors
	}
	return dedupeNonEmpty(out.DraftingPersons)
}

func copyMetadata(in map[string]any) map[string]any {
	if len(in) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		if strings.TrimSpace(k) == "" {
			continue
		}
		out[k] = v
	}
	return out
}

func hasPathSeparator(name string) bool {
	return strings.ContainsRune(name, filepath.Separator) || strings.Contains(name, "/") || strings.Contains(name, `\\`)
}

func asStringSlice(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		if s := strings.TrimSpace(asString(v)); s != "" {
			return []string{s}
		}
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		s := strings.TrimSpace(asString(item))
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func copyExtractedDocMetadata(parsed map[string]any) map[string]any {
	if len(parsed) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(parsed))
	for k, v := range parsed {
		key := strings.TrimSpace(k)
		if key == "" || key == "need_more_pages" {
			continue
		}
		out[key] = v
	}
	normalizeDocMetadataLanguage(out)
	return out
}

func normalizeDocMetadataLanguage(metadata map[string]any) {
	if len(metadata) == 0 {
		return
	}
	if lang := ApiUtils.NormalizeLang(asString(metadata["language"])); lang != "" {
		metadata["language"] = lang
	}
	if nested, ok := metadata["metadata"].(map[string]any); ok {
		if lang := ApiUtils.NormalizeLang(asString(nested["language"])); lang != "" {
			nested["language"] = lang
		}
	}
}

const defaultDocMetaPrompt = `You extract document metadata from line-based document text.
Return strict JSON with fields:
- title: string
- doc_no: string
- publish_date: string
- authors: string[]
- main_drafting_persons: string[]
- drafting_persons: string[]
- metadata: object (other metadata only)
- need_more_pages: boolean

Rules:
- If current pages are insufficient, set need_more_pages=true.
- Never output markdown.
- Prefer empty strings/arrays instead of null.`
