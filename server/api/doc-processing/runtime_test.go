package docprocessing

import (
	"strings"
	"testing"
)

func TestProductionRuntimeAllowedOverridesAreExactAndNonSecret(t *testing.T) {
	r := &ProductionRuntime{}
	got := r.AllowedOverrides()
	if !equalStrings(got["chunking"], []string{"CHUNK_SIZE", "CHUNK_OVERLAP_PERCENT"}) {
		t.Fatalf("chunking overrides=%v", got["chunking"])
	}
	for _, key := range got["extract_metrics"] {
		if strings.Contains(strings.ToLower(key), "key") || strings.Contains(strings.ToLower(key), "token") {
			t.Fatalf("secret-shaped override %q", key)
		}
	}
}

func TestProductionRuntimeResolvedConfigIsRedactedAndDeterministic(t *testing.T) {
	r := &ProductionRuntime{Control: &ControlService{Processors: []Processor{fakeProcessor{name: "chunking"}}}, services: map[string]any{"chunking": map[string]any{"chunk_size": 42, "overlap_percent": 7}}}
	a := r.ResolvedConfig()
	b := r.ResolvedConfig()
	if a.Hash == "" || a.Hash != b.Hash || string(a.CanonicalJSON) != string(b.CanonicalJSON) {
		t.Fatalf("config snapshot is not deterministic: %#v %#v", a, b)
	}
	for _, secret := range []string{"OPENAI_API_KEY", "api_key", "token"} {
		if strings.Contains(strings.ToLower(string(a.CanonicalJSON)), strings.ToLower(secret)) {
			t.Fatalf("snapshot contains secret marker %q", secret)
		}
	}
}
