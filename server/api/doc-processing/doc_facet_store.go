package docprocessing

import (
	"context"
	"database/sql"
	"errors"
)

// DocFacetRecord is the persisted form of ProductionRoutingFacets, keyed by
// record so it can be queried directly (e.g. "which documents are bound to
// store X") without joining through plan-run history.
type DocFacetRecord struct {
	RecordID              int64
	KSStoreID             int64
	KnowledgeStoreBinding string
	InputDocType          string
	SourceLanguage        string
	HasDocumentNumber     bool
}

type DocFacetStore interface {
	UpsertDocFacets(ctx context.Context, rec DocFacetRecord) error
	GetDocFacets(ctx context.Context, recordID int64) (DocFacetRecord, error)
}

func (s SQLStore) UpsertDocFacets(ctx context.Context, rec DocFacetRecord) error {
	if s.DB == nil {
		return errors.New("db is nil")
	}
	if rec.RecordID <= 0 {
		return errors.New("record_id is required")
	}

	const stmt = `
INSERT INTO kb.doc_facets (record_id, ks_store_id, knowledge_store_binding, input_doc_type, source_language, has_document_number)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (record_id) DO UPDATE SET
    ks_store_id = EXCLUDED.ks_store_id,
    knowledge_store_binding = EXCLUDED.knowledge_store_binding,
    input_doc_type = EXCLUDED.input_doc_type,
    source_language = EXCLUDED.source_language,
    has_document_number = EXCLUDED.has_document_number,
    modify_time = NOW()`

	_, err := s.DB.ExecContext(ctx, stmt, rec.RecordID, rec.KSStoreID, rec.KnowledgeStoreBinding, rec.InputDocType, rec.SourceLanguage, rec.HasDocumentNumber)
	return err
}

func (s SQLStore) GetDocFacets(ctx context.Context, recordID int64) (DocFacetRecord, error) {
	if s.DB == nil {
		return DocFacetRecord{}, errors.New("db is nil")
	}
	if recordID <= 0 {
		return DocFacetRecord{}, errors.New("record_id is required")
	}

	const stmt = `
SELECT record_id, ks_store_id, knowledge_store_binding, input_doc_type, source_language, has_document_number
FROM kb.doc_facets
WHERE record_id = $1`

	var rec DocFacetRecord
	err := s.DB.QueryRowContext(ctx, stmt, recordID).Scan(
		&rec.RecordID, &rec.KSStoreID, &rec.KnowledgeStoreBinding, &rec.InputDocType, &rec.SourceLanguage, &rec.HasDocumentNumber,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DocFacetRecord{}, sql.ErrNoRows
		}
		return DocFacetRecord{}, err
	}
	return rec, nil
}
