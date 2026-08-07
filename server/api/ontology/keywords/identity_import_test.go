package keywords

import (
	"context"
	"database/sql"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"

	"github.com/chendingplano/deepdoc/server/api/ontology/semid"
)

func validPromotionFile() ReviewedPromotionFile {
	reviewedAt := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	return ReviewedPromotionFile{
		SchemaVersion: 1, Source: "iec-60050-845", Release: "2020", Scope: "display",
		DeploymentKey: "tier6-primary",
		Promotions: []PositivePromotion{{
			ExternalID: "845-21-050", ConceptID: "kw:luminance", Relation: "exact_equivalent",
			UnitConstraints: []string{"cd/m2"}, DimensionConstraints: []string{"J.L-2"},
			Surfaces: []PromotedSurface{{SurfaceID: "kws_luminance", Evidence: "reviewed exact mapping"}},
			Reviewer: "domain-board", ReviewedAt: reviewedAt,
			ProvenanceLocator: "https://example.test/promotion/845-21-050",
		}},
		NegativePromotions: []NegativePromotion{{
			NodeA: "kw:luminance", NodeB: "kw:brightness", Scope: "display",
			Reason: "luminance is photometric density; brightness is a perceived attribute",
			Triples: []PromotionTriple{{
				Source: "iec-60050-845", Release: "2020",
				SubjectExternalID: "845-21-050", ObjectExternalID: "845-22-059",
				Relation: "different_from", ProvenanceLocator: "https://example.test/review/1",
			}},
			Reviewer: "domain-board", ReviewedAt: reviewedAt,
			ProvenanceLocator: "https://example.test/negative/1",
		}},
	}
}

func expectPositivePromotionValidation(mock sqlmock.Sqlmock, file ReviewedPromotionFile, promotion PositivePromotion, evidenceConcepts ...string) {
	mock.ExpectQuery(regexp.QuoteMeta(promotionConceptSQL)).
		WithArgs(promotion.ConceptID).
		WillReturnRows(sqlmock.NewRows([]string{"scope", "status"}).AddRow(file.Scope, "active"))
	mock.ExpectQuery(regexp.QuoteMeta(promotionPolicySQL)).
		WithArgs(file.Source, file.Release).
		WillReturnRows(sqlmock.NewRows([]string{"authority_role", "license_review_status", "authoritative_relations", "allowed_scopes", "identity_authority"}).
			AddRow("exact_identity_authority", "approved", "{exact_equivalent}", "{display}", true))
	mock.ExpectQuery(regexp.QuoteMeta(promotionDeploymentSQL)).
		WithArgs(file.DeploymentKey).
		WillReturnRows(sqlmock.NewRows([]string{"source", "release", "enabled"}).AddRow(file.Source, file.Release, true))
	mock.ExpectQuery(regexp.QuoteMeta(promotionStagingEntrySQL)).
		WithArgs(file.Source, file.Release, promotion.ExternalID).
		WillReturnRows(sqlmock.NewRows([]string{"native_payload"}).
			AddRow([]byte(`{"unit_constraints":["cd/m2"],"dimension_constraints":["J.L-2"]}`)))
	mock.ExpectQuery(regexp.QuoteMeta(promotionExistingMappingSQL)).
		WithArgs(file.Source, promotion.ExternalID, file.Release).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(regexp.QuoteMeta(promotionNegativeEvidenceSQL)).
		WithArgs(file.Source, file.Release, promotion.ExternalID).
		WillReturnRows(sqlmock.NewRows([]string{"subject_external_id", "object_external_id"}).
			AddRow("845-21-050", "845-22-059"))
	mock.ExpectQuery(regexp.QuoteMeta(promotionNegativeMappingSQL)).
		WithArgs(file.Source, "845-22-059", file.Release, promotion.ConceptID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(regexp.QuoteMeta(promotionSurfaceSQL)).
		WithArgs("kws_luminance").
		WillReturnRows(sqlmock.NewRows([]string{"concept_id"}).AddRow(promotion.ConceptID))
	surfaceConceptRows := sqlmock.NewRows([]string{"concept_id"})
	for _, concept := range evidenceConcepts {
		surfaceConceptRows.AddRow(concept)
	}
	mock.ExpectQuery(regexp.QuoteMeta(promotionSurfaceEvidenceConceptsSQL)).
		WithArgs(pq.Array([]string{"kws_luminance"}), promotion.ConceptID).
		WillReturnRows(surfaceConceptRows)
}

func expectNegativePromotionValidation(mock sqlmock.Sqlmock, file ReviewedPromotionFile, negative NegativePromotion) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT allowed_scopes FROM kb.keyword_sources WHERE source = $1 AND release = $2`)).
		WithArgs(file.Source, file.Release).
		WillReturnRows(sqlmock.NewRows([]string{"allowed_scopes"}).AddRow("{display}"))
	for _, node := range []string{negative.NodeA, negative.NodeB} {
		mock.ExpectQuery(regexp.QuoteMeta(promotionConceptSQL)).
			WithArgs(node).
			WillReturnRows(sqlmock.NewRows([]string{"scope", "status"}).AddRow(file.Scope, "active"))
	}
	for _, triple := range negative.Triples {
		mock.ExpectQuery(regexp.QuoteMeta(promotionPolicySQL)).
			WithArgs(triple.Source, triple.Release).
			WillReturnRows(sqlmock.NewRows([]string{"authority_role", "license_review_status", "authoritative_relations", "allowed_scopes", "identity_authority"}).
				AddRow("exact_identity_authority", "approved", "{exact_equivalent}", "{display}", true))
		mock.ExpectQuery(regexp.QuoteMeta(promotionNegativeDecisionSQL)).
			WithArgs(triple.Source, triple.Release, triple.SubjectExternalID, triple.ObjectExternalID, triple.Relation).
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	}
}

func TestPromotionAppliesFullTripleBatch(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	file := validPromotionFile()

	mock.ExpectBegin()
	expectKeywordIdentityLock(mock)
	expectPositivePromotionValidation(mock, file, file.Promotions[0])
	mock.ExpectExec(regexp.QuoteMeta(promotionExternalInsertSQL)).
		WithArgs(file.Source, "845-21-050", file.Release, "kw:luminance").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(promotionEvidenceInsertSQL)).
		WithArgs("kws_luminance", file.Source, "845-21-050", file.Release, "reviewed exact mapping").
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectNegativePromotionValidation(mock, file, file.NegativePromotions[0])
	expectKeywordIdentityLock(mock)
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO kb.semid_never_merge")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	result, err := (PromotionStore{DB: db}).ApplyReviewedPromotion(context.Background(), file)
	if err != nil {
		t.Fatalf("ApplyReviewedPromotion: %v", err)
	}
	if result.ExternalIDs != 1 || result.SurfaceEvidence != 1 || result.NeverMerges != 1 {
		t.Fatalf("result=%+v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPromotionFullTripleReplayIsIdempotent(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	file := validPromotionFile()

	for range 2 {
		mock.ExpectBegin()
		expectKeywordIdentityLock(mock)
		expectPositivePromotionValidation(mock, file, file.Promotions[0])
		mock.ExpectExec(regexp.QuoteMeta(promotionExternalInsertSQL)).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(regexp.QuoteMeta(promotionEvidenceInsertSQL)).
			WillReturnResult(sqlmock.NewResult(0, 0))
		expectNegativePromotionValidation(mock, file, file.NegativePromotions[0])
		expectKeywordIdentityLock(mock)
		mock.ExpectExec(regexp.QuoteMeta("INSERT INTO kb.semid_never_merge")).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit()
	}

	ctx := context.Background()
	first, err := (PromotionStore{DB: db}).ApplyReviewedPromotion(ctx, file)
	if err != nil {
		t.Fatal(err)
	}
	second, err := (PromotionStore{DB: db}).ApplyReviewedPromotion(ctx, file)
	if err != nil {
		t.Fatal(err)
	}
	if first.ExternalIDs != second.ExternalIDs || first.SurfaceEvidence != second.SurfaceEvidence || first.NeverMerges != second.NeverMerges {
		t.Fatalf("replay counts differ: first=%+v second=%+v", first, second)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPromotionRejectsUnknownConcept(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	file := validPromotionFile()

	mock.ExpectBegin()
	expectKeywordIdentityLock(mock)
	mock.ExpectQuery(regexp.QuoteMeta(promotionConceptSQL)).
		WithArgs("kw:luminance").WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	_, err = (PromotionStore{DB: db}).ApplyReviewedPromotion(context.Background(), file)
	if err == nil || !strings.Contains(err.Error(), `unknown concept "kw:luminance"`) {
		t.Fatalf("error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPromotionRejectsProvisionalTarget(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	file := validPromotionFile()

	mock.ExpectBegin()
	expectKeywordIdentityLock(mock)
	mock.ExpectQuery(regexp.QuoteMeta(promotionConceptSQL)).
		WithArgs("kw:luminance").
		WillReturnRows(sqlmock.NewRows([]string{"scope", "status"}).AddRow("display", "provisional"))
	mock.ExpectRollback()

	_, err = (PromotionStore{DB: db}).ApplyReviewedPromotion(context.Background(), file)
	if err == nil || !strings.Contains(err.Error(), "must be active") {
		t.Fatalf("error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPromotionRejectsScopeMismatch(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	file := validPromotionFile()

	mock.ExpectBegin()
	expectKeywordIdentityLock(mock)
	mock.ExpectQuery(regexp.QuoteMeta(promotionConceptSQL)).
		WithArgs("kw:luminance").
		WillReturnRows(sqlmock.NewRows([]string{"scope", "status"}).AddRow("clinical", "active"))
	mock.ExpectRollback()

	_, err = (PromotionStore{DB: db}).ApplyReviewedPromotion(context.Background(), file)
	if err == nil || !strings.Contains(err.Error(), "scope mismatch") {
		t.Fatalf("error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPromotionRejectsNonAuthoritativePolicy(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	file := validPromotionFile()

	mock.ExpectBegin()
	expectKeywordIdentityLock(mock)
	mock.ExpectQuery(regexp.QuoteMeta(promotionConceptSQL)).
		WithArgs("kw:luminance").
		WillReturnRows(sqlmock.NewRows([]string{"scope", "status"}).AddRow("display", "active"))
	mock.ExpectQuery(regexp.QuoteMeta(promotionPolicySQL)).
		WithArgs(file.Source, file.Release).
		WillReturnRows(sqlmock.NewRows([]string{"authority_role", "license_review_status", "authoritative_relations", "allowed_scopes", "identity_authority"}).
			AddRow("proposal_only", "approved", "{}", "{display}", false))
	mock.ExpectRollback()

	_, err = (PromotionStore{DB: db}).ApplyReviewedPromotion(context.Background(), file)
	if err == nil || !strings.Contains(err.Error(), "not an exact identity authority") {
		t.Fatalf("error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPromotionRejectsNonExactRelationBeforeTransaction(t *testing.T) {
	file := validPromotionFile()
	file.Promotions[0].Relation = "related"
	if _, err := (PromotionStore{DB: nil}).ApplyReviewedPromotion(context.Background(), file); err == nil ||
		!strings.Contains(err.Error(), "cannot be promoted as authoritative evidence") {
		t.Fatalf("error = %v", err)
	}
}

func TestPromotionRejectsConflictingUnitConstraint(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	file := validPromotionFile()

	mock.ExpectBegin()
	expectKeywordIdentityLock(mock)
	mock.ExpectQuery(regexp.QuoteMeta(promotionConceptSQL)).
		WithArgs("kw:luminance").
		WillReturnRows(sqlmock.NewRows([]string{"scope", "status"}).AddRow("display", "active"))
	mock.ExpectQuery(regexp.QuoteMeta(promotionPolicySQL)).
		WithArgs(file.Source, file.Release).
		WillReturnRows(sqlmock.NewRows([]string{"authority_role", "license_review_status", "authoritative_relations", "allowed_scopes", "identity_authority"}).
			AddRow("exact_identity_authority", "approved", "{exact_equivalent}", "{display}", true))
	mock.ExpectQuery(regexp.QuoteMeta(promotionDeploymentSQL)).
		WithArgs(file.DeploymentKey).
		WillReturnRows(sqlmock.NewRows([]string{"source", "release", "enabled"}).AddRow(file.Source, file.Release, true))
	mock.ExpectQuery(regexp.QuoteMeta(promotionStagingEntrySQL)).
		WithArgs(file.Source, file.Release, "845-21-050").
		WillReturnRows(sqlmock.NewRows([]string{"native_payload"}).
			AddRow([]byte(`{"unit_constraints":["m"],"dimension_constraints":["J.L-2"]}`)))
	mock.ExpectRollback()

	_, err = (PromotionStore{DB: db}).ApplyReviewedPromotion(context.Background(), file)
	if err == nil || !strings.Contains(err.Error(), "unit constraint mismatch") {
		t.Fatalf("error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPromotionRejectsConflictingAlignment(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	file := validPromotionFile()
	promotion := file.Promotions[0]

	mock.ExpectBegin()
	expectKeywordIdentityLock(mock)
	mock.ExpectQuery(regexp.QuoteMeta(promotionConceptSQL)).
		WithArgs(promotion.ConceptID).
		WillReturnRows(sqlmock.NewRows([]string{"scope", "status"}).AddRow(file.Scope, "active"))
	mock.ExpectQuery(regexp.QuoteMeta(promotionPolicySQL)).
		WithArgs(file.Source, file.Release).
		WillReturnRows(sqlmock.NewRows([]string{"authority_role", "license_review_status", "authoritative_relations", "allowed_scopes", "identity_authority"}).
			AddRow("exact_identity_authority", "approved", "{exact_equivalent}", "{display}", true))
	mock.ExpectQuery(regexp.QuoteMeta(promotionDeploymentSQL)).
		WithArgs(file.DeploymentKey).
		WillReturnRows(sqlmock.NewRows([]string{"source", "release", "enabled"}).AddRow(file.Source, file.Release, true))
	mock.ExpectQuery(regexp.QuoteMeta(promotionStagingEntrySQL)).
		WithArgs(file.Source, file.Release, promotion.ExternalID).
		WillReturnRows(sqlmock.NewRows([]string{"native_payload"}).
			AddRow([]byte(`{"unit_constraints":["cd/m2"],"dimension_constraints":["J.L-2"]}`)))
	// The mapping already exists to a different concept; the identity is immutable.
	mock.ExpectQuery(regexp.QuoteMeta(promotionExistingMappingSQL)).
		WithArgs(file.Source, "845-21-050", file.Release).
		WillReturnRows(sqlmock.NewRows([]string{"concept_id"}).AddRow("kw:other"))
	mock.ExpectRollback()

	_, err = (PromotionStore{DB: db}).ApplyReviewedPromotion(context.Background(), file)
	if err == nil || !strings.Contains(err.Error(), "already maps to") {
		t.Fatalf("error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPromotionRejectsAuthoritativeNonEquivalence(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	file := validPromotionFile()

	mock.ExpectBegin()
	expectKeywordIdentityLock(mock)
	mock.ExpectQuery(regexp.QuoteMeta(promotionConceptSQL)).
		WithArgs("kw:luminance").
		WillReturnRows(sqlmock.NewRows([]string{"scope", "status"}).AddRow("display", "active"))
	mock.ExpectQuery(regexp.QuoteMeta(promotionPolicySQL)).
		WithArgs(file.Source, file.Release).
		WillReturnRows(sqlmock.NewRows([]string{"authority_role", "license_review_status", "authoritative_relations", "allowed_scopes", "identity_authority"}).
			AddRow("exact_identity_authority", "approved", "{exact_equivalent}", "{display}", true))
	mock.ExpectQuery(regexp.QuoteMeta(promotionDeploymentSQL)).
		WithArgs(file.DeploymentKey).
		WillReturnRows(sqlmock.NewRows([]string{"source", "release", "enabled"}).AddRow(file.Source, file.Release, true))
	mock.ExpectQuery(regexp.QuoteMeta(promotionStagingEntrySQL)).
		WithArgs(file.Source, file.Release, "845-21-050").
		WillReturnRows(sqlmock.NewRows([]string{"native_payload"}).
			AddRow([]byte(`{"unit_constraints":["cd/m2"],"dimension_constraints":["J.L-2"]}`)))
	mock.ExpectQuery(regexp.QuoteMeta(promotionExistingMappingSQL)).
		WithArgs(file.Source, "845-21-050", file.Release).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(regexp.QuoteMeta(promotionNegativeEvidenceSQL)).
		WithArgs(file.Source, file.Release, "845-21-050").
		WillReturnRows(sqlmock.NewRows([]string{"subject_external_id", "object_external_id"}).
			AddRow("845-21-050", "845-22-059"))
	mock.ExpectQuery(regexp.QuoteMeta(promotionNegativeMappingSQL)).
		WithArgs(file.Source, "845-22-059", file.Release, "kw:luminance").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectRollback()

	_, err = (PromotionStore{DB: db}).ApplyReviewedPromotion(context.Background(), file)
	if err == nil || !strings.Contains(err.Error(), "authoritative non-equivalence") {
		t.Fatalf("error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestNegativePromotionRejectsIncompleteProvenance(t *testing.T) {
	file := validPromotionFile()
	file.NegativePromotions[0].Triples[0].ProvenanceLocator = ""
	if _, err := (PromotionStore{DB: nil}).ApplyReviewedPromotion(context.Background(), file); err == nil ||
		!strings.Contains(err.Error(), "full provenance") {
		t.Fatalf("error = %v", err)
	}

	file = validPromotionFile()
	file.NegativePromotions[0].Scope = ""
	if _, err := (PromotionStore{DB: nil}).ApplyReviewedPromotion(context.Background(), file); err == nil ||
		!strings.Contains(err.Error(), "node_a, node_b, and scope") {
		t.Fatalf("error = %v", err)
	}

	file = validPromotionFile()
	file.NegativePromotions[0].NodeB = file.NegativePromotions[0].NodeA
	if _, err := (PromotionStore{DB: nil}).ApplyReviewedPromotion(context.Background(), file); err == nil ||
		!strings.Contains(err.Error(), "must be distinct") {
		t.Fatalf("error = %v", err)
	}
}

func TestNegativePromotionRejectsProposalOnlyDistinction(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	file := validPromotionFile()
	file.Promotions = nil

	mock.ExpectBegin()
	expectKeywordIdentityLock(mock)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT allowed_scopes FROM kb.keyword_sources WHERE source = $1 AND release = $2`)).
		WithArgs(file.Source, file.Release).
		WillReturnRows(sqlmock.NewRows([]string{"allowed_scopes"}).AddRow("{display}"))
	for _, node := range []string{file.NegativePromotions[0].NodeA, file.NegativePromotions[0].NodeB} {
		mock.ExpectQuery(regexp.QuoteMeta(promotionConceptSQL)).
			WithArgs(node).
			WillReturnRows(sqlmock.NewRows([]string{"scope", "status"}).AddRow(file.Scope, "active"))
	}
	mock.ExpectQuery(regexp.QuoteMeta(promotionPolicySQL)).
		WithArgs("iec-60050-845", "2020").
		WillReturnRows(sqlmock.NewRows([]string{"authority_role", "license_review_status", "authoritative_relations", "allowed_scopes", "identity_authority"}).
			AddRow("proposal_only", "approved", "{}", "{display}", false))
	mock.ExpectRollback()

	_, err = (PromotionStore{DB: db}).ApplyReviewedPromotion(context.Background(), file)
	if err == nil || !strings.Contains(err.Error(), "proposal-only distinctions cannot veto") {
		t.Fatalf("error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestNegativePromotionVetoWinsOverPositiveIdentity(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	file := validPromotionFile()

	mock.ExpectBegin()
	expectKeywordIdentityLock(mock)
	expectPositivePromotionValidation(mock, file, file.Promotions[0])
	mock.ExpectExec(regexp.QuoteMeta(promotionExternalInsertSQL)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(promotionEvidenceInsertSQL)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectNegativePromotionValidation(mock, file, file.NegativePromotions[0])
	expectKeywordIdentityLock(mock)
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO kb.semid_never_merge")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if _, err := (PromotionStore{DB: db}).ApplyReviewedPromotion(context.Background(), file); err != nil {
		t.Fatalf("ApplyReviewedPromotion: %v", err)
	}

	// The authoritative veto wins over positive identity at decision time.
	mock.ExpectQuery(regexp.QuoteMeta("SELECT EXISTS (SELECT 1 FROM kb.semid_never_merge WHERE family = $1 AND node_a = $2 AND node_b = $3)")).
		WithArgs("keyword", "kw:brightness", "kw:luminance").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	vetoed, err := (semid.NeverMergeStore{DB: db}).IsNeverMerge(context.Background(), "keyword", "kw:luminance", "kw:brightness")
	if err != nil {
		t.Fatal(err)
	}
	if !vetoed {
		t.Fatal("authoritative never_merge veto must win over positive identity")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPromotionRejectsNeverMergeVetoOnEvidenceLink(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	file := validPromotionFile()
	promotion := file.Promotions[0]

	// The promoted surface already carries evidence for another external
	// identity that maps to kw:other, and the operators have asserted
	// never_merge between kw:luminance and kw:other: promotion must fail.
	mock.ExpectBegin()
	expectKeywordIdentityLock(mock)
	expectPositivePromotionValidation(mock, file, promotion, "kw:other")
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.semid_never_merge")).
		WithArgs("keyword", "kw:luminance", "kw:other").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectRollback()

	_, err = (PromotionStore{DB: db}).ApplyReviewedPromotion(context.Background(), file)
	if err == nil || !strings.Contains(err.Error(), "conflicts with never_merge veto") {
		t.Fatalf("error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPromotionRejectsConflictingAlignmentOnEvidenceLink(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	file := validPromotionFile()
	promotion := file.Promotions[0]

	// The promoted surface already carries evidence for another external
	// identity that maps to kw:other, and kw:luminance/kw:other are aligned
	// to different governed terms: promotion must fail (§7.3 item 4).
	mock.ExpectBegin()
	expectKeywordIdentityLock(mock)
	expectPositivePromotionValidation(mock, file, promotion, "kw:other")
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.semid_never_merge")).
		WithArgs("keyword", "kw:luminance", "kw:other").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(regexp.QuoteMeta(acceptedForConceptSQL)).
		WithArgs("kw:luminance").
		WillReturnRows(alignmentReadRow(1, "kw:luminance", testTermID, []byte(testQualifiers), testScore))
	mock.ExpectQuery(regexp.QuoteMeta(acceptedForConceptSQL)).
		WithArgs("kw:other").
		WillReturnRows(alignmentReadRow(2, "kw:other", testOtherTermID, []byte(testQualifiers), testScore))
	mock.ExpectRollback()

	_, err = (PromotionStore{DB: db}).ApplyReviewedPromotion(context.Background(), file)
	if err == nil || !strings.Contains(err.Error(), "conflicts with alignment") {
		t.Fatalf("error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPromotionRejectsStagingConstraintsPromotionOmits(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	file := validPromotionFile()
	file.Promotions[0].UnitConstraints = nil
	file.Promotions[0].DimensionConstraints = nil

	// The staging entry declares unit/dimension constraints the promotion
	// omits: the comparison must be symmetric and refuse the promotion.
	mock.ExpectBegin()
	expectKeywordIdentityLock(mock)
	mock.ExpectQuery(regexp.QuoteMeta(promotionConceptSQL)).
		WithArgs("kw:luminance").
		WillReturnRows(sqlmock.NewRows([]string{"scope", "status"}).AddRow("display", "active"))
	mock.ExpectQuery(regexp.QuoteMeta(promotionPolicySQL)).
		WithArgs(file.Source, file.Release).
		WillReturnRows(sqlmock.NewRows([]string{"authority_role", "license_review_status", "authoritative_relations", "allowed_scopes", "identity_authority"}).
			AddRow("exact_identity_authority", "approved", "{exact_equivalent}", "{display}", true))
	mock.ExpectQuery(regexp.QuoteMeta(promotionDeploymentSQL)).
		WithArgs(file.DeploymentKey).
		WillReturnRows(sqlmock.NewRows([]string{"source", "release", "enabled"}).AddRow(file.Source, file.Release, true))
	mock.ExpectQuery(regexp.QuoteMeta(promotionStagingEntrySQL)).
		WithArgs(file.Source, file.Release, "845-21-050").
		WillReturnRows(sqlmock.NewRows([]string{"native_payload"}).
			AddRow([]byte(`{"unit_constraints":["cd/m2"],"dimension_constraints":["J.L-2"]}`)))
	mock.ExpectRollback()

	_, err = (PromotionStore{DB: db}).ApplyReviewedPromotion(context.Background(), file)
	if err == nil || !strings.Contains(err.Error(), "promotion declares none") {
		t.Fatalf("error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestNegativePromotionRejectsInvalidVetoNode(t *testing.T) {
	// Unknown node concept fails closed.
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	file := validPromotionFile()
	file.Promotions = nil
	mock.ExpectBegin()
	expectKeywordIdentityLock(mock)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT allowed_scopes FROM kb.keyword_sources WHERE source = $1 AND release = $2`)).
		WithArgs(file.Source, file.Release).
		WillReturnRows(sqlmock.NewRows([]string{"allowed_scopes"}).AddRow("{display}"))
	mock.ExpectQuery(regexp.QuoteMeta(promotionConceptSQL)).
		WithArgs("kw:luminance").WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()
	_, err = (PromotionStore{DB: db}).ApplyReviewedPromotion(context.Background(), file)
	if err == nil || !strings.Contains(err.Error(), `unknown concept "kw:luminance" in never_merge veto`) {
		t.Fatalf("error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	db.Close()

	// A non-active veto node is refused.
	db, mock, err = sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	expectKeywordIdentityLock(mock)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT allowed_scopes FROM kb.keyword_sources WHERE source = $1 AND release = $2`)).
		WithArgs(file.Source, file.Release).
		WillReturnRows(sqlmock.NewRows([]string{"allowed_scopes"}).AddRow("{display}"))
	mock.ExpectQuery(regexp.QuoteMeta(promotionConceptSQL)).
		WithArgs("kw:luminance").
		WillReturnRows(sqlmock.NewRows([]string{"scope", "status"}).AddRow("display", "provisional"))
	mock.ExpectRollback()
	_, err = (PromotionStore{DB: db}).ApplyReviewedPromotion(context.Background(), file)
	if err == nil || !strings.Contains(err.Error(), "must be active") {
		t.Fatalf("error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPromotionRejectsMalformedFile(t *testing.T) {
	file := validPromotionFile()
	file.SchemaVersion = 99
	if _, err := (PromotionStore{DB: nil}).ApplyReviewedPromotion(context.Background(), file); err == nil ||
		!strings.Contains(err.Error(), "unsupported promotion schema_version") {
		t.Fatalf("error = %v", err)
	}
}
