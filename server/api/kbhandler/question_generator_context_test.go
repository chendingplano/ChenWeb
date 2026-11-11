package kbhandler

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
)

func TestMetricQuestionSamplingUsesThreeQuestionsPerDistinctMetric(t *testing.T) {
	original := questionGenerationRandomInt
	t.Cleanup(func() { questionGenerationRandomInt = original })
	calls := 0
	questionGenerationRandomInt = func(n int) int { calls++; return ((calls - 1) / 2) % n }
	metrics := make([]metricReviewInputMetric, 6)
	for i := range metrics {
		metrics[i].MetricID = fmt.Sprintf("m%d", i)
	}
	sets, err := makeMetricQuestionSets(metrics, questionCounts{OneHop: 3, TwoHop: 3, MultiHop: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(sets) != 3 {
		t.Fatalf("got %d sets, want three independently sampled metrics", len(sets))
	}
	seen := map[string]bool{}
	hops := map[string]int{}
	langs := map[string]bool{}
	for i, s := range sets {
		if s.SetNo != i+1 || len(s.Questions) != 3 || seen[s.Metric.MetricID] {
			t.Fatalf("set is not a distinct three-question anchor: %+v", s)
		}
		seen[s.Metric.MetricID] = true
		setHops := map[string]int{}
		for _, target := range s.Questions {
			setHops[target.HopType]++
			hops[target.HopType]++
			langs[target.Lang] = true
		}
		if setHops["one_hop"] != 1 || setHops["two_hop"] != 1 || setHops["multi_hop"] != 1 {
			t.Fatalf("metric does not have one question per hop: %+v", s)
		}
	}
	if hops["one_hop"] != 3 || hops["two_hop"] != 3 || hops["multi_hop"] != 3 || !langs["en"] || !langs["zh-cn"] {
		t.Fatalf("hops=%v langs=%v", hops, langs)
	}
	sets, err = makeMetricQuestionSets(metrics[:2], questionCounts{OneHop: 5, TwoHop: 5, MultiHop: 5})
	if err != nil || len(sets) != 5 {
		t.Fatalf("shortage sets=%v err=%v", sets, err)
	}
	for i := 0; i < 4; i += 2 {
		if sets[i].Metric.MetricID == sets[i+1].Metric.MetricID {
			t.Fatalf("reused metric before pool exhausted: %+v", sets)
		}
	}
}

func TestMetricQuestionContextsUseRealNeighborsAndDisjointEvidence(t *testing.T) {
	lines := make([]docprocessing.Line, 50)
	for i := range lines {
		lines[i] = docprocessing.Line{LineNo: (i + 1) * 3, Content: fmt.Sprintf("line %d", (i+1)*3)}
	}
	// Deliberately shuffle canonical order and keep huge unrelated data outside the context.
	lines[0].Content = strings.Repeat("x", questionGenerationMaxInputChars+1)
	lines[18], lines[19] = lines[19], lines[18]
	sets := []metricQuestionSet{{SetNo: 1, Metric: questionGenerationMetric{metricReviewInputMetric: metricReviewInputMetric{MetricID: "m", SourceLineSpans: json.RawMessage(`["60-63","90"]`)}}}}
	got, source, hash, err := buildMetricQuestionContexts(lines, sets)
	if err != nil {
		t.Fatal(err)
	}
	wantIDs := []int{}
	for id := 30; id < 60; id += 3 {
		wantIDs = append(wantIDs, id)
	}
	wantIDs = append(wantIDs, 60, 63, 90)
	for id := 93; id <= 120; id += 3 {
		wantIDs = append(wantIDs, id)
	}
	if !reflect.DeepEqual(got[0].Metric.ContextLineIDs, wantIDs) {
		t.Fatalf("ids=%v want=%v", got[0].Metric.ContextLineIDs, wantIDs)
	}
	wantText := []string{}
	for _, id := range wantIDs {
		wantText = append(wantText, fmt.Sprintf("line %d", id))
	}
	if got[0].Metric.Context != strings.Join(wantText, "\n") || len(source) != len(wantIDs) || hash == "" {
		t.Fatalf("context=%+v source=%v hash=%s", got[0].Metric, source, hash)
	}
	if sets[0].Metric.Context != "" {
		t.Fatal("input sets mutated")
	}
	selected := []docprocessing.Line{}
	for _, id := range wantIDs {
		selected = append(selected, docprocessing.Line{LineNo: id, Content: source[id]})
	}
	_, _, wantHash, err := buildQuestionGenerationSource(selected)
	if err != nil || hash != wantHash {
		t.Fatalf("snapshot hash mismatch: %s %s %v", hash, wantHash, err)
	}
}

func TestMetricQuestionContextsClampAndRejectMissingEvidence(t *testing.T) {
	lines := []docprocessing.Line{{LineNo: 2, Content: "first"}, {LineNo: 7, Content: "second"}, {LineNo: 13, Content: "last"}}
	for _, span := range []string{`["2"]`, `["13"]`, `["2","2-7"]`} {
		sets := []metricQuestionSet{{Metric: questionGenerationMetric{metricReviewInputMetric: metricReviewInputMetric{SourceLineSpans: json.RawMessage(span)}}}}
		got, source, _, err := buildMetricQuestionContexts(lines, sets)
		if err != nil || len(source) != 3 || !reflect.DeepEqual(got[0].Metric.ContextLineIDs, []int{2, 7, 13}) {
			t.Fatalf("span=%s sets=%v err=%v", span, got, err)
		}
	}
	for _, span := range []string{`["999"]`, `["2","999"]`, `[]`} {
		sets := []metricQuestionSet{{Metric: questionGenerationMetric{metricReviewInputMetric: metricReviewInputMetric{SourceLineSpans: json.RawMessage(span)}}}}
		if _, _, _, err := buildMetricQuestionContexts(lines, sets); err == nil {
			t.Fatalf("accepted missing evidence %s", span)
		}
	}
}

func TestMetricQuestionRejectsAnotherMetricsContext(t *testing.T) {
	req := questionRequest(t, `{"input_record_id":416,"purpose":"search_metrics","one_hop_count":1,"two_hop_count":0,"multi_hop_count":0}`)
	set := metricQuestionSet{SetNo: 1, Metric: questionGenerationMetric{metricReviewInputMetric: metricReviewInputMetric{MetricID: "m", SourceLineSpans: json.RawMessage(`["2"]`)}, Context: "20 kg", ContextLineIDs: []int{2}}, Questions: []questionSlot{{HopType: "one_hop", Lang: "en"}}}
	var payload map[string]any
	if err := json.Unmarshal([]byte(`{"questions":[{"set_no":1,"lang":"en","hop_type":"one_hop","question":"What is the weight?","answer":"20 kg","reasoning_steps":["Read both lines."],"step_line_ids":[[2,8]],"line_ids":[2,8]}]}`), &payload); err != nil {
		t.Fatal(err)
	}
	if _, err := validateMetricQuestions(payload, req, map[int]string{2: "20 kg", 8: "another metric"}, []metricQuestionSet{set}, "hash"); err == nil {
		t.Fatal("accepted citation from another metric context")
	}
	q := payload["questions"].([]any)[0].(map[string]any)
	q["line_ids"] = []int{2}
	q["step_line_ids"] = [][]int{{2}}
	qs, err := validateMetricQuestions(payload, req, map[int]string{2: "20 kg"}, []metricQuestionSet{set}, "hash")
	if err != nil {
		t.Fatal(err)
	}
	var snapshot map[string]any
	if err := json.Unmarshal(qs[0].MetricSnapshot, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot["context"] != "20 kg" || snapshot["metric_id"] != "m" {
		t.Fatalf("snapshot=%v", snapshot)
	}
}

func TestMetricQuestionContextsPreserveTableTextAndDeduplicateUnion(t *testing.T) {
	lines := []docprocessing.Line{{LineNo: 3, Content: "caption"}, {LineNo: 9, LineType: "table", Content: "<table><tr><th>Weight</th></tr><tr><td>20 kg</td></tr></table>"}, {LineNo: 15, Content: "notes"}}
	sets := []metricQuestionSet{
		{SetNo: 1, Metric: questionGenerationMetric{metricReviewInputMetric: metricReviewInputMetric{MetricID: "table", SourceLineSpans: json.RawMessage(`["9"]`)}}},
		{SetNo: 2, Metric: questionGenerationMetric{metricReviewInputMetric: metricReviewInputMetric{MetricID: "caption", SourceLineSpans: json.RawMessage(`["3"]`)}}},
	}
	got, source, hash, err := buildMetricQuestionContexts(lines, sets)
	if err != nil {
		t.Fatal(err)
	}
	_, wantSource, wantHash, err := buildQuestionGenerationSource(lines)
	if err != nil || hash != wantHash || !reflect.DeepEqual(source, wantSource) {
		t.Fatalf("source=%v want=%v hash=%s want=%s error=%v", source, wantSource, hash, wantHash, err)
	}
	want := "caption\n" + docprocessing.LLMLineContent("table", 9, lines[1].Content) + "\nnotes"
	for _, s := range got {
		if s.Metric.Context != want || !reflect.DeepEqual(s.Metric.ContextLineIDs, []int{3, 9, 15}) {
			t.Fatalf("context=%+v", s.Metric)
		}
	}
	sets = append(sets, metricQuestionSet{Metric: questionGenerationMetric{metricReviewInputMetric: metricReviewInputMetric{SourceLineSpans: json.RawMessage(`["999"]`)}}})
	if _, _, _, err := buildMetricQuestionContexts(lines, sets); err == nil {
		t.Fatal("missing source accepted")
	}
	for _, s := range sets {
		if s.Metric.Context != "" || len(s.Metric.ContextLineIDs) != 0 {
			t.Fatal("error mutated input contexts")
		}
	}
}
