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
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

const (
	DefaultFileBlockSize         = 3
	ChunkingMethodTopic          = "topic-chunking"
	defaultSemanticChunkStatusTS = "20060102 15:04:05"
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
}

type SemanticPageBlock struct {
	BlockNo int
	Pages   []int
	Lines   []Line
}

type TopicItem struct {
	SeqNo     int
	TopicType string
	Lines     []string
	Keywords  []string
	Topic     string
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
	applyStructureModelConfigToExtractor(extractor, modelCfg)
	return &SemanticChunkingService{
		Store:         store,
		Extractor:     extractor,
		Logger:        logger,
		Now:           time.Now,
		ChunkDir:      strings.TrimSpace(os.Getenv("CHUNK_DIR")),
		FileBlockSize: envInt("FILE_BLOCK_SIZE", DefaultFileBlockSize, 1),
		ModelRef:      modelRef,
		ModelCfgPath:  modelCfgPath,
		ModelErr:      modelErr,
		ModelName:     modelCfg.ModelName,
		PromptText:    defaultTopicChunkPrompt,
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
	if strings.TrimSpace(s.ChunkDir) == "" {
		procErr := errors.New("(MID_26042105) missing CHUNK_DIR")
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

	if err := writeTopicsFile(s.ChunkDir, rec.ID, topics); err != nil {
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
	linesText := make([]string, 0, len(block.Lines))
	for _, line := range block.Lines {
		linesText = append(linesText, lineRawForChunking(line))
	}

	parsed, err := s.Extractor.ExtractJSON(ctx, llmclients.JSONExtractionInput{
		PromptText: s.PromptText,
		ModelName:  s.ModelName,
		InputText:  strings.Join(linesText, "\n"),
	})
	if err != nil {
		baseURL := ""
		if client, ok := s.Extractor.(*llmclients.OpenAIJSONClient); ok && client != nil {
			baseURL = strings.TrimSpace(client.BaseURL)
		}
		return nil, fmt.Errorf(
			"(MID_26042109) extract topics for block %d failed (model=%q, base_url=%q): %w",
			block.BlockNo,
			s.ModelName,
			baseURL,
			err,
		)
	}

	rawTopics, ok := parsed["topics"].([]any)
	if !ok {
		return []TopicItem{}, nil
	}

	out := make([]TopicItem, 0, len(rawTopics))
	nextSeq := seqStart
	for _, item := range rawTopics {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		lines := compactTopicArray(m["lines"])
		if len(lines) == 0 {
			lines = compactTopicArray(m["line_ranges"])
		}
		topic := sanitizeTopicText(asString(m["topic"]))
		if topic == "" {
			continue
		}
		topicType := strings.ToLower(strings.TrimSpace(asString(m["topic_type"])))
		if topicType == "" {
			topicType = "general"
		}
		keywords := compactTopicArray(m["keywords"])
		out = append(out, TopicItem{
			SeqNo:     nextSeq,
			TopicType: topicType,
			Lines:     lines,
			Keywords:  keywords,
			Topic:     topic,
		})
		nextSeq++
	}

	return out, nil
}

func writeTopicsFile(chunkDir string, recordID int64, topics []TopicItem) error {
	if strings.TrimSpace(chunkDir) == "" {
		return errors.New("(MID_26042110) chunk dir is empty")
	}
	if recordID <= 0 {
		return fmt.Errorf("(MID_26042111) invalid record id: %d", recordID)
	}

	groupID := recordID / 1000
	targetDir := filepath.Join(chunkDir, strconv.FormatInt(groupID, 10), strconv.FormatInt(recordID, 10))
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return err
	}

	path := filepath.Join(targetDir, "topics.txt")
	var b strings.Builder
	for _, topic := range topics {
		b.WriteString(fmt.Sprintf(
			"%d\t%s\t%s\t%s\t%s\n",
			topic.SeqNo,
			strings.TrimSpace(topic.TopicType),
			formatTopicArray(topic.Lines),
			formatTopicArray(topic.Keywords),
			sanitizeTopicText(topic.Topic),
		))
	}
	return os.WriteFile(path, []byte(strings.TrimRight(b.String(), "\n")), 0o644)
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
		return errors.New("font_size is empty")
	}
	n, err := strconv.ParseFloat(fs, 64)
	if err != nil || n <= 0 {
		return fmt.Errorf("invalid font_size: %q", line.FontSize)
	}
	if !coordinatePattern.MatchString(strings.TrimSpace(line.Coordinate)) {
		return fmt.Errorf("invalid coordinate: %q", line.Coordinate)
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

func loadTopicChunkModelFromEnv() (modelRef string, modelPath string, cfg structureModelConfig, err error) {
	modelRef = strings.TrimSpace(os.Getenv("TOPIC_CHUNK_MODEL_NAME"))
	if modelRef == "" {
		modelRef = strings.TrimSpace(os.Getenv("SEMANTIC_CHUNKING_MODEL_NAME"))
	}
	if modelRef == "" {
		return "", "", structureModelConfig{}, errors.New("missing TOPIC_CHUNK_MODEL_NAME")
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
		ModelName:  strings.TrimSpace(modelDef.ModelName),
		APIKey:     strings.TrimSpace(modelDef.APIKey),
		BaseURL:    strings.TrimSpace(modelDef.BaseURL),
		TimeoutSec: modelDef.TimeoutSec,
	}, nil
}

const defaultTopicChunkPrompt = `You extract semantic topics from line-file blocks.
Return strict JSON only:
{
  "topics": [
    {
      "topic_type": "string",
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
- "lines" must reference line numbers/ranges found in the input.
- Always include concise keywords.
- Do not output markdown.`
