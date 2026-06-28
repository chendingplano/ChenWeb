package docreviews

import (
	"context"
	"strings"
	"testing"
)

// Helper to build a finding with defaults.
func makeFinding(id int64, pass, aspect, severity, findingType, title string) FindingItem {
	return FindingItem{
		ID:           id,
		Pass:         pass,
		Aspect:       aspect,
		Severity:     severity,
		FindingType:  findingType,
		Title:        title,
		Description:  "Description for " + title,
		Evidence:     "Evidence for " + title,
		Location:     "Section 3.1",
		Suggestion:   "Suggestion for " + title,
		Confidence:   0.85,
		ReviewStatus: "pending",
	}
}

func newTestReportGenerator() *DocReviewReportGenerator {
	gen := NewDocReviewReportGenerator()
	gen.DB = nil
	gen.PackageOrder = defaultReviewPackageOrder()
	return gen
}

func TestReportBuild_EmptyFindings(t *testing.T) {
	gen := newTestReportGenerator()

	req := &RequestStatus{
		ID: 1, InputRecordID: 416, Tier: "must_review",
		Aspects:     []string{"completeness", "grammar_spelling"},
		Status:      "completed",
		CreateTime:  "2026-06-21T12:00:00Z",
		LatestRunID: 416,
	}

	report, err := gen.Build(context.Background(), req, []FindingItem{})
	if err != nil {
		t.Fatalf("Build() returned error: %v", err)
	}

	if report.Meta.TotalFindings != 0 {
		t.Errorf("TotalFindings = %d, want 0", report.Meta.TotalFindings)
	}
	if report.ExecutiveSummary.OverallAssessment != "pass_with_issues" {
		t.Errorf("OverallAssessment = %q, want %q", report.ExecutiveSummary.OverallAssessment, "pass_with_issues")
	}
	if len(report.FindingsByPass) != 0 {
		t.Errorf("FindingsByPass has %d entries, want 0", len(report.FindingsByPass))
	}
	if len(report.Findings) != 0 {
		t.Errorf("Findings has %d entries, want 0", len(report.Findings))
	}
	if report.Meta.NumReviewersRan != 0 {
		t.Errorf("NumReviewersRan = %d, want 0", report.Meta.NumReviewersRan)
	}
	// DocumentTitle should be empty string (nil DB, no query executed).
	if report.Meta.DocumentTitle != "" {
		t.Errorf("DocumentTitle = %q, want empty (nil DB)", report.Meta.DocumentTitle)
	}
}

func TestReportBuild_MixedSeverities(t *testing.T) {
	gen := newTestReportGenerator()

	req := &RequestStatus{
		ID: 2, InputRecordID: 512, Tier: "must_review",
		Aspects:     []string{"completeness", "grammar_spelling", "technical_accuracy"},
		Status:      "completed",
		CreateTime:  "2026-06-21T14:00:00Z",
		LatestRunID: 512,
	}

	findings := []FindingItem{
		makeFinding(1, "P1", "grammar_spelling", "high", "spelling_error", "Misspelled technical term"),
		makeFinding(2, "P1", "grammar_spelling", "medium", "style_issue", "Inconsistent capitalization"),
		makeFinding(3, "P3", "completeness", "low", "minor_omission", "Missing example for edge case"),
	}

	report, err := gen.Build(context.Background(), req, findings)
	if err != nil {
		t.Fatalf("Build() returned error: %v", err)
	}

	// Total findings.
	if report.Meta.TotalFindings != 3 {
		t.Errorf("TotalFindings = %d, want 3", report.Meta.TotalFindings)
	}

	// Counts in the executive summary text.
	if !strings.Contains(report.ExecutiveSummary.Text, "3 findings") {
		t.Errorf("Summary text should mention '3 findings', got: %s", report.ExecutiveSummary.Text)
	}

	// Overall assessment: fail because high > 0.
	if report.ExecutiveSummary.OverallAssessment != "fail" {
		t.Errorf("OverallAssessment = %q, want %q", report.ExecutiveSummary.OverallAssessment, "fail")
	}

	// Top findings should include the high-severity finding.
	if len(report.ExecutiveSummary.TopFindings) != 1 {
		t.Fatalf("TopFindings has %d entries, want 1", len(report.ExecutiveSummary.TopFindings))
	}
	if report.ExecutiveSummary.TopFindings[0] != "Misspelled technical term" {
		t.Errorf("TopFindings[0] = %q, want %q", report.ExecutiveSummary.TopFindings[0], "Misspelled technical term")
	}

	// Grouped by pass: P1 (2 findings) and P3 (1 finding).
	if len(report.FindingsByPass) != 2 {
		t.Fatalf("FindingsByPass has %d entries, want 2", len(report.FindingsByPass))
	}
	p1Group, ok := report.FindingsByPass["P1"]
	if !ok {
		t.Fatal("FindingsByPass missing P1 key")
	}
	if len(p1Group.Findings) != 2 {
		t.Errorf("P1 has %d findings, want 2", len(p1Group.Findings))
	}
	if p1Group.Label != "Language & Style" {
		t.Errorf("P1 label = %q, want %q", p1Group.Label, "Language & Style")
	}

	p3Group, ok := report.FindingsByPass["P3"]
	if !ok {
		t.Fatal("FindingsByPass missing P3 key")
	}
	if len(p3Group.Findings) != 1 {
		t.Errorf("P3 has %d findings, want 1", len(p3Group.Findings))
	}

	// PassOrder should list P1 then P3.
	if len(report.PassOrder) != 2 {
		t.Fatalf("PassOrder has %d entries, want 2", len(report.PassOrder))
	}
	if report.PassOrder[0] != "P1" {
		t.Errorf("PassOrder[0] = %q, want %q", report.PassOrder[0], "P1")
	}
	if report.PassOrder[1] != "P3" {
		t.Errorf("PassOrder[1] = %q, want %q", report.PassOrder[1], "P3")
	}

	// Flat findings list should have all 3.
	if len(report.Findings) != 3 {
		t.Errorf("Flat findings has %d entries, want 3", len(report.Findings))
	}
}

func TestPackageOrderFromTOMLPreservesDeclarationOrder(t *testing.T) {
	raw := []byte(`
[packages.P5]
name = "Technical & Compliance Standards"

[packages.P4]
name = "Consistency"

[packages.P1]
name = "Language & Style"
`)
	packages := map[string]ReviewPackageConfig{
		"P1": {Name: "Language & Style"},
		"P4": {Name: "Consistency"},
		"P5": {Name: "Technical & Compliance Standards"},
	}

	got := packageOrderFromTOML(raw, packages)
	want := []ReviewPackageInfo{
		{Key: "P5", Label: "Technical & Compliance Standards"},
		{Key: "P4", Label: "Consistency"},
		{Key: "P1", Label: "Language & Style"},
	}
	if len(got) != len(want) {
		t.Fatalf("order len=%d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order[%d]=%#v, want %#v", i, got[i], want[i])
		}
	}
}

func TestReportBuild_UsesConfiguredPackageOrderAndLabels(t *testing.T) {
	gen := NewDocReviewReportGenerator()
	gen.DB = nil
	gen.PackageOrder = []ReviewPackageInfo{
		{Key: "P5", Label: "Technical & Compliance Standards"},
		{Key: "P1", Label: "Language & Style"},
	}
	req := &RequestStatus{
		ID: 5, InputRecordID: 777, Tier: "must_review",
		Status: "completed", CreateTime: "2026-06-21T20:00:00Z", LatestRunID: 777,
	}
	findings := []FindingItem{
		makeFinding(1, "P1", "grammar_spelling", "low", "typo", "Typo"),
		makeFinding(2, "P5", "technical_accuracy", "high", "technical_error", "Wrong range"),
	}

	report, err := gen.Build(context.Background(), req, findings)
	if err != nil {
		t.Fatalf("Build() returned error: %v", err)
	}
	if got := strings.Join(report.PassOrder, ","); got != "P5,P1" {
		t.Fatalf("PassOrder=%q, want P5,P1", got)
	}
	if report.FindingsByPass["P5"].Label != "Technical & Compliance Standards" {
		t.Fatalf("P5 label=%q", report.FindingsByPass["P5"].Label)
	}
	if report.Findings[0].Pass != "P5" {
		t.Fatalf("first flat finding pass=%q, want P5", report.Findings[0].Pass)
	}
}

func TestReportBuild_ComplianceSummary(t *testing.T) {
	gen := newTestReportGenerator()

	req := &RequestStatus{
		ID: 3, InputRecordID: 720, Tier: "must_review",
		Aspects:     []string{"standards_compliance"},
		Status:      "completed",
		CreateTime:  "2026-06-21T16:00:00Z",
		LatestRunID: 720,
		ReferenceDocs: []ReferenceDoc{
			{RecordID: 100, DocNo: "ISO-9001:2023", Title: "Quality Management Systems"},
			{RecordID: 101, DocNo: "IEC-62304", Title: "Medical Device Software"},
		},
	}

	findings := []FindingItem{
		makeFinding(1, "P5", "standards_compliance", "high", "missing_requirement", "Missing quality policy documentation"),
		makeFinding(2, "P5", "standards_compliance", "medium", "missing_requirement", "Risk management not addressed"),
		makeFinding(3, "P5", "technical_accuracy", "low", "minor_error", "Minor spec version mismatch"),
	}

	report, err := gen.Build(context.Background(), req, findings)
	if err != nil {
		t.Fatalf("Build() returned error: %v", err)
	}

	// Reference standards checked.
	if len(report.ComplianceSummary.ReferenceStandardsChecked) != 2 {
		t.Fatalf("ReferenceStandardsChecked has %d entries, want 2", len(report.ComplianceSummary.ReferenceStandardsChecked))
	}
	if report.ComplianceSummary.ReferenceStandardsChecked[0] != "ISO-9001:2023" {
		t.Errorf("ReferenceStandardsChecked[0] = %q, want %q", report.ComplianceSummary.ReferenceStandardsChecked[0], "ISO-9001:2023")
	}
	if report.ComplianceSummary.ReferenceStandardsChecked[1] != "IEC-62304" {
		t.Errorf("ReferenceStandardsChecked[1] = %q, want %q", report.ComplianceSummary.ReferenceStandardsChecked[1], "IEC-62304")
	}

	// Missing requirements (only findings with finding_type = missing_requirement).
	if len(report.ComplianceSummary.MissingRequirements) != 2 {
		t.Fatalf("MissingRequirements has %d entries, want 2", len(report.ComplianceSummary.MissingRequirements))
	}
	if report.ComplianceSummary.MissingRequirements[0] != "Missing quality policy documentation" {
		t.Errorf("MissingRequirements[0] = %q, want %q", report.ComplianceSummary.MissingRequirements[0], "Missing quality policy documentation")
	}
	if report.ComplianceSummary.MissingRequirements[1] != "Risk management not addressed" {
		t.Errorf("MissingRequirements[1] = %q, want %q", report.ComplianceSummary.MissingRequirements[1], "Risk management not addressed")
	}
}

func TestReportBuild_Recommendations(t *testing.T) {
	gen := newTestReportGenerator()

	req := &RequestStatus{
		ID: 4, InputRecordID: 999, Tier: "must_review",
		Aspects:     []string{"completeness", "technical_accuracy", "security"},
		Status:      "completed",
		CreateTime:  "2026-06-21T18:00:00Z",
		LatestRunID: 999,
	}

	findings := []FindingItem{
		makeFinding(1, "P3", "completeness", "high", "missing_content", "Missing security requirements section"),
		makeFinding(2, "P5", "security", "high", "vulnerability", "Plaintext credentials in config example"),
		makeFinding(3, "P5", "technical_accuracy", "medium", "incorrect_value", "Wrong port number in example"),
		makeFinding(4, "P5", "technical_accuracy", "medium", "incorrect_value", "Outdated API endpoint URL"),
		makeFinding(5, "P3", "completeness", "medium", "minor_omission", "Missing error code for timeout"),
	}

	report, err := gen.Build(context.Background(), req, findings)
	if err != nil {
		t.Fatalf("Build() returned error: %v", err)
	}

	// Only high-severity findings produce recommendations.
	if len(report.Recommendations) != 2 {
		t.Fatalf("Recommendations has %d entries, want 2", len(report.Recommendations))
	}

	// Priority numbers should be sequential: 1, 2.
	if report.Recommendations[0].Priority != 1 {
		t.Errorf("Recommendations[0].Priority = %d, want 1", report.Recommendations[0].Priority)
	}
	if report.Recommendations[1].Priority != 2 {
		t.Errorf("Recommendations[1].Priority = %d, want 2", report.Recommendations[1].Priority)
	}

	// Actions should match the suggestion from the high-severity findings.
	if report.Recommendations[0].Action != "Suggestion for Missing security requirements section" {
		t.Errorf("Recommendations[0].Action = %q, want %q", report.Recommendations[0].Action, "Suggestion for Missing security requirements section")
	}
	if report.Recommendations[1].Action != "Suggestion for Plaintext credentials in config example" {
		t.Errorf("Recommendations[1].Action = %q, want %q", report.Recommendations[1].Action, "Suggestion for Plaintext credentials in config example")
	}

	// related_finding_ids should be 1-indexed positions in the flat findings list.
	// Flat findings order: missing_security(0) first, then credentials(1), then wrong_port(2),
	// outdated_url(3), then timeout(4).
	if len(report.Recommendations[0].RelatedFindingIDs) != 1 {
		t.Fatalf("RelatedFindingIDs length = %d, want 1", len(report.Recommendations[0].RelatedFindingIDs))
	}
	// First finding index is 0, so ID should be 1 (= index+1).
	if report.Recommendations[0].RelatedFindingIDs[0] != 1 {
		t.Errorf("First recommendation related_finding_ids[0] = %d, want 1", report.Recommendations[0].RelatedFindingIDs[0])
	}
}

func TestReportBuild_MetaFields(t *testing.T) {
	gen := newTestReportGenerator()

	req := &RequestStatus{
		ID: 5, InputRecordID: 123, Tier: "must_review",
		Aspects:     []string{"grammar_spelling"},
		Status:      "completed",
		CreateTime:  "2026-06-21T20:00:00Z",
		LatestRunID: 123,
	}

	findings := []FindingItem{
		makeFinding(1, "P1", "grammar_spelling", "low", "typo", "Minor typo in abstract"),
	}

	report, err := gen.Build(context.Background(), req, findings)
	if err != nil {
		t.Fatalf("Build() returned error: %v", err)
	}

	if report.Meta.ReportID == "" {
		t.Error("ReportID is empty")
	}
	if !strings.HasPrefix(report.Meta.ReportID, "rpt_") {
		t.Errorf("ReportID = %q, want prefix 'rpt_'", report.Meta.ReportID)
	}

	if report.Meta.DocumentRecordID != 123 {
		t.Errorf("DocumentRecordID = %d, want 123", report.Meta.DocumentRecordID)
	}

	if report.Meta.RunID != 123 {
		t.Errorf("RunID = %d, want 123", report.Meta.RunID)
	}

	if report.Meta.GeneratedAt == "" {
		t.Error("GeneratedAt is empty")
	}

	if report.Meta.TotalFindings != 1 {
		t.Errorf("TotalFindings = %d, want 1", report.Meta.TotalFindings)
	}
}
