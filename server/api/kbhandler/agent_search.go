package kbhandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// AgentKnowledgeSearchResult is the bounded evidence shape exposed to ChenWeb's
// internal agent-tool boundary. It deliberately excludes the full search record.
type AgentKnowledgeSearchResult struct {
	KnowledgeStoreID  string
	DocumentGroup     string
	DocumentID        string
	ArtifactID        string
	ArtifactType      string
	SourceTitle       string
	SourceVersion     string
	SourceFingerprint string
	SourceLineSpans   json.RawMessage
	ValidationStatus  string
	PrimaryLabel      string
	SecondaryLabel    string
	Snippet           string
	Score             float64
}

// SearchAgentKnowledge uses the same flag-gated hybrid retrieval and lexical
// fallback as ChenWeb's normal artifact search, while applying agent scope in
// every candidate query before ranking.
func SearchAgentKnowledge(ctx context.Context, db *sql.DB, query, storeID, documentGroup, documentID, artifactType string, limit int) ([]AgentKnowledgeSearchResult, error) {
	if db == nil || strings.TrimSpace(query) == "" || strings.TrimSpace(storeID) == "" || limit < 1 || limit > 20 {
		return nil, fmt.Errorf("invalid bounded agent search")
	}
	filters := artifactSearchFilters{KnowledgeStoreID: strings.TrimSpace(storeID), DocumentGroup: strings.TrimSpace(documentGroup)}
	if strings.TrimSpace(documentID) != "" {
		id, err := strconv.ParseInt(strings.TrimSpace(documentID), 10, 64)
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("invalid document id")
		}
		filters.InputRecordID = &id
	}
	hits, err := queryRegistrySearchResults(db, strings.TrimSpace(artifactType), strings.TrimSpace(query), filters, 1, limit, loadRegistrySearchConfig())
	if err != nil {
		return nil, err
	}
	out := make([]AgentKnowledgeSearchResult, 0, len(hits))
	for _, hit := range hits {
		var item AgentKnowledgeSearchResult
		item.DocumentID = strconv.FormatInt(hit.InputRecordID, 10)
		item.ArtifactID, item.ArtifactType = hit.ArtifactID, hit.ArtifactType
		item.SourceLineSpans = hit.SourceLineSpans
		item.PrimaryLabel, item.SecondaryLabel, item.Snippet = hit.PrimaryLabel, hit.SecondaryLabel, hit.Snippet
		item.Score = hit.Score
		err := db.QueryRowContext(ctx, `SELECT ks.id::text, COALESCE(i.type, ''), COALESCE(NULLIF(sa.source_title, ''), i.title, ''),
i.modify_time::text, COALESCE(NULLIF(i.md5, ''), 'input:' || i.id::text || ':' || EXTRACT(EPOCH FROM i.modify_time)::bigint::text),
COALESCE(NULLIF(sa.semantic_payload->>'validation_status', ''), 'unreviewed')
FROM kb.search_artifacts sa
JOIN kb.inputs i ON i.id=sa.input_record_id
JOIN kb.knowledge_store ks ON ks.id=i.ks_store_id
WHERE sa.artifact_type=$1 AND sa.artifact_id=$2 AND i.id=$3
  AND ks.status='active' AND ks.id::text=$4 AND ($5='' OR i.type=$5)`,
			hit.ArtifactType, hit.ArtifactID, hit.InputRecordID, filters.KnowledgeStoreID, filters.DocumentGroup).
			Scan(&item.KnowledgeStoreID, &item.DocumentGroup, &item.SourceTitle, &item.SourceVersion, &item.SourceFingerprint, &item.ValidationStatus)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}
