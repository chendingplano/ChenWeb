package docprocessing

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

const (
	DefaultChunkSize      = 300
	DefaultOverlapPercent = 20
	defaultStatusTime     = "20060102 15:04:05"
	ChunkingMethodFixed   = "fix-size-chunking"
)

var (
	numericSectionPattern = regexp.MustCompile(`^\d+\.\d+\b`)
)

type InputRecord struct {
	ID              int64
	ParserName      string
	ResultFilename  string
	StagingFilename string
	FileName        string
	StatusRaw       string
}

type ChunkRunRecord struct {
	SourceRecordID int64
	ChunkingMethod string
	ChunkingSize   int
	OverlapPercent int
	Notes          string
}

type Store interface {
	GetInputRecord(ctx context.Context, id int64) (InputRecord, error)
	InsertChunkRun(ctx context.Context, rec ChunkRunRecord) error
	UpdateInputStatus(ctx context.Context, id int64, statusJSON string, errorMsg *string) error
}

// Embedder abstracts the embedding API call so the chunking service can embed
// topic text without depending on a concrete LLM client type.
type Embedder interface {
	Embed(ctx context.Context, in llmclients.EmbedInput) ([]float64, error)
}

type FixedSizeChunkingService struct {
	Store                      Store
	Extractor                  LLMJSONExtractor
	Embedder                   Embedder
	Logger                     ApiTypes.JimoLogger
	Now                        func() time.Time
	ChunkDir                   string
	TreeRootDir                string
	ArtifactWebDir             string
	ChunkSize                  int
	OverlapPercent             int
	ModelRef                   string
	ModelCfgPath               string
	ModelErr                   error
	ModelName                  string
	PromptText                 string
	PromptRef                  string
	PromptPath                 string
	PromptErr                  error
	SummaryGroupSize           int
	SummaryModelRef            string
	SummaryModelCfgPath        string
	SummaryModelErr            error
	SummaryModelName           string
	SummaryPromptText          string
	SummaryPromptRef           string
	SummaryPromptPath          string
	SummaryPromptErr           error
	TopicEmbeddingModelName    string
	SummaryEmbeddingModelName  string
	CategorySimilarityMinScore float64
	GenerateSummary            func(ctx context.Context, recordID int64, level int, seqNo int, lines []Line, children []SummaryItem) (summaryGenerateResult, error)
}

type ChunkOptions struct {
	ChunkSize      int
	OverlapPercent int
}

type Line struct {
	Raw        string
	LineNo     int
	PageNo     int
	LineType   string
	Font       string
	FontSize   string
	Content    string
	Coordinate string
}

type MarkedLine struct {
	Line Line
	Mark string // r = regular, o = overlap
}

type Chunk struct {
	SeqNo int
	Lines []MarkedLine
}

type protectedBlock struct {
	start      int
	end        int
	splittable bool
}

// FixedSizeChunkingService is the core document processing pipeline for the doc-processor service.
// It processes uploaded documents end-to-end through three stages:
//  1. Chunking — splits document text into fixed-size byte chunks with configurable overlap.
//  2. Topic extraction — calls an LLM via Extractor with a configurable prompt to identify topics per chunk.
//  3. Summary tree generation — groups chunks hierarchically, calls an LLM to produce summaries and
//     category labels at each level, then persists the tree to disk and the database via Store.
//
// The main entry point is HandleInput. In production it is instantiated once at startup in
// cmd/doc-processor/main.go with a SQL store, an OpenAI-compatible JSON client, and a logger.
//
// NewFixedSizeChunkingService constructs the service by loading all model configs, prompts, and
// embedding settings from environment variables. The topic embedder is resolved in priority order:
// a dedicated OpenAI-compatible client if TOPIC_EMBEDDING_MODEL_NAME is set with a valid config,
// then the extractor itself if it implements Embedder, otherwise nil. Summary embedding follows the
// same fallback pattern via SUMMARY_EMBEDDING_MODEL_NAME. A default logger is created when none is provided.
func NewFixedSizeChunkingService(store Store, extractor LLMJSONExtractor, logger ApiTypes.JimoLogger) *FixedSizeChunkingService {
	if logger == nil {
		logger = loggerutil.CreateDefaultLogger("MID_26041901")
	}
	modelRef, modelCfgPath, modelCfg, modelErr := loadFixedSizeTopicModelFromEnv()
	promptText, promptRef, promptPath, promptErr := loadFixedSizeTopicPromptFromEnv()
	summaryModelRef, summaryModelCfgPath, summaryModelCfg, summaryModelErr := loadFixedSizeSummaryModelFromEnv()
	summaryPromptText, summaryPromptRef, summaryPromptPath, summaryPromptErr := loadFixedSizeSummaryPromptFromEnv()
	applyStructureModelConfigToExtractor(extractor, modelCfg)
	topicEmbeddingModelRef := strings.TrimSpace(os.Getenv("TOPIC_EMBEDDING_MODEL_NAME"))
	var embedder Embedder
	var topicEmbeddingModelName string
	if topicEmbeddingModelRef != "" {
		_, _, embCfg, embErr := loadModelConfigFromEnv("TOPIC_EMBEDDING_MODEL_NAME", "")
		if embErr == nil && strings.TrimSpace(embCfg.ModelName) != "" {
			timeoutSec := embCfg.TimeoutSec
			if timeoutSec <= 0 {
				timeoutSec = 60
			}
			embedder = &llmclients.OpenAIJSONClient{
				HTTPClient: &http.Client{Timeout: time.Duration(timeoutSec) * time.Second},
				ModelName:  embCfg.ModelName,
				APIKey:     embCfg.APIKey,
				BaseURL:    embCfg.BaseURL,
			}
			topicEmbeddingModelName = embCfg.ModelName
		} else if e, ok := extractor.(Embedder); ok {
			embedder = e
			topicEmbeddingModelName = topicEmbeddingModelRef
		}
	} else if e, ok := extractor.(Embedder); ok {
		embedder = e
	}
	summaryEmbeddingModelRef := strings.TrimSpace(os.Getenv("SUMMARY_EMBEDDING_MODEL_NAME"))
	if summaryEmbeddingModelRef == "" {
		logger.Error("SUMMARY_EMBEDDING_MODEL_NAME is not defined")
		panic("SUMMARY_EMBEDDING_MODEL_NAME is not defined")
	}
	var summaryEmbeddingModelName string
	if _, _, embCfg, embErr := loadModelConfigFromEnv("SUMMARY_EMBEDDING_MODEL_NAME", ""); embErr == nil && strings.TrimSpace(embCfg.ModelName) != "" {
		summaryEmbeddingModelName = embCfg.ModelName
	} else {
		summaryEmbeddingModelName = summaryEmbeddingModelRef
	}
	categorySimilarityMinScore := envFloat("CATEGORY_SIMILARITY_MIN_SCORE", DefaultCategorySimilarityMinScore, 0)
	return &FixedSizeChunkingService{
		Store:                      store,
		Extractor:                  extractor,
		Logger:                     logger,
		Now:                        time.Now,
		ChunkDir:                   strings.TrimSpace(os.Getenv("ARTIFACT_DIR")),
		TreeRootDir:                strings.TrimSpace(os.Getenv("TOPIC_TREE_ROOT_DIR")),
		ArtifactWebDir:             strings.TrimSpace(os.Getenv("ARTIFACT_WEB_DIR")),
		ChunkSize:                  envInt("CHUNK_SIZE", DefaultChunkSize, 1),
		OverlapPercent:             envInt("CHUNK_OVERLAP_PERCENT", DefaultOverlapPercent, 0),
		ModelRef:                   modelRef,
		ModelCfgPath:               modelCfgPath,
		ModelErr:                   modelErr,
		ModelName:                  modelCfg.ModelName,
		PromptText:                 promptText,
		PromptRef:                  promptRef,
		PromptPath:                 promptPath,
		PromptErr:                  promptErr,
		SummaryGroupSize:           envInt("SUMMARY_GROUP_SIZE", DefaultSummaryGroupSize, 1),
		SummaryModelRef:            summaryModelRef,
		SummaryModelCfgPath:        summaryModelCfgPath,
		SummaryModelErr:            summaryModelErr,
		SummaryModelName:           summaryModelCfg.ModelName,
		SummaryPromptText:          summaryPromptText,
		SummaryPromptRef:           summaryPromptRef,
		SummaryPromptPath:          summaryPromptPath,
		SummaryPromptErr:           summaryPromptErr,
		Embedder:                   embedder,
		TopicEmbeddingModelName:    topicEmbeddingModelName,
		SummaryEmbeddingModelName:  summaryEmbeddingModelName,
		CategorySimilarityMinScore: categorySimilarityMinScore,
	}
}

func (s *FixedSizeChunkingService) HandleInput(ctx context.Context, recordID int64, inputFilename string, inputFile []byte) error {
	if s.Store == nil {
		return errors.New("(MID_26042012) store is nil")
	}
	if s.Extractor == nil {
		return errors.New("(MID_26042013) fixed-size chunking extractor is nil")
	}
	if recordID <= 0 {
		return fmt.Errorf("(MID_26042002) invalid record_id: %d", recordID)
	}

	start := s.Now()
	rec, err := s.Store.GetInputRecord(ctx, recordID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(s.ChunkDir) == "" {
		procErr := errors.New("(MID_26042003) missing ARTIFACT_DIR")
		s.failAndPersist(ctx, rec, inputFilename, 0, 0, 0, start, procErr)
		return procErr
	}
	if strings.TrimSpace(s.TreeRootDir) == "" {
		procErr := errors.New("(MID_26042014) missing TOPIC_TREE_ROOT_DIR")
		s.failAndPersist(ctx, rec, inputFilename, 0, 0, 0, start, procErr)
		return procErr
	}
	if strings.TrimSpace(s.ArtifactWebDir) == "" {
		procErr := errors.New("(MID_26042901) missing ARTIFACT_WEB_DIR")
		s.failAndPersist(ctx, rec, inputFilename, 0, 0, 0, start, procErr)
		return procErr
	}
	if strings.TrimSpace(inputFilename) == "" {
		procErr := errors.New("(MID_26042004) missing input filename")
		s.failAndPersist(ctx, rec, inputFilename, 0, 0, 0, start, procErr)
		return procErr
	}
	if s.ModelErr != nil {
		s.failAndPersist(ctx, rec, inputFilename, 0, 0, 0, start, s.ModelErr)
		return s.ModelErr
	}
	if s.PromptErr != nil {
		procErr := fmt.Errorf("(MID_26042015) load fixed-size chunk prompt %q failed: %w", s.PromptRef, s.PromptErr)
		s.failAndPersist(ctx, rec, inputFilename, 0, 0, 0, start, procErr)
		return procErr
	}
	if s.GenerateSummary == nil && s.SummaryModelErr != nil {
		s.failAndPersist(ctx, rec, inputFilename, 0, 0, 0, start, s.SummaryModelErr)
		return s.SummaryModelErr
	}
	if s.GenerateSummary == nil && s.SummaryPromptErr != nil {
		procErr := fmt.Errorf("(MID_26042903) load fixed-size summary prompt %q failed: %w", s.SummaryPromptRef, s.SummaryPromptErr)
		s.failAndPersist(ctx, rec, inputFilename, 0, 0, 0, start, procErr)
		return procErr
	}

	lines, err := ParseInputLines(inputFile)
	if err != nil {
		s.failAndPersist(ctx, rec, inputFilename, 0, 0, 0, start, err)
		return err
	}
	return s.handleChunkLines(ctx, rec, inputFilename, start, lines)
}

// HandleBlockInput is the block-buffer variant of HandleInput. It uses
// ParseBlockBufferLines to obtain the document lines from an in-memory
// BlockBuffer produced by the BlockingProcessor, skipping the file-read step.
func (s *FixedSizeChunkingService) HandleBlockInput(ctx context.Context, recordID int64, inputFilename string, buf *BlockBuffer) error {
	if buf == nil {
		return errors.New("(MID_26050601) block buffer is nil")
	}
	if s.Store == nil {
		return errors.New("(MID_26050602) store is nil")
	}
	if s.Extractor == nil {
		return errors.New("(MID_26050603) fixed-size chunking extractor is nil")
	}
	if recordID <= 0 {
		return fmt.Errorf("(MID_26050604) invalid record_id: %d", recordID)
	}

	start := s.Now()
	rec, err := s.Store.GetInputRecord(ctx, recordID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(s.ChunkDir) == "" {
		procErr := errors.New("(MID_26050605) missing ARTIFACT_DIR")
		s.failAndPersist(ctx, rec, inputFilename, 0, 0, 0, start, procErr)
		return procErr
	}
	if strings.TrimSpace(s.TreeRootDir) == "" {
		procErr := errors.New("(MID_26050606) missing TOPIC_TREE_ROOT_DIR")
		s.failAndPersist(ctx, rec, inputFilename, 0, 0, 0, start, procErr)
		return procErr
	}
	if strings.TrimSpace(s.ArtifactWebDir) == "" {
		procErr := errors.New("(MID_26050607) missing ARTIFACT_WEB_DIR")
		s.failAndPersist(ctx, rec, inputFilename, 0, 0, 0, start, procErr)
		return procErr
	}
	if strings.TrimSpace(inputFilename) == "" {
		procErr := errors.New("(MID_26050608) missing input filename")
		s.failAndPersist(ctx, rec, inputFilename, 0, 0, 0, start, procErr)
		return procErr
	}
	if s.ModelErr != nil {
		s.failAndPersist(ctx, rec, inputFilename, 0, 0, 0, start, s.ModelErr)
		return s.ModelErr
	}
	if s.PromptErr != nil {
		procErr := fmt.Errorf("(MID_26050609) load fixed-size chunk prompt %q failed: %w", s.PromptRef, s.PromptErr)
		s.failAndPersist(ctx, rec, inputFilename, 0, 0, 0, start, procErr)
		return procErr
	}
	if s.GenerateSummary == nil && s.SummaryModelErr != nil {
		s.failAndPersist(ctx, rec, inputFilename, 0, 0, 0, start, s.SummaryModelErr)
		return s.SummaryModelErr
	}
	if s.GenerateSummary == nil && s.SummaryPromptErr != nil {
		procErr := fmt.Errorf("(MID_26050610) load fixed-size summary prompt %q failed: %w", s.SummaryPromptRef, s.SummaryPromptErr)
		s.failAndPersist(ctx, rec, inputFilename, 0, 0, 0, start, procErr)
		return procErr
	}

	lines := ParseBlockBufferLines(buf)
	return s.handleChunkLines(ctx, rec, inputFilename, start, lines)
}

// handleChunkLines runs chunking, topic extraction, summary generation, and
// status persistence for a pre-parsed set of lines. Called by both HandleInput
// and HandleBlockInput after their respective parse/validate steps.
func (s *FixedSizeChunkingService) handleChunkLines(ctx context.Context, rec InputRecord, inputFilename string, start time.Time, lines []Line) error {
	numPages := uniquePages(lines)
	numLines := len(lines)
	fileType := detectChunkStatusFileType(rec, inputFilename)
	chunks, err := BuildChunks(lines, ChunkOptions{ChunkSize: s.ChunkSize, OverlapPercent: s.OverlapPercent})
	if err != nil {
		s.failAndPersist(ctx, rec, inputFilename, numPages, numLines, 0, start, err)
		return err
	}

	stagingName := strings.TrimSpace(rec.StagingFilename)
	if stagingName == "" {
		stagingName = inputFilename
	}
	artifactBase := buildChunkArtifactBaseName(stagingName, rec.ParserName)
	chunkFilePath, err := writeCombinedChunkFile(s.ChunkDir, rec.ID, artifactBase+".chunks", chunks)
	if err != nil {
		s.failAndPersist(ctx, rec, inputFilename, numPages, numLines, 0, start, err)
		return err
	}
	s.Logger.Info("chunk file generated",
		"record_id", rec.ID,
		"chunk_file", chunkFilePath,
		"num_chunks", len(chunks),
	)

	topics := make([]TopicItem, 0, len(chunks))
	seqStart := 1
	for _, chunk := range chunks {
		chunkLines := make([]Line, 0, len(chunk.Lines))
		for _, marked := range chunk.Lines {
			chunkLines = append(chunkLines, marked.Line)
		}
		chunkTopics, chunkErr := extractTopicsFromLinesWithLLM(
			ctx,
			rec.ID,
			s.Extractor,
			s.Logger,
			s.ModelName,
			s.PromptText,
			s.PromptRef,
			chunkLines,
			seqStart,
			"chunk_seqno",
			chunk.SeqNo,
		)
		if chunkErr != nil {
			s.failAndPersist(ctx, rec, inputFilename, numPages, numLines, len(chunks), start, fmt.Errorf("(MID_26042016) %w", chunkErr))
			return chunkErr
		}
		topics = append(topics, chunkTopics...)
		seqStart += len(chunkTopics)
	}
	topics = dedupeTopicItems(topics)

	if _, err := writeTopicsFile(s.ChunkDir, rec.ID, artifactBase+".topics", topics); err != nil {
		s.failAndPersist(ctx, rec, inputFilename, numPages, numLines, len(chunks), start, err)
		return err
	}
	if err := indexTopicsInTreeDir(s.Logger, s.TreeRootDir, rec.ID, topics); err != nil {
		s.failAndPersist(ctx, rec, inputFilename, numPages, numLines, len(chunks), start, err)
		return err
	}
	if err := s.embedAndWriteTopics(ctx, rec.ID, artifactBase, topics); err != nil {
		s.failAndPersist(ctx, rec, inputFilename, numPages, numLines, len(chunks), start, err)
		return err
	}
	if err := deleteSummaryFiles(s.ChunkDir, rec.ID); err != nil {
		s.failAndPersistSummaries(ctx, rec, inputFilename, numPages, numLines, len(chunks), start, err)
		return err
	}

	leafSummaries := make([]SummaryItem, 0, len(chunks))
	for _, chunk := range chunks {
		chunkLines := make([]Line, 0, len(chunk.Lines))
		for _, marked := range chunk.Lines {
			chunkLines = append(chunkLines, marked.Line)
		}
		res, summaryErr := s.generateSummary(ctx, rec.ID, 0, chunk.SeqNo, chunkLines, nil)
		if summaryErr != nil {
			s.failAndPersistSummaries(ctx, rec, inputFilename, numPages, numLines, len(chunks), start, summaryErr)
			return summaryErr
		}
		_, regularLines := chunkLineNumbers(chunk)
		item := SummaryItem{
			SummaryID:           buildSummaryID(rec.ID, 0, chunk.SeqNo),
			RecordID:            rec.ID,
			Level:               0,
			SeqNo:               chunk.SeqNo,
			Lines:               lineRangesFromNumbers(regularLines),
			Keywords:            res.Keywords,
			KeywordsEn:          res.KeywordsEn,
			CategoryPaths:       res.CategoryPaths,
			CategoryNodes:       res.CategoryNodes,
			CategoryPathItems:   res.CategoryPathItems,
			CategoryPathItemsEn: res.CategoryPathItemsEn,
			Summary:             sanitizeTopicText(res.Summary),
			SummaryEn:           sanitizeTopicText(res.SummaryEn),
		}
		if _, err := writeSummaryFile(s.ChunkDir, rec.ID, item); err != nil {
			s.failAndPersistSummaries(ctx, rec, inputFilename, numPages, numLines, len(chunks), start, err)
			return err
		}
		leafSummaries = append(leafSummaries, item)
	}

	allSummaries, _, err := buildSummaryTree(rec.ID, leafSummaries, s.SummaryGroupSize, func(level int, seqNo int, children []SummaryItem) (summaryGenerateResult, error) {
		return s.generateSummary(ctx, rec.ID, level, seqNo, nil, children)
	})
	if err != nil {
		s.failAndPersistSummaries(ctx, rec, inputFilename, numPages, numLines, len(chunks), start, err)
		return err
	}
	for _, item := range allSummaries {
		if item.Level == 0 {
			continue
		}
		if _, err := writeSummaryFile(s.ChunkDir, rec.ID, item); err != nil {
			s.failAndPersistSummaries(ctx, rec, inputFilename, numPages, numLines, len(chunks), start, err)
			return err
		}
	}
	if err := os.MkdirAll(s.ArtifactWebDir, 0o755); err != nil {
		s.failAndPersistSummaries(ctx, rec, inputFilename, numPages, numLines, len(chunks), start, err)
		return err
	}
	if err := removeSummaryTreeRecord(s.ArtifactWebDir, rec.ID); err != nil {
		s.failAndPersistSummaries(ctx, rec, inputFilename, numPages, numLines, len(chunks), start, err)
		return err
	}
	for _, item := range allSummaries {
		if err := writeSummaryTreeEntry(s.Logger, s.ArtifactWebDir, item, item.CategoryPaths, item.CategoryNodes); err != nil {
			s.failAndPersistSummaries(ctx, rec, inputFilename, numPages, numLines, len(chunks), start, err)
			return err
		}
	}
	if err := s.embedAndWriteSummaries(ctx, rec.ID, allSummaries); err != nil {
		s.failAndPersistSummaries(ctx, rec, inputFilename, numPages, numLines, len(chunks), start, err)
		return err
	}

	if err := s.Store.InsertChunkRun(ctx, ChunkRunRecord{
		SourceRecordID: rec.ID,
		ChunkingMethod: "fix-size",
		ChunkingSize:   s.ChunkSize,
		OverlapPercent: s.OverlapPercent,
	}); err != nil {
		s.failAndPersist(ctx, rec, inputFilename, numPages, numLines, len(chunks), start, err)
		return err
	}

	statusRaw, err := appendSummariesStatus(rec.StatusRaw, summaryStatusParams{
		RecordID:      rec.ID,
		FileType:      fileType,
		InputFilename: inputFilename,
		Start:         start,
		DurationMs:    time.Since(start).Milliseconds(),
		ProcErr:       nil,
	})
	if err != nil {
		return err
	}
	statusRaw, err = appendChunkedStatus(statusRaw, chunkStatusParams{
		RecordID:        rec.ID,
		FileType:        fileType,
		InputFilename:   inputFilename,
		NumPages:        numPages,
		NumLines:        numLines,
		NumLabeledLines: numLines,
		NumChunks:       len(chunks),
		Start:           start,
		DurationMs:      time.Since(start).Milliseconds(),
		ProcErr:         nil,
	})
	if err != nil {
		return err
	}

	if err := s.Store.UpdateInputStatus(ctx, rec.ID, statusRaw, nil); err != nil {
		return err
	}

	s.Logger.Info("chunking completed",
		"record_id", rec.ID,
		"chunk_dir", s.ChunkDir,
		"tree_root_dir", s.TreeRootDir,
		"num_pages", numPages,
		"num_lines", numLines,
		"num_chunks", len(chunks),
		"num_topics", len(topics),
		"num_summaries", len(allSummaries),
		"model_name", s.ModelName,
	)
	return nil
}

// ParseBlockBufferLines converts a BlockBuffer to a sorted, deduplicated slice
// of Line values using only the normal ('n') lines. Overlap ('o') lines are
// skipped because each page already appears as a normal line in its own block.
// Font, FontSize, and Coordinate are left empty since the blocking processor
// drops them; fixed-size chunking does not use these fields.
func ParseBlockBufferLines(buf *BlockBuffer) []Line {
	if buf == nil {
		return nil
	}
	seen := make(map[int]struct{})
	out := make([]Line, 0, 128)
	for _, block := range buf.Blocks {
		for _, bl := range block.Lines {
			if bl.Flag != "n" {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(bl.LineType), "TOC") {
				continue
			}
			if _, ok := seen[bl.LineNumber]; ok {
				continue
			}
			seen[bl.LineNumber] = struct{}{}
			out = append(out, Line{
				LineNo:   bl.LineNumber,
				PageNo:   bl.PageNumber,
				LineType: strings.ToLower(bl.LineType),
				Content:  bl.Content,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LineNo < out[j].LineNo })
	return out
}

func (s *FixedSizeChunkingService) generateSummary(ctx context.Context, recordID int64, level int, seqNo int, lines []Line, children []SummaryItem) (summaryGenerateResult, error) {
	if s.GenerateSummary != nil {
		return s.GenerateSummary(ctx, recordID, level, seqNo, lines, children)
	}
	if s.Extractor == nil {
		return summaryGenerateResult{}, errors.New("(MID_26042904) summary extractor is nil")
	}
	inputText := buildSummaryInputText(lines, children)

	parsed, err := s.Extractor.ExtractJSON(ctx, llmclients.JSONExtractionInput{
		PromptText: appendLanguageInstruction(s.SummaryPromptText, inputText),
		ModelName:  s.SummaryModelName,
		InputText:  inputText,
	})
	if err != nil {
		return summaryGenerateResult{}, fmt.Errorf("(MID_26042905) generate summary for level %d seq %d: %w", level, seqNo, err)
	}
	summary := sanitizeTopicText(asString(parsed["summary"]))
	if summary == "" {
		summary = sanitizeTopicText(asString(parsed["text"]))
		s.Logger.Error("failed retrieving summary", "prompt", s.SummaryPromptText,
			"model_name", s.SummaryModelName,
			"input text", inputText,
			"fallback", summary)
	}
	if summary == "" {
		summary = fallbackSummaryText(inputText)
	}
	summaryEn := sanitizeTopicText(asString(parsed["summary_en"]))

	keywords := compactTopicArray(parsed["keywords"])
	keywordsEn := compactTopicArray(parsed["keywords_en"])

	rawPath := extractCategoryPathFromLLM(parsed)
	path, reason := normalizeAndValidateTopicCategoryPath(rawPath, defaultSummaryTreeFallbackTopicType)
	if reason != "" {
		s.Logger.Warn("(MID_26042906) invalid category path in summary response; using empty path",
			"level", level, "seq", seqNo, "reason", reason, "raw_path", rawPath)
		path = nil
	}
	categoryPathItems := extractCategoryPathDetailFromLLM(parsed)
	categoryPathItemsEn := extractCategoryPathDetailEnFromLLM(parsed)
	var nodes []CategoryPathNode
	if len(categoryPathItems) > 0 {
		nodes = categoryPathItems[0].Nodes
	}

	s.Logger.Info("Generated summary",
		"model_name", s.SummaryModelName,
		"level", level,
		"seq", seqNo,
		"summary", summary,
		"keywords", keywords,
		"category_path", path)
	return summaryGenerateResult{
		Summary:             summary,
		SummaryEn:           summaryEn,
		Keywords:            keywords,
		KeywordsEn:          keywordsEn,
		CategoryPaths:       path,
		CategoryNodes:       nodes,
		CategoryPathItems:   categoryPathItems,
		CategoryPathItemsEn: categoryPathItemsEn,
	}, nil
}

func buildSummaryInputText(lines []Line, children []SummaryItem) string {
	if len(lines) > 0 {
		parts := make([]string, 0, len(lines))
		for _, line := range lines {
			raw := lineRawForChunking(line)
			if strings.TrimSpace(raw) == "" {
				continue
			}
			parts = append(parts, raw)
		}
		return strings.Join(parts, "\n")
	}
	parts := make([]string, 0, len(children))
	for _, child := range children {
		parts = append(parts, child.SummaryID+": "+sanitizeTopicText(child.Summary))
	}
	return strings.Join(parts, "\n")
}

func fallbackSummaryText(inputText string) string {
	inputText = sanitizeTopicText(inputText)
	if inputText == "" {
		return "Summary unavailable"
	}
	parts := strings.Fields(inputText)
	if len(parts) > 20 {
		parts = parts[:20]
	}
	return strings.Join(parts, " ")
}

func (s *FixedSizeChunkingService) failAndPersist(
	ctx context.Context,
	rec InputRecord,
	inputFilename string,
	numPages int,
	numLines int,
	numChunks int,
	start time.Time,
	procErr error,
) {
	statusRaw, err := appendChunkedStatus(rec.StatusRaw, chunkStatusParams{
		RecordID:        rec.ID,
		FileType:        detectChunkStatusFileType(rec, inputFilename),
		InputFilename:   inputFilename,
		NumPages:        numPages,
		NumLines:        numLines,
		NumLabeledLines: numLines,
		NumChunks:       numChunks,
		Start:           start,
		DurationMs:      time.Since(start).Milliseconds(),
		ProcErr:         procErr,
	})
	if err != nil {
		s.Logger.Error("failed building chunk status", "record_id", rec.ID, "error", err)
		return
	}
	errMsg := sanitizeUTF8Text(procErr.Error())
	if updateErr := s.Store.UpdateInputStatus(ctx, rec.ID, statusRaw, &errMsg); updateErr != nil {
		s.Logger.Error("failed persisting chunk failure status", "record_id", rec.ID, "error", updateErr)
		return
	}
	s.Logger.Error("chunking failed", "record_id", rec.ID, "error", procErr)
}

func (s *FixedSizeChunkingService) failAndPersistSummaries(
	ctx context.Context,
	rec InputRecord,
	inputFilename string,
	numPages int,
	numLines int,
	numChunks int,
	start time.Time,
	procErr error,
) {
	statusRaw, err := appendSummariesStatus(rec.StatusRaw, summaryStatusParams{
		RecordID:      rec.ID,
		FileType:      detectChunkStatusFileType(rec, inputFilename),
		InputFilename: inputFilename,
		Start:         start,
		DurationMs:    time.Since(start).Milliseconds(),
		ProcErr:       procErr,
	})
	if err != nil {
		s.Logger.Error("failed building summary status", "record_id", rec.ID, "error", err)
		return
	}
	statusRaw, err = appendChunkedStatus(statusRaw, chunkStatusParams{
		RecordID:        rec.ID,
		FileType:        detectChunkStatusFileType(rec, inputFilename),
		InputFilename:   inputFilename,
		NumPages:        numPages,
		NumLines:        numLines,
		NumLabeledLines: numLines,
		NumChunks:       numChunks,
		Start:           start,
		DurationMs:      time.Since(start).Milliseconds(),
		ProcErr:         procErr,
	})
	if err != nil {
		s.Logger.Error("failed building chunk status", "record_id", rec.ID, "error", err)
		return
	}

	errMsg := sanitizeUTF8Text(procErr.Error())
	if updateErr := s.Store.UpdateInputStatus(ctx, rec.ID, statusRaw, &errMsg); updateErr != nil {
		s.Logger.Error("failed persisting summary failure status", "record_id", rec.ID, "error", updateErr)
		return
	}
	s.Logger.Error("summary generation failed", "record_id", rec.ID, "error", procErr)
}

const (
	embedMaxRetries = 3
	embedRetryDelay = 3 * time.Second
)

func (s *FixedSizeChunkingService) embedWithRetry(ctx context.Context, input llmclients.EmbedInput) ([]float64, error) {
	var lastErr error
	for attempt := 1; attempt <= embedMaxRetries; attempt++ {
		vec, err := s.Embedder.Embed(ctx, input)
		if err == nil {
			return vec, nil
		}
		lastErr = err
		if attempt < embedMaxRetries {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(embedRetryDelay):
			}
		}
	}
	return nil, lastErr
}

func (s *FixedSizeChunkingService) embedAndWriteTopics(ctx context.Context, recordID int64, _ string, topics []TopicItem) error {
	if s.Embedder == nil || strings.TrimSpace(s.TopicEmbeddingModelName) == "" {
		return nil
	}
	if len(topics) == 0 {
		return nil
	}

	targetDir, err := buildRecordArtifactDir(s.ChunkDir, recordID)
	if err != nil {
		return err
	}
	embedDir := filepath.Join(targetDir, "embeddings")
	if err := os.MkdirAll(embedDir, 0o755); err != nil {
		return fmt.Errorf("(MID_26050201) create embeddings dir failed: %w", err)
	}

	for _, topic := range topics {
		vec, err := s.embedWithRetry(ctx, llmclients.EmbedInput{
			ModelName: s.TopicEmbeddingModelName,
			InputText: topic.Topic,
		})
		if err != nil {
			return fmt.Errorf("(MID_26050202) embed topic seq=%d failed: %w", topic.SeqNo, err)
		}
		embedPath := filepath.Join(embedDir, fmt.Sprintf("topic_%d.embed", topic.SeqNo))
		if err := os.WriteFile(embedPath, []byte(formatFloatArray(vec)+"\n"), 0o644); err != nil {
			return fmt.Errorf("(MID_26050203) write embed file for topic %d failed: %w", topic.SeqNo, err)
		}
	}
	return nil
}

func (s *FixedSizeChunkingService) embedAndWriteSummaries(
	ctx context.Context,
	recordID int64,
	summaries []SummaryItem) error {
	if s.Embedder == nil || strings.TrimSpace(s.SummaryEmbeddingModelName) == "" {
		return nil
	}
	if len(summaries) == 0 {
		return nil
	}
	targetDir, err := buildRecordArtifactDir(s.ChunkDir, recordID)
	if err != nil {
		return err
	}
	embedDir := filepath.Join(targetDir, "embeddings")
	if err := os.MkdirAll(embedDir, 0o755); err != nil {
		return fmt.Errorf("(MID_26043100) create embeddings dir failed: %w", err)
	}
	for _, item := range summaries {
		summaryText := strings.TrimSpace(item.Summary)
		if summaryText == "" {
			continue
		}
		vec, err := s.embedWithRetry(ctx, llmclients.EmbedInput{
			ModelName: s.SummaryEmbeddingModelName,
			InputText: summaryText,
		})
		if err != nil {
			return fmt.Errorf("(MID_26043101) embed summary %q failed: %w", item.SummaryID, err)
		}
		embedPath := filepath.Join(embedDir, summaryEmbedFileName(item.Level, item.SeqNo))
		if err := os.WriteFile(embedPath, []byte(formatFloatArray(vec)+"\n"), 0o644); err != nil {
			return fmt.Errorf("(MID_26043102) write embed file for summary %q failed: %w", item.SummaryID, err)
		}
	}
	return nil
}

func ParseInputLines(input []byte) ([]Line, error) {
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
			return nil, fmt.Errorf("(MID_26042005) line %d: %w", lineNo, err)
		}
		out = append(out, parsed)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func parseLine(raw string) (Line, error) {
	fields := strings.Split(raw, "\t")
	if len(fields) != 7 {
		return Line{}, fmt.Errorf("(MID_26042006) invalid input line format, line:%s", raw)
	}
	lineNo, err := strconv.Atoi(strings.TrimSpace(fields[0]))
	if err != nil {
		return Line{}, fmt.Errorf("(MID_26042007) invalid line number: %w", err)
	}
	pageNo, err := strconv.Atoi(strings.TrimSpace(fields[1]))
	if err != nil {
		return Line{}, fmt.Errorf("(MID_26042008) invalid page number: %w", err)
	}
	lineType := strings.ToLower(strings.TrimSpace(fields[2]))
	font := strings.TrimSpace(fields[3])
	fontSize := strings.TrimSpace(fields[4])
	coordinate := strings.TrimSpace(fields[5])
	content := strings.TrimSpace(fields[6])
	if lineType == "" || font == "" || fontSize == "" || coordinate == "" {
		return Line{}, fmt.Errorf("(MID_26042006) invalid input line format, line:%s", raw)
	}
	return Line{
		Raw:        raw,
		LineNo:     lineNo,
		PageNo:     pageNo,
		LineType:   lineType,
		Font:       font,
		FontSize:   fontSize,
		Content:    content,
		Coordinate: coordinate,
	}, nil
}

func BuildChunks(lines []Line, opts ChunkOptions) ([]Chunk, error) {
	if opts.ChunkSize <= 0 {
		return nil, errors.New("(MID_26042009) chunk size must be positive")
	}
	if len(lines) == 0 {
		return []Chunk{}, nil
	}
	overlap := opts.OverlapPercent
	if overlap < 0 {
		overlap = 0
	}
	if overlap > 99 {
		overlap = 99
	}

	blocks := buildProtectedBlocks(lines, opts.ChunkSize)

	chunks := make([]Chunk, 0, max(1, len(lines)/2))
	start := 0
	prevEnd := 0
	seq := 1
	for start < len(lines) {
		target := findTargetByBytes(lines, start, opts.ChunkSize)
		end := adjustChunkEnd(start, target, len(lines), blocks)
		if end <= start {
			end = target
			if end <= start {
				end = start + 1
			}
		}

		c := Chunk{SeqNo: seq, Lines: make([]MarkedLine, 0, end-start)}
		for i := start; i < end; i++ {
			mark := "r"
			if i < prevEnd {
				mark = "o"
			}
			c.Lines = append(c.Lines, MarkedLine{Line: lines[i], Mark: mark})
		}
		chunks = append(chunks, c)

		if end >= len(lines) {
			break
		}
		prevEnd = end
		overlapLines := (end - start) * overlap / 100
		if overlapLines >= (end - start) {
			overlapLines = end - start - 1
		}
		if overlapLines < 0 {
			overlapLines = 0
		}
		nextStart := end - overlapLines
		if nextStart <= start {
			nextStart = start + 1
		}
		start = nextStart
		seq++
	}

	return chunks, nil
}

func findTargetByBytes(lines []Line, start int, targetBytes int) int {
	if start >= len(lines) {
		return len(lines)
	}
	if targetBytes <= 0 {
		return min(start+1, len(lines))
	}
	size := 0
	for i := start; i < len(lines); i++ {
		size += lineRawByteSize(lines[i])
		if size >= targetBytes {
			return i + 1
		}
	}
	return len(lines)
}

/*
func writeChunkFiles(chunkDir string, recordID int64, chunks []Chunk) error {
	if strings.TrimSpace(chunkDir) == "" {
		return errors.New("(MID_26042010) chunk dir is empty")
	}
	if recordID <= 0 {
		return fmt.Errorf("(MID_26042011) invalid record id: %d", recordID)
	}
	groupID := recordID / 1000
	targetDir := filepath.Join(chunkDir, strconv.FormatInt(groupID, 10), strconv.FormatInt(recordID, 10))
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return err
	}

	for _, c := range chunks {
		path := filepath.Join(targetDir, fmt.Sprintf("chunk_%04d", c.SeqNo))
		var b strings.Builder
		for _, ml := range c.Lines {
			b.WriteString(formatMarkedLine(ml))
			b.WriteByte('\n')
		}
		if err := os.WriteFile(path, []byte(strings.TrimRight(b.String(), "\n")), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func formatMarkedLine(ml MarkedLine) string {
	base := lineRawForChunking(ml.Line)
	mark := strings.TrimSpace(ml.Mark)
	if mark == "" {
		mark = "r"
	}
	return mark + " " + base
}
*/

func lineRawForChunking(line Line) string {
	// base := strings.TrimSpace(line.Raw)
	// if base != "" {
	// 	return base
	// }
	if line.LineType == "image" {
		return ""
	}

	return fmt.Sprintf("%d %d %s %s", line.LineNo, line.PageNo, line.LineType, line.Content)
}

func lineRawByteSize(line Line) int {
	return len([]byte(lineRawForChunking(line)))
}

func buildProtectedBlocks(lines []Line, _ int) []*protectedBlock {
	out := make([]*protectedBlock, len(lines))

	for i := 0; i < len(lines); i++ {
		if out[i] != nil {
			continue
		}
		lt := strings.ToLower(strings.TrimSpace(lines[i].LineType))

		if isTableType(lt) || isFormulaType(lt) {
			j := i + 1
			for j < len(lines) {
				nlt := strings.ToLower(strings.TrimSpace(lines[j].LineType))
				if nlt != lt {
					break
				}
				j++
			}
			markBlock(out, &protectedBlock{start: i, end: j, splittable: false})
			i = j - 1
			continue
		}

		if !isListType(lt) {
			continue
		}

		if !isChunkListCandidate(lines[i]) {
			continue
		}

		j := i + 1
		for j < len(lines) && isListType(strings.ToLower(strings.TrimSpace(lines[j].LineType))) && isChunkListCandidate(lines[j]) {
			j++
		}

		if j-i >= 2 {
			markBlock(out, &protectedBlock{start: i, end: j, splittable: false})
		}
		i = j - 1
	}

	return out
}

func markBlock(index []*protectedBlock, block *protectedBlock) {
	for i := block.start; i < block.end; i++ {
		index[i] = block
	}
}

func adjustChunkEnd(start int, target int, total int, blockIndex []*protectedBlock) int {
	cut := target
	seen := map[int]struct{}{}

	for {
		if cut <= start || cut >= total {
			return cut
		}
		if _, ok := seen[cut]; ok {
			return cut
		}
		seen[cut] = struct{}{}

		b := splitUnsplittableBlock(cut, blockIndex)
		if b == nil {
			return cut
		}

		before := b.start
		after := b.end
		if before <= start {
			cut = after
			continue
		}
		if after >= total {
			cut = before
			continue
		}

		if absInt(target-before) < absInt(after-target) {
			cut = before
		} else {
			cut = after
		}
	}
}

func splitUnsplittableBlock(cut int, blockIndex []*protectedBlock) *protectedBlock {
	if cut <= 0 || cut >= len(blockIndex) {
		return nil
	}
	left := blockIndex[cut-1]
	right := blockIndex[cut]
	if left == nil || right == nil || left != right {
		return nil
	}
	if left.splittable {
		return nil
	}
	return left
}

func isChunkListCandidate(line Line) bool {
	if !isListType(line.LineType) {
		return false
	}
	content := strings.TrimSpace(line.Content)
	if content == "" {
		return false
	}
	if numericSectionPattern.MatchString(content) {
		return false
	}
	return true
}

func isListType(lineType string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(lineType)), "list-item")
}

func isTableType(lineType string) bool {
	return strings.Contains(lineType, "table")
}

func isFormulaType(lineType string) bool {
	return strings.Contains(lineType, "formula")
}

type chunkStatusParams struct {
	RecordID        int64
	FileType        string
	InputFilename   string
	NumPages        int
	NumLines        int
	NumLabeledLines int
	NumChunks       int
	Start           time.Time
	DurationMs      int64
	ProcErr         error
}

type summaryStatusParams struct {
	RecordID      int64
	FileType      string
	InputFilename string
	Start         time.Time
	DurationMs    int64
	ProcErr       error
}

func appendChunkedStatus(raw string, p chunkStatusParams) (string, error) {
	entries := decodeStatus(raw)
	entry := map[string]any{
		"record_id":         strconv.FormatInt(p.RecordID, 10),
		"file_type":         sanitizeUTF8Text(strings.ToLower(strings.TrimSpace(p.FileType))),
		"operation":         "chunked",
		"input_filename":    sanitizeUTF8Text(p.InputFilename),
		"num_pages":         p.NumPages,
		"num_lines":         p.NumLines,
		"num_labeled_lines": p.NumLabeledLines,
		"num_chunks":        p.NumChunks,
		"start_time":        p.Start.Format(defaultStatusTime),
		"ms_used":           p.DurationMs,
	}
	if p.ProcErr == nil {
		entry["proc_status"] = "success"
	} else {
		entry["proc_status"] = "failed"
		entry["error"] = sanitizeUTF8Text(p.ProcErr.Error())
	}

	replaced := false
	out := make([]map[string]any, 0, len(entries)+1)
	for _, e := range entries {
		op := strings.ToLower(strings.TrimSpace(asString(e["operation"])))
		if op != "chunked" {
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

func appendSummariesStatus(raw string, p summaryStatusParams) (string, error) {
	entries := decodeStatus(raw)
	entry := map[string]any{
		"record_id":      strconv.FormatInt(p.RecordID, 10),
		"file_type":      sanitizeUTF8Text(strings.ToLower(strings.TrimSpace(p.FileType))),
		"operation":      "generate_summaries",
		"input_filename": sanitizeUTF8Text(p.InputFilename),
		"start_time":     p.Start.Format(defaultStatusTime),
		"ms_used":        p.DurationMs,
	}
	if p.ProcErr == nil {
		entry["proc_status"] = "success"
	} else {
		entry["proc_status"] = "failed"
		entry["error"] = sanitizeUTF8Text(p.ProcErr.Error())
	}

	replaced := false
	out := make([]map[string]any, 0, len(entries)+1)
	for _, e := range entries {
		op := strings.ToLower(strings.TrimSpace(asString(e["operation"])))
		if op != "generate_summaries" {
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

func detectChunkStatusFileType(rec InputRecord, inputFilename string) string {
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

func decodeStatus(raw string) []map[string]any {
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

func uniquePages(lines []Line) int {
	if len(lines) == 0 {
		return 0
	}
	seen := map[int]struct{}{}
	for _, l := range lines {
		if l.PageNo <= 0 {
			continue
		}
		seen[l.PageNo] = struct{}{}
	}
	return len(seen)
}

func asString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case json.Number:
		return x.String()
	case float64:
		return strconv.FormatInt(int64(x), 10)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", x)
	}
}

func envInt(key string, fallback int, min int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	if n < min {
		return min
	}
	return n
}

func envFloat(key string, fallback float64, minVal float64) float64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	n, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return fallback
	}
	if n < minVal {
		return minVal
	}
	return n
}

func loadFixedSizeTopicModelFromEnv() (modelRef string, modelPath string, cfg structureModelConfig, err error) {
	modelRef = strings.TrimSpace(os.Getenv("EXTRACT_TOPIC_MODEL_NAME"))
	if modelRef == "" {
		return "", "", structureModelConfig{ModelName: DefaultChunkTopicModelName}, nil
	}

	modelPath, err = resolveModelsFilePath("CHUNK_EXTRACT_TOPIC_MODELS_FILE")
	if err != nil {
		return modelRef, "", structureModelConfig{ModelName: modelRef}, nil
	}
	raw, err := os.ReadFile(modelPath)
	if err != nil {
		return modelRef, modelPath, structureModelConfig{}, fmt.Errorf("(MID_26042017) read %s failed: %w", modelPath, err)
	}
	parsed := ApiTypes.LLMModelsFile{}
	if err := parseTOMLMap(raw, &parsed); err != nil {
		return modelRef, modelPath, structureModelConfig{}, fmt.Errorf("(MID_26042018) parse %s failed: %w", modelPath, err)
	}
	modelDef, ok := parsed[modelRef]
	if !ok {
		return modelRef, modelPath, structureModelConfig{ModelName: modelRef}, nil
	}
	if strings.TrimSpace(modelDef.ModelName) == "" {
		return modelRef, modelPath, structureModelConfig{}, fmt.Errorf("(MID_26042019) model %q in %s missing model_name", modelRef, modelPath)
	}
	return modelRef, modelPath, structureModelConfig{
		ModelName:    strings.TrimSpace(modelDef.ModelName),
		APIKey:       strings.TrimSpace(modelDef.APIKey),
		BaseURL:      strings.TrimSpace(modelDef.BaseURL),
		TimeoutSec:   modelDef.TimeoutSec,
		ThinkingType: normalizeThinkingType(strings.TrimSpace(modelDef.ThinkingType)),
	}, nil
}

func loadFixedSizeSummaryModelFromEnv() (modelRef string, modelPath string, cfg structureModelConfig, err error) {
	modelRef = strings.TrimSpace(os.Getenv("CHUNK_SUMMARY_MODEL_NAME"))
	if modelRef == "" {
		modelRef = strings.TrimSpace(os.Getenv("EXTRACT_TOPIC_MODEL_NAME"))
	}
	if modelRef == "" {
		return "", "", structureModelConfig{ModelName: DefaultChunkTopicModelName}, nil
	}

	modelPath, err = resolveModelsFilePath("CHUNK_SUMMARY_MODELS_FILE")
	if err != nil {
		if strings.TrimSpace(os.Getenv("CHUNK_EXTRACT_TOPIC_MODELS_FILE")) != "" {
			modelPath, err = resolveModelsFilePath("CHUNK_EXTRACT_TOPIC_MODELS_FILE")
		}
		if err != nil {
			return modelRef, "", structureModelConfig{ModelName: modelRef}, nil
		}
	}
	raw, err := os.ReadFile(modelPath)
	if err != nil {
		return modelRef, modelPath, structureModelConfig{}, fmt.Errorf("(MID_26042906) read %s failed: %w", modelPath, err)
	}
	parsed := ApiTypes.LLMModelsFile{}
	if err := parseTOMLMap(raw, &parsed); err != nil {
		return modelRef, modelPath, structureModelConfig{}, fmt.Errorf("(MID_26042907) parse %s failed: %w", modelPath, err)
	}
	modelDef, ok := parsed[modelRef]
	if !ok {
		return modelRef, modelPath, structureModelConfig{ModelName: modelRef}, nil
	}
	if strings.TrimSpace(modelDef.ModelName) == "" {
		return modelRef, modelPath, structureModelConfig{}, fmt.Errorf("(MID_26042908) model %q in %s missing model_name", modelRef, modelPath)
	}
	return modelRef, modelPath, structureModelConfig{
		ModelName:    strings.TrimSpace(modelDef.ModelName),
		APIKey:       strings.TrimSpace(modelDef.APIKey),
		BaseURL:      strings.TrimSpace(modelDef.BaseURL),
		TimeoutSec:   modelDef.TimeoutSec,
		ThinkingType: normalizeThinkingType(strings.TrimSpace(modelDef.ThinkingType)),
	}, nil
}

func loadFixedSizeTopicPromptFromEnv() (promptText string, promptRef string, promptPath string, promptErr error) {
	promptRef = strings.TrimSpace(os.Getenv("EXTRACT_TOPIC_PROMPT"))
	if promptRef == "" {
		return "", "", "", errors.New("(MID_26042020) missing EXTRACT_TOPIC_PROMPT")
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
			return "", promptRef, candidate, errors.New("(MID_26042021) prompt file is empty")
		}
		return text, promptRef, candidate, nil
	}

	if strings.Contains(promptRef, "\n") || strings.Contains(promptRef, " ") {
		return strings.TrimSpace(promptRef), "inline", "", nil
	}
	if lastErr == nil {
		lastErr = os.ErrNotExist
	}
	return "", promptRef, "", fmt.Errorf("(MID_26042022) prompt file not found: %w", lastErr)
}

func loadFixedSizeSummaryPromptFromEnv() (promptText string, promptRef string, promptPath string, promptErr error) {
	promptRef = strings.TrimSpace(os.Getenv("GENERATE_SUMMARY_PROMPT"))
	if promptRef == "" {
		promptRef = strings.TrimSpace(os.Getenv("EXTRACT_TOPIC_PROMPT"))
	}
	if promptRef == "" {
		return "", "", "", errors.New("(MID_26042909) missing GENERATE_SUMMARY_PROMPT")
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
			return "", promptRef, candidate, errors.New("(MID_26042910) summary prompt file is empty")
		}
		return text, promptRef, candidate, nil
	}
	if strings.Contains(promptRef, "\n") || strings.Contains(promptRef, " ") {
		return strings.TrimSpace(promptRef), "inline", "", nil
	}
	if lastErr == nil {
		lastErr = os.ErrNotExist
	}
	return "", promptRef, "", fmt.Errorf("(MID_26042911) summary prompt file not found: %w", lastErr)
}

func min(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func sanitizeUTF8Text(s string) string {
	return strings.TrimSpace(strings.ToValidUTF8(s, "?"))
}
