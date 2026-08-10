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

func TestRoutingClearanceReplaceRevokesPriorGenerationTransactionally(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	request := RoutingClearanceApproval{Subject: RoutingClearanceSubject{PipelineName: "narrative_default", PipelineVersion: 3, SubjectKind: "processor_rule", SubjectID: 12, SubjectChecksum: "sha256:subject", DocumentKind: "standard", NetPlanDeltaChecksum: "sha256:delta"}, PolicyChecksum: "sha256:policy", ManifestChecksum: "sha256:manifest", BaselineRunID: "00000000-0000-0000-0000-000000000001", RoutedRunID: "00000000-0000-0000-0000-000000000002", Repetitions: 1, PairedCaseCount: 3, GoldPositiveDenominator: 3, BaselineRecallNumerator: 3, BaselineRecallDenominator: 3, RoutedRecallNumerator: 3, RoutedRecallDenominator: 3, Approved: true, Actor: "owner@example.com", Rationale: "replacement evidence"}
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WithArgs(sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT cv.clearance_id").WithArgs("narrative_default", 3, "processor_rule", int64(12), "standard").WillReturnRows(sqlmock.NewRows([]string{"clearance_id"}).AddRow(int64(9)))
	mock.ExpectExec("INSERT INTO kb.pipeline_routing_clearance_revocations").WithArgs(int64(9), "owner@example.com", "replaced: replacement evidence").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("INSERT INTO kb.pipeline_routing_clearances").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(11)))
	mock.ExpectExec("INSERT INTO kb.pipeline_routing_clearance_coverage").WithArgs(int64(11), "narrative_default", 3, "processor_rule", int64(12), "sha256:subject", "standard", "sha256:delta", int64(9)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	got, err := (RoutingClearanceStore{DB: db}).Replace(context.Background(), request)
	if err != nil || got != 11 {
		t.Fatalf("id=%d err=%v", got, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRoutingClearanceRevokeIsAppendOnly(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WithArgs("clearance/11").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO kb.pipeline_routing_clearance_revocations").WithArgs(int64(11), "owner@example.com", "bad evidence").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := (RoutingClearanceStore{DB: db}).Revoke(context.Background(), 11, "owner@example.com", "bad evidence"); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRoutingClearanceResolveEffectiveRequiresExactSliceAndChecksum(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	subject := RoutingClearanceSubject{PipelineName: "narrative_default", PipelineVersion: 3, SubjectKind: "processor_rule", SubjectID: 12, SubjectChecksum: "sha256:subject", DocumentKind: "standard", NetPlanDeltaChecksum: "sha256:delta"}
	query := regexp.QuoteMeta(routingClearanceEffectiveQuery)
	mock.ExpectQuery(query).WithArgs("narrative_default", 3, "processor_rule", int64(12), "standard", "sha256:subject", "sha256:delta").WillReturnRows(
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
			mock.ExpectQuery(regexp.QuoteMeta(routingClearanceEffectiveQuery)).WithArgs("narrative_default", 3, "processor_rule", int64(12), "standard", "sha256:subject", "sha256:delta").WillReturnRows(rows)
			_, err = (RoutingClearanceStore{DB: db}).ResolveEffective(context.Background(), RoutingClearanceSubject{PipelineName: "narrative_default", PipelineVersion: 3, SubjectKind: "processor_rule", SubjectID: 12, SubjectChecksum: "sha256:subject", DocumentKind: "standard", NetPlanDeltaChecksum: "sha256:delta"})
			if !errors.Is(err, tt.want) {
				t.Fatalf("err=%v want=%v", err, tt.want)
			}
		})
	}
}
