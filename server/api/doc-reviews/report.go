package docreviews

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// DocReviewReportGenerator builds structured reports from findings.
type DocReviewReportGenerator struct {
	DB *sql.DB
}

// NewDocReviewReportGenerator creates a report generator.
func NewDocReviewReportGenerator() *DocReviewReportGenerator {
	return &DocReviewReportGenerator{DB: ApiTypes.ProjectDBHandle}
}

// ReportSkeleton is the full report structure stored in report_json.
type ReportSkeleton struct {
	Meta               ReportMeta                `json:"meta"`
	ExecutiveSummary   ExecutiveSummary          `json:"executive_summary"`
	FindingsByPass     map[string]PassGroup      `json:"findings_by_pass"`
	ComplianceSummary  ComplianceSummary         `json:"compliance_summary,omitempty"`
	Findings           []ReportFinding           `json:"findings"`
	Recommendations    []Recommendation          `json:"recommendations"`
	PassOrder          []string                  `json:"pass_order,omitempty"`
}

// ReportMeta contains metadata about the report.
type ReportMeta struct {
	ReportID        string `json:"report_id"`
	DocumentTitle   string `json:"document_title"`
	DocumentRecordID int64  `json:"document_record_id"`
	GeneratedAt     string `json:"generated_at"`
	ReviewRunID     string `json:"review_run_id"`
	NumReviewersRan int    `json:"num_reviewers_ran"`
	TotalFindings   int    `json:"total_findings"`
}

// ExecutiveSummary provides a high-level overview of the review results.
type ExecutiveSummary struct {
	Text              string   `json:"text"`
	TopFindings       []string `json:"top_findings"`
	OverallAssessment string   `json:"overall_assessment"`
}

// PassGroup groups findings by review pass.
type PassGroup struct {
	Label    string          `json:"label"`
	Findings []ReportFinding `json:"findings"`
}

// SourceContext holds one source location with its surrounding context lines.
type SourceContext struct {
	Before string `json:"before,omitempty"` // up to five lines before the source
	Source string `json:"source"`           // the source line(s) from the document
	After  string `json:"after,omitempty"`  // up to five lines after the source
}

// ReportFinding is a single finding within a report.
type ReportFinding struct {
	Pass        string          `json:"pass"`
	Aspect      string          `json:"aspect"`
	Severity    string          `json:"severity"`
	FindingType string          `json:"finding_type"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Evidence    string          `json:"evidence,omitempty"`
	Location    string          `json:"location,omitempty"`
	Suggestion  string          `json:"suggestion,omitempty"`
	Confidence  float64         `json:"confidence"`
	Sources     []SourceContext `json:"sources,omitempty"` // source blocks with before/after context
}

// ComplianceSummary captures compliance-related findings.
type ComplianceSummary struct {
	ReferenceStandardsChecked     []string `json:"reference_standards_checked"`
	ProvisionsSatisfied           int      `json:"provisions_satisfied"`
	ProvisionsPartiallySatisfied  int      `json:"provisions_partially_satisfied"`
	ProvisionsNotAddressed        int      `json:"provisions_not_addressed"`
	ProvisionsNotApplicable       int      `json:"provisions_not_applicable"`
	MissingRequirements           []string `json:"missing_requirements"`
}

// Recommendation suggests an action based on finding severity.
type Recommendation struct {
	Priority          int    `json:"priority"`
	Action            string `json:"action"`
	RelatedFindingIDs []int  `json:"related_finding_ids"`
}

// Build assembles the full report for a given review request.
func (g *DocReviewReportGenerator) Build(ctx context.Context, req *RequestStatus, findings []FindingItem) (*ReportSkeleton, error) {
	// Build meta.
	report := &ReportSkeleton{
		Meta: ReportMeta{
			ReportID:          fmt.Sprintf("rpt_%d_%s", req.InputRecordID, strings.ReplaceAll(req.CreateTime, " ", "T")),
			DocumentRecordID:  req.InputRecordID,
			GeneratedAt:       timeNow(),
			ReviewRunID:       req.ReviewRunID,
			TotalFindings:     len(findings),
		},
		FindingsByPass: make(map[string]PassGroup),
	}

	// Load document metadata (nil-safe for testing).
	var docTitle string
	if g.DB != nil {
		g.DB.QueryRowContext(ctx, `SELECT COALESCE(title,'') FROM kb.inputs WHERE id = $1`, req.InputRecordID).Scan(&docTitle)
	}
	report.Meta.DocumentTitle = docTitle

	// Group findings by pass.
	passLabels := map[string]string{
		"P1": "Language & Style", "P2": "Structure & Organization",
		"P3": "Content Quality", "P4": "Consistency",
		"P5": "Technical & Compliance", "P6": "Meta & Process",
	}
	passFindings := make(map[string][]FindingItem)
	for _, f := range findings {
		passFindings[f.Pass] = append(passFindings[f.Pass], f)
	}

	// Determine how many distinct reviewers ran based on distinct passes.
	report.Meta.NumReviewersRan = len(passFindings)

	// Load document lines once for context extraction (non-fatal if unavailable).
	lineIndex := g.loadDocLines(ctx, req.InputRecordID)

	var totalHigh, totalMedium, totalLow int
	for pass, items := range passFindings {
		var rfList []ReportFinding
		for _, f := range items {
			rf := ReportFinding{
				Pass: f.Pass, Aspect: f.Aspect, Severity: f.Severity,
				FindingType: f.FindingType, Title: f.Title, Description: f.Description,
				Evidence: f.Evidence, Location: f.Location, Suggestion: f.Suggestion,
				Confidence: f.Confidence,
			}
			rf.Sources = g.buildSources(lineIndex, f)
			rfList = append(rfList, rf)
			report.Findings = append(report.Findings, rf)
			switch f.Severity {
			case "high":
				totalHigh++
			case "medium":
				totalMedium++
			default:
				totalLow++
			}
		}
		report.FindingsByPass[pass] = PassGroup{
			Label:    passLabels[pass],
			Findings: rfList,
		}
	}

	// Build pass order (only passes with findings).
	passOrder := []string{"P1", "P2", "P3", "P4", "P5", "P6"}
	for _, p := range passOrder {
		if _, ok := report.FindingsByPass[p]; ok {
			report.PassOrder = append(report.PassOrder, p)
		}
	}

	// Build executive summary.
	overall := "pass_with_issues"
	if totalHigh > 0 {
		overall = "fail"
	} else if totalHigh == 0 && totalMedium == 0 && totalLow == 0 {
		overall = "pass_with_issues" // no findings still needs review
	}

	topFindings := make([]string, 0, 3)
	for _, f := range report.Findings {
		if f.Severity == "high" && len(topFindings) < 3 {
			topFindings = append(topFindings, f.Title)
		}
	}
	if len(topFindings) == 0 && len(report.Findings) > 0 {
		topFindings = append(topFindings, report.Findings[0].Title)
	}

	summaryText := fmt.Sprintf("Reviewed %d aspects across %d passes. Found %d findings (%d high, %d medium, %d low).",
		len(findings), report.Meta.NumReviewersRan, len(findings), totalHigh, totalMedium, totalLow)
	if totalHigh > 0 {
		summaryText += fmt.Sprintf(" %d high-severity issues require immediate attention.", totalHigh)
	}
	report.ExecutiveSummary = ExecutiveSummary{
		Text:              summaryText,
		TopFindings:       topFindings,
		OverallAssessment: overall,
	}

	// Build compliance summary (findings tagged with reference docs).
	var refsChecked []string
	refSeen := map[string]bool{}
	var missingReqs []string
	for _, f := range findings {
		if f.FindingType == "missing_requirement" || f.FindingType == "missing_provision" {
			missingReqs = append(missingReqs, f.Title)
		}
	}
	for _, rd := range req.ReferenceDocs {
		if !refSeen[rd.DocNo] {
			refsChecked = append(refsChecked, rd.DocNo)
			refSeen[rd.DocNo] = true
		}
	}
	report.ComplianceSummary = ComplianceSummary{
		ReferenceStandardsChecked: refsChecked,
		MissingRequirements:       missingReqs,
	}

	// Build recommendations from high-severity findings.
	for i, f := range report.Findings {
		if f.Severity == "high" {
			report.Recommendations = append(report.Recommendations, Recommendation{
				Priority:          len(report.Recommendations) + 1,
				Action:            f.Suggestion,
				RelatedFindingIDs: []int{i + 1},
			})
		}
	}

	return report, nil
}

// Persist saves the report to kb.doc_review_reports and returns the report ID.
func (g *DocReviewReportGenerator) Persist(ctx context.Context, req *RequestStatus, report *ReportSkeleton) (int64, error) {
	reportJSON, err := json.Marshal(report)
	if err != nil {
		return 0, fmt.Errorf("marshal report: %w", err)
	}
	markdown := renderMarkdown(report)

	var id int64
	err = g.DB.QueryRowContext(ctx, `
		INSERT INTO kb.doc_review_reports
			(request_id, input_record_id, review_run_id, report_json, report_markdown,
			 executive_summary, total_findings, high_count, medium_count, low_count,
			 overall_assessment)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING id`,
		req.ID, req.InputRecordID, req.ReviewRunID, reportJSON, markdown,
		report.ExecutiveSummary.Text, report.Meta.TotalFindings,
		countBySeverity(report.Findings, "high"),
		countBySeverity(report.Findings, "medium"),
		countBySeverity(report.Findings, "low"),
		report.ExecutiveSummary.OverallAssessment,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert report: %w", err)
	}
	return id, nil
}

// GetReport returns the full report from the database.
func (g *DocReviewReportGenerator) GetReport(ctx context.Context, reportID int64) (*ReportDetail, error) {
	var d ReportDetail
	var execSummary, markdown string
	var reportJSONBytes []byte

	err := g.DB.QueryRowContext(ctx, `
		SELECT id, request_id, input_record_id, review_run_id,
		       total_findings, high_count, medium_count, low_count,
		       overall_assessment, create_time::text,
		       executive_summary, report_json::text, report_markdown
		FROM kb.doc_review_reports WHERE id = $1`, reportID,
	).Scan(&d.ID, &d.RequestID, &d.InputRecordID, &d.ReviewRunID,
		&d.TotalFindings, &d.HighCount, &d.MediumCount, &d.LowCount,
		&d.OverallAssessment, &d.CreateTime,
		&execSummary, &reportJSONBytes, &markdown)
	if err != nil {
		return nil, fmt.Errorf("load report %d: %w", reportID, err)
	}
	d.ExecutiveSummary = execSummary
	d.ReportMarkdown = markdown
	json.Unmarshal(reportJSONBytes, &d.ReportJSON)
	return &d, nil
}

// GetReportHTML renders the HTML template for a report.
func (g *DocReviewReportGenerator) GetReportHTML(ctx context.Context, reportID int64) (string, error) {
	detail, err := g.GetReport(ctx, reportID)
	if err != nil {
		return "", err
	}
	reportJSON, err := json.Marshal(detail.ReportJSON)
	if err != nil {
		return "", fmt.Errorf("marshal report json: %w", err)
	}
	var skeleton ReportSkeleton
	if err := json.Unmarshal(reportJSON, &skeleton); err != nil {
		return "", fmt.Errorf("unmarshal report skeleton: %w", err)
	}
	return renderHTML(&skeleton)
}

func countBySeverity(findings []ReportFinding, sev string) int {
	var n int
	for _, f := range findings {
		if f.Severity == sev {
			n++
		}
	}
	return n
}

func timeNow() string { return time.Now().UTC().Format(time.RFC3339) }

// loadDocLines loads the document line file for the given input record and
// returns a map of line number → line content text. Returns nil (non-fatal)
// when the DB is unavailable, the record is missing, or the file cannot be read.
func (g *DocReviewReportGenerator) loadDocLines(ctx context.Context, inputRecordID int64) map[int]string {
	if g.DB == nil {
		return nil
	}
	rec, err := docprocessing.DocMetadataSQLStore{DB: g.DB}.GetInputRecord(ctx, inputRecordID)
	if err != nil {
		typstLogger.Warn("loadDocLines: get input record", "record_id", inputRecordID, "error", err)
		return nil
	}
	lineFilePath, err := docprocessing.ResolveInputFilePath(
		docprocessing.LineFileGeneratedEvent{RecordID: inputRecordID},
		rec.ResultFilename, rec.ParserName, rec.StagingFilename,
	)
	if err != nil {
		typstLogger.Warn("loadDocLines: resolve line file", "record_id", inputRecordID, "error", err)
		return nil
	}
	body, err := os.ReadFile(lineFilePath)
	if err != nil {
		typstLogger.Warn("loadDocLines: read line file", "path", lineFilePath, "error", err)
		return nil
	}
	lines, err := docprocessing.ParseInputLinesIncludingTOC(body)
	if err != nil {
		typstLogger.Warn("loadDocLines: parse line file", "path", lineFilePath, "error", err)
		return nil
	}
	index := make(map[int]string, len(lines))
	for _, l := range lines {
		index[l.LineNo] = l.Content
	}
	return index
}

// extractFirstInt returns the first positive integer found in s, or 0.
func extractFirstInt(s string) int {
	s = strings.TrimSpace(s)
	var acc strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			acc.WriteRune(r)
		} else if acc.Len() > 0 {
			break
		}
	}
	if acc.Len() == 0 {
		return 0
	}
	n, _ := strconv.Atoi(acc.String())
	return n
}

// parseLocationRange extracts line numbers from a location string.
//   - "162"    → [162]
//   - "53-56"  → [53, 54, 55, 56]
//   - "53, 87" → [53, 87]
func parseLocationRange(location string) []int {
	location = strings.TrimSpace(location)
	if location == "" {
		return nil
	}
	// Detect "N-M" range: look for a hyphen between two digit runs.
	if idx := strings.IndexByte(location, '-'); idx > 0 {
		after := strings.TrimSpace(location[idx+1:])
		if len(after) > 0 && after[0] >= '0' && after[0] <= '9' {
			start := extractFirstInt(location[:idx])
			end := extractFirstInt(after)
			if start > 0 && end >= start && end-start < 200 {
				out := make([]int, end-start+1)
				for i := range out {
					out[i] = start + i
				}
				return out
			}
		}
	}
	// Comma- or semicolon-separated list of numbers.
	var result []int
	seen := map[int]bool{}
	for _, part := range strings.FieldsFunc(location, func(r rune) bool {
		return r == ',' || r == ';'
	}) {
		if n := extractFirstInt(part); n > 0 && !seen[n] {
			seen[n] = true
			result = append(result, n)
		}
	}
	return result
}

// groupConsecutive partitions a sorted slice of integers into runs of
// consecutive values, e.g. [53,54,55,56,87] → [[53,54,55,56],[87]].
func groupConsecutive(nums []int) [][]int {
	if len(nums) == 0 {
		return nil
	}
	var groups [][]int
	cur := []int{nums[0]}
	for _, n := range nums[1:] {
		if n == cur[len(cur)-1]+1 {
			cur = append(cur, n)
		} else {
			groups = append(groups, cur)
			cur = []int{n}
		}
	}
	return append(groups, cur)
}

// contextLines returns up to (end-start+1) lines from the index in [start,end],
// formatted as "N: content", joined by newlines.
func contextLines(index map[int]string, start, end int) string {
	if len(index) == 0 {
		return ""
	}
	var parts []string
	for n := start; n <= end; n++ {
		if n > 0 {
			if content, ok := index[n]; ok {
				parts = append(parts, fmt.Sprintf("%d: %s", n, content))
			}
		}
	}
	return strings.Join(parts, "\n")
}

// buildSources constructs the []SourceContext for a finding.
// Each distinct location group becomes one SourceContext with ±5 context lines.
func (g *DocReviewReportGenerator) buildSources(lineIndex map[int]string, f FindingItem) []SourceContext {
	lineNos := parseLocationRange(f.Location)

	if len(lineNos) == 0 {
		// No parseable line numbers — use location+evidence text as-is.
		src := f.Location
		if f.Evidence != "" {
			if src != "" {
				src += ": " + f.Evidence
			} else {
				src = f.Evidence
			}
		}
		if src == "" {
			src = "No specific location identified."
		}
		return []SourceContext{{Source: src}}
	}

	var sources []SourceContext
	for _, group := range groupConsecutive(lineNos) {
		start, end := group[0], group[len(group)-1]

		// Build source text from the line index.
		var srcLines []string
		for n := start; n <= end; n++ {
			if content, ok := lineIndex[n]; ok {
				srcLines = append(srcLines, fmt.Sprintf("%d: %s", n, content))
			}
		}
		var sourceStr string
		if len(srcLines) > 0 {
			sourceStr = strings.Join(srcLines, "\n")
		} else {
			// Index unavailable — fall back to location+evidence.
			sourceStr = f.Location
			if f.Evidence != "" {
				if sourceStr != "" {
					sourceStr += ": " + f.Evidence
				} else {
					sourceStr = f.Evidence
				}
			}
		}

		sources = append(sources, SourceContext{
			Before: contextLines(lineIndex, start-5, start-1),
			Source: sourceStr,
			After:  contextLines(lineIndex, end+1, end+5),
		})
	}
	return sources
}

// renderMarkdown renders the report as Markdown.
func renderMarkdown(report *ReportSkeleton) string {
	var b strings.Builder
	b.WriteString("# Document Review Report\n\n")
	b.WriteString(fmt.Sprintf("**Document:** %s (ID: %d)\n", report.Meta.DocumentTitle, report.Meta.DocumentRecordID))
	b.WriteString(fmt.Sprintf("**Generated:** %s\n", report.Meta.GeneratedAt))
	b.WriteString(fmt.Sprintf("**Total Findings:** %d\n\n", report.Meta.TotalFindings))

	b.WriteString("## Executive Summary\n\n")
	b.WriteString(report.ExecutiveSummary.Text + "\n\n")
	b.WriteString(fmt.Sprintf("**Assessment:** %s\n\n", report.ExecutiveSummary.OverallAssessment))

	if len(report.ExecutiveSummary.TopFindings) > 0 {
		b.WriteString("### Top Findings\n")
		for _, tf := range report.ExecutiveSummary.TopFindings {
			b.WriteString(fmt.Sprintf("- %s\n", tf))
		}
		b.WriteString("\n")
	}

	passOrder := []string{"P1", "P2", "P3", "P4", "P5", "P6"}
	for _, p := range passOrder {
		pg, ok := report.FindingsByPass[p]
		if !ok || len(pg.Findings) == 0 {
			continue
		}
		b.WriteString(fmt.Sprintf("## %s — %s\n\n", p, pg.Label))
		for _, f := range pg.Findings {
			b.WriteString(fmt.Sprintf("### [%s] %s\n", strings.ToUpper(f.Severity), f.Title))
			b.WriteString(fmt.Sprintf("**Aspect:** %s | **Type:** %s\n", f.Aspect, f.FindingType))
			if f.Description != "" {
				b.WriteString(fmt.Sprintf("\n%s\n", f.Description))
			}
			if f.Suggestion != "" {
				b.WriteString(fmt.Sprintf("\n*Suggestion:* %s\n", f.Suggestion))
			}
			if f.Location != "" {
				b.WriteString(fmt.Sprintf("\n*Location:* %s\n", f.Location))
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}

// renderHTML renders the HTML report from a ReportSkeleton.
func renderHTML(report *ReportSkeleton) (string, error) {
	tmpl := template.Must(template.New("report").Parse(reportHTMLTemplate))
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, report); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// ListReportPDFs returns all review-report PDF files on disk for the document
// associated with reportID. The entry for the current report is marked
// IsCurrent=true. Returns an empty slice (not an error) when DOC_REVIEW_REPORTS
// is unset or no PDF files match.
func (g *DocReviewReportGenerator) ListReportPDFs(ctx context.Context, reportID int64) ([]ReportPDFFile, error) {
	d, err := g.GetReport(ctx, reportID)
	if err != nil {
		return nil, err
	}

	// All reports for this document, newest first.
	rows, err := g.DB.QueryContext(ctx, `
		SELECT id, request_id, create_time::text
		FROM kb.doc_review_reports
		WHERE input_record_id = $1
		ORDER BY id DESC`, d.InputRecordID)
	if err != nil {
		return nil, fmt.Errorf("list reports for document %d: %w", d.InputRecordID, err)
	}
	defer rows.Close()

	type reportMeta struct {
		ID         int64
		RequestID  int64
		CreateTime string
	}
	var metas []reportMeta
	for rows.Next() {
		var m reportMeta
		if err := rows.Scan(&m.ID, &m.RequestID, &m.CreateTime); err != nil {
			return nil, err
		}
		metas = append(metas, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	outputDir := strings.TrimSpace(os.Getenv("DOC_REVIEW_REPORTS"))
	if outputDir == "" {
		return []ReportPDFFile{}, nil
	}

	var files []ReportPDFFile
	for _, m := range metas {
		match := findReportPDF(outputDir, m.RequestID)
		if match == "" {
			continue
		}
		files = append(files, ReportPDFFile{
			RequestID:  m.RequestID,
			ReportID:   m.ID,
			FileName:   filepath.Base(match),
			CreateTime: m.CreateTime,
			IsCurrent:  m.ID == reportID,
		})
	}
	return files, nil
}

// FindReportPDFPath returns the filesystem path of the most recent PDF file for
// the given report. Returns ("", nil) when DOC_REVIEW_REPORTS is unset or no
// file matches.
func (g *DocReviewReportGenerator) FindReportPDFPath(ctx context.Context, reportID int64) (string, error) {
	d, err := g.GetReport(ctx, reportID)
	if err != nil {
		return "", err
	}
	outputDir := strings.TrimSpace(os.Getenv("DOC_REVIEW_REPORTS"))
	if outputDir == "" {
		return "", nil
	}
	return findReportPDF(outputDir, d.RequestID), nil
}

// findReportPDF scans outputDir for a PDF matching the given requestID under
// either the current naming convention (<stamp>-<id>-reports.pdf) or the
// legacy one (<stamp>-<id>reports.pdf, no hyphen before "reports"). Returns
// the lexicographically last match (newest stamp), or "" when nothing is found.
func findReportPDF(outputDir string, requestID int64) string {
	var matches []string
	for _, pat := range []string{
		filepath.Join(outputDir, fmt.Sprintf("*-%d-reports.pdf", requestID)),
		filepath.Join(outputDir, fmt.Sprintf("*-%dreports.pdf", requestID)),
	} {
		m, _ := filepath.Glob(pat)
		matches = append(matches, m...)
	}
	if len(matches) == 0 {
		return ""
	}
	// Sort ascending so the last element is the newest stamp.
	sort.Strings(matches)
	return matches[len(matches)-1]
}

// reportHTMLTemplate is the HTML template for online report viewing.
const reportHTMLTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>{{.Meta.DocumentTitle}} — Review Report</title>
<style>
  body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; max-width: 960px; margin: 0 auto; padding: 2rem; color: #1a1a2e; background: #f8f9fa; }
  h1 { color: #1a1a2e; border-bottom: 2px solid #4361ee; padding-bottom: 0.5rem; }
  h2 { color: #3a0ca3; margin-top: 2rem; }
  h3 { color: #4361ee; }
  .meta { color: #64748b; font-size: 0.9rem; }
  .assessment { display: inline-block; padding: 0.25rem 0.75rem; border-radius: 4px; font-weight: 600; }
  .assessment.fail { background: #fee2e2; color: #dc2626; }
  .assessment.pass_with_issues { background: #fef9c3; color: #a16207; }
  .finding { background: white; border: 1px solid #e2e8f0; border-radius: 8px; padding: 1rem; margin: 0.75rem 0; }
  .finding.high { border-left: 4px solid #dc2626; }
  .finding.medium { border-left: 4px solid #f59e0b; }
  .finding.low { border-left: 4px solid #10b981; }
  .sev-high { color: #dc2626; font-weight: 600; }
  .sev-medium { color: #f59e0b; font-weight: 600; }
  .sev-low { color: #10b981; font-weight: 600; }
  .badge { display: inline-block; padding: 0.15rem 0.5rem; border-radius: 999px; font-size: 0.75rem; background: #e2e8f0; }
  .summary-cards { display: flex; gap: 1rem; margin: 1rem 0; }
  .summary-card { background: white; border: 1px solid #e2e8f0; border-radius: 8px; padding: 1rem 1.5rem; flex: 1; text-align: center; }
  .summary-card .count { font-size: 2rem; font-weight: 700; }
  .summary-card .label { font-size: 0.8rem; color: #64748b; }
  .summary-card.high .count { color: #dc2626; }
  .summary-card.medium .count { color: #f59e0b; }
  .summary-card.low .count { color: #10b981; }
</style>
</head>
<body>
<h1>Document Review Report</h1>
<p class="meta">Document: {{.Meta.DocumentTitle}} (ID: {{.Meta.DocumentRecordID}})<br>
Generated: {{.Meta.GeneratedAt}}<br>
Review Run: {{.Meta.ReviewRunID}}</p>

<div class="summary-cards">
  <div class="summary-card high"><div class="count">{{.Meta.TotalFindings}}</div><div class="label">Total Findings</div></div>
</div>

<h2>Executive Summary</h2>
<p>{{.ExecutiveSummary.Text}}</p>
<p>Assessment: <span class="assessment {{.ExecutiveSummary.OverallAssessment}}">{{.ExecutiveSummary.OverallAssessment}}</span></p>

{{if .ExecutiveSummary.TopFindings}}
<h3>Top Findings</h3>
<ul>{{range .ExecutiveSummary.TopFindings}}<li>{{.}}</li>{{end}}</ul>
{{end}}

{{range $pass := .PassOrder}}{{$pg := index $.FindingsByPass $pass}}
<h2>{{$pass}} — {{$pg.Label}}</h2>
{{range $pg.Findings}}
<div class="finding {{.Severity}}">
  <strong>{{.Title}}</strong>
  <span class="sev-{{.Severity}}">[{{.Severity}}]</span>
  <span class="badge">{{.Aspect}}</span>
  <p>{{.Description}}</p>
  {{if .Suggestion}}<p><em>Suggestion:</em> {{.Suggestion}}</p>{{end}}
  {{if .Location}}<p class="meta">Location: {{.Location}}</p>{{end}}
</div>
{{end}}
{{end}}

{{if .ComplianceSummary.MissingRequirements}}
<h2>Compliance Gaps</h2>
<ul>{{range .ComplianceSummary.MissingRequirements}}<li>{{.}}</li>{{end}}</ul>
{{end}}
</body>
</html>`
