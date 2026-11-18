// Package attention stores data problems that need a human decision in
// kb.attention_items, one row per problem. item_type classifies the problem;
// (item_type, dedupe_key) identifies it across all statuses. A repeat of an
// open problem updates its row; a repeat of a resolved or dismissed problem
// reopens the row, because a closed problem should not come back.
package attention

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"
)

// Item types. Add new types here; the table needs no migration.
const (
	// TypeAmbiguousObjectName: an artifact object tied between two or more
	// kb.object_nodes candidates and was linked by the deterministic tie-break.
	TypeAmbiguousObjectName = "ambiguous-object-name"

	// TypeUnresolvedMetricValue: extract_metrics produced a metric row whose value
	// semantics it could not pin down (several values in one cell, a min/max
	// expression, a formula reference). The row is kept as written; this item lets a
	// person review the kind of value, not a defect to fix.
	TypeUnresolvedMetricValue = "unresolved-metric-value"

	// TypeUnresolvedReference: the reference resolver could not link a citation to a
	// target artifact with confidence (no locator, a clause that delegates to a table,
	// no matching content). A limit of the resolver, kept for a person to review.
	TypeUnresolvedReference = "unresolved-reference"

	// TypeMissingReferenceDocument: no accessible copy satisfies the citation.
	TypeMissingReferenceDocument = "missing-reference-document"

	// TypeReferenceMetricNoMatch: the cited copy was searched and no metric candidate was found.
	TypeReferenceMetricNoMatch = "reference-metric-no-match"
)

var knownTypes = map[string]bool{
	TypeAmbiguousObjectName:      true,
	TypeUnresolvedMetricValue:    true,
	TypeUnresolvedReference:      true,
	TypeMissingReferenceDocument: true,
	TypeReferenceMetricNoMatch:   true,
}

// KnownType reports whether t is a declared item type.
func KnownType(t string) bool { return knownTypes[t] }

// Statuses.
const (
	StatusOpen      = "open"
	StatusResolved  = "resolved"
	StatusDismissed = "dismissed"
)

// MaxOccurrences caps details.occurrences; occurrence_count keeps the true total.
const MaxOccurrences = 50

var (
	ErrUnknownType = errors.New("unknown attention item type")
	ErrNotFound    = errors.New("attention item not found")
	ErrNotOpen     = errors.New("attention item is not open")
)

// Item is one kb.attention_items row.
type Item struct {
	ID              int64           `json:"id"`
	ItemType        string          `json:"item_type"`
	Status          string          `json:"status"`
	DedupeKey       string          `json:"dedupe_key"`
	Title           string          `json:"title"`
	Details         json.RawMessage `json:"details"`
	InputRecordID   int64           `json:"input_record_id"`
	OccurrenceCount int             `json:"occurrence_count"`
	ReopenCount     int             `json:"reopen_count"`
	FirstSeen       time.Time       `json:"first_seen"`
	LastSeen        time.Time       `json:"last_seen"`
	ResolvedBy      string          `json:"resolved_by"`
	ResolvedAt      *time.Time      `json:"resolved_at"`
	Resolution      string          `json:"resolution"`
}

// Report is one sighting of a problem.
type Report struct {
	ItemType      string
	DedupeKey     string
	Title         string
	InputRecordID int64
	// Details replaces the item's top-level details keys of the same name.
	// "occurrences" and "resolution_history" are managed by the store.
	Details map[string]any
	// Occurrence, if set, is prepended to details.occurrences with a seen_at
	// time. An earlier occurrence with the same fields (ignoring seen_at) is
	// replaced, so the list holds distinct occurrences, newest first.
	Occurrence map[string]any
}

// RecordResult says what Record did.
type RecordResult struct {
	ID       int64
	Created  bool
	Reopened bool
}

// Recorder records problems. ObjectReconciler and other producers depend on
// this interface, not on the SQL store.
type Recorder interface {
	Record(ctx context.Context, r Report) (RecordResult, error)
}

// ListFilter selects items for List. Empty fields do not filter.
type ListFilter struct {
	ItemType string
	Status   string
	Limit    int
	Offset   int
}

// SQLStore implements Recorder and the read/close operations on
// kb.attention_items.
type SQLStore struct {
	DB  *sql.DB
	Now func() time.Time // for tests; defaults to time.Now
}

func (s SQLStore) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Record inserts the problem as an open item, or updates its existing row:
// an open row gains the occurrence, a resolved or dismissed row is reopened
// with its closing kept in details.resolution_history.
func (s SQLStore) Record(ctx context.Context, r Report) (RecordResult, error) {
	if s.DB == nil {
		return RecordResult{}, fmt.Errorf("attention: db is nil")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return RecordResult{}, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := s.RecordTx(ctx, tx, r)
	if err != nil {
		return RecordResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return RecordResult{}, err
	}
	return result, nil
}

// RecordTx records a problem in the caller's transaction so its evidence and
// the producer's state can commit together.
func (s SQLStore) RecordTx(ctx context.Context, tx *sql.Tx, r Report) (RecordResult, error) {
	if !KnownType(r.ItemType) {
		return RecordResult{}, fmt.Errorf("%w: %q", ErrUnknownType, r.ItemType)
	}
	if strings.TrimSpace(r.DedupeKey) == "" {
		return RecordResult{}, fmt.Errorf("attention: empty dedupe key")
	}
	now := s.now().UTC()

	initial, err := json.Marshal(mergeDetails(map[string]any{}, r, now, nil))
	if err != nil {
		return RecordResult{}, err
	}
	var id int64
	// A concurrent insert of the same problem blocks here until the other
	// transaction commits; DO NOTHING then falls through to the update path.
	err = tx.QueryRowContext(ctx, `
INSERT INTO kb.attention_items (item_type, dedupe_key, title, details, input_record_id, first_seen, last_seen)
VALUES ($1, $2, $3, $4::jsonb, $5, $6, $6)
ON CONFLICT (item_type, dedupe_key) DO NOTHING
RETURNING id`, r.ItemType, r.DedupeKey, r.Title, string(initial), nullableID(r.InputRecordID), now).Scan(&id)
	if err == nil {
		return RecordResult{ID: id, Created: true}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return RecordResult{}, err
	}

	var (
		status     string
		detailsRaw []byte
		resolvedBy sql.NullString
		resolvedAt sql.NullTime
		resolution sql.NullString
	)
	if err := tx.QueryRowContext(ctx, `
SELECT id, status, details, resolved_by, resolved_at, resolution
FROM kb.attention_items
WHERE item_type = $1 AND dedupe_key = $2
FOR UPDATE`, r.ItemType, r.DedupeKey).Scan(&id, &status, &detailsRaw, &resolvedBy, &resolvedAt, &resolution); err != nil {
		return RecordResult{}, err
	}
	existing := map[string]any{}
	if len(detailsRaw) > 0 {
		if err := json.Unmarshal(detailsRaw, &existing); err != nil {
			return RecordResult{}, fmt.Errorf("attention: decode details of item %d: %w", id, err)
		}
	}
	reopened := status != StatusOpen
	var closing map[string]any
	if reopened {
		closing = map[string]any{"status": status, "reopened_at": now.Format(time.RFC3339)}
		if resolvedBy.Valid {
			closing["resolved_by"] = resolvedBy.String
		}
		if resolvedAt.Valid {
			closing["resolved_at"] = resolvedAt.Time.UTC().Format(time.RFC3339)
		}
		if resolution.Valid {
			closing["resolution"] = resolution.String
		}
	}
	merged, err := json.Marshal(mergeDetails(existing, r, now, closing))
	if err != nil {
		return RecordResult{}, err
	}
	reopenInc := 0
	if reopened {
		reopenInc = 1
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE kb.attention_items
SET status = 'open',
    title = CASE WHEN $2 <> '' THEN $2 ELSE title END,
    details = $3::jsonb,
    input_record_id = COALESCE($4, input_record_id),
    occurrence_count = occurrence_count + 1,
    reopen_count = reopen_count + $5,
    last_seen = $6,
    resolved_by = NULL,
    resolved_at = NULL,
    resolution = NULL,
    modify_time = $6
WHERE id = $1`, id, r.Title, string(merged), nullableID(r.InputRecordID), reopenInc, now); err != nil {
		return RecordResult{}, err
	}
	return RecordResult{ID: id, Reopened: reopened}, nil
}

// mergeDetails applies one report to existing details. closing, if not nil,
// is appended to resolution_history.
func mergeDetails(existing map[string]any, r Report, now time.Time, closing map[string]any) map[string]any {
	out := make(map[string]any, len(existing)+len(r.Details)+2)
	for k, v := range existing {
		out[k] = v
	}
	for k, v := range r.Details {
		if k == "occurrences" || k == "resolution_history" {
			continue
		}
		out[k] = v
	}
	if r.Occurrence != nil {
		occ := make(map[string]any, len(r.Occurrence)+1)
		for k, v := range r.Occurrence {
			occ[k] = v
		}
		occ["seen_at"] = now.Format(time.RFC3339)
		list := []any{occ}
		for _, prev := range asList(existing["occurrences"]) {
			if sameOccurrence(prev, occ) {
				continue
			}
			list = append(list, prev)
			if len(list) == MaxOccurrences {
				break
			}
		}
		out["occurrences"] = list
	}
	if closing != nil {
		out["resolution_history"] = append(asList(existing["resolution_history"]), closing)
	}
	return out
}

func asList(v any) []any {
	list, _ := v.([]any)
	return list
}

// sameOccurrence compares two occurrences ignoring seen_at. Values are
// compared after a JSON round trip so numbers decoded from the database
// (float64) equal the Go values in a new report.
func sameOccurrence(prev any, occ map[string]any) bool {
	p, ok := prev.(map[string]any)
	if !ok {
		return false
	}
	return reflect.DeepEqual(withoutSeenAt(p), withoutSeenAt(occ))
}

func withoutSeenAt(m map[string]any) map[string]any {
	c := make(map[string]any, len(m))
	for k, v := range m {
		if k != "seen_at" {
			c[k] = v
		}
	}
	raw, _ := json.Marshal(c)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return out
}

func nullableID(id int64) any {
	if id <= 0 {
		return nil
	}
	return id
}

const itemColumns = `id, item_type, status, dedupe_key, title, details, COALESCE(input_record_id, 0),
       occurrence_count, reopen_count, first_seen, last_seen,
       COALESCE(resolved_by, ''), resolved_at, COALESCE(resolution, '')`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanItem(row rowScanner) (Item, error) {
	var (
		it         Item
		details    []byte
		resolvedAt sql.NullTime
	)
	if err := row.Scan(&it.ID, &it.ItemType, &it.Status, &it.DedupeKey, &it.Title, &details, &it.InputRecordID,
		&it.OccurrenceCount, &it.ReopenCount, &it.FirstSeen, &it.LastSeen,
		&it.ResolvedBy, &resolvedAt, &it.Resolution); err != nil {
		return Item{}, err
	}
	it.Details = json.RawMessage(details)
	if resolvedAt.Valid {
		t := resolvedAt.Time
		it.ResolvedAt = &t
	}
	return it, nil
}

// Get loads one item. It returns ErrNotFound if no row has that id.
func (s SQLStore) Get(ctx context.Context, id int64) (Item, error) {
	if s.DB == nil {
		return Item{}, fmt.Errorf("attention: db is nil")
	}
	it, err := scanItem(s.DB.QueryRowContext(ctx, `SELECT `+itemColumns+` FROM kb.attention_items WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Item{}, ErrNotFound
	}
	return it, err
}

// List returns items matching f, most recently seen first, and the number of
// matching items before paging.
func (s SQLStore) List(ctx context.Context, f ListFilter) ([]Item, int, error) {
	if s.DB == nil {
		return nil, 0, fmt.Errorf("attention: db is nil")
	}
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}
	const where = `WHERE ($1 = '' OR item_type = $1) AND ($2 = '' OR status = $2)`
	var total int
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM kb.attention_items `+where, f.ItemType, f.Status).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+itemColumns+` FROM kb.attention_items `+where+`
ORDER BY last_seen DESC, id DESC
LIMIT $3 OFFSET $4`, f.ItemType, f.Status, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	out := []Item{}
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, it)
	}
	return out, total, rows.Err()
}

// Close sets an open item to resolved or dismissed. It returns ErrNotFound for
// an unknown id and ErrNotOpen if the item is already closed.
func (s SQLStore) Close(ctx context.Context, id int64, status, actor, resolution string) (Item, error) {
	if s.DB == nil {
		return Item{}, fmt.Errorf("attention: db is nil")
	}
	if status != StatusResolved && status != StatusDismissed {
		return Item{}, fmt.Errorf("attention: cannot close with status %q", status)
	}
	now := s.now().UTC()
	it, err := scanItem(s.DB.QueryRowContext(ctx, `
UPDATE kb.attention_items
SET status = $2, resolved_by = NULLIF($3, ''), resolved_at = $4, resolution = NULLIF($5, ''), modify_time = $4
WHERE id = $1 AND status = 'open'
RETURNING `+itemColumns, id, status, actor, now, strings.TrimSpace(resolution)))
	if err == nil {
		return it, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Item{}, err
	}
	if _, getErr := s.Get(ctx, id); getErr != nil {
		return Item{}, getErr
	}
	return Item{}, ErrNotOpen
}
