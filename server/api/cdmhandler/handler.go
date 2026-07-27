// Package cdmhandler exposes the CDM Phase 1 packages over HTTP
// (ADR 2026072603 DR2). The handlers are deliberately thin: validation lives
// in cdm/model, persistence and its invariants — optimistic concurrency, the
// frozen-document rule — live in cdm/store, and rendering lives in
// cdm/rendering. Nothing here reimplements any of them.
package cdmhandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/chendingplano/deepdoc/server/api/cdm/model"
	"github.com/chendingplano/deepdoc/server/api/cdm/store"
)

const (
	defaultPageSize = 50
	maxPageSize     = 500
)

// Conflict discriminators, so a client receiving a 409 can tell the three
// cases apart and offer the right recovery: reload, or open a new version, or
// rename a block.
const (
	conflictStaleVersion = "stale_version"
	conflictFrozen       = "frozen"
	conflictBlockSlug    = "block_slug"
)

// errorResponse follows the kbhandler convention (status + error_msg), with
// three optional fields this API needs. Violations carries the full
// model.ValidationError list so the editor can attribute each violation to a
// block rather than showing one flattened string. Conflict and ContentVersion
// are set on 409s.
type errorResponse struct {
	Status         bool     `json:"status"`
	ErrorMsg       string   `json:"error_msg"`
	Violations     []string `json:"violations,omitempty"`
	Conflict       string   `json:"conflict,omitempty"`
	ContentVersion int64    `json:"content_version,omitempty"`
	EditVersion    int64    `json:"edit_version,omitempty"`
}

type documentSummary struct {
	DocumentKey    string `json:"document_key"`
	Title          string `json:"title"`
	ContentVersion int64  `json:"content_version"`
	DocType        string `json:"doc_type,omitempty"`
	RenderingType  string `json:"rendering_type,omitempty"`
	Published      bool   `json:"published"`
	CreateTime     string `json:"create_time"`
	UpdateTime     string `json:"update_time"`
}

type listResponse struct {
	Status   bool              `json:"status"`
	Results  []documentSummary `json:"results"`
	Page     int               `json:"page"`
	PageSize int               `json:"page_size"`
}

type publishResponse struct {
	Status         bool  `json:"status"`
	ContentVersion int64 `json:"content_version"`
	PageCount      int   `json:"page_count"`
}

type renderResponse struct {
	Status         bool     `json:"status"`
	ContentVersion int64    `json:"content_version"`
	Pages          []string `json:"pages"`
}

type versionNodeResponse struct {
	ContentVersion       int64  `json:"content_version"`
	ParentContentVersion *int64 `json:"parent_content_version,omitempty"`
	CreateTime           string `json:"create_time"`
	UpdateTime           string `json:"update_time"`
	SizeBytes            int64  `json:"size_bytes"`
	Current              bool   `json:"current"`
}

type versionsResponse struct {
	Status  bool                  `json:"status"`
	Results []versionNodeResponse `json:"results"`
}

func fail(c echo.Context, code int, format string, args ...any) error {
	return c.JSON(code, errorResponse{Status: false, ErrorMsg: fmt.Sprintf(format, args...)})
}

// writeStoreError maps the typed errors cdm/store returns onto HTTP. Every
// case a client can act on gets a distinct, machine-readable shape; anything
// else is a 500 with the detail logged rather than returned.
func writeStoreError(c echo.Context, err error, logf func(msg string, args ...any)) error {
	var (
		validation *model.ValidationError
		stale      *store.StaleVersionError
		frozen     *store.FrozenError
		conflict   *store.ConflictError
		notFound   *store.NotFoundError
	)
	switch {
	case errors.As(err, &notFound):
		return c.JSON(http.StatusNotFound, errorResponse{
			Status:   false,
			ErrorMsg: notFound.Error(),
		})
	case errors.As(err, &validation):
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:     false,
			ErrorMsg:   "document failed validation (CWB_CDM_010)",
			Violations: validation.Violations,
		})
	case errors.As(err, &stale):
		return c.JSON(http.StatusConflict, errorResponse{
			Status:         false,
			ErrorMsg:       stale.Error(),
			Conflict:       conflictStaleVersion,
			EditVersion:    stale.Actual,
		})
	case errors.As(err, &frozen):
		return c.JSON(http.StatusConflict, errorResponse{
			Status:   false,
			ErrorMsg: frozen.Error(),
			Conflict: conflictFrozen,
		})
	case errors.As(err, &conflict):
		return c.JSON(http.StatusConflict, errorResponse{
			Status:   false,
			ErrorMsg: conflict.Error(),
			Conflict: conflictBlockSlug,
		})
	default:
		logf("cdm store operation failed", "err", err)
		return fail(c, http.StatusInternalServerError, "internal error (CWB_CDM_090)")
	}
}

var slugStripRe = regexp.MustCompile(`[^a-z0-9]+`)

// slugifyTitle turns a document title into the human-readable slug half of a
// document_key (CDM §1.1: document_key is a stable, human-readable identity,
// not a surrogate). Non-alphanumeric runs collapse to a single hyphen.
//
// Note this is ASCII-oriented: a CJK title strips to empty and falls back to
// "document", which is correct but not useful. Improving it needs a
// transliteration decision that the MVP does not need to make.
func slugifyTitle(title string) string {
	s := slugStripRe.ReplaceAllString(strings.ToLower(strings.TrimSpace(title)), "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "document"
	}
	const maxSlug = 80
	if len(s) > maxSlug {
		s = strings.Trim(s[:maxSlug], "-")
	}
	return s
}

// allocateDocumentKey finds an unused document_key for title. Uniqueness is
// global (CDM §1.1) so only the server can do this; the numeric suffix is
// appended only on collision, keeping the common case readable.
//
// The unique index on document_key remains the real guarantee — this loop
// races benignly with a concurrent creator, and the loser gets a conflict
// error from Create rather than a silently wrong key.
func allocateDocumentKey(ctx context.Context, db *sql.DB, title string) (string, error) {
	base := "doc:" + slugifyTitle(title)
	candidate := base
	for i := 2; i < 1000; i++ {
		var exists bool
		err := db.QueryRowContext(ctx,
			`SELECT EXISTS(SELECT 1 FROM kb.cdm_documents WHERE document_key = $1)`, candidate,
		).Scan(&exists)
		if err != nil {
			return "", fmt.Errorf("check document_key %q: %w", candidate, err)
		}
		if !exists {
			return candidate, nil
		}
		candidate = fmt.Sprintf("%s-%d", base, i)
	}
	return "", fmt.Errorf("could not allocate a document_key for %q after 1000 attempts", title)
}

func parsePositiveInt(raw string, fallback int) int {
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}

func decodeDocument(c echo.Context) (*model.Document, error) {
	var doc model.Document
	if err := json.NewDecoder(c.Request().Body).Decode(&doc); err != nil {
		return nil, fmt.Errorf("request body is not valid canonical document JSON: %w", err)
	}
	return &doc, nil
}
