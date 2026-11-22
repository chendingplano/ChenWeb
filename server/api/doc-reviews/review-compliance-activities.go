package docreviews

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"unicode"
)

// The processors whose results Compliance Review reads. For now only the simple metric extraction
// counts as metric extraction.
const (
	complianceMetricsProcessor    = "extract_metrics_simple"
	complianceProvisionsProcessor = "extract_provisions"
)

// complianceStore reads what the reviewer needs from the database (a field so tests need none).
type complianceStore interface {
	// Prerequisites returns the processors among the two required ones that have no successful run
	// on the record.
	MissingProcessors(ctx context.Context, recordID int64) ([]string, error)
	// ArtifactRows returns every metric and provision of the record.
	ArtifactRows(ctx context.Context, recordID int64) ([]complianceArtifactRow, error)
	// Mentions returns the record's citations of other documents.
	Mentions(ctx context.Context, recordID int64) ([]complianceMention, error)
}

// complianceArtifactRow is one metric or provision of the reviewed record.
type complianceArtifactRow struct {
	Kind       string // "metric" or "provision"
	RowID      int64
	ArtifactID string // its kb.search_artifacts id; empty when it is not indexed
	Label      string
	Text       string // the search-index text, or the row's own text when not indexed
	LineStart  int    // 0 when the row has no lines
	LineEnd    int
	DupKey     string // metric name|value|unit|condition, or the provision text
}

// complianceMention is one citation of another document found by reference extraction.
type complianceMention struct {
	Name           string
	LineStart      int
	LineEnd        int
	TargetRecordID int64 // the cited record when reference extraction matched it, else 0
}

type complianceSQLStore struct{ db *sql.DB }

func (s complianceSQLStore) MissingProcessors(ctx context.Context, recordID int64) ([]string, error) {
	if s.db == nil {
		return nil, fmt.Errorf("compliance: database is not available")
	}
	var missing []string
	for _, proc := range []string{complianceMetricsProcessor, complianceProvisionsProcessor} {
		var ok bool
		err := s.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM kb.doc_process_runs
			WHERE record_id = $1 AND status = 'success' AND processors @> jsonb_build_array($2::text))`, recordID, proc).Scan(&ok)
		if err != nil {
			return nil, fmt.Errorf("compliance: check %s runs for record %d: %w", proc, recordID, err)
		}
		if !ok {
			missing = append(missing, proc)
		}
	}
	return missing, nil
}

func (s complianceSQLStore) ArtifactRows(ctx context.Context, recordID int64) ([]complianceArtifactRow, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT 'metric', m.id, COALESCE(sa.artifact_id, ''), COALESCE(NULLIF(sa.primary_label, ''), m.metric_name, ''),
			COALESCE(NULLIF(sa.search_document, ''), m.search_document, m.metric_name, ''),
			COALESCE(lower(m.line_range), 0), COALESCE(upper(m.line_range) - 1, 0),
			concat_ws('|', m.metric_name, m.metric_value, m.metric_unit, m.condition)
		FROM kb.metrics m
		LEFT JOIN kb.search_artifacts sa ON sa.artifact_type = 'metric' AND sa.source_row_id = m.id AND sa.input_record_id = m.input_record_id
		WHERE m.input_record_id = $1
		UNION ALL
		SELECT 'provision', p.id, COALESCE(sa.artifact_id, ''), COALESCE(NULLIF(sa.primary_label, ''), p.prov_name, ''),
			COALESCE(NULLIF(sa.search_document, ''), p.search_document, p.provision, ''),
			COALESCE(lower(p.line_range), 0), COALESCE(upper(p.line_range) - 1, 0),
			COALESCE(p.provision, '')
		FROM kb.provisions p
		LEFT JOIN kb.search_artifacts sa ON sa.artifact_type = 'provision' AND sa.source_row_id = p.id AND sa.input_record_id = p.input_record_id
		WHERE p.input_record_id = $1`, recordID)
	if err != nil {
		return nil, fmt.Errorf("compliance: load metrics and provisions of record %d: %w", recordID, err)
	}
	defer rows.Close()
	var out []complianceArtifactRow
	for rows.Next() {
		var r complianceArtifactRow
		if err := rows.Scan(&r.Kind, &r.RowID, &r.ArtifactID, &r.Label, &r.Text, &r.LineStart, &r.LineEnd, &r.DupKey); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s complianceSQLStore) Mentions(ctx context.Context, recordID int64) ([]complianceMention, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT COALESCE(NULLIF(cited_raw, ''), target_title, ''), COALESCE(source_line_spans::text, '[]'),
			COALESCE(target_input_id, 0)
		FROM kb.ref_mentions WHERE input_record_id = $1 AND NOT COALESCE(internal, false)`, recordID)
	if err != nil {
		return nil, fmt.Errorf("compliance: load citations of record %d: %w", recordID, err)
	}
	defer rows.Close()
	var out []complianceMention
	for rows.Next() {
		var name, spans string
		var target int64
		if err := rows.Scan(&name, &spans, &target); err != nil {
			return nil, err
		}
		start, end := complianceSpanRange(json.RawMessage(spans))
		if strings.TrimSpace(name) == "" || start == 0 {
			continue
		}
		if target == recordID {
			target = 0
		}
		out = append(out, complianceMention{Name: strings.TrimSpace(name), LineStart: start, LineEnd: end, TargetRecordID: target})
	}
	return out, rows.Err()
}

// complianceSpanRange returns the first and last line of a JSON list of line spans ("14", "14-16").
func complianceSpanRange(raw json.RawMessage) (start, end int) {
	var spans []string
	if json.Unmarshal(raw, &spans) != nil {
		return 0, 0
	}
	for _, s := range spans {
		a, b := parseArtifactSpan(s)
		if a == 0 {
			continue
		}
		if start == 0 || a < start {
			start = a
		}
		if b > end {
			end = b
		}
	}
	return start, end
}

// complianceLocation is one place where an activity, or an exact copy of it, appears.
type complianceLocation struct {
	ArtifactID string `json:"artifact_id,omitempty"`
	RowID      int64  `json:"row_id"`
	LineStart  int    `json:"line_start"`
	LineEnd    int    `json:"line_end"`
}

// complianceCitedDoc is a document an activity cites. RecordID is the matched record, 0 when unknown.
type complianceCitedDoc struct {
	Name     string `json:"name"`
	RecordID int64  `json:"record_id,omitempty"`
}

// complianceMember is one activity: a metric, a provision, or a text activity.
type complianceMember struct {
	ID         string               `json:"activity_id"`
	Kind       string               `json:"kind"`
	Label      string               `json:"label"`
	Text       string               `json:"text"`
	LineStart  int                  `json:"line_start"`
	LineEnd    int                  `json:"line_end"`
	ArtifactID string               `json:"artifact_id,omitempty"`
	RowID      int64                `json:"row_id,omitempty"`
	Copies     []complianceLocation `json:"copies,omitempty"`
	Activity   *complianceActivity  `json:"text_activity,omitempty"`
	CitedDocs  []complianceCitedDoc `json:"cited_documents,omitempty"`
}

// complianceUnit is what one judgement call and one finding cover: a text activity, or a group of
// metric and provision activities whose lines overlap.
type complianceUnit struct {
	Members   []*complianceMember
	LineStart int
	LineEnd   int
}

type compliancePlan struct {
	Members             []complianceMember // metric and provision activities, after merging and limits, in line order
	UncheckedMetrics    int
	UncheckedProvisions int
	WithoutLines        int
}

type compliancePlanEntry struct {
	once sync.Once
	plan *compliancePlan
	err  error
}

// plan loads the record's metric and provision activities once per review, however many chunk
// tasks ask for them concurrently. A missing prerequisite is an error.
func (r *complianceReviewer) plan(ctx context.Context, recordID int64) (*compliancePlan, error) {
	v, _ := r.plans.LoadOrStore(recordID, &compliancePlanEntry{})
	e := v.(*compliancePlanEntry)
	e.once.Do(func() {
		if r.store == nil {
			e.err = fmt.Errorf("compliance: no store configured")
			return
		}
		missing, err := r.store.MissingProcessors(ctx, recordID)
		if err != nil {
			e.err = err
			return
		}
		if len(missing) > 0 {
			e.err = fmt.Errorf("compliance: record %d cannot be reviewed: run %s on it first (Compliance Review needs its metrics and provisions)", recordID, strings.Join(missing, " and "))
			return
		}
		rows, err := r.store.ArtifactRows(ctx, recordID)
		if err != nil {
			e.err = err
			return
		}
		mentions, err := r.recordMentions(ctx, recordID)
		if err != nil {
			e.err = err
			return
		}
		e.plan = buildCompliancePlan(rows, mentions, r.settings)
	})
	return e.plan, e.err
}

type complianceMentionsEntry struct {
	once     sync.Once
	mentions []complianceMention
	err      error
}

// recordMentions loads a record's citations once per review.
func (r *complianceReviewer) recordMentions(ctx context.Context, recordID int64) ([]complianceMention, error) {
	if r.store == nil {
		return nil, nil
	}
	v, _ := r.mentions.LoadOrStore(recordID, &complianceMentionsEntry{})
	e := v.(*complianceMentionsEntry)
	e.once.Do(func() { e.mentions, e.err = r.store.Mentions(ctx, recordID) })
	return e.mentions, e.err
}

// buildCompliancePlan merges exact copies, then keeps the first max_metrics metrics and
// max_provisions provisions in line order. Rows without lines are counted, not reviewed.
func buildCompliancePlan(rows []complianceArtifactRow, mentions []complianceMention, s complianceSettings) *compliancePlan {
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].LineStart != rows[j].LineStart {
			return rows[i].LineStart < rows[j].LineStart
		}
		if rows[i].Kind != rows[j].Kind {
			return rows[i].Kind < rows[j].Kind
		}
		return rows[i].RowID < rows[j].RowID
	})
	p := &compliancePlan{}
	var merged []*complianceMember
	byKey := map[string]*complianceMember{}
	for _, row := range rows {
		if row.LineStart <= 0 {
			p.WithoutLines++
			continue
		}
		end := max(row.LineEnd, row.LineStart)
		loc := complianceLocation{ArtifactID: row.ArtifactID, RowID: row.RowID, LineStart: row.LineStart, LineEnd: end}
		key := ""
		if k := complianceDupKey(row.DupKey); k != "" {
			key = row.Kind + "\x00" + k
		}
		if m, ok := byKey[key]; ok && key != "" {
			m.Copies = append(m.Copies, loc)
			continue
		}
		label := strings.TrimSpace(row.Label)
		if label == "" {
			label = complianceCap(row.Text, 80)
		}
		m := &complianceMember{Kind: row.Kind, Label: label, Text: strings.TrimSpace(row.Text), LineStart: row.LineStart, LineEnd: end,
			ArtifactID: row.ArtifactID, RowID: row.RowID}
		if key != "" {
			byKey[key] = m
		}
		merged = append(merged, m)
	}
	metrics, provisions := 0, 0
	for _, m := range merged {
		switch m.Kind {
		case "metric":
			if metrics >= s.MaxMetrics {
				p.UncheckedMetrics++
				continue
			}
			metrics++
		case "provision":
			if provisions >= s.MaxProvisions {
				p.UncheckedProvisions++
				continue
			}
			provisions++
		}
		m.CitedDocs = complianceMentionsIn(mentions, m.LineStart, m.LineEnd)
		p.Members = append(p.Members, *m)
	}
	return p
}

// complianceDupKey normalizes a duplicate key: case, spacing and punctuation do not matter.
func complianceDupKey(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '|' || r == '.' || r == '%' {
			b.WriteRune(r)
		}
	}
	key := strings.Trim(b.String(), "|")
	return key
}

// complianceMentionsIn returns the distinct cited documents whose lines overlap start..end.
func complianceMentionsIn(mentions []complianceMention, start, end int) []complianceCitedDoc {
	var out []complianceCitedDoc
	seen := map[string]bool{}
	for _, m := range mentions {
		if m.LineStart > end || m.LineEnd < start {
			continue
		}
		key := strings.ToLower(m.Name)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, complianceCitedDoc{Name: m.Name, RecordID: m.TargetRecordID})
	}
	return out
}

// membersIn returns copies of the plan's activities whose first line falls in start..end.
func (p *compliancePlan) membersIn(start, end int) []*complianceMember {
	var out []*complianceMember
	for i := range p.Members {
		if p.Members[i].LineStart >= start && p.Members[i].LineStart <= end {
			m := p.Members[i]
			out = append(out, &m)
		}
	}
	return out
}

// buildComplianceUnits groups the chunk's metric and provision activities whose lines overlap and
// gives each text activity its own unit. Units are in line order; activity IDs are a1, a2, ...
// within each unit.
func buildComplianceUnits(members []*complianceMember, texts []complianceActivity) []*complianceUnit {
	parent := make([]int, len(members))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	for i := range members {
		for j := i + 1; j < len(members); j++ {
			if members[i].LineStart <= members[j].LineEnd && members[j].LineStart <= members[i].LineEnd {
				parent[find(j)] = find(i)
			}
		}
	}
	groups := map[int]*complianceUnit{}
	var units []*complianceUnit
	for i, m := range members {
		root := find(i)
		u, ok := groups[root]
		if !ok {
			u = &complianceUnit{LineStart: m.LineStart, LineEnd: m.LineEnd}
			groups[root] = u
			units = append(units, u)
		}
		u.Members = append(u.Members, m)
		u.LineStart = min(u.LineStart, m.LineStart)
		u.LineEnd = max(u.LineEnd, m.LineEnd)
	}
	for i := range texts {
		a := texts[i]
		label := a.Activity
		if a.DocumentLevel {
			label = "document: " + a.Object
		}
		var cited []complianceCitedDoc
		for _, name := range a.CitedDocs {
			cited = append(cited, complianceCitedDoc{Name: name})
		}
		m := &complianceMember{Kind: "text", Label: label, Text: a.SourceText, LineStart: a.LineStart, LineEnd: a.LineEnd, Activity: &a, CitedDocs: cited}
		units = append(units, &complianceUnit{Members: []*complianceMember{m}, LineStart: a.LineStart, LineEnd: a.LineEnd})
	}
	sort.SliceStable(units, func(i, j int) bool { return units[i].LineStart < units[j].LineStart })
	for _, u := range units {
		for k, m := range u.Members {
			m.ID = fmt.Sprintf("a%d", k+1)
		}
	}
	return units
}

// complianceCoveredLines returns the lines that the chunk's metric and provision activities cover.
func complianceCoveredLines(members []*complianceMember) map[int]bool {
	covered := map[int]bool{}
	for _, m := range members {
		for _, loc := range append([]complianceLocation{{LineStart: m.LineStart, LineEnd: m.LineEnd}}, m.Copies...) {
			for l := loc.LineStart; l <= loc.LineEnd; l++ {
				covered[l] = true
			}
		}
	}
	return covered
}

// complianceLinesCovered reports whether every line start..end is covered.
func complianceLinesCovered(covered map[int]bool, start, end int) bool {
	if len(covered) == 0 || start <= 0 || end < start {
		return false
	}
	for l := start; l <= end; l++ {
		if !covered[l] {
			return false
		}
	}
	return true
}

// unitDocumentLevel reports whether the unit is the document's own activity.
func (u *complianceUnit) documentLevel() bool {
	return len(u.Members) == 1 && u.Members[0].Activity != nil && u.Members[0].Activity.DocumentLevel
}

// label names the unit in titles: its first activity, and how many more a group has.
func (u *complianceUnit) label() string {
	if len(u.Members) == 0 {
		return ""
	}
	l := u.Members[0].Label
	if len(u.Members) > 1 {
		l = fmt.Sprintf("%s (+%d more)", l, len(u.Members)-1)
	}
	return l
}
