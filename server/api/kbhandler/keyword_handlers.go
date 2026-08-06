package kbhandler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/ontology/keywords"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

// -- Concept handlers ---------------------------------------------------------

// ListKeywordConcepts handles GET /api/v1/kb/keyword-concepts
func ListKeywordConcepts(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_KW_001")
	defer rc.Close()
	logger := rc.GetLogger()

	store := keywords.ConceptStore{DB: ApiTypes.ProjectDBHandle}
	list, err := store.ListConcepts(c.Request().Context(), c.QueryParam("scope"))
	if err != nil {
		logger.Error("list keyword concepts failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to list keyword concepts (CWB_KB_KW_002)"})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "results": list, "total": len(list)})
}

// CreateKeywordConcept handles POST /api/v1/kb/keyword-concepts
func CreateKeywordConcept(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_KW_010")
	defer rc.Close()
	logger := rc.GetLogger()

	var payload struct {
		ConceptID string  `json:"concept_id"`
		PrefLabel string  `json:"pref_label"`
		Gloss     *string `json:"gloss"`
		Scope     string  `json:"scope"`
		Status    string  `json:"status"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_KW_011)"})
	}
	if strings.TrimSpace(payload.ConceptID) == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "concept_id is required (CWB_KB_KW_012)"})
	}
	if strings.TrimSpace(payload.PrefLabel) == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "pref_label is required (CWB_KB_KW_013)"})
	}

	store := keywords.ConceptStore{DB: ApiTypes.ProjectDBHandle}
	created, err := store.CreateConcept(c.Request().Context(), keywords.Concept{
		ConceptID: strings.TrimSpace(payload.ConceptID),
		PrefLabel: strings.TrimSpace(payload.PrefLabel),
		Gloss:     payload.Gloss,
		Scope:     payload.Scope,
		Status:    payload.Status,
	})
	if err != nil {
		logger.Error("create keyword concept failed", "concept_id", payload.ConceptID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to create keyword concept (CWB_KB_KW_014)"})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "record": created})
}

// GetKeywordConcept handles GET /api/v1/kb/keyword-concepts/:concept_id
func GetKeywordConcept(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_KW_020")
	defer rc.Close()
	logger := rc.GetLogger()

	conceptID := strings.TrimSpace(c.Param("concept_id"))
	if conceptID == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid concept_id (CWB_KB_KW_021)"})
	}
	store := keywords.ConceptStore{DB: ApiTypes.ProjectDBHandle}
	got, err := store.GetConcept(c.Request().Context(), conceptID)
	if err != nil {
		logger.Error("get keyword concept failed", "concept_id", conceptID, "err", err)
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "keyword concept not found (CWB_KB_KW_022)"})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "record": got})
}

// UpdateKeywordConcept handles PUT /api/v1/kb/keyword-concepts/:concept_id
func UpdateKeywordConcept(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_KW_030")
	defer rc.Close()
	logger := rc.GetLogger()

	conceptID := strings.TrimSpace(c.Param("concept_id"))
	if conceptID == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid concept_id (CWB_KB_KW_031)"})
	}
	var payload struct {
		PrefLabel string `json:"pref_label"`
		Gloss     string `json:"gloss"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_KW_032)"})
	}

	store := keywords.ConceptStore{DB: ApiTypes.ProjectDBHandle}
	updated, err := store.UpdateConceptLabel(c.Request().Context(), conceptID, payload.PrefLabel, payload.Gloss)
	if err != nil {
		logger.Error("update keyword concept failed", "concept_id", conceptID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to update keyword concept (CWB_KB_KW_033)"})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "record": updated})
}

// TransitionKeywordConceptStatus handles POST /api/v1/kb/keyword-concepts/:concept_id/status
func TransitionKeywordConceptStatus(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_KW_040")
	defer rc.Close()
	logger := rc.GetLogger()

	conceptID := strings.TrimSpace(c.Param("concept_id"))
	if conceptID == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid concept_id (CWB_KB_KW_041)"})
	}
	var payload struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_KW_042)"})
	}

	store := keywords.ConceptStore{DB: ApiTypes.ProjectDBHandle}
	updated, err := store.TransitionStatus(c.Request().Context(), conceptID, payload.Status)
	if err != nil {
		logger.Error("transition keyword concept failed", "concept_id", conceptID, "to", payload.Status, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to transition keyword concept (CWB_KB_KW_043)"})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "record": updated})
}

// MergeKeywordConcept handles POST /api/v1/kb/keyword-concepts/:concept_id/merge
func MergeKeywordConcept(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_KW_050")
	defer rc.Close()
	logger := rc.GetLogger()

	conceptID := strings.TrimSpace(c.Param("concept_id"))
	if conceptID == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid concept_id (CWB_KB_KW_051)"})
	}
	var payload struct {
		TargetID string `json:"target_id"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_KW_052)"})
	}

	store := keywords.ConceptStore{DB: ApiTypes.ProjectDBHandle}
	merged, err := store.MergeConcept(c.Request().Context(), conceptID, payload.TargetID)
	if err != nil {
		logger.Error("merge keyword concept failed", "from", conceptID, "to", payload.TargetID, "err", err)
		return conceptStoreError(c, err, "failed to merge keyword concept (CWB_KB_KW_053)")
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "message": "merged", "record": merged})
}

// UnmergeKeywordConcept handles POST /api/v1/kb/keyword-concepts/:concept_id/unmerge
// It reverses a merge (§14.4): surfaces whose origin_concept is the concept
// move back to it, and its tombstone is cleared with the restore status.
func UnmergeKeywordConcept(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_KW_060")
	defer rc.Close()
	logger := rc.GetLogger()

	conceptID := strings.TrimSpace(c.Param("concept_id"))
	if conceptID == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid concept_id (CWB_KB_KW_061)"})
	}
	var payload struct {
		Status string `json:"status"`
	}
	_ = json.NewDecoder(c.Request().Body).Decode(&payload)
	if payload.Status == "" {
		payload.Status = "active"
	}

	store := keywords.ConceptStore{DB: ApiTypes.ProjectDBHandle}
	restored, err := store.UnmergeConcept(c.Request().Context(), conceptID, payload.Status)
	if err != nil {
		logger.Error("unmerge keyword concept failed", "concept_id", conceptID, "err", err)
		return conceptStoreError(c, err, "failed to unmerge keyword concept (CWB_KB_KW_062)")
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "message": "unmerged", "record": restored})
}

// conceptStoreError maps concept-store guardrail errors to HTTP statuses:
// sql.ErrNoRows → 404, keywords.ErrMergeRejected → 409 with the guardrail
// reason, anything else → 500.
func conceptStoreError(c echo.Context, err error, fallback string) error {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "keyword concept not found (CWB_KB_KW_054)"})
	case errors.Is(err, keywords.ErrMergeRejected):
		return c.JSON(http.StatusConflict, errorResponse{Status: false, ErrorMsg: err.Error()})
	default:
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: fallback})
	}
}

// -- Surface handlers ---------------------------------------------------------

// CreateKeywordSurface handles POST /api/v1/kb/keyword-surfaces
func CreateKeywordSurface(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_KW_100")
	defer rc.Close()
	logger := rc.GetLogger()

	var payload struct {
		ConceptID  string  `json:"concept_id"`
		Surface    string  `json:"surface"`
		LabelRole  string  `json:"label_role"`
		AliasType  string  `json:"alias_type"`
		Lang       string  `json:"lang"`
		Scope      string  `json:"scope"`
		Confidence float64 `json:"confidence"`
		Provenance string  `json:"provenance"`
		Locked     bool    `json:"locked"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_KW_101)"})
	}

	// K3/D6: norm_key and norm_version are not accepted — the server derives
	// every key from the surface.
	store := keywords.SurfaceStore{DB: ApiTypes.ProjectDBHandle}
	created, err := store.CreateSurface(c.Request().Context(), keywords.Surface{
		ConceptID:  payload.ConceptID,
		Surface:    payload.Surface,
		LabelRole:  payload.LabelRole,
		AliasType:  payload.AliasType,
		Lang:       payload.Lang,
		Scope:      payload.Scope,
		Confidence: payload.Confidence,
		Provenance: payload.Provenance,
		Locked:     payload.Locked,
	})
	if err != nil {
		logger.Error("create keyword surface failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to create keyword surface (CWB_KB_KW_102)"})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "record": created})
}

// GetKeywordSurface handles GET /api/v1/kb/keyword-surfaces/:surface_id
func GetKeywordSurface(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_KW_110")
	defer rc.Close()
	logger := rc.GetLogger()

	surfaceID := strings.TrimSpace(c.Param("surface_id"))
	if surfaceID == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid surface_id (CWB_KB_KW_111)"})
	}
	store := keywords.SurfaceStore{DB: ApiTypes.ProjectDBHandle}
	got, err := store.GetSurface(c.Request().Context(), surfaceID)
	if err != nil {
		logger.Error("get keyword surface failed", "surface_id", surfaceID, "err", err)
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "keyword surface not found (CWB_KB_KW_112)"})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "record": got})
}

// ListKeywordSurfacesByConcept handles GET /api/v1/kb/keyword-concepts/:concept_id/surfaces
func ListKeywordSurfacesByConcept(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_KW_120")
	defer rc.Close()
	logger := rc.GetLogger()

	conceptID := strings.TrimSpace(c.Param("concept_id"))
	if conceptID == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid concept_id (CWB_KB_KW_121)"})
	}
	store := keywords.SurfaceStore{DB: ApiTypes.ProjectDBHandle}
	list, err := store.ListSurfacesByConcept(c.Request().Context(), conceptID)
	if err != nil {
		logger.Error("list keyword surfaces failed", "concept_id", conceptID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to list keyword surfaces (CWB_KB_KW_122)"})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "results": list, "total": len(list)})
}

// LockKeywordSurface handles PUT /api/v1/kb/keyword-surfaces/:surface_id/lock
func LockKeywordSurface(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_KW_130")
	defer rc.Close()
	logger := rc.GetLogger()

	surfaceID := strings.TrimSpace(c.Param("surface_id"))
	if surfaceID == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid surface_id (CWB_KB_KW_131)"})
	}
	var payload struct {
		Locked bool `json:"locked"`
	}
	_ = json.NewDecoder(c.Request().Body).Decode(&payload)

	store := keywords.SurfaceStore{DB: ApiTypes.ProjectDBHandle}
	if err := store.UpdateSurfaceLock(c.Request().Context(), surfaceID, payload.Locked); err != nil {
		logger.Error("lock keyword surface failed", "surface_id", surfaceID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to lock keyword surface (CWB_KB_KW_132)"})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "message": "locked"})
}

// -- Rewrite rule handlers ----------------------------------------------------

// CreateKeywordRewriteRule handles POST /api/v1/kb/keyword-rewrite-rules
func CreateKeywordRewriteRule(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_KW_200")
	defer rc.Close()
	logger := rc.GetLogger()

	var payload struct {
		RuleID      string `json:"rule_id"`
		Pattern     string `json:"pattern"`
		Replacement string `json:"replacement"`
		Scope       string `json:"scope"`
		Provenance  string `json:"provenance"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_KW_201)"})
	}

	store := keywords.RewriteRuleStore{DB: ApiTypes.ProjectDBHandle}
	created, err := store.CreateRule(c.Request().Context(), keywords.RewriteRule{
		RuleID:      payload.RuleID,
		Pattern:     payload.Pattern,
		Replacement: payload.Replacement,
		Scope:       payload.Scope,
		Provenance:  payload.Provenance,
	})
	if err != nil {
		logger.Error("create keyword rewrite rule failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to create keyword rewrite rule (CWB_KB_KW_202)"})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "record": created})
}

// ListKeywordRewriteRules handles GET /api/v1/kb/keyword-rewrite-rules
func ListKeywordRewriteRules(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_KW_210")
	defer rc.Close()
	logger := rc.GetLogger()

	store := keywords.RewriteRuleStore{DB: ApiTypes.ProjectDBHandle}
	list, err := store.ListEnabledRules(c.Request().Context(), c.QueryParam("scope"))
	if err != nil {
		logger.Error("list keyword rewrite rules failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to list keyword rewrite rules (CWB_KB_KW_211)"})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "results": list, "total": len(list)})
}

// ToggleKeywordRewriteRule handles PUT /api/v1/kb/keyword-rewrite-rules/:rule_id/enabled
func ToggleKeywordRewriteRule(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_KW_220")
	defer rc.Close()
	logger := rc.GetLogger()

	ruleID := strings.TrimSpace(c.Param("rule_id"))
	if ruleID == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid rule_id (CWB_KB_KW_221)"})
	}
	var payload struct {
		Enabled bool `json:"enabled"`
	}
	_ = json.NewDecoder(c.Request().Body).Decode(&payload)

	store := keywords.RewriteRuleStore{DB: ApiTypes.ProjectDBHandle}
	if err := store.UpdateRuleEnabled(c.Request().Context(), ruleID, payload.Enabled); err != nil {
		logger.Error("toggle keyword rewrite rule failed", "rule_id", ruleID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to toggle keyword rewrite rule (CWB_KB_KW_222)"})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "message": "updated"})
}

// -- Resolution handler -------------------------------------------------------

// ResolveKeywordSurface handles POST /api/v1/kb/keyword-resolve
func ResolveKeywordSurface(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_KW_300")
	defer rc.Close()
	logger := rc.GetLogger()

	var payload struct {
		Surface string `json:"surface"`
		Scope   string `json:"scope"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_KW_301)"})
	}
	if strings.TrimSpace(payload.Surface) == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "surface is required (CWB_KB_KW_302)"})
	}
	if payload.Scope == "" {
		payload.Scope = "_"
	}

	// K6: the mode gate must fail closed. ResolverMode() maps an unset
	// KEYWORD_RESOLVER_MODE to "off"; reading os.Getenv directly left the
	// endpoint resolving and writing whenever the variable was unset.
	kf := &keywords.KeywordFamily{
		DB:           ApiTypes.ProjectDBHandle,
		ResolverMode: keywords.ResolverMode(),
	}
	res, err := kf.ObserveSurface(c.Request().Context(), payload.Surface, payload.Scope, "", "", "")
	if err != nil {
		logger.Error("resolve keyword surface failed", "surface", payload.Surface, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to resolve keyword surface (CWB_KB_KW_303)"})
	}
	if res == nil {
		return c.JSON(http.StatusOK, map[string]any{"status": true, "resolved": false, "message": "keyword resolver is off"})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "resolution": res})
}
