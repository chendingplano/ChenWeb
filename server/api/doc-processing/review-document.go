package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/chendingplano/shared/go/api/loggerutil"
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

// ReviewerConfig configures one reviewer for a review run.
type ReviewerConfig struct {
	Enabled      bool     // whether this reviewer runs
	ModelName    string   // resolved model name
	PromptText   string   // prompt body
	PromptRef    string   // prompt file name (for logging)
	MaxToolTurns int      // 0 = one-shot, >0 = tool-use conversation loop
	Tools        []string // tool names available (only when MaxToolTurns > 0)
}

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
	Client        LLMJSONExtractor // shared LLM client
	Logger        ApiTypes.JimoLogger
	Now           func() time.Time

	// Reviewer configurations (loaded in constructor).
	GrammarModelCfg   structureModelConfig
	GrammarModelName  string
	GrammarPromptRef  string
	GrammarPromptText string

	MaxConcurrent int // max concurrent chunk workers per chunk-based reviewer

	// ReviewRunID, when set, overrides the self-generated run id so findings are
	// persisted under a caller-supplied run identity (DR15: the DocReviewController
	// assigns the run id at request-accept time and passes it in here).
	ReviewRunID string

	// GrammarClient is a properly-configured LLM client for the grammar reviewer,
	// built from GrammarModelCfg at construction time.
	GrammarClient LLMJSONExtractor
}

// NewReviewProcessor creates a ReviewProcessor.
// Phase I loads only the grammar reviewer config.
func NewReviewProcessor(
	inputStore DocMetadataStore,
	entityStore EntityRelationStore,
	findingsStore ReviewFindingsStore,
	extractor LLMJSONExtractor,
	_ ApiTypes.JimoLogger,
) *ReviewProcessor {
	logger := loggerutil.CreateDefaultLogger("MID_26061801")

	grammarPromptText, grammarPromptRef, _, grammarPromptErr := loadProductPromptFromEnvKeys(
		[]string{"REVIEW_GRAMMAR_PROMPT"},
		"",
	)
	grammarModelRef, _, grammarModelCfg, grammarModelErr := loadModelConfigFromEnvKeys(
		[]string{"REVIEW_GRAMMAR_MODEL_NAME"},
		"MODEL_DEF_FILE",
	)
	if grammarPromptErr != nil {
		logger.Warn("grammar review prompt unavailable; grammar_spelling reviewer disabled",
			"error", grammarPromptErr)
	}
	if grammarModelErr != nil {
		logger.Warn("grammar review model config unavailable; grammar_spelling reviewer disabled",
			"error", grammarModelErr)
	}
	_ = grammarModelRef // tracked in logs; config stored in grammarModelCfg

	var grammarClient LLMJSONExtractor
	if grammarPromptErr == nil && grammarModelErr == nil {
		timeoutSec := grammarModelCfg.TimeoutSec
		if timeoutSec <= 0 {
			timeoutSec = 100
		}
		c, err := llmclients.NewOpenAIJSONClientFromConfig(llmclients.OpenAIJSONClientConfig{
			ModelName:    grammarModelCfg.ModelName,
			APIKey:       grammarModelCfg.APIKey,
			BaseURL:      grammarModelCfg.BaseURL,
			ThinkingType: grammarModelCfg.ThinkingType,
			TimeoutSec:   timeoutSec,
		}, nil)
		if err == nil {
			c.HTTPClient = &http.Client{Timeout: time.Duration(timeoutSec) * time.Second}
		}
		if err != nil {
			logger.Warn("failed to create grammar LLM client; grammar_spelling reviewer disabled", "error", err)
			grammarPromptErr = err
		} else {
			grammarClient = c
		}
	}

	return &ReviewProcessor{
		InputStore:        inputStore,
		EntityStore:       entityStore,
		FindingsStore:     findingsStore,
		Client:            extractor,
		GrammarClient:     grammarClient,
		Logger:            logger,
		Now:               time.Now,
		MaxConcurrent:     envInt("REVIEW_MAX_TASKS", 1, 1),
		GrammarModelCfg:   grammarModelCfg,
		GrammarModelName:  grammarModelCfg.ModelName,
		GrammarPromptRef:  grammarPromptRef,
		GrammarPromptText: grammarPromptText,
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

	var (
		mu          sync.Mutex
		allFindings []ReviewFinding
		wg          sync.WaitGroup
		errs        []error
	)

	for _, r := range reviewers {
		wg.Add(1)
		go func(reviewer Reviewer, cfg ReviewerConfig) {
			defer wg.Done()

			p.Logger.Info("reviewer start",
				"record_id", recordID,
				"reviewer", reviewer.Name(),
				"group", reviewer.Group(),
				"model", cfg.ModelName,
			)

			findings, err := reviewer.ReviewDocument(ctx, recordID, cfg)
			if err != nil {
				p.Logger.Error("reviewer failed",
					"record_id", recordID,
					"reviewer", reviewer.Name(),
					"error", err,
				)
				mu.Lock()
				errs = append(errs, fmt.Errorf("%s: %w", reviewer.Name(), err))
				mu.Unlock()
				return
			}

			p.Logger.Info("reviewer complete",
				"record_id", recordID,
				"reviewer", reviewer.Name(),
				"findings", len(findings),
			)

			mu.Lock()
			allFindings = append(allFindings, findings...)
			mu.Unlock()
		}(r.reviewer, r.cfg)
	}
	wg.Wait()

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
// Phase I: grammar_spelling only.
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
