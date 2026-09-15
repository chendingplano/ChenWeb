package agentservicehandler

import (
	"errors"
	"testing"
	"time"
)

func TestRunCapabilityRejectsTamperExpiryCrossRunAndDisallowedTool(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	signer, err := NewCapabilitySigner([]byte("0123456789abcdef0123456789abcdef"), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	claims := RunCapabilityClaims{
		UserID: "user-1", ProfileSlug: "knowledge-guide", ProfileVersion: "v1", RunID: "run-1",
		AllowedTools: []string{"search_knowledge"}, KnowledgeStoreIDs: []string{"store-1"},
		DocumentGroups: []string{"published"}, DocumentIDs: []string{"doc-1"},
		MaxEvidenceBytes: 65536,
	}
	token, err := signer.Mint(claims, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := signer.Verify(token, "run-1", "search_knowledge")
	if err != nil || verified.UserID != claims.UserID {
		t.Fatalf("Verify() = (%+v, %v)", verified, err)
	}

	replacement := byte('A')
	if token[len(token)-1] == replacement {
		replacement = 'B'
	}
	tampered := token[:len(token)-1] + string(replacement)
	if _, err := signer.Verify(tampered, "run-1", "search_knowledge"); !errors.Is(err, ErrCapabilityInvalid) {
		t.Fatalf("tampered error = %v", err)
	}
	if _, err := signer.Verify(token, "run-2", "search_knowledge"); !errors.Is(err, ErrCapabilityRunMismatch) {
		t.Fatalf("cross-run error = %v", err)
	}
	if _, err := signer.Verify(token, "run-1", "read_source_passages"); !errors.Is(err, ErrCapabilityToolDenied) {
		t.Fatalf("tool error = %v", err)
	}
	now = now.Add(time.Minute)
	if _, err := signer.Verify(token, "run-1", "search_knowledge"); !errors.Is(err, ErrCapabilityExpired) {
		t.Fatalf("expiry error = %v", err)
	}
}

func TestRunCapabilityRequiresBoundedLifetimeAndIdentity(t *testing.T) {
	signer, err := NewCapabilitySigner([]byte("0123456789abcdef0123456789abcdef"), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	base := RunCapabilityClaims{UserID: "u", ProfileSlug: "p", ProfileVersion: "v1", RunID: "r", AllowedTools: []string{"search_knowledge"}, KnowledgeStoreIDs: []string{"s"}, MaxEvidenceBytes: 65536}
	if _, err := signer.Mint(base, 0); !errors.Is(err, ErrCapabilityInvalid) {
		t.Fatalf("zero lifetime error = %v", err)
	}
	base.UserID = ""
	if _, err := signer.Mint(base, time.Minute); !errors.Is(err, ErrCapabilityInvalid) {
		t.Fatalf("missing identity error = %v", err)
	}
}
