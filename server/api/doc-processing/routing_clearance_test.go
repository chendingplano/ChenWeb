package docprocessing

import (
	"context"
	"errors"
	"reflect"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestRoutingClearanceIncomparablePipelineRecordsRemovedSet(t *testing.T) {
	delta, checksum, err := BuildNetPlanDelta(
		[]string{"static_analyzer", "chunking", "extract_metrics"},
		[]string{"static_analyzer", "chunking", "extract_products"},
	)
	if err != nil {
		t.Fatalf("BuildNetPlanDelta: %v", err)
	}
	if want := []string{"extract_metrics"}; !reflect.DeepEqual(delta.RemovedProcessors, want) {
		t.Fatalf("removed=%v want=%v", delta.RemovedProcessors, want)
	}
	if !delta.Suppressive || checksum == "" {
		t.Fatalf("delta=%+v checksum=%q", delta, checksum)
	}
	_, again, err := BuildNetPlanDelta(
		[]string{"extract_metrics", "chunking", "static_analyzer"},
		[]string{"extract_products", "static_analyzer", "chunking"},
	)
	if err != nil || checksum != again {
		t.Fatalf("stable checksum=%q again=%q err=%v", checksum, again, err)
	}
}

func TestRoutingClearanceResolveEffectiveRequiresExactSliceAndChecksum(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	subject := RoutingClearanceSubject{PolicyID: 7, PolicyVersion: 3, SubjectKind: "processor_rule", SubjectID: 12, SubjectChecksum: "sha256:subject", DocumentKind: "standard", NetPlanDeltaChecksum: "sha256:delta"}
	query := regexp.QuoteMeta(routingClearanceEffectiveQuery)
	mock.ExpectQuery(query).WithArgs(int64(7), 3, "processor_rule", int64(12), "standard", "sha256:subject", "sha256:delta").WillReturnRows(
		sqlmock.NewRows([]string{"coverage_id", "clearance_id", "approved_by"}).AddRow(int64(22), int64(11), "owner@example.com"),
	)
	got, err := (RoutingClearanceStore{DB: db}).ResolveEffective(context.Background(), subject)
	if err != nil {
		t.Fatalf("ResolveEffective: %v", err)
	}
	if got.ClearanceID != 11 || got.CoverageID != 22 {
		t.Fatalf("got=%+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRoutingClearanceResolveEffectiveRejectsZeroOrMultiple(t *testing.T) {
	for _, tt := range []struct {
		name string
		rows int
		want error
	}{
		{"zero", 0, ErrNoEffectiveRoutingClearance}, {"multiple", 2, ErrMultipleEffectiveRoutingClearances},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			rows := sqlmock.NewRows([]string{"coverage_id", "clearance_id", "approved_by"})
			for i := 0; i < tt.rows; i++ {
				rows.AddRow(int64(i+1), int64(i+10), "owner")
			}
			mock.ExpectQuery(regexp.QuoteMeta(routingClearanceEffectiveQuery)).WithArgs(int64(7), 3, "processor_rule", int64(12), "standard", "sha256:subject", "sha256:delta").WillReturnRows(rows)
			_, err = (RoutingClearanceStore{DB: db}).ResolveEffective(context.Background(), RoutingClearanceSubject{PolicyID: 7, PolicyVersion: 3, SubjectKind: "processor_rule", SubjectID: 12, SubjectChecksum: "sha256:subject", DocumentKind: "standard", NetPlanDeltaChecksum: "sha256:delta"})
			if !errors.Is(err, tt.want) {
				t.Fatalf("err=%v want=%v", err, tt.want)
			}
		})
	}
}
