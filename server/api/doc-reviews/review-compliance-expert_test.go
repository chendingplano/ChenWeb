package docreviews

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/searchengine"
)

func TestComplianceMetricSearchCannotBeCrowdedOut(t *testing.T) {
	r := newTestComplianceReviewer(&fakeJSONExtractor{out: activityPayload("clearance", "clearance specified")}, func(_ context.Context, req searchengine.Request) (*searchengine.Response, error) {
		if len(req.Filters.ArtifactTypes) != 1 {
			t.Fatalf("each type needs its own search: %+v", req.Filters)
		}
		typ := req.Filters.ArtifactTypes[0]
		return &searchengine.Response{Results: []searchengine.Result{{ArtifactType: typ, ArtifactID: typ + "1", InputRecordID: 9, Snippet: "minimum clearance"}}}, nil
	})
	fs, err := r.processChunk(context.Background(), 7, 2, complianceTestCfg, chunkInput{})
	if err != nil || len(fs) != 1 || !strings.Contains(fs[0].Description, "[metric metric1]") {
		t.Fatalf("metric missing: %v %+v", err, fs)
	}
}

func complianceTestPayload(key string, value any) map[string]any {
	b, _ := json.Marshal(map[string]any{key: value})
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	return out
}

func TestComplianceExpertStagesFindMetricAndVerifyConvertedClaim(t *testing.T) {
	source := complianceEvidence{RecordID: 9, LineStart: 1, LineEnd: 1, Quote: "Clearance shall be at least 5 mm."}
	target := complianceEvidence{RecordID: 7, LineStart: 10, LineEnd: 10, Quote: "Clearance is 0.5 cm."}
	req := complianceRequirement{Text: "Minimum clearance", Kind: "quantitative", Binding: "required", ArtifactKey: "metric/m1", Source: source}
	assessment := complianceAssessment{RequirementID: "r1", Applicability: "applicable", ApplicabilityReason: "The adopted policy covers this design.", Status: "contradicted", Rationale: "Compare the stated clearance.", Evidence: []complianceEvidence{target}, Numeric: &complianceNumeric{Property: "clearance", QuantityKind: "Length", ConditionsMatch: true, MethodsMatch: true, BasisMatch: true, Required: complianceQuantity{Operator: ">=", Value: "5", Unit: "mm"}, Documented: complianceQuantity{Operator: "=", Value: "0.5", Unit: "cm"}}}
	requirements := complianceTestPayload("requirements", []complianceRequirement{req})
	requirements["dependencies"] = []any{}
	fake := &fakeJSONExtractor{seq: []map[string]any{activityPayload("clearance", target.Quote), requirements,
		complianceTestPayload("screenings", []complianceScreening{{RequirementID: "r1", Evidence: []complianceEvidence{target}}}),
		complianceTestPayload("assessments", []complianceAssessment{assessment}),
		complianceTestPayload("checks", []complianceVerification{{RequirementID: "r1", Valid: true, Reason: "Both quotes support the converted inclusive limit."}}),
	}}
	r := newTestComplianceReviewer(fake, func(_ context.Context, request searchengine.Request) (*searchengine.Response, error) {
		if request.Filters.ArtifactTypes[0] != "metric" {
			return &searchengine.Response{}, nil
		}
		return &searchengine.Response{Results: []searchengine.Result{{ArtifactType: "metric", ArtifactID: "m1", InputRecordID: 9, Snippet: source.Quote}}}, nil
	})
	r.assessmentPrompts = complianceAssessmentPromptRefs()
	r.loadPrompt = func(ref string) (string, string, error) { return "test prompt", ref, nil }
	r.loadDocument = func(_ context.Context, id int64) (complianceDocument, error) {
		e := source
		if id == 7 {
			e = target
		}
		return complianceDocument{RecordID: id, Hash: "pinned", Complete: true, Lines: []complianceInputLine{{LineNumber: e.LineStart, Content: e.Quote}}}, nil
	}
	fs, err := r.processChunk(context.Background(), 7, 1, complianceTestCfg, chunkInput{})
	if err != nil || len(fs) != 1 {
		t.Fatalf("err=%v findings=%+v", err, fs)
	}
	if !strings.Contains(fs[0].Description, "evidence=supported") || fs[0].FindingType != "compliance_assessment" {
		t.Fatalf("assessment missing or numeric check failed: %+v", fs[0])
	}
	if !strings.Contains(string(fs[0].ArtifactFields), `"screened_lines":1`) || fake.calledCount != 5 {
		t.Fatalf("coverage/calls missing: %s calls=%d", fs[0].ArtifactFields, fake.calledCount)
	}
}

func TestComplianceWholeBoundaryGuardCannotClaimOmissionFromIncompleteScreening(t *testing.T) {
	req := complianceRequirement{ID: "r1", Kind: "qualitative", Binding: "required"}
	result := complianceAssessment{RequirementID: "r1", Applicability: "applicable", ApplicabilityReason: "Applies", Status: "not_documented", Rationale: "No support found", Suggestion: "Add requirement"}
	cases := []complianceCoverage{
		{ReviewedRecordID: 7, TotalLines: 200, ScreenedLines: 100, EvidenceComplete: true, SourceComplete: true},
		{ReviewedRecordID: 7, TotalLines: 200, ScreenedLines: 200, EvidenceComplete: false, SourceComplete: true},
		{ReviewedRecordID: 7, TotalLines: 200, ScreenedLines: 200, EvidenceComplete: true, SourceComplete: false},
	}
	for _, coverage := range cases {
		results := []complianceAssessment{result}
		if err := validateComplianceAssessments(results, []complianceRequirement{req}, nil, coverage); err != nil {
			t.Fatal(err)
		}
		if results[0].Status != "insufficient_evidence" || results[0].Suggestion != "" {
			t.Fatalf("incomplete omission accepted: %+v %+v", coverage, results)
		}
	}
}

func TestComplianceExpertCanonicalLoadErrorAborts(t *testing.T) {
	r := newTestComplianceReviewer(&fakeJSONExtractor{out: activityPayload("a", "source")}, func(context.Context, searchengine.Request) (*searchengine.Response, error) {
		return &searchengine.Response{Results: []searchengine.Result{{ArtifactType: "metric", ArtifactID: "m1", InputRecordID: 9}}}, nil
	})
	r.assessmentPrompts = complianceAssessmentPromptRefs()
	boom := errors.New("canonical source unavailable")
	r.loadDocument = func(context.Context, int64) (complianceDocument, error) { return complianceDocument{}, boom }
	fs, err := r.processChunk(context.Background(), 7, 1, complianceTestCfg, chunkInput{})
	if !errors.Is(err, boom) || fs != nil {
		t.Fatalf("load failure hidden: %v %+v", err, fs)
	}
}

func TestComplianceQuantitativeChecks(t *testing.T) {
	for _, tc := range []struct {
		operator, value, unit, status string
		compatible                    bool
	}{
		{">=", "0.5", "cm", "supported", true},
		{">", "5", "mm", "contradicted", true},
		{"> ", "5", "mm", "contradicted", true},
		{" < ", "5", "mm", "contradicted", true},
		{">=", "5", "mm", "insufficient_evidence", false},
		{">=", "5", "unknown", "insufficient_evidence", true},
	} {
		n := complianceNumeric{QuantityKind: "Length", Property: "clearance", ConditionsMatch: tc.compatible, MethodsMatch: true, BasisMatch: true,
			Required: complianceQuantity{Operator: tc.operator, Value: "5", Unit: "mm"}, Documented: complianceQuantity{Operator: "=", Value: tc.value, Unit: tc.unit}}
		status, _ := checkComplianceNumeric(n)
		if status != tc.status {
			t.Errorf("%+v: got %s want %s", tc, status, tc.status)
		}
	}
}

func TestComplianceEvidenceMustMatchOriginal(t *testing.T) {
	docs := map[int64]complianceDocument{7: {RecordID: 7, Lines: []complianceInputLine{{LineNumber: 5, Content: "Clearance is 5 mm."}}}}
	for _, quote := range []string{"Clearance is 5 mm.", "Clearance is 50 mm."} {
		err := verifyComplianceEvidence(complianceEvidence{RecordID: 7, LineStart: 5, LineEnd: 5, Quote: quote}, docs)
		if (err == nil) != (quote == "Clearance is 5 mm.") {
			t.Errorf("quote=%q err=%v", quote, err)
		}
	}
}

func TestComplianceImageEvidenceCannotEstablishAbsence(t *testing.T) {
	for _, line := range []Line{{LineType: "image_path", Content: "scan"}, {Content: `<img src="data:image/jpeg;base64,abc">`}, {Content: "figures/table.webp"}, {Content: "figures/table.jpeg"}, {Content: "![table](table.png)"}} {
		if !complianceUnreadableLine(line) {
			t.Errorf("image evidence treated as readable text: %+v", line)
		}
	}
	if complianceUnreadableLine(Line{LineType: "paragraph", Content: "The limit is 5 mm."}) {
		t.Fatal("ordinary text marked unreadable")
	}
}

func TestComplianceNoCandidatesPreservesIndependentActivityContext(t *testing.T) {
	payload := activityPayload("store_logs", "Keep records")
	a := payload["activities"].([]any)[0].(map[string]any)
	a["document_relationship"] = "plans"
	a["actor"] = "operator"
	a["cited_docs"] = []any{"Policy A"}
	r := newTestComplianceReviewer(&fakeJSONExtractor{out: payload}, func(context.Context, searchengine.Request) (*searchengine.Response, error) {
		return &searchengine.Response{}, nil
	})
	fs, err := r.processChunk(context.Background(), 7, 1, complianceTestCfg, chunkInput{})
	if err != nil || len(fs) != 1 {
		t.Fatalf("err=%v findings=%+v", err, fs)
	}
	var meta struct {
		Activity complianceActivity `json:"activity"`
	}
	if err = json.Unmarshal(fs[0].ArtifactFields, &meta); err != nil {
		t.Fatal(err)
	}
	if meta.Activity.Relationship != "plans" || meta.Activity.Actor != "operator" || len(meta.Activity.CitedDocs) != 1 {
		t.Fatalf("activity context lost with no KB hits: %s", fs[0].ArtifactFields)
	}
}
