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

type semanticAssertionListResponse struct {
	Status         bool                   `json:"status"`
	Results        []assertions.Assertion `json:"results"`
	Page, PageSize int                    `json:"page"`
	Total          int64                  `json:"total"`
}
type semanticAssertionResponse struct {
	Status bool                 `json:"status"`
	Record assertions.Assertion `json:"record"`
}
type assertionAction struct {
	To                    string `json:"to"`
	Reason                string `json:"reason"`
	By                    string `json:"by"`
	DependencyFingerprint string `json:"dependency_fingerprint"`
}

func assertionStore() assertions.AssertionStore {
	return assertions.AssertionStore{DB: ApiTypes.ProjectDBHandle}
}

func ListSemanticAssertions(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_SA_001")
	defer rc.Close()
	page, _ := strconv.Atoi(c.QueryParam("page"))
	size, _ := strconv.Atoi(c.QueryParam("page_size"))
	var inputRecordID *int64
	if value, err := strconv.ParseInt(c.QueryParam("input_record_id"), 10, 64); err == nil && value > 0 {
		inputRecordID = &value
	}
	latest := c.QueryParam("latest_only") != "false"
	rows, total, err := assertionStore().ListAdmin(c.Request().Context(), assertions.AssertionListFilter{
		Status: c.QueryParam("status"), LogicalIdentity: c.QueryParam("logical_identity"), SubjectRefKind: c.QueryParam("subject_ref_kind"), SubjectRefID: c.QueryParam("subject_ref_id"), PredicateTermID: c.QueryParam("predicate_term_id"), ObjectRefKind: c.QueryParam("object_ref_kind"), ObjectRefID: c.QueryParam("object_ref_id"), SubjectObjectID: c.QueryParam("subject_object_id"), InputRecordID: inputRecordID, LatestOnly: latest, Page: page, PageSize: size, SortBy: c.QueryParam("sort_by"), SortDir: c.QueryParam("sort_dir"),
	})
	if err != nil {
		rc.GetLogger().Error("list semantic assertions failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to list semantic assertions"})
	}
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 50
	}
	return c.JSON(http.StatusOK, semanticAssertionListResponse{Status: true, Results: rows, Page: page, PageSize: size, Total: total})
}

func GetSemanticAssertion(c echo.Context) error {
	id, err := candidateID(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: err.Error()})
	}
	got, err := assertionStore().GetByID(c.Request().Context(), id)
	if err != nil {
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "semantic assertion not found"})
	}
	return c.JSON(http.StatusOK, semanticAssertionResponse{Status: true, Record: got})
}

func CreateSemanticAssertion(c echo.Context) error {
	var req assertions.Assertion
	if json.NewDecoder(c.Request().Body).Decode(&req) != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body"})
	}
	var got assertions.Assertion
	var err error
	if req.Revision > 0 {
		got, err = assertionStore().CreateRevision(c.Request().Context(), req)
	} else {
		got, err = assertionStore().CreateAssertion(c.Request().Context(), req)
	}
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: err.Error()})
	}
	return c.JSON(http.StatusOK, semanticAssertionResponse{Status: true, Record: got})
}

func TransitionSemanticAssertion(c echo.Context) error {
	id, err := candidateID(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: err.Error()})
	}
	var req assertionAction
	if json.NewDecoder(c.Request().Body).Decode(&req) != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body"})
	}
	got, err := assertionStore().TransitionStatus(c.Request().Context(), id, strings.TrimSpace(req.To), req.Reason, req.By)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: err.Error()})
	}
	return c.JSON(http.StatusOK, semanticAssertionResponse{Status: true, Record: got})
}

func DeferSemanticAssertion(c echo.Context) error {
	id, err := candidateID(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: err.Error()})
	}
	var req assertionAction
	if json.NewDecoder(c.Request().Body).Decode(&req) != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body"})
	}
	got, err := assertionStore().DeferAssertion(c.Request().Context(), id, req.DependencyFingerprint, req.Reason, req.By)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: err.Error()})
	}
	return c.JSON(http.StatusOK, semanticAssertionResponse{Status: true, Record: got})
}

func RetrySemanticAssertion(c echo.Context) error {
	id, err := candidateID(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: err.Error()})
	}
	var req assertionAction
	if json.NewDecoder(c.Request().Body).Decode(&req) != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body"})
	}
	got, err := assertionStore().RetryDeferred(c.Request().Context(), id, req.DependencyFingerprint, req.By)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: err.Error()})
	}
	return c.JSON(http.StatusOK, semanticAssertionResponse{Status: true, Record: got})
}
