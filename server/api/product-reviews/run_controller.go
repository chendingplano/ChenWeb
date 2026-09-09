package productreviews

import (
	"context"
	"encoding/json"
	"fmt"
)

// RunController drives the request → run lifecycle: create a request pinning the
// profile version, run the retrieval, persist results, build the report. A
// failure is recorded on the run and leaves the request re-runnable.
type RunController struct {
	Store     Store
	Runs      RunStore
	Scoper    DocumentScoper
	Retriever Retriever
	Config    *Config
}

// StartReviewInput is the payload for a new review.
type StartReviewInput struct {
	TenantID      string
	ProfileID     int64
	ArtifactTypes []string
	Filters       map[string]any
	Notes         string
	Requester     string
}

// StartReview validates the profile and artifact types, creates the request and
// its first run, and executes it. A draft profile is refused with
// CWB_KB_PMR_030 and an unregistered type with CWB_KB_PMR_020 — in both cases no
// request is created.
func (c RunController) StartReview(ctx context.Context, in StartReviewInput) (*Run, error) {
	profile, err := c.Store.GetProfile(ctx, in.ProfileID)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return nil, fmt.Errorf("profile %d not found", in.ProfileID)
	}
	if profile.Status != ProfileReady {
		return nil, ErrDraftProfile(profile.ID)
	}
	types := in.ArtifactTypes
	if len(types) == 0 {
		types = []string{"metric"}
	}
	if err := ValidateArtifactTypes(types); err != nil {
		return nil, err
	}

	req, err := c.Runs.CreateRequest(ctx, RequestInput{
		TenantID: in.TenantID, ProfileID: profile.ID, ProfileVersion: profile.Version,
		ArtifactTypes: types, Filters: in.Filters, Notes: in.Notes, Requester: in.Requester,
	})
	if err != nil {
		return nil, err
	}
	run, err := c.Runs.CreateRun(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	if err := c.executeRun(ctx, run, profile, req); err != nil {
		logger.Warn("product review run failed", "run_id", run.ID, "request_id", req.ID, "error", err)
	}
	return c.Runs.GetRun(ctx, run.ID)
}

// Rerun creates the next run of an existing request and executes it. If the
// profile has been edited since the request was pinned, the request is re-pinned
// to the current version so the re-run reflects the current scope.
func (c RunController) Rerun(ctx context.Context, requestID int64) (*Run, error) {
	req, err := c.Runs.GetRequest(ctx, requestID)
	if err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("request %d not found", requestID)
	}
	profile, err := c.Store.GetProfile(ctx, req.ProfileID)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return nil, fmt.Errorf("profile %d not found", req.ProfileID)
	}
	if profile.Status != ProfileReady {
		return nil, ErrDraftProfile(profile.ID)
	}
	if profile.Version != req.ProfileVersion {
		if err := c.Runs.RepinRequestVersion(ctx, req.ID, profile.Version); err != nil {
			return nil, err
		}
		req.ProfileVersion = profile.Version
	}
	run, err := c.Runs.CreateRun(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	if err := c.executeRun(ctx, run, profile, req); err != nil {
		logger.Warn("product review re-run failed", "run_id", run.ID, "request_id", req.ID, "error", err)
	}
	return c.Runs.GetRun(ctx, run.ID)
}

// executeRun is the state machine body: running → (completed | failed).
func (c RunController) executeRun(ctx context.Context, run *Run, profile *Profile, req *Request) error {
	if err := c.Runs.MarkRunning(ctx, run.ID); err != nil {
		return err
	}
	fail := func(err error) error {
		_ = c.Runs.FailRun(ctx, run.ID, err.Error())
		return err
	}

	nodes, err := c.Store.LoadNodes(ctx, profile.ID)
	if err != nil {
		return fail(err)
	}
	scopeNodes, err := LoadScopeNodes(ctx, c.Store.DB, profile.ID)
	if err != nil {
		return fail(err)
	}
	scoped, err := c.Scoper.Scope(ctx, scopeNodes)
	if err != nil {
		return fail(err)
	}
	rr, err := c.Retriever.Retrieve(ctx, RetrieveInput{
		Nodes: scopeNodes, ArtifactTypes: req.ArtifactTypes, ScopedDocs: scoped,
	})
	if err != nil {
		return fail(err)
	}
	if err := c.Runs.SaveScopedDocs(ctx, run.ID, scoped); err != nil {
		return fail(err)
	}
	if err := c.Runs.SaveResults(ctx, run.ID, rr.Results); err != nil {
		return fail(err)
	}

	report := BuildReport(profile, nodes, req.ArtifactTypes, scoped, *rr)
	report.ProfileVersion = req.ProfileVersion
	reportJSON, _ := json.Marshal(report)
	counts := RunCounts{
		ResultCount:         len(rr.Results),
		AttributedCount:     rr.AttributedCount,
		DocumentScopeCount:  rr.DocumentScopeCount,
		ScopedDocumentCount: len(scoped),
		TruncatedCount:      rr.TruncatedCount,
	}
	if err := c.Runs.CompleteRun(ctx, run.ID, counts, reportJSON, RenderReportMarkdown(report)); err != nil {
		return fail(err)
	}
	return nil
}

// DiffAgainstPrevious compares a run to the previous run of the same request.
func (c RunController) DiffAgainstPrevious(ctx context.Context, runID int64) (*RunDiff, error) {
	cur, err := c.Runs.GetRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	if cur == nil {
		return nil, fmt.Errorf("run %d not found", runID)
	}
	curResults, err := c.Runs.LoadResults(ctx, cur.ID, ResultFilters{})
	if err != nil {
		return nil, err
	}
	prev, err := c.Runs.PreviousRun(ctx, cur.RequestID, cur.RunNumber)
	if err != nil {
		return nil, err
	}
	if prev == nil {
		d := DiffResults(nil, curResults, 0, runReportVersion(cur), 0)
		return &d, nil
	}
	prevResults, err := c.Runs.LoadResults(ctx, prev.ID, ResultFilters{})
	if err != nil {
		return nil, err
	}
	d := DiffResults(prevResults, curResults, runReportVersion(prev), runReportVersion(cur), prev.RunNumber)
	return &d, nil
}

// runReportVersion pulls the pinned profile_version out of a run's report_json.
func runReportVersion(r *Run) int {
	if r == nil || len(r.ReportJSON) == 0 {
		return 0
	}
	var v struct {
		ProfileVersion int `json:"profile_version"`
	}
	_ = json.Unmarshal(r.ReportJSON, &v)
	return v.ProfileVersion
}
