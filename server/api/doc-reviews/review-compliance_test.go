package docreviews

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/llmusage"
	"github.com/chendingplano/deepdoc/server/api/searchengine"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

// ---- fakes ----

// complianceFakeModel behaves like the shared structured client for one model: it gets the answer
// from answer, runs the contract's check, and returns the same error kinds.
type complianceFakeModel struct {
	mu     sync.Mutex
	name   string
	answer func(stage, input string) (map[string]any, error)
	inputs map[string][]string // stage -> inputs received
}

func (f *complianceFakeModel) ExtractJSON(context.Context, llmclients.JSONExtractionInput) (map[string]any, error) {
	return nil, errors.New("not used")
}

func (f *complianceFakeModel) ExtractStructuredJSON(_ context.Context, in llmclients.JSONExtractionInput, contract llmclients.StructuredOutputContract) (*llmclients.StructuredOutputResult, error) {
	stage := strings.TrimPrefix(in.CallReason, "review_compliance_")
	f.mu.Lock()
	if f.inputs == nil {
		f.inputs = map[string][]string{}
	}
	f.inputs[stage] = append(f.inputs[stage], in.InputText)
	f.mu.Unlock()
	payload, err := f.answer(stage, in.InputText)
	if err != nil {
		return nil, &llmclients.StructuredOutputError{Kind: llmclients.ErrStructuredOutputProvider, Err: err}
	}
	raw, _ := json.Marshal(payload)
	if contract.CheckParsed != nil {
		if cerr := contract.CheckParsed(payload); cerr != nil {
			inner := &llmclients.StructuredOutputError{Kind: llmclients.ErrStructuredOutputCheckFailed, Err: cerr, Raw: string(raw)}
			return nil, &llmclients.StructuredOutputError{Kind: llmclients.ErrStructuredOutputRetriesExhausted, Err: inner, Raw: string(raw)}
		}
	}
	return &llmclients.StructuredOutputResult{Parsed: payload, Raw: string(raw)}, nil
}

func (f *complianceFakeModel) calls(stage string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.inputs[stage]...)
}

type complianceFakeStore struct {
	missing  []string
	rows     []complianceArtifactRow
	mentions map[int64][]complianceMention
}

func (s *complianceFakeStore) MissingProcessors(context.Context, int64) ([]string, error) {
	return s.missing, nil
}
func (s *complianceFakeStore) ArtifactRows(context.Context, int64) ([]complianceArtifactRow, error) {
	return append([]complianceArtifactRow(nil), s.rows...), nil
}
func (s *complianceFakeStore) Mentions(_ context.Context, id int64) ([]complianceMention, error) {
	return s.mentions[id], nil
}

type complianceFakeRelevance struct {
	mu       sync.Mutex
	probs    map[string]float64
	err      error
	failures int // the next failures requests fail
	asked    [][]string
}

func (d *complianceFakeRelevance) Model() string { return "decision-model" }
func (d *complianceFakeRelevance) Decide(_ context.Context, req complianceRelevanceRequest) (map[string]float64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	var keys []string
	for _, c := range req.Candidates {
		keys = append(keys, c.Key)
	}
	d.asked = append(d.asked, keys)
	if d.failures > 0 {
		d.failures--
		return nil, errors.New("connection reset by peer")
	}
	if d.err != nil {
		return nil, d.err
	}
	out := map[string]float64{}
	for _, k := range keys {
		if p, ok := d.probs[k]; ok {
			out[k] = p
		} else {
			out[k] = 0.9
		}
	}
	return out, nil
}

// ---- helpers ----

func complianceChunk(start int, contents ...string) chunkInput {
	rows := [][]any{{"flag", "line_number", "page_number", "line_type", "content"}}
	for i, c := range contents {
		rows = append(rows, []any{"n", start + i, 1, "paragraph", c})
	}
	b, _ := json.Marshal(map[string]any{"doc_context": "D", "lines": rows})
	return chunkInput{inputJSON: string(b), startLine: start, endLine: start + len(contents) - 1}
}

func complianceResult(typ, id string, record int64, text string) searchengine.Result {
	return searchengine.Result{ArtifactType: typ, ArtifactID: id, InputRecordID: record, Snippet: text, SourceTitle: "Doc " + id}
}

type complianceTestRig struct {
	r        *complianceReviewer
	primary  *complianceFakeModel
	corr     *complianceFakeModel
	fall     *complianceFakeModel
	store    *complianceFakeStore
	rel      *complianceFakeRelevance
	mu       sync.Mutex
	searches []searchengine.Request
	failed   []llmusage.FailedCall
}

func newComplianceTestRig(t *testing.T, answer func(stage, input string) (map[string]any, error), results func(req searchengine.Request) []searchengine.Result) *complianceTestRig {
	t.Helper()
	rig := &complianceTestRig{
		primary: &complianceFakeModel{name: "primary", answer: answer},
		corr:    &complianceFakeModel{name: "correction", answer: answer},
		fall:    &complianceFakeModel{name: "fallback", answer: answer},
		store:   &complianceFakeStore{},
		rel:     &complianceFakeRelevance{probs: map[string]float64{}},
	}
	rig.r = &complianceReviewer{
		client:     rig.primary,
		correction: complianceTry{client: rig.corr, model: "correction-model", label: "correction"},
		fallback:   complianceTry{client: rig.fall, model: "fallback-model", label: "fallback"},
		relevance:  rig.rel,
		logger:     loggerutil.CreateDefaultLogger("TEST_COMPLIANCE"),
		store:      rig.store,
		prompts: map[string]compliancePrompt{
			"doc_activity": {Text: "doc", Ref: "prompt-doc.md"}, "relevance": {Text: "rel", Ref: "prompt-rel.md"},
			"judgement": {Text: "judge", Ref: "prompt-judge.md"}, "verify": {Text: "verify", Ref: "prompt-verify.md"},
			"correction": {Text: "Problems:\n{{errors}}\nPrevious answer:\n{{previous_answer}}", Ref: "prompt-correction.md"},
		},
		settings:       complianceSettingsFrom(ReviewAspectConfig{}),
		assessmentDate: "2026-11-22",
		search: func(_ context.Context, req searchengine.Request) (*searchengine.Response, error) {
			rig.mu.Lock()
			rig.searches = append(rig.searches, req)
			rig.mu.Unlock()
			return &searchengine.Response{Results: results(req)}, nil
		},
		recordFailed: func(_ context.Context, call llmusage.FailedCall) {
			rig.mu.Lock()
			rig.failed = append(rig.failed, call)
			rig.mu.Unlock()
		},
	}
	return rig
}

var complianceTestCfg = ReviewerConfig{ModelName: "primary-model", PromptText: "detect", PromptRef: "prompt-detect.md"}

func noActivities(stage, _ string) (map[string]any, error) {
	if stage == "detect" || stage == "doc_activity" {
		return map[string]any{"activities": []any{}}, nil
	}
	return nil, fmt.Errorf("unexpected stage %s", stage)
}

// judgeAll returns a verdict for every activity–candidate pair listed in a judgement input.
func judgeAll(input string, verdict func(activityID, key string) map[string]any) map[string]any {
	var in struct {
		Activities []struct {
			ID   string   `json:"activity_id"`
			Keys []string `json:"candidate_keys"`
		} `json:"activities"`
	}
	_ = json.Unmarshal([]byte(input), &in)
	var out []any
	for _, a := range in.Activities {
		for _, k := range a.Keys {
			v := map[string]any{"activity_id": a.ID, "artifact_key": k, "verdict": "compatible", "binding": "mandatory", "summary": "s"}
			if verdict != nil {
				for kk, vv := range verdict(a.ID, k) {
					v[kk] = vv
				}
			}
			out = append(out, v)
		}
	}
	return map[string]any{"verdicts": out}
}

func findingOfType(fs []ReviewFinding, typ string) (ReviewFinding, bool) {
	for _, f := range fs {
		if f.FindingType == typ {
			return f, true
		}
	}
	return ReviewFinding{}, false
}

// ---- prerequisites and configuration ----

func TestCompliancePrerequisiteMissingStopsReview(t *testing.T) {
	rig := newComplianceTestRig(t, noActivities, func(searchengine.Request) []searchengine.Result { return nil })
	rig.store.missing = []string{complianceProvisionsProcessor}
	_, err := rig.r.processChunk(context.Background(), 7, 0, complianceTestCfg, complianceChunk(1, "a"))
	if err == nil || !strings.Contains(err.Error(), "extract_provisions") {
		t.Fatalf("err = %v, want it to name extract_provisions", err)
	}
	if len(rig.primary.calls("detect")) != 0 {
		t.Error("no model call may run when a prerequisite is missing")
	}
}

func TestComplianceConfigErrorStopsEveryChunk(t *testing.T) {
	rig := newComplianceTestRig(t, noActivities, func(searchengine.Request) []searchengine.Result { return nil })
	rig.r.configErr = errors.New("compliance: configuration incomplete: DOC_REVIEWER_RELEVANCE_MODEL_NAME is not set")
	if _, err := rig.r.processChunk(context.Background(), 7, 3, complianceTestCfg, complianceChunk(1, "a")); err == nil || !strings.Contains(err.Error(), "DOC_REVIEWER_RELEVANCE_MODEL_NAME") {
		t.Fatalf("err = %v", err)
	}
}

func TestComplianceConfigureNamesEverythingMissing(t *testing.T) {
	for _, env := range []string{envComplianceCorrectionModel, envComplianceFallbackModel, envComplianceRelevanceModel} {
		t.Setenv(env, "")
	}
	r := &complianceReviewer{client: &complianceFakeModel{}, logger: loggerutil.CreateDefaultLogger("TEST_COMPLIANCE")}
	err := r.configure(ReviewAspectConfig{})
	if err == nil {
		t.Fatal("configure accepted an empty configuration")
	}
	for _, want := range []string{"doc_activity_prompt", "relevance_prompt", "judgement_prompt", "verifier_prompt", "correction_prompt",
		envComplianceCorrectionModel, envComplianceFallbackModel} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not name %s: %v", want, err)
		}
	}
}

// The shipped doc-review.local.toml names an existing prompt file for every compliance prompt key,
// and the relevance and correction prompts hold their placeholders.
func TestShippedConfigNamesEveryCompliancePrompt(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	raw, err := os.ReadFile(filepath.Join(root, "doc-review.local.toml"))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := parseDocReviewConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	rc := cfg.Reviewers["compliance"]
	keys := map[string]string{"prompt": rc.Prompt, "doc_activity_prompt": rc.DocActivityPrompt, "relevance_prompt": rc.RelevancePrompt,
		"judgement_prompt": rc.JudgementPrompt, "verifier_prompt": rc.VerifierPrompt, "correction_prompt": rc.CorrectionPrompt}
	for key, file := range keys {
		if file == "" {
			t.Errorf("[reviewers.compliance] %s is not set", key)
			continue
		}
		text, err := os.ReadFile(filepath.Join(root, "prompts", file))
		if err != nil {
			t.Errorf("[reviewers.compliance] %s: %v", key, err)
			continue
		}
		switch key {
		case "relevance_prompt":
			if _, q, ok := strings.Cut(string(text), complianceQuestionHeading); !ok || !strings.Contains(q, complianceQuestionKey) {
				t.Errorf("%s has no question section with %s", file, complianceQuestionKey)
			}
		case "correction_prompt":
			if !strings.Contains(string(text), "{{errors}}") || !strings.Contains(string(text), "{{previous_answer}}") {
				t.Errorf("%s lacks {{errors}} or {{previous_answer}}", file)
			}
		}
	}
	s := complianceSettingsFrom(rc)
	if s.TopMetrics != 30 || s.TopProvisions != 30 || s.TopArtifacts != 30 || s.MaxCitedDocuments != 0 || s.MaxMetrics != 100 || s.MaxProvisions != 100 {
		t.Errorf("shipped settings = %+v", s)
	}
}

func TestParseDocReviewConfigRejectsBadComplianceSizes(t *testing.T) {
	if _, err := parseDocReviewConfig([]byte("[reviewers.compliance]\ntop_metrics = 0\n")); err == nil || !strings.Contains(err.Error(), "top_metrics") {
		t.Errorf("top_metrics = 0: err = %v", err)
	}
	if _, err := parseDocReviewConfig([]byte("[reviewers.compliance]\nmax_metrics = -1\n")); err == nil || !strings.Contains(err.Error(), "max_metrics") {
		t.Errorf("max_metrics = -1: err = %v", err)
	}
	cfg, err := parseDocReviewConfig([]byte("[reviewers.compliance]\nmax_cited_documents = 2\nmax_provisions = 0\n"))
	if err != nil {
		t.Fatal(err)
	}
	if s := complianceSettingsFrom(cfg.Reviewers["compliance"]); s.MaxCitedDocuments != 2 || s.MaxProvisions != 0 || s.TopArtifacts != 30 {
		t.Errorf("settings = %+v", s)
	}
}

// ---- activities ----

func TestBuildCompliancePlanMergesCopiesAndAppliesLimits(t *testing.T) {
	rows := []complianceArtifactRow{
		{Kind: "metric", RowID: 3, ArtifactID: "9_mtc_3", Label: "temperature", LineStart: 20, LineEnd: 20, DupKey: "Temperature|5-10|°C|"},
		{Kind: "metric", RowID: 1, ArtifactID: "9_mtc_1", Label: "temperature", LineStart: 10, LineEnd: 10, DupKey: "temperature | 5-10 | °C |"},
		{Kind: "metric", RowID: 2, ArtifactID: "9_mtc_2", Label: "pressure", LineStart: 12, LineEnd: 12, DupKey: "pressure|2|kPa|"},
		{Kind: "metric", RowID: 4, ArtifactID: "9_mtc_4", Label: "no lines", LineStart: 0, DupKey: "x"},
		{Kind: "provision", RowID: 5, ArtifactID: "9_prv_1", Label: "seal", LineStart: 10, LineEnd: 11, DupKey: "The container shall be sealed."},
		{Kind: "provision", RowID: 6, ArtifactID: "9_prv_2", Label: "label", LineStart: 30, LineEnd: 30, DupKey: "Label it."},
	}
	mentions := []complianceMention{{Name: "GB/T 1234", LineStart: 11, LineEnd: 11, TargetRecordID: 55}}
	p := buildCompliancePlan(rows, mentions, complianceSettings{MaxMetrics: 1, MaxProvisions: 1})
	if len(p.Members) != 2 {
		t.Fatalf("members = %+v, want the first metric and the first provision", p.Members)
	}
	m := p.Members[0]
	if m.Kind != "metric" || m.RowID != 1 || len(m.Copies) != 1 || m.Copies[0].RowID != 3 {
		t.Errorf("first metric = %+v, want row 1 with row 3 as its copy", m)
	}
	if p.Members[1].Kind != "provision" || p.Members[1].RowID != 5 || len(p.Members[1].CitedDocs) != 1 || p.Members[1].CitedDocs[0].RecordID != 55 {
		t.Errorf("provision = %+v, want row 5 citing record 55", p.Members[1])
	}
	if p.UncheckedMetrics != 1 || p.UncheckedProvisions != 1 || p.WithoutLines != 1 {
		t.Errorf("unchecked metrics=%d provisions=%d without lines=%d, want 1 1 1", p.UncheckedMetrics, p.UncheckedProvisions, p.WithoutLines)
	}
}

func TestBuildComplianceUnitsGroupsOverlappingLines(t *testing.T) {
	members := []*complianceMember{
		{Kind: "provision", Label: "p", LineStart: 5, LineEnd: 6},
		{Kind: "metric", Label: "m1", LineStart: 6, LineEnd: 6},
		{Kind: "metric", Label: "m2", LineStart: 9, LineEnd: 9},
	}
	texts := []complianceActivity{{Activity: "t", SourceText: "x", LineStart: 7, LineEnd: 7}}
	units := buildComplianceUnits(members, texts)
	if len(units) != 3 {
		t.Fatalf("units = %d, want group(p,m1), text, m2", len(units))
	}
	if len(units[0].Members) != 2 || units[0].Members[0].ID != "a1" || units[0].Members[1].ID != "a2" || units[0].LineStart != 5 || units[0].LineEnd != 6 {
		t.Errorf("group = %+v", units[0])
	}
	if units[1].Members[0].Kind != "text" || units[2].Members[0].Label != "m2" {
		t.Errorf("unit order = %s, %s", units[1].Members[0].Label, units[2].Members[0].Label)
	}
}

func TestComplianceDetectSeesMetricsAndDropsCoveredTextActivities(t *testing.T) {
	answer := func(stage, input string) (map[string]any, error) {
		switch stage {
		case "detect":
			return map[string]any{"activities": []any{
				map[string]any{"activity": "covered", "line_start": 3.0, "line_end": 4.0, "object": "o"},
				map[string]any{"activity": "kept", "line_start": 6.0, "line_end": 6.0, "object": "o"},
			}}, nil
		case "judgement":
			return judgeAll(input, nil), nil
		}
		return nil, fmt.Errorf("unexpected stage %s", stage)
	}
	rig := newComplianceTestRig(t, answer, func(req searchengine.Request) []searchengine.Result {
		return []searchengine.Result{complianceResult("provision", "2_prv_1", 2, "Containers shall be sealed.")}
	})
	rig.store.rows = []complianceArtifactRow{{Kind: "provision", RowID: 1, ArtifactID: "7_prv_1", Label: "seal", Text: "seal text", LineStart: 3, LineEnd: 4, DupKey: "seal"}}
	fs, err := rig.r.processChunk(context.Background(), 7, 1, complianceTestCfg, complianceChunk(1, "l1", "l2", "l3", "l4", "l5", "l6"))
	if err != nil {
		t.Fatal(err)
	}
	if in := rig.primary.calls("detect"); len(in) != 1 || !strings.Contains(in[0], `"existing_activities":[{`) || !strings.Contains(in[0], `"kind":"provision"`) {
		t.Errorf("detect input = %v, want existing_activities with the provision", in)
	}
	var labels []string
	for _, f := range fs {
		labels = append(labels, f.Title)
	}
	joined := strings.Join(labels, " | ")
	if strings.Contains(joined, "covered") || !strings.Contains(joined, "kept") || !strings.Contains(joined, "seal") {
		t.Errorf("findings = %s, want the provision and the uncovered text activity only", joined)
	}
}

func TestComplianceDocumentActivityComesFromFirstChunk(t *testing.T) {
	answer := func(stage, input string) (map[string]any, error) {
		switch stage {
		case "doc_activity":
			return map[string]any{"activities": []any{map[string]any{"activity": "specify", "line_start": 1.0, "line_end": 1.0, "object": "heat pump"}}}, nil
		case "detect":
			return map[string]any{"activities": []any{}}, nil
		}
		return nil, fmt.Errorf("unexpected stage %s", stage)
	}
	rig := newComplianceTestRig(t, answer, func(searchengine.Request) []searchengine.Result { return nil })
	fs, err := rig.r.processChunk(context.Background(), 7, 0, complianceTestCfg, complianceChunk(1, "Heat pump standard"))
	if err != nil {
		t.Fatal(err)
	}
	f, ok := findingOfType(fs, "compliance_no_grounding")
	if !ok || !strings.Contains(f.Title, "document: heat pump") {
		t.Fatalf("findings = %+v, want the document activity with no grounding", fs)
	}
}

func TestParseComplianceActivitiesReportsEveryProblem(t *testing.T) {
	in := complianceChunk(5, "A", "B")
	_, err := parseComplianceActivities(map[string]any{"activities": []any{
		map[string]any{"activity": "", "line_start": 5.0, "line_end": 5.0},
		map[string]any{"activity": "x", "line_start": 50.0, "line_end": 50.0},
		map[string]any{"activity": "y", "source_text": "not there", "line_start": 5.0, "line_end": 5.0},
	}}, false, in)
	if err == nil {
		t.Fatal("malformed activities accepted")
	}
	for _, want := range []string{"activity 1", "activity 2", "activity 3"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not report %s: %v", want, err)
		}
	}
}

func TestComplianceDerivedEvidenceFromCompactInput(t *testing.T) {
	acts, err := parseComplianceActivities(map[string]any{"activities": []any{
		map[string]any{"activity": "x", "line_start": 5.0, "line_end": 6.0},
	}}, false, complianceChunk(5, "A", "B"))
	if err != nil || len(acts) != 1 || acts[0].SourceText != "A\nB" {
		t.Fatalf("derived evidence = %+v, err %v", acts, err)
	}
	bare := `[["flag","line_number","page_number","line_type","content"],["n",402,44,"paragraph","A"],["n",403,44,"paragraph","B"]]`
	acts, err = parseComplianceActivities(map[string]any{"activities": []any{
		map[string]any{"activity": "x", "line_start": 402.0, "line_end": 403.0},
	}}, false, chunkInput{inputJSON: bare})
	if err != nil || len(acts) != 1 || acts[0].SourceText != "A\nB" {
		t.Fatalf("bare compact input = %+v, err %v", acts, err)
	}
}

func TestParseComplianceQueryBilingualKeywords(t *testing.T) {
	q, ok := parseComplianceQuery([]any{map[string]any{"en": "log", "zh": "日志"}, "retention", map[string]any{"en": "log"}})
	if !ok || q.Text != "log 日志 retention" || len(q.Keywords) != 3 {
		t.Fatalf("query = %+v ok=%v", q, ok)
	}
}

// ---- search ----

func TestComplianceSearchesEachKindWithItsOwnLimit(t *testing.T) {
	answer := func(stage, input string) (map[string]any, error) {
		switch stage {
		case "detect":
			return map[string]any{"activities": []any{map[string]any{"activity": "store fish", "line_start": 3.0, "line_end": 3.0, "object": "fish",
				"search_queries": []any{[]any{"fish", "鱼"}}, "cited_docs": []any{"GB/T 1234"}}}}, nil
		case "judgement":
			return judgeAll(input, nil), nil
		}
		return nil, fmt.Errorf("unexpected stage %s", stage)
	}
	rig := newComplianceTestRig(t, answer, func(req searchengine.Request) []searchengine.Result {
		return []searchengine.Result{complianceResult(req.Filters.ArtifactTypes[0], "2_x_1", 2, "text")}
	})
	rig.r.settings.TopMetrics, rig.r.settings.TopArtifacts = 7, 11
	rig.r.duplicateIDs = func(_ context.Context, id int64) ([]int64, error) { return []int64{id, 198}, nil }
	rig.store.rows = []complianceArtifactRow{{Kind: "metric", RowID: 1, ArtifactID: "7_mtc_1", Label: "temperature", LineStart: 1, LineEnd: 1, DupKey: "t"}}
	fs, err := rig.r.processChunk(context.Background(), 7, 1, complianceTestCfg, complianceChunk(1, "t", "x", "fish are kept in tanks"))
	if err != nil {
		t.Fatal(err)
	}
	if len(rig.searches) != 2 {
		t.Fatalf("searches = %d, want one per activity (cited documents are off by default)", len(rig.searches))
	}
	for _, req := range rig.searches {
		if len(req.Filters.ExcludeRecordIDs) != 2 || req.Filters.ExcludeRecordIDs[1] != 198 {
			t.Errorf("exclude = %v, want the record and its copy", req.Filters.ExcludeRecordIDs)
		}
		switch {
		case req.Query.Ref != nil:
			if req.Query.Ref.ID != "7_mtc_1" || strings.Join(req.Filters.ArtifactTypes, ",") != "metric" || req.Options.Limit != 7 {
				t.Errorf("metric search = %+v", req)
			}
		default:
			if strings.Join(req.Filters.ArtifactTypes, ",") != "provision,metric,chunk" || req.Options.Limit != 11 || len(req.Criteria.Keywords) != 2 {
				t.Errorf("text search = %+v", req)
			}
		}
	}
	for _, f := range fs {
		if strings.Contains(f.Title, "store fish") && (len(f.CitedDocuments) != 1 || f.CitedDocuments[0] != "GB/T 1234") {
			t.Errorf("cited documents = %v, want [GB/T 1234]", f.CitedDocuments)
		}
	}
	var meta struct {
		Trace []complianceRetrievalTrace `json:"retrieval_trace"`
	}
	f, _ := findingOfType(fs, "compliance_assessment")
	_ = json.Unmarshal(f.ArtifactFields, &meta)
	if len(meta.Trace) == 0 || len(meta.Trace[0].ExcludedRecordIDs) != 2 || meta.Trace[0].Limit == 0 {
		t.Errorf("stored trace = %+v", meta.Trace)
	}
}

func TestComplianceCitedDocumentsSearchedWithinTheLimit(t *testing.T) {
	answer := func(stage, input string) (map[string]any, error) {
		if stage == "judgement" {
			return judgeAll(input, nil), nil
		}
		return map[string]any{"activities": []any{}}, nil
	}
	rig := newComplianceTestRig(t, answer, func(req searchengine.Request) []searchengine.Result {
		if len(req.Filters.RecordIDs) == 1 {
			return []searchengine.Result{complianceResult("provision", "55_prv_9", 55, "cited")}
		}
		r := complianceResult("provision", "2_prv_1", 2, "found")
		r.SourceLineSpans = json.RawMessage(`["40-41"]`)
		return []searchengine.Result{r}
	})
	rig.r.settings.MaxCitedDocuments = 1
	rig.store.rows = []complianceArtifactRow{{Kind: "provision", RowID: 1, ArtifactID: "7_prv_1", Label: "seal", LineStart: 1, LineEnd: 1, DupKey: "s"}}
	// The candidate cites record 55 on its own lines; the activity cites nothing.
	rig.store.mentions = map[int64][]complianceMention{2: {{Name: "GB/T 5", LineStart: 41, LineEnd: 41, TargetRecordID: 55}}}
	fs, err := rig.r.processChunk(context.Background(), 7, 1, complianceTestCfg, complianceChunk(1, "seal"))
	if err != nil {
		t.Fatal(err)
	}
	if len(rig.searches) != 2 || len(rig.searches[1].Filters.RecordIDs) != 1 || rig.searches[1].Filters.RecordIDs[0] != 55 {
		t.Fatalf("searches = %+v, want the activity search then one search of record 55", rig.searches)
	}
	f, _ := findingOfType(fs, "compliance_assessment")
	if !strings.Contains(f.Description, "[provision 55_prv_9]") {
		t.Errorf("the referenced document's passage was not judged: %s", f.Description)
	}
}

// ---- relevance, judgement, numbers, verification ----

func TestComplianceRelevanceRemovesUnrelatedCandidates(t *testing.T) {
	var judged string
	answer := func(stage, input string) (map[string]any, error) {
		switch stage {
		case "detect":
			return map[string]any{"activities": []any{}}, nil
		case "judgement":
			judged = input
			return judgeAll(input, nil), nil
		}
		return nil, fmt.Errorf("unexpected stage %s", stage)
	}
	rig := newComplianceTestRig(t, answer, func(req searchengine.Request) []searchengine.Result {
		return []searchengine.Result{complianceResult("provision", "2_prv_1", 2, "sealed containers"), complianceResult("provision", "3_prv_1", 3, "frozen fish labels")}
	})
	rig.rel.probs["provision/3_prv_1"] = 0.1
	rig.store.rows = []complianceArtifactRow{{Kind: "provision", RowID: 1, ArtifactID: "7_prv_1", Label: "seal", LineStart: 1, LineEnd: 1, DupKey: "s"}}
	fs, err := rig.r.processChunk(context.Background(), 7, 1, complianceTestCfg, complianceChunk(1, "seal"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(judged, "3_prv_1") || !strings.Contains(judged, "2_prv_1") {
		t.Errorf("judgement input = %s, want only the relevant candidate", judged)
	}
	f, _ := findingOfType(fs, "compliance_assessment")
	if strings.Contains(f.Description, "3_prv_1") {
		t.Errorf("a removed candidate is listed: %s", f.Description)
	}
	var meta struct {
		Relevance []complianceRelevanceDecision `json:"relevance"`
	}
	_ = json.Unmarshal(f.ArtifactFields, &meta)
	if len(meta.Relevance) != 2 || meta.Relevance[1].Kept {
		t.Errorf("stored decisions = %+v", meta.Relevance)
	}

	// When nothing is relevant the finding says no candidate was found.
	rig.rel.probs["provision/2_prv_1"] = 0.05
	rig.r.plans = sync.Map{}
	fs, _ = rig.r.processChunk(context.Background(), 7, 1, complianceTestCfg, complianceChunk(1, "seal"))
	if _, ok := findingOfType(fs, "compliance_no_grounding"); !ok {
		t.Errorf("findings = %+v, want no grounding", fs)
	}
}

func TestComplianceJudgementCheckReportsEveryProblem(t *testing.T) {
	s := &complianceUnitState{
		unit: &complianceUnit{Members: []*complianceMember{{ID: "a1"}, {ID: "a2"}}},
		pool: map[string]*complianceCandidate{"provision/1": {Key: "provision/1", Text: "Logs shall be kept for six months."}, "metric/2": {Key: "metric/2", Text: "t ≤ 5 mm"}},
		kept: map[string][]string{"a1": {"provision/1", "metric/2"}, "a2": {"provision/1"}},
	}
	lines := []complianceInputLine{{LineNumber: 3, Content: "Logs are kept for 30 days."}}
	pairs := []string{"a1\x00provision/1", "a1\x00metric/2", "a2\x00provision/1"}
	_, err := checkComplianceJudgement(map[string]any{"verdicts": []any{
		map[string]any{"activity_id": "a1", "artifact_key": "provision/1", "verdict": "not_compliant", "binding": "mandatory", "summary": "s",
			"candidate_quote": "kept for six months", "document_quote": ""},
		map[string]any{"activity_id": "a1", "artifact_key": "metric/2", "verdict": "compliant", "binding": "recommendation", "summary": "s",
			"candidate_quote": "not in text", "document_quote": "Logs are kept", "document_line_start": 3, "document_line_end": 3},
		map[string]any{"activity_id": "a9", "artifact_key": "provision/1", "verdict": "compatible", "binding": "none", "summary": "s"},
	}}, s, pairs, lines)
	if err == nil {
		t.Fatal("judgement accepted")
	}
	for _, want := range []string{"needs document_quote", "needs binding mandatory", "does not occur in the candidate", "was not asked for", "Activity a2, candidate provision/1: no verdict"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in: %v", want, err)
		}
	}
	// A correct answer passes; quotes may differ in spacing and punctuation.
	v, err := checkComplianceJudgement(map[string]any{"verdicts": []any{
		map[string]any{"activity_id": "a1", "artifact_key": "provision/1", "verdict": "not_compliant", "binding": "mandatory", "summary": "s",
			"candidate_quote": "kept for six months", "document_quote": "Logs are kept for 30 days", "document_line_start": 3, "document_line_end": 3},
		map[string]any{"activity_id": "a1", "artifact_key": "metric/2", "verdict": "unqualified", "summary": "s"},
		map[string]any{"activity_id": "a2", "artifact_key": "provision/1", "verdict": "compatible", "binding": "mandatory", "summary": "s"},
	}}, s, pairs, lines)
	if err != nil || len(v) != 3 {
		t.Fatalf("valid judgement rejected: %v", err)
	}
	// A missing binding is accepted for a verdict that does not need one, and read as none.
	if v[1].Binding != "none" {
		t.Errorf("missing binding = %q, want none", v[1].Binding)
	}
}

func TestComplianceNumbersDecideTheVerdict(t *testing.T) {
	q := func(op, value, unit string) complianceQuantity {
		return complianceQuantity{Operator: op, Value: value, Unit: unit}
	}
	num := func(doc complianceQuantity) *complianceNumeric {
		return &complianceNumeric{Property: "clearance", QuantityKind: "Length", ConditionsMatch: true, MethodsMatch: true, BasisMatch: true,
			Required: q("<=", "5", "mm"), Documented: doc}
	}
	base := complianceVerdict{Verdict: verdictCompatible, Binding: "mandatory", CandidateQuote: "c", DocumentQuote: "d"}
	ok := base
	ok.Numeric = num(q("<=", "0.4", "cm"))
	applyComplianceNumeric(&ok)
	if ok.Verdict != verdictCompliant {
		t.Errorf("stricter limit: %+v", ok)
	}
	bad := base
	bad.Numeric = num(q("<=", "8", "mm"))
	applyComplianceNumeric(&bad)
	if bad.Verdict != verdictNotCompliant {
		t.Errorf("weaker limit: %+v", bad)
	}
	unknown := base
	unknown.Numeric = num(q("<=", "2", "parsec-ish"))
	applyComplianceNumeric(&unknown)
	if unknown.Verdict != verdictUndetermined || unknown.Note == "" {
		t.Errorf("unsupported unit: %+v", unknown)
	}
	rec := base
	rec.Binding = "recommendation"
	rec.Numeric = num(q("<=", "8", "mm"))
	applyComplianceNumeric(&rec)
	if rec.Verdict != verdictCompatible {
		t.Errorf("a recommendation must not be decided by numbers: %+v", rec)
	}
}

func TestComplianceVerifierRejectionLowersVerdict(t *testing.T) {
	answer := func(stage, input string) (map[string]any, error) {
		switch stage {
		case "detect":
			return map[string]any{"activities": []any{}}, nil
		case "judgement":
			return judgeAll(input, func(string, string) map[string]any {
				return map[string]any{"verdict": "not_compliant", "candidate_quote": "six months", "document_quote": "30 days", "document_line_start": 1, "document_line_end": 1}
			}), nil
		case "verify":
			return map[string]any{"checks": []any{map[string]any{"activity_id": "a1", "artifact_key": "provision/2_prv_1", "valid": false, "reason": "different logs"}}}, nil
		}
		return nil, fmt.Errorf("unexpected stage %s", stage)
	}
	rig := newComplianceTestRig(t, answer, func(searchengine.Request) []searchengine.Result {
		return []searchengine.Result{complianceResult("provision", "2_prv_1", 2, "Keep logs for six months.")}
	})
	rig.store.rows = []complianceArtifactRow{{Kind: "provision", RowID: 1, ArtifactID: "7_prv_1", Label: "logs", LineStart: 1, LineEnd: 1, DupKey: "l"}}
	fs, err := rig.r.processChunk(context.Background(), 7, 1, complianceTestCfg, complianceChunk(1, "Keep logs for 30 days."))
	if err != nil {
		t.Fatal(err)
	}
	f, _ := findingOfType(fs, "compliance_assessment")
	if f.Severity != "info" || len(f.RelatedArtifacts) != 1 || f.RelatedArtifacts[0].Relationship != verdictUndetermined || !strings.Contains(f.RelatedArtifacts[0].Note, "different logs") {
		t.Fatalf("finding = %+v", f)
	}
	if !strings.HasPrefix(f.Description, "Candidate verdicts: 1 undetermined") || !strings.Contains(f.Description, "- [provision 2_prv_1] record 2 ") {
		t.Errorf("description = %s", f.Description)
	}
}

func TestComplianceVerifiedNotCompliantIsMedium(t *testing.T) {
	answer := func(stage, input string) (map[string]any, error) {
		switch stage {
		case "detect":
			return map[string]any{"activities": []any{}}, nil
		case "judgement":
			return judgeAll(input, func(string, string) map[string]any {
				return map[string]any{"verdict": "not_compliant", "candidate_quote": "six months", "document_quote": "30 days", "document_line_start": 1, "document_line_end": 1}
			}), nil
		case "verify":
			return map[string]any{"checks": []any{map[string]any{"activity_id": "a1", "artifact_key": "provision/2_prv_1", "valid": true, "reason": "ok"}}}, nil
		}
		return nil, fmt.Errorf("unexpected stage %s", stage)
	}
	rig := newComplianceTestRig(t, answer, func(searchengine.Request) []searchengine.Result {
		return []searchengine.Result{complianceResult("provision", "2_prv_1", 2, "Keep logs for six months.")}
	})
	rig.store.rows = []complianceArtifactRow{{Kind: "provision", RowID: 1, ArtifactID: "7_prv_1", Label: "logs", LineStart: 1, LineEnd: 1, DupKey: "l"}}
	fs, _ := rig.r.processChunk(context.Background(), 7, 1, complianceTestCfg, complianceChunk(1, "Keep logs for 30 days."))
	if f, _ := findingOfType(fs, "compliance_assessment"); f.Severity != "medium" {
		t.Errorf("severity = %q, want medium", f.Severity)
	}
}

// ---- three tries and errors ----

func TestComplianceCorrectionModelFixesAWrongAnswer(t *testing.T) {
	rig := newComplianceTestRig(t, nil, func(searchengine.Request) []searchengine.Result {
		return []searchengine.Result{complianceResult("provision", "2_prv_1", 2, "Keep logs.")}
	})
	rig.primary.answer = func(stage, input string) (map[string]any, error) {
		if stage == "detect" {
			return map[string]any{"activities": []any{}}, nil
		}
		return map[string]any{"verdicts": []any{}}, nil // misses the pair
	}
	rig.corr.answer = func(stage, input string) (map[string]any, error) {
		return judgeAll(strings.SplitN(input, "\n\nProblems:", 2)[0], nil), nil
	}
	rig.store.rows = []complianceArtifactRow{{Kind: "provision", RowID: 1, ArtifactID: "7_prv_1", Label: "logs", LineStart: 1, LineEnd: 1, DupKey: "l"}}
	fs, err := rig.r.processChunk(context.Background(), 7, 1, complianceTestCfg, complianceChunk(1, "Keep logs."))
	if err != nil {
		t.Fatal(err)
	}
	f, ok := findingOfType(fs, "compliance_assessment")
	if !ok || f.ErrorMsg != "" {
		t.Fatalf("findings = %+v", fs)
	}
	in := rig.corr.calls("judgement")
	if len(in) != 1 || !strings.Contains(in[0], "Problems:\nActivity a1, candidate provision/2_prv_1: no verdict") || !strings.Contains(in[0], "Previous answer:\n{\"verdicts\":[]}") {
		t.Errorf("correction request = %v", in)
	}
	if len(rig.failed) != 1 || rig.failed[0].Attempt != 1 || rig.failed[0].Outcome != llmusage.FailedCallRecovered || rig.failed[0].FallbackModel != "correction-model" || rig.failed[0].Aspect != "compliance" {
		t.Errorf("failed calls = %+v", rig.failed)
	}
	if len(rig.fall.calls("judgement")) != 0 {
		t.Error("the fallback model must not be called after the correction model succeeded")
	}
}

func TestComplianceAllTriesFailingRecordsTheErrorAndContinues(t *testing.T) {
	wrong := func(stage, input string) (map[string]any, error) {
		if stage == "detect" {
			return map[string]any{"activities": []any{}}, nil
		}
		return map[string]any{"verdicts": []any{}}, nil
	}
	rig := newComplianceTestRig(t, wrong, func(searchengine.Request) []searchengine.Result {
		return []searchengine.Result{complianceResult("provision", "2_prv_1", 2, "Keep logs.")}
	})
	rig.store.rows = []complianceArtifactRow{
		{Kind: "provision", RowID: 1, ArtifactID: "7_prv_1", Label: "logs", LineStart: 1, LineEnd: 1, DupKey: "l"},
		{Kind: "metric", RowID: 2, ArtifactID: "", Label: "unindexed", LineStart: 2, LineEnd: 2, DupKey: "u"},
	}
	fs, err := rig.r.processChunk(context.Background(), 7, 1, complianceTestCfg, complianceChunk(1, "Keep logs.", "t = 5"))
	if err != nil {
		t.Fatalf("an activity's error must not stop the reviewer: %v", err)
	}
	var incomplete []ReviewFinding
	for _, f := range fs {
		if f.FindingType == "compliance_incomplete" {
			incomplete = append(incomplete, f)
		}
	}
	if len(incomplete) != 2 {
		t.Fatalf("findings = %+v, want two incomplete findings (judgement failed; metric not indexed)", fs)
	}
	for _, f := range incomplete {
		if f.ErrorMsg == "" {
			t.Errorf("incomplete finding without error: %+v", f)
		}
	}
	if len(rig.failed) != 3 || rig.failed[2].Attempt != 3 || rig.failed[2].Model != "fallback-model" || rig.failed[0].Outcome != llmusage.FailedCallFailed {
		t.Errorf("failed calls = %+v", rig.failed)
	}
	// The fallback model gets the same correction request as the correction model.
	if c, fb := rig.corr.calls("judgement"), rig.fall.calls("judgement"); len(c) != 1 || len(fb) != 1 || c[0] != fb[0] {
		t.Errorf("correction and fallback requests differ")
	}
}

func TestComplianceProviderFailureSendsTheOriginalInput(t *testing.T) {
	rig := newComplianceTestRig(t, nil, func(searchengine.Request) []searchengine.Result { return nil })
	rig.primary.answer = func(string, string) (map[string]any, error) { return nil, errors.New("timeout") }
	rig.corr.answer = func(string, string) (map[string]any, error) { return map[string]any{"activities": []any{}}, nil }
	if _, err := rig.r.processChunk(context.Background(), 7, 1, complianceTestCfg, complianceChunk(1, "x")); err != nil {
		t.Fatal(err)
	}
	p, c := rig.primary.calls("detect"), rig.corr.calls("detect")
	if len(p) != 1 || len(c) != 1 || p[0] != c[0] {
		t.Errorf("after a provider failure the correction model must get the original input")
	}
}

func TestComplianceDetectFailureIsRecordedAndMetricsStillReviewed(t *testing.T) {
	answer := func(stage, input string) (map[string]any, error) {
		if stage == "detect" {
			return nil, errors.New("provider down")
		}
		return judgeAll(input, nil), nil
	}
	rig := newComplianceTestRig(t, answer, func(searchengine.Request) []searchengine.Result {
		return []searchengine.Result{complianceResult("metric", "2_mtc_1", 2, "t ≤ 5")}
	})
	rig.store.rows = []complianceArtifactRow{{Kind: "metric", RowID: 1, ArtifactID: "7_mtc_1", Label: "t", LineStart: 1, LineEnd: 1, DupKey: "t"}}
	fs, err := rig.r.processChunk(context.Background(), 7, 1, complianceTestCfg, complianceChunk(1, "t = 4"))
	if err != nil {
		t.Fatal(err)
	}
	if f, ok := findingOfType(fs, "compliance_incomplete"); !ok || !strings.Contains(f.Title, "text activities of lines 1-1") || f.ErrorMsg == "" {
		t.Errorf("findings = %+v", fs)
	}
	if _, ok := findingOfType(fs, "compliance_assessment"); !ok {
		t.Error("the metric was not reviewed")
	}
}

func TestComplianceUncheckedFindingComesWithTheFirstChunk(t *testing.T) {
	rig := newComplianceTestRig(t, noActivities, func(searchengine.Request) []searchengine.Result { return nil })
	rig.r.settings.MaxMetrics = 0
	rig.store.rows = []complianceArtifactRow{{Kind: "metric", RowID: 1, ArtifactID: "7_mtc_1", Label: "t", LineStart: 1, LineEnd: 1, DupKey: "t"}}
	fs, err := rig.r.processChunk(context.Background(), 7, 0, complianceTestCfg, complianceChunk(1, "x"))
	if err != nil {
		t.Fatal(err)
	}
	f, ok := findingOfType(fs, "compliance_unchecked")
	if !ok || !strings.Contains(f.Description, "1 metric(s) and 0 provision(s) were not checked") {
		t.Fatalf("findings = %+v", fs)
	}
	fs, _ = rig.r.processChunk(context.Background(), 7, 1, complianceTestCfg, complianceChunk(1, "x"))
	if _, ok := findingOfType(fs, "compliance_unchecked"); ok {
		t.Error("only the first chunk reports unchecked activities")
	}
}

func TestApplyFindingProcStatusFailsOnlyTheAspectWithErrors(t *testing.T) {
	fs := []ReviewFinding{
		{Aspect: "compliance", Title: "a"},
		{Aspect: "compliance", Title: "b", ErrorMsg: "search failed"},
		{Aspect: "grammar_spelling", Title: "c"},
	}
	errs := applyFindingProcStatus(fs)
	if fs[0].ProcStatus != "failed" || fs[1].ProcStatus != "failed" || fs[2].ProcStatus != "success" {
		t.Errorf("statuses = %s %s %s", fs[0].ProcStatus, fs[1].ProcStatus, fs[2].ProcStatus)
	}
	var unitErr *reviewerExecutionError
	if len(errs) != 1 || !errors.As(errs[0], &unitErr) || unitErr.Aspect != "compliance" || !strings.Contains(errs[0].Error(), "search failed") {
		t.Errorf("errors = %v", errs)
	}
}

func TestComplianceProblemsUnwrapsTheCheckMessage(t *testing.T) {
	inner := &llmclients.StructuredOutputError{Kind: llmclients.ErrStructuredOutputCheckFailed, Err: errors.New("Verdict 1: summary is empty"), Raw: "{}"}
	err := &llmclients.StructuredOutputError{Kind: llmclients.ErrStructuredOutputRetriesExhausted, Err: inner, Raw: "{}"}
	if got := complianceProblems(err); got != "Verdict 1: summary is empty" {
		t.Errorf("problems = %q", got)
	}
	if !complianceWrongAnswer(err) || complianceWrongAnswer(&llmclients.StructuredOutputError{Kind: llmclients.ErrStructuredOutputProvider, Err: errors.New("x")}) {
		t.Error("wrong-answer classification")
	}
}

func TestComplianceLargeGroupIsJudgedInBatches(t *testing.T) {
	answer := func(stage, input string) (map[string]any, error) {
		switch stage {
		case "detect":
			return map[string]any{"activities": []any{}}, nil
		case "judgement":
			return judgeAll(input, nil), nil
		}
		return nil, fmt.Errorf("unexpected stage %s", stage)
	}
	rig := newComplianceTestRig(t, answer, func(req searchengine.Request) []searchengine.Result {
		var out []searchengine.Result
		for i := 0; i < 15; i++ {
			out = append(out, complianceResult("metric", fmt.Sprintf("%s_c%d", req.Query.Ref.ID, i), 2, "limit"))
		}
		return out
	})
	// Three metrics on overlapping lines: one group, 45 activity–candidate pairs.
	for i := 1; i <= 3; i++ {
		rig.store.rows = append(rig.store.rows, complianceArtifactRow{Kind: "metric", RowID: int64(i), ArtifactID: fmt.Sprintf("7_mtc_%d", i),
			Label: fmt.Sprintf("m%d", i), LineStart: 1, LineEnd: 2, DupKey: fmt.Sprintf("m%d", i)})
	}
	fs, err := rig.r.processChunk(context.Background(), 7, 1, complianceTestCfg, complianceChunk(1, "a", "b"))
	if err != nil {
		t.Fatal(err)
	}
	if calls := rig.primary.calls("judgement"); len(calls) != 3 {
		t.Errorf("judgement calls = %d, want 3 (at most %d pairs each)", len(calls), complianceJudgementMaxPairs)
	}
	f, ok := findingOfType(fs, "compliance_assessment")
	if !ok || len(f.RelatedArtifacts) != 45 || f.ErrorMsg != "" {
		t.Fatalf("finding = %d verdicts, error %q", len(f.RelatedArtifacts), f.ErrorMsg)
	}
}

func TestComplianceJudgementWritesInReportLanguage(t *testing.T) {
	answer := func(stage, input string) (map[string]any, error) {
		if stage == "judgement" {
			return judgeAll(input, nil), nil
		}
		return map[string]any{"activities": []any{}}, nil
	}
	rig := newComplianceTestRig(t, answer, func(searchengine.Request) []searchengine.Result {
		return []searchengine.Result{complianceResult("provision", "2_prv_1", 2, "seal")}
	})
	rig.r.language = "zh"
	rig.store.rows = []complianceArtifactRow{{Kind: "provision", RowID: 1, ArtifactID: "7_prv_1", Label: "seal", LineStart: 1, LineEnd: 1, DupKey: "s"}}
	fs, err := rig.r.processChunk(context.Background(), 7, 1, complianceTestCfg, complianceChunk(1, "seal"))
	if err != nil {
		t.Fatal(err)
	}
	calls := rig.primary.calls("judgement")
	if len(calls) != 1 || !strings.Contains(calls[0], `"output_language":"zh"`) {
		t.Fatalf("judgement inputs = %v, want one with output_language zh", calls)
	}
	if f, ok := findingOfType(fs, "compliance_assessment"); !ok || f.Language != "zh" {
		t.Fatalf("assessment finding language = %q, want zh", f.Language)
	}
}

func TestComplianceRelevanceRetriesOnce(t *testing.T) {
	answer := func(stage, input string) (map[string]any, error) {
		if stage == "judgement" {
			return judgeAll(input, nil), nil
		}
		return map[string]any{"activities": []any{}}, nil
	}
	rig := newComplianceTestRig(t, answer, func(searchengine.Request) []searchengine.Result {
		return []searchengine.Result{complianceResult("provision", "2_prv_1", 2, "seal")}
	})
	rig.store.rows = []complianceArtifactRow{{Kind: "provision", RowID: 1, ArtifactID: "7_prv_1", Label: "seal", LineStart: 1, LineEnd: 1, DupKey: "s"}}
	rig.rel.failures = 1
	fs, err := rig.r.processChunk(context.Background(), 7, 1, complianceTestCfg, complianceChunk(1, "seal"))
	if err != nil {
		t.Fatal(err)
	}
	if f, ok := findingOfType(fs, "compliance_assessment"); !ok || f.ErrorMsg != "" || len(rig.rel.asked) != 2 {
		t.Fatalf("findings = %+v, relevance requests = %d", fs, len(rig.rel.asked))
	}
	// Two failures in a row are an error for the activity.
	rig.rel.failures = 2
	rig.r.plans = sync.Map{}
	fs, _ = rig.r.processChunk(context.Background(), 7, 1, complianceTestCfg, complianceChunk(1, "seal"))
	if f, ok := findingOfType(fs, "compliance_incomplete"); !ok || !strings.Contains(f.ErrorMsg, "connection reset") {
		t.Fatalf("findings = %+v", fs)
	}
}
