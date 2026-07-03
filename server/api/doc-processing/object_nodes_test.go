package docprocessing

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestObjectNodeCreateNodeStoresEmptyArraysForNilSlices(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	store := ObjectNodeSQLStore{DB: db}
	mock.ExpectQuery("INSERT INTO kb.object_nodes").
		WithArgs(
			sqlmock.AnyArg(),
			"pump",
			nil,
			nil,
			nil,
			"equipment",
			"[]",
			"[]",
			"[]",
			nil,
			"pump equipment self",
			"active",
			sqlmock.AnyArg(),
		).
		WillReturnRows(sqlmock.NewRows([]string{"object_id"}).AddRow("obj_1"))

	_, err = store.CreateNode(context.Background(), ArtifactObject{
		InputRecordID: 1,
		ObjectName:    "pump",
		ObjectType:    "equipment",
		ObjectRole:    "self",
	})
	if err != nil {
		t.Fatalf("CreateNode: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
