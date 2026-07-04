package docreviews

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
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

	stamp := time.Now().Format("20060102-1504")
	variants, err := buildTypstVariants(ctx, req, skeleton)
	if err != nil {
		return err
	}
	for _, variant := range variants {
		baseName := fmt.Sprintf("%s-%d-%s", stamp, requestID, variant.suffix)
		typPath := filepath.Join(outputDir, baseName+".typ")
		pdfPath := filepath.Join(outputDir, baseName+".pdf")

		src := buildTypstSource(variant.skeleton, req, variant.language, absTemplatePath)
		if err := os.WriteFile(typPath, []byte(src), 0o644); err != nil {
			return fmt.Errorf("write typst file %q: %w", typPath, err)
		}
		typstLogger.Info("typst source written", "request_id", requestID, "language", variant.language, "path", typPath)

		// Compile: --root / lets the #import use an absolute filesystem path.
		cmd := exec.CommandContext(ctx, "typst", "compile", "--root", "/", typPath, pdfPath)
		out, err := cmd.CombinedOutput()
		if err != nil {
			typstLogger.Warn("typst compile failed",
				"request_id", requestID, "language", variant.language, "error", err, "output", string(out))
			return fmt.Errorf("typst compile (%s): %w\n%s", variant.language, err, string(out))
		}
		typstLogger.Info("PDF generated", "request_id", requestID, "language", variant.language, "path", pdfPath)
	}
	return nil
}

type typstVariant struct {
	language string
	suffix   string
	skeleton *ReportSkeleton
}

func buildTypstVariants(ctx context.Context, req *RequestStatus, base *ReportSkeleton) ([]typstVariant, error) {
	findings, metadataByFindingID, err := loadReportFindingsWithMetadata(ctx, ApiTypes.ProjectDBHandle, req)
	if err != nil {
		return nil, fmt.Errorf("load localized findings: %w", err)
	}
	languages := docReviewReportLanguagesFromEnv()
	var variants []typstVariant
	for _, language := range languages {
		variants = append(variants, typstVariant{
			language: language,
			suffix:   reportLanguageSuffix(language),
			skeleton: buildLocalizedTypstSkeleton(ctx, req, base, findings, metadataByFindingID, language),
		})
	}
	return variants, nil
}

func docReviewReportLanguagesFromEnv() []string {
	raw := strings.TrimSpace(os.Getenv("DOC_REVIEW_REPORT_LANGUAGE"))
	if raw == "" {
		return []string{"en"}
	}

	var parsed []string
	if strings.HasPrefix(raw, "[") {
		var items []string
		if err := json.Unmarshal([]byte(raw), &items); err == nil {
			parsed = items
		}
	} else {
		var single string
		if err := json.Unmarshal([]byte(raw), &single); err == nil {
			parsed = []string{single}
		} else {
			parsed = []string{raw}
		}
	}

	seen := map[string]bool{}
	var out []string
	for _, language := range parsed {
		language = strings.ToLower(strings.TrimSpace(language))
		switch language {
		case "", "en", "en-us":
			language = "en"
		}
		if language == "" || seen[language] {
			continue
		}
		seen[language] = true
		out = append(out, language)
	}
	if len(out) == 0 {
		return []string{"en"}
	}
	return out
}

func reportLanguageSuffix(language string) string {
	switch language {
	case "zh", "zh-cn", "zh-hans":
		return "report-cn"
	default:
		return "report-" + language
	}
}

func loadReportFindingsWithMetadata(ctx context.Context, db *sql.DB, req *RequestStatus) ([]FindingItem, map[int64][]byte, error) {
	if db == nil || req == nil || req.InputRecordID == 0 || req.LatestRunID == 0 {
		return nil, nil, nil
	}
	rows, err := db.QueryContext(ctx, `
		SELECT id, pass, aspect, severity, finding_type, title, description,
		       COALESCE(evidence,''), COALESCE(location,''), COALESCE(suggestion,''),
		       COALESCE(confidence,0), COALESCE(review_status,'pending'), COALESCE(metadata, '{}'::jsonb)::text
		FROM kb.doc_review_findings
		WHERE input_record_id = $1 AND run_id = $2
		ORDER BY CASE severity WHEN 'high' THEN 1 WHEN 'medium' THEN 2 WHEN 'low' THEN 3 ELSE 4 END, id ASC`,
		req.InputRecordID, req.LatestRunID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var findings []FindingItem
	metadataByFindingID := map[int64][]byte{}
	for rows.Next() {
		var finding FindingItem
		var metadata string
		if err := rows.Scan(&finding.ID, &finding.Pass, &finding.Aspect, &finding.Severity, &finding.FindingType,
			&finding.Title, &finding.Description, &finding.Evidence, &finding.Location, &finding.Suggestion,
			&finding.Confidence, &finding.ReviewStatus, &metadata); err != nil {
			return nil, nil, err
		}
		findings = append(findings, finding)
		metadataByFindingID[finding.ID] = []byte(metadata)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return findings, metadataByFindingID, nil
}

func buildLocalizedTypstSkeleton(ctx context.Context, req *RequestStatus, base *ReportSkeleton, findings []FindingItem, metadataByFindingID map[int64][]byte, language string) *ReportSkeleton {
	if base == nil {
		return nil
	}
	if len(findings) == 0 {
		return base
	}

	localizedFindings := make([]FindingItem, 0, len(findings))
	for _, finding := range findings {
		if tr, ok := translationFromMetadata(metadataByFindingID[finding.ID], language); ok {
			finding = applyFindingTranslation(finding, tr)
		}
		localizedFindings = append(localizedFindings, finding)
	}

	gen := NewDocReviewReportGenerator()
	gen.DB = nil
	gen.PackageOrder = configuredPackageOrder()
	localized, err := gen.Build(ctx, req, localizedFindings)
	if err != nil {
		typstLogger.Warn("buildLocalizedTypstSkeleton: fallback to base skeleton", "language", language, "error", err)
		return base
	}
	localized.Meta = base.Meta
	copyLocalizedSources(localized, base)
	return localized
}

func copyLocalizedSources(dst, src *ReportSkeleton) {
	if dst == nil || src == nil {
		return
	}
	for i := range dst.Findings {
		if i < len(src.Findings) {
			dst.Findings[i].Sources = src.Findings[i].Sources
		}
	}
	for pass, dstGroup := range dst.FindingsByPass {
		srcGroup, ok := src.FindingsByPass[pass]
		if !ok {
			continue
		}
		for i := range dstGroup.Findings {
			if i < len(srcGroup.Findings) {
				dstGroup.Findings[i].Sources = srcGroup.Findings[i].Sources
			}
		}
		dst.FindingsByPass[pass] = dstGroup
	}
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

	// ── Package-level sections (hierarchical: L1 package → L2 aspects) ──
	var aspectLines []string
	findingIdx := 0
	for _, p := range skeleton.PassOrder {
		pg := skeleton.FindingsByPass[p]
		label, ok := passLabelMap[p]
		if !ok {
			label = p
		}

		// ── Package heading (Level 1) ──
		aspectLines = append(aspectLines, fmt.Sprintf(`  heading(level: 1, "%s"),`, typStr(label)))

		// ── Group findings by aspect within this package ──
		aspectFindingMap := map[string][]ReportFinding{}
		var aspectSlugs []string
		aspectSeen := map[string]bool{}
		for _, f := range pg.Findings {
			if !aspectSeen[f.Aspect] {
				aspectSeen[f.Aspect] = true
				aspectSlugs = append(aspectSlugs, f.Aspect)
			}
			aspectFindingMap[f.Aspect] = append(aspectFindingMap[f.Aspect], f)
		}

		// ── One aspect-section per aspect (Level 2) ──
		for _, aspect := range aspectSlugs {
			af := aspectFindingMap[aspect]
			var findingBlocks []string
			var aspectHighCount int

			for _, f := range af {
				findingIdx++
				fid := fmt.Sprintf("F-%02d", findingIdx)

				var blockB strings.Builder
				fmt.Fprintf(&blockB, "      review-finding(\n")
				fmt.Fprintf(&blockB, "        id: \"%s\",\n", typStr(fid))

				// Emit sources array — one dict per source location group.
				fmt.Fprintf(&blockB, "        sources: (")
				if len(f.Sources) > 0 {
					fmt.Fprintf(&blockB, "\n")
					for _, sc := range f.Sources {
						fmt.Fprintf(&blockB, "          (\n")
						if sc.Before != "" {
							fmt.Fprintf(&blockB, "            before: [%s],\n", typLines(sc.Before))
						} else {
							fmt.Fprintf(&blockB, "            before: none,\n")
						}
						fmt.Fprintf(&blockB, "            source: [%s],\n", typLines(sc.Source))
						if sc.After != "" {
							fmt.Fprintf(&blockB, "            after: [%s],\n", typLines(sc.After))
						} else {
							fmt.Fprintf(&blockB, "            after: none,\n")
						}
						fmt.Fprintf(&blockB, "          ),\n")
					}
					fmt.Fprintf(&blockB, "        ")
				}
				fmt.Fprintf(&blockB, "),\n")

				fmt.Fprintf(&blockB, "        errors: [%s],\n", typContent(f.Title))
				fmt.Fprintf(&blockB, "        explanation: [%s],\n", typContent(f.Description))
				fmt.Fprintf(&blockB, "        correction: [%s],\n", typContent(f.Suggestion))
				fmt.Fprintf(&blockB, "      ),")
				block := blockB.String()
				findingBlocks = append(findingBlocks, block)

				if f.Severity == "high" {
					aspectHighCount++
				}
			}

			assessment := buildAssessment(len(af), aspectHighCount)
			problems := buildProblems([]string{aspect})
			guidelines := buildGuidelines(af)

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
				typStr(aspect),
				findingsArg,
				typContent(assessment),
				typContent(problems),
				typContent(guidelines),
			)
			aspectLines = append(aspectLines, section)
		}
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

/*
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
*/

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

// typContentLine escapes one line of text for use inside a Typst content block.
// Backslash is escaped first to prevent double-escaping. Every Typst markup
// metacharacter must be escaped, including the paired emphasis/raw delimiters
// (`*`, `_`, “ ` “): an odd, unbalanced count of these in finding text (e.g.
// `0.*` / `1.*` wildcard ranges) otherwise opens a delimiter Typst never closes,
// failing compilation with "unclosed delimiter".
func typContentLine(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `$`, `\$`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `[`, `\[`)
	s = strings.ReplaceAll(s, `]`, `\]`)
	s = strings.ReplaceAll(s, `#`, `\#`)
	s = strings.ReplaceAll(s, `@`, `\@`)
	s = strings.ReplaceAll(s, `<`, `\<`)
	s = strings.ReplaceAll(s, `>`, `\>`)
	s = strings.ReplaceAll(s, `*`, `\*`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	s = strings.ReplaceAll(s, "`", "\\`")
	s = strings.ReplaceAll(s, `~`, `\~`)
	return s
}

// typContent escapes a single-line string for use inside a Typst content block ([]).
// HTML <table> elements are converted to Typst #table() calls instead of being escaped.
func typContent(s string) string { return typContentWithHTML(s) }

// typLines escapes a multi-line string for use inside a Typst content block.
// Each line is escaped individually; lines are joined with a Typst forced line
// break (backslash + newline) so each line renders on its own row.
// HTML <table> elements are converted to Typst #table() calls instead of being escaped.
func typLines(s string) string {
	re := htmlTableRe()
	var result strings.Builder
	lastEnd := 0
	for _, loc := range re.FindAllStringIndex(s, -1) {
		segment := s[lastEnd:loc[0]]
		if segment != "" {
			result.WriteString(escapedLines(segment))
			result.WriteString("\\\n")
		}
		result.WriteString(htmlTableToTypst(s[loc[0]:loc[1]]))
		lastEnd = loc[1]
	}
	if remaining := s[lastEnd:]; remaining != "" {
		result.WriteString(escapedLines(remaining))
	}
	return result.String()
}

// escapedLines is the original typLines logic: escape each non-empty line and
// join with Typst forced line breaks.
func escapedLines(s string) string {
	lines := strings.Split(s, "\n")
	parts := make([]string, 0, len(lines))
	for _, line := range lines {
		if esc := typContentLine(line); esc != "" {
			parts = append(parts, esc)
		}
	}
	return strings.Join(parts, "\\\n")
}

// typContentWithHTML escapes s for a Typst content block, converting any
// embedded HTML <table> elements to Typst #table() calls.
func typContentWithHTML(s string) string {
	re := htmlTableRe()
	var result strings.Builder
	lastEnd := 0
	for _, loc := range re.FindAllStringIndex(s, -1) {
		result.WriteString(typContentLine(s[lastEnd:loc[0]]))
		result.WriteString(htmlTableToTypst(s[loc[0]:loc[1]]))
		lastEnd = loc[1]
	}
	result.WriteString(typContentLine(s[lastEnd:]))
	return result.String()
}

var (
	reHTMLTable = regexp.MustCompile(`(?is)<table[^>]*>.*?</table>`)
	reTableRow  = regexp.MustCompile(`(?is)<tr[^>]*>(.*?)</tr>`)
	reTableCell = regexp.MustCompile(`(?is)<t[dh][^>]*>(.*?)</t[dh]>`)
	reAnyTag    = regexp.MustCompile(`<[^>]+>`)
)

func htmlTableRe() *regexp.Regexp { return reHTMLTable }

// htmlTableToTypst converts one HTML <table>...</table> string to a Typst
// #table() call. Cell content has inner tags stripped and HTML entities decoded
// before being escaped for Typst.
func htmlTableToTypst(tableHTML string) string {
	var allRows [][]string
	maxCols := 0
	for _, rowMatch := range reTableRow.FindAllStringSubmatch(tableHTML, -1) {
		var row []string
		for _, cellMatch := range reTableCell.FindAllStringSubmatch(rowMatch[1], -1) {
			cell := reAnyTag.ReplaceAllString(cellMatch[1], "")
			cell = decodeHTMLEntities(strings.TrimSpace(cell))
			row = append(row, cell)
		}
		if len(row) > 0 {
			allRows = append(allRows, row)
			if len(row) > maxCols {
				maxCols = len(row)
			}
		}
	}
	if maxCols == 0 {
		return typContentLine(reAnyTag.ReplaceAllString(tableHTML, ""))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "#table(\n  columns: %d,\n", maxCols)
	for _, row := range allRows {
		for _, cell := range row {
			fmt.Fprintf(&b, "  [%s],", typContentLine(cell))
		}
		b.WriteByte('\n')
	}
	b.WriteString(")")
	return b.String()
}

// decodeHTMLEntities replaces common HTML entities with their text equivalents.
func decodeHTMLEntities(s string) string {
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	s = strings.ReplaceAll(s, "&quot;", `"`)
	s = strings.ReplaceAll(s, "&#39;", "'")
	return s
}

// extractDate returns the YYYY-MM-DD prefix of an RFC3339 or similar timestamp.
func extractDate(ts string) string {
	if len(ts) >= 10 {
		return ts[:10]
	}
	return ts
}
