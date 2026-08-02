package kbhandler

import (
	"errors"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/ontology/policyaudit"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

// newPolicyAuditWriter is the shared E3 policyaudit.Writer constructor used
// by every P5 authoring/activation/clearance handler. Tests override it to
// inject a fake writer without touching ApiTypes.ProjectDBHandle.
var newPolicyAuditWriter = func() policyaudit.Writer {
	return policyaudit.SQLStore{DB: ApiTypes.ProjectDBHandle}
}

// writePolicyAuditEvent best-effort persists a P5 policy/routing audit
// event, deriving the actor from the request's already-authenticated user
// (if any) rather than trusting client input. A write failure is logged and
// never fails the caller's response -- audit persistence must not block
// policy authoring/activation/clearance actions.
func writePolicyAuditEvent(c echo.Context, rc ApiTypes.RequestContext, logger ApiTypes.JimoLogger, event policyaudit.Event) {
	if user := rc.IsAuthenticated(); user != nil {
		event.Actor = strings.TrimSpace(user.UserName)
	}
	if err := newPolicyAuditWriter().WriteEvent(c.Request().Context(), event); err != nil && logger != nil {
		logger.Warn("failed writing policy audit event", "kind", event.Kind, "err", err)
	}
}

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
