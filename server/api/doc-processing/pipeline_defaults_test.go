package docprocessing

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestPipelineDefaultsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "defaults.json")
	want := PipelineDefaults{Processors: []string{"extract_metrics"}, Mode: "auto"}
	if err := SavePipelineDefaults(path, want, []string{"extract_metrics", "extract_products"}); err != nil {
		t.Fatal(err)
	}
	got, err := LoadPipelineDefaults(path)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, %v", got, err)
	}
	if err := SavePipelineDefaults(path, PipelineDefaults{Processors: []string{"unknown"}, Mode: "auto"}, []string{"extract_metrics"}); err == nil {
		t.Fatal("accepted unknown processor")
	}
	if err := SavePipelineDefaults(path, PipelineDefaults{Processors: []string{"extract_metrics"}, Mode: "bad"}, []string{"extract_metrics"}); err == nil {
		t.Fatal("accepted invalid mode")
	}
}

func TestConfiguredDefaultsOverrideStartupConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KB_CONFIG_FILE", filepath.Join(dir, "config.toml"))
	want := PipelineDefaults{Processors: []string{"extract_metrics"}, Mode: "auto_offpeak"}
	if err := SavePipelineDefaults(PipelineDefaultsPath(), want, want.Processors); err != nil {
		t.Fatal(err)
	}
	if got := configuredDefaultNames(); !reflect.DeepEqual(got, want.Processors) {
		t.Fatalf("got %v, want %v", got, want.Processors)
	}
}
