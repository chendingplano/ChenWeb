package kbhandler

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
)

func TestMetricContextLineRanges(t *testing.T) {
	for _, tc := range []struct {
		ids  []int
		want []string
	}{
		{[]int{124, 125, 126, 127, 128}, []string{"124:128"}},
		{[]int{124, 125, 128, 130, 131}, []string{"124:125", "128", "130:131"}},
		{[]int{1, 3, 5}, []string{"1", "3", "5"}},
		{[]int{42}, []string{"42"}},
	} {
		if got := questionContextLineRanges(tc.ids); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("ids=%v ranges=%v want=%v", tc.ids, got, tc.want)
		}
	}
}

func TestMetricContextSerializesRanges(t *testing.T) {
	lines := []docprocessing.Line{{LineNo: 124, Content: "before"}, {LineNo: 125, Content: "metric"}, {LineNo: 126, Content: "after"}, {LineNo: 130, Content: "separate"}}
	sets := []metricQuestionSet{{SetNo: 1, Metric: questionGenerationMetric{metricReviewInputMetric: metricReviewInputMetric{MetricID: "m1", SourceLineSpans: json.RawMessage(`["125"]`)}}}}
	got, _, _, err := buildMetricQuestionContexts(lines, sets)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"context_line_ids"`) {
		t.Fatalf("uncompressed IDs sent: %s", raw)
	}
	if !strings.Contains(string(raw), `"context_line_ranges":["124:126","130"]`) {
		t.Fatalf("wrong ranges: %s", raw)
	}
	if !reflect.DeepEqual(got[0].Metric.ContextLineIDs, []int{124, 125, 126, 130}) {
		t.Fatal("lost exact internal evidence IDs")
	}
}
