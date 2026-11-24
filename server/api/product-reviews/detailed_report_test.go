package productreviews

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
)

func TestReportSourceText(t *testing.T) {
	lines := []docprocessing.Line{
		{LineNo: 10, PageNo: 2, Content: "Display requirement"},
		{LineNo: 11, PageNo: 2, Content: "11#r1: Minimum | 120 cd/m²"},
		{LineNo: 12, PageNo: 3, Content: "End of evidence"},
	}
	for _, tc := range []struct {
		spans    string
		want     []string
		complete bool
	}{
		{`["10-12","11"]`, []string{"L10", "L11", "L12"}, true},
		{`["2:11#r1:c2"]`, []string{"Minimum | 120 cd/m²"}, true},
		{`["10","99"]`, []string{"L10"}, false},
		{`["10-13"]`, []string{"L10", "L11", "L12"}, false},
		{`["bad"]`, nil, false}, {`[]`, nil, false},
	} {
		text, complete := reportSourceText(lines, json.RawMessage(tc.spans))
		if complete != tc.complete {
			t.Fatalf("%s completeness = %v", tc.spans, complete)
		}
		for _, want := range tc.want {
			if !strings.Contains(text, want) || strings.Count(text, want) != 1 {
				t.Fatalf("%s should appear once in %q", want, text)
			}
		}
	}
}

func detailedReportFixture() *DetailedReport {
	node := int64(2)
	return &DetailedReport{
		Run: Run{ID: 42, RequestID: 3, RunNumber: 1, Status: RunCompleted,
			ResultCount: 2, AttributedCount: 1, DocumentScopeCount: 1, ScopedDocumentCount: 1, TruncatedCount: 7},
		Summary: Report{ProfileID: 1, ProfileVersion: 3, ArtifactTypes: []string{"metric"}, TruncatedCount: 7,
			Coverage: []NodeCoverage{{NodeID: node, Label: "Display 显示器", ArtifactCount: 1}},
			Gaps:     []GapEntry{{NodeID: 4, Label: "Transport 运输"}}, PerDocumentTruncated: map[int64]int{5: 7}},
		Request: Request{ID: 3, ProfileID: 1, ProfileVersion: 3, Filters: map[string]any{"kind": "standard"}},
		Profile: &Profile{ID: 1, Name: "Demo 产品", Version: 9, ProductDescription: "Description with #[] *_$ and \\\"quotes\""},
		Nodes:   []ProfileNode{{ID: node, Label: "Renamed current label", Rationale: "Full scope rationale"}},
		Results: []Result{
			{ArtifactID: "5_mtc_1", ArtifactType: "metric", NodeID: &node, Tier: TierPart, InputRecordID: 5, Score: 1.25,
				Paths: []string{"direct", "document_first"}, InclusionReason: "Exact subject match", LineSpans: json.RawMessage(`["2:11#r1:c2"]`)},
			{ArtifactID: "6_mtc_2", ArtifactType: "metric", Tier: TierDocumentScope, InputRecordID: 6,
				InclusionReason: "Document scope only", LineSpans: json.RawMessage(`["99"]`)},
		},
		Scoped:  []ScopedDoc{{InputRecordID: 5, Paths: []string{"path_a"}, Reasons: []string{"Matched product"}}},
		Metrics: map[string]json.RawMessage{"5_mtc_1": json.RawMessage(`{"metric_name":"亮度","metric_value":"120","threshold_or_target":"minimum","condition":"at 25 °C","metric_context":"All context must appear","source_table_rows":[{"line":11,"cells":["r1:c2"]}],"ext_info":{"note":"Full details"},"value_min":9007199254740993}`)},
		Documents: []reportDocument{
			{ID: 5, Metadata: map[string]any{"title": "Display specification", "document_number": "TEST 123"},
				Lines: []docprocessing.Line{{LineNo: 11, PageNo: 2, Content: "11#r1: Minimum luminance | 120 cd/m²"}}},
			{ID: 6, Metadata: map[string]any{"title": "Additional directly matched source"}, Warning: "source-unavailable"},
		},
	}
}

func TestDetailedReportIncludesEveryResultAndField(t *testing.T) {
	d := detailedReportFixture()
	// A large set catches export caps and grouping losses, including unknown/deleted nodes.
	for i := 0; i < 225; i++ {
		node := int64(999)
		d.Results = append(d.Results, Result{ArtifactID: fmt.Sprintf("extra_%d", i), NodeID: &node})
	}
	source := renderDetailedReportTypst(d, "/template.typ", "en")
	for _, want := range []string{"5_mtc_1", "6_mtc_2", "extra_224", "Exact subject match", "minimum", "25 °C",
		"All context must appear", "Full details", "9007199254740993", "Minimum luminance", "TEST 123", "Truncation details", "Renamed current label"} {
		if !strings.Contains(source, want) {
			t.Errorf("detailed report is missing %q", want)
		}
	}
	for i := 0; i < 225; i++ {
		if strings.Count(source, reportQuote(fmt.Sprintf("extra_%d", i))) != 1 {
			t.Errorf("result %d must appear exactly once in the saved match table", i)
		}
	}
}

func TestDetailedReportPDF(t *testing.T) {
	for _, tool := range []string{"typst", "pdftotext"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("PDF integration test requires %s", tool)
		}
	}
	template, _ := filepath.Abs("../../../docs/doc-templates/template-document-report.typ")
	for _, lang := range []string{"en", "zh-cn"} {
		t.Run(lang, func(t *testing.T) {
			dir := t.TempDir()
			src, pdf := filepath.Join(dir, "report.typ"), filepath.Join(dir, "report.pdf")
			d := detailedReportFixture()
			d.Documents[0].Lines[0].Content += "\n" + strings.Repeat("Long source evidence spanning multiple pages.\n", 100) + "LONG-EVIDENCE-END"
			d.Profile.ProductDescription += ` #panic("injected code must stay text")`
			if err := os.WriteFile(src, []byte(renderDetailedReportTypst(d, template, lang)), 0600); err != nil {
				t.Fatal(err)
			}
			if out, err := exec.CommandContext(context.Background(), "typst", "compile", "--root", "/", src, pdf).CombinedOutput(); err != nil {
				t.Fatalf("compile: %v\n%s", err, out)
			}
			out, err := exec.Command("pdftotext", "-layout", pdf, "-").Output()
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{reportLabel("Product Review Report", lang), "5_mtc_1", "6_mtc_2", "120", "minimum", "25 °C", "Full details", "9007199254740993", "Minimum luminance", "TEST 123", "LONG-EVIDENCE-END", "injected code must stay text"} {
				if !strings.Contains(string(out), want) {
					t.Errorf("rendered PDF missing %q", want)
				}
			}
		})
	}
}

func TestLoadDetailedReportKeepsMissingDocumentsAndPinnedVersion(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery("FROM kb.product_review_requests").WithArgs(int64(3)).WillReturnRows(requestReturnRow(3, 1, 9))
	mock.ExpectQuery("FROM kb.product_profiles").WithArgs(int64(1)).WillReturnRows(profileRows(1, "Current product", 9, "ready", false, 0))
	mock.ExpectQuery("FROM kb.product_profile_nodes").WithArgs(int64(1)).WillReturnRows(nodeRowsFrom(1))
	results := sqlmock.NewRows([]string{"type", "artifact", "source", "input", "node", "tier", "score", "paths", "reason", "lines", "name", "name_en", "subject", "subject_en", "value", "unit", "unit_en"})
	results.AddRow("metric", "5_mtc_1", 100, 5, 2, TierPart, 1.2, []byte(`["direct"]`), "Part match", []byte(`["11"]`), "", "", "", "", "", "", "")
	results.AddRow("metric", "6_mtc_2", 101, 6, nil, TierDocumentScope, 0.1, []byte(`["document_first"]`), "Document match", []byte(`["99"]`), "", "", "", "", "", "", "")
	mock.ExpectQuery(`WHERE run_id = \$1\) r`).WithArgs(int64(42)).WillReturnRows(results)
	mock.ExpectQuery("FROM kb.product_review_run_documents").WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"input", "score", "nodes", "paths", "reasons", "kind"}).AddRow(5, 1.0, []byte(`[2]`), []byte(`["path_a"]`), []byte(`["Product match"]`), "standard"))
	mock.ExpectQuery("SELECT r.artifact_id, to_jsonb").WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"artifact", "metric"}).AddRow("5_mtc_1", []byte(`{"metric_value":"120"}`)).AddRow("6_mtc_2", nil))
	for _, id := range []int64{5, 6} {
		mock.ExpectQuery("FROM kb.inputs i").WithArgs(id).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	}
	d, err := loadDetailedReport(context.Background(), db, &Run{ID: 42, RequestID: 3, RunNumber: 1, ReportJSON: json.RawMessage(`{"profile_id":1,"profile_version":3}`)})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Results) != 2 || len(d.Documents) != 2 || d.Documents[1].ID != 6 || d.Documents[1].Warning == "" {
		t.Fatalf("lost a result or missing/directly matched source: %+v", d)
	}
	if d.Request.ProfileVersion != 3 || d.Profile.Version != 9 {
		t.Fatal("the current profile version must not replace the reviewed version")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
