package docreviews

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// TestTypContentLineEscapesMarkupDelimiters guards the fix for the "unclosed
// delimiter" compile failure: finding text containing an unbalanced count of
// Typst emphasis/raw delimiters (`*`, `_`, “ ` “) or `~` must be escaped so it
// renders as literal text instead of opening markup Typst never closes.
func TestTypContentLineEscapesMarkupDelimiters(t *testing.T) {
	// Mirrors the real failure: wildcard ranges like "0.*", "1.*" produced an
	// odd number of asterisks.
	in := `“0.*”, “1.*”, “3.*”, “*”`
	got := typContentLine(in)
	for _, ch := range []string{`*`, `_`, "`", `~`} {
		// Every occurrence of the delimiter must be backslash-escaped.
		if strings.Count(got, ch) != strings.Count(got, `\`+ch) {
			t.Fatalf("delimiter %q not fully escaped in %q", ch, got)
		}
	}
}

// TestHTMLTableToTypst verifies that HTML tables are converted to Typst #table() calls
// and that surrounding text is escaped normally.
func TestHTMLTableToTypst(t *testing.T) {
	t.Run("basic table", func(t *testing.T) {
		in := `<table><tr><td>A</td><td>B</td></tr><tr><td>C</td><td>D</td></tr></table>`
		got := htmlTableToTypst(in)
		if !strings.Contains(got, "#table(") {
			t.Fatalf("expected #table( in output, got: %s", got)
		}
		if !strings.Contains(got, "columns: 2") {
			t.Fatalf("expected columns: 2, got: %s", got)
		}
		for _, cell := range []string{"[A]", "[B]", "[C]", "[D]"} {
			if !strings.Contains(got, cell) {
				t.Fatalf("expected cell %s in output, got: %s", cell, got)
			}
		}
	})

	t.Run("table embedded in text", func(t *testing.T) {
		in := `before <table><tr><td>X</td></tr></table> after`
		got := typContent(in)
		if !strings.Contains(got, "#table(") {
			t.Fatalf("expected #table( in output, got: %s", got)
		}
		if !strings.Contains(got, "before") || !strings.Contains(got, "after") {
			t.Fatalf("surrounding text missing in: %s", got)
		}
	})

	t.Run("html entities decoded", func(t *testing.T) {
		in := `<table><tr><td>A &amp; B</td><td>&lt;C&gt;</td></tr></table>`
		got := htmlTableToTypst(in)
		if strings.Contains(got, "&amp;") || strings.Contains(got, "&lt;") {
			t.Fatalf("HTML entities not decoded in: %s", got)
		}
	})

	t.Run("no table passes through", func(t *testing.T) {
		in := `plain text with no table`
		got := typContent(in)
		if strings.Contains(got, "#table") {
			t.Fatalf("unexpected #table in: %s", got)
		}
	})
}

// TestTypstReportCompilesWithDelimiterHeavyContent compiles a content block
// built the same way report findings are (typLines) from the exact text that
// triggered the "unclosed delimiter" failure (request 21). Skipped when the
// `typst` binary is not available.
func TestTypstReportCompilesWithDelimiterHeavyContent(t *testing.T) {
	if _, err := exec.LookPath("typst"); err != nil {
		t.Skip("typst binary not available; skipping compile check")
	}

	// Representative finding text: wildcard ranges (odd `*` count) and an
	// HTML-ish table fragment (the two cases seen in the request-21 report).
	raw := strings.Join([]string{
		`161: $$\n“ 0 . * ” , “ 1 . * ” , “ 3 . * ” , “ * ” 。\n$$`,
		`1083: <table><tr><td>一级编码</td><td>liquid chromatograph, LC</td></tr></table>`,
	}, "\n")

	src := "#[\n" + typLines(raw) + "\n]\n"

	dir := t.TempDir()
	typPath := dir + "/check.typ"
	if err := os.WriteFile(typPath, []byte(src), 0o644); err != nil {
		t.Fatalf("write typ: %v", err)
	}

	out, err := exec.Command("typst", "compile", "--root", "/", typPath, dir+"/check.pdf").CombinedOutput()
	if err != nil {
		t.Fatalf("typst compile failed: %v\n%s\n--- source ---\n%s", err, out, src)
	}
}

func TestBuildTypstVariantsUsesLocalizedMetadata(t *testing.T) {
	t.Setenv("DOC_REVIEW_REPORT_LANGUAGE", `["en","zh"]`)
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	findingsQuery := regexp.QuoteMeta(`
		SELECT id, pass, aspect, severity, finding_type, title, description,
		       COALESCE(evidence,''), COALESCE(location,''), COALESCE(suggestion,''),
		       COALESCE(confidence,0), COALESCE(review_status,'pending'), COALESCE(metadata, '{}'::jsonb)::text
		FROM kb.doc_review_findings
		WHERE input_record_id = $1 AND run_id = $2
		ORDER BY CASE severity WHEN 'high' THEN 1 WHEN 'medium' THEN 2 WHEN 'low' THEN 3 ELSE 4 END, id ASC`)
	mock.ExpectQuery(findingsQuery).
		WithArgs(int64(88), int64(99)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "pass", "aspect", "severity", "finding_type", "title", "description",
			"evidence", "location", "suggestion", "confidence", "review_status", "metadata",
		}).AddRow(
			int64(7), "P1", "grammar_spelling", "high", "typo",
			"Canonical title", "Canonical description", "Evidence", "12-13", "Canonical suggestion", 0.9, "pending",
			`{"en":{"title":"English title","description":"English description","suggestion":"English suggestion"},"zh":{"title":"中文标题","description":"中文描述","suggestion":"中文建议"}}`,
		))

	req := &RequestStatus{
		ID:            42,
		InputRecordID: 88,
		LatestRunID:   99,
		CreateTime:    "2026-07-04T12:00:00Z",
	}
	base := &ReportSkeleton{
		Meta: ReportMeta{
			ReportID:         "rpt_88_2026-07-04T12:00:00Z",
			DocumentTitle:    "Document title",
			DocumentRecordID: 88,
			GeneratedAt:      "2026-07-04T12:05:00Z",
			RunID:            99,
			TotalFindings:    1,
		},
		FindingsByPass: map[string]PassGroup{
			"P1": {
				Label: "Language & Style",
				Findings: []ReportFinding{{
					Pass:        "P1",
					Aspect:      "grammar_spelling",
					Severity:    "high",
					FindingType: "typo",
					Title:       "Canonical title",
					Description: "Canonical description",
					Suggestion:  "Canonical suggestion",
					Sources:     []SourceContext{{Source: "12: source line"}},
				}},
			},
		},
		Findings: []ReportFinding{{
			Pass:        "P1",
			Aspect:      "grammar_spelling",
			Severity:    "high",
			FindingType: "typo",
			Title:       "Canonical title",
			Description: "Canonical description",
			Suggestion:  "Canonical suggestion",
			Sources:     []SourceContext{{Source: "12: source line"}},
		}},
		Recommendations: []Recommendation{{Priority: 1, Action: "Canonical suggestion", RelatedFindingIDs: []int{1}}},
		PassOrder:       []string{"P1"},
	}

	variants, err := buildTypstVariants(context.Background(), req, base)
	if err != nil {
		t.Fatalf("buildTypstVariants: %v", err)
	}
	if len(variants) != 2 {
		t.Fatalf("variants len=%d, want 2", len(variants))
	}
	if variants[0].suffix != "report-en" || variants[1].suffix != "report-cn" {
		t.Fatalf("suffixes=(%q,%q), want (report-en, report-cn)", variants[0].suffix, variants[1].suffix)
	}
	if got := variants[0].skeleton.Findings[0].Title; got != "English title" {
		t.Fatalf("english title=%q", got)
	}
	if got := variants[1].skeleton.Findings[0].Title; got != "中文标题" {
		t.Fatalf("chinese title=%q", got)
	}
	if got := variants[1].skeleton.Recommendations[0].Action; got != "中文建议" {
		t.Fatalf("chinese recommendation=%q", got)
	}
	if got := variants[1].skeleton.Findings[0].Sources[0].Source; got != "12: source line" {
		t.Fatalf("sources lost during localization: %q", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestBuildTypstVariantsUsesSingleConfiguredLanguage(t *testing.T) {
	t.Setenv("DOC_REVIEW_REPORT_LANGUAGE", "zh")

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	findingsQuery := regexp.QuoteMeta(`
		SELECT id, pass, aspect, severity, finding_type, title, description,
		       COALESCE(evidence,''), COALESCE(location,''), COALESCE(suggestion,''),
		       COALESCE(confidence,0), COALESCE(review_status,'pending'), COALESCE(metadata, '{}'::jsonb)::text
		FROM kb.doc_review_findings
		WHERE input_record_id = $1 AND run_id = $2
		ORDER BY CASE severity WHEN 'high' THEN 1 WHEN 'medium' THEN 2 WHEN 'low' THEN 3 ELSE 4 END, id ASC`)
	mock.ExpectQuery(findingsQuery).
		WithArgs(int64(88), int64(99)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "pass", "aspect", "severity", "finding_type", "title", "description",
			"evidence", "location", "suggestion", "confidence", "review_status", "metadata",
		}).AddRow(
			int64(7), "P1", "grammar_spelling", "high", "typo",
			"Canonical title", "Canonical description", "Evidence", "12-13", "Canonical suggestion", 0.9, "pending",
			`{"zh":{"title":"中文标题","description":"中文描述","suggestion":"中文建议"}}`,
		))

	req := &RequestStatus{ID: 42, InputRecordID: 88, LatestRunID: 99, CreateTime: "2026-07-04T12:00:00Z"}
	base := &ReportSkeleton{
		Meta:            ReportMeta{ReportID: "rpt_88_2026-07-04T12:00:00Z", DocumentTitle: "Document title", DocumentRecordID: 88, GeneratedAt: "2026-07-04T12:05:00Z", RunID: 99, TotalFindings: 1},
		FindingsByPass:  map[string]PassGroup{"P1": {Label: "Language & Style", Findings: []ReportFinding{{Pass: "P1", Aspect: "grammar_spelling", Severity: "high", FindingType: "typo", Title: "Canonical title", Description: "Canonical description", Suggestion: "Canonical suggestion", Sources: []SourceContext{{Source: "12: source line"}}}}}},
		Findings:        []ReportFinding{{Pass: "P1", Aspect: "grammar_spelling", Severity: "high", FindingType: "typo", Title: "Canonical title", Description: "Canonical description", Suggestion: "Canonical suggestion", Sources: []SourceContext{{Source: "12: source line"}}}},
		Recommendations: []Recommendation{{Priority: 1, Action: "Canonical suggestion", RelatedFindingIDs: []int{1}}},
		PassOrder:       []string{"P1"},
	}

	variants, err := buildTypstVariants(context.Background(), req, base)
	if err != nil {
		t.Fatalf("buildTypstVariants: %v", err)
	}
	if len(variants) != 1 {
		t.Fatalf("variants len=%d, want 1", len(variants))
	}
	if variants[0].language != "zh" || variants[0].suffix != "report-cn" {
		t.Fatalf("variant=%+v, want zh/report-cn", variants[0])
	}
	if got := variants[0].skeleton.Findings[0].Title; got != "中文标题" {
		t.Fatalf("zh title=%q", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestFindAllReportPDFsIncludesLanguageSpecificFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{
		"20260704-1200-88-report-en.pdf",
		"20260704-1200-88-report-cn.pdf",
		"20260703-2359-88-reports.pdf",
		"20260704-1200-77-report-en.pdf",
	} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("pdf"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	got := findAllReportPDFs(dir, 88)
	if len(got) != 3 {
		t.Fatalf("matches len=%d, want 3 (%v)", len(got), got)
	}
	first := filepath.Base(got[0])
	if first != "20260704-1200-88-report-en.pdf" && first != "20260704-1200-88-report-cn.pdf" {
		t.Fatalf("newest match=%q, want language-specific report", first)
	}
}
