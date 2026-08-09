package terminologyresourcehandler

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/ontology/keywords"
	"github.com/chendingplano/deepdoc/server/api/ontology/terminology"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

// SetAutoPromotePolicy handles PUT /api/v1/terminology-resources/:source/promotion-policy,
// letting a System Admin operator enable or disable automatic promotion of
// one resource's staged catalog entries into keyword concepts. Access to
// this endpoint is gated the same way as the rest of this package's routes
// (System Admin > Resources) -- there is no additional source-trust gate on
// top of that (keyword-catalog-auto-promotion openspec change, design.md
// Decision 3).
func SetAutoPromotePolicy(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_TER_003")
	defer rc.Close()
	logger := rc.GetLogger()

	id := terminology.ResourceID(c.Param("source"))
	res, ok := terminology.ResourceByID(id)
	if !ok {
		return c.JSON(http.StatusNotFound, errorResponse{false, "unknown resource: " + string(id)})
	}
	enabledStr := strings.TrimSpace(c.FormValue("enabled"))
	enabled, err := strconv.ParseBool(enabledStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{false, "enabled must be true or false"})
	}
	setBy := ""
	if u := rc.IsAuthenticated(); u != nil {
		setBy = strings.TrimSpace(u.Email)
	}
	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return c.JSON(http.StatusInternalServerError, errorResponse{false, "database handle is not configured"})
	}
	if _, err := (keywords.PromotionPolicyStore{DB: db}).Set(c.Request().Context(), res.Source, enabled, setBy); err != nil {
		logger.Warn("set auto-promotion policy failed", "source", res.Source, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{false, err.Error()})
	}
	logger.Info("set auto-promotion policy", "source", res.Source, "enabled", enabled, "set_by", setBy)
	return c.JSON(http.StatusOK, map[string]any{"status": true, "auto_promote_enabled": enabled})
}

// triggerCatalogAutoPromotion fires PromoteCatalogEntries in the background
// for the resource just approved, unless its promotion policy is disabled.
// It never blocks the Approve response and never surfaces its outcome to the
// caller -- errors are logged only (keyword-catalog-auto-promotion openspec
// change). db and logger are captured here, before the goroutine starts,
// since the request's own context and logger are torn down once
// ApproveResource returns.
func triggerCatalogAutoPromotion(ctx context.Context, db *sql.DB, id terminology.ResourceID, st terminology.FetchStatus, logger ApiTypes.JimoLogger) {
	res, ok := terminology.ResourceByID(id)
	if !ok {
		return
	}
	enabled, err := (keywords.PromotionPolicyStore{DB: db}).IsEnabled(ctx, res.Source)
	if err != nil {
		logger.Warn("check auto-promotion policy failed", "source", res.Source, "err", err)
		return
	}
	if !enabled {
		return
	}
	release := res.Release
	if st.Release != "" {
		release = st.Release
	}
	scope := ""
	if len(res.AllowedScopes) > 0 {
		scope = res.AllowedScopes[0]
	}

	go func() {
		counts, err := keywords.PromoteCatalogEntries(context.Background(), db, res.Source, release, scope)
		if err != nil {
			logger.Warn("auto-promote catalog entries failed", "source", res.Source, "release", release, "err", err)
			return
		}
		logger.Info("auto-promoted catalog entries",
			"source", res.Source, "release", release,
			"entries_scanned", counts.EntriesScanned, "concepts_created", counts.ConceptsCreated,
			"concepts_converged", counts.ConceptsConverged, "errors", counts.Errors)
	}()
}
