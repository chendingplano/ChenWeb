package kbhandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

type artifactWikiPayloadLoader func(context.Context, *sql.DB, ApiTypes.JimoLogger, string, string) (artifactWikiPayload, error)
type artifactWikiArticleBuilder func(context.Context, *sql.DB, ApiTypes.JimoLogger, string, string) (json.RawMessage, error)

type artifactWikiProvider struct {
	loadPayload  artifactWikiPayloadLoader
	buildArticle artifactWikiArticleBuilder
}

var artifactWikiProviders = map[string]artifactWikiProvider{
	"metric": {
		loadPayload: func(ctx context.Context, db *sql.DB, logger ApiTypes.JimoLogger, artifactID, lang string) (artifactWikiPayload, error) {
			return getArtifactWikiMetricPayloadFn(ctx, db, logger, artifactID, lang)
		},
		buildArticle: func(ctx context.Context, db *sql.DB, logger ApiTypes.JimoLogger, artifactID, lang string) (json.RawMessage, error) {
			return buildArtifactWikiMetricArticleFn(ctx, db, logger, artifactID, lang)
		},
	},
	"summary": {
		loadPayload:  buildArtifactWikiSummaryPayload,
		buildArticle: generateGenericArtifactWikiArticle,
	},
	"topic": {
		loadPayload:  buildArtifactWikiTopicPayload,
		buildArticle: generateGenericArtifactWikiArticle,
	},
	"semantic_projection": {
		loadPayload:  buildArtifactWikiSemanticProjectionPayload,
		buildArticle: generateGenericArtifactWikiArticle,
	},
	"scene_block": {
		loadPayload:  buildArtifactWikiSceneBlockPayload,
		buildArticle: generateGenericArtifactWikiArticle,
	},
	"product": {
		loadPayload:  buildArtifactWikiProductPayload,
		buildArticle: generateGenericArtifactWikiArticle,
	},
	"inventory_item": {
		loadPayload:  buildArtifactWikiInventoryItemPayload,
		buildArticle: generateGenericArtifactWikiArticle,
	},
	"provision": {
		loadPayload:  buildArtifactWikiProvisionPayload,
		buildArticle: generateGenericArtifactWikiArticle,
	},
	"entity": {
		loadPayload:  buildArtifactWikiEntityPayload,
		buildArticle: generateGenericArtifactWikiArticle,
	},
	"relation": {
		loadPayload:  buildArtifactWikiRelationPayload,
		buildArticle: generateGenericArtifactWikiArticle,
	},
	"knowledge": {
		loadPayload:  buildArtifactWikiKnowledgePayload,
		buildArticle: generateGenericArtifactWikiArticle,
	},
}

type genericArtifactWikiPage struct {
	Title          string                    `json:"title"`
	Lead           string                    `json:"lead"`
	Definition     string                    `json:"definition,omitempty"`
	Background     string                    `json:"background,omitempty"`
	HowUsed        string                    `json:"how_used,omitempty"`
	ChoosingValues string                    `json:"choosing_values,omitempty"`
	Generated      artifactWikiGeneratedMeta `json:"generated"`
}

type genericArtifactWikiContext struct {
	ArtifactType   string          `json:"artifact_type"`
	ArtifactID     string          `json:"artifact_id"`
	Lang           string          `json:"lang"`
	Record         json.RawMessage `json:"record"`
	SourceDocument json.RawMessage `json:"source_document,omitempty"`
}

func getArtifactWikiProvider(artifactType string) (artifactWikiProvider, bool) {
	provider, ok := artifactWikiProviders[strings.TrimSpace(artifactType)]
	return provider, ok
}

func buildArtifactWikiSummaryPayload(ctx context.Context, db *sql.DB, logger ApiTypes.JimoLogger, artifactID, lang string) (artifactWikiPayload, error) {
	recordID, seqNo, err := parseArtifactSearchIDSeq(artifactID, "summary")
	if err != nil {
		return artifactWikiPayload{}, err
	}
	artifactDir := strings.TrimSpace(os.Getenv("ARTIFACT_DIR"))
	if artifactDir == "" {
		return artifactWikiPayload{}, fmt.Errorf("missing ARTIFACT_DIR")
	}
	recordDir, err := resolveRecordArtifactDir(artifactDir, recordID)
	if err != nil {
		return artifactWikiPayload{}, err
	}
	summaryPath, err := findSummaryArtifactPath(recordDir, seqNo)
	if err != nil {
		return artifactWikiPayload{}, err
	}
	parsed, err := readSummaryArtifactFile(summaryPath)
	if err != nil {
		return artifactWikiPayload{}, err
	}
	record := map[string]any{
		"summary_id":     artifactID,
		"record_id":      recordID,
		"level":          parsed.level,
		"keywords":       parsed.keywords,
		"category_paths": parsed.categoryPaths,
		"summary_text":   parsed.summaryText,
		"source_lines":   parsed.lines,
		"source_summary": parsed.summaryID,
	}
	return buildArtifactWikiPayloadFromRecord(db, recordID, artifactID, lang, record)
}

func buildArtifactWikiTopicPayload(ctx context.Context, db *sql.DB, logger ApiTypes.JimoLogger, artifactID, lang string) (artifactWikiPayload, error) {
	recordID, seqNo, err := parseArtifactSearchIDSeq(artifactID, "topic")
	if err != nil {
		return artifactWikiPayload{}, err
	}
	artifactDir := strings.TrimSpace(os.Getenv("ARTIFACT_DIR"))
	if artifactDir == "" {
		return artifactWikiPayload{}, fmt.Errorf("missing ARTIFACT_DIR")
	}
	inputTable, err := resolveInputTable(db)
	if err != nil {
		return artifactWikiPayload{}, err
	}
	stagingExpr, err := resolveStagingOrNameExpr(db, inputTable)
	if err != nil {
		return artifactWikiPayload{}, err
	}
	parserExpr, err := resolveParserNameExpr(db, inputTable)
	if err != nil {
		return artifactWikiPayload{}, err
	}
	meta, err := fetchSummaryArtifactMeta(db, inputTable, stagingExpr, parserExpr, recordID)
	if err != nil {
		return artifactWikiPayload{}, err
	}
	topic, err := readTopicFromArtifact(artifactDir, recordID, meta, seqNo)
	if err != nil {
		return artifactWikiPayload{}, err
	}
	record := map[string]any{
		"topic_id":          artifactID,
		"topic_type":        topic.topicType,
		"topic_text":        topic.topicText,
		"topic_desc_en":     topic.topicDescEn,
		"keywords":          topic.keywords,
		"keywords_en":       topic.keywordsEn,
		"category_paths":    topic.categoryPaths,
		"category_paths_en": topic.categoryPathsEn,
		"source_lines":      topic.lines,
	}
	return buildArtifactWikiPayloadFromRecord(db, recordID, artifactID, lang, record)
}

func buildArtifactWikiSemanticProjectionPayload(ctx context.Context, db *sql.DB, logger ApiTypes.JimoLogger, artifactID, lang string) (artifactWikiPayload, error) {
	record, err := fetchSemanticProjectionByID(db, artifactID)
	if err != nil {
		return artifactWikiPayload{}, err
	}
	return buildArtifactWikiPayloadFromTypedRecord(db, record.InputRecordID, artifactID, lang, record)
}

func buildArtifactWikiSceneBlockPayload(ctx context.Context, db *sql.DB, logger ApiTypes.JimoLogger, artifactID, lang string) (artifactWikiPayload, error) {
	record, recordID, err := fetchSceneBlockByArtifactID(db, artifactID)
	if err != nil {
		return artifactWikiPayload{}, err
	}
	return buildArtifactWikiPayloadFromTypedRecord(db, recordID, artifactID, lang, record)
}

func buildArtifactWikiProductPayload(ctx context.Context, db *sql.DB, logger ApiTypes.JimoLogger, artifactID, lang string) (artifactWikiPayload, error) {
	record, inputID, err := fetchProductByArtifactID(db, artifactID)
	if err != nil {
		return artifactWikiPayload{}, err
	}
	return buildArtifactWikiPayloadFromTypedRecord(db, inputID, artifactID, lang, record)
}

func buildArtifactWikiInventoryItemPayload(ctx context.Context, db *sql.DB, logger ApiTypes.JimoLogger, artifactID, lang string) (artifactWikiPayload, error) {
	record, inputID, err := fetchInventoryItemBySearchID(db, artifactID)
	if err != nil {
		return artifactWikiPayload{}, err
	}
	return buildArtifactWikiPayloadFromTypedRecord(db, inputID, artifactID, lang, record)
}

func buildArtifactWikiProvisionPayload(ctx context.Context, db *sql.DB, logger ApiTypes.JimoLogger, artifactID, lang string) (artifactWikiPayload, error) {
	record, err := fetchProvisionByArtifactID(db, artifactID)
	if err != nil {
		return artifactWikiPayload{}, err
	}
	return buildArtifactWikiPayloadFromTypedRecord(db, record.InputRecordID, artifactID, lang, record)
}

func buildArtifactWikiEntityPayload(ctx context.Context, db *sql.DB, logger ApiTypes.JimoLogger, artifactID, lang string) (artifactWikiPayload, error) {
	record, inputID, err := fetchEntityByArtifactID(db, artifactID)
	if err != nil {
		return artifactWikiPayload{}, err
	}
	return buildArtifactWikiPayloadFromTypedRecord(db, inputID, artifactID, lang, record)
}

func buildArtifactWikiRelationPayload(ctx context.Context, db *sql.DB, logger ApiTypes.JimoLogger, artifactID, lang string) (artifactWikiPayload, error) {
	record, inputID, err := fetchRelationByArtifactID(db, artifactID)
	if err != nil {
		return artifactWikiPayload{}, err
	}
	return buildArtifactWikiPayloadFromTypedRecord(db, inputID, artifactID, lang, record)
}

func buildArtifactWikiKnowledgePayload(ctx context.Context, db *sql.DB, logger ApiTypes.JimoLogger, artifactID, lang string) (artifactWikiPayload, error) {
	record, inputID, err := fetchKnowledgeByArtifactID(db, artifactID)
	if err != nil {
		return artifactWikiPayload{}, err
	}
	return buildArtifactWikiPayloadFromTypedRecord(db, inputID, artifactID, lang, record)
}

func buildArtifactWikiPayloadFromTypedRecord(db *sql.DB, recordID int64, artifactID, lang string, record any) (artifactWikiPayload, error) {
	recordJSON, err := json.Marshal(record)
	if err != nil {
		return artifactWikiPayload{}, err
	}
	docMeta, err := fetchWikiDocMeta(db, recordID)
	if err != nil {
		docMeta = metricWikiDocMeta{RecordID: recordID}
	}
	docJSON, err := json.Marshal(docMeta)
	if err != nil {
		return artifactWikiPayload{}, err
	}
	return artifactWikiPayload{
		RecordID:       recordID,
		ArtifactID:     artifactID,
		Record:         json.RawMessage(recordJSON),
		SourceDocument: json.RawMessage(docJSON),
		Generated: artifactWikiGeneratedMeta{
			Lang:       normalizeArtifactWikiLang(lang),
			SourceHash: metricWikiSourceHash(metricWikiContext{MetricID: artifactID, RecordID: recordID, Document: docMeta}),
		},
	}, nil
}

func buildArtifactWikiPayloadFromRecord(db *sql.DB, recordID int64, artifactID, lang string, record map[string]any) (artifactWikiPayload, error) {
	return buildArtifactWikiPayloadFromTypedRecord(db, recordID, artifactID, lang, record)
}

func parseArtifactRecordIDPrefix(raw, sep string) (int64, error) {
	parts := strings.SplitN(strings.TrimSpace(raw), sep, 2)
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid artifact id %q", raw)
	}
	return strconv.ParseInt(parts[0], 10, 64)
}

func parseArtifactTwoPartID(raw, sep string) (int64, int, error) {
	parts := strings.Split(strings.TrimSpace(raw), sep)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid artifact id %q", raw)
	}
	recordID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, 0, err
	}
	seqNo, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, err
	}
	return recordID, seqNo, nil
}

func findSummaryArtifactPath(recordDir string, seqNo int) (string, error) {
	paths, err := filepath.Glob(filepath.Join(recordDir, "summary_*_*.txt"))
	if err != nil {
		return "", err
	}
	for _, path := range paths {
		parsed, readErr := readSummaryArtifactFile(path)
		if readErr != nil {
			continue
		}
		if parsed.seqNo == seqNo {
			return path, nil
		}
	}
	return "", fmt.Errorf("summary artifact for seq %d not found", seqNo)
}
