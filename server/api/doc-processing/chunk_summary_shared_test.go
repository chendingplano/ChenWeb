package docprocessing

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildSummaryID(t *testing.T) {
	if got := buildSummaryID(93, 1, 12); got != "93_1_0012" {
		t.Fatalf("buildSummaryID=%q, want 93_1_0012", got)
	}
}

func TestSummaryFileName(t *testing.T) {
	if got := summaryFileName(2, 7); got != "summary_2_0007.txt" {
		t.Fatalf("summaryFileName=%q, want summary_2_0007.txt", got)
	}
}

func TestWriteSummaryFile(t *testing.T) {
	tmp := t.TempDir()
	item := SummaryItem{
		SummaryID: "93_1_0001",
		RecordID:  93,
		Level:     1,
		SeqNo:     1,
		Lines:     []string{"1-4"},
		Children:  []string{"93_0_0001", "93_0_0002"},
		Summary:   "Parent summary text",
	}

	path, err := writeSummaryFile(tmp, 93, item)
	if err != nil {
		t.Fatalf("writeSummaryFile: %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read summary file: %v", err)
	}
	text := string(body)
	for _, want := range []string{
		`summary_id: "93_1_0001"`,
		"record_id: 93",
		"children: [\"93_0_0001\", \"93_0_0002\"]",
		"summary_begin:",
		"Parent summary text",
		"summary_end",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected summary file to contain %q, got %q", want, text)
		}
	}
}

func TestBuildSummaryTree(t *testing.T) {
	leafs := []SummaryItem{
		{SummaryID: "93_0_0001", RecordID: 93, Level: 0, SeqNo: 1, Lines: []string{"1-2"}, Summary: "A"},
		{SummaryID: "93_0_0002", RecordID: 93, Level: 0, SeqNo: 2, Lines: []string{"3-4"}, Summary: "B"},
		{SummaryID: "93_0_0003", RecordID: 93, Level: 0, SeqNo: 3, Lines: []string{"5-6"}, Summary: "C"},
		{SummaryID: "93_0_0004", RecordID: 93, Level: 0, SeqNo: 4, Lines: []string{"7-8"}, Summary: "D"},
	}

	all, root, err := buildSummaryTree(93, leafs, 2, func(level int, seqNo int, children []SummaryItem) (string, []string, error) {
		ids := make([]string, 0, len(children))
		for _, child := range children {
			ids = append(ids, child.SummaryID)
		}
		return "L" + asString(level) + ":" + strings.Join(ids, ","), nil, nil
	})
	if err != nil {
		t.Fatalf("buildSummaryTree: %v", err)
	}
	if len(all) != 7 {
		t.Fatalf("summary count=%d, want 7", len(all))
	}
	if root.SummaryID != "93_2_0001" {
		t.Fatalf("root summary id=%q, want 93_2_0001", root.SummaryID)
	}
	if got := strings.Join(root.Children, ","); got != "93_1_0001,93_1_0002" {
		t.Fatalf("root children=%q, want 93_1_0001,93_1_0002", got)
	}
	if got := strings.Join(root.Lines, ","); got != "1-8" {
		t.Fatalf("root lines=%q, want 1-8", got)
	}
}

func TestWriteSummaryTreeRootReference(t *testing.T) {
	tmp := t.TempDir()
	root := SummaryItem{SummaryID: "53_1_0001", RecordID: 53, Level: 1, SeqNo: 1}

	if err := writeSummaryTreeRootReference(nil, tmp, 53, root, []string{"Safety Overview", "Closing Notes"}, nil); err != nil {
		t.Fatalf("writeSummaryTreeRootReference: %v", err)
	}
	leaf := filepath.Join(tmp, "safety_overview", "closing_notes", "summaries.txt")
	body, err := os.ReadFile(leaf)
	if err != nil {
		t.Fatalf("read summaries.txt: %v", err)
	}
	if strings.TrimSpace(string(body)) != "53_1_0001" {
		t.Fatalf("unexpected summaries.txt content: %q", string(body))
	}

	// Reprocessing the same record replaces its prior entry and does not duplicate it.
	root2 := SummaryItem{SummaryID: "53_2_0001", RecordID: 53, Level: 2, SeqNo: 1}
	if err := writeSummaryTreeRootReference(nil, tmp, 53, root2, []string{"Safety Overview", "Closing Notes"}, nil); err != nil {
		t.Fatalf("rewrite summary tree root reference: %v", err)
	}
	body, err = os.ReadFile(leaf)
	if err != nil {
		t.Fatalf("read summaries.txt after rewrite: %v", err)
	}
	if strings.TrimSpace(string(body)) != "53_2_0001" {
		t.Fatalf("unexpected rewritten summaries.txt content: %q", string(body))
	}

	// A second record writing to the same leaf must not overwrite the first record's entry.
	root3 := SummaryItem{SummaryID: "77_1_0001", RecordID: 77, Level: 1, SeqNo: 1}
	if err := writeSummaryTreeRootReference(nil, tmp, 77, root3, []string{"Safety Overview", "Closing Notes"}, nil); err != nil {
		t.Fatalf("write second record summary tree root reference: %v", err)
	}
	body, err = os.ReadFile(leaf)
	if err != nil {
		t.Fatalf("read summaries.txt after second record: %v", err)
	}
	got := strings.TrimSpace(string(body))
	if !strings.Contains(got, "53_2_0001") {
		t.Fatalf("expected summaries.txt to still contain 53_2_0001, got: %q", got)
	}
	if !strings.Contains(got, "77_1_0001") {
		t.Fatalf("expected summaries.txt to contain 77_1_0001, got: %q", got)
	}

	// Reprocessing record 77 replaces its entry and preserves record 53's entry.
	root4 := SummaryItem{SummaryID: "77_2_0001", RecordID: 77, Level: 2, SeqNo: 1}
	if err := writeSummaryTreeRootReference(nil, tmp, 77, root4, []string{"Safety Overview", "Closing Notes"}, nil); err != nil {
		t.Fatalf("reprocess record 77: %v", err)
	}
	body, err = os.ReadFile(leaf)
	if err != nil {
		t.Fatalf("read summaries.txt after reprocess: %v", err)
	}
	got = strings.TrimSpace(string(body))
	if strings.Contains(got, "77_1_0001") {
		t.Fatalf("expected old 77 entry to be removed, got: %q", got)
	}
	if !strings.Contains(got, "77_2_0001") {
		t.Fatalf("expected new 77 entry in summaries.txt, got: %q", got)
	}
	if !strings.Contains(got, "53_2_0001") {
		t.Fatalf("expected record 53 entry preserved after reprocess of 77, got: %q", got)
	}
}

func TestDetectContentLanguage(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"Hello world, this is an English sentence.", "en"},
		{"这是一段中文内容，用于测试语言检测功能。", "zh"},
		{"Mixed content: 部分中文 and some English words here.", "en"},
		{"大量中文内容超过百分之二十，用于语言检测。额外中文。More.", "zh"},
		{"", "en"},
	}
	for _, tc := range cases {
		got := detectContentLanguage(tc.input)
		if got != tc.want {
			t.Errorf("detectContentLanguage(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestStripChineseDocRefPrefix(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"本标准规定了消防员职业健康管理的各项要求", "消防员职业健康管理的各项要求"},
		{"该文本列举了多种禁止或限制从事特定工作的情形", "多种禁止或限制从事特定工作的情形"},
		{"本标准规定了消防员职业健康管理的健康条件", "消防员职业健康管理的健康条件"},
		{"该文件涉及消防安全", "消防安全"},
		{"本标准消防安全管理规定", "消防安全管理规定"},
		{"消防安全管理规定", "消防安全管理规定"},
		{"Firefighter health standards", "Firefighter health standards"},
		{"", ""},
	}
	for _, tc := range cases {
		got := stripChineseDocRefPrefix(tc.input)
		if got != tc.want {
			t.Errorf("stripChineseDocRefPrefix(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestSanitizeClusterLabel_CJK(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{
			"本标准规定了消防员职业健康管理的各项要求",
			"消防员职业健康管理的各项要求",
		},
		{
			// 22 runes after strip → truncated to 20
			"该文本列举了多种禁止或限制从事特定工作的情形包括患有各种疾病",
			"多种禁止或限制从事特定工作的情形包括患有",
		},
		{
			// 12 words → truncated to 6
			"This is an English cluster label that is quite long indeed yes",
			"This is an English cluster label",
		},
		{"", "Cluster"},
		{"本标准", "Cluster"},
	}
	for _, tc := range cases {
		got := sanitizeClusterLabel(tc.input)
		if got != tc.want {
			t.Errorf("sanitizeClusterLabel(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestAppendLanguageInstruction(t *testing.T) {
	enPrompt := appendLanguageInstruction("Summarize this.", "Hello world, this is English content.")
	if !strings.Contains(enPrompt, "English") {
		t.Fatalf("expected English instruction, got: %q", enPrompt)
	}
	zhPrompt := appendLanguageInstruction("Summarize this.", "这是一段中文内容。")
	if !strings.Contains(zhPrompt, "Chinese") {
		t.Fatalf("expected Chinese instruction, got: %q", zhPrompt)
	}
}

func TestCategoryDirMetadata_CreateAndMerge(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "public_health")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)

	// Create a new metadata.txt.
	if err := upsertCategoryDirMetadata(dir, "public_health", "summary", 0.95, []string{"health management", "disease prevention"}, now); err != nil {
		t.Fatalf("upsertCategoryDirMetadata create: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(dir, categoryMetadataFileName))
	if err != nil {
		t.Fatalf("read metadata.txt: %v", err)
	}
	text := string(body)
	for _, want := range []string{`"desc":"public_health"`, `"category_type":"summary"`, `"confidence":0.95`, `"health management"`, `"disease prevention"`, `"create_time":"20260502-120000"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("metadata.txt missing %q, got:\n%s", want, text)
		}
	}

	// Merge new keywords — new keyword added, duplicate skipped.
	if err := upsertCategoryDirMetadata(dir, "public_health", "summary", 0.9, []string{"disease prevention", "immunization"}, now); err != nil {
		t.Fatalf("upsertCategoryDirMetadata merge: %v", err)
	}
	body2, err := os.ReadFile(filepath.Join(dir, categoryMetadataFileName))
	if err != nil {
		t.Fatalf("read metadata.txt after merge: %v", err)
	}
	text2 := string(body2)
	for _, want := range []string{`"health management"`, `"disease prevention"`, `"immunization"`} {
		if !strings.Contains(text2, want) {
			t.Fatalf("merged metadata.txt missing %q, got:\n%s", want, text2)
		}
	}
	// create_time must not change on update.
	if !strings.Contains(text2, `"create_time":"20260502-120000"`) {
		t.Fatalf("create_time changed on merge, got:\n%s", text2)
	}
}

func TestCategoryDirMetadata_ExistingDirWithoutFile(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "preexisting")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Directory exists but has no metadata.txt yet.
	metaPath := filepath.Join(dir, categoryMetadataFileName)
	if _, err := os.Stat(metaPath); !os.IsNotExist(err) {
		t.Fatalf("expected no metadata.txt before test")
	}

	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)
	if err := upsertCategoryDirMetadata(dir, "preexisting", "summary", 0.8, []string{"kw1"}, now); err != nil {
		t.Fatalf("upsertCategoryDirMetadata on existing dir without file: %v", err)
	}

	body, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("metadata.txt not created for existing dir: %v", err)
	}
	text := string(body)
	if !strings.Contains(text, `"desc":"preexisting"`) {
		t.Fatalf("missing desc in metadata.txt, got:\n%s", text)
	}
	if !strings.Contains(text, `"kw1"`) {
		t.Fatalf("missing keyword in metadata.txt, got:\n%s", text)
	}
}

func TestWriteSummaryTreeEntry_WritesMetadata(t *testing.T) {
	tmp := t.TempDir()
	summary := SummaryItem{SummaryID: "10_1_0001", RecordID: 10, Level: 1, SeqNo: 1}
	nodes := []CategoryPathNode{
		{Name: "Safety Overview", Keywords: []string{"safety", "overview"}, Confidence: 0.9},
		{Name: "Closing Notes", Keywords: []string{"closing"}, Confidence: 0.8},
	}
	if err := writeSummaryTreeEntry(nil, tmp, summary, []string{"Safety Overview", "Closing Notes"}, nodes); err != nil {
		t.Fatalf("writeSummaryTreeEntry: %v", err)
	}
	for _, tc := range []struct {
		dir  string
		want []string
	}{
		{
			filepath.Join(tmp, "safety_overview"),
			[]string{`"desc":"Safety Overview"`, `"safety"`, `"overview"`, `"confidence":0.9`},
		},
		{
			filepath.Join(tmp, "safety_overview", "closing_notes"),
			[]string{`"desc":"Closing Notes"`, `"closing"`, `"confidence":0.8`},
		},
	} {
		body, err := os.ReadFile(filepath.Join(tc.dir, categoryMetadataFileName))
		if err != nil {
			t.Fatalf("read metadata.txt in %s: %v", tc.dir, err)
		}
		text := string(body)
		for _, w := range tc.want {
			if !strings.Contains(text, w) {
				t.Fatalf("metadata.txt in %s missing %q, got:\n%s", tc.dir, w, text)
			}
		}
	}
}

func TestClusterStateRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	state := summaryClusterState{
		NextClusterID:       2,
		LastFullReclusterAt: time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC),
	}
	if err := writeSummaryClusterState(tmp, state); err != nil {
		t.Fatalf("writeSummaryClusterState: %v", err)
	}
	got, err := readSummaryClusterState(tmp)
	if err != nil {
		t.Fatalf("readSummaryClusterState: %v", err)
	}
	if got.NextClusterID != 2 {
		t.Fatalf("NextClusterID=%d, want 2", got.NextClusterID)
	}
	if got.LastFullReclusterAt.IsZero() {
		t.Fatalf("expected LastFullReclusterAt to be set")
	}
}
