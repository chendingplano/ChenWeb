package docreviews

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/loggerutil"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
)

// ReviewFinding is a single issue or observation from a document review pass.
// Mirrors kb.doc_review_findings columns.
type ReviewFinding struct {
	Pass        string  `json:"pass"`
	Aspect      string  `json:"aspect"`
	Severity    string  `json:"severity"`
	FindingType string  `json:"finding_type"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Evidence    string  `json:"evidence,omitempty"`
	Location    string  `json:"location,omitempty"`
	Suggestion  string  `json:"suggestion,omitempty"`
	Confidence  float64 `json:"confidence"`
}

// ReviewStrategy selects how a reviewer processes a document.
type ReviewStrategy int

const (
	StrategyChunk    ReviewStrategy = iota // per-chunk, concurrent
	StrategyDocument                       // single document-level call
)

// DefaultInputBlockSize is the default number of pages per block when a
// StrategyDocument reviewer needs to break up a document that is too large
// for a single LLM call. Configurable via InputBlockSize in ReviewerConfig.
const DefaultInputBlockSize = 20

// pageBlock holds one page-aligned segment of lines for document-level
// reviewers that split large documents into blocks.
type pageBlock struct {
	inputJSON string // lines JSON wrapped with doc_context
	pageStart int    // first page in this block
	pageEnd   int    // last page in this block
	lineStart int    // first line number in this block
	lineEnd   int    // last line number in this block
}

// buildPageBlocks splits lines into page-aligned blocks of up to blockSize
// pages each. Each block is wrapped in the doc_context envelope.
// Used by StrategyDocument reviewers when the document is too large for a
// single LLM call (ADR DR2).
func buildPageBlocks(lines []Line, docCtx string, blockSize int) []pageBlock {
	if len(lines) == 0 || blockSize <= 0 {
		return nil
	}
	if blockSize == 0 {
		blockSize = DefaultInputBlockSize
	}

	// Group lines by page number while preserving order.
	var blocks []pageBlock
	i := 0
	for i < len(lines) {
		startPage := lines[i].PageNo
		endPage := startPage + blockSize - 1

		j := i
		for j < len(lines) && lines[j].PageNo <= endPage {
			j++
		}

		slice := lines[i:j]
		objs := rawLinesToJSON(slice)
		jsonText := wrapLinesWithDocContext(objs, docCtx)

		blocks = append(blocks, pageBlock{
			inputJSON: jsonText,
			pageStart: startPage,
			pageEnd:   lines[j-1].PageNo,
			lineStart: slice[0].LineNo,
			lineEnd:   slice[len(slice)-1].LineNo,
		})

		i = j
	}
	return blocks
}

// ReviewerConfig configures one reviewer for a review run.
type ReviewerConfig struct {
	Enabled      bool     // whether this reviewer runs
	ModelName    string   // resolved model name
	PromptText   string   // prompt body
	PromptRef    string   // prompt file name (for logging)
	MaxToolTurns int      // 0 = one-shot, >0 = tool-use conversation loop
	Tools        []string // tool names available (only when MaxToolTurns > 0)
	OnProgress   ReviewerProgressFunc
}

// ReviewerProgress is one live progress snapshot for a reviewer while it works
// through chunk or page-block units.
type ReviewerProgress struct {
	CompletedUnits int
	TotalUnits     int
	FindingCount   int
	Progress       float64
}

// ReviewerProgressFunc receives progress snapshots after each finished unit.
type ReviewerProgressFunc func(ReviewerProgress)

// Reviewer executes one review aspect against a document.
type Reviewer interface {
	// Name returns the aspect name, e.g. "grammar_spelling".
	Name() string
	// Group returns the group key, e.g. "P1".
	Group() string
	// Strategy returns how the reviewer processes the document.
	Strategy() ReviewStrategy
	// ReviewDocument runs the review and returns findings.
	ReviewDocument(ctx context.Context, recordID int64, cfg ReviewerConfig) ([]ReviewFinding, error)
}

// ReviewFindingsStore persists review findings.
type ReviewFindingsStore interface {
	SaveFindings(ctx context.Context, recordID int64, reviewRunID string, findings []ReviewFinding) (int64, error)
	DeleteFindings(ctx context.Context, recordID int64) (int64, error)
}

// ReviewFindingsSQLStore implements ReviewFindingsStore.
type ReviewFindingsSQLStore struct {
	DB *sql.DB
}

func (s ReviewFindingsSQLStore) SaveFindings(ctx context.Context, recordID int64, reviewRunID string, findings []ReviewFinding) (int64, error) {
	if s.DB == nil {
		return 0, fmt.Errorf("db is nil")
	}
	if len(findings) == 0 {
		return 0, nil
	}

	const stmt = `
INSERT INTO kb.doc_review_findings
    (input_record_id, review_run_id, pass, aspect, severity, finding_type,
     title, description, evidence, location, suggestion, confidence)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`

	var inserted int64
	for _, f := range findings {
		var evidence, location, suggestion any
		if f.Evidence != "" {
			evidence = f.Evidence
		}
		if f.Location != "" {
			location = f.Location
		}
		if f.Suggestion != "" {
			suggestion = f.Suggestion
		}
		confidence := f.Confidence
		if confidence <= 0 {
			confidence = 0.5
		}

		_, err := s.DB.ExecContext(ctx, stmt,
			recordID, reviewRunID, f.Pass, f.Aspect, f.Severity, f.FindingType,
			f.Title, f.Description, evidence, location, suggestion, confidence,
		)
		if err != nil {
			return inserted, fmt.Errorf("insert review finding: %w", err)
		}
		inserted++
	}
	return inserted, nil
}

func (s ReviewFindingsSQLStore) DeleteFindings(ctx context.Context, recordID int64) (int64, error) {
	if s.DB == nil {
		return 0, fmt.Errorf("db is nil")
	}
	res, err := s.DB.ExecContext(ctx, `DELETE FROM kb.doc_review_findings WHERE input_record_id = $1`, recordID)
	if err != nil {
		return 0, fmt.Errorf("delete review findings for record %d: %w", recordID, err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// ReviewProcessor orchestrates document review in Phase C (post-processing).
// It owns a set of Reviewers; each runs as a goroutine and reports findings.
type ReviewProcessor struct {
	InputStore    DocMetadataStore
	EntityStore   EntityRelationStore // for tool-using reviewers (Phase II+)
	FindingsStore ReviewFindingsStore
	StatusStore   ReviewStatusStore
	Client        LLMJSONExtractor // shared LLM client
	Logger        ApiTypes.JimoLogger
	Now           func() time.Time

	// Reviewer configurations (loaded in constructor).
	GrammarModelName  string
	GrammarPromptRef  string
	GrammarPromptText string

	ToneVoiceModelName  string
	ToneVoicePromptRef  string
	ToneVoicePromptText string

	FormattingModelName  string
	FormattingPromptRef  string
	FormattingPromptText string

	ReadabilityModelName  string
	ReadabilityPromptRef  string
	ReadabilityPromptText string

	LocalizationModelName  string
	LocalizationPromptRef  string
	LocalizationPromptText string

	LogicalFlowModelName  string
	LogicalFlowPromptRef  string
	LogicalFlowPromptText string

	HeadingHierarchyModelName  string
	HeadingHierarchyPromptRef  string
	HeadingHierarchyPromptText string

	NavigabilityModelName  string
	NavigabilityPromptRef  string
	NavigabilityPromptText string

	SectionBalanceModelName  string
	SectionBalancePromptRef  string
	SectionBalancePromptText string

	ModularityModelName  string
	ModularityPromptRef  string
	ModularityPromptText string

	CompletenessModelName  string
	CompletenessPromptRef  string
	CompletenessPromptText string

	CorrectnessModelName  string
	CorrectnessPromptRef  string
	CorrectnessPromptText string

	ClarityModelName  string
	ClarityPromptRef  string
	ClarityPromptText string

	ConcisenessModelName  string
	ConcisenessPromptRef  string
	ConcisenessPromptText string

	RelevanceModelName  string
	RelevancePromptRef  string
	RelevancePromptText string

	CurrencyModelName  string
	CurrencyPromptRef  string
	CurrencyPromptText string

	MaxConcurrent int // max concurrent chunk workers per chunk-based reviewer

	// ReviewRunID, when set, overrides the self-generated run id so findings are
	// persisted under a caller-supplied run identity (DR15: the DocReviewController
	// assigns the run id at request-accept time and passes it in here).
	ReviewRunID string

	// GrammarClient is a properly-configured LLM client for the grammar reviewer.
	GrammarClient LLMJSONExtractor

	// ToneVoiceClient is a properly-configured LLM client for the tone_voice reviewer.
	ToneVoiceClient LLMJSONExtractor

	// FormattingClient is a properly-configured LLM client for the
	// formatting_consistency reviewer.
	FormattingClient LLMJSONExtractor

	// ReadabilityClient is a properly-configured LLM client for the
	// readability reviewer.
	ReadabilityClient LLMJSONExtractor

	// LocalizationClient is a properly-configured LLM client for the
	// localization reviewer.
	LocalizationClient LLMJSONExtractor

	// LogicalFlowClient is a properly-configured LLM client for the
	// logical_flow reviewer (P2, document-level).
	LogicalFlowClient LLMJSONExtractor

	// HeadingHierarchyClient is a properly-configured LLM client for the
	// heading_hierarchy reviewer (P2, document-level).
	HeadingHierarchyClient LLMJSONExtractor

	// NavigabilityClient is a properly-configured LLM client for the
	// navigability reviewer (P2, document-level).
	NavigabilityClient LLMJSONExtractor

	// SectionBalanceClient is a properly-configured LLM client for the
	// section_balance reviewer (P2, document-level).
	SectionBalanceClient LLMJSONExtractor

	// ModularityClient is a properly-configured LLM client for the
	// modularity reviewer (P2, document-level).
	ModularityClient LLMJSONExtractor

	// CompletenessClient is a properly-configured LLM client for the
	// completeness reviewer (P3, content quality, per-chunk).
	CompletenessClient LLMJSONExtractor

	// CorrectnessClient is a properly-configured LLM client for the
	// correctness reviewer (P3, content quality, per-chunk).
	CorrectnessClient LLMJSONExtractor

	// ClarityClient is a properly-configured LLM client for the
	// clarity reviewer (P3, content quality, per-chunk).
	ClarityClient LLMJSONExtractor

	// ConcisenessClient is a properly-configured LLM client for the
	// conciseness reviewer (P3, content quality, per-chunk).
	ConcisenessClient LLMJSONExtractor

	// RelevanceClient is a properly-configured LLM client for the
	// relevance reviewer (P3, content quality, per-chunk).
	RelevanceClient LLMJSONExtractor

	// CurrencyClient is a properly-configured LLM client for the
	// currency reviewer (P3, content quality, per-chunk).
	CurrencyClient LLMJSONExtractor
}

// resolveReviewerRuntime resolves one P1 reviewer's prompt + model + client from
// doc-review.local.toml (DR3 per-aspect config). The config file is the sole
// source of truth; an absent file, missing aspect block, explicitly-disabled
// aspect, or missing prompt/model leaves the reviewer disabled (ok=false).
func resolveReviewerRuntime(logger ApiTypes.JimoLogger, aspect, group string) (
	client LLMJSONExtractor, modelName, promptText, promptRef string, ok bool,
) {
	cfg, err := GetDocReviewConfig()
	switch {
	case err != nil:
		logger.Warn("doc-review config load failed; reviewer disabled", "aspect", aspect, "error", err)
		return nil, "", "", "", false
	case cfg == nil:
		logger.Info("doc-review config file not found; reviewer disabled", "aspect", aspect)
		return nil, "", "", "", false
	}

	resolved := cfg.ResolveReviewer(aspect, group)
	switch {
	case !resolved.Found:
		logger.Info("reviewer not configured in doc-review config; disabled", "aspect", aspect)
		return nil, "", "", "", false
	case !resolved.Enabled:
		logger.Info("reviewer disabled by doc-review config", "aspect", aspect)
		return nil, "", "", "", false
	}

	if resolved.PromptRef == "" {
		logger.Warn("reviewer prompt not set in doc-review config; disabled", "aspect", aspect)
		return nil, "", "", "", false
	}
	promptText, promptRef, _, err = loadPromptByRef(resolved.PromptRef)
	if err != nil {
		logger.Warn("reviewer prompt unavailable; disabled", "aspect", aspect, "error", err)
		return nil, "", "", "", false
	}

	if resolved.ModelRef == "" {
		logger.Warn("reviewer model not set in doc-review config; disabled", "aspect", aspect)
		return nil, "", "", "", false
	}
	client, modelName, err = docprocessing.BuildReviewerLLMClient(resolved.ModelRef)
	if err != nil {
		logger.Warn("reviewer model/client unavailable; disabled", "aspect", aspect, "error", err)
		return nil, "", "", "", false
	}

	logger.Info("reviewer configured from doc-review config",
		"aspect", aspect, "model_ref", resolved.ModelRef, "prompt_ref", resolved.PromptRef,
		"enabled", resolved.Enabled, "max_tool_turns", resolved.MaxToolTurns)
	return client, modelName, promptText, promptRef, true
}

// NewReviewProcessor creates a ReviewProcessor.
// Phase I loads the P1 reviewer configs (grammar_spelling, tone_voice,
// formatting_consistency, readability, localization) and the P2
// document-level reviewers (logical_flow, heading_hierarchy, navigability,
// section_balance, modularity).
func NewReviewProcessor(
	inputStore DocMetadataStore,
	entityStore EntityRelationStore,
	findingsStore ReviewFindingsStore,
	extractor LLMJSONExtractor,
	_ ApiTypes.JimoLogger,
) *ReviewProcessor {
	logger := loggerutil.CreateDefaultLogger("MID_26061801")

	grammarClient, grammarModel, grammarPrompt, grammarRef, _ := resolveReviewerRuntime(logger, "grammar_spelling", "P1")
	toneClient, toneModel, tonePrompt, toneRef, _ := resolveReviewerRuntime(logger, "tone_voice", "P1")
	formattingClient, formattingModel, formattingPrompt, formattingRef, _ := resolveReviewerRuntime(logger, "formatting_consistency", "P1")
	readabilityClient, readabilityModel, readabilityPrompt, readabilityRef, _ := resolveReviewerRuntime(logger, "readability", "P1")
	localizationClient, localizationModel, localizationPrompt, localizationRef, _ := resolveReviewerRuntime(logger, "localization", "P1")
	logicalFlowClient, logicalFlowModel, logicalFlowPrompt, logicalFlowRef, _ := resolveReviewerRuntime(logger, "logical_flow", "P2")
	headingHierarchyClient, headingHierarchyModel, headingHierarchyPrompt, headingHierarchyRef, _ := resolveReviewerRuntime(logger, "heading_hierarchy", "P2")
	navigabilityClient, navigabilityModel, navigabilityPrompt, navigabilityRef, _ := resolveReviewerRuntime(logger, "navigability", "P2")
	sectionBalanceClient, sectionBalanceModel, sectionBalancePrompt, sectionBalanceRef, _ := resolveReviewerRuntime(logger, "section_balance", "P2")
	modularityClient, modularityModel, modularityPrompt, modularityRef, _ := resolveReviewerRuntime(logger, "modularity", "P2")
	completenessClient, completenessModel, completenessPrompt, completenessRef, _ := resolveReviewerRuntime(logger, "completeness", "P3")
	correctnessClient, correctnessModel, correctnessPrompt, correctnessRef, _ := resolveReviewerRuntime(logger, "correctness", "P3")
	clarityClient, clarityModel, clarityPrompt, clarityRef, _ := resolveReviewerRuntime(logger, "clarity", "P3")
	concisenessClient, concisenessModel, concisenessPrompt, concisenessRef, _ := resolveReviewerRuntime(logger, "conciseness", "P3")
	relevanceClient, relevanceModel, relevancePrompt, relevanceRef, _ := resolveReviewerRuntime(logger, "relevance", "P3")
	currencyClient, currencyModel, currencyPrompt, currencyRef, _ := resolveReviewerRuntime(logger, "currency", "P3")

	return &ReviewProcessor{
		InputStore:    inputStore,
		EntityStore:   entityStore,
		FindingsStore: findingsStore,
		StatusStore:   ReviewStatusSQLStore{DB: ApiTypes.ProjectDBHandle},
		Client:        extractor,
		Logger:        logger,
		Now:           time.Now,
		MaxConcurrent: envInt("REVIEW_MAX_TASKS", 1, 1),

		GrammarClient:     grammarClient,
		GrammarModelName:  grammarModel,
		GrammarPromptRef:  grammarRef,
		GrammarPromptText: grammarPrompt,

		ToneVoiceClient:     toneClient,
		ToneVoiceModelName:  toneModel,
		ToneVoicePromptRef:  toneRef,
		ToneVoicePromptText: tonePrompt,

		FormattingClient:     formattingClient,
		FormattingModelName:  formattingModel,
		FormattingPromptRef:  formattingRef,
		FormattingPromptText: formattingPrompt,

		ReadabilityClient:     readabilityClient,
		ReadabilityModelName:  readabilityModel,
		ReadabilityPromptRef:  readabilityRef,
		ReadabilityPromptText: readabilityPrompt,

		LocalizationClient:     localizationClient,
		LocalizationModelName:  localizationModel,
		LocalizationPromptRef:  localizationRef,
		LocalizationPromptText: localizationPrompt,

		LogicalFlowClient:     logicalFlowClient,
		LogicalFlowModelName:  logicalFlowModel,
		LogicalFlowPromptRef:  logicalFlowRef,
		LogicalFlowPromptText: logicalFlowPrompt,

		HeadingHierarchyClient:     headingHierarchyClient,
		HeadingHierarchyModelName:  headingHierarchyModel,
		HeadingHierarchyPromptRef:  headingHierarchyRef,
		HeadingHierarchyPromptText: headingHierarchyPrompt,

		NavigabilityClient:     navigabilityClient,
		NavigabilityModelName:  navigabilityModel,
		NavigabilityPromptRef:  navigabilityRef,
		NavigabilityPromptText: navigabilityPrompt,

		SectionBalanceClient:     sectionBalanceClient,
		SectionBalanceModelName:  sectionBalanceModel,
		SectionBalancePromptRef:  sectionBalanceRef,
		SectionBalancePromptText: sectionBalancePrompt,

		ModularityClient:     modularityClient,
		ModularityModelName:  modularityModel,
		ModularityPromptRef:  modularityRef,
		ModularityPromptText: modularityPrompt,

		CompletenessClient:     completenessClient,
		CompletenessModelName:  completenessModel,
		CompletenessPromptRef:  completenessRef,
		CompletenessPromptText: completenessPrompt,

		CorrectnessClient:     correctnessClient,
		CorrectnessModelName:  correctnessModel,
		CorrectnessPromptRef:  correctnessRef,
		CorrectnessPromptText: correctnessPrompt,

		ClarityClient:     clarityClient,
		ClarityModelName:  clarityModel,
		ClarityPromptRef:  clarityRef,
		ClarityPromptText: clarityPrompt,

		ConcisenessClient:     concisenessClient,
		ConcisenessModelName:  concisenessModel,
		ConcisenessPromptRef:  concisenessRef,
		ConcisenessPromptText: concisenessPrompt,

		RelevanceClient:     relevanceClient,
		RelevanceModelName:  relevanceModel,
		RelevancePromptRef:  relevanceRef,
		RelevancePromptText: relevancePrompt,

		CurrencyClient:     currencyClient,
		CurrencyModelName:  currencyModel,
		CurrencyPromptRef:  currencyRef,
		CurrencyPromptText: currencyPrompt,
	}
}

func (p *ReviewProcessor) Name() string { return "review_document" }

// HandleEvent is a no-op — review runs entirely in Phase C (DR5).
func (p *ReviewProcessor) HandleEvent(ctx context.Context, payload []byte) error {
	return nil
}

// PostProcessIndex runs all enabled reviewers as concurrent goroutines, collects
// their findings, and persists them to kb.doc_review_findings.
func (p *ReviewProcessor) PostProcessIndex(ctx context.Context, recordID int64) error {
	ctx = withLLMRecordID(ctx, recordID)
	start := p.Now()
	p.Logger.Info("document review start", "record_id", recordID)

	rec, err := p.InputStore.GetInputRecord(ctx, recordID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			p.Logger.Info("document review skipped: record not found", "record_id", recordID)
			return nil
		}
		return fmt.Errorf("(MID_26061802) load kb.inputs record %d: %w", recordID, err)
	}

	reviewers := p.buildReviewers(rec)
	if len(reviewers) == 0 {
		p.Logger.Info("document review skipped: no reviewers enabled", "record_id", recordID)
		return nil
	}

	reviewRunID := p.ReviewRunID
	if reviewRunID == "" {
		reviewRunID = fmt.Sprintf("%d_review_%s", recordID, start.UTC().Format("20060102T150405"))
	}
	p.Logger.Info("document review running",
		"record_id", recordID,
		"review_run_id", reviewRunID,
		"num_reviewers", len(reviewers),
	)

	// Delete previous findings for idempotent re-run (DR7).
	if _, err := p.FindingsStore.DeleteFindings(ctx, recordID); err != nil {
		p.Logger.Warn("failed to delete previous review findings", "record_id", recordID, "error", err)
	}

	allFindings, errs := p.runReviewersPromptCacheOptimized(ctx, recordID, rec, reviewers, reviewRunID)

	if len(allFindings) > 0 {
		inserted, err := p.FindingsStore.SaveFindings(ctx, recordID, reviewRunID, allFindings)
		if err != nil {
			return fmt.Errorf("(MID_26061803) save review findings for record %d: %w", recordID, err)
		}
		p.Logger.Info("review findings saved",
			"record_id", recordID,
			"inserted", inserted,
			"total", len(allFindings),
		)
	}

	msUsed := p.Now().Sub(start).Milliseconds()
	p.Logger.Info("document review complete",
		"record_id", recordID,
		"reviewers", len(reviewers),
		"findings", len(allFindings),
		"errors", len(errs),
		"ms_used", msUsed,
	)

	if len(errs) > 0 {
		return fmt.Errorf("(MID_26061804) %d reviewer(s) failed: %v", len(errs), errs)
	}
	return nil
}

// reviewRunner pairs a Reviewer with its resolved config.
type reviewRunner struct {
	reviewer Reviewer
	cfg      ReviewerConfig
}

// buildReviewers returns the enabled reviewers for this run.
func (p *ReviewProcessor) buildReviewers(_ DocMetadataInputRecord) []reviewRunner {
	var runners []reviewRunner

	if p.GrammarClient != nil && p.GrammarPromptText != "" && p.GrammarModelName != "" {
		runners = append(runners, reviewRunner{
			reviewer: &grammarSpellingReviewer{
				client:     p.GrammarClient,
				logger:     p.Logger,
				chunkStore: SQLStore{DB: ApiTypes.ProjectDBHandle},
				maxTasks:   p.MaxConcurrent,
			},
			cfg: ReviewerConfig{
				Enabled:    true,
				ModelName:  p.GrammarModelName,
				PromptText: p.GrammarPromptText,
				PromptRef:  p.GrammarPromptRef,
			},
		})
	}

	if p.ToneVoiceClient != nil && p.ToneVoicePromptText != "" && p.ToneVoiceModelName != "" {
		runners = append(runners, reviewRunner{
			reviewer: &toneVoiceReviewer{
				client:     p.ToneVoiceClient,
				logger:     p.Logger,
				chunkStore: SQLStore{DB: ApiTypes.ProjectDBHandle},
				maxTasks:   p.MaxConcurrent,
			},
			cfg: ReviewerConfig{
				Enabled:    true,
				ModelName:  p.ToneVoiceModelName,
				PromptText: p.ToneVoicePromptText,
				PromptRef:  p.ToneVoicePromptRef,
			},
		})
	}

	if p.FormattingClient != nil && p.FormattingPromptText != "" && p.FormattingModelName != "" {
		runners = append(runners, reviewRunner{
			reviewer: &formattingConsistencyReviewer{
				client:     p.FormattingClient,
				logger:     p.Logger,
				chunkStore: SQLStore{DB: ApiTypes.ProjectDBHandle},
				maxTasks:   p.MaxConcurrent,
			},
			cfg: ReviewerConfig{
				Enabled:    true,
				ModelName:  p.FormattingModelName,
				PromptText: p.FormattingPromptText,
				PromptRef:  p.FormattingPromptRef,
			},
		})
	}

	if p.ReadabilityClient != nil && p.ReadabilityPromptText != "" && p.ReadabilityModelName != "" {
		runners = append(runners, reviewRunner{
			reviewer: &readabilityReviewer{
				client:     p.ReadabilityClient,
				logger:     p.Logger,
				chunkStore: SQLStore{DB: ApiTypes.ProjectDBHandle},
				maxTasks:   p.MaxConcurrent,
			},
			cfg: ReviewerConfig{
				Enabled:    true,
				ModelName:  p.ReadabilityModelName,
				PromptText: p.ReadabilityPromptText,
				PromptRef:  p.ReadabilityPromptRef,
			},
		})
	}

	if p.LocalizationClient != nil && p.LocalizationPromptText != "" && p.LocalizationModelName != "" {
		runners = append(runners, reviewRunner{
			reviewer: &localizationReviewer{
				client:     p.LocalizationClient,
				logger:     p.Logger,
				chunkStore: SQLStore{DB: ApiTypes.ProjectDBHandle},
				maxTasks:   p.MaxConcurrent,
			},
			cfg: ReviewerConfig{
				Enabled:    true,
				ModelName:  p.LocalizationModelName,
				PromptText: p.LocalizationPromptText,
				PromptRef:  p.LocalizationPromptRef,
			},
		})
	}

	if p.LogicalFlowClient != nil && p.LogicalFlowPromptText != "" && p.LogicalFlowModelName != "" {
		runners = append(runners, reviewRunner{
			reviewer: &logicalFlowReviewer{
				client:     p.LogicalFlowClient,
				logger:     p.Logger,
				chunkStore: SQLStore{DB: ApiTypes.ProjectDBHandle},
				maxTasks:   p.MaxConcurrent,
			},
			cfg: ReviewerConfig{
				Enabled:    true,
				ModelName:  p.LogicalFlowModelName,
				PromptText: p.LogicalFlowPromptText,
				PromptRef:  p.LogicalFlowPromptRef,
			},
		})
	}

	if p.HeadingHierarchyClient != nil && p.HeadingHierarchyPromptText != "" && p.HeadingHierarchyModelName != "" {
		runners = append(runners, reviewRunner{
			reviewer: &headingHierarchyReviewer{
				client:     p.HeadingHierarchyClient,
				logger:     p.Logger,
				chunkStore: SQLStore{DB: ApiTypes.ProjectDBHandle},
				maxTasks:   p.MaxConcurrent,
			},
			cfg: ReviewerConfig{
				Enabled:    true,
				ModelName:  p.HeadingHierarchyModelName,
				PromptText: p.HeadingHierarchyPromptText,
				PromptRef:  p.HeadingHierarchyPromptRef,
			},
		})
	}

	if p.NavigabilityClient != nil && p.NavigabilityPromptText != "" && p.NavigabilityModelName != "" {
		runners = append(runners, reviewRunner{
			reviewer: &navigabilityReviewer{
				client:     p.NavigabilityClient,
				logger:     p.Logger,
				chunkStore: SQLStore{DB: ApiTypes.ProjectDBHandle},
				maxTasks:   p.MaxConcurrent,
			},
			cfg: ReviewerConfig{
				Enabled:    true,
				ModelName:  p.NavigabilityModelName,
				PromptText: p.NavigabilityPromptText,
				PromptRef:  p.NavigabilityPromptRef,
			},
		})
	}

	if p.SectionBalanceClient != nil && p.SectionBalancePromptText != "" && p.SectionBalanceModelName != "" {
		runners = append(runners, reviewRunner{
			reviewer: &sectionBalanceReviewer{
				client:     p.SectionBalanceClient,
				logger:     p.Logger,
				chunkStore: SQLStore{DB: ApiTypes.ProjectDBHandle},
				maxTasks:   p.MaxConcurrent,
			},
			cfg: ReviewerConfig{
				Enabled:    true,
				ModelName:  p.SectionBalanceModelName,
				PromptText: p.SectionBalancePromptText,
				PromptRef:  p.SectionBalancePromptRef,
			},
		})
	}

	if p.ModularityClient != nil && p.ModularityPromptText != "" && p.ModularityModelName != "" {
		runners = append(runners, reviewRunner{
			reviewer: &modularityReviewer{
				client:     p.ModularityClient,
				logger:     p.Logger,
				chunkStore: SQLStore{DB: ApiTypes.ProjectDBHandle},
				maxTasks:   p.MaxConcurrent,
			},
			cfg: ReviewerConfig{
				Enabled:    true,
				ModelName:  p.ModularityModelName,
				PromptText: p.ModularityPromptText,
				PromptRef:  p.ModularityPromptRef,
			},
		})
	}

	if p.CompletenessClient != nil && p.CompletenessPromptText != "" && p.CompletenessModelName != "" {
		runners = append(runners, reviewRunner{
			reviewer: &completenessReviewer{
				client:     p.CompletenessClient,
				logger:     p.Logger,
				chunkStore: SQLStore{DB: ApiTypes.ProjectDBHandle},
				maxTasks:   p.MaxConcurrent,
			},
			cfg: ReviewerConfig{
				Enabled:    true,
				ModelName:  p.CompletenessModelName,
				PromptText: p.CompletenessPromptText,
				PromptRef:  p.CompletenessPromptRef,
			},
		})
	}

	if p.CorrectnessClient != nil && p.CorrectnessPromptText != "" && p.CorrectnessModelName != "" {
		runners = append(runners, reviewRunner{
			reviewer: &correctnessReviewer{
				client:     p.CorrectnessClient,
				logger:     p.Logger,
				chunkStore: SQLStore{DB: ApiTypes.ProjectDBHandle},
				maxTasks:   p.MaxConcurrent,
			},
			cfg: ReviewerConfig{
				Enabled:    true,
				ModelName:  p.CorrectnessModelName,
				PromptText: p.CorrectnessPromptText,
				PromptRef:  p.CorrectnessPromptRef,
			},
		})
	}

	if p.ClarityClient != nil && p.ClarityPromptText != "" && p.ClarityModelName != "" {
		runners = append(runners, reviewRunner{
			reviewer: &clarityReviewer{
				client:     p.ClarityClient,
				logger:     p.Logger,
				chunkStore: SQLStore{DB: ApiTypes.ProjectDBHandle},
				maxTasks:   p.MaxConcurrent,
			},
			cfg: ReviewerConfig{
				Enabled:    true,
				ModelName:  p.ClarityModelName,
				PromptText: p.ClarityPromptText,
				PromptRef:  p.ClarityPromptRef,
			},
		})
	}

	if p.ConcisenessClient != nil && p.ConcisenessPromptText != "" && p.ConcisenessModelName != "" {
		runners = append(runners, reviewRunner{
			reviewer: &concisenessReviewer{
				client:     p.ConcisenessClient,
				logger:     p.Logger,
				chunkStore: SQLStore{DB: ApiTypes.ProjectDBHandle},
				maxTasks:   p.MaxConcurrent,
			},
			cfg: ReviewerConfig{
				Enabled:    true,
				ModelName:  p.ConcisenessModelName,
				PromptText: p.ConcisenessPromptText,
				PromptRef:  p.ConcisenessPromptRef,
			},
		})
	}

	if p.RelevanceClient != nil && p.RelevancePromptText != "" && p.RelevanceModelName != "" {
		runners = append(runners, reviewRunner{
			reviewer: &relevanceReviewer{
				client:     p.RelevanceClient,
				logger:     p.Logger,
				chunkStore: SQLStore{DB: ApiTypes.ProjectDBHandle},
				maxTasks:   p.MaxConcurrent,
			},
			cfg: ReviewerConfig{
				Enabled:    true,
				ModelName:  p.RelevanceModelName,
				PromptText: p.RelevancePromptText,
				PromptRef:  p.RelevancePromptRef,
			},
		})
	}

	if p.CurrencyClient != nil && p.CurrencyPromptText != "" && p.CurrencyModelName != "" {
		runners = append(runners, reviewRunner{
			reviewer: &currencyReviewer{
				client:     p.CurrencyClient,
				logger:     p.Logger,
				chunkStore: SQLStore{DB: ApiTypes.ProjectDBHandle},
				maxTasks:   p.MaxConcurrent,
			},
			cfg: ReviewerConfig{
				Enabled:    true,
				ModelName:  p.CurrencyModelName,
				PromptText: p.CurrencyPromptText,
				PromptRef:  p.CurrencyPromptRef,
			},
		})
	}

	return runners
}

// normalizeFindingsJSON extracts a []ReviewFinding from a raw JSON payload
// returned by the LLM. Expected shape: {"findings": [...]}.
func normalizeFindingsJSON(payload map[string]any) []ReviewFinding {
	if payload == nil {
		return nil
	}
	raw, ok := payload["findings"]
	if !ok {
		return nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]ReviewFinding, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, ReviewFinding{
			Aspect:      strings.TrimSpace(asString(m["aspect"])),
			Severity:    strings.TrimSpace(asString(m["severity"])),
			FindingType: strings.TrimSpace(asString(m["finding_type"])),
			Title:       strings.TrimSpace(asString(m["title"])),
			Description: strings.TrimSpace(asString(m["description"])),
			Evidence:    strings.TrimSpace(asString(m["evidence"])),
			Location:    strings.TrimSpace(asString(m["location"])),
			Suggestion:  strings.TrimSpace(asString(m["suggestion"])),
			Confidence:  asFloat64(m["confidence"]),
		})
	}
	return out
}

func asFloat64(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case json.Number:
		f, _ := x.Float64()
		return f
	default:
		return 0
	}
}

func (p *ReviewProcessor) makeProgressReporter(reviewRunID, aspect string) ReviewerProgressFunc {
	if p.StatusStore == nil || reviewRunID == "" || aspect == "" {
		return nil
	}
	return func(snapshot ReviewerProgress) {
		if err := p.StatusStore.UpdateAspectProgress(context.Background(), reviewRunID, aspect, snapshot.Progress, snapshot.FindingCount); err != nil {
			p.Logger.Warn("reviewer progress update failed",
				"review_run_id", reviewRunID,
				"aspect", aspect,
				"error", err,
			)
		}
	}
}

type reviewerProgressTracker struct {
	report ReviewerProgressFunc
	total  int

	mu           sync.Mutex
	completed    int
	findingCount int
}

func newReviewerProgressTracker(total int, report ReviewerProgressFunc) *reviewerProgressTracker {
	if total <= 0 || report == nil {
		return nil
	}
	return &reviewerProgressTracker{report: report, total: total}
}

func (t *reviewerProgressTracker) add(findings int) {
	if t == nil {
		return
	}
	if findings < 0 {
		findings = 0
	}

	t.mu.Lock()
	t.completed++
	t.findingCount += findings
	snapshot := ReviewerProgress{
		CompletedUnits: t.completed,
		TotalUnits:     t.total,
		FindingCount:   t.findingCount,
		Progress:       float64(t.completed) / float64(t.total),
	}
	t.mu.Unlock()

	t.report(snapshot)
}

func runReviewerConcurrent(
	ctx context.Context,
	maxTasks, total int,
	report ReviewerProgressFunc,
	fn func(ctx context.Context, i int) ([]ReviewFinding, error),
) ([][]ReviewFinding, error) {
	tracker := newReviewerProgressTracker(total, report)
	return runConcurrent(ctx, maxTasks, total, func(workerCtx context.Context, i int) ([]ReviewFinding, error) {
		findings, err := fn(workerCtx, i)
		if err == nil {
			tracker.add(len(findings))
		}
		return findings, err
	})
}
