package kbhandler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
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

var artifactObjectReconcileStatuses = map[string]struct{}{
	docprocessing.ObjectReconcilePending:           {},
	docprocessing.ObjectReconcileMatched:           {},
	docprocessing.ObjectReconcileNew:               {},
	docprocessing.ObjectReconcileAmbiguous:         {},
	docprocessing.ObjectReconcileAmbiguousResolved: {},
	docprocessing.ObjectReconcileRejected:          {},
}

// UpdateArtifactObject handles PATCH /api/v1/kb/objects/artifact-objects/:id
// — partial update of one kb.artifact_objects row from the admin resolution
// page. Setting a non-empty object_id also stamps ext_info.reconcile_method
// = "manual_admin" (merged into existing ext_info, not overwritten) so
// provenance distinguishes manual resolutions from the automated backfill's
// tie_break_deterministic / exact_name / lexical_name / new_node methods.
func UpdateArtifactObject(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_AAO_200")
	defer rc.Close()
	logger := rc.GetLogger()

	idStr := strings.TrimSpace(c.Param("id"))
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid id (CWB_KB_AAO_201)"})
	}

	var payload map[string]json.RawMessage
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_AAO_202)"})
	}

	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "db not initialized (CWB_KB_AAO_210)"})
	}

	sets := make([]string, 0, len(payload)+1)
	args := make([]any, 0, len(payload)+2)
	addSet := func(column string, value any) {
		args = append(args, value)
		sets = append(sets, fmt.Sprintf("%s = $%d", column, len(args)))
	}

	fields := make([]string, 0, len(payload))
	for field := range payload {
		fields = append(fields, field)
	}
	sort.Strings(fields)

	var settingObjectID bool
	for _, field := range fields {
		raw := payload[field]
		switch field {
		case "object_name", "object_type", "object_role", "reconcile_status":
			value, err := decodeStringValue(raw, true)
			if err != nil || value == nil {
				return c.JSON(http.StatusBadRequest, errorResponse{
					Status: false, ErrorMsg: fmt.Sprintf("%s cannot be null (CWB_KB_AAO_211)", field),
				})
			}
			if field == "reconcile_status" {
				if _, ok := artifactObjectReconcileStatuses[*value]; !ok {
					return c.JSON(http.StatusBadRequest, errorResponse{
						Status: false, ErrorMsg: fmt.Sprintf("invalid reconcile_status %q (CWB_KB_AAO_212)", *value),
					})
				}
			}
			addSet(field, *value)

		case "object_name_en", "object_name_zh", "language", "description", "evidence_quote":
			value, err := decodeStringValue(raw, true)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{
					Status: false, ErrorMsg: fmt.Sprintf("invalid %s: %v (CWB_KB_AAO_213)", field, err),
				})
			}
			if value == nil || *value == "" {
				addSet(field, nil)
			} else {
				addSet(field, *value)
			}

		case "object_id":
			value, err := decodeStringValue(raw, true)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{
					Status: false, ErrorMsg: fmt.Sprintf("invalid object_id: %v (CWB_KB_AAO_214)", err),
				})
			}
			if value == nil || *value == "" {
				addSet("object_id", nil)
			} else {
				addSet("object_id", *value)
				settingObjectID = true
			}

		case "aliases", "acronyms":
			if strings.TrimSpace(string(raw)) == "null" {
				addSet(field, "[]")
				break
			}
			compact, err := compactJSONRaw(raw)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{
					Status: false, ErrorMsg: fmt.Sprintf("invalid %s: %v (CWB_KB_AAO_215)", field, err),
				})
			}
			addSet(field, compact)

		case "reconcile_confidence":
			value, err := decodeFloat64Value(raw)
			if err != nil || value == nil {
				return c.JSON(http.StatusBadRequest, errorResponse{
					Status: false, ErrorMsg: fmt.Sprintf("invalid reconcile_confidence: %v (CWB_KB_AAO_216)", err),
				})
			}
			addSet(field, *value)
		}
	}

	if settingObjectID {
		args = append(args, `{"reconcile_method":"manual_admin"}`)
		sets = append(sets, fmt.Sprintf("ext_info = COALESCE(ext_info, '{}'::jsonb) || $%d::jsonb", len(args)))
	}

	if len(sets) == 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "no editable fields in request (CWB_KB_AAO_217)"})
	}

	query := fmt.Sprintf("UPDATE kb.artifact_objects SET %s WHERE id = $%d", strings.Join(sets, ", "), len(args)+1)
	args = append(args, id)
	result, err := db.Exec(query, args...)
	if err != nil {
		logger.Error("update artifact object failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to update artifact object (CWB_KB_AAO_218)"})
	}
	affected, err := result.RowsAffected()
	if err != nil {
		logger.Error("rows affected artifact object failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to verify artifact object update (CWB_KB_AAO_219)"})
	}
	if affected == 0 {
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "artifact object not found (CWB_KB_AAO_220)"})
	}

	action := objectAuditActionEditFields
	if settingObjectID {
		action = objectAuditActionResolveObjectID
	}
	logObjectAudit(c.Request().Context(), db, logger, "kb.artifact_objects", idStr, action, structureActor(rc), payload)

	return c.JSON(http.StatusOK, map[string]any{"status": true})
}

// UpdateObjectNode handles PATCH /api/v1/kb/object-nodes/:object_id —
// partial update of one kb.object_nodes candidate row from the admin
// resolution page.
func UpdateObjectNode(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_AAO_300")
	defer rc.Close()
	logger := rc.GetLogger()

	objectID := strings.TrimSpace(c.Param("object_id"))
	if objectID == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid object_id (CWB_KB_AAO_301)"})
	}

	var payload map[string]json.RawMessage
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_AAO_302)"})
	}

	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "db not initialized (CWB_KB_AAO_310)"})
	}

	sets := make([]string, 0, len(payload))
	args := make([]any, 0, len(payload)+1)
	addSet := func(column string, value any) {
		args = append(args, value)
		sets = append(sets, fmt.Sprintf("%s = $%d", column, len(args)))
	}

	fields := make([]string, 0, len(payload))
	for field := range payload {
		fields = append(fields, field)
	}
	sort.Strings(fields)

	for _, field := range fields {
		raw := payload[field]
		switch field {
		case "canonical_name", "object_type":
			value, err := decodeStringValue(raw, true)
			if err != nil || value == nil {
				return c.JSON(http.StatusBadRequest, errorResponse{
					Status: false, ErrorMsg: fmt.Sprintf("%s cannot be null (CWB_KB_AAO_311)", field),
				})
			}
			addSet(field, *value)

		case "canonical_name_en", "canonical_name_zh", "primary_language", "description":
			value, err := decodeStringValue(raw, true)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{
					Status: false, ErrorMsg: fmt.Sprintf("invalid %s: %v (CWB_KB_AAO_312)", field, err),
				})
			}
			if value == nil || *value == "" {
				addSet(field, nil)
			} else {
				addSet(field, *value)
			}

		case "aliases", "acronyms":
			if strings.TrimSpace(string(raw)) == "null" {
				addSet(field, "[]")
				break
			}
			compact, err := compactJSONRaw(raw)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{
					Status: false, ErrorMsg: fmt.Sprintf("invalid %s: %v (CWB_KB_AAO_313)", field, err),
				})
			}
			addSet(field, compact)
		}
	}

	if len(sets) == 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "no editable fields in request (CWB_KB_AAO_314)"})
	}

	query := fmt.Sprintf("UPDATE kb.object_nodes SET %s WHERE object_id = $%d", strings.Join(sets, ", "), len(args)+1)
	args = append(args, objectID)
	result, err := db.Exec(query, args...)
	if err != nil {
		logger.Error("update object node failed", "object_id", objectID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to update object node (CWB_KB_AAO_315)"})
	}
	affected, err := result.RowsAffected()
	if err != nil {
		logger.Error("rows affected object node failed", "object_id", objectID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to verify object node update (CWB_KB_AAO_316)"})
	}
	if affected == 0 {
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "object node not found (CWB_KB_AAO_317)"})
	}

	logObjectAudit(c.Request().Context(), db, logger, "kb.object_nodes", objectID, objectAuditActionEditFields, structureActor(rc), payload)

	return c.JSON(http.StatusOK, map[string]any{"status": true})
}
