package docprocessing

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFindOrCreateCategorySubdirUsesEnglishDirectoryAndStoresOriginalName(t *testing.T) {
	tmp := t.TempDir()
	now := time.Date(2026, 5, 27, 12, 30, 0, 0, time.UTC)

	dir, err := findOrCreateCategorySubdir(nil, tmp,
		CategoryPathNode{Name: "Public Health", Keywords: []string{"health"}, Confidence: 0.9},
		CategoryPathNode{Name: "公共卫生", Keywords: []string{"公共卫生"}, Confidence: 0.8},
		now,
	)
	if err != nil {
		t.Fatalf("findOrCreateCategorySubdir: %v", err)
	}
	if got := filepath.Base(dir); got != "public_health" {
		t.Fatalf("dir=%q, want public_health", got)
	}

	body, err := os.ReadFile(filepath.Join(dir, topicCategoryMetaFileName))
	if err != nil {
		t.Fatalf("read metadata: %v", err)
	}
	text := string(body)
	for _, want := range []string{
		`"original_names":["公共卫生"]`,
		`"desc":"公共卫生"`,
		`"desc_en":"Public Health"`,
		`"keywords":["公共卫生"]`,
		`"keywords_en":["health"]`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("metadata missing %q, got:\n%s", want, text)
		}
	}
}

func TestPairCategoryPathEntriesPrefersEnglishPathForIndexing(t *testing.T) {
	pairs := pairCategoryPathEntries(
		[]any{
			map[string]any{
				"category_path": []any{
					map[string]any{"name": "公共卫生", "keywords": []any{"公共卫生"}, "confidence": 0.7},
					map[string]any{"name": "疾病预防", "keywords": []any{"防疫"}, "confidence": 0.8},
				},
			},
		},
		[]any{
			map[string]any{
				"category_path": []any{
					map[string]any{"name": "Public Health", "keywords": []any{"health"}, "confidence": 0.9},
					map[string]any{"name": "Disease Prevention", "keywords": []any{"prevention"}, "confidence": 0.95},
				},
			},
		},
	)
	if len(pairs) != 1 {
		t.Fatalf("pairs len=%d, want 1", len(pairs))
	}
	if got := pairs[0].Index.Nodes[0].Name; got != "Public Health" {
		t.Fatalf("index node name=%q", got)
	}
	if got := pairs[0].Original.Nodes[0].Name; got != "公共卫生" {
		t.Fatalf("original node name=%q", got)
	}
}

func TestWriteSummaryTreeEntryLocalizedUsesEnglishDirectories(t *testing.T) {
	tmp := t.TempDir()
	summary := SummaryItem{SummaryID: "10_1_0001", RecordID: 10, Level: 1, SeqNo: 1}
	indexNodes := []CategoryPathNode{
		{Name: "Public Health", Keywords: []string{"health"}, Confidence: 0.9},
		{Name: "Disease Prevention", Keywords: []string{"prevention"}, Confidence: 0.8},
	}
	originalNodes := []CategoryPathNode{
		{Name: "公共卫生", Keywords: []string{"公共卫生"}, Confidence: 0.9},
		{Name: "疾病预防", Keywords: []string{"防疫"}, Confidence: 0.8},
	}

	if err := writeSummaryTreeEntryLocalized(nil, tmp, summary, categoryPathNames(indexNodes), indexNodes, originalNodes); err != nil {
		t.Fatalf("writeSummaryTreeEntryLocalized: %v", err)
	}

	leaf := filepath.Join(tmp, "public_health", "disease_prevention", "summaries.txt")
	body, err := os.ReadFile(leaf)
	if err != nil {
		t.Fatalf("read summaries.txt: %v", err)
	}
	if strings.TrimSpace(string(body)) != "10_1_0001" {
		t.Fatalf("unexpected summaries.txt content: %q", string(body))
	}

	metaBody, err := os.ReadFile(filepath.Join(tmp, "public_health", categoryMetadataFileName))
	if err != nil {
		t.Fatalf("read metadata.txt: %v", err)
	}
	if !strings.Contains(string(metaBody), `"original_names":["公共卫生"]`) {
		t.Fatalf("expected original name in metadata, got:\n%s", string(metaBody))
	}
}

func TestIndexTopicsInTreeDirUsesEnglishDirectoryAndStoresOriginalName(t *testing.T) {
	tmp := t.TempDir()
	topic := TopicItem{
		SeqNo:        1,
		TopicType:    "compliance",
		Lines:        []string{"1-2"},
		Keywords:     []string{"气体防护站"},
		KeywordsEn:   []string{"gas protection station"},
		Topic:        "气体防护站设计规范",
		TopicEn:      "Gas protection station design code",
		CategoryPath: []string{"石油天然气", "行业标准"},
		CategoryPathDetail: []CategoryPathEntry{{
			Nodes: []CategoryPathNode{
				{Name: "石油天然气", Keywords: []string{"石油"}, Confidence: 0.9},
				{Name: "行业标准", Keywords: []string{"标准"}, Confidence: 0.9},
			},
		}},
		CategoryPathDetailEn: []CategoryPathEntry{{
			Nodes: []CategoryPathNode{
				{Name: "Petroleum and Natural Gas", Keywords: []string{"petroleum"}, Confidence: 0.95},
				{Name: "Industry Standards", Keywords: []string{"standards"}, Confidence: 0.95},
			},
		}},
	}

	if err := indexTopicsInTreeDir(nil, tmp, 127, []TopicItem{topic}); err != nil {
		t.Fatalf("indexTopicsInTreeDir: %v", err)
	}

	leaf := filepath.Join(tmp, "petroleum_and_natural_gas", "industry_standards", topicsFileName)
	body, err := os.ReadFile(leaf)
	if err != nil {
		t.Fatalf("read indexed topics: %v", err)
	}
	if !strings.Contains(string(body), "record_id: 127") {
		t.Fatalf("topics.txt missing record id, got:\n%s", string(body))
	}
	if _, err := os.Stat(filepath.Join(tmp, "石油天然气")); !os.IsNotExist(err) {
		t.Fatalf("unexpected non-English category dir created: err=%v", err)
	}

	metaBody, err := os.ReadFile(filepath.Join(tmp, "petroleum_and_natural_gas", topicCategoryMetaFileName))
	if err != nil {
		t.Fatalf("read metadata.txt: %v", err)
	}
	if !strings.Contains(string(metaBody), `"original_names":["石油天然气"]`) {
		t.Fatalf("expected original name in metadata, got:\n%s", string(metaBody))
	}
}
