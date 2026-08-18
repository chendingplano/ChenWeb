package classfoundation

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestSerializeCanonicalClaimKeyIsStableAndExcludesEvidence(t *testing.T) {
	key, err := SerializeCanonicalClaimKey(CanonicalClaimInput{
		KeyVersion:  "identity/v1",
		ClassTermID: "metric:temperature",
		Fields: map[string]string{
			"unit":  "unit:celsius",
			"value": "25",
		},
		Evidence: `{"source":"must not appear"}`,
	})
	if err != nil {
		t.Fatalf("SerializeCanonicalClaimKey: %v", err)
	}
	if got, want := string(key), `{"class_term_id":"metric:temperature","fields":{"unit":"unit:celsius","value":"25"},"key_version":"identity/v1"}`; got != want {
		t.Fatalf("key = %s, want %s", got, want)
	}
}

func TestClaimIdentityStoreConcurrentConvergenceReturnsExistingClaim(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	key := []byte(`{"class_term_id":"metric:temperature","fields":{"value":"25"},"key_version":"identity/v1"}`)
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.semantic_claim_identities")).
		WithArgs("claim-racing", "identity/v1", key, "metric:temperature", `{"value":"25"}`, nil).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT claim_id, identity_payload FROM kb.semantic_claim_identities")).
		WithArgs("identity/v1", key).
		WillReturnRows(sqlmock.NewRows([]string{"claim_id", "identity_payload"}).AddRow("claim-existing", `{"value":"25"}`))

	claimID, created, err := (ClaimIdentityStore{DB: db, NewClaimID: func() string { return "claim-racing" }}).FindOrCreateShadow(context.Background(), CanonicalClaimInput{
		KeyVersion: "identity/v1", ClassTermID: "metric:temperature", Fields: map[string]string{"value": "25"},
	})
	if err != nil || created || claimID != "claim-existing" {
		t.Fatalf("convergence = claim %q created=%v err=%v", claimID, created, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestClaimIdentityStoreReportsCollisionAndEvidenceOnlyChangesConverge(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	key := []byte(`{"class_term_id":"metric:temperature","fields":{"value":"25"},"key_version":"identity/v1"}`)
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.semantic_claim_identities")).
		WithArgs("claim-collision", "identity/v1", key, "metric:temperature", `{"value":"25"}`, nil).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT claim_id, identity_payload FROM kb.semantic_claim_identities")).
		WithArgs("identity/v1", key).
		WillReturnRows(sqlmock.NewRows([]string{"claim_id", "identity_payload"}).AddRow("claim-existing", `{"value":"30"}`))

	_, _, err = (ClaimIdentityStore{DB: db, NewClaimID: func() string { return "claim-collision" }}).FindOrCreateShadow(context.Background(), CanonicalClaimInput{
		KeyVersion: "identity/v1", ClassTermID: "metric:temperature", Fields: map[string]string{"value": "25"}, Evidence: `{"new":"evidence"}`,
	})
	if !errors.Is(err, ErrClaimIdentityCollision) {
		t.Fatalf("collision error = %v, want ErrClaimIdentityCollision", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}

	left, _ := SerializeCanonicalClaimKey(CanonicalClaimInput{KeyVersion: "identity/v1", ClassTermID: "metric:temperature", Fields: map[string]string{"value": "25"}, Evidence: `{"a":1}`})
	right, _ := SerializeCanonicalClaimKey(CanonicalClaimInput{KeyVersion: "identity/v1", ClassTermID: "metric:temperature", Fields: map[string]string{"value": "25"}, Evidence: `{"b":2}`})
	different, _ := SerializeCanonicalClaimKey(CanonicalClaimInput{KeyVersion: "identity/v1", ClassTermID: "metric:temperature", Fields: map[string]string{"value": "30"}})
	if string(left) != string(right) || string(left) == string(different) {
		t.Fatalf("evidence-only keys must converge and identity-bearing change must diverge")
	}
}

func TestClaimIdentityStoreFindOrCreatesShadowClaimWithoutMetricWriter(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	key := []byte(`{"class_term_id":"metric:temperature","fields":{"value":"25"},"key_version":"identity/v1"}`)
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.semantic_claim_identities")).
		WithArgs("claim-test", "identity/v1", key, "metric:temperature", `{"value":"25"}`, "shadow").
		WillReturnRows(sqlmock.NewRows([]string{"claim_id"}).AddRow("claim-test"))

	store := ClaimIdentityStore{DB: db, NewClaimID: func() string { return "claim-test" }}
	claimID, created, err := store.FindOrCreateShadow(context.Background(), CanonicalClaimInput{
		KeyVersion: "identity/v1", ClassTermID: "metric:temperature", Fields: map[string]string{"value": "25"}, By: "shadow",
	})
	if err != nil {
		t.Fatalf("FindOrCreateShadow: %v", err)
	}
	if claimID != "claim-test" || !created {
		t.Fatalf("claim result = %q created=%v", claimID, created)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
