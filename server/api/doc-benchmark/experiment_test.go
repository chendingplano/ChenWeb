package docbenchmark

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type staticDatasetResolver struct{ dataset *Dataset }

func (r staticDatasetResolver) ResolveDataset(id, version string) (*Dataset, error) {
	return r.dataset, nil
}

func TestExperimentExactSpecShape(t *testing.T) {
	raw := []byte(`name = "metrics-model-comparison"
dataset = "doc-processors-synthetic-core@1.0.0"
processors = ["chunking", "extract_metrics"]
repetitions = 3
case_tags = []
timeout = "20m"
max_parallel_cases = 2
max_parallel_variants = 1
max_attempts = 2
attempt_lease = "25m"
allow_upstream_variation = false
retain_workspaces = false

[[variants]]
name = "baseline"

[variants.overrides]
CHUNK_SIZE = "300"
CHUNK_OVERLAP_PERCENT = "20"
EXTRACT_METRIC_CANDIDATES_PROMPT = "prompts/prompt-extract-metric-candidates-v3.md"
EXTRACT_METRIC_CANDIDATES_MODEL_NAME = "deepseek-chat"
ENRICH_METRICS_PROMPT = "prompts/prompt-enrich-metrics-v3.md"
ENRICH_METRICS_MODEL_NAME = "deepseek-chat"
METRIC_ENRICH_GROUP_SIZE = "5"
`)
	exp := resolveExperimentForTest(t, raw)
	if exp.Name != "metrics-model-comparison" || exp.DatasetID != "doc-processors-synthetic-core" || exp.DatasetVersion != "1.0.0" {
		t.Fatalf("identity not parsed: %#v", exp)
	}
	if exp.Repetitions != 3 || exp.Timeout != 20*time.Minute || exp.AttemptLease != 25*time.Minute || exp.MaxParallelCases != 2 || exp.MaxParallelVariants != 1 || exp.MaxAttempts != 2 {
		t.Fatalf("counts/durations not parsed: %#v", exp)
	}
	if exp.AllowUpstreamVariation || exp.RetainWorkspaces {
		t.Fatalf("unexpected boolean options: %#v", exp)
	}
	if len(exp.Variants) != 1 || exp.Variants[0].Name != "baseline" || exp.Variants[0].Overrides["CHUNK_SIZE"] != "300" {
		t.Fatalf("variant not parsed: %#v", exp.Variants)
	}
}

func TestExperimentDefaultsDependOnProcessorClosure(t *testing.T) {
	tests := []struct {
		processors string
		wantReps   int
	}{
		{`["chunking"]`, 1},
		{`["extract_metrics"]`, 3},
		{`["chunking", "extract_metrics"]`, 3},
	}
	for _, tt := range tests {
		t.Run(tt.processors, func(t *testing.T) {
			exp := resolveExperimentForTest(t, minimalExperiment(tt.processors, "base", ""))
			if exp.Repetitions != tt.wantReps || exp.Timeout != 20*time.Minute || exp.AttemptLease != 25*time.Minute {
				t.Fatalf("defaults: %#v", exp)
			}
			if exp.MaxParallelCases != 1 || exp.MaxParallelVariants != 1 || exp.MaxAttempts != 2 || exp.AllowUpstreamVariation || exp.RetainWorkspaces {
				t.Fatalf("defaults: %#v", exp)
			}
		})
	}
}

func TestExpandVariantsUsesExplicitLexicalOrderAndNoCartesianAxes(t *testing.T) {
	raw := []byte(`name="x"
dataset="doc-processors-synthetic-core@1.0.0"
processors=["chunking"]
[[variants]]
name="zeta"
[variants.overrides]
CHUNK_SIZE="400"
[[variants]]
name="alpha"
[variants.overrides]
CHUNK_SIZE="200"
`)
	exp := resolveExperimentForTest(t, raw)
	if got := []string{exp.Variants[0].Name, exp.Variants[1].Name}; !reflect.DeepEqual(got, []string{"alpha", "zeta"}) {
		t.Fatalf("variant order=%v", got)
	}
}

func TestExperimentFiltersAreValidatedDeduplicatedAndByteSorted(t *testing.T) {
	raw := []byte(`name="x"
dataset="doc-processors-synthetic-core@1.0.0"
processors=["extract_metrics"]
case_tags=["overlap", "multiple-units", "overlap"]
allow_upstream_variation=true
[[variants]]
name="base"
`)
	exp := resolveExperimentForTest(t, raw)
	if !reflect.DeepEqual(exp.CaseTags, []string{"multiple-units", "overlap"}) {
		t.Fatalf("case_tags=%v", exp.CaseTags)
	}
	if !exp.AllowUpstreamVariation {
		t.Fatal("allow_upstream_variation was lost")
	}
	if exp.ProcessorCaseSetHashes[ProcessorExtractMetrics] == "" {
		t.Fatal("missing selected processor case-set hash")
	}
}

func TestExperimentRejectsInvalidFieldsCountsDurationsAndLease(t *testing.T) {
	tests := []struct{ name, extra, want string }{
		{"unknown top level", "mystery=true\n", "experiment TOML"},
		{"unknown variant", "flavor=\"x\"\n", "experiment TOML"},
		{"unknown override table", "[variants.environment]\nFOO=\"bar\"\n", "experiment TOML"},
		{"zero repetitions", "repetitions=0\n", "repetitions"},
		{"negative repetitions", "repetitions=-1\n", "repetitions"},
		{"too many repetitions", "repetitions=10001\n", "repetitions"},
		{"zero cases", "max_parallel_cases=0\n", "max_parallel_cases"},
		{"negative variants", "max_parallel_variants=-1\n", "max_parallel_variants"},
		{"zero attempts", "max_attempts=0\n", "max_attempts"},
		{"too many cases", "max_parallel_cases=257\n", "max_parallel_cases"},
		{"too many variants", "max_parallel_variants=65\n", "max_parallel_variants"},
		{"too many attempts", "max_attempts=101\n", "max_attempts"},
		{"integer overflow", "max_attempts=9223372036854775808\n", "experiment TOML"},
		{"bad timeout", "timeout=\"later\"\n", "timeout"},
		{"zero timeout", "timeout=\"0s\"\n", "timeout"},
		{"bad lease", "attempt_lease=\"soon\"\n", "attempt_lease"},
		{"equal lease", "timeout=\"5m\"\nattempt_lease=\"5m\"\n", "attempt_lease"},
		{"short lease", "timeout=\"5m\"\nattempt_lease=\"4m\"\n", "attempt_lease"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := minimalExperiment(`["chunking"]`, "base", tt.extra)
			_, err := ResolveExperiment(raw, staticDatasetResolver{loadValidDataset(t)})
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.want)) {
				t.Fatalf("error=%v, want %q", err, tt.want)
			}
		})
	}
}

func TestExperimentAcceptsDocumentedCountMaxima(t *testing.T) {
	raw := minimalExperiment(`["chunking"]`, "base", "repetitions=10000\nmax_parallel_cases=256\nmax_parallel_variants=64\nmax_attempts=100\n")
	exp := resolveExperimentForTest(t, raw)
	if exp.Repetitions != MaxRepetitions || exp.MaxParallelCases != MaxParallelCases || exp.MaxParallelVariants != MaxParallelVariants || exp.MaxAttempts != MaxAttempts {
		t.Fatalf("maximum counts not materialized: %#v", exp)
	}
}

func TestExperimentRejectsNestedUnknownTOMLAndNonStringOverrides(t *testing.T) {
	for _, raw := range [][]byte{
		[]byte("name=\"x\"\ndataset=\"doc-processors-synthetic-core@1.0.0\"\nprocessors=[\"chunking\"]\n[[variants]]\nname=\"base\"\nflavor=\"x\"\n"),
		[]byte("name=\"x\"\ndataset=\"doc-processors-synthetic-core@1.0.0\"\nprocessors=[\"chunking\"]\n[[variants]]\nname=\"base\"\n[variants.overrides]\nCHUNK_SIZE=300\n"),
	} {
		if _, err := ResolveExperiment(raw, staticDatasetResolver{loadValidDataset(t)}); err == nil {
			t.Fatalf("invalid TOML accepted:\n%s", raw)
		}
	}
}

func TestExperimentRejectsInvalidIdentityProcessorsTagsAndVariants(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		want string
	}{
		{"empty name", []byte("name=\"\"\ndataset=\"doc-processors-synthetic-core@1.0.0\"\nprocessors=[\"chunking\"]\n[[variants]]\nname=\"base\"\n"), "name"},
		{"bad dataset", []byte("name=\"x\"\ndataset=\"bad\"\nprocessors=[\"chunking\"]\n[[variants]]\nname=\"base\"\n"), "dataset"},
		{"empty processors", minimalExperiment(`[]`, "base", ""), "processors"},
		{"unknown processor", minimalExperiment(`["summaries"]`, "base", ""), "processors"},
		{"duplicate processor", minimalExperiment(`["chunking", "chunking"]`, "base", ""), "duplicate"},
		{"unknown tag", minimalExperiment(`["chunking"]`, "base", "case_tags=[\"mystery\"]\n"), "case_tags"},
		{"empty variant", minimalExperiment(`["chunking"]`, "", ""), "variant"},
		{"no variants", []byte("name=\"x\"\ndataset=\"doc-processors-synthetic-core@1.0.0\"\nprocessors=[\"chunking\"]\n"), "variants"},
		{"duplicate variants", append(minimalExperiment(`["chunking"]`, "base", ""), []byte("[[variants]]\nname=\"base\"\n")...), "duplicate"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ResolveExperiment(tt.raw, staticDatasetResolver{loadValidDataset(t)})
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.want)) {
				t.Fatalf("error=%v, want %q", err, tt.want)
			}
		})
	}
}

func TestOverridePolicyUsesExactTransitiveClosure(t *testing.T) {
	chunkKeys := []string{"CHUNK_SIZE", "CHUNK_OVERLAP_PERCENT"}
	metricKeys := []string{
		"EXTRACT_METRIC_CANDIDATES_PROMPT", "EXTRACT_METRIC_CANDIDATES_MODEL_NAME", "EXTRACT_METRIC_CANDIDATES_MODEL_FALLBACK",
		"ENRICH_METRICS_PROMPT", "ENRICH_METRICS_MODEL_NAME", "EXTRACT_METRICS_MODEL_NAME",
		"METRIC_MERGE_RESOLVE_PROMPT", "METRIC_MERGE_RESOLVE_MODEL_NAME", "METRIC_MERGE_RESOLVE_MODEL_FALLBACK",
		"METRIC_ENRICH_GROUP_SIZE", "EXTRACT_METRICS_MAX_TASKS",
	}
	for _, key := range chunkKeys {
		resolveExperimentForTest(t, experimentWithOverride(`["chunking"]`, key))
	}
	for _, key := range append(append([]string{}, chunkKeys...), metricKeys...) {
		resolveExperimentForTest(t, experimentWithOverride(`["extract_metrics"]`, key))
	}
	for _, key := range metricKeys {
		_, err := ResolveExperiment(experimentWithOverride(`["chunking"]`, key), staticDatasetResolver{loadValidDataset(t)})
		if err == nil || !strings.Contains(err.Error(), key) {
			t.Fatalf("chunking accepted %s or returned unclear error: %v", key, err)
		}
	}
	for _, key := range []string{"PATH", "MODEL", "TEMPERATURE", "SEED", "FOO"} {
		_, err := ResolveExperiment(experimentWithOverride(`["extract_metrics"]`, key), staticDatasetResolver{loadValidDataset(t)})
		if err == nil || !strings.Contains(err.Error(), key) {
			t.Fatalf("accepted unknown key %s: %v", key, err)
		}
	}
}

func TestOverrideRejectsSecretShapedKeysRatherThanMasking(t *testing.T) {
	for _, key := range []string{"OPENAI_API_KEY", "apiKey", "AUTH_TOKEN", "PASSWORD", "db_passwd", "passphrase", "CLIENT_SECRET", "credentials", "AWS_CREDENTIAL", "private_key", "access_key"} {
		t.Run(key, func(t *testing.T) {
			_, err := ResolveExperiment(experimentWithOverride(`["extract_metrics"]`, key), staticDatasetResolver{loadValidDataset(t)})
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), "secret") || !strings.Contains(err.Error(), key) {
				t.Fatalf("error=%v, want explicit secret rejection", err)
			}
		})
	}
}

func TestExperimentOverrideExpandsEnvVariableValues(t *testing.T) {
	t.Setenv("DOC_BENCHMARK_METRICS_MODEL_NAME", "deepseek-flash-chen")
	raw := []byte("name=\"x\"\ndataset=\"doc-processors-synthetic-core@1.0.0\"\nprocessors=[\"extract_metrics\"]\n[[variants]]\nname=\"base\"\n[variants.overrides]\nEXTRACT_METRIC_CANDIDATES_MODEL_NAME=\"${DOC_BENCHMARK_METRICS_MODEL_NAME}\"\n")
	exp := resolveExperimentForTest(t, raw)
	if got := exp.Variants[0].Overrides["EXTRACT_METRIC_CANDIDATES_MODEL_NAME"]; got != "deepseek-flash-chen" {
		t.Fatalf("expanded override=%q", got)
	}
}

func TestExperimentOverrideRejectsMissingEnvVariableValue(t *testing.T) {
	t.Setenv("DOC_BENCHMARK_METRICS_MODEL_NAME", "")
	raw := []byte("name=\"x\"\ndataset=\"doc-processors-synthetic-core@1.0.0\"\nprocessors=[\"extract_metrics\"]\n[[variants]]\nname=\"base\"\n[variants.overrides]\nEXTRACT_METRIC_CANDIDATES_MODEL_NAME=\"${DOC_BENCHMARK_METRICS_MODEL_NAME}\"\n")
	_, err := ResolveExperiment(raw, staticDatasetResolver{loadValidDataset(t)})
	if err == nil || !strings.Contains(err.Error(), "DOC_BENCHMARK_METRICS_MODEL_NAME") {
		t.Fatalf("error=%v, want missing env var", err)
	}
}

func TestSnapshotRetainsRequestedIntentAndDatasetProvenance(t *testing.T) {
	raw := minimalExperiment(`["extract_metrics"]`, "base", "case_tags=[\"overlap\"]\n")
	exp := resolveExperimentForTest(t, raw)
	dataset := loadValidDataset(t)
	wantRequest := sha256.Sum256(raw)
	if !bytes.Equal(exp.RawTOML, raw) || exp.RequestHash != hex.EncodeToString(wantRequest[:]) {
		t.Fatalf("raw/hash mismatch: raw=%q hash=%s", exp.RawTOML, exp.RequestHash)
	}
	if exp.DatasetHash == "" || len(exp.FileHashes) != 2 || exp.ProcessorCaseSetHashes[ProcessorExtractMetrics] == "" {
		t.Fatalf("dataset provenance missing: %#v", exp)
	}
	wantCaseSet, err := dataset.CaseSetHash(ProcessorExtractMetrics, []string{"overlap"}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if exp.DatasetHash != dataset.Hash || !reflect.DeepEqual(exp.FileHashes, dataset.FileHashes) || exp.ProcessorCaseSetHashes[ProcessorExtractMetrics] != wantCaseSet {
		t.Fatalf("dataset provenance differs from Task 1 hashes: %#v", exp)
	}
	var requested map[string]map[string]string
	if err := json.Unmarshal(exp.RequestedOverridesJSON, &requested); err != nil {
		t.Fatalf("requested overrides JSON: %v", err)
	}
	if requested["base"] == nil {
		t.Fatalf("requested overrides missing base variant: %s", exp.RequestedOverridesJSON)
	}
	if len(exp.MaterializedConfigJSON) == 0 || bytes.Contains(exp.MaterializedConfigJSON, []byte("resolved_config")) || bytes.Contains(exp.MaterializedConfigJSON, []byte("provider")) {
		t.Fatalf("materialized requested config is missing or claims runtime resolution: %s", exp.MaterializedConfigJSON)
	}

	again := resolveExperimentForTest(t, raw)
	if !bytes.Equal(exp.MaterializedConfigJSON, again.MaterializedConfigJSON) || !bytes.Equal(exp.RequestedOverridesJSON, again.RequestedOverridesJSON) || !reflect.DeepEqual(exp.FileHashes, again.FileHashes) || !reflect.DeepEqual(exp.ProcessorCaseSetHashes, again.ProcessorCaseSetHashes) {
		t.Fatal("same requested intent was not deterministic")
	}
	formatted := append(append([]byte{}, raw...), '\n')
	changed := resolveExperimentForTest(t, formatted)
	if changed.RequestHash == exp.RequestHash {
		t.Fatal("formatting change did not change raw request hash")
	}
}

func TestExperimentDatasetRootResolverVerifiesManifestIdentity(t *testing.T) {
	datasetRoot := loadValidDataset(t).Root
	root := t.TempDir()
	target := filepath.Join(root, "doc-processors-synthetic-core", "1.0.0")
	if err := copyDatasetTree(datasetRoot, target); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadExperiment(minimalExperiment(`["chunking"]`, "base", ""), root); err != nil {
		t.Fatalf("LoadExperiment: %v", err)
	}
	wrongTarget := filepath.Join(root, "other", "1.0.0")
	if err := copyDatasetTree(datasetRoot, wrongTarget); err != nil {
		t.Fatal(err)
	}
	_, err := LoadExperiment([]byte("name=\"x\"\ndataset=\"other@1.0.0\"\nprocessors=[\"chunking\"]\n[[variants]]\nname=\"base\"\n"), root)
	if err == nil || !strings.Contains(err.Error(), "dataset_id") {
		t.Fatalf("identity mismatch error=%v", err)
	}
}

func TestExperimentDatasetRootResolverRejectsDotAndDotDotComponents(t *testing.T) {
	resolver := DatasetRootResolver{Root: t.TempDir()}
	for _, tc := range []struct{ id, version string }{
		{".", "1.0.0"},
		{"..", "1.0.0"},
		{"dataset", "."},
		{"dataset", ".."},
		{"dataset/child", "1.0.0"},
		{"dataset", "1.0.0/child"},
	} {
		if _, err := resolver.ResolveDataset(tc.id, tc.version); err == nil {
			t.Fatalf("unsafe components accepted: id=%q version=%q", tc.id, tc.version)
		}
	}
}

func TestExperimentDatasetRootResolverRejectsSymlinkedIDAndVersionWithoutOutsideRead(t *testing.T) {
	datasetRoot := loadValidDataset(t).Root
	for _, symlinkID := range []bool{true, false} {
		name := "version"
		if symlinkID {
			name = "id"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			outside := t.TempDir()
			idPath := filepath.Join(root, "doc-processors-synthetic-core")
			if symlinkID {
				if err := copyDatasetTree(datasetRoot, filepath.Join(outside, "1.0.0")); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, idPath); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.MkdirAll(idPath, 0755); err != nil {
					t.Fatal(err)
				}
				if err := copyDatasetTree(datasetRoot, outside); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, filepath.Join(idPath, "1.0.0")); err != nil {
					t.Fatal(err)
				}
			}
			_, err := LoadExperiment(minimalExperiment(`["chunking"]`, "base", ""), root)
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), "symlink") {
				t.Fatalf("resolver followed symlink outside configured root: %v", err)
			}
		})
	}
}

func resolveExperimentForTest(t *testing.T, raw []byte) *Experiment {
	t.Helper()
	exp, err := ResolveExperiment(raw, staticDatasetResolver{loadValidDataset(t)})
	if err != nil {
		t.Fatalf("ResolveExperiment: %v\n%s", err, raw)
	}
	return exp
}

func loadValidDataset(t *testing.T) *Dataset {
	t.Helper()
	ds, err := LoadDataset(writeDataset(t, validManifest(), validInput(), validExpected()))
	if err != nil {
		t.Fatal(err)
	}
	return ds
}

func minimalExperiment(processors, variant, extra string) []byte {
	return []byte("name=\"x\"\ndataset=\"doc-processors-synthetic-core@1.0.0\"\nprocessors=" + processors + "\n" + extra + "[[variants]]\nname=\"" + variant + "\"\n")
}

func experimentWithOverride(processors, key string) []byte {
	return []byte("name=\"x\"\ndataset=\"doc-processors-synthetic-core@1.0.0\"\nprocessors=" + processors + "\n[[variants]]\nname=\"base\"\n[variants.overrides]\n" + key + "=\"value\"\n")
}

func copyDatasetTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, body, 0644)
	})
}
