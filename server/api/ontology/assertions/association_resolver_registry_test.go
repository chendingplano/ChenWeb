package assertions

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestMetricAndProvisionResolversSelfRegistered(t *testing.T) {
	if _, ok := LookupAssociationResolver("metric"); !ok {
		t.Fatal("expected the metric resolver to self-register via init()")
	}
	if _, ok := LookupAssociationResolver("provision"); !ok {
		t.Fatal("expected the provision resolver to self-register via init()")
	}
}

func TestRegisteredAssociationResolverTypesIncludesMetricAndProvision(t *testing.T) {
	types := RegisteredAssociationResolverTypes()
	seen := map[string]bool{}
	for _, ty := range types {
		seen[ty] = true
	}
	if !seen["metric"] || !seen["provision"] {
		t.Fatalf("expected metric and provision among registered resolver types, got %v", types)
	}
}

func TestRegisterAssociationResolverRejectsDuplicateType(t *testing.T) {
	resolver := func(ctx context.Context, a AssociateSemantics, dcStore DecisionCandidateStore, asStore AssertionStore, evStore EvidenceStore, dc DecisionCandidate, inputRecordID int64) (string, error) {
		return "deferred", nil
	}
	if err := RegisterAssociationResolver("test_source_type", resolver); err != nil {
		t.Fatalf("first RegisterAssociationResolver: %v", err)
	}
	if err := RegisterAssociationResolver("test_source_type", resolver); err == nil {
		t.Fatal("expected error registering a duplicate source artifact type")
	}
}

func TestRegisterAssociationResolverRequiresFields(t *testing.T) {
	resolver := func(ctx context.Context, a AssociateSemantics, dcStore DecisionCandidateStore, asStore AssertionStore, evStore EvidenceStore, dc DecisionCandidate, inputRecordID int64) (string, error) {
		return "deferred", nil
	}
	if err := RegisterAssociationResolver("", resolver); err == nil {
		t.Fatal("expected error when source artifact type is empty")
	}
	if err := RegisterAssociationResolver("test_source_type_2", nil); err == nil {
		t.Fatal("expected error when resolver is nil")
	}
}

func TestLookupAssociationResolverMissingType(t *testing.T) {
	if _, ok := LookupAssociationResolver("nonexistent_source_type"); ok {
		t.Fatal("expected lookup of an unregistered source artifact type to fail")
	}
}

// TestAssociateSemanticsRunQueriesRegisteredSourceArtifactTypes locks in the
// P3-review fix: Run's candidate query must be driven by
// RegisteredAssociationResolverTypes(), not a hardcoded IN ('metric',
// 'provision') list -- so a third family's resolver is picked up without
// editing this file.
func TestAssociateSemanticsRunQueriesRegisteredSourceArtifactTypes(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("WHERE status = 'deferred' AND source_artifact_type = ANY($1)\n  AND input_record_id = $2")).
		WithArgs(sqlmock.AnyArg(), int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "source_artifact_type", "proposed_payload", "dependency_fingerprint"}))
	mock.ExpectQuery(regexp.QuoteMeta("WHERE status IN ('candidate', 'in_review') AND source_artifact_type = ANY($1)")).
		WithArgs(sqlmock.AnyArg(), int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	a := AssociateSemantics{DB: db}
	report, err := a.Run(context.Background(), 7)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Examined != 0 {
		t.Fatalf("expected zero examined for an empty result set, got %+v", report)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}
