package kbhandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/DATA-DOG/go-sqlmock"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

func TestQuestionDocumentMetadataOmitsEmptyFields(t *testing.T) {
	for _, raw := range []string{
		`{"doc_no":"","authors":[],"metadata":{"notes":["Keep this",null," ",{},[]],"source":{"url":null},"version":null,"keywords":[],"language":"zh","nested":{"empty":{"items":[null,""]}},"zero":0,"flag":false,"large":9007199254740993}}`,
		`{}`, `null`,
	} {
		meta := questionDocumentMetadata{InputRecordID: 753, Title: "Title", DocNo: " \t ", Metadata: json.RawMessage(raw)}
		body, err := json.Marshal(meta)
		if err != nil {
			t.Fatal(err)
		}
		var got map[string]any
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatal(err)
		}
		want := map[string]any{"input_record_id": float64(753), "title": "Title"}
		if strings.Contains(raw, "Keep this") {
			want["metadata"] = map[string]any{"metadata": map[string]any{
				"notes": []any{"Keep this"}, "language": "zh", "zero": float64(0), "flag": false, "large": float64(9007199254740993),
			}}
			if !strings.Contains(string(body), "9007199254740993") {
				t.Fatalf("metadata number lost precision: %s", body)
			}
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("metadata=%s; want %+v", body, want)
		}
	}
}

func TestMetricQuestionPromptEnvironment(t *testing.T) {
	t.Setenv("PROMPT_GENERATE_METRIC_QUESTIONS", "prompt-custom-metric-questions-v1.md")
	if got, err := questionPurposePrompt("search_metrics"); err != nil || got != "prompt-custom-metric-questions-v1.md" {
		t.Fatalf("prompt=%q error=%v", got, err)
	}
	t.Setenv("PROMPT_GENERATE_METRIC_QUESTIONS", "")
	if got, err := questionPurposePrompt("search_metrics"); err != nil || got != questionGenerationPrompt {
		t.Fatalf("default prompt=%q error=%v", got, err)
	}
}

func installQuestionMetadataFixture(t *testing.T) {
	t.Helper()
	old := loadQuestionGenerationMetadata
	t.Cleanup(func() { loadQuestionGenerationMetadata = old })
	loadQuestionGenerationMetadata = func(_ context.Context, _ *sql.DB, id int64) (questionDocumentMetadata, error) {
		return questionDocumentMetadata{InputRecordID: id, Title: "Fixture document", DocNo: "TEST-1", FileName: "fixture.pdf", PublishDate: "2026-10-24", Metadata: json.RawMessage(`{"language":"en"}`)}, nil
	}
}

func TestMetricQuestionLoadsDocumentMetadata(t *testing.T) {
	db, mock := installPolicyDB(t)
	mock.ExpectQuery(`COALESCE\(doc_metadata,'\{\}'::jsonb\) - 'drafting_persons' - 'drafting_organizations'`).WithArgs(int64(416)).WillReturnRows(sqlmock.NewRows([]string{"id", "title", "doc_no", "file_name", "publish_date", "doc_metadata"}).AddRow(416, "Document title", "DOC-416", "document.pdf", "2026-10-24", []byte(`{"authors":["Author"],"language":"zh-cn"}`)))
	meta, err := readQuestionDocumentMetadata(context.Background(), db, 416)
	if err != nil || meta.InputRecordID != 416 || meta.Title != "Document title" || meta.DocNo != "DOC-416" || meta.FileName != "document.pdf" || !strings.Contains(string(meta.Metadata), "Author") {
		t.Fatalf("metadata=%+v error=%v", meta, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMetricQuestionInputUsesSelectedContextsAndRunID(t *testing.T) {
	db, _ := installPolicyDB(t)
	oldLines, oldMetrics, oldRandom := loadQuestionGenerationLines, loadQuestionGenerationMetrics, questionGenerationRandomInt
	t.Cleanup(func() {
		loadQuestionGenerationLines, loadQuestionGenerationMetrics, questionGenerationRandomInt = oldLines, oldMetrics, oldRandom
	})
	installQuestionMetadataFixture(t)
	questionGenerationRandomInt = func(int) int { return 0 }
	loadQuestionGenerationLines = func(context.Context, *sql.DB, int64) ([]docprocessing.Line, error) {
		lines := make([]docprocessing.Line, 1100)
		for i := range lines {
			lines[i] = docprocessing.Line{LineNo: i + 1, Content: fmt.Sprintf("Source line %d", i+1)}
		}
		lines[1089].Content = strings.Repeat("unrelated source ", questionGenerationMaxInputChars)
		return lines, nil
	}
	loadQuestionGenerationMetrics = func(context.Context, *sql.DB, int64) ([]metricReviewInputMetric, error) {
		metrics := make([]metricReviewInputMetric, 50)
		for i := range metrics {
			metrics[i] = metricReviewInputMetric{MetricID: fmt.Sprintf("m%d", i+1), MetricName: fmt.Sprintf("Metric %d", i+1), SourceLineSpans: json.RawMessage(fmt.Sprintf(`["%d"]`, (i+1)*20))}
		}
		metrics = append([]metricReviewInputMetric{{MetricID: "invalid-range", SourceLineSpans: json.RawMessage(`["20:2000000"]`)}}, metrics...)
		return metrics, nil
	}
	calls := 0
	client := questionExtractorFunc(func(_ context.Context, in llmclients.JSONExtractionInput) (map[string]any, error) {
		calls++
		if in.RunID != 90042 || in.RecordID != 416 {
			t.Errorf("missing usage correlation: run=%d record=%d", in.RunID, in.RecordID)
		}
		if !strings.HasPrefix(in.InputText, `{"document_metadata":`) {
			t.Errorf("metadata is not in front: %.100s", in.InputText)
		}
		var input struct {
			DocumentMetadata questionDocumentMetadata `json:"document_metadata"`
			RunID            int64                    `json:"run_id"`
			Sets             []metricQuestionSet      `json:"sets"`
		}
		if err := json.Unmarshal([]byte(in.InputText), &input); err != nil {
			t.Fatal(err)
		}
		if input.DocumentMetadata.Title != "Fixture document" || input.DocumentMetadata.DocNo != "TEST-1" || input.RunID != 42 {
			t.Errorf("missing document/run metadata: %+v", input.DocumentMetadata)
		}
		if len(input.Sets) != 1 {
			t.Fatalf("sent %d metric entries instead of 1 requested metric", len(input.Sets))
		}
		if strings.Contains(in.InputText, `"document_metrics"`) || strings.Contains(in.InputText, `"source_lines"`) || strings.Contains(in.InputText, "unrelated source") {
			t.Fatal("sent full metric list or whole document")
		}
		ids := map[string]bool{}
		questions := []any{}
		for _, set := range input.Sets {
			if ids[set.Metric.MetricID] {
				t.Error("sampled repeated metrics despite sufficient pool")
			}
			ids[set.Metric.MetricID] = true
			if len(set.Questions) != 3 || len(set.Metric.ContextLineRanges) != 1 || set.Metric.ContextLineRanges[0] != "10:30" || set.Metric.Context == "" {
				t.Fatalf("missing per-metric context/target: %+v", set)
			}
			var spans []string
			if err := json.Unmarshal(set.Metric.SourceLineSpans, &spans); err != nil {
				t.Fatal(err)
			}
			var anchor int
			if _, err := fmt.Sscanf(spans[0], "%d", &anchor); err != nil {
				t.Fatal(err)
			}
			for _, target := range set.Questions {
				reasoning := []any{}
				for i := 0; i < map[string]int{"one_hop": 1, "two_hop": 2, "multi_hop": 3}[target.HopType]; i++ {
					reasoning = append(reasoning, map[string]any{"text": fmt.Sprintf("Read fact %d", i), "line_ids": []int{anchor}})
				}
				questions = append(questions, map[string]any{"set_no": set.SetNo, "hop_type": target.HopType, "lang": target.Lang, "question": fmt.Sprintf("Question about %s/%s?", set.Metric.MetricID, target.HopType), "answer": "A supported value", "reasoning": reasoning})
			}
		}
		return map[string]any{"questions": questions}, nil
	})
	req := questionRequest(t, `{"input_record_id":416,"purpose":"search_metrics","one_hop_count":1,"two_hop_count":1,"multi_hop_count":1}`)
	if err := req.normalize(); err != nil {
		t.Fatal(err)
	}
	logger := loggerutil.CreateDefaultLogger("20261024-267")
	defer logger.Close()
	questions, err := generateMetricQuestionDocument(docprocessing.WithLLMRunID(context.Background(), 90042), db, 42, req, ApiTypes.LLMModelDef{ModelName: "fixture"}, client, questionGenerationPrompt, "test fixture", "user", nil, logger)
	if err != nil || len(questions) != 3 || calls != 1 {
		t.Fatalf("questions=%d calls=%d error=%v", len(questions), calls, err)
	}
}

func TestMetricQuestionWorkerLoadsEnvironmentPrompt(t *testing.T) {
	dir := t.TempDir()
	name := "prompt-custom-metric-questions-v1.md"
	if err := os.WriteFile(filepath.Join(dir, name), []byte("custom prompt fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PROMPT_DIR", dir)
	t.Setenv("PROMPT_GENERATE_METRIC_QUESTIONS", name)
	// Loading through the same resolver used by the worker must honor this ref.
	ref, err := questionPurposePrompt("search_metrics")
	if err != nil {
		t.Fatal(err)
	}
	prompt, _, _, err := docprocessing.LoadPromptByRef(ref)
	if err != nil || prompt != "custom prompt fixture" {
		t.Fatalf("prompt=%q error=%v", prompt, err)
	}
}
