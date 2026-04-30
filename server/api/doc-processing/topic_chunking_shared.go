package docprocessing

import (
	"context"
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
		b.WriteString("lines: ")
		b.WriteString(formatTopicArray(topic.Lines))
		b.WriteByte('\n')
		b.WriteString("topic_keywords: ")
		b.WriteString(formatTopicArray(topic.Keywords))
		b.WriteByte('\n')
		b.WriteString("topic: ")
		b.WriteString(strconv.Quote(sanitizeTopicText(topic.Topic)))
		b.WriteByte('\n')
		b.WriteString("category_paths: ")
		b.WriteString(formatCategoryPathEntries(topic.CategoryPathDetail))
		b.WriteByte('\n')
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
	extractor LLMJSONExtractor,
	logger ApiTypes.JimoLogger,
	modelName string,
	promptText string,
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
		ll := lineRawForChunking(line)
		if strings.TrimSpace(ll) != "" {
			linesText = append(linesText, ll)
		}
	}

	logger.Info("llm call start",
		logScopeName, logScopeValue,
		"num_lines", len(linesText),
		"model_name", modelName,
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
		"duration_ms", time.Since(llmStart).Milliseconds(),
		"error", err,
	)
	if err != nil {
		baseURL := ""
		if client, ok := extractor.(*llmclients.OpenAIJSONClient); ok && client != nil {
			baseURL = strings.TrimSpace(client.BaseURL)
		}
		return nil, fmt.Errorf(
			"(MID_26042804) extract topics for %s %d failed (model=%q, base_url=%q): %w",
			logScopeName,
			logScopeValue,
			modelName,
			baseURL,
			err,
		)
	}

	rawTopics, ok := parsed["topics"].([]any)
	if !ok {
		return []TopicItem{}, nil
	}

	out := make([]TopicItem, 0, len(rawTopics))
	nextSeq := seqStart
	for _, item := range rawTopics {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}

		lineRanges := compactTopicArray(m["lines"])
		if len(lineRanges) == 0 {
			lineRanges = compactTopicArray(m["line_ranges"])
		}
		topic := sanitizeTopicText(asString(m["topic"]))
		if topic == "" {
			continue
		}
		topicType := strings.ToLower(strings.TrimSpace(asString(m["topic_type"])))
		if topicType == "" {
			topicType = "general"
		}
		topic_keywords := compactTopicArray(m["keywords"])

		categoryPath, categoryFallbackReason := normalizeAndValidateTopicCategoryPath(extractCategoryPathFromLLM(m), topicType)

		logger.Info("raw topic item from LLM",
			logScopeName, logScopeValue,
			"item", item,
			"m", m,
			"topic", topic,
			"topic_type", topicType,
			"line_ranges", lineRanges,
			"keywords", topic_keywords,
			"category_path", categoryPath,
			"category_fallback_reason", categoryFallbackReason,
		)

		if categoryFallbackReason != "" {
			if kp := keywordCategoryPath(topic_keywords); kp != nil {
				categoryPath = kp
			}
			if logger != nil {
				logger.Warn("topic category fallback applied",
					logScopeName, logScopeValue,
					"topic_seq", nextSeq,
					"reason", categoryFallbackReason,
					"fallback_category", strings.Join(categoryPath, "/"),
					"topic", topic,
				)
			}
		}
		out = append(out, TopicItem{
			SeqNo:              nextSeq,
			TopicType:          topicType,
			Lines:              lineRanges,
			Keywords:           topic_keywords,
			Topic:              topic,
			CategoryPath:       categoryPath,
			CategoryPathDetail: extractCategoryPathDetailFromLLM(m),
		})
		nextSeq++
	}

	return out, nil
}
