package kbhandler

import (
	"strings"
	"testing"
)

func TestCheckMetricNameLeak(t *testing.T) {
	metric := questionGenerationMetric{metricReviewInputMetric: metricReviewInputMetric{MetricName: "试验持续时间"}, MetricNameEN: "Test duration"}
	for _, tc := range []struct {
		hop, question string
		leak          bool
	}{
		{"one_hop", "试验持续时间最少是多少？", false},
		{"two_hop", "达到稳态后，试验持续时间至少多久？", true},
		{"multi_hop", "What is the minimum TEST DURATION after steady state?", true},
		{"two_hop", "After the gas cooler reaches steady state, how long must recording continue?", false},
	} {
		err := checkMetricNameLeak(generatedQuestion{HopType: tc.hop, Question: tc.question}, metric)
		if (err != nil) != tc.leak {
			t.Errorf("%s %q: err=%v, want leak=%v", tc.hop, tc.question, err, tc.leak)
		}
		if err != nil && !strings.Contains(err.Error(), "describe the need") {
			t.Errorf("error lacks correction guidance: %v", err)
		}
	}
	short := questionGenerationMetric{metricReviewInputMetric: metricReviewInputMetric{MetricName: "量"}}
	if err := checkMetricNameLeak(generatedQuestion{HopType: "two_hop", Question: "这个量是多少？"}, short); err != nil {
		t.Errorf("one-character name checked: %v", err)
	}
}
