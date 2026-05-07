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

type ProvisionsProcessor struct {
	InputStore   DocMetadataStore
	Store        ProvisionsStore
	Extractor    LLMJSONExtractor
	Logger       ApiTypes.JimoLogger
	Now          func() time.Time
	PromptText   string
	PromptRef    string
	PromptPath   string
	PromptErr    error
	ModelRef     string
	ModelCfgPath string
	ModelErr     error
	ModelName    string
	BlockSize    int
	ArtifactDir  string
}

type ProvisionsStore interface {
	ProvisionsExist(ctx context.Context, inputRecordID int64, inputFilename string) (bool, error)
	DeleteProvisionsByInputRecordID(ctx context.Context, inputRecordID int64) (int64, error)
	SaveProvisions(ctx context.Context, req SaveProvisionsRequest) (int64, error)
}

type ProvisionsSQLStore struct {
	DB *sql.DB
}

type SaveProvisionsRequest struct {
	InputRecordID int64
	InputFilename string
	ExtractID     string
	Language      string
	Provisions    []map[string]any
}

type provisionExtractionResult struct {
	ExtractID  string
	Language   string
	Provisions []map[string]any
}

func NewProvisionsProcessor(inputStore DocMetadataStore, store ProvisionsStore, extractor LLMJSONExtractor, logger ApiTypes.JimoLogger) *ProvisionsProcessor {
	if logger == nil {
		logger = loggerutil.CreateDefaultLogger("MID_26050520")
	}
	promptText, promptRef, promptPath, promptErr := loadProvisionsPromptFromEnv()
	modelRef, modelCfgPath, modelCfg, modelErr := loadModelConfigFromEnv("EXTRACT_PROVISIONS_MODEL_NAME", "EXTRACT_PROVISIONS_MODELS_FILE")
	applyStructureModelConfigToExtractor(extractor, modelCfg)
	return &ProvisionsProcessor{
		InputStore:   inputStore,
		Store:        store,
		Extractor:    extractor,
		Logger:       logger,
		Now:          time.Now,
		PromptText:   promptText,
		PromptRef:    promptRef,
		PromptPath:   promptPath,
		PromptErr:    promptErr,
		ModelRef:     modelRef,
		ModelCfgPath: modelCfgPath,
		ModelErr:     modelErr,
		ModelName:    modelCfg.ModelName,
		BlockSize:    envInt("INPUT_BLOCK_SIZE", DefaultBlockingBlockSize, 1),
		ArtifactDir:  strings.TrimSpace(os.Getenv("ARTIFACT_DIR")),
	}
}

func (p *ProvisionsProcessor) Name() string { return "extract_provisions" }

func (p *ProvisionsProcessor) HandleEvent(ctx context.Context, payload []byte) error {
	p.Logger.Info("Extract Provisions handle event")
	start := p.Now()
	evt, err := ParseLineFileGeneratedEvent(payload)
	if err != nil {
		return fmt.Errorf("(MID_26050521) parse event payload: %w", err)
	}
	if ShouldSkipLineFileGeneratedEvent(evt) {
		return nil
	}
	if p.PromptErr != nil {
		return fmt.Errorf("(MID_26050522) load provisions prompt %q: %w", p.PromptRef, p.PromptErr)
	}
	if p.InputStore == nil {
		return errors.New("(MID_26050523) input store is nil")
	}
	if p.Store == nil {
		return errors.New("(MID_26050524) provisions store is nil")
	}

	rec, err := p.InputStore.GetInputRecord(ctx, evt.RecordID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			p.Logger.Error("kb.inputs record not found", "record_id", evt.RecordID)
			return nil
		}
		return fmt.Errorf("(MID_26050525) load kb.inputs record %d: %w", evt.RecordID, err)
	}
	if p.ModelErr != nil {
		p.Logger.Warn("provisions extraction skipped: model config error",
			"record_id", evt.RecordID, "model_ref", p.ModelRef, "error", p.ModelErr)
		p.persistProvisionsStatus(ctx, rec, start, p.ModelErr)
		return nil
	}

	inputFilename := filepath.Base(strings.TrimSpace(rec.ResultFilename))
	if inputFilename == "" {
		inputFilename = fmt.Sprintf("record_%d", evt.RecordID)
	}

	if evt.Force {
		_, _ = p.Store.DeleteProvisionsByInputRecordID(ctx, evt.RecordID)
	} else {
		exists, err := p.Store.ProvisionsExist(ctx, evt.RecordID, "")
		if err != nil {
			p.persistProvisionsStatus(ctx, rec, start, err)
			p.Logger.Error("provision exist", "error", err,
				"record_id", evt.RecordID)
			return nil
		}
		if exists {
			p.Logger.Info("provisions extraction skipped", "record_id", evt.RecordID, "reason", "provisions already exist and force=false")
			p.persistProvisionsStatus(ctx, rec, start, nil)
			return nil
		}
	}

	blocks, err := p.resolveBlocks(ctx, evt, rec)
	if err != nil {
		p.persistProvisionsStatus(ctx, rec, start, err)
		p.Logger.Error("resolveBlocks error", "error", err, "record_id", evt.RecordID)
		return nil
	}
	if len(blocks) == 0 {
		err := fmt.Errorf("(MID_26050526) no blocks found for record_id=%d", evt.RecordID)
		p.persistProvisionsStatus(ctx, rec, start, err)
		return nil
	}

	p.Logger.Info("Before calling LLM",
		"num_blocks", len(blocks),
		"record_id", evt.RecordID,
		"filename", inputFilename)

	result, err := p.extractProvisionsFromBlocksWithLLM(ctx, blocks)
	if err != nil {
		p.persistProvisionsStatus(ctx, rec, start, err)
		return nil
	}
	outputRows := p.buildProvisionOutputRows(result.Provisions, start, len(blocks), time.Since(start).Milliseconds())

	inserted, err := p.Store.SaveProvisions(ctx, SaveProvisionsRequest{
		InputRecordID: evt.RecordID,
		InputFilename: inputFilename,
		ExtractID:     result.ExtractID,
		Language:      result.Language,
		Provisions:    outputRows,
	})
	if err != nil {
		p.persistProvisionsStatus(ctx, rec, start, err)
		p.Logger.Error("save provision error", "error", err, "record_id", evt.RecordID)
		return nil
	}
	artifactPath, err := p.writeProvisionsArtifact(evt.RecordID, rec, outputRows)
	if err != nil {
		p.Logger.Error("write provision error", "error", err, "record_id", evt.RecordID)
		p.persistProvisionsStatus(ctx, rec, start, err)
		return nil
	}
	p.Logger.Info("provisions extracted",
		"record_id", evt.RecordID,
		"inserted_rows", inserted,
		"provisions_count", len(outputRows),
		"blocks", len(blocks),
		"artifact_path", artifactPath,
	)
	p.persistProvisionsStatus(ctx, rec, start, nil)
	return nil
}

func (p *ProvisionsProcessor) resolveBlocks(ctx context.Context, evt LineFileGeneratedEvent, rec DocMetadataInputRecord) ([]Block, error) {
	if buf := BlockBufferFromContext(ctx); buf != nil {
		return buf.Blocks, nil
	}

	inputPath, err := ResolveInputFilePath(evt, rec.ResultFilename, rec.ParserName, rec.StagingFilename)
	if err != nil {
		return nil, fmt.Errorf("(MID_26050527) resolve input file for record_id=%d: %w", evt.RecordID, err)
	}
	body, err := os.ReadFile(inputPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("(MID_26050528) input file not exist: %s", inputPath)
		}
		return nil, fmt.Errorf("(MID_26050529) read input file: %w", err)
	}
	buf, err := buildBlocks(body, p.blockSize())
	if err != nil {
		return nil, fmt.Errorf("(MID_26050530) build blocks: %w", err)
	}
	return buf.Blocks, nil
}

func (p *ProvisionsProcessor) blockSize() int {
	if p.BlockSize >= 1 {
		return p.BlockSize
	}
	return DefaultBlockingBlockSize
}

func (p *ProvisionsProcessor) extractProvisionsFromBlocksWithLLM(ctx context.Context, blocks []Block) (provisionExtractionResult, error) {
	extractID := p.Now().Format("20060102-150405")
	language := ""
	provisions := make([]map[string]any, 0, len(blocks))

	for idx, block := range blocks {
		p.Logger.Info("Start extracting provisions",
			"idx", idx,
			"total", len(blocks),
			"model name", p.ModelName,
			"prompt name", p.PromptRef,
		)
		payload, err := p.Extractor.ExtractJSON(ctx, llmclients.JSONExtractionInput{
			PromptText: p.PromptText,
			ModelName:  p.ModelName,
			InputText:  buildProvisionUserPrompt(block),
		})
		if err != nil {
			p.Logger.Error("failed extracting", "error", err)
			return provisionExtractionResult{}, fmt.Errorf("(MID_26050531) extract provisions via llm: %w", err)
		}
		if language == "" {
			language = strings.TrimSpace(asString(payload["language"]))
		}
		raw, ok := payload["provisions"].([]any)
		if !ok {
			p.Logger.Error("missing 'provisions'")
			return provisionExtractionResult{}, fmt.Errorf("(MID_26050532) llm output field 'provisions' must be an array")
		}
		provisions = append(provisions, normalizeProvisionList(raw, blockLineToPage(block), blockLineText(block))...)
		p.Logger.Info("LLM responded", "# provisions", len(provisions))
	}
	if language == "" {
		language = "unknown"
	}
	return provisionExtractionResult{
		ExtractID:  extractID,
		Language:   language,
		Provisions: provisions,
	}, nil
}

func buildProvisionUserPrompt(block Block) string {
	schema := map[string]any{
		"language": "string",
		"provisions": []map[string]any{{
			"name":               "string",
			"type":               "mandatory|recommended|optional",
			"provision_original": "string",
			"provision_en":       "string",
			"source_line_spans":  []string{"2:10"},
			"context":            "string",
			"subject":            "string",
			"location_type":      "string",
			"keywords":           []string{"string"},
			"confidence":         0.0,
			"is_explicit":        true,
			"need_verify":        false,
			"categories":         []any{},
		}},
	}
	lines := make([]string, 0, len(block.Lines))
	for _, line := range block.Lines {
		lines = append(lines, line.String())
	}
	linesJSON, _ := json.Marshal(lines)
	schemaJSON, _ := json.Marshal(schema)
	return "Return JSON only. Use exactly this top-level schema:\n" + string(schemaJSON) +
		"\n\nBlock index: " + strconv.Itoa(block.Index) +
		"\n\nInput block lines (plain):\n" + strings.Join(lines, "\n") +
		"\n\nInput block lines (raw, JSON array):\n" + string(linesJSON)
}

func blockLineToPage(block Block) map[int]int {
	out := make(map[int]int, len(block.Lines))
	for _, line := range block.Lines {
		if line.LineNumber > 0 && line.PageNumber > 0 {
			out[line.LineNumber] = line.PageNumber
		}
	}
	return out
}

func blockLineText(block Block) map[string]string {
	out := make(map[string]string, len(block.Lines))
	for _, line := range block.Lines {
		if line.LineNumber <= 0 || line.PageNumber <= 0 {
			continue
		}
		out[fmt.Sprintf("%d:%d", line.PageNumber, line.LineNumber)] = strings.TrimSpace(line.Content)
	}
	return out
}

func normalizeProvisionList(items []any, _ map[int]int, lineText map[string]string) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		raw, ok := item.(map[string]any)
		if !ok {
			continue
		}
		provisionName := strings.TrimSpace(firstNonEmptyTrimmed(asString(raw["provision_name"]), asString(raw["name"])))
		normalized := map[string]any{
			"provision_name":     provisionName,
			"provision_type":     strings.TrimSpace(firstNonEmptyTrimmed(asString(raw["provision_type"]), asString(raw["type"]))),
			"source_text":        strings.TrimSpace(firstNonEmptyTrimmed(asString(raw["source_text"]), asString(raw["provision_original"]))),
			"provision_original": strings.TrimSpace(asString(raw["provision_original"])),
			"provision_en":       strings.TrimSpace(asString(raw["provision_en"])),
			"context":            strings.TrimSpace(asString(raw["context"])),
			"subject":            strings.TrimSpace(asString(raw["subject"])),
			"location_type":      strings.TrimSpace(asString(raw["location_type"])),
			"keywords":           toStringSlice(raw["keywords"]),
			"confidence":         toFloat(raw["confidence"]),
			"is_explicit":        toBool(raw["is_explicit"]),
			"need_verify":        toBool(raw["need_verify"]),
			"categories":         raw["categories"],
		}
		sourceSpans := normalizeSourceLineSpans(raw["source_line_spans"])
		normalized["source_line_spans"] = sourceSpans
		if strings.TrimSpace(asString(normalized["source_text"])) == "" {
			normalized["source_text"] = sourceTextFromSpans(sourceSpans, lineText)
		}
		out = append(out, normalized)
	}
	return out
}

func sourceTextFromSpans(spans []string, lineText map[string]string) string {
	if len(spans) == 0 || len(lineText) == 0 {
		return ""
	}
	parts := make([]string, 0, len(spans))
	for _, span := range spans {
		text := strings.TrimSpace(lineText[strings.TrimSpace(span)])
		if text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

func (p *ProvisionsProcessor) buildProvisionOutputRows(provisions []map[string]any, now time.Time, numBlocks int, durationMs int64) []map[string]any {
	out := make([]map[string]any, 0, len(provisions))
	timeText := now.Format(defaultDocMetaStatusTime)
	timePerProvision := float64(0)
	if len(provisions) > 0 {
		timePerProvision = float64(durationMs) / float64(len(provisions))
	}
	for i, provision := range provisions {
		publicInfo := map[string]any{
			"provision_type":     strings.TrimSpace(asString(provision["provision_type"])),
			"source_line_spans":  provision["source_line_spans"],
			"provision_original": strings.TrimSpace(asString(provision["provision_original"])),
			"provision_en":       strings.TrimSpace(asString(provision["provision_en"])),
			"need_verify":        toBool(provision["need_verify"]),
		}
		out = append(out, map[string]any{
			"prov_id":            i + 1,
			"prov_name":          strings.TrimSpace(asString(provision["provision_name"])),
			"prov_subject":       strings.TrimSpace(asString(provision["subject"])),
			"prov_desc":          strings.TrimSpace(firstNonEmptyTrimmed(asString(provision["provision_en"]), asString(provision["provision_original"]), asString(provision["source_text"]))),
			"prov_context":       strings.TrimSpace(asString(provision["context"])),
			"prov_keywords":      provision["keywords"],
			"category_paths":     provision["categories"],
			"location_type":      strings.TrimSpace(asString(provision["location_type"])),
			"prov_conf":          toFloat(provision["confidence"]),
			"source_text":        strings.TrimSpace(asString(provision["source_text"])),
			"num_blocks":         numBlocks,
			"num_provisions":     len(provisions),
			"time_per_provision": timePerProvision,
			"model_name":         strings.TrimSpace(p.ModelName),
			"prompt_name":        strings.TrimSpace(p.PromptRef),
			"is_explicit":        toBool(provision["is_explicit"]),
			"status":             "active",
			"create_time":        timeText,
			"modify_time":        timeText,
			"public_info":        publicInfo,
			"private_info":       map[string]any{},
			"notes":              "",
			"error_msg":          "",

			// Keep source spans available for tests and callers that inspect
			// the in-memory request before it is folded into public_info.
			"source_line_spans": provision["source_line_spans"],
		})
	}
	return out
}

func buildProvisionDBRecord(provision map[string]any, language string) map[string]any {
	publicInfo, _ := provision["public_info"].(map[string]any)
	provisionOriginal := strings.TrimSpace(asString(publicInfo["provision_original"]))
	provisionEn := strings.TrimSpace(asString(publicInfo["provision_en"]))
	lang := strings.ToLower(strings.TrimSpace(language))
	if lang == "en" || lang == "eng" || lang == "english" || strings.EqualFold(provisionOriginal, provisionEn) {
		provisionOriginal = ""
	}

	var originalValue any
	if provisionOriginal != "" {
		originalValue = provisionOriginal
	}

	return map[string]any{
		"prov_id":            int(toFloat(provision["prov_id"])),
		"prov_name":          strings.TrimSpace(asString(provision["prov_name"])),
		"provision_type":     strings.TrimSpace(asString(publicInfo["provision_type"])),
		"source_text":        strings.TrimSpace(asString(provision["source_text"])),
		"source_line_spans":  provision["source_line_spans"],
		"provision_original": originalValue,
		"provision_en":       provisionEn,
		"prov_context":       strings.TrimSpace(asString(provision["prov_context"])),
		"provision_subject":  strings.TrimSpace(asString(provision["prov_subject"])),
		"prov_desc":          strings.TrimSpace(asString(provision["prov_desc"])),
		"provision_keywords": provision["prov_keywords"],
		"category_paths":     provision["category_paths"],
		"location_type":      strings.TrimSpace(asString(provision["location_type"])),
		"confidence":         toFloat(provision["prov_conf"]),
		"is_explicit":        toBool(provision["is_explicit"]),
		"need_verify":        toBool(publicInfo["need_verify"]),
		"num_blocks":         int(toFloat(provision["num_blocks"])),
		"num_provisions":     int(toFloat(provision["num_provisions"])),
		"time_per_provision": toFloat(provision["time_per_provision"]),
		"model_name":         strings.TrimSpace(asString(provision["model_name"])),
		"prompt_name":        strings.TrimSpace(asString(provision["prompt_name"])),
		"status":             strings.TrimSpace(firstNonEmptyTrimmed(asString(provision["status"]), "active")),
		"public_info":        publicInfo,
		"private_info":       provision["private_info"],
		"notes":              strings.TrimSpace(asString(provision["notes"])),
		"error_msg":          strings.TrimSpace(asString(provision["error_msg"])),
	}
}

func (p *ProvisionsProcessor) writeProvisionsArtifact(recordID int64, rec DocMetadataInputRecord, rows []map[string]any) (string, error) {
	artifactDir := strings.TrimSpace(p.ArtifactDir)
	if artifactDir == "" {
		artifactDir = strings.TrimSpace(os.Getenv("ARTIFACT_DIR"))
	}
	if artifactDir == "" {
		return "", errors.New("(MID_26050538) missing ARTIFACT_DIR")
	}
	if recordID <= 0 {
		return "", fmt.Errorf("(MID_26050539) invalid record_id: %d", recordID)
	}
	parserName := strings.TrimSpace(rec.ParserName)
	if parserName == "" {
		return "", errors.New("(MID_26050540) missing parser name")
	}
	root := strings.TrimSuffix(filepath.Base(strings.TrimSpace(rec.StagingFilename)), filepath.Ext(strings.TrimSpace(rec.StagingFilename)))
	if root == "" {
		root = strings.TrimSuffix(filepath.Base(strings.TrimSpace(rec.ResultFilename)), filepath.Ext(strings.TrimSpace(rec.ResultFilename)))
	}
	if root == "" {
		root = fmt.Sprintf("record_%d", recordID)
	}

	groupID := recordID / 1000
	dir := filepath.Join(artifactDir, strconv.FormatInt(groupID, 10), strconv.FormatInt(recordID, 10))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("(MID_26050541) create provisions artifact dir: %w", err)
	}
	path := filepath.Join(dir, root+"_"+parserName+".provisions")
	body, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return "", fmt.Errorf("(MID_26050542) marshal provisions artifact: %w", err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return "", fmt.Errorf("(MID_26050543) write provisions artifact: %w", err)
	}
	return path, nil
}

func (p *ProvisionsProcessor) persistProvisionsStatus(ctx context.Context, rec DocMetadataInputRecord, start time.Time, procErr error) {
	errMsg := (*string)(nil)
	if procErr != nil {
		msg := strings.TrimSpace(procErr.Error())
		errMsg = &msg
	}
	statusRaw, err := appendProvisionsStatus(rec.StatusRaw, start, time.Since(start).Milliseconds(), procErr)
	if err != nil {
		p.Logger.Error("failed building provisions status", "record_id", rec.ID, "error", err)
		return
	}
	if err := p.InputStore.UpdateInputMetadata(ctx, rec.ID, DocMetadataUpdate{
		StatusRaw: statusRaw,
		ErrorMsg:  errMsg,
	}); err != nil {
		p.Logger.Error("failed persisting provisions status", "record_id", rec.ID, "error", err)
	}
}

func appendProvisionsStatus(raw string, start time.Time, durationMs int64, procErr error) (string, error) {
	entries := decodeDocMetaStatus(raw)
	entry := map[string]any{
		"operation":  "extract_provisions",
		"start_time": start.Format(defaultDocMetaStatusTime),
		"ms-used":    durationMs,
	}
	if procErr == nil {
		entry["proc-status"] = "success"
		entry["proc_status"] = "success"
	} else {
		entry["proc-status"] = "failed"
		entry["proc_status"] = "failed"
		entry["error"] = strings.TrimSpace(procErr.Error())
	}

	replaced := false
	out := make([]map[string]any, 0, len(entries)+1)
	for _, e := range entries {
		op := strings.ToLower(strings.TrimSpace(asString(e["operation"])))
		if op != "extract_provisions" && op != "extract-provisions" {
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

func loadProvisionsPromptFromEnv() (promptText string, promptRef string, promptPath string, promptErr error) {
	for _, key := range []string{"EXTRACT_PROVISIONS_PROMPT", "PROMPT_FILE_NAME"} {
		promptRef = strings.TrimSpace(os.Getenv(key))
		if promptRef != "" {
			break
		}
	}
	if promptRef == "" {
		promptRef = "prompt-extract-provisions.md"
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
			lastErr = fmt.Errorf("(MID_26050533) failed reading file. Path:%s, error:%w", candidate, err)
			continue
		}
		text := strings.TrimSpace(string(bs))
		if text == "" {
			return "", promptRef, candidate, fmt.Errorf("(MID_26050534) prompt file is empty")
		}
		return text, promptRef, candidate, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("(MID_26050535) no candidate path available")
	}
	return "", promptRef, "", fmt.Errorf("(MID_26050536) prompt file not found: %w", lastErr)
}

func (s ProvisionsSQLStore) ensureReady() error {
	if s.DB == nil {
		return fmt.Errorf("(MID_26050537) db is nil")
	}
	return nil
}

func (s ProvisionsSQLStore) ProvisionsExist(ctx context.Context, inputRecordID int64, inputFilename string) (bool, error) {
	if err := s.ensureReady(); err != nil {
		return false, err
	}
	const q = `
SELECT 1
FROM kb.provisions
WHERE input_record_id = $1
  AND ($2 = '' OR input_filename = $2)
LIMIT 1`
	var one int
	err := s.DB.QueryRowContext(ctx, q, inputRecordID, strings.TrimSpace(inputFilename)).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s ProvisionsSQLStore) DeleteProvisionsByInputRecordID(ctx context.Context, inputRecordID int64) (int64, error) {
	if err := s.ensureReady(); err != nil {
		return 0, err
	}
	res, err := s.DB.ExecContext(ctx, `DELETE FROM kb.provisions WHERE input_record_id = $1`, inputRecordID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s ProvisionsSQLStore) SaveProvisions(ctx context.Context, req SaveProvisionsRequest) (int64, error) {
	if err := s.ensureReady(); err != nil {
		return 0, err
	}
	if len(req.Provisions) == 0 {
		return 0, nil
	}

	const stmt = `
INSERT INTO kb.provisions (
	input_record_id,
	extract_id,
	input_filename,
	prov_id,
	prov_name,
	provision_type,
	source_text,
	source_line_spans,
	provision_original,
	provision_en,
	provision_subject,
	prov_desc,
	prov_context,
	provision_keywords,
	category_paths,
	location_type,
	confidence,
	is_explicit,
	need_verify,
	num_blocks,
	num_provisions,
	time_per_provision,
	model_name,
	prompt_name,
	status,
	public_info,
	private_info,
	notes,
	error_msg
)
VALUES (
		$1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9,$10,$11,$12,$13,$14::jsonb,$15::jsonb,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26::jsonb,$27::jsonb,$28,$29
)
ON CONFLICT (input_record_id, prov_id) DO UPDATE SET
	extract_id = EXCLUDED.extract_id,
	input_filename = EXCLUDED.input_filename,
	prov_name = EXCLUDED.prov_name,
	provision_type = EXCLUDED.provision_type,
	source_text = EXCLUDED.source_text,
	source_line_spans = EXCLUDED.source_line_spans,
	provision_original = EXCLUDED.provision_original,
	provision_en = EXCLUDED.provision_en,
	provision_subject = EXCLUDED.provision_subject,
	prov_desc = EXCLUDED.prov_desc,
	prov_context = EXCLUDED.prov_context,
	provision_keywords = EXCLUDED.provision_keywords,
	category_paths = EXCLUDED.category_paths,
	location_type = EXCLUDED.location_type,
	confidence = EXCLUDED.confidence,
	is_explicit = EXCLUDED.is_explicit,
	need_verify = EXCLUDED.need_verify,
	num_blocks = EXCLUDED.num_blocks,
	num_provisions = EXCLUDED.num_provisions,
	time_per_provision = EXCLUDED.time_per_provision,
	model_name = EXCLUDED.model_name,
	prompt_name = EXCLUDED.prompt_name,
	status = EXCLUDED.status,
	public_info = EXCLUDED.public_info,
	private_info = EXCLUDED.private_info,
	notes = EXCLUDED.notes,
	error_msg = EXCLUDED.error_msg,
	modify_time = NOW()`

	var inserted int64
	for _, provision := range req.Provisions {
		dbRecord := buildProvisionDBRecord(provision, req.Language)
		sourceSpansJSON, _ := json.Marshal(dbRecord["source_line_spans"])
		keywordsJSON, _ := json.Marshal(dbRecord["provision_keywords"])
		categoryPathsJSON, _ := json.Marshal(dbRecord["category_paths"])
		publicInfo := map[string]any{
			"language":       req.Language,
			"schema_version": "1",
		}
		if existing, ok := dbRecord["public_info"].(map[string]any); ok {
			for k, v := range existing {
				publicInfo[k] = v
			}
		}
		publicInfoJSON, _ := json.Marshal(publicInfo)
		privateInfoJSON, _ := json.Marshal(dbRecord["private_info"])

		_, err := s.DB.ExecContext(ctx, stmt,
			req.InputRecordID,
			req.ExtractID,
			req.InputFilename,
			int(toFloat(dbRecord["prov_id"])),
			strings.TrimSpace(asString(dbRecord["prov_name"])),
			strings.TrimSpace(asString(dbRecord["provision_type"])),
			strings.TrimSpace(asString(dbRecord["source_text"])),
			string(sourceSpansJSON),
			dbRecord["provision_original"],
			strings.TrimSpace(asString(dbRecord["provision_en"])),
			strings.TrimSpace(asString(dbRecord["provision_subject"])),
			strings.TrimSpace(asString(dbRecord["prov_desc"])),
			strings.TrimSpace(asString(dbRecord["prov_context"])),
			string(keywordsJSON),
			string(categoryPathsJSON),
			strings.TrimSpace(asString(dbRecord["location_type"])),
			toFloat(dbRecord["confidence"]),
			toBool(dbRecord["is_explicit"]),
			toBool(dbRecord["need_verify"]),
			int(toFloat(dbRecord["num_blocks"])),
			int(toFloat(dbRecord["num_provisions"])),
			toFloat(dbRecord["time_per_provision"]),
			strings.TrimSpace(asString(dbRecord["model_name"])),
			strings.TrimSpace(asString(dbRecord["prompt_name"])),
			strings.TrimSpace(firstNonEmptyTrimmed(asString(dbRecord["status"]), "active")),
			string(publicInfoJSON),
			string(privateInfoJSON),
			strings.TrimSpace(asString(dbRecord["notes"])),
			strings.TrimSpace(asString(dbRecord["error_msg"])),
		)
		if err != nil {
			return inserted, err
		}
		inserted++
	}
	return inserted, nil
}
