package kbhandler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

type provisionGraphNode struct {
	ID            string             `json:"id"`
	CategoryPath  string             `json:"categoryPath"`
	Label         string             `json:"label"`
	Metadata      topicGraphMetadata `json:"metadata"`
	ChildIDs      []string           `json:"childIds"`
	TopicIDs      []string           `json:"topicIds"`
	HasTopicsFile bool               `json:"hasTopicsFile"`
	Expanded      bool               `json:"expanded"`
}

type listProvisionGraphResponse struct {
	Status  bool                 `json:"status"`
	Results []provisionGraphNode `json:"results"`
}

type getProvisionCategoryResponse struct {
	Status       bool                  `json:"status"`
	CategoryPath string                `json:"categoryPath"`
	Topics       []topicCategoryRecord `json:"topics"`
}

type getRecordProvisionsResponse struct {
	Status   bool                  `json:"status"`
	RecordID int64                 `json:"recordId"`
	Topics   []topicCategoryRecord `json:"topics"`
}

type provisionRow struct {
	ItemID          string
	InputRecordID   int64
	ProvID          int
	ProvName        string
	ProvisionType   string
	ProvisionText   string
	Keywords        []string
	CategoryChains  []provisionCategoryChain
	CategoryPaths   []string
	SourceLineSpans []string
	Confidence      float64
}

type provisionCategorySegment struct {
	Name       string   `json:"name"`
	Keywords   []string `json:"keywords"`
	Confidence float64  `json:"confidence"`
}

type provisionCategoryPathPayload struct {
	CategoryPath   []provisionCategorySegment `json:"category_path"`
	PathConfidence float64                    `json:"path_confidence"`
	PathKeywords   []string                   `json:"path_keywords"`
}

type provisionCategoryChain struct {
	Path           string
	Segments       []provisionCategorySegment
	PathConfidence float64
	PathKeywords   []string
}

func ListProvisionGraph(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_PG_001")
	defer rc.Close()
	logger := rc.GetLogger()

	ksStoreID, err := parseOptionalPositiveInt64(c.QueryParam("ks_store_id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: fmt.Sprintf("invalid ks_store_id: %v (CWB_KB_PG_010)", err),
		})
	}

	rows, err := queryProvisionRows(nil, ksStoreID)
	if err != nil {
		logger.Error("query provision graph failed", "ks_store_id", optionalInt64Value(ksStoreID), "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to read provision graph (CWB_KB_PG_011)",
		})
	}
	logProvisionRows(logger, "provision_graph", rows)
	nodes := buildProvisionGraphNodes(rows)
	logProvisionGraphNodes(logger, nodes)

	return c.JSON(http.StatusOK, listProvisionGraphResponse{
		Status:  true,
		Results: nodes,
	})
}

func GetProvisionCategory(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_PCAT_001")
	defer rc.Close()
	logger := rc.GetLogger()

	categoryPath := strings.TrimSpace(c.QueryParam("category_path"))
	if categoryPath == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "missing category_path (CWB_KB_PCAT_010)",
		})
	}
	ksStoreID, err := parseOptionalPositiveInt64(c.QueryParam("ks_store_id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: fmt.Sprintf("invalid ks_store_id: %v (CWB_KB_PCAT_011)", err),
		})
	}

	results, err := readProvisionCategoryRecords(categoryPath, ksStoreID)
	if err != nil {
		logger.Error("read provision category failed", "category_path", categoryPath, "ks_store_id", optionalInt64Value(ksStoreID), "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to read provision category (CWB_KB_PCAT_012)",
		})
	}
	logProvisionCards(logger, "provision_category", results, "category_path", categoryPath, "ks_store_id", optionalInt64Value(ksStoreID))

	return c.JSON(http.StatusOK, getProvisionCategoryResponse{
		Status:       true,
		CategoryPath: categoryPath,
		Topics:       results,
	})
}

func GetRecordProvisions(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_PREC_001")
	defer rc.Close()
	logger := rc.GetLogger()

	recordID, err := parseOptionalPositiveInt64(c.QueryParam("record_id"))
	if err != nil || recordID == nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "missing or invalid record_id (CWB_KB_PREC_010)",
		})
	}

	results, err := readRecordProvisionCards(*recordID)
	if err != nil {
		logger.Error("read record provisions failed", "record_id", *recordID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to read record provisions (CWB_KB_PREC_011)",
		})
	}
	logProvisionCards(logger, "record_provisions", results, "record_id", *recordID)

	return c.JSON(http.StatusOK, getRecordProvisionsResponse{
		Status:   true,
		RecordID: *recordID,
		Topics:   results,
	})
}

func logProvisionRows(logger ApiTypes.JimoLogger, source string, rows []provisionRow) {
	logger.Info("provision rows retrieved", "source", source, "count", len(rows))
	for _, row := range rows {
		logger.Info(
			"provision row retrieved",
			"source", source,
			"item_id", row.ItemID,
			"record_id", row.InputRecordID,
			"prov_id", row.ProvID,
			"prov_name", row.ProvName,
			"provision_type", row.ProvisionType,
			"confidence", row.Confidence,
			"category_paths", row.CategoryPaths,
			"category_chain_count", len(row.CategoryChains),
			"keyword_count", len(row.Keywords),
			"source_line_spans", row.SourceLineSpans,
		)
	}
}

func logProvisionGraphNodes(logger ApiTypes.JimoLogger, nodes []provisionGraphNode) {
	logger.Info("provision graph nodes built", "node_count", len(nodes))
	for _, node := range nodes {
		logger.Info(
			"provision graph node built",
			"node_id", node.ID,
			"category_path", node.CategoryPath,
			"label", node.Label,
			"child_count", len(node.ChildIDs),
			"children", node.ChildIDs,
			"provision_count", len(node.TopicIDs),
			"provision_ids", node.TopicIDs,
			"confidence", node.Metadata.Confidence,
		)
	}
}

func logProvisionCards(logger ApiTypes.JimoLogger, source string, cards []topicCategoryRecord, attrs ...any) {
	args := append([]any{"source", source, "count", len(cards)}, attrs...)
	logger.Info("provision cards returned", args...)
	for _, card := range cards {
		cardArgs := append([]any{
			"source", source,
			"id", card.ID,
			"input_id", card.InputID,
			"pdf_file_name", card.PdfFileName,
			"provision_type", card.TopicType,
			"page", card.Page,
			"target_count", len(card.Targets),
			"keyword_count", len(card.Keywords),
		}, attrs...)
		logger.Info("provision card returned", cardArgs...)
	}
}

func buildProvisionGraphNodes(rows []provisionRow) []provisionGraphNode {
	type aggregate struct {
		node       provisionGraphNode
		keywordSet map[string]struct{}
	}

	nodes := map[string]*aggregate{}
	ensureNode := func(path string) *aggregate {
		if existing, ok := nodes[path]; ok {
			return existing
		}
		label := path
		if idx := strings.LastIndex(path, "/"); idx >= 0 {
			label = path[idx+1:]
		}
		if label == "" {
			label = path
		}
		next := &aggregate{
			node: provisionGraphNode{
				ID:            path,
				CategoryPath:  path,
				Label:         label,
				Metadata:      topicGraphMetadata{Keywords: []string{}},
				ChildIDs:      []string{},
				TopicIDs:      []string{},
				HasTopicsFile: false,
				Expanded:      false,
			},
			keywordSet: map[string]struct{}{},
		}
		nodes[path] = next
		return next
	}

	for _, row := range rows {
		itemID := row.ItemID
		for _, chain := range row.CategoryChains {
			path := strings.Trim(strings.TrimSpace(chain.Path), "/")
			if path == "" || len(chain.Segments) == 0 {
				continue
			}
			for i, segment := range chain.Segments {
				name := strings.TrimSpace(segment.Name)
				if name == "" {
					continue
				}
				currPath := strings.Join(categorySegmentNames(chain.Segments[:i+1]), "/")
				curr := ensureNode(currPath)
				if i > 0 {
					parentPath := strings.Join(categorySegmentNames(chain.Segments[:i]), "/")
					parent := ensureNode(parentPath)
					parent.node.ChildIDs = appendIfMissing(parent.node.ChildIDs, currPath)
				}
				curr.node.Label = name
				for _, kw := range segment.Keywords {
					kw = strings.TrimSpace(kw)
					if kw == "" {
						continue
					}
					if _, ok := curr.keywordSet[kw]; ok {
						continue
					}
					curr.keywordSet[kw] = struct{}{}
					curr.node.Metadata.Keywords = append(curr.node.Metadata.Keywords, kw)
				}
				if segment.Confidence > curr.node.Metadata.Confidence {
					curr.node.Metadata.Confidence = segment.Confidence
				}
				if curr.node.Metadata.Desc == "" {
					curr.node.Metadata.Desc = fmt.Sprintf("Provision category %s", name)
				}
				curr.node.TopicIDs = appendIfMissing(curr.node.TopicIDs, itemID)
				curr.node.HasTopicsFile = true
				if i == len(chain.Segments)-1 {
					if chain.PathConfidence > curr.node.Metadata.Confidence {
						curr.node.Metadata.Confidence = chain.PathConfidence
					}
					if row.Confidence > curr.node.Metadata.Confidence {
						curr.node.Metadata.Confidence = row.Confidence
					}
					for _, kw := range chain.PathKeywords {
						kw = strings.TrimSpace(kw)
						if kw == "" {
							continue
						}
						if _, ok := curr.keywordSet[kw]; ok {
							continue
						}
						curr.keywordSet[kw] = struct{}{}
						curr.node.Metadata.Keywords = append(curr.node.Metadata.Keywords, kw)
					}
					for _, kw := range row.Keywords {
						kw = strings.TrimSpace(kw)
						if kw == "" {
							continue
						}
						if _, ok := curr.keywordSet[kw]; ok {
							continue
						}
						curr.keywordSet[kw] = struct{}{}
						curr.node.Metadata.Keywords = append(curr.node.Metadata.Keywords, kw)
					}
				}
			}
		}
	}

	results := make([]provisionGraphNode, 0, len(nodes))
	for _, agg := range nodes {
		sort.Strings(agg.node.ChildIDs)
		sort.Strings(agg.node.TopicIDs)
		sort.Strings(agg.node.Metadata.Keywords)
		results = append(results, agg.node)
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].CategoryPath < results[j].CategoryPath
	})
	return results
}

func readProvisionCategoryRecords(categoryPath string, ksStoreID *int64) ([]topicCategoryRecord, error) {
	rows, err := queryProvisionRows(nil, ksStoreID)
	if err != nil {
		return nil, err
	}
	filtered := make([]provisionRow, 0)
	for _, row := range rows {
		for _, path := range row.CategoryPaths {
			selectedPath := strings.Trim(strings.TrimSpace(categoryPath), "/")
			candidatePath := strings.Trim(strings.TrimSpace(path), "/")
			if candidatePath == selectedPath || strings.HasPrefix(candidatePath, selectedPath+"/") {
				filtered = append(filtered, row)
				break
			}
		}
	}
	return buildProvisionTopicCards(filtered)
}

func readRecordProvisionCards(recordID int64) ([]topicCategoryRecord, error) {
	rows, err := queryProvisionRows(&recordID, nil)
	if err != nil {
		return nil, err
	}
	return buildProvisionTopicCards(rows)
}

func buildProvisionTopicCards(rows []provisionRow) ([]topicCategoryRecord, error) {
	if len(rows) == 0 {
		return []topicCategoryRecord{}, nil
	}

	db := ApiTypes.ProjectDBHandle
	inputTable, err := resolveInputTable(db)
	if err != nil {
		return nil, err
	}
	stagingExpr, err := resolveStagingOrNameExpr(db, inputTable)
	if err != nil {
		return nil, err
	}
	parserExpr, err := resolveParserNameExpr(db, inputTable)
	if err != nil {
		return nil, err
	}

	artifactDir := strings.TrimSpace(os.Getenv("ARTIFACT_DIR"))
	if artifactDir == "" {
		return nil, fmt.Errorf("missing ARTIFACT_DIR")
	}

	metaCache := map[int64]summaryArtifactMeta{}
	lineTargetCache := map[int64]map[int]summaryLineTarget{}
	results := make([]topicCategoryRecord, 0, len(rows))
	for _, row := range rows {
		meta, ok := metaCache[row.InputRecordID]
		if !ok {
			meta, err = fetchSummaryArtifactMeta(db, inputTable, stagingExpr, parserExpr, row.InputRecordID)
			if err != nil {
				return nil, err
			}
			metaCache[row.InputRecordID] = meta
		}

		lineTargets, ok := lineTargetCache[row.InputRecordID]
		if !ok {
			lineTargets, err = readLineTargetMapForRecord(artifactDir, meta)
			if err != nil {
				return nil, err
			}
			lineTargetCache[row.InputRecordID] = lineTargets
		}

		targets := expandSummaryTargets(convertProvisionSourceSpans(row.SourceLineSpans), lineTargets)
		page, coords := firstSummaryTarget(targets)
		results = append(results, topicCategoryRecord{
			ID:          row.ItemID,
			TopicName:   row.ProvName,
			PdfFileName: filepath.Base(strings.TrimSpace(meta.fileName)),
			TopicType:   row.ProvisionType,
			TopicText:   row.ProvisionText,
			Keywords:    append([]string(nil), row.Keywords...),
			InputID:     row.InputRecordID,
			Page:        page,
			Coords:      coords,
			Targets:     targets,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].ID < results[j].ID
	})
	return results, nil
}

func queryProvisionRows(recordID *int64, ksStoreID *int64) ([]provisionRow, error) {
	db := ApiTypes.ProjectDBHandle
	inputTable, err := resolveInputTable(db)
	if err != nil {
		return nil, err
	}
	provisionTable := "kb.provisions"
	keywordsExpr, err := resolveProvisionJSONArrayExpr(db, provisionTable, []string{"provision_keywords", "prov_keywords"})
	if err != nil {
		return nil, err
	}
	categoryPathsExpr, err := resolveProvisionJSONArrayExpr(db, provisionTable, []string{"category_paths", "categories"})
	if err != nil {
		return nil, err
	}
	confidenceExpr, err := resolveProvisionNumericExpr(db, provisionTable, []string{"confidence", "prov_conf"})
	if err != nil {
		return nil, err
	}
	subjectExpr, err := resolveProvisionTextExpr(db, provisionTable, []string{"provision_subject", "prov_subject"})
	if err != nil {
		return nil, err
	}

	args := make([]any, 0, 2)
	where := make([]string, 0, 2)
	if recordID != nil {
		args = append(args, *recordID)
		where = append(where, fmt.Sprintf("p.input_record_id = $%d", len(args)))
	}
	if ksStoreID != nil {
		args = append(args, *ksStoreID)
		where = append(where, fmt.Sprintf("i.ks_store_id = $%d", len(args)))
	}

	query := fmt.Sprintf(`
SELECT
	p.input_record_id,
	p.prov_id,
	COALESCE(NULLIF(TRIM(p.prov_name), ''), 'provision') AS prov_name,
	COALESCE(TRIM(p.provision_type), '') AS provision_type,
	COALESCE(NULLIF(TRIM(p.prov_desc), ''), NULLIF(TRIM(p.provision_en), ''), NULLIF(TRIM(p.provision_original), ''), NULLIF(TRIM(p.source_text), ''), NULLIF(TRIM(%s), ''), '') AS provision_text,
	%s AS keywords_json,
	%s AS category_paths_json,
	COALESCE(p.source_line_spans, '[]'::jsonb) AS source_line_spans_json,
	%s AS confidence
FROM kb.provisions p
JOIN %s i ON i.id = p.input_record_id`, subjectExpr, keywordsExpr, categoryPathsExpr, confidenceExpr, inputTable)
	if len(where) > 0 {
		query += "\nWHERE " + strings.Join(where, " AND ")
	}
	query += "\nORDER BY p.input_record_id, p.prov_id"

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]provisionRow, 0)
	seenItemIDs := map[string]int{}
	for rows.Next() {
		var rec provisionRow
		var keywordsRaw []byte
		var categoryPathsRaw []byte
		var sourceLineSpansRaw []byte
		if err := rows.Scan(
			&rec.InputRecordID,
			&rec.ProvID,
			&rec.ProvName,
			&rec.ProvisionType,
			&rec.ProvisionText,
			&keywordsRaw,
			&categoryPathsRaw,
			&sourceLineSpansRaw,
			&rec.Confidence,
		); err != nil {
			return nil, err
		}
		rec.Keywords = decodeJSONStringArray(keywordsRaw)
		rec.CategoryChains = decodeProvisionCategoryChains(categoryPathsRaw)
		rec.CategoryPaths = make([]string, 0, len(rec.CategoryChains))
		for _, chain := range rec.CategoryChains {
			if strings.TrimSpace(chain.Path) != "" {
				rec.CategoryPaths = append(rec.CategoryPaths, chain.Path)
			}
		}
		if len(rec.CategoryPaths) == 0 {
			rec.CategoryPaths = decodeJSONStringArray(categoryPathsRaw)
			for _, path := range rec.CategoryPaths {
				rec.CategoryChains = append(rec.CategoryChains, provisionCategoryChain{
					Path:     path,
					Segments: fallbackCategorySegments(path),
				})
			}
		}
		rec.SourceLineSpans = decodeJSONStringArray(sourceLineSpansRaw)
		baseID := formatProvisionID(rec.InputRecordID, rec.ProvID)
		seenItemIDs[baseID]++
		if seenItemIDs[baseID] == 1 {
			rec.ItemID = baseID
		} else {
			rec.ItemID = fmt.Sprintf("%s_%d", baseID, seenItemIDs[baseID])
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func resolveProvisionJSONArrayExpr(db *sql.DB, tableRef string, columns []string) (string, error) {
	schema, table, err := splitQualifiedTable(tableRef)
	if err != nil {
		return "", err
	}
	parts := make([]string, 0, len(columns))
	for _, column := range columns {
		exists, err := columnExists(db, schema, table, column)
		if err != nil {
			return "", err
		}
		if exists {
			parts = append(parts, fmt.Sprintf("p.%s", column))
		}
	}
	parts = append(parts, "'[]'::jsonb")
	return fmt.Sprintf("COALESCE(%s)", strings.Join(parts, ", ")), nil
}

func resolveProvisionNumericExpr(db *sql.DB, tableRef string, columns []string) (string, error) {
	schema, table, err := splitQualifiedTable(tableRef)
	if err != nil {
		return "", err
	}
	parts := make([]string, 0, len(columns))
	for _, column := range columns {
		exists, err := columnExists(db, schema, table, column)
		if err != nil {
			return "", err
		}
		if exists {
			parts = append(parts, fmt.Sprintf("p.%s", column))
		}
	}
	parts = append(parts, "0")
	return fmt.Sprintf("COALESCE(%s)", strings.Join(parts, ", ")), nil
}

func resolveProvisionTextExpr(db *sql.DB, tableRef string, columns []string) (string, error) {
	schema, table, err := splitQualifiedTable(tableRef)
	if err != nil {
		return "", err
	}
	parts := make([]string, 0, len(columns))
	for _, column := range columns {
		exists, err := columnExists(db, schema, table, column)
		if err != nil {
			return "", err
		}
		if exists {
			parts = append(parts, fmt.Sprintf("p.%s", column))
		}
	}
	if len(parts) == 0 {
		return "''", nil
	}
	return fmt.Sprintf("COALESCE(%s)", strings.Join(parts, ", ")), nil
}

func decodeJSONStringArray(raw []byte) []string {
	raw = bytesOrEmptyJSONArray(raw)
	var direct []string
	if err := json.Unmarshal(raw, &direct); err == nil {
		return filterNonEmptyStrings(direct)
	}
	var generic []any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return []string{}
	}
	out := make([]string, 0, len(generic))
	for _, item := range generic {
		out = append(out, strings.TrimSpace(fmt.Sprint(item)))
	}
	return filterNonEmptyStrings(out)
}

func bytesOrEmptyJSONArray(raw []byte) []byte {
	if len(raw) == 0 {
		return []byte("[]")
	}
	return raw
}

func filterNonEmptyStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}

func decodeProvisionCategoryChains(raw []byte) []provisionCategoryChain {
	raw = bytesOrEmptyJSONArray(raw)
	var payloads []provisionCategoryPathPayload
	if err := json.Unmarshal(raw, &payloads); err == nil {
		out := make([]provisionCategoryChain, 0, len(payloads))
		for _, payload := range payloads {
			segments := make([]provisionCategorySegment, 0, len(payload.CategoryPath))
			names := make([]string, 0, len(payload.CategoryPath))
			for _, segment := range payload.CategoryPath {
				name := strings.TrimSpace(segment.Name)
				if name == "" {
					continue
				}
				segments = append(segments, provisionCategorySegment{
					Name:       name,
					Keywords:   filterNonEmptyStrings(segment.Keywords),
					Confidence: segment.Confidence,
				})
				names = append(names, name)
			}
			if len(segments) == 0 {
				continue
			}
			out = append(out, provisionCategoryChain{
				Path:           strings.Join(names, "/"),
				Segments:       segments,
				PathConfidence: payload.PathConfidence,
				PathKeywords:   filterNonEmptyStrings(payload.PathKeywords),
			})
		}
		return out
	}
	return []provisionCategoryChain{}
}

func fallbackCategorySegments(path string) []provisionCategorySegment {
	parts := strings.Split(strings.Trim(strings.TrimSpace(path), "/"), "/")
	out := make([]provisionCategorySegment, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, provisionCategorySegment{Name: part})
	}
	return out
}

func categorySegmentNames(segments []provisionCategorySegment) []string {
	out := make([]string, 0, len(segments))
	for _, segment := range segments {
		name := strings.TrimSpace(segment.Name)
		if name != "" {
			out = append(out, name)
		}
	}
	return out
}

func convertProvisionSourceSpans(spans []string) []string {
	out := make([]string, 0, len(spans))
	for _, span := range spans {
		span = strings.Trim(strings.TrimSpace(span), "[]")
		if span == "" {
			continue
		}
		if strings.Contains(span, ":") {
			parts := strings.SplitN(span, ":", 2)
			span = strings.TrimSpace(parts[1])
		}
		if span == "" {
			continue
		}
		if strings.Contains(span, ",") {
			for _, part := range strings.Split(span, ",") {
				part = strings.TrimSpace(part)
				if part != "" {
					out = append(out, part)
				}
			}
			continue
		}
		out = append(out, span)
	}
	return out
}

func formatProvisionID(recordID int64, provID int) string {
	return fmt.Sprintf("%d_%d", recordID, provID)
}

func appendIfMissing(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
