package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/kbsearch"
	appconfig "github.com/chendingplano/deepdoc/server/cmd/config"
	"github.com/chendingplano/shared/go/api/ApiTypes"
)

func ReplaceSummaryArtifactsForRecord(ctx context.Context, recordID int64, summaries []SummaryItem, logger ApiTypes.JimoLogger) error {
	db := ApiTypes.ProjectDBHandle
	if db == nil {
		if logger != nil {
			logger.Warn("summary table sync skipped because project db handle is nil", "record_id", recordID)
		}
		return nil
	}
	if recordID <= 0 {
		return fmt.Errorf("invalid record id: %d", recordID)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM kb.summaries WHERE input_record_id = $1`, recordID); err != nil {
		return err
	}

	const insertSQL = `
INSERT INTO kb.summaries (
    summary_id,
    input_record_id,
    summary_level,
    summary_seq_no,
    source_line_spans,
    child_summary_ids,
    keywords,
    keywords_en,
    summary_text,
    summary_text_en,
    language
) VALUES (
    $1, $2, $3, $4, $5::jsonb, $6::jsonb, $7::jsonb, $8::jsonb, $9, $10, $11
)`
	for _, item := range summaries {
		if _, err := tx.ExecContext(ctx, insertSQL,
			strings.TrimSpace(item.SummaryID),
			recordID,
			item.Level,
			item.SeqNo,
			mustJSONString(item.Lines, []string{}),
			mustJSONString(item.Children, []string{}),
			mustJSONString(item.Keywords, []string{}),
			mustJSONString(item.KeywordsEn, []string{}),
			strings.TrimSpace(item.Summary),
			strings.TrimSpace(item.SummaryEn),
			strings.TrimSpace(item.Language),
		); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	if logger != nil {
		logger.Info("summary artifacts synced to table", "record_id", recordID, "row_count", len(summaries))
	}
	return nil
}

func ReplaceTopicArtifactsForRecord(ctx context.Context, recordID int64, topics []TopicItem, logger ApiTypes.JimoLogger) error {
	db := ApiTypes.ProjectDBHandle
	if db == nil {
		if logger != nil {
			logger.Warn("topic table sync skipped because project db handle is nil", "record_id", recordID)
		}
		return nil
	}
	if recordID <= 0 {
		return fmt.Errorf("invalid record id: %d", recordID)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM kb.topics WHERE input_record_id = $1`, recordID); err != nil {
		return err
	}

	const insertSQL = `
INSERT INTO kb.topics (
    topic_id,
    input_record_id,
    topic_seq_no,
    topic_type,
    topic_type_en,
    source_line_spans,
    keywords,
    keywords_en,
    category_paths,
    category_paths_en,
    category_path_items,
    category_path_items_en,
    topic_desc,
    topic_desc_en
) VALUES (
    $1, $2, $3, $4, $5, $6::jsonb, $7::jsonb, $8::jsonb, $9::jsonb, $10::jsonb,
    $11::jsonb, $12::jsonb, $13, $14
)`
	for _, item := range topics {
		topicID := fmt.Sprintf("%d_tpc_%d", recordID, item.SeqNo)
		if _, err := tx.ExecContext(ctx, insertSQL,
			topicID,
			recordID,
			item.SeqNo,
			strings.TrimSpace(item.TopicType),
			strings.TrimSpace(item.TopicTypeEn),
			mustJSONString(item.Lines, []string{}),
			mustJSONString(item.Keywords, []string{}),
			mustJSONString(item.KeywordsEn, []string{}),
			mustJSONString(topicCategoryNames(item.CategoryPathDetail), []string{}),
			mustJSONString(topicCategoryNames(item.CategoryPathDetailEn), []string{}),
			mustJSONString(item.CategoryPathDetail, []CategoryPathEntry{}),
			mustJSONString(item.CategoryPathDetailEn, []CategoryPathEntry{}),
			strings.TrimSpace(item.Topic),
			strings.TrimSpace(item.TopicEn),
		); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	if logger != nil {
		logger.Info("topic artifacts synced to table", "record_id", recordID, "row_count", len(topics))
	}
	return nil
}

func ReplaceSummaryArtifactsFromArtifactFiles(ctx context.Context, recordID int64, logger ApiTypes.JimoLogger) error {
	artifactDir := strings.TrimSpace(os.Getenv("ARTIFACT_DIR"))
	recordDir, err := buildRecordArtifactDir(artifactDir, recordID)
	if err != nil {
		return err
	}
	paths, err := filepath.Glob(filepath.Join(recordDir, "summary_*_*.txt"))
	if err != nil {
		return err
	}
	summaries := make([]SummaryItem, 0, len(paths))
	for _, path := range paths {
		item, err := parseSummaryArtifactFile(path)
		if err != nil {
			return err
		}
		summaries = append(summaries, SummaryItem{
			SummaryID:     item.SummaryID,
			RecordID:      recordID,
			Level:         item.Level,
			SeqNo:         item.SeqNo,
			Lines:         append([]string(nil), item.Lines...),
			Children:      append([]string(nil), item.Children...),
			Keywords:      append([]string(nil), item.Keywords...),
			KeywordsEn:    append([]string(nil), item.KeywordsEn...),
			CategoryPaths: append([]string(nil), item.CategoryPaths...),
			Summary:       item.SummaryText,
			SummaryEn:     item.SummaryTextEn,
			Language:      item.Language,
		})
	}
	return ReplaceSummaryArtifactsForRecord(ctx, recordID, summaries, logger)
}

func ReplaceTopicArtifactsFromArtifactFiles(ctx context.Context, recordID int64, logger ApiTypes.JimoLogger) error {
	artifactDir := strings.TrimSpace(os.Getenv("ARTIFACT_DIR"))
	recordDir, err := buildRecordArtifactDir(artifactDir, recordID)
	if err != nil {
		return err
	}
	paths, err := filepath.Glob(filepath.Join(recordDir, "*.topics"))
	if err != nil {
		return err
	}
	topics := make([]TopicItem, 0, len(paths)*4)
	for _, path := range paths {
		items, err := parseTopicArtifactFile(path)
		if err != nil {
			return err
		}
		for _, item := range items {
			seqNo, _ := strconv.Atoi(strings.TrimSpace(item.TopicID))
			topics = append(topics, TopicItem{
				SeqNo:      seqNo,
				TopicType:  item.TopicType,
				Lines:      append([]string(nil), item.Lines...),
				Keywords:   append([]string(nil), item.Keywords...),
				KeywordsEn: append([]string(nil), item.KeywordsEn...),
				Topic:      item.TopicDesc,
				TopicEn:    item.TopicDescEn,
				CategoryPathDetail: []CategoryPathEntry{{
					Nodes: categoryPathNodesFromNames(item.CategoryPaths),
				}},
				CategoryPathDetailEn: []CategoryPathEntry{{
					Nodes: categoryPathNodesFromNames(item.CategoryPathsEn),
				}},
			})
		}
	}
	return ReplaceTopicArtifactsForRecord(ctx, recordID, topics, logger)
}

func buildSummaryRegistryRowsFromDB(ctx context.Context, db *sql.DB, recordID int64, sourceTitle string) ([]kbsearch.RegistryRow, error) {
	const q = `
SELECT id, summary_id, summary_level, summary_seq_no, source_line_spans, keywords, summary_text, summary_text_en, search_document
FROM kb.summaries
WHERE input_record_id = $1
ORDER BY summary_level ASC, summary_seq_no ASC, id ASC`
	rows, err := db.QueryContext(ctx, q, recordID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	weights := appconfig.GetSummarySearchWeightsConfig()
	out := make([]kbsearch.RegistryRow, 0, 16)
	for rows.Next() {
		var (
			id              int64
			summaryID       string
			level           int
			seqNo           int
			sourceLineSpans []byte
			keywords        []byte
			summaryText     string
			summaryTextEn   string
			searchDocument  string
		)
		if err := rows.Scan(&id, &summaryID, &level, &seqNo, &sourceLineSpans, &keywords, &summaryText, &summaryTextEn, &searchDocument); err != nil {
			return nil, err
		}
		payload, _ := json.Marshal(map[string]any{
			"keywords":        rawJSONArrayStrings(keywords),
			"level":           level,
			"seq_no":          seqNo,
			"summary_text":    summaryText,
			"summary_text_en": summaryTextEn,
		})
		weightedSearchDoc := buildSummarySearchDocument(weights, summarySearchFields{
			SummaryText:   summaryText,
			SummaryTextEn: summaryTextEn,
			Keywords:      rawJSONArrayStrings(keywords),
		})
		out = append(out, kbsearch.RegistryRow{
			ArtifactType:    searchArtifactSummary,
			ArtifactID:      kbsearch.BuildArtifactID(recordID, searchArtifactSummary, strconv.Itoa(seqNo)),
			InputRecordID:   recordID,
			SourceRowID:     &id,
			PrimaryLabel:    firstNonEmpty(summaryText, summaryTextEn, summaryID),
			SecondaryLabel:  fmt.Sprintf("Level %d", level),
			SearchDocument:  firstNonEmpty(weightedSearchDoc, searchDocument, strings.TrimSpace(summaryText+" "+summaryTextEn), summaryText, summaryTextEn),
			SnippetBasis:    firstNonEmpty(summaryText, summaryTextEn),
			SourceTitle:     sourceTitle,
			SourceFilename:  sourceTitle,
			SourceLineSpans: json.RawMessage(sourceLineSpans),
			SemanticPayload: payload,
		})
	}
	return out, rows.Err()
}

func buildTopicRegistryRowsFromDB(ctx context.Context, db *sql.DB, recordID int64, sourceTitle string) ([]kbsearch.RegistryRow, error) {
	const q = `
SELECT id, topic_id, topic_type, topic_desc, topic_desc_en, keywords, keywords_en, category_paths, category_paths_en, source_line_spans, search_document
FROM kb.topics
WHERE input_record_id = $1
ORDER BY topic_seq_no ASC, id ASC`
	rows, err := db.QueryContext(ctx, q, recordID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	weights := appconfig.GetTopicSearchWeightsConfig()
	out := make([]kbsearch.RegistryRow, 0, 16)
	for rows.Next() {
		var (
			id              int64
			topicID         string
			topicType       string
			topicDesc       string
			topicDescEn     string
			keywords        []byte
			keywordsEn      []byte
			categoryPaths   []byte
			categoryPathsEn []byte
			sourceLineSpans []byte
			searchDocument  string
		)
		if err := rows.Scan(&id, &topicID, &topicType, &topicDesc, &topicDescEn, &keywords, &keywordsEn, &categoryPaths, &categoryPathsEn, &sourceLineSpans, &searchDocument); err != nil {
			return nil, err
		}
		payload, _ := json.Marshal(map[string]any{
			"topic_type":        topicType,
			"topic_desc":        topicDesc,
			"topic_desc_en":     topicDescEn,
			"keywords":          rawJSONArrayStrings(keywords),
			"keywords_en":       rawJSONArrayStrings(keywordsEn),
			"category_paths":    rawJSONArrayStrings(categoryPaths),
			"category_paths_en": rawJSONArrayStrings(categoryPathsEn),
			"source_line_specs": rawJSONArrayStrings(sourceLineSpans),
		})
		weightedSearchDoc := buildTopicSearchDocument(weights, topicSearchFields{
			TopicType:       topicType,
			TopicDesc:       topicDesc,
			TopicDescEn:     topicDescEn,
			Keywords:        rawJSONArrayStrings(keywords),
			KeywordsEn:      rawJSONArrayStrings(keywordsEn),
			CategoryPaths:   rawJSONArrayStrings(categoryPaths),
			CategoryPathsEn: rawJSONArrayStrings(categoryPathsEn),
		})
		out = append(out, kbsearch.RegistryRow{
			ArtifactType:    searchArtifactTopic,
			ArtifactID:      kbsearch.BuildArtifactID(recordID, searchArtifactTopic, lastDelimitedToken(topicID)),
			InputRecordID:   recordID,
			SourceRowID:     &id,
			PrimaryLabel:    firstNonEmpty(topicDesc, topicDescEn, topicID),
			SecondaryLabel:  topicType,
			SearchDocument:  firstNonEmpty(weightedSearchDoc, searchDocument, strings.TrimSpace(strings.Join([]string{topicType, topicDesc, topicDescEn, jsonArrayTextBytes(keywords), jsonArrayTextBytes(keywordsEn), jsonArrayTextBytes(categoryPaths), jsonArrayTextBytes(categoryPathsEn)}, " "))),
			SnippetBasis:    firstNonEmpty(topicDesc, topicDescEn),
			SourceTitle:     sourceTitle,
			SourceFilename:  sourceTitle,
			CategoryPaths:   json.RawMessage(categoryPaths),
			SourceLineSpans: json.RawMessage(sourceLineSpans),
			SemanticPayload: payload,
		})
	}
	return out, rows.Err()
}

func topicCategoryNames(items []CategoryPathEntry) []string {
	if len(items) == 0 {
		return []string{}
	}
	return append([]string(nil), items[0].NodeNames()...)
}

func mustJSONString(value any, fallback any) string {
	if value == nil {
		value = fallback
	}
	raw, err := json.Marshal(value)
	if err != nil {
		raw, _ = json.Marshal(fallback)
	}
	if string(raw) == "null" {
		raw, _ = json.Marshal(fallback)
	}
	return string(raw)
}

func jsonArrayTextBytes(raw []byte) string {
	var items []string
	if err := json.Unmarshal(raw, &items); err != nil {
		return ""
	}
	return strings.TrimSpace(strings.Join(items, " "))
}

func categoryPathNodesFromNames(names []string) []CategoryPathNode {
	if len(names) == 0 {
		return []CategoryPathNode{}
	}
	out := make([]CategoryPathNode, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		out = append(out, CategoryPathNode{Name: name})
	}
	return out
}

func (entry CategoryPathEntry) NodeNames() []string {
	if len(entry.Nodes) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(entry.Nodes))
	for _, node := range entry.Nodes {
		name := strings.TrimSpace(node.Name)
		if name == "" {
			continue
		}
		out = append(out, name)
	}
	return out
}
