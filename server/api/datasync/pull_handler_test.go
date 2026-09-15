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
		},
		JSONColumns: []string{"aliases", "keywords", "extra_info"},
		Filter:      "source = 'cn_nmpa_medical_device_classification_catalog'",
	}
}

const wantChangesQuery = `SELECT seq_no, sub_catalog, category_l1, category_l2, description, intended_use, product_name, product_name_en, regulatory_class, aliases, keywords, source, notes, extra_info, status, update_time FROM kb.product_names WHERE (source = 'cn_nmpa_medical_device_classification_catalog') AND update_time > $1 ORDER BY update_time, source, seq_no, product_name LIMIT $2`

// JSON columns use []byte, matching what the real Postgres driver (lib/pq)
// returns for jsonb text-format values -- unlike a plain Go string, []byte is
// what database/sql's generic converter accepts when scanning into
// *json.RawMessage.
func addProductNameRow(rows *sqlmock.Rows, updateTime string) *sqlmock.Rows {
	return rows.AddRow(
		"1", "Cat A", "L1", "L2", "desc", "use", "Widget", "Widget EN", "Class II",
		[]byte("[]"), []byte("{}"), "cn_nmpa_medical_device_classification_catalog", "", []byte("{}"), "approved", updateTime,
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

	resp, err := fetchChangesPage(db, item, "", 1)
	if err != nil {
		t.Fatalf("fetchChangesPage() error = %v", err)
	}
	if !resp.HasMore {
		t.Fatalf("HasMore = false, want true (row count == limit)")
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
