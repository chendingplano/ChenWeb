package kbhandler

import (
	"encoding/json"
	"net/http"

	"github.com/chendingplano/deepdoc/server/api/ontology/profiles"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

type ontologyReviewScopeResponse struct {
	Status bool                 `json:"status"`
	Record profiles.ReviewScope `json:"record"`
}

type ontologyReviewExecutionResponse struct {
	Status  bool                            `json:"status"`
	Results []profiles.RuleEvaluationResult `json:"results"`
}

// ExecuteOntologyReviewScope evaluates an already-frozen scope. The rule
// loader resolves only releases pinned in that scope, never current activation.
func ExecuteOntologyReviewScope(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_ORS_300")
	defer rc.Close()
	var payload struct {
		InputRecordID int64                      `json:"input_record_id"`
		RunID         int64                      `json:"run_id"`
		Assertions    []profiles.ReviewAssertion `json:"assertions"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil || payload.InputRecordID == 0 || payload.RunID == 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "input_record_id and run_id are required (CWB_KB_ORS_301)"})
	}
	db := ApiTypes.ProjectDBHandle
	scope, err := (profiles.ReviewScopeStore{DB: db}).Get(c.Request().Context(), c.Param("scope_id"))
	if err != nil {
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "ontology review scope not found (CWB_KB_ORS_302)"})
	}
	service := profiles.ReviewService{Findings: profiles.FindingStore{DB: db}, Rules: profiles.ProfileRuleStore{DB: db}}
	results, err := service.EvaluatePinnedScope(c.Request().Context(), scope, payload.Assertions, payload.InputRecordID, payload.RunID)
	if err != nil {
		rc.GetLogger().Error("execute ontology review scope failed", "err", err)
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "failed to execute frozen ontology review scope (CWB_KB_ORS_303)"})
	}
	return c.JSON(http.StatusOK, ontologyReviewExecutionResponse{Status: true, Results: results})
}

// CreateOntologyReviewScope freezes an explicit or deterministic selection
// before any review execution. The endpoint deliberately exposes no update
// operation: a changed selection requires a new scope id.
func CreateOntologyReviewScope(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_ORS_100")
	defer rc.Close()
	var scope profiles.ReviewScope
	if err := json.NewDecoder(c.Request().Body).Decode(&scope); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_ORS_101)"})
	}
	created, err := (profiles.ReviewScopeStore{DB: ApiTypes.ProjectDBHandle}).Create(c.Request().Context(), scope)
	if err != nil {
		rc.GetLogger().Error("create ontology review scope failed", "err", err)
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "failed to create immutable review scope (CWB_KB_ORS_102)"})
	}
	return c.JSON(http.StatusOK, ontologyReviewScopeResponse{Status: true, Record: created})
}

// GetOntologyReviewScope returns the stored selection facts for a historical
// or future deterministic review.
func GetOntologyReviewScope(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_ORS_200")
	defer rc.Close()
	got, err := (profiles.ReviewScopeStore{DB: ApiTypes.ProjectDBHandle}).Get(c.Request().Context(), c.Param("scope_id"))
	if err != nil {
		rc.GetLogger().Error("get ontology review scope failed", "err", err)
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "ontology review scope not found (CWB_KB_ORS_201)"})
	}
	return c.JSON(http.StatusOK, ontologyReviewScopeResponse{Status: true, Record: got})
}
