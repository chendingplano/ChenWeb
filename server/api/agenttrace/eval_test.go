package agenttrace

import "testing"

func TestRunEvaluationsScoresTraceBehavior(t *testing.T) {
	trace := Trace{
		Output: "Q3 revenue was $4.2M, up 12% YoY.",
		ToolCalls: []ToolCall{
			{Name: "file_search", Arguments: map[string]any{"query": "Q3 report"}},
		},
		Usage: TokenUsage{TotalTokens: 620},
	}

	report := RunEvaluations([]EvalRun{{
		Case: TestCase{
			Name: "q3-summary",
			Scorers: []Scorer{
				ContainsAnswer("4.2M", "12%"),
				UsedTools("file_search"),
				AvoidedTools("send_email"),
				UnderTokenLimit(1000),
			},
		},
		Trace: trace,
	}})

	if !report.Passed {
		t.Fatalf("expected report to pass: %#v", report)
	}
	if report.OverallScore != 1 {
		t.Fatalf("overall score = %f, want 1", report.OverallScore)
	}
}

func TestRunEvaluationsReportsFailures(t *testing.T) {
	report := RunEvaluations([]EvalRun{{
		Case: TestCase{
			Name:    "unsafe",
			Scorers: []Scorer{AvoidedTools("send_email")},
		},
		Trace: Trace{ToolCalls: []ToolCall{{Name: "send_email"}}},
	}})

	if report.Passed {
		t.Fatal("expected report to fail")
	}
	if len(report.Cases) != 1 || len(report.Cases[0].ScorerResults) != 1 {
		t.Fatalf("unexpected report shape: %#v", report)
	}
	if report.Cases[0].ScorerResults[0].Passed {
		t.Fatalf("expected scorer to fail: %#v", report.Cases[0].ScorerResults[0])
	}
}
