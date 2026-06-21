package docreview

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"strings"
	"time"

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

// ReportFinding is a single finding within a report.
type ReportFinding struct {
	Pass        string  `json:"pass"`
	Aspect      string  `json:"aspect"`
	Severity    string  `json:"severity"`
	FindingType string  `json:"finding_type"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Evidence    string  `json:"evidence,omitempty"`
	Location    string  `json:"location,omitempty"`
	Suggestion  string  `json:"suggestion,omitempty"`
	Confidence  float64 `json:"confidence"`
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

// renderMarkdown renders the report as Markdown.
func renderMarkdown(report *ReportSkeleton) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("# Document Review Report\n\n"))
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
