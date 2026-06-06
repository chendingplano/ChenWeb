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
	"sync"
	"time"

	"github.com/chendingplano/deepdoc/server/api/kbsearch"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/google/uuid"
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
	SourceLanguage  string
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
	Store                       Store
	Extractor                   LLMJSONExtractor
	FallbackExtractor           LLMJSONExtractor
	FallbackModelName           string
	Embedder                    Embedder
	Logger                      ApiTypes.JimoLogger
	Now                         func() time.Time
	ChunkDir                    string
	ArtifactWebDir              string
	ChunkSize                   int
	OverlapPercent              int
	ModelRef                    string
	ModelCfgPath                string
	ModelErr                    error
	ModelName                   string
	PromptText                  string
	PromptRef                   string
	PromptPath                  string
	PromptErr                   error
	SummaryGroupSize            int
	GenerateSummaryMaxTasks     int
	SummaryModelRef             string
	SummaryModelCfgPath         string
	SummaryModelErr             error
	SummaryModelName            string
	SummaryPromptText           string
	SummaryPromptRef            string
	SummaryPromptPath           string
	SummaryPromptErr            error
	TranslationModelRef         string
	TranslationModelCfgPath     string
	TranslationModelErr         error
	TranslationModelName        string
	TranslationEnabled          bool
	TopicEmbeddingModelName     string
	SummaryEmbeddingModelName   string
	CategorySimilarityMinScore  float64
	ProcLogger                  DocProcLogger
	GenerateSummary             func(ctx context.Context, recordID int64, level int, seqNo int, lines []MarkedLine, children []SummaryItem) (summaryGenerateResult, error)
	lastDocProcSummaryExtraInfo map[string]any
	summaryProgressTracker      *summaryProgressTracker
}

type summaryProgressTracker struct {
	mu           sync.Mutex
	Total        int
	Completed    int
	LastProgress string
	Persist      func(progress string) error
}

func (t *summaryProgressTracker) advance() string {
	if t == nil {
		return ""
	}
	t.mu.Lock()
	if t.Completed < t.Total {
		t.Completed++
	}
	progress := formatSummaryProgress(t.Completed, t.Total)
	t.LastProgress = progress
	t.mu.Unlock()
	return progress
}

func (t *summaryProgressTracker) current() string {
	if t == nil {
		return ""
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.LastProgress != "" {
		return t.LastProgress
	}
	return formatSummaryProgress(t.Completed, t.Total)
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
// a dedicated OpenAI-compatible client if EMBEDDING_MODEL_NAME is set with a valid config,
// then the extractor itself if it implements Embedder, otherwise nil. Summary embedding follows the
// same fallback pattern via EMBEDDING_MODEL_NAME. A default logger is created when none is provided.
func NewFixedSizeChunkingService(store Store, extractor LLMJSONExtractor, _ ApiTypes.JimoLogger) *FixedSizeChunkingService {
	logger := loggerutil.CreateDefaultLogger("MID_26041901")
	modelRef, modelCfgPath, modelCfg, modelErr := loadFixedSizeTopicModelFromEnv()
	promptText, promptRef, promptPath, promptErr := loadFixedSizeTopicPromptFromEnv()
	summaryModelRef, summaryModelCfgPath, summaryModelCfg, summaryModelErr := loadFixedSizeSummaryModelFromEnv()
	summaryPromptText, summaryPromptRef, summaryPromptPath, summaryPromptErr := loadFixedSizeSummaryPromptFromEnv()
	translationModelRef, translationModelCfgPath, translationModelCfg, translationModelErr := loadOptionalModelConfigFromEnv("TRANSLATION_MODEL_NAME", "MODEL_DEF_FILE")
	applyStructureModelConfigToExtractor(extractor, modelCfg)
	var fallbackExtractor LLMJSONExtractor
	var fallbackModelName string
	if fallbackModelRef := strings.TrimSpace(os.Getenv("EXTRACT_TOPIC_FALLBACK")); fallbackModelRef != "" {
		_, _, fallbackCfg, _ := loadOptionalModelConfigFromEnv("EXTRACT_TOPIC_FALLBACK", "CHUNK_EXTRACT_TOPIC_MODELS_FILE")
		if strings.TrimSpace(fallbackCfg.ModelName) == "" {
			fallbackCfg.ModelName = fallbackModelRef
		}
		timeoutSec := fallbackCfg.TimeoutSec
		if timeoutSec <= 0 {
			timeoutSec = 60
		}
		fallbackExtractor = &llmclients.OpenAIJSONClient{
			HTTPClient: &http.Client{Timeout: time.Duration(timeoutSec) * time.Second},
			ModelName:  fallbackCfg.ModelName,
			APIKey:     fallbackCfg.APIKey,
			BaseURL:    fallbackCfg.BaseURL,
		}
		fallbackModelName = fallbackCfg.ModelName
	}
	topicEmbeddingModelRef := strings.TrimSpace(os.Getenv("EMBEDDING_MODEL_NAME"))
	var embedder Embedder
	var topicEmbeddingModelName string
	if topicEmbeddingModelRef != "" {
		_, _, embCfg, embErr := loadModelConfigFromEnv("EMBEDDING_MODEL_NAME", "")
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
	summaryEmbeddingModelRef := strings.TrimSpace(os.Getenv("EMBEDDING_MODEL_NAME"))
	if summaryEmbeddingModelRef == "" {
		logger.Error("EMBEDDING_MODEL_NAME is not defined")
		panic("EMBEDDING_MODEL_NAME is not defined")
	}
	var summaryEmbeddingModelName string
	if _, _, embCfg, embErr := loadModelConfigFromEnv("EMBEDDING_MODEL_NAME", ""); embErr == nil && strings.TrimSpace(embCfg.ModelName) != "" {
		summaryEmbeddingModelName = embCfg.ModelName
	} else {
		summaryEmbeddingModelName = summaryEmbeddingModelRef
	}
	categorySimilarityMinScore := envFloat("CATEGORY_SIMILARITY_MIN_SCORE", DefaultCategorySimilarityMinScore, 0)
	return &FixedSizeChunkingService{
		Store:                      store,
		Extractor:                  extractor,
		FallbackExtractor:          fallbackExtractor,
		FallbackModelName:          fallbackModelName,
		Logger:                     logger,
		Now:                        time.Now,
		ChunkDir:                   strings.TrimSpace(os.Getenv("ARTIFACT_DIR")),
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
		GenerateSummaryMaxTasks:    envInt("GENERATE_SUMMARY_MAX_TASKS", 1, 1),
		SummaryModelRef:            summaryModelRef,
		SummaryModelCfgPath:        summaryModelCfgPath,
		SummaryModelErr:            summaryModelErr,
		SummaryModelName:           summaryModelCfg.ModelName,
		SummaryPromptText:          summaryPromptText,
		SummaryPromptRef:           summaryPromptRef,
		SummaryPromptPath:          summaryPromptPath,
		SummaryPromptErr:           summaryPromptErr,
		TranslationModelRef:        translationModelRef,
		TranslationModelCfgPath:    translationModelCfgPath,
		TranslationModelErr:        translationModelErr,
		TranslationModelName:       translationModelCfg.ModelName,
		TranslationEnabled:         translationModelErr == nil && strings.TrimSpace(translationModelCfg.ModelName) != "",
		Embedder:                   embedder,
		TopicEmbeddingModelName:    topicEmbeddingModelName,
		SummaryEmbeddingModelName:  summaryEmbeddingModelName,
		CategorySimilarityMinScore: categorySimilarityMinScore,
		ProcLogger:                 DocProcLogger{DB: ApiTypes.ProjectDBHandle},
	}
}

func (s *FixedSizeChunkingService) DocProcModelNames() []string {
	return dedupeNonEmpty([]string{
		s.ModelName,
		s.FallbackModelName,
		s.SummaryModelName,
		s.TranslationModelName,
		s.TopicEmbeddingModelName,
		s.SummaryEmbeddingModelName,
	})
}

func (s *FixedSizeChunkingService) DocProcPromptNames() []string {
	return dedupeNonEmpty([]string{
		s.PromptRef,
		s.SummaryPromptRef,
	})
}

func (s *FixedSizeChunkingService) DocProcSummaryExtraInfo() map[string]any {
	if len(s.lastDocProcSummaryExtraInfo) == 0 {
		return nil
	}
	out := make(map[string]any, len(s.lastDocProcSummaryExtraInfo))
	for k, v := range s.lastDocProcSummaryExtraInfo {
		out[k] = v
	}
	return out
}

func (s *FixedSizeChunkingService) setDocProcSummaryExtraInfo(extra map[string]any) {
	if len(extra) == 0 {
		s.lastDocProcSummaryExtraInfo = nil
		return
	}
	cloned := make(map[string]any, len(extra))
	for k, v := range extra {
		cloned[k] = v
	}
	s.lastDocProcSummaryExtraInfo = cloned
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
	lines := ParseBlockBufferLines(buf)
	return s.handleChunkLines(ctx, rec, inputFilename, start, lines)
}

func (s *FixedSizeChunkingService) HandleGenerateTopicsInput(ctx context.Context, recordID int64, inputFilename string, inputFile []byte) error {
	if s.Store == nil {
		return errors.New("(MID_26052301) store is nil")
	}
	if s.Extractor == nil {
		return errors.New("(MID_26052302) fixed-size topic extractor is nil")
	}
	if recordID <= 0 {
		return fmt.Errorf("(MID_26052303) invalid record_id: %d", recordID)
	}

	start := s.Now()
	rec, err := s.Store.GetInputRecord(ctx, recordID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(s.ChunkDir) == "" {
		procErr := errors.New("(MID_26052304) missing ARTIFACT_DIR")
		s.failAndPersistTopics(ctx, rec, inputFilename, start, 0, procErr)
		return procErr
	}
	if strings.TrimSpace(s.ArtifactWebDir) == "" {
		procErr := errors.New("(MID_26052305) missing ARTIFACT_WEB_DIR")
		s.failAndPersistTopics(ctx, rec, inputFilename, start, 0, procErr)
		return procErr
	}
	if strings.TrimSpace(inputFilename) == "" {
		procErr := errors.New("(MID_26052306) missing input filename")
		s.failAndPersistTopics(ctx, rec, inputFilename, start, 0, procErr)
		return procErr
	}
	if s.ModelErr != nil {
		s.failAndPersistTopics(ctx, rec, inputFilename, start, 0, s.ModelErr)
		return s.ModelErr
	}
	if s.PromptErr != nil {
		procErr := fmt.Errorf("(MID_26052307) load fixed-size topic prompt %q failed: %w", s.PromptRef, s.PromptErr)
		s.failAndPersistTopics(ctx, rec, inputFilename, start, 0, procErr)
		return procErr
	}

	lines, err := ParseInputLinesIncludingTOC(inputFile)
	if err != nil {
		s.failAndPersistTopics(ctx, rec, inputFilename, start, 0, err)
		return err
	}
	return s.handleGenerateTopicsLines(ctx, rec, inputFilename, start, lines)
}

func (s *FixedSizeChunkingService) HandleGenerateTopicsBlockInput(ctx context.Context, recordID int64, inputFilename string, buf *BlockBuffer) error {
	if buf == nil {
		return errors.New("(MID_26052308) block buffer is nil")
	}
	if s.Store == nil {
		return errors.New("(MID_26052309) store is nil")
	}
	if s.Extractor == nil {
		return errors.New("(MID_26052310) fixed-size topic extractor is nil")
	}
	if recordID <= 0 {
		return fmt.Errorf("(MID_26052311) invalid record_id: %d", recordID)
	}

	start := s.Now()
	rec, err := s.Store.GetInputRecord(ctx, recordID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(s.ChunkDir) == "" {
		procErr := errors.New("(MID_26052312) missing ARTIFACT_DIR")
		s.failAndPersistTopics(ctx, rec, inputFilename, start, 0, procErr)
		return procErr
	}
	if strings.TrimSpace(s.ArtifactWebDir) == "" {
		procErr := errors.New("(MID_26052313) missing ARTIFACT_WEB_DIR")
		s.failAndPersistTopics(ctx, rec, inputFilename, start, 0, procErr)
		return procErr
	}
	if strings.TrimSpace(inputFilename) == "" {
		procErr := errors.New("(MID_26052314) missing input filename")
		s.failAndPersistTopics(ctx, rec, inputFilename, start, 0, procErr)
		return procErr
	}
	if s.ModelErr != nil {
		s.failAndPersistTopics(ctx, rec, inputFilename, start, 0, s.ModelErr)
		return s.ModelErr
	}
	if s.PromptErr != nil {
		procErr := fmt.Errorf("(MID_26052315) load fixed-size topic prompt %q failed: %w", s.PromptRef, s.PromptErr)
		s.failAndPersistTopics(ctx, rec, inputFilename, start, 0, procErr)
		return procErr
	}

	return s.handleGenerateTopicsLines(ctx, rec, inputFilename, start, ParseBlockBufferLines(buf))
}

func (s *FixedSizeChunkingService) HandleGenerateSummariesInput(ctx context.Context, recordID int64, inputFilename string, inputFile []byte) error {
	if s.Store == nil {
		return errors.New("(MID_26052316) store is nil")
	}
	if s.Extractor == nil && s.GenerateSummary == nil {
		return errors.New("(MID_26052317) fixed-size summary extractor is nil")
	}
	if recordID <= 0 {
		return fmt.Errorf("(MID_26052318) invalid record_id: %d", recordID)
	}

	start := s.Now()
	rec, err := s.Store.GetInputRecord(ctx, recordID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(s.ChunkDir) == "" {
		procErr := errors.New("(MID_26052319) missing ARTIFACT_DIR")
		s.failAndPersistSummaries(ctx, rec, inputFilename, start, procErr)
		return procErr
	}
	if strings.TrimSpace(s.ArtifactWebDir) == "" {
		procErr := errors.New("(MID_26052320) missing ARTIFACT_WEB_DIR")
		s.failAndPersistSummaries(ctx, rec, inputFilename, start, procErr)
		return procErr
	}
	if strings.TrimSpace(inputFilename) == "" {
		procErr := errors.New("(MID_26052321) missing input filename")
		s.failAndPersistSummaries(ctx, rec, inputFilename, start, procErr)
		return procErr
	}
	if s.GenerateSummary == nil && s.SummaryModelErr != nil {
		s.failAndPersistSummaries(ctx, rec, inputFilename, start, s.SummaryModelErr)
		return s.SummaryModelErr
	}
	if s.GenerateSummary == nil && s.SummaryPromptErr != nil {
		procErr := fmt.Errorf("(MID_26052322) load fixed-size summary prompt %q failed: %w", s.SummaryPromptRef, s.SummaryPromptErr)
		s.failAndPersistSummaries(ctx, rec, inputFilename, start, procErr)
		return procErr
	}

	lines, err := ParseInputLinesIncludingTOC(inputFile)
	if err != nil {
		s.failAndPersistSummaries(ctx, rec, inputFilename, start, err)
		return err
	}
	return s.handleGenerateSummariesLines(ctx, rec, inputFilename, start, lines)
}

func (s *FixedSizeChunkingService) HandleGenerateSummariesBlockInput(ctx context.Context, recordID int64, inputFilename string, buf *BlockBuffer) error {
	if buf == nil {
		return errors.New("(MID_26052323) block buffer is nil")
	}
	if s.Store == nil {
		return errors.New("(MID_26052324) store is nil")
	}
	if s.Extractor == nil && s.GenerateSummary == nil {
		return errors.New("(MID_26052325) fixed-size summary extractor is nil")
	}
	if recordID <= 0 {
		return fmt.Errorf("(MID_26052326) invalid record_id: %d", recordID)
	}

	start := s.Now()
	rec, err := s.Store.GetInputRecord(ctx, recordID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(s.ChunkDir) == "" {
		procErr := errors.New("(MID_26052327) missing ARTIFACT_DIR")
		s.failAndPersistSummaries(ctx, rec, inputFilename, start, procErr)
		return procErr
	}
	if strings.TrimSpace(s.ArtifactWebDir) == "" {
		procErr := errors.New("(MID_26052328) missing ARTIFACT_WEB_DIR")
		s.failAndPersistSummaries(ctx, rec, inputFilename, start, procErr)
		return procErr
	}
	if strings.TrimSpace(inputFilename) == "" {
		procErr := errors.New("(MID_26052329) missing input filename")
		s.failAndPersistSummaries(ctx, rec, inputFilename, start, procErr)
		return procErr
	}
	if s.GenerateSummary == nil && s.SummaryModelErr != nil {
		s.failAndPersistSummaries(ctx, rec, inputFilename, start, s.SummaryModelErr)
		return s.SummaryModelErr
	}
	if s.GenerateSummary == nil && s.SummaryPromptErr != nil {
		procErr := fmt.Errorf("(MID_26052330) load fixed-size summary prompt %q failed: %w", s.SummaryPromptRef, s.SummaryPromptErr)
		s.failAndPersistSummaries(ctx, rec, inputFilename, start, procErr)
		return procErr
	}

	return s.handleGenerateSummariesLines(ctx, rec, inputFilename, start, ParseBlockBufferLines(buf))
}

// handleChunkLines runs the fixed-size chunking phase and persists the chunk
// artifact plus chunk status for a pre-parsed set of lines.
func (s *FixedSizeChunkingService) handleChunkLines(ctx context.Context, rec InputRecord, inputFilename string, start time.Time, lines []Line) error {
	numPages := uniquePages(lines)
	numLines := len(lines)
	fileType := detectChunkStatusFileType(rec, inputFilename)
	s.setDocProcSummaryExtraInfo(nil)
	chunks, err := BuildChunks(lines, ChunkOptions{ChunkSize: s.ChunkSize, OverlapPercent: s.OverlapPercent})
	if err != nil {
		s.failAndPersist(ctx, rec, inputFilename, numPages, numLines, 0, start, err)
		return err
	}
	if err := validateChunksNonEmpty(chunks, "built chunks"); err != nil {
		s.failAndPersist(ctx, rec, inputFilename, numPages, numLines, 0, start, err)
		return err
	}
	storeChunksInContext(ctx, chunks)

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

	if err := s.Store.InsertChunkRun(ctx, ChunkRunRecord{
		SourceRecordID: rec.ID,
		ChunkingMethod: "fix-size",
		ChunkingSize:   s.ChunkSize,
		OverlapPercent: s.OverlapPercent,
	}); err != nil {
		s.failAndPersist(ctx, rec, inputFilename, numPages, numLines, len(chunks), start, err)
		return err
	}

	statusRaw, err := appendChunkedStatus(rec.StatusRaw, chunkStatusParams{
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
		"num_pages", numPages,
		"num_lines", numLines,
		"num_chunks", len(chunks),
	)
	s.setDocProcSummaryExtraInfo(map[string]any{
		"total_chunks": len(chunks),
	})
	return nil
}

func (s *FixedSizeChunkingService) handleGenerateTopicsLines(ctx context.Context, rec InputRecord, inputFilename string, start time.Time, lines []Line) error {
	artifactBase := buildFixedSizeArtifactBase(rec, inputFilename)
	s.setDocProcSummaryExtraInfo(nil)
	chunks, err := loadChunksFromArtifactFile(s.ChunkDir, rec.ID, artifactBase+".chunks", lines)
	if err != nil {
		s.failAndPersistTopics(ctx, rec, inputFilename, start, 0, err)
		return err
	}

	totalChunks := len(chunks)
	initialProgress := extractTopicsProgressFull(0, totalChunks)
	s.Logger.Info("(MID_26052801) extract_topics: starting, updating status to initial progress",
		"record_id", rec.ID, "total_chunks", totalChunks, "progress", initialProgress)
	if updateErr := s.updateInputStatusLocked(ctx, rec.ID, nil, func(current string) (string, error) {
		return updateTopicChunkStatusProgress(current, initialProgress)
	}); updateErr != nil {
		s.Logger.Warn("(MID_26052802) failed to persist initial topic progress", "record_id", rec.ID, "error", updateErr)
	}

	topics := make([]TopicItem, 0, len(chunks))
	seqStart := 1
	topicsSoFar := 0
	stopTopics := func(bgCtx context.Context) {
		s.stopAndPersistTopics(bgCtx, rec, inputFilename, start, len(topics))
	}
	for _, chunk := range chunks {
		// Check for user stop before each LLM call.
		if CheckAndHandleStop(ctx, stopTopics) {
			return ErrPipelineStopped
		}

		chunkStart := s.Now()
		chunkTopics, chunkErr := extractTopicsFromMarkedLinesWithLLM(
			ctx,
			rec.ID,
			s.Extractor,
			s.Logger,
			s.ModelName,
			s.PromptText,
			s.PromptRef,
			chunk.Lines,
			seqStart,
			"chunk_seqno",
			chunk.SeqNo,
		)
		chunkMSUsed := s.Now().Sub(chunkStart).Milliseconds()
		usedFallback := false
		if chunkErr != nil || len(chunkTopics) == 0 {
			if s.FallbackExtractor != nil {
				// Check for stop before attempting the fallback LLM call.
				if CheckAndHandleStop(ctx, stopTopics) {
					return ErrPipelineStopped
				}
				fbStart := s.Now()
				fbTopics, fbErr := extractTopicsFromMarkedLinesWithLLM(
					ctx,
					rec.ID,
					s.FallbackExtractor,
					s.Logger,
					s.FallbackModelName,
					s.PromptText,
					s.PromptRef,
					chunk.Lines,
					seqStart,
					"chunk_seqno",
					chunk.SeqNo,
				)
				chunkMSUsed = s.Now().Sub(fbStart).Milliseconds()
				if fbErr != nil || len(fbTopics) == 0 {
					if CheckAndHandleStop(ctx, stopTopics) {
						return ErrPipelineStopped
					}
					s.Logger.Warn("topic generation empty for both primary and fallback, continuing",
						"record_id", rec.ID, "chunk_seqno", chunk.SeqNo,
						"primary_error", chunkErr, "fallback_error", fbErr,
					)
					chunkTopics = []TopicItem{}
				} else {
					chunkTopics = fbTopics
					usedFallback = true
				}
			} else if chunkErr != nil {
				if CheckAndHandleStop(ctx, stopTopics) {
					return ErrPipelineStopped
				}
				s.failAndPersistTopics(ctx, rec, inputFilename, start, len(topics), fmt.Errorf("(MID_26042016) %w", chunkErr))
				return chunkErr
			}
		}
		topicsSoFar += len(chunkTopics)
		chunkProgress := extractTopicsProgressFull(chunk.SeqNo, totalChunks)

		// s.Logger.Info("(MID_26052803) extract_topics: chunk done, updating status progress",
		// 	"record_id", rec.ID, "chunk_seqno", chunk.SeqNo, "total_chunks", totalChunks,
		// 	"progress", chunkProgress, "num_topics", len(chunkTopics), "topics_so_far", topicsSoFar)
		if updateErr := s.updateInputStatusLocked(ctx, rec.ID, nil, func(current string) (string, error) {
			return updateTopicChunkStatusProgress(current, chunkProgress)
		}); updateErr != nil {
			s.Logger.Warn("(MID_26052804) failed to persist topic progress",
				"record_id", rec.ID, "chunk_seqno", chunk.SeqNo, "error", updateErr)
		}

		// s.Logger.Info("(MID_26052805) extract_topics: inserting doc_proc_logs record",
		// 	"record_id", rec.ID, "chunk_seqno", chunk.SeqNo, "num_topics", len(chunkTopics))
		s.logExtractTopicsChunk(ctx, rec.ID, chunk, totalChunks, chunkTopics, topicsSoFar, chunkMSUsed, chunkProgress, usedFallback)

		topics = append(topics, chunkTopics...)
		seqStart += len(chunkTopics)
	}
	topics = dedupeTopicItems(topics)

	outputFilename, err := writeTopicsFile(s.ChunkDir, rec.ID, artifactBase+".topics", topics)
	if err != nil {
		s.failAndPersistTopics(ctx, rec, inputFilename, start, len(topics), err)
		return err
	}
	if err := indexTopicsInTreeDir(s.Logger, s.ArtifactWebDir, rec.ID, topics); err != nil {
		s.failAndPersistTopics(ctx, rec, inputFilename, start, len(topics), err)
		return err
	}
	if err := s.embedAndWriteTopics(ctx, rec.ID, artifactBase, topics); err != nil {
		s.failAndPersistTopics(ctx, rec, inputFilename, start, len(topics), err)
		return err
	}
	if err := ReplaceTopicArtifactsForRecord(ctx, rec.ID, topics, s.Logger); err != nil {
		s.failAndPersistTopics(ctx, rec, inputFilename, start, len(topics), err)
		return err
	}
	if err := ReindexTopicSearchForRecord(ctx, rec.ID, s.Logger); err != nil {
		s.Logger.Warn("reindex topic search registry failed", "record_id", rec.ID, "error", err)
	}
	// Derive chunk -> topic "has-topic" line_overlap edges (registry-sourced ids).
	if connErr := WriteLineOverlapConnectionsFromRegistry(ctx, rec.ID, searchArtifactTopic, RelationHasTopic, chunksToBlocks(chunks)); connErr != nil {
		s.Logger.Warn("write has-topic connections failed", "record_id", rec.ID, "error", connErr)
	}
	if err := s.updateInputStatusLocked(ctx, rec.ID, nil, func(current string) (string, error) {
		return appendTopicStatus(current, topicStatusParams{
			RecordID:       rec.ID,
			FileType:       detectChunkStatusFileType(rec, inputFilename),
			InputFilename:  inputFilename,
			OutputFilename: outputFilename,
			NumTopics:      len(topics),
			Start:          start,
			DurationMs:     time.Since(start).Milliseconds(),
			ProcErr:        nil,
			Progress:       extractTopicsProgressFull(totalChunks, totalChunks),
		})
	}); err != nil {
		return err
	}

	s.Logger.Info("topic generation completed",
		"record_id", rec.ID,
		"num_chunks", len(chunks),
		"num_topics", len(topics),
		"model_name", s.ModelName,
	)
	s.setDocProcSummaryExtraInfo(map[string]any{
		"total_chunks":     len(chunks),
		"topics_generated": len(topics),
	})
	return nil
}

func (s *FixedSizeChunkingService) logExtractTopicsChunk(
	ctx context.Context,
	recordID int64,
	chunk Chunk,
	totalChunks int,
	topics []TopicItem,
	topicsSoFar int,
	msUsed int64,
	chunkProgress string,
	usedFallback bool,
) {
	if resolveDocProcLogDB(s.ProcLogger.DB) == nil {
		s.Logger.Warn("(MID_26052806) extract_topics: ProcLogger DB is nil, skipping doc_proc_logs insert",
			"record_id", recordID, "chunk_seqno", chunk.SeqNo)
		return
	}
	_, regularLines := chunkLineNumbers(chunk)
	extraJSON := docProcJSONOrNil(map[string]any{
		"chunk":         chunk.SeqNo,
		"total_chunks":  totalChunks,
		"num_topics":    len(topics),
		"topics_so_far": topicsSoFar,
		"percent":       extractTopicsProgress(chunk.SeqNo, totalChunks),
		"lines":         lineRangesFromNumbers(regularLines),
		"used_fallback": usedFallback,
	})
	callID := uuid.NewString()
	activityName := "generate_topic"
	modelNames := dedupeNonEmpty([]string{s.ModelName})
	if usedFallback && strings.TrimSpace(s.FallbackModelName) != "" {
		modelNames = dedupeNonEmpty([]string{s.FallbackModelName})
	}
	artifactJSON := docProcJSONOrNil(topics)
	if err := s.ProcLogger.LogExtractTopics(ctx, DocProcLogRecord{
		CallReason:    "extract topics",
		DocProcName:   "generate_topics",
		ModelNames:    modelNames,
		PromptName:    strings.TrimSpace(s.PromptRef),
		RecordID:      &recordID,
		ProcProgress:  nullableStringPtr(chunkProgress),
		LLMCallID:     &callID,
		ActivityName:  &activityName,
		ArtifactJSON:  artifactJSON,
		ExtraInfoJSON: extraJSON,
		MSUsed:        int64Ptr(msUsed),
	}, "MID-26052807"); err != nil {
		s.Logger.Warn("(MID_26052808) failed to write extract_topics log",
			"record_id", recordID, "chunk_seqno", chunk.SeqNo, "error", err)
	}
}

func (s *FixedSizeChunkingService) handleGenerateSummariesLines(ctx context.Context, rec InputRecord, inputFilename string, start time.Time, lines []Line) error {
	artifactBase := buildFixedSizeArtifactBase(rec, inputFilename)
	s.setDocProcSummaryExtraInfo(nil)
	s.summaryProgressTracker = nil
	defer func() { s.summaryProgressTracker = nil }()
	chunks, err := loadChunksFromArtifactFile(s.ChunkDir, rec.ID, artifactBase+".chunks", lines)
	if err != nil {
		s.failAndPersistSummaries(ctx, rec, inputFilename, start, err)
		return err
	}
	totalSummaries := totalPlannedSummaries(len(chunks), s.SummaryGroupSize)
	s.summaryProgressTracker = &summaryProgressTracker{
		Total: totalSummaries,
		Persist: func(progress string) error {
			return s.updateInputStatusLocked(ctx, rec.ID, nil, func(current string) (string, error) {
				return appendSummariesStatus(current, summaryStatusParams{
					RecordID:      rec.ID,
					FileType:      detectChunkStatusFileType(rec, inputFilename),
					InputFilename: inputFilename,
					Start:         start,
					DurationMs:    time.Since(start).Milliseconds(),
					ProcStatus:    "running",
					ProcProgress:  progress,
				})
			})
		},
	}

	if err := deleteSummaryFiles(s.ChunkDir, rec.ID); err != nil {
		s.failAndPersistSummaries(ctx, rec, inputFilename, start, err)
		return err
	}

	s.Logger.Info("generate summaries",
		"record_id", rec.ID,
		"model_name", s.SummaryModelName,
		"prompt", s.PromptRef)

	leafSummaries, leafErr := runConcurrent(ctx, s.GenerateSummaryMaxTasks, len(chunks), func(genCtx context.Context, i int) (SummaryItem, error) {
		chunk := chunks[i]
		res, err := s.generateSummary(genCtx, rec.ID, 0, chunk.SeqNo, chunk.Lines, nil)
		if err != nil {
			return SummaryItem{}, err
		}
		return SummaryItem{
			SummaryID:           buildSummaryID(rec.ID, 0, chunk.SeqNo),
			RecordID:            rec.ID,
			Level:               0,
			SeqNo:               chunk.SeqNo,
			Lines:               summaryInputLineRanges(chunk.Lines),
			Keywords:            res.Keywords,
			KeywordsEn:          res.KeywordsEn,
			CategoryPaths:       res.CategoryPaths,
			CategoryNodes:       res.CategoryNodes,
			CategoryPathItems:   res.CategoryPathItems,
			CategoryPathItemsEn: res.CategoryPathItemsEn,
			Summary:             sanitizeTopicText(res.Summary),
			SummaryEn:           sanitizeTopicText(res.SummaryEn),
		}, nil
	})
	if leafErr != nil {
		if isCtxStopped(ctx) {
			s.stopAndPersistSummaries(context.Background(), rec, inputFilename, start)
			return ErrPipelineStopped
		}
		s.failAndPersistSummaries(ctx, rec, inputFilename, start, leafErr)
		return leafErr
	}
	for i := range leafSummaries {
		leafSummaries[i].Language = rec.SourceLanguage
	}
	s.Logger.Info("writing leaf summary files", "record_id", rec.ID, "count", len(leafSummaries), "source_language", rec.SourceLanguage)
	for _, item := range leafSummaries {
		if _, err := writeSummaryFile(s.ChunkDir, rec.ID, item); err != nil {
			s.failAndPersistSummaries(ctx, rec, inputFilename, start, err)
			return err
		}
	}

	allSummaries, _, err := buildSummaryTree(rec.ID, leafSummaries, s.SummaryGroupSize, s.GenerateSummaryMaxTasks, func(level int, seqNo int, children []SummaryItem) (summaryGenerateResult, error) {
		if isCtxStopped(ctx) {
			return summaryGenerateResult{}, ErrPipelineStopped
		}
		return s.generateSummary(ctx, rec.ID, level, seqNo, nil, children)
	})
	if err != nil {
		if isCtxStopped(ctx) {
			s.stopAndPersistSummaries(context.Background(), rec, inputFilename, start)
			return ErrPipelineStopped
		}
		s.failAndPersistSummaries(ctx, rec, inputFilename, start, err)
		return err
	}
	for i := range allSummaries {
		allSummaries[i].Language = rec.SourceLanguage
	}
	s.Logger.Info("writing non-leaf summary files", "record_id", rec.ID, "count", len(allSummaries), "source_language", rec.SourceLanguage)
	for _, item := range allSummaries {
		if item.Level == 0 {
			continue
		}
		if _, err := writeSummaryFile(s.ChunkDir, rec.ID, item); err != nil {
			s.failAndPersistSummaries(ctx, rec, inputFilename, start, err)
			return err
		}
	}
	if err := os.MkdirAll(s.ArtifactWebDir, 0o755); err != nil {
		s.failAndPersistSummaries(ctx, rec, inputFilename, start, err)
		return err
	}
	if err := removeSummaryTreeRecord(s.ArtifactWebDir, rec.ID); err != nil {
		s.failAndPersistSummaries(ctx, rec, inputFilename, start, err)
		return err
	}
	for _, item := range allSummaries {
		if err := writeSummaryTreeEntriesForItem(s.Logger, s.ArtifactWebDir, item); err != nil {
			s.failAndPersistSummaries(ctx, rec, inputFilename, start, err)
			return err
		}
	}
	var fixErr error
	allSummaries, fixErr = s.fixSummarySourceLanguage(ctx, rec.SourceLanguage, allSummaries)
	if fixErr != nil {
		s.failAndPersistSummaries(ctx, rec, inputFilename, start, fixErr)
		return fixErr
	}
	if err := validateSummaryArtifacts(rec.ID, rec.SourceLanguage, allSummaries, s.ChunkDir, s.ArtifactWebDir); err != nil {
		err = fmt.Errorf("(MID_26060401) summary sanity check failed: %w", err)
		s.failAndPersistSummaries(ctx, rec, inputFilename, start, err)
		return err
	}
	if err := s.embedAndWriteSummaries(ctx, rec.ID, allSummaries); err != nil {
		s.failAndPersistSummaries(ctx, rec, inputFilename, start, err)
		return err
	}
	s.Logger.Info("upserting summaries to kb.summaries", "record_id", rec.ID, "count", len(allSummaries), "source_language", rec.SourceLanguage)
	if err := ReplaceSummaryArtifactsForRecord(ctx, rec.ID, allSummaries, s.Logger); err != nil {
		s.failAndPersistSummaries(ctx, rec, inputFilename, start, err)
		return err
	}
	if err := ReindexSummarySearchForRecord(ctx, rec.ID, s.Logger); err != nil {
		s.Logger.Warn("reindex summary search registry failed", "record_id", rec.ID, "error", err)
	}
	// Derive chunk -> summary "has-summary" line_overlap edges for Level-0 summaries
	// only. Higher-level (wide-span) summaries are excluded from line_overlap and are
	// related via the summary rollup tree (structural) instead. See
	// KnowledgeStore/Capsules/coding-capsules/deep-wiki/artifact-graph-creation.md
	level0SummaryRefs := make([]ArtifactRef, 0, len(allSummaries))
	for _, item := range allSummaries {
		if item.Level != 0 {
			continue
		}
		level0SummaryRefs = append(level0SummaryRefs, ArtifactRef{
			Type:  searchArtifactSummary,
			ID:    kbsearch.BuildArtifactID(rec.ID, searchArtifactSummary, strconv.Itoa(item.SeqNo)),
			Spans: item.Lines,
		})
	}
	if connErr := WriteLineOverlapConnectionsForRefs(ctx, rec.ID, searchArtifactSummary, RelationHasSummary, chunksToBlocks(chunks), level0SummaryRefs); connErr != nil {
		s.Logger.Warn("write has-summary connections failed", "record_id", rec.ID, "error", connErr)
	}

	if err := s.updateInputStatusLocked(ctx, rec.ID, nil, func(current string) (string, error) {
		return appendSummariesStatus(current, summaryStatusParams{
			RecordID:      rec.ID,
			FileType:      detectChunkStatusFileType(rec, inputFilename),
			InputFilename: inputFilename,
			Start:         start,
			DurationMs:    time.Since(start).Milliseconds(),
			ProcStatus:    "success",
			ProcProgress:  s.currentSummaryProgress(),
			ProcErr:       nil,
		})
	}); err != nil {
		return err
	}

	s.Logger.Info("summary generation completed",
		"record_id", rec.ID,
		"num_chunks", len(chunks),
		"num_summaries", len(allSummaries),
		"model_name", s.SummaryModelName,
	)
	s.setDocProcSummaryExtraInfo(map[string]any{
		"total_chunks":        len(chunks),
		"total_lines":         len(lines),
		"num_summaries":       len(allSummaries),
		"proc_progress":       s.currentSummaryProgress(),
		"summaries_generated": len(allSummaries),
	})
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

func (s *FixedSizeChunkingService) generateSummary(
	ctx context.Context,
	recordID int64,
	level int,
	seqNo int,
	lines []MarkedLine, children []SummaryItem) (summaryGenerateResult, error) {

	startTime := time.Now()
	callID := uuid.NewString()
	lineSpans := summaryLogLineSpans(lines, children)
	logSummaryCall := func(procErr error) {
		procProgress := s.currentSummaryProgress()
		if procErr == nil && s.summaryProgressTracker != nil {
			procProgress = s.summaryProgressTracker.advance()
			if s.summaryProgressTracker.Persist != nil && procProgress != "" {
				if err := s.summaryProgressTracker.Persist(procProgress); err != nil {
					s.Logger.Warn("failed to persist generate_summaries progress", "record_id", recordID, "level", level, "seq", seqNo, "error", err)
				}
			}
		}
		if resolveDocProcLogDB(s.ProcLogger.DB) == nil {
			return
		}
		// Use background context when pipeline context is already cancelled (e.g. user stop).
		logCtx := ctx
		if logCtx.Err() != nil {
			logCtx = context.Background()
		}
		extraJSON := docProcJSONOrNil(map[string]any{
			"level": level,
			"seqno": seqNo,
			"lines": lineSpans,
		})
		var errStr *string
		if procErr != nil {
			msg := procErr.Error()
			errStr = &msg
		}
		activityName := "generate_summary"
		if err := s.ProcLogger.LogGenerateSummary(logCtx, DocProcLogRecord{
			CallReason:    "generate summary",
			DocProcName:   "generate_summary",
			ModelNames:    dedupeNonEmpty([]string{s.SummaryModelName}),
			PromptName:    s.SummaryPromptRef,
			RecordID:      &recordID,
			ProcProgress:  nullableStringPtr(procProgress),
			LLMCallID:     &callID,
			ActivityName:  &activityName,
			ExtraInfoJSON: extraJSON,
			Errors:        errStr,
			MSUsed:        int64Ptr(time.Since(startTime).Milliseconds()),
		}, "MID-26052850"); err != nil {
			s.Logger.Warn("failed to write generate_summary log", "record_id", recordID, "level", level, "seq", seqNo, "error", err)
		}
	}
	defer func() {
		if r := recover(); r != nil {
			logSummaryCall(fmt.Errorf("panic: %v", r))
			panic(r)
		}
	}()

	if s.GenerateSummary != nil {
		result, err := s.GenerateSummary(ctx, recordID, level, seqNo, lines, children)
		logSummaryCall(err)
		return result, err
	}
	if s.Extractor == nil {
		err := errors.New("(MID_26042904) summary extractor is nil")
		logSummaryCall(err)
		return summaryGenerateResult{}, err
	}
	if len(lines) == 0 && len(children) == 0 {
		err := fmt.Errorf("(MID_26053001) empty summary input for level %d seq %d", level, seqNo)
		logSummaryCall(err)
		return summaryGenerateResult{}, err
	}
	inputText := buildSummaryInputText(lines, children)

	s.Logger.Info("summary start",
		"record_id", recordID,
		"level", level,
		"seq", seqNo,
		"call_id", callID[:8])

	// Call the LLM
	in := llmclients.JSONExtractionInput{
		PromptText: appendLanguageInstruction(s.SummaryPromptText, inputText),
		ModelName:  s.SummaryModelName,
		InputText:  inputText,
	}
	var (
		parsed map[string]any
		err    error
	)
	if structuredExtractor, ok := s.Extractor.(LLMStructuredJSONExtractor); ok {
		var result *llmclients.StructuredOutputResult
		result, err = structuredExtractor.ExtractStructuredJSON(ctx, in, summaryExtractionContract())
		if result != nil {
			parsed = result.Parsed
		}
	} else {
		parsed, err = s.Extractor.ExtractJSON(ctx, in)
	}
	if err != nil {
		err = fmt.Errorf("(MID_26042905) generate summary for level %d seq %d: %w", level, seqNo, err)
		logSummaryCall(err)
		return summaryGenerateResult{}, err
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
		if reason != "missing-category" {
			s.Logger.Warn("(MID_26042906) invalid category path in summary response; using empty path",
				"level", level, "seq", seqNo, "reason", reason, "raw_path", rawPath)
		}
		path = nil
	}
	categoryPathItems := extractCategoryPathDetailFromLLM(parsed)
	categoryPathItemsEn := extractCategoryPathDetailEnFromLLM(parsed)
	if len(categoryPathItemsEn) == 0 && len(categoryPathItems) > 0 && !summaryCategoryPathsLookEnglish(categoryPathItems) {
		// s.Logger.Warn("summary category_paths_en missing for non-English categories; translating",
		// 	"record_id", recordID,
		// 	"level", level,
		// 	"seq", seqNo,
		// )
		var translateErr error
		ss := time.Now()
		categoryPathItemsEn, translateErr = s.translateSummaryCategoryPaths(ctx, categoryPathItems)
		if translateErr != nil {
			err := fmt.Errorf("(MID_26052702) translate summary category paths for level %d seq %d: %w", level, seqNo, translateErr)
			logSummaryCall(err)
			return summaryGenerateResult{}, err
		}
		s.Logger.Info("===== Missing english version; translate",
			"model_name", s.TranslationModelName,
			"ms_used", time.Since(ss).Milliseconds())
	}
	var nodes []CategoryPathNode
	if len(categoryPathItems) > 0 {
		nodes = categoryPathItems[0].Nodes
	}

	s.Logger.Info("summary end  ",
		"record_id", recordID,
		"level", level,
		"seq", seqNo,
		"call_id", callID[:8],
		"ms_used", time.Since(startTime).Milliseconds())

	result := summaryGenerateResult{
		Summary:             summary,
		SummaryEn:           summaryEn,
		Keywords:            keywords,
		KeywordsEn:          keywordsEn,
		CategoryPaths:       path,
		CategoryNodes:       nodes,
		CategoryPathItems:   categoryPathItems,
		CategoryPathItemsEn: categoryPathItemsEn,
	}
	logSummaryCall(nil)
	return result, nil
}

func summaryLogLineSpans(lines []MarkedLine, children []SummaryItem) []string {
	if len(lines) > 0 {
		return summaryInputLineRanges(lines)
	}
	var ranges []string
	for _, child := range children {
		ranges = append(ranges, child.Lines...)
	}
	return ranges
}

func summaryInputLineRanges(lines []MarkedLine) []string {
	nums := make([]int, 0, len(lines))
	for _, ml := range lines {
		if ml.Line.LineNo > 0 {
			nums = append(nums, ml.Line.LineNo)
		}
	}
	return lineRangesFromNumbers(nums)
}

func docProcJSONOrNil(v any) *string {
	bs, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	s := string(bs)
	return &s
}

const summaryTextTranslationPrompt = `You are a translation engine.

Translate the provided English summary text to the specified target language.

Rules:
- Translate accurately, preserving meaning, technical terms, and nuance.
- Do not translate proper nouns or domain-specific abbreviations unless a widely accepted translation exists.
- Return strict JSON only.

Output schema:
{
  "summary": "translated text"
}`

const summaryKeywordsTranslationPrompt = `You are a translation engine.

Translate the provided English keywords to the specified target language.

Rules:
- Preserve keyword meaning and keep each keyword concise.
- Preserve domain-specific terms and abbreviations unless a well-known translation exists.
- Return the same number of keywords in the same order.
- Return strict JSON only.

Output schema:
{
  "keywords": ["translated keyword"]
}`

const summaryCategoryTranslationPrompt = `You are a translation engine.

Translate the provided category paths into English.

Rules:
- Preserve the same number of category paths and the same hierarchy depth.
- Translate each category name to concise English noun phrases.
- Translate category keywords and path_keywords to accurate English.
- Preserve confidence values exactly.
- Return strict JSON only.

Output schema:
{
  "category_paths_en": [
    {
      "category_path": [
        {
          "name": "string",
          "keywords": ["string"],
          "confidence": 0.0
        }
      ],
      "path_keywords": ["string"],
      "path_confidence": 0.0
    }
  ]
}`

func (s *FixedSizeChunkingService) translateSummaryCategoryPaths(ctx context.Context, categoryPaths []CategoryPathEntry) ([]CategoryPathEntry, error) {
	if len(categoryPaths) == 0 {
		return nil, nil
	}
	if !s.TranslationEnabled || strings.TrimSpace(s.TranslationModelName) == "" {
		return nil, errors.New("(MID_26052703) TRANSLATION_MODEL_NAME is required when category_paths_en is missing for non-English summary categories")
	}
	if s.Extractor == nil {
		return nil, errors.New("(MID_26052704) summary extractor is nil")
	}
	inputPayload, err := json.Marshal(map[string]any{
		"category_paths": categoryPaths,
	})
	if err != nil {
		return nil, err
	}
	in := llmclients.JSONExtractionInput{
		PromptText: summaryCategoryTranslationPrompt,
		ModelName:  s.TranslationModelName,
		InputText:  string(inputPayload),
	}
	var parsed map[string]any
	if structuredExtractor, ok := s.Extractor.(LLMStructuredJSONExtractor); ok {
		result, extractErr := structuredExtractor.ExtractStructuredJSON(ctx, in, summaryCategoryTranslationContract())
		if extractErr != nil {
			return nil, extractErr
		}
		if result != nil {
			parsed = result.Parsed
		}
	} else {
		parsed, err = s.Extractor.ExtractJSON(ctx, in)
		if err != nil {
			return nil, err
		}
	}
	translated := extractCategoryPathDetailEnFromLLM(parsed)
	if len(translated) == 0 {
		if arr, ok := parsed["category_paths_en"].([]any); ok {
			translated = parseCategoryPathsArray(arr)
		}
	}
	if len(translated) == 0 {
		return nil, errors.New("(MID_26052705) translation returned empty category_paths_en")
	}
	return translated, nil
}

func summaryLanguageName(langCode string) string {
	switch langCode {
	case "zh":
		return "Chinese (Simplified)"
	case "en":
		return "English"
	default:
		return langCode
	}
}

func (s *FixedSizeChunkingService) translateSummaryText(ctx context.Context, targetLang, text string) (string, error) {
	if !s.TranslationEnabled || strings.TrimSpace(s.TranslationModelName) == "" {
		return "", fmt.Errorf("(MID_26052901) TRANSLATION_MODEL_NAME is required to translate summary to %q", targetLang)
	}
	if s.Extractor == nil {
		return "", errors.New("(MID_26052902) summary extractor is nil")
	}
	inputPayload, err := json.Marshal(map[string]any{
		"target_language": targetLang,
		"text":            text,
	})
	if err != nil {
		return "", err
	}
	in := llmclients.JSONExtractionInput{
		PromptText: summaryTextTranslationPrompt,
		ModelName:  s.TranslationModelName,
		InputText:  string(inputPayload),
	}
	var parsed map[string]any
	if structuredExtractor, ok := s.Extractor.(LLMStructuredJSONExtractor); ok {
		result, extractErr := structuredExtractor.ExtractStructuredJSON(ctx, in, summaryTextTranslationContract())
		if extractErr != nil {
			return "", extractErr
		}
		if result != nil {
			parsed = result.Parsed
		}
	} else {
		parsed, err = s.Extractor.ExtractJSON(ctx, in)
		if err != nil {
			return "", err
		}
	}
	translated := sanitizeTopicText(asString(parsed["summary"]))
	if translated == "" {
		return "", fmt.Errorf("(MID_26052903) translation to %q returned empty summary", targetLang)
	}
	return translated, nil
}

func (s *FixedSizeChunkingService) translateSummaryKeywords(ctx context.Context, targetLang string, keywords []string) ([]string, error) {
	if len(keywords) == 0 {
		return nil, nil
	}
	if !s.TranslationEnabled || strings.TrimSpace(s.TranslationModelName) == "" {
		return nil, fmt.Errorf("(MID_26053001) TRANSLATION_MODEL_NAME is required to translate summary keywords to %q", targetLang)
	}
	if s.Extractor == nil {
		return nil, errors.New("(MID_26053002) summary extractor is nil")
	}
	inputPayload, err := json.Marshal(map[string]any{
		"target_language": targetLang,
		"keywords":        keywords,
	})
	if err != nil {
		return nil, err
	}
	in := llmclients.JSONExtractionInput{
		PromptText: summaryKeywordsTranslationPrompt,
		ModelName:  s.TranslationModelName,
		InputText:  string(inputPayload),
	}
	var parsed map[string]any
	if structuredExtractor, ok := s.Extractor.(LLMStructuredJSONExtractor); ok {
		result, extractErr := structuredExtractor.ExtractStructuredJSON(ctx, in, summaryKeywordsTranslationContract())
		if extractErr != nil {
			return nil, extractErr
		}
		if result != nil {
			parsed = result.Parsed
		}
	} else {
		parsed, err = s.Extractor.ExtractJSON(ctx, in)
		if err != nil {
			return nil, err
		}
	}
	translated := compactTopicArray(parsed["keywords"])
	if len(translated) == 0 {
		return nil, fmt.Errorf("(MID_26053003) translation to %q returned empty keywords", targetLang)
	}
	return translated, nil
}

// fixSummarySourceLanguage translates Summary to the source language for any item
// where Summary and SummaryEn are identical (indicating the LLM returned the same
// English text for both), and also translates Keywords when Keywords and
// KeywordsEn are still identical English fallbacks. Translation failures are logged
// as warnings and the English text is kept as fallback — this method never returns
// an error.
func (s *FixedSizeChunkingService) fixSummarySourceLanguage(ctx context.Context, sourceLanguage string, summaries []SummaryItem) ([]SummaryItem, error) {
	normalizedLang := normalizeSummarySourceLanguage(sourceLanguage)
	if normalizedLang == "" || normalizedLang == "en" {
		return summaries, nil
	}
	langName := summaryLanguageName(normalizedLang)
	for i, item := range summaries {
		originalSummary := item.Summary
		originalSummaryEn := item.SummaryEn
		originalKeywords := append([]string(nil), item.Keywords...)
		changed := false
		summary := strings.TrimSpace(item.Summary)
		summaryEn := strings.TrimSpace(item.SummaryEn)
		if summaryEn == "" && summary != "" && detectContentLanguage(summary) == normalizedLang {
			translated, err := s.translateSummaryText(ctx, summaryLanguageName("en"), summary)
			if err != nil {
				s.Logger.Warn("(MID_26052906) failed to translate summary to English; keeping missing summary_en",
					"summary_id", item.SummaryID,
					"source_lang", normalizedLang,
					"error", err,
				)
			} else {
				summaryEn = translated
				summaries[i].SummaryEn = translated
				changed = true
				s.Logger.Info("translated summary to english",
					"summary_id", item.SummaryID,
					"source_lang", normalizedLang,
				)
			}
		}
		if summaryEn == "" && summary != "" && detectContentLanguage(summary) == "en" {
			summaryEn = summary
			summaries[i].SummaryEn = summary
			changed = true
		}
		if summaryEn != "" && summary == summaryEn {
			translated, err := s.translateSummaryText(ctx, langName, summaryEn)
			if err != nil {
				s.Logger.Warn("(MID_26052904) failed to translate summary to source language; keeping English",
					"summary_id", item.SummaryID,
					"target_lang", normalizedLang,
					"error", err,
				)
			} else {
				summaries[i].Summary = translated
				changed = true
				s.Logger.Info("translated summary to source language",
					"summary_id", item.SummaryID,
					"target_lang", normalizedLang,
				)
			}
		}
		if len(item.KeywordsEn) > 0 && equalTrimmedStringSlices(item.Keywords, item.KeywordsEn) {
			translatedKeywords, err := s.translateSummaryKeywords(ctx, langName, item.KeywordsEn)
			if err != nil {
				s.Logger.Warn("(MID_26053004) failed to translate summary keywords to source language; keeping English",
					"summary_id", item.SummaryID,
					"target_lang", normalizedLang,
					"error", err,
				)
			} else {
				summaries[i].Keywords = translatedKeywords
				changed = true
				s.Logger.Info("translated summary keywords to source language",
					"summary_id", item.SummaryID,
					"target_lang", normalizedLang,
				)
			}
		}
		if !changed {
			continue
		}
		if _, err := writeSummaryFile(s.ChunkDir, item.RecordID, summaries[i]); err != nil {
			s.Logger.Warn("(MID_26052905) failed to re-write summary file after translation; keeping original",
				"summary_id", item.SummaryID,
				"error", err,
			)
			summaries[i].Summary = originalSummary
			summaries[i].SummaryEn = originalSummaryEn
			summaries[i].Keywords = originalKeywords
		}
	}
	return summaries, nil
}

func summaryCategoryPathsLookEnglish(entries []CategoryPathEntry) bool {
	if len(entries) == 0 {
		return true
	}
	for _, entry := range entries {
		for _, node := range entry.Nodes {
			if detectContentLanguage(node.Name) != "en" {
				return false
			}
			for _, kw := range node.Keywords {
				if detectContentLanguage(kw) != "en" {
					return false
				}
			}
		}
		for _, kw := range entry.PathKeywords {
			if detectContentLanguage(kw) != "en" {
				return false
			}
		}
	}
	return true
}

func buildSummaryInputText(lines []MarkedLine, children []SummaryItem) string {
	if len(lines) > 0 {
		return buildMarkedChunkInputText(lines)
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

// updateInputStatusLocked re-reads the record's status under the per-record
// status lock, applies build to the freshest status JSON, and persists it via
// Store.UpdateInputStatus. Phase B summary/topic writes run concurrently with
// sibling processors, so they must merge onto the current DB value rather than a
// stale local copy of rec.StatusRaw.
func (s *FixedSizeChunkingService) updateInputStatusLocked(
	ctx context.Context,
	recordID int64,
	errMsg *string,
	build func(current string) (string, error),
) error {
	return withRecordStatusLock(recordID, func() error {
		fresh, err := s.Store.GetInputRecord(ctx, recordID)
		if err != nil {
			return err
		}
		statusRaw, err := build(fresh.StatusRaw)
		if err != nil {
			return err
		}
		return s.Store.UpdateInputStatus(ctx, recordID, statusRaw, errMsg)
	})
}

func (s *FixedSizeChunkingService) failAndPersistTopics(
	ctx context.Context,
	rec InputRecord,
	inputFilename string,
	start time.Time,
	numTopics int,
	procErr error,
) {
	errMsg := sanitizeUTF8Text(procErr.Error())
	if updateErr := s.updateInputStatusLocked(ctx, rec.ID, &errMsg, func(current string) (string, error) {
		return appendTopicStatus(current, topicStatusParams{
			RecordID:      rec.ID,
			FileType:      detectChunkStatusFileType(rec, inputFilename),
			InputFilename: inputFilename,
			NumTopics:     numTopics,
			Start:         start,
			DurationMs:    time.Since(start).Milliseconds(),
			ProcErr:       procErr,
		})
	}); updateErr != nil {
		s.Logger.Error("failed persisting topic failure status", "record_id", rec.ID, "error", updateErr)
		return
	}
	s.Logger.Error("topic generation failed", "record_id", rec.ID, "error", procErr)
}

// stopAndPersistTopics writes a "stopped" status entry to kb.inputs.status and
// logs a finish record to kb.doc_proc_logs. ctx must be a live context (use
// context.Background() when the pipeline context is already cancelled).
func (s *FixedSizeChunkingService) stopAndPersistTopics(
	ctx context.Context,
	rec InputRecord,
	inputFilename string,
	start time.Time,
	numTopics int,
) {
	if updateErr := s.updateInputStatusLocked(ctx, rec.ID, nil, func(current string) (string, error) {
		return appendTopicStatus(current, topicStatusParams{
			RecordID:      rec.ID,
			FileType:      detectChunkStatusFileType(rec, inputFilename),
			InputFilename: inputFilename,
			NumTopics:     numTopics,
			Start:         start,
			DurationMs:    time.Since(start).Milliseconds(),
			ProcStatus:    "stopped",
		})
	}); updateErr != nil {
		s.Logger.Error("(MID_26052822) failed persisting topic stopped status", "record_id", rec.ID, "error", updateErr)
	}
	s.Logger.Info("(MID_26052823) topic generation stopped by user request", "record_id", rec.ID, "topics_so_far", numTopics)
}

func (s *FixedSizeChunkingService) failAndPersistSummaries(
	ctx context.Context,
	rec InputRecord,
	inputFilename string,
	start time.Time,
	procErr error,
) {
	errMsg := sanitizeUTF8Text(procErr.Error())
	if updateErr := s.updateInputStatusLocked(ctx, rec.ID, &errMsg, func(current string) (string, error) {
		return appendSummariesStatus(current, summaryStatusParams{
			RecordID:      rec.ID,
			FileType:      detectChunkStatusFileType(rec, inputFilename),
			InputFilename: inputFilename,
			Start:         start,
			DurationMs:    time.Since(start).Milliseconds(),
			ProcStatus:    "failed",
			ProcProgress:  s.currentSummaryProgress(),
			ProcErr:       procErr,
		})
	}); updateErr != nil {
		s.Logger.Error("failed persisting summary failure status", "record_id", rec.ID, "error", updateErr)
		return
	}
	s.Logger.Error("summary generation failed", "record_id", rec.ID, "error", procErr)
}

func (s *FixedSizeChunkingService) stopAndPersistSummaries(ctx context.Context, rec InputRecord, inputFilename string, start time.Time) {
	if updateErr := s.updateInputStatusLocked(ctx, rec.ID, nil, func(current string) (string, error) {
		return appendSummariesStatus(current, summaryStatusParams{
			RecordID:      rec.ID,
			FileType:      detectChunkStatusFileType(rec, inputFilename),
			InputFilename: inputFilename,
			Start:         start,
			DurationMs:    time.Since(start).Milliseconds(),
			ProcStatus:    "stopped",
		})
	}); updateErr != nil {
		s.Logger.Error("(MID_26052832) failed persisting summaries stopped status", "record_id", rec.ID, "error", updateErr)
	}
	s.Logger.Info("(MID_26052833) summary generation stopped by user request", "record_id", rec.ID)
}

func (s *FixedSizeChunkingService) currentSummaryProgress() string {
	if s.summaryProgressTracker == nil {
		return ""
	}
	return s.summaryProgressTracker.current()
}

func totalPlannedSummaries(leafCount int, groupSize int) int {
	if leafCount <= 0 {
		return 0
	}
	if groupSize <= 1 {
		groupSize = DefaultSummaryGroupSize
	}
	total := 0
	levelCount := leafCount
	for {
		total += levelCount
		if levelCount <= 1 {
			return total
		}
		levelCount = (levelCount + groupSize - 1) / groupSize
	}
}

func formatSummaryProgress(completed int, total int) string {
	if total <= 0 {
		return ""
	}
	if completed < 0 {
		completed = 0
	}
	if completed > total {
		completed = total
	}
	percent := (completed * 100) / total
	return fmt.Sprintf("%d%% (%d/%d)", percent, completed, total)
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

type parseInputLinesOptions struct {
	includeTOC bool
}

func ParseInputLines(input []byte) ([]Line, error) {
	return parseInputLines(input, parseInputLinesOptions{})
}

func ParseInputLinesIncludingTOC(input []byte) ([]Line, error) {
	return parseInputLines(input, parseInputLinesOptions{includeTOC: true})
}

func parseInputLines(input []byte, opts parseInputLinesOptions) ([]Line, error) {
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
		if !opts.includeTOC {
			if fields := strings.Split(raw, "\t"); len(fields) >= 3 && strings.EqualFold(strings.TrimSpace(fields[2]), "TOC") {
				continue
			}
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
	lines = filterTOCLines(lines)
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
		end = extendChunkEndToMinimumRegularBytes(lines, start, end, prevEnd, minRegularBytes(opts.ChunkSize), blocks)

		c := Chunk{SeqNo: seq, Lines: make([]MarkedLine, 0, end-start)}
		for i := start; i < end; i++ {
			mark := "n"
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
		nextStart = trimOverlapStartToMaxBytes(lines, nextStart, end, maxOverlapBytes(opts.ChunkSize))
		start = nextStart
		seq++
	}

	if err := validateChunksNonEmpty(chunks, "built chunks"); err != nil {
		return nil, err
	}
	if err := validateChunkSizeSanity(chunks, opts.ChunkSize); err != nil {
		return nil, err
	}
	return chunks, nil
}

func extendChunkEndToMinimumRegularBytes(lines []Line, start int, end int, prevEnd int, minBytes int, blocks []*protectedBlock) int {
	if minBytes <= 0 || end >= len(lines) {
		return end
	}
	for end < len(lines) && regularBytesInRange(lines, start, end, prevEnd) < minBytes {
		next := adjustChunkEnd(start, end+1, len(lines), blocks)
		if next <= end {
			next = end + 1
		}
		end = min(next, len(lines))
	}
	return end
}

func regularBytesInRange(lines []Line, start int, end int, prevEnd int) int {
	size := 0
	for i := max(start, prevEnd); i < end && i < len(lines); i++ {
		size += lineRawByteSize(lines[i])
	}
	return size
}

func trimOverlapStartToMaxBytes(lines []Line, start int, end int, maxBytes int) int {
	if start >= end {
		return start
	}
	for end-start > 1 && lineRangeByteSize(lines, start, end) > maxBytes {
		start++
	}
	return start
}

func lineRangeByteSize(lines []Line, start int, end int) int {
	size := 0
	for i := max(0, start); i < end && i < len(lines); i++ {
		size += lineRawByteSize(lines[i])
	}
	return size
}

func maxOverlapBytes(minBytes int) int {
	if minBytes <= 0 {
		return 0
	}
	return minBytes * 20 / 100
}

func minRegularBytes(minBytes int) int {
	if minBytes <= 0 {
		return 0
	}
	return (minBytes*80 + 99) / 100
}

func validateChunkSizeSanity(chunks []Chunk, minBytes int) error {
	if minBytes <= 0 {
		return nil
	}
	minRegular := minRegularBytes(minBytes)
	maxOverlap := maxOverlapBytes(minBytes)
	for i := 0; i < len(chunks); i++ {
		chunk := chunks[i]
		regularBytes := 0
		overlapBytes := 0
		overlapCount := 0
		for _, ml := range chunk.Lines {
			if strings.EqualFold(strings.TrimSpace(ml.Mark), "o") {
				overlapBytes += lineRawByteSize(ml.Line)
				overlapCount++
				continue
			}
			regularBytes += lineRawByteSize(ml.Line)
		}
		overlapLines, regularLines := chunkLineNumbers(chunk)
		if overlapCount > 1 && overlapBytes > maxOverlap {
			return fmt.Errorf(
				"(MID_26060202) chunk sanity check failed: chunk %d overlap payload is too large: overlap_bytes=%d max_overlap_bytes=%d overlap=%s lines=%s",
				chunk.SeqNo,
				overlapBytes,
				maxOverlap,
				formatLineNumberRanges(overlapLines),
				formatLineNumberRanges(regularLines),
			)
		}
		if i == len(chunks)-1 {
			continue
		}
		if regularBytes >= minRegular {
			continue
		}
		return fmt.Errorf(
			"(MID_26060201) chunk sanity check failed: non-final chunk %d regular payload is too small: regular_bytes=%d min_regular_bytes=%d overlap=%s lines=%s",
			chunk.SeqNo,
			regularBytes,
			minRegular,
			formatLineNumberRanges(overlapLines),
			formatLineNumberRanges(regularLines),
		)
	}
	return nil
}

func filterTOCLines(lines []Line) []Line {
	if len(lines) == 0 {
		return lines
	}
	filtered := lines[:0]
	for _, line := range lines {
		if strings.EqualFold(strings.TrimSpace(line.LineType), "TOC") {
			continue
		}
		filtered = append(filtered, line)
	}
	return filtered
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

	return fmt.Sprintf("%d\t%d\t%s\t%s", line.LineNo, line.PageNo, line.LineType, line.Content)
}

func normalizeChunkFlag(mark string) string {
	if strings.EqualFold(strings.TrimSpace(mark), "o") {
		return "o"
	}
	return "n"
}

/*
func formatMarkedChunkLine(line Line, mark string) string {
	base := strings.TrimSpace(lineRawForChunking(line))
	if base == "" {
		return ""
	}
	return normalizeChunkFlag(mark) + "\t" + base
}
*/

func buildMarkedChunkInputText(lines []MarkedLine) string {
	return markedLinesToJSON(lines)
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

type topicStatusParams struct {
	RecordID       int64
	FileType       string
	InputFilename  string
	OutputFilename string
	NumTopics      int
	Start          time.Time
	DurationMs     int64
	ProcErr        error
	Progress       string
	// ProcStatus overrides the auto-derived proc_status ("success"/"failed") when non-empty.
	// Use "stopped" to record a user-requested stop.
	ProcStatus string
}

type summaryStatusParams struct {
	RecordID      int64
	FileType      string
	InputFilename string
	Start         time.Time
	DurationMs    int64
	ProcStatus    string
	ProcProgress  string
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

func appendTopicStatus(raw string, p topicStatusParams) (string, error) {
	entries := decodeStatus(raw)
	entry := map[string]any{
		"record_id":      strconv.FormatInt(p.RecordID, 10),
		"file_type":      sanitizeUTF8Text(strings.ToLower(strings.TrimSpace(p.FileType))),
		"operation":      "generate_topics",
		"input_filename": sanitizeUTF8Text(p.InputFilename),
		"num_topics":     p.NumTopics,
		"start_time":     p.Start.Format(defaultStatusTime),
		"ms_used":        p.DurationMs,
	}
	if strings.TrimSpace(p.OutputFilename) != "" {
		entry["output_filename"] = sanitizeUTF8Text(p.OutputFilename)
	}
	if strings.TrimSpace(p.Progress) != "" {
		entry["progress"] = sanitizeUTF8Text(strings.TrimSpace(p.Progress))
	}
	if override := strings.TrimSpace(p.ProcStatus); override != "" {
		entry["proc_status"] = override
		if p.ProcErr != nil {
			entry["error"] = sanitizeUTF8Text(p.ProcErr.Error())
		}
	} else if p.ProcErr == nil {
		entry["proc_status"] = "success"
	} else {
		entry["proc_status"] = "failed"
		entry["error"] = sanitizeUTF8Text(p.ProcErr.Error())
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
	if strings.TrimSpace(p.ProcProgress) != "" {
		entry["progress"] = sanitizeUTF8Text(strings.TrimSpace(p.ProcProgress))
	}
	if strings.TrimSpace(p.ProcStatus) != "" {
		entry["proc_status"] = sanitizeUTF8Text(strings.TrimSpace(p.ProcStatus))
		if p.ProcErr != nil {
			entry["error"] = sanitizeUTF8Text(p.ProcErr.Error())
		}
	} else if p.ProcErr == nil {
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

func buildFixedSizeArtifactBase(rec InputRecord, inputFilename string) string {
	stagingName := strings.TrimSpace(rec.StagingFilename)
	if stagingName == "" {
		stagingName = inputFilename
	}
	return buildChunkArtifactBaseName(stagingName, rec.ParserName)
}

func loadChunksFromArtifactFile(chunkDir string, recordID int64, fileName string, lines []Line) ([]Chunk, error) {
	targetDir, err := buildRecordArtifactDir(chunkDir, recordID)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(targetDir, fileName)
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("(MID_26052331) chunk artifact not found: %s", path)
		}
		return nil, err
	}

	lineByNo := make(map[int]Line, len(lines))
	maxLineNo := 0
	for _, line := range lines {
		lineByNo[line.LineNo] = line
		if line.LineNo > maxLineNo {
			maxLineNo = line.LineNo
		}
	}

	scanner := bufio.NewScanner(strings.NewReader(string(body)))
	scanner.Buffer(make([]byte, 1024), 16*1024*1024)
	chunks := make([]Chunk, 0, 8)
	var overlap, regular []int
	seqNo := 1
	appendChunk := func() error {
		if overlap == nil && regular == nil {
			return nil
		}
		chunkLines := make([]MarkedLine, 0, len(overlap)+len(regular))
		missingOverlap := make([]int, 0, len(overlap))
		missingRegular := make([]int, 0, len(regular))
		for _, lineNo := range overlap {
			line, ok := lineByNo[lineNo]
			if !ok {
				// Skip lines absent from the filtered input (e.g. TOC lines recorded
				// in artifacts created before TOC filtering was enforced in BuildChunks).
				missingOverlap = append(missingOverlap, lineNo)
				continue
			}
			chunkLines = append(chunkLines, MarkedLine{Line: line, Mark: "o"})
		}
		for _, lineNo := range regular {
			line, ok := lineByNo[lineNo]
			if !ok {
				// Skip lines absent from the filtered input (e.g. TOC lines recorded
				// in artifacts created before TOC filtering was enforced in BuildChunks).
				missingRegular = append(missingRegular, lineNo)
				continue
			}
			chunkLines = append(chunkLines, MarkedLine{Line: line, Mark: "n"})
		}
		if len(chunkLines) == 0 {
			return fmt.Errorf(
				"(MID_26053002) chunk artifact %s for record %d seq %d references only missing/filtered lines: overlap=%s regular=%s missing_overlap=%s missing_regular=%s",
				fileName,
				recordID,
				seqNo,
				formatLineNumberRanges(overlap),
				formatLineNumberRanges(regular),
				formatLineNumberRanges(missingOverlap),
				formatLineNumberRanges(missingRegular),
			)
		}
		chunks = append(chunks, Chunk{SeqNo: seqNo, Lines: chunkLines})
		seqNo++
		overlap = nil
		regular = nil
		return nil
	}

	for scanner.Scan() {
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" {
			if err := appendChunk(); err != nil {
				return nil, err
			}
			continue
		}
		switch {
		case strings.HasPrefix(raw, "overlap:"):
			values, err := parseLineNumberRanges(strings.TrimSpace(strings.TrimPrefix(raw, "overlap:")))
			if err != nil {
				return nil, fmt.Errorf("(MID_26052334) parse overlap ranges: %w", err)
			}
			overlap = values
		case strings.HasPrefix(raw, "lines:"):
			values, err := parseLineNumberRanges(strings.TrimSpace(strings.TrimPrefix(raw, "lines:")))
			if err != nil {
				return nil, fmt.Errorf("(MID_26052335) parse line ranges: %w", err)
			}
			regular = values
		default:
			return nil, fmt.Errorf("(MID_26052336) invalid chunk artifact line: %s", raw)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if err := appendChunk(); err != nil {
		return nil, err
	}
	if err := validateChunksNonEmpty(chunks, "loaded chunks"); err != nil {
		return nil, err
	}
	return chunks, nil
}

func validateChunksNonEmpty(chunks []Chunk, source string) error {
	for _, chunk := range chunks {
		if len(chunk.Lines) == 0 {
			return fmt.Errorf("(MID_26053003) %s contain empty chunk seq %d", source, chunk.SeqNo)
		}
	}
	return nil
}

func parseLineNumberRanges(raw string) ([]int, error) {
	text := strings.TrimSpace(raw)
	if text == "" || text == "[]" {
		return nil, nil
	}
	if !strings.HasPrefix(text, "[") || !strings.HasSuffix(text, "]") {
		return nil, fmt.Errorf("invalid range list: %q", raw)
	}
	text = strings.TrimSpace(text[1 : len(text)-1])
	if text == "" {
		return nil, nil
	}

	parts := strings.Split(text, ",")
	out := make([]int, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" {
			continue
		}
		if strings.Contains(item, "-") {
			bounds := strings.SplitN(item, "-", 2)
			if len(bounds) != 2 {
				return nil, fmt.Errorf("invalid range: %q", item)
			}
			start, err := strconv.Atoi(strings.TrimSpace(bounds[0]))
			if err != nil {
				return nil, err
			}
			end, err := strconv.Atoi(strings.TrimSpace(bounds[1]))
			if err != nil {
				return nil, err
			}
			if end < start {
				return nil, fmt.Errorf("invalid descending range: %q", item)
			}
			for n := start; n <= end; n++ {
				out = append(out, n)
			}
			continue
		}
		n, err := strconv.Atoi(item)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, nil
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
