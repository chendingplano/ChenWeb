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
		WithArgs("scope-1", `[2]`, `["obj-1"]`, `["ventilator-display:display_module"]`, "2026-08-01", "CN", nil, `[{"profile_id":"ventilator-display:display_metrics","release_id":42}]`, "explicit", `{}`, `["display_metrics"]`, "reviewer", "fixture").
		WillReturnRows(sqlmock.NewRows([]string{"review_scope_id", "reviewed_document_ids", "target_object_ids", "target_class_term_ids", "as_of_date", "jurisdiction", "operating_context", "selected_profiles", "selection_mode", "precedence_policy", "closed_dimensions", "selected_by", "selection_reason", "create_time"}).
			AddRow("scope-1", []byte(`[2]`), []byte(`["obj-1"]`), []byte(`["ventilator-display:display_module"]`), "2026-08-01", "CN", nil, []byte(`[{"profile_id":"ventilator-display:display_metrics","release_id":42}]`), "explicit", []byte(`{}`), []byte(`["display_metrics"]`), "reviewer", "fixture", now))

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
