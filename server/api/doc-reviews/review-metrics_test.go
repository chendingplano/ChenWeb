package docreviews

import (
	"context"
	"strings"
	"testing"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

func TestParseJSONStringArray(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{`["a","b"]`, []string{"a", "b"}},
		{``, nil},
		{`[]`, nil},
		{`null`, nil},
		{`[" a ",""]`, []string{"a"}},
		{`{`, nil},
	}
	for _, c := range cases {
		got := parseJSONStringArray([]byte(c.in))
		if strings.Join(got, ",") != strings.Join(c.want, ",") {
			t.Errorf("parseJSONStringArray(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func dm(id int64, metricID string, cats ...string) docMetric {
	return docMetric{id: id, view: metricView{MetricID: metricID, Categories: cats}}
}

func TestAssembleMatches_Branches_DedupExclusionCap(t *testing.T) {
	const recordID = int64(1)
	docMetrics := []docMetric{
		dm(11, "1_m_1", "pressure"), // index 0
		dm(12, "1_m_2", "temp"),     // index 1
	}

	hsEdges := []docprocessing.Connection{
		// Branch A: M1 -> cross-doc metric 2_m_9 (semantically_related).
		{SourceID: "1_m_1", TargetID: "2_m_9", RelationName: docprocessing.RelationSemanticallyRelated, Confidence: 0.9},
		// Branch A duplicate target reached again via entity branch below (dedup).
		{SourceID: "1_m_1", TargetID: "3_m_7", RelationName: docprocessing.RelationSemanticallyRelated, Confidence: 0.2},
		// Wrong relation_name -> ignored.
		{SourceID: "1_m_1", TargetID: "9_m_9", RelationName: "other", Confidence: 1.0},
		// Same-document target -> excluded.
		{SourceID: "1_m_1", TargetID: "1_m_5", RelationName: docprocessing.RelationSemanticallyRelated, Confidence: 0.5},
	}
	entEdges := []docprocessing.Connection{
		// Branch C: entity -> metric 3_m_7 (shares "pressure" with M1).
		{TargetID: "3_m_7", Confidence: 0.3},
	}
	resolved := map[string]resolvedMetric{
		"2_m_9": {view: metricView{MetricID: "2_m_9", Categories: []string{"pressure"}}, recordID: 2},
		"3_m_7": {view: metricView{MetricID: "3_m_7", Categories: []string{"pressure"}}, recordID: 3},
		"1_m_5": {view: metricView{MetricID: "1_m_5"}, recordID: recordID}, // same doc
	}
	siblings := []resolvedMetric{
		// Branch B: cross-doc metric sharing "temp" with M2.
		{view: metricView{MetricID: "4_m_2", Categories: []string{"temp"}}, recordID: 4},
	}

	// No cap first: verify branch coverage + dedup + exclusion.
	got := assembleMatches(recordID, docMetrics, hsEdges, nil, entEdges, resolved, siblings, 0)

	// M1 (index 0): 2_m_9 (A) and 3_m_7 (A+C deduped) = 2 matches; 1_m_5 excluded.
	if len(got[0]) != 2 {
		t.Fatalf("M1 matches = %d, want 2 (%v)", len(got[0]), got[0])
	}
	ids := map[string]bool{}
	for _, m := range got[0] {
		ids[m.view.MetricID] = true
		if m.recordID == recordID {
			t.Errorf("M1 match %s is same-document, should be excluded", m.view.MetricID)
		}
	}
	if !ids["2_m_9"] || !ids["3_m_7"] {
		t.Errorf("M1 matches missing expected ids: %v", ids)
	}

	// M2 (index 1): 4_m_2 via category branch.
	if len(got[1]) != 1 || got[1][0].view.MetricID != "4_m_2" || got[1][0].via != "metric_category" {
		t.Fatalf("M2 matches = %v, want [4_m_2 via metric_category]", got[1])
	}

	// Cap = 1: M1 keeps only the highest-confidence match (2_m_9 @0.9).
	capped := assembleMatches(recordID, docMetrics, hsEdges, nil, entEdges, resolved, siblings, 1)
	if len(capped[0]) != 1 || capped[0][0].view.MetricID != "2_m_9" {
		t.Fatalf("capped M1 = %v, want [2_m_9]", capped[0])
	}
}

func TestAssembleMatches_BranchA_InboundAndDedup(t *testing.T) {
	const recordID = int64(1)
	docMetrics := []docMetric{
		dm(11, "1_m_1"), // index 0
	}

	// Outbound: M1 -> 2_m_9 (match discovered when our doc was indexed).
	hsOut := []docprocessing.Connection{
		{SourceID: "1_m_1", TargetID: "2_m_9", RelationName: docprocessing.RelationSemanticallyRelated, Confidence: 0.7},
	}
	// Inbound: 5_m_3 -> M1 (a later-indexed doc linked to us); 2_m_9 -> M1 duplicates
	// the outbound match and must collapse; same-document inbound source excluded.
	hsIn := []docprocessing.Connection{
		{SourceID: "5_m_3", TargetID: "1_m_1", RelationName: docprocessing.RelationSemanticallyRelated, Confidence: 0.9},
		{SourceID: "2_m_9", TargetID: "1_m_1", RelationName: docprocessing.RelationSemanticallyRelated, Confidence: 0.4},
		{SourceID: "1_m_8", TargetID: "1_m_1", RelationName: docprocessing.RelationSemanticallyRelated, Confidence: 0.6},
		{SourceID: "9_m_9", TargetID: "1_m_1", RelationName: "other", Confidence: 1.0}, // wrong relation
	}
	resolved := map[string]resolvedMetric{
		"2_m_9": {view: metricView{MetricID: "2_m_9"}, recordID: 2},
		"5_m_3": {view: metricView{MetricID: "5_m_3"}, recordID: 5},
		"1_m_8": {view: metricView{MetricID: "1_m_8"}, recordID: recordID}, // same doc
	}

	got := assembleMatches(recordID, docMetrics, hsOut, hsIn, nil, resolved, nil, 0)

	// Expect 2_m_9 (out+in deduped) and 5_m_3 (inbound only); 1_m_8 excluded, wrong relation ignored.
	if len(got[0]) != 2 {
		t.Fatalf("M1 matches = %d, want 2 (%v)", len(got[0]), got[0])
	}
	ids := map[string]bool{}
	for _, m := range got[0] {
		ids[m.view.MetricID] = true
		if m.recordID == recordID {
			t.Errorf("M1 match %s is same-document, should be excluded", m.view.MetricID)
		}
	}
	if !ids["2_m_9"] || !ids["5_m_3"] {
		t.Errorf("M1 matches missing expected inbound/outbound ids: %v", ids)
	}
}

func TestReviewMetric_PayloadAndFindingTagging(t *testing.T) {
	fake := &fakeJSONExtractor{
		out: map[string]any{
			"findings": []any{
				map[string]any{"title": "Conflict", "description": "values differ"},
			},
		},
	}
	r := &metricsReviewer{
		client: fake,
		logger: loggerutil.CreateDefaultLogger("TEST_METRICS"),
	}
	doc := docMetric{
		id:    7,
		view:  metricView{MetricID: "1_m_1", MetricName: "压力", Value: "1.6", Unit: "MPa"},
		spans: []string{"12:14"},
	}
	ms := []matchedMetric{
		{view: metricView{MetricID: "2_m_9", Value: "2.5", Unit: "MPa"}, recordID: 2, filename: "other.pdf", via: "hybrid_search", confidence: 0.9},
	}

	findings := r.reviewMetric(context.Background(), 1, 0, ReviewerConfig{
		ModelName:  "metric-model",
		PromptText: "compare metrics",
		PromptRef:  "prompt-review-metrics-v1.md",
	}, doc, ms)

	if len(findings) != 1 {
		t.Fatalf("findings = %d, want 1", len(findings))
	}
	f := findings[0]
	if f.Pass != "P5" || f.Aspect != "metrics" {
		t.Errorf("pass/aspect = %q/%q, want P5/metrics", f.Pass, f.Aspect)
	}
	if f.FindingType != "issue" || f.Severity != "low" {
		t.Errorf("defaults: finding_type=%q severity=%q, want issue/low", f.FindingType, f.Severity)
	}
	if f.Location != "12:14" {
		t.Errorf("location = %q, want 12:14", f.Location)
	}
	if len(fake.documentFirst) != 1 || !fake.documentFirst[0] {
		t.Errorf("documentFirst = %v, want [true]", fake.documentFirst)
	}
	if len(fake.promptNames) != 1 || fake.promptNames[0] != "prompt-review-metrics-v1.md" {
		t.Errorf("promptNames = %v", fake.promptNames)
	}
	in := fake.inputTexts[0]
	for _, want := range []string{"metric_under_review", "matching_metrics", "2_m_9", "hybrid_search"} {
		if !strings.Contains(in, want) {
			t.Errorf("input JSON missing %q: %s", want, in)
		}
	}
}
