package kbhandler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/ontology/policyaudit"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

// fakePolicyAuditWriter records every event handed to it so tests can assert
// the exact content-safe event a handler emitted, without a database.
type fakePolicyAuditWriter struct {
	events []policyaudit.Event
}

func (w *fakePolicyAuditWriter) WriteEvent(_ context.Context, event policyaudit.Event) error {
	w.events = append(w.events, event)
	return nil
}

// installPolicyAuditFake overrides newPolicyAuditWriter for the duration of
// the test and restores it afterwards.
func installPolicyAuditFake(t *testing.T) *fakePolicyAuditWriter {
	t.Helper()
	fake := &fakePolicyAuditWriter{}
	old := newPolicyAuditWriter
	newPolicyAuditWriter = func() policyaudit.Writer { return fake }
	t.Cleanup(func() { newPolicyAuditWriter = old })
	return fake
}

func TestPolicyAuthorizerDistinguishesAuthenticationAndAuthorization(t *testing.T) {
	a := RolePolicyAuthorizer{}
	if !errors.Is(a.Authorize(nil, PolicyActionClearance), ErrPolicyUnauthenticated) {
		t.Fatal("nil user must be unauthenticated")
	}
	plain := &ApiTypes.UserInfo{UserName: "reader", Roles: []string{"reader"}}
	if !errors.Is(a.Authorize(plain, PolicyActionClearance), ErrPolicyUnauthorized) {
		t.Fatal("reader must be unauthorized")
	}
	for _, user := range []*ApiTypes.UserInfo{
		{UserName: "owner", IsOwner: true},
		{UserName: "admin-flag", Admin: true},
		{UserName: "admin-role", Roles: []string{" ADMIN "}},
	} {
		if err := a.Authorize(user, PolicyActionClearance); err != nil {
			t.Fatalf("user=%+v err=%v", user, err)
		}
	}
}

func TestWritePolicyAuditEventDerivesActorFromAuthenticatedUserNotClientInput(t *testing.T) {
	fake := installPolicyAuditFake(t)
	oldAuth := EchoFactory.DefaultAuthenticator
	EchoFactory.DefaultAuthenticator = func(ApiTypes.RequestContext) (*ApiTypes.UserInfo, error) {
		return &ApiTypes.UserInfo{UserName: "owner@example.com", IsOwner: true}, nil
	}
	t.Cleanup(func() { EchoFactory.DefaultAuthenticator = oldAuth })

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	rc := EchoFactory.NewFromEcho(c, "TEST_AUDIT")
	defer rc.Close()

	writePolicyAuditEvent(c, rc, rc.GetLogger(), policyaudit.Event{
		Kind: policyaudit.EventBindingAuthored, PipelineName: "legacy_default", SubjectKind: "conditional_binding", SubjectID: 9,
		Actor: "forged-actor", // must be overwritten by the authenticated user, never trusted from a caller
	})

	if len(fake.events) != 1 {
		t.Fatalf("events=%+v, want exactly one", fake.events)
	}
	if fake.events[0].Actor != "owner@example.com" {
		t.Fatalf("actor=%q, want the authenticated user, not caller-supplied input", fake.events[0].Actor)
	}
	if fake.events[0].Kind != policyaudit.EventBindingAuthored || fake.events[0].SubjectID != 9 {
		t.Fatalf("event=%+v", fake.events[0])
	}
}

func TestPolicyAuthorizerAllowsKnowledgeEngineerOnlyForProposals(t *testing.T) {
	a := RolePolicyAuthorizer{}
	user := &ApiTypes.UserInfo{UserName: "engineer", Roles: []string{" K_ENGINEER "}}
	if err := a.Authorize(user, PolicyActionProposal); err != nil {
		t.Fatalf("proposal err=%v", err)
	}
	if !errors.Is(a.Authorize(user, PolicyActionActivation), ErrPolicyUnauthorized) {
		t.Fatal("knowledge engineer must not activate policies")
	}
}
