package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/chendingplano/deepdoc/server/api/cdm/rendering"
)

// RendererVersion identifies the renderer implementation, recorded on every
// rendering and anchor row so a stale artifact is detectable after an
// upgrade (spec §5.7, design D11 risk: "Typst layout changes shift
// coordinates").
const RendererVersion = "cdm-typst-renderer-v1"

// Publisher renders a saved CDM document, extracts its anchors, generates
// its line file, persists the rendered artifacts, and transitions its
// kb.inputs row from `editing` to `published` (spec §10.1). It is the single
// place spanning cdm/model, cdm/rendering, and cdm/store, matching design D4
// (rendering itself stays database-free).
type Publisher struct {
	db       *sql.DB
	docs     *Store
	inputs   *InputRegistrar
	typstBin string
	themeSrc []byte
}

// NewPublisher returns a Publisher backed by db. themeSrc is the Typst theme
// file content to make available alongside the rendered document (spec
// §5.3/§5.4); typstBin is the typst executable to invoke, defaulting to
// "typst" on PATH when empty.
func NewPublisher(db *sql.DB, themeSrc []byte, typstBin string) *Publisher {
	if typstBin == "" {
		typstBin = "typst"
	}
	return &Publisher{
		db: db, docs: New(db), inputs: NewInputRegistrar(db),
		typstBin: typstBin, themeSrc: themeSrc,
	}
}

// PublishResult summarizes what a successful Publish produced.
type PublishResult struct {
	ContentVersion int64
	PageCount      int
	LineCount      int
}

// Publish renders the document identified by documentKey, whose kb.inputs
// row is inputRecordID, and carries it through rendered -> line_file_generated
// -> published (spec §10.1): Typst render with anchor marks, SVG pages,
// anchor extraction and fragment derivation, line-file generation, and
// finally clearing kb.inputs' doc_processing status so the standard
// doc-processing worklist picks the document up.
func (p *Publisher) Publish(ctx context.Context, documentKey string, inputRecordID int64) (*PublishResult, error) {
	res, err := p.Render(ctx, documentKey)
	if err != nil {
		return nil, err
	}

	if err := p.inputs.Publish(ctx, inputRecordID); err != nil {
		return nil, err
	}

	return &PublishResult{
		ContentVersion: res.ContentVersion,
		PageCount:      res.PageCount,
		LineCount:      res.LineCount,
	}, nil
}

// RenderResult summarizes what a successful Render produced.
type RenderResult struct {
	ContentVersion int64
	PageCount      int
	LineCount      int
}

// Render produces and stores a document's rendered artifacts — Typst source,
// SVG pages, line file, and anchor map — for its current content_version,
// without touching the editorial lifecycle.
//
// Split out of Publish so the editor can preview a draft (ADR 2026072603 DR4)
// without publishing it. Calling Publish for a preview would freeze the
// document (D8), which is the opposite of what an author previewing their
// work-in-progress wants. Publish is now exactly Render plus the kb.inputs
// transition, so the published artifacts are produced by the same code path
// the preview used and cannot drift from it.
//
// Artifacts are keyed by content_version, so re-rendering an unchanged
// version overwrites identical rows and a later version's artifacts sit
// alongside rather than replacing the earlier ones.
func (p *Publisher) Render(ctx context.Context, documentKey string) (*RenderResult, error) {
	doc, err := p.docs.Load(ctx, documentKey)
	if err != nil {
		return nil, err
	}

	var docRow int64
	if err := p.db.QueryRowContext(ctx, `SELECT id FROM kb.cdm_documents WHERE document_key = $1`, documentKey).Scan(&docRow); err != nil {
		return nil, fmt.Errorf("cdm: resolve document row for %q: %w", documentKey, err)
	}

	dir, err := os.MkdirTemp("", "cdm-publish-*")
	if err != nil {
		return nil, fmt.Errorf("cdm: create publish workdir: %w", err)
	}
	defer os.RemoveAll(dir)

	if err := os.WriteFile(filepath.Join(dir, "theme.typ"), p.themeSrc, 0o644); err != nil {
		return nil, fmt.Errorf("cdm: write theme: %w", err)
	}

	r := &rendering.TypstRenderer{}
	typstSrc, err := r.RenderDocument(doc)
	if err != nil {
		return nil, fmt.Errorf("cdm: render document: %w", err)
	}
	typPath := filepath.Join(dir, "doc.typ")
	if err := os.WriteFile(typPath, typstSrc, 0o644); err != nil {
		return nil, fmt.Errorf("cdm: write rendered source: %w", err)
	}

	marks, err := rendering.ExtractAnchors(p.typstBin, typPath)
	if err != nil {
		return nil, fmt.Errorf("cdm: extract anchors: %w", err)
	}
	frags, err := rendering.DeriveFragments(marks)
	if err != nil {
		return nil, fmt.Errorf("cdm: derive fragments: %w", err)
	}
	fragsByUnit := map[string][]rendering.Fragment{}
	for _, f := range frags {
		fragsByUnit[f.UnitID] = append(fragsByUnit[f.UnitID], f)
	}

	units := rendering.CollectUnits(doc)
	lineFile, lineUnitIDs, err := rendering.GenerateLineFile(units, fragsByUnit)
	if err != nil {
		return nil, fmt.Errorf("cdm: generate line file: %w", err)
	}

	pages, err := rendering.RenderSVGPages(p.typstBin, typPath)
	if err != nil {
		return nil, fmt.Errorf("cdm: render svg pages: %w", err)
	}

	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("cdm: begin publish transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op if committed

	if err := storeRendering(ctx, tx, docRow, doc.ContentVersion, "typst", "text/x-typst", 0, typstSrc); err != nil {
		return nil, err
	}
	if err := storeRendering(ctx, tx, docRow, doc.ContentVersion, "line-file", "text/plain", 0, []byte(lineFile)); err != nil {
		return nil, err
	}
	for i, page := range pages {
		if err := storeRendering(ctx, tx, docRow, doc.ContentVersion, "svg", "image/svg+xml", i+1, page); err != nil {
			return nil, err
		}
	}

	if err := storeAnchors(ctx, tx, docRow, doc.ContentVersion, lineUnitIDs, fragsByUnit); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("cdm: commit render transaction: %w", err)
	}

	return &RenderResult{
		ContentVersion: doc.ContentVersion,
		PageCount:      len(pages),
		LineCount:      len(lineUnitIDs),
	}, nil
}

func storeRendering(ctx context.Context, tx *sql.Tx, documentID int64, contentVersion int64, renderer, mediaType string, page int, content []byte) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO kb.cdm_renderings
			(document_id, content_version, renderer, renderer_version, media_type, page, rendered_content)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (document_id, content_version, renderer, renderer_version, page)
		DO UPDATE SET rendered_content = EXCLUDED.rendered_content
	`, documentID, contentVersion, renderer, RendererVersion, mediaType, page, content)
	if err != nil {
		return fmt.Errorf("cdm: store %s rendering (page %d): %w", renderer, page, err)
	}
	return nil
}

func storeAnchors(ctx context.Context, tx *sql.Tx, documentID int64, contentVersion int64, lineUnitIDs []string, fragsByUnit map[string][]rendering.Fragment) error {
	for lineIdx, unitID := range lineUnitIDs {
		lineNumber := lineIdx + 1
		for ordinal, f := range fragsByUnit[unitID] {
			_, err := tx.ExecContext(ctx, `
				INSERT INTO kb.cdm_anchors
					(document_id, content_version, renderer_version, line_number,
					 block_id, fragment_ordinal, page, x, y, w, h)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
				ON CONFLICT (document_id, content_version, renderer_version, line_number, fragment_ordinal)
				DO UPDATE SET page = EXCLUDED.page, x = EXCLUDED.x, y = EXCLUDED.y, w = EXCLUDED.w, h = EXCLUDED.h
			`, documentID, contentVersion, RendererVersion, lineNumber,
				unitBlockID(unitID), ordinal, f.Page, f.X, f.Y, f.W, f.H)
			if err != nil {
				return fmt.Errorf("cdm: store anchor for unit %q line %d: %w", unitID, lineNumber, err)
			}
		}
	}
	return nil
}

// unitBlockID strips a table-row unit id ("table1/row0") down to its owning
// block id ("table1") for the anchor row's informational block_id column;
// line_number remains the authoritative lookup key.
func unitBlockID(unitID string) string {
	for i := 0; i < len(unitID); i++ {
		if unitID[i] == '/' {
			return unitID[:i]
		}
	}
	return unitID
}

// ResolveHighlight resolves an artifact's source_line_spans to highlight
// fragments for a CDM document, returning the same {page, bbox} shape the
// PDF-based path returns (spec §5.7, "Navigate and highlight parity").
func (p *Publisher) ResolveHighlight(ctx context.Context, documentKey string, contentVersion int64, lineNumbers []int) ([]rendering.Fragment, error) {
	var docRow int64
	if err := p.db.QueryRowContext(ctx, `SELECT id FROM kb.cdm_documents WHERE document_key = $1`, documentKey).Scan(&docRow); err != nil {
		return nil, fmt.Errorf("cdm: resolve document row for %q: %w", documentKey, err)
	}

	var frags []rendering.Fragment
	for _, ln := range lineNumbers {
		rows, err := p.db.QueryContext(ctx, `
			SELECT block_id, page, x, y, w, h
			FROM kb.cdm_anchors
			WHERE document_id = $1 AND content_version = $2 AND line_number = $3
			ORDER BY fragment_ordinal
		`, docRow, contentVersion, ln)
		if err != nil {
			return nil, fmt.Errorf("cdm: resolve highlight for line %d: %w", ln, err)
		}
		for rows.Next() {
			var f rendering.Fragment
			if err := rows.Scan(&f.UnitID, &f.Page, &f.X, &f.Y, &f.W, &f.H); err != nil {
				rows.Close()
				return nil, fmt.Errorf("cdm: scan anchor row: %w", err)
			}
			frags = append(frags, f)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	return frags, nil
}
