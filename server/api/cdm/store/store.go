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
	"fmt"
	"strings"

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

// SaveResult reports the outcome of a successful Save.
type SaveResult struct {
	DocumentID     int64
	ContentVersion int64
}

// Save validates doc, then persists it transactionally: upserts
// kb.cdm_documents by document_key, increments content_version, and fully
// rebuilds kb.cdm_blocks from the new content (spec "Canonical JSON is
// authoritative"; design D5). No write occurs if validation fails.
//
// content_version is server-assigned; on success, doc.ContentVersion is
// updated in place to the resolved value. doc is left unmodified if Save
// returns an error.
func (s *Store) Save(ctx context.Context, doc *model.Document) (*SaveResult, error) {
	if err := model.Validate(doc); err != nil {
		return nil, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("cdm: begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op if committed

	// content_version is server-assigned (the DB column is the counter), so
	// the upsert runs once to resolve it, then the caller's document is
	// stamped with the resolved version and (re-)marshalled before being
	// written as semantic_document. Otherwise a Save+Load round trip could
	// return a stale content_version baked into the stored JSON from before
	// this write — the "Content versioning" requirement (spec §11) is about
	// the resolved version, not whatever the caller happened to pass in.
	var (
		id             int64
		contentVersion int64
	)

	err = tx.QueryRowContext(ctx, `
		INSERT INTO kb.cdm_documents
			(document_key, title, language, schema_version, content_version,
			 doc_type, rendering_type, authors, doc_version, semantic_document,
			 update_time)
		VALUES ($1, $2, $3, $4, 1, $5, $6, $7, $8, '{}'::jsonb, NOW())
		ON CONFLICT (document_key) DO UPDATE SET
			title             = EXCLUDED.title,
			language          = EXCLUDED.language,
			schema_version    = EXCLUDED.schema_version,
			content_version   = kb.cdm_documents.content_version + 1,
			doc_type          = EXCLUDED.doc_type,
			rendering_type    = EXCLUDED.rendering_type,
			authors           = EXCLUDED.authors,
			doc_version       = EXCLUDED.doc_version,
			update_time       = NOW()
		RETURNING id, content_version
	`,
		doc.Key, doc.Title, nullableString(doc.Language), doc.SchemaVersion,
		nullableString(doc.Metadata.DocType), nullableString(doc.Metadata.RenderingType),
		pq.Array(nonNilStrings(doc.Metadata.Authors)), nullableString(doc.Metadata.Version),
	).Scan(&id, &contentVersion)
	if err != nil {
		return nil, fmt.Errorf("cdm: upsert document: %w", err)
	}

	// Marshal a stamped copy rather than mutating the caller's doc here: the
	// transaction can still fail below (a block slug conflict, for example),
	// and the caller's Document should not appear to have a new
	// content_version unless the save actually commits.
	stamped := *doc
	stamped.ContentVersion = contentVersion
	docJSON, err := json.Marshal(&stamped)
	if err != nil {
		return nil, fmt.Errorf("cdm: marshal document: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE kb.cdm_documents SET semantic_document = $1 WHERE id = $2
	`, docJSON, id); err != nil {
		return nil, fmt.Errorf("cdm: store semantic_document: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM kb.cdm_blocks WHERE document_id = $1`, id); err != nil {
		return nil, fmt.Errorf("cdm: clear prior blocks: %w", err)
	}

	rows := flattenBlocks(doc.Blocks, "")
	for _, r := range rows {
		contentJSON, err := json.Marshal(r.block)
		if err != nil {
			return nil, fmt.Errorf("cdm: marshal block %q: %w", r.blockID, err)
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
				return nil, &ConflictError{BlockID: r.blockID}
			}
			return nil, fmt.Errorf("cdm: insert block %q: %w", r.blockID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("cdm: commit: %w", err)
	}

	doc.ContentVersion = contentVersion
	return &SaveResult{DocumentID: id, ContentVersion: contentVersion}, nil
}

// Load reads a document by its document_key.
func (s *Store) Load(ctx context.Context, documentKey string) (*model.Document, error) {
	var docJSON []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT semantic_document FROM kb.cdm_documents WHERE document_key = $1
	`, documentKey).Scan(&docJSON)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("cdm: document %q not found", documentKey)
	}
	if err != nil {
		return nil, fmt.Errorf("cdm: load document %q: %w", documentKey, err)
	}

	var doc model.Document
	if err := json.Unmarshal(docJSON, &doc); err != nil {
		return nil, fmt.Errorf("cdm: decode document %q: %w", documentKey, err)
	}
	return &doc, nil
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

// nonNilStrings returns ss, or an empty (non-nil) slice if ss is nil, so
// pq.Array never encodes as SQL NULL against the NOT NULL authors column.
func nonNilStrings(ss []string) []string {
	if ss == nil {
		return []string{}
	}
	return ss
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

