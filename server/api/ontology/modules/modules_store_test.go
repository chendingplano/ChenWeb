package modules

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
)

func TestModuleStoreCreateModuleInsertsIdentity(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_modules")).
		WithArgs("core", "Core semantic module", "platform", nil, pq.Array([]string{}), "draft", "tester", "tester").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "module_id", "title", "owner", "description", "depends_on", "status",
			"create_time", "create_by", "modify_time", "modify_by",
		}).AddRow(int64(1), "core", "Core semantic module", "platform", "", []byte("{}"), "draft",
			now, "tester", now, "tester"))

	store := ModuleStore{DB: db}
	got, err := store.CreateModule(context.Background(), Module{
		ModuleID: "core",
		Title:    "Core semantic module",
		Owner:    "platform",
		CreateBy: "tester",
		ModifyBy: "tester",
	})
	if err != nil {
		t.Fatalf("CreateModule: %v", err)
	}
	if got.ModuleID != "core" || got.Status != "draft" {
		t.Fatalf("unexpected module: %#v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}
