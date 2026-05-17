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
	Store                      Store
	Extractor                  LLMJSONExtractor
	Embedder                   Embedder
	Logger                     ApiTypes.JimoLogger
	Now                        func() time.Time
	ChunkDir                   string
	ArtifactWebDir             string
	FileBlockSize              int
	ModelRef                   string
	ModelCfgPath               string
	ModelErr                   error
	ModelName                  string
	PromptText                 string
	PromptRef                  string
	PromptPath                 string
	PromptErr                  error
	TopicEmbeddingModelName    string
	CategorySimilarityMinScore float64
}

type SemanticPageBlock struct {
	BlockNo int
	Pages   []int
	Lines   []Line
}

// CategoryPathNode represents one node (segment) in a category path
// as defined by spec-category-extraction.md.
type CategoryPathNode struct {
	Name       string   `json:"name"`
	Keywords   []string `json:"keywords"`
	Confidence float64  `json:"confidence"`
}

// CategoryPathEntry represents one full category path with detail
// as defined by spec-category-extraction.md.
type CategoryPathEntry struct {
	PathKeywords   []string           `json:"path_keywords"`
	PathConfidence float64            `json:"path_confidence"`
	Nodes          []CategoryPathNode `json:"category_path"`
}

type TopicItem struct {
	SeqNo                int
	TopicType            string
	TopicTypeEn          string
	Lines                []string
	Keywords             []string
	KeywordsEn           []string
	Topic                string              // topic_desc in .topics file format
	TopicEn              string              // topic_desc_en in .topics file format
	CategoryPath         []string            // flat list of category names (for tree files)
	CategoryPathDetail   []CategoryPathEntry // detailed structure from LLM (for .topics file)
	CategoryPathDetailEn []CategoryPathEntry // English translation (only when input is non-English)
}

type topicChunkStatusParams struct {
	RecordID       int64
	FileType       string
	InputFilename  string
	OutputFilename string
	NumTopics      int
	Start          time.Time
	DurationMs     int64
	ProcErr        error
}

func NewSemanticChunkingService(store Store, extractor LLMJSONExtractor, logger ApiTypes.JimoLogger) *SemanticChunkingService {
	if logger == nil {
		logger = loggerutil.CreateDefaultLogger("MID_26042101")
	}
	modelRef, modelCfgPath, modelCfg, modelErr := loadFixedSizeTopicModelFromEnv()
	promptText, promptRef, promptPath, promptErr := loadTopicChunkPromptFromEnv()
	applyStructureModelConfigToExtractor(extractor, modelCfg)
	var embedder Embedder
	topicEmbeddingModelName := strings.TrimSpace(os.Getenv("TOPIC_EMBEDDING_MODEL_NAME"))
	if e, ok := extractor.(Embedder); ok {
		embedder = e
	}

	return &SemanticChunkingService{
		Store:                      store,
		Extractor:                  extractor,
		Embedder:                   embedder,
		Logger:                     logger,
		Now:                        time.Now,
		ChunkDir:                   strings.TrimSpace(os.Getenv("ARTIFACT_DIR")),
		ArtifactWebDir:             strings.TrimSpace(os.Getenv("ARTIFACT_WEB_DIR")),
		FileBlockSize:              envInt("FILE_BLOCK_SIZE", DefaultFileBlockSize, 1),
		ModelRef:                   modelRef,
		ModelCfgPath:               modelCfgPath,
		ModelErr:                   modelErr,
		ModelName:                  modelCfg.ModelName,
		PromptText:                 promptText,
		PromptRef:                  promptRef,
		PromptPath:                 promptPath,
		PromptErr:                  promptErr,
		TopicEmbeddingModelName:    topicEmbeddingModelName,
		CategorySimilarityMinScore: envFloat("CATEGORY_SIMILARITY_MIN_SCORE", DefaultCategorySimilarityMinScore, 0),
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
		s.failAndPersist(ctx, rec, inputFilename, start, s.ModelErr)
		return s.ModelErr
	}
	if s.PromptErr != nil {
		s.failAndPersist(ctx, rec, inputFilename, start, fmt.Errorf("(MID_26042440) load topic chunk prompt %q failed: %w", s.PromptRef, s.PromptErr))
		return s.PromptErr
	}
	if strings.TrimSpace(s.ChunkDir) == "" {
		procErr := errors.New("(MID_26042105) missing ARTIFACT_DIR")
		s.failAndPersist(ctx, rec, inputFilename, start, procErr)
		return procErr
	}
	if strings.TrimSpace(s.ArtifactWebDir) == "" {
		procErr := errors.New("(MID_26051310) missing ARTIFACT_WEB_DIR")
		s.failAndPersist(ctx, rec, inputFilename, start, procErr)
		return procErr
	}
	if strings.TrimSpace(inputFilename) == "" {
		procErr := errors.New("(MID_26042106) missing input filename")
		s.failAndPersist(ctx, rec, inputFilename, start, procErr)
		return procErr
	}

	lines, err := ParseSemanticInputLines(inputFile)
	if err != nil {
		s.failAndPersist(ctx, rec, inputFilename, start, err)
		return err
	}
	return s.handleSemanticLines(ctx, rec, inputFilename, start, lines)
}

// HandleBlockInput is the block-buffer variant of HandleInput. It uses
// ParseBlockBufferLines to obtain document lines from the BlockingProcessor's
// in-memory output, skipping file I/O and canonical-line validation
// (FontSize/Coordinate are not carried by the block format).
func (s *SemanticChunkingService) HandleBlockInput(ctx context.Context, recordID int64, inputFilename string, buf *BlockBuffer) error {
	if buf == nil {
		return errors.New("(MID_26050620) block buffer is nil")
	}
	if s.Store == nil {
		return errors.New("(MID_26050621) store is nil")
	}
	if s.Extractor == nil {
		return errors.New("(MID_26050622) semantic chunking extractor is nil")
	}
	if recordID <= 0 {
		return fmt.Errorf("(MID_26050623) invalid record_id: %d", recordID)
	}
	start := s.Now()

	rec, err := s.Store.GetInputRecord(ctx, recordID)
	if err != nil {
		return err
	}
	if s.ModelErr != nil {
		s.failAndPersist(ctx, rec, inputFilename, start, s.ModelErr)
		return s.ModelErr
	}
	if s.PromptErr != nil {
		s.failAndPersist(ctx, rec, inputFilename, start, fmt.Errorf("(MID_26050624) load topic chunk prompt %q failed: %w", s.PromptRef, s.PromptErr))
		return s.PromptErr
	}
	if strings.TrimSpace(s.ChunkDir) == "" {
		procErr := errors.New("(MID_26050625) missing ARTIFACT_DIR")
		s.failAndPersist(ctx, rec, inputFilename, start, procErr)
		return procErr
	}
	if strings.TrimSpace(s.ArtifactWebDir) == "" {
		procErr := errors.New("(MID_26051311) missing ARTIFACT_WEB_DIR")
		s.failAndPersist(ctx, rec, inputFilename, start, procErr)
		return procErr
	}
	if strings.TrimSpace(inputFilename) == "" {
		procErr := errors.New("(MID_26050626) missing input filename")
		s.failAndPersist(ctx, rec, inputFilename, start, procErr)
		return procErr
	}

	lines := ParseBlockBufferLines(buf)
	return s.handleSemanticLines(ctx, rec, inputFilename, start, lines)
}

// handleSemanticLines runs topic extraction, tree indexing, and status
// persistence for a pre-parsed set of lines. Called by both HandleInput
// (after ParseSemanticInputLines) and HandleBlockInput (after ParseBlockBufferLines).
func (s *SemanticChunkingService) handleSemanticLines(ctx context.Context, rec InputRecord, inputFilename string, start time.Time, lines []Line) error {
	blocks := BuildSemanticPageBlocks(lines, s.FileBlockSize)
	topics := make([]TopicItem, 0, 128)
	seqNo := 1
	for _, block := range blocks {
		blockTopics, blockErr := s.extractTopicsForBlock(ctx, rec.ID, block, seqNo)
		if blockErr != nil {
			s.failAndPersist(ctx, rec, inputFilename, start, blockErr)
			return blockErr
		}
		topics = append(topics, blockTopics...)
		seqNo += len(blockTopics)
	}
	topics = dedupeTopicItems(topics)

	artifactBase := buildChunkArtifactBaseName(rec.StagingFilename, rec.ParserName)
	outputFilename, err := writeTopicsFile(s.ChunkDir, rec.ID, artifactBase+".topics", topics)
	if err != nil {
		s.failAndPersist(ctx, rec, inputFilename, start, err)
		return err
	}
	if err := indexTopicsInTreeDir(s.Logger, s.ArtifactWebDir, rec.ID, topics); err != nil {
		s.failAndPersist(ctx, rec, inputFilename, start, err)
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
		s.failAndPersist(ctx, rec, inputFilename, start, err)
		return err
	}

	statusRaw, err := appendTopicChunkStatus(rec.StatusRaw, topicChunkStatusParams{
		RecordID:       rec.ID,
		FileType:       strings.TrimPrefix(strings.ToLower(filepath.Ext(inputFilename)), "."),
		InputFilename:  inputFilename,
		OutputFilename: outputFilename,
		NumTopics:      len(topics),
		Start:          start,
		DurationMs:     time.Since(start).Milliseconds(),
		ProcErr:        nil,
	})
	if err != nil {
		return err
	}
	if err := s.Store.UpdateInputStatus(ctx, rec.ID, statusRaw, nil); err != nil {
		return err
	}

	s.Logger.Info("semantic chunking completed",
		"record_id", rec.ID,
		"num_topics", len(topics),
		"filename", outputFilename,
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

func (s *SemanticChunkingService) extractTopicsForBlock(
	ctx context.Context,
	record_id int64,
	block SemanticPageBlock,
	seqStart int) ([]TopicItem, error) {
	topics, err := extractTopicsFromLinesWithLLM(
		ctx,
		record_id,
		s.Extractor,
		s.Logger,
		s.ModelName,
		s.PromptText,
		s.PromptRef,
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

func writeTopicsCategoryTree(logger ApiTypes.JimoLogger, chunkDir string, recordID int64, topics []TopicItem) error {
	targetDir, err := buildRecordArtifactDir(chunkDir, recordID)
	if err != nil {
		return err
	}
	return writeTopicsCategoryTreeToDir(logger, targetDir, recordID, topics)
}

/*
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
*/

// readTopicsFile reads a .topics file in the spec-compliant text format and
// returns parsed topic rows. It auto-detects the format: if the first non-empty
// line starts with "topic_id:" it parses the new spec format; otherwise it
// falls back to the legacy tab-delimited format.
func readTopicsFile(path string) ([]legacyTopicRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)

	// Peek at the first non-empty line to detect format.
	var firstLine string
	for scanner.Scan() {
		firstLine = strings.TrimSpace(scanner.Text())
		if firstLine != "" {
			break
		}
	}
	if firstLine == "" {
		return nil, nil
	}

	if strings.HasPrefix(firstLine, "topic_id:") {
		return parseSpecTopicsFile(scanner, firstLine)
	}
	return parseLegacyTopicsFile(scanner, firstLine)
}

func parseSpecTopicsFile(scanner *bufio.Scanner, firstLine string) ([]legacyTopicRow, error) {
	out := make([]legacyTopicRow, 0, 128)
	var cur *legacyTopicRow

	finishRow := func() {
		if cur != nil {
			out = append(out, *cur)
			cur = nil
		}
	}

	processLine := func(line string) {
		line = strings.TrimSpace(line)
		if line == "" {
			finishRow()
			return
		}
		key, val := splitSpecLine(line)
		if key == "" {
			return
		}
		if cur == nil {
			cur = &legacyTopicRow{}
		}
		switch key {
		case "topic_id":
			cur.SeqNo, _ = strconv.Atoi(val)
		case "topic_type":
			cur.TopicType = unquoteSpec(val)
		case "lines":
			cur.Lines = val
		case "topic_keywords":
			cur.Keywords = val
		case "topic_desc", "topic":
			cur.Topic = unquoteSpec(val)
		}
	}

	processLine(firstLine)
	for scanner.Scan() {
		processLine(scanner.Text())
	}
	finishRow()

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func parseLegacyTopicsFile(scanner *bufio.Scanner, firstLine string) ([]legacyTopicRow, error) {
	out := make([]legacyTopicRow, 0, 128)

	processLine := func(line string) {
		parts := strings.Split(line, "\t")
		if len(parts) != 5 {
			return
		}
		seq, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil || seq <= 0 {
			return
		}
		out = append(out, legacyTopicRow{
			SeqNo:     seq,
			TopicType: strings.TrimSpace(parts[1]),
			Lines:     strings.TrimSpace(parts[2]),
			Keywords:  strings.TrimSpace(parts[3]),
			Topic:     strings.TrimSpace(parts[4]),
		})
	}

	processLine(firstLine)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		processLine(line)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func splitSpecLine(line string) (key, value string) {
	idx := strings.Index(line, ":")
	if idx < 0 {
		return "", ""
	}
	return strings.TrimSpace(line[:idx]), strings.TrimSpace(line[idx+1:])
}

func unquoteSpec(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		unquoted, err := strconv.Unquote(s)
		if err == nil {
			return unquoted
		}
	}
	return s
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
		"record_id":       p.RecordID,
		"file_type":       p.FileType,
		"operation":       "generate_topics",
		"input_filename":  strings.TrimSpace(p.InputFilename),
		"output_filename": strings.TrimSpace(p.OutputFilename),
		"num_topics":      p.NumTopics,
		"ms_used":         p.DurationMs,
		"start_time":      p.Start.Format(defaultSemanticChunkStatusTS),
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
		if op != "generate_topics" {
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
	start time.Time,
	procErr error,
) {
	statusRaw, err := appendTopicChunkStatus(rec.StatusRaw, topicChunkStatusParams{
		RecordID:      rec.ID,
		FileType:      strings.TrimPrefix(strings.ToLower(filepath.Ext(inputFilename)), "."),
		InputFilename: inputFilename,
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
	// 1) Direct category_path — flat strings or nested [{name, keywords, confidence}, ...]
	if arr, ok := m["category_path"].([]any); ok {
		if names := extractNamesFromCategoryPathArray(arr); len(names) > 0 {
			return names
		}
	}

	// 2) Nested categories payload: [{category_path: [...], path_keywords, path_confidence}, ...]
	// Accepts both "category_paths" (new key) and "categories" (legacy key).
	for _, containerKey := range []string{"category_paths", "categories"} {
		arr, ok := m[containerKey].([]any)
		if !ok {
			continue
		}
		for _, it := range arr {
			catObj, ok := it.(map[string]any)
			if !ok {
				if s := asString(it); s != "" {
					return []string{s}
				}
				continue
			}
			if cp, ok := catObj["category_path"].([]any); ok {
				if names := extractNamesFromCategoryPathArray(cp); len(names) > 0 {
					return names
				}
			}
		}
	}

	// 3) Flat string fallbacks: "category", "topic_category" (slash-delimited)
	for _, key := range []string{"category", "topic_category"} {
		if s := strings.TrimSpace(asString(m[key])); s != "" {
			return strings.Split(s, "/")
		}
	}

	return nil
}

// extractNamesFromCategoryPathArray extracts category names from a []any that
// contains either plain strings or objects with a "name" field (the nested
// {name, keywords, confidence} shape defined by spec-category-extraction.md).
func extractNamesFromCategoryPathArray(arr []any) []string {
	names := make([]string, 0, len(arr))
	for _, it := range arr {
		if obj, ok := it.(map[string]any); ok {
			if name := asString(obj["name"]); name != "" {
				names = append(names, name)
			}
		} else if s := asString(it); s != "" {
			names = append(names, s)
		}
	}
	return names
}

// extractCategoryPathDetailFromLLM extracts full category path detail from an
// LLM response map, returning the structured entries as defined by
// spec-category-extraction.md. Returns nil if no structured data is found.
func extractCategoryPathDetailFromLLM(m map[string]any) []CategoryPathEntry {
	// 1) Direct category_path array of nodes: [{name, keywords, confidence}, ...]
	if arr, ok := m["category_path"].([]any); ok {
		nodes := extractCategoryPathNodes(arr)
		if len(nodes) > 0 {
			return []CategoryPathEntry{{
				Nodes:          nodes,
				PathKeywords:   collectNodeKeywords(nodes),
				PathConfidence: avgNodeConfidence(nodes),
			}}
		}
	}

	// 2) Nested categories payload: [{category_path: [...], path_keywords, path_confidence}, ...]
	// Accepts both "category_paths" (new key) and "categories" (legacy key).
	for _, containerKey := range []string{"category_paths", "categories"} {
		arr, ok := m[containerKey].([]any)
		if !ok {
			continue
		}
		if entries := parseCategoryPathsArray(arr); len(entries) > 0 {
			return entries
		}
	}

	return nil
}

// parseCategoryPathsArray converts a raw LLM category-paths array (the value of
// "category_paths" or "categories") into a slice of CategoryPathEntry.
func parseCategoryPathsArray(arr []any) []CategoryPathEntry {
	entries := make([]CategoryPathEntry, 0, len(arr))
	for _, it := range arr {
		catObj, ok := it.(map[string]any)
		if !ok {
			continue
		}
		nodes := extractCategoryPathNodes(catObj["category_path"])
		if len(nodes) == 0 {
			continue
		}
		entry := CategoryPathEntry{Nodes: nodes}
		if pks, ok := catObj["path_keywords"].([]any); ok {
			entry.PathKeywords = compactTopicArray(pks)
		}
		if entry.PathKeywords == nil {
			entry.PathKeywords = collectNodeKeywords(nodes)
		}
		if pc, ok := catObj["path_confidence"].(float64); ok {
			entry.PathConfidence = pc
		} else {
			entry.PathConfidence = avgNodeConfidence(nodes)
		}
		entries = append(entries, entry)
	}
	return entries
}

// extractCategoryPathDetailEnFromLLM extracts the English-translated category
// path detail from "categories_en" or "category_paths_en" in an LLM response map.
func extractCategoryPathDetailEnFromLLM(m map[string]any) []CategoryPathEntry {
	for _, key := range []string{"categories_en", "category_paths_en"} {
		arr, ok := m[key].([]any)
		if !ok {
			continue
		}
		if entries := parseCategoryPathsArray(arr); len(entries) > 0 {
			return entries
		}
	}
	return nil
}

func extractCategoryPathNodes(raw any) []CategoryPathNode {
	arr, ok := raw.([]any)
	if !ok {
		return nil
	}
	nodes := make([]CategoryPathNode, 0, len(arr))
	for _, it := range arr {
		obj, ok := it.(map[string]any)
		if !ok {
			continue
		}
		name := asString(obj["name"])
		if name == "" {
			continue
		}
		node := CategoryPathNode{Name: name}
		if ks, ok := obj["keywords"].([]any); ok {
			node.Keywords = compactTopicArray(ks)
		}
		if node.Keywords == nil {
			node.Keywords = []string{}
		}
		if cf, ok := obj["confidence"].(float64); ok {
			node.Confidence = cf
		}
		nodes = append(nodes, node)
	}
	return nodes
}

func collectNodeKeywords(nodes []CategoryPathNode) []string {
	seen := make(map[string]struct{}, len(nodes)*3)
	out := make([]string, 0, len(nodes)*3)
	for _, n := range nodes {
		for _, kw := range n.Keywords {
			if _, ok := seen[kw]; ok {
				continue
			}
			seen[kw] = struct{}{}
			out = append(out, kw)
		}
	}
	if out == nil {
		return []string{}
	}
	return out
}

func avgNodeConfidence(nodes []CategoryPathNode) float64 {
	if len(nodes) == 0 {
		return 0
	}
	var sum float64
	for _, n := range nodes {
		sum += n.Confidence
	}
	return sum / float64(len(nodes))
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
			err_msg := fmt.Sprintf("(MID_26042931) segment-too-long, segment:%s, max_len:%d", seg, maxCategoryNameLen)
			return fallbackCategoryPath(topicType), err_msg
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

// keywordCategoryPath builds a category path from keywords when LLM-generated
// categories are non-descriptive. Returns nil if no usable keywords are found.
func keywordCategoryPath(keywords []string) []string {
	result := make([]string, 0, 5)
	for _, kw := range keywords {
		seg := normalizeCategorySegment(kw)
		if seg != "" && isDescriptiveCategorySegment(seg) {
			result = append(result, seg)
		}
		if len(result) == 5 {
			break
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
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
		isCJK := (r >= 0x4E00 && r <= 0x9FFF) || // CJK Unified Ideographs
			(r >= 0x3400 && r <= 0x4DBF) || // CJK Extension A
			(r >= 0xF900 && r <= 0xFAFF) // CJK Compatibility Ideographs
		if isAlphaNum || isCJK {
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

func loadTopicChunkPromptFromEnv() (promptText string, promptRef string, promptPath string, promptErr error) {
	for _, key := range []string{"EXTRACT_TOPIC_PROMPT", "SEMANTIC_CHUNKING_PROMPT"} {
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
