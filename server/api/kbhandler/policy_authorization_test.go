package kbhandler

import (
	"errors"
	"testing"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

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
