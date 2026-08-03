package docprocessing

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chendingplano/shared/go/api/ApiTypes"
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

type stubRegistryStore struct{ err error }

func (s stubRegistryStore) ListPipelines(context.Context) ([]ProductionPipelineSpec, error) {
	return nil, s.err
}

type stubBindingStore struct{ err error }

func (s stubBindingStore) ListPipelineBindings(context.Context) ([]PipelineBinding, error) {
	return nil, s.err
}

type stubGateStore struct{ err error }

func (s stubGateStore) ListPipelineGates(context.Context) ([]PipelineGate, error) {
	return nil, s.err
}

// TestLoadProductionPipelinePolicyStateRegistryFailureKeepsLegacyFallback
// proves a registry-load failure keeps its documented legacy-equivalent
// fallback (plan 2026080303 chunk C1) -- only bindings/gates must fail
// runtime construction.
func TestLoadProductionPipelinePolicyStateRegistryFailureKeepsLegacyFallback(t *testing.T) {
	err := loadProductionPipelinePolicyState(context.Background(), nil,
		stubRegistryStore{err: errors.New("registry unavailable")},
		stubBindingStore{}, stubGateStore{})
	if err != nil {
		t.Fatalf("registry-load failure must not fail runtime construction, got: %v", err)
	}
}

// TestLoadProductionPipelinePolicyStateBindingFailureFailsConstruction
// proves a canonical-binding load failure is NOT downgraded to a warning --
// spec 2026080102 section 11: a binding/gate load failure at startup must
// not leave the process running as though no policy existed (P5 review
// 2026080302 finding P5-11).
func TestLoadProductionPipelinePolicyStateBindingFailureFailsConstruction(t *testing.T) {
	underlying := errors.New("connection refused")
	err := loadProductionPipelinePolicyState(context.Background(), nil,
		stubRegistryStore{}, stubBindingStore{err: underlying}, stubGateStore{})
	if err == nil {
		t.Fatal("expected a binding load failure to fail runtime construction")
	}
	if !errors.Is(err, underlying) {
		t.Fatalf("error does not wrap the underlying cause %v: %v", underlying, err)
	}
}

// TestLoadProductionPipelinePolicyStateGateFailureFailsConstruction mirrors
// the binding case for processor-gate loading.
func TestLoadProductionPipelinePolicyStateGateFailureFailsConstruction(t *testing.T) {
	underlying := errors.New("connection refused")
	err := loadProductionPipelinePolicyState(context.Background(), nil,
		stubRegistryStore{}, stubBindingStore{}, stubGateStore{err: underlying})
	if err == nil {
		t.Fatal("expected a gate load failure to fail runtime construction")
	}
	if !errors.Is(err, underlying) {
		t.Fatalf("error does not wrap the underlying cause %v: %v", underlying, err)
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

// TestNewProductionRuntimeResolverNilWhenFlagOff proves the tier-3
// applicability resolver stays unset by default (D4: CLASSIFY_DOCUMENT_ENABLED
// defaults to false), keeping the runtime byte-identical to the pre-classifier
// build -- no model config, prompt, or DB is touched when the flag is off.
func TestNewProductionRuntimeResolverNilWhenFlagOff(t *testing.T) {
	t.Setenv("CLASSIFY_DOCUMENT_ENABLED", "")
	original := buildProductionRuntimeComponents
	buildProductionRuntimeComponents = func(ApiTypes.JimoLogger) productionRuntimeComponents {
		return productionRuntimeComponents{
			fixed: &FixedSizeChunkingService{},
			processors: []Processor{
				runtimeSelectionProcessor("static_analyzer"), runtimeSelectionProcessor("chunking"),
				runtimeSelectionProcessor("generate_summaries"), runtimeSelectionProcessor("extract_doc_metadata"),
				runtimeSelectionProcessor("extract_metrics"), runtimeSelectionProcessor("extract_provisions"),
			},
		}
	}
	t.Cleanup(func() { buildProductionRuntimeComponents = original })

	r, err := NewProductionRuntime(ProductionRuntimeOptions{RequiredProcessors: []string{"extract_metrics"}})
	if err != nil {
		t.Fatalf("NewProductionRuntime: %v", err)
	}
	if r.Control.Resolver != nil {
		t.Fatal("expected Resolver to stay nil when CLASSIFY_DOCUMENT_ENABLED is unset")
	}
}

// TestNewProductionRuntimeResolverWiredWhenFlagOn proves that with
// CLASSIFY_DOCUMENT_ENABLED=true the production ControlService carries a
// non-nil Resolver whose Classifier was built through the real constructor
// path (model config + versioned prompt), not an injected stub.
func TestNewProductionRuntimeResolverWiredWhenFlagOn(t *testing.T) {
	tmp := t.TempDir()
	modelsPath := filepath.Join(tmp, ".models.toml")
	modelsBody := `
[classify-document-model]
host = "cloud"
model_name = "deepseek-chat"
api_key = "sk-test-classify"
base_url = "https://api.deepseek.com"
timeout_sec = 45
`
	if err := os.WriteFile(modelsPath, []byte(modelsBody), 0o644); err != nil {
		t.Fatalf("write models file: %v", err)
	}
	promptPath := filepath.Join(tmp, "prompt-classify-document-v1.md")
	if err := os.WriteFile(promptPath, []byte("classify the document's governed attributes"), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}
	t.Setenv("CLASSIFY_DOCUMENT_ENABLED", "true")
	t.Setenv("MODEL_DEF_FILE", modelsPath)
	t.Setenv("CLASSIFY_DOCUMENT_MODEL_NAME", "classify-document-model")
	t.Setenv("PROMPT_DIR", tmp)

	original := buildProductionRuntimeComponents
	buildProductionRuntimeComponents = func(ApiTypes.JimoLogger) productionRuntimeComponents {
		return productionRuntimeComponents{
			fixed: &FixedSizeChunkingService{},
			processors: []Processor{
				runtimeSelectionProcessor("static_analyzer"), runtimeSelectionProcessor("chunking"),
				runtimeSelectionProcessor("generate_summaries"), runtimeSelectionProcessor("extract_doc_metadata"),
				runtimeSelectionProcessor("extract_metrics"), runtimeSelectionProcessor("extract_provisions"),
			},
		}
	}
	t.Cleanup(func() { buildProductionRuntimeComponents = original })

	r, err := NewProductionRuntime(ProductionRuntimeOptions{RequiredProcessors: []string{"extract_metrics"}})
	if err != nil {
		t.Fatalf("NewProductionRuntime: %v", err)
	}
	if r.Control.Resolver == nil {
		t.Fatal("expected Resolver to be wired when CLASSIFY_DOCUMENT_ENABLED=true")
	}
	if r.Control.Resolver.Classifier == nil {
		t.Fatal("expected Resolver.Classifier to be non-nil")
	}
	if r.Control.Resolver.Facets == nil {
		t.Fatal("expected Resolver.Facets store to be wired")
	}
	if r.Control.Resolver.VocabularyReleases == nil {
		t.Fatal("expected Resolver.VocabularyReleases store to be wired")
	}
}
