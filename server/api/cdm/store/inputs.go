package store

import (
	"context"
	"database/sql"
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
	tenantID := in.TenantID
	if tenantID == "" {
		tenantID = "-"
	}

	var id int64
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO kb.inputs (tenant_id, ks_store_id, type, title, status)
		VALUES ($1, $2, $3, $4, $5::jsonb)
		RETURNING id
	`, tenantID, in.KSStoreID, cdmInputType, in.Title, draftStatus).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("cdm: create draft input row: %w", err)
	}
	return id, nil
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
