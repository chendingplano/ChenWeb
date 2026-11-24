package productreviews

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
)

// DetailedReport combines saved retrieval evidence with explicitly current record details.
// No UI filters or export caps apply to the saved result set.
type DetailedReport struct {
	Run       Run
	Summary   Report
	Request   Request
	Profile   *Profile
	Nodes     []ProfileNode
	Results   []Result
	Scoped    []ScopedDoc
	Metrics   map[string]json.RawMessage
	Documents []reportDocument
	Diff      *RunDiff
}

type reportDocument struct {
	ID       int64
	Metadata map[string]any
	Lines    []docprocessing.Line
	Warning  string
}

func loadDetailedReport(ctx context.Context, db *sql.DB, run *Run) (*DetailedReport, error) {
	d := &DetailedReport{Run: *run, Metrics: map[string]json.RawMessage{}}
	if err := json.Unmarshal(run.ReportJSON, &d.Summary); err != nil {
		return nil, fmt.Errorf("decode run report: %w", err)
	}
	if d.Summary.ProfileID <= 0 || d.Summary.ProfileVersion <= 0 {
		return nil, fmt.Errorf("run %d has no saved profile identity", run.ID)
	}
	store := RunStore{DB: db}
	req, err := store.GetRequest(ctx, run.RequestID)
	if err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("review request %d is unavailable", run.RequestID)
	}
	d.Request = *req
	// The saved report pins the reviewed version even after a request is rerun.
	d.Request.ProfileID, d.Request.ProfileVersion = d.Summary.ProfileID, d.Summary.ProfileVersion
	d.Profile, err = (Store{DB: db}).GetProfile(ctx, d.Summary.ProfileID)
	if err != nil {
		return nil, err
	}
	d.Nodes, err = (Store{DB: db}).LoadNodes(ctx, d.Summary.ProfileID)
	if err != nil {
		return nil, err
	}
	d.Results, err = store.LoadResults(ctx, run.ID, ResultFilters{})
	if err != nil {
		return nil, err
	}
	d.Scoped, err = store.LoadScopedDocs(ctx, run.ID)
	if err != nil {
		return nil, err
	}
	if run.RunNumber > 1 {
		previous, err := store.PreviousRun(ctx, run.RequestID, run.RunNumber)
		if err != nil {
			return nil, err
		}
		if previous != nil {
			results, err := store.LoadResults(ctx, previous.ID, ResultFilters{})
			if err != nil {
				return nil, err
			}
			diff := DiffResults(results, d.Results, runReportVersion(previous), d.Summary.ProfileVersion, previous.RunNumber)
			d.Diff = &diff
		}
	}
	// One query for all metric fields, including structured values and table citations.
	rows, err := db.QueryContext(ctx, `SELECT r.artifact_id, to_jsonb(m)
		FROM kb.product_review_results r LEFT JOIN kb.metrics m
		ON r.artifact_type = 'metric' AND m.id = r.source_row_id AND m.ref_mention_id IS NULL
		WHERE r.run_id = $1 ORDER BY r.artifact_id`, run.ID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id string
		var raw []byte
		if err := rows.Scan(&id, &raw); err != nil {
			rows.Close()
			return nil, err
		}
		d.Metrics[id] = append(json.RawMessage(nil), raw...)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	ids := map[int64]bool{}
	for _, r := range d.Results {
		ids[r.InputRecordID] = true
	}
	for _, doc := range d.Scoped {
		ids[doc.InputRecordID] = true
	}
	ordered := make([]int64, 0, len(ids))
	for id := range ids {
		ordered = append(ordered, id)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	for _, id := range ordered {
		doc := reportDocument{ID: id}
		// Metadata failures are errors; a missing source file is visible in the PDF.
		rec, err := (docprocessing.DocMetadataSQLStore{DB: db}).GetInputRecord(ctx, id)
		if err == sql.ErrNoRows {
			doc.Warning = "source-unavailable"
			doc.Metadata = map[string]any{"input_record_id": id}
			d.Documents = append(d.Documents, doc)
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("source document %d: %w", id, err)
		}
		doc.Metadata = map[string]any{"title": rec.Title, "document_number": rec.DocumentNumber,
			"file_name": rec.FileName, "document_type": rec.InputDocType,
			"source_language": rec.SourceLanguage, "publish_date": rec.PublishDate}
		if rec.DocMetadataJSON != "" {
			doc.Metadata["document_metadata"] = json.RawMessage(rec.DocMetadataJSON)
		}
		doc.Lines, err = docprocessing.LoadRecordLinesByID(ctx, db, id)
		if err != nil {
			doc.Warning = "source-unavailable"
		}
		d.Documents = append(d.Documents, doc)
	}
	return d, nil
}

// reportSourceText includes every cited canonical line, once, in source order.
// Ranges and table-qualified citations retain the complete canonical table text.
func reportSourceText(lines []docprocessing.Line, spans json.RawMessage) (string, bool) {
	var refs []string
	if err := json.Unmarshal(spans, &refs); err != nil || len(refs) == 0 {
		return "", false
	}
	selected := map[int]bool{}
	complete := true
	for _, ref := range refs {
		// Page prefixes and row/cell suffixes qualify canonical line numbers.
		ref, _, _ = strings.Cut(ref, "#")
		if _, tail, ok := strings.Cut(ref, ":"); ok {
			ref = tail
		}
		parts := strings.SplitN(ref, "-", 2)
		start, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		end := start
		if len(parts) == 2 {
			end, err = strconv.Atoi(strings.TrimSpace(parts[1]))
		}
		if err != nil || start < 1 || end < start {
			complete = false
			continue
		}
		found := 0
		for _, line := range lines {
			if line.LineNo >= start && line.LineNo <= end {
				selected[line.LineNo] = true
				found++
			}
		}
		if found != end-start+1 {
			complete = false
		}
	}
	var b strings.Builder
	for _, line := range lines {
		if selected[line.LineNo] {
			fmt.Fprintf(&b, "[p%d · L%d] %s\n", line.PageNo, line.LineNo, line.Content)
		}
	}
	return b.String(), complete
}
