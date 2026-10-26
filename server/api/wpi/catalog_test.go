package wpi

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestSplitAndQuoteTable(t *testing.T) {
	if s, n := splitTable("kb.wpi_demo_customers"); s != "kb" || n != "wpi_demo_customers" {
		t.Fatalf("got %s.%s", s, n)
	}
	if s, n := splitTable("customers"); s != "public" || n != "customers" {
		t.Fatalf("got %s.%s", s, n)
	}
	if got := quoteTable("kb.wpi_demo_customers"); got != `"kb"."wpi_demo_customers"` {
		t.Fatal(got)
	}
}

func TestCatalogColumns(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	q := `SELECT column_name FROM information_schema.columns WHERE table_schema = \$1 AND table_name = \$2`
	mock.ExpectQuery(q).WithArgs("kb", "wpi_demo_customers").WillReturnRows(sqlmock.NewRows([]string{"column_name"}).AddRow("id").AddRow("email"))
	mock.ExpectQuery(q).WithArgs("public", "missing").WillReturnRows(sqlmock.NewRows([]string{"column_name"}))
	mock.ExpectQuery(q).WithArgs("public", "broken").WillReturnError(errors.New("db down"))

	cols := catalogColumns(db)
	got, err := cols(context.Background(), "kb.wpi_demo_customers")
	if err != nil || !got["id"] || !got["email"] || len(got) != 2 {
		t.Fatalf("got %v, %v", got, err)
	}
	if got, err := cols(context.Background(), "missing"); err != nil || got != nil {
		t.Fatalf("missing table: got %v, %v", got, err)
	}
	if _, err := cols(context.Background(), "broken"); err == nil {
		t.Fatal("database errors must be returned")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
