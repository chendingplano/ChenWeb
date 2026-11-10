package docreviews

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
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

// testChunks mirrors record 416's first saved chunks: each chunk is its overlap
// lines (the previous chunk's tail) followed by its regular lines.
func testChunks() []docprocessing.Chunk {
	mk := func(seq int, overlap, regular [2]int) docprocessing.Chunk {
		c := docprocessing.Chunk{SeqNo: seq}
		add := func(r [2]int, mark string) {
			for n := r[0]; n > 0 && n <= r[1]; n++ {
				c.Lines = append(c.Lines, docprocessing.MarkedLine{
					Line: docprocessing.Line{LineNo: n, LineType: "paragraph", Content: "line"},
					Mark: mark,
				})
			}
		}
		add(overlap, "o")
		add(regular, "n")
		return c
	}
	return []docprocessing.Chunk{
		mk(1, [2]int{0, 0}, [2]int{1, 26}),
		mk(2, [2]int{22, 26}, [2]int{27, 50}),
		mk(3, [2]int{50, 50}, [2]int{51, 69}),
	}
}

func TestBuildArtifactSourceChunksRanges(t *testing.T) {
	chunks := buildArtifactSourceChunks(testChunks(), "Doc | NO 1")
	c := chunks[1]
	if c.startLine != 22 || c.endLine != 50 || c.regularStart != 27 || c.regularEnd != 50 {
		t.Fatalf("chunk 2 ranges = %d-%d regular %d-%d, want 22-50 regular 27-50",
			c.startLine, c.endLine, c.regularStart, c.regularEnd)
	}
	if !strings.HasPrefix(c.text, "Document: Doc | NO 1\n[22] line\n") || !strings.HasSuffix(c.text, "[50] line") {
		t.Fatalf("chunk 2 text is not the compact passage of lines 22-50:\n%s", c.text)
	}
}

func TestChunkForSpans(t *testing.T) {
	chunks := buildArtifactSourceChunks(testChunks(), "")
	cases := []struct {
		name          string
		spans         []string
		wantIdx       int
		wantTruncated bool
	}{
		{"inside one chunk's regular lines", []string{"43"}, 1, false},
		{"multiple spans use earliest start", []string{"45", "30-31"}, 1, false},
		{"in the overlap of the next chunk", []string{"50-52"}, 2, false},
		{"spans run past every chunk", []string{"40-60"}, 1, true},
		{"overlap line owned by the earlier chunk", []string{"24"}, 0, false},
		{"unparseable spans", []string{"abc"}, -1, false},
		{"no spans", nil, -1, false},
		{"start beyond all chunks", []string{"999"}, -1, false},
	}
	for _, c := range cases {
		idx, truncated := chunkForSpans(c.spans, chunks)
		if idx != c.wantIdx || truncated != c.wantTruncated {
			t.Errorf("%s: chunkForSpans(%v) = (%d,%v), want (%d,%v)",
				c.name, c.spans, idx, truncated, c.wantIdx, c.wantTruncated)
		}
	}
	if idx, _ := chunkForSpans([]string{"12"}, nil); idx != -1 {
		t.Errorf("no chunks: got %d, want -1", idx)
	}
}

func TestSourceChunkForReturnsPassage(t *testing.T) {
	chunks := buildArtifactSourceChunks(testChunks(), "")
	idx, passage, truncated := sourceChunkFor([]string{"43"}, chunks)
	if idx != 1 || passage != chunks[1].text || truncated {
		t.Fatalf("sourceChunkFor = (%d, %q…, %v), want chunk 1's passage", idx, passage[:min(len(passage), 20)], truncated)
	}
	if idx, passage, _ := sourceChunkFor([]string{"abc"}, chunks); idx != -1 || passage != "" {
		t.Fatalf("unresolved spans: got (%d, %q), want (-1, \"\")", idx, passage)
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

// One seed per chunk must start before any sibling of that chunk; siblings
// (and chunkless units) run in the remainder phase while seeds still run.
func TestRunArtifactUnitsChunkGroupedSeedPerChunk(t *testing.T) {
	t.Setenv("LLM_CALL_STAGGER", "0")

	seedsRelease := make(chan struct{})
	seedStarted := make(chan struct{}, 2)
	var seedStarts int32
	remainderStarted := make(chan int, 3)

	mkSeedRun := func() func(context.Context) []ReviewFinding {
		return func(context.Context) []ReviewFinding {
			atomic.AddInt32(&seedStarts, 1)
			seedStarted <- struct{}{}
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
		{chunkIdx: 0, run: mkSeedRun()},        // seed of chunk 0
		{chunkIdx: 0, run: mkRemainderRun(1)},  // sibling of chunk 0
		{chunkIdx: 1, run: mkSeedRun()},        // seed of chunk 1
		{chunkIdx: 1, run: mkRemainderRun(3)},  // sibling of chunk 1
		{chunkIdx: -1, run: mkRemainderRun(4)}, // chunkless → remainder
	}

	done := make(chan struct{})
	var findings []ReviewFinding
	var runErr error
	go func() {
		findings, runErr = runArtifactUnitsChunkGrouped(
			context.Background(),
			4,
			units,
			ReviewerConfig{MaxFindings: 99, MaxAnalyses: 99, ReviewDepth: 1},
			"artifact_test",
			&captureLogger{},
			0,
			nil,
		)
		close(done)
	}()

	// Wait for both seed callbacks to enter. Goroutine launch order is not a
	// scheduling guarantee when the configured stagger is zero.
	for i := range 2 {
		select {
		case <-seedStarted:
		case <-time.After(2 * time.Second):
			t.Fatalf("seed unit %d did not start", i+1)
		}
	}

	// All three remainder units must start while both seeds are still blocked.
	for i := range 3 {
		select {
		case <-remainderStarted:
		case <-time.After(2 * time.Second):
			t.Fatalf("remainder unit %d did not start while seeds were running", i+1)
		}
	}
	if got := atomic.LoadInt32(&seedStarts); got != 2 {
		t.Fatalf("seed starts = %d, want 2 (exactly one per chunk)", got)
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
func TestRunArtifactUnitsChunkGroupedProgress(t *testing.T) {
	t.Setenv("LLM_CALL_STAGGER", "0")

	var mu sync.Mutex
	var snapshots []ReviewerProgress
	onProgress := func(p ReviewerProgress) {
		mu.Lock()
		snapshots = append(snapshots, p)
		mu.Unlock()
	}

	units := []artifactReviewUnit{
		{chunkIdx: 0, run: func(context.Context) []ReviewFinding { return []ReviewFinding{{Title: "a"}} }},
		{chunkIdx: 0, run: func(context.Context) []ReviewFinding { return nil }},
	}
	findings, err := runArtifactUnitsChunkGrouped(
		context.Background(),
		2,
		units,
		ReviewerConfig{MaxFindings: 99, MaxAnalyses: 99, ReviewDepth: 1},
		"artifact_test",
		&captureLogger{},
		0,
		onProgress,
	)
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

func TestRunArtifactUnitsChunkGroupedLimitSkipsRemainders(t *testing.T) {
	t.Setenv("LLM_CALL_STAGGER", "1")

	logger := &captureLogger{}
	started := make(chan int, 4)
	seed0Done := make(chan struct{})
	seed2Release := make(chan struct{})
	var mu sync.Mutex
	var snapshots []ReviewerProgress
	errCh := make(chan error, 1)
	findingsCh := make(chan []ReviewFinding, 1)

	go func() {
		findings, err := runArtifactUnitsChunkGrouped(
			context.Background(),
			2,
			[]artifactReviewUnit{
				{
					chunkIdx: 0,
					run: func(context.Context) []ReviewFinding {
						started <- 0
						close(seed0Done)
						return []ReviewFinding{{Title: "seed0", FindingType: "issue"}}
					},
				},
				{
					chunkIdx: 0,
					run: func(context.Context) []ReviewFinding {
						started <- 1
						return []ReviewFinding{{Title: "remainder1"}}
					},
				},
				{
					chunkIdx: 1,
					run: func(context.Context) []ReviewFinding {
						started <- 2
						<-seed2Release
						return nil
					},
				},
				{
					chunkIdx: 1,
					run: func(context.Context) []ReviewFinding {
						started <- 3
						return []ReviewFinding{{Title: "remainder3"}}
					},
				},
			},
			ReviewerConfig{MaxFindings: 1, MaxAnalyses: 10, ReviewDepth: 2},
			"metrics",
			logger,
			42,
			func(p ReviewerProgress) {
				mu.Lock()
				snapshots = append(snapshots, p)
				mu.Unlock()
			},
		)
		findingsCh <- findings
		errCh <- err
	}()

	<-seed0Done
	select {
	case extra := <-started:
		if extra == 1 || extra == 3 {
			t.Fatalf("remainder unit %d started before seed 2 released", extra)
		}
	case <-time.After(50 * time.Millisecond):
	}
	close(seed2Release)

	findings := <-findingsCh
	err := <-errCh
	if err != nil {
		t.Fatalf("runArtifactUnitsChunkGrouped err = %v, want nil", err)
	}
	if len(findings) != 1 {
		t.Fatalf("findings len = %d, want 1", len(findings))
	}
	mu.Lock()
	defer mu.Unlock()
	if len(snapshots) == 0 {
		t.Fatal("expected progress snapshots")
	}
	last := snapshots[len(snapshots)-1]
	if last.CompletedUnits != 4 || last.TotalUnits != 4 || last.FindingCount != 1 {
		t.Fatalf("final progress = %+v, want completed=4 total=4 findings=1", last)
	}

	entry := findLogEntry(logger.entries, "warn", outputLimitWarningMessage)
	if entry == nil {
		t.Fatal("expected output limit warning")
	}
	got := logArgsToMap(entry.args)
	want := map[string]any{
		"reviewer":      "metrics",
		"review_depth":  2,
		"max_findings":  1,
		"max_analyses":  10,
		"findings":      1,
		"analyses":      0,
		"skipped_units": 2,
		"record_id":     int64(42),
	}
	for k, v := range want {
		if !reflect.DeepEqual(got[k], v) {
			t.Fatalf("warning arg %q = %#v, want %#v (all args=%v)", k, got[k], v, got)
		}
	}
	if skipped, ok := got["skipped_unit_indexes"].([]int); !ok || !reflect.DeepEqual(skipped, []int{1, 3}) {
		t.Fatalf("skipped_unit_indexes = %#v, want [1 3]", got["skipped_unit_indexes"])
	}
}

// One-shot layout: with a source chunk, the compact passage is the document
// input and the rubric + per-artifact payload form the task text; without a
// chunk the payload is the document input.
func TestReviewMetricSourceFirstLayout(t *testing.T) {
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
	passage := "Document: t\n[195] 设计压力 1.6 MPa"

	r.reviewMetric(context.Background(), 1, 0, ReviewerConfig{
		ModelName:  "m",
		PromptText: "RUBRIC",
		PromptRef:  "prompt-review-metrics-v2.md",
	}, doc, ms, passage, true)

	if fake.inputTexts[0] != passage {
		t.Errorf("input text = %q, want the source passage", fake.inputTexts[0])
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

	// Without a chunk the payload itself is the document input.
	fake2 := &fakeJSONExtractor{out: map[string]any{"findings": []any{}}}
	r.client = fake2
	r.reviewMetric(context.Background(), 1, 0, ReviewerConfig{
		ModelName:  "m",
		PromptText: "RUBRIC",
		PromptRef:  "prompt-review-metrics-v2.md",
	}, doc, ms, "", false)
	if !strings.Contains(fake2.inputTexts[0], "metric_under_review") {
		t.Errorf("chunkless input text should be the payload, got %q", fake2.inputTexts[0])
	}
	if fake2.promptTexts[0] != "RUBRIC" {
		t.Errorf("chunkless prompt should be the bare rubric, got %q", fake2.promptTexts[0])
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
	findings := normalizeFindingsJSON(payload, "test")
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
