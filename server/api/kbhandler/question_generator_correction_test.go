package kbhandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

func TestQuestionValidationReportsLaterMissingFields(t *testing.T) {
	for _, field := range []string{"question", "answer"} {
		t.Run(field, func(t *testing.T) {
			req := questionRequest(t, `{"input_record_id":753,"purpose":"search_metrics","metrics_per_document":1}`)
			if err := req.normalize(); err != nil {
				t.Fatal(err)
			}
			items := []any{}
			for i, hop := range []string{"one_hop", "two_hop", "multi_hop"} {
				items = append(items, map[string]any{
					"hop_type": hop, "question": hop + " question?", "answer": "Daily",
					"reasoning_steps": []string{"Find rule.", "Read frequency.", "Apply rule."}[:i+1],
					"step_line_ids":   [][]int{{101}, {101}, {101}}[:i+1], "line_ids": []int{101},
				})
			}
			items[0].(map[string]any)["step_line_ids"] = [][]int{}
			items[2].(map[string]any)[field] = " \t "
			qs, err := validateGeneratedQuestions(map[string]any{"questions": items}, req, map[int]string{101: "Daily"})
			want := "question 3 has empty question text"
			if field == "answer" {
				want = "question 3 has no answer"
			}
			if err == nil || len(qs) != 0 || !strings.Contains(err.Error(), "question 1") || !strings.Contains(err.Error(), want) {
				t.Fatalf("questions=%d error=%v; want initial citation error and %q", len(qs), err, want)
			}
		})
	}
}

// Reproduce run 6: two explanations but one citation array for its first question.
func TestMetricQuestionCorrectsMisalignedCitations(t *testing.T) {
	for _, mode := range []string{"corrected", "still-invalid", "provider-error"} {
		t.Run(mode, func(t *testing.T) {
			db, _ := installPolicyDB(t)
			oldLines, oldMetrics, oldRandom := loadQuestionGenerationLines, loadQuestionGenerationMetrics, questionGenerationRandomInt
			t.Cleanup(func() {
				loadQuestionGenerationLines, loadQuestionGenerationMetrics, questionGenerationRandomInt = oldLines, oldMetrics, oldRandom
			})
			installQuestionMetadataFixture(t)
			questionGenerationRandomInt = func(int) int { return 0 }
			loadQuestionGenerationLines = func(context.Context, *sql.DB, int64) ([]docprocessing.Line, error) {
				return []docprocessing.Line{{LineNo: 101, Content: "Perishable waste is collected daily."}}, nil
			}
			loadQuestionGenerationMetrics = func(context.Context, *sql.DB, int64) ([]metricReviewInputMetric, error) {
				return []metricReviewInputMetric{{MetricID: "m1", SourceLineSpans: json.RawMessage(`["101"]`)}}, nil
			}
			calls := 0
			var originalSets json.RawMessage
			var rejected map[string]any
			client := questionExtractorFunc(func(_ context.Context, in llmclients.JSONExtractionInput) (map[string]any, error) {
				calls++
				if in.RunID != 6 || in.RecordID != 416 {
					t.Fatal("lost usage correlation")
				}
				var input struct {
					Sets             json.RawMessage `json:"sets"`
					ValidationError  string          `json:"validation_error"`
					PreviousResponse map[string]any  `json:"previous_response"`
				}
				if err := json.Unmarshal([]byte(in.InputText), &input); err != nil {
					t.Fatal(err)
				}
				if calls == 1 {
					originalSets = input.Sets
					if input.ValidationError != "" || input.PreviousResponse != nil {
						t.Fatal("feedback on initial attempt")
					}
				} else {
					if string(input.Sets) != string(originalSets) {
						t.Fatal("correction resampled metrics or context")
					}
					if !strings.Contains(input.ValidationError, "2 reasoning steps") || !strings.Contains(input.ValidationError, "1 citation arrays") {
						t.Fatalf("missing precise feedback: %q", input.ValidationError)
					}
					if !strings.Contains(input.ValidationError, "question 3 duplicates question 2") {
						t.Fatalf("correction did not include later duplicate: %q", input.ValidationError)
					}
					if !reflect.DeepEqual(input.PreviousResponse, rejected) {
						t.Fatal("missing rejected response")
					}
				}
				if mode == "provider-error" {
					return nil, errors.New("provider unavailable")
				}
				var payload map[string]any
				if err := json.Unmarshal([]byte(`{"questions":[{"set_no":1,"hop_type":"one_hop","lang":"en","question":"How often is waste collected?","answer":"Daily","reasoning_steps":["Locate collection rules.","Read the daily frequency."],"step_line_ids":[[101],[101]],"line_ids":[101]},{"set_no":1,"hop_type":"two_hop","lang":"en","question":"For which waste is daily collection required?","answer":"Perishable waste","reasoning_steps":["Find waste type.","Read frequency."],"step_line_ids":[[101],[101]],"line_ids":[101]},{"set_no":1,"hop_type":"multi_hop","lang":"en","question":"What waste, operation, and frequency does the rule specify?","answer":"Perishable waste is collected daily.","reasoning_steps":["Find waste type.","Find operation.","Read frequency."],"step_line_ids":[[101],[101],[101]],"line_ids":[101]}]}`), &payload); err != nil {
					t.Fatal(err)
				}
				if calls == 1 || mode == "still-invalid" {
					payload["questions"].([]any)[0].(map[string]any)["step_line_ids"] = []any{[]any{float64(101)}}
					payload["questions"].([]any)[2].(map[string]any)["question"] = "  FOR WHICH WASTE is daily collection required?  "
					rejected = payload
				} else {
					// A version 2 correction returns paired objects even when the
					// rejected response used malformed legacy parallel arrays.
					for _, item := range payload["questions"].([]any) {
						q := item.(map[string]any)
						reasoning := []any{}
						for i, text := range q["reasoning_steps"].([]any) {
							reasoning = append(reasoning, map[string]any{"text": text, "line_ids": q["step_line_ids"].([]any)[i]})
						}
						q["reasoning"] = reasoning
						delete(q, "reasoning_steps")
						delete(q, "step_line_ids")
						delete(q, "line_ids")
					}
				}
				return payload, nil
			})
			req := questionRequest(t, `{"input_record_id":416,"purpose":"search_metrics","metrics_per_document":1}`)
			if err := req.normalize(); err != nil {
				t.Fatal(err)
			}
			logger := loggerutil.CreateDefaultLogger("20261024-619")
			defer logger.Close()
			qs, err := generateMetricQuestionDocument(context.Background(), db, 6, req, ApiTypes.LLMModelDef{ModelName: "fixture"}, client, questionGenerationPrompt, "fixture prompt", "admin", nil, logger)
			if mode == "corrected" {
				if err != nil || len(qs) != 3 || calls != 2 {
					t.Fatalf("questions=%d calls=%d error=%v", len(qs), calls, err)
				}
			} else {
				expectedCalls := 2
				if mode == "provider-error" {
					expectedCalls = 1
				}
				if err == nil || len(qs) != 0 || calls != expectedCalls {
					t.Fatalf("questions=%d calls=%d error=%v", len(qs), calls, err)
				}
			}
		})
	}
}
