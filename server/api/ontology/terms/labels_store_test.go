package terms

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestLabelStoreCreateLabelEnforcesOnePrefLabelPerLang(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	// prefLabelExists check returns true -> the create must be refused with
	// no INSERT.
	mock.ExpectQuery(regexp.QuoteMeta("SELECT EXISTS")).
		WithArgs("core:assertion", "en").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	store := LabelStore{DB: db}
	_, err = store.CreateLabel(context.Background(), TermLabel{
		TermID:    "core:assertion",
		Label:     "assertion",
		Lang:      "en",
		LabelRole: "prefLabel",
	})
	if err == nil {
		t.Fatal("expected error for duplicate prefLabel")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestLabelStoreCreateLabelInsertsWhenNoPrefLabelExists(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT EXISTS")).
		WithArgs("core:assertion", "en").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_term_labels")).
		WithArgs("core:assertion", "assertion", "en", "prefLabel", "draft", nil, "tester", "tester").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "term_id", "version", "label", "lang", "label_role", "status",
			"source_candidate_id", "create_time", "create_by", "modify_time", "modify_by",
		}).AddRow(int64(1), "core:assertion", 1, "assertion", "en", "prefLabel", "draft",
			nil, now, "tester", now, "tester"))

	store := LabelStore{DB: db}
	got, err := store.CreateLabel(context.Background(), TermLabel{
		TermID:    "core:assertion",
		Label:     "assertion",
		Lang:      "en",
		LabelRole: "prefLabel",
		Status:    "draft",
		CreateBy:  "tester",
		ModifyBy:  "tester",
	})
	if err != nil {
		t.Fatalf("CreateLabel: %v", err)
	}
	if got.Label != "assertion" || got.Version != 1 {
		t.Fatalf("unexpected label: %#v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestLabelStoreCreateAltLabelSkipsPrefLabelCheck(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	now := time.Now()
	// altLabel: no prefLabel uniqueness check -> only the INSERT query.
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_term_labels")).
		WithArgs("core:assertion", "断言", "zh_cn", "altLabel", "draft", nil, "tester", "tester").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "term_id", "version", "label", "lang", "label_role", "status",
			"source_candidate_id", "create_time", "create_by", "modify_time", "modify_by",
		}).AddRow(int64(1), "core:assertion", 1, "断言", "zh_cn", "altLabel", "draft",
			nil, now, "tester", now, "tester"))

	store := LabelStore{DB: db}
	got, err := store.CreateLabel(context.Background(), TermLabel{
		TermID:    "core:assertion",
		Label:     "断言",
		Lang:      "zh_cn",
		LabelRole: "altLabel",
		Status:    "draft",
		CreateBy:  "tester",
		ModifyBy:  "tester",
	})
	if err != nil {
		t.Fatalf("CreateLabel: %v", err)
	}
	if got.LabelRole != "altLabel" {
		t.Fatalf("unexpected label role: %q", got.LabelRole)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}
