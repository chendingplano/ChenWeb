package docprocessing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
)

const DefaultChunkTopicModelName = "gpt-5-4-mini"

func buildRecordArtifactDir(baseDir string, recordID int64) (string, error) {
	if strings.TrimSpace(baseDir) == "" {
		return "", errors.New("(MID_26042801) artifact dir is empty")
	}
	if recordID <= 0 {
		return "", fmt.Errorf("(MID_26042805) invalid record id: %d", recordID)
	}
	groupID := recordID / 1000
	targetDir := filepath.Join(baseDir, strconv.FormatInt(groupID, 10), strconv.FormatInt(recordID, 10))
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return "", err
	}
	return targetDir, nil
}

func buildChunkArtifactBaseName(stagingFilename string, parserName string) string {
	root := strings.TrimSuffix(filepath.Base(strings.TrimSpace(stagingFilename)), filepath.Ext(strings.TrimSpace(stagingFilename)))
	if root == "" {
		root = "result"
	}
	parser := strings.TrimSpace(parserName)
	if parser == "" {
		parser = "unknown"
	}
	return root + "_" + parser
}

func writeTopicsFile(chunkDir string, recordID int64, fileName string, topics []TopicItem) (string, error) {
	if strings.TrimSpace(fileName) == "" {
		return "", errors.New("(MID_26043001) topic file name is empty")
	}
	targetDir, err := buildRecordArtifactDir(chunkDir, recordID)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	for i, topic := range topics {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(fmt.Sprintf("topic_id: %d\n", topic.SeqNo))
		b.WriteString(fmt.Sprintf("topic_type: %q\n", strings.TrimSpace(topic.TopicType)))
		if strings.TrimSpace(topic.TopicTypeEn) != "" {
			b.WriteString(fmt.Sprintf("topic_type_en: %q\n", strings.TrimSpace(topic.TopicTypeEn)))
		}
		b.WriteString("lines: ")
		b.WriteString(formatTopicArray(topic.Lines))
		b.WriteByte('\n')
		b.WriteString("topic_keywords: ")
		b.WriteString(formatTopicArray(topic.Keywords))
		b.WriteByte('\n')
		if len(topic.KeywordsEn) > 0 {
			b.WriteString("topic_keywords_en: ")
			b.WriteString(formatTopicArray(topic.KeywordsEn))
			b.WriteByte('\n')
		}
		b.WriteString("topic_desc: ")
		b.WriteString(strconv.Quote(sanitizeTopicText(topic.Topic)))
		b.WriteByte('\n')
		if strings.TrimSpace(topic.TopicEn) != "" {
			b.WriteString("topic_desc_en: ")
			b.WriteString(strconv.Quote(sanitizeTopicText(topic.TopicEn)))
			b.WriteByte('\n')
		}
		b.WriteString("category_paths: ")
		b.WriteString(formatCategoryPathEntries(topic.CategoryPathDetail))
		b.WriteByte('\n')
		if len(topic.CategoryPathDetailEn) > 0 {
			b.WriteString("category_paths_en: ")
			b.WriteString(formatCategoryPathEntries(topic.CategoryPathDetailEn))
			b.WriteByte('\n')
		}
	}

	path := filepath.Join(targetDir, fileName)
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func formatCategoryPathEntries(entries []CategoryPathEntry) string {
	if len(entries) == 0 {
		return "[]"
	}
	parts := make([]string, 0, len(entries))
	for _, entry := range entries {
		parts = append(parts, formatCategoryPathEntry(entry))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func formatCategoryPathEntry(entry CategoryPathEntry) string {
	var b strings.Builder
	b.WriteByte('(')
	b.WriteString(formatStringSlice(entry.PathKeywords))
	b.WriteString(", ")
	b.WriteString(strconv.FormatFloat(entry.PathConfidence, 'f', -1, 64))
	b.WriteString(", [")
	nodeParts := make([]string, 0, len(entry.Nodes))
	for _, node := range entry.Nodes {
		nodeParts = append(nodeParts, formatCategoryPathNode(node))
	}
	b.WriteString(strings.Join(nodeParts, ", "))
	b.WriteString("])")
	return b.String()
}

func formatCategoryPathNode(node CategoryPathNode) string {
	var b strings.Builder
	b.WriteByte('(')
	b.WriteString(strconv.Quote(node.Name))
	b.WriteString(", ")
	b.WriteString(formatStringSlice(node.Keywords))
	b.WriteString(", ")
	b.WriteString(strconv.FormatFloat(node.Confidence, 'f', -1, 64))
	b.WriteByte(')')
	return b.String()
}

func formatStringSlice(items []string) string {
	if items == nil {
		return "[]"
	}
	quoted := make([]string, 0, len(items))
	for _, item := range items {
		quoted = append(quoted, strconv.Quote(item))
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

func writeCombinedChunkFile(chunkDir string, recordID int64, fileName string, chunks []Chunk) (string, error) {
	if strings.TrimSpace(fileName) == "" {
		return "", errors.New("(MID_26042803) chunk file name is empty")
	}
	targetDir, err := buildRecordArtifactDir(chunkDir, recordID)
	if err != nil {
		return "", err
	}

	path := filepath.Join(targetDir, fileName)
	var b strings.Builder
	for i, c := range chunks {
		if i > 0 {
			b.WriteByte('\n')
		}
		overlapLines, regularLines := chunkLineNumbers(c)
		b.WriteString("overlap: ")
		b.WriteString(formatLineNumberRanges(overlapLines))
		b.WriteByte('\n')
		b.WriteString("lines: ")
		b.WriteString(formatLineNumberRanges(regularLines))
		b.WriteByte('\n')
	}
	if err := os.WriteFile(path, []byte(strings.TrimRight(b.String(), "\n")), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func chunkLineNumbers(chunk Chunk) (overlap []int, regular []int) {
	overlap = make([]int, 0, len(chunk.Lines))
	regular = make([]int, 0, len(chunk.Lines))
	for _, ml := range chunk.Lines {
		if ml.Line.LineNo <= 0 {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(ml.Mark), "o") {
			overlap = append(overlap, ml.Line.LineNo)
			continue
		}
		regular = append(regular, ml.Line.LineNo)
	}
	return overlap, regular
}

func formatLineNumberRanges(nums []int) string {
	if len(nums) == 0 {
		return "[]"
	}

	sorted := append([]int(nil), nums...)
	sort.Ints(sorted)
	deduped := sorted[:0]
	last := -1
	for _, n := range sorted {
		if n <= 0 || n == last {
			continue
		}
		deduped = append(deduped, n)
		last = n
	}
	if len(deduped) == 0 {
		return "[]"
	}

	parts := make([]string, 0, len(deduped))
	start := deduped[0]
	prev := deduped[0]
	for i := 1; i < len(deduped); i++ {
		n := deduped[i]
		if n == prev+1 {
			prev = n
			continue
		}
		parts = append(parts, formatLineNumberRange(start, prev))
		start = n
		prev = n
	}
	parts = append(parts, formatLineNumberRange(start, prev))
	return "[" + strings.Join(parts, ", ") + "]"
}

func formatLineNumberRange(start, end int) string {
	if start == end {
		return strconv.Itoa(start)
	}
	return fmt.Sprintf("%d-%d", start, end)
}

func writeTopicsCategoryTreeToDir(logger ApiTypes.JimoLogger, targetDir string, recordID int64, topics []TopicItem) error {
	if strings.TrimSpace(targetDir) == "" {
		return errors.New("(MID_26042110) topic tree dir is empty")
	}
	if recordID <= 0 {
		return fmt.Errorf("(MID_26042111) invalid record id: %d", recordID)
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return err
	}

	existingByLeaf, err := loadLeafRowsExcludingRecord(targetDir, recordID)
	if err != nil {
		return err
	}

	leafRows := make(map[string][]string, len(topics))
	normalizedSourceByPath := make(map[string]string, len(topics))
	for _, topic := range topics {
		categoryPath := topic.CategoryPath
		if len(categoryPath) == 0 {
			categoryPath = fallbackCategoryPath(topic.TopicType)
		}
		path := filepath.Join(categoryPath...)
		rawCategorySource := strings.Join(categoryPath, "/")
		if prev, ok := normalizedSourceByPath[path]; ok && prev != rawCategorySource && logger != nil {
			logger.Warn("normalized topic category collision",
				"record_id", recordID,
				"path", path,
				"source_a", prev,
				"source_b", rawCategorySource,
			)
		} else {
			normalizedSourceByPath[path] = rawCategorySource
		}
		outRow := fmt.Sprintf(
			"%d\t%s\t%s\t%s\t%s",
			recordID,
			strings.TrimSpace(topic.TopicType),
			formatTopicArray(topic.Lines),
			formatTopicArray(topic.Keywords),
			sanitizeTopicText(topic.Topic),
		)
		leafRel := path + ".txt"
		leafRows[leafRel] = append(leafRows[leafRel], outRow)
	}

	allLeafPaths := make(map[string]struct{}, len(existingByLeaf)+len(leafRows))
	for p := range existingByLeaf {
		allLeafPaths[p] = struct{}{}
	}
	for p := range leafRows {
		allLeafPaths[p] = struct{}{}
	}
	leafPaths := make([]string, 0, len(allLeafPaths))
	for p := range allLeafPaths {
		leafPaths = append(leafPaths, p)
	}
	sort.Strings(leafPaths)

	for _, leafRel := range leafPaths {
		rows := append([]string{}, existingByLeaf[leafRel]...)
		rows = append(rows, leafRows[leafRel]...)
		sort.Strings(rows)
		filePath := filepath.Join(targetDir, leafRel)
		if len(rows) == 0 {
			if err := os.Remove(filePath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			continue
		}
		parentDir := filepath.Dir(filePath)
		if err := os.MkdirAll(parentDir, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filePath, []byte(strings.Join(rows, "\n")), 0o644); err != nil {
			return err
		}
	}

	return nil
}

func extractTopicsFromLinesWithLLM(
	ctx context.Context,
	record_id int64,
	extractor LLMJSONExtractor,
	logger ApiTypes.JimoLogger,
	modelName string,
	promptText string,
	promptRef string,
	lines []Line,
	seqStart int,
	logScopeName string,
	logScopeValue int,
) ([]TopicItem, error) {
	if extractor == nil {
		return nil, errors.New("(MID_26042802) topic extractor is nil")
	}

	linesText := make([]string, 0, len(lines))
	for _, line := range lines {
		ll := formatMarkedChunkLine(line, "n")
		if strings.TrimSpace(ll) != "" {
			linesText = append(linesText, ll)
		}
	}

	logger.Info("llm call start",
		logScopeName, logScopeValue,
		"num_lines", len(linesText),
		"model_name", modelName,
		"inputText", strings.Join(linesText, "\n"),
	)

	llmStart := time.Now()
	parsed, err := extractor.ExtractJSON(ctx, llmclients.JSONExtractionInput{
		PromptText: promptText,
		ModelName:  modelName,
		InputText:  strings.Join(linesText, "\n"),
	})
	logger.Info("llm call end",
		logScopeName, logScopeValue,
		"model_name", modelName,
		"prompt_name", promptRef,
		"record_id", record_id,
		"parsed", parsed,
		"duration_ms", time.Since(llmStart).Milliseconds(),
		"error", err,
	)
	if err != nil {
		baseURL := ""
		if client, ok := extractor.(*llmclients.OpenAIJSONClient); ok && client != nil {
			baseURL = strings.TrimSpace(client.BaseURL)
		}
		return nil, fmt.Errorf(
			"(MID_26042804) extract topics for %s %d failed (model=%q, base_url=%q): %w, record_id=%d",
			logScopeName,
			logScopeValue,
			modelName,
			baseURL,
			err,
			record_id,
		)
	}

	rawTopics, ok := parsed["topics"].([]any)
	if !ok {
		return []TopicItem{}, nil
	}
	return normalizeExtractedTopics(rawTopics, seqStart, logger, logScopeName, logScopeValue, record_id), nil
}

func extractTopicsFromMarkedLinesWithLLM(
	ctx context.Context,
	record_id int64,
	extractor LLMJSONExtractor,
	logger ApiTypes.JimoLogger,
	modelName string,
	promptText string,
	promptRef string,
	lines []MarkedLine,
	seqStart int,
	logScopeName string,
	logScopeValue int,
) ([]TopicItem, error) {
	if extractor == nil {
		return nil, errors.New("(MID_26042802) topic extractor is nil")
	}

	inputText := buildMarkedChunkInputText(lines)
	logger.Info("extract topics llm call start",
		logScopeName, logScopeValue,
		"record_id", record_id,
		"num_lines", len(lines),
		"model_name", modelName,
	)

	llmStart := time.Now()
	parsed, err := extractor.ExtractJSON(ctx, llmclients.JSONExtractionInput{
		PromptText: promptText,
		ModelName:  modelName,
		InputText:  inputText,
	})
	logger.Info("extract topics llm call end",
		logScopeName, logScopeValue,
		"record_id", record_id,
		"model_name", modelName,
		"prompt_name", promptRef,
		"duration_ms", time.Since(llmStart).Milliseconds(),
		"error", err,
	)
	if err != nil {
		baseURL := ""
		if client, ok := extractor.(*llmclients.OpenAIJSONClient); ok && client != nil {
			baseURL = strings.TrimSpace(client.BaseURL)
		}
		return nil, fmt.Errorf(
			"(MID_26042804) extract topics for %s %d failed (model=%q, base_url=%q): %w, record_id=%d",
			logScopeName,
			logScopeValue,
			modelName,
			baseURL,
			err,
			record_id,
		)
	}

	rawTopics, ok := parsed["topics"].([]any)
	if !ok {
		return []TopicItem{}, nil
	}
	return normalizeExtractedTopics(rawTopics, seqStart, logger, logScopeName, logScopeValue, record_id), nil
}

func normalizeExtractedTopics(rawTopics []any, seqStart int, logger ApiTypes.JimoLogger, logScopeName string, logScopeValue int, recordID int64) []TopicItem {
	out := make([]TopicItem, 0, len(rawTopics))
	nextSeq := seqStart
	for idx, item := range rawTopics {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}

		lineRanges := compactTopicArray(m["lines"])
		if len(lineRanges) == 0 {
			lineRanges = compactTopicArray(m["line_ranges"])
		}
		topic := sanitizeTopicText(asString(m["topic_desc"]))
		if topic == "" {
			topic = sanitizeTopicText(asString(m["topic"]))
		}
		if topic == "" {
			continue
		}
		topicType := strings.ToLower(strings.TrimSpace(asString(m["topic_type"])))
		if topicType == "" {
			topicType = "general"
		}
		topicTypeEn := strings.ToLower(strings.TrimSpace(asString(m["topic_type_en"])))
		topicKeywords := compactTopicArray(m["topic_keywords"])
		if len(topicKeywords) == 0 {
			topicKeywords = compactTopicArray(m["keywords"])
		}
		topicKeywordsEn := compactTopicArray(m["topic_keywords_en"])
		topicEn := sanitizeTopicText(asString(m["topic_desc_en"]))

		categoryPath, categoryFallbackReason := normalizeAndValidateTopicCategoryPath(extractCategoryPathFromLLM(m), topicType)

		logger.Info("extracted topic from LLM",
			logScopeName, logScopeValue,
			"record_id", recordID,
			"idx", idx,
			"total", len(rawTopics),
			"topic_type", topicType,
			"line_ranges", lineRanges,
		)

		if categoryFallbackReason != "" {
			if kp := keywordCategoryPath(topicKeywords); kp != nil {
				categoryPath = kp
			}
			if logger != nil {
				logger.Warn("topic category fallback applied",
					logScopeName, logScopeValue,
					"record_id", recordID,
					"topic_seq", nextSeq,
					"reason", categoryFallbackReason,
					"fallback_category", strings.Join(categoryPath, "/"),
					"topic", topic,
				)
			}
		}

		out = append(out, TopicItem{
			SeqNo:                nextSeq,
			TopicType:            topicType,
			TopicTypeEn:          topicTypeEn,
			Lines:                lineRanges,
			Keywords:             topicKeywords,
			KeywordsEn:           topicKeywordsEn,
			Topic:                topic,
			TopicEn:              topicEn,
			CategoryPath:         categoryPath,
			CategoryPathDetail:   extractCategoryPathDetailFromLLM(m),
			CategoryPathDetailEn: extractCategoryPathDetailEnFromLLM(m),
		})
		nextSeq++
	}
	return out
}

// ---------------------------------------------------------------------------
// New topic tree indexing (spec Section 6.5)
// ---------------------------------------------------------------------------

const (
	topicCategoryMetaFileName         = "metadata.txt"
	topicsFileName                    = "topics.txt"
	DefaultCategorySimilarityMinScore = 0.85
)

// indexTopicsInTreeDir indexes all topics from a single record into the topic
// tree at treeRootDir using category name-based directory matching.
func indexTopicsInTreeDir(
	logger ApiTypes.JimoLogger,
	treeRootDir string,
	recordID int64,
	topics []TopicItem,
) error {
	if strings.TrimSpace(treeRootDir) == "" {
		return errors.New("(MID_26050210) topic tree root dir is empty")
	}
	if recordID <= 0 {
		return fmt.Errorf("(MID_26050211) invalid record id: %d", recordID)
	}
	if err := os.MkdirAll(treeRootDir, 0o755); err != nil {
		return err
	}
	if err := removeTopicTreeRecord(treeRootDir, recordID); err != nil {
		return fmt.Errorf("(MID_26050212) remove old topic tree entries for record %d: %w", recordID, err)
	}
	now := time.Now()
	for _, topic := range topics {
		paths := topic.CategoryPathDetail
		if len(paths) == 0 && len(topic.CategoryPath) > 0 {
			// synthesise a path entry from the flat CategoryPath
			nodes := make([]CategoryPathNode, 0, len(topic.CategoryPath))
			for _, seg := range topic.CategoryPath {
				nodes = append(nodes, CategoryPathNode{Name: seg})
			}
			paths = []CategoryPathEntry{{Nodes: nodes}}
		}
		for _, pathEntry := range paths {
			if len(pathEntry.Nodes) == 0 {
				continue
			}
			if err := indexTopicPathInTree(logger, treeRootDir, recordID, topic, pathEntry, now); err != nil {
				return fmt.Errorf("(MID_26050213) index topic %d path: %w", topic.SeqNo, err)
			}
		}
	}
	return nil
}

// indexTopicPathInTree navigates/creates the category directory hierarchy for
// one category-path entry of a topic and upserts the topic to topics.txt at
// the leaf directory.
func indexTopicPathInTree(
	logger ApiTypes.JimoLogger,
	treeRootDir string,
	recordID int64,
	topic TopicItem,
	pathEntry CategoryPathEntry,
	now time.Time,
) error {
	currentDir := treeRootDir
	for _, node := range pathEntry.Nodes {
		subdir, err := findOrCreateCategorySubdir(logger, currentDir, node, now)
		if err != nil {
			return err
		}
		currentDir = subdir
	}
	return upsertTopicToLeafDir(currentDir, recordID, topic)
}

// findOrCreateCategorySubdir: Given a node 'CategoryPathNode', it normalize the node name
// and checks whether the directory exists. If yes, it merges the keywords and returns the
// directory name. Otherwise, it creates the directory, its 'metadata.txt' and returns the
// directory name.
func findOrCreateCategorySubdir(
	logger ApiTypes.JimoLogger,
	parentDir string,
	node CategoryPathNode,
	now time.Time,
) (string, error) {
	normalizedName := normalizeCategorySegment(node.Name)
	if normalizedName == "" {
		return "", fmt.Errorf("(MID_26051401) category name is empty")
	}

	// Step 1: exact name match.
	exactDir := filepath.Join(parentDir, normalizedName)
	if _, err := os.Stat(exactDir); err == nil {
		mergeTopicCategoryKeywords(filepath.Join(exactDir, topicCategoryMetaFileName), node.Keywords) //nolint:errcheck
		return exactDir, nil
	}

	/* Embedding is no longer supported
	// Step 2: cosine-similarity match (only when embedder is configured).
	var nodeVec []float64
	if embedder != nil && strings.TrimSpace(embeddingModelName) != "" {
		embedText := node.Name
		if len(node.Keywords) > 0 {
			embedText += " " + strings.Join(node.Keywords, " ")
		}
		vec, err := embedder.Embed(ctx, llmclients.EmbedInput{
			ModelName: embeddingModelName,
			InputText: embedText,
		})
		if err == nil {
			nodeVec = vec
		} else if logger != nil {
			logger.Warn("(MID_26050220) embed category node failed; skipping similarity search",
				"node_name", node.Name, "error", err)
		}
		if len(nodeVec) > 0 {
			entries, readErr := os.ReadDir(parentDir)
			if readErr == nil {
				bestScore := -1.0
				bestDir := ""
				for _, entry := range entries {
					if !entry.IsDir() {
						continue
					}
					embedPath := filepath.Join(parentDir, entry.Name(), topicCategoryEmbedFileName)
					existingVec, loadErr := loadFloatEmbedding(embedPath)
					if loadErr != nil || len(existingVec) == 0 {
						continue
					}
					score := cosineSimilarity(nodeVec, existingVec)
					if score > bestScore {
						bestScore = score
						bestDir = filepath.Join(parentDir, entry.Name())
					}
				}
				if bestDir != "" && bestScore >= similarityThreshold {
					mergeTopicCategoryKeywords(filepath.Join(bestDir, topicCategoryMetaFileName), node.Keywords) //nolint:errcheck
					return bestDir, nil
				}
			}
		}
	}
	*/

	// Step 3: create a new directory using the normalized name (no suffix).
	if logger != nil {
		logger.Info("(MID_26050223) creating new category dir",
			"node_name", node.Name,
			"normalized_name", normalizedName,
			"parent_dir", parentDir,
			"exact_dir", exactDir,
		)
	}
	if err := os.MkdirAll(exactDir, 0o755); err != nil {
		return "", fmt.Errorf("(MID_26050221) create category dir %q: %w", exactDir, err)
	}

	if err := writeTopicCategoryMetadata(exactDir, node, now); err != nil {
		return "", fmt.Errorf("(MID_26050222) write metadata for %q: %w", exactDir, err)
	}
	return exactDir, nil
}

// upsertTopicToLeafDir upserts a topic entry to topics.txt in leafDir.
// Existing entries for recordID are removed before writing new ones.
func upsertTopicToLeafDir(leafDir string, recordID int64, topic TopicItem) error {
	filePath := filepath.Join(leafDir, topicsFileName)
	existing, err := readTopicsFileEntries(filePath)
	if err != nil {
		return fmt.Errorf("(MID_26050230) read %s: %w", filePath, err)
	}
	// Remove old entries for this record (already purged by removeTopicTreeRecord,
	// but guard against duplicates within a single run).
	filtered := existing[:0]
	for _, e := range existing {
		if e.RecordID != recordID {
			filtered = append(filtered, e)
		}
	}
	filtered = append(filtered, topicsFileEntry{
		RecordID:        recordID,
		SeqNo:           topic.SeqNo,
		TopicType:       topic.TopicType,
		Lines:           formatTopicArray(topic.Lines),
		Keywords:        formatTopicArray(topic.Keywords),
		KeywordsEn:      formatTopicArray(topic.KeywordsEn),
		TopicDesc:       sanitizeTopicText(topic.Topic),
		TopicDescEn:     sanitizeTopicText(topic.TopicEn),
		CategoryPaths:   formatCategoryPathEntries(topic.CategoryPathDetail),
		CategoryPathsEn: formatCategoryPathEntries(topic.CategoryPathDetailEn),
	})
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].RecordID != filtered[j].RecordID {
			return filtered[i].RecordID < filtered[j].RecordID
		}
		return filtered[i].SeqNo < filtered[j].SeqNo
	})
	if err := writeTopicsFileEntries(filePath, filtered); err != nil {
		return fmt.Errorf("(MID_26050231) write %s: %w", filePath, err)
	}
	return nil
}

// removeTopicTreeRecord walks treeRootDir and removes all topics.txt entries
// whose record_id matches recordID.
func removeTopicTreeRecord(treeRootDir string, recordID int64) error {
	return filepath.WalkDir(treeRootDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != topicsFileName {
			return nil
		}
		entries, readErr := readTopicsFileEntries(path)
		if readErr != nil {
			return readErr
		}
		filtered := entries[:0]
		for _, e := range entries {
			if e.RecordID != recordID {
				filtered = append(filtered, e)
			}
		}
		if len(filtered) == len(entries) {
			return nil // nothing changed
		}
		if len(filtered) == 0 {
			return os.Remove(path)
		}
		return writeTopicsFileEntries(path, filtered)
	})
}

// topicsFileEntry is one topic block in a topics.txt file.
type topicsFileEntry struct {
	RecordID        int64
	SeqNo           int
	TopicType       string
	Lines           string
	Keywords        string
	KeywordsEn      string
	TopicDesc       string
	TopicDescEn     string
	CategoryPaths   string
	CategoryPathsEn string
}

// readTopicsFileEntries parses the multi-line topics.txt format (spec 6.5.3).
// Returns an empty slice if the file does not exist.
func readTopicsFileEntries(path string) ([]topicsFileEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []topicsFileEntry{}, nil
		}
		return nil, err
	}
	return parseTopicsFileEntries(data), nil
}

func parseTopicsFileEntries(data []byte) []topicsFileEntry {
	out := make([]topicsFileEntry, 0, 8)
	var cur *topicsFileEntry
	for _, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			if cur != nil {
				out = append(out, *cur)
				cur = nil
			}
			continue
		}
		key, val := splitSpecLine(line)
		if key == "" {
			continue
		}
		if cur == nil {
			cur = &topicsFileEntry{}
		}
		switch key {
		case "record_id":
			// value may have a trailing comma: "123," or "123"
			v := strings.TrimSuffix(strings.TrimSpace(val), ",")
			cur.RecordID, _ = strconv.ParseInt(v, 10, 64)
		case "topic_id":
			v := strings.TrimSuffix(strings.TrimSpace(val), ",")
			n, _ := strconv.Atoi(strings.Trim(v, `"`))
			cur.SeqNo = n
		case "topic_type":
			cur.TopicType = unquoteSpec(val)
		case "lines":
			cur.Lines = val
		case "topic_keywords":
			cur.Keywords = val
		case "topic_keywords_en":
			cur.KeywordsEn = val
		case "topic_desc":
			cur.TopicDesc = unquoteSpec(val)
		case "topic": // backward compat with old format
			cur.TopicDesc = unquoteSpec(val)
		case "topic_desc_en":
			cur.TopicDescEn = unquoteSpec(val)
		case "category_paths":
			cur.CategoryPaths = val
		case "category_paths_en":
			cur.CategoryPathsEn = val
		}
	}
	if cur != nil {
		out = append(out, *cur)
	}
	return out
}

func writeTopicsFileEntries(path string, entries []topicsFileEntry) error {
	var b strings.Builder
	for i, e := range entries {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(fmt.Sprintf("record_id: %d\n", e.RecordID))
		b.WriteString(fmt.Sprintf("topic_id: %d\n", e.SeqNo))
		b.WriteString(fmt.Sprintf("topic_type: %q\n", e.TopicType))
		b.WriteString("lines: ")
		b.WriteString(e.Lines)
		b.WriteByte('\n')
		b.WriteString("topic_keywords: ")
		b.WriteString(e.Keywords)
		b.WriteByte('\n')
		if e.KeywordsEn != "" && e.KeywordsEn != "[]" {
			b.WriteString("topic_keywords_en: ")
			b.WriteString(e.KeywordsEn)
			b.WriteByte('\n')
		}
		b.WriteString("topic_desc: ")
		b.WriteString(strconv.Quote(e.TopicDesc))
		b.WriteByte('\n')
		if strings.TrimSpace(e.TopicDescEn) != "" {
			b.WriteString("topic_desc_en: ")
			b.WriteString(strconv.Quote(e.TopicDescEn))
			b.WriteByte('\n')
		}
		b.WriteString("category_paths: ")
		b.WriteString(e.CategoryPaths)
		b.WriteByte('\n')
		if e.CategoryPathsEn != "" && e.CategoryPathsEn != "[]" {
			b.WriteString("category_paths_en: ")
			b.WriteString(e.CategoryPathsEn)
			b.WriteByte('\n')
		}
	}
	return os.WriteFile(path, []byte(strings.TrimRight(b.String(), "\n")), 0o644)
}

// writeTopicCategoryMetadata writes metadata.txt for a topic category directory
// (spec 6.5.1). Does not include category_type (unlike the summary tree metadata).
func writeTopicCategoryMetadata(dir string, node CategoryPathNode, now time.Time) error {
	metaPath := filepath.Join(dir, topicCategoryMetaFileName)
	if _, err := os.Stat(metaPath); err == nil {
		// Merge keywords into existing metadata without overwriting confidence/desc.
		return mergeTopicCategoryKeywords(metaPath, node.Keywords)
	}
	keywords := node.Keywords
	if keywords == nil {
		keywords = []string{}
	}
	descJSON, _ := json.Marshal(node.Name)
	kwJSON, err := json.Marshal(keywords)
	if err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf(`"desc":%s`, string(descJSON)) + "\n")
	b.WriteString(fmt.Sprintf(`"confidence":%s`, strconv.FormatFloat(node.Confidence, 'f', -1, 64)) + "\n")
	b.WriteString(fmt.Sprintf(`"keywords":%s`, string(kwJSON)) + "\n")
	b.WriteString(fmt.Sprintf(`"create_time":"%s"`, now.Format("20060102-150405")))
	return os.WriteFile(metaPath, []byte(b.String()), 0o644)
}

// mergeTopicCategoryKeywords adds any new keywords to the existing metadata.txt.
func mergeTopicCategoryKeywords(metaPath string, newKeywords []string) error {
	if len(newKeywords) == 0 {
		return nil
	}
	body, err := os.ReadFile(metaPath)
	if err != nil {
		return err
	}
	lines := strings.Split(string(body), "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, `"keywords":`) {
			continue
		}
		valJSON := strings.TrimSpace(strings.TrimPrefix(trimmed, `"keywords":`))
		var existing []string
		if err := json.Unmarshal([]byte(valJSON), &existing); err != nil {
			return err
		}
		added := false
		for _, kw := range newKeywords {
			found := false
			for _, e := range existing {
				if e == kw {
					found = true
					break
				}
			}
			if !found {
				existing = append(existing, kw)
				added = true
			}
		}
		if !added {
			return nil
		}
		kwJSON, err := json.Marshal(existing)
		if err != nil {
			return err
		}
		lines[i] = fmt.Sprintf(`"keywords":%s`, string(kwJSON))
		return os.WriteFile(metaPath, []byte(strings.Join(lines, "\n")), 0o644)
	}
	return nil
}
