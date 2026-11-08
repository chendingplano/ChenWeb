package kbhandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

func partialQuestionPayload() map[string]any {
	items := []any{}
	for i, hop := range []string{"one_hop", "two_hop", "multi_hop"} {
		steps, citations := []any{}, []any{}
		for j := 0; j <= i; j++ {
			steps = append(steps, fmt.Sprintf("Reason %d", j))
			citations = append(citations, []any{float64(101)})
		}
		text := []string{"What length is required?", "How far must the protective material extend?", "How should 保温层长度 be selected for this setup?"}[i]
		items = append(items, map[string]any{"set_no": float64(1), "hop_type": hop, "lang": "en", "question": text, "answer": "1 m", "reasoning_steps": steps, "step_line_ids": citations, "line_ids": []any{float64(101)}})
	}
	return map[string]any{"questions": items}
}

func installPartialQuestionFixture(t *testing.T) {
	t.Helper()
	oldMetrics, oldLines, oldRandom := loadQuestionGenerationMetrics, loadQuestionGenerationLines, questionGenerationRandomInt
	t.Cleanup(func() {
		loadQuestionGenerationMetrics, loadQuestionGenerationLines, questionGenerationRandomInt = oldMetrics, oldLines, oldRandom
	})
	installQuestionMetadataFixture(t)
	questionGenerationRandomInt = func(int) int { return 0 }
	loadQuestionGenerationMetrics = func(context.Context, *sql.DB, int64) ([]metricReviewInputMetric, error) {
		return []metricReviewInputMetric{{MetricID: "m1", MetricName: "保温层长度", SourceLineSpans: json.RawMessage(`["101"]`)}}, nil
	}
	loadQuestionGenerationLines = func(context.Context, *sql.DB, int64) ([]docprocessing.Line, error) {
		return []docprocessing.Line{{LineNo: 101, Content: "Protective material extends 1 m."}}, nil
	}
}

func TestMetricQuestionRetainsSuccessfulQuestionsAcrossAttempts(t *testing.T) {
	for _, mode := range []string{"still-invalid", "correction-regresses", "correction-duplicates-replaced-text", "correction-provider-fails", "omitted-question", "malformed-question"} {
		t.Run(mode, func(t *testing.T) {
			db, _ := installPolicyDB(t)
			installPartialQuestionFixture(t)
			calls := 0
			client := questionExtractorFunc(func(context.Context, llmclients.JSONExtractionInput) (map[string]any, error) {
				calls++
				if mode == "correction-provider-fails" && calls == 2 {
					return nil, fmt.Errorf("provider unavailable")
				}
				payload := partialQuestionPayload()
				items := payload["questions"].([]any)
				if mode == "correction-regresses" && calls == 2 {
					items[0].(map[string]any)["answer"] = ""
					items[2].(map[string]any)["question"] = "How far should protection extend after considering the installation conditions?"
				}
				if mode == "correction-duplicates-replaced-text" && calls == 2 {
					text := "How far must protection extend after considering the setup?"
					items[0].(map[string]any)["question"] = text
					items[2].(map[string]any)["question"] = text
				}
				if mode == "omitted-question" {
					payload["questions"] = items[:2]
				}
				if mode == "malformed-question" {
					items[2].(map[string]any)["line_ids"] = "not an array"
				}
				return payload, nil
			})
			req := questionRequest(t, `{"input_record_id":753,"purpose":"search_metrics","metrics_per_document":1}`)
			if err := req.normalize(); err != nil {
				t.Fatal(err)
			}
			logger := loggerutil.CreateDefaultLogger("20261025-618")
			defer logger.Close()
			questions, err := generateMetricQuestionDocument(context.Background(), db, 12, req, ApiTypes.LLMModelDef{ModelName: "fixture"}, client, questionGenerationPrompt, "fixture prompt", "admin", nil, logger)
			want := 2
			if mode == "correction-regresses" || mode == "correction-duplicates-replaced-text" {
				want = 3
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("missing rejection diagnostic")
			}
			if len(questions) != want || calls != 2 {
				t.Fatalf("saved candidates=%d want=%d calls=%d error=%v", len(questions), want, calls, err)
			}
			for _, q := range questions {
				if q.MetricID != "m1" || q.SourceSHA256 == "" {
					t.Fatalf("missing provenance: %+v", q)
				}
			}
		})
	}
}

func TestQuestionGenerationPersistsValidQuestionsWhenDocumentFails(t *testing.T) {
	db, mock := installPolicyDB(t)
	installPartialQuestionFixture(t)
	oldClient := newQuestionGenerationClient
	t.Cleanup(func() { newQuestionGenerationClient = oldClient })
	dir, err := filepath.Abs("../../../prompts")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PROMPT_DIR", dir)
	newQuestionGenerationClient = func(string, ApiTypes.JimoLogger) (ApiTypes.LLMModelDef, metricJSONExtractor, error) {
		return ApiTypes.LLMModelDef{ModelName: "fixture"}, questionExtractorFunc(func(context.Context, llmclients.JSONExtractionInput) (map[string]any, error) {
			return partialQuestionPayload(), nil
		}), nil
	}
	expectTaskLLMExecution(mock, "testbed.generated_question_runs", 12)
	mock.ExpectExec("UPDATE testbed.generated_question_runs SET model_name").WithArgs(int64(12), "fixture").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectBegin()
	for range 2 {
		mock.ExpectExec("INSERT INTO testbed.generated_questions").WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectExec("UPDATE testbed.generated_question_runs SET").WithArgs(int64(12), false, 1, 1, 0).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectExec("UPDATE testbed.generated_question_runs").WithArgs(int64(12), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	req := questionRequest(t, `{"input_record_id":753,"purpose":"search_metrics","metrics_per_document":1}`)
	if err := req.normalize(); err != nil {
		t.Fatal(err)
	}
	runQuestionGeneration(db, 12, req, "fixture", "admin")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMetricQuestionPartialValidationReportsAllRejectedQuestions(t *testing.T) {
	req := questionRequest(t, `{"input_record_id":753,"purpose":"search_metrics","metrics_per_document":1}`)
	if err := req.normalize(); err != nil {
		t.Fatal(err)
	}
	payload := partialQuestionPayload()
	payload["questions"].([]any)[0].(map[string]any)["answer"] = ""
	sets := []metricQuestionSet{{SetNo: 1, Metric: questionGenerationMetric{metricReviewInputMetric: metricReviewInputMetric{MetricID: "m1", MetricName: "保温层长度", SourceLineSpans: json.RawMessage(`["101"]`)}, ContextLineIDs: []int{101}}, Questions: []metricQuestionTarget{{HopType: "one_hop", Lang: "en"}, {HopType: "two_hop", Lang: "en"}, {HopType: "multi_hop", Lang: "en"}}}}
	questions, err := collectMetricQuestions(payload, req, map[int]string{101: "1 m"}, sets, "fixture-hash", nil)
	if len(questions) != 1 || questions[0].HopType != "two_hop" || err == nil || !strings.Contains(err.Error(), "question 1 has no answer") || !strings.Contains(err.Error(), "question 3 (multi_hop) contains its target metric name") {
		t.Fatalf("questions=%+v error=%v", questions, err)
	}
}

func TestMetricQuestionSavesBeforeCorrectionCancellation(t *testing.T) {
	db, _ := installPolicyDB(t)
	installPartialQuestionFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls, saved := 0, 0
	client := questionExtractorFunc(func(context.Context, llmclients.JSONExtractionInput) (map[string]any, error) {
		calls++
		return partialQuestionPayload(), nil
	})
	req := questionRequest(t, `{"input_record_id":753,"purpose":"search_metrics","metrics_per_document":1}`)
	if err := req.normalize(); err != nil {
		t.Fatal(err)
	}
	logger := loggerutil.CreateDefaultLogger("20261025-619")
	defer logger.Close()
	questions, err := generateMetricQuestionDocument(ctx, db, 12, req, ApiTypes.LLMModelDef{ModelName: "fixture"}, client, questionGenerationPrompt, "fixture", "admin", nil, logger, func(qs []generatedQuestion) error {
		saved = len(qs)
		cancel()
		return nil
	})
	if saved != 2 || len(questions) != 2 || calls != 1 || err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("saved=%d returned=%d calls=%d error=%v", saved, len(questions), calls, err)
	}
}

func TestQuestionGenerationCorrectionSavesOnlyNewQuestions(t *testing.T) {
	db, mock := installPolicyDB(t)
	installPartialQuestionFixture(t)
	oldClient := newQuestionGenerationClient
	t.Cleanup(func() { newQuestionGenerationClient = oldClient })
	dir, err := filepath.Abs("../../../prompts")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PROMPT_DIR", dir)
	calls := 0
	newQuestionGenerationClient = func(string, ApiTypes.JimoLogger) (ApiTypes.LLMModelDef, metricJSONExtractor, error) {
		return ApiTypes.LLMModelDef{ModelName: "fixture"}, questionExtractorFunc(func(ctx context.Context, _ llmclients.JSONExtractionInput) (map[string]any, error) {
			calls++
			payload := partialQuestionPayload()
			if calls == 2 {
				var saved int
				if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM testbed.generated_questions WHERE run_id = 12").Scan(&saved); err != nil || saved != 2 {
					t.Fatalf("before correction saved=%d error=%v", saved, err)
				}
				items := payload["questions"].([]any)
				items[0].(map[string]any)["answer"] = "" // Never overwrite the earlier valid question.
				items[2].(map[string]any)["question"] = "How far must protection extend after considering the setup?"
			}
			return payload, nil
		}), nil
	}
	expectTaskLLMExecution(mock, "testbed.generated_question_runs", 12)
	mock.ExpectExec("UPDATE testbed.generated_question_runs SET model_name").WithArgs(int64(12), "fixture").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectBegin()
	for range 2 {
		mock.ExpectExec("INSERT INTO testbed.generated_questions").WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectExec("UPDATE testbed.generated_question_runs SET.*documents_processed = documents_processed \\+ 1").WithArgs(int64(12), false, 1, 1, 0).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM testbed.generated_questions").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO testbed.generated_questions").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE testbed.generated_question_runs SET.*documents_processed = documents_processed \\+ 0").WithArgs(int64(12), true, 0, 0, 1).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	req := questionRequest(t, `{"input_record_id":753,"purpose":"search_metrics","metrics_per_document":1}`)
	if err := req.normalize(); err != nil {
		t.Fatal(err)
	}
	runQuestionGeneration(db, 12, req, "fixture", "admin")
	if calls != 2 {
		t.Fatalf("calls=%d", calls)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestQuestionGenerationBatchPreservesPartialAndLaterDocuments(t *testing.T) {
	db, mock := installPolicyDB(t)
	installPartialQuestionFixture(t)
	oldClient, oldCandidates := newQuestionGenerationClient, questionGenerationCandidates
	t.Cleanup(func() { newQuestionGenerationClient, questionGenerationCandidates = oldClient, oldCandidates })
	questionGenerationCandidates = func(context.Context, *sql.DB) ([]int64, error) { return []int64{753, 754}, nil }
	dir, err := filepath.Abs("../../../prompts")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PROMPT_DIR", dir)
	calls := 0
	newQuestionGenerationClient = func(string, ApiTypes.JimoLogger) (ApiTypes.LLMModelDef, metricJSONExtractor, error) {
		return ApiTypes.LLMModelDef{ModelName: "fixture"}, questionExtractorFunc(func(_ context.Context, in llmclients.JSONExtractionInput) (map[string]any, error) {
			calls++
			payload := partialQuestionPayload()
			if in.RecordID == 754 {
				items := payload["questions"].([]any)
				items[2].(map[string]any)["question"] = "How far must protection extend after considering the setup?"
				for _, item := range items {
					q := item.(map[string]any)
					q["question"] = q["question"].(string) + " For the second document."
				}
			}
			return payload, nil
		}), nil
	}
	expectTaskLLMExecution(mock, "testbed.generated_question_runs", 12)
	mock.ExpectExec("UPDATE testbed.generated_question_runs SET model_name").WithArgs(int64(12), "fixture").WillReturnResult(sqlmock.NewResult(0, 1))
	for _, counts := range []questionCounts{{1, 1, 0}, {1, 1, 1}} {
		mock.ExpectBegin()
		for range counts.total() {
			mock.ExpectExec("INSERT INTO testbed.generated_questions").WillReturnResult(sqlmock.NewResult(0, 1))
		}
		mock.ExpectExec("UPDATE testbed.generated_question_runs SET.*documents_processed = documents_processed \\+ 1").WithArgs(int64(12), false, counts.OneHop, counts.TwoHop, counts.MultiHop).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
	}
	mock.ExpectExec("UPDATE testbed.generated_question_runs").WithArgs(int64(12), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	req := questionRequest(t, `{"purpose":"search_metrics","metrics_per_document":1,"total_metrics":2}`)
	if err := req.normalize(); err != nil {
		t.Fatal(err)
	}
	runQuestionGeneration(db, 12, req, "fixture", "admin")
	if calls != 3 {
		t.Fatalf("calls=%d", calls)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
