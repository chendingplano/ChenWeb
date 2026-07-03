package docreviews

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

func TestParseArtifactSpan(t *testing.T) {
	cases := []struct {
		in         string
		start, end int
	}{
		{"14", 14, 14},
		{"14-16", 14, 16},
		{"14:16", 14, 16},
		{" 14 - 16 ", 14, 16},
		{"16-14", 0, 0}, // end < start
		{"abc", 0, 0},
		{"", 0, 0},
		{"-3", 0, 0},
	}
	for _, c := range cases {
		s, e := parseArtifactSpan(c.in)
		if s != c.start || e != c.end {
			t.Errorf("parseArtifactSpan(%q) = (%d,%d), want (%d,%d)", c.in, s, e, c.start, c.end)
		}
	}
}

func testWindows() []chunkInput {
	return []chunkInput{
		{inputJSON: "w0", startLine: 1, endLine: 200},
		{inputJSON: "w1", startLine: 201, endLine: 400},
		{inputJSON: "w2", startLine: 401, endLine: 600},
	}
}

func TestWindowIndexForSpans(t *testing.T) {
	windows := testWindows()
	cases := []struct {
		name  string
		spans []string
		want  int
	}{
		{"inside first window", []string{"12-14"}, 0},
		{"inside middle window", []string{"250:260"}, 1},
		{"boundary-crossing maps to span-start window", []string{"195-210"}, 0},
		{"multiple spans use earliest start", []string{"450", "205-207"}, 1},
		{"unparseable spans", []string{"abc"}, -1},
		{"no spans", nil, -1},
		{"start beyond all windows", []string{"999"}, -1},
	}
	for _, c := range cases {
		if got := windowIndexForSpans(c.spans, windows); got != c.want {
			t.Errorf("%s: windowIndexForSpans(%v) = %d, want %d", c.name, c.spans, got, c.want)
		}
	}
	if got := windowIndexForSpans([]string{"12"}, nil); got != -1 {
		t.Errorf("empty windows: got %d, want -1", got)
	}
}

func TestSpansTruncatedByWindow(t *testing.T) {
	w := chunkInput{startLine: 1, endLine: 200}
	if spansTruncatedByWindow([]string{"12-14"}, w) {
		t.Error("span fully inside window reported truncated")
	}
	if !spansTruncatedByWindow([]string{"195-210"}, w) {
		t.Error("span crossing window end not reported truncated")
	}
	if spansTruncatedByWindow([]string{"abc"}, w) {
		t.Error("unparseable span reported truncated")
	}
}

func TestDocAuthorityClass(t *testing.T) {
	cases := []struct {
		docNo, title, filename, want string
	}{
		{"GB/T 12237", "", "", "standard"},
		{"", "", "GB_50316_pipe_design.pdf", "standard"},
		{"ISO 9001:2015", "", "", "standard"},
		{"", "管道设计标准", "", "standard"},
		{"", "特种设备安全监察条例", "", "regulation"},
		{"", "EU Machinery Directive", "", "regulation"},
		{"", "中华人民共和国安全生产法", "", "regulation"},
		{"", "Acme internal pipe spec", "acme_spec.pdf", "peer_document"},
		{"", "", "", "peer_document"},
	}
	for _, c := range cases {
		if got := docAuthorityClass(c.docNo, c.title, c.filename); got != c.want {
			t.Errorf("docAuthorityClass(%q,%q,%q) = %q, want %q", c.docNo, c.title, c.filename, got, c.want)
		}
	}
}

// One seed per window must start before any sibling of that window; siblings
// (and windowless units) run in the remainder phase while seeds still run.
func TestRunArtifactUnitsWindowGroupedSeedPerWindow(t *testing.T) {
	t.Setenv("LLM_CALL_STAGGER", "0")

	seedsRelease := make(chan struct{})
	var seedStarts int32
	remainderStarted := make(chan int, 3)

	mkSeedRun := func() func(context.Context) []ReviewFinding {
		return func(context.Context) []ReviewFinding {
			atomic.AddInt32(&seedStarts, 1)
			<-seedsRelease
			return []ReviewFinding{{Title: "seed"}}
		}
	}
	mkRemainderRun := func(id int) func(context.Context) []ReviewFinding {
		return func(context.Context) []ReviewFinding {
			remainderStarted <- id
			return []ReviewFinding{{Title: "sibling"}}
		}
	}

	units := []artifactReviewUnit{
		{windowIdx: 0, run: mkSeedRun()},        // seed of window 0
		{windowIdx: 0, run: mkRemainderRun(1)},  // sibling of window 0
		{windowIdx: 1, run: mkSeedRun()},        // seed of window 1
		{windowIdx: 1, run: mkRemainderRun(3)},  // sibling of window 1
		{windowIdx: -1, run: mkRemainderRun(4)}, // windowless → remainder
	}

	done := make(chan struct{})
	var findings []ReviewFinding
	var runErr error
	go func() {
		findings, runErr = runArtifactUnitsWindowGrouped(context.Background(), 4, units, nil)
		close(done)
	}()

	// All three remainder units must start while both seeds are still blocked.
	for i := range 3 {
		select {
		case <-remainderStarted:
		case <-time.After(2 * time.Second):
			t.Fatalf("remainder unit %d did not start while seeds were running", i+1)
		}
	}
	if got := atomic.LoadInt32(&seedStarts); got != 2 {
		t.Fatalf("seed starts = %d, want 2 (exactly one per window)", got)
	}

	close(seedsRelease)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runner did not finish after seeds were released")
	}
	if runErr != nil {
		t.Fatalf("err = %v, want nil", runErr)
	}
	if len(findings) != 5 {
		t.Fatalf("findings = %d, want 5", len(findings))
	}
}

// Progress is reported once per unit with the aggregate finding count.
func TestRunArtifactUnitsWindowGroupedProgress(t *testing.T) {
	t.Setenv("LLM_CALL_STAGGER", "0")

	var mu sync.Mutex
	var snapshots []ReviewerProgress
	onProgress := func(p ReviewerProgress) {
		mu.Lock()
		snapshots = append(snapshots, p)
		mu.Unlock()
	}

	units := []artifactReviewUnit{
		{windowIdx: 0, run: func(context.Context) []ReviewFinding { return []ReviewFinding{{Title: "a"}} }},
		{windowIdx: 0, run: func(context.Context) []ReviewFinding { return nil }},
	}
	findings, err := runArtifactUnitsWindowGrouped(context.Background(), 2, units, onProgress)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("findings = %d, want 1", len(findings))
	}
	mu.Lock()
	defer mu.Unlock()
	if len(snapshots) != 2 {
		t.Fatalf("progress snapshots = %d, want 2", len(snapshots))
	}
	last := snapshots[len(snapshots)-1]
	if last.CompletedUnits != 2 || last.TotalUnits != 2 || last.FindingCount != 1 {
		t.Fatalf("final snapshot = %+v, want completed=2 total=2 findings=1", last)
	}
}

// AR2 one-shot layout: with a window, the window JSON is the document input and
// the rubric + per-artifact payload form the task text; without a window the
// payload is the document input (pre-AR2 layout).
func TestReviewMetricWindowFirstLayout(t *testing.T) {
	fake := &fakeJSONExtractor{out: map[string]any{"findings": []any{}}}
	r := &metricsReviewer{
		client: fake,
		logger: loggerutil.CreateDefaultLogger("TEST_METRICS_AR2"),
	}
	doc := docMetric{
		view:  metricView{MetricID: "1_m_1", Value: "1.6", Unit: "MPa"},
		spans: []string{"195-210"},
	}
	ms := []matchedMetric{
		{view: metricView{MetricID: "2_m_9"}, recordID: 2, filename: "GB_50316.pdf", via: "hybrid_search"},
	}
	windowJSON := `{"doc_context":"t","lines":[]}`

	r.reviewMetric(context.Background(), 1, 0, ReviewerConfig{
		ModelName:  "m",
		PromptText: "RUBRIC",
		PromptRef:  "prompt-review-metrics-v2.md",
	}, doc, ms, windowJSON, true)

	if fake.inputTexts[0] != windowJSON {
		t.Errorf("input text = %q, want the window JSON", fake.inputTexts[0])
	}
	prompt := fake.promptTexts[0]
	if !strings.HasPrefix(prompt, "RUBRIC") {
		t.Errorf("task text must start with the rubric, got %q", prompt[:min(len(prompt), 40)])
	}
	for _, want := range []string{"metric_under_review", "artifact_line_spans", "context_truncated", "source_doc_authority", "match_rank"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("task text missing %q", want)
		}
	}
	if strings.Contains(prompt, `"confidence":0.`) {
		t.Errorf("task text still carries raw match confidence: %s", prompt)
	}

	// Without a window the payload itself is the document input.
	fake2 := &fakeJSONExtractor{out: map[string]any{"findings": []any{}}}
	r.client = fake2
	r.reviewMetric(context.Background(), 1, 0, ReviewerConfig{
		ModelName:  "m",
		PromptText: "RUBRIC",
		PromptRef:  "prompt-review-metrics-v2.md",
	}, doc, ms, "", false)
	if !strings.Contains(fake2.inputTexts[0], "metric_under_review") {
		t.Errorf("windowless input text should be the payload, got %q", fake2.inputTexts[0])
	}
	if fake2.promptTexts[0] != "RUBRIC" {
		t.Errorf("windowless prompt should be the bare rubric, got %q", fake2.promptTexts[0])
	}
	if strings.Contains(fake2.inputTexts[0], "context_truncated") {
		t.Errorf("context_truncated must be omitted when spans are not truncated")
	}
}

// AR5 §6: findings parse the structured cross-reference fields.
func TestNormalizeFindingsJSONRelatedArtifact(t *testing.T) {
	payload := map[string]any{
		"findings": []any{
			map[string]any{
				"title":               "Conflict",
				"description":         "values differ",
				"finding_type":        "issue",
				"related_artifact_id": "2002_m_3",
				"related_record_id":   float64(2002),
			},
		},
	}
	findings := normalizeFindingsJSON(payload)
	if len(findings) != 1 {
		t.Fatalf("findings = %d, want 1", len(findings))
	}
	if findings[0].RelatedArtifactID != "2002_m_3" || findings[0].RelatedRecordID != 2002 {
		t.Fatalf("related = %q/%d, want 2002_m_3/2002", findings[0].RelatedArtifactID, findings[0].RelatedRecordID)
	}
}

// The related cross-reference round-trips through the metadata envelope and is
// not misread as a language-code translation key.
func TestFindingMetadataEnvelopeRelatedRoundTrip(t *testing.T) {
	env := FindingMetadataEnvelope{
		I18N:              FindingI18NMetadata{SchemaVersion: 1},
		RelatedArtifactID: "2002_prv_9",
		RelatedRecordID:   2002,
	}
	b, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back FindingMetadataEnvelope
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.RelatedArtifactID != "2002_prv_9" || back.RelatedRecordID != 2002 {
		t.Fatalf("round-trip related = %q/%d", back.RelatedArtifactID, back.RelatedRecordID)
	}
	if len(back.I18N.Translations) != 0 {
		t.Fatalf("related keys leaked into translations: %v", back.I18N.Translations)
	}
}

// get_artifact_context validates its arguments and reports unknown ids as
// found=false rather than an error.
func TestGetArtifactContextToolArgsAndUnknownID(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	tool := buildToolRegistry(db)["get_artifact_context"]
	if tool.Execute == nil {
		t.Fatal("get_artifact_context missing from registry")
	}

	if _, err := tool.Execute(context.Background(), 1, map[string]any{"artifact_id": "x"}); err == nil {
		t.Error("missing record_id must error")
	}
	if _, err := tool.Execute(context.Background(), 1, map[string]any{"record_id": float64(2)}); err == nil {
		t.Error("missing artifact_id must error")
	}

	mock.ExpectQuery("SELECT 'metric'").
		WithArgs(int64(2), "nope").
		WillReturnRows(sqlmock.NewRows([]string{"artifact_type", "spans"}))
	out, err := tool.Execute(context.Background(), 1, map[string]any{"record_id": float64(2), "artifact_id": "nope"})
	if err != nil {
		t.Fatalf("unknown id: err = %v, want found=false result", err)
	}
	m, ok := out.(map[string]any)
	if !ok || m["found"] != false {
		t.Fatalf("unknown id result = %v, want found=false", out)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock expectations: %v", err)
	}
}
