package docprocessing

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestEntityObjectSQLStoreLoadEntitiesForRecordReadsRow(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("FROM kb.entities").
		WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{
			"entity_id", "entity", "entity_en", "entity_type", "entity_type_en",
			"aliases", "aliases_en", "desc_text", "desc_text_en",
		}).AddRow(
			"9_ent_1", "Pump A", "Pump A",
			"equipment", "equipment",
			[]byte(`["泵A"]`), []byte(`[]`),
			"a pump", "a pump",
		))

	store := EntityObjectSQLStore{DB: db}
	rows, err := store.LoadEntitiesForRecord(context.Background(), 9)
	if err != nil {
		t.Fatalf("LoadEntitiesForRecord: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %+v, want 1", rows)
	}
	got := rows[0]
	if got.EntityID != "9_ent_1" || got.Entity != "Pump A" || got.EntityType != "equipment" {
		t.Fatalf("got %+v, unexpected shape", got)
	}
	if len(got.Aliases) != 1 || got.Aliases[0] != "泵A" {
		t.Fatalf("got.Aliases = %+v, want [泵A]", got.Aliases)
	}
	if got.InputRecordID != 9 {
		t.Fatalf("got.InputRecordID = %d, want 9", got.InputRecordID)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestEntityObjectSQLStoreSetEntityObjectLinkStatusWritesRow(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.entities")).
		WithArgs(entityObjectLinkExcluded, "9_ent_1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	store := EntityObjectSQLStore{DB: db}
	if err := store.SetEntityObjectLinkStatus(context.Background(), "9_ent_1", entityObjectLinkExcluded); err != nil {
		t.Fatalf("SetEntityObjectLinkStatus: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestArtifactObjectSQLStoreInsertOneWritesSingleRowWithoutDeletingSiblings(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO kb.artifact_objects")).
		WillReturnResult(sqlmock.NewResult(1, 1))

	store := ArtifactObjectSQLStore{DB: db}
	err = store.InsertOne(context.Background(), ArtifactObject{
		SourceRecordID: 9,
		InputRecordID:  9,
		ArtifactType:   "entity",
		ArtifactID:     "9_ent_1",
		ObjectName:     "Pump A",
		ObjectType:     "equipment",
		ObjectRole:     "represented_entity",
	})
	if err != nil {
		t.Fatalf("InsertOne: %v", err)
	}
	// No DELETE should be issued by a single-row insert — that would wipe
	// sibling entity-object rows from other records processed earlier in the
	// same Phase 4 backlog-drain pass (unlike ReplaceObjectsForRecord, which
	// is only safe for Phase 3's per-record batch).
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
