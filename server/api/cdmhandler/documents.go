package cdmhandler

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"

	"github.com/chendingplano/deepdoc/server/api/cdm/rendering"
	"github.com/chendingplano/deepdoc/server/api/cdm/store"
)

// keyParam reads the :key path parameter decoded.
//
// Echo's c.Param does not URL-decode path segments (confirmed against the
// installed echo/v4: a route registered as "/documents/:key" hit with
// "/documents/doc%3Achentest-01" returns c.Param("key") == "doc%3Achentest-01"
// verbatim). document_key contains ":" (design D5, "doc:<slug>"), which every
// client must percent-encode to survive as a single path segment
// (cdm-client.ts's encodeURIComponent), so every handler that reads this
// param needs the matching decode or it looks up a key nothing ever created —
// same fix useradminhandler.go's normalizeEmailParam already applies to its
// own path parameter.
func keyParam(c echo.Context) (string, error) {
	return url.PathUnescape(c.Param("key"))
}

// CreateDocument handles POST /api/v1/cdm/documents.
//
// The body is canonical document JSON; any document_key it carries is
// ignored, since keys are server-allocated (ADR 2026072603 DR5). tenant_id
// and ks_store_id come from query parameters, matching how the existing
// upload handler takes them.
func CreateDocument(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_CDM_001")
	defer rc.Close()
	logger := rc.GetLogger()

	doc, err := decodeDocument(c)
	if err != nil {
		return fail(c, http.StatusBadRequest, "%v (CWB_CDM_002)", err)
	}
	if strings.TrimSpace(doc.Title) == "" {
		return fail(c, http.StatusBadRequest, "title is required (CWB_CDM_003)")
	}
	if doc.SchemaVersion == "" {
		return fail(c, http.StatusBadRequest, "schema_version is required (CWB_CDM_004)")
	}

	tenantID := strings.TrimSpace(c.QueryParam("tenant_id"))
	if tenantID == "" {
		return fail(c, http.StatusBadRequest, "tenant_id is required (CWB_CDM_005)")
	}
	var ksStoreID sql.NullInt64
	if raw := strings.TrimSpace(c.QueryParam("ks_store_id")); raw != "" {
		v := parsePositiveInt(raw, 0)
		if v == 0 {
			return fail(c, http.StatusBadRequest, "invalid ks_store_id (CWB_CDM_006)")
		}
		ksStoreID = sql.NullInt64{Int64: int64(v), Valid: true}
	}

	ctx := c.Request().Context()
	db := ApiTypes.ProjectDBHandle

	key, err := allocateDocumentKey(ctx, db, doc.Title)
	if err != nil {
		logger.Error("allocate document_key failed", "err", err)
		return fail(c, http.StatusInternalServerError, "could not allocate a document key (CWB_CDM_007)")
	}
	doc.Key = key

	res, err := store.New(db).Create(ctx, doc, store.DraftInput{
		TenantID:  tenantID,
		KSStoreID: ksStoreID,
		Title:     doc.Title,
	})
	if err != nil {
		return writeStoreError(c, err, logger.Error)
	}

	logger.Info("cdm document created",
		"document_key", doc.Key, "input_record_id", res.InputRecordID, "tenant_id", tenantID)
	return c.JSON(http.StatusCreated, doc)
}

// GetDocument handles GET /api/v1/cdm/documents/:key, returning the canonical
// document JSON directly — there is no wrapper and no DTO (design D2).
func GetDocument(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_CDM_020")
	defer rc.Close()
	logger := rc.GetLogger()

	key, err := keyParam(c)
	if err != nil {
		return fail(c, http.StatusBadRequest, "invalid document key (CWB_CDM_022)")
	}
	if strings.TrimSpace(key) == "" {
		return fail(c, http.StatusBadRequest, "document key is required (CWB_CDM_021)")
	}

	doc, err := store.New(ApiTypes.ProjectDBHandle).Load(c.Request().Context(), key)
	if err != nil {
		return writeStoreError(c, err, logger.Error)
	}
	return c.JSON(http.StatusOK, doc)
}

// SaveDocument handles PUT /api/v1/cdm/documents/:key.
//
// The body's own content_version is the caller's expectation (DR6): a client
// loads version 7, sends it back, and the store writes 8 only if 7 is still
// current. This needs no extra header or parameter because the version is
// already a first-class field of the document.
func SaveDocument(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_CDM_030")
	defer rc.Close()
	logger := rc.GetLogger()

	key, err := keyParam(c)
	if err != nil {
		return fail(c, http.StatusBadRequest, "invalid document key (CWB_CDM_034)")
	}
	if strings.TrimSpace(key) == "" {
		return fail(c, http.StatusBadRequest, "document key is required (CWB_CDM_031)")
	}

	doc, err := decodeDocument(c)
	if err != nil {
		return fail(c, http.StatusBadRequest, "%v (CWB_CDM_032)", err)
	}
	if doc.Key != "" && doc.Key != key {
		return fail(c, http.StatusBadRequest,
			"document_key in the body (%q) does not match the URL (%q) (CWB_CDM_033)", doc.Key, key)
	}
	doc.Key = key

	expected := doc.ContentVersion
	if _, err := store.New(ApiTypes.ProjectDBHandle).Save(c.Request().Context(), doc, expected); err != nil {
		return writeStoreError(c, err, logger.Error)
	}

	logger.Info("cdm document saved", "document_key", key, "content_version", doc.ContentVersion)
	return c.JSON(http.StatusOK, doc)
}

// ListDocuments handles GET /api/v1/cdm/documents.
//
// tenant_id is a query parameter rather than something derived from the
// session, because ApiTypes.UserInfo carries no tenant and the rest of this
// API takes tenant_id from the client the same way. That makes this a filter,
// not an isolation boundary — see the note in the change's design.
func ListDocuments(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_CDM_040")
	defer rc.Close()
	logger := rc.GetLogger()

	page := parsePositiveInt(c.QueryParam("page"), 1)
	pageSize := parsePositiveInt(c.QueryParam("page_size"), defaultPageSize)
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	tenantID := strings.TrimSpace(c.QueryParam("tenant_id"))

	rows, err := ApiTypes.ProjectDBHandle.QueryContext(c.Request().Context(), `
		SELECT d.document_key, d.title, d.content_version,
		       COALESCE(d.doc_type, ''), COALESCE(d.rendering_type, ''),
		       COALESCE(NOT (i.status @> '[{"operation":"doc_processing"}]'::jsonb), false),
		       d.create_time, d.update_time
		FROM kb.cdm_documents d
		LEFT JOIN kb.inputs i ON i.id = d.input_record_id
		WHERE ($1 = '' OR i.tenant_id = $1)
		ORDER BY d.update_time DESC
		LIMIT $2 OFFSET $3
	`, tenantID, pageSize, (page-1)*pageSize)
	if err != nil {
		logger.Error("list cdm documents failed", "err", err)
		return fail(c, http.StatusInternalServerError, "internal error (CWB_CDM_041)")
	}
	defer rows.Close()

	results := make([]documentSummary, 0, pageSize)
	for rows.Next() {
		var (
			r                      documentSummary
			createTime, updateTime sql.NullTime
		)
		if err := rows.Scan(&r.DocumentKey, &r.Title, &r.ContentVersion,
			&r.DocType, &r.RenderingType, &r.Published, &createTime, &updateTime); err != nil {
			logger.Error("scan cdm document row failed", "err", err)
			return fail(c, http.StatusInternalServerError, "internal error (CWB_CDM_042)")
		}
		if createTime.Valid {
			r.CreateTime = createTime.Time.UTC().Format(timeFormat)
		}
		if updateTime.Valid {
			r.UpdateTime = updateTime.Time.UTC().Format(timeFormat)
		}
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		logger.Error("iterate cdm document rows failed", "err", err)
		return fail(c, http.StatusInternalServerError, "internal error (CWB_CDM_043)")
	}

	return c.JSON(http.StatusOK, listResponse{
		Status: true, Results: results, Page: page, PageSize: pageSize,
	})
}

// PublishDocument handles POST /api/v1/cdm/documents/:key/publish. It renders
// the document, stores its SVG pages, anchors, and line file, and transitions
// the kb.inputs row so the standard doc-processing worklist enqueues it
// (CDM §10.1). Publishing also freezes the document (D8).
func PublishDocument(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_CDM_050")
	defer rc.Close()
	logger := rc.GetLogger()

	key, err := keyParam(c)
	if err != nil {
		return fail(c, http.StatusBadRequest, "invalid document key (CWB_CDM_055)")
	}
	if strings.TrimSpace(key) == "" {
		return fail(c, http.StatusBadRequest, "document key is required (CWB_CDM_051)")
	}

	ctx := c.Request().Context()
	db := ApiTypes.ProjectDBHandle

	inputID, err := lookupInputRecordID(ctx, db, key)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fail(c, http.StatusNotFound, "document %q not found (CWB_CDM_052)", key)
		}
		if errors.Is(err, errNoInputRow) {
			return fail(c, http.StatusConflict,
				"document %q has no linked input row and cannot be published (CWB_CDM_053)", key)
		}
		logger.Error("lookup input record failed", "document_key", key, "err", err)
		return fail(c, http.StatusInternalServerError, "internal error (CWB_CDM_054)")
	}

	pub := store.NewPublisher(db, rendering.DefaultTheme, "")
	res, err := pub.Publish(ctx, key, inputID)
	if err != nil {
		return writeStoreError(c, err, logger.Error)
	}

	logger.Info("cdm document published",
		"document_key", key, "content_version", res.ContentVersion, "pages", res.PageCount)
	return c.JSON(http.StatusOK, publishResponse{
		Status: true, ContentVersion: res.ContentVersion, PageCount: res.PageCount,
	})
}

// RenderDocument handles GET /api/v1/cdm/documents/:key/render, returning the
// SVG pages for the document's current content_version.
//
// Rendering is on demand and cached by content_version (design D9): a repeat
// request for an unchanged version is a table read, and Typst is invoked only
// on a miss. It is never triggered per keystroke.
func RenderDocument(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_CDM_060")
	defer rc.Close()
	logger := rc.GetLogger()

	key, err := keyParam(c)
	if err != nil {
		return fail(c, http.StatusBadRequest, "invalid document key (CWB_CDM_065)")
	}
	if strings.TrimSpace(key) == "" {
		return fail(c, http.StatusBadRequest, "document key is required (CWB_CDM_061)")
	}

	ctx := c.Request().Context()
	db := ApiTypes.ProjectDBHandle

	var (
		docID          int64
		contentVersion int64
	)
	err = db.QueryRowContext(ctx,
		`SELECT id, content_version FROM kb.cdm_documents WHERE document_key = $1`, key,
	).Scan(&docID, &contentVersion)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fail(c, http.StatusNotFound, "document %q not found (CWB_CDM_062)", key)
		}
		logger.Error("load document for render failed", "document_key", key, "err", err)
		return fail(c, http.StatusInternalServerError, "internal error (CWB_CDM_063)")
	}

	pages, err := loadCachedPages(ctx, db, docID, contentVersion)
	if err != nil {
		logger.Error("read cached renderings failed", "document_key", key, "err", err)
		return fail(c, http.StatusInternalServerError, "internal error (CWB_CDM_064)")
	}

	if len(pages) == 0 {
		// Render, never Publish: previewing a draft must not freeze it (D8).
		pub := store.NewPublisher(db, rendering.DefaultTheme, "")
		if _, err := pub.Render(ctx, key); err != nil {
			return writeStoreError(c, err, logger.Error)
		}
		if pages, err = loadCachedPages(ctx, db, docID, contentVersion); err != nil {
			logger.Error("read renderings after render failed", "document_key", key, "err", err)
			return fail(c, http.StatusInternalServerError, "internal error (CWB_CDM_065)")
		}
	}

	return c.JSON(http.StatusOK, renderResponse{
		Status: true, ContentVersion: contentVersion, Pages: pages,
	})
}

const timeFormat = "2006-01-02T15:04:05Z"

var errNoInputRow = errors.New("cdm: document has no linked kb.inputs row")

// lookupInputRecordID resolves a document_key to the kb.inputs row backing
// it. This is the traversal Store.Create's input_record_id exists to make
// possible; a NULL means the document predates Store.Create.
func lookupInputRecordID(ctx context.Context, db *sql.DB, key string) (int64, error) {
	var id sql.NullInt64
	if err := db.QueryRowContext(ctx,
		`SELECT input_record_id FROM kb.cdm_documents WHERE document_key = $1`, key,
	).Scan(&id); err != nil {
		return 0, err
	}
	if !id.Valid {
		return 0, errNoInputRow
	}
	return id.Int64, nil
}

// loadCachedPages reads the stored SVG pages for a content version, in page
// order. An empty result means nothing has been rendered for that version yet.
func loadCachedPages(ctx context.Context, db *sql.DB, docID, contentVersion int64) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT rendered_content
		FROM kb.cdm_renderings
		WHERE document_id = $1 AND content_version = $2 AND media_type = 'image/svg+xml'
		ORDER BY page
	`, docID, contentVersion)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pages []string
	for rows.Next() {
		var content []byte
		if err := rows.Scan(&content); err != nil {
			return nil, err
		}
		pages = append(pages, string(content))
	}
	return pages, rows.Err()
}
