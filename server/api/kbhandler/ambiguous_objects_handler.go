package kbhandler

import (
	"net/http"
	"strconv"
	"strings"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

// artifactObjectDTO is the wire shape for one kb.artifact_objects row on the
// Resolve Ambiguous Objects admin page.
type artifactObjectDTO struct {
	ID                  int64    `json:"id"`
	SourceRecordID      int64    `json:"source_record_id"`
	ArtifactType        string   `json:"artifact_type"`
	ArtifactID          string   `json:"artifact_id"`
	ObjectName          string   `json:"object_name"`
	ObjectNameEn        string   `json:"object_name_en"`
	ObjectNameZh        string   `json:"object_name_zh"`
	Language            string   `json:"language"`
	ObjectType          string   `json:"object_type"`
	ObjectRole          string   `json:"object_role"`
	Aliases             []string `json:"aliases"`
	Acronyms            []string `json:"acronyms"`
	Description         string   `json:"description"`
	EvidenceQuote       string   `json:"evidence_quote"`
	ObjectID            string   `json:"object_id"`
	ReconcileStatus     string   `json:"reconcile_status"`
	ReconcileConfidence float64  `json:"reconcile_confidence"`
}

// objectNodeCandidateDTO is the wire shape for one candidate kb.object_nodes
// row shown alongside an ambiguous artifact_object.
type objectNodeCandidateDTO struct {
	ObjectID        string   `json:"object_id"`
	CanonicalName   string   `json:"canonical_name"`
	CanonicalNameEn string   `json:"canonical_name_en"`
	CanonicalNameZh string   `json:"canonical_name_zh"`
	PrimaryLanguage string   `json:"primary_language"`
	ObjectType      string   `json:"object_type"`
	Aliases         []string `json:"aliases"`
	Acronyms        []string `json:"acronyms"`
	Description     string   `json:"description"`
	Score           float64  `json:"score"`
	Method          string   `json:"method"`
	Recommended     bool     `json:"recommended"`
}

// ambiguousObjectSummaryDTO is the wire shape for the left-panel list.
type ambiguousObjectSummaryDTO struct {
	ID           int64   `json:"id"`
	ArtifactType string  `json:"artifact_type"`
	ArtifactID   string  `json:"artifact_id"`
	ObjectName   string  `json:"object_name"`
	ObjectNameEn string  `json:"object_name_en"`
	Confidence   float64 `json:"confidence"`
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func toArtifactObjectDTO(obj docprocessing.ArtifactObject) artifactObjectDTO {
	return artifactObjectDTO{
		ID:                  obj.ID,
		SourceRecordID:      obj.SourceRecordID,
		ArtifactType:        obj.ArtifactType,
		ArtifactID:          obj.ArtifactID,
		ObjectName:          obj.ObjectName,
		ObjectNameEn:        obj.ObjectNameEn,
		ObjectNameZh:        obj.ObjectNameZh,
		Language:            obj.Language,
		ObjectType:          obj.ObjectType,
		ObjectRole:          obj.ObjectRole,
		Aliases:             nonNilStrings(obj.Aliases),
		Acronyms:            nonNilStrings(obj.Acronyms),
		Description:         obj.Description,
		EvidenceQuote:       obj.EvidenceQuote,
		ObjectID:            obj.ObjectID,
		ReconcileStatus:     obj.ReconcileStatus,
		ReconcileConfidence: obj.ReconcileConfidence,
	}
}

func toObjectNodeCandidateDTO(c docprocessing.ObjectNodeCandidate, recommendedID string) objectNodeCandidateDTO {
	return objectNodeCandidateDTO{
		ObjectID:        c.Node.ObjectID,
		CanonicalName:   c.Node.CanonicalName,
		CanonicalNameEn: c.Node.CanonicalNameEn,
		CanonicalNameZh: c.Node.CanonicalNameZh,
		PrimaryLanguage: c.Node.PrimaryLanguage,
		ObjectType:      c.Node.ObjectType,
		Aliases:         nonNilStrings(c.Node.Aliases),
		Acronyms:        nonNilStrings(c.Node.Acronyms),
		Description:     c.Node.Description,
		Score:           c.Score,
		Method:          c.Method,
		Recommended:     recommendedID != "" && c.Node.ObjectID == recommendedID,
	}
}

// ListAmbiguousObjects handles GET /api/v1/kb/objects/ambiguous — the
// left-panel list for the Resolve Ambiguous Objects admin page.
func ListAmbiguousObjects(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_AAO_001")
	defer rc.Close()
	logger := rc.GetLogger()

	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "db not initialized (CWB_KB_AAO_010)"})
	}

	store := docprocessing.ArtifactObjectSQLStore{DB: db}
	rows, err := store.ListAmbiguousSummaries(c.Request().Context())
	if err != nil {
		logger.Error("list ambiguous objects failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to list ambiguous objects (CWB_KB_AAO_011)"})
	}

	out := make([]ambiguousObjectSummaryDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, ambiguousObjectSummaryDTO{
			ID:           row.ID,
			ArtifactType: row.ArtifactType,
			ArtifactID:   row.ArtifactID,
			ObjectName:   row.ObjectName,
			ObjectNameEn: row.ObjectNameEn,
			Confidence:   row.Confidence,
		})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "rows": out})
}

// GetAmbiguousObjectDetail handles GET /api/v1/kb/objects/ambiguous/:id — the
// right-panel detail (artifact object + ranked candidate object nodes) for
// one ambiguous row.
func GetAmbiguousObjectDetail(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_AAO_100")
	defer rc.Close()
	logger := rc.GetLogger()

	idStr := strings.TrimSpace(c.Param("id"))
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid id (CWB_KB_AAO_101)"})
	}

	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "db not initialized (CWB_KB_AAO_110)"})
	}

	store := docprocessing.ArtifactObjectSQLStore{DB: db}
	obj, found, err := store.LoadByID(c.Request().Context(), id)
	if err != nil {
		logger.Error("load artifact object failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to load artifact object (CWB_KB_AAO_111)"})
	}
	if !found {
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "artifact object not found (CWB_KB_AAO_112)"})
	}

	reconciler := docprocessing.ObjectReconciler{
		Store:   docprocessing.ObjectNodeSQLStore{DB: db},
		Options: docprocessing.ObjectReconcileOptionsFromEnv(),
	}
	candidates, recommendedID, err := docprocessing.RankAmbiguousCandidates(c.Request().Context(), reconciler, obj)
	if err != nil {
		logger.Error("rank ambiguous candidates failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to load candidates (CWB_KB_AAO_113)"})
	}

	candidateDTOs := make([]objectNodeCandidateDTO, 0, len(candidates))
	for _, cand := range candidates {
		candidateDTOs = append(candidateDTOs, toObjectNodeCandidateDTO(cand, recommendedID))
	}

	return c.JSON(http.StatusOK, map[string]any{
		"status":          true,
		"artifact_object": toArtifactObjectDTO(obj),
		"candidates":      candidateDTOs,
	})
}
