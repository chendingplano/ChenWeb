package docreviews

import (
	"context"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestLoadObjectAnchoredPeerIDsExactAndComparableObjects(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("FROM kb.artifact_objects").
		WithArgs(int64(1), "metric", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"artifact_id", "object_id"}).
			AddRow("1_m_1", "obj_pipe"))

	mock.ExpectQuery("FROM kb.object_nodes").
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"object_id", "object_type", "normalized_names"}).
			AddRow("obj_pipe", "equipment", `["pipe system"]`))

	mock.ExpectQuery("FROM kb.object_nodes").
		WithArgs("equipment", "obj_pipe", sqlmock.AnyArg(), objectAnchorComparableLimit).
		WillReturnRows(sqlmock.NewRows([]string{"object_id"}).
			AddRow("obj_pipe_alias"))

	mock.ExpectQuery("FROM kb.artifact_connections").
		WithArgs("metric", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"target_id", "artifact_ids"}).
			AddRow("obj_pipe", `["1_m_1","2_m_9"]`).
			AddRow("obj_pipe_alias", `["3_m_7"]`))

	got, err := loadObjectAnchoredPeerIDs(context.Background(), db, 1, "metric", []string{"1_m_1"})
	if err != nil {
		t.Fatalf("loadObjectAnchoredPeerIDs err = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1 (%v)", len(got), got)
	}
	if strings.Join(got[0], ",") != "1_m_1,2_m_9,3_m_7" {
		t.Fatalf("got[0] = %v, want [1_m_1 2_m_9 3_m_7]", got[0])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("mock: %v", err)
	}
}
