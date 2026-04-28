package docprocessing

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

const (
	DefaultFileBlockSize         = 3
	ChunkingMethodTopic          = "topic-chunking"
	defaultSemanticChunkStatusTS = "20060102 15:04:05"
	maxCategoryDepth             = 6
	maxCategoryNameLen           = 64
)

var coordinatePattern = regexp.MustCompile(`^\[\s*[-+]?\d*\.?\d+\s*,\s*[-+]?\d*\.?\d+\s*,\s*[-+]?\d*\.?\d+\s*,\s*[-+]?\d*\.?\d+\s*\]$`)

type SemanticChunkingService struct {
	Store         Store
	Extractor     LLMJSONExtractor
	Logger        ApiTypes.JimoLogger
	Now           func() time.Time
	ChunkDir      string
	FileBlockSize int
	ModelRef      string
	ModelCfgPath  string
	ModelErr      error
	ModelName     string
	PromptText    string
	PromptRef     string
	PromptPath    string
	PromptErr     error
}

type SemanticPageBlock struct {
	BlockNo int
	Pages   []int
	Lines   []Line
}

type TopicItem struct {
	SeqNo        int
	TopicType    string
	Lines        []string
	Keywords     []string
	Topic        string
	CategoryPath []string
}

type topicChunkStatusParams struct {
	InputFilename string
	NumPages      int
	NumLines      int
	NumChunks     int
	Start         time.Time
	DurationMs    int64
	ProcErr       error
}

func NewSemanticChunkingService(store Store, extractor LLMJSONExtractor, logger ApiTypes.JimoLogger) *SemanticChunkingService {
	if logger == nil {
		logger = loggerutil.CreateDefaultLogger("MID_26042101")
	}
	modelRef, modelCfgPath, modelCfg, modelErr := loadTopicChunkModelFromEnv()
	promptText, promptRef, promptPath, promptErr := loadTopicChunkPromptFromEnv()
	applyStructureModelConfigToExtractor(extractor, modelCfg)
	return &SemanticChunkingService{
		Store:         store,
		Extractor:     extractor,
		Logger:        logger,
		Now:           time.Now,
		ChunkDir:      strings.TrimSpace(os.Getenv("ARTIFACT_DIR")),
		FileBlockSize: envInt("FILE_BLOCK_SIZE", DefaultFileBlockSize, 1),
		ModelRef:      modelRef,
		ModelCfgPath:  modelCfgPath,
		ModelErr:      modelErr,
		ModelName:     modelCfg.ModelName,
		PromptText:    promptText,
		PromptRef:     promptRef,
		PromptPath:    promptPath,
		PromptErr:     promptErr,
	}
}

func (s *SemanticChunkingService) HandleInput(ctx context.Context, recordID int64, inputFilename string, inputFile []byte) error {
	if s.Store == nil {
		return errors.New("(MID_26042102) store is nil")
	}
	if s.Extractor == nil {
		return errors.New("(MID_26042103) semantic chunking extractor is nil")
	}
	if recordID <= 0 {
		return fmt.Errorf("(MID_26042104) invalid record_id: %d", recordID)
	}
	start := s.Now()

	rec, err := s.Store.GetInputRecord(ctx, recordID)
	if err != nil {
		return err
	}
	if s.ModelErr != nil {
		s.failAndPersist(ctx, rec, inputFilename, 0, 0, 0, start, s.ModelErr)
		return s.ModelErr
	}
	if s.PromptErr != nil {
		s.failAndPersist(ctx, rec, inputFilename, 0, 0, 0, start, fmt.Errorf("(MID_26042440) load topic chunk prompt %q failed: %w", s.PromptRef, s.PromptErr))
		return s.PromptErr
	}
	if strings.TrimSpace(s.ChunkDir) == "" {
		procErr := errors.New("(MID_26042105) missing ARTIFACT_DIR")
		s.failAndPersist(ctx, rec, inputFilename, 0, 0, 0, start, procErr)
		return procErr
	}
	if strings.TrimSpace(inputFilename) == "" {
		procErr := errors.New("(MID_26042106) missing input filename")
		s.failAndPersist(ctx, rec, inputFilename, 0, 0, 0, start, procErr)
		return procErr
	}

	lines, err := ParseSemanticInputLines(inputFile)
	if err != nil {
		s.failAndPersist(ctx, rec, inputFilename, 0, 0, 0, start, err)
		return err
	}
	numPages := uniquePages(lines)
	numLines := len(lines)

	blocks := BuildSemanticPageBlocks(lines, s.FileBlockSize)
	topics := make([]TopicItem, 0, 128)
	seqNo := 1
	for _, block := range blocks {
		blockTopics, blockErr := s.extractTopicsForBlock(ctx, block, seqNo)
		if blockErr != nil {
			s.failAndPersist(ctx, rec, inputFilename, numPages, numLines, len(topics), start, blockErr)
			return blockErr
		}
		topics = append(topics, blockTopics...)
		seqNo += len(blockTopics)
	}
	topics = dedupeTopicItems(topics)

	topicsPath, err := writeTopicsFile(s.ChunkDir, rec.ID, topics)
	if err != nil {
		s.failAndPersist(ctx, rec, inputFilename, numPages, numLines, len(topics), start, err)
		return err
	}
	if err := writeTopicsCategoryTree(s.Logger, s.ChunkDir, rec.ID, topicsPath, topics); err != nil {
		s.failAndPersist(ctx, rec, inputFilename, numPages, numLines, len(topics), start, err)
		return err
	}

	overlapPercent := 0
	if s.FileBlockSize > 0 {
		overlapPercent = 100 / s.FileBlockSize
	}
	if err := s.Store.InsertChunkRun(ctx, ChunkRunRecord{
		SourceRecordID: rec.ID,
		ChunkingMethod: ChunkingMethodTopic,
		ChunkingSize:   s.FileBlockSize,
		OverlapPercent: overlapPercent,
		Notes:          "semantic topic chunking with 1-page overlap",
	}); err != nil {
		s.failAndPersist(ctx, rec, inputFilename, numPages, numLines, len(topics), start, err)
		return err
	}

	statusRaw, err := appendTopicChunkStatus(rec.StatusRaw, topicChunkStatusParams{
		InputFilename: inputFilename,
		NumPages:      numPages,
		NumLines:      numLines,
		NumChunks:     len(topics),
		Start:         start,
		DurationMs:    time.Since(start).Milliseconds(),
		ProcErr:       nil,
	})
	if err != nil {
		return err
	}
	if err := s.Store.UpdateInputStatus(ctx, rec.ID, statusRaw, nil); err != nil {
		return err
	}

	s.Logger.Info("semantic chunking completed",
		"record_id", rec.ID,
		"num_pages", numPages,
		"num_lines", numLines,
		"num_topics", len(topics),
		"chunk_dir", s.ChunkDir,
		"model_name", s.ModelName,
	)
	return nil
}

func ParseSemanticInputLines(input []byte) ([]Line, error) {
	sc := bufio.NewScanner(strings.NewReader(string(input)))
	sc.Buffer(make([]byte, 1024), 16*1024*1024)

	out := make([]Line, 0, 128)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		raw := strings.TrimSpace(sc.Text())
		if raw == "" {
			continue
		}
		if fields := strings.Split(raw, "\t"); len(fields) >= 3 && strings.EqualFold(strings.TrimSpace(fields[2]), "TOC") {
			continue
		}
		parsed, err := parseLine(raw)
		if err != nil {
			return nil, fmt.Errorf("(MID_26042107) line %d: %w", lineNo, err)
		}
		if err := validateCanonicalLine(parsed); err != nil {
			return nil, fmt.Errorf("(MID_26042108) line %d: %w", lineNo, err)
		}
		out = append(out, parsed)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func BuildSemanticPageBlocks(lines []Line, fileBlockSize int) []SemanticPageBlock {
	if len(lines) == 0 {
		return []SemanticPageBlock{}
	}
	if fileBlockSize <= 0 {
		fileBlockSize = 1
	}

	pages := make([]int, 0, len(lines))
	seenPages := make(map[int]struct{}, len(lines))
	for _, line := range lines {
		if line.PageNo <= 0 {
			continue
		}
		if _, ok := seenPages[line.PageNo]; ok {
			continue
		}
		seenPages[line.PageNo] = struct{}{}
		pages = append(pages, line.PageNo)
	}
	if len(pages) == 0 {
		return []SemanticPageBlock{{BlockNo: 1, Pages: []int{1}, Lines: lines}}
	}

	blocks := make([]SemanticPageBlock, 0, max(1, len(pages)/fileBlockSize+1))
	for contentStart := 0; contentStart < len(pages); contentStart += fileBlockSize {
		contentEnd := min(contentStart+fileBlockSize, len(pages))
		blockPages := make([]int, 0, fileBlockSize+1)
		if contentStart > 0 {
			blockPages = append(blockPages, pages[contentStart-1])
		}
		blockPages = append(blockPages, pages[contentStart:contentEnd]...)

		pageSet := make(map[int]struct{}, len(blockPages))
		for _, p := range blockPages {
			pageSet[p] = struct{}{}
		}

		blockLines := make([]Line, 0, len(lines))
		for _, line := range lines {
			if _, ok := pageSet[line.PageNo]; ok {
				blockLines = append(blockLines, line)
			}
		}

		blocks = append(blocks, SemanticPageBlock{
			BlockNo: len(blocks) + 1,
			Pages:   blockPages,
			Lines:   blockLines,
		})
	}
	return blocks
}

func (s *SemanticChunkingService) extractTopicsForBlock(ctx context.Context, block SemanticPageBlock, seqStart int) ([]TopicItem, error) {
	topics, err := extractTopicsFromLinesWithLLM(
		ctx,
		s.Extractor,
		s.Logger,
		s.ModelName,
		s.PromptText,
		block.Lines,
		seqStart,
		"block_no",
		block.BlockNo,
	)
	if err != nil {
		return nil, fmt.Errorf("(MID_26042109) %w", err)
	}
	return topics, nil
}

type legacyTopicRow struct {
	SeqNo     int
	TopicType string
	Lines     string
	Keywords  string
	Topic     string
}

func writeTopicsFile(chunkDir string, recordID int64, topics []TopicItem) (string, error) {
	return writeNamedTopicsFile(chunkDir, recordID, "topics.txt", topics)
}

func writeTopicsCategoryTree(logger ApiTypes.JimoLogger, chunkDir string, recordID int64, topicsPath string, topics []TopicItem) error {
	targetDir, err := buildRecordArtifactDir(chunkDir, recordID)
	if err != nil {
		return err
	}
	return writeTopicsCategoryTreeToDir(logger, targetDir, recordID, topicsPath, topics)
}

func readLegacyTopicRows(path string) ([]legacyTopicRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := make([]legacyTopicRow, 0, 128)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(strings.TrimRight(scanner.Text(), "\r\n"))
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != 5 {
			continue
		}
		seq, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil || seq <= 0 {
			continue
		}
		out = append(out, legacyTopicRow{
			SeqNo:     seq,
			TopicType: strings.TrimSpace(parts[1]),
			Lines:     strings.TrimSpace(parts[2]),
			Keywords:  strings.TrimSpace(parts[3]),
			Topic:     strings.TrimSpace(parts[4]),
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func loadLeafRowsExcludingRecord(targetDir string, recordID int64) (map[string][]string, error) {
	out := map[string][]string{}
	err := filepath.WalkDir(targetDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Base(path), "topics.txt") || !strings.EqualFold(filepath.Ext(path), ".txt") {
			return nil
		}
		rel, err := filepath.Rel(targetDir, path)
		if err != nil {
			return err
		}
		rows, managed, err := readLeafRowsExcludingRecord(path, recordID)
		if err != nil {
			return err
		}
		if !managed {
			return nil
		}
		out[rel] = rows
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func readLeafRowsExcludingRecord(path string, recordID int64) ([]string, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()

	rows := []string{}
	managed := true
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(strings.TrimRight(scanner.Text(), "\r\n"))
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != 5 {
			managed = false
			break
		}
		id, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
		if err != nil {
			managed = false
			break
		}
		if id == recordID {
			continue
		}
		rows = append(rows, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, false, err
	}
	if !managed {
		return nil, false, nil
	}
	return rows, true, nil
}

func appendTopicChunkStatus(raw string, p topicChunkStatusParams) (string, error) {
	entries := decodeStatus(raw)
	entry := map[string]any{
		"operation":      "topic_chunk",
		"input_filename": strings.TrimSpace(p.InputFilename),
		"num_pages":      p.NumPages,
		"num_lines":      p.NumLines,
		"num_chunks":     p.NumChunks,
		"ms_used":        p.DurationMs,
		"start_time":     p.Start.Format(defaultSemanticChunkStatusTS),
	}
	if p.ProcErr == nil {
		entry["proc_status"] = "success"
	} else {
		entry["proc_status"] = "failed"
		entry["error"] = p.ProcErr.Error()
	}

	replaced := false
	out := make([]map[string]any, 0, len(entries)+1)
	for _, e := range entries {
		op := strings.ToLower(strings.TrimSpace(asString(e["operation"])))
		if op != "topic_chunk" {
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

func (s *SemanticChunkingService) failAndPersist(
	ctx context.Context,
	rec InputRecord,
	inputFilename string,
	numPages int,
	numLines int,
	numChunks int,
	start time.Time,
	procErr error,
) {
	statusRaw, err := appendTopicChunkStatus(rec.StatusRaw, topicChunkStatusParams{
		InputFilename: inputFilename,
		NumPages:      numPages,
		NumLines:      numLines,
		NumChunks:     numChunks,
		Start:         start,
		DurationMs:    time.Since(start).Milliseconds(),
		ProcErr:       procErr,
	})
	if err != nil {
		s.Logger.Error("failed building semantic chunk status", "record_id", rec.ID, "error", err)
		return
	}

	errMsg := strings.TrimSpace(procErr.Error())
	if updateErr := s.Store.UpdateInputStatus(ctx, rec.ID, statusRaw, &errMsg); updateErr != nil {
		s.Logger.Error("failed persisting semantic chunk failure status", "record_id", rec.ID, "error", updateErr)
		return
	}
	s.Logger.Error("semantic chunking failed", "record_id", rec.ID, "error", procErr)
}

func validateCanonicalLine(line Line) error {
	fs := strings.TrimSpace(line.FontSize)
	if fs == "" {
		return errors.New("(MID_26042446) font_size is empty")
	}
	n, err := strconv.ParseFloat(fs, 64)
	if err != nil || n <= 0 {
		return fmt.Errorf("(MID_26042441) invalid font_size: %q", line.FontSize)
	}
	if !coordinatePattern.MatchString(strings.TrimSpace(line.Coordinate)) {
		return fmt.Errorf("(MID_26042442) invalid coordinate: %q", line.Coordinate)
	}
	return nil
}

func compactTopicArray(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		s := strings.TrimSpace(asString(v))
		if s == "" {
			return nil
		}
		return []string{s}
	}
	out := make([]string, 0, len(arr))
	seen := make(map[string]struct{}, len(arr))
	for _, item := range arr {
		s := strings.TrimSpace(asString(item))
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func formatTopicArray(items []string) string {
	if len(items) == 0 {
		return "[]"
	}
	return "[" + strings.Join(items, ", ") + "]"
}

func sanitizeTopicText(s string) string {
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}

func dedupeTopicItems(in []TopicItem) []TopicItem {
	if len(in) == 0 {
		return []TopicItem{}
	}
	out := make([]TopicItem, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, item := range in {
		key := strings.ToLower(strings.TrimSpace(item.TopicType)) + "|" +
			strings.Join(item.CategoryPath, "/") + "|" +
			formatTopicArray(item.Lines) + "|" +
			sanitizeTopicText(item.Topic)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	for i := range out {
		out[i].SeqNo = i + 1
	}
	return out
}

func extractCategoryPathFromLLM(m map[string]any) []string {
	candidates := []string{}
	if arr, ok := m["category_path"].([]any); ok {
		for _, it := range arr {
			candidates = append(candidates, asString(it))
		}
	}
	if len(candidates) == 0 {
		if arr, ok := m["categories"].([]any); ok {
			for _, it := range arr {
				candidates = append(candidates, asString(it))
			}
		}
	}
	if len(candidates) == 0 {
		for _, key := range []string{"category", "topic_category"} {
			if s := strings.TrimSpace(asString(m[key])); s != "" {
				candidates = append(candidates, strings.Split(s, "/")...)
				break
			}
		}
	}
	return candidates
}

func normalizeAndValidateTopicCategoryPath(raw []string, topicType string) ([]string, string) {
	normalized := make([]string, 0, len(raw))
	for _, part := range raw {
		s := normalizeCategorySegment(part)
		if s == "" {
			continue
		}
		normalized = append(normalized, s)
	}
	if len(normalized) == 0 {
		return fallbackCategoryPath(topicType), "missing-category"
	}
	if len(normalized) > maxCategoryDepth {
		return fallbackCategoryPath(topicType), "depth-exceeds-limit"
	}
	for _, seg := range normalized {
		if len(seg) > maxCategoryNameLen {
			return fallbackCategoryPath(topicType), "segment-too-long"
		}
		if !isDescriptiveCategorySegment(seg) {
			return fallbackCategoryPath(topicType), "non-descriptive-segment"
		}
	}
	return normalized, ""
}

func fallbackCategoryPath(topicType string) []string {
	leaf := normalizeCategorySegment(topicType)
	if leaf == "" {
		leaf = "general"
	}
	return []string{"uncategorized", leaf}
}

func normalizeCategorySegment(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" {
		return ""
	}
	var b strings.Builder
	lastUnderscore := false
	for _, r := range s {
		isAlphaNum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if isAlphaNum {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteRune('_')
			lastUnderscore = true
		}
	}
	out := strings.Trim(b.String(), "_")
	for strings.Contains(out, "__") {
		out = strings.ReplaceAll(out, "__", "_")
	}
	return out
}

func isDescriptiveCategorySegment(seg string) bool {
	if seg == "" {
		return false
	}
	if _, err := strconv.Atoi(seg); err == nil {
		return false
	}
	switch seg {
	case "general", "misc", "other", "category", "topic", "unknown":
		return false
	}
	return true
}

func loadTopicChunkModelFromEnv() (modelRef string, modelPath string, cfg structureModelConfig, err error) {
	modelRef = strings.TrimSpace(os.Getenv("TOPIC_CHUNK_MODEL_NAME"))
	if modelRef == "" {
		modelRef = strings.TrimSpace(os.Getenv("SEMANTIC_CHUNKING_MODEL_NAME"))
	}
	if modelRef == "" {
		return "", "", structureModelConfig{}, errors.New("(MID_26042447) missing TOPIC_CHUNK_MODEL_NAME")
	}

	modelPath, err = resolveModelsFilePath("TOPIC_CHUNK_MODELS_FILE")
	if err != nil {
		if strings.TrimSpace(os.Getenv("SEMANTIC_CHUNKING_MODELS_FILE")) != "" {
			modelPath, err = resolveModelsFilePath("SEMANTIC_CHUNKING_MODELS_FILE")
		}
		if err != nil {
			return modelRef, "", structureModelConfig{}, err
		}
	}

	raw, err := os.ReadFile(modelPath)
	if err != nil {
		return modelRef, modelPath, structureModelConfig{}, fmt.Errorf("(MID_26042117) read %s failed: %w", modelPath, err)
	}
	parsed := ApiTypes.LLMModelsFile{}
	if err := parseTOMLMap(raw, &parsed); err != nil {
		return modelRef, modelPath, structureModelConfig{}, fmt.Errorf("(MID_26042118) parse %s failed: %w", modelPath, err)
	}
	modelDef, ok := parsed[modelRef]
	if !ok {
		return modelRef, modelPath, structureModelConfig{}, fmt.Errorf("(MID_26042119) model %q not found in %s", modelRef, modelPath)
	}
	if strings.TrimSpace(modelDef.ModelName) == "" {
		return modelRef, modelPath, structureModelConfig{}, fmt.Errorf("(MID_26042120) model %q in %s missing model_name", modelRef, modelPath)
	}
	return modelRef, modelPath, structureModelConfig{
		ModelName:    strings.TrimSpace(modelDef.ModelName),
		APIKey:       strings.TrimSpace(modelDef.APIKey),
		BaseURL:      strings.TrimSpace(modelDef.BaseURL),
		TimeoutSec:   modelDef.TimeoutSec,
		ThinkingType: normalizeThinkingType(strings.TrimSpace(modelDef.ThinkingType)),
	}, nil
}

func loadTopicChunkPromptFromEnv() (promptText string, promptRef string, promptPath string, promptErr error) {
	for _, key := range []string{"TOPIC_CHUNK_PROMPT", "SEMANTIC_CHUNKING_PROMPT"} {
		promptRef = strings.TrimSpace(os.Getenv(key))
		if promptRef != "" {
			break
		}
	}
	if promptRef == "" {
		return defaultTopicChunkPrompt, "", "", nil
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
			return "", promptRef, candidate, errors.New("(MID_26042445) prompt file is empty")
		}
		return text, promptRef, candidate, nil
	}

	if strings.Contains(promptRef, "\n") || strings.Contains(promptRef, " ") {
		return strings.TrimSpace(promptRef), "inline", "", nil
	}
	if lastErr == nil {
		lastErr = os.ErrNotExist
	}
	return "", promptRef, "", fmt.Errorf("(MID_26042443) prompt file not found: %w", lastErr)
}

const defaultTopicChunkPrompt = `You extract semantic topics from line-file blocks.
Return strict JSON only:
{
  "topics": [
    {
      "topic_type": "string",
      "category_path": ["snake_case", "snake_case"],
      "lines": ["38-45", "47"],
      "keywords": ["k1", "k2"],
      "topic": "one-sentence topic description"
    }
  ]
}

Rules:
- Ignore table of contents lines.
- If a cover page exists, treat it as one topic.
- For table content, create a topic with topic_type "table".
- For formula content, create a topic with topic_type "formula".
- For list content, create a topic with topic_type "list".
- Include all other meaningful topic types (workflows, policies, rules, etc.) when present.
- category_path must be descriptive snake_case segments.
- category_path depth must be <= 6.
- each category_path segment length must be <= 64.
- "lines" must reference line numbers/ranges found in the input.
- Always include concise keywords.
- Do not output markdown.`
