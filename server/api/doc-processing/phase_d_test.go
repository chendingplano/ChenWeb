package docprocessing

import "testing"

func TestSemanticAssociationEnabledFromEnvDefaultsTrue(t *testing.T) {
	t.Setenv("SEMANTIC_ASSOCIATION_ENABLED", "")
	if !SemanticAssociationEnabledFromEnv() {
		t.Fatal("expected enabled by default")
	}
}

func TestSemanticAssociationEnabledFromEnvTrue(t *testing.T) {
	t.Setenv("SEMANTIC_ASSOCIATION_ENABLED", "true")
	if !SemanticAssociationEnabledFromEnv() {
		t.Fatal("expected enabled when set to true")
	}
}

func TestSemanticAssociationEnabledFromEnvExplicitFalseIsDisabled(t *testing.T) {
	t.Setenv("SEMANTIC_ASSOCIATION_ENABLED", "false")
	if SemanticAssociationEnabledFromEnv() {
		t.Fatal("expected disabled when explicitly set to false")
	}
}

func TestSemanticAssociationEnabledFromEnvInvalidIsEnabled(t *testing.T) {
	t.Setenv("SEMANTIC_ASSOCIATION_ENABLED", "not-a-bool")
	if !SemanticAssociationEnabledFromEnv() {
		t.Fatal("expected enabled for an unparseable value, falling back to the default")
	}
}

// TestNormalizeAssertionsProcessorDependsOnExtractMetrics locks in ADR
// 2026081401 DR6's ordering requirement: normalize_assertions must not read
// a record's kb.metrics rows until extract_metrics's Phase C mapping check
// has finished for that record.
func TestNormalizeAssertionsProcessorDependsOnExtractMetrics(t *testing.T) {
	deps := NewNormalizeAssertionsProcessor().PostProcessDependsOn()
	if len(deps) != 2 || deps[0] != "extract_metrics" || deps[1] != "extract_metrics_simple" {
		t.Fatalf("PostProcessDependsOn() = %v, want [extract_metrics extract_metrics_simple]", deps)
	}
}
