package kbhandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/lib/pq"
)

// Questions whose targets are search registry rows (kb.search_artifacts) of any type: every purpose
// except search_metrics (openspec question-generator-artifact-types).

const (
	// A target's formatted source context may not exceed this; larger targets (mostly whole-document
	// summaries) are never sampled, so capacity counts stay exact (design D5).
	questionArtifactContextMaxChars = 20000
	questionArtifactSnippetMaxChars = 1000
)

var loadQuestionArtifactRows = readQuestionArtifactRows

// questionArtifactRow is the registry row of a target; it is also the stored artifact_snapshot.
type questionArtifactRow struct {
	ArtifactType    string          `json:"artifact_type"`
	ArtifactID      string          `json:"artifact_id"`
	PrimaryLabel    string          `json:"primary_label"`
	SecondaryLabel  string          `json:"secondary_label"`
	SnippetBasis    string          `json:"snippet_basis"`
	SourceTitle     string          `json:"source_title"`
	SourceLineSpans json.RawMessage `json:"source_line_spans"`
	SemanticPayload json.RawMessage `json:"semantic_payload"`
}

func (r questionArtifactRow) label() string { return r.ArtifactType + " " + r.ArtifactID }

// questionArtifactInput is a target as the model sees it (design D6).
type questionArtifactInput struct {
	ArtifactType      string          `json:"artifact_type"`
	ArtifactID        string          `json:"artifact_id"`
	PrimaryLabel      string          `json:"primary_label,omitempty"`
	SecondaryLabel    string          `json:"secondary_label,omitempty"`
	Snippet           string          `json:"snippet,omitempty"`
	SemanticPayload   any             `json:"semantic_payload,omitempty"`
	SourceLineSpans   json.RawMessage `json:"source_line_spans"`
	Context           string          `json:"context"`
	ContextLineRanges []string        `json:"context_line_ranges"`
}

type artifactQuestionSet struct {
	SetNo      int                   `json:"set_no"`
	Artifact   questionArtifactInput `json:"artifact"`
	Questions  []questionSlot        `json:"questions"`
	row        questionArtifactRow
	contextIDs []int
}

func readQuestionArtifactRows(ctx context.Context, db *sql.DB, recordID int64, artifactTypes []string) ([]questionArtifactRow, error) {
	rows, err := db.QueryContext(ctx, `SELECT artifact_type, artifact_id, primary_label, secondary_label, snippet_basis,
 source_title, source_line_spans, semantic_payload FROM kb.search_artifacts
 WHERE input_record_id = $1 AND artifact_type = ANY($2) ORDER BY artifact_type, artifact_id`, recordID, pq.Array(artifactTypes))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []questionArtifactRow{}
	for rows.Next() {
		var r questionArtifactRow
		var spans, payload []byte
		if err := rows.Scan(&r.ArtifactType, &r.ArtifactID, &r.PrimaryLabel, &r.SecondaryLabel, &r.SnippetBasis, &r.SourceTitle, &spans, &payload); err != nil {
			return nil, err
		}
		r.SourceLineSpans, r.SemanticPayload = json.RawMessage(spans), json.RawMessage(payload)
		out = append(out, r)
	}
	return out, rows.Err()
}

// eligibleQuestionArtifacts keeps one row per (type, ID) whose spans exist among the canonical lines
// and whose context fits questionArtifactContextMaxChars.
func eligibleQuestionArtifacts(rows []questionArtifactRow, index questionLineIndex) []questionArtifactRow {
	eligible := make([]questionArtifactRow, 0, len(rows))
	seen := map[string]bool{}
	for _, row := range rows {
		key := row.label()
		if row.ArtifactID == "" || seen[key] {
			continue
		}
		seen[key] = true
		ids, err := index.contextIDs(key, row.SourceLineSpans)
		if err != nil || index.contextChars(ids) > questionArtifactContextMaxChars {
			continue
		}
		eligible = append(eligible, row)
	}
	return eligible
}

// questionPurposeNoun names a purpose's targets in messages: "provision", or "search" for Mix.
func questionPurposeNoun(purpose string) string {
	types, _ := questionPurposeArtifactTypes(purpose)
	if len(types) == 1 {
		return types[0]
	}
	return "search"
}

func loadGroundedQuestionArtifacts(ctx context.Context, db *sql.DB, recordID int64, purpose string, logger ApiTypes.JimoLogger) ([]questionArtifactRow, []docprocessing.Line, error) {
	types, ok := questionPurposeArtifactTypes(purpose)
	if !ok {
		return nil, nil, fmt.Errorf("unsupported purpose %q", purpose)
	}
	rows, err := loadQuestionArtifactRows(ctx, db, recordID, types)
	if err != nil {
		logger.Error("load question artifacts failed", "record_id", recordID, "purpose", purpose, "error", err)
		return nil, nil, fmt.Errorf("cannot load document %s artifacts", questionPurposeNoun(purpose))
	}
	if len(rows) == 0 {
		return []questionArtifactRow{}, nil, nil
	}
	lines, err := loadQuestionGenerationLines(ctx, db, recordID)
	if err != nil {
		logger.Error("load artifact question source failed", "record_id", recordID, "error", err)
		return nil, nil, fmt.Errorf("cannot read this record's canonical line file")
	}
	index, err := newQuestionLineIndex(lines)
	if err != nil {
		return nil, nil, err
	}
	return eligibleQuestionArtifacts(rows, index), lines, nil
}

// makeArtifactQuestionSets samples n targets. Each draw first picks a type uniformly among types with
// unsampled targets, then a target of that type, so a large type (entities, relations) cannot crowd
// out small ones in a Mix run. Pools refill only when every type is used up (design D12).
func makeArtifactQuestionSets(targets []questionArtifactRow, n int) ([]artifactQuestionSet, error) {
	if len(targets) == 0 {
		return nil, fmt.Errorf("document has no eligible targets")
	}
	if n < 1 {
		return nil, fmt.Errorf("each target requires one question of each hop type")
	}
	types := []string{}
	byType := map[string][]questionArtifactRow{}
	for _, target := range targets {
		if _, ok := byType[target.ArtifactType]; !ok {
			types = append(types, target.ArtifactType)
		}
		byType[target.ArtifactType] = append(byType[target.ArtifactType], target)
	}
	pools := map[string][]int{}
	refill := func() {
		for _, t := range types {
			pools[t] = make([]int, len(byType[t]))
			for i := range pools[t] {
				pools[t][i] = i
			}
		}
	}
	sets := make([]artifactQuestionSet, 0, n)
	for range n {
		open := []string{}
		for _, t := range types {
			if len(pools[t]) > 0 {
				open = append(open, t)
			}
		}
		if len(open) == 0 {
			refill()
			open = types
		}
		t := open[questionGenerationRandomInt(len(open))]
		index := questionGenerationRandomInt(len(pools[t]))
		row := byType[t][pools[t][index]]
		pools[t] = append(pools[t][:index], pools[t][index+1:]...)
		sets = append(sets, artifactQuestionSet{SetNo: len(sets) + 1, row: row, Questions: newQuestionSlots()})
	}
	return sets, nil
}

// buildArtifactQuestionInputs fills each set's model input: labels, a short snippet, the pruned
// semantic payload and the source context.
func buildArtifactQuestionInputs(lines []docprocessing.Line, sets []artifactQuestionSet) ([]artifactQuestionSet, map[int]string, string, error) {
	targets := make([]questionContextTarget, len(sets))
	for i, set := range sets {
		targets[i] = questionContextTarget{Label: set.row.label(), Spans: set.row.SourceLineSpans}
	}
	contextIDs, contexts, source, hash, err := buildQuestionTargetContexts(lines, targets)
	if err != nil {
		return nil, nil, "", err
	}
	result := append([]artifactQuestionSet(nil), sets...)
	for i := range result {
		row := result[i].row
		snippet := strings.TrimSpace(row.SnippetBasis)
		if utf8.RuneCountInString(snippet) > questionArtifactSnippetMaxChars {
			snippet = string([]rune(snippet)[:questionArtifactSnippetMaxChars]) + "…"
		}
		var payload any
		if len(row.SemanticPayload) > 0 && json.Unmarshal(row.SemanticPayload, &payload) == nil {
			payload = pruneQuestionMetadata(payload)
		}
		result[i].contextIDs = contextIDs[i]
		result[i].Artifact = questionArtifactInput{ArtifactType: row.ArtifactType, ArtifactID: row.ArtifactID,
			PrimaryLabel: row.PrimaryLabel, SecondaryLabel: row.SecondaryLabel, Snippet: snippet, SemanticPayload: payload,
			SourceLineSpans: row.SourceLineSpans, Context: contexts[i], ContextLineRanges: questionContextLineRanges(contextIDs[i])}
	}
	return result, source, hash, nil
}

// artifactLeakNames are the names a two- or multi-hop question may not contain: the primary label,
// and an inventory item's English name. Other secondary labels are codes such as "mandatory" (D7).
func artifactLeakNames(row questionArtifactRow) []string {
	if row.ArtifactType == "inventory_item" {
		return []string{row.PrimaryLabel, row.SecondaryLabel}
	}
	return []string{row.PrimaryLabel}
}

func collectArtifactQuestions(payload map[string]any, req questionGenerationRequest, source map[int]string, sets []artifactQuestionSet, hash string, retainedSlots map[string]bool) ([]generatedQuestion, error) {
	targets := make([]questionTargetSet, len(sets))
	for i, set := range sets {
		row := set.row
		targets[i] = questionTargetSet{SetNo: set.SetNo, Slots: set.Questions, Spans: row.SourceLineSpans, ContextIDs: set.contextIDs,
			LeakNames: artifactLeakNames(row), Noun: row.ArtifactType,
			AnchorHint: fmt.Sprintf(" (source_line_spans %s); its answer must come from those lines", strings.Join(metricReviewSpansFromJSON(row.SourceLineSpans), ", ")),
			Stamp: func(q *generatedQuestion) error {
				snapshot, err := json.Marshal(row)
				if err != nil {
					return err
				}
				q.ArtifactType, q.ArtifactID, q.ArtifactSnapshot = row.ArtifactType, row.ArtifactID, snapshot
				return nil
			}}
	}
	return collectTargetQuestions(payload, req, source, targets, hash, retainedSlots)
}

func generateArtifactQuestionDocument(ctx context.Context, db *sql.DB, runID int64, req questionGenerationRequest, cfg ApiTypes.LLMModelDef, client metricJSONExtractor, promptName, prompt, userID string, previousQuestions []string, logger ApiTypes.JimoLogger, persistPartial ...func([]generatedQuestion) error) ([]generatedQuestion, error) {
	noun := questionPurposeNoun(req.Purpose)
	grounded, lines := req.preparedArtifacts, req.preparedLines
	if grounded == nil {
		var err error
		grounded, lines, err = loadGroundedQuestionArtifacts(ctx, db, req.InputRecordID, req.Purpose, logger)
		if err != nil {
			return nil, err
		}
	}
	if len(grounded) == 0 {
		if lines == nil {
			return nil, fmt.Errorf("document has no %s artifacts", noun)
		}
		return nil, fmt.Errorf("document has no %s artifacts linked to usable source lines", noun)
	}
	if req.NumQuestionsToGenerate != nil && *req.OneHopCount > len(grounded) {
		return nil, fmt.Errorf("not enough distinct targets for the confirmed question count")
	}
	sets, err := makeArtifactQuestionSets(grounded, *req.OneHopCount)
	if err != nil {
		return nil, err
	}
	sets, source, hash, err := buildArtifactQuestionInputs(lines, sets)
	if err != nil {
		return nil, err
	}
	metadata, err := loadQuestionGenerationMetadata(ctx, db, req.InputRecordID)
	if err != nil {
		logger.Error("load question document metadata failed", "record_id", req.InputRecordID, "error", err)
		return nil, fmt.Errorf("cannot load document metadata")
	}
	request := questionModelRequest{DocumentMetadata: metadata, RunID: runID, InputRecordID: req.InputRecordID,
		Purpose: req.Purpose, Description: req.Description, Sets: sets, PreviousQuestions: previousQuestions}
	return runQuestionModel(ctx, client, request, questionModelCall{RunID: runID, Noun: "target", Wanted: 3 * len(sets),
		Collect: func(payload map[string]any, retained map[string]bool) ([]generatedQuestion, error) {
			return collectArtifactQuestions(payload, req, source, sets, hash, retained)
		},
		Extraction: llmclients.JSONExtractionInput{UserID: userID, RunID: docprocessing.LLMRunIDFromContext(ctx), RecordID: req.InputRecordID, PromptName: promptName, PromptText: prompt, ModelName: cfg.ModelName, CallReason: "question_generation_" + req.Purpose, CallLoc: "20261111-742"},
	}, logger, persistPartial...)
}
