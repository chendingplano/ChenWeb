package classfoundation

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestContractStoreCreatesIdentityOnlyClassWithStableTermID(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO kb.ontology_term_headers")).
		WithArgs("metric:amount", "metrics", "resolver").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_class_contract_revisions")).
		WithArgs("metric:amount", "contract/v1", "identity/v1", "identity_only", "{}", "deterministic_resolution", nil, nil, "{}", "resolver").
		WillReturnRows(sqlmock.NewRows([]string{"id", "term_id", "revision", "definition_state", "create_time"}).
			AddRow(int64(42), "metric:amount", 1, "identity_only", time.Now()))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.ontology_term_headers")).
		WithArgs(int64(42), "metric:amount").
		WillReturnResult(sqlmock.NewResult(0, 1))

	store := ContractStore{DB: db}
	got, err := store.CreateIdentityOnlyClass(context.Background(), ClassIdentity{
		TermID: "metric:amount", ModuleID: "metrics", By: "resolver",
	})
	if err != nil {
		t.Fatalf("CreateIdentityOnlyClass: %v", err)
	}
	if got.ID != 42 || got.TermID != "metric:amount" || got.Revision != 1 || got.DefinitionState != DefinitionIdentityOnly {
		t.Fatalf("unexpected contract: %#v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestContractStoreRejectsUnstableContractInput(t *testing.T) {
	store := ContractStore{}
	if _, err := store.AppendContractRevision(context.Background(), ContractRevision{TermID: "", DefinitionState: DefinitionIdentityOnly}); err == nil {
		t.Fatal("expected missing term ID error")
	}
	if _, err := store.AppendContractRevision(context.Background(), ContractRevision{TermID: "metric:amount", DefinitionState: "unknown"}); err == nil {
		t.Fatal("expected unknown definition state error")
	}
}
