package docprocessing

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestLoadActiveCategoriesParsesRow(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{"category_id", "category_key", "status", "canonical_of", "match_keys", "embedding"}).
		AddRow(int64(7), "latency", "approved", "", []byte(`["latency","lag"]`), []byte(`[0.1,0.2]`))
	mock.ExpectQuery("SELECT .* FROM kb\\.artifact_categories").
		WithArgs("metric").
		WillReturnRows(rows)

	reg := artifactCategoryRegistry{DB: db}
	got, err := reg.loadActiveCategories(context.Background(), "metric")
	if err != nil {
		t.Fatalf("loadActiveCategories error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d records, want 1", len(got))
	}
	if got[0].CategoryID != 7 || got[0].CategoryKey != "latency" {
		t.Errorf("record = %+v", got[0])
	}
	if len(got[0].MatchKeys) != 2 || got[0].MatchKeys[1] != "lag" {
		t.Errorf("MatchKeys = %#v", got[0].MatchKeys)
	}
	if len(got[0].Embedding) != 2 {
		t.Errorf("Embedding = %#v", got[0].Embedding)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestMintCategoryReturnsCategoryID(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO kb.artifact_categories (
    category_key, category_type, status, display_names, aliases, acronyms,
    category_desc, category_keywords, match_keys, search_document, required_attrs,
    specs, plausible_ranges, parent_categories, related_categories, embedding, seen_count
) VALUES ($1, $2, 'pending_review', $3::jsonb, $4::jsonb, $5::jsonb, $6, $7::jsonb,
    $8::jsonb, $9, $10::jsonb, $11::jsonb, $12::jsonb, $13::jsonb, $14::jsonb, $15::jsonb, 1)
ON CONFLICT (category_type, category_key) DO UPDATE SET
    seen_count   = kb.artifact_categories.seen_count + 1,
    last_seen_at = NOW()
RETURNING category_id`)).
		WithArgs(
			"response time",
			"metric",
			`["Latency"]`,
			`[]`,
			`["RT"]`,
			"",
			`[]`,
			`["response time","latency","rt"]`,
			"response time Latency RT",
			`[]`,
			`{}`,
			`{}`,
			`["performance metric"]`,
			`["throughput"]`,
			`[0.1,0.2]`,
		).
		WillReturnRows(sqlmock.NewRows([]string{"category_id"}).AddRow(int64(99)))

	reg := artifactCategoryRegistry{DB: db}
	c := createdCategory{
		CategoryKey:       "Response Time",
		DisplayNames:      []string{"Latency"},
		Acronyms:          []string{"RT"},
		ParentCategories:  []string{"performance metric"},
		RelatedCategories: []string{"throughput"},
	}
	id, err := reg.mintCategory(context.Background(), c, "metric", []float64{0.1, 0.2})
	if err != nil {
		t.Fatalf("mintCategory error: %v", err)
	}
	if id != 99 {
		t.Fatalf("mintCategory id = %d, want 99", id)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestAbsorbAliasIssuesConditionalUpdate(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	mock.ExpectExec(regexp.QuoteMeta(`UPDATE kb.artifact_categories`)).
		WithArgs(int64(7), "rt").
		WillReturnResult(sqlmock.NewResult(0, 1))

	reg := artifactCategoryRegistry{DB: db}
	if err := reg.absorbAlias(context.Background(), 7, "RT"); err != nil {
		t.Fatalf("absorbAlias error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestCategoryStatusesLoadsScopedType(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{"category_key", "status"}).
		AddRow("furniture", InventoryCategoryStatusApproved).
		AddRow("medical instrument", InventoryCategoryStatusPending)
	mock.ExpectQuery("SELECT category_key, status FROM kb\\.artifact_categories WHERE category_type = \\$1").
		WithArgs("inventory_item").
		WillReturnRows(rows)

	reg := artifactCategoryRegistry{DB: db}
	got, err := reg.categoryStatuses(context.Background(), "inventory_item")
	if err != nil {
		t.Fatalf("categoryStatuses error: %v", err)
	}
	if got["furniture"] != InventoryCategoryStatusApproved {
		t.Fatalf("furniture status=%q, want approved", got["furniture"])
	}
	if got["medical instrument"] != InventoryCategoryStatusPending {
		t.Fatalf("medical instrument status=%q, want pending_review", got["medical instrument"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
