package docreviews

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
)

// LineEdit is one (line number, content) pair exchanged with the GUI for the
// Edit Tool and the LLM Auto Fix preview.
type LineEdit struct {
	LineNo  int    `json:"line_no"`
	Content string `json:"content"`
}

// AutoFixResult is the outcome of an LLM Auto Fix attempt. When Fixable is false
// the GUI surfaces Message to the user; otherwise Original/Corrected hold the
// before/after of the lines that were rewritten in the line-file.
type AutoFixResult struct {
	Fixable   bool       `json:"fixable"`
	Message   string     `json:"message,omitempty"`
	Original  []LineEdit `json:"original,omitempty"`
	Corrected []LineEdit `json:"corrected,omitempty"`
}

// findingContext is the subset of a finding row needed to drive a fix.
type findingContext struct {
	ID            int64
	InputRecordID int64
	ReviewRunID   string
	Location      string
	Title         string
	Description   string
	Suggestion    string
	Aspect        string
	Severity      string
	Evidence      string
}

// loadFindingContext loads one finding row by id.
func (c *DocReviewController) loadFindingContext(ctx context.Context, id int64) (*findingContext, error) {
	var f findingContext
	err := c.DB.QueryRowContext(ctx, `
		SELECT id, input_record_id, COALESCE(review_run_id,''), COALESCE(location,''),
		       title, COALESCE(description,''), COALESCE(suggestion,''),
		       aspect, severity, COALESCE(evidence,'')
		FROM kb.doc_review_findings WHERE id = $1`, id,
	).Scan(&f.ID, &f.InputRecordID, &f.ReviewRunID, &f.Location,
		&f.Title, &f.Description, &f.Suggestion, &f.Aspect, &f.Severity, &f.Evidence)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("finding %d not found", id)
		}
		return nil, fmt.Errorf("load finding %d: %w", id, err)
	}
	return &f, nil
}

// resolveLineFilePath returns the absolute path of the extracted line-file for a
// document record (the same file report.go's loadDocLines reads).
func resolveLineFilePath(ctx context.Context, db *sql.DB, recordID int64) (string, error) {
	if db == nil {
		return "", fmt.Errorf("no database handle")
	}
	rec, err := docprocessing.DocMetadataSQLStore{DB: db}.GetInputRecord(ctx, recordID)
	if err != nil {
		return "", fmt.Errorf("get input record %d: %w", recordID, err)
	}
	path, err := docprocessing.ResolveInputFilePath(
		docprocessing.LineFileGeneratedEvent{RecordID: recordID},
		rec.ResultFilename, rec.ParserName, rec.StagingFilename,
	)
	if err != nil {
		return "", fmt.Errorf("resolve line file for record %d: %w", recordID, err)
	}
	return path, nil
}

// readLineContents returns the current content (field 7) of each requested line
// number from the line-file at path.
func readLineContents(path string, lineNos []int) (map[int]string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read line file: %w", err)
	}
	want := make(map[int]bool, len(lineNos))
	for _, n := range lineNos {
		want[n] = true
	}
	out := make(map[int]string, len(lineNos))
	for _, raw := range strings.Split(string(body), "\n") {
		raw = strings.TrimRight(raw, "\r")
		fields := strings.Split(raw, "\t")
		if len(fields) < 7 {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(fields[0]))
		if err != nil {
			continue
		}
		if want[n] {
			out[n] = fields[6]
		}
	}
	return out, nil
}

// applyLineEdits rewrites the line-file at path, replacing only the content
// (the 7th tab-separated field) of each edited line number. All other fields and
// every untouched physical line are preserved verbatim. Returns the number of
// lines actually changed.
func applyLineEdits(path string, edits map[int]string) (int, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("read line file: %w", err)
	}
	info, statErr := os.Stat(path)
	mode := os.FileMode(0o644)
	if statErr == nil {
		mode = info.Mode().Perm()
	}

	rawLines := strings.Split(string(body), "\n")
	changed := 0
	for i, raw := range rawLines {
		hasCR := strings.HasSuffix(raw, "\r")
		line := strings.TrimRight(raw, "\r")
		fields := strings.Split(line, "\t")
		if len(fields) < 7 {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(fields[0]))
		if err != nil {
			continue
		}
		newContent, ok := edits[n]
		if !ok || fields[6] == newContent {
			continue
		}
		fields[6] = newContent
		line = strings.Join(fields, "\t")
		if hasCR {
			line += "\r"
		}
		rawLines[i] = line
		changed++
	}
	if changed == 0 {
		return 0, nil
	}
	if err := os.WriteFile(path, []byte(strings.Join(rawLines, "\n")), mode); err != nil {
		return 0, fmt.Errorf("write line file: %w", err)
	}
	return changed, nil
}

// orderedLineEdits converts a line→content map into a slice sorted by line number.
func orderedLineEdits(m map[int]string) []LineEdit {
	nos := make([]int, 0, len(m))
	for n := range m {
		nos = append(nos, n)
	}
	sort.Ints(nos)
	out := make([]LineEdit, 0, len(nos))
	for _, n := range nos {
		out = append(out, LineEdit{LineNo: n, Content: m[n]})
	}
	return out
}

// GetFindingLines returns the current content of the line(s) the finding points
// at, so the Edit Tool dialog can show the offending text.
func (c *DocReviewController) GetFindingLines(ctx context.Context, findingID int64) ([]LineEdit, error) {
	f, err := c.loadFindingContext(ctx, findingID)
	if err != nil {
		return nil, err
	}
	lineNos := parseLocationRange(f.Location)
	if len(lineNos) == 0 {
		return []LineEdit{}, nil
	}
	path, err := resolveLineFilePath(ctx, c.DB, f.InputRecordID)
	if err != nil {
		return nil, err
	}
	current, err := readLineContents(path, lineNos)
	if err != nil {
		return nil, err
	}
	// Preserve the location order; include known lines only.
	out := make([]LineEdit, 0, len(lineNos))
	seen := map[int]bool{}
	for _, n := range lineNos {
		if seen[n] {
			continue
		}
		seen[n] = true
		if content, ok := current[n]; ok {
			out = append(out, LineEdit{LineNo: n, Content: content})
		}
	}
	return out, nil
}

// ApplyFindingEdit writes user-edited line content back to the line-file (the
// Edit Tool "Save" action) and marks the finding 'fixed' if anything changed.
func (c *DocReviewController) ApplyFindingEdit(ctx context.Context, findingID int64, edits []LineEdit) (int, error) {
	f, err := c.loadFindingContext(ctx, findingID)
	if err != nil {
		return 0, err
	}
	if len(edits) == 0 {
		return 0, nil
	}
	path, err := resolveLineFilePath(ctx, c.DB, f.InputRecordID)
	if err != nil {
		return 0, err
	}
	editMap := make(map[int]string, len(edits))
	for _, e := range edits {
		editMap[e.LineNo] = e.Content
	}
	changed, err := applyLineEdits(path, editMap)
	if err != nil {
		return 0, err
	}
	if changed > 0 {
		if _, err := c.DB.ExecContext(ctx,
			`UPDATE kb.doc_review_findings SET review_status = 'fixed' WHERE id = $1`, findingID,
		); err != nil {
			logger.Warn("apply edit: mark finding fixed", "finding_id", findingID, "error", err)
		}
		logger.Info("finding edited via Edit Tool", "finding_id", findingID, "lines_changed", changed)
	}
	return changed, nil
}

// autoFixPrompt instructs the model to rewrite the offending line(s) so the
// reported issue is resolved, returning a strict JSON object.
const autoFixPrompt = `You are a meticulous document editor. You are given one or more numbered source lines from a document and a quality issue found in them. Rewrite ONLY the offending line(s) so the issue is resolved, while preserving the original meaning, language, terminology, and formatting conventions. Do not merge or split lines: return exactly one corrected string per input line number you change. If the issue cannot be safely fixed by editing these line(s) alone, set "fixable" to false and explain why in "reason".

Respond with a single JSON object of this exact shape:
{
  "fixable": true,
  "reason": "",
  "fixes": [ { "line_no": <int>, "corrected": "<full corrected line content>" } ]
}
Only include a line in "fixes" when its content actually changes. Output JSON only.`

// AutoFixFinding runs the configured LLM to correct the offending line(s) and
// writes the result back to the line-file. A non-error result with Fixable=false
// means the GUI should prompt the user (e.g. unfixable or no model configured).
func (c *DocReviewController) AutoFixFinding(ctx context.Context, findingID int64) (*AutoFixResult, error) {
	f, err := c.loadFindingContext(ctx, findingID)
	if err != nil {
		return nil, err
	}
	lineNos := parseLocationRange(f.Location)
	if len(lineNos) == 0 {
		return &AutoFixResult{Fixable: false, Message: "This finding has no specific line location to auto-fix."}, nil
	}
	path, err := resolveLineFilePath(ctx, c.DB, f.InputRecordID)
	if err != nil {
		return nil, err
	}
	current, err := readLineContents(path, lineNos)
	if err != nil {
		return nil, err
	}
	if len(current) == 0 {
		return &AutoFixResult{Fixable: false, Message: "Could not locate the offending line(s) in the document."}, nil
	}

	// Resolve the auto-fix model: AUTO_FIX_MODEL_NAME primary, AUTO_FIX_CALLBACK fallback.
	primaryRef := strings.TrimSpace(os.Getenv("AUTO_FIX_MODEL_NAME"))
	fallbackRef := strings.TrimSpace(os.Getenv("AUTO_FIX_CALLBACK"))
	if primaryRef == "" {
		primaryRef = fallbackRef
	}
	if primaryRef == "" {
		return &AutoFixResult{Fixable: false, Message: "Auto-fix model is not configured (set AUTO_FIX_MODEL_NAME)."}, nil
	}
	client, modelName, err := docprocessing.BuildReviewerLLMClient(primaryRef)
	if err != nil && fallbackRef != "" && fallbackRef != primaryRef {
		logger.Warn("auto-fix primary model unavailable; trying fallback", "model_ref", primaryRef, "error", err)
		client, modelName, err = docprocessing.BuildReviewerLLMClient(fallbackRef)
	}
	if err != nil {
		return nil, fmt.Errorf("auto-fix model unavailable: %w", err)
	}

	// Build the LLM input: the issue plus the offending lines in numeric order.
	inputLines := orderedLineEdits(current)
	inputObj := map[string]any{
		"issue": map[string]any{
			"title":       f.Title,
			"description": f.Description,
			"suggestion":  f.Suggestion,
			"aspect":      f.Aspect,
			"severity":    f.Severity,
			"evidence":    f.Evidence,
		},
		"lines": inputLines,
	}
	inputJSON, err := json.Marshal(inputObj)
	if err != nil {
		return nil, fmt.Errorf("marshal auto-fix input: %w", err)
	}

	llmCtx := docprocessing.WithLLMRecordID(ctx, f.InputRecordID)
	in := docprocessing.NewLLMJSONInput(llmCtx, "auto_fix", autoFixPrompt, modelName,
		string(inputJSON), "doc_review_auto_fix", "MID-CWB-AUTOFIX")
	resp, err := client.ExtractJSON(llmCtx, in)
	if err != nil {
		return nil, fmt.Errorf("auto-fix LLM call failed: %w", err)
	}

	fixable, _ := resp["fixable"].(bool)
	reason := strings.TrimSpace(docprocessing.AsString(resp["reason"]))
	if !fixable {
		if reason == "" {
			reason = "The model determined this issue cannot be auto-fixed."
		}
		return &AutoFixResult{Fixable: false, Message: reason}, nil
	}

	edits := map[int]string{}
	if rawFixes, ok := resp["fixes"].([]any); ok {
		for _, rf := range rawFixes {
			m, ok := rf.(map[string]any)
			if !ok {
				continue
			}
			lineNo := int(toFloat(m["line_no"]))
			corrected := docprocessing.AsString(m["corrected"])
			cur, known := current[lineNo]
			if !known || corrected == "" || corrected == cur {
				continue
			}
			edits[lineNo] = corrected
		}
	}
	if len(edits) == 0 {
		msg := "The model did not propose any change to the offending line(s)."
		if reason != "" {
			msg = reason
		}
		return &AutoFixResult{Fixable: false, Message: msg}, nil
	}

	changed, err := applyLineEdits(path, edits)
	if err != nil {
		return nil, err
	}
	if changed == 0 {
		return &AutoFixResult{Fixable: false, Message: "No changes were applied to the document."}, nil
	}

	if _, err := c.DB.ExecContext(ctx,
		`UPDATE kb.doc_review_findings SET review_status = 'fixed' WHERE id = $1`, findingID,
	); err != nil {
		logger.Warn("auto-fix: mark finding fixed", "finding_id", findingID, "error", err)
	}
	logger.Info("finding auto-fixed", "finding_id", findingID, "lines_changed", changed, "model", modelName)

	original := make([]LineEdit, 0, len(edits))
	corrected := make([]LineEdit, 0, len(edits))
	nos := make([]int, 0, len(edits))
	for n := range edits {
		nos = append(nos, n)
	}
	sort.Ints(nos)
	for _, n := range nos {
		original = append(original, LineEdit{LineNo: n, Content: current[n]})
		corrected = append(corrected, LineEdit{LineNo: n, Content: edits[n]})
	}
	return &AutoFixResult{Fixable: true, Original: original, Corrected: corrected}, nil
}

// toFloat coerces a JSON number (float64) or numeric string to float64.
func toFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(n), 64)
		return f
	}
	return 0
}

// RegenerateReport rebuilds the report JSON/markdown and the Typst report PDF for
// an existing report id from the current (non-deleted) findings, updating the
// same report row in place so the report id / URL stays stable.
func (c *DocReviewController) RegenerateReport(ctx context.Context, reportID int64) error {
	var requestID, inputRecordID int64
	var reviewRunID string
	err := c.DB.QueryRowContext(ctx,
		`SELECT request_id, input_record_id, COALESCE(review_run_id,'')
		 FROM kb.doc_review_reports WHERE id = $1`, reportID,
	).Scan(&requestID, &inputRecordID, &reviewRunID)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("report %d not found", reportID)
		}
		return fmt.Errorf("load report %d: %w", reportID, err)
	}

	req, err := c.loadRequest(ctx, requestID)
	if err != nil {
		return err
	}

	findings, err := c.loadActiveFindings(ctx, inputRecordID, reviewRunID)
	if err != nil {
		return err
	}

	gen := NewDocReviewReportGenerator()
	report, err := gen.Build(ctx, req, findings)
	if err != nil {
		return fmt.Errorf("rebuild report: %w", err)
	}

	reportJSON, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}
	markdown := renderMarkdown(report)

	_, err = c.DB.ExecContext(ctx, `
		UPDATE kb.doc_review_reports
		SET report_json = $1, report_markdown = $2, executive_summary = $3,
		    total_findings = $4, high_count = $5, medium_count = $6, low_count = $7,
		    overall_assessment = $8
		WHERE id = $9`,
		reportJSON, markdown, report.ExecutiveSummary.Text, report.Meta.TotalFindings,
		countBySeverity(report.Findings, "high"),
		countBySeverity(report.Findings, "medium"),
		countBySeverity(report.Findings, "low"),
		report.ExecutiveSummary.OverallAssessment, reportID,
	)
	if err != nil {
		return fmt.Errorf("update report %d: %w", reportID, err)
	}

	if typErr := GenerateTypstReport(ctx, requestID, report, req); typErr != nil {
		logger.Warn("regenerate: typst PDF generation failed", "report_id", reportID, "error", typErr)
	}
	logger.Info("report regenerated", "report_id", reportID, "findings", len(findings))
	return nil
}

// loadActiveFindings loads the findings for a run, excluding any soft-deleted
// ('deleted') by the reviewer, ordered by id.
func (c *DocReviewController) loadActiveFindings(ctx context.Context, recordID int64, reviewRunID string) ([]FindingItem, error) {
	rows, err := c.DB.QueryContext(ctx, `
		SELECT id, pass, aspect, severity, finding_type, title, description,
		       COALESCE(evidence,''), COALESCE(location,''), COALESCE(suggestion,''),
		       COALESCE(confidence,0), COALESCE(review_status,'pending')
		FROM kb.doc_review_findings
		WHERE input_record_id = $1 AND review_run_id = $2 AND COALESCE(review_status,'') <> 'deleted'
		ORDER BY id`, recordID, reviewRunID)
	if err != nil {
		return nil, fmt.Errorf("load active findings: %w", err)
	}
	defer rows.Close()
	var out []FindingItem
	for rows.Next() {
		var f FindingItem
		if err := rows.Scan(&f.ID, &f.Pass, &f.Aspect, &f.Severity, &f.FindingType,
			&f.Title, &f.Description, &f.Evidence, &f.Location, &f.Suggestion,
			&f.Confidence, &f.ReviewStatus); err != nil {
			return nil, fmt.Errorf("scan finding: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
