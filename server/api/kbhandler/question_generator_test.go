package kbhandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/labstack/echo/v4"
)

func TestQuestionGenerationClientDisablesReasoning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Thinking struct {
				Type string `json:"type"`
			} `json:"thinking"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Thinking.Type != "disabled" {
			t.Errorf("outgoing thinking.type=%q, want disabled", body.Thinking.Type)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": `{"questions":[]}`}, "finish_reason": "stop"}}})
	}))
	defer server.Close()
	modelFile := filepath.Join(t.TempDir(), "models.toml")
	profile := fmt.Sprintf("[question-reasoning-fixture]\nmodel_name='fixture-model'\nmodel_type='llm'\napi_key='test-only'\nbase_url='%s'\nthinking_type='enabled'\nmax_output_tokens=2048\ntimeout_sec=10\n", server.URL)
	if err := os.WriteFile(modelFile, []byte(profile), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MODEL_DEF_FILE", modelFile)
	logger := loggerutil.CreateDefaultLogger("20261024-852")
	defer logger.Close()
	cfg, client, err := newQuestionGenerationClient("question-reasoning-fixture", logger)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ThinkingType != "disabled" {
		t.Errorf("generator thinking_type=%q, want disabled", cfg.ThinkingType)
	}
	if cfg.MaxOutputTokens != 2048 {
		t.Errorf("max_output_tokens=%d", cfg.MaxOutputTokens)
	}
	if _, err := client.ExtractJSON(context.Background(), llmclients.JSONExtractionInput{UserID: "fixture-user", CallReason: "question_generation_search_metrics", CallLoc: "20261024-852", ModelName: cfg.ModelName, PromptName: questionGenerationPrompt, PromptText: "test fixture", InputText: "{}"}); err != nil {
		t.Fatal(err)
	}
	unchanged, err := loadModelDefByRef("question-reasoning-fixture")
	if err != nil || unchanged.ThinkingType != "enabled" {
		t.Fatalf("profile mutated: thinking_type=%q error=%v", unchanged.ThinkingType, err)
	}
}

func questionRequest(t *testing.T, body string) questionGenerationRequest {
	t.Helper()
	var req questionGenerationRequest
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatal(err)
	}
	return req
}

func TestQuestionGenerationRequest(t *testing.T) {
	req := questionRequest(t, `{"input_record_id":416,"purpose":"  search_metrics ","description":" context "}`)
	if err := req.normalize(); err != nil {
		t.Fatal(err)
	}
	if *req.OneHopCount != 10 || *req.TwoHopCount != 10 || *req.MultiHopCount != 10 || req.Lang != "mixed" || req.Purpose != "search_metrics" || req.Description != "context" {
		t.Fatalf("incorrect defaults: %+v", req)
	}
	for _, body := range []string{
		`{"purpose":"x"}`, `{"input_record_id":1}`, `{"input_record_id":1,"purpose":"x","one_hop_count":-1}`,
		`{"input_record_id":1,"purpose":"x","two_hop_count":51}`, `{"input_record_id":1,"purpose":"x","lang":"fr"}`,
		`{"input_record_id":1,"purpose":"x","one_hop_count":0,"two_hop_count":0,"multi_hop_count":0}`,
	} {
		req := questionRequest(t, body)
		if req.normalize() == nil {
			t.Errorf("accepted %s", body)
		}
	}
	req = questionRequest(t, `{"input_record_id":1,"purpose":"search_metrics","one_hop_count":0,"lang":"zh-cn"}`)
	if err := req.normalize(); err != nil || *req.OneHopCount != 0 {
		t.Fatalf("explicit zero: %+v %v", req, err)
	}
}

func TestQuestionGenerationOutput(t *testing.T) {
	req := questionRequest(t, `{"input_record_id":416,"purpose":"search_metrics","one_hop_count":1,"two_hop_count":1,"multi_hop_count":1}`)
	if err := req.normalize(); err != nil {
		t.Fatal(err)
	}
	source := map[int]string{2: "Definition A", 8: "Property B", 15: "Relationship C"}
	valid := `{"questions":[
 {"hop_type":"one_hop","question":"What is A?","answer":"Definition A","reasoning_steps":["Read definition"],"line_ids":[2],"step_line_ids":[[2]]},
 {"hop_type":"two_hop","question":"How does B apply to A?","answer":"A uses B","reasoning_steps":["Identify A","Apply B"],"line_ids":[2,8],"step_line_ids":[[2],[8]]},
 {"hop_type":"multi_hop","question":"How do A, B and C connect?","answer":"A uses B via C","reasoning_steps":["Identify A","Apply B","Connect C"],"line_ids":[2,8,15],"step_line_ids":[[2],[8],[15]]}]}`
	decode := func(body string) map[string]any {
		var p map[string]any
		if err := json.Unmarshal([]byte(body), &p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	qs, err := validateGeneratedQuestions(decode(valid), req, source)
	if err != nil {
		t.Fatal(err)
	}
	if len(qs) != 3 || qs[1].Evidence[1].Text != "Property B" {
		t.Fatalf("missing source snapshot: %+v", qs)
	}
	for name, body := range map[string]string{
		"missing questions": `{}`, "empty questions": `{"questions":[]}`,
		"unknown citation":   strings.Replace(valid, `"line_ids":[2]`, `"line_ids":[99]`, 1),
		"missing answer":     strings.Replace(valid, `"answer":"Definition A"`, `"answer":" "`, 1),
		"unknown hop":        strings.Replace(valid, `"one_hop"`, `"four_hop"`, 1),
		"duplicate question": strings.Replace(valid, `How does B apply to A?`, `what is a?`, 1),
		"missing reasoning":  strings.Replace(valid, `"Identify A","Apply B"`, `"Identify A"`, 1),
		"missing evidence":   strings.Replace(valid, `"line_ids":[2]`, `"line_ids":[]`, 1),
		"repeated evidence":  strings.Replace(valid, `"line_ids":[2,8]`, `"line_ids":[2,2]`, 1),
		"wrong counts":       strings.Replace(valid, `"two_hop"`, `"one_hop"`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := validateGeneratedQuestions(decode(body), req, source); err == nil {
				t.Fatal("accepted invalid output")
			}
		})
	}
}

func TestQuestionGenerationSource(t *testing.T) {
	lines := []docprocessing.Line{{LineNo: 2, Content: "First fact"}, {LineNo: 8, Content: "第二个事实"}}
	input, source, hash, err := buildQuestionGenerationSource(lines)
	if err != nil || !strings.Contains(input, "第二个事实") || source[8] != "第二个事实" || len(hash) != 64 {
		t.Fatalf("source: %s %v %q %v", input, source, hash, err)
	}
	_, _, hash2, err := buildQuestionGenerationSource(lines)
	if err != nil || hash != hash2 {
		t.Fatal("unstable source hash")
	}
	for _, invalid := range [][]docprocessing.Line{nil, {{LineNo: 1, Content: " "}}, {{LineNo: 1, Content: strings.Repeat("字", questionGenerationMaxInputChars+1)}}} {
		if _, _, _, err := buildQuestionGenerationSource(invalid); err == nil {
			t.Fatal("accepted empty or oversized source")
		}
	}
}

func TestSaveGeneratedQuestionsAtomic(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			db, mock := installPolicyDB(t)
			qs := []generatedQuestion{{HopType: "one_hop", Question: "Q?", Answer: "A", ReasoningSteps: []string{"Read"}, Evidence: []questionEvidence{{LineID: 2, Text: "fact"}}}}
			mock.ExpectBegin()
			insert := mock.ExpectExec("INSERT INTO testbed.generated_questions")
			if fail {
				insert.WillReturnError(fmt.Errorf("write failed"))
				mock.ExpectRollback()
			} else {
				insert.WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectExec("UPDATE testbed.generated_question_runs").WithArgs(int64(12), true, 1, 0, 0).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
			}
			err := saveGeneratedQuestions(context.Background(), db, 12, 416, qs, true)
			if (err != nil) != fail {
				t.Fatalf("error=%v fail=%v", err, fail)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestQuestionGenerationExpiredResultRollsBack(t *testing.T) {
	db, mock := installPolicyDB(t)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO testbed.generated_questions").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE testbed.generated_question_runs").WithArgs(int64(12), true, 0, 0, 0).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()
	err := saveGeneratedQuestions(context.Background(), db, 12, 416, []generatedQuestion{{Question: "Q", Answer: "A"}}, true)
	if err == nil {
		t.Fatal("late completion was accepted")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestQuestionGeneratorAdmin(t *testing.T) {
	original := EchoFactory.DefaultAuthenticator
	t.Cleanup(func() { EchoFactory.DefaultAuthenticator = original })
	installPolicyDB(t)
	for _, tc := range []struct {
		user   *ApiTypes.UserInfo
		status int
	}{
		{nil, http.StatusUnauthorized},
		{&ApiTypes.UserInfo{UserId: "reader"}, http.StatusForbidden},
		{&ApiTypes.UserInfo{UserId: "owner", IsOwner: true}, http.StatusNoContent},
	} {
		EchoFactory.DefaultAuthenticator = func(ApiTypes.RequestContext) (*ApiTypes.UserInfo, error) { return tc.user, nil }
		e := echo.New()
		rec := httptest.NewRecorder()
		c := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
		if err := QuestionGeneratorAdmin(func(c echo.Context) error { return c.NoContent(http.StatusNoContent) })(c); err != nil {
			t.Fatal(err)
		}
		if rec.Code != tc.status {
			t.Fatalf("status=%d want %d", rec.Code, tc.status)
		}
	}
}

func TestStartQuestionGenerationPersistsBeforeLaunch(t *testing.T) {
	_, mock := installPolicyDB(t)
	t.Setenv("QUESTION_GEN_MODEL_NAME", "")
	original := launchQuestionGeneration
	t.Cleanup(func() { launchQuestionGeneration = original })
	launched := false
	launchQuestionGeneration = func(_ *sql.DB, id int64, req questionGenerationRequest, model, user string) {
		launched = true
		if id != 12 || model != "deepseek-flash-processor" || user != "admin" || *req.OneHopCount != 10 {
			t.Errorf("incorrect launch: %d %s %s %+v", id, model, user, req)
		}
	}
	mock.ExpectQuery("SELECT EXISTS").WithArgs(int64(416)).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	columns := strings.Split(questionRunColumns, ",")
	for i := range columns {
		columns[i] = strings.TrimSpace(columns[i])
	}
	mock.ExpectQuery("INSERT INTO testbed.generated_question_runs").WithArgs(int64(416), "search_metrics", "context", 10, 10, 10, "mixed", "deepseek-flash-processor", questionGenerationPrompt, "admin", 10, 10, 10, 600).WillReturnRows(sqlmock.NewRows(columns).AddRow(12, 416, "search_metrics", "context", 10, 10, 10, "mixed", "running", "", "", questionGenerationPrompt, "", time.Now(), nil, 10, 10, 10, 0, 0, 0, 0, time.Now().Add(time.Hour)))
	e := echo.New()
	rec := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"input_record_id":416,"purpose":"search_metrics","description":"context"}`)), rec)
	c.Set("question-generator-user-id", "admin")
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

func TestQuestionGeneratorInvalidRequests(t *testing.T) {
	for _, body := range []string{`bad JSON`, `{}`, `{"input_record_id":416,"purpose":"x","one_hop_count":-1}`} {
		e := echo.New()
		rec := httptest.NewRecorder()
		c := e.NewContext(httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)), rec)
		if err := StartQuestionGeneration(c); err != nil {
			t.Fatal(err)
		}
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("accepted %q: %d", body, rec.Code)
		}
	}
	_, mock := installPolicyDB(t)
	mock.ExpectQuery("SELECT EXISTS").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	e := echo.New()
	rec := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"input_record_id":416,"purpose":"search_metrics"}`)), rec)
	if err := StartQuestionGeneration(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d", rec.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestQuestionGeneratorWorkerPersistsSourceFailure(t *testing.T) {
	db, mock := installPolicyDB(t)
	original, originalMetrics, originalClient := loadQuestionGenerationLines, loadQuestionGenerationMetrics, newQuestionGenerationClient
	t.Cleanup(func() {
		loadQuestionGenerationLines, loadQuestionGenerationMetrics, newQuestionGenerationClient = original, originalMetrics, originalClient
	})
	promptDir, err := filepath.Abs("../../../prompts")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PROMPT_DIR", promptDir)
	loadQuestionGenerationMetrics = func(context.Context, *sql.DB, int64) ([]metricReviewInputMetric, error) {
		return []metricReviewInputMetric{{MetricID: "m1"}}, nil
	}
	newQuestionGenerationClient = func(string, ApiTypes.JimoLogger) (ApiTypes.LLMModelDef, metricJSONExtractor, error) {
		return ApiTypes.LLMModelDef{ModelName: "fixture-model"}, nil, nil
	}
	loadQuestionGenerationLines = func(context.Context, *sql.DB, int64) ([]docprocessing.Line, error) {
		return nil, fmt.Errorf("file missing")
	}
	mock.ExpectExec("UPDATE testbed.generated_question_runs SET model_name").WithArgs(int64(12), "fixture-model").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE testbed.generated_question_runs").WithArgs(int64(12), "cannot read this record's canonical line file").WillReturnResult(sqlmock.NewResult(0, 1))
	req := questionRequest(t, `{"input_record_id":416,"purpose":"search_metrics"}`)
	if err := req.normalize(); err != nil {
		t.Fatal(err)
	}
	runQuestionGeneration(db, 12, req, "model", "admin")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

type questionExtractorFunc func(context.Context, llmclients.JSONExtractionInput) (map[string]any, error)

func (f questionExtractorFunc) ExtractJSON(ctx context.Context, in llmclients.JSONExtractionInput) (map[string]any, error) {
	return f(ctx, in)
}

func TestQuestionGeneratorWorkerModelToSavedQuestions(t *testing.T) {
	for _, modelFails := range []bool{false, true} {
		t.Run(fmt.Sprint(modelFails), func(t *testing.T) {
			db, mock := installPolicyDB(t)
			originalLines, originalClient, originalMetrics, originalRand := loadQuestionGenerationLines, newQuestionGenerationClient, loadQuestionGenerationMetrics, questionGenerationRandomInt
			t.Cleanup(func() {
				loadQuestionGenerationLines, newQuestionGenerationClient, loadQuestionGenerationMetrics, questionGenerationRandomInt = originalLines, originalClient, originalMetrics, originalRand
			})
			loadQuestionGenerationMetrics = func(context.Context, *sql.DB, int64) ([]metricReviewInputMetric, error) {
				return []metricReviewInputMetric{{MetricID: "m1", SourceLineSpans: json.RawMessage(`["7"]`)}}, nil
			}
			questionGenerationRandomInt = func(n int) int { return n - 1 }
			promptDir, err := filepath.Abs("../../../prompts")
			if err != nil {
				t.Fatal(err)
			}
			t.Setenv("PROMPT_DIR", promptDir)
			loadQuestionGenerationLines = func(context.Context, *sql.DB, int64) ([]docprocessing.Line, error) {
				return []docprocessing.Line{{LineNo: 7, Content: "The limit is 20 kg."}}, nil
			}
			newQuestionGenerationClient = func(model string, _ ApiTypes.JimoLogger) (ApiTypes.LLMModelDef, metricJSONExtractor, error) {
				if model != "fixture" {
					t.Fatalf("model=%s", model)
				}
				return ApiTypes.LLMModelDef{ModelName: "fixture-model"}, questionExtractorFunc(func(ctx context.Context, in llmclients.JSONExtractionInput) (map[string]any, error) {
					if _, ok := ctx.Deadline(); !ok {
						t.Error("worker call has no deadline")
					}
					if in.UserID != "admin" || in.PromptName != questionGenerationPrompt || in.PromptText == "" || in.ModelName != "fixture-model" {
						t.Errorf("missing call provenance: %+v", in)
					}
					var input struct {
						Purpose     string              `json:"purpose"`
						Description string              `json:"description"`
						Sets        []metricQuestionSet `json:"sets"`
						SourceLines []questionEvidence  `json:"source_lines"`
					}
					if err := json.Unmarshal([]byte(in.InputText), &input); err != nil {
						t.Fatal(err)
					}
					if input.Purpose != "search_metrics" || input.Description != "weights" || len(input.Sets) != 1 || input.Sets[0].Metric.MetricID != "m1" || input.Sets[0].Questions[0].Lang != "zh-cn" || len(input.SourceLines) != 1 || input.SourceLines[0].LineID != 7 {
						t.Errorf("wrong model input: %+v", input)
					}
					if modelFails {
						return nil, fmt.Errorf("upstream failure")
					}
					var payload map[string]any
					if err := json.Unmarshal([]byte(`{"questions":[{"set_no":1,"lang":"zh-cn","hop_type":"one_hop","question":"限重是多少？","answer":"20公斤","reasoning_steps":["读取重量上限。"],"line_ids":[7],"step_line_ids":[[7]]}]}`), &payload); err != nil {
						t.Fatal(err)
					}
					return payload, nil
				}), nil
			}
			req := questionRequest(t, `{"input_record_id":416,"purpose":"search_metrics","description":"weights","lang":"zh-cn","one_hop_count":1,"two_hop_count":0,"multi_hop_count":0}`)
			if err := req.normalize(); err != nil {
				t.Fatal(err)
			}
			mock.ExpectExec("UPDATE testbed.generated_question_runs SET model_name").WithArgs(int64(12), "fixture-model").WillReturnResult(sqlmock.NewResult(0, 1))
			if modelFails {
				mock.ExpectExec("UPDATE testbed.generated_question_runs").WithArgs(int64(12), "question generation model request failed or timed out").WillReturnResult(sqlmock.NewResult(0, 1))
			} else {
				mock.ExpectBegin()
				mock.ExpectExec("INSERT INTO testbed.generated_questions").WithArgs(int64(12), int64(416), "one_hop", "限重是多少？", "20公斤", `["读取重量上限。"]`, `[[7]]`, `[{"line_id":7,"text":"The limit is 20 kg."}]`, "m1", sqlmock.AnyArg(), 1, "zh-cn", sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectExec("UPDATE testbed.generated_question_runs").WithArgs(int64(12), true, 1, 0, 0).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
			}
			runQuestionGeneration(db, 12, req, "fixture", "admin")
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestQuestionGeneratorRunReadHandlers(t *testing.T) {
	_, mock := installPolicyDB(t)
	cols := strings.Split(questionRunColumns, ",")
	for i := range cols {
		cols[i] = strings.TrimSpace(cols[i])
	}
	mock.ExpectExec("UPDATE testbed.generated_question_runs").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT .* FROM testbed.generated_question_runs").WithArgs(int64(416)).WillReturnRows(sqlmock.NewRows(cols).AddRow(12, 416, "search", "", 1, 0, 0, "en", "done", "", "model", questionGenerationPrompt, "hash", time.Now(), time.Now(), 1, 0, 0, 1, 0, 0, 1, time.Now().Add(time.Hour)))
	e := echo.New()
	rec := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/?input_record_id=416", nil), rec)
	if err := ListQuestionGenerationRuns(c); err != nil {
		t.Fatal(err)
	}
	var listed struct {
		Runs []questionGenerationRun `json:"runs"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || len(listed.Runs) != 1 {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}
	mock.ExpectExec("UPDATE testbed.generated_question_runs").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT .* FROM testbed.generated_question_runs").WithArgs(int64(99)).WillReturnError(sql.ErrNoRows)
	rec = httptest.NewRecorder()
	c = e.NewContext(httptest.NewRequest(http.MethodGet, "/99", nil), rec)
	c.SetParamNames("id")
	c.SetParamValues("99")
	if err := GetQuestionGenerationRun(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d", rec.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
