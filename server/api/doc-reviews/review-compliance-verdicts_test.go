package docreviews

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/searchengine"
	llmclients "github.com/chendingplano/shared/go/api/llm"
)

// scorerCandidateRef mirrors compliance-eval/score.go candidateRef: the reported-passage lines the
// compliance scorer reads from a row's text.
var scorerCandidateRef = regexp.MustCompile(`(?m)^- \[(\S+) ([^\]]+)\](?: \(cosine ([0-9.]+)\))? record (\d+) `)

// verdictTestReviewer retrieves one metric (expressing a requirement) and one chunk (background).
func verdictTestReviewer(fake *fakeJSONExtractor) *complianceReviewer {
	source := complianceEvidence{RecordID: 9, LineStart: 1, LineEnd: 1, Quote: "Access logs shall be retained for at least six months."}
	target := complianceEvidence{RecordID: 7, LineStart: 10, LineEnd: 10, Quote: "Access logs are kept for twelve months."}
	r := newTestComplianceReviewer(fake, func(_ context.Context, req searchengine.Request) (*searchengine.Response, error) {
		switch req.Filters.ArtifactTypes[0] {
		case "metric":
			return &searchengine.Response{Results: []searchengine.Result{{ArtifactType: "metric", ArtifactID: "9_mtc_1", InputRecordID: 9, Snippet: source.Quote}}}, nil
		case "chunk":
			return &searchengine.Response{Results: []searchengine.Result{{ArtifactType: "chunk", ArtifactID: "9_chk_2", InputRecordID: 9, Snippet: "An access log records who accessed a system."}}}, nil
		}
		return &searchengine.Response{}, nil
	})
	r.assessmentPrompts = complianceAssessmentPromptRefs()
	r.loadPrompt = func(ref string) (string, string, error) { return "test prompt", ref, nil }
	r.loadArtifacts = func(_ context.Context, refs []searchengine.ArtifactRef) ([]searchengine.Result, error) {
		var out []searchengine.Result
		for _, ref := range refs {
			out = append(out, searchengine.Result{ArtifactType: ref.Type, ArtifactID: ref.ID, InputRecordID: 9, SourceTitle: "Log policy", Snippet: "text of " + ref.ID})
		}
		return out, nil
	}
	r.loadDocument = func(_ context.Context, id int64) (complianceDocument, error) {
		e := source
		if id == 7 {
			e = target
		}
		return complianceDocument{RecordID: id, Hash: "pinned", Complete: true, Lines: []complianceInputLine{{LineNumber: e.LineStart, Content: e.Quote}}}, nil
	}
	return r
}

func TestComplianceCandidateVerdictsForEveryCandidate(t *testing.T) {
	source := complianceEvidence{RecordID: 9, LineStart: 1, LineEnd: 1, Quote: "Access logs shall be retained for at least six months."}
	target := complianceEvidence{RecordID: 7, LineStart: 10, LineEnd: 10, Quote: "Access logs are kept for twelve months."}
	req := complianceRequirement{Text: "Retain access logs for at least six months", Kind: "qualitative", Binding: "required", ArtifactKeys: []string{"metric/9_mtc_1"}, Source: source}
	requirements := complianceTestPayload("requirements", []complianceRequirement{req})
	requirements["dependencies"] = []any{}
	assessment := complianceAssessment{RequirementID: "r1", Applicability: "applicable", ApplicabilityReason: "The policy covers these logs.", Status: "supported", Rationale: "Twelve months exceeds six.", Evidence: []complianceEvidence{target}}
	verdicts := []complianceCandidateVerdict{
		{ArtifactKey: "chunk/9_chk_2", Verdict: verdictUnqualified, RequirementIDs: []string{}, Summary: "Defines the term only."},
		{ArtifactKey: "metric/9_mtc_1", Verdict: verdictCompliant, RequirementIDs: []string{"r1"}, Summary: "Requires six months; the document keeps logs twelve months."},
	}
	fake := &fakeJSONExtractor{seq: []map[string]any{activityPayload("retain access logs", target.Quote), requirements,
		complianceTestPayload("screenings", []complianceScreening{{RequirementID: "r1", Evidence: []complianceEvidence{target}}}),
		complianceTestPayload2("assessments", []complianceAssessment{assessment}, "candidates", verdicts),
		complianceTestPayload2("checks", []complianceVerification{{RequirementID: "r1", Valid: true, Reason: "Quotes support it."}}, "candidate_checks", []complianceCandidateCheck{
			{ArtifactKey: "metric/9_mtc_1", Valid: true, Reason: "Follows from r1."}, {ArtifactKey: "chunk/9_chk_2", Valid: true, Reason: "Definition only."}}),
	}}
	fs, err := verdictTestReviewer(fake).processChunk(context.Background(), 7, 1, complianceTestCfg, chunkInput{})
	if err != nil || len(fs) != 1 {
		t.Fatalf("err=%v findings=%+v", err, fs)
	}
	f := fs[0]
	if len(f.RelatedArtifacts) != 2 || f.RelatedArtifacts[0].Relationship != verdictCompliant || f.RelatedArtifacts[1].Relationship != verdictUnqualified {
		t.Fatalf("verdicts must cover every candidate in verdict order: %+v", f.RelatedArtifacts)
	}
	if f.RelatedArtifacts[0].RelatedArtifactID != "9_mtc_1" || f.RelatedArtifacts[0].RelatedRecordID != 9 || f.RelatedArtifacts[0].Summary == "" {
		t.Fatalf("related artifact identity/summary missing: %+v", f.RelatedArtifacts[0])
	}
	if !strings.HasPrefix(f.Description, "Candidate verdicts: 1 compliant, 1 unqualified\n") || !strings.Contains(f.Description, "compliant [metric 9_mtc_1] record 9: Requires six months") {
		t.Fatalf("row text must lead with verdicts: %s", f.Description)
	}
	refs := scorerCandidateRef.FindAllStringSubmatch(f.Description, -1)
	if len(refs) != 2 || refs[0][2] != "9_mtc_1" && refs[1][2] != "9_mtc_1" {
		t.Fatalf("scorer must read exactly the two candidate lines, not verdict lines: %v", refs)
	}
	var meta map[string]any
	_ = json.Unmarshal(f.ArtifactFields, &meta)
	if meta["version"] != float64(2) || len(meta["candidate_verdicts"].([]any)) != 2 {
		t.Fatalf("metadata missing verdicts: %s", f.ArtifactFields)
	}
	if !strings.Contains(fake.inputTexts[3], `"linked_requirement_ids":["r1"]`) || !strings.Contains(fake.inputTexts[3], `"text":"text of 9_chk_2"`) {
		t.Fatalf("assessment input must carry candidate text and links: %s", fake.inputTexts[3])
	}
}

func TestComplianceZeroRequirementActivityStillGetsVerdicts(t *testing.T) {
	requirements := complianceTestPayload("requirements", []complianceRequirement{})
	requirements["dependencies"] = []any{}
	fake := &fakeJSONExtractor{seq: []map[string]any{activityPayload("retain access logs", "Access logs are kept."), requirements,
		complianceTestPayload2("assessments", []complianceAssessment{}, "candidates", []complianceCandidateVerdict{
			{ArtifactKey: "metric/9_mtc_1", Verdict: verdictUndetermined, RequirementIDs: []string{}, Summary: "States a retention period that was not extracted."},
			{ArtifactKey: "chunk/9_chk_2", Verdict: verdictUnqualified, RequirementIDs: []string{}, Summary: "Defines the term only."}}),
		complianceTestPayload2("checks", []complianceVerification{}, "candidate_checks", []complianceCandidateCheck{
			{ArtifactKey: "metric/9_mtc_1", Valid: true, Reason: "Honest abstention."}, {ArtifactKey: "chunk/9_chk_2", Valid: true, Reason: "Definition only."}}),
	}}
	fs, err := verdictTestReviewer(fake).processChunk(context.Background(), 7, 1, complianceTestCfg, chunkInput{})
	if err != nil || len(fs) != 1 {
		t.Fatalf("err=%v findings=%+v", err, fs)
	}
	if fake.calledCount != 4 {
		t.Fatalf("no screening call expected; calls=%d reasons=%v", fake.calledCount, fake.callReasons)
	}
	f := fs[0]
	if f.FindingType != "compliance_grounding_candidates" || len(f.RelatedArtifacts) != 2 || !strings.Contains(f.Description, "no assessable governing requirement") {
		t.Fatalf("zero-requirement row wrong: %s %+v", f.FindingType, f)
	}
}

func verdictCheckFixture() ([]complianceCandidateInput, []complianceRequirement) {
	cands := []complianceCandidateInput{
		{ArtifactKey: "metric/m1", LinkedRequirementIDs: []string{"r1"}},
		{ArtifactKey: "chunk/c1", LinkedRequirementIDs: []string{}},
		{ArtifactKey: "provision/p1", LinkedRequirementIDs: []string{"r2"}},
	}
	reqs := []complianceRequirement{{ID: "r1", Binding: "required"}, {ID: "r2", Binding: "recommended"}}
	return cands, reqs
}

func TestComplianceCandidateDraftChecksNameCandidateAndField(t *testing.T) {
	cands, reqs := verdictCheckFixture()
	contradicted := []complianceAssessment{{RequirementID: "r1", Applicability: "applicable", Status: "contradicted"}, {RequirementID: "r2", Applicability: "applicable", Status: "supported"}}
	supported := []complianceAssessment{{RequirementID: "r1", Applicability: "applicable", Status: "supported"}, {RequirementID: "r2", Applicability: "applicable", Status: "supported"}}
	gap := []complianceAssessment{{RequirementID: "r1", Applicability: "applicable", Status: "not_documented"}, {RequirementID: "r2", Applicability: "applicable", Status: "supported"}}
	ok := func(v string) complianceCandidateVerdict {
		return complianceCandidateVerdict{ArtifactKey: "metric/m1", Verdict: v, RequirementIDs: []string{"r1"}, Summary: "s"}
	}
	rest := []complianceCandidateVerdict{{ArtifactKey: "chunk/c1", Verdict: verdictUnqualified, Summary: "s"}, {ArtifactKey: "provision/p1", Verdict: verdictCompatible, Summary: "s"}}
	for _, tc := range []struct {
		name    string
		first   complianceCandidateVerdict
		drafts  []complianceAssessment
		extra   []complianceCandidateVerdict
		wantErr string
	}{
		{"valid not_compliant", ok(verdictNotCompliant), contradicted, rest, ""},
		{"valid compliant", ok(verdictCompliant), supported, rest, ""},
		{"contradiction forces not_compliant", ok(verdictCompatible), contradicted, rest, "Candidate metric/m1 has verdict compatible, but its required requirement r1 is assessed applicable and contradicted"},
		{"not_compliant needs contradiction", ok(verdictNotCompliant), gap, rest, "Candidate metric/m1 has verdict not_compliant"},
		{"compliant needs every required supported", ok(verdictCompliant), gap, rest, "Candidate metric/m1 has verdict compliant"},
		{"unqualified refused when applicable", ok(verdictUnqualified), gap, rest, "Candidate metric/m1 has verdict unqualified"},
		{"unknown verdict", ok("compatable"), gap, rest, `Candidate metric/m1 has verdict "compatable". Use one of: not_compliant, compatible`},
		{"unlinked requirement id", complianceCandidateVerdict{ArtifactKey: "metric/m1", Verdict: verdictCompatible, RequirementIDs: []string{"r2"}, Summary: "s"}, gap, rest, `Candidate metric/m1 cites requirement_id "r2", which is not linked to it. Use only ids from its linked_requirement_ids: [r1]`},
		{"empty summary", complianceCandidateVerdict{ArtifactKey: "metric/m1", Verdict: verdictCompatible, Summary: " "}, gap, rest, "Candidate metric/m1 has an empty summary"},
		{"unlinked candidate cannot be compatible", ok(verdictCompatible), gap, []complianceCandidateVerdict{{ArtifactKey: "chunk/c1", Verdict: verdictCompatible, Summary: "s"}, rest[1]}, "Candidate chunk/c1 has verdict compatible but no linked requirement"},
		{"recommendation cannot be compliant", ok(verdictCompatible), gap, []complianceCandidateVerdict{rest[0], {ArtifactKey: "provision/p1", Verdict: verdictCompliant, Summary: "s"}}, "Candidate provision/p1 has verdict compliant"},
		{"recommendation cannot be not_compliant", ok(verdictCompatible), gap, []complianceCandidateVerdict{rest[0], {ArtifactKey: "provision/p1", Verdict: verdictNotCompliant, Summary: "s"}}, "Candidate provision/p1 has verdict not_compliant"},
		{"missing candidate", ok(verdictCompatible), gap, rest[:1], "Candidate provision/p1 has no entry in candidates"},
		{"unknown candidate", ok(verdictCompatible), gap, append([]complianceCandidateVerdict{{ArtifactKey: "metric/zz", Verdict: verdictUnqualified, Summary: "s"}}, rest...), `has artifact_key "metric/zz", which is not a supplied candidate`},
	} {
		err := checkComplianceCandidateDrafts(append([]complianceCandidateVerdict{tc.first}, tc.extra...), cands, reqs, tc.drafts)
		switch {
		case tc.wantErr == "" && err != nil:
			t.Errorf("%s: unexpected error %v", tc.name, err)
		case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
			t.Errorf("%s: want %q, got %v", tc.name, tc.wantErr, err)
		}
	}
}

func TestComplianceCandidateChecksRequireOnePerCandidate(t *testing.T) {
	cands, _ := verdictCheckFixture()
	err := checkComplianceCandidateChecks([]complianceCandidateCheck{{ArtifactKey: "metric/m1", Valid: true, Reason: "ok"}, {ArtifactKey: "chunk/c1", Valid: true}}, cands)
	if err == nil || !strings.Contains(err.Error(), "The check for candidate chunk/c1 has an empty reason") || !strings.Contains(err.Error(), "Candidate provision/p1 has no entry in candidate_checks") {
		t.Fatalf("want both problems reported together, got %v", err)
	}
}

func TestComplianceFinalVerdictsOnlyDowngrade(t *testing.T) {
	cands, reqs := verdictCheckFixture()
	allValid := []complianceCandidateCheck{{ArtifactKey: "metric/m1", Valid: true, Reason: "ok"}, {ArtifactKey: "chunk/c1", Valid: true, Reason: "ok"}, {ArtifactKey: "provision/p1", Valid: true, Reason: "ok"}}
	run := func(verdict string, final complianceAssessment, checks []complianceCandidateCheck) complianceCandidateVerdict {
		vs := []complianceCandidateVerdict{{ArtifactKey: "metric/m1", Verdict: verdict, RequirementIDs: []string{"r1"}, Summary: "draft summary"}}
		finalizeComplianceVerdicts(vs, cands, reqs, []complianceAssessment{final}, checks)
		return vs[0]
	}
	// Incomplete source context turns the supporting result into insufficient evidence.
	v := run(verdictCompliant, complianceAssessment{RequirementID: "r1", Applicability: "applicable", Status: "insufficient_evidence", ResultNote: "Source context is incomplete."}, allValid)
	if v.Verdict != verdictUndetermined || v.DraftVerdict != verdictCompliant || !strings.Contains(v.Note, "Source context is incomplete.") || v.Summary != "draft summary" {
		t.Fatalf("incomplete source must downgrade with its cause: %+v", v)
	}
	// The numeric check disagreed with the drafted contradiction.
	v = run(verdictNotCompliant, complianceAssessment{RequirementID: "r1", Applicability: "applicable", Status: "supported", NumericCheck: "12 months >= 6 months"}, allValid)
	if v.Verdict != verdictUndetermined || !strings.Contains(v.Note, "12 months >= 6 months") {
		t.Fatalf("numeric disagreement must downgrade, never raise: %+v", v)
	}
	// The verifier rejected the candidate verdict.
	rejected := append([]complianceCandidateCheck{{ArtifactKey: "metric/m1", Valid: false, Reason: "The candidate states a requirement on this object."}}, allValid[1:]...)
	v = run(verdictUnqualified, complianceAssessment{RequirementID: "r1", Applicability: "unknown", Status: "not_assessed"}, rejected)
	if v.Verdict != verdictUndetermined || v.Note != "The candidate states a requirement on this object." || v.Verification != v.Note {
		t.Fatalf("verifier rejection must give undetermined with the reason: %+v", v)
	}
	// A verdict the final results still support is kept.
	v = run(verdictNotCompliant, complianceAssessment{RequirementID: "r1", Applicability: "applicable", Status: "contradicted"}, allValid)
	if v.Verdict != verdictNotCompliant || v.Note != "" {
		t.Fatalf("supported verdict changed: %+v", v)
	}
}

// The same model is asked again with a correction that names the candidate and the field, and the
// corrected answer is accepted (ChenWeb CLAUDE.md, correction messages).
func TestComplianceAssessmentCorrectionRetry(t *testing.T) {
	cands, reqs := verdictCheckFixture()
	draft := `{"assessments":[{"requirement_id":"r1","applicability":"applicable","status":"contradicted"}],"candidates":[` +
		`{"artifact_key":"metric/m1","verdict":"%s","requirement_ids":["r1"],"summary":"Requires six months; the document keeps 30 days."},` +
		`{"artifact_key":"chunk/c1","verdict":"unqualified","requirement_ids":[],"summary":"Definition only."},` +
		`{"artifact_key":"provision/p1","verdict":"compatible","requirement_ids":[],"summary":"Recommendation only."}]}`
	var requests atomic.Int32
	var secondBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		verdict := verdictNotCompliant
		if requests.Add(1) == 1 {
			verdict = verdictCompatible
		} else {
			b, _ := io.ReadAll(req.Body)
			secondBody = string(b)
		}
		content := strings.Replace(draft, "%s", verdict, 1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": content}}}})
	}))
	defer srv.Close()
	r := &complianceReviewer{client: &llmclients.OpenAIJSONClient{BaseURL: srv.URL, APIKey: "test-key", ModelName: "test", HTTPClient: srv.Client()}, assessmentPrompts: complianceAssessmentPromptRefs(),
		loadPrompt: func(ref string) (string, string, error) { return "test prompt", ref, nil }}
	payload, err := r.structuredAssessmentCall(context.Background(), "assess", complianceTestCfg, map[string]any{"candidates": cands}, complianceAssessmentSchema(), func(parsed map[string]any) error {
		drafts, verdicts, derr := complianceDecodeAssessment(parsed)
		if derr != nil {
			return derr
		}
		return checkComplianceCandidateDrafts(verdicts, cands, reqs, drafts)
	})
	if err != nil || requests.Load() != 2 {
		t.Fatalf("requests=%d err=%v; the inconsistent verdict must be corrected by a retry", requests.Load(), err)
	}
	if !strings.Contains(secondBody, "Candidate metric/m1 has verdict compatible, but its required requirement r1 is assessed applicable and contradicted. Return not_compliant") {
		t.Fatalf("retry request must carry the correction: %s", secondBody)
	}
	_, verdicts, _ := complianceDecodeAssessment(payload)
	if verdicts[0].Verdict != verdictNotCompliant {
		t.Fatalf("corrected answer not accepted: %+v", verdicts)
	}
}

func TestComplianceDiscoveryRetriesQuotesOutsidePinnedSource(t *testing.T) {
	const draft = `{"requirements":[{"text":"Retain access logs.","kind":"qualitative","binding":"required","artifact_keys":["provision/p1"],"source":{"record_id":9,"line_start":613,"line_end":624,"quote":"Retain access logs."},"scope_evidence":[{"record_id":9,"line_start":625,"line_end":625,"quote":"Applies to access logs."}]}],"dependencies":[]}`
	const corrected = `{"requirements":[{"text":"Retain access logs.","kind":"qualitative","binding":"required","artifact_keys":["provision/p1"],"source":{"record_id":9,"line_start":486,"line_end":486,"quote":"Retain access logs."}}],"dependencies":[]}`
	var requests atomic.Int32
	retryBody := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		content := draft
		attempt := requests.Add(1)
		if attempt == 2 {
			b, _ := io.ReadAll(req.Body)
			retryBody <- string(b)
		}
		if attempt > 1 {
			content = corrected
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": content}}}})
	}))
	defer srv.Close()
	doc := complianceDocument{RecordID: 9, Lines: []complianceInputLine{{LineNumber: 485, Content: "Access logs"}, {LineNumber: 486, Content: "Retain access logs."}}}
	r := &complianceReviewer{
		client:            &llmclients.OpenAIJSONClient{BaseURL: srv.URL, APIKey: "test-key", ModelName: "test", HTTPClient: srv.Client()},
		assessmentPrompts: complianceAssessmentPromptRefs(),
		loadPrompt:        func(ref string) (string, string, error) { return "test prompt", ref, nil },
		loadDocument:      func(context.Context, int64) (complianceDocument, error) { return doc, nil },
	}
	artifacts := []complianceArtifact{{Type: "provision", ID: "p1", RecordID: 9}}
	reqs, err := r.discoverRequirements(context.Background(), complianceActivity{}, complianceTestCfg, &artifacts, map[int64]complianceDocument{}, &complianceCoverage{})
	if err != nil || requests.Load() != 2 {
		t.Fatalf("requests=%d err=%v; the quote outside the supplied lines must trigger a retry", requests.Load(), err)
	}
	secondBody := <-retryBody
	for _, want := range []string{"Requirement 1 source", "pinned record 9 lines 613-624", "Requirement 1 scope_evidence[0]", "pinned record 9 lines 625-625", "lines 485-486"} {
		if !strings.Contains(secondBody, want) {
			t.Errorf("retry request missing %q", want)
		}
	}
	if len(reqs) != 1 || reqs[0].ID != "r1" || reqs[0].Source.LineStart != 486 {
		t.Fatalf("corrected requirement not accepted: %+v", reqs)
	}
}
