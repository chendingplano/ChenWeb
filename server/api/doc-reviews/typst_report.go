package docreviews

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/loggerutil"
)

var typstLogger = loggerutil.CreateDefaultLogger("DR14")

var passLabelMap = map[string]string{
	"P1": "Language & Style",
	"P2": "Structure & Organization",
	"P3": "Content Quality",
	"P4": "Consistency",
	"P5": "Technical & Compliance",
	"P6": "Meta & Process",
}

// GenerateTypstReport generates a Typst source file from the review report and
// compiles it to PDF. Output is written under $DOC_REVIEW_REPORTS. If that env
// var is unset the function is a no-op. Errors are non-fatal: callers should
// log and continue.
func GenerateTypstReport(ctx context.Context, requestID int64, skeleton *ReportSkeleton, req *RequestStatus) error {
	outputDir := strings.TrimSpace(os.Getenv("DOC_REVIEW_REPORTS"))
	if outputDir == "" {
		typstLogger.Info("DOC_REVIEW_REPORTS not set; skipping PDF generation", "request_id", requestID)
		return nil
	}

	lang := strings.TrimSpace(os.Getenv("DOC_REVIEW_REPORT_LANGUAGE"))
	if lang == "" {
		lang = "en"
	}

	templatePath := strings.TrimSpace(os.Getenv("DOC_REVIEW_TEMPLATE_FILENAME"))
	if templatePath == "" {
		templatePath = "docs/doc-templates/template-document-report.typ"
	}
	absTemplatePath, err := filepath.Abs(templatePath)
	if err != nil {
		return fmt.Errorf("resolve template path %q: %w", templatePath, err)
	}
	if _, err := os.Stat(absTemplatePath); err != nil {
		return fmt.Errorf("template not found at %q: %w", absTemplatePath, err)
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("create output dir %q: %w", outputDir, err)
	}

	// Filename: yyyymmdd-hhmm-{requestID}reports.{ext}
	stamp := time.Now().Format("20060102-1504")
	baseName := fmt.Sprintf("%s-%dreports", stamp, requestID)
	typPath := filepath.Join(outputDir, baseName+".typ")
	pdfPath := filepath.Join(outputDir, baseName+".pdf")

	src := buildTypstSource(skeleton, req, lang, absTemplatePath)
	if err := os.WriteFile(typPath, []byte(src), 0o644); err != nil {
		return fmt.Errorf("write typst file %q: %w", typPath, err)
	}
	typstLogger.Info("typst source written", "request_id", requestID, "path", typPath)

	// Compile: --root / lets the #import use an absolute filesystem path.
	cmd := exec.CommandContext(ctx, "typst", "compile", "--root", "/", typPath, pdfPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		typstLogger.Warn("typst compile failed",
			"request_id", requestID, "error", err, "output", string(out))
		return fmt.Errorf("typst compile: %w\n%s", err, string(out))
	}
	typstLogger.Info("PDF generated", "request_id", requestID, "path", pdfPath)
	return nil
}

// buildTypstSource returns the full .typ source for the review report.
func buildTypstSource(skeleton *ReportSkeleton, req *RequestStatus, lang, absTemplatePath string) string {
	var b strings.Builder

	// Import template functions.  Module content output is discarded; only
	// the #let bindings become available in this document.
	fmt.Fprintf(&b, "#import \"%s\": *\n\n", absTemplatePath)

	// Propagate non-English language to the document level.  The template's
	// own set-text(lang:"en") is scoped inside the function body and will
	// override this for content the function generates; this line covers any
	// surrounding markup we might add in the future.
	if lang != "en" {
		fmt.Fprintf(&b, "#set text(lang: %q)\n\n", lang)
	}

	// ── Metadata ────────────────────────────────────────────────
	docTitle := skeleton.Meta.DocumentTitle
	docID := skeleton.Meta.ReportID
	reviewDate := extractDate(skeleton.Meta.GeneratedAt)
	reviewer := req.RequesterName
	if reviewer == "" {
		reviewer = "Automated Review"
	}

	// Review scope: human-readable list of pass labels.
	var scopeParts []string
	for _, p := range skeleton.PassOrder {
		if label, ok := passLabelMap[p]; ok {
			scopeParts = append(scopeParts, label)
		}
	}
	reviewScope := strings.Join(scopeParts, ", ")

	// ── aspect-stats ─────────────────────────────────────────────
	var statLines []string
	for _, p := range skeleton.PassOrder {
		pg := skeleton.FindingsByPass[p]
		label := passLabelMap[p]
		statLines = append(statLines, fmt.Sprintf("    (aspect: \"%s\", count: %d),", typStr(label), len(pg.Findings)))
	}

	// ── aspects (one section per pass) ───────────────────────────
	var aspectLines []string
	findingIdx := 0
	for _, p := range skeleton.PassOrder {
		pg := skeleton.FindingsByPass[p]
		label, ok := passLabelMap[p]
		if !ok {
			label = p
		}

		var findingBlocks []string
		var highCount int
		aspectSeen := map[string]bool{}
		var distinctAspects []string

		for _, f := range pg.Findings {
			findingIdx++
			fid := fmt.Sprintf("F-%02d", findingIdx)

			related := buildRelated(f.Location, f.Evidence)

			block := fmt.Sprintf(
				"      review-finding(\n"+
					"        id: \"%s\",\n"+
					"        related: [%s],\n"+
					"        errors: [%s],\n"+
					"        explanation: [%s],\n"+
					"        correction: [%s],\n"+
					"      ),",
				typStr(fid),
				typContent(related),
				typContent(f.Title),
				typContent(f.Description),
				typContent(f.Suggestion),
			)
			findingBlocks = append(findingBlocks, block)

			if f.Severity == "high" {
				highCount++
			}
			if !aspectSeen[f.Aspect] {
				aspectSeen[f.Aspect] = true
				distinctAspects = append(distinctAspects, f.Aspect)
			}
		}

		assessment := buildAssessment(len(pg.Findings), highCount)
		problems := buildProblems(distinctAspects)
		guidelines := buildGuidelines(pg.Findings)

		var findingsArg string
		if len(findingBlocks) > 0 {
			findingsArg = "\n" + strings.Join(findingBlocks, "\n") + "\n    "
		}

		section := fmt.Sprintf(
			"    aspect-section(\n"+
				"      title: \"%s\",\n"+
				"      findings: (%s),\n"+
				"      assessment: [%s],\n"+
				"      problems: [%s],\n"+
				"      guidelines: [%s],\n"+
				"    ),",
			typStr(label),
			findingsArg,
			typContent(assessment),
			typContent(problems),
			typContent(guidelines),
		)
		aspectLines = append(aspectLines, section)
	}

	// ── grounding refs (from reference documents) ─────────────────
	var groundingLines []string
	for i, rd := range req.ReferenceDocs {
		gid := fmt.Sprintf("[G%d]", i+1)
		groundingLines = append(groundingLines, fmt.Sprintf(
			"    (id: \"%s\", title: \"%s\", description: \"%s\"),",
			typStr(gid), typStr(rd.Title), typStr(rd.DocNo),
		))
	}

	// ── supporting refs (from high-severity recommendations) ───────
	var supportingLines []string
	for i, rec := range skeleton.Recommendations {
		sid := fmt.Sprintf("[S%d]", i+1)
		supportingLines = append(supportingLines, fmt.Sprintf(
			"    (id: \"%s\", title: \"%s\", description: \"\"),",
			typStr(sid), typStr(rec.Action),
		))
	}

	// ── Main template call ─────────────────────────────────────────
	fmt.Fprintf(&b, "#document-review-report(\n")
	fmt.Fprintf(&b, "  doc-title: \"%s\",\n", typStr(docTitle))
	fmt.Fprintf(&b, "  doc-id: \"%s\",\n", typStr(docID))
	fmt.Fprintf(&b, "  doc-date: \"%s\",\n", typStr(reviewDate))
	fmt.Fprintf(&b, "  reviewer: \"%s\",\n", typStr(reviewer))
	fmt.Fprintf(&b, "  review-date: \"%s\",\n", typStr(reviewDate))
	fmt.Fprintf(&b, "  review-scope: \"%s\",\n", typStr(reviewScope))
	fmt.Fprintf(&b, "  summary: [%s],\n", typContent(skeleton.ExecutiveSummary.Text))
	writeTypstArray(&b, "  aspect-stats", statLines)
	writeTypstArray(&b, "  aspects", aspectLines)
	writeTypstArray(&b, "  grounding-refs", groundingLines)
	writeTypstArray(&b, "  supporting-refs", supportingLines)
	fmt.Fprintf(&b, ")\n")

	return b.String()
}

// writeTypstArray writes a named Typst array argument.
func writeTypstArray(b *strings.Builder, name string, lines []string) {
	if len(lines) == 0 {
		fmt.Fprintf(b, "%s: (),\n", name)
		return
	}
	fmt.Fprintf(b, "%s: (\n", name)
	for _, l := range lines {
		fmt.Fprintf(b, "%s\n", l)
	}
	fmt.Fprintf(b, "  ),\n")
}

// buildRelated constructs the "related source lines" snippet from a finding.
func buildRelated(location, evidence string) string {
	switch {
	case location != "" && evidence != "":
		return location + ": " + evidence
	case location != "":
		return location
	case evidence != "":
		return evidence
	default:
		return "No specific location identified."
	}
}

// buildAssessment produces the overall-assessment sentence for a pass section.
func buildAssessment(total, high int) string {
	if total == 0 {
		return "No issues found in this section."
	}
	s := fmt.Sprintf("Found %d finding(s) in this section.", total)
	if high > 0 {
		s += fmt.Sprintf(" %d high-severity issue(s) require immediate attention.", high)
	}
	return s
}

// buildProblems summarises the distinct aspects that have issues.
func buildProblems(aspects []string) string {
	if len(aspects) == 0 {
		return "No major issues identified."
	}
	return "Issues found in: " + strings.Join(aspects, ", ") + "."
}

// buildGuidelines returns bullet-point guidelines from high-severity suggestions.
func buildGuidelines(findings []ReportFinding) string {
	var lines []string
	for _, f := range findings {
		if f.Severity == "high" && strings.TrimSpace(f.Suggestion) != "" {
			lines = append(lines, "- "+f.Suggestion)
		}
	}
	if len(lines) == 0 {
		return "Continue monitoring this area."
	}
	return strings.Join(lines, "\n")
}

// typStr escapes s for use inside a Typst string literal (between "").
func typStr(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

// typContent escapes s for use inside a Typst content block ([]).
// Backslash must be escaped first to avoid double-escaping.
func typContent(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `[`, `\[`)
	s = strings.ReplaceAll(s, `]`, `\]`)
	s = strings.ReplaceAll(s, `#`, `\#`)
	s = strings.ReplaceAll(s, `@`, `\@`)
	return s
}

// extractDate returns the YYYY-MM-DD prefix of an RFC3339 or similar timestamp.
func extractDate(ts string) string {
	if len(ts) >= 10 {
		return ts[:10]
	}
	return ts
}
