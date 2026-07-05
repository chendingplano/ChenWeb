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
// Mirrors kb.doc_review_findings columns; the related-artifact cross-reference
// (ADR 2026070201 AR5 §6) is persisted in the metadata JSONB so the report
// generator and GUI can link cross-document findings without parsing prose.
type ReviewFinding struct {
	Pass        string  `json:"pass"`
	Aspect      string  `json:"aspect"`
	Severity    string  `json:"severity"`
	FindingType string  `json:"finding_type"`
	Language    string  `json:"language,omitempty"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Evidence    string  `json:"evidence,omitempty"`
	Location    string  `json:"location,omitempty"`
	Suggestion  string  `json:"suggestion,omitempty"`
	Confidence  float64 `json:"confidence"`

	// RelatedArtifactID / RelatedRecordID identify the matched cross-document
	// artifact a finding is about (metric_id / prov_id / inventory_item_id and
	// its kb.inputs record). Zero values mean "no cross-reference".
	RelatedArtifactID string `json:"related_artifact_id,omitempty"`
	RelatedRecordID   int64  `json:"related_record_id,omitempty"`
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

// DefaultChunkInputSize is the number of lines per chunk for per-chunk reviewers
// in the prompt-cache scheduler.
const DefaultChunkInputSize = 200

// chunkInput holds the lines JSON and line range for one per-chunk LLM call.
// All per-chunk reviewers share this type so the prompt-cache scheduler can
// build chunk inputs once and dispatch them across reviewers.
type chunkInput struct {
	inputJSON string
	startLine int
	endLine   int
}

// buildChunkInputs splits lines into fixed-size chunks, wrapping each in the
// doc_context envelope. Used by the prompt-cache scheduler for all per-chunk
// reviewers; size is typically DefaultChunkInputSize (200 lines).
func buildChunkInputs(lines []Line, docCtx string, size int) []chunkInput {
	if size <= 0 {
		size = DefaultChunkInputSize
	}
	var chunks []chunkInput
	for i := 0; i < len(lines); i += size {
		end := min(i+size, len(lines))
		slice := lines[i:end]
		objs := rawLinesToJSON(slice)
		jsonText := wrapLinesWithDocContext(objs, docCtx)
		chunks = append(chunks, chunkInput{
			inputJSON: jsonText,
			startLine: slice[0].LineNo,
			endLine:   slice[len(slice)-1].LineNo,
		})
	}
	return chunks
}

// chunkReviewer is implemented by reviewers whose config input = "per-chunk".
// The prompt-cache scheduler calls processChunk once per chunk input.
type chunkReviewer interface {
	processChunk(ctx context.Context, recordID int64, index int, cfg ReviewerConfig, input chunkInput) []ReviewFinding
}

// blockReviewer is implemented by reviewers whose config input = "per-block".
// The prompt-cache scheduler calls processBlock once per page-aligned block.
type blockReviewer interface {
	processBlock(ctx context.Context, recordID int64, index, total int, cfg ReviewerConfig, b pageBlock) []ReviewFinding
}

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
	Enabled       bool     // whether this reviewer runs
	Input         string   // "per-chunk" or "per-block"; drives prompt-cache scheduler dispatch
	ModelName     string   // resolved model name
	PromptText    string   // prompt body
	PromptRef     string   // prompt file name (for logging)
	MaxToolTurns  int      // 0 = one-shot, >0 = tool-use conversation loop
	MaxToolTokens int      // cumulative token budget for the tool-use loop (DR10c); 0 = code default
	Tools         []string // tool names available (only when MaxToolTurns > 0)
	OnProgress    ReviewerProgressFunc
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
	SaveFindings(ctx context.Context, recordID int64, runID int64, findings []ReviewFinding) (int64, error)
	DeleteFindings(ctx context.Context, runID int64) (int64, error)
}

// ReviewFindingsSQLStore implements ReviewFindingsStore.
type ReviewFindingsSQLStore struct {
	DB         *sql.DB
	Translator FindingTranslator
	Languages  []string
}

func (s ReviewFindingsSQLStore) SaveFindings(ctx context.Context, recordID int64, runID int64, findings []ReviewFinding) (int64, error) {
	if s.DB == nil {
		return 0, fmt.Errorf("db is nil")
	}
	if len(findings) == 0 {
		return 0, nil
	}

	const stmt = `
INSERT INTO kb.doc_review_findings
    (input_record_id, run_id, pass, aspect, severity, finding_type,
     title, description, evidence, location, suggestion, confidence, metadata)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`

	preparedFindings, err := runConcurrent(ctx, maxDocReviewerTasks(len(findings)), len(findings), func(workerCtx context.Context, i int) (preparedFindingForStorage, error) {
		prepared, err := prepareFindingForStorage(workerCtx, s.Translator, s.Languages, findings[i])
		if err != nil {
			return preparedFindingForStorage{}, fmt.Errorf("prepare review finding for storage: %w", err)
		}
		return prepared, nil
	})
	if err != nil {
		return 0, err
	}

	var inserted int64
	for _, prepared := range preparedFindings {
		var evidence, location, suggestion any
		if prepared.Canonical.Evidence != "" {
			evidence = prepared.Canonical.Evidence
		}
		if prepared.Canonical.Location != "" {
			location = prepared.Canonical.Location
		}
		if prepared.Canonical.Suggestion != "" {
			suggestion = prepared.Canonical.Suggestion
		}
		confidence := prepared.Canonical.Confidence
		if confidence <= 0 {
			confidence = 0.5
		}
		metadata, err := json.Marshal(prepared.Metadata)
		if err != nil {
			return inserted, fmt.Errorf("marshal finding metadata: %w", err)
		}

		_, err = s.DB.ExecContext(ctx, stmt,
			recordID, runID, prepared.Canonical.Pass, prepared.Canonical.Aspect, prepared.Canonical.Severity, prepared.Canonical.FindingType,
			prepared.Canonical.Title, prepared.Canonical.Description, evidence, location, suggestion, confidence, metadata,
		)
		if err != nil {
			return inserted, fmt.Errorf("insert review finding: %w", err)
		}
		inserted++
	}
	return inserted, nil
}

func (s ReviewFindingsSQLStore) DeleteFindings(ctx context.Context, runID int64) (int64, error) {
	if s.DB == nil {
		return 0, fmt.Errorf("db is nil")
	}
	res, err := s.DB.ExecContext(ctx, `DELETE FROM kb.doc_review_findings WHERE run_id = $1`, runID)
	if err != nil {
		return 0, fmt.Errorf("delete review findings for run %d: %w", runID, err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// ReviewProcessor orchestrates document review in Phase C (post-processing).
// It owns a set of Reviewers; each runs as a goroutine and reports findings.
type ReviewProcessor struct {
	InputStore      DocMetadataStore
	EntityStore     EntityRelationStore // for tool-using reviewers (Phase II+)
	FindingsStore   ReviewFindingsStore
	ReviewLogsStore ReviewLogsStore
	StatusStore     ReviewStatusStore
	Client          LLMJSONExtractor // shared LLM client
	Logger          ApiTypes.JimoLogger
	Now             func() time.Time

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

	ExamplesModelName  string
	ExamplesPromptRef  string
	ExamplesPromptText string

	DiagramsModelName  string
	DiagramsPromptRef  string
	DiagramsPromptText string

	TestableClaimsModelName  string
	TestableClaimsPromptRef  string
	TestableClaimsPromptText string

	EvidenceRationaleModelName  string
	EvidenceRationalePromptRef  string
	EvidenceRationalePromptText string

	// EvidenceRationaleTool fields — only non-zero/empty when the reviewer
	// uses the tool-use path (max_tool_turns > 0 in doc-review.local.toml).
	EvidenceRationaleMaxToolTurns  int
	EvidenceRationaleMaxToolTokens int
	EvidenceRationaleTools         []string

	// Metrics is the cross-document metric consistency reviewer (P5, ADR 2026063002).
	MetricsModelName  string
	MetricsPromptRef  string
	MetricsPromptText string

	// Provisions is the cross-document provision consistency reviewer (P5, ADR 2026063003).
	ProvisionsModelName  string
	ProvisionsPromptRef  string
	ProvisionsPromptText string

	// Entities is the cross-document entity consistency reviewer (P5, ADR 2026063004).
	EntitiesModelName  string
	EntitiesPromptRef  string
	EntitiesPromptText string

	// InventoryItems is the cross-document inventory-item consistency reviewer (P5, ADR 2026063005).
	InventoryItemsModelName  string
	InventoryItemsPromptRef  string
	InventoryItemsPromptText string

	MaxConcurrent int // max concurrent chunk workers per chunk-based reviewer

	// RunID must be set by the caller (DocReviewController) before PostProcessIndex
	// is invoked. Findings and progress updates are scoped to this run.
	RunID int64

	// RequestedAspects limits a run to the accepted request's selected aspects.
	// When empty, all configured reviewers remain eligible.
	RequestedAspects []string

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

	// ExamplesClient is a properly-configured LLM client for the
	// examples reviewer (P3, content quality, per-chunk).
	ExamplesClient LLMJSONExtractor

	// DiagramsClient is a properly-configured LLM client for the
	// diagrams reviewer (P3, content quality, per-chunk).
	DiagramsClient LLMJSONExtractor

	// TestableClaimsClient is a properly-configured LLM client for the
	// testable_claims reviewer (P3, content quality, per-chunk).
	TestableClaimsClient LLMJSONExtractor

	// EvidenceRationaleClient is a properly-configured LLM client for the
	// evidence_rationale reviewer (P3, content quality, per-chunk).
	EvidenceRationaleClient LLMJSONExtractor

	// EvidenceRationaleToolClient is a tool-capable LLM client (shared llm.Client)
	// for the evidence_rationale reviewer when it runs the tool-use path.
	// nil when max_tool_turns = 0 or no tool client could be resolved.
	EvidenceRationaleToolClient LLMChatClient

	// ── P4 — Consistency ───────────────────────────────────────────────────

	InternalContradictionsModelName  string
	InternalContradictionsPromptRef  string
	InternalContradictionsPromptText string

	TerminologyConsistencyModelName  string
	TerminologyConsistencyPromptRef  string
	TerminologyConsistencyPromptText string

	CrossReferenceCorrectnessModelName  string
	CrossReferenceCorrectnessPromptRef  string
	CrossReferenceCorrectnessPromptText string

	RequirementTraceabilityModelName  string
	RequirementTraceabilityPromptRef  string
	RequirementTraceabilityPromptText string

	// P4 tool fields — only non-zero/empty when the reviewer uses the tool-use
	// path (max_tool_turns > 0 in doc-review.local.toml).
	InternalContradictionsMaxToolTurns  int
	InternalContradictionsMaxToolTokens int
	InternalContradictionsTools         []string

	TerminologyConsistencyMaxToolTurns  int
	TerminologyConsistencyMaxToolTokens int
	TerminologyConsistencyTools         []string

	CrossReferenceCorrectnessMaxToolTurns  int
	CrossReferenceCorrectnessMaxToolTokens int
	CrossReferenceCorrectnessTools         []string

	RequirementTraceabilityMaxToolTurns  int
	RequirementTraceabilityMaxToolTokens int
	RequirementTraceabilityTools         []string

	InternalContradictionsClient    LLMJSONExtractor
	TerminologyConsistencyClient    LLMJSONExtractor
	CrossReferenceCorrectnessClient LLMJSONExtractor
	RequirementTraceabilityClient   LLMJSONExtractor

	InternalContradictionsToolClient    LLMChatClient
	TerminologyConsistencyToolClient    LLMChatClient
	CrossReferenceCorrectnessToolClient LLMChatClient
	RequirementTraceabilityToolClient   LLMChatClient

	// ── P5 — Technical & Compliance ───────────────────────────────────────────

	TechnicalAccuracyModelName  string
	TechnicalAccuracyPromptRef  string
	TechnicalAccuracyPromptText string

	AssumptionsModelName  string
	AssumptionsPromptRef  string
	AssumptionsPromptText string

	PrerequisitesModelName  string
	PrerequisitesPromptRef  string
	PrerequisitesPromptText string

	StandardsComplianceModelName  string
	StandardsCompliancePromptRef  string
	StandardsCompliancePromptText string

	LegalComplianceModelName  string
	LegalCompliancePromptRef  string
	LegalCompliancePromptText string

	RegulatoryComplianceModelName  string
	RegulatoryCompliancePromptRef  string
	RegulatoryCompliancePromptText string

	InternalPolicyModelName  string
	InternalPolicyPromptRef  string
	InternalPolicyPromptText string

	SecurityModelName  string
	SecurityPromptRef  string
	SecurityPromptText string

	PerformanceModelName  string
	PerformancePromptRef  string
	PerformancePromptText string

	ErrorHandlingModelName  string
	ErrorHandlingPromptRef  string
	ErrorHandlingPromptText string

	LimitationsModelName  string
	LimitationsPromptRef  string
	LimitationsPromptText string

	// P5 tool fields
	TechnicalAccuracyMaxToolTurns  int
	TechnicalAccuracyMaxToolTokens int
	TechnicalAccuracyTools         []string

	AssumptionsMaxToolTurns  int
	AssumptionsMaxToolTokens int
	AssumptionsTools         []string

	PrerequisitesMaxToolTurns  int
	PrerequisitesMaxToolTokens int
	PrerequisitesTools         []string

	StandardsComplianceMaxToolTurns  int
	StandardsComplianceMaxToolTokens int
	StandardsComplianceTools         []string

	LegalComplianceMaxToolTurns  int
	LegalComplianceMaxToolTokens int
	LegalComplianceTools         []string

	RegulatoryComplianceMaxToolTurns  int
	RegulatoryComplianceMaxToolTokens int
	RegulatoryComplianceTools         []string

	InternalPolicyMaxToolTurns  int
	InternalPolicyMaxToolTokens int
	InternalPolicyTools         []string

	SecurityMaxToolTurns  int
	SecurityMaxToolTokens int
	SecurityTools         []string

	PerformanceMaxToolTurns  int
	PerformanceMaxToolTokens int
	PerformanceTools         []string

	ErrorHandlingMaxToolTurns  int
	ErrorHandlingMaxToolTokens int
	ErrorHandlingTools         []string

	LimitationsMaxToolTurns  int
	LimitationsMaxToolTokens int
	LimitationsTools         []string

	TechnicalAccuracyClient    LLMJSONExtractor
	AssumptionsClient          LLMJSONExtractor
	PrerequisitesClient        LLMJSONExtractor
	StandardsComplianceClient  LLMJSONExtractor
	LegalComplianceClient      LLMJSONExtractor
	RegulatoryComplianceClient LLMJSONExtractor
	InternalPolicyClient       LLMJSONExtractor
	SecurityClient             LLMJSONExtractor
	PerformanceClient          LLMJSONExtractor
	ErrorHandlingClient        LLMJSONExtractor
	LimitationsClient          LLMJSONExtractor

	TechnicalAccuracyToolClient    LLMChatClient
	AssumptionsToolClient          LLMChatClient
	PrerequisitesToolClient        LLMChatClient
	StandardsComplianceToolClient  LLMChatClient
	LegalComplianceToolClient      LLMChatClient
	RegulatoryComplianceToolClient LLMChatClient
	InternalPolicyToolClient       LLMChatClient
	SecurityToolClient             LLMChatClient
	PerformanceToolClient          LLMChatClient
	ErrorHandlingToolClient        LLMChatClient
	LimitationsToolClient          LLMChatClient

	// MetricsClient is the LLM client for the cross-document metrics reviewer.
	MetricsClient LLMJSONExtractor

	// ProvisionsClient is the LLM client for the cross-document provisions reviewer.
	ProvisionsClient LLMJSONExtractor

	// EntitiesClient is the LLM client for the cross-document entities reviewer.
	EntitiesClient LLMJSONExtractor

	// InventoryItemsClient is the LLM client for the cross-document inventory-items reviewer.
	InventoryItemsClient LLMJSONExtractor

	// Tool-use budgets and chat clients for the artifact reviewers
	// (ADR 2026070201 AR4). The entities reviewer stays one-shot (out of scope).
	MetricsMaxToolTurns  int
	MetricsMaxToolTokens int
	MetricsTools         []string
	MetricsToolClient    LLMChatClient

	ProvisionsMaxToolTurns  int
	ProvisionsMaxToolTokens int
	ProvisionsTools         []string
	ProvisionsToolClient    LLMChatClient

	InventoryItemsMaxToolTurns  int
	InventoryItemsMaxToolTokens int
	InventoryItemsTools         []string
	InventoryItemsToolClient    LLMChatClient

	// MetricsCompleteness* fields for the object-anchored missing-metric pass
	// (ADR 2026070201 AR6, Stage 5).
	MetricsCompletenessClient        LLMJSONExtractor
	MetricsCompletenessModelName     string
	MetricsCompletenessPromptRef     string
	MetricsCompletenessPromptText    string
	MetricsCompletenessMaxToolTurns  int
	MetricsCompletenessMaxToolTokens int
	MetricsCompletenessTools         []string
	MetricsCompletenessToolClient    LLMChatClient
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

	// logger.Info("reviewer configured from doc-review config",
	// 	"aspect", aspect, "model_ref", resolved.ModelRef, "prompt_ref", resolved.PromptRef,
	// 	"enabled", resolved.Enabled, "max_tool_turns", resolved.MaxToolTurns)
	return client, modelName, promptText, promptRef, true
}

// resolveReviewerBudget resolves MaxToolTurns, MaxToolTokens, and Tools for a
// reviewer from doc-review.local.toml, matching the resolution path of
// resolveReviewerRuntime. Returns zero values when no config is present.
func resolveReviewerBudget(aspect, group string) (maxToolTurns, maxToolTokens int, tools []string) {
	cfg, err := GetDocReviewConfig()
	if err != nil || cfg == nil {
		return 0, 0, nil
	}
	resolved := cfg.ResolveReviewer(aspect, group)
	if !resolved.Found {
		return 0, 0, nil
	}
	return resolved.MaxToolTurns, resolved.MaxToolTokens, resolved.Tools
}

// resolveReviewerToolClient builds a tool-capable chat client when the
// reviewer's resolved MaxToolTurns > 0. Returns nil when the reviewer is
// configured for one-shot or no model ref resolves.
func resolveReviewerToolClient(logger ApiTypes.JimoLogger, aspect, group string, maxToolTurns int) LLMChatClient {
	if maxToolTurns <= 0 {
		return nil
	}
	cfg, err := GetDocReviewConfig()
	if err != nil || cfg == nil {
		return nil
	}
	resolved := cfg.ResolveReviewer(aspect, group)
	if !resolved.Found || resolved.ModelRef == "" {
		return nil
	}
	client, _, err := BuildReviewerToolClient(resolved.ModelRef)
	if err != nil {
		logger.Warn("tool client resolution failed; reviewer stays one-shot",
			"aspect", aspect, "model_ref", resolved.ModelRef, "error", err)
		return nil
	}
	return client
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
	examplesClient, examplesModel, examplesPrompt, examplesRef, _ := resolveReviewerRuntime(logger, "examples", "P3")
	diagramsClient, diagramsModel, diagramsPrompt, diagramsRef, _ := resolveReviewerRuntime(logger, "diagrams", "P3")
	testableClaimsClient, testableClaimsModel, testableClaimsPrompt, testableClaimsRef, _ := resolveReviewerRuntime(logger, "testable_claims", "P3")
	evidenceRationaleClient, evidenceRationaleModel, evidenceRationalePrompt, evidenceRationaleRef, _ := resolveReviewerRuntime(logger, "evidence_rationale", "P3")
	evidenceRationaleMaxTurns, evidenceRationaleMaxTokens, evidenceRationaleToolList := resolveReviewerBudget("evidence_rationale", "P3")
	evidenceRationaleToolClient := resolveReviewerToolClient(logger, "evidence_rationale", "P3", evidenceRationaleMaxTurns)

	// P4 — Consistency
	internalContradictionsClient, internalContradictionsModel, internalContradictionsPrompt, internalContradictionsRef, _ := resolveReviewerRuntime(logger, "internal_contradictions", "P4")
	terminologyConsistencyClient, terminologyConsistencyModel, terminologyConsistencyPrompt, terminologyConsistencyRef, _ := resolveReviewerRuntime(logger, "terminology_consistency", "P4")
	crossReferenceCorrectnessClient, crossReferenceCorrectnessModel, crossReferenceCorrectnessPrompt, crossReferenceCorrectnessRef, _ := resolveReviewerRuntime(logger, "cross_reference_correctness", "P4")
	requirementTraceabilityClient, requirementTraceabilityModel, requirementTraceabilityPrompt, requirementTraceabilityRef, _ := resolveReviewerRuntime(logger, "requirement_traceability", "P4")
	internalContradictionsMaxTurns, internalContradictionsMaxTokens, internalContradictionsToolList := resolveReviewerBudget("internal_contradictions", "P4")
	terminologyConsistencyMaxTurns, terminologyConsistencyMaxTokens, terminologyConsistencyToolList := resolveReviewerBudget("terminology_consistency", "P4")
	crossReferenceCorrectnessMaxTurns, crossReferenceCorrectnessMaxTokens, crossReferenceCorrectnessToolList := resolveReviewerBudget("cross_reference_correctness", "P4")
	requirementTraceabilityMaxTurns, requirementTraceabilityMaxTokens, requirementTraceabilityToolList := resolveReviewerBudget("requirement_traceability", "P4")
	internalContradictionsToolClient := resolveReviewerToolClient(logger, "internal_contradictions", "P4", internalContradictionsMaxTurns)
	terminologyConsistencyToolClient := resolveReviewerToolClient(logger, "terminology_consistency", "P4", terminologyConsistencyMaxTurns)
	crossReferenceCorrectnessToolClient := resolveReviewerToolClient(logger, "cross_reference_correctness", "P4", crossReferenceCorrectnessMaxTurns)
	requirementTraceabilityToolClient := resolveReviewerToolClient(logger, "requirement_traceability", "P4", requirementTraceabilityMaxTurns)

	// P5 — Technical & Compliance
	technicalAccuracyClient, technicalAccuracyModel, technicalAccuracyPrompt, technicalAccuracyRef, _ := resolveReviewerRuntime(logger, "technical_accuracy", "P5")
	assumptionsClient, assumptionsModel, assumptionsPrompt, assumptionsRef, _ := resolveReviewerRuntime(logger, "assumptions", "P5")
	prerequisitesClient, prerequisitesModel, prerequisitesPrompt, prerequisitesRef, _ := resolveReviewerRuntime(logger, "prerequisites", "P5")
	standardsComplianceClient, standardsComplianceModel, standardsCompliancePrompt, standardsComplianceRef, _ := resolveReviewerRuntime(logger, "standards_compliance", "P5")
	legalComplianceClient, legalComplianceModel, legalCompliancePrompt, legalComplianceRef, _ := resolveReviewerRuntime(logger, "legal_compliance", "P5")
	regulatoryComplianceClient, regulatoryComplianceModel, regulatoryCompliancePrompt, regulatoryComplianceRef, _ := resolveReviewerRuntime(logger, "regulatory_compliance", "P5")
	internalPolicyClient, internalPolicyModel, internalPolicyPrompt, internalPolicyRef, _ := resolveReviewerRuntime(logger, "internal_policy", "P5")
	securityClient, securityModel, securityPrompt, securityRef, _ := resolveReviewerRuntime(logger, "security", "P5")
	performanceClient, performanceModel, performancePrompt, performanceRef, _ := resolveReviewerRuntime(logger, "performance", "P5")
	errorHandlingClient, errorHandlingModel, errorHandlingPrompt, errorHandlingRef, _ := resolveReviewerRuntime(logger, "error_handling", "P5")
	limitationsClient, limitationsModel, limitationsPrompt, limitationsRef, _ := resolveReviewerRuntime(logger, "limitations", "P5")
	metricsClient, metricsModel, metricsPrompt, metricsRef, _ := resolveReviewerRuntime(logger, "metrics", "P5")
	provisionsClient, provisionsModel, provisionsPrompt, provisionsRef, _ := resolveReviewerRuntime(logger, "provisions", "P5")
	entitiesClient, entitiesModel, entitiesPrompt, entitiesRef, _ := resolveReviewerRuntime(logger, "entities", "P5")
	inventoryItemsClient, inventoryItemsModel, inventoryItemsPrompt, inventoryItemsRef, _ := resolveReviewerRuntime(logger, "inventory_items", "P5")

	// Artifact reviewers gain tool-use (ADR 2026070201 AR4): max_tool_turns > 0
	// in doc-review.local.toml enables the DR10b loop with the listed tools.
	metricsMaxTurns, metricsMaxTokens, metricsToolList := resolveReviewerBudget("metrics", "P5")
	provisionsMaxTurns, provisionsMaxTokens, provisionsToolList := resolveReviewerBudget("provisions", "P5")
	inventoryItemsMaxTurns, inventoryItemsMaxTokens, inventoryItemsToolList := resolveReviewerBudget("inventory_items", "P5")
	metricsToolClient := resolveReviewerToolClient(logger, "metrics", "P5", metricsMaxTurns)
	provisionsToolClient := resolveReviewerToolClient(logger, "provisions", "P5", provisionsMaxTurns)
	inventoryItemsToolClient := resolveReviewerToolClient(logger, "inventory_items", "P5", inventoryItemsMaxTurns)

	// metrics_completeness — object-anchored missing-metric pass (ADR 2026070201 AR6).
	metricsCompletenessClient, metricsCompletenessModel, metricsCompletenessPrompt, metricsCompletenessRef, _ := resolveReviewerRuntime(logger, "metrics_completeness", "P5")
	metricsCompletenessMaxTurns, metricsCompletenessMaxTokens, metricsCompletenessToolList := resolveReviewerBudget("metrics_completeness", "P5")
	metricsCompletenessToolClient := resolveReviewerToolClient(logger, "metrics_completeness", "P5", metricsCompletenessMaxTurns)

	technicalAccuracyMaxTurns, technicalAccuracyMaxTokens, technicalAccuracyToolList := resolveReviewerBudget("technical_accuracy", "P5")
	assumptionsMaxTurns, assumptionsMaxTokens, assumptionsToolList := resolveReviewerBudget("assumptions", "P5")
	prerequisitesMaxTurns, prerequisitesMaxTokens, prerequisitesToolList := resolveReviewerBudget("prerequisites", "P5")
	standardsComplianceMaxTurns, standardsComplianceMaxTokens, standardsComplianceToolList := resolveReviewerBudget("standards_compliance", "P5")
	legalComplianceMaxTurns, legalComplianceMaxTokens, legalComplianceToolList := resolveReviewerBudget("legal_compliance", "P5")
	regulatoryComplianceMaxTurns, regulatoryComplianceMaxTokens, regulatoryComplianceToolList := resolveReviewerBudget("regulatory_compliance", "P5")
	internalPolicyMaxTurns, internalPolicyMaxTokens, internalPolicyToolList := resolveReviewerBudget("internal_policy", "P5")
	securityMaxTurns, securityMaxTokens, securityToolList := resolveReviewerBudget("security", "P5")
	performanceMaxTurns, performanceMaxTokens, performanceToolList := resolveReviewerBudget("performance", "P5")
	errorHandlingMaxTurns, errorHandlingMaxTokens, errorHandlingToolList := resolveReviewerBudget("error_handling", "P5")
	limitationsMaxTurns, limitationsMaxTokens, limitationsToolList := resolveReviewerBudget("limitations", "P5")

	technicalAccuracyToolClient := resolveReviewerToolClient(logger, "technical_accuracy", "P5", technicalAccuracyMaxTurns)
	assumptionsToolClient := resolveReviewerToolClient(logger, "assumptions", "P5", assumptionsMaxTurns)
	prerequisitesToolClient := resolveReviewerToolClient(logger, "prerequisites", "P5", prerequisitesMaxTurns)
	standardsComplianceToolClient := resolveReviewerToolClient(logger, "standards_compliance", "P5", standardsComplianceMaxTurns)
	legalComplianceToolClient := resolveReviewerToolClient(logger, "legal_compliance", "P5", legalComplianceMaxTurns)
	regulatoryComplianceToolClient := resolveReviewerToolClient(logger, "regulatory_compliance", "P5", regulatoryComplianceMaxTurns)
	internalPolicyToolClient := resolveReviewerToolClient(logger, "internal_policy", "P5", internalPolicyMaxTurns)
	securityToolClient := resolveReviewerToolClient(logger, "security", "P5", securityMaxTurns)
	performanceToolClient := resolveReviewerToolClient(logger, "performance", "P5", performanceMaxTurns)
	errorHandlingToolClient := resolveReviewerToolClient(logger, "error_handling", "P5", errorHandlingMaxTurns)
	limitationsToolClient := resolveReviewerToolClient(logger, "limitations", "P5", limitationsMaxTurns)

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

		ExamplesClient:     examplesClient,
		ExamplesModelName:  examplesModel,
		ExamplesPromptRef:  examplesRef,
		ExamplesPromptText: examplesPrompt,

		DiagramsClient:     diagramsClient,
		DiagramsModelName:  diagramsModel,
		DiagramsPromptRef:  diagramsRef,
		DiagramsPromptText: diagramsPrompt,

		TestableClaimsClient:     testableClaimsClient,
		TestableClaimsModelName:  testableClaimsModel,
		TestableClaimsPromptRef:  testableClaimsRef,
		TestableClaimsPromptText: testableClaimsPrompt,

		EvidenceRationaleClient:     evidenceRationaleClient,
		EvidenceRationaleModelName:  evidenceRationaleModel,
		EvidenceRationalePromptRef:  evidenceRationaleRef,
		EvidenceRationalePromptText: evidenceRationalePrompt,

		EvidenceRationaleMaxToolTurns:  evidenceRationaleMaxTurns,
		EvidenceRationaleMaxToolTokens: evidenceRationaleMaxTokens,
		EvidenceRationaleTools:         evidenceRationaleToolList,

		EvidenceRationaleToolClient: evidenceRationaleToolClient,

		// P4 — Consistency
		InternalContradictionsClient:     internalContradictionsClient,
		InternalContradictionsModelName:  internalContradictionsModel,
		InternalContradictionsPromptRef:  internalContradictionsRef,
		InternalContradictionsPromptText: internalContradictionsPrompt,

		TerminologyConsistencyClient:     terminologyConsistencyClient,
		TerminologyConsistencyModelName:  terminologyConsistencyModel,
		TerminologyConsistencyPromptRef:  terminologyConsistencyRef,
		TerminologyConsistencyPromptText: terminologyConsistencyPrompt,

		CrossReferenceCorrectnessClient:     crossReferenceCorrectnessClient,
		CrossReferenceCorrectnessModelName:  crossReferenceCorrectnessModel,
		CrossReferenceCorrectnessPromptRef:  crossReferenceCorrectnessRef,
		CrossReferenceCorrectnessPromptText: crossReferenceCorrectnessPrompt,

		RequirementTraceabilityClient:     requirementTraceabilityClient,
		RequirementTraceabilityModelName:  requirementTraceabilityModel,
		RequirementTraceabilityPromptRef:  requirementTraceabilityRef,
		RequirementTraceabilityPromptText: requirementTraceabilityPrompt,

		InternalContradictionsMaxToolTurns:  internalContradictionsMaxTurns,
		InternalContradictionsMaxToolTokens: internalContradictionsMaxTokens,
		InternalContradictionsTools:         internalContradictionsToolList,

		TerminologyConsistencyMaxToolTurns:  terminologyConsistencyMaxTurns,
		TerminologyConsistencyMaxToolTokens: terminologyConsistencyMaxTokens,
		TerminologyConsistencyTools:         terminologyConsistencyToolList,

		CrossReferenceCorrectnessMaxToolTurns:  crossReferenceCorrectnessMaxTurns,
		CrossReferenceCorrectnessMaxToolTokens: crossReferenceCorrectnessMaxTokens,
		CrossReferenceCorrectnessTools:         crossReferenceCorrectnessToolList,

		RequirementTraceabilityMaxToolTurns:  requirementTraceabilityMaxTurns,
		RequirementTraceabilityMaxToolTokens: requirementTraceabilityMaxTokens,
		RequirementTraceabilityTools:         requirementTraceabilityToolList,

		InternalContradictionsToolClient:    internalContradictionsToolClient,
		TerminologyConsistencyToolClient:    terminologyConsistencyToolClient,
		CrossReferenceCorrectnessToolClient: crossReferenceCorrectnessToolClient,
		RequirementTraceabilityToolClient:   requirementTraceabilityToolClient,

		// P5 — Technical & Compliance
		TechnicalAccuracyClient:     technicalAccuracyClient,
		TechnicalAccuracyModelName:  technicalAccuracyModel,
		TechnicalAccuracyPromptRef:  technicalAccuracyRef,
		TechnicalAccuracyPromptText: technicalAccuracyPrompt,

		AssumptionsClient:     assumptionsClient,
		AssumptionsModelName:  assumptionsModel,
		AssumptionsPromptRef:  assumptionsRef,
		AssumptionsPromptText: assumptionsPrompt,

		PrerequisitesClient:     prerequisitesClient,
		PrerequisitesModelName:  prerequisitesModel,
		PrerequisitesPromptRef:  prerequisitesRef,
		PrerequisitesPromptText: prerequisitesPrompt,

		StandardsComplianceClient:     standardsComplianceClient,
		StandardsComplianceModelName:  standardsComplianceModel,
		StandardsCompliancePromptRef:  standardsComplianceRef,
		StandardsCompliancePromptText: standardsCompliancePrompt,

		LegalComplianceClient:     legalComplianceClient,
		LegalComplianceModelName:  legalComplianceModel,
		LegalCompliancePromptRef:  legalComplianceRef,
		LegalCompliancePromptText: legalCompliancePrompt,

		RegulatoryComplianceClient:     regulatoryComplianceClient,
		RegulatoryComplianceModelName:  regulatoryComplianceModel,
		RegulatoryCompliancePromptRef:  regulatoryComplianceRef,
		RegulatoryCompliancePromptText: regulatoryCompliancePrompt,

		InternalPolicyClient:     internalPolicyClient,
		InternalPolicyModelName:  internalPolicyModel,
		InternalPolicyPromptRef:  internalPolicyRef,
		InternalPolicyPromptText: internalPolicyPrompt,

		SecurityClient:     securityClient,
		SecurityModelName:  securityModel,
		SecurityPromptRef:  securityRef,
		SecurityPromptText: securityPrompt,

		PerformanceClient:     performanceClient,
		PerformanceModelName:  performanceModel,
		PerformancePromptRef:  performanceRef,
		PerformancePromptText: performancePrompt,

		ErrorHandlingClient:     errorHandlingClient,
		ErrorHandlingModelName:  errorHandlingModel,
		ErrorHandlingPromptRef:  errorHandlingRef,
		ErrorHandlingPromptText: errorHandlingPrompt,

		LimitationsClient:     limitationsClient,
		LimitationsModelName:  limitationsModel,
		LimitationsPromptRef:  limitationsRef,
		LimitationsPromptText: limitationsPrompt,

		TechnicalAccuracyMaxToolTurns:  technicalAccuracyMaxTurns,
		TechnicalAccuracyMaxToolTokens: technicalAccuracyMaxTokens,
		TechnicalAccuracyTools:         technicalAccuracyToolList,

		AssumptionsMaxToolTurns:  assumptionsMaxTurns,
		AssumptionsMaxToolTokens: assumptionsMaxTokens,
		AssumptionsTools:         assumptionsToolList,

		PrerequisitesMaxToolTurns:  prerequisitesMaxTurns,
		PrerequisitesMaxToolTokens: prerequisitesMaxTokens,
		PrerequisitesTools:         prerequisitesToolList,

		StandardsComplianceMaxToolTurns:  standardsComplianceMaxTurns,
		StandardsComplianceMaxToolTokens: standardsComplianceMaxTokens,
		StandardsComplianceTools:         standardsComplianceToolList,

		LegalComplianceMaxToolTurns:  legalComplianceMaxTurns,
		LegalComplianceMaxToolTokens: legalComplianceMaxTokens,
		LegalComplianceTools:         legalComplianceToolList,

		RegulatoryComplianceMaxToolTurns:  regulatoryComplianceMaxTurns,
		RegulatoryComplianceMaxToolTokens: regulatoryComplianceMaxTokens,
		RegulatoryComplianceTools:         regulatoryComplianceToolList,

		InternalPolicyMaxToolTurns:  internalPolicyMaxTurns,
		InternalPolicyMaxToolTokens: internalPolicyMaxTokens,
		InternalPolicyTools:         internalPolicyToolList,

		SecurityMaxToolTurns:  securityMaxTurns,
		SecurityMaxToolTokens: securityMaxTokens,
		SecurityTools:         securityToolList,

		PerformanceMaxToolTurns:  performanceMaxTurns,
		PerformanceMaxToolTokens: performanceMaxTokens,
		PerformanceTools:         performanceToolList,

		ErrorHandlingMaxToolTurns:  errorHandlingMaxTurns,
		ErrorHandlingMaxToolTokens: errorHandlingMaxTokens,
		ErrorHandlingTools:         errorHandlingToolList,

		LimitationsMaxToolTurns:  limitationsMaxTurns,
		LimitationsMaxToolTokens: limitationsMaxTokens,
		LimitationsTools:         limitationsToolList,

		TechnicalAccuracyToolClient:    technicalAccuracyToolClient,
		AssumptionsToolClient:          assumptionsToolClient,
		PrerequisitesToolClient:        prerequisitesToolClient,
		StandardsComplianceToolClient:  standardsComplianceToolClient,
		LegalComplianceToolClient:      legalComplianceToolClient,
		RegulatoryComplianceToolClient: regulatoryComplianceToolClient,
		InternalPolicyToolClient:       internalPolicyToolClient,
		SecurityToolClient:             securityToolClient,
		PerformanceToolClient:          performanceToolClient,
		ErrorHandlingToolClient:        errorHandlingToolClient,
		LimitationsToolClient:          limitationsToolClient,

		MetricsClient:        metricsClient,
		MetricsModelName:     metricsModel,
		MetricsPromptRef:     metricsRef,
		MetricsPromptText:    metricsPrompt,
		MetricsMaxToolTurns:  metricsMaxTurns,
		MetricsMaxToolTokens: metricsMaxTokens,
		MetricsTools:         metricsToolList,
		MetricsToolClient:    metricsToolClient,

		ProvisionsClient:        provisionsClient,
		ProvisionsModelName:     provisionsModel,
		ProvisionsPromptRef:     provisionsRef,
		ProvisionsPromptText:    provisionsPrompt,
		ProvisionsMaxToolTurns:  provisionsMaxTurns,
		ProvisionsMaxToolTokens: provisionsMaxTokens,
		ProvisionsTools:         provisionsToolList,
		ProvisionsToolClient:    provisionsToolClient,

		EntitiesClient:     entitiesClient,
		EntitiesModelName:  entitiesModel,
		EntitiesPromptRef:  entitiesRef,
		EntitiesPromptText: entitiesPrompt,

		InventoryItemsClient:        inventoryItemsClient,
		InventoryItemsModelName:     inventoryItemsModel,
		InventoryItemsPromptRef:     inventoryItemsRef,
		InventoryItemsPromptText:    inventoryItemsPrompt,
		InventoryItemsMaxToolTurns:  inventoryItemsMaxTurns,
		InventoryItemsMaxToolTokens: inventoryItemsMaxTokens,
		InventoryItemsTools:         inventoryItemsToolList,
		InventoryItemsToolClient:    inventoryItemsToolClient,

		MetricsCompletenessClient:        metricsCompletenessClient,
		MetricsCompletenessModelName:     metricsCompletenessModel,
		MetricsCompletenessPromptRef:     metricsCompletenessRef,
		MetricsCompletenessPromptText:    metricsCompletenessPrompt,
		MetricsCompletenessMaxToolTurns:  metricsCompletenessMaxTurns,
		MetricsCompletenessMaxToolTokens: metricsCompletenessMaxTokens,
		MetricsCompletenessTools:         metricsCompletenessToolList,
		MetricsCompletenessToolClient:    metricsCompletenessToolClient,
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

	p.Logger.Info("document review running",
		"record_id", recordID,
		"run_id", p.RunID,
		"num_reviewers", len(reviewers),
	)

	// Delete previous findings for this run (supports restart of a crashed run).
	if _, err := p.FindingsStore.DeleteFindings(ctx, p.RunID); err != nil {
		p.Logger.Warn("failed to delete previous review findings", "run_id", p.RunID, "error", err)
	}
	if p.ReviewLogsStore != nil {
		if _, err := p.ReviewLogsStore.DeleteLogs(ctx, p.RunID); err != nil {
			p.Logger.Warn("failed to delete previous review logs", "run_id", p.RunID, "error", err)
		}
	}

	allFindings, errs := p.runReviewersPromptCacheOptimized(ctx, recordID, rec, reviewers, p.RunID)

	if len(allFindings) > 0 {
		inserted, err := p.FindingsStore.SaveFindings(ctx, recordID, p.RunID, allFindings)
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

	if p.ExamplesClient != nil && p.ExamplesPromptText != "" && p.ExamplesModelName != "" {
		runners = append(runners, reviewRunner{
			reviewer: &examplesReviewer{
				client:     p.ExamplesClient,
				logger:     p.Logger,
				chunkStore: SQLStore{DB: ApiTypes.ProjectDBHandle},
				maxTasks:   p.MaxConcurrent,
			},
			cfg: ReviewerConfig{
				Enabled:    true,
				ModelName:  p.ExamplesModelName,
				PromptText: p.ExamplesPromptText,
				PromptRef:  p.ExamplesPromptRef,
			},
		})
	}

	if p.DiagramsClient != nil && p.DiagramsPromptText != "" && p.DiagramsModelName != "" {
		runners = append(runners, reviewRunner{
			reviewer: &diagramsReviewer{
				client:     p.DiagramsClient,
				logger:     p.Logger,
				chunkStore: SQLStore{DB: ApiTypes.ProjectDBHandle},
				maxTasks:   p.MaxConcurrent,
			},
			cfg: ReviewerConfig{
				Enabled:    true,
				ModelName:  p.DiagramsModelName,
				PromptText: p.DiagramsPromptText,
				PromptRef:  p.DiagramsPromptRef,
			},
		})
	}

	if p.TestableClaimsClient != nil && p.TestableClaimsPromptText != "" && p.TestableClaimsModelName != "" {
		runners = append(runners, reviewRunner{
			reviewer: &testableClaimsReviewer{
				client:     p.TestableClaimsClient,
				logger:     p.Logger,
				chunkStore: SQLStore{DB: ApiTypes.ProjectDBHandle},
				maxTasks:   p.MaxConcurrent,
			},
			cfg: ReviewerConfig{
				Enabled:    true,
				ModelName:  p.TestableClaimsModelName,
				PromptText: p.TestableClaimsPromptText,
				PromptRef:  p.TestableClaimsPromptRef,
			},
		})
	}

	if p.EvidenceRationaleClient != nil && p.EvidenceRationalePromptText != "" && p.EvidenceRationaleModelName != "" {
		runners = append(runners, reviewRunner{
			reviewer: &evidenceRationaleReviewer{
				client:       p.EvidenceRationaleClient,
				toolClient:   p.EvidenceRationaleToolClient,
				toolRegistry: defaultToolRegistry(),
				logger:       p.Logger,
				chunkStore:   SQLStore{DB: ApiTypes.ProjectDBHandle},
				maxTasks:     p.MaxConcurrent,
			},
			cfg: ReviewerConfig{
				Enabled:       true,
				ModelName:     p.EvidenceRationaleModelName,
				PromptText:    p.EvidenceRationalePromptText,
				PromptRef:     p.EvidenceRationalePromptRef,
				MaxToolTurns:  p.EvidenceRationaleMaxToolTurns,
				MaxToolTokens: p.EvidenceRationaleMaxToolTokens,
				Tools:         p.EvidenceRationaleTools,
			},
		})
	}

	// ── P4 — Consistency ────────────────────────────────────────────

	if p.InternalContradictionsClient != nil && p.InternalContradictionsPromptText != "" && p.InternalContradictionsModelName != "" {
		runners = append(runners, reviewRunner{
			reviewer: &internalContradictionsReviewer{
				client:       p.InternalContradictionsClient,
				toolClient:   p.InternalContradictionsToolClient,
				toolRegistry: defaultToolRegistry(),
				logger:       p.Logger,
				chunkStore:   SQLStore{DB: ApiTypes.ProjectDBHandle},
				maxTasks:     p.MaxConcurrent,
			},
			cfg: ReviewerConfig{
				Enabled:       true,
				ModelName:     p.InternalContradictionsModelName,
				PromptText:    p.InternalContradictionsPromptText,
				PromptRef:     p.InternalContradictionsPromptRef,
				MaxToolTurns:  p.InternalContradictionsMaxToolTurns,
				MaxToolTokens: p.InternalContradictionsMaxToolTokens,
				Tools:         p.InternalContradictionsTools,
			},
		})
	}

	if p.TerminologyConsistencyClient != nil && p.TerminologyConsistencyPromptText != "" && p.TerminologyConsistencyModelName != "" {
		runners = append(runners, reviewRunner{
			reviewer: &terminologyConsistencyReviewer{
				client:       p.TerminologyConsistencyClient,
				toolClient:   p.TerminologyConsistencyToolClient,
				toolRegistry: defaultToolRegistry(),
				logger:       p.Logger,
				chunkStore:   SQLStore{DB: ApiTypes.ProjectDBHandle},
				maxTasks:     p.MaxConcurrent,
			},
			cfg: ReviewerConfig{
				Enabled:       true,
				ModelName:     p.TerminologyConsistencyModelName,
				PromptText:    p.TerminologyConsistencyPromptText,
				PromptRef:     p.TerminologyConsistencyPromptRef,
				MaxToolTurns:  p.TerminologyConsistencyMaxToolTurns,
				MaxToolTokens: p.TerminologyConsistencyMaxToolTokens,
				Tools:         p.TerminologyConsistencyTools,
			},
		})
	}

	if p.CrossReferenceCorrectnessClient != nil && p.CrossReferenceCorrectnessPromptText != "" && p.CrossReferenceCorrectnessModelName != "" {
		runners = append(runners, reviewRunner{
			reviewer: &crossReferenceCorrectnessReviewer{
				client:       p.CrossReferenceCorrectnessClient,
				toolClient:   p.CrossReferenceCorrectnessToolClient,
				toolRegistry: defaultToolRegistry(),
				logger:       p.Logger,
				chunkStore:   SQLStore{DB: ApiTypes.ProjectDBHandle},
				maxTasks:     p.MaxConcurrent,
			},
			cfg: ReviewerConfig{
				Enabled:       true,
				ModelName:     p.CrossReferenceCorrectnessModelName,
				PromptText:    p.CrossReferenceCorrectnessPromptText,
				PromptRef:     p.CrossReferenceCorrectnessPromptRef,
				MaxToolTurns:  p.CrossReferenceCorrectnessMaxToolTurns,
				MaxToolTokens: p.CrossReferenceCorrectnessMaxToolTokens,
				Tools:         p.CrossReferenceCorrectnessTools,
			},
		})
	}

	if p.RequirementTraceabilityClient != nil && p.RequirementTraceabilityPromptText != "" && p.RequirementTraceabilityModelName != "" {
		runners = append(runners, reviewRunner{
			reviewer: &requirementTraceabilityReviewer{
				client:       p.RequirementTraceabilityClient,
				toolClient:   p.RequirementTraceabilityToolClient,
				toolRegistry: defaultToolRegistry(),
				logger:       p.Logger,
				chunkStore:   SQLStore{DB: ApiTypes.ProjectDBHandle},
				maxTasks:     p.MaxConcurrent,
			},
			cfg: ReviewerConfig{
				Enabled:       true,
				ModelName:     p.RequirementTraceabilityModelName,
				PromptText:    p.RequirementTraceabilityPromptText,
				PromptRef:     p.RequirementTraceabilityPromptRef,
				MaxToolTurns:  p.RequirementTraceabilityMaxToolTurns,
				MaxToolTokens: p.RequirementTraceabilityMaxToolTokens,
				Tools:         p.RequirementTraceabilityTools,
			},
		})
	}

	// ── P5 — Technical & Compliance ─────────────────────────────────────────

	if p.TechnicalAccuracyClient != nil && p.TechnicalAccuracyPromptText != "" && p.TechnicalAccuracyModelName != "" {
		runners = append(runners, reviewRunner{
			reviewer: &technicalAccuracyReviewer{
				client:       p.TechnicalAccuracyClient,
				toolClient:   p.TechnicalAccuracyToolClient,
				toolRegistry: defaultToolRegistry(),
				logger:       p.Logger,
				chunkStore:   SQLStore{DB: ApiTypes.ProjectDBHandle},
				maxTasks:     p.MaxConcurrent,
			},
			cfg: ReviewerConfig{
				Enabled:       true,
				ModelName:     p.TechnicalAccuracyModelName,
				PromptText:    p.TechnicalAccuracyPromptText,
				PromptRef:     p.TechnicalAccuracyPromptRef,
				MaxToolTurns:  p.TechnicalAccuracyMaxToolTurns,
				MaxToolTokens: p.TechnicalAccuracyMaxToolTokens,
				Tools:         p.TechnicalAccuracyTools,
			},
		})
	}

	if p.AssumptionsClient != nil && p.AssumptionsPromptText != "" && p.AssumptionsModelName != "" {
		runners = append(runners, reviewRunner{
			reviewer: &assumptionsReviewer{
				client:       p.AssumptionsClient,
				toolClient:   p.AssumptionsToolClient,
				toolRegistry: defaultToolRegistry(),
				logger:       p.Logger,
				chunkStore:   SQLStore{DB: ApiTypes.ProjectDBHandle},
				maxTasks:     p.MaxConcurrent,
			},
			cfg: ReviewerConfig{
				Enabled:       true,
				ModelName:     p.AssumptionsModelName,
				PromptText:    p.AssumptionsPromptText,
				PromptRef:     p.AssumptionsPromptRef,
				MaxToolTurns:  p.AssumptionsMaxToolTurns,
				MaxToolTokens: p.AssumptionsMaxToolTokens,
				Tools:         p.AssumptionsTools,
			},
		})
	}

	if p.PrerequisitesClient != nil && p.PrerequisitesPromptText != "" && p.PrerequisitesModelName != "" {
		runners = append(runners, reviewRunner{
			reviewer: &prerequisitesReviewer{
				client:       p.PrerequisitesClient,
				toolClient:   p.PrerequisitesToolClient,
				toolRegistry: defaultToolRegistry(),
				logger:       p.Logger,
				chunkStore:   SQLStore{DB: ApiTypes.ProjectDBHandle},
				maxTasks:     p.MaxConcurrent,
			},
			cfg: ReviewerConfig{
				Enabled:       true,
				ModelName:     p.PrerequisitesModelName,
				PromptText:    p.PrerequisitesPromptText,
				PromptRef:     p.PrerequisitesPromptRef,
				MaxToolTurns:  p.PrerequisitesMaxToolTurns,
				MaxToolTokens: p.PrerequisitesMaxToolTokens,
				Tools:         p.PrerequisitesTools,
			},
		})
	}

	if p.StandardsComplianceClient != nil && p.StandardsCompliancePromptText != "" && p.StandardsComplianceModelName != "" {
		runners = append(runners, reviewRunner{
			reviewer: &standardsComplianceReviewer{
				client:       p.StandardsComplianceClient,
				toolClient:   p.StandardsComplianceToolClient,
				toolRegistry: defaultToolRegistry(),
				logger:       p.Logger,
				chunkStore:   SQLStore{DB: ApiTypes.ProjectDBHandle},
				maxTasks:     p.MaxConcurrent,
			},
			cfg: ReviewerConfig{
				Enabled:       true,
				ModelName:     p.StandardsComplianceModelName,
				PromptText:    p.StandardsCompliancePromptText,
				PromptRef:     p.StandardsCompliancePromptRef,
				MaxToolTurns:  p.StandardsComplianceMaxToolTurns,
				MaxToolTokens: p.StandardsComplianceMaxToolTokens,
				Tools:         p.StandardsComplianceTools,
			},
		})
	}

	if p.LegalComplianceClient != nil && p.LegalCompliancePromptText != "" && p.LegalComplianceModelName != "" {
		runners = append(runners, reviewRunner{
			reviewer: &legalComplianceReviewer{
				client:       p.LegalComplianceClient,
				toolClient:   p.LegalComplianceToolClient,
				toolRegistry: defaultToolRegistry(),
				logger:       p.Logger,
				chunkStore:   SQLStore{DB: ApiTypes.ProjectDBHandle},
				maxTasks:     p.MaxConcurrent,
			},
			cfg: ReviewerConfig{
				Enabled:       true,
				ModelName:     p.LegalComplianceModelName,
				PromptText:    p.LegalCompliancePromptText,
				PromptRef:     p.LegalCompliancePromptRef,
				MaxToolTurns:  p.LegalComplianceMaxToolTurns,
				MaxToolTokens: p.LegalComplianceMaxToolTokens,
				Tools:         p.LegalComplianceTools,
			},
		})
	}

	if p.RegulatoryComplianceClient != nil && p.RegulatoryCompliancePromptText != "" && p.RegulatoryComplianceModelName != "" {
		runners = append(runners, reviewRunner{
			reviewer: &regulatoryComplianceReviewer{
				client:       p.RegulatoryComplianceClient,
				toolClient:   p.RegulatoryComplianceToolClient,
				toolRegistry: defaultToolRegistry(),
				logger:       p.Logger,
				chunkStore:   SQLStore{DB: ApiTypes.ProjectDBHandle},
				maxTasks:     p.MaxConcurrent,
			},
			cfg: ReviewerConfig{
				Enabled:       true,
				ModelName:     p.RegulatoryComplianceModelName,
				PromptText:    p.RegulatoryCompliancePromptText,
				PromptRef:     p.RegulatoryCompliancePromptRef,
				MaxToolTurns:  p.RegulatoryComplianceMaxToolTurns,
				MaxToolTokens: p.RegulatoryComplianceMaxToolTokens,
				Tools:         p.RegulatoryComplianceTools,
			},
		})
	}

	if p.InternalPolicyClient != nil && p.InternalPolicyPromptText != "" && p.InternalPolicyModelName != "" {
		runners = append(runners, reviewRunner{
			reviewer: &internalPolicyReviewer{
				client:       p.InternalPolicyClient,
				toolClient:   p.InternalPolicyToolClient,
				toolRegistry: defaultToolRegistry(),
				logger:       p.Logger,
				chunkStore:   SQLStore{DB: ApiTypes.ProjectDBHandle},
				maxTasks:     p.MaxConcurrent,
			},
			cfg: ReviewerConfig{
				Enabled:       true,
				ModelName:     p.InternalPolicyModelName,
				PromptText:    p.InternalPolicyPromptText,
				PromptRef:     p.InternalPolicyPromptRef,
				MaxToolTurns:  p.InternalPolicyMaxToolTurns,
				MaxToolTokens: p.InternalPolicyMaxToolTokens,
				Tools:         p.InternalPolicyTools,
			},
		})
	}

	if p.SecurityClient != nil && p.SecurityPromptText != "" && p.SecurityModelName != "" {
		runners = append(runners, reviewRunner{
			reviewer: &securityReviewer{
				client:       p.SecurityClient,
				toolClient:   p.SecurityToolClient,
				toolRegistry: defaultToolRegistry(),
				logger:       p.Logger,
				chunkStore:   SQLStore{DB: ApiTypes.ProjectDBHandle},
				maxTasks:     p.MaxConcurrent,
			},
			cfg: ReviewerConfig{
				Enabled:       true,
				ModelName:     p.SecurityModelName,
				PromptText:    p.SecurityPromptText,
				PromptRef:     p.SecurityPromptRef,
				MaxToolTurns:  p.SecurityMaxToolTurns,
				MaxToolTokens: p.SecurityMaxToolTokens,
				Tools:         p.SecurityTools,
			},
		})
	}

	if p.PerformanceClient != nil && p.PerformancePromptText != "" && p.PerformanceModelName != "" {
		runners = append(runners, reviewRunner{
			reviewer: &performanceReviewer{
				client:       p.PerformanceClient,
				toolClient:   p.PerformanceToolClient,
				toolRegistry: defaultToolRegistry(),
				logger:       p.Logger,
				chunkStore:   SQLStore{DB: ApiTypes.ProjectDBHandle},
				maxTasks:     p.MaxConcurrent,
			},
			cfg: ReviewerConfig{
				Enabled:       true,
				ModelName:     p.PerformanceModelName,
				PromptText:    p.PerformancePromptText,
				PromptRef:     p.PerformancePromptRef,
				MaxToolTurns:  p.PerformanceMaxToolTurns,
				MaxToolTokens: p.PerformanceMaxToolTokens,
				Tools:         p.PerformanceTools,
			},
		})
	}

	if p.ErrorHandlingClient != nil && p.ErrorHandlingPromptText != "" && p.ErrorHandlingModelName != "" {
		runners = append(runners, reviewRunner{
			reviewer: &errorHandlingReviewer{
				client:       p.ErrorHandlingClient,
				toolClient:   p.ErrorHandlingToolClient,
				toolRegistry: defaultToolRegistry(),
				logger:       p.Logger,
				chunkStore:   SQLStore{DB: ApiTypes.ProjectDBHandle},
				maxTasks:     p.MaxConcurrent,
			},
			cfg: ReviewerConfig{
				Enabled:       true,
				ModelName:     p.ErrorHandlingModelName,
				PromptText:    p.ErrorHandlingPromptText,
				PromptRef:     p.ErrorHandlingPromptRef,
				MaxToolTurns:  p.ErrorHandlingMaxToolTurns,
				MaxToolTokens: p.ErrorHandlingMaxToolTokens,
				Tools:         p.ErrorHandlingTools,
			},
		})
	}

	if p.LimitationsClient != nil && p.LimitationsPromptText != "" && p.LimitationsModelName != "" {
		runners = append(runners, reviewRunner{
			reviewer: &limitationsReviewer{
				client:       p.LimitationsClient,
				toolClient:   p.LimitationsToolClient,
				toolRegistry: defaultToolRegistry(),
				logger:       p.Logger,
				chunkStore:   SQLStore{DB: ApiTypes.ProjectDBHandle},
				maxTasks:     p.MaxConcurrent,
			},
			cfg: ReviewerConfig{
				Enabled:       true,
				ModelName:     p.LimitationsModelName,
				PromptText:    p.LimitationsPromptText,
				PromptRef:     p.LimitationsPromptRef,
				MaxToolTurns:  p.LimitationsMaxToolTurns,
				MaxToolTokens: p.LimitationsMaxToolTokens,
				Tools:         p.LimitationsTools,
			},
		})
	}

	// metrics — cross-document metric consistency (ADR 2026063002). Input="artifact"
	// routes it through runReviewersLegacy -> ReviewDocument (not the chunk scheduler).
	//
	// NOTE (ADR 2026070201 AR1): a non-empty ReviewerConfig.Input set here takes
	// precedence over the `input` field in doc-review.local.toml — the scheduler
	// only consults the TOML when cfg.Input is empty. Keep the two in agreement.
	if p.MetricsClient != nil && p.MetricsPromptText != "" && p.MetricsModelName != "" {
		runners = append(runners, reviewRunner{
			reviewer: &metricsReviewer{
				client:       p.MetricsClient,
				toolClient:   p.MetricsToolClient,
				toolRegistry: defaultToolRegistry(),
				logger:       p.Logger,
				db:           ApiTypes.ProjectDBHandle,
				runID:        p.RunID,
				logStore:     p.ReviewLogsStore,
				maxTasks:     maxDocReviewerTasks(p.MaxConcurrent),
				maxMatches:   envInt("METRIC_REVIEW_MAX_MATCHES", 20, 1),
				maxMetrics:   envInt("METRIC_REVIEW_MAX_METRICS", 0, 0),
			},
			cfg: ReviewerConfig{
				Enabled:       true,
				Input:         "artifact",
				ModelName:     p.MetricsModelName,
				PromptText:    p.MetricsPromptText,
				PromptRef:     p.MetricsPromptRef,
				MaxToolTurns:  p.MetricsMaxToolTurns,
				MaxToolTokens: p.MetricsMaxToolTokens,
				Tools:         p.MetricsTools,
			},
		})
	}

	// provisions — cross-document provision consistency (ADR 2026063003). Input="artifact"
	// routes it through runReviewersLegacy -> ReviewDocument (not the chunk scheduler);
	// see the AR1 precedence note on the metrics reviewer above.
	if p.ProvisionsClient != nil && p.ProvisionsPromptText != "" && p.ProvisionsModelName != "" {
		runners = append(runners, reviewRunner{
			reviewer: &provisionsReviewer{
				client:       p.ProvisionsClient,
				toolClient:   p.ProvisionsToolClient,
				toolRegistry: defaultToolRegistry(),
				logger:       p.Logger,
				db:           ApiTypes.ProjectDBHandle,
				maxTasks:     maxDocReviewerTasks(p.MaxConcurrent),
				maxMatches:   envInt("PROVISION_REVIEW_MAX_MATCHES", 20, 1),
				maxProvision: envInt("PROVISION_REVIEW_MAX_PROVISIONS", 0, 0),
			},
			cfg: ReviewerConfig{
				Enabled:       true,
				Input:         "artifact",
				ModelName:     p.ProvisionsModelName,
				PromptText:    p.ProvisionsPromptText,
				PromptRef:     p.ProvisionsPromptRef,
				MaxToolTurns:  p.ProvisionsMaxToolTurns,
				MaxToolTokens: p.ProvisionsMaxToolTokens,
				Tools:         p.ProvisionsTools,
			},
		})
	}

	// entities — cross-document entity consistency (ADR 2026063004). Input="artifact"
	// routes it through runReviewersLegacy -> ReviewDocument (not the chunk scheduler).
	if p.EntitiesClient != nil && p.EntitiesPromptText != "" && p.EntitiesModelName != "" {
		runners = append(runners, reviewRunner{
			reviewer: &entitiesReviewer{
				client:     p.EntitiesClient,
				logger:     p.Logger,
				db:         ApiTypes.ProjectDBHandle,
				maxTasks:   maxDocReviewerTasks(p.MaxConcurrent),
				maxMatches: envInt("ENTITY_REVIEW_MAX_MATCHES", 20, 1),
				maxEntity:  envInt("ENTITY_REVIEW_MAX_ENTITIES", 0, 0),
			},
			cfg: ReviewerConfig{
				Enabled:    true,
				Input:      "artifact",
				ModelName:  p.EntitiesModelName,
				PromptText: p.EntitiesPromptText,
				PromptRef:  p.EntitiesPromptRef,
			},
		})
	}

	// inventory_items — cross-document inventory-item consistency (ADR 2026063005).
	// Input="artifact" routes it through runReviewersLegacy -> ReviewDocument;
	// see the AR1 precedence note on the metrics reviewer above.
	if p.InventoryItemsClient != nil && p.InventoryItemsPromptText != "" && p.InventoryItemsModelName != "" {
		runners = append(runners, reviewRunner{
			reviewer: &inventoryItemsReviewer{
				client:       p.InventoryItemsClient,
				toolClient:   p.InventoryItemsToolClient,
				toolRegistry: defaultToolRegistry(),
				logger:       p.Logger,
				db:           ApiTypes.ProjectDBHandle,
				maxTasks:     maxDocReviewerTasks(p.MaxConcurrent),
				maxMatches:   envInt("INVENTORY_REVIEW_MAX_MATCHES", 20, 1),
				maxItems:     envInt("INVENTORY_REVIEW_MAX_ITEMS", 0, 0),
			},
			cfg: ReviewerConfig{
				Enabled:       true,
				Input:         "artifact",
				ModelName:     p.InventoryItemsModelName,
				PromptText:    p.InventoryItemsPromptText,
				PromptRef:     p.InventoryItemsPromptRef,
				MaxToolTurns:  p.InventoryItemsMaxToolTurns,
				MaxToolTokens: p.InventoryItemsMaxToolTokens,
				Tools:         p.InventoryItemsTools,
			},
		})
	}

	// metrics_completeness — object-anchored missing-metric detection
	// (ADR 2026070201 AR6). This is a separate pass from the metrics
	// conflict reviewer; it compares the doc's object→metric set against
	// what peer documents attach to the same canonical objects. Runs as a
	// sibling aspect in the same group (P5); tool-use is enabled for the
	// mandatory search_metrics absence-verification step (AR6 §4).
	if p.MetricsCompletenessClient != nil && p.MetricsCompletenessPromptText != "" && p.MetricsCompletenessModelName != "" {
		runners = append(runners, reviewRunner{
			reviewer: &metricsCompletenessReviewer{
				client:       p.MetricsCompletenessClient,
				toolClient:   p.MetricsCompletenessToolClient,
				toolRegistry: defaultToolRegistry(),
				logger:       p.Logger,
				db:           ApiTypes.ProjectDBHandle,
				maxTasks:     maxDocReviewerTasks(p.MaxConcurrent),
				maxObjects:   envInt("METRIC_COMPLETENESS_REVIEW_MAX_OBJECTS", 0, 0),
			},
			cfg: ReviewerConfig{
				Enabled:       true,
				Input:         "artifact",
				ModelName:     p.MetricsCompletenessModelName,
				PromptText:    p.MetricsCompletenessPromptText,
				PromptRef:     p.MetricsCompletenessPromptRef,
				MaxToolTurns:  p.MetricsCompletenessMaxToolTurns,
				MaxToolTokens: p.MetricsCompletenessMaxToolTokens,
				Tools:         p.MetricsCompletenessTools,
			},
		})
	}

	if len(p.RequestedAspects) == 0 {
		return runners
	}

	allowed := make(map[string]struct{}, len(p.RequestedAspects))
	for _, aspect := range p.RequestedAspects {
		aspect = strings.TrimSpace(aspect)
		if aspect == "" {
			continue
		}
		allowed[aspect] = struct{}{}
	}

	filtered := make([]reviewRunner, 0, len(runners))
	for _, runner := range runners {
		if _, ok := allowed[runner.reviewer.Name()]; ok {
			filtered = append(filtered, runner)
		}
	}

	if p.Logger != nil && len(filtered) != len(runners) {
		p.Logger.Info("document review filtered reviewers by request aspects",
			"run_id", p.RunID,
			"requested_aspects", len(allowed),
			"selected_reviewers", len(filtered),
			"configured_reviewers", len(runners),
		)
	}

	return filtered
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
			Aspect:            strings.TrimSpace(asString(m["aspect"])),
			Severity:          strings.TrimSpace(asString(m["severity"])),
			FindingType:       strings.TrimSpace(asString(m["finding_type"])),
			Language:          normalizeReviewFindingLanguage(asString(m["language"])),
			Title:             strings.TrimSpace(asString(m["title"])),
			Description:       strings.TrimSpace(asString(m["description"])),
			Evidence:          strings.TrimSpace(asString(m["evidence"])),
			Location:          strings.TrimSpace(asString(m["location"])),
			Suggestion:        strings.TrimSpace(asString(m["suggestion"])),
			Confidence:        asFloat64(m["confidence"]),
			RelatedArtifactID: strings.TrimSpace(asString(m["related_artifact_id"])),
			RelatedRecordID:   int64(asFloat64(m["related_record_id"])),
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

func (p *ReviewProcessor) makeProgressReporter(runID int64, aspect string) ReviewerProgressFunc {
	if p.StatusStore == nil || runID <= 0 || aspect == "" {
		return nil
	}
	return func(snapshot ReviewerProgress) {
		if err := p.StatusStore.UpdateAspectProgress(context.Background(), runID, aspect, snapshot.Progress, snapshot.FindingCount); err != nil {
			p.Logger.Warn("reviewer progress update failed",
				"run_id", runID,
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
