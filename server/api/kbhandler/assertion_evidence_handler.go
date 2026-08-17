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

type assertionEvidenceListResponse struct {
	Status         bool                  `json:"status"`
	Results        []assertions.Evidence `json:"results"`
	Page, PageSize int                   `json:"page"`
	Total          int64                 `json:"total"`
}
type assertionEvidenceResponse struct {
	Status bool                `json:"status"`
	Record assertions.Evidence `json:"record"`
}
type evidenceAction struct{ Reason, By string }

func evidenceStore() assertions.EvidenceStore {
	return assertions.EvidenceStore{DB: ApiTypes.ProjectDBHandle, Assertions: assertions.AssertionStore{DB: ApiTypes.ProjectDBHandle}}
}

func ListAssertionEvidence(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_AE_001")
	defer rc.Close()
	page, _ := strconv.Atoi(c.QueryParam("page"))
	size, _ := strconv.Atoi(c.QueryParam("page_size"))
	var inputID *int64
	if raw := strings.TrimSpace(c.QueryParam("input_record_id")); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || v <= 0 {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid input_record_id"})
		}
		inputID = &v
	}
	assertionID, _ := strconv.ParseInt(c.QueryParam("assertion_id"), 10, 64)
	rows, total, err := evidenceStore().ListAdmin(c.Request().Context(), assertions.EvidenceListFilter{AssertionID: assertionID, ArtifactType: c.QueryParam("artifact_type"), ArtifactID: c.QueryParam("artifact_id"), EvidenceRole: c.QueryParam("evidence_role"), ActorKind: c.QueryParam("actor_kind"), InputRecordID: inputID, IncludeDeleted: c.QueryParam("include_deleted") == "true", Page: page, PageSize: size, SortBy: c.QueryParam("sort_by"), SortDir: c.QueryParam("sort_dir")})
	if err != nil {
		rc.GetLogger().Error("list assertion evidence failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to list assertion evidence"})
	}
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 50
	}
	return c.JSON(http.StatusOK, assertionEvidenceListResponse{Status: true, Results: rows, Page: page, PageSize: size, Total: total})
}

func GetAssertionEvidence(c echo.Context) error {
	id, err := candidateID(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: err.Error()})
	}
	got, err := evidenceStore().GetByID(c.Request().Context(), id)
	if err != nil {
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "assertion evidence not found"})
	}
	return c.JSON(http.StatusOK, assertionEvidenceResponse{Status: true, Record: got})
}

func CreateAssertionEvidence(c echo.Context) error {
	var req assertions.Evidence
	if json.NewDecoder(c.Request().Body).Decode(&req) != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body"})
	}
	got, err := evidenceStore().AddEvidence(c.Request().Context(), req)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: err.Error()})
	}
	return c.JSON(http.StatusOK, assertionEvidenceResponse{Status: true, Record: got})
}

func DeleteAssertionEvidence(c echo.Context) error {
	id, err := candidateID(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: err.Error()})
	}
	var req evidenceAction
	if json.NewDecoder(c.Request().Body).Decode(&req) != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body"})
	}
	if err := evidenceStore().DeleteEvidence(c.Request().Context(), id, req.By, req.Reason); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: err.Error()})
	}
	got, err := evidenceStore().GetByID(c.Request().Context(), id)
	if err != nil {
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "assertion evidence not found"})
	}
	return c.JSON(http.StatusOK, assertionEvidenceResponse{Status: true, Record: got})
}

func RestoreAssertionEvidence(c echo.Context) error {
	id, err := candidateID(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: err.Error()})
	}
	got, err := evidenceStore().RestoreEvidence(c.Request().Context(), id)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: err.Error()})
	}
	return c.JSON(http.StatusOK, assertionEvidenceResponse{Status: true, Record: got})
}
