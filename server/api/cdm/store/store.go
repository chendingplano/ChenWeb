// Package store persists CDM canonical documents to the kb.cdm_* schema
// (spec §11) and registers them in kb.inputs (spec §10.1). This is the only
// cdm package that touches the database.
package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/chendingplano/deepdoc/server/api/cdm/model"
	"github.com/lib/pq"
)

// Store persists CDM documents against a *sql.DB. All methods are safe for
// concurrent use to the extent the underlying *sql.DB is.
type Store struct {
	db *sql.DB
}

// New returns a Store backed by db.
func New(db *sql.DB) *Store {
	return &Store{db: db}
}

// ConflictError is returned when a save fails because a block slug collides
// with an existing block in the same document (spec §11, "Block slug
// uniqueness is enforced by the database"). Per ADR 2026072501 DR1/D8, the
// system never auto-renames a colliding slug — the caller must resolve it.
type ConflictError struct {
	BlockID string
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("cdm: block id %q already exists in this document", e.BlockID)
}

// NotFoundError is returned when a document_key resolves to no document.
type NotFoundError struct {
	DocumentKey string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("cdm: document %q not found", e.DocumentKey)
}

// StaleVersionError is returned when a save's expected edit_version no longer
// matches what is stored — the document changed since the caller loaded it.
// Actual is 0 when the document does not exist at all.
type StaleVersionError struct {
	DocumentKey string
	Expected    int64
	Actual      int64
}

func (e *StaleVersionError) Error() string {
	if e.Actual == 0 {
		return fmt.Sprintf("cdm: document %q does not exist (expected edit_version %d)",
			e.DocumentKey, e.Expected)
	}
	return fmt.Sprintf("cdm: document %q changed since it was loaded (expected edit_version %d, found %d)",
		e.DocumentKey, e.Expected, e.Actual)
}

// FrozenError is returned when a save targets a published document. A
// published document is read-only; continuing requires opening a new version
// (spec 2026072502 D8).
type FrozenError struct {
	DocumentKey string
}

func (e *FrozenError) Error() string {
	return fmt.Sprintf("cdm: document %q is published and cannot be modified", e.DocumentKey)
}

// SaveResult reports the outcome of a successful save.
type SaveResult struct {
	DocumentID     int64
	ContentVersion int64
	EditVersion    int64
}

// CreateResult reports the outcome of a successful Create.
type CreateResult struct {
	DocumentID     int64
	InputRecordID  int64
	ContentVersion int64
	EditVersion    int64
}

// VersionNode is one visible content version in a document's version history.
type VersionNode struct {
	ContentVersion       int64
	ParentContentVersion sql.NullInt64
	CreateTime           time.Time
	UpdateTime           time.Time
	SizeBytes            int64
	Current              bool
}

// Save persists doc transactionally without advancing the visible
// content_version. edit_version, not content_version, is the concurrency
// token for "save current version".
func (s *Store) Save(ctx context.Context, doc *model.Document, expectedVersion int64) (*SaveResult, error) {
	return s.save(ctx, doc, expectedVersion, false)
}

// SaveToNewVersion persists doc transactionally while advancing the visible
// content_version and recording a new version-history snapshot.
func (s *Store) SaveToNewVersion(ctx context.Context, doc *model.Document, expectedVersion int64) (*SaveResult, error) {
	return s.save(ctx, doc, expectedVersion, true)
}

func (s *Store) save(ctx context.Context, doc *model.Document, expectedVersion int64, advanceContentVersion bool) (*SaveResult, error) {
	if err := model.Validate(doc); err != nil {
		return nil, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("cdm: begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op if committed

	st, err := lockDocStateTx(ctx, tx, doc.Key)
	if err != nil {
		return nil, err
	}
	switch {
	case st.exists && st.published:
		return nil, &FrozenError{DocumentKey: doc.Key}
	case st.exists && st.editVersion != expectedVersion:
		return nil, &StaleVersionError{DocumentKey: doc.Key, Expected: expectedVersion, Actual: st.editVersion}
	case !st.exists && expectedVersion != 0:
		return nil, &StaleVersionError{DocumentKey: doc.Key, Expected: expectedVersion, Actual: 0}
	}

	var (
		id             int64
		contentVersion int64
		editVersion    int64
	)

	updateContentExpr := "kb.cdm_documents.content_version"
	if advanceContentVersion {
		updateContentExpr = "kb.cdm_documents.content_version + 1"
	}

	err = tx.QueryRowContext(ctx, fmt.Sprintf(`
		INSERT INTO kb.cdm_documents
			(document_key, title, language, schema_version, content_version, edit_version,
			 doc_type, rendering_type, authors, doc_version, semantic_document,
			 update_time)
		VALUES ($1, $2, $3, $4, 1, 1, $5, $6, $7, $8, '{}'::jsonb, NOW())
		ON CONFLICT (document_key) DO UPDATE SET
			title             = EXCLUDED.title,
			language          = EXCLUDED.language,
			schema_version    = EXCLUDED.schema_version,
			content_version   = %s,
			edit_version      = kb.cdm_documents.edit_version + 1,
			doc_type          = EXCLUDED.doc_type,
			rendering_type    = EXCLUDED.rendering_type,
			authors           = EXCLUDED.authors,
			doc_version       = EXCLUDED.doc_version,
			update_time       = NOW()
		RETURNING id, content_version, edit_version
	`, updateContentExpr),
		doc.Key, doc.Title, nullableString(doc.Language), doc.SchemaVersion,
		nullableString(doc.Metadata.DocType), nullableString(doc.Metadata.RenderingType),
		pq.Array(nonNilStrings(doc.Metadata.Authors)), nullableString(doc.Metadata.Version),
	).Scan(&id, &contentVersion, &editVersion)
	if err != nil {
		return nil, fmt.Errorf("cdm: upsert document: %w", err)
	}

	parentVersion := sql.NullInt64{}
	if advanceContentVersion && st.exists {
		parentVersion = sql.NullInt64{Int64: st.contentVersion, Valid: true}
	}
	if err := writeContentTx(ctx, tx, id, contentVersion, editVersion, doc, advanceContentVersion, parentVersion); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("cdm: commit: %w", err)
	}

	doc.ContentVersion = contentVersion
	doc.EditVersion = editVersion
	return &SaveResult{DocumentID: id, ContentVersion: contentVersion, EditVersion: editVersion}, nil
}

// Create writes a new CDM document and the kb.inputs row backing it in a
// single transaction, linking them via kb.cdm_documents.input_record_id.
func (s *Store) Create(ctx context.Context, doc *model.Document, in DraftInput) (*CreateResult, error) {
	if err := model.Validate(doc); err != nil {
		return nil, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("cdm: begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op if committed

	inputID, err := createDraftTx(ctx, tx, in)
	if err != nil {
		return nil, err
	}

	const (
		initialVersion     = 1
		initialEditVersion = 1
	)
	var id int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO kb.cdm_documents
			(document_key, title, language, schema_version, content_version, edit_version,
			 doc_type, rendering_type, authors, doc_version, input_record_id,
			 semantic_document, update_time)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, '{}'::jsonb, NOW())
		RETURNING id
	`,
		doc.Key, doc.Title, nullableString(doc.Language), doc.SchemaVersion, initialVersion, initialEditVersion,
		nullableString(doc.Metadata.DocType), nullableString(doc.Metadata.RenderingType),
		pq.Array(nonNilStrings(doc.Metadata.Authors)), nullableString(doc.Metadata.Version),
		inputID,
	).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, fmt.Errorf("cdm: document_key %q already exists", doc.Key)
		}
		return nil, fmt.Errorf("cdm: insert document: %w", err)
	}

	if err := writeContentTx(ctx, tx, id, initialVersion, initialEditVersion, doc, true, sql.NullInt64{}); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("cdm: commit: %w", err)
	}

	doc.ContentVersion = initialVersion
	doc.EditVersion = initialEditVersion
	return &CreateResult{
		DocumentID:     id,
		InputRecordID:  inputID,
		ContentVersion: initialVersion,
		EditVersion:    initialEditVersion,
	}, nil
}

// Load reads a document by its document_key.
func (s *Store) Load(ctx context.Context, documentKey string) (*model.Document, error) {
	var (
		docJSON        []byte
		contentVersion int64
		editVersion    int64
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT semantic_document, content_version, edit_version
		FROM kb.cdm_documents
		WHERE document_key = $1
	`, documentKey).Scan(&docJSON, &contentVersion, &editVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &NotFoundError{DocumentKey: documentKey}
	}
	if err != nil {
		return nil, fmt.Errorf("cdm: load document %q: %w", documentKey, err)
	}

	var doc model.Document
	if err := json.Unmarshal(docJSON, &doc); err != nil {
		return nil, fmt.Errorf("cdm: decode document %q: %w", documentKey, err)
	}
	doc.ContentVersion = contentVersion
	doc.EditVersion = editVersion
	return &doc, nil
}

// ListVersions returns the visible content-version history for one document.
func (s *Store) ListVersions(ctx context.Context, documentKey string) ([]VersionNode, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT v.content_version,
		       v.parent_content_version,
		       v.create_time,
		       v.update_time,
		       v.size_bytes,
		       (v.content_version = d.content_version) AS current
		FROM kb.cdm_document_versions v
		JOIN kb.cdm_documents d ON d.id = v.document_id
		WHERE d.document_key = $1
		ORDER BY v.content_version DESC
	`, documentKey)
	if err != nil {
		return nil, fmt.Errorf("cdm: list versions for %q: %w", documentKey, err)
	}
	defer rows.Close()

	var versions []VersionNode
	for rows.Next() {
		var v VersionNode
		if err := rows.Scan(
			&v.ContentVersion,
			&v.ParentContentVersion,
			&v.CreateTime,
			&v.UpdateTime,
			&v.SizeBytes,
			&v.Current,
		); err != nil {
			return nil, fmt.Errorf("cdm: scan version for %q: %w", documentKey, err)
		}
		versions = append(versions, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cdm: iterate versions for %q: %w", documentKey, err)
	}
	if len(versions) == 0 {
		return nil, &NotFoundError{DocumentKey: documentKey}
	}
	return versions, nil
}

func writeContentTx(
	ctx context.Context,
	tx *sql.Tx,
	id, contentVersion, editVersion int64,
	doc *model.Document,
	newVersion bool,
	parentVersion sql.NullInt64,
) error {
	stamped := *doc
	stamped.ContentVersion = contentVersion
	stamped.EditVersion = editVersion
	docJSON, err := json.Marshal(&stamped)
	if err != nil {
		return fmt.Errorf("cdm: marshal document: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE kb.cdm_documents SET semantic_document = $1 WHERE id = $2
	`, docJSON, id); err != nil {
		return fmt.Errorf("cdm: store semantic_document: %w", err)
	}

	if newVersion {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO kb.cdm_document_versions
				(document_id, content_version, parent_content_version, semantic_document, size_bytes, update_time)
			VALUES ($1, $2, $3, $4, $5, NOW())
			ON CONFLICT (document_id, content_version) DO UPDATE SET
				parent_content_version = EXCLUDED.parent_content_version,
				semantic_document = EXCLUDED.semantic_document,
				size_bytes = EXCLUDED.size_bytes,
				update_time = NOW()
		`, id, contentVersion, nullableInt64(parentVersion), docJSON, int64(len(docJSON))); err != nil {
			return fmt.Errorf("cdm: upsert document version %d: %w", contentVersion, err)
		}
	} else {
		if _, err := tx.ExecContext(ctx, `
			UPDATE kb.cdm_document_versions
			SET semantic_document = $3, size_bytes = $4, update_time = NOW()
			WHERE document_id = $1 AND content_version = $2
		`, id, contentVersion, docJSON, int64(len(docJSON))); err != nil {
			return fmt.Errorf("cdm: update document version %d: %w", contentVersion, err)
		}
		if err := clearArtifactsForVersionTx(ctx, tx, id, contentVersion); err != nil {
			return err
		}
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM kb.cdm_blocks WHERE document_id = $1`, id); err != nil {
		return fmt.Errorf("cdm: clear prior blocks: %w", err)
	}

	rows := flattenBlocks(doc.Blocks, "")
	for _, r := range rows {
		contentJSON, err := json.Marshal(r.block)
		if err != nil {
			return fmt.Errorf("cdm: marshal block %q: %w", r.blockID, err)
		}

		_, err = tx.ExecContext(ctx, `
			INSERT INTO kb.cdm_blocks
				(document_id, block_id, parent_block_id, block_type, block_role,
				 ordinal, semantic_content, content_hash, update_time)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
		`,
			id, r.blockID, nullableString(r.parentBlockID), r.block.Type,
			nullableString(r.block.Role), r.ordinal, contentJSON, contentHash(contentJSON),
		)
		if err != nil {
			if isUniqueViolation(err) {
				return &ConflictError{BlockID: r.blockID}
			}
			return fmt.Errorf("cdm: insert block %q: %w", r.blockID, err)
		}
	}
	return nil
}

type blockRow struct {
	block         model.Block
	blockID       string
	parentBlockID string
	ordinal       int
}

// flattenBlocks walks the block tree and produces one row per block,
// including nested blocks in children and list items. List items are
// flattened in item order with a running ordinal under the list block, since
// full grouping is retained in the authoritative semantic_document JSON
// (spec §11: kb.cdm_blocks is a derived index, not the source of truth).
func flattenBlocks(blocks []model.Block, parentBlockID string) []blockRow {
	var rows []blockRow
	ordinal := 0
	for _, b := range blocks {
		rows = append(rows, blockRow{block: b, blockID: b.ID, parentBlockID: parentBlockID, ordinal: ordinal})
		ordinal++
		rows = append(rows, flattenBlocks(b.Children, b.ID)...)
		for _, item := range b.Items {
			for _, ib := range item {
				rows = append(rows, blockRow{block: ib, blockID: ib.ID, parentBlockID: b.ID, ordinal: ordinal})
				ordinal++
				rows = append(rows, flattenBlocks(ib.Children, ib.ID)...)
			}
		}
	}
	return rows
}

func contentHash(canonicalJSON []byte) string {
	sum := sha256.Sum256(canonicalJSON)
	return hex.EncodeToString(sum[:])
}

func nullableString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}

func nullableInt64(v sql.NullInt64) any {
	if !v.Valid {
		return nil
	}
	return v.Int64
}

// nonNilStrings returns ss, or an empty (non-nil) slice if ss is nil, so
// pq.Array never encodes as SQL NULL against the NOT NULL authors column.
func nonNilStrings(ss []string) []string {
	if ss == nil {
		return []string{}
	}
	return ss
}

func clearArtifactsForVersionTx(ctx context.Context, tx *sql.Tx, documentID, contentVersion int64) error {
	for _, table := range []string{"kb.cdm_renderings", "kb.cdm_projections", "kb.cdm_anchors"} {
		if _, err := tx.ExecContext(ctx,
			fmt.Sprintf(`DELETE FROM %s WHERE document_id = $1 AND content_version = $2`, table),
			documentID, contentVersion,
		); err != nil {
			return fmt.Errorf("cdm: clear %s for version %d: %w", table, contentVersion, err)
		}
	}
	return nil
}

// isUniqueViolation reports whether an error is a Postgres unique-constraint
// violation, matching the convention in
// server/api/agentplatformhandler/workspace.go: string-match on SQLSTATE
// 23505 rather than importing the pq error type.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "duplicate key value") ||
		strings.Contains(s, "23505") ||
		strings.Contains(s, "unique constraint")
}
