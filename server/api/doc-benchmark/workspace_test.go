package docbenchmark

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type testOwnershipStore struct {
	own                                    Ownership
	lockErr, adapterErr, inputErr, markErr error
	states                                 []string
	callbackCalled                         bool
}

func (s *testOwnershipStore) SaveOwnership(o Ownership) error         { s.own = o; return nil }
func (s *testOwnershipStore) LoadOwnership(string) (Ownership, error) { return s.own, nil }
func (s *testOwnershipStore) LockOwnership(string) (Ownership, error) {
	if s.lockErr != nil {
		return Ownership{}, s.lockErr
	}
	return s.own, nil
}
func (s *testOwnershipStore) MarkVerified(_ string, hash string, size int64, m AllocationMarker) error {
	if s.markErr != nil {
		return s.markErr
	}
	s.own.Verified = true
	s.own.VerifiedHash = hash
	s.own.VerifiedSize = size
	s.own.VerifiedMarker = markerDigest(m)
	return nil
}
func (s *testOwnershipStore) MarkCleanupState(_ string, state string, cause error) error {
	s.states = append(s.states, state)
	if cause != nil {
		return cause
	}
	s.own.CleanupState = state
	return nil
}
func (s *testOwnershipStore) CleanupTransaction(_ context.Context, _ string, fn func() error) error {
	s.callbackCalled = true
	if s.adapterErr != nil {
		return s.adapterErr
	}
	return fn()
}
func testConfig(d string, st *testOwnershipStore) WorkspaceConfig {
	return WorkspaceConfig{WorkRoot: filepath.Join(d, "w"), EvidenceRoot: filepath.Join(d, "e"), AttemptID: "a", CaseID: "c", RunID: "r", Store: st}
}

func TestAllocateWorkspaceRejectsTraversalAndOverlap(t *testing.T) {
	root := t.TempDir()
	evidence := filepath.Join(root, "evidence")
	work := filepath.Join(root, "work")
	if _, err := AllocateWorkspace(WorkspaceConfig{WorkRoot: work, EvidenceRoot: evidence, AttemptID: "a-1", CaseID: "case", RunID: "run", Store: &testOwnershipStore{}}); err != nil {
		t.Fatal(err)
	}
	if _, err := AllocateWorkspace(WorkspaceConfig{WorkRoot: root, EvidenceRoot: filepath.Join(root, "e"), AttemptID: "../bad", CaseID: "c", RunID: "r", Store: &testOwnershipStore{}}); err == nil || !strings.Contains(err.Error(), "component") {
		t.Fatalf("expected unsafe id, got %v", err)
	}
	if _, err := AllocateWorkspace(WorkspaceConfig{WorkRoot: root, EvidenceRoot: filepath.Join(root, "sub"), AttemptID: "a", CaseID: "c", RunID: "r", Store: &testOwnershipStore{}}); err == nil {
		t.Fatal("expected overlapping roots rejection")
	}
}

func TestCaptureDurableAndCleanupNeverDeletesVerified(t *testing.T) {
	d := t.TempDir()
	st := &testOwnershipStore{}
	a, err := AllocateWorkspace(testConfig(d, st))
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

func TestCaptureFailurePointsLeaveRetryableEvidence(t *testing.T) {
	points := []CaptureFailurePoint{FailAfterPartialCreate, FailAfterCopy, FailAfterFileSync, FailAfterRename, FailAfterHash, FailAfterDirectorySync, FailBeforeVerifiedCommit}
	for _, p := range points {
		t.Run(string(p), func(t *testing.T) {
			d := t.TempDir()
			a, e := AllocateWorkspace(testConfig(d, &testOwnershipStore{}))
			if e != nil {
				t.Fatal(e)
			}
			_, _ = a.CaptureWithOptions(strings.NewReader("x"), "f", CaptureOptions{Failure: p})
			if p == FailAfterRename || p == FailAfterHash || p == FailAfterDirectorySync || p == FailBeforeVerifiedCommit {
				if _, e = a.Reverify("f"); e != nil {
					t.Fatal(e)
				}
			}
			if _, e = a.Capture(strings.NewReader("x"), "f"); e != nil {
				t.Fatal(e)
			}
		})
	}
}

func TestCleanupPersistsPendingAndRejectsVerifiedDiscard(t *testing.T) {
	d := t.TempDir()
	st := &testOwnershipStore{}
	a, err := AllocateWorkspace(testConfig(d, st))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.Capture(strings.NewReader("ok"), "f"); err != nil {
		t.Fatal(err)
	}
	if err = a.Cleanup(CleanupOptions{DiscardUnverified: true}); !errors.Is(err, ErrVerifiedImmutable) {
		t.Fatalf("discard verified: %v", err)
	}
	st2 := &testOwnershipStore{adapterErr: errors.New("db down")}
	a2, err := AllocateWorkspace(testConfig(filepath.Join(d, "x"), st2))
	if err != nil {
		t.Fatal(err)
	}
	if err = a2.Cleanup(CleanupOptions{DiscardUnverified: true}); err == nil || len(st2.states) == 0 || st2.states[0] != "db_pending" {
		t.Fatalf("pending state: err=%v states=%v", err, st2.states)
	}
}

func TestCleanupTransactionCallbackErrorDoesNotCommit(t *testing.T) {
	s := &testOwnershipStore{}
	called := false
	err := s.CleanupTransaction(context.Background(), "a", func() error { called = true; return errors.New("adapter failed") })
	if err == nil || !called {
		t.Fatalf("expected callback error, called=%v err=%v", called, err)
	}
	if s.own.Verified || len(s.states) != 0 {
		t.Fatalf("transaction state changed: %+v", s)
	}
}

func TestCaptureRejectsMarkerMismatchAndRootReplacement(t *testing.T) {
	d := t.TempDir()
	st := &testOwnershipStore{}
	a, err := AllocateWorkspace(testConfig(d, st))
	if err != nil {
		t.Fatal(err)
	}
	st.own.Verified = true
	st.own.VerifiedHash = "bad"
	st.own.VerifiedSize = 1
	st.own.VerifiedMarker = markerDigest(a.Marker)
	if _, err = a.Capture(strings.NewReader("x"), "f"); err != nil { /* initial capture is allowed */
	}
	if err = os.RemoveAll(a.Config.EvidenceRoot); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(d, a.Config.EvidenceRoot); err != nil {
		t.Fatal(err)
	}
	if _, err = a.Capture(strings.NewReader("x"), "g"); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("root replacement: %v", err)
	}
}
