// Package terminologyresourcehandler implements the External Terminology
// Resources admin API backing System Admin > Resources. Downloads run on the
// server and write local artifacts + unapproved draft manifests under
// TERMINOLOGY_DIR (or <DATA_HOME_DIR>/terminology); the same directory feeds
// terminology-import after operator approval.
package terminologyresourcehandler

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/chendingplano/deepdoc/server/api/ontology/keywords"
	"github.com/chendingplano/deepdoc/server/api/ontology/terminology"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

const downloadTimeout = 10 * time.Minute

type errorResponse struct {
	Status   bool   `json:"status"`
	ErrorMsg string `json:"error"`
}

// resourceView is the JSON shape served to the admin page: catalog entry
// merged with the persisted download status.
type resourceView struct {
	ID                 string     `json:"id"`
	Name               string     `json:"name"`
	Description        string     `json:"description"`
	URL                string     `json:"url"`
	Release            string     `json:"release"`
	License            string     `json:"license"`
	LicenseURL         string     `json:"license_url"`
	CanDownload        bool       `json:"can_download"`
	PermissionRequired bool       `json:"permission_required"`
	Notes              string     `json:"notes"`
	Downloaded         bool       `json:"downloaded"`
	DownloadedAt       *time.Time `json:"downloaded_at"`
	Downloading        bool       `json:"downloading"`
	DownloadedBytes    int64      `json:"downloaded_bytes"`
	TotalBytes         int64      `json:"total_bytes"`
	SHA256             string     `json:"sha256"`
	SizeBytes          int64      `json:"size_bytes"`
	ExpectedSizeBytes  int64      `json:"expected_size_bytes"`
	MaxBytes           int64      `json:"max_bytes"`
	Cadence            string     `json:"update_cadence"`
	Artifact           string     `json:"artifact"`
	SourceURL          string     `json:"source_url"`
	ManifestDraft      string     `json:"manifest_draft"`
	// ReviewStatus is the draft manifest's license_review_status
	// ("pending_review", "approved", "disapproved", or "" when there is no
	// draft). The Review page lists downloaded resources still pending review.
	ReviewStatus   string     `json:"review_status"`
	ReviewComments string     `json:"review_comments"`
	ReviewedBy     string     `json:"reviewed_by"`
	ReviewedAt     *time.Time `json:"reviewed_at"`
	Error          string     `json:"error"`
	// AutoPromoteEnabled reflects kb.keyword_source_promotion_policy: whether
	// approving this resource auto-promotes its staged catalog entries into
	// keyword concepts. True (the default) unless an admin disabled it.
	AutoPromoteEnabled bool `json:"auto_promote_enabled"`
}

// importOutcome reports the result of the import started after approval.
type importOutcome struct {
	OK     bool                      `json:"ok"`
	Result *terminology.ImportResult `json:"result,omitempty"`
	Error  string                    `json:"error,omitempty"`
}

// terminologyDir resolves the fetch storage directory. TERMINOLOGY_DIR is the
// explicit override; DATA_HOME_DIR is the fallback so downloads work on any
// deployment that already sets it (mirrors videohandler).
func terminologyDir() string {
	return terminology.Dir()
}

func viewFor(dir string, res terminology.Resource, st terminology.FetchStatus) resourceView {
	v := resourceView{
		ID: string(res.ID), Name: res.Name, Description: res.Description,
		URL: res.URL, Release: res.Release, License: res.License,
		LicenseURL: res.LicenseURL, CanDownload: res.Downloadable,
		PermissionRequired: res.PermissionRequired, Notes: res.Notes,
		Downloaded: st.Downloaded, SHA256: st.SHA256, SizeBytes: st.SizeBytes,
		Downloading: st.Downloading, DownloadedBytes: st.DownloadedBytes, TotalBytes: st.TotalBytes,
		ExpectedSizeBytes: res.ExpectedSizeBytes, MaxBytes: res.MaxBytes, Cadence: res.Cadence,
		Artifact: st.Artifact, SourceURL: st.SourceURL, ManifestDraft: st.ManifestDraft,
		Error: st.Error, AutoPromoteEnabled: true,
	}
	if db := ApiTypes.ProjectDBHandle; db != nil {
		if enabled, err := (keywords.PromotionPolicyStore{DB: db}).IsEnabled(context.Background(), res.Source); err == nil {
			v.AutoPromoteEnabled = enabled
		}
	}
	if st.Downloaded {
		if rev, err := terminology.ReadDraftReview(dir, res.ID); err == nil {
			v.ReviewStatus = rev.Status
			v.ReviewComments = rev.Comments
			v.ReviewedBy = rev.ReviewedBy
			v.ReviewedAt = rev.ReviewedAt
		} else {
			v.Error = err.Error()
		}
	}
	if !st.DownloadedAt.IsZero() {
		t := st.DownloadedAt
		v.DownloadedAt = &t
	}
	if st.Release != "" {
		v.Release = st.Release
	}
	return v
}

// ListResources returns every portfolio resource with its persisted status.
func ListResources(c echo.Context) error {
	dir := terminologyDir()
	if dir == "" {
		return c.JSON(http.StatusInternalServerError, errorResponse{false, "TERMINOLOGY_DIR or DATA_HOME_DIR is not set"})
	}
	views := make([]resourceView, 0, len(terminology.Resources()))
	for _, res := range terminology.Resources() {
		st, err := terminology.ReadStatus(dir, res.ID)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, errorResponse{false, err.Error()})
		}
		views = append(views, viewFor(dir, res, st))
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "resources": views})
}

// ApproveResource completes the operator license review of one pending draft
// manifest: it records license_review_status=approved plus approved_by
// (authenticated user email unless the body overrides it), approved_at, and
// the review comments, then starts the offline import of the now-approved
// local manifest. Approval is persisted even if the import fails so the
// operator can retry the import; the response reports both.
func ApproveResource(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_TER_001")
	defer rc.Close()
	logger := rc.GetLogger()

	dir := terminologyDir()
	if dir == "" {
		logger.Error("TERMINOLOGY_DIR is not configured")
		return c.JSON(http.StatusInternalServerError, errorResponse{false, "TERMINOLOGY_DIR or DATA_HOME_DIR is not set"})
	}
	id := terminology.ResourceID(c.Param("source"))
	if _, ok := terminology.ResourceByID(id); !ok {
		return c.JSON(http.StatusNotFound, errorResponse{false, "unknown resource: " + string(id)})
	}
	approvedBy := strings.TrimSpace(c.FormValue("approved_by"))
	if approvedBy == "" {
		if u := rc.IsAuthenticated(); u != nil {
			approvedBy = strings.TrimSpace(u.Email)
		}
	}
	if approvedBy == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{false, "approved_by is required"})
	}
	comments := strings.TrimSpace(c.FormValue("comments"))
	st, err := terminology.ApproveDraft(dir, id, approvedBy, comments, time.Now().UTC())
	if err != nil {
		code := http.StatusInternalServerError
		if errors.Is(err, terminology.ErrNotDownloaded) ||
			errors.Is(err, terminology.ErrNoDraftManifest) ||
			errors.Is(err, terminology.ErrAlreadyApproved) ||
			errors.Is(err, terminology.ErrAlreadyDisapproved) {
			code = http.StatusConflict
		}
		logger.Warn("approve terminology draft failed", "source", string(id), "err", err)
		return c.JSON(code, errorResponse{false, err.Error()})
	}
	logger.Info("approved terminology draft", "source", string(id), "approved_by", approvedBy)

	outcome := importOutcome{}
	var governance qudtGovernanceOutcome
	hasGovernance := false
	manifestPath := filepath.Join(dir, string(id), "manifest.draft.json")
	ctx := c.Request().Context()
	if db := ApiTypes.ProjectDBHandle; db != nil {
		isQUDT := id == terminology.ResourceQUDT
		if alreadyImportedIdentical(ctx, db, manifestPath) {
			// The release is already registered with byte-identical content
			// (for example after a re-download and re-approval): the desired
			// end state already holds, so the import is an idempotent replay.
			outcome.OK = true
			logger.Info("approved terminology release already imported with identical content", "source", string(id))
			if isQUDT {
				hasGovernance = true
				governance = writeQUDTTermsReplay(ctx, db, manifestPath, approvedBy)
				if !governance.OK {
					outcome.OK = false
					outcome.Error = governance.Error
					logger.Warn("ontology write after approval replay failed", "source", string(id), "err", governance.Error)
				}
			}
		} else if isQUDT {
			hasGovernance = true
			var result terminology.ImportResult
			result, governance = writeQUDTTermsFresh(ctx, db, manifestPath, approvedBy)
			if governance.OK {
				outcome.OK = true
				outcome.Result = &result
				logger.Info("imported approved terminology release", "source", result.Source, "release", result.Release, "replayed", result.Replayed)
			} else {
				outcome.Error = governance.Error
				logger.Warn("import after approval failed", "source", string(id), "err", governance.Error)
			}
		} else {
			result, importErr := (terminology.Runner{DB: db}).Import(ctx, manifestPath)
			if importErr != nil {
				outcome.Error = importErr.Error()
				logger.Warn("import after approval failed", "source", string(id), "err", importErr)
			} else {
				outcome.OK = true
				outcome.Result = &result
				logger.Info("imported approved terminology release", "source", result.Source, "release", result.Release, "replayed", result.Replayed)
			}
		}

		if hasGovernance && governance.OK {
			version, relErr := releaseQUDTIfPending(ctx, db, approvedBy)
			if relErr != nil {
				governance.Error = relErr.Error()
				outcome.OK = false
				if outcome.Error == "" {
					outcome.Error = relErr.Error()
				}
				logger.Warn("release quantity module after approval failed", "source", string(id), "err", relErr)
			} else if version != "" {
				governance.ReleaseVersion = version
				logger.Info("released and activated quantity module", "source", string(id), "release", version)
			}
		}

		if outcome.OK {
			triggerCatalogAutoPromotion(ctx, db, id, st, logger)
		}
	} else {
		outcome.Error = "database handle is not configured; run terminology-import import manually"
	}
	res, _ := terminology.ResourceByID(id)
	respBody := map[string]any{"status": true, "resource": viewFor(dir, res, st), "import": outcome}
	if hasGovernance {
		respBody["ontology"] = governance
	}
	return c.JSON(http.StatusOK, respBody)
}

// alreadyImportedIdentical reports whether the approved local manifest is an
// exact-content replay of an already imported release: the release exists in
// kb.keyword_sources and its registered content checksum matches the
// manifest's. The immutable release guard would otherwise reject the
// re-import because a re-download/re-approval changes retrieved_at and
// approved_at, so this treats identical content as an idempotent replay.
func alreadyImportedIdentical(ctx context.Context, db *sql.DB, manifestPath string) bool {
	manifest, _, err := terminology.ParseAndVerifyManifest(manifestPath)
	if err != nil {
		return false
	}
	var registered string
	if err := db.QueryRowContext(ctx,
		`SELECT content_checksum FROM kb.keyword_sources WHERE source = $1 AND release = $2`,
		manifest.Policy.Source, manifest.Policy.Release,
	).Scan(&registered); err != nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(registered), strings.TrimSpace(manifest.Policy.ContentChecksum))
}

// DisapproveResource records an operator rejection of one pending draft
// manifest: license_review_status becomes disapproved and the review comments
// and reviewer are saved. It never imports anything.
func DisapproveResource(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_TER_002")
	defer rc.Close()
	logger := rc.GetLogger()

	dir := terminologyDir()
	if dir == "" {
		logger.Error("TERMINOLOGY_DIR is not configured")
		return c.JSON(http.StatusInternalServerError, errorResponse{false, "TERMINOLOGY_DIR or DATA_HOME_DIR is not set"})
	}
	id := terminology.ResourceID(c.Param("source"))
	if _, ok := terminology.ResourceByID(id); !ok {
		return c.JSON(http.StatusNotFound, errorResponse{false, "unknown resource: " + string(id)})
	}
	reviewer := strings.TrimSpace(c.FormValue("reviewed_by"))
	if reviewer == "" {
		if u := rc.IsAuthenticated(); u != nil {
			reviewer = strings.TrimSpace(u.Email)
		}
	}
	if reviewer == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{false, "reviewer is required"})
	}
	comments := strings.TrimSpace(c.FormValue("comments"))
	st, err := terminology.DisapproveDraft(dir, id, reviewer, comments, time.Now().UTC())
	if err != nil {
		code := http.StatusInternalServerError
		if errors.Is(err, terminology.ErrNotDownloaded) ||
			errors.Is(err, terminology.ErrNoDraftManifest) ||
			errors.Is(err, terminology.ErrAlreadyApproved) ||
			errors.Is(err, terminology.ErrAlreadyDisapproved) {
			code = http.StatusConflict
		}
		logger.Warn("disapprove terminology draft failed", "source", string(id), "err", err)
		return c.JSON(code, errorResponse{false, err.Error()})
	}
	logger.Info("disapproved terminology draft", "source", string(id), "reviewed_by", reviewer)
	res, _ := terminology.ResourceByID(id)
	return c.JSON(http.StatusOK, map[string]any{"status": true, "resource": viewFor(dir, res, st)})
}

// DownloadResource fetches one resource and returns its updated status.
func DownloadResource(c echo.Context) error {
	dir := terminologyDir()
	if dir == "" {
		return c.JSON(http.StatusInternalServerError, errorResponse{false, "TERMINOLOGY_DIR or DATA_HOME_DIR is not set"})
	}
	id := terminology.ResourceID(c.Param("source"))
	if _, ok := terminology.ResourceByID(id); !ok {
		return c.JSON(http.StatusNotFound, errorResponse{false, "unknown resource: " + string(id)})
	}
	opts := []terminology.FetchOption{terminology.WithClient(&http.Client{Timeout: downloadTimeout})}
	if ids := c.QueryParam("ids"); ids != "" {
		opts = append(opts, terminology.WithWikidataIDs(strings.Split(ids, "|")))
	}
	ctx, cancel := context.WithTimeout(c.Request().Context(), downloadTimeout)
	defer cancel()

	st, err := terminology.Fetch(ctx, id, dir, opts...)
	res, _ := terminology.ResourceByID(id)
	if err != nil {
		// Permission-gated resources are a client error; transport failures are server-side.
		code := http.StatusInternalServerError
		if !res.Downloadable {
			code = http.StatusForbidden
		}
		return c.JSON(code, errorResponse{false, err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "resource": viewFor(dir, res, st)})
}
