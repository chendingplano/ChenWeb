package docprocessing

import (
	"context"
	"fmt"
	"os"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
)

// grammarSpellingReviewer checks grammar, spelling, and basic language quality.
// P1, StrategyChunk, one-shot (no tool use). Uses a cheap model.
type grammarSpellingReviewer struct {
	client   LLMJSONExtractor
	logger   ApiTypes.JimoLogger
	chunkStore SQLStore // for loading chunks
	maxTasks   int
}

func (r *grammarSpellingReviewer) Name() string      { return "grammar_spelling" }
func (r *grammarSpellingReviewer) Group() string     { return "P1" }
func (r *grammarSpellingReviewer) Strategy() ReviewStrategy { return StrategyChunk }

func (r *grammarSpellingReviewer) ReviewDocument(
	ctx context.Context,
	recordID int64,
	cfg ReviewerConfig,
) ([]ReviewFinding, error) {
	// Load the record to locate the line file.
	rec, err := (&DocMetadataSQLStore{DB: ApiTypes.ProjectDBHandle}).GetInputRecord(ctx, recordID)
	if err != nil {
		return nil, fmt.Errorf("(MID_26061805) load record %d: %w", recordID, err)
	}

	// Resolve line file path. Use empty event but record fields.
	lineFilePath, err := ResolveInputFilePath(
		LineFileGeneratedEvent{RecordID: recordID},
		rec.ResultFilename,
		rec.ParserName,
		rec.StagingFilename,
	)
	if err != nil {
		return nil, fmt.Errorf("(MID_26061806) resolve line file for record %d: %w", recordID, err)
	}

	body, err := os.ReadFile(lineFilePath)
	if err != nil {
		return nil, fmt.Errorf("(MID_26061807) read line file %s: %w", lineFilePath, err)
	}

	lines, err := ParseInputLinesIncludingTOC(body)
	if err != nil {
		return nil, fmt.Errorf("(MID_26061808) parse line file for record %d: %w", recordID, err)
	}

	if len(lines) == 0 {
		r.logger.Info("grammar review skipped: no lines", "record_id", recordID)
		return nil, nil
	}

	// Build document context for the envelope.
	docCtx := buildDocContextLine(rec)

	// Split lines into windows for the LLM. Use a fixed window of 100 lines
	// per call — grammar checks are local and cheap.
	const windowSize = 100
	windows := buildGrammarWindows(lines, docCtx, windowSize)

	if len(windows) == 0 {
		return nil, nil
	}

	results, runErr := runConcurrent(ctx, r.maxTasks, len(windows),
		func(workerCtx context.Context, i int) ([]ReviewFinding, error) {
			if isCtxStopped(workerCtx) {
				return nil, ErrPipelineStopped
			}
			return r.processWindow(workerCtx, recordID, i, cfg, windows[i]), nil
		},
	)
	if runErr != nil {
		if isCtxStopped(ctx) {
			return nil, ErrPipelineStopped
		}
		return nil, runErr
	}

	var allFindings []ReviewFinding
	for _, wf := range results {
		allFindings = append(allFindings, wf...)
	}
	return allFindings, nil
}

// grammarWindow holds the lines JSON and page range for one LLM call.
type grammarWindow struct {
	inputJSON string
	startLine int
	endLine   int
}

// buildGrammarWindows splits lines into fixed-size windows, wrapping each in
// the doc_context envelope.
func buildGrammarWindows(lines []Line, docCtx string, size int) []grammarWindow {
	var windows []grammarWindow
	for i := 0; i < len(lines); i += size {
		end := i + size
		if end > len(lines) {
			end = len(lines)
		}
		slice := lines[i:end]
		objs := rawLinesToJSON(slice) // from input_lines.go
		jsonText := wrapLinesWithDocContext(objs, docCtx)

		startLine := slice[0].LineNo
		endLine := slice[len(slice)-1].LineNo

		windows = append(windows, grammarWindow{
			inputJSON: jsonText,
			startLine: startLine,
			endLine:   endLine,
		})
	}
	return windows
}

func (r *grammarSpellingReviewer) processWindow(
	ctx context.Context,
	recordID int64,
	index int,
	cfg ReviewerConfig,
	w grammarWindow,
) []ReviewFinding {
	r.logger.Info("grammar review window start",
		"record_id", recordID,
		"window", index,
		"lines", fmt.Sprintf("%d-%d", w.startLine, w.endLine),
	)

	payload, err := r.client.ExtractJSON(ctx, llmclients.JSONExtractionInput{
		PromptText: cfg.PromptText,
		ModelName:  cfg.ModelName,
		InputText:  w.inputJSON,
	})
	if err != nil {
		r.logger.Warn("grammar review window failed; skipping",
			"record_id", recordID,
			"window", index,
			"error", err,
		)
		return nil
	}

	findings := normalizeFindingsJSON(payload)
	for i := range findings {
		findings[i].Pass = "P1"
		findings[i].Aspect = "grammar_spelling"
		if findings[i].FindingType == "" {
			findings[i].FindingType = "issue"
		}
		if findings[i].Severity == "" {
			findings[i].Severity = "low"
		}
		// Enrich location with line range if not set by the LLM.
		if findings[i].Location == "" {
			findings[i].Location = fmt.Sprintf("%d-%d", w.startLine, w.endLine)
		}
	}

	r.logger.Info("grammar review window complete",
		"record_id", recordID,
		"window", index,
		"findings", len(findings),
	)
	return findings
}
