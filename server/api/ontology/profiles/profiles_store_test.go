package profiles

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
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

func TestLoadReleasedProfilesPinsReleaseIDsAndChecksums(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	now := time.Now()
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT ar.module_id, ar.release_id, r.version, r.content_checksum\nFROM kb.ontology_active_releases ar")).
		WillReturnRows(sqlmock.NewRows([]string{"module_id", "release_id", "version", "content_checksum"}).
			AddRow("ventilator-display", int64(42), "0.1.0", "sha256:aaa").
			AddRow("ventilator-domain", int64(43), "0.2.0", "sha256:bbb"))
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_profiles p\nJOIN kb.ontology_module_releases r ON r.id = p.released_in_release_id")).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "profile_id", "version", "module_id", "status", "title", "applicability", "closed_dimensions", "release_id", "release_version", "create_time", "create_by", "modify_time", "modify_by",
		}).AddRow(
			int64(7), "ventilator-display:display_metrics", 1, "ventilator-display", "included_in_release", "Display metrics", []byte(`{}`), []byte(`[]`), int64(42), "0.1.0", now, "curator", now, "curator",
		))
	mock.ExpectCommit()

	got, err := (ProfileStore{DB: db}).LoadReleasedProfiles(context.Background())
	if err != nil {
		t.Fatalf("LoadReleasedProfiles: %v", err)
	}
	if len(got.Releases) != 2 {
		t.Fatalf("pins = %#v", got.Releases)
	}
	if got.Releases[0].ModuleID != "ventilator-display" || got.Releases[0].ReleaseID != 42 || got.Releases[0].Version != "0.1.0" || got.Releases[0].Checksum != "sha256:aaa" {
		t.Fatalf("pin[0] = %#v", got.Releases[0])
	}
	if got.Releases[1].ReleaseID != 43 || got.Releases[1].Checksum != "sha256:bbb" {
		t.Fatalf("pin[1] = %#v", got.Releases[1])
	}
	if len(got.Profiles) != 1 || got.Profiles[0].ProfileID != "ventilator-display:display_metrics" || got.Profiles[0].ReleaseID != 42 || got.Profiles[0].ReleaseVersion != "0.1.0" {
		t.Fatalf("profiles = %#v", got.Profiles)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestLoadReleasedProfilesOnlyLoadsIncludedInReleaseProfiles(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	now := time.Now()
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT ar.module_id, ar.release_id, r.version, r.content_checksum\nFROM kb.ontology_active_releases ar")).
		WillReturnRows(sqlmock.NewRows([]string{"module_id", "release_id", "version", "content_checksum"}).
			AddRow("ventilator-display", int64(42), "0.1.0", "sha256:aaa"))
	// The profile query must filter by the pinned release ids and by
	// included_in_release status, never joining current activation; draft or
	// unreleased profiles can never become visible through this path. The
	// anchored ^...$ match asserts the complete statement, so an accidental
	// kb.ontology_active_releases join or a wrong release alias fails the
	// test instead of passing a substring match.
	profileQuery := "SELECT " + releasedProfileColumns + `
FROM kb.ontology_profiles p
JOIN kb.ontology_module_releases r ON r.id = p.released_in_release_id
WHERE r.id = ANY($1::bigint[])
  AND p.status = 'included_in_release'
ORDER BY p.profile_id, p.version DESC`
	mock.ExpectQuery("^" + regexp.QuoteMeta(profileQuery) + "$").
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "profile_id", "version", "module_id", "status", "title", "applicability", "closed_dimensions", "release_id", "release_version", "create_time", "create_by", "modify_time", "modify_by",
		}).AddRow(
			int64(7), "ventilator-display:display_metrics", 1, "ventilator-display", "included_in_release", "Display metrics", []byte(`{}`), []byte(`[]`), int64(42), "0.1.0", now, "curator", now, "curator",
		))
	mock.ExpectCommit()

	got, err := (ProfileStore{DB: db}).LoadReleasedProfiles(context.Background())
	if err != nil {
		t.Fatalf("LoadReleasedProfiles: %v", err)
	}
	if len(got.Profiles) != 1 || got.Profiles[0].Status != "included_in_release" {
		t.Fatalf("loaded profiles = %#v", got.Profiles)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestLoadReleasedProfilesRunsInRepeatableReadTransaction(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	now := time.Now()
	// The loader must read+pin+load inside one short repeatable-read
	// transaction (spec §6 item 2) and end it before returning, so no
	// classifier call can ever run inside the transaction. sqlmock's ordered
	// expectations pin the structure: BEGIN, the two reads, COMMIT, and
	// nothing after COMMIT.
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_active_releases ar")).
		WillReturnRows(sqlmock.NewRows([]string{"module_id", "release_id", "version", "content_checksum"}).
			AddRow("ventilator-display", int64(42), "0.1.0", "sha256:aaa"))
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_profiles p\nJOIN kb.ontology_module_releases r ON r.id = p.released_in_release_id")).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "profile_id", "version", "module_id", "status", "title", "applicability", "closed_dimensions", "release_id", "release_version", "create_time", "create_by", "modify_time", "modify_by",
		}).AddRow(
			int64(7), "ventilator-display:display_metrics", 1, "ventilator-display", "included_in_release", "Display metrics", []byte(`{}`), []byte(`[]`), int64(42), "0.1.0", now, "curator", now, "curator",
		))
	mock.ExpectCommit()

	got, err := (ProfileStore{DB: db}).LoadReleasedProfiles(context.Background())
	if err != nil {
		t.Fatalf("LoadReleasedProfiles: %v", err)
	}
	if len(got.Releases) != 1 || len(got.Profiles) != 1 {
		t.Fatalf("LoadReleasedProfiles = %#v", got)
	}
	// Every expectation was consumed exactly once and the transaction ended
	// with COMMIT before the loader returned; nothing else ran.
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestProfileStoreDeriveKnowledgeStoreSingleStore(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT ks_store_id FROM kb.inputs WHERE id = ANY($1::bigint[])")).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"ks_store_id"}).AddRow(int64(9)).AddRow(int64(9)))

	got, err := (ProfileStore{DB: db}).DeriveKnowledgeStore(context.Background(), []int64{101, 102})
	if err != nil {
		t.Fatalf("DeriveKnowledgeStore: %v", err)
	}
	if got != 9 {
		t.Fatalf("knowledge store = %d, want 9", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestProfileStoreDeriveKnowledgeStoreRejectsMixedStores(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT ks_store_id FROM kb.inputs WHERE id = ANY($1::bigint[])")).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"ks_store_id"}).AddRow(int64(9)).AddRow(int64(11)))

	_, err = (ProfileStore{DB: db}).DeriveKnowledgeStore(context.Background(), []int64{101, 102})
	if err == nil || !strings.Contains(err.Error(), "multiple knowledge stores") {
		t.Fatalf("DeriveKnowledgeStore error = %v, want mixed-store rejection", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestProfileStoreDeriveKnowledgeStoreRejectsUnresolvableDocuments(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	// Only one of the two documents resolves; the selection cannot derive a
	// complete single knowledge store from the request.
	mock.ExpectQuery(regexp.QuoteMeta("SELECT ks_store_id FROM kb.inputs WHERE id = ANY($1::bigint[])")).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"ks_store_id"}).AddRow(int64(9)))

	_, err = (ProfileStore{DB: db}).DeriveKnowledgeStore(context.Background(), []int64{101, 102})
	if err == nil || !strings.Contains(err.Error(), "do not resolve") {
		t.Fatalf("DeriveKnowledgeStore error = %v, want unresolvable-document error", err)
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
