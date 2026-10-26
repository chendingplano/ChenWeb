package wpi

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

const latestQuery = `SELECT DISTINCT ON \(descriptor_id\) descriptor_id, version, body FROM kb\.wpi_descriptors WHERE kind = \$1 AND status = 'active' AND descriptor_id = ANY\(\$2\) ORDER BY descriptor_id, version DESC`

func TestStoreLatest(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// The query returns one row per descriptor (DISTINCT ON); drafts and
	// older versions are filtered out by SQL, so only version 2 arrives here.
	mock.ExpectQuery(latestQuery).WithArgs("page", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"descriptor_id", "version", "body"}).
			AddRow("customer-form", 2, []byte(`{"descriptor_id": "customer-form", "limit": 5}`)))
	got, err := store{db}.latest(context.Background(), KindPage, []string{"customer-form", "missing"})
	if err != nil {
		t.Fatal(err)
	}
	d, ok := got["customer-form"]
	if !ok || d.Version != 2 || d.Kind != KindPage || len(got) != 1 {
		t.Fatalf("got %+v", got)
	}
	if n, _ := asInt(d.Body["limit"]); n != 5 {
		t.Fatalf("numbers must decode as json.Number: %#v", d.Body["limit"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStoreLatestRejectsNonObjectBody(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectQuery(latestQuery).WillReturnRows(sqlmock.NewRows([]string{"descriptor_id", "version", "body"}).AddRow("x", 1, []byte(`null`)))
	if _, err := (store{db}).latest(context.Background(), KindPage, []string{"x"}); err == nil {
		t.Fatal("a null body must be an error")
	}
}

func TestStoreInsertVersion(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectQuery(`INSERT INTO kb\.wpi_descriptors .+ COALESCE\(MAX\(version\), 0\) \+ 1,.+ RETURNING version`).
		WithArgs("page", "customers", SchemaVersion, `{"a":1}`, "cding@example.com").
		WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(3))
	v, err := store{db}.insertVersion(context.Background(), KindPage, "customers", []byte(`{"a":1}`), "cding@example.com")
	if err != nil || v != 3 {
		t.Fatalf("got %d, %v", v, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
