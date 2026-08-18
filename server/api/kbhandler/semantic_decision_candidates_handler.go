package kbhandler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/ontology/assertions"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

type semanticDecisionCandidateListResponse struct {
	Status   bool                           `json:"status"`
	Results  []assertions.DecisionCandidate `json:"results"`
	Page     int                            `json:"page"`
	PageSize int                            `json:"page_size"`
	Total    int64                          `json:"total"`
}
type semanticDecisionCandidateResponse struct {
	Status bool                         `json:"status"`
	Record assertions.DecisionCandidate `json:"record"`
}
type candidateAction struct {
	To                    string `json:"to"`
	Reason                string `json:"reason"`
	By                    string `json:"by"`
	DependencyFingerprint string `json:"dependency_fingerprint"`
	Outcome               string `json:"outcome"`
	AssertionID           int64  `json:"assertion_id"`
}

func candidateStore() assertions.DecisionCandidateStore {
	return assertions.DecisionCandidateStore{DB: ApiTypes.ProjectDBHandle}
}
func candidateID(c echo.Context) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id <= 0 {
		return 0, echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	return id, nil
}

func ListSemanticDecisionCandidates(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_SDC_001")
	defer rc.Close()
	logger := rc.GetLogger()
	page, _ := strconv.Atoi(c.QueryParam("page"))
	pageSize, _ := strconv.Atoi(c.QueryParam("page_size"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}
	var inputID *int64
	if raw := strings.TrimSpace(c.QueryParam("input_record_id")); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || v <= 0 {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid input_record_id"})
		}
		inputID = &v
	}
	rows, total, err := candidateStore().List(c.Request().Context(), assertions.DecisionCandidateListFilter{Status: c.QueryParam("status"), CandidateKind: c.QueryParam("candidate_kind"), Method: c.QueryParam("method"), LogicalIdentity: c.QueryParam("logical_identity"), SourceArtifactType: c.QueryParam("source_artifact_type"), SourceArtifactID: c.QueryParam("source_artifact_id"), InputRecordID: inputID, Page: page, PageSize: pageSize, SortBy: c.QueryParam("sort_by"), SortDir: c.QueryParam("sort_dir")})
	if err != nil {
		logger.Error("list semantic decision candidates failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to list semantic decision candidates"})
	}
	return c.JSON(http.StatusOK, semanticDecisionCandidateListResponse{Status: true, Results: rows, Page: page, PageSize: pageSize, Total: total})
}

func CreateSemanticDecisionCandidate(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_SDC_100")
	defer rc.Close()
	logger := rc.GetLogger()
	var req assertions.DecisionCandidate
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body"})
	}
	got, err := candidateStore().Propose(c.Request().Context(), req)
	if err != nil {
		logger.Error("create semantic decision candidate failed", "err", err)
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: err.Error()})
	}
	return c.JSON(http.StatusOK, semanticDecisionCandidateResponse{Status: true, Record: got})
}

func GetSemanticDecisionCandidate(c echo.Context) error {
	id, err := candidateID(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: err.Error()})
	}
	got, err := candidateStore().GetByID(c.Request().Context(), id)
	if err != nil {
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "semantic decision candidate not found"})
	}
	return c.JSON(http.StatusOK, semanticDecisionCandidateResponse{Status: true, Record: got})
}

func TransitionSemanticDecisionCandidate(c echo.Context) error {
	id, err := candidateID(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: err.Error()})
	}
	var req candidateAction
	if json.NewDecoder(c.Request().Body).Decode(&req) != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body"})
	}
	got, err := candidateStore().TransitionStatus(c.Request().Context(), id, strings.TrimSpace(req.To), req.Reason, req.By)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: err.Error()})
	}
	return c.JSON(http.StatusOK, semanticDecisionCandidateResponse{Status: true, Record: got})
}
func ResolveSemanticDecisionCandidate(c echo.Context) error {
	id, err := candidateID(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: err.Error()})
	}
	var req candidateAction
	if json.NewDecoder(c.Request().Body).Decode(&req) != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body"})
	}
	got, err := candidateStore().SetResolution(c.Request().Context(), id, strings.TrimSpace(req.Outcome), req.Reason)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: err.Error()})
	}
	return c.JSON(http.StatusOK, semanticDecisionCandidateResponse{Status: true, Record: got})
}
func DeferSemanticDecisionCandidate(c echo.Context) error {
	id, err := candidateID(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: err.Error()})
	}
	var req candidateAction
	if json.NewDecoder(c.Request().Body).Decode(&req) != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body"})
	}
	got, err := candidateStore().DeferCandidate(c.Request().Context(), id, req.DependencyFingerprint, req.Reason, req.By)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: err.Error()})
	}
	return c.JSON(http.StatusOK, semanticDecisionCandidateResponse{Status: true, Record: got})
}
func RetrySemanticDecisionCandidate(c echo.Context) error {
	id, err := candidateID(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: err.Error()})
	}
	var req candidateAction
	if json.NewDecoder(c.Request().Body).Decode(&req) != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body"})
	}
	got, err := candidateStore().RetryDeferred(c.Request().Context(), id, req.DependencyFingerprint, req.By)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: err.Error()})
	}
	return c.JSON(http.StatusOK, semanticDecisionCandidateResponse{Status: true, Record: got})
}
func LinkSemanticDecisionCandidateAssertion(c echo.Context) error {
	id, err := candidateID(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: err.Error()})
	}
	var req candidateAction
	if json.NewDecoder(c.Request().Body).Decode(&req) != nil || req.AssertionID <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "assertion_id is required"})
	}
	got, err := candidateStore().SetResultingAssertion(c.Request().Context(), id, req.AssertionID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: err.Error()})
	}
	return c.JSON(http.StatusOK, semanticDecisionCandidateResponse{Status: true, Record: got})
}
