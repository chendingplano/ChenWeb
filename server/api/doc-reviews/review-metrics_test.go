package docreviews

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

type fakeReviewLogsStore struct {
	logs []ReviewLogEntry
}

func (f *fakeReviewLogsStore) SaveLogs(_ context.Context, logs []ReviewLogEntry) (int64, error) {
	f.logs = append(f.logs, logs...)
	return int64(len(logs)), nil
}

func (f *fakeReviewLogsStore) DeleteLogs(_ context.Context, _ int64) (int64, error) {
	return 0, nil
}

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

	// Branch A: live hybrid-search hits for M1 (index 0), keyed by doc metric index.
	hybridMatches := map[int][]docprocessing.OnTheFlySemanticMatch{
		0: {
			{ArtifactID: "2_m_9", RecordID: 2, RRFScore: 0.9},
			// Also reached via the object-anchor branch below -> must dedup.
			{ArtifactID: "3_m_7", RecordID: 3, RRFScore: 0.2},
			// Same-document hit -> excluded by add().
			{ArtifactID: "1_m_5", RecordID: recordID, RRFScore: 0.5},
		},
	}
	resolved := map[string]resolvedMetric{
		"2_m_9": {view: metricView{MetricID: "2_m_9", Categories: []string{"pressure"}}, recordID: 2},
		"3_m_7": {view: metricView{MetricID: "3_m_7", Categories: []string{"pressure"}}, recordID: 3},
		"1_m_5": {view: metricView{MetricID: "1_m_5"}, recordID: recordID}, // same doc
	}
	objectMatches := map[int][]resolvedMetric{
		0: {
			// Branch C: object-anchor -> metric 3_m_7 for M1.
			{view: metricView{MetricID: "3_m_7", Categories: []string{"pressure"}}, recordID: 3},
			// Same-document object peer -> excluded by add().
			{view: metricView{MetricID: "1_m_5"}, recordID: recordID},
		},
	}
	siblings := []resolvedMetric{
		// Branch B: cross-doc metric sharing "temp" with M2.
		{view: metricView{MetricID: "4_m_2", Categories: []string{"temp"}}, recordID: 4},
	}

	// No cap first: verify branch coverage + dedup + exclusion + source ordering.
	got := assembleMatches(recordID, docMetrics, hybridMatches, objectMatches, resolved, siblings, 0)

	// M1 (index 0): 3_m_7 (A+C deduped, object_anchor provenance wins) then
	// 2_m_9 (A); 1_m_5 excluded. Object-anchored matches sort before hybrid.
	if len(got[0]) != 2 {
		t.Fatalf("M1 matches = %d, want 2 (%v)", len(got[0]), got[0])
	}
	for _, m := range got[0] {
		if m.recordID == recordID {
			t.Errorf("M1 match %s is same-document, should be excluded", m.view.MetricID)
		}
	}
	if got[0][0].view.MetricID != "3_m_7" || got[0][0].via != "object_anchor" {
		t.Errorf("M1 first match = %s via %s, want 3_m_7 via object_anchor", got[0][0].view.MetricID, got[0][0].via)
	}
	if got[0][1].view.MetricID != "2_m_9" || got[0][1].via != "hybrid_search" {
		t.Errorf("M1 second match = %s via %s, want 2_m_9 via hybrid_search", got[0][1].view.MetricID, got[0][1].via)
	}

	// M2 (index 1): 4_m_2 via category branch.
	if len(got[1]) != 1 || got[1][0].view.MetricID != "4_m_2" || got[1][0].via != "metric_category" {
		t.Fatalf("M2 matches = %v, want [4_m_2 via metric_category]", got[1])
	}

	// Cap = 1: M1 keeps only the highest-priority match (object-anchored 3_m_7).
	capped := assembleMatches(recordID, docMetrics, hybridMatches, objectMatches, resolved, siblings, 1)
	if len(capped[0]) != 1 || capped[0][0].view.MetricID != "3_m_7" {
		t.Fatalf("capped M1 = %v, want [3_m_7]", capped[0])
	}
}

func TestAssembleMatches_SourcePriorityOrdering(t *testing.T) {
	const recordID = int64(1)
	docMetrics := []docMetric{
		dm(11, "1_m_1", "pressure"), // index 0
	}

	// One match from every branch, hybrid added with the highest confidence to
	// prove source priority outranks confidence.
	hybridMatches := map[int][]docprocessing.OnTheFlySemanticMatch{
		0: {
			{ArtifactID: "2_m_1", RecordID: 2, RRFScore: 0.9},
			{ArtifactID: "5_m_2", RecordID: 5, RRFScore: 0.4},
		},
	}
	resolved := map[string]resolvedMetric{
		"2_m_1": {view: metricView{MetricID: "2_m_1"}, recordID: 2},
		"5_m_2": {view: metricView{MetricID: "5_m_2"}, recordID: 5},
	}
	objectMatches := map[int][]resolvedMetric{
		0: {{view: metricView{MetricID: "3_m_1"}, recordID: 3}},
	}
	siblings := []resolvedMetric{
		{view: metricView{MetricID: "4_m_1", Categories: []string{"pressure"}}, recordID: 4},
	}

	got := assembleMatches(recordID, docMetrics, hybridMatches, objectMatches, resolved, siblings, 0)

	want := []struct{ id, via string }{
		{"3_m_1", "object_anchor"},
		{"4_m_1", "metric_category"},
		{"2_m_1", "hybrid_search"}, // hybrid hits keep confidence-desc order
		{"5_m_2", "hybrid_search"},
	}
	if len(got[0]) != len(want) {
		t.Fatalf("matches = %d, want %d (%v)", len(got[0]), len(want), got[0])
	}
	for i, w := range want {
		if got[0][i].view.MetricID != w.id || got[0][i].via != w.via {
			t.Errorf("match[%d] = %s via %s, want %s via %s",
				i, got[0][i].view.MetricID, got[0][i].via, w.id, w.via)
		}
	}
}

func TestAssembleMatches_BranchA_DedupAndExclusion(t *testing.T) {
	const recordID = int64(1)
	docMetrics := []docMetric{
		dm(11, "1_m_1"), // index 0
	}

	// A single live hybrid search per doc metric returns a flat, direction-free hit list.
	// Duplicate ids collapse (first-seen wins) and same-document hits are excluded.
	hybridMatches := map[int][]docprocessing.OnTheFlySemanticMatch{
		0: {
			{ArtifactID: "2_m_9", RecordID: 2, RRFScore: 0.7},
			{ArtifactID: "5_m_3", RecordID: 5, RRFScore: 0.9},
			{ArtifactID: "2_m_9", RecordID: 2, RRFScore: 0.4},        // duplicate -> collapse
			{ArtifactID: "1_m_8", RecordID: recordID, RRFScore: 0.6}, // same doc -> excluded
		},
	}
	resolved := map[string]resolvedMetric{
		"2_m_9": {view: metricView{MetricID: "2_m_9"}, recordID: 2},
		"5_m_3": {view: metricView{MetricID: "5_m_3"}, recordID: 5},
		"1_m_8": {view: metricView{MetricID: "1_m_8"}, recordID: recordID}, // same doc
	}

	got := assembleMatches(recordID, docMetrics, hybridMatches, nil, resolved, nil, 0)

	// Expect 2_m_9 (deduped) and 5_m_3; 1_m_8 excluded as same-document.
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
		t.Errorf("M1 matches missing expected ids: %v", ids)
	}
}

func TestReviewMetric_PayloadAndFindingTagging(t *testing.T) {
	t.Setenv("MAX_MATCHES_TO_LLM", "3")

	fake := &fakeJSONExtractor{
		out: map[string]any{
			"findings": []any{
				map[string]any{"title": "Conflict", "description": "values differ"},
			},
		},
	}
	logStore := &fakeReviewLogsStore{}
	r := &metricsReviewer{
		client:   fake,
		logger:   loggerutil.CreateDefaultLogger("TEST_METRICS"),
		runID:    28,
		logStore: logStore,
	}
	doc := docMetric{
		id:    7,
		view:  metricView{MetricID: "1_m_1", MetricName: "压力", Value: "1.6", Unit: "MPa"},
		spans: []string{"12:14"},
	}
	matchIDs := []string{"m1", "m2", "m3", "m4", "m5"}
	ms := make([]matchedMetric, len(matchIDs))
	for i, matchID := range matchIDs {
		ms[i] = matchedMetric{
			view:     metricView{MetricID: matchID, Value: "2.5", Unit: "MPa", SourceLineSpans: []string{"30:32"}},
			recordID: 2,
			filename: "other.pdf",
			via:      "hybrid_search",
			context: []map[string]any{
				{"line_number": 20, "content": "context before"},
				{"line_number": 30, "content": "matched metric line"},
			},
			confidence: 0.9 - float64(i)/10,
		}
	}

	findings := r.reviewMetric(context.Background(), 1, 0, ReviewerConfig{
		ModelName:  "metric-model",
		PromptText: "compare metrics",
		PromptRef:  "prompt-review-metrics-v1.md",
	}, doc, ms, "", false)

	if len(findings) != 1 {
		t.Fatalf("findings = %d, want 1", len(findings))
	}
	f := findings[0]
	if f.Pass != "P5" || f.Aspect != "metrics" {
		t.Errorf("pass/aspect = %q/%q, want P5/metrics", f.Pass, f.Aspect)
	}
	if f.ArtifactID != "1_m_1" {
		t.Errorf("artifact_id = %q, want 1_m_1", f.ArtifactID)
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
	var payload struct {
		Matches []struct {
			Metric struct {
				MetricID string `json:"metric_id"`
			} `json:"metric"`
		} `json:"matching_metrics"`
	}
	if err := json.Unmarshal([]byte(in), &payload); err != nil {
		t.Fatalf("decode input JSON: %v", err)
	}
	if len(payload.Matches) != 3 {
		t.Fatalf("matching_metrics = %d, want 3: %s", len(payload.Matches), in)
	}
	for i, match := range payload.Matches {
		if want := matchIDs[i]; match.Metric.MetricID != want {
			t.Errorf("matching_metrics[%d].metric.metric_id = %q, want %q", i, match.Metric.MetricID, want)
		}
	}
	for _, want := range []string{"metric_under_review", "matching_metrics", "m1", "hybrid_search", "source_line_spans", "source_context", "matched metric line"} {
		if !strings.Contains(in, want) {
			t.Errorf("input JSON missing %q: %s", want, in)
		}
	}
	if len(logStore.logs) != 1 {
		t.Fatalf("review logs = %d, want 1", len(logStore.logs))
	}
	logEntry := logStore.logs[0]
	if logEntry.RunID != 28 || logEntry.InputRecordID != 1 {
		t.Fatalf("log run/record = %d/%d, want 28/1", logEntry.RunID, logEntry.InputRecordID)
	}
	if logEntry.Aspect != "metrics" || logEntry.UnitType != "metric" || logEntry.UnitKey != "1_m_1#7" {
		t.Fatalf("log aspect/unit = %q/%q/%q", logEntry.Aspect, logEntry.UnitType, logEntry.UnitKey)
	}
	if logEntry.Outcome != "findings_emitted" {
		t.Fatalf("log outcome = %q, want findings_emitted", logEntry.Outcome)
	}
	if len(logEntry.Findings) != 1 || logEntry.Findings[0].Title != "Conflict" {
		t.Fatalf("log findings = %+v", logEntry.Findings)
	}
	if len(logEntry.MatchedUnits) != 5 {
		t.Fatalf("log matched_units = %d, want all 5 retrieved matches", len(logEntry.MatchedUnits))
	}
}

func TestReviewMetric_LogsNoIssue(t *testing.T) {
	fake := &fakeJSONExtractor{
		out: map[string]any{
			"findings": []any{},
		},
	}
	logStore := &fakeReviewLogsStore{}
	r := &metricsReviewer{
		client:   fake,
		logger:   loggerutil.CreateDefaultLogger("TEST_METRICS"),
		runID:    29,
		logStore: logStore,
	}
	doc := docMetric{
		id:    8,
		view:  metricView{MetricID: "1_m_2", MetricName: "温度"},
		spans: []string{"20:21"},
	}
	ms := []matchedMetric{
		{view: metricView{MetricID: "2_m_4", Value: "90", Unit: "C"}, recordID: 2, filename: "other.pdf", via: "metric_category"},
	}

	findings := r.reviewMetric(context.Background(), 1, 0, ReviewerConfig{
		ModelName:  "metric-model",
		PromptText: "compare metrics",
		PromptRef:  "prompt-review-metrics-v1.md",
	}, doc, ms, "", false)

	if len(findings) != 0 {
		t.Fatalf("findings = %d, want 0", len(findings))
	}
	if len(logStore.logs) != 1 {
		t.Fatalf("review logs = %d, want 1", len(logStore.logs))
	}
	if logStore.logs[0].Outcome != "no_issue" {
		t.Fatalf("log outcome = %q, want no_issue", logStore.logs[0].Outcome)
	}
}

func TestReviewMetric_ReturnsAnalysesAsFindings(t *testing.T) {
	fake := &fakeJSONExtractor{
		out: map[string]any{
			"findings": []any{},
			"analyses": []any{
				map[string]any{
					"related_artifact_id": "2_m_9",
					"related_record_id":   float64(2),
					"relationship":        "same_consistent",
					"summary":             "Same quantity, same conditions, unit-equivalent values, no conflict.",
				},
			},
		},
	}
	r := &metricsReviewer{
		client: fake,
		logger: loggerutil.CreateDefaultLogger("TEST_METRICS"),
	}
	doc := docMetric{
		id:    7,
		view:  metricView{MetricID: "1_m_1"},
		spans: []string{"20:24"},
	}

	findings := r.reviewMetric(context.Background(), 1, 0, ReviewerConfig{
		ModelName:  "metric-model",
		PromptText: "compare metrics",
		PromptRef:  "prompt-review-metrics-v4.md",
	}, doc, nil, "", false)

	if len(findings) != 1 {
		t.Fatalf("findings = %d, want 1 analysis finding: %+v", len(findings), findings)
	}
	f := findings[0]
	if f.Pass != "P5" || f.Aspect != "metrics" || f.ArtifactID != "1_m_1" {
		t.Fatalf("analysis identity = pass:%q aspect:%q artifact:%q, want P5/metrics/1_m_1", f.Pass, f.Aspect, f.ArtifactID)
	}
	if f.FindingType != "analysis" || f.Severity != "info" {
		t.Fatalf("analysis type/severity = %q/%q, want analysis/info", f.FindingType, f.Severity)
	}
	if f.RelatedArtifactID != "2_m_9" || f.RelatedRecordID != 2 {
		t.Fatalf("analysis related = %q/%d, want 2_m_9/2", f.RelatedArtifactID, f.RelatedRecordID)
	}
	if f.Location != "20:24" {
		t.Fatalf("analysis location = %q, want 20:24", f.Location)
	}
	if !strings.Contains(f.Title, "1_m_1") || !strings.Contains(f.Title, "2_m_9") {
		t.Fatalf("analysis title = %q, want both metric ids", f.Title)
	}
	if f.Description != "Same quantity, same conditions, unit-equivalent values, no conflict." {
		t.Fatalf("analysis description = %q", f.Description)
	}
	if f.ResultKind != "metric_analysis" || f.AnalysisRelationship != "same_consistent" {
		t.Fatalf("analysis metadata = result_kind:%q relationship:%q, want metric_analysis/same_consistent", f.ResultKind, f.AnalysisRelationship)
	}
}

func TestParseMetricAnalysesJSON(t *testing.T) {
	payload := map[string]any{
		"findings": []any{},
		"analyses": []any{
			map[string]any{
				"related_artifact_id": "2_m_9",
				"related_record_id":   float64(2),
				"relationship":        "same_consistent",
				"summary":             "Identical value, no conflict.",
			},
			map[string]any{
				"relationship": "unrelated",
				"summary":      "",
			},
		},
	}

	got := parseMetricAnalysesJSON(payload)
	if len(got) != 1 {
		t.Fatalf("analyses = %d, want 1 (empty summary should be skipped): %+v", len(got), got)
	}
	a := got[0]
	if a.RelatedArtifactID != "2_m_9" || a.RelatedRecordID != 2 || a.Relationship != "same_consistent" {
		t.Errorf("analysis = %+v, want related_artifact_id=2_m_9 related_record_id=2 relationship=same_consistent", a)
	}
}

func TestParseMetricAnalysesJSON_NoAnalysesKey(t *testing.T) {
	if got := parseMetricAnalysesJSON(map[string]any{"findings": []any{}}); got != nil {
		t.Errorf("analyses = %+v, want nil when key absent", got)
	}
}

func TestBuildReviewers_MetricsUsesDocReviewerMaxTasks(t *testing.T) {
	t.Setenv("MAX_DOC_REVIEWER_TASKS", "4")

	p := &ReviewProcessor{
		MetricsClient:     &fakeJSONExtractor{},
		MetricsModelName:  "metric-model",
		MetricsPromptRef:  "prompt-review-metrics-v2.md",
		MetricsPromptText: "compare metrics",
		MaxConcurrent:     1,
	}

	runners := p.buildReviewers(DocMetadataInputRecord{})
	for _, runner := range runners {
		if reviewer, ok := runner.reviewer.(*metricsReviewer); ok {
			if reviewer.maxTasks != 4 {
				t.Fatalf("metrics reviewer maxTasks = %d, want 4", reviewer.maxTasks)
			}
			return
		}
	}
	t.Fatal("metrics reviewer not found")
}

func TestBuildReviewers_ArtifactReviewersUseDocReviewerMaxTasks(t *testing.T) {
	t.Setenv("MAX_DOC_REVIEWER_TASKS", "5")

	p := &ReviewProcessor{
		MetricsClient:                 &fakeJSONExtractor{},
		MetricsModelName:              "metric-model",
		MetricsPromptRef:              "prompt-review-metrics-v2.md",
		MetricsPromptText:             "compare metrics",
		ProvisionsClient:              &fakeJSONExtractor{},
		ProvisionsModelName:           "provision-model",
		ProvisionsPromptRef:           "prompt-review-provisions-v2.md",
		ProvisionsPromptText:          "compare provisions",
		EntitiesClient:                &fakeJSONExtractor{},
		EntitiesModelName:             "entity-model",
		EntitiesPromptRef:             "prompt-review-entities-v1.md",
		EntitiesPromptText:            "compare entities",
		InventoryItemsClient:          &fakeJSONExtractor{},
		InventoryItemsModelName:       "inventory-model",
		InventoryItemsPromptRef:       "prompt-review-inventory-items-v2.md",
		InventoryItemsPromptText:      "compare items",
		MetricsCompletenessClient:     &fakeJSONExtractor{},
		MetricsCompletenessModelName:  "metric-completeness-model",
		MetricsCompletenessPromptRef:  "prompt-review-metrics-missing-v1.md",
		MetricsCompletenessPromptText: "check missing metrics",
		MaxConcurrent:                 1,
	}

	want := map[string]bool{
		"metrics":              false,
		"provisions":           false,
		"entities":             false,
		"inventory_items":      false,
		"metrics_completeness": false,
	}
	runners := p.buildReviewers(DocMetadataInputRecord{})
	for _, runner := range runners {
		switch reviewer := runner.reviewer.(type) {
		case *metricsReviewer:
			if reviewer.maxTasks != 5 {
				t.Fatalf("metrics maxTasks = %d, want 5", reviewer.maxTasks)
			}
			want["metrics"] = true
		case *provisionsReviewer:
			if reviewer.maxTasks != 5 {
				t.Fatalf("provisions maxTasks = %d, want 5", reviewer.maxTasks)
			}
			want["provisions"] = true
		case *entitiesReviewer:
			if reviewer.maxTasks != 5 {
				t.Fatalf("entities maxTasks = %d, want 5", reviewer.maxTasks)
			}
			want["entities"] = true
		case *inventoryItemsReviewer:
			if reviewer.maxTasks != 5 {
				t.Fatalf("inventory_items maxTasks = %d, want 5", reviewer.maxTasks)
			}
			want["inventory_items"] = true
		case *metricsCompletenessReviewer:
			if reviewer.maxTasks != 5 {
				t.Fatalf("metrics_completeness maxTasks = %d, want 5", reviewer.maxTasks)
			}
			want["metrics_completeness"] = true
		}
	}
	for aspect, seen := range want {
		if !seen {
			t.Fatalf("%s reviewer not found", aspect)
		}
	}
}

func TestMetricLogUnitKey_IncludesRowID(t *testing.T) {
	got := metricLogUnitKey(docMetric{
		id:   42,
		view: metricView{MetricID: "416_mtc_20"},
	})
	if got != "416_mtc_20#42" {
		t.Fatalf("metricLogUnitKey = %q, want %q", got, "416_mtc_20#42")
	}

	got = metricLogUnitKey(docMetric{id: 99})
	if got != "99" {
		t.Fatalf("metricLogUnitKey fallback = %q, want %q", got, "99")
	}
}

func TestArtifactSourceContextLines(t *testing.T) {
	var lines []Line
	for i := 1; i <= 50; i++ {
		lines = append(lines, Line{LineNo: i, Content: "line"})
	}

	got := artifactSourceContextLines(lines, []string{"20:22", "40"})
	if len(got) != 22 {
		t.Fatalf("context lines = %d, want 22", len(got))
	}
	if got[0]["line_number"] != 10 {
		t.Fatalf("first line = %v, want 10", got[0]["line_number"])
	}
	if got[len(got)-1]["line_number"] != 40 {
		t.Fatalf("last line = %v, want actual second span line 40", got[len(got)-1]["line_number"])
	}
	seen22 := false
	for _, row := range got {
		if row["line_number"] == 22 {
			seen22 = true
		}
	}
	if !seen22 {
		t.Fatalf("context did not include actual range line 22: %v", got)
	}
}
