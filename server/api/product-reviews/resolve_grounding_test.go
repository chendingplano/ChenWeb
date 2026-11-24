package productreviews

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestResolveGroundingRejectsInvalidInput(t *testing.T) {
	for _, input := range []ResolveGroundingInput{{}, {Create: true}, {Create: true, Name: "Pump", ObjectID: "obj_1"}} {
		if _, err := (Store{}).ResolveGrounding(context.Background(), 1, 2, input); !errors.Is(err, errGroundingInput) {
			t.Fatalf("input %+v: %v", input, err)
		}
	}
}

func TestResolveGroundingDoesNotLinkNodeFromAnotherProfile(t *testing.T) {
	store, mock, done := newMockStore(t)
	defer done()
	mock.ExpectBegin()
	mock.ExpectQuery(`WHERE profile_id = \$1 AND id = \$2 FOR UPDATE`).WithArgs(int64(99), int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"kind", "grounding", "status"}))
	mock.ExpectRollback()
	_, err := store.ResolveGrounding(context.Background(), 99, 2, ResolveGroundingInput{ObjectID: "obj_1"})
	if !errors.Is(err, ErrNodeNotFound) {
		t.Fatalf("got %v, want missing node", err)
	}
}

func TestResolveGrounding(t *testing.T) {
	for _, tc := range []struct {
		name, kind, grounding, status, objectStatus string
		create, duplicate, updateFails              bool
		wantErr                                     error
	}{
		{name: "link existing", kind: KindPart, grounding: GroundingUngrounded, status: StatusAccepted, objectStatus: "active"},
		{name: "create and link", kind: KindModule, grounding: GroundingUngrounded, status: StatusAccepted, objectStatus: "active", create: true},
		{name: "duplicate", kind: KindPart, grounding: GroundingUngrounded, status: StatusAccepted, create: true, duplicate: true, wantErr: errGroundingDuplicate},
		{name: "already grounded", kind: KindPart, grounding: GroundingObjectNode, wantErr: errGroundingConflict},
		{name: "aspect excluded", kind: KindAspect, grounding: GroundingUngrounded, wantErr: errGroundingConflict},
		{name: "rejected node", kind: KindModule, grounding: GroundingUngrounded, status: StatusRejected, wantErr: errGroundingConflict},
		{name: "ambiguous object", kind: KindPart, grounding: GroundingUngrounded, objectStatus: "ambiguous", wantErr: errGroundingObject},
		{name: "merged object", kind: KindPart, grounding: GroundingUngrounded, objectStatus: "merged", wantErr: errGroundingObject},
		{name: "rollback new object if linking fails", kind: KindPart, grounding: GroundingUngrounded, objectStatus: "active", create: true, updateFails: true, wantErr: errors.New("update failed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, mock, done := newMockStore(t)
			defer done()
			mock.ExpectBegin()
			mock.ExpectQuery("FROM kb.product_profile_nodes WHERE profile_id").WithArgs(int64(1), int64(2)).WillReturnRows(sqlmock.NewRows([]string{"kind", "grounding", "status"}).AddRow(tc.kind, tc.grounding, tc.status))
			if tc.wantErr != errGroundingConflict {
				if tc.create {
					mock.ExpectExec("pg_advisory_xact_lock").WithArgs("pump").WillReturnResult(sqlmock.NewResult(0, 1))
					mock.ExpectQuery("SELECT EXISTS").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(tc.duplicate))
					if !tc.duplicate {
						mock.ExpectQuery("INSERT INTO kb.object_nodes").WillReturnRows(sqlmock.NewRows([]string{"object_id"}).AddRow("obj_1"))
					}
				}
				if !tc.duplicate {
					mock.ExpectQuery("FROM kb.object_nodes.*FOR SHARE").WithArgs("obj_1").WillReturnRows(sqlmock.NewRows([]string{"reconcile"}).AddRow(tc.objectStatus))
					if tc.wantErr != errGroundingObject {
						update := mock.ExpectExec("UPDATE kb.product_profile_nodes").WithArgs(int64(1), int64(2), "obj_1", tc.objectStatus)
						if tc.updateFails {
							update.WillReturnError(tc.wantErr)
						} else {
							update.WillReturnResult(sqlmock.NewResult(0, 1))
						}
					}
				}
			}
			if tc.wantErr != nil {
				mock.ExpectRollback()
			} else {
				mock.ExpectCommit()
			}
			input := ResolveGroundingInput{ObjectID: "obj_1"}
			if tc.create {
				input = ResolveGroundingInput{Create: true, Name: "Pump"}
			}
			id, err := store.ResolveGrounding(context.Background(), 1, 2, input)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("got %v, want %v", err, tc.wantErr)
			}
			if err == nil && id != "obj_1" {
				t.Fatalf("object ID = %q", id)
			}
		})
	}
}
