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
	"sync"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

const (
	DefaultSummaryGroupSize             = 5
	DefaultSummaryClusterSimilarity     = 0.2
	DefaultSummaryReclusteringDays      = 7
	defaultSummaryClusterStateFileName  = "_cluster_state.json"
	defaultSummaryTreeFallbackTopicType = "summary"
	categoryMetadataFileName            = "metadata.txt"
)

// summaryGenerateResult carries all LLM output fields for one summary generation call.
// It is returned by generateSummary and used to populate a SummaryItem.
type summaryGenerateResult struct {
	Summary             string
	SummaryEn           string
	Keywords            []string
	KeywordsEn          []string
	CategoryPaths       []string            // flat segment names used for tree-dir indexing
	CategoryNodes       []CategoryPathNode  // per-node metadata for the first category path
	CategoryPathItems   []CategoryPathEntry // all category paths in rich format (written to file)
	CategoryPathItemsEn []CategoryPathEntry // English translations of category paths
}

type SummaryItem struct {
	SummaryID           string
	RecordID            int64
	Level               int
	SeqNo               int
	Lines               []string
	Children            []string
	Keywords            []string
	KeywordsEn          []string
	CategoryPaths       []string            // flat segment names for tree-dir indexing
	CategoryNodes       []CategoryPathNode  // per-node metadata for first category path
	CategoryPathItems   []CategoryPathEntry // rich format written to summary file
	CategoryPathItemsEn []CategoryPathEntry // English translations
	Summary             string
	SummaryEn           string
	Language            string
	Embedding           []float64
}

type SummaryCluster struct {
	ClusterID         string
	ClusterName       string
	ClusterLevel      string
	CreatedAt         string
	UpdatedAt         string
	SummaryIDs        []string
	RepresentativeIDs []string
	RelatedClusterIDs []string
	CentroidEmbedding []float64
	ClusterSummary    string
}

type summaryClusterState struct {
	NextClusterID       int       `json:"next_cluster_id"`
	LastFullReclusterAt time.Time `json:"last_full_recluster_at"`
}

func buildSummaryID(recordID int64, level int, seqNo int) string {
	return fmt.Sprintf("%d_%d_%04d", recordID, level, seqNo)
}

func summaryFileName(level int, seqNo int) string {
	return fmt.Sprintf("summary_%d_%04d.txt", level, seqNo)
}

func summaryEmbedFileName(level int, seqNo int) string {
	return fmt.Sprintf("summary_%d_%04d.embed", level, seqNo)
}

func writeSummaryFile(baseDir string, recordID int64, item SummaryItem) (string, error) {
	targetDir, err := buildRecordArtifactDir(baseDir, recordID)
	if err != nil {
		return "", err
	}
	path := filepath.Join(targetDir, summaryFileName(item.Level, item.SeqNo))
	var b strings.Builder
	b.WriteString(fmt.Sprintf("summary_id: %q\n", strings.TrimSpace(item.SummaryID)))
	b.WriteString(fmt.Sprintf("record_id: %d\n", item.RecordID))
	b.WriteString(fmt.Sprintf("level: %d\n", item.Level))
	b.WriteString("lines: ")
	b.WriteString(formatTopicArray(item.Lines))
	b.WriteByte('\n')
	b.WriteString("children: ")
	b.WriteString(formatQuotedArray(item.Children))
	b.WriteByte('\n')
	b.WriteString(fmt.Sprintf("language: %q\n", strings.TrimSpace(item.Language)))
	b.WriteString("keywords: ")
	b.WriteString(formatQuotedArray(item.Keywords))
	b.WriteByte('\n')
	if len(item.KeywordsEn) > 0 {
		b.WriteString("keywords_en: ")
		b.WriteString(formatQuotedArray(item.KeywordsEn))
		b.WriteByte('\n')
	}
	b.WriteString("category_paths: ")
	b.WriteString(formatCategoryPathEntries(item.CategoryPathItems))
	b.WriteByte('\n')
	if len(item.CategoryPathItemsEn) > 0 {
		b.WriteString("category_paths_en: ")
		b.WriteString(formatCategoryPathEntries(item.CategoryPathItemsEn))
		b.WriteByte('\n')
	}
	b.WriteString("summary_begin\n")
	b.WriteString(strings.TrimSpace(item.Summary))
	b.WriteByte('\n')
	b.WriteString("summary_end\n")
	if strings.TrimSpace(item.SummaryEn) != "" {
		b.WriteString("summary_en_begin\n")
		b.WriteString(strings.TrimSpace(item.SummaryEn))
		b.WriteByte('\n')
		b.WriteString("summary_en_end\n")
	}
	if err := os.WriteFile(path, []byte(strings.TrimRight(b.String(), "\n")), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func validateSummaryArtifacts(recordID int64, sourceLanguage string, summaries []SummaryItem, artifactDir string, summaryTreeDir string) error {
	if len(summaries) == 0 {
		return errors.New("(MID-26060403) no summaries generated")
	}

	normalizedSourceLanguage := normalizeSummarySourceLanguage(sourceLanguage)
	seqByLevel := make(map[int][]int)
	for _, item := range summaries {
		if err := validateSingleSummaryArtifact(recordID, normalizedSourceLanguage, item, artifactDir, summaryTreeDir); err != nil {
			return err
		}
		seqByLevel[item.Level] = append(seqByLevel[item.Level], item.SeqNo)
	}

	for level, seqs := range seqByLevel {
		sort.Ints(seqs)
		for i, seq := range seqs {
			want := i + 1
			if seq != want {
				return fmt.Errorf("(MID-26060402) level %d seqno %d is not continuous; want %d", level, seq, want)
			}
		}
	}
	return nil
}

func validateSingleSummaryArtifact(recordID int64, sourceLanguage string, item SummaryItem, artifactDir string, summaryTreeDir string) error {
	if strings.TrimSpace(item.SummaryID) == "" {
		return errors.New("(MID-26060412) summary_id is empty")
	}
	if strings.TrimSpace(item.Summary) == "" {
		return fmt.Errorf("(MID-26060413) summary %q is empty", item.SummaryID)
	}
	if item.Lines == nil {
		return fmt.Errorf("(MID-26060414) summary %q lines must be an array", item.SummaryID)
	}
	recordIDInSummary, level, seqNo, ok := parseSummaryID(item.SummaryID)
	if !ok {
		return fmt.Errorf("(MID-26060415) summary %q has invalid summary_id format", item.SummaryID)
	}
	if recordIDInSummary != recordID || item.RecordID != recordID {
		return fmt.Errorf("(MID-26060416) summary %q record_id mismatch", item.SummaryID)
	}
	if level != item.Level || seqNo != item.SeqNo {
		return fmt.Errorf("(MID-26060417) summary %q level/seq mismatch", item.SummaryID)
	}
	if strings.TrimSpace(item.SummaryID) != buildSummaryID(recordID, item.Level, item.SeqNo) {
		return fmt.Errorf("(MID-26060418) summary %q does not match canonical summary_id", item.SummaryID)
	}
	if item.Level < 0 {
		return fmt.Errorf("(MID-26060419) summary %q level must be non-negative", item.SummaryID)
	}
	if err := validateSummaryLines(item.SummaryID, item.Lines); err != nil {
		return err
	}
	// Only enforce "summary and summary_en must differ" for known non-English sources.
	// For English sources the fields may legitimately be identical.
	// For unknown sources we have no basis to require a translation.
	if sourceLanguage != "" && sourceLanguage != "en" {
		if summaryEn := strings.TrimSpace(item.SummaryEn); summaryEn != "" && strings.TrimSpace(item.Summary) == summaryEn {
			return fmt.Errorf("(MID-26060420) summary %q summary and summary_en must differ", item.SummaryID)
		}
	}
	// keywords may legitimately match keywords_en when translation is unavailable;
	// fixSummarySourceLanguage attempts a best-effort backfill before validation.
	// Skip language check when summary == summary_en: this indicates translation was
	// not available and the English text was kept as a fallback, which is acceptable.
	summaryEn := strings.TrimSpace(item.SummaryEn)
	if sourceLanguage != "" && (summaryEn == "" || strings.TrimSpace(item.Summary) != summaryEn) {
		if detectContentLanguage(item.Summary) != sourceLanguage {
			return fmt.Errorf("(MID-26060421) summary %q language mismatch: got %s want %s", item.SummaryID, detectContentLanguage(item.Summary), sourceLanguage)
		}
	}
	if err := validateSummaryArtifactFile(recordID, item, artifactDir); err != nil {
		return err
	}
	for _, categoryPath := range summaryCategoryPathsForValidation(item) {
		if err := validateSummaryTreeReference(summaryTreeDir, categoryPath, item.SummaryID); err != nil {
			return err
		}
	}
	return nil
}

func validateSummaryArtifactFile(recordID int64, item SummaryItem, artifactDir string) error {
	targetDir, err := buildRecordArtifactDir(artifactDir, recordID)
	if err != nil {
		return err
	}
	path := filepath.Join(targetDir, summaryFileName(item.Level, item.SeqNo))
	body, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("(MID-26060405) read summary file %q: %w", path, err)
	}
	text := string(body)
	if !strings.Contains(text, fmt.Sprintf("summary_id: %q", strings.TrimSpace(item.SummaryID))) {
		return fmt.Errorf("(MID-26060406) summary file %q missing summary_id %q", path, item.SummaryID)
	}
	if !strings.Contains(text, "summary_begin") || !strings.Contains(text, "summary_end") {
		return fmt.Errorf("(MID-26060407) summary file %q missing summary markers", path)
	}
	if strings.TrimSpace(item.SummaryEn) != "" {
		hasSummaryEnStart := strings.Contains(text, "summary_en_begin") || strings.Contains(text, "summary_en_start")
		if !hasSummaryEnStart || !strings.Contains(text, "summary_en_end") {
			return fmt.Errorf("(MID-26060408) summary file %q missing summary_en markers", path)
		}
	}
	return nil
}

func validateSummaryTreeReference(summaryTreeDir string, categoryPath []string, summaryID string) error {
	if len(categoryPath) == 0 {
		return fmt.Errorf("summary %q has empty category path", summaryID)
	}
	normalizedPath := make([]string, 0, len(categoryPath))
	for _, segment := range categoryPath {
		normalized := normalizeCategorySegment(segment)
		if normalized == "" {
			return fmt.Errorf("summary %q has invalid category path segment %q", summaryID, segment)
		}
		normalizedPath = append(normalizedPath, normalized)
	}
	leaf := filepath.Join(append([]string{summaryTreeDir}, append(normalizedPath, "summaries.txt")...)...)
	body, err := os.ReadFile(leaf)
	if err != nil {
		return fmt.Errorf("read summaries.txt for %q: %w", summaryID, err)
	}
	for _, line := range strings.Split(string(body), "\n") {
		if strings.TrimSpace(line) == strings.TrimSpace(summaryID) {
			return nil
		}
	}
	return fmt.Errorf("summaries.txt missing summary_id %q", summaryID)
}

func validateSummaryLines(summaryID string, lines []string) error {
	if len(lines) == 0 {
		return fmt.Errorf("(MID-26060409) summary %q lines must not be empty", summaryID)
	}
	prevStart, prevEnd := 0, 0
	for _, span := range lines {
		start, end, ok := parseSummaryLineSpan(span)
		if !ok {
			return fmt.Errorf("(MID-26060410) summary %q has invalid line span %q", summaryID, span)
		}
		if start < prevStart || (start == prevStart && end < prevEnd) {
			return fmt.Errorf("(MID-26060411) summary %q lines must be sorted", summaryID)
		}
		prevStart, prevEnd = start, end
	}
	return nil
}

func parseSummaryLineSpan(span string) (int, int, bool) {
	trimmed := strings.TrimSpace(strings.Trim(span, "[]"))
	if trimmed == "" {
		return 0, 0, false
	}
	if !strings.Contains(trimmed, "-") {
		n, err := strconv.Atoi(trimmed)
		if err != nil || n <= 0 {
			return 0, 0, false
		}
		return n, n, true
	}
	parts := strings.SplitN(trimmed, "-", 2)
	start, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	end, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || start <= 0 || end < start {
		return 0, 0, false
	}
	return start, end, true
}

/*
func hasSummaryCategoryPath(item SummaryItem) bool {
	return len(summaryCategoryPathsForValidation(item)) > 0
}
*/

func summaryCategoryPathsForValidation(item SummaryItem) [][]string {
	if len(item.CategoryPathItemsEn) > 0 {
		out := make([][]string, 0, len(item.CategoryPathItemsEn))
		for _, entry := range item.CategoryPathItemsEn {
			names := entry.NodeNames()
			if len(names) == 0 {
				continue
			}
			out = append(out, names)
		}
		if len(out) > 0 {
			return out
		}
	}
	if len(item.CategoryPathItems) > 0 {
		out := make([][]string, 0, len(item.CategoryPathItems))
		for _, entry := range item.CategoryPathItems {
			names := entry.NodeNames()
			if len(names) == 0 {
				continue
			}
			out = append(out, names)
		}
		if len(out) > 0 {
			return out
		}
	}
	if len(item.CategoryPaths) == 0 {
		return nil
	}
	return [][]string{append([]string(nil), item.CategoryPaths...)}
}

func equalTrimmedStringSlices(a []string, b []string) bool {
	left := trimStringSlice(a)
	right := trimStringSlice(b)
	if len(left) == 0 || len(right) == 0 || len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func normalizeSummarySourceLanguage(language string) string {
	lang := strings.ToLower(strings.TrimSpace(language))
	switch {
	case lang == "":
		return ""
	case strings.HasPrefix(lang, "zh"):
		return "zh"
	case strings.HasPrefix(lang, "en"):
		return "en"
	default:
		return ""
	}
}

func deleteSummaryFiles(baseDir string, recordID int64) error {
	targetDir, err := buildRecordArtifactDir(baseDir, recordID)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(targetDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			continue
		}
		if strings.HasPrefix(name, "summary_") && strings.HasSuffix(name, ".txt") {
			if err := os.Remove(filepath.Join(targetDir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			continue
		}
		if strings.HasPrefix(name, "summary_") && strings.HasSuffix(name, ".embed") {
			if err := os.Remove(filepath.Join(targetDir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}
	return nil
}

// runConcurrent executes n jobs with up to maxTasks goroutines in flight.
// Results are returned in the original index order. The first error cancels remaining jobs.
func runConcurrent[T any](ctx context.Context, maxTasks, n int, fn func(ctx context.Context, i int) (T, error)) ([]T, error) {
	results := make([]T, n)
	if n == 0 {
		return results, nil
	}
	if maxTasks <= 1 || n == 1 {
		for i := range n {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			res, err := fn(ctx, i)
			if err != nil {
				return nil, err
			}
			results[i] = res
		}
		return results, nil
	}

	type jobResult struct {
		idx int
		val T
		err error
	}

	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	sem := make(chan struct{}, maxTasks)
	resultCh := make(chan jobResult, n)
	var wg sync.WaitGroup

	for i := range n {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			if cancelCtx.Err() != nil {
				resultCh <- jobResult{idx: idx, err: cancelCtx.Err()}
				return
			}
			sem <- struct{}{}
			defer func() { <-sem }()
			val, err := fn(cancelCtx, idx)
			if err != nil {
				cancel()
			}
			resultCh <- jobResult{idx: idx, val: val, err: err}
		}(i)
	}

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	var firstErr error
	for r := range resultCh {
		if r.err != nil && firstErr == nil {
			firstErr = r.err
		}
		if r.err == nil {
			results[r.idx] = r.val
		}
	}
	return results, firstErr
}

func buildSummaryTree(
	recordID int64,
	leafs []SummaryItem,
	groupSize int,
	maxTasks int,
	summarize func(level int, seqNo int, children []SummaryItem) (summaryGenerateResult, error),
) ([]SummaryItem, SummaryItem, error) {
	if len(leafs) == 0 {
		return nil, SummaryItem{}, errors.New("no leaf summaries")
	}
	if groupSize <= 0 {
		groupSize = DefaultSummaryGroupSize
	}
	all := append([]SummaryItem(nil), leafs...)
	current := append([]SummaryItem(nil), leafs...)
	level := 1
	for len(current) > 1 {
		numGroups := (len(current) + groupSize - 1) / groupSize
		groups := make([][]SummaryItem, numGroups)
		for i := range numGroups {
			end := min((i+1)*groupSize, len(current))
			groups[i] = append([]SummaryItem(nil), current[i*groupSize:end]...)
		}
		parents, err := runConcurrent(context.Background(), maxTasks, numGroups, func(workerCtx context.Context, i int) (SummaryItem, error) {
			if workerCtx.Err() != nil {
				return SummaryItem{}, workerCtx.Err()
			}
			seqNo := i + 1
			res, err := summarize(level, seqNo, groups[i])
			if err != nil {
				return SummaryItem{}, err
			}
			return SummaryItem{
				SummaryID:           buildSummaryID(recordID, level, seqNo),
				RecordID:            recordID,
				Level:               level,
				SeqNo:               seqNo,
				Lines:               mergeSummaryLineRanges(groups[i]),
				Children:            collectSummaryIDs(groups[i]),
				Keywords:            res.Keywords,
				KeywordsEn:          res.KeywordsEn,
				CategoryPaths:       res.CategoryPaths,
				CategoryNodes:       res.CategoryNodes,
				CategoryPathItems:   res.CategoryPathItems,
				CategoryPathItemsEn: res.CategoryPathItemsEn,
				Summary:             sanitizeTopicText(res.Summary),
				SummaryEn:           sanitizeTopicText(res.SummaryEn),
			}, nil
		})
		if err != nil {
			return nil, SummaryItem{}, err
		}
		all = append(all, parents...)
		current = parents
		level++
	}
	return all, current[0], nil
}

func mergeSummaryLineRanges(items []SummaryItem) []string {
	nums := make([]int, 0, len(items)*2)
	for _, item := range items {
		for _, span := range item.Lines {
			nums = append(nums, expandLineSpan(span)...)
		}
	}
	return lineRangesFromNumbers(nums)
}

func expandLineSpan(span string) []int {
	span = strings.Trim(strings.TrimSpace(span), "[]")
	if span == "" {
		return nil
	}
	if !strings.Contains(span, "-") {
		n, err := strconv.Atoi(span)
		if err != nil || n <= 0 {
			return nil
		}
		return []int{n}
	}
	parts := strings.SplitN(span, "-", 2)
	start, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	end, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || start <= 0 || end < start {
		return nil
	}
	out := make([]int, 0, end-start+1)
	for i := start; i <= end; i++ {
		out = append(out, i)
	}
	return out
}

func lineRangesFromNumbers(nums []int) []string {
	if len(nums) == 0 {
		return []string{}
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
		return []string{}
	}
	out := make([]string, 0, len(deduped))
	start := deduped[0]
	prev := deduped[0]
	for i := 1; i < len(deduped); i++ {
		n := deduped[i]
		if n == prev+1 {
			prev = n
			continue
		}
		out = append(out, formatLineNumberRange(start, prev))
		start = n
		prev = n
	}
	out = append(out, formatLineNumberRange(start, prev))
	return out
}

func collectSummaryIDs(items []SummaryItem) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.SummaryID) == "" {
			continue
		}
		out = append(out, strings.TrimSpace(item.SummaryID))
	}
	return out
}

// upsertCategoryDirMetadata creates metadata.txt in dir if it does not exist,
// or merges new keywords not already present if the file exists.
// This covers both new directories and existing directories that are missing
// the file (edge case: directory pre-existed without a metadata.txt).
func upsertCategoryDirMetadata(dir, name, categoryType string, confidence float64, keywords []string, now time.Time) error {
	return upsertCategoryDirMetadataLocalized(dir, name, name, categoryType, confidence, keywords, nil, now)
}

func upsertCategoryDirMetadataLocalized(
	dir string,
	originalName string,
	englishName string,
	categoryType string,
	confidence float64,
	originalKeywords []string,
	englishKeywords []string,
	now time.Time,
) error {
	metaPath := filepath.Join(dir, categoryMetadataFileName)
	return upsertLocalizedCategoryMetadata(metaPath, localizedCategoryMetadata{
		OriginalNames: appendUniqueString([]string(nil), strings.TrimSpace(originalName)),
		Desc:          firstNonEmptyTrimmed(originalName, englishName),
		DescEn:        localizedEnglishDesc(originalName, englishName),
		CategoryType:  strings.TrimSpace(categoryType),
		Confidence:    confidence,
		Keywords:      trimStringSlice(originalKeywords),
		KeywordsEn:    localizedEnglishKeywords(originalName, englishName, englishKeywords),
	}, now)
}

func localizedEnglishDesc(originalName string, englishName string) string {
	original := strings.TrimSpace(originalName)
	english := strings.TrimSpace(englishName)
	if english == "" || english == original {
		return ""
	}
	return english
}

func localizedEnglishKeywords(originalName string, englishName string, englishKeywords []string) []string {
	if localizedEnglishDesc(originalName, englishName) == "" {
		return nil
	}
	return trimStringSlice(englishKeywords)
}

// writeSummaryTreeEntry appends summary.SummaryID to
// baseDir/<category_path>/summaries.txt. The caller must create baseDir and
// remove stale entries for the record before the first call.
// pathNodes optionally provides per-node metadata (keywords, confidence) for
// each segment of the category path; nil is safe and falls back to defaults.
func writeSummaryTreeEntry(logger ApiTypes.JimoLogger, baseDir string, summary SummaryItem, rawCategories []string, pathNodes []CategoryPathNode) error {
	return writeSummaryTreeEntryLocalized(logger, baseDir, summary, rawCategories, pathNodes, nil)
}

func writeSummaryTreeEntryLocalized(logger ApiTypes.JimoLogger, baseDir string, summary SummaryItem, rawCategories []string, pathNodes []CategoryPathNode, originalPathNodes []CategoryPathNode) error {
	categoryPath, reason := normalizeAndValidateTopicCategoryPath(rawCategories, defaultSummaryTreeFallbackTopicType)
	if reason != "" {
		return fmt.Errorf("(MID_26042930) summary tree category path invalid (%s): %v", reason, rawCategories)
	}
	for _, seg := range categoryPath {
		if seg == "uncategorized" {
			return fmt.Errorf("(MID_26042931) summary tree category path must not contain 'uncategorized': %v", categoryPath)
		}
	}
	leaf := filepath.Join(append([]string{baseDir}, append(categoryPath, "summaries.txt")...)...)
	leafDir := filepath.Dir(leaf)
	if _, err := os.Stat(leafDir); os.IsNotExist(err) {
		if logger != nil {
			logger.Info("creating new summary tree directory", "path", leafDir)
		}
	}
	if err := os.MkdirAll(leafDir, 0o755); err != nil {
		return err
	}
	currentDir := baseDir
	for i, seg := range categoryPath {
		currentDir = filepath.Join(currentDir, seg)
		name, confidence := seg, 0.0
		keywords, keywordsEn := []string(nil), []string(nil)
		if i < len(pathNodes) {
			if pathNodes[i].Name != "" {
				name = pathNodes[i].Name
			}
			confidence = pathNodes[i].Confidence
			keywords = pathNodes[i].Keywords
		}
		originalName := name
		if i < len(originalPathNodes) && strings.TrimSpace(originalPathNodes[i].Name) != "" {
			originalName = originalPathNodes[i].Name
			if len(originalPathNodes[i].Keywords) > 0 {
				keywords = originalPathNodes[i].Keywords
				keywordsEn = pathNodes[i].Keywords
			}
		}
		if err := upsertCategoryDirMetadataLocalized(currentDir, originalName, name, defaultSummaryTreeFallbackTopicType, confidence, keywords, keywordsEn, time.Now()); err != nil {
			return err
		}
	}
	existing := make([]string, 0)
	if bs, err := os.ReadFile(leaf); err == nil {
		for _, row := range strings.Split(string(bs), "\n") {
			row = strings.TrimSpace(row)
			if row != "" {
				existing = append(existing, row)
			}
		}
	}
	existing = appendUniqueString(existing, strings.TrimSpace(summary.SummaryID))
	sort.Strings(existing)
	return os.WriteFile(leaf, []byte(strings.Join(existing, "\n")), 0o644)
}

func writeSummaryTreeRootReference(logger ApiTypes.JimoLogger, baseDir string, recordID int64, root SummaryItem, rawCategories []string, pathNodes []CategoryPathNode) error {
	return writeSummaryTreeRootReferenceLocalized(logger, baseDir, recordID, root, rawCategories, pathNodes, nil)
}

func writeSummaryTreeRootReferenceLocalized(logger ApiTypes.JimoLogger, baseDir string, recordID int64, root SummaryItem, rawCategories []string, pathNodes []CategoryPathNode, originalPathNodes []CategoryPathNode) error {
	if strings.TrimSpace(baseDir) == "" {
		return errors.New("summary tree dir is empty")
	}
	if recordID <= 0 {
		return fmt.Errorf("invalid record id: %d", recordID)
	}
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return err
	}
	if err := removeSummaryTreeRecord(baseDir, recordID); err != nil {
		return err
	}
	categoryPath, reason := normalizeAndValidateTopicCategoryPath(rawCategories, defaultSummaryTreeFallbackTopicType)
	if reason != "" {
		return fmt.Errorf("(MID_26042915) summary tree category path invalid (%s): %v", reason, rawCategories)
	}
	for _, seg := range categoryPath {
		if seg == "uncategorized" {
			return fmt.Errorf("(MID_26042916) summary tree category path must not contain 'uncategorized': %v", categoryPath)
		}
	}
	leaf := filepath.Join(append([]string{baseDir}, append(categoryPath, "summaries.txt")...)...)
	leafDir := filepath.Dir(leaf)
	if _, err := os.Stat(leafDir); os.IsNotExist(err) {
		if logger != nil {
			logger.Info("creating new summary tree directory", "path", leafDir)
		}
	}
	if err := os.MkdirAll(leafDir, 0o755); err != nil {
		return err
	}
	currentDir := baseDir
	for i, seg := range categoryPath {
		currentDir = filepath.Join(currentDir, seg)
		name, confidence := seg, 0.0
		keywords, keywordsEn := []string(nil), []string(nil)
		if i < len(pathNodes) {
			if pathNodes[i].Name != "" {
				name = pathNodes[i].Name
			}
			confidence = pathNodes[i].Confidence
			keywords = pathNodes[i].Keywords
		}
		originalName := name
		if i < len(originalPathNodes) && strings.TrimSpace(originalPathNodes[i].Name) != "" {
			originalName = originalPathNodes[i].Name
			if len(originalPathNodes[i].Keywords) > 0 {
				keywords = originalPathNodes[i].Keywords
				keywordsEn = pathNodes[i].Keywords
			}
		}
		if err := upsertCategoryDirMetadataLocalized(currentDir, originalName, name, defaultSummaryTreeFallbackTopicType, confidence, keywords, keywordsEn, time.Now()); err != nil {
			return err
		}
	}
	existing := make([]string, 0)
	if bs, err := os.ReadFile(leaf); err == nil {
		for _, row := range strings.Split(string(bs), "\n") {
			row = strings.TrimSpace(row)
			if row != "" {
				existing = append(existing, row)
			}
		}
	}
	existing = appendUniqueString(existing, strings.TrimSpace(root.SummaryID))
	sort.Strings(existing)
	return os.WriteFile(leaf, []byte(strings.Join(existing, "\n")), 0o644)
}

func removeSummaryTreeRecord(baseDir string, recordID int64) error {
	prefix := strconv.FormatInt(recordID, 10) + "_"
	return filepath.WalkDir(baseDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != "summaries.txt" {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rows := make([]string, 0)
		for _, row := range strings.Split(string(body), "\n") {
			row = strings.TrimSpace(row)
			if row == "" || strings.HasPrefix(row, prefix) {
				continue
			}
			rows = append(rows, row)
		}
		if len(rows) == 0 {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			return nil
		}
		sort.Strings(rows)
		return os.WriteFile(path, []byte(strings.Join(rows, "\n")), 0o644)
	})
}

func readSummaryClusterState(clusterDir string) (summaryClusterState, error) {
	state := summaryClusterState{NextClusterID: 1}
	if strings.TrimSpace(clusterDir) == "" {
		return state, errors.New("summary cluster dir is empty")
	}
	path := filepath.Join(clusterDir, defaultSummaryClusterStateFileName)
	bs, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return state, nil
		}
		return state, err
	}
	if err := json.Unmarshal(bs, &state); err != nil {
		return state, err
	}
	if state.NextClusterID <= 0 {
		state.NextClusterID = 1
	}
	return state, nil
}

func writeSummaryClusterState(clusterDir string, state summaryClusterState) error {
	if strings.TrimSpace(clusterDir) == "" {
		return errors.New("summary cluster dir is empty")
	}
	if state.NextClusterID <= 0 {
		state.NextClusterID = 1
	}
	if err := os.MkdirAll(clusterDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(clusterDir, defaultSummaryClusterStateFileName)
	bs, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, bs, 0o644)
}

/*
func summaryClusterFileName(clusterID string, label string) string {
	slug := slugifyClusterLabel(label)
	if slug == "" {
		slug = "cluster"
	}
	return clusterID + "_" + slug + ".md"
}

func slugifyClusterLabel(label string) string {
	label = strings.ToLower(strings.TrimSpace(label))
	if label == "" {
		return ""
	}
	var b strings.Builder
	lastUnderscore := false
	for _, r := range label {
		isAlphaNum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		isCJK := r >= '\u4e00' && r <= '\u9fff'
		if isAlphaNum || isCJK {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteRune('_')
			lastUnderscore = true
		}
	}
	out := strings.Trim(b.String(), "_")
	for strings.Contains(out, "__") {
		out = strings.ReplaceAll(out, "__", "_")
	}
	if len(out) > 60 {
		out = strings.Trim(out[:60], "_")
	}
	return out
}

func clusterSummariesForRecord(
	clusterDir string,
	recordID int64,
	summaries []SummaryItem,
	now time.Time,
	threshold float64,
	reclusteringDays int,
	labeler func([]SummaryItem) (string, error),
) error {
	if strings.TrimSpace(clusterDir) == "" {
		return errors.New("summary cluster dir is empty")
	}
	if err := os.MkdirAll(clusterDir, 0o755); err != nil {
		return err
	}
	state, err := readSummaryClusterState(clusterDir)
	if err != nil {
		return err
	}
	clusters, err := readSummaryClusters(clusterDir)
	if err != nil {
		return err
	}
	clusters = removeRecordFromClusters(clusters, recordID)
	candidates := summaryClusterCandidates(summaries)
	for _, summary := range candidates {
		bestIdx := -1
		bestScore := -1.0
		for i := range clusters {
			score := summarySimilarity(summary.Summary, clusterSimilarityText(clusters[i]))
			if score > bestScore {
				bestScore = score
				bestIdx = i
			}
		}
		if bestIdx >= 0 && bestScore >= threshold {
			clusters[bestIdx].SummaryIDs = appendUniqueString(clusters[bestIdx].SummaryIDs, summary.SummaryID)
			clusters[bestIdx].RepresentativeIDs = representativeSummaryIDs(clusters[bestIdx].SummaryIDs)
			clusters[bestIdx].UpdatedAt = now.Format("2006-01-02")
			continue
		}
		clusterID := fmt.Sprintf("cluster_%06d", state.NextClusterID)
		state.NextClusterID++
		label := summary.Summary
		if labeler != nil {
			customLabel, err := labeler([]SummaryItem{summary})
			if err != nil {
				return err
			}
			if strings.TrimSpace(customLabel) != "" {
				label = customLabel
			}
		}
		label = sanitizeClusterLabel(label)
		clusters = append(clusters, SummaryCluster{
			ClusterID:         clusterID,
			ClusterName:       label,
			ClusterLevel:      fmt.Sprintf("level_%d", summary.Level),
			CreatedAt:         now.Format("2006-01-02"),
			UpdatedAt:         now.Format("2006-01-02"),
			SummaryIDs:        []string{summary.SummaryID},
			RepresentativeIDs: []string{summary.SummaryID},
			ClusterSummary:    sanitizeTopicText(summary.Summary),
		})
	}
	if reclusteringDays > 0 {
		if state.LastFullReclusterAt.IsZero() || now.Sub(state.LastFullReclusterAt) >= time.Duration(reclusteringDays)*24*time.Hour {
			state.LastFullReclusterAt = now
		}
	}
	if err := writeSummaryClusters(clusterDir, clusters); err != nil {
		return err
	}
	return writeSummaryClusterState(clusterDir, state)
}

func summaryClusterCandidates(summaries []SummaryItem) []SummaryItem {
	out := make([]SummaryItem, 0, len(summaries))
	for _, item := range summaries {
		if item.Level >= 1 {
			out = append(out, item)
		}
	}
	if len(out) == 0 {
		return append([]SummaryItem(nil), summaries...)
	}
	return out
}

func summarySimilarity(a string, b string) float64 {
	left := tokenSet(a)
	right := tokenSet(b)
	if len(left) == 0 || len(right) == 0 {
		return 0
	}
	intersection := 0
	union := make(map[string]struct{}, len(left)+len(right))
	for token := range left {
		union[token] = struct{}{}
		if _, ok := right[token]; ok {
			intersection++
		}
	}
	for token := range right {
		union[token] = struct{}{}
	}
	return float64(intersection) / float64(len(union))
}

func tokenSet(text string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, token := range strings.Fields(strings.ToLower(strings.TrimSpace(text))) {
		token = strings.Trim(token, ".,;:!?()[]{}\"'")
		if len(token) < 2 {
			continue
		}
		out[token] = struct{}{}
	}
	return out
}
*/

// chineseDocRefPrefixes lists common document-referential opener phrases that add
// no semantic value as cluster labels (e.g. "本标准规定了", "该文本列举了").
// Longer/more-specific entries must come before shorter/overlapping ones.
var chineseDocRefPrefixes = []string{
	"本标准规定了", "本标准列举了", "本标准说明了", "本标准描述了",
	"本标准涉及", "本标准包含了", "本标准阐述了", "本标准概述了",
	"本标准介绍了", "本标准强调了", "本标准指出了", "本标准提到了",
	"本标准分析了", "本标准总结了", "本标准讨论了",
	"该文本规定了", "该文本列举了", "该文本说明了", "该文本描述了",
	"该文本涉及", "该文本包含了", "该文本阐述了", "该文本概述了",
	"该文件规定了", "该文件列举了", "该文件说明了", "该文件描述了",
	"该文件涉及", "该文件包含了",
	"该文档规定了", "该文档列举了",
	"本文规定了", "本文列举了", "本文说明了", "本文描述了",
	"本规范规定了", "本规范列举了",
	"本标准", "该文本", "该文件", "该文档", "本规范",
}

func stripChineseDocRefPrefix(s string) string {
	for _, prefix := range chineseDocRefPrefixes {
		if strings.HasPrefix(s, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(s, prefix))
		}
	}
	return s
}

func sanitizeClusterLabel(text string) string {
	text = sanitizeTopicText(text)
	if text == "" {
		return "Cluster"
	}
	text = stripChineseDocRefPrefix(text)
	text = strings.TrimSpace(text)
	if text == "" {
		return "Cluster"
	}
	if detectContentLanguage(text) == "zh" {
		runes := []rune(text)
		if len(runes) > 20 {
			text = strings.TrimSpace(string(runes[:20]))
		}
	} else {
		parts := strings.Fields(text)
		if len(parts) > 6 {
			parts = parts[:6]
		}
		text = strings.Join(parts, " ")
	}
	if text == "" {
		return "Cluster"
	}
	return text
}

/*
func clusterSimilarityText(cluster SummaryCluster) string {
	if strings.TrimSpace(cluster.ClusterSummary) != "" {
		return cluster.ClusterSummary
	}
	return cluster.ClusterName
}

func representativeSummaryIDs(ids []string) []string {
	if len(ids) <= 5 {
		return append([]string(nil), ids...)
	}
	return append([]string(nil), ids[:5]...)
}

func removeRecordFromClusters(clusters []SummaryCluster, recordID int64) []SummaryCluster {
	out := make([]SummaryCluster, 0, len(clusters))
	for _, cluster := range clusters {
		filtered := cluster.SummaryIDs[:0]
		for _, id := range cluster.SummaryIDs {
			if parseSummaryIDRecordID(id) == recordID {
				continue
			}
			filtered = append(filtered, id)
		}
		cluster.SummaryIDs = append([]string(nil), filtered...)
		cluster.RepresentativeIDs = representativeSummaryIDs(cluster.SummaryIDs)
		if len(cluster.SummaryIDs) == 0 {
			continue
		}
		out = append(out, cluster)
	}
	return out
}

func parseSummaryIDRecordID(summaryID string) int64 {
	parts := strings.Split(strings.TrimSpace(summaryID), "_")
	if len(parts) != 3 {
		return 0
	}
	id, _ := strconv.ParseInt(parts[0], 10, 64)
	return id
}

func readSummaryClusters(clusterDir string) ([]SummaryCluster, error) {
	entries, err := os.ReadDir(clusterDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []SummaryCluster{}, nil
		}
		return nil, err
	}
	out := make([]SummaryCluster, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "cluster_") || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		cluster, err := parseSummaryClusterFile(filepath.Join(clusterDir, entry.Name()))
		if err != nil {
			return nil, err
		}
		out = append(out, cluster)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ClusterID < out[j].ClusterID })
	return out, nil
}

func parseSummaryClusterFile(path string) (SummaryCluster, error) {
	bs, err := os.ReadFile(path)
	if err != nil {
		return SummaryCluster{}, err
	}
	text := string(bs)
	cluster := SummaryCluster{}
	lines := strings.Split(text, "\n")
	section := ""
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "cluster_id:"):
			cluster.ClusterID = strings.TrimSpace(strings.TrimPrefix(trimmed, "cluster_id:"))
		case strings.HasPrefix(trimmed, "cluster_name:"):
			cluster.ClusterName = strings.TrimSpace(strings.TrimPrefix(trimmed, "cluster_name:"))
		case strings.HasPrefix(trimmed, "cluster_level:"):
			cluster.ClusterLevel = strings.TrimSpace(strings.TrimPrefix(trimmed, "cluster_level:"))
		case strings.HasPrefix(trimmed, "created_at:"):
			cluster.CreatedAt = strings.TrimSpace(strings.TrimPrefix(trimmed, "created_at:"))
		case strings.HasPrefix(trimmed, "updated_at:"):
			cluster.UpdatedAt = strings.TrimSpace(strings.TrimPrefix(trimmed, "updated_at:"))
		case trimmed == "## Cluster Summary":
			section = "summary"
		case trimmed == "## Representative Summaries":
			section = "representative"
		case trimmed == "## Source Summaries":
			section = "source"
		case trimmed == "## Related clusters":
			section = "related"
		case strings.HasPrefix(trimmed, "# "):
			continue
		case trimmed == "":
			continue
		default:
			switch section {
			case "summary":
				cluster.ClusterSummary = sanitizeTopicText(trimmed)
			case "representative":
				cluster.RepresentativeIDs = splitCommaList(trimmed)
			case "source":
				cluster.SummaryIDs = splitCommaList(trimmed)
			case "related":
				cluster.RelatedClusterIDs = splitCommaList(trimmed)
			}
		}
	}
	return cluster, nil
}

func splitCommaList(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(part), "-"))
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func writeSummaryClusters(clusterDir string, clusters []SummaryCluster) error {
	if err := os.MkdirAll(clusterDir, 0o755); err != nil {
		return err
	}
	existing, err := filepath.Glob(filepath.Join(clusterDir, "cluster_*.md"))
	if err != nil {
		return err
	}
	for _, path := range existing {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	sort.Slice(clusters, func(i, j int) bool { return clusters[i].ClusterID < clusters[j].ClusterID })
	for _, cluster := range clusters {
		path := filepath.Join(clusterDir, summaryClusterFileName(cluster.ClusterID, cluster.ClusterName))
		var b strings.Builder
		b.WriteString(fmt.Sprintf("cluster_id: %s\n", cluster.ClusterID))
		b.WriteString(fmt.Sprintf("cluster_name: %s\n", cluster.ClusterName))
		b.WriteString(fmt.Sprintf("cluster_level: %s\n", cluster.ClusterLevel))
		b.WriteString(fmt.Sprintf("created_at: %s\n", cluster.CreatedAt))
		b.WriteString(fmt.Sprintf("updated_at: %s\n", cluster.UpdatedAt))
		b.WriteString(fmt.Sprintf("summary_count: %d\n\n", len(cluster.SummaryIDs)))
		b.WriteString("# " + cluster.ClusterName + "\n\n")
		b.WriteString("## Cluster Summary\n\n")
		summaryText := strings.TrimSpace(cluster.ClusterSummary)
		if summaryText == "" {
			summaryText = cluster.ClusterName
		}
		b.WriteString(summaryText + "\n\n")
		b.WriteString("## Representative Summaries\n\n")
		b.WriteString(strings.Join(cluster.RepresentativeIDs, ", ") + "\n\n")
		b.WriteString("## Source Summaries\n\n")
		b.WriteString(strings.Join(cluster.SummaryIDs, ", ") + "\n\n")
		b.WriteString("## Related clusters\n\n")
		b.WriteString(strings.Join(cluster.RelatedClusterIDs, ", ") + "\n")
		if err := os.WriteFile(path, []byte(strings.TrimRight(b.String(), "\n")), 0o644); err != nil {
			return err
		}
	}
	return nil
}
*/

// detectContentLanguage returns "zh" when more than 20% of non-whitespace runes
// are CJK characters, otherwise "en".
func detectContentLanguage(text string) string {
	total, cjk := 0, 0
	for _, r := range text {
		if r == ' ' || r == '\n' || r == '\t' || r == '\r' {
			continue
		}
		total++
		if r >= '一' && r <= '鿿' {
			cjk++
		}
	}
	if total == 0 {
		return "en"
	}
	if float64(cjk)/float64(total) > 0.2 {
		return "zh"
	}
	return "en"
}

// appendLanguageInstruction appends a language directive to prompt based on the
// detected language of inputText, so the LLM generates the summary in the same
// language as the source content.
func appendLanguageInstruction(prompt string, inputText string) string {
	lang := detectContentLanguage(inputText)
	if lang == "zh" {
		return prompt + "\n\nIMPORTANT: Generate the summary in Chinese."
	}
	return prompt + "\n\nIMPORTANT: Generate the summary in English."
}

func appendUniqueString(items []string, v string) []string {
	for _, item := range items {
		if item == v {
			return items
		}
	}
	return append(items, v)
}

func formatQuotedArray(items []string) string {
	if len(items) == 0 {
		return "[]"
	}
	quoted := make([]string, 0, len(items))
	for _, item := range items {
		quoted = append(quoted, strconv.Quote(item))
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

func formatFloatArray(items []float64) string {
	if len(items) == 0 {
		return "[]"
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, strconv.FormatFloat(item, 'f', -1, 64))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}
