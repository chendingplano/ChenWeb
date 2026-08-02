package kbhandler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	docbenchmark "github.com/chendingplano/deepdoc/server/api/doc-benchmark"
	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

type routingClearanceEvidenceLoader interface {
	LoadRoutingClearanceEvidence(context.Context, string, string, string) (docbenchmark.RoutingClearanceEvidence, error)
}

type routingClearanceWriter interface {
	Approve(context.Context, docprocessing.RoutingClearanceApproval) (int64, error)
	Replace(context.Context, docprocessing.RoutingClearanceApproval) (int64, error)
	Revoke(context.Context, int64, string, string) error
}

var (
	newRoutingClearanceEvidenceLoader = func() routingClearanceEvidenceLoader {
		return docbenchmark.SQLStore{DB: ApiTypes.ProjectDBHandle}
	}
	newRoutingClearanceWriter = func() routingClearanceWriter {
		return docprocessing.RoutingClearanceStore{DB: ApiTypes.ProjectDBHandle}
	}
	pipelineRoutingAuthorizer PolicyAuthorizer = RolePolicyAuthorizer{}
)

type routingClearanceRequest struct {
	PolicyID             int64              `json:"policy_id"`
	PolicyVersion        int                `json:"policy_version"`
	SubjectKind          string             `json:"subject_kind"`
	SubjectID            int64              `json:"subject_id"`
	SubjectChecksum      string             `json:"subject_checksum"`
	DocumentKind         string             `json:"document_kind"`
	NetPlanDeltaChecksum string             `json:"net_plan_delta_checksum"`
	BaselineRunID        string             `json:"baseline_run_id"`
	RoutedRunID          string             `json:"routed_run_id"`
	PolicyChecksum       string             `json:"policy_checksum"`
	BindingChecksums     []string           `json:"binding_checksums,omitempty"`
	GateChecksums        []string           `json:"gate_checksums,omitempty"`
	CostYield            map[string]float64 `json:"cost_yield,omitempty"`
	DecisionTraces       []string           `json:"decision_traces,omitempty"`
	Rationale            string             `json:"rationale"`
}

// ApprovePipelineRoutingClearance reloads benchmark evidence, evaluates it on
// the server, and persists an append-only approval scoped to the exact plan.
func ApprovePipelineRoutingClearance(c echo.Context) error {
	return writePipelineRoutingClearance(c, false)
}

// ReplacePipelineRoutingClearance atomically revokes the current generation
// and writes a newly evaluated approval.
func ReplacePipelineRoutingClearance(c echo.Context) error {
	return writePipelineRoutingClearance(c, true)
}

func writePipelineRoutingClearance(c echo.Context, replace bool) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_PRC_001")
	defer rc.Close()
	user := rc.IsAuthenticated()
	if err := pipelineRoutingAuthorizer.Authorize(user, PolicyActionClearance); err != nil {
		return policyAuthorizationResponse(c, err)
	}

	var request routingClearanceRequest
	if err := decodeStrictJSON(c, &request); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error": err.Error()})
	}
	if err := request.validate(); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error": err.Error()})
	}

	evidence, err := newRoutingClearanceEvidenceLoader().LoadRoutingClearanceEvidence(
		c.Request().Context(), request.BaselineRunID, request.RoutedRunID, request.DocumentKind,
	)
	if err != nil {
		return c.JSON(http.StatusUnprocessableEntity, map[string]any{"status": false, "error": err.Error()})
	}
	evidence.DocumentKind = strings.TrimSpace(request.DocumentKind)
	evidence.PolicyChecksum = strings.TrimSpace(request.PolicyChecksum)
	evidence.BindingChecksums = append([]string(nil), request.BindingChecksums...)
	evidence.GateChecksums = append([]string(nil), request.GateChecksums...)
	evidence.CostYield = request.CostYield
	evidence.DecisionTraces = append([]string(nil), request.DecisionTraces...)
	decision := docbenchmark.EvaluateRoutingClearance(evidence)
	if !decision.Approved {
		return c.JSON(http.StatusUnprocessableEntity, map[string]any{"status": false, "decision": decision})
	}

	approval := routingApprovalFromEvidence(request, evidence, decision, user.UserName)
	writer := newRoutingClearanceWriter()
	var id int64
	if replace {
		id, err = writer.Replace(c.Request().Context(), approval)
	} else {
		id, err = writer.Approve(c.Request().Context(), approval)
	}
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"status": false, "error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "id": id, "decision": decision})
}

type revokeRoutingClearanceRequest struct {
	Reason string `json:"reason"`
}

// RevokePipelineRoutingClearance records a revocation without mutating the
// original approval or its evidence.
func RevokePipelineRoutingClearance(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_PRC_002")
	defer rc.Close()
	user := rc.IsAuthenticated()
	if err := pipelineRoutingAuthorizer.Authorize(user, PolicyActionClearance); err != nil {
		return policyAuthorizationResponse(c, err)
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error": "valid clearance id is required"})
	}
	var request revokeRoutingClearanceRequest
	if err := decodeStrictJSON(c, &request); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error": err.Error()})
	}
	if strings.TrimSpace(request.Reason) == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error": "reason is required"})
	}
	if err := newRoutingClearanceWriter().Revoke(c.Request().Context(), id, user.UserName, request.Reason); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"status": false, "error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "id": id})
}

func (r routingClearanceRequest) validate() error {
	if r.PolicyID <= 0 || r.PolicyVersion <= 0 || r.SubjectID <= 0 {
		return errors.New("policy_id, policy_version, and subject_id must be positive")
	}
	values := map[string]string{
		"subject_kind": r.SubjectKind, "subject_checksum": r.SubjectChecksum,
		"document_kind": r.DocumentKind, "net_plan_delta_checksum": r.NetPlanDeltaChecksum,
		"baseline_run_id": r.BaselineRunID, "routed_run_id": r.RoutedRunID,
		"policy_checksum": r.PolicyChecksum, "rationale": r.Rationale,
	}
	for field, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", field)
		}
	}
	if strings.TrimSpace(r.BaselineRunID) == strings.TrimSpace(r.RoutedRunID) {
		return errors.New("baseline_run_id and routed_run_id must differ")
	}
	return nil
}

func decodeStrictJSON(c echo.Context, target any) error {
	decoder := json.NewDecoder(c.Request().Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return errors.New("request body must contain one JSON object")
		}
		return err
	}
	return nil
}

func policyAuthorizationResponse(c echo.Context, err error) error {
	if errors.Is(err, ErrPolicyUnauthenticated) {
		return c.JSON(http.StatusUnauthorized, map[string]any{"status": false, "error": err.Error()})
	}
	return c.JSON(http.StatusForbidden, map[string]any{"status": false, "error": err.Error()})
}

func routingApprovalFromEvidence(request routingClearanceRequest, evidence docbenchmark.RoutingClearanceEvidence, decision docbenchmark.RoutingClearanceDecision, actor string) docprocessing.RoutingClearanceApproval {
	approval := docprocessing.RoutingClearanceApproval{
		Subject: docprocessing.RoutingClearanceSubject{
			PolicyID: request.PolicyID, PolicyVersion: request.PolicyVersion,
			SubjectKind: strings.TrimSpace(request.SubjectKind), SubjectID: request.SubjectID,
			SubjectChecksum: strings.TrimSpace(request.SubjectChecksum), DocumentKind: strings.TrimSpace(request.DocumentKind),
			NetPlanDeltaChecksum: strings.TrimSpace(request.NetPlanDeltaChecksum),
		},
		PolicyChecksum: strings.TrimSpace(request.PolicyChecksum), ManifestChecksum: evidence.Baseline.ManifestChecksum,
		BaselineRunID: evidence.Baseline.RunID, RoutedRunID: evidence.Routed.RunID,
		Repetitions: evidence.Baseline.Repetitions, PairedCaseCount: decision.CasePairs,
		GoldPositiveDenominator: decision.GoldPositiveDenom, ProcessorFailures: decision.ProcessorFailures,
		InfrastructureFailures: decision.InfrastructureFailures, ScorerFailures: decision.ScorerFailures,
		CostYield: decision.CostYield, DecisionTraces: decision.DecisionTraces,
		Approved: decision.Approved, Actor: strings.TrimSpace(actor), Rationale: strings.TrimSpace(request.Rationale),
	}
	for _, item := range evidence.Baseline.Cases {
		approval.BaselineRecallNumerator += item.RecallNumerator
		approval.BaselineRecallDenominator += item.RecallDenominator
	}
	for _, item := range evidence.Routed.Cases {
		approval.RoutedRecallNumerator += item.RecallNumerator
		approval.RoutedRecallDenominator += item.RecallDenominator
	}
	return approval
}
