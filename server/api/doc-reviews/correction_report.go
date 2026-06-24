package docreviews

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/chendingplano/deepdoc/server/api/docactivity"
)

// activityKindLabel maps an activity_type to a human-readable label used in the
// correction report (table + entry headers).
var activityKindLabel = map[string]string{
	docactivity.TypeAutoFix:         "LLM Auto Fix",
	docactivity.TypeEditTool:        "Edit Tool",
	docactivity.TypeFindingDelete:   "Finding Deleted",
	docactivity.TypeStructureModify: "Document Structure Edit",
	docactivity.TypeStructureSplit:  "Document Structure Split",
	docactivity.TypeStructureDelete: "Document Structure Delete",
}

// kindOrder fixes the ordering of activity kinds in the summary table.
var kindOrder = []string{
	docactivity.TypeAutoFix,
	docactivity.TypeEditTool,
	docactivity.TypeFindingDelete,
	docactivity.TypeStructureModify,
	docactivity.TypeStructureSplit,
	docactivity.TypeStructureDelete,
}

// GenerateCorrectionReport builds a "Document Review Correction Report" Typst
// source from the activities recorded against a report's document/run and
// compiles it to PDF. Output mirrors GenerateTypstReport: files are written under
// $DOC_REVIEW_REPORTS as <stamp>-<reportID>-corrections.{typ,pdf}. When that env
// var is unset the function is a no-op. It returns the generated PDF path (empty
// when skipped).
func (c *DocReviewController) GenerateCorrectionReport(ctx context.Context, reportID int64) (string, error) {
	var inputRecordID int64
	var reviewRunID string
	var requestID int64
	err := c.DB.QueryRowContext(ctx,
		`SELECT request_id, input_record_id, COALESCE(review_run_id,'')
		 FROM kb.doc_review_reports WHERE id = $1`, reportID,
	).Scan(&requestID, &inputRecordID, &reviewRunID)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("report %d not found", reportID)
		}
		return "", fmt.Errorf("load report %d: %w", reportID, err)
	}

	req, err := c.loadRequest(ctx, requestID)
	if err != nil {
		return "", err
	}

	activities, err := docactivity.List(ctx, c.DB, inputRecordID, reviewRunID)
	if err != nil {
		return "", fmt.Errorf("load activities: %w", err)
	}

	outputDir := strings.TrimSpace(os.Getenv("DOC_REVIEW_REPORTS"))
	if outputDir == "" {
		typstLogger.Info("DOC_REVIEW_REPORTS not set; skipping correction PDF generation", "report_id", reportID)
		return "", nil
	}

	lang := strings.TrimSpace(os.Getenv("DOC_REVIEW_REPORT_LANGUAGE"))
	if lang == "" {
		lang = "en"
	}

	templatePath := strings.TrimSpace(os.Getenv("DOC_REVIEW_CORRECTION_TEMPLATE_FILENAME"))
	if templatePath == "" {
		templatePath = "docs/doc-templates/template-correction-report.typ"
	}
	absTemplatePath, err := filepath.Abs(templatePath)
	if err != nil {
		return "", fmt.Errorf("resolve template path %q: %w", templatePath, err)
	}
	if _, err := os.Stat(absTemplatePath); err != nil {
		return "", fmt.Errorf("correction template not found at %q: %w", absTemplatePath, err)
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", fmt.Errorf("create output dir %q: %w", outputDir, err)
	}

	// Document title for the cover/metadata.
	var docTitle string
	c.DB.QueryRowContext(ctx, `SELECT COALESCE(title,'') FROM kb.inputs WHERE id = $1`, inputRecordID).Scan(&docTitle)
	if docTitle == "" {
		docTitle = fmt.Sprintf("Document %d", inputRecordID)
	}
	generatedBy := req.RequesterName
	if generatedBy == "" {
		generatedBy = "Automated Review"
	}

	// Filename: yyyymmdd-hhmm-{reportID}-corrections.{ext}
	stamp := time.Now().Format("20060102-1504")
	baseName := fmt.Sprintf("%s-%d-corrections", stamp, reportID)
	typPath := filepath.Join(outputDir, baseName+".typ")
	pdfPath := filepath.Join(outputDir, baseName+".pdf")

	src := buildCorrectionTypstSource(activities, absTemplatePath, lang, docTitle, inputRecordID, generatedBy)
	if err := os.WriteFile(typPath, []byte(src), 0o644); err != nil {
		return "", fmt.Errorf("write correction typst file %q: %w", typPath, err)
	}
	typstLogger.Info("correction typst source written", "report_id", reportID, "path", typPath, "activities", len(activities))

	cmd := exec.CommandContext(ctx, "typst", "compile", "--root", "/", typPath, pdfPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		typstLogger.Warn("correction typst compile failed",
			"report_id", reportID, "error", err, "output", string(out))
		return "", fmt.Errorf("typst compile: %w\n%s", err, string(out))
	}
	typstLogger.Info("correction PDF generated", "report_id", reportID, "path", pdfPath)
	return pdfPath, nil
}

// buildCorrectionTypstSource returns the full .typ source for the correction report.
func buildCorrectionTypstSource(activities []docactivity.Activity, absTemplatePath, lang, docTitle string, inputRecordID int64, generatedBy string) string {
	var b strings.Builder

	fmt.Fprintf(&b, "#import \"%s\": *\n\n", absTemplatePath)
	if lang != "en" {
		fmt.Fprintf(&b, "#set text(lang: %q)\n\n", lang)
	}

	// ── stats (counts by kind, in fixed order) ───────────────────
	counts := map[string]int{}
	for _, a := range activities {
		counts[a.ActivityType]++
	}
	var statLines []string
	for _, k := range kindOrder {
		if counts[k] == 0 {
			continue
		}
		statLines = append(statLines, fmt.Sprintf("    (kind: \"%s\", count: %d),", typStr(activityKindLabel[k]), counts[k]))
	}

	// ── corrections (one entry per activity, chronological) ──────
	var entryLines []string
	for i, a := range activities {
		cid := fmt.Sprintf("C-%02d", i+1)
		kind := activityKindLabel[a.ActivityType]
		if kind == "" {
			kind = a.ActivityType
		}

		var blockB strings.Builder
		fmt.Fprintf(&blockB, "    correction-entry(\n")
		fmt.Fprintf(&blockB, "      id: \"%s\",\n", typStr(cid))
		fmt.Fprintf(&blockB, "      kind: \"%s\",\n", typStr(kind))
		fmt.Fprintf(&blockB, "      location: \"%s\",\n", typStr(correctionLocation(a)))
		fmt.Fprintf(&blockB, "      actor: \"%s\",\n", typStr(a.Actor))
		fmt.Fprintf(&blockB, "      time: \"%s\",\n", typStr(formatActivityTime(a.CreateTime)))

		if strings.TrimSpace(a.OldContent) != "" {
			fmt.Fprintf(&blockB, "      before: [%s],\n", typLines(a.OldContent))
		} else {
			fmt.Fprintf(&blockB, "      before: none,\n")
		}
		if strings.TrimSpace(a.NewContent) != "" {
			fmt.Fprintf(&blockB, "      after: [%s],\n", typLines(a.NewContent))
		} else {
			fmt.Fprintf(&blockB, "      after: none,\n")
		}

		if note := correctionNote(a); note != "" {
			fmt.Fprintf(&blockB, "      note: [%s],\n", typContent(note))
		} else {
			fmt.Fprintf(&blockB, "      note: none,\n")
		}
		fmt.Fprintf(&blockB, "    ),")
		entryLines = append(entryLines, blockB.String())
	}

	summary := buildCorrectionSummary(counts, len(activities))

	// ── Main template call ───────────────────────────────────────
	fmt.Fprintf(&b, "#document-correction-report(\n")
	fmt.Fprintf(&b, "  doc-title: \"%s\",\n", typStr(docTitle))
	fmt.Fprintf(&b, "  doc-id: \"%d\",\n", inputRecordID)
	fmt.Fprintf(&b, "  doc-date: \"%s\",\n", typStr(time.Now().Format("2006-01-02")))
	fmt.Fprintf(&b, "  generated-by: \"%s\",\n", typStr(generatedBy))
	fmt.Fprintf(&b, "  generated-date: \"%s\",\n", typStr(time.Now().Format("2006-01-02 15:04")))
	fmt.Fprintf(&b, "  summary: [%s],\n", typContent(summary))
	writeTypstArray(&b, "  stats", statLines)
	writeTypstArray(&b, "  corrections", entryLines)
	fmt.Fprintf(&b, ")\n")

	return b.String()
}

// correctionLocation renders the document location of an activity for display.
func correctionLocation(a docactivity.Activity) string {
	if a.PageNumber > 0 || a.LineNumber > 0 {
		return fmt.Sprintf("page %d, line %d", a.PageNumber, a.LineNumber)
	}
	return a.Location
}

// correctionNote renders a short context note from an activity's detail map.
func correctionNote(a docactivity.Activity) string {
	if a.Detail == nil {
		return ""
	}
	var parts []string
	if title := detailStr(a.Detail, "title"); title != "" {
		finding := "Finding: " + title
		var tags []string
		if aspect := detailStr(a.Detail, "aspect"); aspect != "" {
			tags = append(tags, aspect)
		}
		if sev := detailStr(a.Detail, "severity"); sev != "" {
			tags = append(tags, sev)
		}
		if len(tags) > 0 {
			finding += " (" + strings.Join(tags, ", ") + ")"
		}
		parts = append(parts, finding)
	}
	if lt := detailStr(a.Detail, "line_type"); lt != "" {
		parts = append(parts, "Line type: "+lt)
	}
	if model := detailStr(a.Detail, "model"); model != "" {
		parts = append(parts, "Model: "+model)
	}
	return strings.Join(parts, " · ")
}

// detailStr safely reads a string value from a detail map.
func detailStr(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// formatActivityTime trims a timestamp to "YYYY-MM-DD HH:MM" for display.
func formatActivityTime(ts string) string {
	ts = strings.TrimSpace(ts)
	if len(ts) >= 16 {
		// "2026-06-24 09:12:..." or "2026-06-24T09:12:..."
		return strings.Replace(ts[:16], "T", " ", 1)
	}
	return ts
}

// buildCorrectionSummary produces the high-level summary sentence.
func buildCorrectionSummary(counts map[string]int, total int) string {
	if total == 0 {
		return "No correction actions were recorded for this document during review."
	}
	var parts []string
	for _, k := range kindOrder {
		if counts[k] == 0 {
			continue
		}
		parts = append(parts, fmt.Sprintf("%d %s", counts[k], activityKindLabel[k]))
	}
	return fmt.Sprintf("A total of %d correction action(s) were applied to this document during review: %s.",
		total, strings.Join(parts, ", "))
}
