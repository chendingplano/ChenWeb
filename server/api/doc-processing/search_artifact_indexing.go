package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/kbsearch"
	"github.com/chendingplano/shared/go/api/ApiTypes"
)

var summaryIndexConfig = artifactIndexConfig{
	SelfType:             searchArtifactSummary,
	CategoryType:         "summary",
	InstanceSource:       "generate_summaries",
	Table:                "kb.summaries",
	IDColumn:             "summary_id",
	CategoryTreeFilename: "summaries.txt",
	LogPrefix:            "summary indexing",
}

var topicIndexConfig = artifactIndexConfig{
	SelfType:             searchArtifactTopic,
	CategoryType:         "topic",
	InstanceSource:       "generate_topics",
	Table:                "kb.topics",
	IDColumn:             "topic_id",
	CategoryTreeFilename: "topics.txt",
	LogPrefix:            "topic indexing",
}

var provisionIndexConfig = artifactIndexConfig{
	SelfType:             searchArtifactProvision,
	CategoryType:         "provision",
	InstanceSource:       "extract_provisions",
	Table:                "kb.provisions",
	IDColumn:             "prov_id",
	CategoryTreeFilename: "provisions.txt",
	LogPrefix:            "provision indexing",
}

var entityIndexConfig = artifactIndexConfig{
	SelfType:             searchArtifactEntity,
	CategoryType:         "entity",
	InstanceSource:       "extract_entity_relation",
	Table:                "kb.entities",
	IDColumn:             "entity_id",
	CategoryTreeFilename: "entities.txt",
	LogPrefix:            "entity indexing",
}

var relationIndexConfig = artifactIndexConfig{
	SelfType:             searchArtifactRelation,
	CategoryType:         "relation",
	InstanceSource:       "extract_entity_relation",
	Table:                "kb.relations",
	IDColumn:             "relation_id",
	CategoryTreeFilename: "relations.txt",
	LogPrefix:            "relation indexing",
}

func IndexSummariesForRecord(ctx context.Context, recordID int64, inputChunks []Block, logger ApiTypes.JimoLogger) {
	indexArtifactsByOverlapAndConnect(ctx, recordID, inputChunks, logger, summaryIndexConfig, loadIndexedSummariesForRecord)
}

func IndexTopicsForRecord(ctx context.Context, recordID int64, inputChunks []Block, logger ApiTypes.JimoLogger) {
	indexArtifactsByOverlapAndConnect(ctx, recordID, inputChunks, logger, topicIndexConfig, loadIndexedTopicsForRecord)
}

func IndexProvisionsForRecord(ctx context.Context, recordID int64, inputChunks []Block, logger ApiTypes.JimoLogger) {
	indexArtifactsByOverlapAndConnect(ctx, recordID, inputChunks, logger, provisionIndexConfig, loadIndexedProvisionsForRecord)
}

func IndexEntitiesForRecord(ctx context.Context, recordID int64, inputChunks []Block, logger ApiTypes.JimoLogger) {
	indexArtifactsByOverlapAndConnect(ctx, recordID, inputChunks, logger, entityIndexConfig, loadIndexedEntitiesForRecord)
}

func IndexRelationsForRecord(ctx context.Context, recordID int64, inputChunks []Block, logger ApiTypes.JimoLogger) {
	indexArtifactsByOverlapAndConnect(ctx, recordID, inputChunks, logger, relationIndexConfig, loadIndexedRelationsForRecord)
}

type indexedArtifactLoader func(context.Context, *sql.DB, int64) ([]indexedArtifact, error)

func indexArtifactsByOverlapAndConnect(ctx context.Context, recordID int64, inputChunks []Block, logger ApiTypes.JimoLogger, cfg artifactIndexConfig, loader indexedArtifactLoader) {
	db := ApiTypes.ProjectDBHandle
	if db == nil {
		if logger != nil {
			logger.Warn(cfg.LogPrefix+" skipped: nil project db handle", "record_id", recordID)
		}
		return
	}
	artifacts, err := loader(ctx, db, recordID)
	if err != nil {
		if logger != nil {
			logger.Warn(cfg.LogPrefix+": load artifacts failed", "record_id", recordID, "error", err.Error())
		}
		return
	}
	if len(artifacts) == 0 {
		return
	}
	connectedCount := buildArtifactConnectedArtifacts(ctx, db, recordID, inputChunks, artifacts, cfg, logger)
	semanticLinks := connectArtifactsBySearch(ctx, db, recordID, artifacts, cfg, logger)
	if logger != nil {
		logger.Info(cfg.LogPrefix+" result",
			"record_id", recordID,
			"artifacts", len(artifacts),
			"connected_artifacts", connectedCount,
			"semantic_links", semanticLinks,
		)
	}
}

func loadIndexedSummariesForRecord(ctx context.Context, db *sql.DB, recordID int64) ([]indexedArtifact, error) {
	rows, err := db.QueryContext(ctx, `SELECT summary_id, summary_seq_no, COALESCE(source_line_spans, '[]'::jsonb), COALESCE(search_document, '') FROM kb.summaries WHERE input_record_id = $1 ORDER BY id`, recordID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []indexedArtifact
	for rows.Next() {
		var (
			summaryID string
			seq       int
			spansRaw  []byte
			searchDoc string
		)
		if err := rows.Scan(&summaryID, &seq, &spansRaw, &searchDoc); err != nil {
			return nil, err
		}
		var spansAny any
		if len(spansRaw) > 0 {
			_ = json.Unmarshal(spansRaw, &spansAny)
		}
		out = append(out, indexedArtifact{
			ID:             kbsearch.BuildArtifactID(recordID, searchArtifactSummary, strconv.Itoa(seq)),
			UpdateKey:      strings.TrimSpace(summaryID),
			SourceSpans:    normalizeSourceLineSpans(spansAny),
			SearchDocument: strings.TrimSpace(searchDoc),
		})
	}
	return out, rows.Err()
}

func loadIndexedTopicsForRecord(ctx context.Context, db *sql.DB, recordID int64) ([]indexedArtifact, error) {
	rows, err := db.QueryContext(ctx, `SELECT topic_id, COALESCE(source_line_spans, '[]'::jsonb), COALESCE(search_document, '') FROM kb.topics WHERE input_record_id = $1 ORDER BY id`, recordID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []indexedArtifact
	for rows.Next() {
		var (
			topicID   string
			spansRaw  []byte
			searchDoc string
		)
		if err := rows.Scan(&topicID, &spansRaw, &searchDoc); err != nil {
			return nil, err
		}
		var spansAny any
		if len(spansRaw) > 0 {
			_ = json.Unmarshal(spansRaw, &spansAny)
		}
		out = append(out, indexedArtifact{
			ID:             kbsearch.BuildArtifactID(recordID, searchArtifactTopic, strings.TrimSpace(topicID)),
			UpdateKey:      strings.TrimSpace(topicID),
			SourceSpans:    normalizeSourceLineSpans(spansAny),
			SearchDocument: strings.TrimSpace(searchDoc),
		})
	}
	return out, rows.Err()
}

func loadIndexedProvisionsForRecord(ctx context.Context, db *sql.DB, recordID int64) ([]indexedArtifact, error) {
	rows, err := db.QueryContext(ctx, `SELECT prov_id, id, COALESCE(source_line_spans, '[]'::jsonb), COALESCE(search_document, '') FROM kb.provisions WHERE input_record_id = $1 ORDER BY id`, recordID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []indexedArtifact
	for rows.Next() {
		var (
			provID    string
			rowID     int64
			spansRaw  []byte
			searchDoc string
		)
		if err := rows.Scan(&provID, &rowID, &spansRaw, &searchDoc); err != nil {
			return nil, err
		}
		artifactID := strings.TrimSpace(provID)
		if artifactID == "" {
			artifactID = strconv.FormatInt(rowID, 10)
		}
		var spansAny any
		if len(spansRaw) > 0 {
			_ = json.Unmarshal(spansRaw, &spansAny)
		}
		out = append(out, indexedArtifact{
			ID:             kbsearch.BuildArtifactID(recordID, searchArtifactProvision, artifactID),
			UpdateKey:      rowIDToProvisionKey(provID, rowID),
			SourceSpans:    normalizeSourceLineSpans(spansAny),
			SearchDocument: strings.TrimSpace(searchDoc),
		})
	}
	return out, rows.Err()
}

func loadIndexedEntitiesForRecord(ctx context.Context, db *sql.DB, recordID int64) ([]indexedArtifact, error) {
	rows, err := db.QueryContext(ctx, `SELECT entity_id, id, COALESCE(line_spans, '[]'::jsonb), COALESCE(search_document, '') FROM kb.entities WHERE input_record_id = $1 ORDER BY id`, recordID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []indexedArtifact
	for rows.Next() {
		var (
			entityID  string
			rowID     int64
			spansRaw  []byte
			searchDoc string
		)
		if err := rows.Scan(&entityID, &rowID, &spansRaw, &searchDoc); err != nil {
			return nil, err
		}
		seq := lastDelimitedToken(entityID)
		if seq == "" {
			seq = strconv.FormatInt(rowID, 10)
		}
		var spansAny any
		if len(spansRaw) > 0 {
			_ = json.Unmarshal(spansRaw, &spansAny)
		}
		out = append(out, indexedArtifact{
			ID:             kbsearch.BuildArtifactID(recordID, searchArtifactEntity, seq),
			UpdateKey:      strings.TrimSpace(entityID),
			SourceSpans:    normalizeSourceLineSpans(spansAny),
			SearchDocument: strings.TrimSpace(searchDoc),
		})
	}
	return out, rows.Err()
}

func loadIndexedRelationsForRecord(ctx context.Context, db *sql.DB, recordID int64) ([]indexedArtifact, error) {
	rows, err := db.QueryContext(ctx, `SELECT relation_id, id, COALESCE(line_spans, '[]'::jsonb), COALESCE(search_document, '') FROM kb.relations WHERE input_record_id = $1 ORDER BY id`, recordID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []indexedArtifact
	for rows.Next() {
		var (
			relationID string
			rowID      int64
			spansRaw   []byte
			searchDoc  string
		)
		if err := rows.Scan(&relationID, &rowID, &spansRaw, &searchDoc); err != nil {
			return nil, err
		}
		seq := lastDelimitedToken(relationID)
		if seq == "" {
			seq = strconv.FormatInt(rowID, 10)
		}
		var spansAny any
		if len(spansRaw) > 0 {
			_ = json.Unmarshal(spansRaw, &spansAny)
		}
		out = append(out, indexedArtifact{
			ID:             kbsearch.BuildArtifactID(recordID, searchArtifactRelation, seq),
			UpdateKey:      strings.TrimSpace(relationID),
			SourceSpans:    normalizeSourceLineSpans(spansAny),
			SearchDocument: strings.TrimSpace(searchDoc),
		})
	}
	return out, rows.Err()
}

func rowIDToProvisionKey(provID string, rowID int64) any {
	provID = strings.TrimSpace(provID)
	if provID == "" {
		return rowID
	}
	if n, err := strconv.Atoi(provID); err == nil {
		return n
	}
	return provID
}

/*
func loadSimpleIndexedArtifacts(ctx context.Context, db *sql.DB, query string, recordID int64) ([]indexedArtifact, error) {
	rows, err := db.QueryContext(ctx, query, recordID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []indexedArtifact
	for rows.Next() {
		var (
			id        string
			spansRaw  []byte
			searchDoc string
		)
		if err := rows.Scan(&id, &spansRaw, &searchDoc); err != nil {
			return nil, err
		}
		var spansAny any
		if len(spansRaw) > 0 {
			_ = json.Unmarshal(spansRaw, &spansAny)
		}
		out = append(out, indexedArtifact{
			ID:             strings.TrimSpace(id),
			SourceSpans:    normalizeSourceLineSpans(spansAny),
			SearchDocument: strings.TrimSpace(searchDoc),
		})
	}
	return out, rows.Err()
}
*/
