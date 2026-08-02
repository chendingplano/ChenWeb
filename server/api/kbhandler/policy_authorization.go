package kbhandler

import (
	"errors"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

type PolicyAction string

const (
	PolicyActionClearance  PolicyAction = "clearance"
	PolicyActionActivation PolicyAction = "activation"
	PolicyActionProposal   PolicyAction = "proposal"
)

var (
	ErrPolicyUnauthenticated = errors.New("policy actor is unauthenticated")
	ErrPolicyUnauthorized    = errors.New("policy actor is unauthorized")
)

type PolicyAuthorizer interface {
	Authorize(user *ApiTypes.UserInfo, action PolicyAction) error
}

type RolePolicyAuthorizer struct{}

func (RolePolicyAuthorizer) Authorize(user *ApiTypes.UserInfo, action PolicyAction) error {
	if user == nil || strings.TrimSpace(user.UserName) == "" {
		return ErrPolicyUnauthenticated
	}
	if user.IsOwner || user.Admin || hasNormalizedRole(user.Roles, "admin") {
		return nil
	}
	if action == PolicyActionProposal && hasNormalizedRole(user.Roles, "k_engineer") {
		return nil
	}
	return ErrPolicyUnauthorized
}

func hasNormalizedRole(roles []string, wanted string) bool {
	for _, role := range roles {
		if strings.EqualFold(strings.TrimSpace(role), wanted) {
			return true
		}
	}
	return false
}
