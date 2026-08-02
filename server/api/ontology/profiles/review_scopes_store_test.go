package profiles

import (
	"context"
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestReviewScopeStoreCreateFreezesSelectedProfiles(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_review_scopes")).
		WithArgs("scope-1", `[2]`, `["obj-1"]`, `["ventilator-display:display_module"]`, "2026-08-01", "CN", nil, `[{"profile_id":"ventilator-display:display_metrics","release_id":42}]`, "explicit", `{}`, `["display_metrics"]`, "reviewer", "fixture", nil, nil, nil, nil, nil).
		WillReturnRows(sqlmock.NewRows([]string{"review_scope_id", "reviewed_document_ids", "target_object_ids", "target_class_term_ids", "as_of_date", "jurisdiction", "operating_context", "selected_profiles", "selection_mode", "precedence_policy", "closed_dimensions", "selected_by", "selection_reason", "create_time", "knowledge_store_id", "selection_attempt_id", "selection_status", "fact_snapshot", "selection_snapshot"}).
			AddRow("scope-1", []byte(`[2]`), []byte(`["obj-1"]`), []byte(`["ventilator-display:display_module"]`), "2026-08-01", "CN", nil, []byte(`[{"profile_id":"ventilator-display:display_metrics","release_id":42}]`), "explicit", []byte(`{}`), []byte(`["display_metrics"]`), "reviewer", "fixture", now, nil, nil, nil, nil, nil))

	got, err := (ReviewScopeStore{DB: db}).Create(context.Background(), ReviewScope{
		ReviewScopeID: "scope-1", ReviewedDocumentIDs: json.RawMessage(`[2]`), TargetObjectIDs: json.RawMessage(`["obj-1"]`), TargetClassTermIDs: json.RawMessage(`["ventilator-display:display_module"]`), AsOfDate: "2026-08-01", Jurisdiction: "CN", SelectedProfiles: json.RawMessage(`[{"profile_id":"ventilator-display:display_metrics","release_id":42}]`), SelectionMode: "explicit", PrecedencePolicy: json.RawMessage(`{}`), ClosedDimensions: json.RawMessage(`["display_metrics"]`), SelectedBy: "reviewer", SelectionReason: "fixture",
	})
	if err != nil || got.ReviewScopeID != "scope-1" {
		t.Fatalf("Create = %#v, %v", got, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReviewScopeStoreCreateP5ColumnsNullableForExplicitMode(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	now := time.Now()
	// An explicit-mode create must remain valid without populating the P5
	// columns: the store inserts NULL for each of them and reads them back as
	// zero values (explicit-mode-compatible defaults).
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_review_scopes")).
		WithArgs("scope-1", `[2]`, `["obj-1"]`, `["ventilator-display:display_module"]`, "2026-08-01", "CN", nil, `[{"profile_id":"p","release_id":42}]`, "explicit", `{}`, `["display_metrics"]`, "reviewer", "fixture", nil, nil, nil, nil, nil).
		WillReturnRows(sqlmock.NewRows([]string{"review_scope_id", "reviewed_document_ids", "target_object_ids", "target_class_term_ids", "as_of_date", "jurisdiction", "operating_context", "selected_profiles", "selection_mode", "precedence_policy", "closed_dimensions", "selected_by", "selection_reason", "create_time", "knowledge_store_id", "selection_attempt_id", "selection_status", "fact_snapshot", "selection_snapshot"}).
			AddRow("scope-1", []byte(`[2]`), []byte(`["obj-1"]`), []byte(`["ventilator-display:display_module"]`), "2026-08-01", "CN", nil, []byte(`[{"profile_id":"p","release_id":42}]`), "explicit", []byte(`{}`), []byte(`["display_metrics"]`), "reviewer", "fixture", now, nil, nil, nil, nil, nil))

	got, err := (ReviewScopeStore{DB: db}).Create(context.Background(), ReviewScope{
		ReviewScopeID: "scope-1", ReviewedDocumentIDs: json.RawMessage(`[2]`), TargetObjectIDs: json.RawMessage(`["obj-1"]`), TargetClassTermIDs: json.RawMessage(`["ventilator-display:display_module"]`), AsOfDate: "2026-08-01", Jurisdiction: "CN", SelectedProfiles: json.RawMessage(`[{"profile_id":"p","release_id":42}]`), SelectionMode: "explicit", PrecedencePolicy: json.RawMessage(`{}`), ClosedDimensions: json.RawMessage(`["display_metrics"]`), SelectedBy: "reviewer", SelectionReason: "fixture",
	})
	if err != nil || got.ReviewScopeID != "scope-1" {
		t.Fatalf("Create = %#v, %v", got, err)
	}
	if got.SelectionMode != "explicit" || got.KnowledgeStoreID != 0 || got.SelectionAttemptID != "" || got.SelectionStatus != "" || len(got.FactSnapshot) != 0 || len(got.SelectionSnapshot) != 0 {
		t.Fatalf("explicit scope must round-trip zero P5 columns, got %#v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReviewScopeStoreCreateAndGetStableSelectionAttemptID(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	now := time.Now()
	// A deterministic scope carries a stable selection_attempt_id; it must
	// survive the create/read-back round trip unchanged.
	fact := `{"document.doc_kind":"standard"}`
	snap := `[{"profile_id":"p","release_id":42}]`
	stmt := regexp.QuoteMeta("INSERT INTO kb.ontology_review_scopes")
	mock.ExpectQuery(stmt).
		WithArgs("scope-1", `[2]`, `[]`, `[]`, "2026-08-01", "CN", nil, `[{"profile_id":"p","release_id":42}]`, "deterministic_rule", `{}`, `[]`, "selector", "auto", int64(9), "attempt-abc", "complete", fact, snap).
		WillReturnRows(sqlmock.NewRows([]string{"review_scope_id", "reviewed_document_ids", "target_object_ids", "target_class_term_ids", "as_of_date", "jurisdiction", "operating_context", "selected_profiles", "selection_mode", "precedence_policy", "closed_dimensions", "selected_by", "selection_reason", "create_time", "knowledge_store_id", "selection_attempt_id", "selection_status", "fact_snapshot", "selection_snapshot"}).
			AddRow("scope-1", []byte(`[2]`), []byte(`[]`), []byte(`[]`), "2026-08-01", "CN", nil, []byte(`[{"profile_id":"p","release_id":42}]`), "deterministic_rule", []byte(`{}`), []byte(`[]`), "selector", "auto", now, int64(9), "attempt-abc", "complete", []byte(fact), []byte(snap)))

	got, err := (ReviewScopeStore{DB: db}).Create(context.Background(), ReviewScope{
		ReviewScopeID: "scope-1", ReviewedDocumentIDs: json.RawMessage(`[2]`), AsOfDate: "2026-08-01", Jurisdiction: "CN", SelectedProfiles: json.RawMessage(`[{"profile_id":"p","release_id":42}]`), SelectionMode: "deterministic_rule", SelectedBy: "selector", SelectionReason: "auto", KnowledgeStoreID: 9, SelectionAttemptID: "attempt-abc", SelectionStatus: "complete", FactSnapshot: json.RawMessage(fact), SelectionSnapshot: json.RawMessage(snap),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got.SelectionAttemptID != "attempt-abc" || got.KnowledgeStoreID != 9 || got.SelectionStatus != "complete" {
		t.Fatalf("created deterministic scope = %#v", got)
	}

	// Get returns the same stable attempt id and snapshots from history.
	mock.ExpectQuery(regexp.QuoteMeta("SELECT review_scope_id, reviewed_document_ids")).
		WithArgs("scope-1").
		WillReturnRows(sqlmock.NewRows([]string{"review_scope_id", "reviewed_document_ids", "target_object_ids", "target_class_term_ids", "as_of_date", "jurisdiction", "operating_context", "selected_profiles", "selection_mode", "precedence_policy", "closed_dimensions", "selected_by", "selection_reason", "create_time", "knowledge_store_id", "selection_attempt_id", "selection_status", "fact_snapshot", "selection_snapshot"}).
			AddRow("scope-1", []byte(`[2]`), []byte(`[]`), []byte(`[]`), "2026-08-01", "CN", nil, []byte(`[{"profile_id":"p","release_id":42}]`), "deterministic_rule", []byte(`{}`), []byte(`[]`), "selector", "auto", now, int64(9), "attempt-abc", "complete", []byte(fact), []byte(snap)))

	hist, err := (ReviewScopeStore{DB: db}).Get(context.Background(), "scope-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if hist.SelectionAttemptID != "attempt-abc" || string(hist.SelectionSnapshot) != snap || string(hist.FactSnapshot) != fact || hist.KnowledgeStoreID != 9 {
		t.Fatalf("historical Get = %#v", hist)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReviewScopeStoreGetLoadsHistoricalSelection(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT review_scope_id, reviewed_document_ids")).
		WithArgs("scope-1").
		WillReturnRows(sqlmock.NewRows([]string{"review_scope_id", "reviewed_document_ids", "target_object_ids", "target_class_term_ids", "as_of_date", "jurisdiction", "operating_context", "selected_profiles", "selection_mode", "precedence_policy", "closed_dimensions", "selected_by", "selection_reason", "create_time", "knowledge_store_id", "selection_attempt_id", "selection_status", "fact_snapshot", "selection_snapshot"}).
			AddRow("scope-1", []byte(`[2]`), []byte(`[]`), []byte(`[]`), "2026-08-01", "CN", nil, []byte(`[{"profile_id":"p","release_id":42}]`), "explicit", []byte(`{}`), []byte(`[]`), "reviewer", "historical", now, nil, nil, nil, nil, nil))
	got, err := (ReviewScopeStore{DB: db}).Get(context.Background(), "scope-1")
	if err != nil || string(got.SelectedProfiles) != `[{"profile_id":"p","release_id":42}]` {
		t.Fatalf("Get = %#v, %v", got, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
