package assertions

import (
	"context"
	"testing"
)

func TestEvidenceStoreAddEvidenceRequiresAssertionID(t *testing.T) {
	store := EvidenceStore{DB: nil}
	_, err := store.AddEvidence(context.Background(), Evidence{
		ArtifactType: "metric",
		ArtifactID:   "m1",
	})
	if err == nil {
		t.Fatal("expected error when assertion_id is zero")
	}
}

func TestEvidenceStoreAddEvidenceRejectsBadRole(t *testing.T) {
	store := EvidenceStore{DB: nil}
	_, err := store.AddEvidence(context.Background(), Evidence{
		AssertionID:  1,
		ArtifactType: "metric",
		ArtifactID:   "m1",
		EvidenceRole: "maybe",
	})
	if err == nil {
		t.Fatal("expected error for invalid evidence_role")
	}
}

func TestEvidenceStoreAddEvidenceRequiresArtifactFields(t *testing.T) {
	store := EvidenceStore{DB: nil}
	_, err := store.AddEvidence(context.Background(), Evidence{AssertionID: 1})
	if err == nil {
		t.Fatal("expected error when artifact_type/artifact_id are missing")
	}
}
