package docbenchmark

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadDatasetValid(t *testing.T) {
	root := writeDataset(t, validManifest(), validInput(), validExpected())
	ds, err := LoadDataset(root)
	if err != nil {
		t.Fatalf("LoadDataset: %v", err)
	}
	if ds.Manifest.DatasetVersion != "1.0.0" || len(ds.Cases) != 1 {
		t.Fatalf("unexpected dataset: %#v", ds)
	}
}

func TestLoadDatasetAcceptsSemVerAndStableMetricFields(t *testing.T) {
	var m, e map[string]any
	json.Unmarshal(validManifest(), &m)
	json.Unmarshal(validExpected(), &e)
	m["dataset_version"] = "1.2.3-rc.1+build.7"
	metric := e["extract_metrics"].(map[string]any)["metrics"].([]any)[0].(map[string]any)
	metric["metric_desc"] = "SLO threshold"
	metric["metric_unit_en"] = "milliseconds"
	metric["confidence"] = 0.99
	mb, _ := json.Marshal(m)
	eb, _ := json.Marshal(e)
	if _, err := LoadDataset(writeDataset(t, mb, validInput(), eb)); err != nil {
		t.Fatalf("LoadDataset rejected valid SemVer/stable fields: %v", err)
	}
}

func TestLoadDatasetRequiresSeedAndNonEmptyCases(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]any)
		want   string
	}{
		{"missing seed", func(m map[string]any) { delete(m, "seed") }, "seed: required"},
		{"missing cases", func(m map[string]any) { delete(m, "cases") }, "cases: required"},
		{"empty cases", func(m map[string]any) { m["cases"] = []any{} }, "cases: must not be empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m map[string]any
			json.Unmarshal(validManifest(), &m)
			tt.mutate(m)
			manifest, _ := json.Marshal(m)
			_, err := LoadDataset(writeDataset(t, manifest, validInput(), validExpected()))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error=%v, want %q", err, tt.want)
			}
		})
	}
}

func TestLoadDatasetAllowsPresentZeroSeed(t *testing.T) {
	var m map[string]any
	json.Unmarshal(validManifest(), &m)
	m["seed"] = 0
	manifest, _ := json.Marshal(m)
	if _, err := LoadDataset(writeDataset(t, manifest, validInput(), validExpected())); err != nil {
		t.Fatalf("present zero seed rejected: %v", err)
	}
}

func TestLoadDatasetRejectsManifestAndFilesystemViolations(t *testing.T) {
	tests := []struct {
		name string
		edit func(t *testing.T, root string, m map[string]any)
		want []string
	}{
		{"invalid semver", func(_ *testing.T, _ string, m map[string]any) { m["dataset_version"] = "1.0" }, []string{"dataset_version"}},
		{"unsupported schema", func(_ *testing.T, _ string, m map[string]any) { m["schema_version"] = 2 }, []string{"schema_version"}},
		{"duplicate case", func(_ *testing.T, _ string, m map[string]any) { m["cases"] = append(m["cases"].([]any), cloneCase(m)) }, []string{"metric-001", "cases[1].case_id"}},
		{"invalid case id", func(_ *testing.T, _ string, m map[string]any) { firstCase(m)["case_id"] = "métric" }, []string{"métric", "cases[0].case_id"}},
		{"invalid path chars", func(_ *testing.T, _ string, m map[string]any) {
			firstCase(m)["input"] = "cases/metric 001/input.lines.txt"
		}, []string{"metric-001", "cases[0].input"}},
		{"absolute", func(_ *testing.T, _ string, m map[string]any) { firstCase(m)["input"] = "/tmp/input" }, []string{"metric-001", "cases[0].input"}},
		{"traversal", func(_ *testing.T, _ string, m map[string]any) { firstCase(m)["input"] = "cases/../manifest.json" }, []string{"metric-001", "cases[0].input"}},
		{"missing", func(_ *testing.T, root string, _ map[string]any) {
			os.Remove(filepath.Join(root, "cases/metric-001/input.lines.txt"))
		}, []string{"metric-001", "cases[0].input"}},
		{"duplicate normalized reference", func(t *testing.T, root string, m map[string]any) {
			c := cloneCase(m)
			c["case_id"] = "metric-002"
			c["input"] = "cases//metric-001/input.lines.txt"
			c["expected"] = "cases/metric-002/expected.json"
			mustWrite(t, filepath.Join(root, "cases/metric-002/expected.json"), validExpected())
			m["cases"] = append(m["cases"].([]any), c)
		}, []string{"metric-002", "cases[1].input", "duplicate normalized reference"}},
		{"unsupported processor", func(_ *testing.T, _ string, m map[string]any) { firstCase(m)["processors"] = []any{"summaries"} }, []string{"metric-001", "cases[0].processors[0]"}},
		{"empty applicability", func(_ *testing.T, _ string, m map[string]any) { firstCase(m)["processors"] = []any{} }, []string{"metric-001", "cases[0].processors"}},
		{"symlink", func(t *testing.T, root string, m map[string]any) {
			outside := filepath.Join(t.TempDir(), "input.lines.txt")
			mustWrite(t, outside, validInput())
			link := filepath.Join(root, "linked.lines.txt")
			if err := os.Symlink(outside, link); err != nil {
				t.Fatal(err)
			}
			firstCase(m)["input"] = "linked.lines.txt"
		}, []string{"metric-001", "cases[0].input", "symlink"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := writeDataset(t, validManifest(), validInput(), validExpected())
			var m map[string]any
			if err := json.Unmarshal(validManifest(), &m); err != nil {
				t.Fatal(err)
			}
			tt.edit(t, root, m)
			b, _ := json.Marshal(m)
			mustWrite(t, filepath.Join(root, "manifest.json"), b)
			_, err := LoadDataset(root)
			if err == nil {
				t.Fatal("LoadDataset unexpectedly succeeded")
			}
			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
		})
	}
}

func TestLoadDatasetRootDescriptorSurvivesPathReplacement(t *testing.T) {
	parent := t.TempDir()
	original := filepath.Join(parent, "dataset")
	mustWrite(t, filepath.Join(original, "safe.txt"), []byte("inside"))
	root, err := os.OpenRoot(original)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	moved := filepath.Join(parent, "moved")
	if err := os.Rename(original, moved); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	mustWrite(t, filepath.Join(outside, "safe.txt"), []byte("outside"))
	if err := os.Symlink(outside, original); err != nil {
		t.Fatal(err)
	}

	got, err := readRegularFile(root, "safe.txt")
	if err != nil {
		t.Fatalf("descriptor-rooted read: %v", err)
	}
	if string(got) != "inside" {
		t.Fatalf("read %q after root path replacement, want original descriptor content", got)
	}
}

func TestValidationErrorsErrorDoesNotMutateAndIsDeterministic(t *testing.T) {
	problems := validationErrors{"z-field", "a-field"}
	wantBacking := append(validationErrors(nil), problems...)
	first := problems.Error()
	second := problems.Error()
	if !reflect.DeepEqual(problems, wantBacking) {
		t.Fatalf("Error mutated receiver: got %v want %v", problems, wantBacking)
	}
	if first != second || strings.Index(first, "a-field") > strings.Index(first, "z-field") {
		t.Fatalf("nondeterministic or unsorted error: first=%q second=%q", first, second)
	}
}

func TestValidateDatasetRejectsSemanticGoldViolations(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]any, map[string]any)
		want   string
	}{
		{"unknown tag", func(m, _ map[string]any) { firstCase(m)["tags"] = []any{"mystery"} }, "cases[0].tags[0]"},
		{"repeated tag", func(m, _ map[string]any) { firstCase(m)["tags"] = []any{"overlap", "overlap"} }, "cases[0].tags[1]"},
		{"section mismatch", func(m, _ map[string]any) { firstCase(m)["processors"] = []any{"chunking"} }, "cases[0].processors"},
		{"stale chunk line", func(_, e map[string]any) {
			e["chunking"].(map[string]any)["chunks"].([]any)[0].(map[string]any)["normal_lines"] = []any{1, 99}
		}, "expected.chunking.chunks[0].normal_lines[1]"},
		{"metric span", func(_, e map[string]any) {
			e["extract_metrics"].(map[string]any)["metrics"].([]any)[0].(map[string]any)["source_lines"] = []any{0, 3}
		}, "expected.extract_metrics.metrics[0].source_lines[0]"},
		{"duplicate gold id", func(_, e map[string]any) {
			ms := e["extract_metrics"].(map[string]any)["metrics"].([]any)
			e["extract_metrics"].(map[string]any)["metrics"] = append(ms, ms[0])
		}, "expected.extract_metrics.metrics[1].gold_id"},
		{"protected policy", func(_, e map[string]any) {
			e["chunking"].(map[string]any)["protected_groups"].([]any)[0].(map[string]any)["split_policy"] = "sometimes"
		}, "expected.chunking.protected_groups[0].split_policy"},
		{"protected repeated line", func(_, e map[string]any) {
			e["chunking"].(map[string]any)["protected_groups"].([]any)[0].(map[string]any)["lines"] = []any{1, 1}
		}, "expected.chunking.protected_groups[0].lines[1]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m, e map[string]any
			json.Unmarshal(validManifest(), &m)
			json.Unmarshal(validExpected(), &e)
			tt.mutate(m, e)
			mb, _ := json.Marshal(m)
			eb, _ := json.Marshal(e)
			root := writeDataset(t, mb, validInput(), eb)
			_, err := LoadDataset(root)
			if err == nil || !strings.Contains(err.Error(), "metric-001") || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error=%v, want case ID and %q", err, tt.want)
			}
		})
	}
}

func TestValidateProtectedGroupAssignments(t *testing.T) {
	tests := []struct {
		name      string
		policy    string
		chunks    []any
		wantError string
	}{
		{
			name: "never group split across normal chunks", policy: "never",
			chunks:    []any{map[string]any{"sequence": 1, "overlap_lines": []any{}, "normal_lines": []any{1}}, map[string]any{"sequence": 2, "overlap_lines": []any{}, "normal_lines": []any{2}}},
			wantError: "expected.chunking.protected_groups[0].split_policy",
		},
		{
			name: "expected group line missing assignment", policy: "expected",
			chunks:    []any{map[string]any{"sequence": 1, "overlap_lines": []any{}, "normal_lines": []any{1}}},
			wantError: "expected.chunking.protected_groups[0].lines[1]",
		},
		{
			name: "expected group split assignment is valid", policy: "expected",
			chunks: []any{map[string]any{"sequence": 1, "overlap_lines": []any{}, "normal_lines": []any{1}}, map[string]any{"sequence": 2, "overlap_lines": []any{}, "normal_lines": []any{2}}},
		},
		{
			name: "never group same chunk is valid", policy: "never",
			chunks: []any{map[string]any{"sequence": 1, "overlap_lines": []any{}, "normal_lines": []any{1, 2}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var e map[string]any
			json.Unmarshal(validExpected(), &e)
			chunking := e["chunking"].(map[string]any)
			chunking["protected_groups"].([]any)[0].(map[string]any)["split_policy"] = tt.policy
			chunking["chunks"] = tt.chunks
			expected, _ := json.Marshal(e)
			_, err := LoadDataset(writeDataset(t, validManifest(), validInput(), expected))
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("valid protected group rejected: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), "metric-001") || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("error=%v, want case ID and %q", err, tt.wantError)
			}
		})
	}
}

func TestDatasetHashUsesExactFramedRawBytes(t *testing.T) {
	manifest := validManifest()
	root := writeDataset(t, manifest, validInput(), validExpected())
	ds, err := LoadDataset(root)
	if err != nil {
		t.Fatal(err)
	}
	var framed bytes.Buffer
	framed.WriteString(datasetHashPrefix)
	binary.Write(&framed, binary.BigEndian, uint64(3))
	entries := []struct {
		path string
		body []byte
	}{
		{"cases/metric-001/expected.json", validExpected()},
		{"cases/metric-001/input.lines.txt", validInput()},
		{"manifest.json", manifest},
	}
	for _, e := range entries {
		writeFrame(&framed, e.path, e.body)
	}
	want := sha256.Sum256(framed.Bytes())
	if ds.Hash != hex.EncodeToString(want[:]) {
		t.Fatalf("hash=%s want=%x", ds.Hash, want)
	}
}

func TestDatasetHashChangesWithEveryRawArtifact(t *testing.T) {
	base := loadHash(t, validManifest(), validInput(), validExpected())
	variants := []string{
		loadHash(t, append([]byte(" \n"), validManifest()...), validInput(), validExpected()),
		loadHash(t, validManifest(), append(validInput(), '\n'), validExpected()),
		loadHash(t, validManifest(), validInput(), append(validExpected(), '\n')),
	}
	for _, got := range variants {
		if got == base {
			t.Errorf("raw byte change did not change hash %s", got)
		}
	}
}

func TestFileHashesAreRawSHA256(t *testing.T) {
	ds, err := LoadDataset(writeDataset(t, validManifest(), validInput(), validExpected()))
	if err != nil {
		t.Fatal(err)
	}
	for path, body := range map[string][]byte{"cases/metric-001/input.lines.txt": validInput(), "cases/metric-001/expected.json": validExpected()} {
		sum := sha256.Sum256(body)
		if ds.FileHashes[path] != hex.EncodeToString(sum[:]) {
			t.Errorf("%s hash=%s want=%x", path, ds.FileHashes[path], sum)
		}
	}
}

func TestCaseSetHashSortsUnitsAndReflectsFiltersAndRepetitions(t *testing.T) {
	ds, err := LoadDataset(writeDataset(t, validManifest(), validInput(), validExpected()))
	if err != nil {
		t.Fatal(err)
	}
	a, err := ds.CaseSetHash(ProcessorChunking, nil, 2)
	if err != nil {
		t.Fatal(err)
	}
	b, err := ds.CaseSetHash(ProcessorChunking, []string{"overlap"}, 2)
	if err != nil {
		t.Fatal(err)
	}
	c, err := ds.CaseSetHash(ProcessorChunking, []string{"no-metric"}, 2)
	if err != nil {
		t.Fatal(err)
	}
	d, err := ds.CaseSetHash(ProcessorChunking, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Errorf("matching filter changed case set: %s != %s", a, b)
	}
	if a == c || a == d {
		t.Errorf("filter/repetitions did not alter appropriate hash: %s %s %s", a, c, d)
	}
}

func TestCaseSetHashSortsReverseManifestCasesAndMatchesExactFrames(t *testing.T) {
	forward := loadTwoCaseDataset(t, false)
	reverse := loadTwoCaseDataset(t, true)
	gotForward, err := forward.CaseSetHash(ProcessorChunking, nil, 2)
	if err != nil {
		t.Fatal(err)
	}
	gotReverse, err := reverse.CaseSetHash(ProcessorChunking, nil, 2)
	if err != nil {
		t.Fatal(err)
	}
	if gotForward != gotReverse {
		t.Fatalf("manifest order changed case-set hash: %s != %s", gotForward, gotReverse)
	}
	var framed bytes.Buffer
	binary.Write(&framed, binary.BigEndian, uint64(4))
	for _, unit := range []struct{ id, repetition string }{{"metric-001", "1"}, {"metric-001", "2"}, {"metric-002", "1"}, {"metric-002", "2"}} {
		writeFrame(&framed, unit.id, []byte(unit.repetition))
	}
	want := sha256.Sum256(framed.Bytes())
	if gotForward != hex.EncodeToString(want[:]) {
		t.Fatalf("hash=%s want=%x", gotForward, want)
	}
}

func TestCaseSetHashRejectsExcessiveRepetitions(t *testing.T) {
	ds, err := LoadDataset(writeDataset(t, validManifest(), validInput(), validExpected()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ds.CaseSetHash(ProcessorChunking, nil, MaxRepetitions+1); err == nil || !strings.Contains(err.Error(), "repetitions") {
		t.Fatalf("error=%v, want repetitions limit", err)
	}
}

func validManifest() []byte {
	return []byte(`{"schema_version":1,"dataset_id":"doc-processors-synthetic-core","dataset_version":"1.0.0","generator_version":"1.0.0","seed":20260713,"cases":[{"case_id":"metric-001","input":"cases/metric-001/input.lines.txt","expected":"cases/metric-001/expected.json","processors":["chunking","extract_metrics"],"tags":["overlap","multiple-units"]}]}`)
}
func validInput() []byte {
	return []byte("1\t1\tparagraph\tArial\t12\t[0,0,1,1]\tLatency is 200 ms.\n2\t1\tparagraph\tArial\t12\t[0,0,1,1]\tTarget is 250 ms.")
}
func validExpected() []byte {
	return []byte(`{"schema_version":1,"chunking":{"protected_groups":[{"group_id":"list-1","kind":"non_numeric_list","split_policy":"never","lines":[1,2]}],"chunks":[{"sequence":1,"overlap_lines":[],"normal_lines":[1,2]}]},"extract_metrics":{"metrics":[{"gold_id":"m1","metric_name":"Maximum response time","metric_subject":"service endpoint","metric_value":"200","metric_unit":"ms","is_explicit_metric":true,"source_lines":[1]}]}}`)
}
func writeDataset(t *testing.T, manifest, input, expected []byte) string {
	t.Helper()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "manifest.json"), manifest)
	mustWrite(t, filepath.Join(root, "cases/metric-001/input.lines.txt"), input)
	mustWrite(t, filepath.Join(root, "cases/metric-001/expected.json"), expected)
	return root
}
func mustWrite(t *testing.T, p string, b []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, b, 0644); err != nil {
		t.Fatal(err)
	}
}
func firstCase(m map[string]any) map[string]any { return m["cases"].([]any)[0].(map[string]any) }
func cloneCase(m map[string]any) map[string]any {
	b, _ := json.Marshal(firstCase(m))
	var c map[string]any
	json.Unmarshal(b, &c)
	return c
}
func loadHash(t *testing.T, m, i, e []byte) string {
	t.Helper()
	ds, err := LoadDataset(writeDataset(t, m, i, e))
	if err != nil {
		t.Fatal(err)
	}
	return ds.Hash
}

func loadTwoCaseDataset(t *testing.T, reverse bool) *Dataset {
	t.Helper()
	var manifest map[string]any
	json.Unmarshal(validManifest(), &manifest)
	second := cloneCase(manifest)
	second["case_id"] = "metric-002"
	second["input"] = "cases/metric-002/input.lines.txt"
	second["expected"] = "cases/metric-002/expected.json"
	cases := []any{firstCase(manifest), second}
	if reverse {
		cases[0], cases[1] = cases[1], cases[0]
	}
	manifest["cases"] = cases
	raw, _ := json.Marshal(manifest)
	root := writeDataset(t, raw, validInput(), validExpected())
	mustWrite(t, filepath.Join(root, "cases/metric-002/input.lines.txt"), validInput())
	mustWrite(t, filepath.Join(root, "cases/metric-002/expected.json"), validExpected())
	ds, err := LoadDataset(root)
	if err != nil {
		t.Fatal(err)
	}
	return ds
}
