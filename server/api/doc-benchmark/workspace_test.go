package docbenchmark

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAllocateWorkspaceRejectsTraversalAndOverlap(t *testing.T) {
	root := t.TempDir()
	evidence := filepath.Join(root, "evidence")
	work := filepath.Join(root, "work")
	if _, err := AllocateWorkspace(WorkspaceConfig{WorkRoot: work, EvidenceRoot: evidence, AttemptID: "a-1", CaseID: "case", RunID: "run"}); err != nil {
		t.Fatal(err)
	}
	if _, err := AllocateWorkspace(WorkspaceConfig{WorkRoot: root, EvidenceRoot: filepath.Join(root, "e"), AttemptID: "../bad", CaseID: "c", RunID: "r"}); err == nil || !strings.Contains(err.Error(), "component") {
		t.Fatalf("expected unsafe id, got %v", err)
	}
	if _, err := AllocateWorkspace(WorkspaceConfig{WorkRoot: root, EvidenceRoot: filepath.Join(root, "sub"), AttemptID: "a", CaseID: "c", RunID: "r"}); err == nil {
		t.Fatal("expected overlapping roots rejection")
	}
}

func TestCaptureDurableAndCleanupNeverDeletesVerified(t *testing.T) {
	d := t.TempDir()
	a, err := AllocateWorkspace(WorkspaceConfig{WorkRoot: filepath.Join(d, "w"), EvidenceRoot: filepath.Join(d, "e"), AttemptID: "a", CaseID: "c", RunID: "r"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := a.Capture(strings.NewReader("hello"), "actual.json")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Verified || got.SizeBytes != 5 || got.SHA256 == "" {
		t.Fatalf("bad artifact: %+v", got)
	}
	if err := a.Cleanup(CleanupOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(got.Path); err != nil {
		t.Fatalf("verified evidence removed: %v", err)
	}
}
