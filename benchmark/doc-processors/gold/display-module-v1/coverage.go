package gold

import (
	"strconv"
	"strings"
)

// CoverageJudgement is the outcome of checking one gold clause against a real
// pipeline run's extraction, per bug 2026073001 (KnowledgeStore/doc-repo/bugs/
// 202607/2026073001-bug-extract-metrics-recall-instability-across-identical-calls.md):
// a plain missing/extracted binary cannot express that some source
// statements are inherently vague, non-verifiable prose, where a miss is
// acceptable rather than a defect.
type CoverageJudgement string

const (
	// Captured means a real metric row's source_line_spans covered this
	// clause's line, regardless of the clause's Expectation.
	Captured CoverageJudgement = "captured"
	// MissingRequired means the clause was not captured and its Expectation
	// is "required" (the default) -- a real extraction defect.
	MissingRequired CoverageJudgement = "missing_required"
	// MissingBestEffort means the clause was not captured but its
	// Expectation is "best_effort" -- acceptable, not a defect.
	MissingBestEffort CoverageJudgement = "missing_best_effort"
)

// ClauseCoverage is one clause's judgement within one document.
type ClauseCoverage struct {
	ClauseID    string
	Document    string
	Line        int // 1-indexed line-file position, matching BuildDocuments' block order
	Expectation string
	Judgement   CoverageJudgement
}

// ClauseLines returns document's clauses in the same order BuildDocuments
// assigns them to line-file lines: clauses[i] is line i+1. Keeping this
// derivation in one place means a coverage check and the generated document
// can never disagree about which clause a line number refers to.
func ClauseLines(f File, document string) []Clause {
	var clauses []Clause
	for _, c := range f.Clause {
		if c.Document == document {
			clauses = append(clauses, c)
		}
	}
	return clauses
}

// ScoreCoverage judges every clause in document against coveredLines -- the
// set of line-file line numbers a real pipeline run actually produced a
// metric for (typically derived from kb.metrics.source_line_spans; see
// ParseLineSpans). It is a pure function: the caller is responsible for
// turning whatever the real extraction looked like (a DB query, a JSON blob,
// a benchmark case's captured output) into that set.
func ScoreCoverage(f File, document string, coveredLines map[int]bool) []ClauseCoverage {
	clauses := ClauseLines(f, document)
	out := make([]ClauseCoverage, len(clauses))
	for i, c := range clauses {
		line := i + 1
		expectation := c.Expectation
		if expectation == "" {
			expectation = "required"
		}
		judgement := MissingRequired
		if expectation == "best_effort" {
			judgement = MissingBestEffort
		}
		if coveredLines[line] {
			judgement = Captured
		}
		out[i] = ClauseCoverage{ClauseID: c.ID, Document: document, Line: line, Expectation: expectation, Judgement: judgement}
	}
	return out
}

// ParseLineSpans expands kb.metrics.source_line_spans-style span strings
// (each either a single line number "42" or an inclusive range "13:15") into
// the set of line numbers they cover. Malformed entries are skipped rather
// than erroring, since this is scoring convenience over already-persisted
// data, not a validating parser.
func ParseLineSpans(spans []string) map[int]bool {
	out := map[int]bool{}
	for _, s := range spans {
		s = strings.TrimSpace(s)
		if lo, hi, ok := strings.Cut(s, ":"); ok {
			loN, err1 := strconv.Atoi(strings.TrimSpace(lo))
			hiN, err2 := strconv.Atoi(strings.TrimSpace(hi))
			if err1 != nil || err2 != nil || hiN < loN {
				continue
			}
			for n := loN; n <= hiN; n++ {
				out[n] = true
			}
			continue
		}
		if n, err := strconv.Atoi(s); err == nil {
			out[n] = true
		}
	}
	return out
}
