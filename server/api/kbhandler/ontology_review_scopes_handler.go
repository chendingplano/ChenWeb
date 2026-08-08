package kbhandler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/deepdoc/server/api/ontology/assertions"
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
	Run     profiles.ReviewRun              `json:"run"`
}

// ExecuteOntologyReviewScope evaluates an already-frozen scope. The rule
// loader resolves only releases pinned in that scope, never current
// activation, and the assertion set is loaded from governed
// kb.semantic_assertions rows for the scope's own pinned target_object_ids
// -- never from the request body -- so a scope's result stays reproducible
// and auditable from the scope record alone.
func ExecuteOntologyReviewScope(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_ORS_300")
	defer rc.Close()
	var payload struct {
		InputRecordID int64 `json:"input_record_id"`
		RunID         int64 `json:"run_id"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil || payload.InputRecordID == 0 || payload.RunID == 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "input_record_id and run_id are required (CWB_KB_ORS_301)"})
	}
	db := ApiTypes.ProjectDBHandle
	scope, err := (profiles.ReviewScopeStore{DB: db}).Get(c.Request().Context(), c.Param("scope_id"))
	if err != nil {
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "ontology review scope not found (CWB_KB_ORS_302)"})
	}
	service := profiles.ReviewService{
		Findings:   profiles.FindingStore{DB: db},
		Rules:      profiles.ProfileRuleStore{DB: db},
		Assertions: reviewAssertionLoader{Store: assertions.AssertionStore{DB: db}},
		Runs:       profiles.ReviewRunStore{DB: db},
	}
	results, run, err := service.EvaluatePinnedScope(c.Request().Context(), scope, payload.InputRecordID, payload.RunID)
	if err != nil {
		rc.GetLogger().Error("execute ontology review scope failed", "err", err)
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "failed to execute frozen ontology review scope (CWB_KB_ORS_303)"})
	}
	return c.JSON(http.StatusOK, ontologyReviewExecutionResponse{Status: true, Results: results, Run: run})
}

// CreateOntologyReviewScope freezes an explicit or deterministic selection
// before any review execution. The endpoint deliberately exposes no update
// operation: a changed selection requires a new scope id.
//
// A deterministic request (selection_mode=deterministic_rule) must not supply
// selected_profiles: the selector derives the frozen selection from the pinned
// released profiles, derives the knowledge store from kb.inputs.ks_store_id
// (never client input), and the scope is created with the P5 provenance
// columns populated. An explicit request is preserved exactly as before.
func CreateOntologyReviewScope(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_ORS_100")
	defer rc.Close()
	var scope profiles.ReviewScope
	if err := json.NewDecoder(c.Request().Body).Decode(&scope); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_ORS_101)"})
	}
	if scope.SelectionMode == profiles.SelectionModeDeterministicRule {
		created, err := createDeterministicReviewScope(c.Request().Context(), scope, rc.GetLogger())
		if err != nil {
			rc.GetLogger().Error("create deterministic ontology review scope failed", "err", err)
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "failed to create deterministic immutable review scope (CWB_KB_ORS_103)"})
		}
		return c.JSON(http.StatusOK, ontologyReviewScopeResponse{Status: true, Record: created})
	}
	created, err := (profiles.ReviewScopeStore{DB: ApiTypes.ProjectDBHandle}).Create(c.Request().Context(), scope)
	if err != nil {
		rc.GetLogger().Error("create ontology review scope failed", "err", err)
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "failed to create immutable review scope (CWB_KB_ORS_102)"})
	}
	return c.JSON(http.StatusOK, ontologyReviewScopeResponse{Status: true, Record: created})
}

// createDeterministicReviewScope runs automatic profile selection (spec
// 2026080102 section 6) and freezes the derived scope. The request must not
// supply selected_profiles; the selector pins the active releases, derives the
// knowledge store, evaluates each released profile per subject, and returns
// the immutable selection. The concrete subject-facts loader uses the
// doc-processing extraction fact builders so review applicability and
// extraction routing evaluate identical predicates to identical results
// (acceptance criterion 9). The alarm writer raises one warning per
// indeterminate scope, deduplicated by scope id (spec section 11).
// defaultJSONArray treats an unset or empty JSON raw message as an empty
// array, mirroring ReviewScopeStore.Create's defaults, so a deterministic
// request that omits target_object_ids or closed_dimensions still decodes.
func defaultJSONArray(raw json.RawMessage) []byte {
	if len(raw) == 0 {
		return []byte(`[]`)
	}
	return raw
}

func createDeterministicReviewScope(ctx context.Context, scope profiles.ReviewScope, logger ApiTypes.JimoLogger) (profiles.ReviewScope, error) {
	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return profiles.ReviewScope{}, errors.New("db is nil")
	}
	if len(scope.SelectedProfiles) > 0 {
		return profiles.ReviewScope{}, errors.New("a deterministic scope must not supply selected_profiles")
	}
	var documentIDs []int64
	if err := json.Unmarshal(defaultJSONArray(scope.ReviewedDocumentIDs), &documentIDs); err != nil {
		return profiles.ReviewScope{}, err
	}
	var targetObjectIDs []string
	if err := json.Unmarshal(defaultJSONArray(scope.TargetObjectIDs), &targetObjectIDs); err != nil {
		return profiles.ReviewScope{}, err
	}
	var closedDimensions []string
	if err := json.Unmarshal(defaultJSONArray(scope.ClosedDimensions), &closedDimensions); err != nil {
		return profiles.ReviewScope{}, err
	}
	selector := profiles.Selector{
		Source: profiles.ProfileStore{DB: db},
		Alarms: profiles.SelectionAlarmSQLWriter{DB: db},
		Logger: logger,
	}
	// Wire the tier-3 classification pass (spec 2026080102 section 7).
	// NewProductionApplicabilityResolver degrades to nil on configuration
	// failure (missing model config/prompt, logging a warning), leaving the
	// selector to evaluate base facts -- classify_document itself is a
	// routed processor (processor_plan.go) gated per-document by an
	// authored kb.pipeline_rules row, not by an env var.
	if resolver := docprocessing.NewProductionApplicabilityResolver(db, logger); resolver != nil {
		selector.Enricher = reviewFactEnricher{resolver: resolver, db: db}
	}
	selection, err := selector.Select(ctx, profiles.SelectionRequest{
		ReviewScopeID:       scope.ReviewScopeID,
		ReviewedDocumentIDs: documentIDs,
		TargetObjectIDs:     targetObjectIDs,
		ReviewContext: profiles.ReviewApplicabilityContext{
			AsOfDate: scope.AsOfDate, Jurisdiction: scope.Jurisdiction, OperatingContext: scope.OperatingContext,
			Purpose: "", ReleaseID: 0,
		},
		ClosedDimensions: closedDimensions,
		SelectedBy:       scope.SelectedBy,
		SelectionReason:  scope.SelectionReason,
		SubjectFacts:     reviewDocumentFactsLoader{DB: db},
	})
	if err != nil {
		return profiles.ReviewScope{}, err
	}
	selectedJSON, err := json.Marshal(selection.SelectedProfiles)
	if err != nil {
		return profiles.ReviewScope{}, err
	}
	factJSON, err := json.Marshal(selection.FactSnapshot)
	if err != nil {
		return profiles.ReviewScope{}, err
	}
	snapshotJSON, err := json.Marshal(selection.Snapshot)
	if err != nil {
		return profiles.ReviewScope{}, err
	}
	scope.SelectedProfiles = selectedJSON
	scope.KnowledgeStoreID = selection.KnowledgeStoreID
	scope.SelectionAttemptID = selection.SelectionAttemptID
	scope.SelectionStatus = selection.SelectionStatus
	scope.FactSnapshot = factJSON
	scope.SelectionSnapshot = snapshotJSON
	created, err := (profiles.ReviewScopeStore{DB: db}).Create(ctx, scope)
	if err != nil {
		return profiles.ReviewScope{}, err
	}
	return created, nil
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
