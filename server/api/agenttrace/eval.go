package agenttrace

import (
	"fmt"
	"strings"
)

type ScoreResult struct {
	ScorerName string         `json:"scorer_name"`
	Passed     bool           `json:"passed"`
	Score      float64        `json:"score"`
	Reason     string         `json:"reason,omitempty"`
	Details    map[string]any `json:"details,omitempty"`
}

type Scorer interface {
	Name() string
	Score(Trace) ScoreResult
}

type TestCase struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Scorers     []Scorer `json:"-"`
}

type EvalRun struct {
	Case  TestCase `json:"case"`
	Trace Trace    `json:"trace"`
}

type CaseResult struct {
	CaseName      string        `json:"case_name"`
	Tags          []string      `json:"tags,omitempty"`
	Passed        bool          `json:"passed"`
	Score         float64       `json:"score"`
	ScorerResults []ScoreResult `json:"scorer_results"`
}

type Report struct {
	Passed       bool         `json:"passed"`
	OverallScore float64      `json:"overall_score"`
	NumPassed    int          `json:"num_passed"`
	NumFailed    int          `json:"num_failed"`
	Cases        []CaseResult `json:"cases"`
}

func RunEvaluations(runs []EvalRun) Report {
	report := Report{Passed: true, Cases: make([]CaseResult, 0, len(runs))}
	for _, run := range runs {
		result := CaseResult{
			CaseName:      run.Case.Name,
			Tags:          append([]string(nil), run.Case.Tags...),
			Passed:        true,
			ScorerResults: make([]ScoreResult, 0, len(run.Case.Scorers)),
		}
		for _, scorer := range run.Case.Scorers {
			sr := scorer.Score(run.Trace)
			if sr.ScorerName == "" {
				sr.ScorerName = scorer.Name()
			}
			result.ScorerResults = append(result.ScorerResults, sr)
			result.Score += sr.Score
			if !sr.Passed {
				result.Passed = false
			}
		}
		if len(result.ScorerResults) > 0 {
			result.Score /= float64(len(result.ScorerResults))
		}
		if result.Passed {
			report.NumPassed++
		} else {
			report.NumFailed++
			report.Passed = false
		}
		report.OverallScore += result.Score
		report.Cases = append(report.Cases, result)
	}
	if len(report.Cases) > 0 {
		report.OverallScore /= float64(len(report.Cases))
	}
	return report
}

type stringSetScorer struct {
	name   string
	values []string
	fn     func(Trace, []string) ScoreResult
}

func (s stringSetScorer) Name() string { return s.name }
func (s stringSetScorer) Score(t Trace) ScoreResult {
	result := s.fn(t, s.values)
	result.ScorerName = s.name
	return result
}

func ContainsAnswer(needles ...string) Scorer {
	return stringSetScorer{name: "ContainsAnswer", values: needles, fn: func(t Trace, values []string) ScoreResult {
		output := strings.ToLower(t.Output)
		missing := []string{}
		for _, value := range values {
			if !strings.Contains(output, strings.ToLower(value)) {
				missing = append(missing, value)
			}
		}
		return passFail(len(missing) == 0, 1-float64(len(missing))/float64(max(1, len(values))),
			fmt.Sprintf("missing answer fragments: %s", strings.Join(missing, ", ")))
	}}
}

func UsedTools(names ...string) Scorer {
	return stringSetScorer{name: "UsedTools", values: names, fn: func(t Trace, values []string) ScoreResult {
		called := map[string]bool{}
		for _, name := range t.ToolNames() {
			called[name] = true
		}
		missing := []string{}
		for _, name := range values {
			if !called[name] {
				missing = append(missing, name)
			}
		}
		return passFail(len(missing) == 0, 1-float64(len(missing))/float64(max(1, len(values))),
			fmt.Sprintf("missing tools: %s", strings.Join(missing, ", ")))
	}}
}

func AvoidedTools(names ...string) Scorer {
	return stringSetScorer{name: "AvoidedTools", values: names, fn: func(t Trace, values []string) ScoreResult {
		forbidden := map[string]bool{}
		for _, name := range values {
			forbidden[name] = true
		}
		violations := []string{}
		for _, name := range t.ToolNames() {
			if forbidden[name] {
				violations = append(violations, name)
			}
		}
		return passFail(len(violations) == 0, boolScore(len(violations) == 0),
			fmt.Sprintf("forbidden tools called: %s", strings.Join(violations, ", ")))
	}}
}

type tokenLimitScorer struct {
	maxTokens int
}

func UnderTokenLimit(maxTokens int) Scorer { return tokenLimitScorer{maxTokens: maxTokens} }
func (s tokenLimitScorer) Name() string    { return "UnderTokenLimit" }
func (s tokenLimitScorer) Score(t Trace) ScoreResult {
	passed := t.Usage.TotalTokens <= s.maxTokens
	return passFail(passed, boolScore(passed), fmt.Sprintf("token usage %d exceeds limit %d", t.Usage.TotalTokens, s.maxTokens))
}

func passFail(passed bool, score float64, reason string) ScoreResult {
	if passed {
		return ScoreResult{Passed: true, Score: score}
	}
	return ScoreResult{Passed: false, Score: score, Reason: reason}
}

func boolScore(v bool) float64 {
	if v {
		return 1
	}
	return 0
}
