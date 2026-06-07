package docprocessing

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/kbsearch"
	appconfig "github.com/chendingplano/deepdoc/server/cmd/config"
	"github.com/chendingplano/shared/go/api/ApiTypes"
)

const (
	searchArtifactSummary            = "summary"
	searchArtifactTopic              = "topic"
	searchArtifactSceneBlock         = "scene_block"
	searchArtifactMetric             = "metric"
	searchArtifactProvision          = "provision"
	searchArtifactProduct            = "product"
	searchArtifactSemanticProjection = "semantic_projection"
	searchArtifactKnowledge          = "knowledge"
	searchArtifactEntity             = "entity"
	searchArtifactRelation           = "relation"
)

func ReindexSummarySearchForRecord(ctx context.Context, recordID int64, logger ApiTypes.JimoLogger) error {
	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return fmt.Errorf("project db handle is nil")
	}
	sourceTitle, err := fetchSearchSourceTitle(ctx, db, recordID)
	if err != nil {
		return err
	}
	rows, err := buildSummaryRegistryRows(ctx, db, recordID, sourceTitle)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		if err := ReplaceSummaryArtifactsFromArtifactFiles(ctx, recordID, logger); err == nil {
			rows, err = buildSummaryRegistryRowsFromDB(ctx, db, recordID, sourceTitle)
			if err != nil {
				return err
			}
		}
	}
	return replaceRegistryRows(ctx, db, searchArtifactSummary, recordID, rows, logger)
}

func ReindexTopicSearchForRecord(ctx context.Context, recordID int64, logger ApiTypes.JimoLogger) error {
	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return fmt.Errorf("project db handle is nil")
	}
	sourceTitle, err := fetchSearchSourceTitle(ctx, db, recordID)
	if err != nil {
		return err
	}
	rows, err := buildTopicRegistryRows(ctx, db, recordID, sourceTitle)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		if err := ReplaceTopicArtifactsFromArtifactFiles(ctx, recordID, logger); err == nil {
			rows, err = buildTopicRegistryRowsFromDB(ctx, db, recordID, sourceTitle)
			if err != nil {
				return err
			}
		}
	}
	return replaceRegistryRows(ctx, db, searchArtifactTopic, recordID, rows, logger)
}

func ReindexSceneBlockSearchForRecord(ctx context.Context, recordID int64, logger ApiTypes.JimoLogger) error {
	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return fmt.Errorf("project db handle is nil")
	}
	rows, err := buildSceneBlockRegistryRows(ctx, db, recordID)
	if err != nil {
		return err
	}
	return replaceRegistryRows(ctx, db, searchArtifactSceneBlock, recordID, rows, logger)
}

func ReindexMetricSearchForRecord(ctx context.Context, recordID int64, logger ApiTypes.JimoLogger) error {
	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return fmt.Errorf("project db handle is nil")
	}
	rows, err := buildMetricRegistryRows(ctx, db, recordID)
	if err != nil {
		return err
	}
	return replaceRegistryRows(ctx, db, searchArtifactMetric, recordID, rows, logger)
}

func ReindexProvisionSearchForRecord(ctx context.Context, recordID int64, logger ApiTypes.JimoLogger) error {
	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return fmt.Errorf("project db handle is nil")
	}
	rows, err := buildProvisionRegistryRows(ctx, db, recordID)
	if err != nil {
		return err
	}
	return replaceRegistryRows(ctx, db, searchArtifactProvision, recordID, rows, logger)
}

func ReindexSemanticProjectionSearchForRecord(ctx context.Context, recordID int64, logger ApiTypes.JimoLogger) error {
	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return fmt.Errorf("project db handle is nil")
	}
	rows, err := buildSemanticProjectionRegistryRows(ctx, db, recordID)
	if err != nil {
		return err
	}
	return replaceRegistryRows(ctx, db, searchArtifactSemanticProjection, recordID, rows, logger)
}

func ReindexKnowledgeSearchForRecord(ctx context.Context, recordID int64, logger ApiTypes.JimoLogger) error {
	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return fmt.Errorf("project db handle is nil")
	}
	rows, err := buildKnowledgeRegistryRows(ctx, db, recordID)
	if err != nil {
		return err
	}
	return replaceRegistryRows(ctx, db, searchArtifactKnowledge, recordID, rows, logger)
}

func ReindexEntitySearchForRecord(ctx context.Context, recordID int64, logger ApiTypes.JimoLogger) error {
	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return fmt.Errorf("project db handle is nil")
	}
	rows, err := buildEntityRegistryRows(ctx, db, recordID)
	if err != nil {
		return err
	}
	return replaceRegistryRows(ctx, db, searchArtifactEntity, recordID, rows, logger)
}

func ReindexRelationSearchForRecord(ctx context.Context, recordID int64, logger ApiTypes.JimoLogger) error {
	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return fmt.Errorf("project db handle is nil")
	}
	rows, err := buildRelationRegistryRows(ctx, db, recordID)
	if err != nil {
		return err
	}
	return replaceRegistryRows(ctx, db, searchArtifactRelation, recordID, rows, logger)
}

func ReindexProductSearchForRecord(ctx context.Context, recordID int64, logger ApiTypes.JimoLogger) error {
	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return fmt.Errorf("project db handle is nil")
	}
	rows, err := buildProductRegistryRows(ctx, db, recordID)
	if err != nil {
		return err
	}
	return replaceRegistryRows(ctx, db, searchArtifactProduct, recordID, rows, logger)
}

func replaceRegistryRows(ctx context.Context, db *sql.DB, artifactType string, recordID int64, rows []kbsearch.RegistryRow, logger ApiTypes.JimoLogger) error {
	// Best-effort semantic enrichment. Only when the hybrid-search feature flag is
	// on (which requires the pgvector migration applied); otherwise rows stay
	// lexical-only and InsertSearchRegistryRows uses the embedding-free statement.
	if kbsearch.SemanticSearchEnabled() {
		embedRegistryRows(ctx, rows, logger)
	}
	deleted, err := kbsearch.DeleteSearchRegistryRowsForRecord(ctx, db, artifactType, recordID)
	if err != nil {
		return err
	}
	inserted, err := kbsearch.InsertSearchRegistryRows(ctx, db, rows)
	if err != nil {
		return err
	}
	if logger != nil {
		logger.Info("search registry reindexed",
			"artifact_type", artifactType,
			"input_record_id", recordID,
			"deleted_rows", deleted,
			"inserted_rows", inserted,
		)
	}
	return nil
}

func reindexExistingSearchOnSkip(ctx context.Context, artifactType string, recordID int64, logger ApiTypes.JimoLogger, reindex func(context.Context, int64, ApiTypes.JimoLogger) error) {
	if reindex == nil {
		return
	}
	if err := reindex(ctx, recordID, logger); err != nil && logger != nil {
		logger.Warn("reindex search registry for existing artifacts failed",
			"artifact_type", artifactType,
			"input_record_id", recordID,
			"error", err)
	}
}

func fetchSearchSourceTitle(ctx context.Context, db *sql.DB, recordID int64) (string, error) {
	var fileName sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT file_name FROM kb.inputs WHERE id = $1`, recordID).Scan(&fileName); err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(fileName.String), nil
}

func buildSummaryRegistryRows(ctx context.Context, db *sql.DB, recordID int64, sourceTitle string) ([]kbsearch.RegistryRow, error) {
	rows, err := buildSummaryRegistryRowsFromDB(ctx, db, recordID, sourceTitle)
	if err != nil {
		return nil, err
	}
	if len(rows) > 0 {
		return rows, nil
	}
	return buildSummaryRegistryRowsFromFiles(recordID, sourceTitle)
}

func buildSummaryRegistryRowsFromFiles(recordID int64, sourceTitle string) ([]kbsearch.RegistryRow, error) {
	artifactDir := strings.TrimSpace(os.Getenv("ARTIFACT_DIR"))
	recordDir, err := buildRecordArtifactDir(artifactDir, recordID)
	if err != nil {
		return nil, err
	}
	paths, err := filepath.Glob(filepath.Join(recordDir, "summary_*_*.txt"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	weights := appconfig.GetSummarySearchWeightsConfig()
	rows := make([]kbsearch.RegistryRow, 0, len(paths))
	for _, path := range paths {
		item, err := parseSummaryArtifactFile(path)
		if err != nil {
			return nil, err
		}
		payload, _ := json.Marshal(map[string]any{
			"keywords":       item.Keywords,
			"category_paths": item.CategoryPaths,
			"level":          item.Level,
			"seq_no":         item.SeqNo,
			"summary_text":   item.SummaryText,
			"lines":          item.Lines,
		})
		searchDoc := buildSummarySearchDocument(weights, summarySearchFields{
			SummaryText:   item.SummaryText,
			SummaryTextEn: item.SummaryTextEn,
			Keywords:      item.Keywords,
			KeywordsEn:    item.KeywordsEn,
			CategoryPaths: item.CategoryPaths,
		})
		rows = append(rows, kbsearch.RegistryRow{
			ArtifactType:    searchArtifactSummary,
			ArtifactID:      kbsearch.BuildArtifactID(recordID, searchArtifactSummary, strconv.Itoa(item.SeqNo)),
			InputRecordID:   recordID,
			PrimaryLabel:    firstNonEmpty(item.SummaryText, item.SummaryID),
			SecondaryLabel:  fmt.Sprintf("Level %d", item.Level),
			SearchDocument:  searchDoc,
			SnippetBasis:    item.SummaryText,
			SourceTitle:     sourceTitle,
			SourceFilename:  sourceTitle,
			CategoryPaths:   mustJSON(item.CategoryPaths, []string{}),
			SourceLineSpans: mustJSON(item.Lines, []string{}),
			SemanticPayload: payload,
		})
	}
	return rows, nil
}

func buildTopicRegistryRows(ctx context.Context, db *sql.DB, recordID int64, sourceTitle string) ([]kbsearch.RegistryRow, error) {
	rows, err := buildTopicRegistryRowsFromDB(ctx, db, recordID, sourceTitle)
	if err != nil {
		return nil, err
	}
	if len(rows) > 0 {
		return rows, nil
	}
	return buildTopicRegistryRowsFromFiles(recordID, sourceTitle)
}

func buildTopicRegistryRowsFromFiles(recordID int64, sourceTitle string) ([]kbsearch.RegistryRow, error) {
	artifactDir := strings.TrimSpace(os.Getenv("ARTIFACT_DIR"))
	recordDir, err := buildRecordArtifactDir(artifactDir, recordID)
	if err != nil {
		return nil, err
	}
	paths, err := filepath.Glob(filepath.Join(recordDir, "*.topics"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	weights := appconfig.GetTopicSearchWeightsConfig()
	rows := make([]kbsearch.RegistryRow, 0, len(paths)*4)
	for _, path := range paths {
		topics, err := parseTopicArtifactFile(path)
		if err != nil {
			return nil, err
		}
		for _, item := range topics {
			payload, _ := json.Marshal(map[string]any{
				"topic_type":        item.TopicType,
				"topic_desc":        item.TopicDesc,
				"topic_desc_en":     item.TopicDescEn,
				"keywords":          item.Keywords,
				"keywords_en":       item.KeywordsEn,
				"category_paths":    item.CategoryPaths,
				"category_paths_en": item.CategoryPathsEn,
				"source_line_specs": item.Lines,
			})
			searchDoc := buildTopicSearchDocument(weights, topicSearchFields{
				TopicType:       item.TopicType,
				TopicDesc:       item.TopicDesc,
				TopicDescEn:     item.TopicDescEn,
				Keywords:        item.Keywords,
				KeywordsEn:      item.KeywordsEn,
				CategoryPaths:   item.CategoryPaths,
				CategoryPathsEn: item.CategoryPathsEn,
			})
			rows = append(rows, kbsearch.RegistryRow{
				ArtifactType:    searchArtifactTopic,
				ArtifactID:      kbsearch.BuildArtifactID(recordID, searchArtifactTopic, lastDelimitedToken(item.TopicID)),
				InputRecordID:   recordID,
				PrimaryLabel:    firstNonEmpty(item.TopicDesc, item.TopicID),
				SecondaryLabel:  item.TopicType,
				SearchDocument:  searchDoc,
				SnippetBasis:    firstNonEmpty(item.TopicDesc, item.TopicDescEn),
				SourceTitle:     sourceTitle,
				SourceFilename:  sourceTitle,
				CategoryPaths:   mustJSON(item.CategoryPaths, []string{}),
				SourceLineSpans: mustJSON(item.Lines, []string{}),
				SemanticPayload: payload,
			})
		}
	}
	return rows, nil
}

func buildSceneBlockRegistryRows(ctx context.Context, db *sql.DB, recordID int64) ([]kbsearch.RegistryRow, error) {
	const q = `
SELECT id, object_id, title, scene_type, summary, keywords, line_spans, search_document
FROM kb.scene_objects
WHERE input_record_id = $1
ORDER BY id`
	rows, err := db.QueryContext(ctx, q, recordID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	weights := appconfig.GetSceneBlockSearchWeightsConfig()
	out := make([]kbsearch.RegistryRow, 0, 16)
	for rows.Next() {
		var (
			id        int64
			objectID  string
			title     string
			sceneType string
			summary   string
			keywords  []byte
			lineSpans []byte
			searchDoc string
		)
		if err := rows.Scan(&id, &objectID, &title, &sceneType, &summary, &keywords, &lineSpans, &searchDoc); err != nil {
			return nil, err
		}
		kw := rawJSONArrayStrings(keywords)
		payload, _ := json.Marshal(map[string]any{
			"scene_type": sceneType,
			"summary":    summary,
			"keywords":   kw,
		})
		seq := lastDelimitedToken(objectID)
		if seq == "" {
			seq = strconv.FormatInt(id, 10)
		}
		weightedSearchDoc := buildSceneBlockSearchDocument(weights, sceneBlockSearchFields{
			Title:     title,
			SceneType: sceneType,
			Summary:   summary,
			Keywords:  kw,
		})
		out = append(out, kbsearch.RegistryRow{
			ArtifactType:    searchArtifactSceneBlock,
			ArtifactID:      kbsearch.BuildArtifactID(recordID, searchArtifactSceneBlock, seq),
			InputRecordID:   recordID,
			SourceRowID:     &id,
			PrimaryLabel:    firstNonEmpty(title, objectID),
			SecondaryLabel:  sceneType,
			SearchDocument:  firstNonEmpty(weightedSearchDoc, searchDoc, strings.Join([]string{title, summary}, " ")),
			SnippetBasis:    firstNonEmpty(summary, title),
			SourceLineSpans: json.RawMessage(lineSpans),
			SemanticPayload: payload,
		})
	}
	return out, rows.Err()
}

func buildMetricRegistryRows(ctx context.Context, db *sql.DB, recordID int64) ([]kbsearch.RegistryRow, error) {
	const q = `
SELECT id, metric_id, metric_name, metric_unit, metric_desc, metric_keywords, category_paths, source_line_spans, search_document
FROM kb.metrics
WHERE input_record_id = $1
ORDER BY id`
	rows, err := db.QueryContext(ctx, q, recordID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]kbsearch.RegistryRow, 0, 16)
	for rows.Next() {
		var (
			id              int64
			metricID        string
			metricName      string
			metricUnit      string
			metricDesc      string
			metricKeywords  []byte
			categoryPaths   []byte
			sourceLineSpans []byte
			searchDoc       string
		)
		if err := rows.Scan(&id, &metricID, &metricName, &metricUnit, &metricDesc, &metricKeywords, &categoryPaths, &sourceLineSpans, &searchDoc); err != nil {
			return nil, err
		}
		payload, _ := json.Marshal(map[string]any{
			"metric_unit":     metricUnit,
			"metric_desc":     metricDesc,
			"metric_keywords": rawJSONArrayStrings(metricKeywords),
		})
		seq := lastDelimitedToken(metricID)
		if seq == "" {
			seq = strconv.FormatInt(id, 10)
		}
		out = append(out, kbsearch.RegistryRow{
			ArtifactType:    searchArtifactMetric,
			ArtifactID:      kbsearch.BuildArtifactID(recordID, searchArtifactMetric, seq),
			InputRecordID:   recordID,
			SourceRowID:     &id,
			PrimaryLabel:    firstNonEmpty(metricName, metricID),
			SecondaryLabel:  metricUnit,
			SearchDocument:  firstNonEmpty(searchDoc, strings.Join([]string{metricName, metricDesc, metricUnit}, " ")),
			SnippetBasis:    firstNonEmpty(metricDesc, metricName),
			CategoryPaths:   json.RawMessage(categoryPaths),
			SourceLineSpans: json.RawMessage(sourceLineSpans),
			SemanticPayload: payload,
		})
	}
	return out, rows.Err()
}

func buildProvisionRegistryRows(ctx context.Context, db *sql.DB, recordID int64) ([]kbsearch.RegistryRow, error) {
	const q = `
SELECT id, prov_id, prov_name, provision_type, prov_desc, provision_keywords, category_paths, source_line_spans, input_filename, search_document
FROM kb.provisions
WHERE input_record_id = $1
ORDER BY id`
	rows, err := db.QueryContext(ctx, q, recordID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	weights := appconfig.GetProvisionSearchWeightsConfig()
	out := make([]kbsearch.RegistryRow, 0, 16)
	for rows.Next() {
		var (
			id              int64
			provID          string
			provName        string
			provisionType   string
			provDesc        string
			keywords        []byte
			categoryPaths   []byte
			sourceLineSpans []byte
			inputFilename   string
			searchDoc       string
		)
		if err := rows.Scan(&id, &provID, &provName, &provisionType, &provDesc, &keywords, &categoryPaths, &sourceLineSpans, &inputFilename, &searchDoc); err != nil {
			return nil, err
		}
		payload, _ := json.Marshal(map[string]any{
			"provision_type": provisionType,
			"prov_desc":      provDesc,
			"keywords":       rawJSONArrayStrings(keywords),
		})
		searchDocWeighted := buildProvisionRegistrySearchDocument(weights, provisionSearchFields{
			ProvisionName: provName,
			ProvisionType: provisionType,
			ProvisionDesc: provDesc,
			Keywords:      rawJSONArrayStrings(keywords),
			CategoryPaths: flattenSearchCategoryPaths(categoryPaths),
		})
		artifactID := provID
		if strings.TrimSpace(artifactID) == "" {
			artifactID = strconv.FormatInt(id, 10)
		}
		artifactID = kbsearch.BuildArtifactID(recordID, searchArtifactProvision, artifactID)
		out = append(out, kbsearch.RegistryRow{
			ArtifactType:    searchArtifactProvision,
			ArtifactID:      artifactID,
			InputRecordID:   recordID,
			SourceRowID:     &id,
			PrimaryLabel:    firstNonEmpty(provName, provID),
			SecondaryLabel:  provisionType,
			SearchDocument:  firstNonEmpty(searchDocWeighted, searchDoc, strings.Join([]string{provName, provDesc, provisionType}, " ")),
			SnippetBasis:    firstNonEmpty(provDesc, provName),
			SourceTitle:     inputFilename,
			SourceFilename:  inputFilename,
			CategoryPaths:   json.RawMessage(categoryPaths),
			SourceLineSpans: json.RawMessage(sourceLineSpans),
			SemanticPayload: payload,
		})
	}
	return out, rows.Err()
}

func buildSemanticProjectionRegistryRows(ctx context.Context, db *sql.DB, recordID int64) ([]kbsearch.RegistryRow, error) {
	const q = `
SELECT id, semantic_proj_id, language, descriptive_name, descriptive_name_en, keywords, keywords_en, semantic_projection, semantic_projection_en, category_paths, category_paths_en, line_spans
FROM kb.semantic_projections
WHERE input_record_id = $1
ORDER BY id`
	rows, err := db.QueryContext(ctx, q, recordID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	weights := appconfig.GetSemanticProjectionSearchWeightsConfig()
	out := make([]kbsearch.RegistryRow, 0, 16)
	for rows.Next() {
		var (
			id                   int64
			semanticProjID       string
			language             string
			descriptiveName      string
			descriptiveNameEn    sql.NullString
			keywords             []byte
			keywordsEn           []byte
			semanticProjection   sql.NullString
			semanticProjectionEn sql.NullString
			categoryPaths        []byte
			categoryPathsEn      []byte
			lineSpans            []byte
		)
		if err := rows.Scan(&id, &semanticProjID, &language, &descriptiveName, &descriptiveNameEn, &keywords, &keywordsEn, &semanticProjection, &semanticProjectionEn, &categoryPaths, &categoryPathsEn, &lineSpans); err != nil {
			return nil, err
		}
		kw := rawJSONArrayStrings(keywords)
		kwEn := rawJSONArrayStrings(keywordsEn)
		searchDoc := buildSemanticProjectionSearchDocument(weights, semanticProjectionSearchFields{
			DescriptiveName:      descriptiveName,
			DescriptiveNameEn:    descriptiveNameEn.String,
			Keywords:             kw,
			KeywordsEn:           kwEn,
			SemanticProjection:   semanticProjection.String,
			SemanticProjectionEn: semanticProjectionEn.String,
			CategoryPaths:        flattenSearchCategoryPaths(categoryPaths),
			CategoryPathsEn:      flattenSearchCategoryPaths(categoryPathsEn),
		})
		payload, _ := json.Marshal(map[string]any{
			"language":         language,
			"descriptive_name": descriptiveName,
			"keywords":         kw,
		})
		seq := lastDelimitedToken(semanticProjID)
		if seq == "" {
			seq = strconv.FormatInt(id, 10)
		}
		artifactID := strings.TrimSpace(semanticProjID)
		if artifactID == "" {
			artifactID = kbsearch.BuildArtifactID(recordID, searchArtifactSemanticProjection, seq)
		}
		out = append(out, kbsearch.RegistryRow{
			ArtifactType:    searchArtifactSemanticProjection,
			ArtifactID:      artifactID,
			InputRecordID:   recordID,
			SourceRowID:     &id,
			PrimaryLabel:    firstNonEmpty(descriptiveName, semanticProjID),
			SecondaryLabel:  language,
			SearchDocument:  searchDoc,
			SnippetBasis:    firstNonEmpty(descriptiveName, descriptiveNameEn.String),
			CategoryPaths:   json.RawMessage(categoryPaths),
			SourceLineSpans: json.RawMessage(lineSpans),
			SemanticPayload: payload,
		})
	}
	return out, rows.Err()
}

type semanticProjectionSearchFields struct {
	DescriptiveName      string
	DescriptiveNameEn    string
	Keywords             []string
	KeywordsEn           []string
	SemanticProjection   string
	SemanticProjectionEn string
	CategoryPaths        []string
	CategoryPathsEn      []string
}

type sceneBlockSearchFields struct {
	Title     string
	SceneType string
	Summary   string
	Keywords  []string
}

type summarySearchFields struct {
	SummaryText   string
	SummaryTextEn string
	Keywords      []string
	KeywordsEn    []string
	CategoryPaths []string
}

type topicSearchFields struct {
	TopicType       string
	TopicDesc       string
	TopicDescEn     string
	Keywords        []string
	KeywordsEn      []string
	CategoryPaths   []string
	CategoryPathsEn []string
}

type provisionSearchFields struct {
	ProvisionName string
	ProvisionType string
	ProvisionDesc string
	Keywords      []string
	CategoryPaths []string
}

type entitySearchFields struct {
	Entity       string
	EntityEn     string
	EntityType   string
	EntityTypeEn string
	Aliases      []string
	AliasesEn    []string
	DescText     string
	DescTextEn   string
	Keywords     []string
	KeywordsEn   []string
}

type relationSearchFields struct {
	Subject     string
	SubjectEn   string
	Predicate   string
	PredicateEn string
	Object      string
	ObjectEn    string
	DescText    string
	DescTextEn  string
	Keywords    []string
	KeywordsEn  []string
}

func buildSemanticProjectionSearchDocument(weights appconfig.SemanticProjectionSearchWeightsConfig, fields semanticProjectionSearchFields) string {
	parts := make([]string, 0, 16)
	parts = appendWeightedText(parts, fields.DescriptiveName, weightToSearchRepeats(weights.DescriptiveName))
	parts = appendWeightedText(parts, fields.DescriptiveNameEn, weightToSearchRepeats(weights.DescriptiveName))
	parts = appendWeightedText(parts, strings.Join(fields.Keywords, " "), weightToSearchRepeats(weights.Keywords))
	parts = appendWeightedText(parts, strings.Join(fields.KeywordsEn, " "), weightToSearchRepeats(weights.Keywords))
	parts = appendWeightedText(parts, fields.SemanticProjection, weightToSearchRepeats(weights.SemanticProjection))
	parts = appendWeightedText(parts, fields.SemanticProjectionEn, weightToSearchRepeats(weights.SemanticProjection))
	parts = appendWeightedText(parts, strings.Join(fields.CategoryPaths, " "), weightToSearchRepeats(weights.CategoryPaths))
	parts = appendWeightedText(parts, strings.Join(fields.CategoryPathsEn, " "), weightToSearchRepeats(weights.CategoryPaths))
	return strings.TrimSpace(strings.Join(parts, " "))
}

func buildSceneBlockSearchDocument(weights appconfig.SceneBlockSearchWeightsConfig, fields sceneBlockSearchFields) string {
	parts := make([]string, 0, 8)
	parts = appendWeightedText(parts, fields.Title, weightToSearchRepeats(weights.Title))
	parts = appendWeightedText(parts, fields.SceneType, weightToSearchRepeats(weights.SceneType))
	parts = appendWeightedText(parts, fields.Summary, weightToSearchRepeats(weights.Summary))
	parts = appendWeightedText(parts, strings.Join(fields.Keywords, " "), weightToSearchRepeats(weights.Keywords))
	return strings.TrimSpace(strings.Join(parts, " "))
}

func buildSummarySearchDocument(weights appconfig.SummarySearchWeightsConfig, fields summarySearchFields) string {
	parts := make([]string, 0, 10)
	parts = appendWeightedText(parts, fields.SummaryText, weightToSearchRepeats(weights.SummaryText))
	parts = appendWeightedText(parts, fields.SummaryTextEn, weightToSearchRepeats(weights.SummaryText))
	parts = appendWeightedText(parts, strings.Join(fields.Keywords, " "), weightToSearchRepeats(weights.Keywords))
	parts = appendWeightedText(parts, strings.Join(fields.KeywordsEn, " "), weightToSearchRepeats(weights.Keywords))
	parts = appendWeightedText(parts, strings.Join(fields.CategoryPaths, " "), weightToSearchRepeats(weights.CategoryPaths))
	return strings.TrimSpace(strings.Join(parts, " "))
}

func buildTopicSearchDocument(weights appconfig.TopicSearchWeightsConfig, fields topicSearchFields) string {
	parts := make([]string, 0, 12)
	parts = appendWeightedText(parts, fields.TopicType, weightToSearchRepeats(weights.TopicType))
	parts = appendWeightedText(parts, fields.TopicDesc, weightToSearchRepeats(weights.TopicDesc))
	parts = appendWeightedText(parts, fields.TopicDescEn, weightToSearchRepeats(weights.TopicDesc))
	parts = appendWeightedText(parts, strings.Join(fields.Keywords, " "), weightToSearchRepeats(weights.Keywords))
	parts = appendWeightedText(parts, strings.Join(fields.KeywordsEn, " "), weightToSearchRepeats(weights.Keywords))
	parts = appendWeightedText(parts, strings.Join(fields.CategoryPaths, " "), weightToSearchRepeats(weights.CategoryPaths))
	parts = appendWeightedText(parts, strings.Join(fields.CategoryPathsEn, " "), weightToSearchRepeats(weights.CategoryPaths))
	return strings.TrimSpace(strings.Join(parts, " "))
}

func buildProvisionRegistrySearchDocument(weights appconfig.ProvisionSearchWeightsConfig, fields provisionSearchFields) string {
	parts := make([]string, 0, 8)
	parts = appendWeightedText(parts, fields.ProvisionName, weightToSearchRepeats(weights.ProvisionName))
	parts = appendWeightedText(parts, fields.ProvisionType, weightToSearchRepeats(weights.ProvisionType))
	parts = appendWeightedText(parts, fields.ProvisionDesc, weightToSearchRepeats(weights.ProvisionDesc))
	parts = appendWeightedText(parts, strings.Join(fields.Keywords, " "), weightToSearchRepeats(weights.Keywords))
	parts = appendWeightedText(parts, strings.Join(fields.CategoryPaths, " "), weightToSearchRepeats(weights.CategoryPaths))
	return strings.TrimSpace(strings.Join(parts, " "))
}

func buildEntitySearchDocument(weights appconfig.EntitySearchWeightsConfig, fields entitySearchFields) string {
	parts := make([]string, 0, 14)
	parts = appendWeightedText(parts, fields.Entity, weightToSearchRepeats(weights.Entity))
	parts = appendWeightedText(parts, fields.EntityEn, weightToSearchRepeats(weights.Entity))
	parts = appendWeightedText(parts, fields.EntityType, weightToSearchRepeats(weights.EntityType))
	parts = appendWeightedText(parts, fields.EntityTypeEn, weightToSearchRepeats(weights.EntityType))
	parts = appendWeightedText(parts, strings.Join(fields.Aliases, " "), weightToSearchRepeats(weights.Aliases))
	parts = appendWeightedText(parts, strings.Join(fields.AliasesEn, " "), weightToSearchRepeats(weights.Aliases))
	parts = appendWeightedText(parts, fields.DescText, weightToSearchRepeats(weights.DescText))
	parts = appendWeightedText(parts, fields.DescTextEn, weightToSearchRepeats(weights.DescText))
	parts = appendWeightedText(parts, strings.Join(fields.Keywords, " "), weightToSearchRepeats(weights.Keywords))
	parts = appendWeightedText(parts, strings.Join(fields.KeywordsEn, " "), weightToSearchRepeats(weights.Keywords))
	return strings.TrimSpace(strings.Join(parts, " "))
}

func buildRelationSearchDocument(weights appconfig.RelationSearchWeightsConfig, fields relationSearchFields) string {
	parts := make([]string, 0, 16)
	parts = appendWeightedText(parts, fields.Subject, weightToSearchRepeats(weights.Subject))
	parts = appendWeightedText(parts, fields.SubjectEn, weightToSearchRepeats(weights.Subject))
	parts = appendWeightedText(parts, fields.Predicate, weightToSearchRepeats(weights.Predicate))
	parts = appendWeightedText(parts, fields.PredicateEn, weightToSearchRepeats(weights.Predicate))
	parts = appendWeightedText(parts, fields.Object, weightToSearchRepeats(weights.Object))
	parts = appendWeightedText(parts, fields.ObjectEn, weightToSearchRepeats(weights.Object))
	parts = appendWeightedText(parts, fields.DescText, weightToSearchRepeats(weights.DescText))
	parts = appendWeightedText(parts, fields.DescTextEn, weightToSearchRepeats(weights.DescText))
	parts = appendWeightedText(parts, strings.Join(fields.Keywords, " "), weightToSearchRepeats(weights.Keywords))
	parts = appendWeightedText(parts, strings.Join(fields.KeywordsEn, " "), weightToSearchRepeats(weights.Keywords))
	return strings.TrimSpace(strings.Join(parts, " "))
}

func appendWeightedText(dst []string, text string, repeats int) []string {
	text = strings.TrimSpace(text)
	if text == "" || repeats <= 0 {
		return dst
	}
	for i := 0; i < repeats; i++ {
		dst = append(dst, text)
	}
	return dst
}

func weightToSearchRepeats(weight float64) int {
	if weight <= 0 {
		return 0
	}
	repeats := int(math.Round(weight * 2))
	if repeats < 1 {
		return 1
	}
	return repeats
}

func flattenSearchCategoryPaths(raw []byte) []string {
	return flattenSearchJSONTerms(raw)
}

func flattenSearchJSONTerms(raw []byte) []string {
	var decoded any
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil
	}
	out := make([]string, 0, 8)
	collectCategoryPathTerms(decoded, &out)
	return uniqueStrings(out)
}

func collectCategoryPathTerms(v any, out *[]string) {
	switch x := v.(type) {
	case string:
		if s := strings.TrimSpace(x); s != "" {
			*out = append(*out, s)
		}
	case []any:
		for _, item := range x {
			collectCategoryPathTerms(item, out)
		}
	case map[string]any:
		if name, ok := x["name"].(string); ok {
			collectCategoryPathTerms(name, out)
		}
		if kws, ok := x["keywords"]; ok {
			collectCategoryPathTerms(kws, out)
		}
		if pks, ok := x["path_keywords"]; ok {
			collectCategoryPathTerms(pks, out)
		}
		if cp, ok := x["category_path"]; ok {
			collectCategoryPathTerms(cp, out)
		}
	}
}

func buildKnowledgeRegistryRows(ctx context.Context, db *sql.DB, recordID int64) ([]kbsearch.RegistryRow, error) {
	const q = `
SELECT id, knowledge_id, knowledge_type, knowledge_value, knowledge_value_en, desc_text, desc_text_en, keywords, keywords_en, category_paths, search_document
FROM kb.knowledges
WHERE input_record_id = $1
ORDER BY id`
	rows, err := db.QueryContext(ctx, q, recordID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]kbsearch.RegistryRow, 0, 16)
	for rows.Next() {
		var (
			id               int64
			knowledgeID      string
			knowledgeType    string
			knowledgeValue   string
			knowledgeValueEn sql.NullString
			descText         string
			descTextEn       sql.NullString
			keywords         []byte
			keywordsEn       []byte
			categoryPaths    []byte
			searchDoc        sql.NullString
		)
		if err := rows.Scan(&id, &knowledgeID, &knowledgeType, &knowledgeValue, &knowledgeValueEn, &descText, &descTextEn, &keywords, &keywordsEn, &categoryPaths, &searchDoc); err != nil {
			return nil, err
		}
		kw := rawJSONArrayStrings(keywords)
		kwEn := rawJSONArrayStrings(keywordsEn)
		searchParts := []string{knowledgeType, knowledgeValue, knowledgeValueEn.String, descText, descTextEn.String, strings.Join(kw, " "), strings.Join(kwEn, " ")}
		payload, _ := json.Marshal(map[string]any{
			"knowledge_type":  knowledgeType,
			"knowledge_value": knowledgeValue,
			"desc_text":       descText,
			"keywords":        kw,
		})
		seq := lastDelimitedToken(knowledgeID)
		if seq == "" {
			seq = strconv.FormatInt(id, 10)
		}
		out = append(out, kbsearch.RegistryRow{
			ArtifactType:    searchArtifactKnowledge,
			ArtifactID:      kbsearch.BuildArtifactID(recordID, searchArtifactKnowledge, seq),
			InputRecordID:   recordID,
			SourceRowID:     &id,
			PrimaryLabel:    firstNonEmpty(knowledgeValue, knowledgeID),
			SecondaryLabel:  knowledgeType,
			SearchDocument:  firstNonEmpty(searchDoc.String, strings.TrimSpace(strings.Join(searchParts, " "))),
			SnippetBasis:    firstNonEmpty(descText, knowledgeValue),
			CategoryPaths:   json.RawMessage(categoryPaths),
			SemanticPayload: payload,
		})
	}
	return out, rows.Err()
}

func buildEntityRegistryRows(ctx context.Context, db *sql.DB, recordID int64) ([]kbsearch.RegistryRow, error) {
	const q = `
SELECT id, entity_id, language, entity, entity_en, entity_type, entity_type_en,
       aliases, aliases_en, desc_text, desc_text_en, keywords, keywords_en,
       line_spans, search_document
FROM kb.entities
WHERE input_record_id = $1
ORDER BY id`
	rows, err := db.QueryContext(ctx, q, recordID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	weights := appconfig.GetEntitySearchWeightsConfig()
	out := make([]kbsearch.RegistryRow, 0, 16)
	for rows.Next() {
		var (
			id              int64
			entityID        sql.NullString
			language        sql.NullString
			entity          sql.NullString
			entityEn        sql.NullString
			entityType      sql.NullString
			entityTypeEn    sql.NullString
			aliases         []byte
			aliasesEn       []byte
			descText        sql.NullString
			descTextEn      sql.NullString
			keywords        []byte
			keywordsEn      []byte
			sourceLineSpans []byte
			searchDoc       sql.NullString
		)
		if err := rows.Scan(&id, &entityID, &language, &entity, &entityEn, &entityType, &entityTypeEn,
			&aliases, &aliasesEn, &descText, &descTextEn, &keywords, &keywordsEn,
			&sourceLineSpans, &searchDoc); err != nil {
			return nil, err
		}
		kw := rawJSONArrayStrings(keywords)
		kwEn := rawJSONArrayStrings(keywordsEn)
		aliasList := rawJSONArrayStrings(aliases)
		aliasEnList := rawJSONArrayStrings(aliasesEn)
		searchParts := []string{
			entity.String, entityEn.String,
			entityType.String, entityTypeEn.String,
			strings.Join(aliasList, " "), strings.Join(aliasEnList, " "),
			descText.String, descTextEn.String,
			strings.Join(kw, " "), strings.Join(kwEn, " "),
		}
		payload, _ := json.Marshal(map[string]any{
			"entity":      entity.String,
			"entity_type": entityType.String,
			"desc":        descText.String,
			"aliases":     aliasList,
			"keywords":    kw,
			"language":    language.String,
		})
		seq := lastDelimitedToken(entityID.String)
		if seq == "" {
			seq = strconv.FormatInt(id, 10)
		}
		spans := sourceLineSpans
		if len(spans) == 0 {
			spans = []byte("[]")
		}
		weightedSearchDoc := buildEntitySearchDocument(weights, entitySearchFields{
			Entity:       entity.String,
			EntityEn:     entityEn.String,
			EntityType:   entityType.String,
			EntityTypeEn: entityTypeEn.String,
			Aliases:      aliasList,
			AliasesEn:    aliasEnList,
			DescText:     descText.String,
			DescTextEn:   descTextEn.String,
			Keywords:     kw,
			KeywordsEn:   kwEn,
		})
		out = append(out, kbsearch.RegistryRow{
			ArtifactType:    searchArtifactEntity,
			ArtifactID:      kbsearch.BuildArtifactID(recordID, searchArtifactEntity, seq),
			InputRecordID:   recordID,
			SourceRowID:     &id,
			PrimaryLabel:    firstNonEmpty(entity.String, entityID.String),
			SecondaryLabel:  firstNonEmpty(entityType.String, entityTypeEn.String),
			SearchDocument:  firstNonEmpty(weightedSearchDoc, searchDoc.String, strings.TrimSpace(strings.Join(searchParts, " "))),
			SnippetBasis:    firstNonEmpty(descText.String, entity.String),
			CategoryPaths:   json.RawMessage("[]"),
			SourceLineSpans: json.RawMessage(spans),
			SemanticPayload: payload,
		})
	}
	return out, rows.Err()
}

func buildRelationRegistryRows(ctx context.Context, db *sql.DB, recordID int64) ([]kbsearch.RegistryRow, error) {
	const q = `
SELECT id, relation_id, language, subject, subject_en, predicate, predicate_en,
       object, object_en, desc_text, desc_text_en, keywords, keywords_en,
       line_spans, search_document
FROM kb.relations
WHERE input_record_id = $1
ORDER BY id`
	rows, err := db.QueryContext(ctx, q, recordID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	weights := appconfig.GetRelationSearchWeightsConfig()
	out := make([]kbsearch.RegistryRow, 0, 16)
	for rows.Next() {
		var (
			id              int64
			relationID      sql.NullString
			language        sql.NullString
			subject         sql.NullString
			subjectEn       sql.NullString
			predicate       sql.NullString
			predicateEn     sql.NullString
			object          sql.NullString
			objectEn        sql.NullString
			descText        sql.NullString
			descTextEn      sql.NullString
			keywords        []byte
			keywordsEn      []byte
			sourceLineSpans []byte
			searchDoc       sql.NullString
		)
		if err := rows.Scan(&id, &relationID, &language, &subject, &subjectEn, &predicate, &predicateEn,
			&object, &objectEn, &descText, &descTextEn, &keywords, &keywordsEn,
			&sourceLineSpans, &searchDoc); err != nil {
			return nil, err
		}
		kw := rawJSONArrayStrings(keywords)
		kwEn := rawJSONArrayStrings(keywordsEn)
		spo := strings.TrimSpace(strings.Join([]string{subject.String, predicate.String, object.String}, " "))
		searchParts := []string{
			subject.String, subjectEn.String,
			predicate.String, predicateEn.String,
			object.String, objectEn.String,
			descText.String, descTextEn.String,
			strings.Join(kw, " "), strings.Join(kwEn, " "),
		}
		payload, _ := json.Marshal(map[string]any{
			"subject":   subject.String,
			"predicate": predicate.String,
			"object":    object.String,
			"desc":      descText.String,
			"keywords":  kw,
			"language":  language.String,
		})
		seq := lastDelimitedToken(relationID.String)
		if seq == "" {
			seq = strconv.FormatInt(id, 10)
		}
		spans := sourceLineSpans
		if len(spans) == 0 {
			spans = []byte("[]")
		}
		weightedSearchDoc := buildRelationSearchDocument(weights, relationSearchFields{
			Subject:     subject.String,
			SubjectEn:   subjectEn.String,
			Predicate:   predicate.String,
			PredicateEn: predicateEn.String,
			Object:      object.String,
			ObjectEn:    objectEn.String,
			DescText:    descText.String,
			DescTextEn:  descTextEn.String,
			Keywords:    kw,
			KeywordsEn:  kwEn,
		})
		out = append(out, kbsearch.RegistryRow{
			ArtifactType:    searchArtifactRelation,
			ArtifactID:      kbsearch.BuildArtifactID(recordID, searchArtifactRelation, seq),
			InputRecordID:   recordID,
			SourceRowID:     &id,
			PrimaryLabel:    firstNonEmpty(spo, relationID.String),
			SecondaryLabel:  predicate.String,
			SearchDocument:  firstNonEmpty(weightedSearchDoc, searchDoc.String, strings.TrimSpace(strings.Join(searchParts, " "))),
			SnippetBasis:    firstNonEmpty(descText.String, spo),
			CategoryPaths:   json.RawMessage("[]"),
			SourceLineSpans: json.RawMessage(spans),
			SemanticPayload: payload,
		})
	}
	return out, rows.Err()
}

func buildProductRegistryRows(ctx context.Context, db *sql.DB, recordID int64) ([]kbsearch.RegistryRow, error) {
	const q = `
SELECT id, product_rel_id, product_name, product_type, relation_type, relation_summary, category_paths, evidence_lines, input_record_id, search_document
FROM kb.products
WHERE input_record_id = $1
ORDER BY id`
	rows, err := db.QueryContext(ctx, q, recordID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]kbsearch.RegistryRow, 0, 16)
	for rows.Next() {
		var (
			id            int64
			productRelID  string
			productName   string
			productType   string
			relationType  string
			relationSum   string
			categoryPaths []byte
			evidenceLines []byte
			inputID       int64
			searchDoc     string
		)
		if err := rows.Scan(&id, &productRelID, &productName, &productType, &relationType, &relationSum, &categoryPaths, &evidenceLines, &inputID, &searchDoc); err != nil {
			return nil, err
		}
		payload, _ := json.Marshal(map[string]any{
			"product_type":     productType,
			"relation_type":    relationType,
			"relation_summary": relationSum,
		})
		seq := lastDelimitedToken(productRelID)
		if seq == "" {
			seq = strconv.FormatInt(id, 10)
		}
		out = append(out, kbsearch.RegistryRow{
			ArtifactType:    searchArtifactProduct,
			ArtifactID:      kbsearch.BuildArtifactID(inputID, searchArtifactProduct, seq),
			InputRecordID:   inputID,
			SourceRowID:     &id,
			PrimaryLabel:    firstNonEmpty(productName, productRelID),
			SecondaryLabel:  productType,
			SearchDocument:  firstNonEmpty(searchDoc, strings.Join([]string{productName, productType, relationSum}, " ")),
			SnippetBasis:    firstNonEmpty(relationSum, productName),
			CategoryPaths:   json.RawMessage(categoryPaths),
			SourceLineSpans: json.RawMessage(evidenceLines),
			SemanticPayload: payload,
		})
	}
	return out, rows.Err()
}

type summaryArtifactRecord struct {
	SummaryID     string
	Level         int
	SeqNo         int
	Keywords      []string
	KeywordsEn    []string
	CategoryPaths []string
	SummaryText   string
	SummaryTextEn string
	Lines         []string
	Children      []string
	Language      string
}

func parseSummaryArtifactFile(path string) (summaryArtifactRecord, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return summaryArtifactRecord{}, err
	}
	out := summaryArtifactRecord{}
	scanner := bufio.NewScanner(strings.NewReader(string(body)))
	scanner.Buffer(make([]byte, 1024), 8*1024*1024)
	var summaryLines []string
	var summaryEnLines []string
	inSummary := false
	inSummaryEn := false
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "summary_begin" || trimmed == "summary_begin:" {
			inSummary = true
			continue
		}
		if trimmed == "summary_end" {
			inSummary = false
			continue
		}
		if trimmed == "summary_en_begin" {
			inSummaryEn = true
			continue
		}
		if trimmed == "summary_en_end" {
			inSummaryEn = false
			continue
		}
		if inSummary {
			summaryLines = append(summaryLines, line)
			continue
		}
		if inSummaryEn {
			summaryEnLines = append(summaryEnLines, line)
			continue
		}
		switch {
		case strings.HasPrefix(trimmed, "summary_id:"):
			out.SummaryID = strings.Trim(strings.TrimSpace(strings.TrimPrefix(trimmed, "summary_id:")), `"`)
		case strings.HasPrefix(trimmed, "level:"):
			out.Level, _ = strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(trimmed, "level:")))
		case strings.HasPrefix(trimmed, "lines:"):
			out.Lines = parseQuotedStringArraySearch(strings.TrimSpace(strings.TrimPrefix(trimmed, "lines:")))
		case strings.HasPrefix(trimmed, "children:"):
			out.Children = parseQuotedStringArraySearch(strings.TrimSpace(strings.TrimPrefix(trimmed, "children:")))
		case strings.HasPrefix(trimmed, "language:"):
			out.Language = strings.Trim(strings.TrimSpace(strings.TrimPrefix(trimmed, "language:")), `"`)
		case strings.HasPrefix(trimmed, "keywords_en:"):
			out.KeywordsEn = parseQuotedStringArraySearch(strings.TrimSpace(strings.TrimPrefix(trimmed, "keywords_en:")))
		case strings.HasPrefix(trimmed, "keywords:"):
			out.Keywords = parseQuotedStringArraySearch(strings.TrimSpace(strings.TrimPrefix(trimmed, "keywords:")))
		case strings.HasPrefix(trimmed, "category_paths:"):
			out.CategoryPaths = parseQuotedStringArraySearch(strings.TrimSpace(strings.TrimPrefix(trimmed, "category_paths:")))
		}
	}
	out.SummaryText = strings.TrimSpace(strings.Join(summaryLines, "\n"))
	out.SummaryTextEn = strings.TrimSpace(strings.Join(summaryEnLines, "\n"))
	_, level, seqNo, ok := parseSummaryID(out.SummaryID)
	if ok {
		out.Level = level
		out.SeqNo = seqNo
	}
	return out, scanner.Err()
}

type topicArtifactRecord struct {
	TopicID         string
	TopicType       string
	TopicDesc       string
	TopicDescEn     string
	Keywords        []string
	KeywordsEn      []string
	CategoryPaths   []string
	CategoryPathsEn []string
	Lines           []string
}

func parseTopicArtifactFile(path string) ([]topicArtifactRecord, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []topicArtifactRecord
	var cur *topicArtifactRecord
	scanner := bufio.NewScanner(strings.NewReader(string(body)))
	scanner.Buffer(make([]byte, 1024), 8*1024*1024)
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if cur != nil {
				out = append(out, *cur)
				cur = nil
			}
			continue
		}
		if cur == nil {
			cur = &topicArtifactRecord{}
		}
		switch {
		case strings.HasPrefix(trimmed, "topic_id:"):
			cur.TopicID = strings.Trim(strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(trimmed, "topic_id:")), ","), `"`)
		case strings.HasPrefix(trimmed, "topic_type:"):
			cur.TopicType = strings.Trim(strings.TrimSpace(strings.TrimPrefix(trimmed, "topic_type:")), `"`)
		case strings.HasPrefix(trimmed, "lines:"):
			cur.Lines = parseQuotedStringArraySearch(strings.TrimSpace(strings.TrimPrefix(trimmed, "lines:")))
		case strings.HasPrefix(trimmed, "topic_keywords_en:"):
			cur.KeywordsEn = parseQuotedStringArraySearch(strings.TrimSpace(strings.TrimPrefix(trimmed, "topic_keywords_en:")))
		case strings.HasPrefix(trimmed, "topic_keywords:"):
			cur.Keywords = parseQuotedStringArraySearch(strings.TrimSpace(strings.TrimPrefix(trimmed, "topic_keywords:")))
		case strings.HasPrefix(trimmed, "category_paths_en:"):
			cur.CategoryPathsEn = parseQuotedStringArraySearch(strings.TrimSpace(strings.TrimPrefix(trimmed, "category_paths_en:")))
		case strings.HasPrefix(trimmed, "category_paths:"):
			cur.CategoryPaths = parseQuotedStringArraySearch(strings.TrimSpace(strings.TrimPrefix(trimmed, "category_paths:")))
		case strings.HasPrefix(trimmed, "topic_desc_en:"):
			cur.TopicDescEn = strings.Trim(strings.TrimSpace(strings.TrimPrefix(trimmed, "topic_desc_en:")), `"`)
		case strings.HasPrefix(trimmed, "topic_desc:"):
			cur.TopicDesc = strings.Trim(strings.TrimSpace(strings.TrimPrefix(trimmed, "topic_desc:")), `"`)
		}
	}
	if cur != nil {
		out = append(out, *cur)
	}
	return out, scanner.Err()
}

func parseQuotedStringArraySearch(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" {
		return []string{}
	}
	if strings.HasPrefix(raw, "[") && strings.HasSuffix(raw, "]") {
		var parsed []string
		if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
			return trimStringSlice(parsed)
		}
	}
	return []string{}
}

func trimStringSlice(in []string) []string {
	out := make([]string, 0, len(in))
	for _, item := range in {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		out = append(out, item)
	}
	return out
}

func parseSummaryID(summaryID string) (int64, int, int, bool) {
	parts := strings.Split(strings.TrimSpace(summaryID), "_")
	if len(parts) != 4 || parts[1] != "sum" {
		return 0, 0, 0, false
	}
	recordID, err1 := strconv.ParseInt(parts[0], 10, 64)
	level, err2 := strconv.Atoi(parts[2])
	seqNo, err3 := strconv.Atoi(parts[3])
	return recordID, level, seqNo, err1 == nil && err2 == nil && err3 == nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func mustJSON(value any, fallback any) json.RawMessage {
	target := value
	if target == nil {
		target = fallback
	}
	bs, err := json.Marshal(target)
	if err != nil {
		bs, _ = json.Marshal(fallback)
	}
	return json.RawMessage(bs)
}

func rawJSONArrayStrings(raw []byte) []string {
	var out []string
	_ = json.Unmarshal(raw, &out)
	return out
}

func lastDelimitedToken(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parts := strings.Split(raw, "_")
	return strings.TrimSpace(parts[len(parts)-1])
}
