package searchengine

import (
	"context"
	"testing"
)

func TestConnectEnvDefaults(t *testing.T) {
	t.Setenv("ARTIFACT_CONNECT_MIN_COSINE", "")
	t.Setenv("ARTIFACT_CONNECT_MAX_LINKS", "")
	if got := ConnectMinCosine(); got != defaultConnectMinCosine {
		t.Errorf("default min cosine = %v", got)
	}
	if got := ConnectMaxLinks(); got != defaultConnectMaxLinks {
		t.Errorf("default max links = %v", got)
	}
	t.Setenv("ARTIFACT_CONNECT_MIN_COSINE", "0.9")
	t.Setenv("ARTIFACT_CONNECT_MAX_LINKS", "3")
	if ConnectMinCosine() != 0.9 || ConnectMaxLinks() != 3 {
		t.Errorf("env values not read: %v %v", ConnectMinCosine(), ConnectMaxLinks())
	}
	t.Setenv("ARTIFACT_CONNECT_MAX_LINKS", "0")
	if ConnectMaxLinks() != defaultConnectMaxLinks {
		t.Errorf("invalid max links should fall back")
	}
}

func TestSimilarNilDB(t *testing.T) {
	if _, err := Similar(context.Background(), nil, ArtifactRef{Type: "metric", ID: "1"}, SimilarOptions{}); err == nil {
		t.Fatal("want error for nil db")
	}
}
