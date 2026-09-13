package productreviews

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func ptr[T any](v T) *T { return &v }

// Scenario: Profile created from a product name (spec: product-scope-profile).
func TestCreateProfile(t *testing.T) {
	store, mock, done := newMockStore(t)
	defer done()

	mock.ExpectBegin()
	mock.ExpectQuery(rx("INSERT INTO kb.product_profiles")).
		WithArgs("-", "Ventilator", "", []byte("[]"), "").
		WillReturnRows(profileRows(1, "Ventilator", 1, "draft", false, 0))
	mock.ExpectExec(rx("INSERT INTO kb.product_profile_nodes")).
		WithArgs(int64(1), KindProduct, "Ventilator", OriginUserAdded, StatusAccepted).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	p, err := store.CreateProfile(context.Background(), NewProfileInput{Name: "Ventilator"})
	if err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}
	if p.ID != 1 || p.Version != 1 || p.Status != ProfileDraft {
		t.Fatalf("profile = %+v, want id 1 / version 1 / draft", p)
	}
}

// Scenario: Cycle is rejected (spec: product-scope-profile) — CWB_KB_PMR_010,
// profile left unchanged (transaction rolled back).
func TestUpdateNodeCycleRejected(t *testing.T) {
	store, mock, done := newMockStore(t)
	defer done()

	one, two := int64(1), int64(2)
	mock.ExpectBegin()
	mock.ExpectQuery(rx("FROM kb.product_profile_nodes WHERE profile_id = $1")).
		WithArgs(int64(1)).
		WillReturnRows(nodeRowsFrom(1,
			nodeRow{ID: 1, Kind: KindProduct, Label: "Ventilator"},
			nodeRow{ID: 2, ParentID: &one, Label: "Display"},
			nodeRow{ID: 3, ParentID: &two, Label: "Backlight"},
		))
	mock.ExpectRollback()

	err := store.UpdateNode(context.Background(), 1, 2, NodeEdit{ParentNodeID: ptr(int64(3))})
	var pmr *PMRError
	if !errors.As(err, &pmr) || pmr.Code != CodeCycle {
		t.Fatalf("err = %v, want CWB_KB_PMR_010", err)
	}
}

// Scenario: any node-set mutation increments the version (spec: Profile
// curation and versioning).
func TestAddNodeBumpsVersion(t *testing.T) {
	store, mock, done := newMockStore(t)
	defer done()

	mock.ExpectBegin()
	mock.ExpectQuery(rx("FROM kb.product_profile_nodes WHERE profile_id = $1")).
		WithArgs(int64(1)).
		WillReturnRows(nodeRowsFrom(1, nodeRow{ID: 1, Kind: KindProduct, Label: "Ventilator", Status: StatusAccepted, Origin: OriginUserAdded}))
	mock.ExpectQuery(rx("INSERT INTO kb.product_profile_nodes")).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(5)))
	mock.ExpectExec(rx("UPDATE kb.product_profiles")).
		WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	id, err := store.AddNode(context.Background(), 1, ptr(int64(1)), ProfileNode{NodeKind: KindPart, Label: "Oxygen Sensor"})
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	if id != 5 {
		t.Fatalf("new node id = %d, want 5", id)
	}
}

// A profile has exactly one product root (spec: product-scope-profile model).
func TestAddNodeRejectsSecondRoot(t *testing.T) {
	store, mock, done := newMockStore(t)
	defer done()

	mock.ExpectBegin()
	mock.ExpectQuery(rx("FROM kb.product_profile_nodes WHERE profile_id = $1")).
		WithArgs(int64(1)).
		WillReturnRows(nodeRowsFrom(1, nodeRow{ID: 1, Kind: KindProduct, Label: "Ventilator"}))
	mock.ExpectRollback()

	_, err := store.AddNode(context.Background(), 1, nil, ProfileNode{NodeKind: KindProduct, Label: "Anaesthesia Machine"})
	if !errors.Is(err, ErrMultipleRoots) {
		t.Fatalf("err = %v, want ErrMultipleRoots", err)
	}
}

// Deleting a node bumps the version (spec: Rejecting a node ... version
// increments — same rule for delete).
func TestDeleteNodeBumpsVersion(t *testing.T) {
	store, mock, done := newMockStore(t)
	defer done()

	mock.ExpectBegin()
	mock.ExpectExec(rx("DELETE FROM kb.product_profile_nodes")).
		WithArgs(int64(5), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(rx("UPDATE kb.product_profiles")).
		WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := store.DeleteNode(context.Background(), 1, 5); err != nil {
		t.Fatalf("DeleteNode: %v", err)
	}
}

// Scenario: keywords/notes captured on the intake form round-trip through
// create (spec: product-review-intake — keywords are stored on the profile).
func TestCreateProfileWithKeywordsAndNotes(t *testing.T) {
	store, mock, done := newMockStore(t)
	defer done()

	mock.ExpectBegin()
	mock.ExpectQuery(rx("INSERT INTO kb.product_profiles")).
		WithArgs("-", "Ventilator", "", []byte(`["icu","respiratory"]`), "urgent, needs recheck").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "name", "product_description", "keywords", "notes", "version", "status",
			"truncated", "truncated_count", "created_at", "updated_at",
		}).AddRow(1, "-", "Ventilator", "", []byte(`["icu","respiratory"]`), "urgent, needs recheck", 1, "draft", false, 0, time.Now(), time.Now()))
	mock.ExpectExec(rx("INSERT INTO kb.product_profile_nodes")).
		WithArgs(int64(1), KindProduct, "Ventilator", OriginUserAdded, StatusAccepted).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	p, err := store.CreateProfile(context.Background(), NewProfileInput{
		Name: "Ventilator", Keywords: []string{"icu", "respiratory"}, Notes: "urgent, needs recheck",
	})
	if err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}
	if len(p.Keywords) != 2 || p.Keywords[0] != "icu" || p.Notes != "urgent, needs recheck" {
		t.Fatalf("profile = %+v, want keywords [icu respiratory] / notes preserved", p)
	}
}

// Scenario: submitting a new product name finds no existing profile (spec:
// product-review-intake — first-time product name).
func TestFindProfileByNameNoMatch(t *testing.T) {
	store, mock, done := newMockStore(t)
	defer done()

	mock.ExpectQuery(rx("WHERE tenant_id = $1 AND LOWER(TRIM(name)) = LOWER(TRIM($2))")).
		WithArgs("-", "Ventilator").
		WillReturnError(sql.ErrNoRows)

	p, err := store.FindProfileByName(context.Background(), "", "Ventilator")
	if err != nil {
		t.Fatalf("FindProfileByName: %v", err)
	}
	if p != nil {
		t.Fatalf("p = %+v, want nil (no match)", p)
	}
}

// Scenario: submitting a previously reviewed product name matches, even with
// different case/whitespace (spec: product-review-intake — duplicate
// detection is normalized exact match).
func TestFindProfileByNameMatch(t *testing.T) {
	store, mock, done := newMockStore(t)
	defer done()

	mock.ExpectQuery(rx("WHERE tenant_id = $1 AND LOWER(TRIM(name)) = LOWER(TRIM($2))")).
		WithArgs("acme", "  ventilator  ").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "name", "product_description", "keywords", "notes", "version", "status",
			"truncated", "truncated_count", "created_at", "updated_at",
		}).AddRow(9, "acme", "Ventilator", "", []byte("[]"), "", 2, "ready", false, 0, time.Now(), time.Now()))

	p, err := store.FindProfileByName(context.Background(), "acme", "  ventilator  ")
	if err != nil {
		t.Fatalf("FindProfileByName: %v", err)
	}
	if p == nil || p.ID != 9 || p.Status != ProfileReady {
		t.Fatalf("p = %+v, want id 9 / ready", p)
	}
}

// Scenario: duplicate with a completed prior run (spec: product-review-intake)
// — offers both "view results" and "re-run" via latest_request_id/latest_run.
func TestDuplicateProfileResponseWithCompletedRun(t *testing.T) {
	store, mock, done := newMockStore(t)
	defer done()
	runs := RunStore{DB: store.DB}

	mock.ExpectQuery(rx("WHERE tenant_id = $1 AND LOWER(TRIM(name)) = LOWER(TRIM($2))")).
		WithArgs("-", "Ventilator").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "name", "product_description", "keywords", "notes", "version", "status",
			"truncated", "truncated_count", "created_at", "updated_at",
		}).AddRow(9, "-", "Ventilator", "", []byte("[]"), "", 1, "ready", false, 0, time.Now(), time.Now()))
	mock.ExpectQuery(rx("FROM kb.product_review_requests")).WithArgs(int64(9)).
		WillReturnRows(requestReturnRow(200, 9, 1))
	mock.ExpectQuery(rx("FROM kb.product_review_runs")).WithArgs(int64(200)).
		WillReturnRows(runGetRow(300, 200, 1, RunCompleted, ""))

	resp, err := duplicateProfileResponse(context.Background(), store, runs, "", "Ventilator")
	if err != nil {
		t.Fatalf("duplicateProfileResponse: %v", err)
	}
	if resp["duplicate"] != true || resp["latest_request_id"] != int64(200) {
		t.Fatalf("resp = %+v, want duplicate=true, latest_request_id=200", resp)
	}
	run, ok := resp["latest_run"].(*Run)
	if !ok || run.ID != 300 {
		t.Fatalf("resp[latest_run] = %+v, want run id 300", resp["latest_run"])
	}
}

// Scenario: duplicate with no completed run yet (spec: product-review-intake)
// — no latest_run key, so the caller can only offer "re-run".
func TestDuplicateProfileResponseNoRunYet(t *testing.T) {
	store, mock, done := newMockStore(t)
	defer done()
	runs := RunStore{DB: store.DB}

	mock.ExpectQuery(rx("WHERE tenant_id = $1 AND LOWER(TRIM(name)) = LOWER(TRIM($2))")).
		WithArgs("-", "Ventilator").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "name", "product_description", "keywords", "notes", "version", "status",
			"truncated", "truncated_count", "created_at", "updated_at",
		}).AddRow(9, "-", "Ventilator", "", []byte("[]"), "", 1, "draft", false, 0, time.Now(), time.Now()))
	mock.ExpectQuery(rx("FROM kb.product_review_requests")).WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "profile_id", "profile_version", "artifact_types",
			"filters", "notes", "requester", "created_at", "updated_at",
		}))

	resp, err := duplicateProfileResponse(context.Background(), store, runs, "", "Ventilator")
	if err != nil {
		t.Fatalf("duplicateProfileResponse: %v", err)
	}
	if resp["duplicate"] != true {
		t.Fatalf("resp = %+v, want duplicate=true", resp)
	}
	if _, has := resp["latest_run"]; has {
		t.Fatalf("resp = %+v, want no latest_run key", resp)
	}
	if _, has := resp["latest_request_id"]; has {
		t.Fatalf("resp = %+v, want no latest_request_id key (no request exists)", resp)
	}
}

// Scenario: first-time product name (spec: product-review-intake) — no
// existing profile, so the caller proceeds to create one.
func TestDuplicateProfileResponseNoMatch(t *testing.T) {
	store, mock, done := newMockStore(t)
	defer done()
	runs := RunStore{DB: store.DB}

	mock.ExpectQuery(rx("WHERE tenant_id = $1 AND LOWER(TRIM(name)) = LOWER(TRIM($2))")).
		WithArgs("-", "Ventilator").
		WillReturnError(sql.ErrNoRows)

	resp, err := duplicateProfileResponse(context.Background(), store, runs, "", "Ventilator")
	if err != nil {
		t.Fatalf("duplicateProfileResponse: %v", err)
	}
	if resp != nil {
		t.Fatalf("resp = %+v, want nil (no existing profile)", resp)
	}
}

// Scenario: listing profiles with mixed run history (spec:
// product-review-history-list) — a profile with a completed run and a
// profile with no request yet both come back, most-recently-updated first,
// each reflecting its own latest run (or lack of one).
func TestListProfilesMixedRunHistory(t *testing.T) {
	store, mock, done := newMockStore(t)
	defer done()

	now := time.Now()
	cols := []string{
		"id", "tenant_id", "name", "product_description", "keywords", "notes", "version", "status",
		"truncated", "truncated_count", "created_at", "updated_at",
		"latest_request_id", "latest_run_id", "latest_run_status", "latest_run_finished_at",
	}
	mock.ExpectQuery(rx("FROM kb.product_profiles p")).
		WithArgs("acme", 50).
		WillReturnRows(sqlmock.NewRows(cols).
			AddRow(9, "acme", "Ventilator", "", []byte(`["icu"]`), "", 2, "ready", false, 0, now, now,
				int64(200), int64(300), RunCompleted, now).
			AddRow(8, "acme", "Drone", "", []byte("[]"), "", 1, "draft", false, 0, now, now,
				nil, nil, nil, nil))

	out, err := store.ListProfiles(context.Background(), "acme", 50)
	if err != nil {
		t.Fatalf("ListProfiles: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("len(out) = %d, want 2", len(out))
	}
	if out[0].ID != 9 || out[0].LatestRequestID == nil || *out[0].LatestRequestID != 200 ||
		out[0].LatestRunID == nil || *out[0].LatestRunID != 300 || out[0].LatestRunStatus != RunCompleted {
		t.Fatalf("out[0] = %+v, want id 9 with latest request 200 / run 300 completed", out[0])
	}
	if len(out[0].Keywords) != 1 || out[0].Keywords[0] != "icu" {
		t.Fatalf("out[0].Keywords = %+v, want [icu]", out[0].Keywords)
	}
	if out[1].ID != 8 || out[1].LatestRequestID != nil || out[1].LatestRunID != nil {
		t.Fatalf("out[1] = %+v, want id 8 with no request/run", out[1])
	}
}

func TestWouldCreateCycle(t *testing.T) {
	one, two := int64(1), int64(2)
	nodes := []scannedNode{
		{ProfileNode: ProfileNode{ID: 1}},
		{ProfileNode: ProfileNode{ID: 2, ParentNodeID: &one}},
		{ProfileNode: ProfileNode{ID: 3, ParentNodeID: &two}},
	}
	cases := []struct {
		node, parent int64
		want         bool
	}{
		{2, 3, true},  // 3 descends from 2
		{2, 2, true},  // self
		{3, 1, false}, // fine
		{2, 1, false}, // unchanged parent
	}
	for _, c := range cases {
		if got := wouldCreateCycle(nodes, c.node, c.parent); got != c.want {
			t.Errorf("wouldCreateCycle(%d, %d) = %v, want %v", c.node, c.parent, got, c.want)
		}
	}
}
