package productreviews

import (
	"errors"
	"testing"
)

// Scenario A: Metric type is registered (spec: Registered artifact types).
func TestMetricTypeRegistered(t *testing.T) {
	a, err := AdapterFor("metric")
	if err != nil {
		t.Fatalf("AdapterFor(metric): %v", err)
	}
	if a.Type() != "metric" || a.Partition() != "metric" {
		t.Fatalf("adapter Type/Partition = %q/%q, want metric/metric", a.Type(), a.Partition())
	}
	if got := RegisteredTypes(); len(got) != 1 || got[0] != "metric" {
		t.Fatalf("RegisteredTypes = %v, want [metric] only (v1)", got)
	}
}

// Scenario B: Unregistered type is refused (spec) — CWB_KB_PMR_020 naming the type.
func TestUnregisteredTypeRefused(t *testing.T) {
	_, err := AdapterFor("provision")
	var pmr *PMRError
	if !errors.As(err, &pmr) || pmr.Code != CodeUnregisteredType {
		t.Fatalf("err = %v, want CWB_KB_PMR_020", err)
	}
	if err := ValidateArtifactTypes([]string{"metric", "provision"}); err == nil {
		t.Fatal("ValidateArtifactTypes accepted an unregistered type")
	}
	if err := ValidateArtifactTypes([]string{"metric"}); err != nil {
		t.Fatalf("ValidateArtifactTypes([metric]) = %v, want nil", err)
	}
}
