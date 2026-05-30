package docprocessing

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

type categoryPathPair struct {
	Index    CategoryPathEntry
	Original CategoryPathEntry
}

type localizedCategoryMetadata struct {
	OriginalNames []string
	Desc          string
	DescEn        string
	CategoryType  string
	Confidence    float64
	Keywords      []string
	KeywordsEn    []string
}

func pairCategoryPathEntries(originalRaw, englishRaw any) []categoryPathPair {
	originalEntries := parseCategoryPathsAny(originalRaw)
	englishEntries := parseCategoryPathsAny(englishRaw)
	return pairCategoryPathEntrySlices(originalEntries, englishEntries)
}

func pairCategoryPathEntrySlices(originalEntries, englishEntries []CategoryPathEntry) []categoryPathPair {
	if len(englishEntries) > 0 {
		out := make([]categoryPathPair, 0, len(englishEntries))
		for i, entry := range englishEntries {
			original := entry
			if i < len(originalEntries) && len(originalEntries[i].Nodes) > 0 {
				original = originalEntries[i]
			}
			out = append(out, categoryPathPair{
				Index:    entry,
				Original: original,
			})
		}
		return out
	}
	out := make([]categoryPathPair, 0, len(originalEntries))
	for _, entry := range originalEntries {
		out = append(out, categoryPathPair{
			Index:    entry,
			Original: entry,
		})
	}
	return out
}

func categoryTreeLeafDirForEntry(
	logger ApiTypes.JimoLogger,
	treeRootDir string,
	indexEntry CategoryPathEntry,
	originalEntry CategoryPathEntry,
	now time.Time,
) (string, error) {
	if len(indexEntry.Nodes) == 0 {
		return "", nil
	}
	currentDir := treeRootDir
	for i := range indexEntry.Nodes {
		indexNode, originalNode := categoryNodePair(indexEntry, originalEntry, i)
		subdir, err := findOrCreateCategorySubdir(logger, currentDir, indexNode, originalNode, now)
		if err != nil {
			return "", err
		}
		currentDir = subdir
	}
	return filepath.Clean(currentDir), nil
}

func parseCategoryPathsAny(raw any) []CategoryPathEntry {
	if raw == nil {
		return nil
	}
	if arr, ok := raw.([]any); ok {
		return parseCategoryPathsArray(arr)
	}
	bs, err := json.Marshal(raw)
	if err != nil || len(bs) == 0 || string(bs) == "null" || string(bs) == "[]" {
		return nil
	}
	var arr []any
	if err := json.Unmarshal(bs, &arr); err != nil {
		return nil
	}
	return parseCategoryPathsArray(arr)
}

func categoryPathNames(nodes []CategoryPathNode) []string {
	if len(nodes) == 0 {
		return nil
	}
	out := make([]string, 0, len(nodes))
	for _, node := range nodes {
		name := strings.TrimSpace(node.Name)
		if name == "" {
			continue
		}
		out = append(out, name)
	}
	return out
}

/*
func summaryTreeIndexData(item SummaryItem) ([]string, []CategoryPathNode, []CategoryPathNode) {
	if len(item.CategoryPathItemsEn) > 0 {
		indexNodes := append([]CategoryPathNode(nil), item.CategoryPathItemsEn[0].Nodes...)
		originalNodes := indexNodes
		if len(item.CategoryPathItems) > 0 && len(item.CategoryPathItems[0].Nodes) > 0 {
			originalNodes = append([]CategoryPathNode(nil), item.CategoryPathItems[0].Nodes...)
		}
		return categoryPathNames(indexNodes), indexNodes, originalNodes
	}
	if len(item.CategoryPathItems) > 0 {
		nodes := append([]CategoryPathNode(nil), item.CategoryPathItems[0].Nodes...)
		return categoryPathNames(nodes), nodes, nodes
	}
	return append([]string(nil), item.CategoryPaths...), append([]CategoryPathNode(nil), item.CategoryNodes...), append([]CategoryPathNode(nil), item.CategoryNodes...)
}
*/

func summaryTreeIndexPairs(item SummaryItem) []categoryPathPair {
	if len(item.CategoryPathItemsEn) > 0 {
		out := make([]categoryPathPair, 0, len(item.CategoryPathItemsEn))
		for i, entry := range item.CategoryPathItemsEn {
			original := entry
			if i < len(item.CategoryPathItems) && len(item.CategoryPathItems[i].Nodes) > 0 {
				original = item.CategoryPathItems[i]
			}
			out = append(out, categoryPathPair{
				Index:    entry,
				Original: original,
			})
		}
		return out
	}
	if len(item.CategoryPathItems) > 0 {
		out := make([]categoryPathPair, 0, len(item.CategoryPathItems))
		for _, entry := range item.CategoryPathItems {
			out = append(out, categoryPathPair{
				Index:    entry,
				Original: entry,
			})
		}
		return out
	}
	if len(item.CategoryPaths) == 0 {
		return nil
	}
	entry := CategoryPathEntry{Nodes: categoryPathNodesFromNames(item.CategoryPaths)}
	return []categoryPathPair{{
		Index:    entry,
		Original: entry,
	}}
}

func writeSummaryTreeEntriesForItem(logger ApiTypes.JimoLogger, baseDir string, item SummaryItem) error {
	pairs := summaryTreeIndexPairs(item)
	if len(pairs) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(pairs))
	for _, pair := range pairs {
		indexPath := categoryPathNames(pair.Index.Nodes)
		if len(indexPath) == 0 {
			continue
		}
		normalizedPath := make([]string, 0, len(indexPath))
		for _, segment := range indexPath {
			normalizedPath = append(normalizedPath, normalizeCategorySegment(segment))
		}
		key := strings.Join(normalizedPath, "\x00")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if err := writeSummaryTreeEntryLocalized(logger, baseDir, item, indexPath, pair.Index.Nodes, pair.Original.Nodes); err != nil {
			return err
		}
	}
	return nil
}

func categoryNodePair(indexEntry, originalEntry CategoryPathEntry, idx int) (CategoryPathNode, CategoryPathNode) {
	var indexNode CategoryPathNode
	if idx < len(indexEntry.Nodes) {
		indexNode = indexEntry.Nodes[idx]
	}
	originalNode := indexNode
	if idx < len(originalEntry.Nodes) && strings.TrimSpace(originalEntry.Nodes[idx].Name) != "" {
		originalNode = originalEntry.Nodes[idx]
	}
	return indexNode, originalNode
}

func buildLocalizedCategoryMetadata(indexNode, originalNode CategoryPathNode, categoryType string) localizedCategoryMetadata {
	desc := strings.TrimSpace(originalNode.Name)
	if desc == "" {
		desc = strings.TrimSpace(indexNode.Name)
	}
	descEn := ""
	if strings.TrimSpace(indexNode.Name) != "" && strings.TrimSpace(indexNode.Name) != desc {
		descEn = strings.TrimSpace(indexNode.Name)
	}
	originalNames := []string{}
	if strings.TrimSpace(originalNode.Name) != "" {
		originalNames = appendUniqueString(originalNames, strings.TrimSpace(originalNode.Name))
	}
	if len(originalNames) == 0 && strings.TrimSpace(indexNode.Name) != "" {
		originalNames = appendUniqueString(originalNames, strings.TrimSpace(indexNode.Name))
	}
	keywords := trimStringSlice(originalNode.Keywords)
	keywordsEn := []string(nil)
	if descEn != "" {
		keywordsEn = trimStringSlice(indexNode.Keywords)
	} else if len(keywords) == 0 {
		keywords = trimStringSlice(indexNode.Keywords)
	}
	return localizedCategoryMetadata{
		OriginalNames: uniqueStrings(originalNames),
		Desc:          desc,
		DescEn:        descEn,
		CategoryType:  strings.TrimSpace(categoryType),
		Confidence:    maxFloat(indexNode.Confidence, originalNode.Confidence),
		Keywords:      keywords,
		KeywordsEn:    keywordsEn,
	}
}

func upsertLocalizedCategoryMetadata(path string, fields localizedCategoryMetadata, now time.Time) error {
	if _, err := os.Stat(path); err == nil {
		return mergeLocalizedCategoryMetadata(path, fields, now)
	} else if !os.IsNotExist(err) {
		return err
	}
	return writeLocalizedCategoryMetadata(path, fields, now)
}

func writeLocalizedCategoryMetadata(path string, fields localizedCategoryMetadata, now time.Time) error {
	lines, err := renderLocalizedCategoryMetadata(nil, fields, now)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
}

func mergeLocalizedCategoryMetadata(path string, fields localizedCategoryMetadata, now time.Time) error {
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines, err := renderLocalizedCategoryMetadata(strings.Split(string(body), "\n"), fields, now)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
}

func renderLocalizedCategoryMetadata(lines []string, fields localizedCategoryMetadata, now time.Time) ([]string, error) {
	if len(lines) == 0 {
		out := make([]string, 0, 8)
		out = appendJSONMetadataLine(out, "original_names", uniqueStrings(fields.OriginalNames))
		out = appendJSONMetadataLine(out, "desc", strings.TrimSpace(fields.Desc))
		if strings.TrimSpace(fields.DescEn) != "" {
			out = appendJSONMetadataLine(out, "desc_en", strings.TrimSpace(fields.DescEn))
		}
		if strings.TrimSpace(fields.CategoryType) != "" {
			out = appendJSONMetadataLine(out, "category_type", strings.TrimSpace(fields.CategoryType))
		}
		out = appendRawMetadataLine(out, "confidence", fmt.Sprintf("%s", formatFloat(fields.Confidence)))
		out = appendJSONMetadataLine(out, "keywords", trimStringSlice(fields.Keywords))
		if len(trimStringSlice(fields.KeywordsEn)) > 0 {
			out = appendJSONMetadataLine(out, "keywords_en", trimStringSlice(fields.KeywordsEn))
		}
		out = appendJSONMetadataLine(out, "create_time", now.Format("20060102-150405"))
		return out, nil
	}

	existingOriginalNames, _, _ := metadataStringArray(lines, "original_names")
	mergedOriginalNames := uniqueStrings(append(existingOriginalNames, fields.OriginalNames...))
	lines = upsertJSONMetadataLine(lines, "original_names", mergedOriginalNames)

	if value, idx, ok := metadataLineValue(lines, "desc"); !ok || strings.TrimSpace(value) == "" || idx < 0 {
		lines = upsertJSONMetadataLine(lines, "desc", strings.TrimSpace(fields.Desc))
	}
	if strings.TrimSpace(fields.DescEn) != "" {
		lines = upsertJSONMetadataLine(lines, "desc_en", strings.TrimSpace(fields.DescEn))
	}
	if strings.TrimSpace(fields.CategoryType) != "" {
		if _, _, ok := metadataLineValue(lines, "category_type"); !ok {
			lines = upsertJSONMetadataLine(lines, "category_type", strings.TrimSpace(fields.CategoryType))
		}
	}
	if _, _, ok := metadataLineValue(lines, "confidence"); !ok {
		lines = appendRawMetadataLine(lines, "confidence", formatFloat(fields.Confidence))
	}

	existingKeywords, _, _ := metadataStringArray(lines, "keywords")
	lines = upsertJSONMetadataLine(lines, "keywords", uniqueStrings(append(existingKeywords, trimStringSlice(fields.Keywords)...)))
	if len(trimStringSlice(fields.KeywordsEn)) > 0 {
		existingKeywordsEn, _, _ := metadataStringArray(lines, "keywords_en")
		lines = upsertJSONMetadataLine(lines, "keywords_en", uniqueStrings(append(existingKeywordsEn, trimStringSlice(fields.KeywordsEn)...)))
	}
	if _, _, ok := metadataLineValue(lines, "create_time"); !ok {
		lines = appendJSONMetadataLine(lines, "create_time", now.Format("20060102-150405"))
	}
	return lines, nil
}

func metadataLineValue(lines []string, key string) (string, int, bool) {
	prefix := fmt.Sprintf("%q:", key)
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, prefix)), i, true
		}
	}
	return "", -1, false
}

func metadataStringArray(lines []string, key string) ([]string, int, error) {
	value, idx, ok := metadataLineValue(lines, key)
	if !ok {
		return nil, -1, nil
	}
	var out []string
	if err := json.Unmarshal([]byte(value), &out); err != nil {
		return nil, idx, err
	}
	return out, idx, nil
}

func appendJSONMetadataLine(lines []string, key string, value any) []string {
	bs, _ := json.Marshal(value)
	return append(lines, fmt.Sprintf(`%q:%s`, key, string(bs)))
}

func appendRawMetadataLine(lines []string, key string, value string) []string {
	return append(lines, fmt.Sprintf(`%q:%s`, key, strings.TrimSpace(value)))
}

func upsertJSONMetadataLine(lines []string, key string, value any) []string {
	bs, _ := json.Marshal(value)
	line := fmt.Sprintf(`%q:%s`, key, string(bs))
	if _, idx, ok := metadataLineValue(lines, key); ok && idx >= 0 {
		lines[idx] = line
		return lines
	}
	return append(lines, line)
}

func formatFloat(v float64) string {
	return strings.TrimSpace(fmt.Sprintf("%g", v))
}

func uniqueStrings(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		out = appendUniqueString(out, trimmed)
	}
	return out
}
