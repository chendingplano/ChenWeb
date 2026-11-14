package complianceeval

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// GoldAct is a gold activity as the scorer sees it: its span, the records its final judgements
// label `governs`, and the final label of every judged passage. Ambiguous judgements are counted
// but never used as a label.
type GoldAct struct {
	Record        int64
	LineStart     int
	LineEnd       int
	DocumentLevel bool
	Governs       map[int64]bool    // candidate record ids with final label governs
	Labels        map[string]string // "<artifact_type>/<artifact_id>" -> final label
	// OwnCopies are the document and its byte-identical copies. The gold never pools them, and a
	// passage on them copies the activity's own text, so it is unrelated by rule J3 without labelling.
	OwnCopies map[int64]bool
	Ambiguous int
	Pooled    int
}

// label is the gold's label of a reported passage for this activity: its judgement, unrelated for a
// passage on the document's own copies, or "" when the gold never judged it.
func (g GoldAct) label(c ReviewCandidate) string {
	if l := g.Labels[c.Key]; l != "" {
		return l
	}
	if g.OwnCopies[c.Record] {
		return labelUnrelated
	}
	return ""
}

// ReviewCandidate is one passage a reviewer row reports.
type ReviewCandidate struct {
	Key    string // "<artifact_type>/<artifact_id>"
	Record int64
	Cosine *float64
}

// ReviewRow is one Compliance Review row: an activity with its reported candidates.
type ReviewRow struct {
	Record        int64
	LineStart     int
	LineEnd       int
	DocumentLevel bool
	NoGrounding   bool
	Candidates    map[int64]bool // records of the reported candidates
	Artifacts     []ReviewCandidate
}

// Metrics are the figures of one scoring, each with the count behind it. A ratio whose
// denominator is zero is reported as not defined (negative N means no data), never as 0 or 1.
type Metrics struct {
	GoldActivities, ReviewRows        int
	GoldWithGoverns                   int // gold activities with a governing passage; the rest expect no grounding
	MatchedActivities                 int // gold activities covered by a reviewer row
	SupportedRows                     int // reviewer rows covering a gold activity
	ActivityPrecision, ActivityRecall Ratio
	// Grounding recall within the pool: governing records the reviewer reported, over governing
	// records in the judged pool, across matched activities. The pool bounds true recall.
	GroundingRecall Ratio
	// Of matched activities the reviewer reported as no-grounding: share where gold had no governing record.
	NoGroundingPrecision Ratio
	// Of those, share where gold had a governing record the reviewer's search did not report.
	NoGroundingMissedBySearch Ratio
	// Of matched activities with no governing record in gold: share where the reviewer reported
	// a passage the gold judged.
	FalseGrounding Ratio
	// Matched activities with no governing record in gold whose reported passages the gold never
	// judged (on records added after the snapshot, or outside the pool): left out of FalseGrounding.
	UnjudgedActivities int
	// Reported candidate records added after the snapshot, counted once per matched activity.
	UnjudgedCandidates int
	// Candidate-level figures over (reviewer row, reported passage) for rows covering a gold activity
	// (design D12). A passage takes its best label over the gold activities the row covers.
	CandidatePrecision     Ratio          // governs over governs + topical + unrelated
	CandidateLabels        map[string]int // count per label, ambiguous included
	CandidatesNotInPool    int            // on snapshot records but never judged: compliance-gold augment judges them
	CandidatesOnNewRecords int            // on records added after the snapshot
	CandidatesOnOwnCopies  int            // on the document itself or a byte-identical copy: unrelated (J3)
	AmbiguousJudgements    int
	Headline               Ratio
}

// labelRank orders labels for a passage judged under several gold activities.
var labelRank = map[string]int{labelGoverns: 4, labelAmbiguous: 3, labelTopical: 2, labelUnrelated: 1}

// Ratio is Num/Den; Den == 0 means not defined.
type Ratio struct{ Num, Den int }

func (r Ratio) Defined() bool { return r.Den > 0 }
func (r Ratio) Value() float64 {
	if r.Den == 0 {
		return 0
	}
	return float64(r.Num) / float64(r.Den)
}
func (r Ratio) String() string {
	if r.Den == 0 {
		return "n/a (0 cases)"
	}
	return fmt.Sprintf("%.3f (%d/%d)", r.Value(), r.Num, r.Den)
}

func spansMatch(aS, aE, bS, bE int, aDoc, bDoc bool) bool {
	if aDoc != bDoc {
		return false
	}
	lo, hi := max(aS, bS), min(aE, bE)
	if hi < lo {
		return false
	}
	shorter := min(aE-aS, bE-bS) + 1
	return float64(hi-lo+1)/float64(shorter) >= overlapThreshold
}

// Score compares reviewer rows with gold activities. Matching is by coverage, not one-to-one,
// because the reviewer merges neighbouring lines into one activity while annotators often label
// each clause: a gold activity is found when some reviewer row (same record, same document-level
// flag) overlaps its span by at least half of the shorter span, and a reviewer row is supported when
// it overlaps some gold activity. For a found gold activity, grounding is judged on all rows that
// cover it: a governing record counts as reported when any of them lists it, and the activity counts
// as "no grounding" only when every covering row says so. The headline is the mean of activity F1,
// grounding recall and one minus the false-grounding rate over the components that are defined;
// it is a summary, and the components are the result.
//
// inSnapshot is the gold snapshot's record set. A reviewer candidate on a record outside it was
// indexed after the snapshot, so gold never judged it: it is unjudged, never a false grounding. Nil
// treats every record as inside the snapshot. A passage on a snapshot record that the gold never
// judged is unjudged too (outside the pool); it is never counted as wrong (design D12).
func Score(gold []GoldAct, rows []ReviewRow, inSnapshot map[int64]bool) Metrics {
	m := Metrics{GoldActivities: len(gold), ReviewRows: len(rows), CandidateLabels: map[string]int{}}
	rowSupported := make([]bool, len(rows))
	rowCovers := make([][]int, len(rows)) // gold activities each row covers
	for gi, g := range gold {
		m.AmbiguousJudgements += g.Ambiguous
		if len(g.Governs) > 0 {
			m.GoldWithGoverns++
		}
		var cover []ReviewRow
		for i, r := range rows {
			if r.Record == g.Record && spansMatch(g.LineStart, g.LineEnd, r.LineStart, r.LineEnd, g.DocumentLevel, r.DocumentLevel) {
				cover = append(cover, r)
				rowSupported[i] = true
				rowCovers[i] = append(rowCovers[i], gi)
			}
		}
		if len(cover) == 0 {
			continue
		}
		m.MatchedActivities++
		reported := map[int64]bool{}
		allNo := true
		judgedReport := false // some reported passage was judged for this activity
		for _, r := range cover {
			for rec := range r.Candidates {
				reported[rec] = true
			}
			for _, c := range r.Artifacts {
				if g.label(c) != "" {
					judgedReport = true
				}
			}
			allNo = allNo && r.NoGrounding
		}
		if len(g.Governs) > 0 {
			m.GroundingRecall.Den += len(g.Governs)
			for rec := range g.Governs {
				if reported[rec] {
					m.GroundingRecall.Num++
				}
			}
		} else {
			// The gold judges passages on snapshot records only, so a judged report is a judgeable one.
			switch {
			case allNo:
				m.FalseGrounding.Den++
			case judgedReport:
				m.FalseGrounding.Den++
				m.FalseGrounding.Num++
			default:
				m.UnjudgedActivities++
			}
		}
		for rec := range reported {
			if inSnapshot != nil && !inSnapshot[rec] {
				m.UnjudgedCandidates++
			}
		}
		if allNo {
			m.NoGroundingPrecision.Den++
			m.NoGroundingMissedBySearch.Den++
			if len(g.Governs) == 0 {
				m.NoGroundingPrecision.Num++
			} else {
				m.NoGroundingMissedBySearch.Num++
			}
		}
	}
	for i, ok := range rowSupported {
		if !ok {
			continue
		}
		m.SupportedRows++
		for _, c := range rows[i].Artifacts {
			if inSnapshot != nil && !inSnapshot[c.Record] {
				m.CandidatesOnNewRecords++
				continue
			}
			best, ownCopy := "", false
			for _, gi := range rowCovers[i] {
				if l := gold[gi].label(c); labelRank[l] > labelRank[best] {
					best = l
				}
				ownCopy = ownCopy || gold[gi].OwnCopies[c.Record]
			}
			if best == "" {
				m.CandidatesNotInPool++
				continue
			}
			if ownCopy {
				m.CandidatesOnOwnCopies++
			}
			m.CandidateLabels[best]++
			if best != labelAmbiguous {
				m.CandidatePrecision.Den++
				if best == labelGoverns {
					m.CandidatePrecision.Num++
				}
			}
		}
	}
	m.ActivityPrecision = Ratio{m.SupportedRows, len(rows)}
	m.ActivityRecall = Ratio{m.MatchedActivities, len(gold)}
	var parts []float64
	if p, r := m.ActivityPrecision, m.ActivityRecall; p.Defined() && r.Defined() {
		if p.Num == 0 || r.Num == 0 {
			parts = append(parts, 0)
		} else {
			parts = append(parts, 2*p.Value()*r.Value()/(p.Value()+r.Value()))
		}
	}
	if m.GroundingRecall.Defined() {
		parts = append(parts, m.GroundingRecall.Value())
	}
	if m.FalseGrounding.Defined() {
		parts = append(parts, 1-m.FalseGrounding.Value())
	}
	if len(parts) > 0 {
		sum := 0.0
		for _, v := range parts {
			sum += v
		}
		m.Headline = Ratio{int(sum/float64(len(parts))*100 + 0.5), 100}
	}
	return m
}

// candidateRef reads one reported passage of a row description: "- [chunk 18342] (cosine 0.612) record 45 ...".
var candidateRef = regexp.MustCompile(`(?m)^- \[(\S+) ([^\]]+)\](?: \(cosine ([0-9.]+)\))? record (\d+) `)

// ParseReviewRow turns one saved Compliance Review finding into a ReviewRow. Location is
// "start-end"; the document-level row's title names the activity "document: ...".
func ParseReviewRow(record int64, findingType, title, location, description string) (ReviewRow, error) {
	parts := strings.SplitN(location, "-", 2)
	if len(parts) != 2 {
		return ReviewRow{}, fmt.Errorf("finding location %q is not start-end", location)
	}
	s, err1 := strconv.Atoi(parts[0])
	e, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return ReviewRow{}, fmt.Errorf("finding location %q is not start-end", location)
	}
	row := ReviewRow{Record: record, LineStart: s, LineEnd: e, NoGrounding: findingType == "compliance_no_grounding",
		DocumentLevel: strings.Contains(title, ": document: "), Candidates: map[int64]bool{}}
	for _, mm := range candidateRef.FindAllStringSubmatch(description, -1) {
		id, err := strconv.ParseInt(mm[4], 10, 64)
		if err != nil {
			continue
		}
		row.Candidates[id] = true
		c := ReviewCandidate{Key: mm[1] + "/" + mm[2], Record: id}
		if v, err := strconv.ParseFloat(mm[3], 64); err == nil {
			c.Cosine = &v
		}
		row.Artifacts = append(row.Artifacts, c)
	}
	return row, nil
}

// ActivityListsEqual reports whether two runs of the same document found the same activities (same
// spans and level), and lists the differences. It is the scope-stability check: grounding may change
// with the search scope, the activity list must not.
func ActivityListsEqual(a, b []ReviewRow) (bool, []string) {
	key := func(r ReviewRow) string { return fmt.Sprintf("%d-%d doc=%t", r.LineStart, r.LineEnd, r.DocumentLevel) }
	count := func(rs []ReviewRow) map[string]int {
		m := map[string]int{}
		for _, r := range rs {
			m[key(r)]++
		}
		return m
	}
	ca, cb := count(a), count(b)
	var diffs []string
	for k, n := range ca {
		if cb[k] != n {
			diffs = append(diffs, fmt.Sprintf("%s: %d in first run, %d in second", k, n, cb[k]))
		}
	}
	for k, n := range cb {
		if _, ok := ca[k]; !ok {
			diffs = append(diffs, fmt.Sprintf("%s: 0 in first run, %d in second", k, n))
		}
	}
	sort.Strings(diffs)
	return len(diffs) == 0, diffs
}
