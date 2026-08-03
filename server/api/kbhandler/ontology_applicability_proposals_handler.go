package kbhandler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/ontology/modules"
	"github.com/chendingplano/deepdoc/server/api/ontology/policyaudit"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

var newProposalStore = func() modules.ProposalStore {
	return modules.ProposalStore{DB: ApiTypes.ProjectDBHandle}
}

type createProposalRequest struct {
	ModuleID              string          `json:"module_id"`
	ReleaseID             int64           `json:"release_id"`
	Predicate             json.RawMessage `json:"predicate"`
	SourceReleaseChecksum string          `json:"source_release_checksum,omitempty"`
}

// CreateApplicabilityProposal creates a new draft proposal.
func CreateApplicabilityProposal(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_PAP_001")
	defer rc.Close()
	user := rc.IsAuthenticated()
	if err := pipelineRoutingAuthorizer.Authorize(user, PolicyActionProposal); err != nil {
		return policyAuthorizationResponse(c, err)
	}

	var request createProposalRequest
	if err := decodeStrictJSON(c, &request); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error": err.Error()})
	}

	store := newProposalStore()
	proposal, err := store.CreateProposal(
		c.Request().Context(), request.ModuleID, request.ReleaseID,
		request.Predicate, user.UserName, request.SourceReleaseChecksum,
	)
	if err != nil {
		return c.JSON(http.StatusUnprocessableEntity, map[string]any{"status": false, "error": err.Error()})
	}
	writePolicyAuditEvent(c, rc, rc.GetLogger(), policyaudit.Event{
		Kind: policyaudit.EventProposalCreated, SubjectKind: "applicability_proposal",
		SubjectID: proposal.ID,
		Detail: map[string]any{
			"module_id": request.ModuleID, "release_id": request.ReleaseID,
			"predicate_checksum": proposal.PredicateChecksum,
		},
	})
	return c.JSON(http.StatusCreated, map[string]any{"status": true, "proposal": proposal})
}

// GetApplicabilityProposal returns a proposal by ID.
func GetApplicabilityProposal(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_PAP_002")
	defer rc.Close()
	rc.IsAuthenticated()

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error": "valid proposal id is required"})
	}

	store := newProposalStore()
	proposal, err := store.GetProposal(c.Request().Context(), id)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]any{"status": false, "error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "proposal": proposal})
}

type transitionProposalRequest struct {
	Status              string `json:"status"`
	IncludedInReleaseID *int64 `json:"included_in_release_id,omitempty"`
}

// TransitionApplicabilityProposal transitions a proposal's status.
func TransitionApplicabilityProposal(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_PAP_003")
	defer rc.Close()
	user := rc.IsAuthenticated()
	if err := pipelineRoutingAuthorizer.Authorize(user, PolicyActionProposal); err != nil {
		return policyAuthorizationResponse(c, err)
	}

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error": "valid proposal id is required"})
	}

	var request transitionProposalRequest
	if err := decodeStrictJSON(c, &request); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error": err.Error()})
	}
	if strings.TrimSpace(request.Status) == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error": "status is required"})
	}

	store := newProposalStore()
	proposal, err := store.TransitionProposal(
		c.Request().Context(), id, request.Status, user.UserName, request.IncludedInReleaseID,
	)
	if err != nil {
		return c.JSON(http.StatusUnprocessableEntity, map[string]any{"status": false, "error": err.Error()})
	}
	writePolicyAuditEvent(c, rc, rc.GetLogger(), policyaudit.Event{
		Kind: policyaudit.EventProposalTransitioned, SubjectKind: "applicability_proposal",
		SubjectID: proposal.ID,
		Detail: map[string]any{
			"target_status": request.Status, "actor": user.UserName,
		},
	})
	return c.JSON(http.StatusOK, map[string]any{"status": true, "proposal": proposal})
}

// ListApplicabilityProposals lists proposals with optional release_id and
// status filters via query parameters.
func ListApplicabilityProposals(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_PAP_004")
	defer rc.Close()
	rc.IsAuthenticated()

	var releaseID int64
	if raw := strings.TrimSpace(c.QueryParam("release_id")); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed <= 0 {
			return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error": "invalid release_id"})
		}
		releaseID = parsed
	}
	status := strings.TrimSpace(c.QueryParam("status"))

	store := newProposalStore()
	proposals, err := store.ListProposals(c.Request().Context(), releaseID, status)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"status": false, "error": err.Error()})
	}
	if proposals == nil {
		proposals = []modules.ApplicabilityProposal{}
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "proposals": proposals})
}
