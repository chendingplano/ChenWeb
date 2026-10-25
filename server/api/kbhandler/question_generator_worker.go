package kbhandler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
)

var (
	loadQuestionGenerationLines    = docprocessing.LoadRecordLinesByID
	loadQuestionGenerationMetrics  = loadMetricReviewInputMetrics
	loadQuestionGenerationMetadata = readQuestionDocumentMetadata
	loadQuestionMetricEnglishNames = readQuestionMetricEnglishNames
	questionGenerationCandidates   = randomQuestionDocumentIDs
)

type questionDocumentMetadata struct {
	InputRecordID int64           `json:"input_record_id"`
	Title         string          `json:"title"`
	DocNo         string          `json:"doc_no"`
	FileName      string          `json:"file_name"`
	PublishDate   string          `json:"publish_date"`
	Metadata      json.RawMessage `json:"metadata"`
}

// MarshalJSON removes empty metadata only from model input, leaving stored data
// untouched. Decode numbers losslessly before traversing the nested metadata.
func (meta questionDocumentMetadata) MarshalJSON() ([]byte, error) {
	type plainMetadata questionDocumentMetadata
	raw, err := json.Marshal(plainMetadata(meta))
	if err != nil {
		return nil, err
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return json.Marshal(pruneQuestionMetadata(value))
}

func pruneQuestionMetadata(value any) any {
	switch v := value.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
	case map[string]any:
		for key, item := range v {
			if cleaned := pruneQuestionMetadata(item); cleaned != nil {
				v[key] = cleaned
			} else {
				delete(v, key)
			}
		}
		if len(v) == 0 {
			return nil
		}
	case []any:
		items := make([]any, 0, len(v))
		for _, item := range v {
			if cleaned := pruneQuestionMetadata(item); cleaned != nil {
				items = append(items, cleaned)
			}
		}
		if len(items) == 0 {
			return nil
		}
		return items
	}
	return value
}

func readQuestionDocumentMetadata(ctx context.Context, db *sql.DB, id int64) (questionDocumentMetadata, error) {
	var meta questionDocumentMetadata
	var raw []byte
	err := db.QueryRowContext(ctx, `SELECT id, COALESCE(title,''), COALESCE(doc_no,''),
 COALESCE(file_name,''), COALESCE(publish_date::text,''),
 COALESCE(doc_metadata,'{}'::jsonb) - 'drafting_persons' - 'drafting_organizations'
 FROM kb.inputs WHERE id = $1`, id).Scan(&meta.InputRecordID, &meta.Title, &meta.DocNo, &meta.FileName, &meta.PublishDate, &raw)
	meta.Metadata = json.RawMessage(raw)
	return meta, err
}

// Check span endpoints in constant time per span, without scanning the entire
// document once for every stored metric. Only sampled spans are expanded later.
func metricSourceAvailable(metric metricReviewInputMetric, index map[int]string) bool {
	spans := metricReviewSpansFromJSON(metric.SourceLineSpans)
	if len(spans) == 0 {
		return false
	}
	for _, span := range spans {
		match := metricReviewLineSpanPattern.FindStringSubmatch(span)
		if match == nil {
			return false
		}
		start, err := strconv.Atoi(match[1])
		if err != nil {
			return false
		}
		end := start
		if match[2] != "" {
			end, err = strconv.Atoi(match[2])
			if err != nil {
				return false
			}
		}
		if end < start {
			return false
		}
		if _, ok := index[start]; !ok {
			return false
		}
		if _, ok := index[end]; !ok {
			return false
		}
	}
	return true
}

func readQuestionMetricEnglishNames(ctx context.Context, db *sql.DB, recordID int64) (map[string]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT metric_id, COALESCE(metric_name_en, '') FROM kb.metrics
 WHERE input_record_id = $1 AND metric_id IS NOT NULL AND COALESCE(metric_name_en, '') <> ''`, recordID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	names := map[string]string{}
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		names[id] = name
	}
	return names, rows.Err()
}

// Sampling eligible inputs directly is equivalent to repeatedly drawing kb.inputs
// and skipping metric-free documents, without wasting a round trip per empty draw.
func randomQuestionDocumentIDs(ctx context.Context, db *sql.DB) ([]int64, error) {
	rows, err := db.QueryContext(ctx, `SELECT i.id FROM kb.inputs i WHERE EXISTS
  (SELECT 1 FROM kb.metrics m WHERE m.input_record_id = i.id AND m.ref_mention_id IS NULL) ORDER BY random()`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func generateQuestionsForRun(ctx context.Context, db *sql.DB, runID int64, req questionGenerationRequest, modelRef, userID string, logger ApiTypes.JimoLogger) error {
	cfg, client, err := newQuestionGenerationClient(modelRef, logger)
	if err != nil {
		logger.Error("configure question model failed", "run_id", runID, "error", err)
		return fmt.Errorf("cannot configure question generation model %q", modelRef)
	}
	promptName, err := questionPurposePrompt(req.Purpose)
	if err != nil {
		return err
	}
	prompt, _, _, err := docprocessing.LoadPromptByRef(promptName)
	if err != nil {
		logger.Error("load metric question prompt failed", "run_id", runID, "error", err)
		return fmt.Errorf("cannot load purpose-specific question generation prompt")
	}
	if _, err := db.ExecContext(ctx, `UPDATE testbed.generated_question_runs SET model_name = $2 WHERE id = $1 AND status = 'running' AND expires_at > NOW()`, runID, cfg.ModelName); err != nil {
		return fmt.Errorf("cannot save question model provenance")
	}
	generated := questionCounts{}
	previousQuestions := []string{}
	seenQuestions := map[string]bool{}
	duplicatePasses := 0
	var documentErrors []error
	for questionDocumentCounts(req, generated).total() > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		ids := []int64{req.InputRecordID}
		if req.InputRecordID == 0 {
			ids, err = questionGenerationCandidates(ctx, db)
			if err != nil {
				logger.Error("sample question documents failed", "run_id", runID, "error", err)
				return fmt.Errorf("cannot sample input documents")
			}
		}
		documentsDone := 0
		sawDuplicate := false
		lastFailure := "no documents with metrics are available"
		for _, recordID := range ids {
			if err := ctx.Err(); err != nil {
				return err
			}
			counts := questionDocumentCounts(req, generated)
			if counts.total() == 0 {
				return errors.Join(documentErrors...)
			}
			docReq := req
			docReq.InputRecordID = recordID
			docReq.OneHopCount, docReq.TwoHopCount, docReq.MultiHopCount = &counts.OneHop, &counts.TwoHop, &counts.MultiHop
			persisted := map[string]bool{}
			persistedQuestions := []generatedQuestion{}
			documentSaved := false
			var partialSaveErr error
			savePartial := func(accepted []generatedQuestion) error {
				pending := []generatedQuestion{}
				for _, q := range accepted {
					if !seenQuestions[strings.ToLower(strings.Join(strings.Fields(q.Question), " "))] {
						pending = append(pending, q)
					}
				}
				if len(pending) == 0 {
					return nil
				}
				partialSaveErr = saveGeneratedQuestionBatch(ctx, db, runID, recordID, pending, false, true)
				if partialSaveErr != nil {
					return partialSaveErr
				}
				documentSaved = true
				persistedQuestions = pending
				for _, q := range pending {
					persisted[fmt.Sprintf("%d/%s", q.SetNo, q.HopType)] = true
				}
				logger.Info("valid questions saved before correction", "run_id", runID, "record_id", recordID, "saved_questions", len(pending))
				return nil
			}
			questions, documentErr := generateMetricQuestionDocument(ctx, db, runID, docReq, cfg, client, promptName, prompt, userID, previousQuestions, logger, savePartial)
			if partialSaveErr != nil {
				logger.Error("save valid questions before correction failed", "run_id", runID, "record_id", recordID, "error", partialSaveErr)
				return fmt.Errorf("cannot save questions: run expired or database write failed")
			}
			if documentErr != nil && len(questions) == 0 {
				logger.Warn("question generation document skipped", "run_id", runID, "record_id", recordID, "error", documentErr)
				if req.InputRecordID > 0 {
					return documentErr
				}
				lastFailure = documentErr.Error()
				continue
			}
			unique := make([]generatedQuestion, 0, len(questions))
			for _, q := range questions {
				if persisted[fmt.Sprintf("%d/%s", q.SetNo, q.HopType)] {
					continue
				}
				if seenQuestions[strings.ToLower(strings.Join(strings.Fields(q.Question), " "))] {
					documentErr = errors.Join(documentErr, fmt.Errorf("model repeated a previously generated question"))
					continue
				}
				unique = append(unique, q)
			}
			questions = unique
			if len(questions) == 0 && !documentSaved {
				sawDuplicate = true
				lastFailure = "model repeated a previously generated question"
				logger.Warn("duplicate metric question document retried", "run_id", runID, "record_id", recordID)
				continue
			}
			if documentErr != nil {
				documentErrors = append(documentErrors, fmt.Errorf("record %d: %w", recordID, documentErr))
				logger.Warn("saving valid questions from incomplete document", "run_id", runID, "record_id", recordID, "saved_questions", len(persistedQuestions)+len(questions), "error", documentErr)
			}
			next := generated.add(counts)
			finished := questionDocumentCounts(req, next).total() == 0
			done := finished && len(documentErrors) == 0
			if len(questions) > 0 || done {
				if err := saveGeneratedQuestionBatch(ctx, db, runID, recordID, questions, done, !documentSaved); err != nil {
					logger.Error("save generated document questions failed", "run_id", runID, "record_id", recordID, "error", err)
					return fmt.Errorf("cannot save questions: run expired or database write failed")
				}
			}
			generated = next
			for _, q := range append(persistedQuestions, questions...) {
				previousQuestions = append(previousQuestions, q.Question)
				seenQuestions[strings.ToLower(strings.Join(strings.Fields(q.Question), " "))] = true
			}
			documentsDone++
			logger.Info("metric question document saved", "run_id", runID, "record_id", recordID, "saved_questions", len(persistedQuestions)+len(questions), "requested_questions", counts.total())
			if finished {
				return errors.Join(documentErrors...)
			}
		}
		if documentsDone == 0 {
			if sawDuplicate && duplicatePasses < 2 {
				duplicatePasses++
				continue
			}
			return fmt.Errorf("no eligible document could generate the remaining questions: %s", lastFailure)
		}
		duplicatePasses = 0
	}
	return errors.Join(documentErrors...)
}

func loadGroundedQuestionMetrics(ctx context.Context, db *sql.DB, recordID int64, logger ApiTypes.JimoLogger) ([]metricReviewInputMetric, []docprocessing.Line, error) {
	metrics, err := loadQuestionGenerationMetrics(ctx, db, recordID)
	if err != nil {
		logger.Error("load question metrics failed", "record_id", recordID, "error", err)
		return nil, nil, fmt.Errorf("cannot load document metrics")
	}
	if len(metrics) == 0 {
		return []metricReviewInputMetric{}, nil, nil
	}
	lines, err := loadQuestionGenerationLines(ctx, db, recordID)
	if err != nil {
		logger.Error("load metric question source failed", "record_id", recordID, "error", err)
		return nil, nil, fmt.Errorf("cannot read this record's canonical line file")
	}
	// Use the full line index only to find grounded metrics. Only sampled metrics'
	// context windows are formatted, size-checked, and sent to the model.
	lineIndex := make(map[int]string, len(lines))
	for _, line := range lines {
		if line.LineNo > 0 && strings.TrimSpace(line.Content) != "" {
			lineIndex[line.LineNo] = line.Content
		}
	}
	grounded := make([]metricReviewInputMetric, 0, len(metrics))
	seen := map[string]bool{}
	for _, metric := range metrics {
		if metric.MetricID != "" && !seen[metric.MetricID] && metricSourceAvailable(metric, lineIndex) {
			grounded = append(grounded, metric)
			seen[metric.MetricID] = true
		}
	}
	return grounded, lines, nil
}

func generateMetricQuestionDocument(ctx context.Context, db *sql.DB, runID int64, req questionGenerationRequest, cfg ApiTypes.LLMModelDef, client metricJSONExtractor, promptName, prompt, userID string, previousQuestions []string, logger ApiTypes.JimoLogger, persistPartial ...func([]generatedQuestion) error) ([]generatedQuestion, error) {
	grounded, lines := req.preparedMetrics, req.preparedLines
	if grounded == nil {
		var err error
		grounded, lines, err = loadGroundedQuestionMetrics(ctx, db, req.InputRecordID, logger)
		if err != nil {
			return nil, err
		}
	}
	if len(grounded) == 0 {
		if lines == nil {
			return nil, fmt.Errorf("document has no metrics")
		}
		return nil, fmt.Errorf("document has no metrics linked to usable source lines")
	}
	if req.NumQuestionsToGenerate != nil && *req.OneHopCount > len(grounded) {
		return nil, fmt.Errorf("not enough distinct metrics for the confirmed question count")
	}
	sets, err := makeMetricQuestionSets(grounded, questionCounts{*req.OneHopCount, *req.TwoHopCount, *req.MultiHopCount})
	if err != nil {
		return nil, err
	}
	sets, source, hash, err := buildMetricQuestionContexts(lines, sets)
	if err != nil {
		return nil, err
	}
	// English names only strengthen the name-leak check; without them the Chinese name is checked.
	if names, err := loadQuestionMetricEnglishNames(ctx, db, req.InputRecordID); err != nil {
		logger.Warn("load metric English names failed; name check uses the stored name only", "record_id", req.InputRecordID, "error", err)
	} else {
		for i := range sets {
			sets[i].Metric.MetricNameEN = names[sets[i].Metric.MetricID]
		}
	}
	metadata, err := loadQuestionGenerationMetadata(ctx, db, req.InputRecordID)
	if err != nil {
		logger.Error("load question document metadata failed", "record_id", req.InputRecordID, "error", err)
		return nil, fmt.Errorf("cannot load document metadata")
	}
	// Struct field order keeps document metadata ahead of the metric listing in
	// both the model input and the archived LLM request body.
	request := struct {
		DocumentMetadata  questionDocumentMetadata `json:"document_metadata"`
		RunID             int64                    `json:"run_id"`
		InputRecordID     int64                    `json:"input_record_id"`
		Purpose           string                   `json:"purpose"`
		Description       string                   `json:"description"`
		Sets              []metricQuestionSet      `json:"sets"`
		PreviousQuestions []string                 `json:"previous_questions"`
		ValidationError   string                   `json:"validation_error,omitempty"`
		PreviousResponse  map[string]any           `json:"previous_response,omitempty"`
	}{DocumentMetadata: metadata, RunID: runID, InputRecordID: req.InputRecordID,
		Purpose: req.Purpose, Description: req.Description, Sets: sets, PreviousQuestions: previousQuestions}
	accepted := []generatedQuestion{}
	acceptedSlots := map[string]bool{}
	acceptedText := map[string]bool{}
	wanted := 0
	for _, set := range sets {
		wanted += len(set.Questions)
	}
	var lastValidationErr error
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return accepted, errors.Join(lastValidationErr, err)
		}
		input, err := json.Marshal(request)
		if err != nil {
			return accepted, fmt.Errorf("cannot format metric question input")
		}
		if utf8.RuneCount(input) > questionGenerationMaxInputChars {
			return accepted, fmt.Errorf("document and metric context too large after formatting")
		}
		payload, err := client.ExtractJSON(ctx, llmclients.JSONExtractionInput{UserID: userID, RunID: runID, RecordID: req.InputRecordID, PromptName: promptName, PromptText: prompt, ModelName: cfg.ModelName, InputText: string(input), CallReason: "question_generation_search_metrics", CallLoc: "20261024-741"})
		if err != nil {
			logger.Error("metric question model call failed", "run_id", runID, "record_id", req.InputRecordID, "error", err)
			return accepted, errors.Join(lastValidationErr, fmt.Errorf("question generation model request failed or timed out"))
		}
		questions, validationErr := collectMetricQuestions(payload, req, source, sets, hash, acceptedSlots)
		for _, q := range questions {
			key := fmt.Sprintf("%d/%s", q.SetNo, q.HopType)
			if acceptedSlots[key] {
				continue
			}
			text := strings.ToLower(strings.Join(strings.Fields(q.Question), " "))
			if acceptedText[text] {
				validationErr = errors.Join(validationErr, fmt.Errorf("question for set %d (%s) duplicates an accepted question", q.SetNo, q.HopType))
				continue
			}
			accepted = append(accepted, q)
			acceptedSlots[key], acceptedText[text] = true, true
		}
		if len(accepted) == wanted {
			return accepted, nil
		}
		lastValidationErr = validationErr
		// Commit successful rows before a correction call can fail, time out, or be interrupted.
		if attempt == 0 && len(accepted) > 0 && len(persistPartial) > 0 {
			if err := persistPartial[0](accepted); err != nil {
				return accepted, err
			}
		}
		// Give malformed model output one correction attempt without changing its
		// sampled metrics, language targets, or source evidence. Never relax validation.
		if attempt == 1 {
			return accepted, validationErr
		}
		logger.Warn("metric question output invalid; requesting correction", "run_id", runID, "record_id", req.InputRecordID, "accepted_questions", len(accepted), "error", validationErr)
		request.ValidationError, request.PreviousResponse = validationErr.Error(), payload
	}
}
