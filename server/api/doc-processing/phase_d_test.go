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
