package productreviews

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func ptr[T any](v T) *T { return &v }

// Scenario: Profile created from a product name (spec: product-scope-profile).
func TestCreateProfile(t *testing.T) {
	store, mock, done := newMockStore(t)
	defer done()

	mock.ExpectBegin()
	mock.ExpectQuery(rx("INSERT INTO kb.product_profiles")).
		WithArgs("-", "Ventilator", "").
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
