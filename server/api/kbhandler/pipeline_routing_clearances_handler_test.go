package kbhandler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	docbenchmark "github.com/chendingplano/deepdoc/server/api/doc-benchmark"
	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/deepdoc/server/api/ontology/policyaudit"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

type fakeRoutingEvidenceLoader struct {
	evidence docbenchmark.RoutingClearanceEvidence
	calls    int
}

func (f *fakeRoutingEvidenceLoader) LoadRoutingClearanceEvidence(context.Context, string, string, string) (docbenchmark.RoutingClearanceEvidence, error) {
	f.calls++
	return f.evidence, nil
}

type fakeRoutingClearanceWriter struct {
	approval  docprocessing.RoutingClearanceApproval
	calls     int
	revokedID int64
	revokedBy string
	reason    string
}

func (f *fakeRoutingClearanceWriter) Approve(_ context.Context, a docprocessing.RoutingClearanceApproval) (int64, error) {
	f.calls++
	f.approval = a
	return 44, nil
}
func (f *fakeRoutingClearanceWriter) Replace(_ context.Context, a docprocessing.RoutingClearanceApproval) (int64, error) {
	f.calls++
	f.approval = a
	return 44, nil
}
func (f *fakeRoutingClearanceWriter) Revoke(_ context.Context, id int64, actor, reason string) error {
	f.calls++
	f.revokedID, f.revokedBy, f.reason = id, actor, reason
	return nil
}

func TestApprovePipelineRoutingClearanceReloadsAndEvaluatesEvidence(t *testing.T) {
	loader := &fakeRoutingEvidenceLoader{evidence: handlerRoutingEvidence()}
	writer := &fakeRoutingClearanceWriter{}
	restore := installRoutingHandlerFakes(t, &ApiTypes.UserInfo{UserName: "owner@example.com", IsOwner: true}, loader, writer)
	defer restore()
	audit := installPolicyAuditFake(t)
	c, rec := routingClearanceContext(http.MethodPost, "/api/v1/kb/pipeline-routing-clearances/approve", `{"pipeline_name":"narrative_default","pipeline_version":3,"subject_kind":"processor_rule","subject_id":12,"subject_checksum":"sha256:subject","document_kind":"standard","net_plan_delta_checksum":"sha256:delta","baseline_run_id":"baseline","routed_run_id":"routed","policy_checksum":"sha256:policy","rationale":"no recall loss"}`)
	if err := ApprovePipelineRoutingClearance(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || loader.calls != 1 || writer.calls != 1 {
		t.Fatalf("code=%d loader=%d writer=%d body=%s", rec.Code, loader.calls, writer.calls, rec.Body.String())
	}
	if writer.approval.Actor != "owner@example.com" || !writer.approval.Approved {
		t.Fatalf("approval=%+v", writer.approval)
	}
	if len(audit.events) != 1 || audit.events[0].Kind != policyaudit.EventClearanceApproved || audit.events[0].SubjectID != 12 || audit.events[0].Actor != "owner@example.com" {
		t.Fatalf("audit events=%+v, want one clearance_approved event for subject 12 by owner@example.com", audit.events)
	}
}

func TestApprovePipelineRoutingClearanceRejectsBodyActorAndPrecomputedDecision(t *testing.T) {
	loader := &fakeRoutingEvidenceLoader{evidence: handlerRoutingEvidence()}
	writer := &fakeRoutingClearanceWriter{}
	restore := installRoutingHandlerFakes(t, &ApiTypes.UserInfo{UserName: "owner", IsOwner: true}, loader, writer)
	defer restore()
	for _, extra := range []string{`,"actor":"forged"`, `,"approved":true`} {
		c, rec := routingClearanceContext(http.MethodPost, "/api/v1/kb/pipeline-routing-clearances/approve", `{"pipeline_name":"narrative_default","pipeline_version":3,"subject_kind":"processor_rule","subject_id":12,"subject_checksum":"sha256:subject","document_kind":"standard","net_plan_delta_checksum":"sha256:delta","baseline_run_id":"baseline","routed_run_id":"routed","policy_checksum":"sha256:policy","rationale":"x"`+extra+`}`)
		if err := ApprovePipelineRoutingClearance(c); err != nil {
			t.Fatal(err)
		}
		if rec.Code != http.StatusBadRequest || writer.calls != 0 {
			t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
		}
	}
}

func TestApprovePipelineRoutingClearanceRejectsFailedEvaluation(t *testing.T) {
	evidence := handlerRoutingEvidence()
	evidence.Routed.Cases[0].RecallNumerator = 0
	loader := &fakeRoutingEvidenceLoader{evidence: evidence}
	writer := &fakeRoutingClearanceWriter{}
	restore := installRoutingHandlerFakes(t, &ApiTypes.UserInfo{UserName: "owner", IsOwner: true}, loader, writer)
	defer restore()
	c, rec := routingClearanceContext(http.MethodPost, "/api/v1/kb/pipeline-routing-clearances/approve", `{"pipeline_name":"narrative_default","pipeline_version":3,"subject_kind":"processor_rule","subject_id":12,"subject_checksum":"sha256:subject","document_kind":"standard","net_plan_delta_checksum":"sha256:delta","baseline_run_id":"baseline","routed_run_id":"routed","policy_checksum":"sha256:policy","rationale":"x"}`)
	if err := ApprovePipelineRoutingClearance(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusUnprocessableEntity || writer.calls != 0 {
		t.Fatalf("code=%d writer=%d body=%s", rec.Code, writer.calls, rec.Body.String())
	}
}

func TestPipelineRoutingClearanceRequiresAdminAuthorization(t *testing.T) {
	tests := map[string]struct {
		user *ApiTypes.UserInfo
		want int
	}{
		"unauthenticated": {user: nil, want: http.StatusUnauthorized},
		"unauthorized":    {user: &ApiTypes.UserInfo{UserName: "reader", Roles: []string{"reader"}}, want: http.StatusForbidden},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			loader := &fakeRoutingEvidenceLoader{evidence: handlerRoutingEvidence()}
			writer := &fakeRoutingClearanceWriter{}
			restore := installRoutingHandlerFakes(t, test.user, loader, writer)
			defer restore()
			c, rec := routingClearanceContext(http.MethodPost, "/api/v1/kb/pipeline-routing-clearances/approve", `{}`)
			if err := ApprovePipelineRoutingClearance(c); err != nil {
				t.Fatal(err)
			}
			if rec.Code != test.want || loader.calls != 0 || writer.calls != 0 {
				t.Fatalf("code=%d loader=%d writer=%d body=%s", rec.Code, loader.calls, writer.calls, rec.Body.String())
			}
		})
	}
}

func TestRevokePipelineRoutingClearanceUsesAuthenticatedActor(t *testing.T) {
	loader := &fakeRoutingEvidenceLoader{}
	writer := &fakeRoutingClearanceWriter{}
	restore := installRoutingHandlerFakes(t, &ApiTypes.UserInfo{UserName: "admin@example.com", Admin: true}, loader, writer)
	defer restore()
	audit := installPolicyAuditFake(t)
	c, rec := routingClearanceContext(http.MethodPost, "/api/v1/kb/pipeline-routing-clearances/44/revoke", `{"reason":"obsolete evidence"}`)
	c.SetPath("/api/v1/kb/pipeline-routing-clearances/:id/revoke")
	c.SetParamNames("id")
	c.SetParamValues("44")
	if err := RevokePipelineRoutingClearance(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || writer.calls != 1 || writer.revokedID != 44 || writer.revokedBy != "admin@example.com" || writer.reason != "obsolete evidence" {
		t.Fatalf("code=%d writer=%+v body=%s", rec.Code, writer, rec.Body.String())
	}
	if len(audit.events) != 1 || audit.events[0].Kind != policyaudit.EventClearanceRevoked || audit.events[0].SubjectID != 44 || audit.events[0].Actor != "admin@example.com" {
		t.Fatalf("audit events=%+v, want one clearance_revoked event for subject 44 by admin@example.com", audit.events)
	}
}

func routingClearanceContext(method, target, body string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}
func installRoutingHandlerFakes(t *testing.T, user *ApiTypes.UserInfo, loader routingClearanceEvidenceLoader, writer routingClearanceWriter) func() {
	t.Helper()
	oldAuth := EchoFactory.DefaultAuthenticator
	oldLoader := newRoutingClearanceEvidenceLoader
	oldWriter := newRoutingClearanceWriter
	EchoFactory.DefaultAuthenticator = func(ApiTypes.RequestContext) (*ApiTypes.UserInfo, error) { return user, nil }
	newRoutingClearanceEvidenceLoader = func() routingClearanceEvidenceLoader { return loader }
	newRoutingClearanceWriter = func() routingClearanceWriter { return writer }
	return func() {
		EchoFactory.DefaultAuthenticator = oldAuth
		newRoutingClearanceEvidenceLoader = oldLoader
		newRoutingClearanceWriter = oldWriter
	}
}
func handlerRoutingEvidence() docbenchmark.RoutingClearanceEvidence {
	cases := []docbenchmark.RoutingClearanceCaseEvidence{{CaseID: "a", Repetition: 1, GoldPositiveDenominator: 1, RecallNumerator: 1, RecallDenominator: 1}, {CaseID: "b", Repetition: 1, GoldPositiveDenominator: 1, RecallNumerator: 1, RecallDenominator: 1}, {CaseID: "c", Repetition: 1, GoldPositiveDenominator: 1, RecallNumerator: 1, RecallDenominator: 1}}
	return docbenchmark.RoutingClearanceEvidence{DocumentKind: "standard", Baseline: docbenchmark.RoutingClearanceRunEvidence{RunID: "baseline", ManifestChecksum: "sha256:manifest", Repetitions: 1, Cases: cases}, Routed: docbenchmark.RoutingClearanceRunEvidence{RunID: "routed", ManifestChecksum: "sha256:manifest", Repetitions: 1, Cases: append([]docbenchmark.RoutingClearanceCaseEvidence(nil), cases...)}}
}
