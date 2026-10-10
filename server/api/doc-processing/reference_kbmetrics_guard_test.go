package docprocessing

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// kbMetricsReaderAllowlist names files whose kb.metrics readers deliberately see resolved reference
// rows, or carry the predicate somewhere this line-based check cannot see.
var kbMetricsReaderAllowlist = map[string]string{
	"api/kbhandler/metrics_handler.go":                  "metric list API: returns resolved rows with their ref_* fields",
	"api/kbhandler/metric_wiki_handler.go":              "display lookup of one metric by id",
	"api/kbhandler/metric_ontology_analysis_handler.go": "predicate is in the shared where clause",
	"api/doc-processing/reference_metric_resolver.go":   "copies one target row by primary key, chosen from own rows only",
}

// TestKBMetricsReadersExcludeResolvedRows enforces the reader default of openspec change
// extract-references-processor (design D13): every non-test query reading kb.metrics excludes
// resolved reference rows (ref_mention_id IS NULL) or is allowlisted with a reason.
func TestKBMetricsReadersExcludeResolvedRows(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	site := regexp.MustCompile(`(?i)\b(FROM|JOIN)\s+kb\.metrics\b([^_]|$)`)
	var bad []string
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if _, ok := kbMetricsReaderAllowlist[filepath.ToSlash(rel)]; ok {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		lines := strings.Split(string(raw), "\n")
		for i, l := range lines {
			trim := strings.TrimSpace(l)
			if !site.MatchString(l) || strings.HasPrefix(trim, "//") || strings.Contains(strings.ToUpper(l), "DELETE FROM KB.METRICS") {
				continue
			}
			// The predicate must appear in the same SQL clause: this line and the following lines
			// up to the end of the string literal.
			ok := false
			for j := i; j < len(lines) && j < i+16; j++ {
				if strings.Contains(lines[j], "ref_mention_id") {
					ok = true
					break
				}
				if j > i && strings.Contains(lines[j], "`") {
					break
				}
			}
			if !ok {
				bad = append(bad, filepath.ToSlash(rel)+":"+strconv.Itoa(i+1)+": "+trim)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range bad {
		t.Errorf("kb.metrics reader without ref_mention_id predicate (add `ref_mention_id IS NULL` or allowlist with a reason): %s", b)
	}
}
