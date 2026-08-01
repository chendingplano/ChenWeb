package docprocessing

import "testing"

func TestSemanticAssociationEnabledFromEnvDefaultsFalse(t *testing.T) {
	t.Setenv("SEMANTIC_ASSOCIATION_ENABLED", "")
	if SemanticAssociationEnabledFromEnv() {
		t.Fatal("expected disabled by default")
	}
}

func TestSemanticAssociationEnabledFromEnvTrue(t *testing.T) {
	t.Setenv("SEMANTIC_ASSOCIATION_ENABLED", "true")
	if !SemanticAssociationEnabledFromEnv() {
		t.Fatal("expected enabled when set to true")
	}
}

func TestSemanticAssociationEnabledFromEnvInvalidIsDisabled(t *testing.T) {
	t.Setenv("SEMANTIC_ASSOCIATION_ENABLED", "not-a-bool")
	if SemanticAssociationEnabledFromEnv() {
		t.Fatal("expected disabled for an unparseable value")
	}
}
