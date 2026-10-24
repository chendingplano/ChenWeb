package kbhandler

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestQuestionPairedReasoning(t *testing.T) {
	for _, mode := range []string{"valid", "missing-citation", "unknown-line", "empty-text", "too-few-steps", "mixed-formats"} {
		t.Run(mode, func(t *testing.T) {
			req := questionRequest(t, `{"input_record_id":753,"purpose":"search_metrics","metrics_per_document":1}`)
			if err := req.normalize(); err != nil {
				t.Fatal(err)
			}
			var payload map[string]any
			if err := json.Unmarshal([]byte(`{"questions":[
{"hop_type":"one_hop","question":"What is the limit?","answer":"20 kg","reasoning":[{"text":"Locate the parameter.","line_ids":[101]},{"text":"Read its limit.","line_ids":[101,102]}]},
{"hop_type":"two_hop","question":"Which limit applies?","answer":"20 kg","reasoning":[{"text":"Find condition.","line_ids":[101]},{"text":"Read limit.","line_ids":[102]}]},
{"hop_type":"multi_hop","question":"How do the conditions combine?","answer":"20 kg","reasoning":[{"text":"Find category.","line_ids":[101]},{"text":"Find condition.","line_ids":[102]},{"text":"Read limit.","line_ids":[101]}]}
]}`), &payload); err != nil {
				t.Fatal(err)
			}
			q := payload["questions"].([]any)[0].(map[string]any)
			step := q["reasoning"].([]any)[1].(map[string]any)
			wantError := ""
			switch mode {
			case "missing-citation":
				delete(step, "line_ids")
				wantError = "unsupported reasoning step"
			case "unknown-line":
				step["line_ids"] = []int{999}
				wantError = "unknown or repeated line 999"
			case "empty-text":
				step["text"] = " "
				wantError = "unsupported reasoning step"
			case "too-few-steps":
				payload["questions"].([]any)[2].(map[string]any)["reasoning"] = []any{step}
				wantError = "requires at least 3"
			case "mixed-formats":
				q["step_line_ids"] = [][]int{{999}}
				wantError = "mixes reasoning formats"
			}
			qs, err := validateGeneratedQuestions(payload, req, map[int]string{101: "Parameter", 102: "20 kg"})
			if wantError != "" {
				if err == nil || len(qs) != 0 || !strings.Contains(err.Error(), wantError) {
					t.Fatalf("questions=%d error=%v; want %q", len(qs), err, wantError)
				}
				return
			}
			if err != nil || len(qs) != 3 {
				t.Fatalf("questions=%d error=%v", len(qs), err)
			}
			if !reflect.DeepEqual(qs[0].ReasoningSteps, []string{"Locate the parameter.", "Read its limit."}) ||
				!reflect.DeepEqual(qs[0].StepLineIDs, [][]int{{101}, {101, 102}}) || len(qs[0].Evidence) != 2 {
				t.Fatalf("lost paired reasoning or duplicated evidence: %+v", qs[0])
			}
		})
	}
}
