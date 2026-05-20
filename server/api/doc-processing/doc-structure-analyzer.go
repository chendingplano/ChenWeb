package docprocessing

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/chendingplano/shared/go/api/loggerutil"
	toml "github.com/pelletier/go-toml/v2"
)

type StructureAnalyzerProcessor struct {
	Store          DocMetadataStore
	Extractor      LLMJSONExtractor
	Logger         ApiTypes.JimoLogger
	Now            func() time.Time
	PromptText     string
	PromptRef      string
	PromptPath     string
	PromptErr      error
	ModelRef       string
	ModelCfgPath   string
	ModelErr       error
	ModelName      string
	APIKey         string
	BaseURL        string
	StructureDir   string
	MaxRetries     int
	InputBlockSize int
}

type structureLine struct {
	LineNumber int
	PageNumber int
	LineType   string
	Font       string
	FontSize   string
	Coordinate string
	Content    string
}

type structureLabel struct {
	LineNumber        int
	CorrectedLineType string
	Confidence        float64
	Reason            string
}

type structureOutput struct {
	CoverPages []int
	Labels     []structureLabel
}

type LLMTextExtractor interface {
	ExtractText(ctx context.Context, in llmclients.JSONExtractionInput) (string, error)
}

func NewStructureAnalyzerProcessor(store DocMetadataStore, extractor LLMJSONExtractor, logger ApiTypes.JimoLogger) *StructureAnalyzerProcessor {
	if logger == nil {
		logger = loggerutil.CreateDefaultLogger("MID_26042146")
	}
	promptText, promptRef, promptPath, promptErr := loadStructurePromptFromEnv()
	modelRef, modelCfgPath, modelCfg, modelErr := loadStructureModelFromEnv()
	applyStructureModelConfigToExtractor(extractor, modelCfg)
	return &StructureAnalyzerProcessor{
		Store:          store,
		Extractor:      extractor,
		Logger:         logger,
		Now:            time.Now,
		PromptText:     promptText,
		PromptRef:      promptRef,
		PromptPath:     promptPath,
		PromptErr:      promptErr,
		ModelRef:       modelRef,
		ModelCfgPath:   modelCfgPath,
		ModelErr:       modelErr,
		ModelName:      modelCfg.ModelName,
		APIKey:         modelCfg.APIKey,
		BaseURL:        modelCfg.BaseURL,
		StructureDir:   strings.TrimSpace(os.Getenv("ARTIFACT_DIR")),
		MaxRetries:     envInt("STRUCTURE_LLM_MAX_RETRIES", 2, 0),
		InputBlockSize: envInt("INPUT_BLOCK_SIZE", 8, 1),
	}
}

func (p *StructureAnalyzerProcessor) Name() string { return "structure_analyzer" }

func (p *StructureAnalyzerProcessor) HandleEvent(ctx context.Context, payload []byte) error {
	start := p.Now()
	evt, err := ParseLineFileGeneratedEvent(payload)
	if err != nil {
		return fmt.Errorf("(MID_26042147) parse event payload: %w", err)
	}
	if ShouldSkipLineFileGeneratedEvent(evt) {
		return nil
	}

	if err := p.validateRequiredEnv(); err != nil {
		return err
	}
	if p.ModelErr != nil {
		return p.ModelErr
	}
	if p.PromptErr != nil {
		return fmt.Errorf("(MID_26042148) load prompt file %q failed: %w", p.PromptRef, p.PromptErr)
	}
	if p.Extractor == nil {
		return errors.New("(MID_26042438) llm extractor is nil")
	}

	rec, err := p.Store.GetInputRecord(ctx, evt.RecordID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("(MID_26042149) kb.inputs record not found: %d", evt.RecordID)
		}
		return fmt.Errorf("(MID_26042150) load kb.inputs record %d: %w", evt.RecordID, err)
	}
	if strings.TrimSpace(rec.ParserName) == "" {
		return p.failAndPersist(ctx, rec, start, "", 0, 0, 0, 0, errors.New("(MID_26042421) missing parser name"))
	}
	if strings.TrimSpace(rec.ResultFilename) == "" {
		return p.failAndPersist(ctx, rec, start, "", 0, 0, 0, 0, errors.New("(MID_26042422) missing result filename"))
	}

	inputPath, err := ResolveInputFilePath(evt, rec.ResultFilename, rec.ParserName, rec.StagingFilename)
	if err != nil {
		return p.failAndPersist(ctx, rec, start, "", 0, 0, 0, 0, err)
	}
	inputFilename := filepath.Base(strings.TrimSpace(inputPath))
	if strings.TrimSpace(evt.Filename) != "" {
		inputFilename = filepath.Base(strings.TrimSpace(evt.Filename))
	}
	body, err := os.ReadFile(inputPath)
	if err != nil {
		return p.failAndPersist(ctx, rec, start, inputFilename, 0, 0, 0, 0, fmt.Errorf("(MID_26042151) read input file: %w", err))
	}

	lines, numPages, err := parseStructureLines(body)
	if err != nil {
		return p.failAndPersist(ctx, rec, start, inputFilename, 0, 0, 0, 0, err)
	}
	if len(lines) == 0 {
		return p.failAndPersist(ctx, rec, start, inputFilename, numPages, 0, 0, 0, errors.New("(MID_26042423) input file has no valid lines"))
	}

	blocks := buildStructureBlocks(lines, p.InputBlockSize)
	if len(blocks) == 0 {
		return p.failAndPersist(ctx, rec, start, inputFilename, numPages, len(lines), 0, 0, errors.New("(MID_26042424) input file has no block to analyze"))
	}

	aggregatedCoverPages := make(map[int]struct{}, numPages)
	aggregatedLabels := make(map[int]structureLabel, len(lines))
	for _, block := range blocks {
		out, err := p.analyzeStructureBlock(ctx, block.Lines)
		if err != nil {
			return p.failAndPersist(ctx, rec, start, inputFilename, numPages, len(lines), len(aggregatedLabels), len(aggregatedCoverPages), err)
		}

		labelByLine := make(map[int]structureLabel, len(out.Labels))
		for _, lbl := range out.Labels {
			labelByLine[lbl.LineNumber] = lbl
		}
		for _, lineNumber := range block.IncludeLineNumbers {
			lbl, ok := labelByLine[lineNumber]
			if !ok {
				return p.failAndPersist(ctx, rec, start, inputFilename, numPages, len(lines), len(aggregatedLabels), len(aggregatedCoverPages), fmt.Errorf("(MID_26042152) missing label for line_number=%d in block %d", lineNumber, block.Index))
			}
			aggregatedLabels[lineNumber] = lbl
		}
		for _, pageNo := range out.CoverPages {
			if pageNo > 0 {
				aggregatedCoverPages[pageNo] = struct{}{}
			}
		}
	}

	finalLabels := make([]structureLabel, 0, len(lines))
	for _, line := range lines {
		lbl, ok := aggregatedLabels[line.LineNumber]
		if !ok {
			return p.failAndPersist(ctx, rec, start, inputFilename, numPages, len(lines), len(aggregatedLabels), len(aggregatedCoverPages), fmt.Errorf("(MID_26042153) missing aggregated label for line_number=%d", line.LineNumber))
		}
		finalLabels = append(finalLabels, lbl)
	}
	finalCoverPages := make([]int, 0, len(aggregatedCoverPages))
	for pno := range aggregatedCoverPages {
		finalCoverPages = append(finalCoverPages, pno)
	}
	sort.Ints(finalCoverPages)
	output := structureOutput{
		CoverPages: finalCoverPages,
		Labels:     finalLabels,
	}

	if err := p.writeStructureArtifacts(rec.ID, lines, output); err != nil {
		return p.failAndPersist(ctx, rec, start, inputFilename, numPages, len(lines), 0, 0, err)
	}

	statusRaw, err := appendStructureStatus(rec.StatusRaw, structureStatusParams{
		InputFilename:   inputFilename,
		NumPages:        numPages,
		NumLines:        len(lines),
		NumLabeledLines: len(output.Labels),
		NumCoverPages:   len(output.CoverPages),
		Start:           start,
		DurationMs:      time.Since(start).Milliseconds(),
		ProcErr:         nil,
	})
	if err != nil {
		return fmt.Errorf("(MID_26042154) append structure status: %w", err)
	}

	if err := p.Store.UpdateInputMetadata(ctx, rec.ID, DocMetadataUpdate{
		StatusRaw: statusRaw,
		ErrorMsg:  nil,
	}); err != nil {
		return fmt.Errorf("(MID_26042155) update kb.inputs status: %w", err)
	}

	if p.Logger != nil {
		p.Logger.Info("structure analysis completed",
			"record_id", rec.ID,
			"input_filename", inputFilename,
			"num_pages", numPages,
			"num_lines", len(lines),
			"num_labeled_lines", len(output.Labels),
			"num_cover_pages", len(output.CoverPages),
			"num_blocks", len(blocks),
		)
	}
	return nil
}

type structurePageBlock struct {
	Index              int
	Lines              []structureLine
	IncludeLineNumbers []int
}

func buildStructureBlocks(lines []structureLine, inputBlockSize int) []structurePageBlock {
	if len(lines) == 0 {
		return nil
	}

	seenPages := map[int]struct{}{}
	pages := make([]int, 0, 32)
	linesByPage := make(map[int][]structureLine, 32)
	for _, line := range lines {
		if _, ok := seenPages[line.PageNumber]; !ok {
			seenPages[line.PageNumber] = struct{}{}
			pages = append(pages, line.PageNumber)
		}
		linesByPage[line.PageNumber] = append(linesByPage[line.PageNumber], line)
	}
	sort.Ints(pages)
	if inputBlockSize < 1 {
		inputBlockSize = 1
	}

	blocks := make([]structurePageBlock, 0, len(pages))
	start := 0
	end := min(start+inputBlockSize, len(pages))
	blockIndex := 1
	for start < len(pages) {
		pageSlice := pages[start:end]
		blockLines := make([]structureLine, 0, len(lines))
		includeLineNumbers := make([]int, 0, len(lines))
		for i, pageNo := range pageSlice {
			pageLines := linesByPage[pageNo]
			blockLines = append(blockLines, pageLines...)
			if blockIndex > 1 && i == 0 {
				continue
			}
			for _, line := range pageLines {
				includeLineNumbers = append(includeLineNumbers, line.LineNumber)
			}
		}
		blocks = append(blocks, structurePageBlock{
			Index:              blockIndex,
			Lines:              blockLines,
			IncludeLineNumbers: includeLineNumbers,
		})

		if end >= len(pages) {
			break
		}
		nextStart := end - 1
		nextContentStart := end
		nextEnd := min(nextContentStart+inputBlockSize, len(pages))
		start = nextStart
		end = nextEnd
		blockIndex++
	}

	return blocks
}

func (p *StructureAnalyzerProcessor) analyzeStructureBlock(ctx context.Context, lines []structureLine) (structureOutput, error) {
	var output structureOutput
	var validationErr error
	retryFeedback := ""
	maxAttempts := p.MaxRetries + 1
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		inputText := buildStructureUserInput(lines, retryFeedback)
		out, valErr := p.extractAndValidateStructureOutput(ctx, lines, inputText)
		if valErr == nil {
			output = out
			validationErr = nil
			break
		}
		validationErr = valErr
		if attempt < maxAttempts {
			retryFeedback = "schema issues: " + valErr.Error()
			continue
		}
	}
	if validationErr != nil {
		return structureOutput{}, fmt.Errorf("(MID_26042156) invalid llm output after retries: %w", validationErr)
	}
	return output, nil
}

func (p *StructureAnalyzerProcessor) extractAndValidateStructureOutput(ctx context.Context, lines []structureLine, inputText string) (structureOutput, error) {
	in := llmclients.JSONExtractionInput{
		PromptText: p.PromptText,
		ModelName:  p.ModelName,
		InputText:  inputText,
	}
	if textExtractor, ok := p.Extractor.(LLMTextExtractor); ok {
		rawText, err := textExtractor.ExtractText(ctx, in)
		if err != nil {
			return structureOutput{}, fmt.Errorf("(MID_26042101) structure llm request failed: %w", err)
		}
		return validateStructureTextOutput(rawText, lines)
	}

	// Backward-compatible fallback for extractors that only return JSON.
	parsed, err := p.Extractor.ExtractJSON(ctx, in)
	if err != nil {
		return structureOutput{}, fmt.Errorf("(MID_26042102) structure llm request failed: %w", err)
	}
	return validateStructureOutput(parsed, lines)
}

func (p *StructureAnalyzerProcessor) validateRequiredEnv() error {
	if strings.TrimSpace(p.StructureDir) == "" {
		return errors.New("(MID_26042425) missing PROMPT_DIR")
	}
	if strings.TrimSpace(p.ModelRef) == "" {
		return errors.New("(MID_26042426) missing STRUCTURE_MODEL_NAME")
	}
	if strings.TrimSpace(p.PromptRef) == "" {
		return errors.New("(MID_26042427) missing STRUCTURE_PROMPT")
	}
	return nil
}

type structureModelConfig struct {
	ModelName    string
	APIKey       string
	BaseURL      string
	TimeoutSec   int
	ThinkingType string
}

func loadStructureModelFromEnv() (modelRef string, modelPath string, cfg structureModelConfig, err error) {
	return loadModelConfigFromEnv("STRUCTURE_MODEL_NAME", "STRUCTURE_MODELS_FILE")
}

func loadModelConfigFromEnv(modelRefEnv string, modelsFileEnv string) (modelRef string, modelPath string, cfg structureModelConfig, err error) {
	modelRef = strings.TrimSpace(os.Getenv(modelRefEnv))
	if modelRef == "" {
		return "", "", structureModelConfig{}, fmt.Errorf("missing %s", modelRefEnv)
	}

	modelPath, err = resolveModelsFilePath(modelsFileEnv)
	if err != nil {
		return modelRef, "", structureModelConfig{}, err
	}

	raw, err := os.ReadFile(modelPath)
	if err != nil {
		return modelRef, modelPath, structureModelConfig{}, fmt.Errorf("(MID_26042103) read %s failed: %w", modelPath, err)
	}

	parsed := ApiTypes.LLMModelsFile{}
	if err := parseTOMLMap(raw, &parsed); err != nil {
		return modelRef, modelPath, structureModelConfig{}, fmt.Errorf("(MID_26042104) parse %s failed: %w", modelPath, err)
	}

	modelDef, ok := parsed[modelRef]
	if !ok {
		return modelRef, modelPath, structureModelConfig{}, fmt.Errorf("(MID_26042105) %s %q not found in %s", modelRefEnv, modelRef, modelPath)
	}
	if strings.TrimSpace(modelDef.ModelName) == "" {
		return modelRef, modelPath, structureModelConfig{}, fmt.Errorf("(MID_26042106) model %q in %s missing model_name", modelRef, modelPath)
	}
	cfg = structureModelConfig{
		ModelName:    strings.TrimSpace(modelDef.ModelName),
		APIKey:       strings.TrimSpace(modelDef.APIKey),
		BaseURL:      strings.TrimSpace(modelDef.BaseURL),
		TimeoutSec:   modelDef.TimeoutSec,
		ThinkingType: normalizeThinkingType(strings.TrimSpace(modelDef.ThinkingType)),
	}
	return modelRef, modelPath, cfg, nil
}

func resolveModelsFilePath(modelsFileEnv string) (string, error) {
	if override := strings.TrimSpace(os.Getenv(modelsFileEnv)); override != "" {
		if _, err := os.Stat(override); err != nil {
			return "", fmt.Errorf("(MID_26042107) %s %q is invalid: %w", modelsFileEnv, override, err)
		}
		return override, nil
	}
	if override := strings.TrimSpace(os.Getenv("MODELS_FILE")); override != "" {
		if _, err := os.Stat(override); err != nil {
			return "", fmt.Errorf("(MID_26042107) MODELS_FILE %q is invalid: %w", override, err)
		}
		return override, nil
	}

	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("(MID_26042108) getwd failed: %w", err)
	}

	cur := wd
	for {
		candidate := filepath.Join(cur, ".models.toml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}

	return "", fmt.Errorf(".models.toml not found; set %s or MODELS_FILE, or place .models.toml in the current directory tree", modelsFileEnv)
}

func parseTOMLMap(data []byte, out any) error {
	return toml.Unmarshal(data, out)
}

func applyStructureModelConfigToExtractor(extractor LLMJSONExtractor, cfg structureModelConfig) {
	client, ok := extractor.(*llmclients.OpenAIJSONClient)
	if !ok || client == nil {
		return
	}
	client.ModelName = strings.TrimSpace(cfg.ModelName)
	if v := strings.TrimSpace(cfg.APIKey); v != "" {
		client.APIKey = v
	}
	if v := strings.TrimSpace(cfg.BaseURL); v != "" {
		client.BaseURL = v
	}
	client.ThinkingType = normalizeThinkingType(cfg.ThinkingType)
	if cfg.TimeoutSec <= 0 {
		return
	}
	if client.HTTPClient == nil {
		client.HTTPClient = &http.Client{Timeout: time.Duration(cfg.TimeoutSec) * time.Second}
		return
	}
	client.HTTPClient.Timeout = time.Duration(cfg.TimeoutSec) * time.Second
}

func normalizeThinkingType(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "enabled", "disabled":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return ""
	}
}

func parseStructureLines(body []byte) ([]structureLine, int, error) {
	sc := bufio.NewScanner(strings.NewReader(string(body)))
	sc.Buffer(make([]byte, 1024), 16*1024*1024)
	lines := make([]structureLine, 0, 256)
	seenLineNumber := map[int]struct{}{}
	pageSet := map[int]struct{}{}
	rawLineNo := 0
	for sc.Scan() {
		rawLineNo++
		raw := strings.TrimSpace(sc.Text())
		if raw == "" {
			continue
		}
		parsed, err := parseLine(raw)
		if err != nil {
			return nil, 0, fmt.Errorf("(MID_26042109) line %d malformed: %w", rawLineNo, err)
		}
		if _, ok := seenLineNumber[parsed.LineNo]; ok {
			return nil, 0, fmt.Errorf("(MID_26042110) line %d malformed: duplicate line_number=%d", rawLineNo, parsed.LineNo)
		}
		seenLineNumber[parsed.LineNo] = struct{}{}
		pageSet[parsed.PageNo] = struct{}{}
		lines = append(lines, structureLine{
			LineNumber: parsed.LineNo,
			PageNumber: parsed.PageNo,
			LineType:   parsed.LineType,
			Font:       parsed.Font,
			FontSize:   parsed.FontSize,
			Coordinate: parsed.Coordinate,
			Content:    parsed.Content,
		})
	}
	if err := sc.Err(); err != nil {
		return nil, 0, err
	}
	return lines, len(pageSet), nil
}

func buildStructureUserInput(lines []structureLine, retryFeedback string) string {
	payload := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		payload = append(payload, map[string]any{
			"line_number": line.LineNumber,
			"page_number": line.PageNumber,
			"line_type":   line.LineType,
			"font":        line.Font,
			"font_size":   line.FontSize,
			"coordinate":  line.Coordinate,
			"content":     line.Content,
		})
	}
	bs, _ := json.Marshal(payload)
	var b strings.Builder
	b.WriteString("Output must be strict text.\n")
	b.WriteString("First line: cover_pages: [page-number...]\n")
	b.WriteString("Then one corrected line per line in format: <line_number>\\t<original_line_type>\\t<corrected_line_type>\\t<confidence>\\t<reason>\n")
	b.WriteString("Only output corrected lines. Do not output unchanged lines.\n")
	if strings.TrimSpace(retryFeedback) != "" {
		b.WriteString(retryFeedback)
		b.WriteByte('\n')
	}
	b.WriteString("Input ordered lines:\n")
	b.Write(bs)
	return b.String()
}

func validateStructureTextOutput(raw string, lines []structureLine) (structureOutput, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return structureOutput{}, errors.New("(MID_26042428) empty llm output")
	}

	all := strings.Split(trimmed, "\n")
	nonEmpty := make([]string, 0, len(all))
	for _, line := range all {
		t := strings.TrimSpace(line)
		if t != "" {
			nonEmpty = append(nonEmpty, t)
		}
	}
	if len(nonEmpty) == 0 {
		return structureOutput{}, errors.New("(MID_26042429) empty llm output")
	}
	if !strings.HasPrefix(strings.ToLower(nonEmpty[0]), "cover_pages:") {
		return structureOutput{}, errors.New("(MID_26042430) first output line must start with cover_pages:")
	}

	coverPages, err := parseCoverPagesLine(nonEmpty[0])
	if err != nil {
		return structureOutput{}, err
	}

	inputByLine := make(map[int]structureLine, len(lines))
	for _, line := range lines {
		inputByLine[line.LineNumber] = line
	}

	corrections := make(map[int]structureLabel, len(nonEmpty))
	for i := 1; i < len(nonEmpty); i++ {
		fields := strings.Split(nonEmpty[i], "\t")
		if len(fields) != 5 {
			return structureOutput{}, fmt.Errorf("(MID_26042111) correction line %d must have 5 tab-separated fields", i+1)
		}

		lineNumber, err := strconv.Atoi(strings.TrimSpace(fields[0]))
		if err != nil || lineNumber <= 0 {
			return structureOutput{}, fmt.Errorf("(MID_26042112) correction line %d has invalid line_number", i+1)
		}
		sourceLine, ok := inputByLine[lineNumber]
		if !ok {
			return structureOutput{}, fmt.Errorf("(MID_26042113) correction line %d references unknown line_number=%d", i+1, lineNumber)
		}
		if _, exists := corrections[lineNumber]; exists {
			return structureOutput{}, fmt.Errorf("(MID_26042114) duplicate correction for line_number=%d", lineNumber)
		}

		originalType := strings.ToLower(strings.TrimSpace(fields[1]))
		if originalType == "" {
			return structureOutput{}, fmt.Errorf("(MID_26042115) correction line %d has empty original_line_type", i+1)
		}
		if originalType != strings.ToLower(strings.TrimSpace(sourceLine.LineType)) {
			return structureOutput{}, fmt.Errorf("(MID_26042116) correction line %d original_line_type mismatch for line_number=%d", i+1, lineNumber)
		}

		correctedType := strings.ToLower(strings.TrimSpace(fields[2]))
		if !isValidCorrectedLineType(correctedType) {
			return structureOutput{}, fmt.Errorf("(MID_26042117) correction line %d has invalid corrected_line_type=%q", i+1, correctedType)
		}
		if correctedType == originalType {
			return structureOutput{}, fmt.Errorf("(MID_26042118) correction line %d must not include unchanged line_number=%d", i+1, lineNumber)
		}

		confidence, err := strconv.ParseFloat(strings.TrimSpace(fields[3]), 64)
		if err != nil || confidence < 0 || confidence > 1 {
			return structureOutput{}, fmt.Errorf("(MID_26042119) correction line %d has invalid confidence", i+1)
		}

		reason := strings.TrimSpace(fields[4])
		if reason == "" {
			return structureOutput{}, fmt.Errorf("(MID_26042120) correction line %d has empty reason", i+1)
		}

		corrections[lineNumber] = structureLabel{
			LineNumber:        lineNumber,
			CorrectedLineType: correctedType,
			Confidence:        confidence,
			Reason:            reason,
		}
	}

	labels := make([]structureLabel, 0, len(lines))
	for _, line := range lines {
		if corrected, ok := corrections[line.LineNumber]; ok {
			labels = append(labels, corrected)
			continue
		}
		labels = append(labels, structureLabel{
			LineNumber:        line.LineNumber,
			CorrectedLineType: strings.ToLower(strings.TrimSpace(line.LineType)),
			Confidence:        1.0,
			Reason:            "unchanged",
		})
	}

	return structureOutput{
		CoverPages: coverPages,
		Labels:     labels,
	}, nil
}

func parseCoverPagesLine(line string) ([]int, error) {
	const prefix = "cover_pages:"
	if !strings.HasPrefix(strings.ToLower(line), prefix) {
		return nil, errors.New("(MID_26042431) cover_pages line missing prefix")
	}
	rest := strings.TrimSpace(line[len(prefix):])
	if rest == "" {
		return []int{}, nil
	}
	if !strings.HasPrefix(rest, "[") || !strings.HasSuffix(rest, "]") {
		return nil, errors.New("(MID_26042432) cover_pages must be bracketed list, e.g. [1,2]")
	}
	var pages []int
	if err := json.Unmarshal([]byte(rest), &pages); err != nil {
		return nil, fmt.Errorf("(MID_26042121) cover_pages invalid: %w", err)
	}
	seen := map[int]struct{}{}
	out := make([]int, 0, len(pages))
	for i, p := range pages {
		if p <= 0 {
			return nil, fmt.Errorf("(MID_26042122) cover_pages[%d] invalid: must be positive integer", i)
		}
		if _, exists := seen[p]; exists {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	sort.Ints(out)
	return out, nil
}

func validateStructureOutput(parsed map[string]any, lines []structureLine) (structureOutput, error) {
	rawLabels, ok := parsed["labels"].([]any)
	if !ok {
		return structureOutput{}, errors.New("(MID_26042433) labels must be an array")
	}

	labels := make([]structureLabel, 0, len(rawLabels))
	labelByLine := make(map[int]structureLabel, len(rawLabels))
	for i, item := range rawLabels {
		row, ok := item.(map[string]any)
		if !ok {
			return structureOutput{}, fmt.Errorf("(MID_26042123) labels[%d] must be an object", i)
		}
		lineNumber, err := parseStructurePositiveInt(row["line_number"])
		if err != nil {
			return structureOutput{}, fmt.Errorf("(MID_26042124) labels[%d].line_number invalid: %w", i, err)
		}
		if _, exists := labelByLine[lineNumber]; exists {
			return structureOutput{}, fmt.Errorf("(MID_26042125) duplicate label line_number: %d", lineNumber)
		}
		correctedType := strings.TrimSpace(asString(row["corrected_line_type"]))
		if !isValidCorrectedLineType(correctedType) {
			return structureOutput{}, fmt.Errorf("(MID_26042126) labels[%d].corrected_line_type invalid: %q", i, correctedType)
		}
		confidence := toFloat(row["confidence"])
		if confidence < 0 || confidence > 1 {
			return structureOutput{}, fmt.Errorf("(MID_26042127) labels[%d].confidence must be in [0,1]", i)
		}
		reason := strings.TrimSpace(asString(row["reason"]))
		if reason == "" {
			return structureOutput{}, fmt.Errorf("(MID_26042128) labels[%d].reason is required", i)
		}
		label := structureLabel{
			LineNumber:        lineNumber,
			CorrectedLineType: correctedType,
			Confidence:        confidence,
			Reason:            reason,
		}
		labels = append(labels, label)
		labelByLine[lineNumber] = label
	}

	if len(labels) != len(lines) {
		return structureOutput{}, fmt.Errorf("(MID_26042129) labels count=%d does not match input lines=%d", len(labels), len(lines))
	}
	for _, line := range lines {
		if _, ok := labelByLine[line.LineNumber]; !ok {
			return structureOutput{}, fmt.Errorf("(MID_26042130) missing label for line_number=%d", line.LineNumber)
		}
	}

	coverPages, err := normalizeCoverPages(parsed["cover_pages"])
	if err != nil {
		return structureOutput{}, err
	}

	return structureOutput{
		CoverPages: coverPages,
		Labels:     labels,
	}, nil
}

func normalizeCoverPages(v any) ([]int, error) {
	if v == nil {
		return []int{}, nil
	}
	items, ok := v.([]any)
	if !ok {
		return nil, errors.New("(MID_26042434) cover_pages must be an array")
	}
	seen := map[int]struct{}{}
	out := make([]int, 0, len(items))
	for i, item := range items {
		p, err := parseStructurePositiveInt(item)
		if err != nil {
			return nil, fmt.Errorf("(MID_26042131) cover_pages[%d] invalid: %w", i, err)
		}
		if _, exists := seen[p]; exists {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	sort.Ints(out)
	return out, nil
}

func parseStructurePositiveInt(v any) (int, error) {
	switch x := v.(type) {
	case float64:
		if x <= 0 || float64(int(x)) != x {
			return 0, fmt.Errorf("(MID_26042132) must be positive integer")
		}
		return int(x), nil
	case int:
		if x <= 0 {
			return 0, fmt.Errorf("(MID_26042133) must be positive integer")
		}
		return x, nil
	case int64:
		if x <= 0 {
			return 0, fmt.Errorf("(MID_26042134) must be positive integer")
		}
		return int(x), nil
	case string:
		s := strings.TrimSpace(x)
		n, err := strconv.Atoi(s)
		if err != nil || n <= 0 {
			return 0, fmt.Errorf("(MID_26042135) must be positive integer")
		}
		return n, nil
	default:
		return 0, fmt.Errorf("(MID_26042136) must be positive integer")
	}
}

func isValidCorrectedLineType(v string) bool {
	s := strings.ToLower(strings.TrimSpace(v))
	if s == "" {
		return false
	}
	if strings.HasPrefix(s, "heading-") {
		lvl, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(s, "heading-")))
		return err == nil && lvl > 0
	}
	switch s {
	case "paragraph", "list-item", "table", "formula", "toc", "footer", "cover", "other":
		return true
	default:
		return false
	}
}

func (p *StructureAnalyzerProcessor) writeStructureArtifacts(recordID int64, lines []structureLine, out structureOutput) error {
	groupID := recordID / 1000
	runDir := filepath.Join(p.StructureDir, strconv.FormatInt(groupID, 10), strconv.FormatInt(recordID, 10))
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return fmt.Errorf("(MID_26042137) create run dir: %w", err)
	}

	labelByLine := make(map[int]structureLabel, len(out.Labels))
	for _, label := range out.Labels {
		labelByLine[label.LineNumber] = label
	}

	labelPath := filepath.Join(runDir, "structure_labels.jsonl")
	var labelBuilder strings.Builder
	for _, line := range lines {
		lbl, ok := labelByLine[line.LineNumber]
		if !ok {
			return fmt.Errorf("(MID_26042138) internal error: missing label for line_number=%d", line.LineNumber)
		}
		row := map[string]any{
			"line_number":         line.LineNumber,
			"page_number":         line.PageNumber,
			"original_line_type":  line.LineType,
			"corrected_line_type": lbl.CorrectedLineType,
			"confidence":          lbl.Confidence,
			"reason":              lbl.Reason,
		}
		bs, err := json.Marshal(row)
		if err != nil {
			return fmt.Errorf("(MID_26042139) marshal labels jsonl row: %w", err)
		}
		labelBuilder.Write(bs)
		labelBuilder.WriteByte('\n')
	}
	if err := os.WriteFile(labelPath, []byte(strings.TrimRight(labelBuilder.String(), "\n")), 0o644); err != nil {
		return fmt.Errorf("(MID_26042140) write structure_labels.jsonl: %w", err)
	}

	summary := map[string]any{
		"record_id":         recordID,
		"cover_pages":       out.CoverPages,
		"num_pages":         countUniquePages(lines),
		"num_lines":         len(lines),
		"num_labeled_lines": len(out.Labels),
		"model":             p.ModelName,
	}
	summaryPath := filepath.Join(runDir, "structure_summary.json")
	bs, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return fmt.Errorf("(MID_26042141) marshal structure summary: %w", err)
	}
	if err := os.WriteFile(summaryPath, bs, 0o644); err != nil {
		return fmt.Errorf("(MID_26042142) write structure_summary.json: %w", err)
	}
	return nil
}

func countUniquePages(lines []structureLine) int {
	seen := map[int]struct{}{}
	for _, line := range lines {
		if line.PageNumber > 0 {
			seen[line.PageNumber] = struct{}{}
		}
	}
	return len(seen)
}

type structureStatusParams struct {
	InputFilename   string
	NumPages        int
	NumLines        int
	NumLabeledLines int
	NumCoverPages   int
	Start           time.Time
	DurationMs      int64
	ProcErr         error
}

func appendStructureStatus(raw string, p structureStatusParams) (string, error) {
	entries := decodeDocMetaStatus(raw)
	entry := map[string]any{
		"operation":         "structure_analyzer",
		"input_filename":    strings.TrimSpace(p.InputFilename),
		"num_pages":         p.NumPages,
		"num_lines":         p.NumLines,
		"num_labeled_lines": p.NumLabeledLines,
		"num_cover_pages":   p.NumCoverPages,
		"start_time":        p.Start.Format(defaultDocMetaStatusTime),
		"ms_used":           p.DurationMs,
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
		if op != "structure_analyzer" && op != "structure-analyzer" {
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

func (p *StructureAnalyzerProcessor) failAndPersist(
	ctx context.Context,
	rec DocMetadataInputRecord,
	start time.Time,
	inputFilename string,
	numPages int,
	numLines int,
	numLabeledLines int,
	numCoverPages int,
	procErr error,
) error {
	statusRaw, err := appendStructureStatus(rec.StatusRaw, structureStatusParams{
		InputFilename:   inputFilename,
		NumPages:        numPages,
		NumLines:        numLines,
		NumLabeledLines: numLabeledLines,
		NumCoverPages:   numCoverPages,
		Start:           start,
		DurationMs:      time.Since(start).Milliseconds(),
		ProcErr:         procErr,
	})
	if err != nil {
		return fmt.Errorf("(MID_26042143) append structure status: %w", err)
	}
	errMsg := strings.TrimSpace(procErr.Error())
	if err := p.Store.UpdateInputMetadata(ctx, rec.ID, DocMetadataUpdate{
		StatusRaw: statusRaw,
		ErrorMsg:  &errMsg,
	}); err != nil {
		return fmt.Errorf("(MID_26042144) persist failure status: %w", err)
	}
	return procErr
}

func loadStructurePromptFromEnv() (promptText string, promptRef string, promptPath string, promptErr error) {
	promptRef = strings.TrimSpace(os.Getenv("STRUCTURE_PROMPT"))
	if promptRef == "" {
		return "", "", "", errors.New("(MID_26042435) missing STRUCTURE_PROMPT")
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
			return "", promptRef, candidate, errors.New("(MID_26042436) prompt file is empty")
		}
		return text, promptRef, candidate, nil
	}

	if strings.Contains(promptRef, "\n") || strings.Contains(promptRef, " ") {
		return strings.TrimSpace(promptRef), "inline", "", nil
	}
	if lastErr == nil {
		lastErr = errors.New("(MID_26042437) no candidate path available")
	}
	return "", promptRef, "", fmt.Errorf("(MID_26042145) prompt file not found: %w", lastErr)
}
