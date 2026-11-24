package docreviews

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/lib/pq"
)

// hydrateComplianceFindings gives report building the full text of normalized compliance
// findings (openspec compliance-review-storage): content from their details plus a judgement
// section rendered from their judged candidate rows and their activity list, in the form the
// description used before the storage change, without the judged-candidate list. Findings
// without a details row (legacy) are left as stored.
func hydrateComplianceFindings(ctx context.Context, db *sql.DB, findings []FindingItem) error {
	var ids []int64
	for _, f := range findings {
		if f.Aspect == "compliance" {
			ids = append(ids, f.ID)
		}
	}
	if db == nil || len(ids) == 0 {
		return nil
	}
	texts, err := complianceReportTexts(ctx, db, ids)
	if err != nil {
		return err
	}
	for i := range findings {
		if t, ok := texts[findings[i].ID]; ok {
			f := &findings[i]
			f.Description, f.Evidence, f.Suggestion = t.description, t.evidence, t.suggestion
			// A migrated finding may still hold its legacy payload until it is stripped; the
			// report must not copy it.
			f.Metadata, f.ArtifactFields, f.RelatedArtifactFields, f.RelatedArtifactID, f.RelatedRecordID = nil, nil, nil, "", 0
		}
	}
	return nil
}

type complianceReportText struct {
	description, evidence, suggestion string
}

type complianceReportVerdict struct {
	label, activityLabel, verdict, ctype, cid, summary, note string
	record                                                   int64
}

type complianceReportActivity struct {
	label, kind, name string
	start, end        int
}

// complianceReportTexts renders the report text of each normalized finding in ids.
func complianceReportTexts(ctx context.Context, db *sql.DB, ids []int64) (map[int64]complianceReportText, error) {
	out := map[int64]complianceReportText{}
	rows, err := db.QueryContext(ctx, `SELECT finding_id, description, evidence, suggestion
		FROM kb.doc_review_finding_details WHERE finding_id = ANY($1)`, pq.Array(ids))
	if err != nil {
		return nil, fmt.Errorf("load compliance finding details: %w", err)
	}
	for rows.Next() {
		var id int64
		var t complianceReportText
		if err := rows.Scan(&id, &t.description, &t.evidence, &t.suggestion); err != nil {
			rows.Close()
			return nil, err
		}
		out[id] = t
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return out, nil
	}

	activities := map[int64][]complianceReportActivity{}
	rows, err = db.QueryContext(ctx, `SELECT finding_id, local_label, kind, label, COALESCE(line_start, 0), COALESCE(line_end, 0)
		FROM kb.doc_review_compliance_activities WHERE finding_id = ANY($1) ORDER BY finding_id, display_order, id`, pq.Array(ids))
	if err != nil {
		return nil, fmt.Errorf("load compliance activities: %w", err)
	}
	for rows.Next() {
		var id int64
		var a complianceReportActivity
		if err := rows.Scan(&id, &a.label, &a.kind, &a.name, &a.start, &a.end); err != nil {
			rows.Close()
			return nil, err
		}
		activities[id] = append(activities[id], a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	verdicts := map[int64][]complianceReportVerdict{}
	rows, err = db.QueryContext(ctx, `SELECT a.finding_id, a.local_label, a.label, c.verdict, c.candidate_type, c.candidate_id,
			COALESCE(c.candidate_record_id, 0), c.summary, c.note
		FROM kb.doc_review_compliance_candidates c
		JOIN kb.doc_review_compliance_activities a ON a.activity_id = c.activity_id
		WHERE a.finding_id = ANY($1) AND c.status = 'judged'
		ORDER BY a.finding_id, c.display_order, c.id`, pq.Array(ids))
	if err != nil {
		return nil, fmt.Errorf("load compliance verdicts: %w", err)
	}
	for rows.Next() {
		var id int64
		var v complianceReportVerdict
		if err := rows.Scan(&id, &v.label, &v.activityLabel, &v.verdict, &v.ctype, &v.cid, &v.record, &v.summary, &v.note); err != nil {
			rows.Close()
			return nil, err
		}
		verdicts[id] = append(verdicts[id], v)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for id, t := range out {
		t.description = renderComplianceReportText(t.description, verdicts[id], activities[id])
		out[id] = t
	}
	return out, nil
}

// renderComplianceReportText is the report text of one finding: the verdict rows, the
// finding-level description, then one line per activity.
func renderComplianceReportText(description string, verdicts []complianceReportVerdict, activities []complianceReportActivity) string {
	var parts []string
	if v := renderComplianceVerdictRows(verdicts, len(activities) > 1); v != "" {
		parts = append(parts, v)
	}
	if d := strings.TrimSpace(description); d != "" {
		parts = append(parts, d)
	}
	if a := renderComplianceActivityLines(activities); a != "" {
		parts = append(parts, a)
	}
	return strings.Join(parts, "\n\n")
}

// renderComplianceVerdictRows is the verdict counts and one line per judged candidate, with
// its note; in a group each line names its activity.
func renderComplianceVerdictRows(verdicts []complianceReportVerdict, group bool) string {
	if len(verdicts) == 0 {
		return ""
	}
	var b strings.Builder
	counts := map[string]int{}
	for _, v := range verdicts {
		counts[v.verdict]++
	}
	var parts []string
	for _, verdict := range complianceVerdictOrder {
		if counts[verdict] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", counts[verdict], verdict))
		}
	}
	fmt.Fprintf(&b, "Candidate verdicts: %s\n", strings.Join(parts, ", "))
	for _, v := range verdicts {
		member := ""
		if group {
			member = fmt.Sprintf(" (activity %s: %s)", v.label, v.activityLabel)
		}
		fmt.Fprintf(&b, "%s [%s %s] record %d%s: %s\n", v.verdict, v.ctype, v.cid, v.record, member, v.summary)
		if v.note != "" {
			fmt.Fprintf(&b, "  Note: %s\n", v.note)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// renderComplianceActivityLines is one line per activity.
func renderComplianceActivityLines(activities []complianceReportActivity) string {
	lines := make([]string, 0, len(activities))
	for _, a := range activities {
		lines = append(lines, fmt.Sprintf("Activity %s [%s, lines %d-%d]: %s", a.label, a.kind, a.start, a.end, a.name))
	}
	return strings.Join(lines, "\n")
}
