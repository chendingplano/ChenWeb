package productreviews

import (
	"encoding/csv"
	"errors"
	"net/http"
	"strconv"
	"strings"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/deepdoc/server/api/kbsearch"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

// ── wiring ──────────────────────────────────────────────────────────────────

func newStore() Store { return Store{DB: ApiTypes.ProjectDBHandle} }

func newRunController() (RunController, error) {
	cfg, err := GetConfig()
	if err != nil {
		return RunController{}, err
	}
	if cfg == nil {
		cfg = &Config{Budgets: BudgetsConfig{}.withDefaults()}
	}
	db := ApiTypes.ProjectDBHandle
	return RunController{
		Store:     Store{DB: db},
		Runs:      RunStore{DB: db},
		Scoper:    DocumentScoper{DB: db, Config: cfg},
		Retriever: Retriever{DB: db, Config: cfg},
		Config:    cfg,
	}, nil
}

// newBuilder wires the profile-construction passes with the real LLM client and
// the shared search embedder.
func newBuilder() (Builder, error) {
	cfg, err := GetConfig()
	if err != nil {
		return Builder{}, err
	}
	if cfg == nil {
		return Builder{}, errors.New("product-review.local.toml not found")
	}
	promptText, _, _, err := docprocessing.LoadPromptByRef("prompt-product-structure-v1.md")
	if err != nil {
		return Builder{}, err
	}
	extractor, modelName, err := docprocessing.BuildReviewerLLMClient(cfg.Models.Structure)
	if err != nil {
		return Builder{}, err
	}
	store := newStore()
	return Builder{
		Store: store,
		Proposer: Proposer{
			Store: store, Extractor: extractor, ModelName: modelName,
			PromptText: promptText, Budgets: cfg.Budgets,
		},
		Grounder: Grounder{
			Store: store, DB: store.DB,
			Embed:           docprocessing.EmbedSearchQuery,
			SemanticEnabled: kbsearch.SemanticSearchEnabled(),
			Thresholds:      cfg.Grounding,
		},
		Expander: Expander{Store: store, DB: store.DB, Budgets: cfg.Budgets},
		Config:   cfg,
	}, nil
}

// ── helpers ─────────────────────────────────────────────────────────────────

func fail(c echo.Context, err error) error {
	var pmr *PMRError
	if errors.As(err, &pmr) {
		return c.JSON(http.StatusUnprocessableEntity, map[string]any{
			"status": false, "error_code": pmr.Code, "error_msg": pmr.Msg,
		})
	}
	if errors.Is(err, ErrCycle(0)) || errors.Is(err, ErrMultipleRoots) || errors.Is(err, ErrNodeNotFound) {
		return c.JSON(http.StatusUnprocessableEntity, map[string]any{"status": false, "error_msg": err.Error()})
	}
	return c.JSON(http.StatusInternalServerError, map[string]any{"status": false, "error_msg": err.Error()})
}

func idParam(c echo.Context, name string) (int64, error) {
	return strconv.ParseInt(strings.TrimSpace(c.Param(name)), 10, 64)
}

// ── profile endpoints ───────────────────────────────────────────────────────

// CreateProductProfile — POST /kb/product-profiles
func CreateProductProfile(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_PMR_H01")
	defer rc.Close()
	var body struct {
		Name               string   `json:"name"`
		ProductDescription string   `json:"product_description"`
		Keywords           []string `json:"keywords"`
		Notes              string   `json:"notes"`
		TenantID           string   `json:"tenant_id"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error_msg": err.Error()})
	}
	p, err := newStore().CreateProfile(c.Request().Context(), NewProfileInput{
		Name: body.Name, ProductDescription: body.ProductDescription,
		Keywords: body.Keywords, Notes: body.Notes, TenantID: body.TenantID,
	})
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "profile": p})
}

// GetProductProfile — GET /kb/product-profiles/:id
func GetProductProfile(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_PMR_H02")
	defer rc.Close()
	id, err := idParam(c, "id")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error_msg": "bad profile id"})
	}
	store := newStore()
	ctx := c.Request().Context()
	p, err := store.GetProfile(ctx, id)
	if err != nil {
		return fail(c, err)
	}
	if p == nil {
		return c.JSON(http.StatusNotFound, map[string]any{"status": false, "error_msg": "profile not found"})
	}
	nodes, err := store.LoadNodes(ctx, id)
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "profile": p, "nodes": nodes})
}

// ListProductProfiles — GET /kb/product-profiles?limit= (spec:
// product-review-history-list): most-recently-updated profiles, each
// carrying its latest review request/run so the intake page can render past
// reviews as cards without further round trips.
func ListProductProfiles(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_PMR_H19")
	defer rc.Close()
	limit := parseListLimit(c.QueryParam("limit"), defaultProfileListLimit, maxProfileListLimit)
	tenantID := c.QueryParam("tenant_id")
	profiles, err := newStore().ListProfiles(c.Request().Context(), tenantID, limit)
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "profiles": profiles})
}

const (
	defaultProfileListLimit = 50
	maxProfileListLimit     = 200
)

// parseListLimit parses a "limit" query param, falling back to def on an
// absent/invalid/non-positive value and clamping to max — extracted from the
// handler so it's unit-testable without the global DB/config wiring the rest
// of this package's handlers depend on.
func parseListLimit(raw string, def, max int) int {
	limit := def
	if v := strings.TrimSpace(raw); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > max {
		limit = max
	}
	return limit
}

// BuildProductProfile — POST /kb/product-profiles/:id/build
func BuildProductProfile(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_PMR_H03")
	defer rc.Close()
	id, err := idParam(c, "id")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error_msg": "bad profile id"})
	}
	var body struct {
		SeedExcerpts []string `json:"seed_excerpts"`
	}
	_ = c.Bind(&body)
	builder, err := newBuilder()
	if err != nil {
		return fail(c, err)
	}
	if err := builder.Build(c.Request().Context(), id, ProposeInput{SeedExcerpts: body.SeedExcerpts}); err != nil {
		return fail(c, err)
	}
	nodes, err := builder.Store.LoadNodes(c.Request().Context(), id)
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "nodes": nodes})
}

// AddProductProfileNode — POST /kb/product-profiles/:id/nodes
func AddProductProfileNode(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_PMR_H04")
	defer rc.Close()
	id, err := idParam(c, "id")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error_msg": "bad profile id"})
	}
	var body struct {
		ParentNodeID *int64   `json:"parent_node_id"`
		NodeKind     string   `json:"node_kind"`
		Label        string   `json:"label"`
		LabelEN      string   `json:"label_en"`
		Aliases      []string `json:"aliases"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error_msg": err.Error()})
	}
	newID, err := newStore().AddNode(c.Request().Context(), id, body.ParentNodeID, ProfileNode{
		NodeKind: body.NodeKind, Label: body.Label, LabelEN: body.LabelEN, Aliases: body.Aliases,
	})
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "node_id": newID})
}

// UpdateProductProfileNode — PATCH /kb/product-profiles/:id/nodes/:node_id
func UpdateProductProfileNode(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_PMR_H05")
	defer rc.Close()
	id, err := idParam(c, "id")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error_msg": "bad profile id"})
	}
	nodeID, err := idParam(c, "node_id")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error_msg": "bad node id"})
	}
	var body struct {
		Label        *string   `json:"label"`
		LabelEN      *string   `json:"label_en"`
		Aliases      *[]string `json:"aliases"`
		NodeKind     *string   `json:"node_kind"`
		Status       *string   `json:"status"`
		ParentNodeID *int64    `json:"parent_node_id"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error_msg": err.Error()})
	}
	err = newStore().UpdateNode(c.Request().Context(), id, nodeID, NodeEdit{
		Label: body.Label, LabelEN: body.LabelEN, Aliases: body.Aliases,
		NodeKind: body.NodeKind, Status: body.Status, ParentNodeID: body.ParentNodeID,
	})
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true})
}

// DeleteProductProfileNode — DELETE /kb/product-profiles/:id/nodes/:node_id
func DeleteProductProfileNode(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_PMR_H06")
	defer rc.Close()
	id, err := idParam(c, "id")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error_msg": "bad profile id"})
	}
	nodeID, err := idParam(c, "node_id")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error_msg": "bad node id"})
	}
	if err := newStore().DeleteNode(c.Request().Context(), id, nodeID); err != nil {
		return fail(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true})
}

// SetProductProfileReady — POST /kb/product-profiles/:id/ready
func SetProductProfileReady(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_PMR_H07")
	defer rc.Close()
	id, err := idParam(c, "id")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error_msg": "bad profile id"})
	}
	var body struct {
		Ready *bool `json:"ready"`
	}
	_ = c.Bind(&body)
	status := ProfileReady
	if body.Ready != nil && !*body.Ready {
		status = ProfileDraft
	}
	if err := newStore().SetProfileStatus(c.Request().Context(), id, status); err != nil {
		return fail(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "profile_status": status})
}

// SetProductProfileDrawing — PATCH /kb/product-profiles/:id/drawing (spec:
// product-review-results-layout). Associates a kept product-drawings row with
// the profile so the Results page shows it without regenerating.
func SetProductProfileDrawing(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_PMR_H22")
	defer rc.Close()
	id, err := idParam(c, "id")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error_msg": "bad profile id"})
	}
	var body struct {
		DrawingID int64 `json:"drawing_id"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error_msg": err.Error()})
	}
	if err := newStore().SetProfileDrawing(c.Request().Context(), id, body.DrawingID); err != nil {
		return fail(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true})
}

// ListProductReviewAspects — GET /kb/product-reviews/aspects?lang=
func ListProductReviewAspects(c echo.Context) error {
	cfg, err := GetConfig()
	if err != nil {
		return fail(c, err)
	}
	var vocab []AspectEntry
	if cfg != nil {
		vocab = cfg.AspectVocabulary(strings.TrimSpace(c.QueryParam("lang")))
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "aspects": vocab})
}

// ── review / run endpoints ──────────────────────────────────────────────────

// CreateProductReview — POST /kb/product-reviews
func CreateProductReview(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_PMR_H10")
	defer rc.Close()
	var body struct {
		ProfileID     int64          `json:"profile_id"`
		ArtifactTypes []string       `json:"artifact_types"`
		Filters       map[string]any `json:"filters"`
		Notes         string         `json:"notes"`
		TenantID      string         `json:"tenant_id"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error_msg": err.Error()})
	}
	ctrl, err := newRunController()
	if err != nil {
		return fail(c, err)
	}
	requester := ""
	if u := EchoFactory.NewFromEcho(c, "CWB_KB_PMR_H10U").IsAuthenticated(); u != nil {
		requester = u.UserName
	}
	run, err := ctrl.StartReview(c.Request().Context(), StartReviewInput{
		TenantID: body.TenantID, ProfileID: body.ProfileID, ArtifactTypes: body.ArtifactTypes,
		Filters: body.Filters, Notes: body.Notes, Requester: requester,
	})
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "run": run})
}

// ListProductReviews — GET /kb/product-reviews?profile_id=
func ListProductReviews(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_PMR_H11")
	defer rc.Close()
	var profileID int64
	if v := strings.TrimSpace(c.QueryParam("profile_id")); v != "" {
		profileID, _ = strconv.ParseInt(v, 10, 64)
	}
	reqs, err := RunStore{DB: ApiTypes.ProjectDBHandle}.ListRequests(c.Request().Context(), profileID)
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "requests": reqs})
}

// GetProductReview — GET /kb/product-reviews/:id
func GetProductReview(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_PMR_H12")
	defer rc.Close()
	id, err := idParam(c, "id")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error_msg": "bad request id"})
	}
	store := RunStore{DB: ApiTypes.ProjectDBHandle}
	ctx := c.Request().Context()
	req, err := store.GetRequest(ctx, id)
	if err != nil {
		return fail(c, err)
	}
	if req == nil {
		return c.JSON(http.StatusNotFound, map[string]any{"status": false, "error_msg": "review request not found"})
	}
	runs, err := store.ListRuns(ctx, id)
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "request": req, "runs": runs})
}

// RerunProductReview — POST /kb/product-reviews/:id/rerun
func RerunProductReview(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_PMR_H13")
	defer rc.Close()
	id, err := idParam(c, "id")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error_msg": "bad request id"})
	}
	ctrl, err := newRunController()
	if err != nil {
		return fail(c, err)
	}
	run, err := ctrl.Rerun(c.Request().Context(), id)
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "run": run})
}

// GetProductReviewRun — GET /kb/product-reviews/runs/:run_id
func GetProductReviewRun(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_PMR_H14")
	defer rc.Close()
	runID, err := idParam(c, "run_id")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error_msg": "bad run id"})
	}
	run, err := RunStore{DB: ApiTypes.ProjectDBHandle}.GetRun(c.Request().Context(), runID)
	if err != nil {
		return fail(c, err)
	}
	if run == nil {
		return c.JSON(http.StatusNotFound, map[string]any{"status": false, "error_msg": "run not found"})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "run": run})
}

func runResultFilters(c echo.Context) ResultFilters {
	f := ResultFilters{
		Tier:         strings.TrimSpace(c.QueryParam("tier")),
		Path:         strings.TrimSpace(c.QueryParam("path")),
		ArtifactType: strings.TrimSpace(c.QueryParam("artifact_type")),
	}
	if v := strings.TrimSpace(c.QueryParam("node_id")); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			f.NodeID = &n
		}
	}
	if v := strings.TrimSpace(c.QueryParam("input_record_id")); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			f.InputRecordID = &n
		}
	}
	for _, ex := range strings.Split(c.QueryParam("exclude_tier"), ",") {
		if ex = strings.TrimSpace(ex); ex != "" {
			f.ExcludeTiers = append(f.ExcludeTiers, ex)
		}
	}
	return f
}

// GetProductReviewRunResults — GET /kb/product-reviews/runs/:run_id/results
func GetProductReviewRunResults(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_PMR_H15")
	defer rc.Close()
	runID, err := idParam(c, "run_id")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error_msg": "bad run id"})
	}
	results, err := RunStore{DB: ApiTypes.ProjectDBHandle}.LoadResults(c.Request().Context(), runID, runResultFilters(c))
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "results": results, "count": len(results)})
}

// GetProductReviewMetricDetail — GET /kb/product-reviews/artifacts/:artifact_id/metric
func GetProductReviewMetricDetail(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_PMR_H23")
	defer rc.Close()
	artifactID := strings.TrimSpace(c.Param("artifact_id"))
	if artifactID == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error_msg": "missing artifact id"})
	}
	detail, err := FetchMetricDetail(c.Request().Context(), ApiTypes.ProjectDBHandle, artifactID)
	if err != nil {
		return fail(c, err)
	}
	if detail == nil {
		return c.JSON(http.StatusNotFound, map[string]any{"status": false, "error_msg": "metric not found"})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "metric": detail})
}

// GetProductReviewObjectNames — GET /kb/product-reviews/objects?ids=obj_1,obj_2
func GetProductReviewObjectNames(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_PMR_H24")
	defer rc.Close()
	var ids []string
	for _, id := range strings.Split(c.QueryParam("ids"), ",") {
		if id = strings.TrimSpace(id); id != "" {
			ids = append(ids, id)
		}
	}
	names, err := FetchObjectNames(c.Request().Context(), ApiTypes.ProjectDBHandle, ids)
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "objects": names})
}

// GetProductReviewRunDocuments — GET /kb/product-reviews/runs/:run_id/documents
func GetProductReviewRunDocuments(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_PMR_H16")
	defer rc.Close()
	runID, err := idParam(c, "run_id")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error_msg": "bad run id"})
	}
	docs, err := RunStore{DB: ApiTypes.ProjectDBHandle}.LoadScopedDocs(c.Request().Context(), runID)
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "documents": docs, "count": len(docs)})
}

// GetProductReviewRunDiff — GET /kb/product-reviews/runs/:run_id/diff
func GetProductReviewRunDiff(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_PMR_H17")
	defer rc.Close()
	runID, err := idParam(c, "run_id")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error_msg": "bad run id"})
	}
	ctrl, err := newRunController()
	if err != nil {
		return fail(c, err)
	}
	diff, err := ctrl.DiffAgainstPrevious(c.Request().Context(), runID)
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "diff": diff})
}

// ExportProductReviewRunResults — GET /kb/product-reviews/runs/:run_id/export (CSV)
func ExportProductReviewRunResults(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_PMR_H18")
	defer rc.Close()
	runID, err := idParam(c, "run_id")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error_msg": "bad run id"})
	}
	results, err := RunStore{DB: ApiTypes.ProjectDBHandle}.LoadResults(c.Request().Context(), runID, runResultFilters(c))
	if err != nil {
		return fail(c, err)
	}
	c.Response().Header().Set("Content-Type", "text/csv")
	c.Response().Header().Set("Content-Disposition", "attachment; filename=product-review-run-"+strconv.FormatInt(runID, 10)+".csv")
	w := csv.NewWriter(c.Response())
	_ = w.Write([]string{"artifact_type", "artifact_id", "primary_label", "document_record_id",
		"source_line_spans", "matched_node_id", "tier", "score", "paths", "inclusion_reason"})
	for _, r := range results {
		node := ""
		if r.NodeID != nil {
			node = strconv.FormatInt(*r.NodeID, 10)
		}
		_ = w.Write([]string{
			r.ArtifactType, r.ArtifactID, r.PrimaryLabel, strconv.FormatInt(r.InputRecordID, 10),
			string(r.LineSpans), node, r.Tier, strconv.FormatFloat(r.Score, 'f', 4, 64),
			strings.Join(r.Paths, "|"), r.InclusionReason,
		})
	}
	w.Flush()
	return w.Error()
}

// ── product name catalog ────────────────────────────────────────────────────

// ListProductNames — GET /kb/product-names: the full approved+proposed
// kb.product_names catalog, for the intake page's Product Name typeahead
// (design: product-name-typeahead). The catalog is small and changes
// rarely, so the browser caches this response and searches it in memory
// rather than querying per keystroke.
func ListProductNames(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_PMR_H21")
	defer rc.Close()
	names, err := newStore().ListProductNames(c.Request().Context())
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "product_names": names})
}
