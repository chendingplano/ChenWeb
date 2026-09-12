package productreviews

import (
	"context"
	"net/http"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

// IntakeProductReview — POST /kb/product-reviews/intake
//
// Orchestrates the self-service "start a review" flow (spec:
// product-review-intake). If a profile for the same product name already
// exists, this returns it — plus its latest request/run, if any — instead of
// creating a duplicate, so the caller can offer "view results" or "re-run".
// Otherwise (or when resume_profile_id resumes a profile whose earlier
// attempt never finished building) it drives build -> ready -> start-review
// against one profile in a single request, matching the synchronous style of
// the existing per-step endpoints.
func IntakeProductReview(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_PMR_H20")
	defer rc.Close()
	var body struct {
		Name               string   `json:"name"`
		ProductDescription string   `json:"product_description"`
		Keywords           []string `json:"keywords"`
		Notes              string   `json:"notes"`
		TenantID           string   `json:"tenant_id"`
		ResumeProfileID    int64    `json:"resume_profile_id"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error_msg": err.Error()})
	}
	ctx := c.Request().Context()
	store := newStore()
	runs := RunStore{DB: ApiTypes.ProjectDBHandle}

	var profile *Profile
	if body.ResumeProfileID > 0 {
		p, err := store.GetProfile(ctx, body.ResumeProfileID)
		if err != nil {
			return fail(c, err)
		}
		if p == nil {
			return c.JSON(http.StatusNotFound, map[string]any{"status": false, "error_msg": "profile not found"})
		}
		profile = p
	} else {
		dupResp, err := duplicateProfileResponse(ctx, store, runs, body.TenantID, body.Name)
		if err != nil {
			return fail(c, err)
		}
		if dupResp != nil {
			return c.JSON(http.StatusOK, dupResp)
		}
		created, err := store.CreateProfile(ctx, NewProfileInput{
			TenantID: body.TenantID, Name: body.Name, ProductDescription: body.ProductDescription,
			Keywords: body.Keywords, Notes: body.Notes,
		})
		if err != nil {
			return fail(c, err)
		}
		profile = created
	}

	if profile.Status != ProfileReady {
		builder, err := newBuilder()
		if err != nil {
			return fail(c, err)
		}
		if err := builder.Build(ctx, profile.ID, ProposeInput{}); err != nil {
			return fail(c, err)
		}
		// Self-service intake has no manual curation step, so accept the
		// LLM-proposed tree wholesale rather than leaving every module/part
		// node stuck at status=proposed (and so invisible to retrieval).
		if err := store.AcceptAllProposed(ctx, profile.ID); err != nil {
			return fail(c, err)
		}
		if err := store.SetProfileStatus(ctx, profile.ID, ProfileReady); err != nil {
			return fail(c, err)
		}
	}

	ctrl, err := newRunController()
	if err != nil {
		return fail(c, err)
	}
	requester := ""
	if u := EchoFactory.NewFromEcho(c, "CWB_KB_PMR_H20U").IsAuthenticated(); u != nil {
		requester = u.UserName
	}
	run, err := ctrl.StartReview(ctx, StartReviewInput{
		TenantID: body.TenantID, ProfileID: profile.ID, Notes: body.Notes, Requester: requester,
	})
	if err != nil {
		return fail(c, err)
	}
	refreshed, err := store.GetProfile(ctx, profile.ID)
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{
		"status": true, "duplicate": false, "profile": refreshed, "run": run,
	})
}

// duplicateProfileResponse looks up an existing profile by name and, if found,
// builds the response payload offering "view results" (when a completed run
// exists) or "re-run" (spec: product-review-intake, duplicate detection).
// Returns nil when no matching profile exists.
func duplicateProfileResponse(ctx context.Context, store Store, runs RunStore, tenantID, name string) (map[string]any, error) {
	existing, err := store.FindProfileByName(ctx, tenantID, name)
	if err != nil || existing == nil {
		return nil, err
	}
	req, run, err := runs.LatestRequestAndRun(ctx, existing.ID)
	if err != nil {
		return nil, err
	}
	resp := map[string]any{"status": true, "duplicate": true, "profile": existing}
	if req != nil {
		resp["latest_request_id"] = req.ID
	}
	if run != nil {
		resp["latest_run"] = run
	}
	return resp, nil
}
