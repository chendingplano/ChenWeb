package kbhandler

import (
	"net/http"
	"strconv"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

// GetOntologyReviewFinding returns the immutable provenance links of a P4
// finding without conflating it with generic document-review metadata.
func GetOntologyReviewFinding(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid finding id"})
	}
	var out struct {
		ID            int64  `json:"id"`
		ReviewScopeID string `json:"review_scope_id"`
		ProfileRuleID int64  `json:"profile_rule_id"`
		AssertionID   int64  `json:"assertion_id"`
	}
	err = ApiTypes.ProjectDBHandle.QueryRowContext(c.Request().Context(), `SELECT id, COALESCE(review_scope_id, ''), COALESCE(profile_rule_id, 0), COALESCE(assertion_id, 0) FROM kb.doc_review_findings WHERE id = $1`, id).Scan(&out.ID, &out.ReviewScopeID, &out.ProfileRuleID, &out.AssertionID)
	if err != nil {
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "ontology review finding not found"})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "record": out})
}
