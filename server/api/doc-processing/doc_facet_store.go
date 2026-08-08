package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
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

type FacetObservationStore interface {
	InsertFacetObservation(ctx context.Context, observation FacetObservation) (FacetObservation, error)
	ListFacetObservations(ctx context.Context, recordID, vocabularyReleaseID int64) ([]FacetObservation, error)
	// ListFacetObservationsAnyRelease returns every observation for a
	// record regardless of vocabulary_release_id. ListFacetObservations'
	// exact-release match exists for classify_document's own retry-dedup
	// (spec section 11: pin one invocation to one release); tiers 1-2 are
	// deterministic/metadata-derived, not release-scoped LLM output, so
	// enriching BaseFacts (S16.1 "Facet tiers 1-2") needs everything known
	// about the record, not one release's slice of it.
	ListFacetObservationsAnyRelease(ctx context.Context, recordID int64) ([]FacetObservation, error)
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

func (s SQLStore) InsertFacetObservation(ctx context.Context, observation FacetObservation) (FacetObservation, error) {
	if s.DB == nil {
		return FacetObservation{}, errors.New("db is nil")
	}
	if observation.RecordID <= 0 {
		return FacetObservation{}, errors.New("record_id is required")
	}
	if observation.Path == "" {
		return FacetObservation{}, errors.New("path is required")
	}
	if observation.DecisionAttemptID == "" {
		return FacetObservation{}, errors.New("decision_attempt_id is required")
	}
	if observation.InvocationID == "" {
		return FacetObservation{}, errors.New("invocation_id is required")
	}
	if observation.State == "" {
		observation.State = "known"
	}
	valueJSON, err := json.Marshal(observation.Value)
	if err != nil {
		return FacetObservation{}, err
	}
	evidenceJSON, err := json.Marshal(observation.Evidence)
	if err != nil {
		return FacetObservation{}, err
	}
	if len(evidenceJSON) == 0 || string(evidenceJSON) == "null" {
		evidenceJSON = []byte(`{}`)
	}

	const stmt = `
WITH inserted AS (
    INSERT INTO kb.doc_facet_values (
        record_id, path, value, state, method, confidence, evidence, source_fingerprint,
        decision_attempt_id, invocation_id, vocabulary_release_id
    )
    VALUES ($1, $2, $3::jsonb, $4, $5, $6, $7::jsonb, $8, $9, $10, $11)
    ON CONFLICT (record_id, path, decision_attempt_id, invocation_id) DO NOTHING
    RETURNING id, record_id, path, value, state, method, confidence, evidence, source_fingerprint,
              decision_attempt_id, invocation_id, vocabulary_release_id
)
SELECT id, record_id, path, value, state, method, confidence, evidence, source_fingerprint,
       decision_attempt_id, invocation_id, vocabulary_release_id
FROM inserted
UNION ALL
SELECT id, record_id, path, value, state, method, confidence, evidence, source_fingerprint,
       decision_attempt_id, invocation_id, vocabulary_release_id
FROM kb.doc_facet_values
WHERE record_id = $1 AND path = $2 AND decision_attempt_id = $9 AND invocation_id = $10
  AND NOT EXISTS (SELECT 1 FROM inserted)`
	return scanFacetObservation(s.DB.QueryRowContext(
		ctx,
		stmt,
		observation.RecordID,
		observation.Path,
		string(valueJSON),
		string(observation.State),
		observation.Method,
		observation.Confidence,
		string(evidenceJSON),
		observation.SourceFingerprint,
		observation.DecisionAttemptID,
		observation.InvocationID,
		observation.VocabularyReleaseID,
	))
}

func (s SQLStore) ListFacetObservations(ctx context.Context, recordID, vocabularyReleaseID int64) ([]FacetObservation, error) {
	if s.DB == nil {
		return nil, errors.New("db is nil")
	}
	if recordID <= 0 {
		return nil, errors.New("record_id is required")
	}
	const stmt = `
SELECT id, record_id, path, value, state, method, confidence, evidence, source_fingerprint,
       decision_attempt_id, invocation_id, vocabulary_release_id
FROM kb.doc_facet_values
WHERE record_id = $1 AND vocabulary_release_id = $2
ORDER BY path, method, decision_attempt_id, invocation_id, id`
	rows, err := s.DB.QueryContext(ctx, stmt, recordID, vocabularyReleaseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var observations []FacetObservation
	for rows.Next() {
		observation, err := scanFacetObservation(rows)
		if err != nil {
			return nil, err
		}
		observations = append(observations, observation)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return observations, nil
}

func (s SQLStore) ListFacetObservationsAnyRelease(ctx context.Context, recordID int64) ([]FacetObservation, error) {
	if s.DB == nil {
		return nil, errors.New("db is nil")
	}
	if recordID <= 0 {
		return nil, errors.New("record_id is required")
	}
	const stmt = `
SELECT id, record_id, path, value, state, method, confidence, evidence, source_fingerprint,
       decision_attempt_id, invocation_id, vocabulary_release_id
FROM kb.doc_facet_values
WHERE record_id = $1
ORDER BY path, method, decision_attempt_id, invocation_id, id`
	rows, err := s.DB.QueryContext(ctx, stmt, recordID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var observations []FacetObservation
	for rows.Next() {
		observation, err := scanFacetObservation(rows)
		if err != nil {
			return nil, err
		}
		observations = append(observations, observation)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return observations, nil
}

type facetObservationScanner interface {
	Scan(dest ...any) error
}

func scanFacetObservation(scanner facetObservationScanner) (FacetObservation, error) {
	var observation FacetObservation
	var valueRaw, evidenceRaw []byte
	var confidence sql.NullFloat64
	err := scanner.Scan(
		&observation.ID,
		&observation.RecordID,
		&observation.Path,
		&valueRaw,
		&observation.State,
		&observation.Method,
		&confidence,
		&evidenceRaw,
		&observation.SourceFingerprint,
		&observation.DecisionAttemptID,
		&observation.InvocationID,
		&observation.VocabularyReleaseID,
	)
	if err != nil {
		return FacetObservation{}, err
	}
	if len(valueRaw) > 0 {
		if err := json.Unmarshal(valueRaw, &observation.Value); err != nil {
			return FacetObservation{}, err
		}
	}
	if len(evidenceRaw) > 0 {
		if err := json.Unmarshal(evidenceRaw, &observation.Evidence); err != nil {
			return FacetObservation{}, err
		}
	}
	if confidence.Valid {
		value := confidence.Float64
		observation.Confidence = &value
	}
	return observation, nil
}
