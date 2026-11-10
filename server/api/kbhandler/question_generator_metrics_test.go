package kbhandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/labstack/echo/v4"
)

func TestQuestionGenerationMetricBatchRequest(t *testing.T) {
	req := questionRequest(t, `{"purpose":"search_metrics","metrics_per_document":3,"total_metrics":7}`)
	if err := req.normalize(); err != nil {
		t.Fatal(err)
	}
	if req.InputRecordID != 0 || req.Lang != "mixed" || *req.TotalOneHopCount != 7 {
		t.Fatalf("batch request: %+v", req)
	}
	next := questionDocumentCounts(req, questionCounts{OneHop: 6, TwoHop: 6, MultiHop: 6})
	if next != (questionCounts{OneHop: 1, TwoHop: 1, MultiHop: 1}) {
		t.Fatalf("last document exceeded targets: %+v", next)
	}
	single := questionRequest(t, `{"input_record_id":416,"purpose":"search_metrics","metrics_per_document":1,"total_metrics":-99}`)
	if err := single.normalize(); err != nil {
		t.Fatalf("single did not ignore totals: %v", err)
	}
	if *single.TotalOneHopCount != 1 || *single.TotalTwoHopCount != 1 || single.Lang != "mixed" {
		t.Fatalf("totals not overridden: %+v", single)
	}
	for _, body := range []string{
		`{"purpose":"other"}`, `{"input_record_id":-1,"purpose":"search_metrics"}`,
		`{"purpose":"search_metrics","total_metrics":1001}`, `{"purpose":"search_metrics","metrics_per_document":51}`, `{"purpose":"search_metrics","metrics_per_document":-1}`, `{"purpose":"search_metrics","metrics_per_document":0}`, `{"purpose":"search_metrics","total_metrics":0}`, `{"purpose":"search_metrics","one_hop_count":3,"two_hop_count":2,"multi_hop_count":1}`,
		`{"purpose":"search_metrics","one_hop_count":0,"total_one_hop_count":1}`,
		`{"purpose":"search_metrics","total_one_hop_count":0,"total_two_hop_count":0,"total_multi_hop_count":0}`,
	} {
		r := questionRequest(t, body)
		if r.normalize() == nil {
			t.Errorf("accepted %s", body)
		}
	}
	for _, body := range []string{
		`{"purpose":"search_metrics"}`,
		`{"purpose":"search_metrics","metrics_per_document":50,"total_metrics":1000}`,
		`{"purpose":"search_metrics","one_hop_count":2,"two_hop_count":2,"multi_hop_count":2,"total_one_hop_count":7,"total_two_hop_count":7,"total_multi_hop_count":7}`,
	} {
		r := questionRequest(t, body)
		if err := r.normalize(); err != nil {
			t.Fatalf("rejected valid metric counts %s: %v", body, err)
		}
		if *r.OneHopCount != *r.TwoHopCount || *r.OneHopCount != *r.MultiHopCount || *r.MetricsPerDocument != *r.OneHopCount || *r.TotalMetrics != *r.TotalOneHopCount {
			t.Fatalf("unbalanced triplet counts: %+v", r)
		}
	}
	if prompt, err := questionPurposePrompt("search_metrics"); err != nil || prompt != "prompt-question-generator-search-metrics-v3.md" {
		t.Fatalf("purpose prompt=%q %v", prompt, err)
	}
	if _, err := questionPurposePrompt("free text"); err == nil {
		t.Fatal("unknown purpose has no dedicated prompt")
	}
}

func TestQuestionGenerationMetricCountContract(t *testing.T) {
	for _, tc := range []struct {
		name, body         string
		perDocument, total int
	}{
		{"defaults", `{"purpose":"search_metrics"}`, 10, 10},
		{"lower bounds", `{"purpose":"search_metrics","metrics_per_document":1,"total_metrics":1}`, 1, 1},
		{"upper bounds", `{"purpose":"search_metrics","metrics_per_document":50,"total_metrics":1000}`, 50, 1000},
		{"legacy balanced", `{"purpose":"search_metrics","one_hop_count":2,"two_hop_count":2,"multi_hop_count":2,"total_one_hop_count":7,"total_two_hop_count":7,"total_multi_hop_count":7}`, 2, 7},
		{"new fields take precedence", `{"purpose":"search_metrics","metrics_per_document":3,"total_metrics":8,"one_hop_count":-1,"two_hop_count":9,"multi_hop_count":0,"total_one_hop_count":-99,"total_two_hop_count":50000}`, 3, 8},
		{"single ignores totals", `{"purpose":"search_metrics","input_record_id":416,"metrics_per_document":2,"total_metrics":100000,"total_one_hop_count":-99,"total_two_hop_count":50000}`, 2, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := questionRequest(t, tc.body)
			if err := req.normalize(); err != nil {
				t.Fatal(err)
			}
			if *req.MetricsPerDocument != tc.perDocument || *req.TotalMetrics != tc.total || *req.OneHopCount != tc.perDocument || *req.TwoHopCount != tc.perDocument || *req.MultiHopCount != tc.perDocument || *req.TotalOneHopCount != tc.total || *req.TotalTwoHopCount != tc.total || *req.TotalMultiHopCount != tc.total {
				t.Fatalf("wrong normalized counts: %+v", req)
			}
		})
	}
	for _, body := range []string{
		`{"purpose":"search_metrics","total_metrics":-1}`,
		`{"purpose":"search_metrics","total_one_hop_count":1,"total_two_hop_count":2,"total_multi_hop_count":1}`,
		`{"purpose":"search_metrics","metrics_per_document":0,"one_hop_count":2,"two_hop_count":2,"multi_hop_count":2}`,
	} {
		req := questionRequest(t, body)
		if req.normalize() == nil {
			t.Errorf("accepted invalid metric counts: %s", body)
		}
	}
}

func TestQuestionGeneratorBatchSkipsAndHitsExactTargets(t *testing.T) {
	installQuestionMetadataFixture(t)
	db, mock := installPolicyDB(t)
	oldLines, oldMetrics, oldCandidates, oldClient := loadQuestionGenerationLines, loadQuestionGenerationMetrics, questionGenerationCandidates, newQuestionGenerationClient
	t.Cleanup(func() {
		loadQuestionGenerationLines, loadQuestionGenerationMetrics, questionGenerationCandidates, newQuestionGenerationClient = oldLines, oldMetrics, oldCandidates, oldClient
	})
	promptDir, err := filepath.Abs("../../../prompts")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PROMPT_DIR", promptDir)
	rounds, calls := 0, 0
	questionGenerationCandidates = func(context.Context, *sql.DB) ([]int64, error) { rounds++; return []int64{11, 22, 33}, nil }
	loadQuestionGenerationMetrics = func(_ context.Context, _ *sql.DB, id int64) ([]metricReviewInputMetric, error) {
		if id == 11 {
			return nil, nil
		}
		return []metricReviewInputMetric{{MetricID: fmt.Sprintf("%d_m1", id), SourceLineSpans: json.RawMessage(`["7"]`)}}, nil
	}
	loadQuestionGenerationLines = func(_ context.Context, _ *sql.DB, id int64) ([]docprocessing.Line, error) {
		if id == 11 {
			t.Fatal("metricless document should not read a file")
		}
		if id == 22 {
			return nil, fmt.Errorf("file unavailable")
		}
		return []docprocessing.Line{{LineNo: 7, Content: "Type A limit 20 kg; type B limit 30 kg; class C uses type A."}}, nil
	}
	newQuestionGenerationClient = func(string, ApiTypes.JimoLogger) (ApiTypes.LLMModelDef, metricJSONExtractor, error) {
		return ApiTypes.LLMModelDef{ModelName: "fixture"}, questionExtractorFunc(func(ctx context.Context, in llmclients.JSONExtractionInput) (map[string]any, error) {
			calls++
			deadline, ok := ctx.Deadline()
			if !ok || time.Until(deadline) < 50*time.Minute {
				t.Error("batch did not get its hour deadline")
			}
			var input struct {
				RecordID int64               `json:"input_record_id"`
				Sets     []metricQuestionSet `json:"sets"`
			}
			if err := json.Unmarshal([]byte(in.InputText), &input); err != nil {
				t.Fatal(err)
			}
			if input.RecordID != 33 {
				t.Errorf("called model for record %d", input.RecordID)
			}
			count := map[string]int{}
			questions := []any{}
			for _, set := range input.Sets {
				for _, target := range set.Questions {
					count[target.HopType]++
					steps := []string{}
					stepLines := [][]int{}
					for i := 0; i < map[string]int{"one_hop": 1, "two_hop": 2, "multi_hop": 3}[target.HopType]; i++ {
						steps = append(steps, fmt.Sprintf("Fact %d", i))
						stepLines = append(stepLines, []int{7})
					}
					questions = append(questions, map[string]any{"set_no": set.SetNo, "hop_type": target.HopType, "lang": target.Lang, "question": fmt.Sprintf("Question %d/%d/%s?", calls, set.SetNo, target.HopType), "answer": "20 kg", "reasoning_steps": steps, "step_line_ids": stepLines, "line_ids": []int{7}})
				}
			}
			if calls == 1 && (count["one_hop"] != 2 || count["two_hop"] != 2 || count["multi_hop"] != 2) {
				t.Errorf("first doc counts=%v", count)
			}
			if calls == 2 && (count["one_hop"] != 1 || count["two_hop"] != 1 || count["multi_hop"] != 1) {
				t.Errorf("last doc exceeded targets: %v", count)
			}
			return map[string]any{"questions": questions}, nil
		}), nil
	}
	expectTaskLLMExecution(mock, "testbed.generated_question_runs", 12)
	mock.ExpectExec("UPDATE testbed.generated_question_runs SET model_name").WithArgs(int64(12), "fixture").WillReturnResult(sqlmock.NewResult(0, 1))
	for i, n := range []int{6, 3} {
		mock.ExpectBegin()
		for j := 0; j < n; j++ {
			mock.ExpectExec("INSERT INTO testbed.generated_questions").WillReturnResult(sqlmock.NewResult(1, 1))
		}
		if i == 0 {
			mock.ExpectExec("UPDATE testbed.generated_question_runs").WithArgs(int64(12), false, 2, 2, 2).WillReturnResult(sqlmock.NewResult(0, 1))
		} else {
			mock.ExpectExec("UPDATE testbed.generated_question_runs").WithArgs(int64(12), true, 1, 1, 1).WillReturnResult(sqlmock.NewResult(0, 1))
		}
		mock.ExpectCommit()
	}
	req := questionRequest(t, `{"purpose":"search_metrics","metrics_per_document":2,"total_metrics":3}`)
	if err := req.normalize(); err != nil {
		t.Fatal(err)
	}
	runQuestionGeneration(db, 12, req, "fixture", "admin")
	if rounds != 2 || calls != 2 {
		t.Fatalf("rounds=%d calls=%d", rounds, calls)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestQuestionGeneratorBatchStopsWithoutMetrics(t *testing.T) {
	db, mock := installPolicyDB(t)
	oldMetrics, oldCandidates, oldClient := loadQuestionGenerationMetrics, questionGenerationCandidates, newQuestionGenerationClient
	t.Cleanup(func() {
		loadQuestionGenerationMetrics, questionGenerationCandidates, newQuestionGenerationClient = oldMetrics, oldCandidates, oldClient
	})
	promptDir, err := filepath.Abs("../../../prompts")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PROMPT_DIR", promptDir)
	rounds := 0
	questionGenerationCandidates = func(context.Context, *sql.DB) ([]int64, error) { rounds++; return []int64{11}, nil }
	loadQuestionGenerationMetrics = func(context.Context, *sql.DB, int64) ([]metricReviewInputMetric, error) { return nil, nil }
	newQuestionGenerationClient = func(string, ApiTypes.JimoLogger) (ApiTypes.LLMModelDef, metricJSONExtractor, error) {
		return ApiTypes.LLMModelDef{ModelName: "fixture"}, questionExtractorFunc(func(context.Context, llmclients.JSONExtractionInput) (map[string]any, error) {
			t.Fatal("metricless document called model")
			return nil, nil
		}), nil
	}
	expectTaskLLMExecution(mock, "testbed.generated_question_runs", 12)
	mock.ExpectExec("UPDATE testbed.generated_question_runs SET model_name").WithArgs(int64(12), "fixture").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE testbed.generated_question_runs").WithArgs(int64(12), "no eligible document could generate the remaining questions: document has no metrics").WillReturnResult(sqlmock.NewResult(0, 1))
	req := questionRequest(t, `{"purpose":"search_metrics"}`)
	if err := req.normalize(); err != nil {
		t.Fatal(err)
	}
	runQuestionGeneration(db, 12, req, "fixture", "admin")
	if rounds != 1 {
		t.Fatalf("retried a pass without progress: %d", rounds)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStartQuestionGenerationBatchWithoutInput(t *testing.T) {
	_, mock := installPolicyDB(t)
	t.Setenv("QUESTION_GEN_MODEL_NAME", "test-question-model")
	old := launchQuestionGeneration
	t.Cleanup(func() { launchQuestionGeneration = old })
	launched := false
	launchQuestionGeneration = func(_ *sql.DB, _ int64, req questionGenerationRequest, _, _ string) {
		launched = true
		if req.InputRecordID != 0 || *req.TotalOneHopCount != 7 {
			t.Errorf("wrong batch request: %+v", req)
		}
	}
	cols := strings.Split(questionRunColumns, ",")
	for i := range cols {
		cols[i] = strings.TrimSpace(cols[i])
	}
	mock.ExpectQuery("INSERT INTO testbed.generated_question_runs").WithArgs(int64(0), "search_metrics", "", 10, 10, 10, "mixed", "test-question-model", questionGenerationPrompt, "", 7, 7, 7, 3600).WillReturnRows(sqlmock.NewRows(cols).AddRow(12, nil, "search_metrics", "", 10, 10, 10, "mixed", "running", "", "", questionGenerationPrompt, "", time.Now(), nil, 7, 7, 7, 0, 0, 0, 0, time.Now().Add(time.Hour)))
	e := echo.New()
	rec := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"purpose":"search_metrics","total_metrics":7}`)), rec)
	if err := StartQuestionGeneration(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusAccepted || !launched {
		t.Fatalf("status=%d launched=%v body=%s", rec.Code, launched, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestQuestionGeneratorBatchRetriesDuplicateOutput(t *testing.T) {
	installQuestionMetadataFixture(t)
	for _, alwaysDuplicate := range []bool{false, true} {
		t.Run(fmt.Sprint(alwaysDuplicate), func(t *testing.T) {
			db, mock := installPolicyDB(t)
			oldLines, oldMetrics, oldCandidates, oldClient, oldRand := loadQuestionGenerationLines, loadQuestionGenerationMetrics, questionGenerationCandidates, newQuestionGenerationClient, questionGenerationRandomInt
			t.Cleanup(func() {
				loadQuestionGenerationLines, loadQuestionGenerationMetrics, questionGenerationCandidates, newQuestionGenerationClient, questionGenerationRandomInt = oldLines, oldMetrics, oldCandidates, oldClient, oldRand
			})
			promptDir, err := filepath.Abs("../../../prompts")
			if err != nil {
				t.Fatal(err)
			}
			t.Setenv("PROMPT_DIR", promptDir)
			questionGenerationRandomInt = func(int) int { return 0 }
			questionGenerationCandidates = func(context.Context, *sql.DB) ([]int64, error) { return []int64{33}, nil }
			loadQuestionGenerationLines = func(context.Context, *sql.DB, int64) ([]docprocessing.Line, error) {
				return []docprocessing.Line{{LineNo: 7, Content: "Limit 20 kg"}}, nil
			}
			loadQuestionGenerationMetrics = func(context.Context, *sql.DB, int64) ([]metricReviewInputMetric, error) {
				return []metricReviewInputMetric{{MetricID: "m1", SourceLineSpans: json.RawMessage(`["7"]`)}}, nil
			}
			calls := 0
			newQuestionGenerationClient = func(string, ApiTypes.JimoLogger) (ApiTypes.LLMModelDef, metricJSONExtractor, error) {
				return ApiTypes.LLMModelDef{ModelName: "fixture"}, questionExtractorFunc(func(_ context.Context, in llmclients.JSONExtractionInput) (map[string]any, error) {
					calls++
					var input struct {
						Previous []string `json:"previous_questions"`
					}
					if err := json.Unmarshal([]byte(in.InputText), &input); err != nil {
						t.Fatal(err)
					}
					if calls > 1 && (len(input.Previous) != 3 || input.Previous[0] != "What is the limit? one_hop") {
						t.Errorf("lost prior question context: %+v", input)
					}
					question := "What is the limit?"
					if calls >= 3 && !alwaysDuplicate {
						question = "Which weight must not be exceeded?"
					}
					questions := []any{}
					for i, hop := range []string{"one_hop", "two_hop", "multi_hop"} {
						steps, ids := []string{}, [][]int{}
						for step := 0; step <= i; step++ {
							steps = append(steps, "Read limit")
							ids = append(ids, []int{7})
						}
						questions = append(questions, map[string]any{"set_no": 1, "lang": "en", "hop_type": hop, "question": question + " " + hop, "answer": "20 kg", "reasoning_steps": steps, "step_line_ids": ids, "line_ids": []int{7}})
					}
					return map[string]any{"questions": questions}, nil
				}), nil
			}
			expectTaskLLMExecution(mock, "testbed.generated_question_runs", 12)
			mock.ExpectExec("UPDATE testbed.generated_question_runs SET model_name").WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectBegin()
			for range 3 {
				mock.ExpectExec("INSERT INTO testbed.generated_questions").WillReturnResult(sqlmock.NewResult(1, 1))
			}
			mock.ExpectExec("UPDATE testbed.generated_question_runs").WithArgs(int64(12), false, 1, 1, 1).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit()
			if alwaysDuplicate {
				mock.ExpectExec("UPDATE testbed.generated_question_runs").WithArgs(int64(12), "no eligible document could generate the remaining questions: model repeated a previously generated question").WillReturnResult(sqlmock.NewResult(0, 1))
			} else {
				mock.ExpectBegin()
				for range 3 {
					mock.ExpectExec("INSERT INTO testbed.generated_questions").WillReturnResult(sqlmock.NewResult(1, 1))
				}
				mock.ExpectExec("UPDATE testbed.generated_question_runs").WithArgs(int64(12), true, 1, 1, 1).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
			}
			req := questionRequest(t, `{"purpose":"search_metrics","metrics_per_document":1,"total_metrics":2}`)
			if err := req.normalize(); err != nil {
				t.Fatal(err)
			}
			runQuestionGeneration(db, 12, req, "fixture", "admin")
			wantCalls := 3
			if alwaysDuplicate {
				wantCalls = 4
			}
			if calls != wantCalls {
				t.Fatalf("calls=%d want %d", calls, wantCalls)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestMetricQuestionSetsAndLanguages(t *testing.T) {
	original := questionGenerationRandomInt
	t.Cleanup(func() { questionGenerationRandomInt = original })
	counter := 0
	questionGenerationRandomInt = func(n int) int { v := (counter / 2) % n; counter++; return v }
	metrics := []metricReviewInputMetric{{MetricID: "m1", MetricName: "Weight", SourceLineSpans: json.RawMessage(`["2"]`)}, {MetricID: "m2", MetricName: "Time", SourceLineSpans: json.RawMessage(`["8"]`)}}
	sets, err := makeMetricQuestionSets(metrics, questionCounts{OneHop: 3, TwoHop: 3, MultiHop: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(sets) != 3 {
		t.Fatalf("sets=%+v", sets)
	}
	count := map[string]int{}
	langs := map[string]bool{}
	for i, set := range sets {
		if set.SetNo != i+1 || set.Metric.MetricID == "" {
			t.Fatalf("missing anchor: %+v", set)
		}
		for _, q := range set.Questions {
			count[q.HopType]++
			langs[q.Lang] = true
		}
	}
	if count["one_hop"] != 3 || count["two_hop"] != 3 || count["multi_hop"] != 3 || !langs["en"] || !langs["zh-cn"] {
		t.Fatalf("counts=%v languages=%v", count, langs)
	}
	for _, counts := range []questionCounts{{}, {OneHop: 2, TwoHop: 1, MultiHop: 1}, {OneHop: 1, TwoHop: 1}, {OneHop: -1, TwoHop: -1, MultiHop: -1}} {
		if _, err := makeMetricQuestionSets(metrics, counts); err == nil {
			t.Errorf("accepted non-triplet counts: %+v", counts)
		}
	}
	if _, err := makeMetricQuestionSets(nil, questionCounts{OneHop: 1}); err == nil {
		t.Fatal("empty metrics accepted")
	}
}

func TestMetricQuestionValidationBindsAnchorAndLanguage(t *testing.T) {
	req := questionRequest(t, `{"input_record_id":416,"purpose":"search_metrics","one_hop_count":1,"two_hop_count":0,"multi_hop_count":0}`)
	sets := []metricQuestionSet{{SetNo: 1, Metric: questionGenerationMetric{metricReviewInputMetric: metricReviewInputMetric{MetricID: "m1", SourceLineSpans: json.RawMessage(`["2"]`)}, ContextLineIDs: []int{2}}, Questions: []metricQuestionTarget{{HopType: "one_hop", Lang: "zh-cn"}}}}
	valid := `{"questions":[{"set_no":1,"lang":"zh-cn","hop_type":"one_hop","question":"限重是多少？","answer":"20公斤","reasoning_steps":["读取限重。"],"step_line_ids":[[2]],"line_ids":[2]}]}`
	decode := func(body string) map[string]any {
		var p map[string]any
		if err := json.Unmarshal([]byte(body), &p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	qs, err := validateMetricQuestions(decode(valid), req, map[int]string{2: "20 kg", 8: "other"}, sets, "hash")
	if err != nil || len(qs) != 1 || qs[0].MetricID != "m1" || qs[0].Lang != "zh-cn" || qs[0].SourceSHA256 != "hash" || !json.Valid(qs[0].MetricSnapshot) {
		t.Fatalf("questions=%+v error=%v", qs, err)
	}
	for _, mutate := range []func(map[string]any){
		func(q map[string]any) { q["lang"] = "en" },
		func(q map[string]any) { q["set_no"] = float64(2) },
		func(q map[string]any) {
			q["line_ids"] = []any{float64(8)}
			q["step_line_ids"] = []any{[]any{float64(8)}}
		},
	} {
		payload := decode(valid)
		mutate(payload["questions"].([]any)[0].(map[string]any))
		if _, err := validateMetricQuestions(payload, req, map[int]string{2: "20 kg", 8: "other"}, sets, "hash"); err == nil {
			t.Fatal("accepted wrong anchor, set or language")
		}
	}
}
