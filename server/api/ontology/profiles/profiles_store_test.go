package profiles

import (
	"context"
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestActiveProfilesExcludeDraft(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_profiles p\nJOIN kb.ontology_module_releases r")).
		WithArgs("ventilator-display").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "profile_id", "version", "module_id", "status", "title", "applicability", "closed_dimensions", "release_id", "release_version", "create_time", "create_by", "modify_time", "modify_by",
		}).AddRow(
			int64(7), "ventilator-display:display_metrics", 1, "ventilator-display", "included_in_release", "Display metrics", []byte(`{}`), []byte(`[]`), int64(42), "0.1.0", now, "curator", now, "curator",
		))

	store := ProfileStore{DB: db}
	got, err := store.ListActiveProfiles(context.Background(), "ventilator-display")
	if err != nil {
		t.Fatalf("ListActiveProfiles: %v", err)
	}
	if len(got) != 1 || got[0].ProfileID != "ventilator-display:display_metrics" || got[0].ReleaseID != 42 {
		t.Fatalf("active profiles = %#v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestProfileStoreListApprovedProfilesForRelease(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_profiles\nWHERE module_id = $1 AND status = 'approved'")).
		WithArgs("ventilator-display").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "profile_id", "version", "module_id", "status", "title", "applicability", "closed_dimensions", "create_time", "create_by", "modify_time", "modify_by",
		}).AddRow(
			int64(7), "ventilator-display:display_metrics", 1, "ventilator-display", "approved", "Display metrics", []byte(`{}`), []byte(`[]`), now, "curator", now, "curator",
		))

	got, err := (ProfileStore{DB: db}).ListApprovedProfiles(context.Background(), "ventilator-display")
	if err != nil {
		t.Fatalf("ListApprovedProfiles: %v", err)
	}
	if len(got) != 1 || got[0].Status != "approved" {
		t.Fatalf("approved profiles = %#v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestProfileStoreCreateProfileStartsDraftVersionOne(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_profiles")).
		WithArgs("ventilator-display:display_metrics", "ventilator-display", "draft", "Display metrics", `{"facet":"doc_kind"}`, `["display_metrics"]`, "curator", "curator").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "profile_id", "version", "module_id", "status", "title", "applicability", "closed_dimensions", "create_time", "create_by", "modify_time", "modify_by",
		}).AddRow(
			int64(7), "ventilator-display:display_metrics", 1, "ventilator-display", "draft", "Display metrics", []byte(`{"facet":"doc_kind"}`), []byte(`["display_metrics"]`), now, "curator", now, "curator",
		))

	got, err := (ProfileStore{DB: db}).CreateProfile(context.Background(), Profile{
		ProfileID:        "ventilator-display:display_metrics",
		ModuleID:         "ventilator-display",
		Title:            "Display metrics",
		Applicability:    json.RawMessage(`{"facet":"doc_kind"}`),
		ClosedDimensions: json.RawMessage(`["display_metrics"]`),
		CreateBy:         "curator",
		ModifyBy:         "curator",
	})
	if err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}
	if got.Version != 1 || got.Status != "draft" {
		t.Fatalf("created profile = %#v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestProfileStoreTransitionStatusAllowsDraftToInReview(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_profiles\nWHERE profile_id = $1 AND version = $2")).WithArgs("p", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "profile_id", "version", "module_id", "status", "title", "applicability", "closed_dimensions", "create_time", "create_by", "modify_time", "modify_by"}).AddRow(int64(1), "p", 1, "m", "draft", "", []byte(`{}`), []byte(`[]`), now, "", now, ""))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.ontology_profiles")).WithArgs("p", 1, "in_review", "curator").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_profiles\nWHERE profile_id = $1 AND version = $2")).WithArgs("p", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "profile_id", "version", "module_id", "status", "title", "applicability", "closed_dimensions", "create_time", "create_by", "modify_time", "modify_by"}).AddRow(int64(1), "p", 1, "m", "in_review", "", []byte(`{}`), []byte(`[]`), now, "", now, "curator"))
	got, err := (ProfileStore{DB: db}).TransitionStatus(context.Background(), "p", 1, "in_review", "curator")
	if err != nil || got.Status != "in_review" {
		t.Fatalf("TransitionStatus = %#v, %v", got, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
