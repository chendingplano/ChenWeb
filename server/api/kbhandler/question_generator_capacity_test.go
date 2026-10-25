package kbhandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/labstack/echo/v4"
)

func TestQuestionGenerationRequestedQuestionsRoundUp(t *testing.T) {
	for _, tc := range []struct{ questions, metrics int }{{1, 1}, {3, 1}, {10, 4}, {100, 34}, {151, 51}, {3000, 1000}} {
		body, _ := json.Marshal(map[string]any{"input_record_id": 753, "purpose": "search_metrics", "num_questions_to_generate": tc.questions})
		req := questionRequest(t, string(body))
		if err := req.normalize(); err != nil {
			t.Fatal(err)
		}
		if *req.OneHopCount != tc.metrics || *req.TotalOneHopCount != tc.metrics {
			t.Fatalf("%d questions: got %d metrics, want %d", tc.questions, *req.OneHopCount, tc.metrics)
		}
	}
	for _, body := range []string{
		`{"input_record_id":753,"purpose":"search_metrics","num_questions_to_generate":0}`,
		`{"input_record_id":753,"purpose":"search_metrics","num_questions_to_generate":3001}`,
		`{"purpose":"search_metrics","num_questions_to_generate":10}`,
	} {
		req := questionRequest(t, body)
		if req.normalize() == nil {
			t.Fatalf("accepted %s", body)
		}
	}
}

func TestQuestionGenerationCapacityStartsWithPreparedPool(t *testing.T) {
	for _, tc := range []struct {
		name                                   string
		available, wanted, confirmed, expected int
	}{
		{"confirmed shortage", 2, 10, 6, 2},
		{"enough metrics", 6, 10, -1, 4},
		{"more than fifty metrics", 60, 151, -1, 51},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, mock := installPolicyDB(t)
			oldMetrics, oldLines, oldLaunch := loadQuestionGenerationMetrics, loadQuestionGenerationLines, launchQuestionGeneration
			t.Cleanup(func() {
				loadQuestionGenerationMetrics, loadQuestionGenerationLines, launchQuestionGeneration = oldMetrics, oldLines, oldLaunch
			})
			loads := 0
			loadQuestionGenerationMetrics = func(context.Context, *sql.DB, int64) ([]metricReviewInputMetric, error) {
				loads++
				var metrics []metricReviewInputMetric
				for i := 0; i < tc.available; i++ {
					metrics = append(metrics, metricReviewInputMetric{MetricID: fmt.Sprintf("m%d", i), SourceLineSpans: json.RawMessage(`["7"]`)})
				}
				// Duplicate identifiers must not inflate the distinct metric capacity.
				return append(metrics, metrics[0]), nil
			}
			loadQuestionGenerationLines = func(context.Context, *sql.DB, int64) ([]docprocessing.Line, error) {
				return []docprocessing.Line{{LineNo: 7, Content: "20 kg"}}, nil
			}
			launched := false
			launchQuestionGeneration = func(_ *sql.DB, _ int64, req questionGenerationRequest, _, _ string) {
				launched = true
				if len(req.preparedMetrics) != tc.available || len(req.preparedLines) != 1 || *req.OneHopCount != tc.expected || *req.TotalOneHopCount != tc.expected {
					t.Fatalf("wrong generation plan: %+v", req)
				}
				sets, err := makeMetricQuestionSets(req.preparedMetrics, questionCounts{tc.expected, tc.expected, tc.expected})
				if err != nil {
					t.Fatal(err)
				}
				seen := map[string]bool{}
				for _, set := range sets {
					if seen[set.Metric.MetricID] {
						t.Fatal("reused a metric")
					}
					seen[set.Metric.MetricID] = true
				}
			}
			mock.ExpectQuery("SELECT EXISTS").WithArgs(int64(753)).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
			columns := strings.Split(strings.ReplaceAll(questionRunColumns, "\n", ""), ",")
			for i := range columns {
				columns[i] = strings.TrimSpace(columns[i])
			}
			mock.ExpectQuery("INSERT INTO testbed.generated_question_runs").WithArgs(int64(753), "search_metrics", "", tc.expected, tc.expected, tc.expected, "mixed", "deepseek-flash-processor", questionGenerationPrompt, "", tc.expected, tc.expected, tc.expected, 600).WillReturnRows(sqlmock.NewRows(columns).AddRow(12, 753, "search_metrics", "", tc.expected, tc.expected, tc.expected, "mixed", "running", "", "", questionGenerationPrompt, "", time.Now(), nil, tc.expected, tc.expected, tc.expected, 0, 0, 0, 0, time.Now().Add(time.Hour)))
			body := map[string]any{"input_record_id": 753, "purpose": "search_metrics", "num_questions_to_generate": tc.wanted}
			if tc.confirmed >= 0 {
				body["confirmed_available_questions"] = tc.confirmed
			}
			raw, _ := json.Marshal(body)
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(raw))), rec)
			if err := StartQuestionGeneration(c); err != nil {
				t.Fatal(err)
			}
			if rec.Code != http.StatusAccepted || !launched || loads != 1 {
				t.Fatalf("status=%d launched=%v loads=%d body=%s", rec.Code, launched, loads, rec.Body.String())
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestQuestionGenerationMissingSourceDoesNotStartRun(t *testing.T) {
	_, mock := installPolicyDB(t)
	oldMetrics, oldLines, oldLaunch := loadQuestionGenerationMetrics, loadQuestionGenerationLines, launchQuestionGeneration
	t.Cleanup(func() {
		loadQuestionGenerationMetrics, loadQuestionGenerationLines, launchQuestionGeneration = oldMetrics, oldLines, oldLaunch
	})
	loadQuestionGenerationMetrics = func(context.Context, *sql.DB, int64) ([]metricReviewInputMetric, error) {
		return []metricReviewInputMetric{{MetricID: "m1", SourceLineSpans: json.RawMessage(`["7"]`)}}, nil
	}
	loadQuestionGenerationLines = func(context.Context, *sql.DB, int64) ([]docprocessing.Line, error) {
		return nil, fmt.Errorf("source unavailable")
	}
	launchQuestionGeneration = func(*sql.DB, int64, questionGenerationRequest, string, string) {
		t.Fatal("run started without source")
	}
	mock.ExpectQuery("SELECT EXISTS").WithArgs(int64(753)).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"input_record_id":753,"purpose":"search_metrics","num_questions_to_generate":10}`)), rec)
	if err := StartQuestionGeneration(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestQuestionGenerationShortageDoesNotStartRun(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		available, confirmed int
	}{
		{"needs confirmation", 2, -1}, {"capacity changed", 1, 6}, {"capacity now sufficient", 4, 6}, {"no usable metrics", 0, -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, mock := installPolicyDB(t)
			oldMetrics, oldLines, oldLaunch := loadQuestionGenerationMetrics, loadQuestionGenerationLines, launchQuestionGeneration
			t.Cleanup(func() {
				loadQuestionGenerationMetrics, loadQuestionGenerationLines, launchQuestionGeneration = oldMetrics, oldLines, oldLaunch
			})
			loadQuestionGenerationMetrics = func(context.Context, *sql.DB, int64) ([]metricReviewInputMetric, error) {
				var metrics []metricReviewInputMetric
				for i := 0; i < tc.available; i++ {
					metrics = append(metrics, metricReviewInputMetric{MetricID: fmt.Sprintf("m%d", i), SourceLineSpans: json.RawMessage(`["7"]`)})
				}
				return append(metrics, metricReviewInputMetric{MetricID: "invalid", SourceLineSpans: json.RawMessage(`["999"]`)}), nil
			}
			loadQuestionGenerationLines = func(context.Context, *sql.DB, int64) ([]docprocessing.Line, error) {
				return []docprocessing.Line{{LineNo: 7, Content: "20 kg"}}, nil
			}
			launchQuestionGeneration = func(*sql.DB, int64, questionGenerationRequest, string, string) { t.Fatal("unconfirmed run launched") }
			mock.ExpectQuery("SELECT EXISTS").WithArgs(int64(753)).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
			body := map[string]any{"input_record_id": 753, "purpose": "search_metrics", "num_questions_to_generate": 10}
			if tc.confirmed >= 0 {
				body["confirmed_available_questions"] = tc.confirmed
			}
			raw, _ := json.Marshal(body)
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(raw))), rec)
			if err := StartQuestionGeneration(c); err != nil {
				t.Fatal(err)
			}
			if rec.Code != http.StatusConflict {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			var result struct {
				AvailableQuestions   int  `json:"available_questions"`
				ConfirmationRequired bool `json:"confirmation_required"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.AvailableQuestions != tc.available*3 || result.ConfirmationRequired != (tc.available > 0) {
				t.Fatalf("unexpected capacity: %+v", result)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
