package kbhandler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/chendingplano/deepdoc/server/api/ontology/candidates"
	"github.com/labstack/echo/v4"
)

type ontologyCandidateListResponse struct {
	Status  bool                 `json:"status"`
	Results []candidates.Candidate `json:"results"`
	Total   int                  `json:"total"`
}

type ontologyCandidateDetailResponse struct {
	Status bool                 `json:"status"`
	Record candidates.Candidate `json:"record"`
}

// ListOntologyCandidates handles GET /api/v1/kb/ontology/candidates?status=&kind=&module_id=.
func ListOntologyCandidates(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_OC_001")
	defer rc.Close()
	logger := rc.GetLogger()

	store := candidates.CandidateStore{DB: ApiTypes.ProjectDBHandle}
	list, err := store.ListCandidates(c.Request().Context(), c.QueryParam("status"), c.QueryParam("kind"), c.QueryParam("module_id"))
	if err != nil {
		logger.Error("list ontology candidates failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to retrieve ontology candidates (CWB_KB_OC_002)"})
	}
	return c.JSON(http.StatusOK, ontologyCandidateListResponse{Status: true, Results: list, Total: len(list)})
}

// CreateOntologyCandidate handles POST /api/v1/kb/ontology/candidates. The
// fingerprint is computed from the normalized payload, source, and module; an
// identical existing proposal is returned with reused=true instead of
// creating duplicate review work (spec §16.3 item 1).
func CreateOntologyCandidate(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_OC_100")
	defer rc.Close()
	logger := rc.GetLogger()

	var payload struct {
		CandidateKind     string          `json:"candidate_kind"`
		ProposedPayload   json.RawMessage `json:"proposed_payload"`
		ProposedModuleID  string          `json:"proposed_module_id"`
		SourceType        string          `json:"source_type"`
		SourceRef         string          `json:"source_ref"`
		SourceLineSpans   json.RawMessage `json:"source_line_spans"`
		DiscoveryMethod   string          `json:"discovery_method"`
		Confidence        *float64        `json:"confidence"`
		CandidateMatches  json.RawMessage `json:"candidate_matches"`
		ProposedBy        string          `json:"proposed_by"`
		CreateBy          string          `json:"create_by"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_OC_101)"})
	}
	if !candidates.AllowedCandidateKinds[payload.CandidateKind] {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "unsupported candidate_kind (CWB_KB_OC_102)"})
	}

	store := candidates.CandidateStore{DB: ApiTypes.ProjectDBHandle}
	created, err := store.CreateCandidate(c.Request().Context(), candidates.Candidate{
		CandidateKind:    payload.CandidateKind,
		ProposedPayload:  payload.ProposedPayload,
		ProposedModuleID: strings.TrimSpace(payload.ProposedModuleID),
		SourceType:       payload.SourceType,
		SourceRef:        payload.SourceRef,
		SourceLineSpans:  payload.SourceLineSpans,
		DiscoveryMethod:  payload.DiscoveryMethod,
		Confidence:       payload.Confidence,
		CandidateMatches: payload.CandidateMatches,
		ProposedBy:       payload.ProposedBy,
		CreateBy:         payload.CreateBy,
		ModifyBy:         payload.CreateBy,
	})
	if err != nil {
		logger.Error("create ontology candidate failed", "kind", payload.CandidateKind, "err", err)
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "failed to create ontology candidate (CWB_KB_OC_103)"})
	}
	return c.JSON(http.StatusOK, ontologyCandidateDetailResponse{Status: true, Record: created})
}

// GetOntologyCandidate handles GET /api/v1/kb/ontology/candidates/:id.
func GetOntologyCandidate(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_OC_200")
	defer rc.Close()
	logger := rc.GetLogger()

	id, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid id (CWB_KB_OC_201)"})
	}
	store := candidates.CandidateStore{DB: ApiTypes.ProjectDBHandle}
	got, err := store.GetCandidate(c.Request().Context(), id)
	if err != nil {
		logger.Error("get ontology candidate failed", "id", id, "err", err)
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "ontology candidate not found (CWB_KB_OC_202)"})
	}
	return c.JSON(http.StatusOK, ontologyCandidateDetailResponse{Status: true, Record: got})
}

// TransitionOntologyCandidate handles POST /api/v1/kb/ontology/candidates/:id/transition,
// moving a candidate along the spec §9.3 state machine.
func TransitionOntologyCandidate(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_OC_300")
	defer rc.Close()
	logger := rc.GetLogger()

	id, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid id (CWB_KB_OC_301)"})
	}
	var payload struct {
		To string `json:"to"`
		By string `json:"by"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_OC_302)"})
	}
	store := candidates.CandidateStore{DB: ApiTypes.ProjectDBHandle}
	got, err := store.TransitionStatus(c.Request().Context(), id, strings.TrimSpace(payload.To), payload.By)
	if err != nil {
		logger.Error("transition ontology candidate failed", "id", id, "to", payload.To, "err", err)
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "failed to transition ontology candidate (CWB_KB_OC_303)"})
	}
	return c.JSON(http.StatusOK, ontologyCandidateDetailResponse{Status: true, Record: got})
}

// DeferOntologyCandidate handles POST /api/v1/kb/ontology/candidates/:id/defer,
// moving an editable candidate to deferred and recording the dependency
// fingerprint it is blocked on (spec §9.3). Retry requires that fingerprint to
// change.
func DeferOntologyCandidate(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_OC_350")
	defer rc.Close()
	logger := rc.GetLogger()

	id, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid id (CWB_KB_OC_351)"})
	}
	var payload struct {
		DependencyFingerprint string `json:"dependency_fingerprint"`
		Reason                string `json:"reason"`
		By                    string `json:"by"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_OC_352)"})
	}
	store := candidates.CandidateStore{DB: ApiTypes.ProjectDBHandle}
	got, err := store.DeferCandidate(c.Request().Context(), id, strings.TrimSpace(payload.DependencyFingerprint), payload.Reason, payload.By)
	if err != nil {
		logger.Error("defer ontology candidate failed", "id", id, "err", err)
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "failed to defer ontology candidate (CWB_KB_OC_353)"})
	}
	return c.JSON(http.StatusOK, ontologyCandidateDetailResponse{Status: true, Record: got})
}

// RetryOntologyCandidate handles POST /api/v1/kb/ontology/candidates/:id/retry,
// re-opening a deferred candidate as draft only when the dependency
// fingerprint has changed.
func RetryOntologyCandidate(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_OC_400")
	defer rc.Close()
	logger := rc.GetLogger()

	id, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid id (CWB_KB_OC_401)"})
	}
	var payload struct {
		DependencyFingerprint string `json:"dependency_fingerprint"`
		By                    string `json:"by"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_OC_402)"})
	}
	store := candidates.CandidateStore{DB: ApiTypes.ProjectDBHandle}
	got, err := store.RetryDeferred(c.Request().Context(), id, strings.TrimSpace(payload.DependencyFingerprint), payload.By)
	if err != nil {
		logger.Error("retry deferred ontology candidate failed", "id", id, "err", err)
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "failed to retry deferred ontology candidate (CWB_KB_OC_403)"})
	}
	return c.JSON(http.StatusOK, ontologyCandidateDetailResponse{Status: true, Record: got})
}

// UpdateOntologyCandidatePayload handles PUT /api/v1/kb/ontology/candidates/:id/payload.
// Edits are refused once the candidate is in_review (the reviewed payload is
// frozen; spec §9.3).
func UpdateOntologyCandidatePayload(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_OC_500")
	defer rc.Close()
	logger := rc.GetLogger()

	id, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid id (CWB_KB_OC_501)"})
	}
	var payload struct {
		ProposedPayload json.RawMessage `json:"proposed_payload"`
		By              string          `json:"by"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_OC_502)"})
	}
	store := candidates.CandidateStore{DB: ApiTypes.ProjectDBHandle}
	got, err := store.UpdatePayload(c.Request().Context(), id, payload.ProposedPayload, payload.By)
	if err != nil {
		logger.Error("update ontology candidate payload failed", "id", id, "err", err)
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "failed to update ontology candidate payload (CWB_KB_OC_503)"})
	}
	return c.JSON(http.StatusOK, ontologyCandidateDetailResponse{Status: true, Record: got})
}

// PromoteOntologyCandidate handles POST /api/v1/kb/ontology/candidates/:id/promote,
// materializing an approved candidate into the governed content tables. The
// candidate stays approved; inclusion in a module release is chunk B.
func PromoteOntologyCandidate(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_OC_600")
	defer rc.Close()
	logger := rc.GetLogger()

	id, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid id (CWB_KB_OC_601)"})
	}
	var payload struct {
		By string `json:"by"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil && err.Error() != "EOF" {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_OC_602)"})
	}
	store := candidates.CandidateStore{DB: ApiTypes.ProjectDBHandle}
	got, err := store.PromoteToContent(c.Request().Context(), id, payload.By)
	if err != nil {
		logger.Error("promote ontology candidate failed", "id", id, "err", err)
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "failed to promote ontology candidate (CWB_KB_OC_603)"})
	}
	return c.JSON(http.StatusOK, ontologyCandidateDetailResponse{Status: true, Record: got})
}
