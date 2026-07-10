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
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

var (
	staticNumericHeadingRE  = regexp.MustCompile(`^([0-9OoSsLl]+(?:\s*\.\s*[0-9OoSsLl]+)*)\s*\.?\s*(.*)$`)
	staticAppendixHeadingRE = regexp.MustCompile(`^([A-Za-z])\s*\.\s*([0-9OoSsLl]+(?:\s*\.\s*[0-9OoSsLl]+)*)\s*\.?\s*(.*)$`)
	staticNumListRE         = regexp.MustCompile(`^\d+[\.)]?\s+\S+`)
	staticSingleSymListRE   = regexp.MustCompile(`^[*\-•·—]\s+\S+`)
	staticMultiSymListRE    = regexp.MustCompile(`^([A-Za-z]+|[ivxlcdmIVXLCDM]+)\)\s+\S+`)
	staticTOCDotLeaderRE    = regexp.MustCompile(`\.{3,}`)
	staticPageImagePathRE   = regexp.MustCompile(`^(.+)/imageFile(\d+)\.png$`)
	staticLegacyHeadingRE   = regexp.MustCompile(`^heading\((\d+)\)$`)
	// matches 6 or more consecutive \dots (each separated by optional spaces/tabs)
	staticDotsSeqRE = regexp.MustCompile(`\\dots(?:[ \t]*\\dots){5,}`)
)

type StaticAnalyzerProcessor struct {
	Store          DocMetadataStore
	Logger         ApiTypes.JimoLogger
	ProcLogger     DocProcLogger
	Now            func() time.Time
	ArtifactDir    string
	OverrideOrigin bool
	TOCDetector    *staticTOCDetector
}

// staticTOCDetector holds the LLM client and configuration for LLM-based TOC line detection.
// When nil or not ready, detection falls back to static pattern matching.
type staticTOCDetector struct {
	Client            LLMJSONExtractor
	PromptText        string
	PromptRef         string
	PromptErr         error
	ModelName         string
	ModelCfg          structureModelConfig
	FallbackModelName string
	FallbackModelCfg  structureModelConfig
	FallbackModelErr  error
	NumPages          int
	FallbackPages     int
	Logger            ApiTypes.JimoLogger
	ProcLogger        *DocProcLogger
	mu                sync.Mutex
	primaryRetryAfter time.Time
}

func (d *staticTOCDetector) isReady() bool {
	return d != nil && d.Client != nil && d.PromptErr == nil && d.ModelName != ""
}

func tocProviderCooldown() time.Duration {
	secs := envInt("DETECT_TOC_PROVIDER_COOLDOWN_SEC", 300, 1)
	return time.Duration(secs) * time.Second
}

func isProviderCooldownError(err error) bool {
	if err == nil {
		return false
	}
	if !errors.Is(err, llmclients.ErrStructuredOutputProvider) {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(msg, "status 402") ||
		strings.Contains(msg, "insufficient balance") ||
		strings.Contains(msg, "status 429") ||
		strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "too many requests")
}

func (d *staticTOCDetector) primaryRetryDeadline(now time.Time) time.Time {
	if d == nil {
		return time.Time{}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.primaryRetryAfter.After(now) {
		return d.primaryRetryAfter
	}
	return time.Time{}
}

func (d *staticTOCDetector) markPrimaryRetryCooldown(now time.Time, err error) time.Time {
	if d == nil || !isProviderCooldownError(err) {
		return time.Time{}
	}
	deadline := now.Add(tocProviderCooldown())
	d.mu.Lock()
	if deadline.After(d.primaryRetryAfter) {
		d.primaryRetryAfter = deadline
	}
	deadline = d.primaryRetryAfter
	d.mu.Unlock()
	return deadline
}

type staticInputLine struct {
	LineNo            int
	PageNo            int
	OriginalLineType  string
	OriginalLineLower string
	Font              string
	FontSize          string
	Coordinate        string
	Content           string
}

type staticAnalyzeResult struct {
	Lines         []staticInputLine
	CorrectedType map[int]string
	NumPages      int
	NumLines      int
	OutputChanged bool
}

type staticStatusParams struct {
	RecordID        int64
	FileType        string
	InputFilename   string
	NumPages        int
	NumLines        int
	NumLabeledLines int
	Start           time.Time
	DurationMs      int64
	ProcErr         error
}

func NewStaticAnalyzerProcessor(store DocMetadataStore, client LLMJSONExtractor, logger ApiTypes.JimoLogger) *StaticAnalyzerProcessor {
	if logger == nil {
		logger = loggerutil.CreateDefaultLogger("MID_26042301")
	}
	extractPrompt := strings.TrimSpace(os.Getenv("EXTRACT_DOCMETA_PROMPT"))

	var tocDetector *staticTOCDetector
	if client != nil {
		promptText, promptRef, _, promptErr := loadTOCDetectPromptFromEnv()
		modelRef, _, modelCfg, modelErr := loadModelConfigFromEnv("EXTRACT_DOCMETA_MODEL_NAME", "EXTRACT_DOCMETA_MODELS_FILE")
		fallbackRef, _, fallbackCfg, fallbackErr := loadOptionalModelConfigFromEnv("EXTRACT_DOCMETA_MODEL_FALLBACK", "EXTRACT_DOCMETA_MODELS_FILE")
		_ = modelRef
		_ = fallbackRef
		if modelErr != nil {
			promptErr = modelErr
		}
		pl := DocProcLogger{DB: ApiTypes.ProjectDBHandle}
		tocDetector = &staticTOCDetector{
			Client:            client,
			PromptText:        promptText,
			PromptRef:         promptRef,
			PromptErr:         promptErr,
			ModelName:         modelCfg.ModelName,
			ModelCfg:          modelCfg,
			FallbackModelName: fallbackCfg.ModelName,
			FallbackModelCfg:  fallbackCfg,
			FallbackModelErr:  fallbackErr,
			NumPages:          envInt("DETECT_TOC_NUM_PAGES", 3, 1),
			FallbackPages:     envInt("DETECT_TOC_FALLBACK_PAGES", 5, 1),
			Logger:            logger,
			ProcLogger:        &pl,
		}
	}

	return &StaticAnalyzerProcessor{
		Store:          store,
		Logger:         logger,
		ProcLogger:     DocProcLogger{DB: ApiTypes.ProjectDBHandle},
		Now:            time.Now,
		ArtifactDir:    strings.TrimSpace(os.Getenv("ARTIFACT_DIR")),
		OverrideOrigin: strings.ToLower(extractPrompt) != "false",
		TOCDetector:    tocDetector,
	}
}

func (p *StaticAnalyzerProcessor) Name() string { return "static_analyzer" }

func (p *StaticAnalyzerProcessor) HandleEvent(ctx context.Context, payload []byte) error {
	start := p.now()
	if p.Logger != nil {
		p.Logger.Info("static analyzer invoked", "payload_bytes", len(payload))
	}
	evt, err := ParseLineFileGeneratedEvent(payload)
	if err != nil {
		return fmt.Errorf("(MID_26042302) parse event payload: %w", err)
	}
	ctx = withLLMRecordID(ctx, evt.RecordID)
	if ShouldSkipLineFileGeneratedEvent(evt) {
		return nil
	}
	if !p.OverrideOrigin && strings.TrimSpace(p.ArtifactDir) == "" {
		return errors.New("(MID_26050814) missing ARTIFACT_DIR")
	}
	if p.Store == nil {
		return errors.New("(MID_26050815) doc metadata store is nil")
	}

	rec, err := p.Store.GetInputRecord(ctx, evt.RecordID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("(MID_26042303) kb.inputs record not found: %d", evt.RecordID)
		}
		return fmt.Errorf("(MID_26042304) load kb.inputs record %d: %w", evt.RecordID, err)
	}
	if strings.TrimSpace(rec.ParserName) == "" {
		return p.failAndPersist(ctx, rec, start, "", 0, 0, 0, errors.New("(MID_26050816) missing parser name"))
	}
	if strings.TrimSpace(rec.ResultFilename) == "" {
		return p.failAndPersist(ctx, rec, start, "", 0, 0, 0, errors.New("(MID_26050817) missing result filename"))
	}

	inputPath, err := ResolveInputFilePath(evt, rec.ResultFilename, rec.ParserName, rec.StagingFilename)
	if err != nil {
		return p.failAndPersist(ctx, rec, start, "", 0, 0, 0, err)
	}
	inputFilename := filepath.Base(strings.TrimSpace(inputPath))
	if strings.TrimSpace(evt.Filename) != "" {
		inputFilename = filepath.Base(strings.TrimSpace(evt.Filename))
	}
	body, err := os.ReadFile(inputPath)
	if err != nil {
		return p.failAndPersist(ctx, rec, start, inputFilename, 0, 0, 0, fmt.Errorf("(MID_26042305) read input file: %w", err))
	}

	out, err := analyzeStaticStructureCtx(ctx, body, p.Logger, p.TOCDetector, &rec.ID)
	if err != nil {
		return p.failAndPersist(ctx, rec, start, inputFilename, 0, 0, 0, err)
	}
	if err := p.writeCorrectedArtifact(rec.ID, inputFilename, inputPath, out); err != nil {
		return p.failAndPersist(ctx, rec, start, inputFilename, out.NumPages, out.NumLines, len(out.Lines), err)
	}

	statusRaw, err := appendStaticAnalyzerStatus(rec.StatusRaw, staticStatusParams{
		RecordID:        rec.ID,
		FileType:        detectStaticStatusFileType(rec, inputFilename),
		InputFilename:   inputFilename,
		NumPages:        out.NumPages,
		NumLines:        out.NumLines,
		NumLabeledLines: len(out.Lines),
		Start:           start,
		DurationMs:      time.Since(start).Milliseconds(),
		ProcErr:         nil,
	})
	if err != nil {
		return fmt.Errorf("(MID_26042306) append static analyzer status: %w", err)
	}
	if err := p.Store.UpdateInputMetadata(ctx, rec.ID, DocMetadataUpdate{
		StatusRaw: statusRaw,
		ErrorMsg:  nil,
	}); err != nil {
		return fmt.Errorf("(MID_26042307) update kb.inputs status: %w", err)
	}
	p.logSummary(ctx, rec.ID, start, p.Now(), out.NumPages, out.NumLines, len(out.CorrectedType), nil)
	return nil
}

func (p *StaticAnalyzerProcessor) logSummary(ctx context.Context, recordID int64, start, end time.Time, numPages, numLines, numLabeledLines int, procErr error) {
	if p.ProcLogger.DB == nil {
		return
	}
	extraInfo, _ := json.Marshal(map[string]any{
		"num_lines":         numLines,
		"num_pages":         numPages,
		"num_labeled_lines": numLabeledLines,
	})
	extraStr := string(extraInfo)
	var errStr *string
	if procErr != nil {
		s := procErr.Error()
		errStr = &s
	}
	if err := p.ProcLogger.LogStaticAnalyzer(ctx, DocProcLogRecord{
		DocProcName:   p.Name(),
		ModelNames:    []string{},
		PromptName:    "",
		RecordID:      int64Ptr(recordID),
		ExtraInfoJSON: &extraStr,
		Errors:        errStr,
		MSUsed:        int64Ptr(end.Sub(start).Milliseconds()),
	}, "MID-26052851"); err != nil {
		p.Logger.Warn("failed to write static_analyzer log", "error", err)
	}
}

func (p *StaticAnalyzerProcessor) writeCorrectedArtifact(recordID int64, inputFilename string, inputPath string, out staticAnalyzeResult) error {
	if p.OverrideOrigin {
		if !out.OutputChanged {
			return nil
		}
	}

	var filePath string
	if p.OverrideOrigin {
		filePath = inputPath
	} else {
		groupID := recordID / 1000
		runDir := filepath.Join(p.ArtifactDir, strconv.FormatInt(groupID, 10), strconv.FormatInt(recordID, 10))
		if err := os.MkdirAll(runDir, 0o755); err != nil {
			return fmt.Errorf("(MID_26042308) create run dir: %w", err)
		}
		root := strings.TrimSuffix(filepath.Base(strings.TrimSpace(inputFilename)), filepath.Ext(strings.TrimSpace(inputFilename)))
		if root == "" {
			root = "result"
		}
		filePath = filepath.Join(runDir, root+".corrected")
	}

	var b strings.Builder
	for i, line := range out.Lines {
		corrected := strings.TrimSpace(out.CorrectedType[line.LineNo])
		if corrected == "" {
			corrected = "unchanged"
		}
		// Re-number sequentially; deletions earlier in the pipeline leave gaps in the original LineNo.
		outputLineNo := strconv.Itoa(i + 1)
		var row []string
		if p.OverrideOrigin {
			lineType := line.OriginalLineType
			if corrected != "unchanged" {
				lineType = corrected
			}
			row = []string{
				outputLineNo,
				strconv.Itoa(line.PageNo),
				lineType,
				line.Font,
				line.FontSize,
				line.Coordinate,
				line.Content,
			}
		} else {
			row = []string{
				outputLineNo,
				strconv.Itoa(line.PageNo),
				line.OriginalLineType,
				corrected,
				line.Font,
				line.FontSize,
				line.Coordinate,
				line.Content,
			}
		}
		b.WriteString(strings.Join(row, "\t"))
		if i < len(out.Lines)-1 {
			b.WriteByte('\n')
		}
	}
	if err := writeAtomicFile(filePath, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("(MID_26042309) write corrected file: %w", err)
	}
	if p.Logger != nil {
		p.Logger.Info("static analyzer output written", "output_path", filePath, "line_count", len(out.Lines), "override_origin", p.OverrideOrigin)
	}
	// Re-numbering changes line numbers, so any .chunks artifacts built from the
	// old numbering are now stale. Delete them so the next chunking run rebuilds them.
	if p.OverrideOrigin {
		p.deleteStaleChunkArtifacts(inputPath, recordID)
	}
	return nil
}

func (p *StaticAnalyzerProcessor) deleteStaleChunkArtifacts(inputPath string, recordID int64) {
	recordDirs := staleChunkCandidateDirs(inputPath, p.ArtifactDir, recordID)
	seen := make(map[string]struct{}, len(recordDirs))
	for _, recordDir := range recordDirs {
		recordDir = strings.TrimSpace(recordDir)
		if recordDir == "" {
			continue
		}
		recordDir = filepath.Clean(recordDir)
		if _, ok := seen[recordDir]; ok {
			continue
		}
		seen[recordDir] = struct{}{}
		entries, err := os.ReadDir(recordDir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".chunks") {
				continue
			}
			path := filepath.Join(recordDir, entry.Name())
			if removeErr := os.Remove(path); removeErr != nil {
				if p.Logger != nil {
					p.Logger.Warn("failed to remove stale chunk artifact", "path", path, "error", removeErr)
				}
			} else if p.Logger != nil {
				p.Logger.Info("removed stale chunk artifact", "path", path)
			}
		}
	}
}

func staleChunkCandidateDirs(inputPath, artifactDir string, recordID int64) []string {
	dirs := make([]string, 0, 2)
	if dir := strings.TrimSpace(filepath.Dir(strings.TrimSpace(inputPath))); dir != "" && dir != "." {
		dirs = append(dirs, dir)
	}
	if artifactDir = strings.TrimSpace(artifactDir); artifactDir != "" && recordID > 0 {
		groupID := recordID / 1000
		dirs = append(dirs, filepath.Join(artifactDir, strconv.FormatInt(groupID, 10), strconv.FormatInt(recordID, 10)))
	}
	return dirs
}

func writeAtomicFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		_ = tmp.Close()
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, perm); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

// analyzeStaticStructure is the main entry for pure-static analysis (no LLM). Tests use this.
func analyzeStaticStructure(body []byte, logger ApiTypes.JimoLogger) (staticAnalyzeResult, error) {
	return analyzeStaticStructureCtx(context.Background(), body, logger, nil, nil)
}

// analyzeStaticStructureCtx is the full implementation. When toc is ready, LLM detects TOC lines;
// otherwise falls back to static pattern matching.
func analyzeStaticStructureCtx(ctx context.Context, body []byte, logger ApiTypes.JimoLogger, toc *staticTOCDetector, recordID *int64) (staticAnalyzeResult, error) {
	if logger == nil {
		logger = loggerutil.CreateDefaultLogger("MID_26051001")
	}
	sc := bufio.NewScanner(strings.NewReader(string(body)))
	sc.Buffer(make([]byte, 1024), 16*1024*1024)

	lines := make([]staticInputLine, 0, 256)
	seenLineNo := make(map[int]struct{}, 256)
	pageSet := make(map[int]struct{}, 64)
	numLines := 0
	for sc.Scan() {
		raw := strings.TrimSpace(sc.Text())
		if raw == "" {
			continue
		}
		numLines++
		line, err := parseStaticInputLine(raw)
		if err != nil {
			logger.Warn("skipping malformed input line", "Error", err, "raw", raw)
			continue
		}
		if _, exists := seenLineNo[line.LineNo]; exists {
			continue
		}
		seenLineNo[line.LineNo] = struct{}{}
		if line.PageNo > 0 {
			pageSet[line.PageNo] = struct{}{}
		}
		lines = append(lines, line)
	}
	if err := sc.Err(); err != nil {
		return staticAnalyzeResult{}, fmt.Errorf("(MID_26042310) read input lines: %w", err)
	}
	lines = applyStaticRemoveFullPageImageArtifactLines(lines, logger)
	lines = applyStaticRemoveWeBoosWatermarkLines(lines, logger)
	lines = applyStaticTruncateDots(lines, logger)
	lines = applyStaticCorrectHeadings(lines, logger)

	corrected := make(map[int]string, len(lines))
	for _, line := range lines {
		corrected[line.LineNo] = "unchanged"
	}

	logger.Info("detect table of content invoked", "line_count", len(lines))
	var lastTOC int
	if toc.isReady() {
		lastTOC = applyLLMTOCLabels(ctx, lines, corrected, toc, recordID)
	} else {
		lastTOC = applyStaticTOCLabels(lines, corrected, logger)
	}
	applyStaticDetectHeadings(lastTOC, lines, corrected, logger)
	lines = applyStaticMergeLines(lines, corrected, logger)
	applyStaticDetectItemLists(lines, corrected, logger)

	outputChanged := len(lines) != len(seenLineNo)
	if !outputChanged {
		for _, line := range lines {
			if strings.TrimSpace(corrected[line.LineNo]) != "unchanged" {
				outputChanged = true
				break
			}
		}
	}

	return staticAnalyzeResult{
		Lines:         lines,
		CorrectedType: corrected,
		NumPages:      len(pageSet),
		NumLines:      numLines,
		OutputChanged: outputChanged,
	}, nil
}

// applyLLMTOCLabels uses the LLM to detect TOC lines and updates the corrected map.
// It falls back to static detection if the LLM call fails.
func applyLLMTOCLabels(ctx context.Context, lines []staticInputLine, corrected map[int]string, toc *staticTOCDetector, recordID *int64) int {
	_, firstTOC := findStaticTOCHint(lines)

	// Collect the page range to send to the LLM.
	pageSet := make(map[int]struct{}, toc.NumPages)
	if firstTOC >= 0 {
		startPage := lines[firstTOC].PageNo
		for p := startPage; p < startPage+toc.NumPages; p++ {
			pageSet[p] = struct{}{}
		}
	} else {
		for p := 1; p <= toc.FallbackPages; p++ {
			pageSet[p] = struct{}{}
		}
	}

	var targetLines []staticInputLine
	for _, line := range lines {
		if _, ok := pageSet[line.PageNo]; ok {
			targetLines = append(targetLines, line)
		}
	}
	if len(targetLines) == 0 {
		return -1
	}

	callID := fmt.Sprintf("%s_detect_toc", eventIDFromContext(ctx))
	startTime := time.Now()
	inputJSON := staticInputLinesToJSON(targetLines)
	tocLineNums, usedModel, err := toc.detectTOCLines(ctx, inputJSON)
	elapsed := time.Since(startTime)

	logTOCDetectionCall(ctx, toc, callID, usedModel, recordID, tocLineNums, err, elapsed)

	if err != nil {
		toc.Logger.Warn("LLM TOC detection failed, falling back to static detection",
			"error", err,
			"prompt", toc.PromptRef,
		)
		return applyStaticTOCLabels(lines, corrected, toc.Logger)
	}

	if len(tocLineNums) == 0 {
		toc.Logger.Info("LLM TOC detection: no TOC lines found", "model", usedModel)
		return -1
	}

	toc.Logger.Info("LLM TOC detection succeeded",
		"model", usedModel,
		"toc_line_count", len(tocLineNums),
		"toc_lines", tocLineNums,
		"ms_used", elapsed.Milliseconds(),
	)

	lineNoSet := make(map[int]struct{}, len(tocLineNums))
	for _, n := range tocLineNums {
		lineNoSet[n] = struct{}{}
	}

	lastIdx := -1
	for i, line := range lines {
		if _, ok := lineNoSet[line.LineNo]; ok {
			corrected[line.LineNo] = "toc"
			lastIdx = i
		}
	}
	return lastIdx
}

// findStaticTOCHint returns the index of the TOC title line (start) and
// the index of the first actual TOC entry (firstTOC) using static patterns.
// Returns (-1, -1) if no TOC title is found.
func findStaticTOCHint(lines []staticInputLine) (start, firstTOC int) {
	start = -1
	tocPage := 0
	for i, line := range lines {
		n := normalizeStaticTitle(line.Content)
		if n == "tableofcontent" || n == "tableofcontents" || n == "目录" || n == "目次" {
			start = i
			tocPage = line.PageNo
			break
		}
	}
	if start < 0 {
		return -1, -1
	}
	firstTOC = -1
	for i := start + 1; i < len(lines); i++ {
		if lines[i].PageNo != tocPage {
			break
		}
		if isStaticTOCLine(lines[i]) {
			firstTOC = i
			break
		}
	}
	return start, firstTOC
}

// detectTOCLines calls the LLM (with fallback) and returns the line numbers identified as TOC.
func (d *staticTOCDetector) detectTOCLines(ctx context.Context, inputJSON string) ([]int, string, error) {
	now := time.Now()
	if retryAfter := d.primaryRetryDeadline(now); !retryAfter.IsZero() {
		if d.FallbackModelName != "" && d.FallbackModelErr == nil {
			if d.Logger != nil {
				d.Logger.Info("skipping primary TOC LLM call during provider cooldown",
					"primary_model", d.ModelName,
					"fallback_model", d.FallbackModelName,
					"retry_after", retryAfter.UTC().Format(time.RFC3339),
				)
			}
			result, fallbackErr := d.callLLM(ctx, d.FallbackModelName, d.FallbackModelCfg, inputJSON)
			if fallbackErr != nil {
				return nil, d.FallbackModelName, fmt.Errorf("(MID_26061104) TOC primary in cooldown until %s; fallback failed: %v", retryAfter.UTC().Format(time.RFC3339), fallbackErr)
			}
			return parseTOCDetectionResult(result), d.FallbackModelName, nil
		}
		return nil, d.ModelName, fmt.Errorf("(MID_26061105) TOC primary model %q is in provider cooldown until %s", d.ModelName, retryAfter.UTC().Format(time.RFC3339))
	}

	result, err := d.callLLM(ctx, d.ModelName, d.ModelCfg, inputJSON)
	if err == nil {
		return parseTOCDetectionResult(result), d.ModelName, nil
	}

	if d.FallbackModelName == "" || d.FallbackModelErr != nil {
		return nil, d.ModelName, fmt.Errorf("(MID_26061101) TOC LLM detection failed and fallback not configured: %w, result:%s", err, result)
	}

	d.Logger.Warn("primary TOC LLM call failed, trying fallback",
		"primary_model", d.ModelName,
		"fallback_model", d.FallbackModelName,
		"error", err,
	)
	retryAfter := d.markPrimaryRetryCooldown(now, err)
	if !retryAfter.IsZero() && d.Logger != nil {
		d.Logger.Warn("primary TOC LLM provider unavailable; enabling cooldown",
			"primary_model", d.ModelName,
			"retry_after", retryAfter.UTC().Format(time.RFC3339),
			"cooldown_seconds", int(tocProviderCooldown().Seconds()),
			"error", err,
		)
	}
	result, fallbackErr := d.callLLM(ctx, d.FallbackModelName, d.FallbackModelCfg, inputJSON)
	if fallbackErr != nil {
		return nil, d.FallbackModelName, fmt.Errorf("(MID_26061102) TOC primary failed: %w; fallback failed: %v", err, fallbackErr)
	}
	return parseTOCDetectionResult(result), d.FallbackModelName, nil
}

func (d *staticTOCDetector) callLLM(ctx context.Context, modelName string, cfg structureModelConfig, inputJSON string) (map[string]any, error) {
	applyStructureModelConfigToExtractor(d.Client, cfg)
	in := newLLMJSONInput(ctx, d.PromptRef, d.PromptText, modelName, inputJSON, "detect_toc", "MID-CWB-DETECT-TOC")
	if structured, ok := d.Client.(LLMStructuredJSONExtractor); ok {
		result, err := structured.ExtractStructuredJSON(ctx, in, tocDetectionContract())
		if err != nil {
			return nil, err
		}
		if result == nil {
			return map[string]any{}, nil
		}
		return result.Parsed, nil
	}
	return d.Client.ExtractJSON(ctx, in)
}

func parseTOCDetectionResult(parsed map[string]any) []int {
	raw, ok := parsed["toc_line_numbers"]
	if !ok {
		return nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil
	}
	result := make([]int, 0, len(arr))
	for _, v := range arr {
		s, isStr := v.(string)
		if isStr {
			if idx := strings.Index(s, "-"); idx > 0 {
				from, err1 := strconv.Atoi(strings.TrimSpace(s[:idx]))
				to, err2 := strconv.Atoi(strings.TrimSpace(s[idx+1:]))
				if err1 == nil && err2 == nil && from > 0 && to >= from {
					for n := from; n <= to; n++ {
						result = append(result, n)
					}
					continue
				}
			}
			n, err := strconv.Atoi(strings.TrimSpace(s))
			if err == nil && n > 0 {
				result = append(result, n)
			}
			continue
		}
		n := int(toFloat(v))
		if n > 0 {
			result = append(result, n)
		}
	}
	return result
}

func logTOCDetectionCall(ctx context.Context, toc *staticTOCDetector, callID, usedModel string, recordID *int64, tocLineNums []int, callErr error, elapsed time.Duration) {
	if toc.ProcLogger == nil || toc.ProcLogger.DB == nil {
		return
	}
	var artifactStr *string
	if callErr == nil && tocLineNums != nil {
		if bs, err := json.Marshal(map[string]any{"toc_line_numbers": tocLineNums}); err == nil {
			s := string(bs)
			artifactStr = &s
		}
	}
	var errStr *string
	if callErr != nil {
		s := callErr.Error()
		errStr = &s
	}
	modelNames := []string{}
	if usedModel != "" {
		modelNames = []string{strings.TrimSpace(usedModel)}
	}
	activity := "detect_toc_lines"
	rec := DocProcLogRecord{
		CallReason:   "detect toc lines",
		DocProcName:  "static_analyzer",
		ModelNames:   modelNames,
		PromptName:   toc.PromptRef,
		RecordID:     recordID,
		ActivityName: &activity,
		LLMCallID:    &callID,
		ArtifactJSON: artifactStr,
		Errors:       errStr,
		MSUsed:       int64Ptr(elapsed.Milliseconds()),
	}
	if err := toc.ProcLogger.LogLLMCall(ctx, rec, "MID-26061201"); err != nil {
		toc.Logger.Warn("failed to write detect_toc_lines log", "call_id", callID, "error", err)
	}
}

// staticInputLinesToJSON serialises static input lines as a JSON array for LLM input.
func staticInputLinesToJSON(lines []staticInputLine) string {
	type lineObj struct {
		Flag       string `json:"flag"`
		LineNumber int    `json:"line_number"`
		PageNumber int    `json:"page_number"`
		LineType   string `json:"line_type"`
		Content    string `json:"content"`
	}
	objs := make([]lineObj, 0, len(lines))
	for _, line := range lines {
		if strings.EqualFold(line.OriginalLineLower, "image") {
			continue
		}
		objs = append(objs, lineObj{
			Flag:       "n",
			LineNumber: line.LineNo,
			PageNumber: line.PageNo,
			LineType:   line.OriginalLineType,
			Content:    line.Content,
		})
	}
	bs, _ := json.Marshal(objs)
	return string(bs)
}

// loadTOCDetectPromptFromEnv loads the TOC detection prompt from the DETECT_TOC_LINES_PROMPT env var.
func loadTOCDetectPromptFromEnv() (promptText, promptRef, promptPath string, promptErr error) {
	promptRef = strings.TrimSpace(os.Getenv("DETECT_TOC_LINES_PROMPT"))
	if promptRef == "" {
		return "", "", "", fmt.Errorf("(MID_26061103) DETECT_TOC_LINES_PROMPT not set")
	}

	paths := make([]string, 0, 5)
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
			return "", promptRef, candidate, fmt.Errorf("(MID_26061104) TOC detect prompt file is empty")
		}
		return text, promptRef, candidate, nil
	}
	if lastErr == nil {
		lastErr = errors.New("(MID_26061105) no candidate path available")
	}
	return "", promptRef, "", fmt.Errorf("(MID_26061106) TOC detect prompt file not found: %w", lastErr)
}

func applyStaticRemoveFullPageImageArtifactLines(lines []staticInputLine, logger ApiTypes.JimoLogger) []staticInputLine {
	if logger != nil {
		logger.Info("remove full-page image artifact lines invoked", "line_count", len(lines))
	}
	return removeStaticPageImageArtifacts(lines)
}

func applyStaticRemoveWeBoosWatermarkLines(lines []staticInputLine, logger ApiTypes.JimoLogger) []staticInputLine {
	if logger != nil {
		logger.Info("remove weboos watermark lines invoked", "line_count", len(lines))
	}
	return removeStaticWeBoosWatermarkLines(lines)
}

func removeStaticWeBoosWatermarkLines(lines []staticInputLine) []staticInputLine {
	const watermark = "www.weboos.com"

	// Collect pages that have exactly one line with content == watermark.
	matchesByPage := make(map[int]int) // page → index in lines
	for i, line := range lines {
		if strings.TrimSpace(line.Content) != watermark {
			continue
		}
		if _, seen := matchesByPage[line.PageNo]; seen {
			// More than one exact match on this page — condition cannot be met.
			return lines
		}
		matchesByPage[line.PageNo] = i
	}
	if len(matchesByPage) < 3 {
		return lines
	}

	// If a page has more than one watermark-like line, keep the whole document
	// unchanged to avoid deleting legitimate content.
	watermarkishCountByPage := make(map[int]int)
	for _, line := range lines {
		content := strings.TrimSpace(line.Content)
		if content == watermark || (strings.HasPrefix(content, "www") && len(content) <= len(watermark)) {
			watermarkishCountByPage[line.PageNo]++
			if watermarkishCountByPage[line.PageNo] > 1 {
				return lines
			}
		}
	}

	// Build removal set from exact-match lines and the "starts with www" fallback
	// for pages that have no exact match.
	removed := make(map[int]struct{}, len(matchesByPage))
	for _, idx := range matchesByPage {
		removed[idx] = struct{}{}
	}

	// For pages without an exact match: remove the sole www-prefixed short line, if any.
	wwwCandidates := make(map[int][]int) // page → indexes of www-prefix lines
	for i, line := range lines {
		if _, ok := removed[i]; ok {
			continue
		}
		if _, hasExact := matchesByPage[line.PageNo]; hasExact {
			continue
		}
		content := strings.TrimSpace(line.Content)
		if strings.HasPrefix(content, "www") && len(content) <= len(watermark) {
			wwwCandidates[line.PageNo] = append(wwwCandidates[line.PageNo], i)
		}
	}
	for _, idxs := range wwwCandidates {
		if len(idxs) == 1 {
			removed[idxs[0]] = struct{}{}
		}
	}

	// Build output: skip removed lines and preserve embedded watermark text in
	// other content such as table rows.
	filtered := make([]staticInputLine, 0, len(lines)-len(removed))
	for i, line := range lines {
		if _, ok := removed[i]; ok {
			continue
		}
		filtered = append(filtered, line)
	}
	return filtered
}

func applyStaticTruncateDots(lines []staticInputLine, logger ApiTypes.JimoLogger) []staticInputLine {
	if logger != nil {
		logger.Info("truncate long dots sequences invoked", "line_count", len(lines))
	}
	for i, line := range lines {
		if !strings.Contains(line.Content, `\dots`) {
			continue
		}
		replaced := staticDotsSeqRE.ReplaceAllString(line.Content, `\dots \dots \dots \dots \dots`)
		if replaced != line.Content {
			lines[i].Content = replaced
		}
	}
	return lines
}

func applyStaticCorrectHeadings(lines []staticInputLine, logger ApiTypes.JimoLogger) []staticInputLine {
	if logger != nil {
		logger.Info("correct headings invoked", "line_count", len(lines))
	}
	for i := range lines {
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(lines[i].OriginalLineLower)), "heading-") {
			continue
		}
		if normalized, changed := normalizeStaticHeadingContent(lines[i].Content); changed {
			lines[i].Content = normalized
		}
	}
	return lines
}

func applyStaticDetectHeadings(lastTOC int, lines []staticInputLine, corrected map[int]string, logger ApiTypes.JimoLogger) {
	if logger != nil {
		logger.Info("detect headings invoked", "line_count", len(lines))
	}
	applyStaticHeadingLabels(lastTOC, lines, corrected, logger)
}

func applyStaticMergeLines(lines []staticInputLine, corrected map[int]string, logger ApiTypes.JimoLogger) []staticInputLine {
	if logger != nil {
		logger.Info("merge lines invoked", "line_count", len(lines))
	}
	return mergeStaticParagraphLines(lines, corrected)
}

func applyStaticDetectItemLists(lines []staticInputLine, corrected map[int]string, logger ApiTypes.JimoLogger) {
	if logger != nil {
		logger.Info("detect item lists invoked", "line_count", len(lines))
	}
	applyStaticListLabels(lines, corrected)
}

func removeStaticPageImageArtifacts(lines []staticInputLine) []staticInputLine {
	type artifactMatch struct {
		lineIndex  int
		pageInFile int
	}

	matchesByPrefix := make(map[string][]artifactMatch, 4)
	for i, line := range lines {
		if line.OriginalLineLower != "image" {
			continue
		}
		if !isStaticPageImageArtifactCoordinate(line.Coordinate) {
			continue
		}
		m := staticPageImagePathRE.FindStringSubmatch(strings.TrimSpace(line.Content))
		if len(m) != 3 {
			continue
		}
		pageInFile, err := strconv.Atoi(strings.TrimSpace(m[2]))
		if err != nil || pageInFile <= 0 || pageInFile != line.PageNo {
			continue
		}
		prefix := strings.TrimSpace(m[1])
		if prefix == "" {
			continue
		}
		matchesByPrefix[prefix] = append(matchesByPrefix[prefix], artifactMatch{
			lineIndex:  i,
			pageInFile: pageInFile,
		})
	}

	removed := make(map[int]struct{}, len(lines))
	for _, matches := range matchesByPrefix {
		if len(matches) < 2 {
			continue
		}
		seenPages := make(map[int]struct{}, len(matches))
		for _, match := range matches {
			seenPages[match.pageInFile] = struct{}{}
		}
		if len(seenPages) < 2 {
			continue
		}
		for _, match := range matches {
			removed[match.lineIndex] = struct{}{}
		}
	}
	if len(removed) == 0 {
		return lines
	}
	if len(removed) >= len(lines) {
		// Removing would leave an empty document; images are the content, not artifacts.
		return lines
	}

	filtered := make([]staticInputLine, 0, len(lines)-len(removed))
	for i, line := range lines {
		if _, ok := removed[i]; ok {
			continue
		}
		filtered = append(filtered, line)
	}
	return filtered
}

func isStaticPageImageArtifactCoordinate(raw string) bool {
	geom, ok := parseStaticLineGeometry(raw)
	if !ok {
		return false
	}
	return geom.X1 <= 5 && geom.Y1 <= 5
}

func parseStaticInputLine(raw string) (staticInputLine, error) {
	fields := strings.Split(raw, "\t")
	if len(fields) != 7 {
		return staticInputLine{}, errors.New("(MID_26050810) invalid field count")
	}
	lineNo, err := strconv.Atoi(strings.TrimSpace(fields[0]))
	if err != nil || lineNo <= 0 {
		return staticInputLine{}, errors.New("(MID_26050811) invalid line number")
	}
	pageNo, err := strconv.Atoi(strings.TrimSpace(fields[1]))
	if err != nil || pageNo <= 0 {
		return staticInputLine{}, errors.New("(MID_26050812) invalid page number")
	}
	originalLineType := normalizeStaticLineType(strings.TrimSpace(fields[2]))
	font := strings.TrimSpace(fields[3])
	fontSize := strings.TrimSpace(fields[4])
	coordinate := strings.TrimSpace(fields[5])
	content := strings.TrimSpace(fields[6])
	if originalLineType == "" || font == "" || fontSize == "" || coordinate == "" {
		return staticInputLine{}, errors.New("(MID_26050813) empty required field")
	}
	return staticInputLine{
		LineNo:            lineNo,
		PageNo:            pageNo,
		OriginalLineType:  originalLineType,
		OriginalLineLower: strings.ToLower(originalLineType),
		Font:              font,
		FontSize:          fontSize,
		Coordinate:        coordinate,
		Content:           content,
	}, nil
}

type staticLineGeometry struct {
	X1 float64
	Y1 float64
	X2 float64
	Y2 float64
}

type staticPageLayout struct {
	LineStart float64
	LineEnd   float64
	StartTol  float64
	EndTol    float64
}

func mergeStaticParagraphLines(lines []staticInputLine, corrected map[int]string) []staticInputLine {
	if len(lines) < 2 {
		return lines
	}

	layoutByPage := buildStaticPageLayouts(lines, corrected)
	if len(layoutByPage) == 0 {
		return lines
	}

	merged := make([]staticInputLine, 0, len(lines))
	for i := 0; i < len(lines); i++ {
		cur := lines[i]
		layout, ok := layoutByPage[cur.PageNo]
		if !ok || !isStaticMergeableParagraph(cur, corrected) {
			merged = append(merged, cur)
			continue
		}

		curGeom, ok := parseStaticLineGeometry(cur.Coordinate)
		if !ok || !isStaticParagraphStart(curGeom, layout) {
			merged = append(merged, cur)
			continue
		}

		// A line that already ends with sentence-terminal punctuation is a
		// complete paragraph on its own. It can still look like an
		// unfinished wrap purely by geometry (its text happens to run
		// close to the page's line-end margin), so the geometry checks
		// alone are not enough to rule out merging an unrelated following
		// paragraph into it.
		if isStaticSentenceEnd(cur.Content) {
			merged = append(merged, cur)
			continue
		}

		parts := []string{strings.TrimSpace(cur.Content)}
		bbox := curGeom
		tailGeom := curGeom
		lastIdx := i
		for j := i + 1; j < len(lines); j++ {
			next := lines[j]
			if next.PageNo != cur.PageNo || !isStaticMergeableParagraph(next, corrected) {
				break
			}
			if !isStaticNearLineEnd(tailGeom, layout) {
				break
			}
			nextGeom, ok := parseStaticLineGeometry(next.Coordinate)
			if !ok || !isStaticNearLineStart(nextGeom, layout) {
				break
			}
			parts = append(parts, strings.TrimSpace(next.Content))
			bbox = unionStaticLineGeometry(bbox, nextGeom)
			tailGeom = nextGeom
			lastIdx = j
			if isStaticSentenceEnd(next.Content) {
				// The merged text now ends a complete sentence; stop
				// before pulling in what would be a new paragraph.
				break
			}
		}

		if lastIdx == i {
			merged = append(merged, cur)
			continue
		}

		cur.Content = strings.Join(parts, "")
		cur.Coordinate = formatStaticLineGeometry(bbox)
		merged = append(merged, cur)
		i = lastIdx
	}
	return merged
}

func buildStaticPageLayouts(lines []staticInputLine, corrected map[int]string) map[int]staticPageLayout {
	x1ByPage := make(map[int][]float64, 8)
	x2ByPage := make(map[int][]float64, 8)
	for _, line := range lines {
		if !isStaticMergeableParagraph(line, corrected) {
			continue
		}
		geom, ok := parseStaticLineGeometry(line.Coordinate)
		if !ok {
			continue
		}
		x1ByPage[line.PageNo] = append(x1ByPage[line.PageNo], geom.X1)
		x2ByPage[line.PageNo] = append(x2ByPage[line.PageNo], geom.X2)
	}

	layouts := make(map[int]staticPageLayout, len(x1ByPage))
	for pageNo, x1s := range x1ByPage {
		x2s := x2ByPage[pageNo]
		if len(x1s) < 2 || len(x2s) < 2 {
			continue
		}
		lineStart := staticPercentile(x1s, 0.1)
		lineEnd := staticPercentile(x2s, 0.9)
		width := lineEnd - lineStart
		if width <= 0 {
			continue
		}
		startTol := maxFloat(3, width*0.03)
		endTol := maxFloat(5, width*0.04)
		layouts[pageNo] = staticPageLayout{
			LineStart: lineStart,
			LineEnd:   lineEnd,
			StartTol:  startTol,
			EndTol:    endTol,
		}
	}
	return layouts
}

func isStaticMergeableParagraph(line staticInputLine, corrected map[int]string) bool {
	if strings.ToLower(strings.TrimSpace(line.OriginalLineLower)) != "paragraph" {
		return false
	}
	if isStaticWatermarkLikeLine(line.Content) {
		return false
	}
	if isStaticTOCLine(line) {
		return false
	}

	if line.LineNo == 25 {
		line.LineNo = 25
	}

	switch normalizeStaticTitle(line.Content) {
	case "tableofcontent", "tableofcontents", "目录", "目次":
		return false
	}
	return strings.TrimSpace(corrected[line.LineNo]) == "unchanged"
}

func isStaticWatermarkLikeLine(content string) bool {
	const watermark = "www.weboos.com"
	trimmed := strings.TrimSpace(content)
	return trimmed == watermark || (strings.HasPrefix(trimmed, "www") && len(trimmed) <= len(watermark))
}

func parseStaticLineGeometry(raw string) (staticLineGeometry, bool) {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "[") || !strings.HasSuffix(trimmed, "]") {
		return staticLineGeometry{}, false
	}
	fields := strings.Split(trimmed[1:len(trimmed)-1], ",")
	if len(fields) != 4 {
		return staticLineGeometry{}, false
	}
	vals := [4]float64{}
	for i, field := range fields {
		v, err := strconv.ParseFloat(strings.TrimSpace(field), 64)
		if err != nil {
			return staticLineGeometry{}, false
		}
		vals[i] = v
	}
	return staticLineGeometry{
		X1: vals[0],
		Y1: vals[1],
		X2: vals[2],
		Y2: vals[3],
	}, true
}

func isStaticParagraphStart(geom staticLineGeometry, layout staticPageLayout) bool {
	return isStaticNearLineStart(geom, layout) || geom.X1 > layout.LineStart+layout.StartTol
}

// isStaticSentenceEnd reports whether content already ends with
// sentence-terminal punctuation (Chinese or ASCII). A line ending this way
// is a complete sentence/paragraph on its own, regardless of how close its
// text runs to the page's line-end margin.
func isStaticSentenceEnd(content string) bool {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return false
	}
	r := []rune(trimmed)
	switch r[len(r)-1] {
	case '。', '！', '？', '!', '?', '.':
		return true
	}
	return false
}

func isStaticNearLineStart(geom staticLineGeometry, layout staticPageLayout) bool {
	return absFloat(geom.X1-layout.LineStart) <= layout.StartTol
}

func isStaticNearLineEnd(geom staticLineGeometry, layout staticPageLayout) bool {
	return geom.X2 >= layout.LineEnd-layout.EndTol
}

func unionStaticLineGeometry(a staticLineGeometry, b staticLineGeometry) staticLineGeometry {
	return staticLineGeometry{
		X1: minFloat(a.X1, b.X1),
		Y1: minFloat(a.Y1, b.Y1),
		X2: maxFloat(a.X2, b.X2),
		Y2: maxFloat(a.Y2, b.Y2),
	}
}

func formatStaticLineGeometry(geom staticLineGeometry) string {
	return fmt.Sprintf("[%g,%g,%g,%g]", geom.X1, geom.Y1, geom.X2, geom.Y2)
}

func staticPercentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	cp := append([]float64(nil), values...)
	sort.Float64s(cp)
	if len(cp) == 1 {
		return cp[0]
	}
	if p <= 0 {
		return cp[0]
	}
	if p >= 1 {
		return cp[len(cp)-1]
	}
	idx := int(float64(len(cp)-1) * p)
	if idx < 0 {
		idx = 0
	}
	if idx >= len(cp) {
		idx = len(cp) - 1
	}
	return cp[idx]
}

func minFloat(a float64, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxFloat(a float64, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func normalizeStaticLineType(lineType string) string {
	lineType = strings.TrimSpace(lineType)
	if strings.EqualFold(lineType, "heading") {
		return "heading-1"
	}
	m := staticLegacyHeadingRE.FindStringSubmatch(strings.ToLower(lineType))
	if len(m) == 2 {
		return "heading-" + m[1]
	}
	return lineType
}

// applyStaticTOCLabels identifies lines that are part of a table of contents and
// updates the corrected map with "toc" for those lines. It returns the index in
// 'lines' of the last TOC-marked line, or -1 if no TOC is detected.
func applyStaticTOCLabels(lines []staticInputLine, corrected map[int]string, logger ApiTypes.JimoLogger) int {
	// Find the TOC title line.
	start := -1
	tocPage := 0
	for i, line := range lines {
		n := normalizeStaticTitle(line.Content)
		if n == "tableofcontent" || n == "tableofcontents" || n == "目录" || n == "目次" {
			start = i
			tocPage = line.PageNo
			break
		}
	}
	if start < 0 {
		return -1
	}

	// Find the first actual TOC entry on the title page.
	firstTOC := -1
	for i := start + 1; i < len(lines); i++ {
		if lines[i].PageNo != tocPage {
			break
		}
		if isStaticTOCLine(lines[i]) {
			firstTOC = i
			break
		}
	}
	if firstTOC < 0 {
		return -1
	}

	// Determine the full extent of the TOC region page by page.
	// On the title page, every line from start through the end of the page is included.
	// Subsequent pages are included only if they contain at least one TOC entry.
	tocCount := 0
	endIdx := start
	currentPage := tocPage

	for i := start; i < len(lines); i++ {
		line := lines[i]

		if line.PageNo != currentPage {
			// Peek ahead: does this new page have any TOC entries?
			nextPage := line.PageNo
			hasTOC := false
			for j := i; j < len(lines) && lines[j].PageNo == nextPage; j++ {
				if isStaticTOCLine(lines[j]) {
					hasTOC = true
					break
				}
			}
			if !hasTOC {
				break
			}
			currentPage = nextPage
		}

		endIdx = i
		if isStaticTOCLine(line) {
			tocCount++
		}
	}

	if tocCount < 2 {
		logger.Warn("TOC detected but too few entries, likely a false positive",
			"FirstTOCLineNo", lines[firstTOC].LineNo,
			"Start", start,
			"EndIdx", endIdx,
			"TOCCount", tocCount)
		return -1
	}

	for i := start; i <= endIdx; i++ {
		corrected[lines[i].LineNo] = "toc"
	}

	logger.Info("TOC detected and labeled",
		"FirstTOCLineNo", lines[firstTOC].LineNo,
		"Start", start,
		"EndIdx", endIdx,
		"TOCCount", tocCount)
	return endIdx
}

func normalizeStaticTitle(s string) string {
	s = strings.ReplaceAll(s, "　", " ") // Chinese ideographic space → ASCII space
	return strings.Join(strings.Fields(strings.ToLower(s)), "")
}

func isStaticTOCLine(line staticInputLine) bool {
	s := strings.TrimSpace(line.Content)
	if s == "" {
		return false
	}
	stripped := strings.ReplaceAll(s, " ", "")
	if strings.Contains(stripped, "…") ||
		strings.Contains(stripped, "...") ||
		strings.Contains(stripped, "．．") ||
		strings.Contains(stripped, "··") {
		return true
	}
	return staticTOCDotLeaderRE.MatchString(s)
}

func applyStaticHeadingLabels(_ int, lines []staticInputLine, corrected map[int]string, logger ApiTypes.JimoLogger) {
	var prevNumeric []int
	hasNumericSeq := false
	appendixSeq := make(map[string][]int, 8)

	for i, line := range lines {
		if corrected[line.LineNo] == "toc" {
			continue
		}
		if line.OriginalLineLower != "paragraph" &&
			line.OriginalLineLower != "heading" &&
			!strings.HasPrefix(line.OriginalLineLower, "heading-") &&
			!staticLegacyHeadingRE.MatchString(line.OriginalLineLower) {
			continue
		}
		if isStaticTOCLine(line) {
			continue
		}

		if symbol, parts, title, ok := parseStaticAppendixHeading(line.Content); ok {
			if title == "" && i+1 < len(lines) && lines[i+1].OriginalLineLower == "paragraph" {
				title = strings.TrimSpace(lines[i+1].Content)
			}
			if title != "" && len(parts) > 0 && symbol != "" {
				if prev, exists := appendixSeq[symbol]; !exists || isStaticNextHeading(prev, parts) || len(parts) == 1 {
					corrected[line.LineNo] = fmt.Sprintf("heading-%d", len(parts))
					lines[i].Content, _ = normalizeStaticHeadingContent(lines[i].Content)
					appendixSeq[symbol] = parts
				}
				continue
			}
		}

		if i == 88 {
			logger.Debug("Debugging line 88")
		}

		parts, title, ok := parseStaticNumericHeading(line.Content)
		if !ok {
			continue
		}
		if title == "" && i+1 < len(lines) && lines[i+1].OriginalLineLower == "paragraph" {
			title = strings.TrimSpace(lines[i+1].Content)
		}
		if title == "" || len(parts) == 0 {
			continue
		}
		if len(parts) >= 3 && parts[1] == 0 {
			parts = append([]int{parts[0]}, parts[2:]...)
		}
		if len(parts) == 0 {
			continue
		}

		if !hasNumericSeq {
			corrected[line.LineNo] = fmt.Sprintf("heading-%d", len(parts))
			lines[i].Content, _ = normalizeStaticHeadingContent(lines[i].Content)
			prevNumeric = parts
			hasNumericSeq = true
			continue
		}
		if isStaticNextHeading(prevNumeric, parts) {
			corrected[line.LineNo] = fmt.Sprintf("heading-%d", len(parts))
			lines[i].Content, _ = normalizeStaticHeadingContent(lines[i].Content)
			prevNumeric = parts
			hasNumericSeq = true
		}
	}
}

func parseStaticNumericHeading(content string) ([]int, string, bool) {
	m := staticNumericHeadingRE.FindStringSubmatch(strings.TrimSpace(content))
	if len(m) != 3 {
		return nil, "", false
	}
	normalized := normalizeStaticHeadingNumber(m[1])
	parts, ok := parseStaticHeadingParts(normalized)
	if !ok {
		return nil, "", false
	}
	return parts, strings.TrimSpace(m[2]), true
}

func parseStaticAppendixHeading(content string) (string, []int, string, bool) {
	m := staticAppendixHeadingRE.FindStringSubmatch(strings.TrimSpace(content))
	if len(m) != 4 {
		return "", nil, "", false
	}
	normalized := normalizeStaticHeadingNumber(m[2])
	parts, ok := parseStaticHeadingParts(normalized)
	if !ok {
		return "", nil, "", false
	}
	return strings.ToUpper(strings.TrimSpace(m[1])), parts, strings.TrimSpace(m[3]), true
}

func normalizeStaticHeadingNumber(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.ReplaceAll(s, " ", "")
	s = strings.TrimSuffix(s, ".")
	s = strings.NewReplacer(
		"O", "0",
		"o", "0",
		"S", "5",
		"s", "5",
		"l", "1",
		"L", "1",
	).Replace(s)
	return s
}

func normalizeStaticHeadingContent(content string) (string, bool) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return content, false
	}

	if m := staticNumericHeadingRE.FindStringSubmatch(trimmed); len(m) == 3 {
		normalizedNumber := normalizeStaticHeadingNumber(m[1])
		if _, ok := parseStaticHeadingParts(normalizedNumber); ok {
			normalized := normalizedNumber
			if title := strings.TrimSpace(m[2]); title != "" {
				normalized += " " + title
			}
			return normalized, normalized != trimmed
		}
	}

	if m := staticAppendixHeadingRE.FindStringSubmatch(trimmed); len(m) == 4 {
		symbol := strings.ToUpper(strings.TrimSpace(m[1]))
		normalizedNumber := normalizeStaticHeadingNumber(m[2])
		if _, ok := parseStaticHeadingParts(normalizedNumber); ok {
			normalized := symbol + "." + normalizedNumber
			if title := strings.TrimSpace(m[3]); title != "" {
				normalized += " " + title
			}
			return normalized, normalized != trimmed
		}
	}

	return content, false
}

func parseStaticHeadingParts(raw string) ([]int, bool) {
	norm := strings.ReplaceAll(strings.TrimSpace(raw), " ", "")
	norm = strings.TrimSuffix(norm, ".")
	if norm == "" {
		return nil, false
	}
	strParts := strings.Split(norm, ".")
	out := make([]int, 0, len(strParts))
	for _, p := range strParts {
		if strings.TrimSpace(p) == "" {
			return nil, false
		}
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || n < 0 {
			return nil, false
		}
		out = append(out, n)
	}
	return out, len(out) > 0
}

func isStaticNextHeading(prev []int, cur []int) bool {
	if len(prev) == 0 || len(cur) == 0 {
		return false
	}
	if len(cur) == len(prev)+1 {
		if cur[len(cur)-1] != 1 {
			return false
		}
		for i := range prev {
			if prev[i] != cur[i] {
				return false
			}
		}
		return true
	}
	if len(cur) == len(prev) {
		for i := 0; i < len(prev)-1; i++ {
			if prev[i] != cur[i] {
				return false
			}
		}
		return cur[len(cur)-1] == prev[len(prev)-1]+1
	}
	if len(cur) < len(prev) {
		for targetLen := len(cur); targetLen >= 1; targetLen-- {
			if len(cur) != targetLen {
				continue
			}
			matchPrefix := true
			for i := 0; i < targetLen-1; i++ {
				if cur[i] != prev[i] {
					matchPrefix = false
					break
				}
			}
			if !matchPrefix {
				continue
			}
			if cur[targetLen-1] == prev[targetLen-1]+1 {
				return true
			}
		}
	}
	return false
}

// applyStticListLabels
// For each line with type 'list-teim', it uses regexp to determine
// the list type and change the list type if matches
func applyStaticListLabels(lines []staticInputLine, corrected map[int]string) {
	for _, line := range lines {
		if line.OriginalLineLower != "list-item" {
			continue
		}
		if corrected[line.LineNo] != "unchanged" {
			continue
		}
		content := strings.TrimSpace(line.Content)
		switch {
		case staticNumListRE.MatchString(content):
			corrected[line.LineNo] = "list-item-num"

		case staticSingleSymListRE.MatchString(content):
			corrected[line.LineNo] = "list-item-s-sym"

		case staticMultiSymListRE.MatchString(content):
			corrected[line.LineNo] = "list-item_m-sym"
		}
	}
}

func appendStaticAnalyzerStatus(raw string, p staticStatusParams) (string, error) {
	entries := decodeDocMetaStatus(raw)
	entry := map[string]any{
		"record_id":      strconv.FormatInt(p.RecordID, 10),
		"file_type":      sanitizeUTF8Text(strings.ToLower(strings.TrimSpace(p.FileType))),
		"operation":      "static_analzyer",
		"input_filename": sanitizeUTF8Text(p.InputFilename),
		"start_time":     p.Start.Format(defaultDocMetaStatusTime),
		"ms_used":        p.DurationMs,
	}
	if p.ProcErr == nil {
		entry["proc_status"] = "success"
		entry["num_pages"] = p.NumPages
		entry["num_lines"] = p.NumLines
		entry["num_labeled_lines"] = p.NumLabeledLines
	} else {
		entry["proc_status"] = "failed"
		entry["error"] = sanitizeUTF8Text(p.ProcErr.Error())
	}

	replaced := false
	out := make([]map[string]any, 0, len(entries)+1)
	for _, e := range entries {
		op := strings.ToLower(strings.TrimSpace(asString(e["operation"])))
		if op != "static_analyzer" && op != "static-analyzer" && op != "static_analzyer" {
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

func detectStaticStatusFileType(rec DocMetadataInputRecord, inputFilename string) string {
	candidates := []string{
		rec.FileName,
		rec.StagingFilename,
		rec.ResultFilename,
		inputFilename,
	}
	for _, candidate := range candidates {
		ext := strings.ToLower(strings.TrimSpace(filepath.Ext(strings.TrimSpace(candidate))))
		if ext != "" {
			return strings.TrimPrefix(ext, ".")
		}
	}
	return ""
}

func (p *StaticAnalyzerProcessor) failAndPersist(
	ctx context.Context,
	rec DocMetadataInputRecord,
	start time.Time,
	inputFilename string,
	numPages int,
	numLines int,
	numLabeledLines int,
	procErr error,
) error {
	statusRaw, err := appendStaticAnalyzerStatus(rec.StatusRaw, staticStatusParams{
		RecordID:        rec.ID,
		FileType:        detectStaticStatusFileType(rec, inputFilename),
		InputFilename:   inputFilename,
		NumPages:        numPages,
		NumLines:        numLines,
		NumLabeledLines: numLabeledLines,
		Start:           start,
		DurationMs:      time.Since(start).Milliseconds(),
		ProcErr:         procErr,
	})
	if err != nil {
		return fmt.Errorf("(MID_26042311) append static analyzer status: %w", err)
	}
	errMsg := strings.TrimSpace(procErr.Error())
	if err := p.Store.UpdateInputMetadata(ctx, rec.ID, DocMetadataUpdate{
		StatusRaw: statusRaw,
		ErrorMsg:  &errMsg,
	}); err != nil {
		return fmt.Errorf("(MID_26042312) persist static analyzer failure: %w", err)
	}
	return procErr
}

func (p *StaticAnalyzerProcessor) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}
