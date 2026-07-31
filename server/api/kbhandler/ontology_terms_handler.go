package kbhandler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/chendingplano/deepdoc/server/api/ontology/terms"
	"github.com/labstack/echo/v4"
)

type ontologyTermListResponse struct {
	Status  bool         `json:"status"`
	Results []terms.Term `json:"results"`
	Total   int          `json:"total"`
}

type ontologyTermDetailResponse struct {
	Status bool       `json:"status"`
	Record terms.Term `json:"record"`
}

type ontologyLabelListResponse struct {
	Status  bool           `json:"status"`
	Results []terms.TermLabel `json:"results"`
	Total   int            `json:"total"`
}

type ontologyLabelDetailResponse struct {
	Status bool            `json:"status"`
	Record terms.TermLabel `json:"record"`
}

// ListOntologyTerms handles GET /api/v1/kb/ontology/terms?module_id=&status=.
func ListOntologyTerms(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_OT_001")
	defer rc.Close()
	logger := rc.GetLogger()

	store := terms.TermStore{DB: ApiTypes.ProjectDBHandle}
	list, err := store.ListTerms(c.Request().Context(), c.QueryParam("module_id"), c.QueryParam("status"))
	if err != nil {
		logger.Error("list ontology terms failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to retrieve ontology terms (CWB_KB_OT_002)"})
	}
	return c.JSON(http.StatusOK, ontologyTermListResponse{Status: true, Results: list, Total: len(list)})
}

// CreateOntologyTerm handles POST /api/v1/kb/ontology/terms, authoring
// version 1 of a governed term.
func CreateOntologyTerm(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_OT_100")
	defer rc.Close()
	logger := rc.GetLogger()

	var payload struct {
		TermID     string `json:"term_id"`
		TermKind   string `json:"term_kind"`
		ModuleID   string `json:"module_id"`
		Status     string `json:"status"`
		Definition string `json:"definition"`
		Scope      string `json:"scope"`
		CreateBy   string `json:"create_by"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_OT_101)"})
	}
	if strings.TrimSpace(payload.TermID) == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "term_id is required (CWB_KB_OT_102)"})
	}
	if !terms.AllowedTermKinds[payload.TermKind] {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "unsupported term_kind (CWB_KB_OT_103)"})
	}

	store := terms.TermStore{DB: ApiTypes.ProjectDBHandle}
	created, err := store.CreateTerm(c.Request().Context(), terms.Term{
		TermID:     strings.TrimSpace(payload.TermID),
		TermKind:   payload.TermKind,
		ModuleID:   strings.TrimSpace(payload.ModuleID),
		Status:     payload.Status,
		Definition: payload.Definition,
		Scope:      payload.Scope,
		CreateBy:   payload.CreateBy,
		ModifyBy:   payload.CreateBy,
	})
	if err != nil {
		logger.Error("create ontology term failed", "term_id", payload.TermID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to create ontology term (CWB_KB_OT_104)"})
	}
	return c.JSON(http.StatusOK, ontologyTermDetailResponse{Status: true, Record: created})
}

// GetOntologyTerm handles GET /api/v1/kb/ontology/terms/:term_id, returning
// the latest version.
func GetOntologyTerm(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_OT_200")
	defer rc.Close()
	logger := rc.GetLogger()

	termID := strings.TrimSpace(c.Param("term_id"))
	if termID == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid term_id (CWB_KB_OT_201)"})
	}
	store := terms.TermStore{DB: ApiTypes.ProjectDBHandle}
	got, err := store.GetTermLatest(c.Request().Context(), termID)
	if err != nil {
		logger.Error("get ontology term failed", "term_id", termID, "err", err)
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "ontology term not found (CWB_KB_OT_202)"})
	}
	return c.JSON(http.StatusOK, ontologyTermDetailResponse{Status: true, Record: got})
}

// CreateOntologyTermVersion handles POST /api/v1/kb/ontology/terms/:term_id/versions,
// inserting the next version row for the term (an accepted change preserves
// every prior version).
func CreateOntologyTermVersion(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_OT_300")
	defer rc.Close()
	logger := rc.GetLogger()

	termID := strings.TrimSpace(c.Param("term_id"))
	if termID == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid term_id (CWB_KB_OT_301)"})
	}
	var payload struct {
		TermKind   string `json:"term_kind"`
		ModuleID   string `json:"module_id"`
		Status     string `json:"status"`
		Definition string `json:"definition"`
		Scope      string `json:"scope"`
		CreateBy   string `json:"create_by"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_OT_302)"})
	}
	store := terms.TermStore{DB: ApiTypes.ProjectDBHandle}
	created, err := store.CreateTermVersion(c.Request().Context(), terms.Term{
		TermID:     termID,
		TermKind:   payload.TermKind,
		ModuleID:   strings.TrimSpace(payload.ModuleID),
		Status:     payload.Status,
		Definition: payload.Definition,
		Scope:      payload.Scope,
		CreateBy:   payload.CreateBy,
		ModifyBy:   payload.CreateBy,
	})
	if err != nil {
		logger.Error("create ontology term version failed", "term_id", termID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to create ontology term version (CWB_KB_OT_303)"})
	}
	return c.JSON(http.StatusOK, ontologyTermDetailResponse{Status: true, Record: created})
}

// TransitionOntologyTermStatus handles POST
// /api/v1/kb/ontology/terms/:term_id/:version/status, moving a content row
// along the governed lifecycle (spec §9.3).
func TransitionOntologyTermStatus(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_OT_400")
	defer rc.Close()
	logger := rc.GetLogger()

	termID := strings.TrimSpace(c.Param("term_id"))
	version, err := strconv.Atoi(strings.TrimSpace(c.Param("version")))
	if err != nil || version < 1 || termID == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid term_id or version (CWB_KB_OT_401)"})
	}
	var payload struct {
		To string `json:"to"`
		By string `json:"by"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_OT_402)"})
	}
	store := terms.TermStore{DB: ApiTypes.ProjectDBHandle}
	got, err := store.TransitionStatus(c.Request().Context(), termID, version, strings.TrimSpace(payload.To), payload.By)
	if err != nil {
		logger.Error("transition ontology term status failed", "term_id", termID, "version", version, "to", payload.To, "err", err)
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "failed to transition ontology term status (CWB_KB_OT_403)"})
	}
	return c.JSON(http.StatusOK, ontologyTermDetailResponse{Status: true, Record: got})
}

// CreateOntologyTermLabel handles POST /api/v1/kb/ontology/terms/:term_id/labels.
func CreateOntologyTermLabel(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_OT_500")
	defer rc.Close()
	logger := rc.GetLogger()

	termID := strings.TrimSpace(c.Param("term_id"))
	if termID == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid term_id (CWB_KB_OT_501)"})
	}
	var payload struct {
		Label     string `json:"label"`
		Lang      string `json:"lang"`
		LabelRole string `json:"label_role"`
		Status    string `json:"status"`
		CreateBy  string `json:"create_by"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_OT_502)"})
	}
	store := terms.LabelStore{DB: ApiTypes.ProjectDBHandle}
	created, err := store.CreateLabel(c.Request().Context(), terms.TermLabel{
		TermID:    termID,
		Label:     strings.TrimSpace(payload.Label),
		Lang:      payload.Lang,
		LabelRole: payload.LabelRole,
		Status:    payload.Status,
		CreateBy:  payload.CreateBy,
		ModifyBy:  payload.CreateBy,
	})
	if err != nil {
		logger.Error("create ontology term label failed", "term_id", termID, "err", err)
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "failed to create ontology term label (CWB_KB_OT_503)"})
	}
	return c.JSON(http.StatusOK, ontologyLabelDetailResponse{Status: true, Record: created})
}

// ListOntologyTermLabels handles GET /api/v1/kb/ontology/terms/:term_id/labels.
func ListOntologyTermLabels(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_OT_600")
	defer rc.Close()
	logger := rc.GetLogger()

	termID := strings.TrimSpace(c.Param("term_id"))
	if termID == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid term_id (CWB_KB_OT_601)"})
	}
	store := terms.LabelStore{DB: ApiTypes.ProjectDBHandle}
	list, err := store.ListLabels(c.Request().Context(), termID)
	if err != nil {
		logger.Error("list ontology term labels failed", "term_id", termID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to retrieve ontology term labels (CWB_KB_OT_602)"})
	}
	return c.JSON(http.StatusOK, ontologyLabelListResponse{Status: true, Results: list, Total: len(list)})
}
