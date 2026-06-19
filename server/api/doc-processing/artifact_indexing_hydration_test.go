package docprocessing

import (
	"context"
	"database/sql"
	"testing"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

func TestIndexMetricsForRecordHydratesStoredEmbeddingsBeforeSemanticLinking(t *testing.T) {
	oldHydrate := hydrateArtifactEmbeddingsFunc
	defer func() { hydrateArtifactEmbeddingsFunc = oldHydrate }()

	called := false
	hydrateArtifactEmbeddingsFunc = func(_ context.Context, _ *sql.DB, recordID int64, artifactType string, artifacts []indexedArtifact, _ ApiTypes.JimoLogger, _ string) {
		called = true
		if recordID != 100 {
			t.Fatalf("recordID=%d, want 100", recordID)
		}
		if artifactType != searchArtifactMetric {
			t.Fatalf("artifactType=%q, want %q", artifactType, searchArtifactMetric)
		}
		if len(artifacts) != 1 {
			t.Fatalf("artifacts=%d, want 1", len(artifacts))
		}
	}

	runArtifactHydrationForSemanticLinking(context.Background(), nil, 100, []indexedArtifact{{ID: "100_met_1"}}, metricIndexConfig, nil)

	if !called {
		t.Fatal("expected hydrateArtifactEmbeddingsFunc to be called")
	}
}

func TestIndexInventoryItemsForRecordHydratesStoredEmbeddingsBeforeSemanticLinking(t *testing.T) {
	oldHydrate := hydrateArtifactEmbeddingsFunc
	defer func() { hydrateArtifactEmbeddingsFunc = oldHydrate }()

	called := false
	hydrateArtifactEmbeddingsFunc = func(_ context.Context, _ *sql.DB, recordID int64, artifactType string, artifacts []indexedArtifact, _ ApiTypes.JimoLogger, _ string) {
		called = true
		if recordID != 200 {
			t.Fatalf("recordID=%d, want 200", recordID)
		}
		if artifactType != searchArtifactInventoryItem {
			t.Fatalf("artifactType=%q, want %q", artifactType, searchArtifactInventoryItem)
		}
		if len(artifacts) != 1 {
			t.Fatalf("artifacts=%d, want 1", len(artifacts))
		}
	}

	runArtifactHydrationForSemanticLinking(context.Background(), nil, 200, []indexedArtifact{{ID: "200_inv_1"}}, inventoryItemIndexConfig, nil)

	if !called {
		t.Fatal("expected hydrateArtifactEmbeddingsFunc to be called")
	}
}
