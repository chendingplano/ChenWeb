package kbhandler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/deepdoc/server/api/ontology/semrules"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

// installProposalAuth overrides the echo authenticator for the duration of the
// test. A nil user simulates a not-logged-in request, which the authorizer
// maps to 401. (Returning an auth ERROR here would trip the shared activity-
// log cache that is not initialized under test, so the not-logged-in state is
// used instead.)
func installProposalAuth(t *testing.T, user *ApiTypes.UserInfo) func() {
	t.Helper()
	old := EchoFactory.DefaultAuthenticator
	EchoFactory.DefaultAuthenticator = func(ApiTypes.RequestContext) (*ApiTypes.UserInfo, error) {
		return user, nil
	}
	return func() { EchoFactory.DefaultAuthenticator = old }
}

func newProposalContext(t *testing.T, method, target, id, body string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if id != "" {
		c.SetParamNames("id")
		c.SetParamValues(id)
	}
	return c, rec
}

const validProposalPredicate = `{"version":1,"expression":{"kind":"fact","path":"document.doc_kind","op":"eq","value":"product_specification"}}`

// proposalReturningRow matches CreateProposal's 10-column RETURNING.
func proposalReturningRow(createTime time.Time, predicate string, checksum string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "module_id", "release_id", "proposal_kind", "predicate", "predicate_checksum",
		"status", "source_release_checksum", "created_by", "create_time",
	}).AddRow(int64(7), "vent", int64(42), "routing", []byte(predicate), checksum,
		"draft", "src-checksum", "owner@example.com", createTime)
}

// TestCreateApplicabilityProposalUnauthenticatedReturns401 proves an
// unauthenticated request is rejected with 401 before any store call (plan H1:
// "unauthenticated/unauthorized rejection").
func TestCreateApplicabilityProposalUnauthenticatedReturns401(t *testing.T) {
	restore := installProposalAuth(t, nil)
	defer restore()

	c, rec := newProposalContext(t, http.MethodPost, "/api/v1/kb/ontology/applicability-proposals", "", validProposalPredicate)
	if err := CreateApplicabilityProposal(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code=%d want 401", rec.Code)
	}
}

// TestCreateApplicabilityProposalUnauthorizedReturns403 proves an
// authenticated-but-unauthorized user (no owner/admin/k_engineer role) is
// rejected with 403.
func TestCreateApplicabilityProposalUnauthorizedReturns403(t *testing.T) {
	restore := installProposalAuth(t, &ApiTypes.UserInfo{UserName: "reader@example.com", Roles: []string{"reader"}})
	defer restore()

	c, rec := newProposalContext(t, http.MethodPost, "/api/v1/kb/ontology/applicability-proposals", "", validProposalPredicate)
	if err := CreateApplicabilityProposal(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("code=%d want 403", rec.Code)
	}
}

// TestCreateApplicabilityProposalOwnerSuccessDerivesActorFromUser proves an
// owner can create a draft proposal; the created_by column is derived from the
// authenticated user's UserName (the request body carries no actor field), and
// the stored predicate/checksum are canonical with the source release checksum
// pinned from the request (plan H1: actor derivation, source-release pinning,
// predicate/checksum validation).
func TestCreateApplicabilityProposalOwnerSuccessDerivesActorFromUser(t *testing.T) {
	restore := installProposalAuth(t, &ApiTypes.UserInfo{UserName: "owner@example.com", IsOwner: true})
	defer restore()
	_, mock := installPolicyDB(t)
	installPolicyAuditFake(t)

	canonical, checksum, err := canonicalProposalValues()
	if err != nil {
		t.Fatal(err)
	}
	createTime := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO kb.ontology_applicability_proposals
    (module_id, release_id, proposal_kind, predicate, predicate_checksum, status, source_release_checksum, created_by)
VALUES ($1, $2, 'routing', $3::jsonb, $4, 'draft', $5, $6)
RETURNING id, module_id, release_id, proposal_kind, predicate, predicate_checksum, status,
          COALESCE(source_release_checksum, ''), COALESCE(created_by, ''), create_time`)).
		WithArgs("vent", int64(42), string(canonical), checksum, "src-checksum", "owner@example.com").
		WillReturnRows(proposalReturningRow(createTime, string(canonical), checksum))

	body := `{"module_id":"vent","release_id":42,"predicate":` + validProposalPredicate + `,"source_release_checksum":"src-checksum"}`
	c, rec := newProposalContext(t, http.MethodPost, "/api/v1/kb/ontology/applicability-proposals", "", body)
	if err := CreateApplicabilityProposal(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("code=%d body=%s want 201", rec.Code, rec.Body.String())
	}
	// Actor is derived from the authenticated user, not the body (the body has
	// no actor field): the INSERT created_by arg above is owner@example.com.
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestCreateApplicabilityProposalKEngineerSuccess proves a k_engineer role is
// authorized for proposal authoring (plan H1: owner/admin/k_engineer success).
func TestCreateApplicabilityProposalKEngineerSuccess(t *testing.T) {
	restore := installProposalAuth(t, &ApiTypes.UserInfo{UserName: "eng@example.com", Roles: []string{"k_engineer"}})
	defer restore()
	_, mock := installPolicyDB(t)

	canonical, checksum, err := canonicalProposalValues()
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO kb.ontology_applicability_proposals`)).
		WithArgs("vent", int64(42), string(canonical), checksum, nil, "eng@example.com").
		WillReturnRows(proposalReturningRow(time.Now(), string(canonical), checksum))

	body := `{"module_id":"vent","release_id":42,"predicate":` + validProposalPredicate + `}`
	c, rec := newProposalContext(t, http.MethodPost, "/api/v1/kb/ontology/applicability-proposals", "", body)
	if err := CreateApplicabilityProposal(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("code=%d body=%s want 201", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestCreateApplicabilityProposalInvalidPredicateRejected proves predicate
// validation happens before any database write: a malformed predicate returns
// 422 with no INSERT issued (plan H1: predicate/checksum validation).
func TestCreateApplicabilityProposalInvalidPredicateRejected(t *testing.T) {
	restore := installProposalAuth(t, &ApiTypes.UserInfo{UserName: "owner@example.com", IsOwner: true})
	defer restore()
	_, mock := installPolicyDB(t)

	body := `{"module_id":"vent","release_id":42,"predicate":"{not json"}`
	c, rec := newProposalContext(t, http.MethodPost, "/api/v1/kb/ontology/applicability-proposals", "", body)
	if err := CreateApplicabilityProposal(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("code=%d want 422", rec.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("no database write should occur for an invalid predicate: %v", err)
	}
}

// TestTransitionApplicabilityProposalInvalidTransitionRejected proves an
// invalid transition (draft -> approved) is rejected with 422 before any
// UPDATE (plan H1: invalid transitions).
func TestTransitionApplicabilityProposalInvalidTransitionRejected(t *testing.T) {
	restore := installProposalAuth(t, &ApiTypes.UserInfo{UserName: "owner@example.com", IsOwner: true})
	defer restore()
	_, mock := installPolicyDB(t)

	// GetProposal reads the current status as 'draft'.
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, module_id, release_id, proposal_kind, predicate, predicate_checksum, status,
       COALESCE(source_release_checksum, ''), COALESCE(approved_by, ''), approved_at,
       included_in_release_id, COALESCE(created_by, ''), create_time
FROM kb.ontology_applicability_proposals WHERE id = $1`)).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "module_id", "release_id", "proposal_kind", "predicate", "predicate_checksum",
			"status", "source_release_checksum", "approved_by", "approved_at",
			"included_in_release_id", "created_by", "create_time",
		}).AddRow(int64(7), "vent", int64(42), "routing", []byte(`{}`), "abc",
			"draft", "", "", nil, nil, "owner@example.com", time.Now()))

	c, rec := newProposalContext(t, http.MethodPost, "/api/v1/kb/ontology/applicability-proposals/7/transition", "7", `{"status":"approved"}`)
	if err := TransitionApplicabilityProposal(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("code=%d want 422 for draft->approved", rec.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("no UPDATE should occur for an invalid transition: %v", err)
	}
}

// TestTransitionApplicabilityProposalApprovalDerivesActor proves an
// in_review -> approved transition succeeds and the approved_by column is the
// authenticated user's UserName (actor derived, never from the body).
func TestTransitionApplicabilityProposalApprovalDerivesActor(t *testing.T) {
	restore := installProposalAuth(t, &ApiTypes.UserInfo{UserName: "curator@example.com", Admin: true})
	defer restore()
	_, mock := installPolicyDB(t)
	installPolicyAuditFake(t)

	// GetProposal reads the current status as 'in_review'.
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, module_id, release_id, proposal_kind, predicate, predicate_checksum, status,
       COALESCE(source_release_checksum, ''), COALESCE(approved_by, ''), approved_at,
       included_in_release_id, COALESCE(created_by, ''), create_time
FROM kb.ontology_applicability_proposals WHERE id = $1`)).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "module_id", "release_id", "proposal_kind", "predicate", "predicate_checksum",
			"status", "source_release_checksum", "approved_by", "approved_at",
			"included_in_release_id", "created_by", "create_time",
		}).AddRow(int64(7), "vent", int64(42), "routing", []byte(`{}`), "abc",
			"in_review", "", "", nil, nil, "curator@example.com", time.Now()))
	// Guarded transition to approved; approved_by = the authenticated actor.
	approvedAt := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta(`UPDATE kb.ontology_applicability_proposals`)).
		WithArgs(int64(7), "approved", "curator@example.com", sqlmock.AnyArg(), "in_review").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "module_id", "release_id", "proposal_kind", "predicate", "predicate_checksum",
			"status", "source_release_checksum", "approved_by", "approved_at",
			"included_in_release_id", "created_by", "create_time",
		}).AddRow(int64(7), "vent", int64(42), "routing", []byte(`{}`), "abc",
			"approved", "", "curator@example.com", approvedAt, nil, "curator@example.com", time.Now()))

	c, rec := newProposalContext(t, http.MethodPost, "/api/v1/kb/ontology/applicability-proposals/7/transition", "7", `{"status":"approved"}`)
	if err := TransitionApplicabilityProposal(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s want 200", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestGetApplicabilityProposalNotFound proves a missing proposal returns 404.
func TestGetApplicabilityProposalNotFound(t *testing.T) {
	restore := installProposalAuth(t, &ApiTypes.UserInfo{UserName: "reader@example.com"})
	defer restore()
	_, mock := installPolicyDB(t)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, module_id, release_id, proposal_kind, predicate, predicate_checksum, status,
       COALESCE(source_release_checksum, ''), COALESCE(approved_by, ''), approved_at,
       included_in_release_id, COALESCE(created_by, ''), create_time
FROM kb.ontology_applicability_proposals WHERE id = $1`)).
		WithArgs(int64(999)).
		WillReturnError(sql.ErrNoRows)

	c, rec := newProposalContext(t, http.MethodGet, "/api/v1/kb/ontology/applicability-proposals/999", "999", "")
	if err := GetApplicabilityProposal(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code=%d want 404", rec.Code)
	}
}

func canonicalProposalValues() (canonical []byte, checksum string, err error) {
	var doc semrules.Document
	if err := json.Unmarshal([]byte(validProposalPredicate), &doc); err != nil {
		return nil, "", err
	}
	canonical, checksum, err = semrules.Canonicalize(doc)
	return canonical, checksum, err
}
