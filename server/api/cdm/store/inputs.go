package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// cdmInputType marks a kb.inputs row as SemOS-authored rather than uploaded
// (spec §10.1, ADR 2026072501 DR13).
const cdmInputType = "cdm"

// draftStatus is the status JSONB written when a CDM document is created.
// Both the "parsed" and "doc_processing" entries are terminal ("success"),
// so kb.input_status_parse_state derives 'parsed_success' (there is no file
// to parse; the CDM AST is the parse result) and
// kb.input_status_pipeline_state derives 'success' — keeping the row off
// BOTH the parse worklist (handler.go:679) and the doc-processing worklist
// (handler.go:748) while the document is a draft. This is deliberate: a
// draft is processed only when an author explicitly triggers it (CDM Editor
// scope, not built in this change), never by a worklist poller.
const draftStatus = `[{"operation":"parsed","proc_status":"success"},{"operation":"doc_processing","proc_status":"success"}]`

// publishedStatus is the status JSONB written at publish. It clears the
// doc_processing entry (rather than creating a new row), so
// pipeline_state derives back to 'pending' and the standard doc-processing
// worklist enqueues the document for its authoritative run, on the same
// terms as an uploaded document.
const publishedStatus = `[{"operation":"parsed","proc_status":"success"}]`

// DraftInput carries the fields needed to register a new CDM document's
// kb.inputs row.
type DraftInput struct {
	TenantID  string
	KSStoreID sql.NullInt64
	Title     string
}

// InputRegistrar manages the kb.inputs row backing a CDM document's
// lifecycle (spec §10.1): editing -> published -> rendered ->
// line_file_generated -> doc-process pipeline.
type InputRegistrar struct {
	db *sql.DB
}

// NewInputRegistrar returns an InputRegistrar backed by db.
func NewInputRegistrar(db *sql.DB) *InputRegistrar {
	return &InputRegistrar{db: db}
}

// CreateDraft inserts a kb.inputs row for a new CDM document in the
// `editing` state. The row exists from creation (not deferred to publish) so
// that an author-triggered doc-processing run has an input row to attach
// artifacts to before publication.
func (r *InputRegistrar) CreateDraft(ctx context.Context, in DraftInput) (int64, error) {
	return createDraftTx(ctx, r.db, in)
}

// execQuerier is the subset of *sql.DB and *sql.Tx that createDraftTx needs,
// so the same insert serves both the standalone CreateDraft above and
// Store.Create, which must write this row inside its own transaction to keep
// the kb.inputs row and the kb.cdm_documents row atomic (design D2).
type execQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func createDraftTx(ctx context.Context, q execQuerier, in DraftInput) (int64, error) {
	tenantID := in.TenantID
	if tenantID == "" {
		tenantID = "-"
	}

	var id int64
	err := q.QueryRowContext(ctx, `
		INSERT INTO kb.inputs (tenant_id, ks_store_id, type, title, status)
		VALUES ($1, $2, $3, $4, $5::jsonb)
		RETURNING id
	`, tenantID, in.KSStoreID, cdmInputType, in.Title, draftStatus).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("cdm: create draft input row: %w", err)
	}
	return id, nil
}

// docState is the pre-write state of a document, read under a row lock so
// that the version check and the version increment cannot interleave with a
// concurrent writer (design D3).
type docState struct {
	exists         bool
	contentVersion int64
	editVersion    int64
	published      bool
}

// lockDocStateTx reads a document's current version and publication state,
// taking a row lock on kb.cdm_documents for the duration of the caller's
// transaction.
//
// The lock is what makes optimistic concurrency correct. Without FOR UPDATE,
// two transactions could both read version 7, both find it matches what their
// client expected, and both increment — producing versions 8 and 9 and
// silently losing one client's edit. With it, the second transaction blocks
// until the first commits, then re-reads version 8 and correctly reports the
// caller stale.
//
// Publication is derived from the absence of the doc_processing status entry
// (see publishedStatus), which is the same signal that makes pipeline_state
// derive back to 'pending' for the worklist — so this asks the question the
// worklist already asks rather than introducing a second source of truth for
// "is this document published" (design D4). A document whose input_record_id
// is NULL (any document written before Store.Create existed) reads as a
// draft, which is the safe default: it stays editable.
func lockDocStateTx(ctx context.Context, q execQuerier, documentKey string) (docState, error) {
	var st docState
	err := q.QueryRowContext(ctx, `
		SELECT d.content_version,
		       d.edit_version,
		       COALESCE(NOT (i.status @> '[{"operation":"doc_processing"}]'::jsonb), false)
		FROM kb.cdm_documents d
		LEFT JOIN kb.inputs i ON i.id = d.input_record_id
		WHERE d.document_key = $1
		FOR UPDATE OF d
	`, documentKey).Scan(&st.contentVersion, &st.editVersion, &st.published)
	if errors.Is(err, sql.ErrNoRows) {
		return docState{exists: false}, nil
	}
	if err != nil {
		return docState{}, fmt.Errorf("cdm: read state for %q: %w", documentKey, err)
	}
	st.exists = true
	return st, nil
}

// Publish transitions a CDM document's kb.inputs row from `editing` to
// `published`: it clears the doc_processing status entry so pipeline_state
// derives back to 'pending', handing the document to the standard
// doc-processing worklist. This is a status transition on the existing row,
// never a new row.
func (r *InputRegistrar) Publish(ctx context.Context, inputRecordID int64) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE kb.inputs
		SET status = $1::jsonb, modify_time = NOW()
		WHERE id = $2 AND type = $3
	`, publishedStatus, inputRecordID, cdmInputType)
	if err != nil {
		return fmt.Errorf("cdm: publish input row %d: %w", inputRecordID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("cdm: publish input row %d: %w", inputRecordID, err)
	}
	if n == 0 {
		return fmt.Errorf("cdm: publish input row %d: no matching cdm input row", inputRecordID)
	}
	return nil
}

// InputState reports the derived worklist state of a kb.inputs row, for
// tests and diagnostics.
type InputState struct {
	ParseState    string
	PipelineState string
}

// LoadInputState reads back the derived parse_state/pipeline_state columns
// for a kb.inputs row.
func (r *InputRegistrar) LoadInputState(ctx context.Context, inputRecordID int64) (*InputState, error) {
	var st InputState
	err := r.db.QueryRowContext(ctx, `
		SELECT parse_state, pipeline_state FROM kb.inputs WHERE id = $1
	`, inputRecordID).Scan(&st.ParseState, &st.PipelineState)
	if err != nil {
		return nil, fmt.Errorf("cdm: load input state %d: %w", inputRecordID, err)
	}
	return &st, nil
}
