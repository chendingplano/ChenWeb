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

	mock.ExpectQuery("INSERT INTO kb\\.artifact_categories").
		WillReturnRows(sqlmock.NewRows([]string{"category_id"}).AddRow(int64(99)))

	reg := artifactCategoryRegistry{DB: db}
	c := createdCategory{CategoryKey: "Response Time", DisplayNames: []string{"Latency"}, Acronyms: []string{"RT"}}
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
