package docprocessing

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/kbsearch"
	"github.com/chendingplano/shared/go/api/ApiTypes"
)

const (
	searchArtifactSummary    = "summary"
	searchArtifactTopic      = "topic"
	searchArtifactSceneBlock = "scene_block"
	searchArtifactMetric     = "metric"
	searchArtifactProvision  = "provision"
	searchArtifactProduct    = "product"
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
		rows = append(rows, kbsearch.RegistryRow{
			ArtifactType:    searchArtifactSummary,
			ArtifactID:      kbsearch.BuildArtifactID(recordID, searchArtifactSummary, strconv.Itoa(item.SeqNo)),
			InputRecordID:   recordID,
			PrimaryLabel:    firstNonEmpty(item.SummaryText, item.SummaryID),
			SecondaryLabel:  fmt.Sprintf("Level %d", item.Level),
			SearchDocument:  strings.TrimSpace(strings.Join([]string{item.SummaryText, strings.Join(item.Keywords, " "), strings.Join(item.CategoryPaths, " ")}, " ")),
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
			rows = append(rows, kbsearch.RegistryRow{
				ArtifactType:    searchArtifactTopic,
				ArtifactID:      kbsearch.BuildArtifactID(recordID, searchArtifactTopic, item.TopicID),
				InputRecordID:   recordID,
				PrimaryLabel:    firstNonEmpty(item.TopicDesc, item.TopicID),
				SecondaryLabel:  item.TopicType,
				SearchDocument:  strings.TrimSpace(strings.Join([]string{item.TopicType, item.TopicDesc, item.TopicDescEn, strings.Join(item.Keywords, " "), strings.Join(item.KeywordsEn, " "), strings.Join(item.CategoryPaths, " "), strings.Join(item.CategoryPathsEn, " ")}, " ")),
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
SELECT id, object_id, title, scene_type, summary, keywords, source_refs, search_document
FROM kb.scene_objects
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
			id         int64
			objectID   string
			title      string
			sceneType  string
			summary    string
			keywords   []byte
			sourceRefs []byte
			searchDoc  string
		)
		if err := rows.Scan(&id, &objectID, &title, &sceneType, &summary, &keywords, &sourceRefs, &searchDoc); err != nil {
			return nil, err
		}
		payload, _ := json.Marshal(map[string]any{
			"scene_type": sceneType,
			"summary":    summary,
			"keywords":   rawJSONArrayStrings(keywords),
		})
		seq := lastDelimitedToken(objectID)
		if seq == "" {
			seq = strconv.FormatInt(id, 10)
		}
		out = append(out, kbsearch.RegistryRow{
			ArtifactType:    searchArtifactSceneBlock,
			ArtifactID:      kbsearch.BuildArtifactID(recordID, searchArtifactSceneBlock, seq),
			InputRecordID:   recordID,
			SourceRowID:     &id,
			PrimaryLabel:    firstNonEmpty(title, objectID),
			SecondaryLabel:  sceneType,
			SearchDocument:  firstNonEmpty(searchDoc, strings.Join([]string{title, summary}, " ")),
			SnippetBasis:    firstNonEmpty(summary, title),
			SourceLineSpans: json.RawMessage(sourceRefs),
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
			SearchDocument:  firstNonEmpty(searchDoc, strings.Join([]string{provName, provDesc, provisionType}, " ")),
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
	CategoryPaths []string
	SummaryText   string
	Lines         []string
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
	inSummary := false
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
		if inSummary {
			summaryLines = append(summaryLines, line)
			continue
		}
		switch {
		case strings.HasPrefix(trimmed, "summary_id:"):
			out.SummaryID = strings.Trim(strings.TrimSpace(strings.TrimPrefix(trimmed, "summary_id:")), `"`)
		case strings.HasPrefix(trimmed, "level:"):
			out.Level, _ = strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(trimmed, "level:")))
		case strings.HasPrefix(trimmed, "lines:"):
			out.Lines = parseQuotedStringArraySearch(strings.TrimSpace(strings.TrimPrefix(trimmed, "lines:")))
		case strings.HasPrefix(trimmed, "keywords:"):
			out.Keywords = parseQuotedStringArraySearch(strings.TrimSpace(strings.TrimPrefix(trimmed, "keywords:")))
		case strings.HasPrefix(trimmed, "category_paths:"):
			out.CategoryPaths = parseQuotedStringArraySearch(strings.TrimSpace(strings.TrimPrefix(trimmed, "category_paths:")))
		}
	}
	out.SummaryText = strings.TrimSpace(strings.Join(summaryLines, "\n"))
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
	if len(parts) != 3 {
		return 0, 0, 0, false
	}
	recordID, err1 := strconv.ParseInt(parts[0], 10, 64)
	level, err2 := strconv.Atoi(parts[1])
	seqNo, err3 := strconv.Atoi(parts[2])
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
