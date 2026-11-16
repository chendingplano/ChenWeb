package datasync

import (
	"encoding/json"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func testItem() TableSyncItem {
	return TableSyncItem{
		ID:         "kb_product_names",
		Table:      "kb.product_names",
		CursorCol:  "update_time",
		NaturalKey: []string{"source", "seq_no", "product_name"},
		Columns: []string{
			"seq_no", "sub_catalog", "category_l1", "category_l2", "description",
			"intended_use", "product_name", "product_name_en", "regulatory_class",
			"aliases", "keywords", "source", "notes", "extra_info", "status", "update_time",
			"code_group", "child_code_group", "industry_code", "cpc", "entry_no", "entry_no_new",
		},
		JSONColumns: []string{"aliases", "keywords", "extra_info"},
		Filter:      "status = 'approved'",
	}
}

const wantChangesQuery = `SELECT seq_no, sub_catalog, category_l1, category_l2, description, intended_use, product_name, product_name_en, regulatory_class, aliases, keywords, source, notes, extra_info, status, update_time, code_group, child_code_group, industry_code, cpc, entry_no, entry_no_new FROM kb.product_names WHERE (status = 'approved') AND update_time > $1 ORDER BY update_time, source, seq_no, product_name LIMIT $2`

// JSON columns use []byte, matching what the real Postgres driver (lib/pq)
// returns for jsonb text-format values -- unlike a plain Go string, []byte is
// what database/sql's generic converter accepts when scanning into
// *json.RawMessage.
const wantTieQuery = `SELECT seq_no, sub_catalog, category_l1, category_l2, description, intended_use, product_name, product_name_en, regulatory_class, aliases, keywords, source, notes, extra_info, status, update_time, code_group, child_code_group, industry_code, cpc, entry_no, entry_no_new FROM kb.product_names WHERE (status = 'approved') AND update_time = $1 ORDER BY update_time, source, seq_no, product_name`

func addProductNameRow(rows *sqlmock.Rows, updateTime string) *sqlmock.Rows {
	return addProductNameRowSeq(rows, updateTime, "1")
}

// addProductNameRowSeq differs from addProductNameRow only in seq_no, which is
// part of the natural key, so rows built with different seq values are
// different rows.
func addProductNameRowSeq(rows *sqlmock.Rows, updateTime, seqNo string) *sqlmock.Rows {
	return rows.AddRow(
		seqNo, "Cat A", "L1", "L2", "desc", "use", "Widget", "Widget EN", "Class II",
		[]byte("[]"), []byte("{}"), "cn_nmpa_medical_device_classification_catalog", "", []byte("{}"), "approved", updateTime,
		"", "", "", "", "", "",
	)
}

func TestFetchChangesPageEmptyCursor(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	item := testItem()
	rows := addProductNameRow(sqlmock.NewRows(item.Columns), "2026-09-15T00:00:00Z")
	// An empty `since` binds as the "-infinity" sentinel, since Postgres
	// rejects "" as a timestamptz literal (caught by the real-Postgres
	// integration test, see integration_test.go).
	mock.ExpectQuery(regexp.QuoteMeta(wantChangesQuery)).WithArgs("-infinity", 10).WillReturnRows(rows)

	resp, err := fetchChangesPage(db, item, "", 10)
	if err != nil {
		t.Fatalf("fetchChangesPage() error = %v", err)
	}
	if len(resp.Rows) != 1 {
		t.Fatalf("len(Rows) = %d, want 1", len(resp.Rows))
	}
	if resp.NextCursor != "2026-09-15T00:00:00Z" {
		t.Fatalf("NextCursor = %q, want 2026-09-15T00:00:00Z", resp.NextCursor)
	}
	if resp.HasMore {
		t.Fatalf("HasMore = true, want false (1 row < limit 10)")
	}
	if string(resp.Rows[0]["aliases"]) != "[]" {
		t.Fatalf("aliases = %s, want raw JSON []", resp.Rows[0]["aliases"])
	}
	var name string
	if err := json.Unmarshal(resp.Rows[0]["product_name"], &name); err != nil || name != "Widget" {
		t.Fatalf("product_name decode = %q, err=%v, want Widget", name, err)
	}
}

func TestFetchChangesPageNonEmptyCursor(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	item := testItem()
	rows := addProductNameRow(sqlmock.NewRows(item.Columns), "2026-09-16T00:00:00Z")
	mock.ExpectQuery(regexp.QuoteMeta(wantChangesQuery)).
		WithArgs("2026-09-15T00:00:00Z", 10).WillReturnRows(rows)

	resp, err := fetchChangesPage(db, item, "2026-09-15T00:00:00Z", 10)
	if err != nil {
		t.Fatalf("fetchChangesPage() error = %v", err)
	}
	if resp.NextCursor != "2026-09-16T00:00:00Z" {
		t.Fatalf("NextCursor = %q, want 2026-09-16T00:00:00Z", resp.NextCursor)
	}
}

func TestFetchChangesPageHasMoreBoundary(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	item := testItem()
	rows := addProductNameRow(sqlmock.NewRows(item.Columns), "2026-09-15T00:00:00Z")
	mock.ExpectQuery(regexp.QuoteMeta(wantChangesQuery)).WithArgs("-infinity", 1).WillReturnRows(rows)
	// A full page also fetches the rest of its final cursor value; here that
	// is just the row already returned.
	tieRows := addProductNameRow(sqlmock.NewRows(item.Columns), "2026-09-15T00:00:00Z")
	mock.ExpectQuery(regexp.QuoteMeta(wantTieQuery)).WithArgs("2026-09-15T00:00:00Z").WillReturnRows(tieRows)

	resp, err := fetchChangesPage(db, item, "", 1)
	if err != nil {
		t.Fatalf("fetchChangesPage() error = %v", err)
	}
	if !resp.HasMore {
		t.Fatalf("HasMore = false, want true (row count == limit)")
	}
	if len(resp.Rows) != 1 {
		t.Fatalf("len(Rows) = %d, want 1 (the tie query must not duplicate the returned row)", len(resp.Rows))
	}
}

// A page that fills exactly at a group of rows sharing one cursor value must
// include the whole group; the target resumes with CursorCol > next_cursor, so
// anything left behind would never be pulled.
func TestFetchChangesPageCompletesTieGroup(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	item := testItem()
	const tied = "2026-09-15T00:00:00Z"
	page := addProductNameRow(sqlmock.NewRows(item.Columns), tied)
	mock.ExpectQuery(regexp.QuoteMeta(wantChangesQuery)).WithArgs("-infinity", 1).WillReturnRows(page)

	// The tie query returns the row already sent (seq 1) plus two more with the
	// same cursor value but different natural keys.
	tieRows := addProductNameRowSeq(sqlmock.NewRows(item.Columns), tied, "1")
	tieRows = addProductNameRowSeq(tieRows, tied, "2")
	tieRows = addProductNameRowSeq(tieRows, tied, "3")
	mock.ExpectQuery(regexp.QuoteMeta(wantTieQuery)).WithArgs(tied).WillReturnRows(tieRows)

	resp, err := fetchChangesPage(db, item, "", 1)
	if err != nil {
		t.Fatalf("fetchChangesPage() error = %v", err)
	}
	if resp.NextCursor != tied {
		t.Fatalf("NextCursor = %q, want %q", resp.NextCursor, tied)
	}
	if len(resp.Rows) != 3 {
		t.Fatalf("len(Rows) = %d, want 3 (the page plus the two other rows at the same cursor, no duplicate of seq 1)", len(resp.Rows))
	}
	if !resp.HasMore {
		t.Fatalf("HasMore = false, want true")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestFetchChangesPageEmptyResultKeepsCursor(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	item := testItem()
	rows := sqlmock.NewRows(item.Columns)
	mock.ExpectQuery(regexp.QuoteMeta(wantChangesQuery)).
		WithArgs("2026-09-15T00:00:00Z", 10).WillReturnRows(rows)

	resp, err := fetchChangesPage(db, item, "2026-09-15T00:00:00Z", 10)
	if err != nil {
		t.Fatalf("fetchChangesPage() error = %v", err)
	}
	if resp.NextCursor != "2026-09-15T00:00:00Z" {
		t.Fatalf("NextCursor = %q, want unchanged 2026-09-15T00:00:00Z", resp.NextCursor)
	}
	if len(resp.Rows) != 0 {
		t.Fatalf("len(Rows) = %d, want 0", len(resp.Rows))
	}
}

// A NULL jsonb value must not fail the page (database/sql cannot scan a NULL
// into a json.RawMessage). SQL NULL travels as an absent key and the jsonb
// value `null` as the JSON literal null, so the two stay distinguishable.
func TestFetchChangesPageDistinguishesSQLNullFromJSONNull(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	item := testItem()
	rows := sqlmock.NewRows(item.Columns).AddRow(
		"1", "Cat A", "L1", "L2", "desc", "use", "Widget", "Widget EN", "Class II",
		nil, []byte("null"), "cn_nmpa_medical_device_classification_catalog", "", []byte("{}"), "approved", "2026-09-15T00:00:00Z",
		"", "", "", "", "", "",
	)
	mock.ExpectQuery(regexp.QuoteMeta(wantChangesQuery)).WithArgs("-infinity", 10).WillReturnRows(rows)

	resp, err := fetchChangesPage(db, item, "", 10)
	if err != nil {
		t.Fatalf("fetchChangesPage() error = %v", err)
	}
	if len(resp.Rows) != 1 {
		t.Fatalf("len(Rows) = %d, want 1", len(resp.Rows))
	}
	if _, present := resp.Rows[0]["aliases"]; present {
		t.Errorf("aliases = %s, want the key absent for SQL NULL", resp.Rows[0]["aliases"])
	}
	if string(resp.Rows[0]["keywords"]) != "null" {
		t.Errorf("keywords = %s, want the JSON literal null", resp.Rows[0]["keywords"])
	}
	if string(resp.Rows[0]["extra_info"]) != "{}" {
		t.Errorf("extra_info = %s, want {}", resp.Rows[0]["extra_info"])
	}
}
