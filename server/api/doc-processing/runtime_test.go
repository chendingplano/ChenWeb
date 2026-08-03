package docprocessing

import (
	"context"
	"errors"
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
