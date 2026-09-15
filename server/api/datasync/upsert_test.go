package datasync

import (
	"context"
	"encoding/json"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

const wantUpsertQuery = `INSERT INTO kb.product_names (seq_no, sub_catalog, category_l1, category_l2, description, intended_use, product_name, product_name_en, regulatory_class, aliases, keywords, source, notes, extra_info, status, update_time) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16) ON CONFLICT (source, seq_no, product_name) DO UPDATE SET sub_catalog = EXCLUDED.sub_catalog, category_l1 = EXCLUDED.category_l1, category_l2 = EXCLUDED.category_l2, description = EXCLUDED.description, intended_use = EXCLUDED.intended_use, product_name_en = EXCLUDED.product_name_en, regulatory_class = EXCLUDED.regulatory_class, aliases = EXCLUDED.aliases, keywords = EXCLUDED.keywords, notes = EXCLUDED.notes, extra_info = EXCLUDED.extra_info, status = EXCLUDED.status, update_time = EXCLUDED.update_time`

func jsonStr(t *testing.T, v string) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal(%q) error = %v", v, err)
	}
	return b
}

func testRow(t *testing.T) Row {
	return Row{
		"seq_no":           jsonStr(t, "1"),
		"sub_catalog":      jsonStr(t, "Cat A"),
		"category_l1":      jsonStr(t, "L1"),
		"category_l2":      jsonStr(t, "L2"),
		"description":      jsonStr(t, "desc"),
		"intended_use":     jsonStr(t, "use"),
		"product_name":     jsonStr(t, "Widget"),
		"product_name_en":  jsonStr(t, "Widget EN"),
		"regulatory_class": jsonStr(t, "Class II"),
		"aliases":          json.RawMessage(`["W"]`),
		"keywords":         json.RawMessage(`{"k":1}`),
		"source":           jsonStr(t, "cn_nmpa_medical_device_classification_catalog"),
		"notes":            nullJSON,
		"extra_info":       json.RawMessage(`{}`),
		"status":           jsonStr(t, "approved"),
		"update_time":      jsonStr(t, "2026-09-15T00:00:00Z"),
	}
}

// TestUpsertRowsAppliesInsertOrUpdate covers both the "new row" and "existing
// row" cases: Postgres decides INSERT vs. the ON CONFLICT DO UPDATE branch at
// runtime, so both paths issue the identical statement with the same args --
// this asserts that statement and its argument decoding are correct.
func TestUpsertRowsAppliesInsertOrUpdate(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	item := testItem()
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(wantUpsertQuery)).
		WithArgs(
			"1", "Cat A", "L1", "L2", "desc", "use", "Widget", "Widget EN", "Class II",
			`["W"]`, `{"k":1}`, "cn_nmpa_medical_device_classification_catalog", nil, `{}`, "approved", "2026-09-15T00:00:00Z",
		).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := upsertRows(context.Background(), db, item, []Row{testRow(t)}); err != nil {
		t.Fatalf("upsertRows() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestUpsertRowsNoRowsIsNoop(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	if err := upsertRows(context.Background(), db, testItem(), nil); err != nil {
		t.Fatalf("upsertRows() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected DB interaction for empty rows: %v", err)
	}
}

func TestRowArgsJSONColumnsPassThroughRaw(t *testing.T) {
	args, err := rowArgs(testItem(), testRow(t))
	if err != nil {
		t.Fatalf("rowArgs() error = %v", err)
	}
	// aliases is index 9 in item.Columns.
	if args[9] != `["W"]` {
		t.Fatalf("aliases arg = %v, want raw JSON string [\"W\"]", args[9])
	}
}

func TestRowArgsNullColumnBecomesNilArg(t *testing.T) {
	args, err := rowArgs(testItem(), testRow(t))
	if err != nil {
		t.Fatalf("rowArgs() error = %v", err)
	}
	// notes is index 12 in item.Columns.
	if args[12] != nil {
		t.Fatalf("notes arg = %v, want nil", args[12])
	}
}
