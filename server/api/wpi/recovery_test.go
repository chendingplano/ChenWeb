package wpi

import (
	"github.com/DATA-DOG/go-sqlmock"
	"testing"
)

func TestRecoveryRequiresAdmin(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()
	for _, tc := range []struct {
		name    string
		handler *Handler
		status  int
	}{
		{"anonymous", testHandler(db, nil), 401}, {"non-admin", testHandler(db, plainUser), 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := call(t, tc.handler.PageRecovery, "GET", "", "pageId", "page")
			if rec.Code != tc.status {
				t.Fatalf("got %d", rec.Code)
			}
		})
	}
}

func TestRecoveryRejectsStaleVersion(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).WithArgs("page").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT version FROM kb\.frontend_pages`).WithArgs("page").WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(2))
	mock.ExpectRollback()
	rec := call(t, testHandler(db, adminUser).PageRecovery, "PUT", `{"version":1,"page":{"descriptor_id":"page"}}`, "pageId", "page")
	if rec.Code != 409 {
		t.Fatalf("got %d: %s", rec.Code, rec.Body)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
