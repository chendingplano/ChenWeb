package kbhandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

// metricQuestionGoldenInputs pins the metric model input, including a correction attempt, so the
// shared question core cannot change what search_metrics runs send (openspec
// question-generator-artifact-types, D3/D4). Regenerate only for an intended metric input change.
var metricQuestionGoldenInputs = []string{
	`{"document_metadata":{"doc_no":"TEST-1","file_name":"fixture.pdf","input_record_id":416,"metadata":{"language":"en"},"publish_date":"2026-10-24","title":"Fixture document"},"run_id":12,"input_record_id":416,"purpose":"search_metrics","description":"limits","sets":[{"set_no":1,"metric":{"metric_id":"416_mtc_2","source_line_spans":["20-21","40"],"metric_name":"保温层长度","condition":"outdoor","context":"Line content K\nLine content L\nLine content M\nLine content N\nLine content O\nLine content P\nLine content Q\nLine content R\nLine content S\nLine content T\nLine content U\nLine content V\nLine content O\nLine content P\nLine content Q\nLine content R\nLine content S\nLine content T\nLine content U\nLine content V\nLine content W\nLine content X\nLine content Y","context_line_ranges":["10:21","40:50"]},"questions":[{"hop_type":"one_hop","lang":"zh-cn"},{"hop_type":"two_hop","lang":"zh-cn"},{"hop_type":"multi_hop","lang":"zh-cn"}]},{"set_no":2,"metric":{"metric_id":"416_mtc_1","source_line_spans":["5"],"metric_name":"汞","metric_value":"≤2","metric_unit":"mg/kg","metric_name_en":"Mercury","context":"Line content B\nLine content C\nLine content D\nLine content E\nLine content F\nLine content G\nLine content H\nLine content I\nLine content J\nLine content K\nLine content L\nLine content M\nLine content N\nLine content O\nLine content P","context_line_ranges":["1:15"]},"questions":[{"hop_type":"one_hop","lang":"zh-cn"},{"hop_type":"two_hop","lang":"zh-cn"},{"hop_type":"multi_hop","lang":"zh-cn"}]}],"previous_questions":["Earlier question?"]}`,
	`{"document_metadata":{"doc_no":"TEST-1","file_name":"fixture.pdf","input_record_id":416,"metadata":{"language":"en"},"publish_date":"2026-10-24","title":"Fixture document"},"run_id":12,"input_record_id":416,"purpose":"search_metrics","description":"limits","sets":[{"set_no":1,"metric":{"metric_id":"416_mtc_2","source_line_spans":["20-21","40"],"metric_name":"保温层长度","condition":"outdoor","context":"Line content K\nLine content L\nLine content M\nLine content N\nLine content O\nLine content P\nLine content Q\nLine content R\nLine content S\nLine content T\nLine content U\nLine content V\nLine content O\nLine content P\nLine content Q\nLine content R\nLine content S\nLine content T\nLine content U\nLine content V\nLine content W\nLine content X\nLine content Y","context_line_ranges":["10:21","40:50"]},"questions":[{"hop_type":"one_hop","lang":"zh-cn"},{"hop_type":"two_hop","lang":"zh-cn"},{"hop_type":"multi_hop","lang":"zh-cn"}]},{"set_no":2,"metric":{"metric_id":"416_mtc_1","source_line_spans":["5"],"metric_name":"汞","metric_value":"≤2","metric_unit":"mg/kg","metric_name_en":"Mercury","context":"Line content B\nLine content C\nLine content D\nLine content E\nLine content F\nLine content G\nLine content H\nLine content I\nLine content J\nLine content K\nLine content L\nLine content M\nLine content N\nLine content O\nLine content P","context_line_ranges":["1:15"]},"questions":[{"hop_type":"one_hop","lang":"zh-cn"},{"hop_type":"two_hop","lang":"zh-cn"},{"hop_type":"multi_hop","lang":"zh-cn"}]}],"previous_questions":["Earlier question?"],"validation_error":"model returned the wrong total question count\nmodel returned 0 one_hop questions; expected 2\nmodel returned 0 two_hop questions; expected 2\nmodel returned 0 multi_hop questions; expected 2\n6 requested metric questions are missing or rejected","previous_response":{"questions":[]}}`,
}

func TestMetricQuestionModelInputUnchanged(t *testing.T) {
	db, _ := installPolicyDB(t)
	installQuestionMetadataFixture(t)
	oldMetrics, oldLines, oldRandom, oldNames := loadQuestionGenerationMetrics, loadQuestionGenerationLines, questionGenerationRandomInt, loadQuestionMetricEnglishNames
	t.Cleanup(func() {
		loadQuestionGenerationMetrics, loadQuestionGenerationLines, questionGenerationRandomInt, loadQuestionMetricEnglishNames = oldMetrics, oldLines, oldRandom, oldNames
	})
	questionGenerationRandomInt = func(n int) int { return n - 1 }
	loadQuestionGenerationMetrics = func(context.Context, *sql.DB, int64) ([]metricReviewInputMetric, error) {
		return []metricReviewInputMetric{
			{MetricID: "416_mtc_1", MetricName: "汞", MetricUnit: "mg/kg", MetricValue: "≤2", SourceLineSpans: json.RawMessage(`["5"]`)},
			{MetricID: "416_mtc_2", MetricName: "保温层长度", Condition: "outdoor", SourceLineSpans: json.RawMessage(`["20-21","40"]`)},
		}, nil
	}
	loadQuestionGenerationLines = func(context.Context, *sql.DB, int64) ([]docprocessing.Line, error) {
		lines := []docprocessing.Line{}
		for i := 1; i <= 60; i++ {
			lines = append(lines, docprocessing.Line{LineNo: i, Content: "Line content " + string(rune('A'+i%26))})
		}
		lines[39].LineType = "table"
		return lines, nil
	}
	loadQuestionMetricEnglishNames = func(context.Context, *sql.DB, int64) (map[string]string, error) {
		return map[string]string{"416_mtc_1": "Mercury"}, nil
	}
	inputs := []string{}
	client := questionExtractorFunc(func(_ context.Context, in llmclients.JSONExtractionInput) (map[string]any, error) {
		inputs = append(inputs, in.InputText)
		// The first answer is invalid (no questions), forcing one correction attempt.
		return map[string]any{"questions": []any{}}, nil
	})
	req := questionRequest(t, `{"input_record_id":416,"purpose":"search_metrics","description":"limits","metrics_per_document":2}`)
	if err := req.normalize(); err != nil {
		t.Fatal(err)
	}
	logger := loggerutil.CreateDefaultLogger("20261111-913")
	defer logger.Close()
	if _, err := generateMetricQuestionDocument(context.Background(), db, 12, req, ApiTypes.LLMModelDef{ModelName: "fixture"}, client, questionGenerationPrompt, "fixture prompt", "admin", []string{"Earlier question?"}, logger); err == nil {
		t.Fatal("empty output accepted")
	}
	if os.Getenv("PRINT_METRIC_QUESTION_GOLDEN") == "1" {
		for _, input := range inputs {
			t.Logf("GOLDEN %s", input)
		}
	}
	if len(inputs) != len(metricQuestionGoldenInputs) {
		t.Fatalf("model calls=%d want %d", len(inputs), len(metricQuestionGoldenInputs))
	}
	for i := range inputs {
		if sortedValidationError(inputs[i]) != sortedValidationError(metricQuestionGoldenInputs[i]) {
			t.Errorf("metric model input %d changed:\n got %s\nwant %s", i+1, inputs[i], metricQuestionGoldenInputs[i])
		}
	}
}

// sortedValidationError orders the correction's error lines, which validation reports in map order.
func sortedValidationError(input string) string {
	var v struct {
		ValidationError string `json:"validation_error"`
	}
	if json.Unmarshal([]byte(input), &v) != nil || v.ValidationError == "" {
		return input
	}
	original, _ := json.Marshal(v.ValidationError)
	lines := strings.Split(v.ValidationError, "\n")
	sort.Strings(lines)
	sorted, _ := json.Marshal(strings.Join(lines, "\n"))
	return strings.Replace(input, string(original), string(sorted), 1)
}
