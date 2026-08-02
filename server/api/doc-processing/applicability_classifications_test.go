package docprocessing

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/deepdoc/server/api/ontology/semrules"
)

func TestClassificationFactLoaderLoadsAcceptedInstanceOfIntoObjectClass(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	stmt := regexp.QuoteMeta(`
SELECT a.id, COALESCE(a.subject_object_id, ''), a.object_ref_id, COALESCE(a.confidence, e.confidence)
FROM kb.semantic_assertions a
JOIN kb.assertion_evidence e ON e.assertion_id = a.id
WHERE e.input_record_id = $1
  AND a.predicate_term_id = 'core:instance_of'
  AND a.object_ref_kind = 'ontology_term'
  AND a.status = 'accepted'
  AND e.deleted = false
ORDER BY a.object_ref_id, a.id`)
	mock.ExpectQuery(stmt).WithArgs(int64(91)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "subject_object_id", "object_ref_id", "confidence"}).
			AddRow(int64(7), "obj-1", "core:device", 0.9).
			AddRow(int64(8), "obj-2", "core:module", 0.7))

	facts, err := (ClassificationFactLoader{DB: db}).LoadObjectClassFacts(context.Background(), 91, 42)
	if err != nil {
		t.Fatalf("LoadObjectClassFacts: %v", err)
	}
	fact := facts["object.class"]
	if fact.State != semrules.FactKnown {
		t.Fatalf("object.class state=%s, want known", fact.State)
	}
	gotValues := fact.Value.([]string)
	if len(gotValues) != 2 || gotValues[0] != "core:device" || gotValues[1] != "core:module" {
		t.Fatalf("object.class=%v", gotValues)
	}
	if fact.Confidence == nil || *fact.Confidence != 0.7 {
		t.Fatalf("confidence=%v, want minimum 0.7", fact.Confidence)
	}
	if fact.ReleaseID != "42" {
		t.Fatalf("release=%q, want 42", fact.ReleaseID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestClassificationFactLoaderReturnsMissingWhenNoAcceptedClasses(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("FROM kb.semantic_assertions").WithArgs(int64(91)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "subject_object_id", "object_ref_id", "confidence"}))

	facts, err := (ClassificationFactLoader{DB: db}).LoadObjectClassFacts(context.Background(), 91, 42)
	if err != nil {
		t.Fatalf("LoadObjectClassFacts: %v", err)
	}
	if facts["object.class"].State != semrules.FactMissing {
		t.Fatalf("object.class=%+v, want missing", facts["object.class"])
	}
}

func TestDeploymentFactsAndDuplicateKnownProducerRejection(t *testing.T) {
	facts, err := BuildDeploymentFacts(DeploymentFactContext{
		Workspace:      "workspace-a",
		Tenant:         "tenant-a",
		KnowledgeStore: "ks-42",
		User:           "user-a",
		Corpus:         "benchmark",
	})
	if err != nil {
		t.Fatalf("BuildDeploymentFacts: %v", err)
	}
	if facts["deployment.tenant"].Value != "tenant-a" || facts["deployment.knowledge_store"].Value != "ks-42" {
		t.Fatalf("deployment facts=%+v", facts)
	}

	_, err = MergeApplicabilityFactSets(
		facts,
		semrules.FactSet{"deployment.tenant": {Path: "deployment.tenant", State: semrules.FactKnown, Value: "tenant-b"}},
	)
	if err == nil {
		t.Fatal("duplicate known deployment.tenant producer succeeded")
	}
}
