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

// StaleVersionError is returned when a save's expected content_version no
// longer matches what is stored — the document changed since the caller
// loaded it (ADR 2026072603 DR6). Actual is 0 when the document does not
// exist at all.
type StaleVersionError struct {
	DocumentKey string
	Expected    int64
	Actual      int64
}

func (e *StaleVersionError) Error() string {
	if e.Actual == 0 {
		return fmt.Sprintf("cdm: document %q does not exist (expected content_version %d)",
			e.DocumentKey, e.Expected)
	}
	return fmt.Sprintf("cdm: document %q changed since it was loaded (expected content_version %d, found %d)",
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

// SaveResult reports the outcome of a successful Save.
type SaveResult struct {
	DocumentID     int64
	ContentVersion int64
}

// CreateResult reports the outcome of a successful Create.
type CreateResult struct {
	DocumentID     int64
	InputRecordID  int64
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
func (s *Store) Save(ctx context.Context, doc *model.Document, expectedVersion int64) (*SaveResult, error) {
	if err := model.Validate(doc); err != nil {
		return nil, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("cdm: begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op if committed

	// Read the pre-write state under a row lock, then decide, then write.
	// Both checks below have to happen inside this transaction and behind
	// that lock: a caller-side load-compare-save would let two writers pass
	// the same check and lose one edit (design D3).
	st, err := lockDocStateTx(ctx, tx, doc.Key)
	if err != nil {
		return nil, err
	}
	switch {
	case st.exists && st.published:
		return nil, &FrozenError{DocumentKey: doc.Key}
	case st.exists && st.version != expectedVersion:
		return nil, &StaleVersionError{DocumentKey: doc.Key, Expected: expectedVersion, Actual: st.version}
	case !st.exists && expectedVersion != 0:
		// The caller believes it is updating a document that is not there.
		return nil, &StaleVersionError{DocumentKey: doc.Key, Expected: expectedVersion, Actual: 0}
	}

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

	if err := writeContentTx(ctx, tx, id, contentVersion, doc); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("cdm: commit: %w", err)
	}

	doc.ContentVersion = contentVersion
	return &SaveResult{DocumentID: id, ContentVersion: contentVersion}, nil
}

// Create writes a new CDM document and the kb.inputs row backing it in a
// single transaction, linking them via kb.cdm_documents.input_record_id.
//
// The link is what lets everything downstream find a document's input row
// from its document_key alone: tenant scoping, the publish transition, and
// the frozen check in Save all traverse it. Writing both rows here — the one
// moment both are being created anyway — is the only place they cannot
// diverge (design D2).
//
// Creating a document whose document_key already exists is an error; use Save
// to update an existing document.
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

	const initialVersion = 1
	var id int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO kb.cdm_documents
			(document_key, title, language, schema_version, content_version,
			 doc_type, rendering_type, authors, doc_version, input_record_id,
			 semantic_document, update_time)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, '{}'::jsonb, NOW())
		RETURNING id
	`,
		doc.Key, doc.Title, nullableString(doc.Language), doc.SchemaVersion, initialVersion,
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

	if err := writeContentTx(ctx, tx, id, initialVersion, doc); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("cdm: commit: %w", err)
	}

	doc.ContentVersion = initialVersion
	return &CreateResult{
		DocumentID:     id,
		InputRecordID:  inputID,
		ContentVersion: initialVersion,
	}, nil
}

// writeContentTx stores the canonical JSON and rebuilds kb.cdm_blocks for a
// document row that already exists in this transaction. Shared by Create and
// Save so the two cannot drift in how a document's content is persisted.
func writeContentTx(ctx context.Context, tx *sql.Tx, id, contentVersion int64, doc *model.Document) error {
	// Marshal a stamped copy rather than mutating the caller's doc here: the
	// transaction can still fail below (a block slug conflict, for example),
	// and the caller's Document should not appear to have a new
	// content_version unless the write actually commits.
	stamped := *doc
	stamped.ContentVersion = contentVersion
	docJSON, err := json.Marshal(&stamped)
	if err != nil {
		return fmt.Errorf("cdm: marshal document: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE kb.cdm_documents SET semantic_document = $1 WHERE id = $2
	`, docJSON, id); err != nil {
		return fmt.Errorf("cdm: store semantic_document: %w", err)
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

